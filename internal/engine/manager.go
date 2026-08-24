package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openlyfree/dappendble/internal/models"
)

const flushInterval = time.Second

type Manager struct {
	Tables     map[string]*Table
	BaseDir    string
	tablesMu   sync.RWMutex
	commitMu   sync.Mutex
	commitFile *os.File
	stopCh     chan struct{}
	flushDone  chan struct{}
	closeOnce  sync.Once
	closeErr   error
	closed     bool
	internKey  string
}

type refManager struct {
	m    *Manager
	refs int
}

var (
	managersMu sync.Mutex
	managers   = map[string]*refManager{}
)

func NewManager(baseDir string) (*Manager, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %v", err)
	}

	commitFile, err := os.OpenFile(filepath.Join(baseDir, commitsFileName), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	last, err := scanCommits(commitFile)
	if err != nil {
		commitFile.Close()
		return nil, err
	}

	tables := make(map[string]*Table)
	err = filepath.WalkDir(baseDir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(filepath.Base(path)) != ".dptb" {
			return nil
		}
		name := strings.Split(filepath.Base(path), ".")[0]
		committed, ok := last[name]
		if !ok {
			committed = 0
		}
		tbl, err := loadTable(path, committed)
		if err != nil {
			return err
		}
		tables[name] = tbl
		return nil
	})
	if err != nil {
		commitFile.Close()
		for _, t := range tables {
			t.close()
		}
		return nil, err
	}

	m := &Manager{
		Tables:     tables,
		BaseDir:    baseDir,
		commitFile: commitFile,
		stopCh:     make(chan struct{}),
		flushDone:  make(chan struct{}),
	}
	for _, t := range tables {
		t.mgr = m
	}
	go m.runFlusher()
	return m, nil
}

func Acquire(baseDir string) (*Manager, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	managersMu.Lock()
	if r, ok := managers[abs]; ok {
		r.refs++
		managersMu.Unlock()
		return r.m, nil
	}
	managersMu.Unlock()

	m, err := NewManager(abs)
	if err != nil {
		return nil, err
	}
	m.internKey = abs

	managersMu.Lock()
	if r, ok := managers[abs]; ok {
		managersMu.Unlock()
		_ = m.shutdown()
		r.refs++
		return r.m, nil
	}
	managers[abs] = &refManager{m: m, refs: 1}
	managersMu.Unlock()
	return m, nil
}

func (m *Manager) CreateTable(name string, schema *models.Schema) (*Table, error) {
	m.tablesMu.Lock()
	defer m.tablesMu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	if tbl, ok := m.Tables[name]; ok {
		return tbl, nil
	}

	path := filepath.Join(m.BaseDir, name+".dptb")
	tbl, err := NewTable(path, schema)
	if err != nil {
		return nil, err
	}
	tbl.mgr = m
	m.Tables[name] = tbl
	return tbl, nil
}

func (m *Manager) Table(name string) (*Table, bool) {
	m.tablesMu.RLock()
	defer m.tablesMu.RUnlock()
	t, ok := m.Tables[name]
	return t, ok
}

func (m *Manager) Begin() (*Tx, error) {
	m.tablesMu.RLock()
	defer m.tablesMu.RUnlock()
	if m.closed {
		return nil, ErrClosed
	}
	return &Tx{
		mgr:     m,
		overlay: make(map[coordKey][]byte),
	}, nil
}

func (m *Manager) commit(writes []mutation) error {
	m.commitMu.Lock()
	defer m.commitMu.Unlock()

	m.tablesMu.RLock()
	defer m.tablesMu.RUnlock()
	if m.closed {
		return ErrClosed
	}

	byTable := make(map[string][]mutation)
	for _, w := range writes {
		byTable[w.table] = append(byTable[w.table], w)
	}
	names := make([]string, 0, len(byTable))
	for n := range byTable {
		names = append(names, n)
	}
	sort.Strings(names)

	tables := make([]*Table, 0, len(names))
	for _, n := range names {
		t, ok := m.Tables[n]
		if !ok {
			return fmt.Errorf("table %s not found", n)
		}
		tables = append(tables, t)
	}
	for _, t := range tables {
		t.keeper.Lock()
	}
	defer func() {
		for i := len(tables) - 1; i >= 0; i-- {
			tables[i].keeper.Unlock()
		}
	}()

	type applied struct {
		t         *Table
		coord     Coordinate
		off       int64
		tombstone bool
	}
	oldEnds := make(map[*Table]int64, len(tables))
	var apps []applied

	for _, t := range tables {
		oldEnds[t] = t.end
		for _, w := range byTable[t.Name] {
			payload := w.payload
			if w.deleted {
				payload = nil
			}
			off, err := t.appendEntry(models.Entry{
				ColId:   w.col,
				RowId:   w.row,
				Payload: payload,
			})
			if err != nil {
				rollbackEnds(oldEnds)
				return err
			}
			apps = append(apps, applied{
				t:         t,
				coord:     Coordinate{ColId: w.col, RowId: w.row},
				off:       off,
				tombstone: w.deleted || len(payload) == 0,
			})
		}
	}

	offsets := make(map[string]int64, len(m.Tables))
	for name, t := range m.Tables {
		offsets[name] = t.end
	}
	if err := m.writeCommit(offsets); err != nil {
		rollbackEnds(oldEnds)
		return err
	}

	for _, a := range apps {
		if a.tombstone {
			delete(a.t.index, a.coord)
		} else {
			a.t.index[a.coord] = a.off
		}
	}
	return nil
}

func rollbackEnds(old map[*Table]int64) {
	for t, end := range old {
		_ = t.file.Truncate(end)
		t.end = end
	}
}

func (m *Manager) Sync() error {
	m.tablesMu.RLock()
	defer m.tablesMu.RUnlock()
	if m.closed {
		return ErrClosed
	}
	for _, t := range m.Tables {
		if err := t.sync(); err != nil {
			return err
		}
	}
	if m.commitFile != nil {
		return m.commitFile.Sync()
	}
	return nil
}

func (m *Manager) Close() error {
	if m.internKey != "" {
		return releaseIntern(m)
	}
	return m.shutdown()
}

func releaseIntern(m *Manager) error {
	managersMu.Lock()
	r, ok := managers[m.internKey]
	if !ok {
		managersMu.Unlock()
		return m.shutdown()
	}
	r.refs--
	if r.refs > 0 {
		managersMu.Unlock()
		return nil
	}
	delete(managers, m.internKey)
	managersMu.Unlock()
	return m.shutdown()
}

func (m *Manager) shutdown() error {
	m.closeOnce.Do(func() {
		m.closeErr = m.closeFiles()
	})
	return m.closeErr
}

func (m *Manager) closeFiles() error {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	<-m.flushDone

	m.tablesMu.Lock()
	defer m.tablesMu.Unlock()
	m.closed = true

	var err error
	for _, t := range m.Tables {
		if e := t.sync(); e != nil && err == nil {
			err = e
		}
		if e := t.close(); e != nil && err == nil {
			err = e
		}
	}
	if m.commitFile != nil {
		if e := m.commitFile.Sync(); e != nil && err == nil {
			err = e
		}
		if e := m.commitFile.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (m *Manager) runFlusher() {
	defer close(m.flushDone)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = m.Sync()
		case <-m.stopCh:
			return
		}
	}
}

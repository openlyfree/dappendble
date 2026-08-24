package engine

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/edsrzf/mmap-go"
	"github.com/openlyfree/dappendble/internal/models"
)

type Coordinate struct {
	ColId uint64
	RowId uint64
}

type Table struct {
	Name   string
	path   string
	file   *os.File
	index  map[Coordinate]int64
	schema *models.Schema
	keeper sync.RWMutex
	end    int64
	mgr    *Manager
}

func (t *Table) Schema() *models.Schema {
	return t.schema
}

// expects path to contain name and extension of file
func NewTable(path string, schema *models.Schema) (*Table, error) {
	dbmeta, err := os.OpenFile(path+".meta", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	defer func() {
		dbmeta.Sync()
		dbmeta.Close()
	}()

	schemeBytes := make([]byte, (*schema).Size())
	schema.MarshalBin(&schemeBytes)

	if _, err := dbmeta.Write(schemeBytes); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	return &Table{
		Name:   strings.Split(filepath.Base(path), ".")[0],
		path:   path,
		file:   f,
		index:  make(map[Coordinate]int64),
		schema: schema,
		keeper: sync.RWMutex{},
		end:    0,
	}, nil
}

func (t *Table) Add(columnId uint64, rowId uint64, data []byte) error {
	if t.mgr != nil {
		tx, err := t.mgr.Begin()
		if err != nil {
			return err
		}
		if err := tx.Add(t.Name, columnId, rowId, data); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	t.keeper.Lock()
	defer t.keeper.Unlock()
	return t.addLocked(columnId, rowId, data)
}

func (t *Table) addLocked(columnId uint64, rowId uint64, data []byte) error {
	m := models.Entry{
		ColId:   columnId,
		RowId:   rowId,
		Payload: data,
	}

	off, err := t.appendEntry(m)
	if err != nil {
		return err
	}

	coord := Coordinate{ColId: columnId, RowId: rowId}
	if len(data) == 0 {
		delete(t.index, coord)
	} else {
		t.index[coord] = off
	}
	return nil
}

func (t *Table) appendEntry(m models.Entry) (int64, error) {
	marshalArr := make([]byte, m.Size())
	m.MarshalBin(&marshalArr)

	off := t.end
	n, err := t.file.WriteAt(marshalArr, off)
	if err != nil {
		return 0, err
	}
	if n != len(marshalArr) {
		return 0, io.ErrShortWrite
	}
	t.end += int64(n)
	return off, nil
}

func LoadTable(path string) (*Table, error) {
	return loadTable(path, -1)
}

func loadTable(path string, committed int64) (*Table, error) {
	dbmeta, err := os.OpenFile(path+".meta", os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	schemaBytes, err := io.ReadAll(dbmeta)
	dbmeta.Close()
	if err != nil {
		return nil, err
	}
	var schema models.Schema
	schema.UnmarshalBin(&schemaBytes)

	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	end := st.Size()
	if committed >= 0 && committed < end {
		if err := f.Truncate(committed); err != nil {
			f.Close()
			return nil, err
		}
		end = committed
	}

	t := &Table{
		Name:   strings.Split(filepath.Base(path), ".")[0],
		path:   path,
		file:   f,
		index:  make(map[Coordinate]int64),
		schema: &schema,
		keeper: sync.RWMutex{},
		end:    end,
	}

	if end == 0 {
		return t, nil
	}

	m, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		f.Close()
		return nil, err
	}
	b := []byte(m)[:end]
	t.rebuildIndex(b)
	_ = m.Unmap()
	return t, nil
}

func (t *Table) rebuildIndex(b []byte) {
	off := int64(0)
	for len(b) >= 8 {
		entrySize := int64(binary.LittleEndian.Uint64(b))
		if entrySize < models.EntryMinSize || entrySize > int64(len(b)) {
			break
		}
		if crc32.ChecksumIEEE(b[:entrySize-4]) != binary.LittleEndian.Uint32(b[entrySize-4:entrySize]) {
			break
		}
		var en models.Entry
		en.UnmarshalBin(&b)
		coord := Coordinate{ColId: en.ColId, RowId: en.RowId}
		if len(en.Payload) == 0 {
			delete(t.index, coord)
		} else {
			t.index[coord] = off
		}
		off += entrySize
	}
}

func (t *Table) At(columnId uint64, rowId uint64) ([]byte, error) {
	t.keeper.RLock()
	defer t.keeper.RUnlock()

	if !t.checkValid(columnId, rowId) {
		return nil, os.ErrNotExist
	}

	dataOffset := t.index[Coordinate{ColId: columnId, RowId: rowId}]

	header := make([]byte, 8)
	if _, err := t.file.ReadAt(header, dataOffset); err != nil {
		return nil, err
	}

	size := binary.LittleEndian.Uint64(header)
	if size < models.EntryMinSize {
		return nil, ErrCorrupt
	}
	data := make([]byte, size-8)
	if _, err := t.file.ReadAt(data, dataOffset+8); err != nil {
		return nil, err
	}

	full := make([]byte, size)
	copy(full[:8], header)
	copy(full[8:], data)
	if !models.EntryCRCValid(full) {
		return nil, ErrCorrupt
	}

	var ent models.Entry
	ent.UnmarshalFullBin(&data)

	if len(ent.Payload) == 0 {
		return nil, os.ErrNotExist
	}

	return ent.Payload, nil
}

func (t *Table) Delete(columnId uint64, rowId uint64) error {
	if t.mgr != nil {
		tx, err := t.mgr.Begin()
		if err != nil {
			return err
		}
		if err := tx.Delete(t.Name, columnId, rowId); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	t.keeper.Lock()
	defer t.keeper.Unlock()
	if !t.checkValid(columnId, rowId) {
		return os.ErrNotExist
	}
	if err := t.addLocked(columnId, rowId, nil); err != nil {
		return err
	}
	return nil
}

func (t *Table) checkValid(columnId uint64, rowId uint64) bool {
	_, ok := t.index[Coordinate{ColId: columnId, RowId: rowId}]
	return ok
}

func (t *Table) sync() error {
	return t.file.Sync()
}

func (t *Table) close() error {
	return t.file.Close()
}

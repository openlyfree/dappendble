package engine

import (
	"bytes"
	"fmt"
	"os"
	"sync"
)

type coordKey struct {
	table string
	col   uint64
	row   uint64
}

type mutation struct {
	table   string
	col     uint64
	row     uint64
	payload []byte
	deleted bool
}

type Tx struct {
	mgr     *Manager
	mu      sync.Mutex
	overlay map[coordKey][]byte
	writes  []mutation
	done    bool
}

func (tx *Tx) Add(table string, col, row uint64, data []byte) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return ErrTxDone
	}
	if _, ok := tx.mgr.Table(table); !ok {
		return fmt.Errorf("table %s not found", table)
	}

	var p []byte
	if data != nil {
		p = bytes.Clone(data)
	}
	tx.overlay[coordKey{table, col, row}] = p
	tx.writes = append(tx.writes, mutation{
		table:   table,
		col:     col,
		row:     row,
		payload: p,
		deleted: len(p) == 0,
	})
	return nil
}

func (tx *Tx) Delete(table string, col, row uint64) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return ErrTxDone
	}
	ok, err := tx.existsLocked(table, col, row)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	tx.overlay[coordKey{table, col, row}] = nil
	tx.writes = append(tx.writes, mutation{
		table:   table,
		col:     col,
		row:     row,
		deleted: true,
	})
	return nil
}

func (tx *Tx) At(table string, col, row uint64) ([]byte, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return nil, ErrTxDone
	}
	if v, ok := tx.overlay[coordKey{table, col, row}]; ok {
		if v == nil {
			return nil, os.ErrNotExist
		}
		return bytes.Clone(v), nil
	}
	tbl, ok := tx.mgr.Table(table)
	if !ok {
		return nil, fmt.Errorf("table %s not found", table)
	}
	return tbl.At(col, row)
}

func (tx *Tx) existsLocked(table string, col, row uint64) (bool, error) {
	if v, ok := tx.overlay[coordKey{table, col, row}]; ok {
		return v != nil, nil
	}
	tbl, ok := tx.mgr.Table(table)
	if !ok {
		return false, fmt.Errorf("table %s not found", table)
	}
	_, err := tbl.At(col, row)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (tx *Tx) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return ErrTxDone
	}
	if len(tx.writes) == 0 {
		tx.done = true
		return nil
	}
	if err := tx.mgr.commit(tx.writes); err != nil {
		return err
	}
	tx.done = true
	tx.overlay = nil
	tx.writes = nil
	return nil
}

func (tx *Tx) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.done {
		return ErrTxDone
	}
	tx.done = true
	tx.overlay = nil
	tx.writes = nil
	return nil
}

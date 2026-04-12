package engine

import (
	"encoding/binary"
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
	file   *os.File
	index  map[Coordinate]int64
	schema *models.Schema
	keeper sync.RWMutex
}

// expects path to contain name and extension of file
func NewTable(path string, schema *models.Schema) (*Table, error) {
	//write schema to db meta file
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
	// open table file
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	return &Table{
		Name:   strings.Split(filepath.Base(path), ".")[0],
		file:   f,
		index:  make(map[Coordinate]int64),
		schema: schema,
		keeper: sync.RWMutex{},
	}, nil
}

func (t *Table) Add(columnId uint64, rowId uint64, data []byte) error {
	t.keeper.Lock()
	defer t.keeper.Unlock()

	m := models.Entry{
		ColId:   columnId,
		RowId:   rowId,
		Payload: data,
	}

	off, err := t.file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	marshalArr := make([]byte, m.Size())
	m.MarshalBin(&marshalArr)

	if _, err := t.file.Write(marshalArr); err != nil {
		return err
	}

	t.index[Coordinate{ColId: columnId, RowId: rowId}] = off

	return nil
}

func LoadTable(path string) (*Table, error) {

	// get schema from meta file
	dbmeta, err := os.OpenFile(path+".meta", os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer dbmeta.Close()

	schemaBytes, err := io.ReadAll(dbmeta)
	if err != nil {
		return nil, err
	}
	dbmeta.Close()
	var schema models.Schema
	schema.UnmarshalBin(&schemaBytes)

	// open table file
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	t := &Table{
		Name:   strings.Split(filepath.Base(path), ".")[0],
		file:   f,
		index:  make(map[Coordinate]int64),
		schema: &schema,
		keeper: sync.RWMutex{},
	}

	var en models.Entry
	m, _ := mmap.Map(f, mmap.RDWR, 0)
	b := []byte(m)
	defer m.Unmap()
	for off := int64(0); ; off += int64(en.Size()) {
		if off >= int64(len(b)) || len(b) < 8 {
			break
		}
		en.UnmarshalBin(&b)
		t.index[Coordinate{ColId: en.ColId, RowId: en.RowId}] = off
	}
	return t, nil
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
	data := make([]byte, size-8)
	if _, err := t.file.ReadAt(data, dataOffset+8); err != nil {
		return nil, err
	}

	var ent models.Entry
	ent.UnmarshalFullBin(&data)

	if ent.Payload == nil {
		return nil, os.ErrNotExist
	}

	return ent.Payload, nil
}

func (t *Table) Delete(columnId uint64, rowId uint64) error {
	if !t.checkValid(columnId, rowId) {
		return os.ErrNotExist
	}
	if err := t.Add(columnId, rowId, nil); err != nil {
		return err
	}
	delete(t.index, Coordinate{ColId: columnId, RowId: rowId})

	return nil
}

func (t *Table) checkValid(columnId uint64, rowId uint64) bool {
	_, ok := t.index[Coordinate{ColId: columnId, RowId: rowId}]
	return ok
}

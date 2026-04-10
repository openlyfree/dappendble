package engine

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/openlyfree/dappendble/internal/models"
	"google.golang.org/protobuf/proto"
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
	defer dbmeta.Close()
	schemaBytes, err := proto.Marshal(schema)
	if err != nil {
		return nil, err
	}
	if _, err := dbmeta.Write(schemaBytes); err != nil {
		return nil, err
	}
	dbmeta.Sync()
	dbmeta.Close()

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
		ColumnId: columnId,
		RowId:    rowId,
		Payload:  data,
	}

	bs, err := proto.Marshal(&m)
	if err != nil {
		return err
	}

	off, err := t.file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	size := uint32(len(bs))
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, size)

	if _, err := t.file.Write(header); err != nil {
		return err
	}
	if _, err := t.file.Write(bs); err != nil {
		return err
	}

	if t.index[Coordinate{ColId: columnId, RowId: rowId}] == 0 {
		t.index[Coordinate{ColId: columnId, RowId: rowId}] = off + 4
	}
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
	if err := proto.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, err
	}

	// open table file
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
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

	var off int64 = 0
	header := make([]byte, 4)
	for {

		if _, err := f.ReadAt(header, off); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		size := binary.LittleEndian.Uint32(header)

		data := make([]byte, size)
		if _, err := f.ReadAt(data, off+4); err != nil {
			return nil, err
		}

		var m models.Entry
		if err := proto.Unmarshal(data, &m); err != nil {
			return nil, err
		}

		if m.Payload == nil {
			delete(t.index, Coordinate{ColId: m.ColumnId, RowId: m.RowId})
		} else {
			t.index[Coordinate{ColId: m.ColumnId, RowId: m.RowId}] = off + 4
		}

		off += 4 + int64(size)
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

	header := make([]byte, 4)
	if _, err := t.file.ReadAt(header, dataOffset-4); err != nil {
		return nil, err
	}

	size := binary.LittleEndian.Uint32(header)
	data := make([]byte, size)
	if _, err := t.file.ReadAt(data, dataOffset); err != nil {
		return nil, err
	}

	var ent models.Entry
	if err := proto.Unmarshal(data, &ent); err != nil {
		return nil, err
	}

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

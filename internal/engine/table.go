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

// switch to flat array for index later
type Table struct {
	Name   string
	file   *os.File
	index  map[uint64]map[uint64]int64
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
		index:  make(map[uint64]map[uint64]int64),
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

	if t.index[columnId] == nil {
		t.index[columnId] = make(map[uint64]int64)
	}
	t.index[columnId][rowId] = off + 4
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
		index:  make(map[uint64]map[uint64]int64),
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
			if col, ok := t.index[m.ColumnId]; ok {
				delete(col, m.RowId)
				if len(col) == 0 {
					delete(t.index, m.ColumnId)
				}
			}
		} else {
			if t.index[m.ColumnId] == nil {
				t.index[m.ColumnId] = make(map[uint64]int64)
			}
			t.index[m.ColumnId][m.RowId] = off + 4
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

	dataOffset := t.index[columnId][rowId]

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
	if col, ok := t.index[columnId]; ok {
		delete(col, rowId)
		if len(col) == 0 {
			delete(t.index, columnId)
		}
	}

	return nil
}

func (t *Table) checkValid(columnId uint64, rowId uint64) bool {
	col, ok := t.index[columnId]
	if !ok {
		return false
	}
	_, ok = col[rowId]
	if !ok {
		return false
	}
	return true
}

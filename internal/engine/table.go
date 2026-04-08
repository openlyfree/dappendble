package engine

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/openlyfree/dappendble/internal/models"
	"google.golang.org/protobuf/proto"
)

type Table struct {
	Name   string
	file   *os.File
	index  map[uint64]map[uint64]int64
	keeper sync.RWMutex
}

func NewTable(name string, path string) (*Table, error) {
	realPath := filepath.Join(path, name+".tytb")

	f, err := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	return &Table{
		Name:   name,
		file:   f,
		index:  make(map[uint64]map[uint64]int64),
		keeper: sync.RWMutex{},
	}, nil
}

func (t *Table) FileAdd(columnId uint64, rowId uint64, data []byte) error {
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

func LoadTable(name string, path string) (*Table, error) {
	realPath := filepath.Join(path, name+".tytb")

	f, err := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	t := &Table{
		Name:   name,
		file:   f,
		index:  make(map[uint64]map[uint64]int64),
		keeper: sync.RWMutex{},
	}

	var off int64 = 0
	for {
		header := make([]byte, 4)
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
	if err := t.FileAdd(columnId, rowId, nil); err != nil {
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

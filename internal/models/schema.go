package models

import "encoding/binary"

type Schema struct {
	Columns []*Column
}

type Column struct {
	Id   uint64
	Name string
	Type ColumnType
}

type ColumnType int

const (
	COLUMN_TYPE_STRING ColumnType = iota
	COLUMN_TYPE_INT
	COLUMN_TYPE_FLOAT
	COLUMN_TYPE_BYTES
)

func (s *Schema) MarshalBin(b *[]byte) {
	off := 0
	binary.LittleEndian.PutUint64((*b)[off:off+8], uint64(len(s.Columns)))
	off += 8
	for _, v := range s.Columns {
		binary.LittleEndian.PutUint64((*b)[off:off+8], v.Id)
		binary.LittleEndian.PutUint64((*b)[off+8:off+16], uint64(v.Type))
		binary.LittleEndian.PutUint64((*b)[off+16:off+24], uint64(len(v.Name)))
		copy((*b)[off+24:off+24+len(v.Name)], v.Name)
		off += 24 + len(v.Name)
	}
}

func (s *Schema) Size() uint64 {
	size := uint64(8) // for number of columns
	for _, v := range s.Columns {
		size += 24 + uint64(len(v.Name)) // 8 for Id, 8 for Type, 8 for Name length + name bytes
	}
	return size
}

func (s *Schema) UnmarshalBin(b *[]byte) {
	numCols := binary.LittleEndian.Uint64((*b)[0:8])
	*b = (*b)[8:]

	s.Columns = make([]*Column, numCols)
	for i := range numCols {
		colId := binary.LittleEndian.Uint64((*b)[0:8])
		colType := ColumnType(binary.LittleEndian.Uint64((*b)[8:16]))
		nameLen := binary.LittleEndian.Uint64((*b)[16:24])
		name := string((*b)[24 : 24+nameLen])
		s.Columns[i] = &Column{Id: colId, Type: colType, Name: name}
		*b = (*b)[24+nameLen:]
	}
}

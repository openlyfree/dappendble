package models

import (
	"encoding/binary"
)

type Entry struct {
	ColId   uint64
	RowId   uint64
	Payload []byte
}

func (e *Entry) MarshalBin(b *[]byte) int {
	binary.LittleEndian.PutUint64((*b)[0:8], uint64(24+len(e.Payload)))
	binary.LittleEndian.PutUint64((*b)[8:16], e.RowId)
	binary.LittleEndian.PutUint64((*b)[16:24], e.ColId)
	copy((*b)[24:], e.Payload)
	return 24 + len(e.Payload)
}

func (e *Entry) UnmarshalBin(b *[]byte) {
	e.RowId = binary.LittleEndian.Uint64((*b)[8:16])
	e.ColId = binary.LittleEndian.Uint64((*b)[16:24])
	e.Payload = ((*b)[24:(binary.LittleEndian.Uint64(*b))])
	*b = (*b)[binary.LittleEndian.Uint64(*b):]
}

func (e *Entry) Size() uint64 {
	return uint64(24 + len(e.Payload))
}

func (e *Entry) UnmarshalFullBin(b *[]byte) {
	e.RowId = binary.LittleEndian.Uint64((*b)[0:8])
	e.ColId = binary.LittleEndian.Uint64((*b)[8:16])
	e.Payload = ((*b)[16:])
}

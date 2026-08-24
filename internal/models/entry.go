package models

import (
	"encoding/binary"
	"hash/crc32"
)

const (
	// EntryMinSize is size(u64) + row(u64) + col(u64) + crc32(u32).
	EntryMinSize = 28
)

type Entry struct {
	ColId   uint64
	RowId   uint64
	Payload []byte
}

func (e *Entry) MarshalBin(b *[]byte) int {
	n := int(e.Size())
	binary.LittleEndian.PutUint64((*b)[0:8], uint64(n))
	binary.LittleEndian.PutUint64((*b)[8:16], e.RowId)
	binary.LittleEndian.PutUint64((*b)[16:24], e.ColId)
	copy((*b)[24:24+len(e.Payload)], e.Payload)
	crc := crc32.ChecksumIEEE((*b)[:24+len(e.Payload)])
	binary.LittleEndian.PutUint32((*b)[24+len(e.Payload):n], crc)
	return n
}

func (e *Entry) UnmarshalBin(b *[]byte) {
	size := binary.LittleEndian.Uint64(*b)
	e.RowId = binary.LittleEndian.Uint64((*b)[8:16])
	e.ColId = binary.LittleEndian.Uint64((*b)[16:24])
	payloadLen := int(size) - EntryMinSize
	e.Payload = (*b)[24 : 24+payloadLen]
	*b = (*b)[size:]
}

func (e *Entry) Size() uint64 {
	return uint64(EntryMinSize + len(e.Payload))
}

func (e *Entry) UnmarshalFullBin(b *[]byte) {
	raw := *b
	e.RowId = binary.LittleEndian.Uint64(raw[0:8])
	e.ColId = binary.LittleEndian.Uint64(raw[8:16])
	e.Payload = raw[16 : len(raw)-4]
}

func EntryCRCValid(rec []byte) bool {
	if len(rec) < EntryMinSize {
		return false
	}
	size := binary.LittleEndian.Uint64(rec)
	if size < EntryMinSize || int(size) != len(rec) {
		return false
	}
	got := binary.LittleEndian.Uint32(rec[size-4:])
	return crc32.ChecksumIEEE(rec[:size-4]) == got
}

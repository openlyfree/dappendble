package engine

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"sort"
)

const (
	commitsFileName = "commits"
	commitMinSize   = 16 // size(u64) + ntables(u32) + crc32(u32)
)

func encodeCommit(offsets map[string]int64) []byte {
	names := make([]string, 0, len(offsets))
	for n := range offsets {
		names = append(names, n)
	}
	sort.Strings(names)

	payload := 4 // ntables
	for _, n := range names {
		payload += 2 + len(n) + 8
	}
	size := 8 + payload + 4
	rec := make([]byte, size)
	binary.LittleEndian.PutUint64(rec[0:8], uint64(size))
	binary.LittleEndian.PutUint32(rec[8:12], uint32(len(names)))
	off := 12
	for _, n := range names {
		binary.LittleEndian.PutUint16(rec[off:off+2], uint16(len(n)))
		off += 2
		copy(rec[off:], n)
		off += len(n)
		binary.LittleEndian.PutUint64(rec[off:off+8], uint64(offsets[n]))
		off += 8
	}
	crc := crc32.ChecksumIEEE(rec[:size-4])
	binary.LittleEndian.PutUint32(rec[size-4:], crc)
	return rec
}

func parseCommit(rec []byte) (map[string]int64, bool) {
	if len(rec) < commitMinSize {
		return nil, false
	}
	size := binary.LittleEndian.Uint64(rec)
	if int(size) != len(rec) || size < commitMinSize {
		return nil, false
	}
	got := binary.LittleEndian.Uint32(rec[size-4:])
	if crc32.ChecksumIEEE(rec[:size-4]) != got {
		return nil, false
	}
	ntables := binary.LittleEndian.Uint32(rec[8:12])
	off := 12
	out := make(map[string]int64, ntables)
	limit := int(size) - 4
	for i := uint32(0); i < ntables; i++ {
		if off+2 > limit {
			return nil, false
		}
		nameLen := int(binary.LittleEndian.Uint16(rec[off : off+2]))
		off += 2
		if off+nameLen+8 > limit {
			return nil, false
		}
		name := string(rec[off : off+nameLen])
		off += nameLen
		endOff := int64(binary.LittleEndian.Uint64(rec[off : off+8]))
		off += 8
		out[name] = endOff
	}
	if off != limit {
		return nil, false
	}
	return out, true
}

func scanCommits(f *os.File) (map[string]int64, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var last map[string]int64
	valid := 0
	rest := b
	for len(rest) >= 8 {
		size := int(binary.LittleEndian.Uint64(rest))
		if size < commitMinSize || size > len(rest) {
			break
		}
		parsed, ok := parseCommit(rest[:size])
		if !ok {
			break
		}
		last = parsed
		valid += size
		rest = rest[size:]
	}

	if int64(valid) != int64(len(b)) {
		if err := f.Truncate(int64(valid)); err != nil {
			return nil, err
		}
	}
	if _, err := f.Seek(int64(valid), io.SeekStart); err != nil {
		return nil, err
	}
	if last == nil {
		last = map[string]int64{}
	}
	return last, nil
}

func (m *Manager) writeCommit(offsets map[string]int64) error {
	rec := encodeCommit(offsets)
	n, err := m.commitFile.Write(rec)
	if err != nil {
		return err
	}
	if n != len(rec) {
		return io.ErrShortWrite
	}
	return nil
}

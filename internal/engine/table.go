package engine

import (
	"os"
	"sync"
)

type Table struct {
	Name     string
	File     *os.File
	FileName string
	Index    map[uint64]map[uint64]int64
	Keeper   sync.RWMutex
}

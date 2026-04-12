package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/openlyfree/dappendble/internal/models"
)

type Manager struct {
	Tables  map[string]*Table
	BaseDir string
}

func NewManager(baseDir string) (*Manager, error) {
	dbPath := baseDir

	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %v", err)
	}
	tables := make(map[string]*Table)

	err := filepath.WalkDir(baseDir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(filepath.Base(path)) == ".dptb" {
			tables[strings.Split(filepath.Base(path), ".")[0]], err = LoadTable(path)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &Manager{
		Tables:  tables,
		BaseDir: baseDir,
	}, nil
}

func (m *Manager) CreateTable(name string, schema *models.Schema) (*Table, error) {
	path := filepath.Join(m.BaseDir, name+".dptb")
	tbl, err := NewTable(path, schema)
	if err != nil {
		return nil, err
	}

	m.Tables[name] = tbl
	return tbl, nil
}

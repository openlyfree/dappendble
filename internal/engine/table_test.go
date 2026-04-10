package engine

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/openlyfree/dappendble/internal/models"
)

func TestTable_Concurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrency.db")
	schema := &models.Schema{
		Columns: []*models.Column{{Id: 1, Type: models.ColumnType_COLUMN_TYPE_STRING}},
	}
	tbl, _ := NewTable(path, schema)

	numWorkers := 10
	iterations := 100
	var wg sync.WaitGroup

	// Start parallel writers
	for w := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := range iterations {
				rowID := uint64(workerID*iterations + i)
				data := fmt.Appendf(nil, "data-%d-%d", workerID, i)

				if err := tbl.Add(1, rowID, data); err != nil {
					t.Errorf("Worker %d failed to add: %v", workerID, err)
				}
			}
		}(w)
	}

	// Start parallel readers
	wg.Go(func() {
		for i := 0; i < iterations*numWorkers; i++ {
			// Randomly attempt to read row 0 during the process
			_, _ = tbl.At(1, 0)
		}
	})

	wg.Wait()

	// Verify all data exists
	for w := range numWorkers {
		for i := range iterations {
			rowID := uint64(w*iterations + i)
			expected := fmt.Sprintf("data-%d-%d", w, i)

			got, err := tbl.At(1, rowID)
			if err != nil || string(got) != expected {
				t.Errorf("Data loss at row %d. Expected %s, got %s", rowID, expected, got)
			}
		}
	}
}

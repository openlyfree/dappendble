package engine

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openlyfree/dappendble/internal/models"
)

func TestTable_Concurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrency.db")
	schema := &models.Schema{
		Columns: []*models.Column{{Id: 1, Type: models.COLUMN_TYPE_STRING, Name: "col1"}},
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

func BenchmarkTableAdd(b *testing.B) {
	path := filepath.Join(b.TempDir(), "bench_add.db")
	schema := &models.Schema{
		Columns: []*models.Column{{Id: 1, Type: models.COLUMN_TYPE_BYTES, Name: "col1"}},
	}

	tbl, err := NewTable(path, schema)
	if err != nil {
		b.Fatal(err)
	}

	data := []byte("benchmark-payload")
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	

	for i := 0; b.Loop(); i++ {
		if err := tbl.Add(1, uint64(i), data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTableAt(b *testing.B) {
	path := filepath.Join(b.TempDir(), "bench_at.db")
	schema := &models.Schema{
		Columns: []*models.Column{{Id: 1, Type: models.COLUMN_TYPE_BYTES, Name: "col1"}},
	}

	tbl, err := NewTable(path, schema)
	if err != nil {
		b.Fatal(err)
	}

	data := []byte("benchmark-payload")
	const count = 10000
	for i := range count {
		if err := tbl.Add(1, uint64(i), data); err != nil {
			b.Fatal(err)
		}
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	

	for i := 0; b.Loop(); i++ {
		idx := uint64(i % count)
		if _, err := tbl.At(1, idx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTableAddOpsPerSec(b *testing.B) {
    path := filepath.Join(b.TempDir(), "bench_add_ops.db")
    schema := &models.Schema{
        Columns: []*models.Column{{Id: 1, Type: models.COLUMN_TYPE_BYTES, Name: "col1"}},
    }

    tbl, err := NewTable(path, schema)
    if err != nil {
        b.Fatal(err)
    }

    data := []byte("benchmark-payload")
    b.SetBytes(int64(len(data)))
    b.ReportAllocs()

    b.ResetTimer()
    start := time.Now()
    for i := 0; i < b.N; i++ {
        if err := tbl.Add(1, uint64(i), data); err != nil {
            b.Fatal(err)
        }
    }
    elapsed := time.Since(start)

    opsPerSec := float64(b.N) / elapsed.Seconds()
    b.ReportMetric(opsPerSec, "ops/s")
}
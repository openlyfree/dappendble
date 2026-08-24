package engine

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/openlyfree/dappendble/internal/models"
)

func testSchema() *models.Schema {
	return &models.Schema{
		Columns: []*models.Column{{Id: 1, Type: models.COLUMN_TYPE_STRING, Name: "col1"}},
	}
}

func TestTx_CommitVisibleAndReload(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateTable("t", testSchema()); err != nil {
		t.Fatal(err)
	}

	tx, err := m.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Add("t", 1, 1, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	tbl, ok := m.Table("t")
	if !ok {
		t.Fatal("missing table")
	}
	got, err := tbl.At(1, 1)
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q err %v", got, err)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	tbl2, ok := m2.Table("t")
	if !ok {
		t.Fatal("missing table after reload")
	}
	got, err = tbl2.At(1, 1)
	if err != nil || string(got) != "hello" {
		t.Fatalf("reload got %q err %v", got, err)
	}
}

func TestTx_RollbackInvisible(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.CreateTable("t", testSchema()); err != nil {
		t.Fatal(err)
	}

	tx, err := m.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Add("t", 1, 1, []byte("nope")); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.At("t", 1, 1); err != nil {
		t.Fatal("tx should see its own write")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	tbl, _ := m.Table("t")
	if _, err := tbl.At(1, 1); !os.IsNotExist(err) {
		t.Fatalf("rollback should hide write, err=%v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	tbl2, _ := m2.Table("t")
	if _, err := tbl2.At(1, 1); !os.IsNotExist(err) {
		t.Fatalf("reload after rollback should miss row, err=%v", err)
	}
}

func TestTx_DirtyReadsHidden(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.CreateTable("t", testSchema()); err != nil {
		t.Fatal(err)
	}

	tx, err := m.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Add("t", 1, 7, []byte("secret")); err != nil {
		t.Fatal(err)
	}

	tbl, _ := m.Table("t")
	if _, err := tbl.At(1, 7); !os.IsNotExist(err) {
		t.Fatalf("other readers must not see uncommitted write, err=%v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := tbl.At(1, 7); !os.IsNotExist(err) {
			t.Errorf("concurrent reader saw uncommitted write: %v", err)
		}
	}()
	wg.Wait()

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := tbl.At(1, 7)
	if err != nil || string(got) != "secret" {
		t.Fatalf("after commit got %q err %v", got, err)
	}
}

func TestRecovery_TruncatesUncommittedTail(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateTable("t", testSchema()); err != nil {
		t.Fatal(err)
	}
	tx, err := m.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Add("t", 1, 1, []byte("keep")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	tail := models.Entry{ColId: 1, RowId: 2, Payload: []byte("uncommitted")}
	buf := make([]byte, tail.Size())
	tail.MarshalBin(&buf)
	f, err := os.OpenFile(filepath.Join(dir, "t.dptb"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	f.Close()

	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	tbl, _ := m2.Table("t")
	got, err := tbl.At(1, 1)
	if err != nil || string(got) != "keep" {
		t.Fatalf("committed row missing: %q %v", got, err)
	}
	if _, err := tbl.At(1, 2); !os.IsNotExist(err) {
		t.Fatalf("uncommitted tail should be truncated, err=%v", err)
	}
}

func TestTx_MultiTableAtomicRecovery(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateTable("a", testSchema()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateTable("b", testSchema()); err != nil {
		t.Fatal(err)
	}

	tx, err := m.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Add("a", 1, 1, []byte("A")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Add("b", 1, 1, []byte("B")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	tail := models.Entry{ColId: 1, RowId: 9, Payload: []byte("tail")}
	buf := make([]byte, tail.Size())
	tail.MarshalBin(&buf)
	for _, name := range []string{"a.dptb", "b.dptb"} {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(buf); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	m2, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	for _, name := range []string{"a", "b"} {
		tbl, ok := m2.Table(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if _, err := tbl.At(1, 9); !os.IsNotExist(err) {
			t.Fatalf("%s uncommitted tail survived: %v", name, err)
		}
		got, err := tbl.At(1, 1)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"a": "A", "b": "B"}
		if string(got) != want[name] {
			t.Fatalf("%s committed value = %q, want %q", name, got, want[name])
		}
	}
}

func TestTx_DeleteRollback(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.CreateTable("t", testSchema()); err != nil {
		t.Fatal(err)
	}
	tx, _ := m.Begin()
	_ = tx.Add("t", 1, 1, []byte("stay"))
	_ = tx.Commit()

	tx, _ = m.Begin()
	if err := tx.Delete("t", 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.At("t", 1, 1); !os.IsNotExist(err) {
		t.Fatal("delete should hide row inside tx")
	}
	tbl, _ := m.Table("t")
	got, err := tbl.At(1, 1)
	if err != nil || string(got) != "stay" {
		t.Fatalf("other readers still see row before commit, got %q err %v", got, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	got, err = tbl.At(1, 1)
	if err != nil || string(got) != "stay" {
		t.Fatalf("rollback restore failed: %q %v", got, err)
	}
}

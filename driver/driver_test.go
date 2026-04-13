package driver

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)

func TestDriver_FullCycle(t *testing.T) {
	db, err := sql.Open("dappendble", "./test_data")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("./test_data.dpdb")

	_, err = db.Exec("CREATE TABLE users (id INT, name STRING)")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err = db.Exec("INSERT INTO users VALUES (?, ?, ?)", 1, 42, "funky")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	var name string
	err = db.QueryRow("SELECT name FROM users WHERE id = ?", 42).Scan(&name)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if name != "funky" {
		t.Errorf("Expected funky, got %s", name)
	}
}

func TestDriver_PreparedStatement(t *testing.T) {
	db, _ := sql.Open("dappendble", "./test_stmt")
	defer os.RemoveAll("./test_stmt.dpdb")
	db.Exec("CREATE TABLE bench (id INT, val STRING)")

	stmt, err := db.Prepare("INSERT INTO bench VALUES (?, ?, ?)")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()

	for i := range 100 {
		_, err := stmt.Exec(0, i, fmt.Sprintf("val-%d", i))
		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}
	}
}

func TestDriver_MultiTable(t *testing.T) {
	db, _ := sql.Open("dappendble", "./test_multi")
	defer os.RemoveAll("./test_multi.dpdb")

	db.Exec("CREATE TABLE logs (id INT, msg STRING)")
	db.Exec("CREATE TABLE auth (id INT, token STRING)")

	db.Exec("INSERT INTO logs VALUES (?, ?, ?)", 1, 1, "System Boot")
	db.Exec("INSERT INTO auth VALUES (?, ?, ?)", 1, 1, "SECRET_TOKEN")

	var msg, token string
	db.QueryRow("SELECT msg FROM logs WHERE id = 1").Scan(&msg)
	db.QueryRow("SELECT token FROM auth WHERE id = 1").Scan(&token)

	if msg == "" || token == "" || msg == token {
		t.Error("Table isolation failed or data cross-contamination")
	}
}

func TestDriver_Placeholders(t *testing.T) {
	db, _ := sql.Open("dappendble", "./test_place")
	defer os.RemoveAll("./test_place.dpdb")
	db.Exec("CREATE TABLE data (id INT, content STRING)")
	_, err := db.Exec("INSERT INTO data VALUES (?, ?, ?)", 0, 500, "Standard Placeholder")
	if err != nil {
		t.Errorf("Placeholder failed: %v", err)
	}
}

func BenchmarkDriver_Insert(b *testing.B) {
	db, _ := sql.Open("dappendble", "./bench_sql")
	defer os.RemoveAll("./bench_sql.dpdb")
	db.Exec("CREATE TABLE bench (id INT, val STRING)")

	stmt, _ := db.Prepare("INSERT INTO bench VALUES (?, ?, ?)")

	for i := 0; b.Loop(); i++ {
		stmt.Exec(0, i, "some_data")
	}
}

func BenchmarkDappendble_OpsPerSec(b *testing.B) {
	dbPath := "./bench_ops"
	defer os.RemoveAll(dbPath + ".dpdb")

	db, err := sql.Open("dappendble", dbPath)
	if err != nil {
		b.Fatal(err)
	}

	_, err = db.Exec("CREATE TABLE bench (id INT, data STRING)")
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}

	b.Run("ParallelWrite", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			stmt, _ := db.Prepare("INSERT INTO bench VALUES (?, ?, ?)")
			defer stmt.Close()

			i := 0
			for pb.Next() {
				_, err := stmt.Exec(0, i, "fast_payload")
				if err != nil {
					b.Error(err)
					return
				}
				i++
			}
		})
	})

	for i := range 1000 {
		_, _ = db.Exec("INSERT INTO bench VALUES (?, ?, ?)", 0, i, "read_me")
	}

	b.Run("ParallelRead", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			stmt, _ := db.Prepare("SELECT data FROM bench WHERE id = ?")
			defer stmt.Close()

			i := 0
			for pb.Next() {
				var res string
				_ = stmt.QueryRow(i % 1000).Scan(&res)
				i++
			}
		})
	})
}

func BenchmarkSQLite_Insert(b *testing.B) {
	dbPath := "./bench_sqlite.db"
	os.Remove(dbPath)
	db, _ := sql.Open("sqlite3", dbPath)
	defer os.Remove(dbPath)
	db.Exec("CREATE TABLE bench (id INT, val STRING)")

	stmt, _ := db.Prepare("INSERT INTO bench VALUES (?, ?)")

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		stmt.Exec(i, "some_data")
	}
}

func BenchmarkSQLite_OpsPerSec(b *testing.B) {
	dbPath := "./bench_sqlite_ops.db"
	os.RemoveAll(dbPath)
	os.RemoveAll(dbPath + "-wal")
	os.RemoveAll(dbPath + "-shm")

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		db.Close()
		os.RemoveAll(dbPath)
		os.RemoveAll(dbPath + "-wal")
		os.RemoveAll(dbPath + "-shm")
	}()

	_, err = db.Exec("CREATE TABLE bench (id INT, data STRING)")
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}

	b.Run("ParallelWrite", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			stmt, _ := db.Prepare("INSERT INTO bench VALUES (?, ?)")
			defer stmt.Close()

			i := 0
			for pb.Next() {
				_, err := stmt.Exec(i, "fast_payload")
				if err != nil {
					b.Error(err)
					return
				}
				i++
			}
		})
	})

	for i := range 1000 {
		_, _ = db.Exec("INSERT INTO bench VALUES (?, ?)", i, "read_me")
	}

	b.Run("ParallelRead", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			stmt, _ := db.Prepare("SELECT data FROM bench WHERE id = ?")
			defer stmt.Close()

			i := 0
			for pb.Next() {
				var res string
				_ = stmt.QueryRow(i % 1000).Scan(&res)
				i++
			}
		})
	})
}

func BenchmarkPureGoSQLite_Insert(b *testing.B) {
	dbPath := "./bench_pure_sqlite.db"
	os.Remove(dbPath)
	db, _ := sql.Open("sqlite", dbPath)
	defer os.Remove(dbPath)
	db.Exec("CREATE TABLE bench (id INT, val STRING)")

	stmt, _ := db.Prepare("INSERT INTO bench VALUES (?, ?)")

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		stmt.Exec(i, "some_data")
	}
}

func BenchmarkPureGoSQLite_OpsPerSec(b *testing.B) {
	dbPath := "./bench_pure_sqlite_ops.db"
	os.RemoveAll(dbPath)
	os.RemoveAll(dbPath + "-wal")
	os.RemoveAll(dbPath + "-shm")

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		db.Close()
		os.RemoveAll(dbPath)
		os.RemoveAll(dbPath + "-wal")
		os.RemoveAll(dbPath + "-shm")
	}()

	_, err = db.Exec("CREATE TABLE bench (id INT, data STRING)")
	if err != nil {
		b.Fatalf("Failed to create table: %v", err)
	}

	b.Run("ParallelWrite", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			stmt, _ := db.Prepare("INSERT INTO bench VALUES (?, ?)")
			defer stmt.Close()

			i := 0
			for pb.Next() {
				_, err := stmt.Exec(i, "fast_payload")
				if err != nil {
					b.Error(err)
					return
				}
				i++
			}
		})
	})

	for i := range 1000 {
		_, _ = db.Exec("INSERT INTO bench VALUES (?, ?)", i, "read_me")
	}

	b.Run("ParallelRead", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			stmt, _ := db.Prepare("SELECT data FROM bench WHERE id = ?")
			defer stmt.Close()

			i := 0
			for pb.Next() {
				var res string
				_ = stmt.QueryRow(i % 1000).Scan(&res)
				i++
			}
		})
	})
}

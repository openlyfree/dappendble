# dappendble

**dappendble** is an experimental embedded rdbms (if it can be considered that) written in go. It's pretty fast. I think.
I KNOW I KNOW ITS ONLY BECAUSE THE INDEX IS IN MEMORY

## How To Use
- Please don't.
- But if you do, there's a database/sql compatible driver!
- It's at ```github.com/openlyfree/dappendble/driver```
- If anything data gets lost: not my fault

## Structure

*   **`driver/`**: The database/sql compatible driver
*   **`internal/engine/`**: The trenches (tables, managers).
*   **`internal/models/`**: Data models (entries, schemas).

## Development

## Benchmarks
My tests compare the native `dappendble` engine against both **CGO-based SQLite** (`github.com/mattn/go-sqlite3`) and **Pure Go SQLite** (`modernc.org/sqlite`):

```bash
go test -bench . ./driver -benchmem
```

Output on my pc (Its gonna vary but yeah something like this):
```text

# dappendble
BenchmarkDriver_Insert-8                      274047     4397 ns/op    399 B/op   7 allocs/op
BenchmarkDappendble_OpsPerSec/ParallelWrite-8 296131     3711 ns/op    395 B/op   7 allocs/op
BenchmarkDappendble_OpsPerSec/ParallelRead-8  804580     3033 ns/op   1121 B/op  21 allocs/op

# mattn/go-sqlite3 (CGO)
BenchmarkSQLite_Insert-8                       48657    24327 ns/op    174 B/op   6 allocs/op
BenchmarkSQLite_OpsPerSec/ParallelWrite-8        679   810740 ns/op    198 B/op   6 allocs/op
BenchmarkSQLite_OpsPerSec/ParallelRead-8       13166     7122 ns/op    571 B/op  20 allocs/op

# modernc.org/sqlite (Pure Go)
BenchmarkPureGoSQLite_Insert-8                      39  2961515 ns/op    160 B/op   6 allocs/op
BenchmarkPureGoSQLite_OpsPerSec/ParallelWrite-8     33  4086063 ns/op   1045 B/op  14 allocs/op
BenchmarkPureGoSQLite_OpsPerSec/ParallelRead-8   11226    11422 ns/op    524 B/op  18 allocs/op
```
On an Intel Ultra 5 226V 16GB RAM NVME SSD Arch linux

## What's Next?

- io_uring backend would be cool
- PRIORITY sectioned writers for real concurrent writes 
- PRIORITY fix the in memory index's limitations on database size
- PRIORITY mmap for reading instead of only for loading
- rewrite in another language like rust or zig, probably never gonna happen tho
- more apis for other languages cus i want more than just gophers
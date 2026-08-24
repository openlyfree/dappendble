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
GOAMD64=v3 CGO_ENABLED=1 \
CGO_CFLAGS="-O3 -march=native -mtune=native -flto -fomit-frame-pointer -DNDEBUG" \
CGO_LDFLAGS="-O3 -flto -march=native" \
go test -a -bench . ./driver -benchmem
```

Output on my pc (Its gonna vary but yeah something like this):
```text

# dappendble
BenchmarkDriver_Insert-8                       115251    13646 ns/op   1214 B/op  16 allocs/op
BenchmarkDappendble_OpsPerSec/ParallelWrite-8   86427    17749 ns/op   1131 B/op  15 allocs/op
BenchmarkDappendble_OpsPerSec/ParallelRead-8   276878     4098 ns/op   1171 B/op  25 allocs/op

# mattn/go-sqlite3 (CGO)
BenchmarkSQLite_Insert-8                        41067    28453 ns/op    175 B/op   6 allocs/op
BenchmarkSQLite_OpsPerSec/ParallelWrite-8       20386    50879 ns/op    175 B/op   6 allocs/op
BenchmarkSQLite_OpsPerSec/ParallelRead-8       157615     6463 ns/op    571 B/op  20 allocs/op

# modernc.org/sqlite (Pure Go)
BenchmarkPureGoSQLite_Insert-8                    381  3135812 ns/op    165 B/op   6 allocs/op
BenchmarkPureGoSQLite_OpsPerSec/ParallelWrite-8   649  1749767 ns/op    194 B/op   7 allocs/op
BenchmarkPureGoSQLite_OpsPerSec/ParallelRead-8  84788    13638 ns/op    513 B/op  18 allocs/op
```
On an Intel Ultra 5 226V 16GB RAM NVME SSD Arch linux

## What's Next?

- io_uring backend would be cool
- PRIORITY sectioned writers for real concurrent writes 
- PRIORITY fix the in memory index's limitations on database size
- PRIORITY mmap for reading instead of only for loading
- rewrite in another language like rust or zig, probably never gonna happen tho
- more apis for other languages cus i want more than just gophers
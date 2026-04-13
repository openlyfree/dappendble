package driver

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/ajitpratap0/GoSQLX/pkg/gosqlx"
	"github.com/ajitpratap0/GoSQLX/pkg/sql/ast"
	"github.com/openlyfree/dappendble/internal/engine"
	"github.com/openlyfree/dappendble/internal/models"
)

type DappendDriver struct{}

func (d *DappendDriver) Open(name string) (driver.Conn, error) {
	dbPath := name
	if !strings.HasSuffix(dbPath, ".dpdb") {
		dbPath += ".dpdb"
	}
	_ = os.MkdirAll(dbPath, 0755)

	m, err := engine.NewManager(dbPath)
	if err != nil {
		return nil, err
	}
	return &conn{
		mgr:   m,
		cache: make(map[string]*ast.AST),
	}, nil
}

func init() {
	sql.Register("dappendble", &DappendDriver{})
}


type conn struct {
	mgr   *engine.Manager
	cache map[string]*ast.AST
	mu    sync.RWMutex
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	c.mu.RLock()
	if astObj, ok := c.cache[query]; ok {
		c.mu.RUnlock()
		return &stmt{conn: c, query: query, ast: astObj}, nil
	}
	c.mu.RUnlock()

	originalQuery := query
	paramCount := 1
	for strings.Contains(query, "?") {
		query = strings.Replace(query, "?", fmt.Sprintf("$%d", paramCount), 1)
		paramCount++
	}

	astObj, err := gosqlx.Parse(query)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if len(c.cache) < 1000 { // cache size limit at 1000
		c.cache[originalQuery] = astObj
	}
	c.mu.Unlock()

	return &stmt{conn: c, query: query, ast: astObj}, nil
}

func (c *conn) Close() error              { return nil }
func (c *conn) Begin() (driver.Tx, error) { return &tx{}, nil }


type stmt struct {
	conn  *conn
	query string
	ast   *ast.AST
}

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	if len(s.ast.Statements) == 0 {
		return nil, io.EOF
	}

	switch st := s.ast.Statements[0].(type) {
	case *ast.CreateTableStatement:
		schema := &models.Schema{}
		for i, col := range st.Columns {
			schema.Columns = append(schema.Columns, &models.Column{
				Id:   uint64(i),
				Name: col.Name,
				Type: models.COLUMN_TYPE_STRING,
			})
		}
		_, err := s.conn.mgr.CreateTable(st.Name, schema)
		return driver.RowsAffected(0), err

	case *ast.InsertStatement:
		tbl, ok := s.conn.mgr.Tables[st.TableName]
		if !ok {
			return nil, fmt.Errorf("table %s not found", st.TableName)
		}
		colID := uint64(args[0].(int64))
		rowID := uint64(args[1].(int64))
		data := fmt.Appendf(nil, "%v", args[2])

		err := tbl.Add(colID, rowID, data)
		return driver.RowsAffected(1), err
	}
	return nil, fmt.Errorf("unsupported exec statement")
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	var tableName string
	var whereExpr ast.Expression
	var columns []ast.Expression

	switch st := s.ast.Statements[0].(type) {
	case *ast.SelectStatement:
		tableName = st.TableName
		whereExpr = st.Where
		columns = st.Columns
	default:
		return nil, fmt.Errorf("unsupported query statement")
	}

	tbl, ok := s.conn.mgr.Tables[tableName]
	if !ok {
		return nil, fmt.Errorf("table %s not found", tableName)
	}

	rowID, err := s.getRowIdFromWhere(whereExpr, args)
	if err != nil {
		return nil, err
	}

	var cols []string
	var colIDs []uint64

	for _, expr := range columns {
		ident, ok := expr.(*ast.Identifier)
		if !ok {
			return nil, fmt.Errorf("expected identifier in select columns, got %T", expr)
		}

		if ident.Name == "*" {
			for _, col := range tbl.Schema().Columns {
				cols = append(cols, col.Name)
				colIDs = append(colIDs, col.Id)
			}
		} else {
			found := false
			for _, col := range tbl.Schema().Columns {
				if col.Name == ident.Name {
					cols = append(cols, col.Name)
					colIDs = append(colIDs, col.Id)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("column %s not found in table %s", ident.Name, tableName)
			}
		}
	}

	var values [][]byte
	for _, colID := range colIDs {
		data, err := tbl.At(colID, rowID)
		if err != nil {
			if os.IsNotExist(err) {
				values = append(values, nil)
			} else {
				return nil, err
			}
		} else {
			values = append(values, data)
		}
	}

	return &rows{cols: cols, values: values}, nil
}

func (s *stmt) getRowIdFromWhere(where ast.Expression, args []driver.Value) (uint64, error) {
	be, ok := where.(*ast.BinaryExpression)
	if !ok {
		return 0, fmt.Errorf("expected WHERE id = X")
	}

	var val any
	if lit, ok := be.Right.(*ast.LiteralValue); ok {
		if lit.Type == "placeholder" {
			idx, _ := strconv.Atoi(strings.TrimPrefix(lit.Value.(string), "$"))
			val = args[idx-1]
		} else {
			val = lit.Value
		}
	}

	switch v := val.(type) {
	case int64:
		return uint64(v), nil
	case string:
		return strconv.ParseUint(v, 10, 64)
	default:
		return 0, fmt.Errorf("invalid id type")
	}
}

func (s *stmt) Close() error  { return nil }
func (s *stmt) NumInput() int { return -1 }

type rows struct {
	cols   []string
	values [][]byte
	done   bool
}

func (r *rows) Columns() []string { return r.cols }
func (r *rows) Close() error      { return nil }
func (r *rows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	for i, v := range r.values {
		dest[i] = v
	}
	r.done = true
	return nil
}

type tx struct{}

func (t *tx) Commit() error   { return nil }
func (t *tx) Rollback() error { return nil }

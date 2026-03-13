package mssql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type explainDriverState struct {
	execs       []string
	execConnIDs []int64
	querySQL    string
	queryConnID int64
	columns     []string
	rows        [][]driver.Value
	nextConnID  uint64
}

type explainDriver struct {
	state *explainDriverState
}

func (d *explainDriver) Open(name string) (driver.Conn, error) {
	connID := int64(atomic.AddUint64(&d.state.nextConnID, 1))
	return &explainConn{state: d.state, id: connID}, nil
}

type explainConn struct {
	state           *explainDriverState
	id              int64
	showplanEnabled bool
}

func (c *explainConn) Prepare(query string) (driver.Stmt, error) { return nil, assert.AnError }
func (c *explainConn) Close() error                              { return nil }
func (c *explainConn) Begin() (driver.Tx, error)                 { return nil, assert.AnError }

func (c *explainConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.execs = append(c.state.execs, query)
	c.state.execConnIDs = append(c.state.execConnIDs, c.id)
	switch query {
	case "SET SHOWPLAN_TEXT ON":
		c.showplanEnabled = true
	case "SET SHOWPLAN_TEXT OFF":
		c.showplanEnabled = false
	}
	return explainResult(0), nil
}

func (c *explainConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !c.showplanEnabled {
		return nil, fmt.Errorf("query executed without showplan enabled on conn %d", c.id)
	}
	c.state.querySQL = query
	c.state.queryConnID = c.id
	return &explainRows{columns: c.state.columns, rows: c.state.rows}, nil
}

type explainResult int64

func (r explainResult) LastInsertId() (int64, error) { return 0, nil }
func (r explainResult) RowsAffected() (int64, error) { return int64(r), nil }

type explainRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *explainRows) Columns() []string { return r.columns }
func (r *explainRows) Close() error      { return nil }
func (r *explainRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

var explainDriverCounter uint64

func newExplainTestDB(t *testing.T, state *explainDriverState) *sql.DB {
	t.Helper()

	driverName := fmt.Sprintf("mssql-explain-test-%d", atomic.AddUint64(&explainDriverCounter, 1))
	sql.Register(driverName, &explainDriver{state: state})

	db, err := sql.Open(driverName, "")
	require.NoError(t, err)
	db.SetMaxIdleConns(0)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestQueryExecutor_ExplainQuery_UsesShowplanWithoutExecuting(t *testing.T) {
	state := &explainDriverState{
		columns: []string{"StmtText"},
		rows: [][]driver.Value{
			{"  |--Clustered Index Scan(OBJECT:([dbo].[events]))"},
		},
	}
	executor := &QueryExecutor{db: newExplainTestDB(t, state)}

	result, err := executor.ExplainQuery(context.Background(), "SELECT * FROM events")
	require.NoError(t, err)

	assert.Equal(t, []string{"SET SHOWPLAN_TEXT ON", "SET SHOWPLAN_TEXT OFF"}, state.execs)
	assert.Equal(t, []int64{1, 1}, state.execConnIDs)
	assert.Equal(t, "SELECT * FROM events", state.querySQL)
	assert.Equal(t, int64(1), state.queryConnID)
	assert.Contains(t, result.Plan, "SQL Server Execution Plan:")
	assert.Contains(t, result.Plan, "Clustered Index Scan")
	assert.NotEmpty(t, result.PerformanceHints)
}

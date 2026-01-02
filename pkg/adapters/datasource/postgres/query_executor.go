//go:build postgres || all_adapters

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// QueryExecutor provides PostgreSQL query execution.
type QueryExecutor struct {
	pool         *pgxpool.Pool
	connMgr      *datasource.ConnectionManager
	projectID    uuid.UUID
	userID       string
	datasourceID uuid.UUID
	ownedPool    bool // true if we created the pool (for tests or direct instantiation)
}

// NewQueryExecutor creates a PostgreSQL query executor using the connection manager.
// If connMgr is nil, creates an unmanaged pool (for tests or direct instantiation).
func NewQueryExecutor(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*QueryExecutor, error) {
	connStr := buildConnectionString(cfg)

	if connMgr == nil {
		// Fallback for direct instantiation (tests)
		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}

		return &QueryExecutor{
			pool:      pool,
			ownedPool: true,
		}, nil
	}

	// Use connection manager for reusable pool
	pool, err := connMgr.GetOrCreatePool(ctx, projectID, userID, datasourceID, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get pooled connection: %w", err)
	}

	return &QueryExecutor{
		pool:         pool,
		connMgr:      connMgr,
		projectID:    projectID,
		userID:       userID,
		datasourceID: datasourceID,
		ownedPool:    false,
	}, nil
}

// ExecuteQuery runs a SQL query and returns the results.
func (e *QueryExecutor) ExecuteQuery(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
	// Apply limit if specified
	queryToRun := sqlQuery
	if limit > 0 {
		queryToRun = fmt.Sprintf("SELECT * FROM (%s) AS _limited LIMIT %d", sqlQuery, limit)
	}

	rows, err := e.pool.Query(ctx, queryToRun)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Get column names and types
	fieldDescs := rows.FieldDescriptions()
	columns := make([]datasource.ColumnInfo, len(fieldDescs))
	for i, fd := range fieldDescs {
		columns[i] = datasource.ColumnInfo{
			Name: string(fd.Name),
			Type: pgTypeNameFromOID(fd.DataTypeOID),
		}
	}

	// Collect rows
	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("failed to read row values: %w", err)
		}

		rowMap := make(map[string]any)
		for i, col := range columns {
			rowMap[col.Name] = values[i]
		}
		resultRows = append(resultRows, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &datasource.QueryExecutionResult{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: len(resultRows),
	}, nil
}

// ExecuteQueryWithParams runs a parameterized SQL query with positional parameters.
// The SQL should use $1, $2, etc. for parameter placeholders.
// pgx handles parameterized queries natively, preventing SQL injection.
func (e *QueryExecutor) ExecuteQueryWithParams(ctx context.Context, sqlQuery string, params []any, limit int) (*datasource.QueryExecutionResult, error) {
	// Apply limit if specified
	queryToRun := sqlQuery
	if limit > 0 {
		queryToRun = fmt.Sprintf("SELECT * FROM (%s) AS _limited LIMIT %d", sqlQuery, limit)
	}

	// Execute with parameters - pgx handles parameterized queries natively
	rows, err := e.pool.Query(ctx, queryToRun, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute parameterized query: %w", err)
	}
	defer rows.Close()

	// Get column names and types
	fieldDescs := rows.FieldDescriptions()
	columns := make([]datasource.ColumnInfo, len(fieldDescs))
	for i, fd := range fieldDescs {
		columns[i] = datasource.ColumnInfo{
			Name: string(fd.Name),
			Type: pgTypeNameFromOID(fd.DataTypeOID),
		}
	}

	// Collect rows
	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("failed to read row values: %w", err)
		}

		rowMap := make(map[string]any)
		for i, col := range columns {
			rowMap[col.Name] = values[i]
		}
		resultRows = append(resultRows, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &datasource.QueryExecutionResult{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: len(resultRows),
	}, nil
}

// Execute runs any SQL statement (DDL/DML) and returns results.
func (e *QueryExecutor) Execute(ctx context.Context, sqlStatement string) (*datasource.ExecuteResult, error) {
	rows, err := e.pool.Query(ctx, sqlStatement)
	if err != nil {
		return nil, fmt.Errorf("failed to execute statement: %w", err)
	}
	defer rows.Close()

	result := &datasource.ExecuteResult{}

	// Check if the statement returns rows (SELECT, INSERT/UPDATE/DELETE with RETURNING)
	fieldDescs := rows.FieldDescriptions()
	if len(fieldDescs) > 0 {
		// Statement returns rows - collect them
		result.Columns = make([]string, len(fieldDescs))
		for i, fd := range fieldDescs {
			result.Columns[i] = string(fd.Name)
		}

		result.Rows = make([]map[string]any, 0)
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				return nil, fmt.Errorf("failed to read row values: %w", err)
			}

			rowMap := make(map[string]any)
			for i, col := range result.Columns {
				rowMap[col] = values[i]
			}
			result.Rows = append(result.Rows, rowMap)
		}
		result.RowCount = len(result.Rows)
	} else {
		// For DDL/DML without RETURNING, we must still consume the result
		// to trigger execution and populate errors/CommandTag.
		// pgx defers execution until rows are consumed.
		for rows.Next() {
			// No rows expected, but iteration triggers execution
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during execution: %w", err)
	}

	// Get rows affected from command tag
	cmdTag := rows.CommandTag()
	result.RowsAffected = cmdTag.RowsAffected()

	return result, nil
}

// ValidateQuery checks if a SQL query is syntactically valid without executing it.
func (e *QueryExecutor) ValidateQuery(ctx context.Context, sqlQuery string) error {
	// Use EXPLAIN to validate without executing
	_, err := e.pool.Exec(ctx, "EXPLAIN "+sqlQuery)
	if err != nil {
		return fmt.Errorf("invalid SQL: %w", err)
	}
	return nil
}

// Close releases the adapter (but NOT the pool if managed).
func (e *QueryExecutor) Close() error {
	if e.ownedPool && e.pool != nil {
		e.pool.Close()
	}
	// If using connection manager, don't close the pool - it's managed by TTL
	return nil
}

// QuoteIdentifier safely quotes a SQL identifier to prevent SQL injection.
// Uses PostgreSQL's standard double-quote quoting.
func (e *QueryExecutor) QuoteIdentifier(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// pgTypeNameFromOID maps PostgreSQL type OIDs to human-readable type names.
// This covers the most common types; unknown types return "UNKNOWN".
func pgTypeNameFromOID(oid uint32) string {
	switch oid {
	case 16:
		return "BOOL"
	case 17:
		return "BYTEA"
	case 18:
		return "CHAR"
	case 20:
		return "INT8"
	case 21:
		return "INT2"
	case 23:
		return "INT4"
	case 25:
		return "TEXT"
	case 26:
		return "OID"
	case 114:
		return "JSON"
	case 142:
		return "XML"
	case 700:
		return "FLOAT4"
	case 701:
		return "FLOAT8"
	case 790:
		return "MONEY"
	case 1042:
		return "BPCHAR"
	case 1043:
		return "VARCHAR"
	case 1082:
		return "DATE"
	case 1083:
		return "TIME"
	case 1114:
		return "TIMESTAMP"
	case 1184:
		return "TIMESTAMPTZ"
	case 1186:
		return "INTERVAL"
	case 1266:
		return "TIMETZ"
	case 1700:
		return "NUMERIC"
	case 2950:
		return "UUID"
	case 3802:
		return "JSONB"
	// Array types
	case 1000:
		return "BOOL[]"
	case 1005:
		return "INT2[]"
	case 1007:
		return "INT4[]"
	case 1016:
		return "INT8[]"
	case 1009:
		return "TEXT[]"
	case 1015:
		return "VARCHAR[]"
	case 1021:
		return "FLOAT4[]"
	case 1022:
		return "FLOAT8[]"
	case 2951:
		return "UUID[]"
	case 3807:
		return "JSONB[]"
	default:
		return "UNKNOWN"
	}
}

// Ensure QueryExecutor implements datasource.QueryExecutor at compile time.
var _ datasource.QueryExecutor = (*QueryExecutor)(nil)

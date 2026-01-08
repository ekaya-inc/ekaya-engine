//go:build postgres || all_adapters

package postgres

import (
	"context"
	"fmt"
	"strings"

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

// ExplainQuery returns EXPLAIN ANALYZE output for a SQL query with performance insights.
func (e *QueryExecutor) ExplainQuery(ctx context.Context, sqlQuery string) (*datasource.ExplainResult, error) {
	// Use EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) to get detailed execution plan
	explainSQL := "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) " + sqlQuery
	rows, err := e.pool.Query(ctx, explainSQL)
	if err != nil {
		return nil, fmt.Errorf("EXPLAIN ANALYZE failed: %w", err)
	}
	defer rows.Close()

	// Collect all plan lines
	var planLines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, fmt.Errorf("failed to scan EXPLAIN output: %w", err)
		}
		planLines = append(planLines, line)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading EXPLAIN output: %w", err)
	}

	// Parse the plan to extract timing and generate hints
	result := &datasource.ExplainResult{
		Plan: strings.Join(planLines, "\n"),
	}

	// Extract execution and planning times from the plan
	var executionTime, planningTime float64
	for _, line := range planLines {
		if strings.Contains(line, "Execution Time:") {
			fmt.Sscanf(line, " Execution Time: %f ms", &executionTime)
		} else if strings.Contains(line, "Planning Time:") {
			fmt.Sscanf(line, " Planning Time: %f ms", &planningTime)
		}
	}
	result.ExecutionTimeMs = executionTime
	result.PlanningTimeMs = planningTime

	// Generate performance hints based on plan analysis
	result.PerformanceHints = generatePerformanceHints(planLines, executionTime)

	return result, nil
}

// generatePerformanceHints analyzes the EXPLAIN plan and provides optimization suggestions.
func generatePerformanceHints(planLines []string, executionTimeMs float64) []string {
	var hints []string
	planText := strings.Join(planLines, "\n")

	// Check for sequential scans on large tables
	if strings.Contains(planText, "Seq Scan") {
		hints = append(hints, "Sequential scan detected - consider adding an index if this table is large")
	}

	// Check for missing indexes in joins
	if strings.Contains(planText, "Hash Join") && strings.Contains(planText, "Seq Scan") {
		hints = append(hints, "Hash join with sequential scan - an index on join columns may improve performance")
	}

	// Check for nested loop joins (can be slow with large datasets)
	if strings.Contains(planText, "Nested Loop") {
		hints = append(hints, "Nested loop join detected - ensure join columns are indexed for better performance")
	}

	// Check for sorts that spill to disk
	if strings.Contains(planText, "external merge") || strings.Contains(planText, "Sort Method: external") {
		hints = append(hints, "Sort operation spilled to disk - consider increasing work_mem or reducing result set")
	}

	// Check for bitmap heap scans (often indicates partial index usage)
	if strings.Contains(planText, "Bitmap Heap Scan") {
		hints = append(hints, "Bitmap heap scan detected - query may benefit from more selective conditions or better index coverage")
	}

	// Check for high buffer usage
	if strings.Contains(planText, "Buffers: shared read=") {
		hints = append(hints, "High buffer usage detected - query is reading significant data from disk/memory")
	}

	// Check for slow execution time
	if executionTimeMs > 1000 {
		hints = append(hints, fmt.Sprintf("Query execution took %.2f ms - consider optimization if this is a frequent query", executionTimeMs))
	} else if executionTimeMs > 100 {
		hints = append(hints, "Query execution is moderately slow - review plan for optimization opportunities")
	}

	// If no specific hints, provide a positive message
	if len(hints) == 0 {
		hints = append(hints, "Query plan looks efficient - no obvious optimization opportunities detected")
	}

	return hints
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

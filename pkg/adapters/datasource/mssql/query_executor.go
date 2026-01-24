package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// QueryExecutor provides SQL Server query execution.
type QueryExecutor struct {
	config *Config
	db     *sql.DB
}

// NewQueryExecutor creates a SQL Server query executor.
// Uses connection manager for connection pooling.
func NewQueryExecutor(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*QueryExecutor, error) {
	// Extract Azure token from context for user_delegation before validation
	if cfg.AuthMethod == "user_delegation" {
		if err := extractAndSetAzureToken(ctx, cfg); err != nil {
			return nil, err
		}
	}

	// Validate config - token is now set for user_delegation
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Use the same connection logic as Adapter
	adapter, err := NewAdapter(ctx, cfg, connMgr, projectID, datasourceID, userID)
	if err != nil {
		return nil, err
	}

	return &QueryExecutor{
		config: cfg,
		db:     adapter.DB(),
	}, nil
}

// Query runs a SELECT statement and returns bounded results.
// See datasource.QueryExecutor.Query for limit behavior.
func (e *QueryExecutor) Query(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
	// Apply limit - always wrap query with bounded limit using SQL Server's TOP clause
	effectiveLimit := limit
	if effectiveLimit <= 0 || effectiveLimit > datasource.MaxQueryLimit {
		effectiveLimit = datasource.MaxQueryLimit
	}
	queryToRun := fmt.Sprintf("SELECT TOP (%d) * FROM (%s) AS _limited", effectiveLimit, sqlQuery)

	rows, err := e.db.QueryContext(ctx, queryToRun)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Get column names
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Get column types for proper scanning
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("failed to get column types: %w", err)
	}

	// Convert to ColumnInfo slice
	columns := make([]datasource.ColumnInfo, len(columnNames))
	for i, colName := range columnNames {
		columns[i] = datasource.ColumnInfo{
			Name: colName,
			Type: mapSQLServerType(columnTypes[i].DatabaseTypeName()),
		}
	}

	// Collect rows
	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		// Create slice of interface{} to hold values
		values := make([]any, len(columnNames))
		valuePtrs := make([]any, len(columnNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row into value pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		rowMap := make(map[string]any)
		for i, col := range columnNames {
			val := values[i]

			// Handle SQL Server specific types
			if val != nil {
				// Convert []byte to string for text columns
				if b, ok := val.([]byte); ok {
					colType := columnTypes[i].DatabaseTypeName()
					// Convert CHAR, VARCHAR, NCHAR, NVARCHAR, TEXT to string
					if isStringType(colType) {
						val = string(b)
					}
				}
			}

			rowMap[col] = val
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

// QueryWithParams runs a parameterized SELECT with bounded results.
// The SQL should use $1, $2, etc. for parameter placeholders (PostgreSQL style).
// These are converted to SQL Server's @p1, @p2, etc. named parameters.
// See datasource.QueryExecutor.Query for limit behavior.
func (e *QueryExecutor) QueryWithParams(ctx context.Context, sqlQuery string, params []any, limit int) (*datasource.QueryExecutionResult, error) {
	// Convert PostgreSQL-style positional parameters ($1, $2, ...) to SQL Server named parameters (@p1, @p2, ...)
	convertedQuery := convertPostgreSQLParamsToMSSQL(sqlQuery)

	// Apply limit - always wrap query with bounded limit using SQL Server's TOP clause
	effectiveLimit := limit
	if effectiveLimit <= 0 || effectiveLimit > datasource.MaxQueryLimit {
		effectiveLimit = datasource.MaxQueryLimit
	}
	queryToRun := fmt.Sprintf("SELECT TOP (%d) * FROM (%s) AS _limited", effectiveLimit, convertedQuery)

	// Build named parameters for SQL Server
	namedParams := make([]any, len(params))
	for i, param := range params {
		namedParams[i] = sql.Named(fmt.Sprintf("p%d", i+1), param)
	}

	rows, err := e.db.QueryContext(ctx, queryToRun, namedParams...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute parameterized query: %w", err)
	}
	defer rows.Close()

	// Get column names
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Get column types for proper scanning
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("failed to get column types: %w", err)
	}

	// Convert to ColumnInfo slice
	columns := make([]datasource.ColumnInfo, len(columnNames))
	for i, colName := range columnNames {
		columns[i] = datasource.ColumnInfo{
			Name: colName,
			Type: mapSQLServerType(columnTypes[i].DatabaseTypeName()),
		}
	}

	// Collect rows
	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		// Create slice of interface{} to hold values
		values := make([]any, len(columnNames))
		valuePtrs := make([]any, len(columnNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row into value pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		rowMap := make(map[string]any)
		for i, col := range columnNames {
			val := values[i]

			// Handle SQL Server specific types
			if val != nil {
				// Convert []byte to string for text columns
				if b, ok := val.([]byte); ok {
					colType := columnTypes[i].DatabaseTypeName()
					// Convert CHAR, VARCHAR, NCHAR, NVARCHAR, TEXT to string
					if isStringType(colType) {
						val = string(b)
					}
				}
			}

			rowMap[col] = val
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
// For statements with OUTPUT clauses, returns rows in the result.
// For INSERT/UPDATE/DELETE without OUTPUT, returns RowsAffected.
func (e *QueryExecutor) Execute(ctx context.Context, sqlStatement string) (*datasource.ExecuteResult, error) {
	result := &datasource.ExecuteResult{}

	// Try QueryContext first to check if statement returns rows
	rows, err := e.db.QueryContext(ctx, sqlStatement)
	if err != nil {
		// QueryContext failed - likely a DML statement without OUTPUT
		// Fall back to ExecContext to get rows affected
		execResult, execErr := e.db.ExecContext(ctx, sqlStatement)
		if execErr != nil {
			return nil, fmt.Errorf("failed to execute statement: %w", execErr)
		}

		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to get rows affected: %w", err)
		}

		result.RowsAffected = rowsAffected
		result.RowCount = 0
		return result, nil
	}
	defer rows.Close()

	// Check if the statement returns rows by checking column types
	columnTypes, err := rows.ColumnTypes()
	if err != nil || len(columnTypes) == 0 {
		// No columns - use ExecContext instead
		rows.Close()
		execResult, execErr := e.db.ExecContext(ctx, sqlStatement)
		if execErr != nil {
			return nil, fmt.Errorf("failed to execute statement: %w", execErr)
		}

		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to get rows affected: %w", err)
		}

		result.RowsAffected = rowsAffected
		result.RowCount = 0
		return result, nil
	}

	// Statement returns rows - collect them
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	result.Columns = columnNames
	result.Rows = make([]map[string]any, 0)

	for rows.Next() {
		// Create slice of interface{} to hold values
		values := make([]any, len(columnNames))
		valuePtrs := make([]any, len(columnNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row into value pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		rowMap := make(map[string]any)
		for i, col := range columnNames {
			val := values[i]

			// Handle SQL Server specific types
			if val != nil {
				// Convert []byte to string for text columns
				if b, ok := val.([]byte); ok {
					colType := columnTypes[i].DatabaseTypeName()
					if isStringType(colType) {
						val = string(b)
					}
				}
			}

			rowMap[col] = val
		}
		result.Rows = append(result.Rows, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	result.RowCount = len(result.Rows)
	return result, nil
}

// QuoteIdentifier safely quotes a SQL identifier to prevent SQL injection.
// Uses SQL Server's square bracket syntax: [name]
func (e *QueryExecutor) QuoteIdentifier(name string) string {
	return quoteName(name)
}

// convertPostgreSQLParamsToMSSQL converts PostgreSQL-style positional parameters
// ($1, $2, ...) to SQL Server named parameters (@p1, @p2, ...)
func convertPostgreSQLParamsToMSSQL(query string) string {
	// Match $ followed by one or more digits
	re := regexp.MustCompile(`\$(\d+)`)
	return re.ReplaceAllStringFunc(query, func(match string) string {
		// Extract the number
		numStr := match[1:] // Skip the $
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return match // Return unchanged if parsing fails
		}
		return fmt.Sprintf("@p%d", num)
	})
}

// ValidateQuery checks if a SQL query is syntactically valid without executing it.
func (e *QueryExecutor) ValidateQuery(ctx context.Context, sqlQuery string) error {
	// Use SET FMTONLY ON to validate syntax without executing
	// This is a legacy approach but works well for validation
	// Modern alternative would be to use sp_describe_first_result_set

	// Try to prepare the statement - this validates syntax
	stmt, err := e.db.PrepareContext(ctx, sqlQuery)
	if err != nil {
		return fmt.Errorf("invalid SQL: %w", err)
	}
	defer stmt.Close()

	return nil
}

// ExplainQuery returns execution plan output for a SQL query with performance insights.
// Uses SQL Server's SET SHOWPLAN_TEXT ON to get execution plan without executing the query.
func (e *QueryExecutor) ExplainQuery(ctx context.Context, sqlQuery string) (*datasource.ExplainResult, error) {
	// Use SHOWPLAN_TEXT which shows plan without executing
	// This is simpler and more reliable than STATISTICS PROFILE
	_, err := e.db.ExecContext(ctx, "SET SHOWPLAN_TEXT ON")
	if err != nil {
		return nil, fmt.Errorf("failed to enable showplan: %w", err)
	}
	defer e.db.ExecContext(ctx, "SET SHOWPLAN_TEXT OFF")

	// Execute the query - this will return the plan, not the results
	rows, err := e.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("EXPLAIN query failed: %w", err)
	}
	defer rows.Close()

	// Collect plan lines
	var planLines []string
	for rows.Next() {
		var stmtText string
		if err := rows.Scan(&stmtText); err != nil {
			// If single column scan fails, try getting all columns
			columns, _ := rows.Columns()
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err == nil {
				// StmtText is usually the first column
				if len(values) > 0 {
					if stmt, ok := values[0].(string); ok && stmt != "" {
						planLines = append(planLines, stmt)
					} else if stmt, ok := values[0].([]byte); ok && len(stmt) > 0 {
						planLines = append(planLines, string(stmt))
					}
				}
			}
			continue
		}
		if stmtText != "" {
			planLines = append(planLines, stmtText)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading execution plan: %w", err)
	}

	// Build the result
	result := &datasource.ExplainResult{
		Plan:            "",
		ExecutionTimeMs: 0, // SHOWPLAN_TEXT doesn't provide timing
		PlanningTimeMs:  0,
	}

	if len(planLines) > 0 {
		result.Plan = "SQL Server Execution Plan:\n" + strings.Join(planLines, "\n")
	} else {
		result.Plan = "Execution plan not available. Query syntax may be invalid."
	}

	// Generate performance hints
	result.PerformanceHints = generateMSSQLPerformanceHints(planLines, 0)

	return result, nil
}

// generateMSSQLPerformanceHints analyzes the execution plan and provides optimization suggestions.
func generateMSSQLPerformanceHints(planLines []string, executionTimeMs float64) []string {
	var hints []string
	planText := ""
	if len(planLines) > 0 {
		for _, line := range planLines {
			planText += line + " "
		}
	}

	// Check for table scans (case-insensitive)
	if containsIgnoreCase(planText, "Table Scan") || containsIgnoreCase(planText, "Clustered Index Scan") {
		hints = append(hints, "Table scan detected - consider adding an index if this table is large")
	}

	// Check for missing indexes
	if containsIgnoreCase(planText, "Missing Index") {
		hints = append(hints, "SQL Server suggests a missing index - review the execution plan for index recommendations")
	}

	// Check for nested loop joins
	if containsIgnoreCase(planText, "Nested Loops") {
		hints = append(hints, "Nested loop join detected - ensure join columns are indexed for better performance")
	}

	// Check for hash joins
	if containsIgnoreCase(planText, "Hash Match") {
		hints = append(hints, "Hash join detected - an index on join columns may improve performance")
	}

	// Check for sorts
	if containsIgnoreCase(planText, "Sort") {
		hints = append(hints, "Sort operation detected - consider adding an index to avoid sorting")
	}

	// Check for high cost operations
	if executionTimeMs > 1000 {
		hints = append(hints, fmt.Sprintf("Query execution cost is high (%.2f ms) - consider optimization if this is a frequent query", executionTimeMs))
	} else if executionTimeMs > 100 {
		hints = append(hints, "Query execution cost is moderate - review plan for optimization opportunities")
	}

	// If no specific hints, provide a positive message
	if len(hints) == 0 {
		hints = append(hints, "Query plan looks efficient - no obvious optimization opportunities detected")
	}

	return hints
}

// containsIgnoreCase performs case-insensitive string contains check
func containsIgnoreCase(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	sLower := make([]byte, len(s))
	substrLower := make([]byte, len(substr))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			sLower[i] = c + ('a' - 'A')
		} else {
			sLower[i] = c
		}
	}
	for i := 0; i < len(substr); i++ {
		c := substr[i]
		if c >= 'A' && c <= 'Z' {
			substrLower[i] = c + ('a' - 'A')
		} else {
			substrLower[i] = c
		}
	}
	return indexOfBytes(sLower, substrLower) >= 0
}

func indexOfBytes(s, substr []byte) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// Close releases the database connection.
func (e *QueryExecutor) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

// ExecuteWithParams runs a parameterized DML statement (INSERT/UPDATE/DELETE/CALL).
// Converts PostgreSQL-style $1, $2 placeholders to SQL Server @p1, @p2 named parameters.
func (e *QueryExecutor) ExecuteWithParams(ctx context.Context, sqlStatement string, params []any) (*datasource.ExecuteResult, error) {
	result := &datasource.ExecuteResult{}

	// Convert PostgreSQL-style parameters ($1, $2, ...) to SQL Server (@p1, @p2, ...)
	convertedQuery := convertPostgreSQLParamsToMSSQL(sqlStatement)

	// Convert params slice to named parameters for SQL Server
	namedParams := make([]any, len(params))
	for i, p := range params {
		namedParams[i] = sql.Named(fmt.Sprintf("p%d", i+1), p)
	}

	// Try QueryContext first to check if statement returns rows
	rows, err := e.db.QueryContext(ctx, convertedQuery, namedParams...)
	if err != nil {
		// QueryContext failed - likely a DML statement without OUTPUT
		// Fall back to ExecContext to get rows affected
		execResult, execErr := e.db.ExecContext(ctx, convertedQuery, namedParams...)
		if execErr != nil {
			return nil, fmt.Errorf("failed to execute statement: %w", execErr)
		}

		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to get rows affected: %w", err)
		}

		result.RowsAffected = rowsAffected
		result.RowCount = 0
		return result, nil
	}
	defer rows.Close()

	// Check if the statement returns rows by checking column types
	columnTypes, err := rows.ColumnTypes()
	if err != nil || len(columnTypes) == 0 {
		// No columns - use ExecContext instead
		rows.Close()
		execResult, execErr := e.db.ExecContext(ctx, convertedQuery, namedParams...)
		if execErr != nil {
			return nil, fmt.Errorf("failed to execute statement: %w", execErr)
		}

		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to get rows affected: %w", err)
		}

		result.RowsAffected = rowsAffected
		result.RowCount = 0
		return result, nil
	}

	// Statement returns rows - collect them
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	result.Columns = columnNames
	result.Rows = make([]map[string]any, 0)

	for rows.Next() {
		// Create slice of interface{} to hold values
		values := make([]any, len(columnNames))
		valuePtrs := make([]any, len(columnNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row into value pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		rowMap := make(map[string]any)
		for i, col := range columnNames {
			val := values[i]

			// Handle SQL Server specific types
			if val != nil {
				// Convert []byte to string for text columns
				if b, ok := val.([]byte); ok {
					colType := columnTypes[i].DatabaseTypeName()
					if isStringType(colType) {
						val = string(b)
					}
				}
			}

			rowMap[col] = val
		}
		result.Rows = append(result.Rows, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	result.RowCount = len(result.Rows)
	return result, nil
}

// Ensure QueryExecutor implements datasource.QueryExecutor at compile time.
var _ datasource.QueryExecutor = (*QueryExecutor)(nil)

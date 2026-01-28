package datasource

import "context"

// ConnectionTester tests database connectivity.
// Each implementation owns its connection and must be closed when done.
type ConnectionTester interface {
	// TestConnection verifies the database is reachable with valid credentials.
	// Returns nil if connection is healthy, error otherwise.
	TestConnection(ctx context.Context) error

	// Close releases the database connection.
	Close() error
}

// SchemaExtractor extracts database schema information.
// Used for schema discovery in text2sql workflows.
type SchemaExtractor interface {
	// GetTables returns all tables in the database.
	GetTables(ctx context.Context) ([]Table, error)

	// GetColumns returns columns for a specific table.
	GetColumns(ctx context.Context, table string) ([]Column, error)

	// GetForeignKeys returns foreign key relationships for a table.
	GetForeignKeys(ctx context.Context, table string) ([]ForeignKey, error)
}

// SQLExecutor executes SQL queries against the database.
// Used for running generated SQL in text2sql workflows.
type SQLExecutor interface {
	// Execute runs a query and returns results.
	Execute(ctx context.Context, query string, params ...any) (*QueryResult, error)
}

// Table represents a database table.
type Table struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

// Column represents a database column.
type Column struct {
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	IsNullable bool   `json:"is_nullable"`
	IsPrimary  bool   `json:"is_primary"`
}

// ForeignKey represents a foreign key relationship.
type ForeignKey struct {
	Column           string `json:"column"`
	ReferencedTable  string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
}

// QueryResult contains the results of a SQL query execution.
type QueryResult struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
	RowsAff int64            `json:"rows_affected"`
}

// SchemaDiscoverer discovers database schema for metadata tracking.
// Each implementation owns its connection and must be closed when done.
// This is separate from SchemaExtractor which is for text2sql workflows.
type SchemaDiscoverer interface {
	// DiscoverTables returns all user tables (excludes system schemas).
	DiscoverTables(ctx context.Context) ([]TableMetadata, error)

	// DiscoverColumns returns columns for a specific table.
	DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]ColumnMetadata, error)

	// DiscoverForeignKeys returns all foreign key relationships.
	DiscoverForeignKeys(ctx context.Context) ([]ForeignKeyMetadata, error)

	// SupportsForeignKeys returns true if the database supports FK discovery.
	SupportsForeignKeys() bool

	// AnalyzeColumnStats gathers statistics for columns (for relationship inference).
	AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]ColumnStats, error)

	// CheckValueOverlap checks value overlap between two columns (for relationship inference).
	CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn,
		targetSchema, targetTable, targetColumn string, sampleLimit int) (*ValueOverlapResult, error)

	// AnalyzeJoin performs join analysis between two columns (for relationship inference).
	AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn,
		targetSchema, targetTable, targetColumn string) (*JoinAnalysis, error)

	// GetDistinctValues returns up to limit distinct non-null values from a column.
	// Values are returned as strings, sorted alphabetically.
	// Used during the scanning phase to collect sample values for enum detection.
	GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error)

	// GetEnumValueDistribution analyzes value distribution for an enum column.
	// Returns count and percentage for each distinct value, sorted by count descending.
	// If completionTimestampCol is non-empty, also computes completion rate per value
	// to identify initial vs terminal states in state machine columns.
	GetEnumValueDistribution(ctx context.Context, schemaName, tableName, columnName string,
		completionTimestampCol string, limit int) (*EnumDistributionResult, error)

	// Close releases the database connection.
	Close() error
}

// MaxQueryLimit is the hard cap on rows returned by Query methods.
// This protects against unbounded queries that could crash the server.
const MaxQueryLimit = 1000

// QueryExecutor executes SQL against a datasource.
// Provides two access patterns:
//   - Query/QueryWithParams: Safe, bounded SELECT queries (always wrapped with limit)
//   - Execute: Dangerous, unrestricted DDL/DML (for Developer Tools only)
//
// Each implementation owns its connection and must be closed when done.
type QueryExecutor interface {
	// Query runs a SELECT statement and returns bounded results.
	// The query is ALWAYS wrapped with a dialect-specific limit:
	//   - PostgreSQL: SELECT * FROM (query) AS _q LIMIT n
	//   - SQL Server: SELECT TOP (n) * FROM (query) AS _q
	//
	// Limit behavior:
	//   - limit <= 0: uses MaxQueryLimit (1000)
	//   - limit > MaxQueryLimit: capped to MaxQueryLimit (1000)
	//   - otherwise: uses specified limit
	//
	// This ensures all queries are bounded and prevents runaway results.
	Query(ctx context.Context, sqlQuery string, limit int) (*QueryExecutionResult, error)

	// QueryWithParams runs a parameterized SELECT with bounded results.
	// The SQL should use $1, $2, etc. for parameter placeholders.
	// The params slice provides values in order corresponding to the placeholders.
	// See Query for limit behavior - same wrapping and capping applies.
	QueryWithParams(ctx context.Context, sqlQuery string, params []any, limit int) (*QueryExecutionResult, error)

	// Execute runs any SQL statement (DDL/DML) without modification.
	// DANGEROUS: No wrapping, no limits. Use only for:
	//   - CREATE/DROP/ALTER statements
	//   - INSERT/UPDATE/DELETE operations
	//   - Other statements that modify the database
	//
	// For statements with RETURNING clauses, returns rows in the result.
	// For INSERT/UPDATE/DELETE without RETURNING, returns RowsAffected.
	Execute(ctx context.Context, sqlStatement string) (*ExecuteResult, error)

	// ExecuteWithParams runs a parameterized DML statement (INSERT/UPDATE/DELETE/CALL).
	// The SQL should use $1, $2, etc. for parameter placeholders.
	// The params slice provides values in order corresponding to the placeholders.
	//
	// For statements with RETURNING clauses, returns rows in the result.
	// For INSERT/UPDATE/DELETE without RETURNING, returns RowsAffected.
	ExecuteWithParams(ctx context.Context, sqlStatement string, params []any) (*ExecuteResult, error)

	// ValidateQuery checks if a SQL query is syntactically valid without executing it.
	// Returns nil if valid, error with details if invalid.
	ValidateQuery(ctx context.Context, sqlQuery string) error

	// ExplainQuery returns EXPLAIN ANALYZE output for a SQL query.
	// Provides performance insights including execution plan, timing, and hints.
	ExplainQuery(ctx context.Context, sqlQuery string) (*ExplainResult, error)

	// QuoteIdentifier safely quotes a SQL identifier (table, column, schema name)
	// to prevent SQL injection. Each adapter implements dialect-specific quoting.
	QuoteIdentifier(name string) string

	// Close releases any resources held by the executor.
	Close() error
}

// ExecuteResult holds the results from executing a DDL/DML statement.
type ExecuteResult struct {
	Columns      []string         `json:"columns,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	RowCount     int              `json:"row_count"`
	RowsAffected int64            `json:"rows_affected"`
}

// ColumnInfo describes a result column with database-agnostic type information.
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"` // Database type name (e.g., "TEXT", "INT4", "VARCHAR")
}

// QueryExecutionResult holds the results from executing a query.
type QueryExecutionResult struct {
	Columns  []ColumnInfo     `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

// ExplainResult holds the results from EXPLAIN ANALYZE output.
type ExplainResult struct {
	Plan             string   `json:"plan"`              // Full execution plan as text
	ExecutionTimeMs  float64  `json:"execution_time_ms"` // Actual execution time in milliseconds
	PlanningTimeMs   float64  `json:"planning_time_ms"`  // Query planning time in milliseconds
	PerformanceHints []string `json:"performance_hints"` // Suggestions for optimization
}

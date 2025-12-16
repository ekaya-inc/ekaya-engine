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

	// Close releases the database connection.
	Close() error
}

// QueryExecutor executes SQL queries against a datasource.
// Used for running saved queries from the Queries feature.
// Each implementation owns its connection and must be closed when done.
type QueryExecutor interface {
	// ExecuteQuery runs a SQL query and returns the results.
	// The limit parameter caps the number of rows returned (0 = no limit).
	ExecuteQuery(ctx context.Context, sqlQuery string, limit int) (*QueryExecutionResult, error)

	// ValidateQuery checks if a SQL query is syntactically valid without executing it.
	// Returns nil if valid, error with details if invalid.
	ValidateQuery(ctx context.Context, sqlQuery string) error

	// Close releases any resources held by the executor.
	Close() error
}

// QueryExecutionResult holds the results from executing a query.
type QueryExecutionResult struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

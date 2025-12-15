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

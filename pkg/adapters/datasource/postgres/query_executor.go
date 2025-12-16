//go:build postgres || all_adapters

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// QueryExecutor provides PostgreSQL query execution.
type QueryExecutor struct {
	pool *pgxpool.Pool
}

// NewQueryExecutor creates a PostgreSQL query executor.
func NewQueryExecutor(ctx context.Context, cfg *Config) (*QueryExecutor, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	return &QueryExecutor{pool: pool}, nil
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

	// Get column names
	fieldDescs := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		columns[i] = string(fd.Name)
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
			rowMap[col] = values[i]
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

// ValidateQuery checks if a SQL query is syntactically valid without executing it.
func (e *QueryExecutor) ValidateQuery(ctx context.Context, sqlQuery string) error {
	// Use EXPLAIN to validate without executing
	_, err := e.pool.Exec(ctx, "EXPLAIN "+sqlQuery)
	if err != nil {
		return fmt.Errorf("invalid SQL: %w", err)
	}
	return nil
}

// Close releases the connection pool.
func (e *QueryExecutor) Close() error {
	e.pool.Close()
	return nil
}

// Ensure QueryExecutor implements datasource.QueryExecutor at compile time.
var _ datasource.QueryExecutor = (*QueryExecutor)(nil)

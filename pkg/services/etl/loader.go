package etl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Loader creates tables and loads rows into a datasource.
type Loader struct {
	logger *zap.Logger
}

// NewLoader creates a new Loader.
func NewLoader(logger *zap.Logger) *Loader {
	return &Loader{logger: logger}
}

// CreateTable generates and executes CREATE TABLE DDL from the given schema.
func (l *Loader) CreateTable(ctx context.Context, executor datasource.QueryExecutor, tableName string, columns []models.InferredColumn) error {
	ddl := generateCreateTableDDL(executor, tableName, columns)
	l.logger.Info("creating table",
		zap.String("table", tableName),
		zap.Int("columns", len(columns)),
	)

	_, err := executor.Execute(ctx, ddl)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}
	return nil
}

// LoadRows inserts rows into the target table in batches.
func (l *Loader) LoadRows(ctx context.Context, executor datasource.QueryExecutor, tableName string, columns []models.InferredColumn, rows [][]string, batchSize int) *models.LoadResult {
	if batchSize <= 0 {
		batchSize = 500
	}

	result := &models.LoadResult{
		TableName:     tableName,
		RowsAttempted: len(rows),
	}

	colNames := make([]string, len(columns))
	for i, c := range columns {
		colNames[i] = executor.QuoteIdentifier(c.Name)
	}

	for batchStart := 0; batchStart < len(rows); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(rows) {
			batchEnd = len(rows)
		}
		batch := rows[batchStart:batchEnd]

		loaded, skipped, errs := l.insertBatch(ctx, executor, tableName, colNames, columns, batch)
		result.RowsLoaded += loaded
		result.RowsSkipped += skipped
		result.Errors = append(result.Errors, errs...)
	}

	return result
}

func (l *Loader) insertBatch(ctx context.Context, executor datasource.QueryExecutor, tableName string, quotedColNames []string, columns []models.InferredColumn, rows [][]string) (loaded, skipped int, errs []string) {
	if len(rows) == 0 {
		return 0, 0, nil
	}

	// Build multi-row INSERT
	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(executor.QuoteIdentifier(tableName))
	sb.WriteString(" (")
	sb.WriteString(strings.Join(quotedColNames, ", "))
	sb.WriteString(") VALUES ")

	var params []any
	paramIdx := 1

	for rowIdx, row := range rows {
		if rowIdx > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(")
		for colIdx := range columns {
			if colIdx > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("$%d", paramIdx))
			paramIdx++

			if colIdx < len(row) {
				val := strings.TrimSpace(row[colIdx])
				if val == "" {
					params = append(params, nil)
				} else {
					params = append(params, val)
				}
			} else {
				params = append(params, nil)
			}
		}
		sb.WriteString(")")
	}

	_, err := executor.ExecuteWithParams(ctx, sb.String(), params)
	if err != nil {
		// Fall back to row-by-row insertion on batch failure
		l.logger.Warn("batch insert failed, falling back to row-by-row",
			zap.String("table", tableName),
			zap.Error(err),
		)
		return l.insertRowByRow(ctx, executor, tableName, quotedColNames, columns, rows)
	}

	return len(rows), 0, nil
}

func (l *Loader) insertRowByRow(ctx context.Context, executor datasource.QueryExecutor, tableName string, quotedColNames []string, columns []models.InferredColumn, rows [][]string) (loaded, skipped int, errs []string) {
	for rowIdx, row := range rows {
		var sb strings.Builder
		sb.WriteString("INSERT INTO ")
		sb.WriteString(executor.QuoteIdentifier(tableName))
		sb.WriteString(" (")
		sb.WriteString(strings.Join(quotedColNames, ", "))
		sb.WriteString(") VALUES (")

		var params []any
		for colIdx := range columns {
			if colIdx > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("$%d", colIdx+1))

			if colIdx < len(row) {
				val := strings.TrimSpace(row[colIdx])
				if val == "" {
					params = append(params, nil)
				} else {
					params = append(params, val)
				}
			} else {
				params = append(params, nil)
			}
		}
		sb.WriteString(")")

		_, err := executor.ExecuteWithParams(ctx, sb.String(), params)
		if err != nil {
			skipped++
			errs = append(errs, fmt.Sprintf("row %d: %v", rowIdx+1, err))
		} else {
			loaded++
		}
	}
	return
}

// generateCreateTableDDL builds a CREATE TABLE IF NOT EXISTS statement.
func generateCreateTableDDL(executor datasource.QueryExecutor, tableName string, columns []models.InferredColumn) string {
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(executor.QuoteIdentifier(tableName))
	sb.WriteString(" (\n")

	for i, col := range columns {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString("  ")
		sb.WriteString(executor.QuoteIdentifier(col.Name))
		sb.WriteString(" ")
		sb.WriteString(col.SQLType)
		if !col.Nullable {
			sb.WriteString(" NOT NULL")
		}
	}

	sb.WriteString("\n)")
	return sb.String()
}

// SanitizeTableName converts a filename into a valid SQL table identifier.
func SanitizeTableName(filename string) string {
	// Remove file extension
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base == "" {
		return "imported_data"
	}

	name := sanitizeColumnName(base) // Reuse column sanitization logic
	if name == "" || name == "column" {
		name = "imported_data"
	}
	return name
}

//go:build postgres || all_adapters

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// SchemaDiscoverer provides PostgreSQL schema discovery.
type SchemaDiscoverer struct {
	pool *pgxpool.Pool
}

// NewSchemaDiscoverer creates a PostgreSQL schema discoverer.
func NewSchemaDiscoverer(ctx context.Context, cfg *Config) (*SchemaDiscoverer, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	return &SchemaDiscoverer{pool: pool}, nil
}

// Close releases the connection pool.
func (d *SchemaDiscoverer) Close() error {
	d.pool.Close()
	return nil
}

// SupportsForeignKeys returns true since PostgreSQL supports FK discovery.
func (d *SchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

// DiscoverTables returns all user tables (excludes system schemas).
func (d *SchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	const query = `
		SELECT
			t.table_schema,
			t.table_name,
			COALESCE(c.reltuples::bigint, 0) as row_count
		FROM information_schema.tables t
		LEFT JOIN pg_class c ON c.relname = t.table_name
		LEFT JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = t.table_schema
		WHERE t.table_type = 'BASE TABLE'
		  AND t.table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY t.table_schema, t.table_name
	`

	rows, err := d.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []datasource.TableMetadata
	for rows.Next() {
		var t datasource.TableMetadata
		if err := rows.Scan(&t.SchemaName, &t.TableName, &t.RowCount); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}

	return tables, nil
}

// DiscoverColumns returns columns for a specific table.
func (d *SchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	const query = `
		SELECT
			c.column_name,
			c.data_type,
			c.is_nullable = 'YES' as is_nullable,
			COALESCE(pk.is_pk, false) as is_primary_key,
			c.ordinal_position
		FROM information_schema.columns c
		LEFT JOIN (
			SELECT kcu.column_name, true as is_pk
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON tc.constraint_name = kcu.constraint_name
				AND tc.table_schema = kcu.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
			  AND tc.table_schema = $1
			  AND tc.table_name = $2
		) pk ON c.column_name = pk.column_name
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position
	`

	rows, err := d.pool.Query(ctx, query, schemaName, tableName)
	if err != nil {
		return nil, fmt.Errorf("query columns: %w", err)
	}
	defer rows.Close()

	var columns []datasource.ColumnMetadata
	for rows.Next() {
		var c datasource.ColumnMetadata
		if err := rows.Scan(&c.ColumnName, &c.DataType, &c.IsNullable, &c.IsPrimaryKey, &c.OrdinalPosition); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		columns = append(columns, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns: %w", err)
	}

	return columns, nil
}

// DiscoverForeignKeys returns all foreign key relationships.
func (d *SchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	const query = `
		SELECT
			tc.constraint_name,
			kcu.table_schema as source_schema,
			kcu.table_name as source_table,
			kcu.column_name as source_column,
			ccu.table_schema as target_schema,
			ccu.table_name as target_table,
			ccu.column_name as target_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
	`

	rows, err := d.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query foreign keys: %w", err)
	}
	defer rows.Close()

	var fks []datasource.ForeignKeyMetadata
	for rows.Next() {
		var fk datasource.ForeignKeyMetadata
		if err := rows.Scan(&fk.ConstraintName, &fk.SourceSchema, &fk.SourceTable, &fk.SourceColumn,
			&fk.TargetSchema, &fk.TargetTable, &fk.TargetColumn); err != nil {
			return nil, fmt.Errorf("scan foreign key: %w", err)
		}
		fks = append(fks, fk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign keys: %w", err)
	}

	return fks, nil
}

// AnalyzeColumnStats gathers statistics for columns.
func (d *SchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if len(columnNames) == 0 {
		return nil, nil
	}

	// Quote identifiers to prevent SQL injection
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	quotedTable := pgx.Identifier{tableName}.Sanitize()

	var stats []datasource.ColumnStats
	for _, colName := range columnNames {
		quotedCol := pgx.Identifier{colName}.Sanitize()

		query := fmt.Sprintf(`
			SELECT
				COUNT(*) as row_count,
				COUNT(%s) as non_null_count,
				COUNT(DISTINCT %s) as distinct_count
			FROM %s.%s
		`, quotedCol, quotedCol, quotedSchema, quotedTable)

		var s datasource.ColumnStats
		s.ColumnName = colName

		row := d.pool.QueryRow(ctx, query)
		if err := row.Scan(&s.RowCount, &s.NonNullCount, &s.DistinctCount); err != nil {
			return nil, fmt.Errorf("analyze column %s: %w", colName, err)
		}

		stats = append(stats, s)
	}

	return stats, nil
}

// CheckValueOverlap checks value overlap between two columns.
func (d *SchemaDiscoverer) CheckValueOverlap(ctx context.Context,
	sourceSchema, sourceTable, sourceColumn,
	targetSchema, targetTable, targetColumn string,
	sampleLimit int) (*datasource.ValueOverlapResult, error) {

	// Quote identifiers to prevent SQL injection
	srcSchema := pgx.Identifier{sourceSchema}.Sanitize()
	srcTable := pgx.Identifier{sourceTable}.Sanitize()
	srcCol := pgx.Identifier{sourceColumn}.Sanitize()
	tgtSchema := pgx.Identifier{targetSchema}.Sanitize()
	tgtTable := pgx.Identifier{targetTable}.Sanitize()
	tgtCol := pgx.Identifier{targetColumn}.Sanitize()

	query := fmt.Sprintf(`
		WITH source_vals AS (
			SELECT DISTINCT %s::text as val
			FROM %s.%s
			WHERE %s IS NOT NULL
			LIMIT $1
		),
		target_vals AS (
			SELECT DISTINCT %s::text as val
			FROM %s.%s
			WHERE %s IS NOT NULL
			LIMIT $1
		)
		SELECT
			(SELECT COUNT(*) FROM source_vals) as source_distinct,
			(SELECT COUNT(*) FROM target_vals) as target_distinct,
			(SELECT COUNT(*) FROM source_vals s JOIN target_vals t ON s.val = t.val) as matched_count
	`, srcCol, srcSchema, srcTable, srcCol, tgtCol, tgtSchema, tgtTable, tgtCol)

	var result datasource.ValueOverlapResult
	row := d.pool.QueryRow(ctx, query, sampleLimit)
	if err := row.Scan(&result.SourceDistinct, &result.TargetDistinct, &result.MatchedCount); err != nil {
		return nil, fmt.Errorf("check value overlap: %w", err)
	}

	// Calculate match rate
	if result.SourceDistinct > 0 {
		result.MatchRate = float64(result.MatchedCount) / float64(result.SourceDistinct)
	}

	return &result, nil
}

// AnalyzeJoin performs join analysis between two columns.
func (d *SchemaDiscoverer) AnalyzeJoin(ctx context.Context,
	sourceSchema, sourceTable, sourceColumn,
	targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {

	// Quote identifiers to prevent SQL injection
	srcSchema := pgx.Identifier{sourceSchema}.Sanitize()
	srcTable := pgx.Identifier{sourceTable}.Sanitize()
	srcCol := pgx.Identifier{sourceColumn}.Sanitize()
	tgtSchema := pgx.Identifier{targetSchema}.Sanitize()
	tgtTable := pgx.Identifier{targetTable}.Sanitize()
	tgtCol := pgx.Identifier{targetColumn}.Sanitize()

	query := fmt.Sprintf(`
		WITH join_stats AS (
			SELECT
				COUNT(*) as join_count,
				COUNT(DISTINCT s.%s) as source_matched,
				COUNT(DISTINCT t.%s) as target_matched
			FROM %s.%s s
			JOIN %s.%s t ON s.%s = t.%s
		),
		orphan_stats AS (
			SELECT COUNT(*) as orphan_count
			FROM %s.%s s
			LEFT JOIN %s.%s t ON s.%s = t.%s
			WHERE t.%s IS NULL AND s.%s IS NOT NULL
		)
		SELECT join_count, source_matched, target_matched, orphan_count
		FROM join_stats, orphan_stats
	`, srcCol, tgtCol, srcSchema, srcTable, tgtSchema, tgtTable, srcCol, tgtCol,
		srcSchema, srcTable, tgtSchema, tgtTable, srcCol, tgtCol, tgtCol, srcCol)

	var result datasource.JoinAnalysis
	row := d.pool.QueryRow(ctx, query)
	if err := row.Scan(&result.JoinCount, &result.SourceMatched, &result.TargetMatched, &result.OrphanCount); err != nil {
		return nil, fmt.Errorf("analyze join: %w", err)
	}

	return &result, nil
}

// GetDistinctValues returns up to limit distinct non-null values from a column.
func (d *SchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	// Quote identifiers to prevent SQL injection
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	quotedTable := pgx.Identifier{tableName}.Sanitize()
	quotedCol := pgx.Identifier{columnName}.Sanitize()

	query := fmt.Sprintf(`
		SELECT DISTINCT %s::text
		FROM %s.%s
		WHERE %s IS NOT NULL
		ORDER BY 1
		LIMIT $1
	`, quotedCol, quotedSchema, quotedTable, quotedCol)

	rows, err := d.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get distinct values for %s.%s.%s: %w", schemaName, tableName, columnName, err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			return nil, fmt.Errorf("scan distinct value: %w", err)
		}
		values = append(values, val)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate distinct values: %w", err)
	}

	return values, nil
}

// Ensure SchemaDiscoverer implements datasource.SchemaDiscoverer at compile time.
var _ datasource.SchemaDiscoverer = (*SchemaDiscoverer)(nil)

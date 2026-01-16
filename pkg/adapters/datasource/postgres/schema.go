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

// SchemaDiscoverer provides PostgreSQL schema discovery.
type SchemaDiscoverer struct {
	pool         *pgxpool.Pool
	connMgr      *datasource.ConnectionManager
	projectID    uuid.UUID
	userID       string
	datasourceID uuid.UUID
	ownedPool    bool // true if we created the pool (for tests or direct instantiation)
}

// NewSchemaDiscoverer creates a PostgreSQL schema discoverer using the connection manager.
// If connMgr is nil, creates an unmanaged pool (for tests or direct instantiation).
func NewSchemaDiscoverer(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*SchemaDiscoverer, error) {
	connStr := buildConnectionString(cfg)

	if connMgr == nil {
		// Fallback for direct instantiation (tests)
		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}

		return &SchemaDiscoverer{
			pool:      pool,
			ownedPool: true,
		}, nil
	}

	// Use connection manager for reusable pool
	connector, err := connMgr.GetOrCreateConnection(ctx, "postgres", projectID, userID, datasourceID, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get pooled connection: %w", err)
	}

	// Extract underlying PostgreSQL pool from connector
	pool, err := datasource.GetPostgresPool(connector)
	if err != nil {
		return nil, fmt.Errorf("failed to extract postgres pool: %w", err)
	}

	return &SchemaDiscoverer{
		pool:         pool,
		connMgr:      connMgr,
		projectID:    projectID,
		userID:       userID,
		datasourceID: datasourceID,
		ownedPool:    false,
	}, nil
}

// Close releases the adapter (but NOT the pool if managed).
func (d *SchemaDiscoverer) Close() error {
	if d.ownedPool && d.pool != nil {
		d.pool.Close()
	}
	// If using connection manager, don't close the pool - it's managed by TTL
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
// Uses pg_index for primary key and unique detection, which correctly identifies
// primary keys even when created as unique indexes (common with GORM/ORMs).
func (d *SchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	const query = `
		SELECT
			c.column_name,
			c.data_type,
			c.is_nullable = 'YES' as is_nullable,
			COALESCE(pk.is_pk, false) as is_primary_key,
			COALESCE(uq.is_unique, false) as is_unique,
			c.ordinal_position,
			c.column_default
		FROM information_schema.columns c
		LEFT JOIN (
			-- Use pg_index.indisprimary which correctly detects PKs even when
			-- created as unique indexes (e.g., GORM creates "tablename_pkey" indexes)
			SELECT a.attname as column_name, true as is_pk
			FROM pg_index ix
			JOIN pg_class t ON t.oid = ix.indrelid
			JOIN pg_class i ON i.oid = ix.indexrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
			WHERE ix.indisprimary = true
			  AND n.nspname = $1
			  AND t.relname = $2
			  AND array_length(ix.indkey, 1) = 1  -- Single-column PKs only
		) pk ON c.column_name = pk.column_name
		LEFT JOIN (
			-- Use pg_index.indisunique for unique constraint detection
			SELECT a.attname as column_name, true as is_unique
			FROM pg_index ix
			JOIN pg_class t ON t.oid = ix.indrelid
			JOIN pg_class i ON i.oid = ix.indexrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
			WHERE ix.indisunique = true
			  AND ix.indisprimary = false  -- Exclude PKs (they're handled above)
			  AND n.nspname = $1
			  AND t.relname = $2
			  AND array_length(ix.indkey, 1) = 1  -- Single-column unique indexes only
		) uq ON c.column_name = uq.column_name
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
		if err := rows.Scan(&c.ColumnName, &c.DataType, &c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.OrdinalPosition, &c.DefaultValue); err != nil {
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

		// Query includes length stats for text columns (used to detect uniform-length IDs like UUIDs)
		query := fmt.Sprintf(`
			SELECT
				COUNT(*) as row_count,
				COUNT(%s) as non_null_count,
				COUNT(DISTINCT %s) as distinct_count,
				MIN(LENGTH(%s::text)) as min_length,
				MAX(LENGTH(%s::text)) as max_length
			FROM %s.%s
		`, quotedCol, quotedCol, quotedCol, quotedCol, quotedSchema, quotedTable)

		var s datasource.ColumnStats
		s.ColumnName = colName

		row := d.pool.QueryRow(ctx, query)
		if err := row.Scan(&s.RowCount, &s.NonNullCount, &s.DistinctCount, &s.MinLength, &s.MaxLength); err != nil {
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

	// Cast columns to text to handle cross-type comparisons (e.g., text vs bigint)
	query := fmt.Sprintf(`
		WITH join_stats AS (
			SELECT
				COUNT(*) as join_count,
				COUNT(DISTINCT s.%s) as source_matched,
				COUNT(DISTINCT t.%s) as target_matched
			FROM %s.%s s
			JOIN %s.%s t ON s.%s::text = t.%s::text
		),
		orphan_stats AS (
			SELECT COUNT(*) as orphan_count
			FROM %s.%s s
			LEFT JOIN %s.%s t ON s.%s::text = t.%s::text
			WHERE t.%s IS NULL AND s.%s IS NOT NULL
		),
		max_target AS (
			SELECT MAX(
				CASE
					WHEN t.%s::text ~ '^-?[0-9]+(\.[0-9]+)?$'
					THEN (t.%s::text)::numeric
					ELSE NULL
				END
			) as max_source_value
			FROM %s.%s t
			WHERE t.%s IS NOT NULL
		)
		SELECT join_count, source_matched, target_matched, orphan_count, max_source_value
		FROM join_stats, orphan_stats, max_target
	`, srcCol, tgtCol, srcSchema, srcTable, tgtSchema, tgtTable, srcCol, tgtCol,
		srcSchema, srcTable, tgtSchema, tgtTable, srcCol, tgtCol, tgtCol, srcCol,
		tgtCol, tgtCol, tgtSchema, tgtTable, tgtCol)

	var result datasource.JoinAnalysis
	row := d.pool.QueryRow(ctx, query)
	if err := row.Scan(&result.JoinCount, &result.SourceMatched, &result.TargetMatched, &result.OrphanCount, &result.MaxSourceValue); err != nil {
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

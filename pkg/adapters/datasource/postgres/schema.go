//go:build postgres || all_adapters

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// qualifiedTableName returns a properly quoted table reference.
// If schemaName is empty, returns just the quoted table name.
// Otherwise returns "schema"."table".
func qualifiedTableName(schemaName, tableName string) string {
	quotedTable := pgx.Identifier{tableName}.Sanitize()
	if schemaName == "" {
		return quotedTable
	}
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	return quotedSchema + "." + quotedTable
}

// SchemaDiscoverer provides PostgreSQL schema discovery.
type SchemaDiscoverer struct {
	pool         *pgxpool.Pool
	connMgr      *datasource.ConnectionManager
	projectID    uuid.UUID
	userID       string
	datasourceID uuid.UUID
	ownedPool    bool // true if we created the pool (for tests or direct instantiation)
	logger       *zap.Logger
}

// NewSchemaDiscoverer creates a PostgreSQL schema discoverer using the connection manager.
// If connMgr is nil, creates an unmanaged pool (for tests or direct instantiation).
// If logger is nil, a no-op logger is used.
func NewSchemaDiscoverer(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string, logger *zap.Logger) (*SchemaDiscoverer, error) {
	connStr := buildConnectionString(cfg)

	if logger == nil {
		logger = zap.NewNop()
	}

	if connMgr == nil {
		// Fallback for direct instantiation (tests)
		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}

		return &SchemaDiscoverer{
			pool:      pool,
			ownedPool: true,
			logger:    logger,
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
		logger:       logger,
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
// Continues processing other columns when one fails (e.g., type cast errors for arrays/bytea).
// If the main query fails, retries with a simplified query (without length calculation).
// Failed columns are included in results with zero/nil stats.
func (d *SchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if len(columnNames) == 0 {
		return nil, nil
	}

	// Build qualified table name (handles empty schema)
	tableRef := qualifiedTableName(schemaName, tableName)

	var stats []datasource.ColumnStats
	var retriedColumns []string
	for _, colName := range columnNames {
		quotedCol := pgx.Identifier{colName}.Sanitize()

		// Query includes length stats for text-compatible columns (used to detect uniform-length IDs like UUIDs).
		// For non-text types (arrays, bytea, json, etc.), length is set to NULL to avoid cast errors.
		// We use a subquery to determine the column type, then conditionally calculate length.
		query := fmt.Sprintf(`
			WITH col_type AS (
				SELECT pg_typeof(%s)::text AS dtype
				FROM %s
				WHERE %s IS NOT NULL
				LIMIT 1
			)
			SELECT
				COUNT(*) as row_count,
				COUNT(%s) as non_null_count,
				COUNT(DISTINCT %s) as distinct_count,
				CASE
					WHEN (SELECT dtype FROM col_type) IN ('text', 'character varying', 'character', 'uuid', 'name', 'bpchar')
					THEN MIN(LENGTH(%s::text))
					ELSE NULL
				END as min_length,
				CASE
					WHEN (SELECT dtype FROM col_type) IN ('text', 'character varying', 'character', 'uuid', 'name', 'bpchar')
					THEN MAX(LENGTH(%s::text))
					ELSE NULL
				END as max_length
			FROM %s
		`, quotedCol, tableRef, quotedCol,
			quotedCol, quotedCol, quotedCol, quotedCol, tableRef)

		var s datasource.ColumnStats
		s.ColumnName = colName

		row := d.pool.QueryRow(ctx, query)
		if err := row.Scan(&s.RowCount, &s.NonNullCount, &s.DistinctCount, &s.MinLength, &s.MaxLength); err != nil {
			// Main query failed - retry with simplified query (no length calculation).
			// This handles edge cases where pg_typeof or the CTE causes issues.
			retriedColumns = append(retriedColumns, colName)

			simplifiedQuery := fmt.Sprintf(`
				SELECT
					COUNT(*) as row_count,
					COUNT(%s) as non_null_count,
					COUNT(DISTINCT %s) as distinct_count
				FROM %s
			`, quotedCol, quotedCol, tableRef)

			retryRow := d.pool.QueryRow(ctx, simplifiedQuery)
			if retryErr := retryRow.Scan(&s.RowCount, &s.NonNullCount, &s.DistinctCount); retryErr != nil {
				// Both queries failed - log warning and use zero values
				d.logger.Warn("Failed to analyze column stats after retry, using zero values",
					zap.String("schema", schemaName),
					zap.String("table", tableName),
					zap.String("column", colName),
					zap.Error(err),
					zap.NamedError("retry_error", retryErr))
				s.RowCount = 0
				s.NonNullCount = 0
				s.DistinctCount = 0
			} else {
				d.logger.Debug("Column stats raw scan (retry)",
					zap.String("column", colName),
					zap.Int64("row_count", s.RowCount),
					zap.Int64("non_null_count", s.NonNullCount),
					zap.Int64("distinct_count", s.DistinctCount))
			}
			// Length stats are nil for retried columns (simplified query doesn't calculate them)
			s.MinLength = nil
			s.MaxLength = nil
		} else {
			d.logger.Debug("Column stats raw scan",
				zap.String("column", colName),
				zap.Int64("row_count", s.RowCount),
				zap.Int64("non_null_count", s.NonNullCount),
				zap.Int64("distinct_count", s.DistinctCount))
		}

		stats = append(stats, s)
	}

	// Log summary if any columns needed retry
	if len(retriedColumns) > 0 {
		d.logger.Info("Some columns required simplified stats query (no length calculation)",
			zap.String("schema", schemaName),
			zap.String("table", tableName),
			zap.Int("retried_count", len(retriedColumns)),
			zap.Strings("retried_columns", retriedColumns))
	}

	return stats, nil
}

// CheckValueOverlap checks value overlap between two columns.
func (d *SchemaDiscoverer) CheckValueOverlap(ctx context.Context,
	sourceSchema, sourceTable, sourceColumn,
	targetSchema, targetTable, targetColumn string,
	sampleLimit int) (*datasource.ValueOverlapResult, error) {

	// Build qualified table names (handles empty schema)
	srcTableRef := qualifiedTableName(sourceSchema, sourceTable)
	tgtTableRef := qualifiedTableName(targetSchema, targetTable)
	srcCol := pgx.Identifier{sourceColumn}.Sanitize()
	tgtCol := pgx.Identifier{targetColumn}.Sanitize()

	query := fmt.Sprintf(`
		WITH source_vals AS (
			SELECT DISTINCT %s::text as val
			FROM %s
			WHERE %s IS NOT NULL
			LIMIT $1
		),
		target_vals AS (
			SELECT DISTINCT %s::text as val
			FROM %s
			WHERE %s IS NOT NULL
			LIMIT $1
		)
		SELECT
			(SELECT COUNT(*) FROM source_vals) as source_distinct,
			(SELECT COUNT(*) FROM target_vals) as target_distinct,
			(SELECT COUNT(*) FROM source_vals s JOIN target_vals t ON s.val = t.val) as matched_count
	`, srcCol, srcTableRef, srcCol, tgtCol, tgtTableRef, tgtCol)

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
// Computes both source→target orphans and target→source (reverse) orphans
// to detect false positive relationships (e.g., identity_provider → jobs.id).
func (d *SchemaDiscoverer) AnalyzeJoin(ctx context.Context,
	sourceSchema, sourceTable, sourceColumn,
	targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {

	// Quote identifiers to prevent SQL injection
	srcTableRef := qualifiedTableName(sourceSchema, sourceTable)
	tgtTableRef := qualifiedTableName(targetSchema, targetTable)
	srcCol := pgx.Identifier{sourceColumn}.Sanitize()
	tgtCol := pgx.Identifier{targetColumn}.Sanitize()

	// Cast columns to text to handle cross-type comparisons (e.g., text vs bigint)
	// Computes:
	// - orphan_count: source values that don't exist in target (source→target)
	// - reverse_orphan_count: target values that don't exist in source (target→source)
	//
	// The reverse orphan check catches false positives like:
	// - identity_provider has 3 values {1,2,3}, jobs.id has 83 values {1-83}
	// - Source→target: all 3 exist in jobs.id → 0 orphans → would PASS
	// - Target→source: 80 values (4-83) don't exist → reverse_orphan_count = 80 → REJECT
	query := fmt.Sprintf(`
		WITH join_stats AS (
			SELECT
				COUNT(*) as join_count,
				COUNT(DISTINCT s.%s) as source_matched,
				COUNT(DISTINCT t.%s) as target_matched
			FROM %s s
			JOIN %s t ON s.%s::text = t.%s::text
		),
		orphan_stats AS (
			SELECT COUNT(DISTINCT s.%s) as orphan_count
			FROM %s s
			LEFT JOIN %s t ON s.%s::text = t.%s::text
			WHERE t.%s IS NULL AND s.%s IS NOT NULL
		),
		reverse_orphan_stats AS (
			SELECT COUNT(DISTINCT t.%s) as reverse_orphan_count
			FROM %s t
			LEFT JOIN %s s ON t.%s::text = s.%s::text
			WHERE s.%s IS NULL AND t.%s IS NOT NULL
		),
		max_source AS (
			SELECT MAX(
				CASE
					WHEN s.%s::text ~ '^-?[0-9]+(\.[0-9]+)?$'
					THEN (s.%s::text)::numeric
					ELSE NULL
				END
			) as max_source_value
			FROM %s s
			WHERE s.%s IS NOT NULL
		)
		SELECT join_count, source_matched, target_matched, orphan_count, reverse_orphan_count, max_source_value
		FROM join_stats, orphan_stats, reverse_orphan_stats, max_source
	`,
		// join_stats
		srcCol, tgtCol, srcTableRef, tgtTableRef, srcCol, tgtCol,
		// orphan_stats
		srcCol, srcTableRef, tgtTableRef, srcCol, tgtCol, tgtCol, srcCol,
		// reverse_orphan_stats
		tgtCol, tgtTableRef, srcTableRef, tgtCol, srcCol, srcCol, tgtCol,
		// max_source
		srcCol, srcCol, srcTableRef, srcCol)

	var result datasource.JoinAnalysis
	row := d.pool.QueryRow(ctx, query)
	if err := row.Scan(&result.JoinCount, &result.SourceMatched, &result.TargetMatched, &result.OrphanCount, &result.ReverseOrphanCount, &result.MaxSourceValue); err != nil {
		return nil, fmt.Errorf("analyze join: %w", err)
	}

	return &result, nil
}

// GetDistinctValues returns up to limit distinct non-null values from a column.
func (d *SchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	// Quote identifiers to prevent SQL injection
	tableRef := qualifiedTableName(schemaName, tableName)
	quotedCol := pgx.Identifier{columnName}.Sanitize()

	query := fmt.Sprintf(`
		SELECT DISTINCT %s::text
		FROM %s
		WHERE %s IS NOT NULL
		ORDER BY 1
		LIMIT $1
	`, quotedCol, tableRef, quotedCol)

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

// GetEnumValueDistribution analyzes value distribution for an enum column.
// Returns count and percentage for each distinct value, sorted by count descending.
// If completionTimestampCol is provided, also computes completion rate per value.
func (d *SchemaDiscoverer) GetEnumValueDistribution(ctx context.Context, schemaName, tableName, columnName string, completionTimestampCol string, limit int) (*datasource.EnumDistributionResult, error) {
	// Build qualified table name (handles empty schema)
	tableRef := qualifiedTableName(schemaName, tableName)
	quotedCol := pgx.Identifier{columnName}.Sanitize()

	// Get total row count and null count first
	totalQuery := fmt.Sprintf(`
		SELECT COUNT(*) as total_rows,
		       COUNT(*) FILTER (WHERE %s IS NULL) as null_count,
		       COUNT(DISTINCT %s) as distinct_count
		FROM %s
	`, quotedCol, quotedCol, tableRef)

	var totalRows, nullCount, distinctCount int64
	if err := d.pool.QueryRow(ctx, totalQuery).Scan(&totalRows, &nullCount, &distinctCount); err != nil {
		return nil, fmt.Errorf("get totals for %s.%s.%s: %w", schemaName, tableName, columnName, err)
	}

	result := &datasource.EnumDistributionResult{
		ColumnName:    columnName,
		TotalRows:     totalRows,
		DistinctCount: distinctCount,
		NullCount:     nullCount,
		Distributions: []datasource.EnumValueDistribution{},
	}

	// Build the distribution query - with or without completion timestamp analysis
	var query string
	if completionTimestampCol != "" {
		quotedCompletionCol := pgx.Identifier{completionTimestampCol}.Sanitize()
		result.CompletionTimestampCol = completionTimestampCol

		query = fmt.Sprintf(`
			SELECT %s::text as value,
			       COUNT(*) as count,
			       ROUND(100.0 * COUNT(*) / NULLIF($1::numeric, 0), 2) as percentage,
			       COUNT(*) FILTER (WHERE %s IS NOT NULL) as has_completion_at,
			       ROUND(100.0 * COUNT(*) FILTER (WHERE %s IS NOT NULL) / NULLIF(COUNT(*), 0), 2) as completion_rate
			FROM %s
			WHERE %s IS NOT NULL
			GROUP BY %s
			ORDER BY count DESC
			LIMIT $2
		`, quotedCol, quotedCompletionCol, quotedCompletionCol, tableRef, quotedCol, quotedCol)
	} else {
		query = fmt.Sprintf(`
			SELECT %s::text as value,
			       COUNT(*) as count,
			       ROUND(100.0 * COUNT(*) / NULLIF($1::numeric, 0), 2) as percentage,
			       0 as has_completion_at,
			       0.0 as completion_rate
			FROM %s
			WHERE %s IS NOT NULL
			GROUP BY %s
			ORDER BY count DESC
			LIMIT $2
		`, quotedCol, tableRef, quotedCol, quotedCol)
	}

	rows, err := d.pool.Query(ctx, query, totalRows, limit)
	if err != nil {
		return nil, fmt.Errorf("get enum distribution for %s.%s.%s: %w", schemaName, tableName, columnName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var dist datasource.EnumValueDistribution
		var percentage, completionRate float64
		if err := rows.Scan(&dist.Value, &dist.Count, &percentage, &dist.HasCompletionAt, &completionRate); err != nil {
			return nil, fmt.Errorf("scan distribution row: %w", err)
		}
		dist.Percentage = percentage
		dist.CompletionRate = completionRate
		dist.TotalRows = totalRows
		result.Distributions = append(result.Distributions, dist)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate distribution rows: %w", err)
	}

	// Infer state semantics if completion timestamp was provided and we have data
	if completionTimestampCol != "" && len(result.Distributions) > 0 {
		result.HasStateSemantics = inferStateSemantics(result.Distributions)
	}

	return result, nil
}

// inferStateSemantics analyzes distribution data to classify values as initial/terminal/error states.
// Sets the Is* flags on each EnumValueDistribution.
func inferStateSemantics(distributions []datasource.EnumValueDistribution) bool {
	if len(distributions) == 0 {
		return false
	}

	// Find max and min counts for relative comparisons
	var maxCount, minCount int64 = distributions[0].Count, distributions[0].Count
	var totalCount int64
	for _, d := range distributions {
		if d.Count > maxCount {
			maxCount = d.Count
		}
		if d.Count < minCount {
			minCount = d.Count
		}
		totalCount += d.Count
	}

	if totalCount == 0 {
		return false
	}

	foundInitial := false
	foundTerminal := false
	avgCount := totalCount / int64(len(distributions))

	for i := range distributions {
		d := &distributions[i]

		// Terminal state: high completion rate (~100%) and significant count
		// States with ~100% completion are terminal (records stay in this state)
		if d.CompletionRate >= 95.0 && d.Count > 0 {
			d.IsLikelyTerminalState = true
			foundTerminal = true
		}

		// Initial state: low completion rate (~0%) and high count
		// States with ~0% completion but high count are initial/pending states
		if d.CompletionRate <= 5.0 && d.Count >= avgCount/2 {
			d.IsLikelyInitialState = true
			foundInitial = true
		}

		// Error/rare state: very low count relative to others
		// If count is <5% of max count, it's likely a rare/error state
		if maxCount > 0 && float64(d.Count)/float64(maxCount) < 0.05 {
			d.IsLikelyErrorState = true
		}
	}

	return foundInitial || foundTerminal
}

// Ensure SchemaDiscoverer implements datasource.SchemaDiscoverer at compile time.
var _ datasource.SchemaDiscoverer = (*SchemaDiscoverer)(nil)

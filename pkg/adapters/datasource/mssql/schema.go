package mssql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// SchemaDiscoverer implements datasource.SchemaDiscoverer for SQL Server.
type SchemaDiscoverer struct {
	config *Config
	db     *sql.DB
	logger *zap.Logger
}

// NewSchemaDiscoverer creates a new SQL Server schema discoverer.
// Uses connection manager for connection pooling.
// If logger is nil, a no-op logger is used.
func NewSchemaDiscoverer(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string, logger *zap.Logger) (*SchemaDiscoverer, error) {
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

	if logger == nil {
		logger = zap.NewNop()
	}

	// Use the same connection logic as Adapter
	adapter, err := NewAdapter(ctx, cfg, connMgr, projectID, datasourceID, userID)
	if err != nil {
		return nil, err
	}

	return &SchemaDiscoverer{
		config: cfg,
		db:     adapter.DB(),
		logger: logger,
	}, nil
}

// DiscoverTables returns all user tables (excludes system schemas).
func (s *SchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	query := `
	SET NOCOUNT ON;
	SELECT
	    SCHEMA_NAME(t.schema_id) AS table_schema,
	    t.name AS table_name,
	    SUM(p.rows) AS row_count
	FROM sys.tables t
	INNER JOIN sys.partitions p ON t.object_id = p.object_id
	WHERE p.index_id IN (0, 1)  -- Heap or clustered index
	  AND t.is_ms_shipped = 0   -- Exclude system tables
	GROUP BY t.schema_id, t.name
	ORDER BY table_schema, table_name
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []datasource.TableMetadata
	for rows.Next() {
		var table datasource.TableMetadata
		err := rows.Scan(&table.SchemaName, &table.TableName, &table.RowCount)
		if err != nil {
			return nil, fmt.Errorf("scan table row: %w", err)
		}
		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table rows: %w", err)
	}

	return tables, nil
}

// DiscoverColumns returns columns for a specific table.
func (s *SchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	query := `
	SET NOCOUNT ON;
	SELECT
	    c.name AS column_name,
	    tp.name AS data_type,
	    CASE WHEN c.is_nullable = 1 THEN 1 ELSE 0 END AS is_nullable,
	    c.column_id AS ordinal_position,
	    CASE WHEN pk.column_id IS NOT NULL THEN 1 ELSE 0 END AS is_primary_key
	FROM sys.columns c
	INNER JOIN sys.types tp ON c.user_type_id = tp.user_type_id
	LEFT JOIN (
	    SELECT ic.object_id, ic.column_id
	    FROM sys.index_columns ic
	    INNER JOIN sys.indexes i ON ic.object_id = i.object_id AND ic.index_id = i.index_id
	    WHERE i.is_primary_key = 1
	) pk ON c.object_id = pk.object_id AND c.column_id = pk.column_id
	WHERE c.object_id = OBJECT_ID(QUOTENAME(@schema) + N'.' + QUOTENAME(@table))
	ORDER BY c.column_id
	`

	rows, err := s.db.QueryContext(ctx, query,
		sql.Named("schema", schemaName),
		sql.Named("table", tableName),
	)
	if err != nil {
		return nil, fmt.Errorf("query columns: %w", err)
	}
	defer rows.Close()

	var columns []datasource.ColumnMetadata
	for rows.Next() {
		var col datasource.ColumnMetadata
		var isNullable, isPrimary int

		err := rows.Scan(
			&col.ColumnName,
			&col.DataType,
			&isNullable,
			&col.OrdinalPosition,
			&isPrimary,
		)
		if err != nil {
			return nil, fmt.Errorf("scan column row: %w", err)
		}

		col.IsNullable = isNullable == 1
		col.IsPrimaryKey = isPrimary == 1
		col.DataType = mapSQLServerType(col.DataType)

		columns = append(columns, col)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate column rows: %w", err)
	}

	return columns, nil
}

// DiscoverForeignKeys returns all foreign key relationships.
func (s *SchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	query := `
	SET NOCOUNT ON;
	SELECT
	    fk.name AS constraint_name,
	    SCHEMA_NAME(fk.schema_id) AS source_schema,
	    OBJECT_NAME(fk.parent_object_id) AS source_table,
	    COL_NAME(fkc.parent_object_id, fkc.parent_column_id) AS source_column,
	    SCHEMA_NAME(rt.schema_id) AS target_schema,
	    OBJECT_NAME(fk.referenced_object_id) AS target_table,
	    COL_NAME(fkc.referenced_object_id, fkc.referenced_column_id) AS target_column
	FROM sys.foreign_keys fk
	INNER JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
	INNER JOIN sys.tables rt ON fk.referenced_object_id = rt.object_id
	WHERE fk.is_ms_shipped = 0
	ORDER BY source_schema, source_table, fk.name, fkc.constraint_column_id
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query foreign keys: %w", err)
	}
	defer rows.Close()

	var fks []datasource.ForeignKeyMetadata
	for rows.Next() {
		var fk datasource.ForeignKeyMetadata
		err := rows.Scan(
			&fk.ConstraintName,
			&fk.SourceSchema,
			&fk.SourceTable,
			&fk.SourceColumn,
			&fk.TargetSchema,
			&fk.TargetTable,
			&fk.TargetColumn,
		)
		if err != nil {
			return nil, fmt.Errorf("scan foreign key row: %w", err)
		}
		fks = append(fks, fk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign key rows: %w", err)
	}

	return fks, nil
}

// SupportsForeignKeys returns true since SQL Server supports foreign keys.
func (s *SchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

// getColumnType queries the data type of a column from SQL Server metadata.
// Returns the type name (e.g., "varchar", "int", "uniqueidentifier") or empty string if not found.
func (s *SchemaDiscoverer) getColumnType(ctx context.Context, schemaName, tableName, columnName string) (string, error) {
	query := `
		SET NOCOUNT ON;
		SELECT tp.name
		FROM sys.columns c
		INNER JOIN sys.types tp ON c.user_type_id = tp.user_type_id
		WHERE c.object_id = OBJECT_ID(QUOTENAME(@schema) + N'.' + QUOTENAME(@table))
		AND c.name = @column
	`
	var typeName string
	err := s.db.QueryRowContext(ctx, query,
		sql.Named("schema", schemaName),
		sql.Named("table", tableName),
		sql.Named("column", columnName),
	).Scan(&typeName)
	if err != nil {
		return "", err
	}
	return typeName, nil
}

// AnalyzeColumnStats gathers statistics for columns.
// Continues processing other columns when one fails (e.g., type cast errors).
// If the main query fails, retries with a simplified query (without length calculation).
// Failed columns are included in results with zero/nil stats.
func (s *SchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if len(columnNames) == 0 {
		return nil, nil
	}

	fullyQualifiedTable := buildFullyQualifiedName(schemaName, tableName)

	var stats []datasource.ColumnStats
	var retriedColumns []string
	for _, colName := range columnNames {
		quotedCol := quoteName(colName)

		var stat datasource.ColumnStats
		stat.ColumnName = colName

		// Query column type from metadata to determine if length calculation is appropriate.
		// This is more reliable than SQL_VARIANT_PROPERTY which only works on sql_variant columns.
		colType, typeErr := s.getColumnType(ctx, schemaName, tableName, colName)
		if typeErr != nil {
			// Column type lookup failed (column may not exist) - use simplified query
			s.logger.Debug("Could not determine column type, using simplified stats query",
				zap.String("schema", schemaName),
				zap.String("table", tableName),
				zap.String("column", colName),
				zap.Error(typeErr))
		}

		var query string
		if typeErr == nil && isTextCompatibleType(colType) {
			// Text-compatible type: include length calculation
			query = fmt.Sprintf(`
				SET NOCOUNT ON;
				SELECT
					COUNT(*) as row_count,
					COUNT(%s) as non_null_count,
					COUNT(DISTINCT %s) as distinct_count,
					MIN(LEN(CAST(%s AS NVARCHAR(MAX)))) as min_length,
					MAX(LEN(CAST(%s AS NVARCHAR(MAX)))) as max_length
				FROM %s WITH (NOLOCK)
			`, quotedCol, quotedCol, quotedCol, quotedCol, fullyQualifiedTable)

			row := s.db.QueryRowContext(ctx, query)
			if err := row.Scan(&stat.RowCount, &stat.NonNullCount, &stat.DistinctCount, &stat.MinLength, &stat.MaxLength); err != nil {
				// Query failed - retry with simplified query
				retriedColumns = append(retriedColumns, colName)
				stat = s.analyzeColumnStatsSimplified(ctx, schemaName, tableName, colName, fullyQualifiedTable, err)
			}
		} else {
			// Non-text type or type lookup failed: use simplified query (no length calculation)
			query = fmt.Sprintf(`
				SET NOCOUNT ON;
				SELECT
					COUNT(*) as row_count,
					COUNT(%s) as non_null_count,
					COUNT(DISTINCT %s) as distinct_count
				FROM %s WITH (NOLOCK)
			`, quotedCol, quotedCol, fullyQualifiedTable)

			row := s.db.QueryRowContext(ctx, query)
			if err := row.Scan(&stat.RowCount, &stat.NonNullCount, &stat.DistinctCount); err != nil {
				// Query failed - log warning and use zero values
				s.logger.Warn("Failed to analyze column stats, using zero values",
					zap.String("schema", schemaName),
					zap.String("table", tableName),
					zap.String("column", colName),
					zap.Error(err))
				stat.RowCount = 0
				stat.NonNullCount = 0
				stat.DistinctCount = 0
			}
			// Length stats are nil for non-text types
			stat.MinLength = nil
			stat.MaxLength = nil
		}

		stats = append(stats, stat)
	}

	// Log summary if any columns needed retry
	if len(retriedColumns) > 0 {
		s.logger.Info("Some columns required simplified stats query (no length calculation)",
			zap.String("schema", schemaName),
			zap.String("table", tableName),
			zap.Int("retried_count", len(retriedColumns)),
			zap.Strings("retried_columns", retriedColumns))
	}

	return stats, nil
}

// analyzeColumnStatsSimplified runs a simplified stats query without length calculation.
// Used as a fallback when the main query fails.
func (s *SchemaDiscoverer) analyzeColumnStatsSimplified(ctx context.Context, schemaName, tableName, colName, fullyQualifiedTable string, originalErr error) datasource.ColumnStats {
	quotedCol := quoteName(colName)

	stat := datasource.ColumnStats{
		ColumnName: colName,
	}

	simplifiedQuery := fmt.Sprintf(`
		SET NOCOUNT ON;
		SELECT
			COUNT(*) as row_count,
			COUNT(%s) as non_null_count,
			COUNT(DISTINCT %s) as distinct_count
		FROM %s WITH (NOLOCK)
	`, quotedCol, quotedCol, fullyQualifiedTable)

	row := s.db.QueryRowContext(ctx, simplifiedQuery)
	if retryErr := row.Scan(&stat.RowCount, &stat.NonNullCount, &stat.DistinctCount); retryErr != nil {
		// Both queries failed - log warning and use zero values
		s.logger.Warn("Failed to analyze column stats after retry, using zero values",
			zap.String("schema", schemaName),
			zap.String("table", tableName),
			zap.String("column", colName),
			zap.Error(originalErr),
			zap.NamedError("retry_error", retryErr))
		stat.RowCount = 0
		stat.NonNullCount = 0
		stat.DistinctCount = 0
	}

	// Length stats are nil for retried columns
	stat.MinLength = nil
	stat.MaxLength = nil

	return stat
}

// CheckValueOverlap checks value overlap between two columns (for relationship inference).
func (s *SchemaDiscoverer) CheckValueOverlap(ctx context.Context,
	sourceSchema, sourceTable, sourceColumn,
	targetSchema, targetTable, targetColumn string,
	sampleLimit int) (*datasource.ValueOverlapResult, error) {
	if sampleLimit <= 0 {
		sampleLimit = 1000 // Default sample size
	}

	query := fmt.Sprintf(`
	SET NOCOUNT ON;
	WITH source_sample AS (
	    SELECT DISTINCT TOP (%d) %s AS val
	    FROM %s WITH (NOLOCK)
	    WHERE %s IS NOT NULL
	),
	target_sample AS (
	    SELECT DISTINCT TOP (%d) %s AS val
	    FROM %s WITH (NOLOCK)
	    WHERE %s IS NOT NULL
	),
	overlap AS (
	    SELECT COUNT(*) AS matched
	    FROM source_sample s
	    INNER JOIN target_sample t ON s.val = t.val
	)
	SELECT
	    (SELECT COUNT(*) FROM source_sample) AS source_distinct,
	    (SELECT COUNT(*) FROM target_sample) AS target_distinct,
	    (SELECT matched FROM overlap) AS matched_count
	`,
		sampleLimit, quoteName(sourceColumn),
		buildFullyQualifiedName(sourceSchema, sourceTable),
		quoteName(sourceColumn),
		sampleLimit, quoteName(targetColumn),
		buildFullyQualifiedName(targetSchema, targetTable),
		quoteName(targetColumn),
	)

	var result datasource.ValueOverlapResult
	err := s.db.QueryRowContext(ctx, query).Scan(
		&result.SourceDistinct,
		&result.TargetDistinct,
		&result.MatchedCount,
	)
	if err != nil {
		return nil, fmt.Errorf("query value overlap: %w", err)
	}

	// Calculate match rate
	if result.SourceDistinct > 0 {
		result.MatchRate = float64(result.MatchedCount) / float64(result.SourceDistinct)
	}

	return &result, nil
}

// AnalyzeJoin performs join analysis between two columns (for relationship inference).
func (s *SchemaDiscoverer) AnalyzeJoin(ctx context.Context,
	sourceSchema, sourceTable, sourceColumn,
	targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	query := fmt.Sprintf(`
	SET NOCOUNT ON;
	WITH join_result AS (
	    SELECT
	        src.%s AS src_val,
	        tgt.%s AS tgt_val
	    FROM %s src WITH (NOLOCK)
	    LEFT JOIN %s tgt WITH (NOLOCK)
	        ON src.%s = tgt.%s
	    WHERE src.%s IS NOT NULL
	)
	SELECT
	    COUNT(*) AS join_count,
	    COUNT(DISTINCT CASE WHEN tgt_val IS NOT NULL THEN src_val END) AS source_matched,
	    (SELECT COUNT(DISTINCT %s) FROM %s WITH (NOLOCK) WHERE %s IS NOT NULL) AS target_matched,
	    COUNT(CASE WHEN tgt_val IS NULL THEN 1 END) AS orphan_count
	FROM join_result
	`,
		quoteName(sourceColumn),
		quoteName(targetColumn),
		buildFullyQualifiedName(sourceSchema, sourceTable),
		buildFullyQualifiedName(targetSchema, targetTable),
		quoteName(sourceColumn), quoteName(targetColumn),
		quoteName(sourceColumn),
		quoteName(targetColumn),
		buildFullyQualifiedName(targetSchema, targetTable),
		quoteName(targetColumn),
	)

	var result datasource.JoinAnalysis
	err := s.db.QueryRowContext(ctx, query).Scan(
		&result.JoinCount,
		&result.SourceMatched,
		&result.TargetMatched,
		&result.OrphanCount,
	)
	if err != nil {
		return nil, fmt.Errorf("query join analysis: %w", err)
	}

	return &result, nil
}

// GetDistinctValues returns up to limit distinct non-null values from a column.
// Values are returned as strings, sorted alphabetically.
// Used during the scanning phase to collect sample values for enum detection.
func (s *SchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	query := fmt.Sprintf(`
	SET NOCOUNT ON;
	SELECT DISTINCT TOP (%d) CAST(%s AS NVARCHAR(MAX)) AS val
	FROM %s WITH (NOLOCK)
	WHERE %s IS NOT NULL
	ORDER BY 1
	`,
		limit,
		quoteName(columnName),
		buildFullyQualifiedName(schemaName, tableName),
		quoteName(columnName),
	)

	rows, err := s.db.QueryContext(ctx, query)
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

// Close releases the database connection.
func (s *SchemaDiscoverer) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Ensure SchemaDiscoverer implements datasource.SchemaDiscoverer at compile time.
var _ datasource.SchemaDiscoverer = (*SchemaDiscoverer)(nil)

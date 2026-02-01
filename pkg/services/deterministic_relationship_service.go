package services

// DEPRECATED: This file is scheduled for removal.
// The threshold-based heuristics in this service produce ~90% incorrect relationship inferences.
// Use LLMRelationshipDiscoveryService (relationship_discovery_service.go) instead, which uses
// LLM validation for semantic accuracy.
//
// Migration path:
// - DeterministicRelationshipService → LLMRelationshipDiscoveryService
// - DiscoverFKRelationships → handled by LLMRelationshipDiscoveryService.preserveDBDeclaredFKs
// - DiscoverPKMatchRelationships → handled by RelationshipCandidateCollector + RelationshipValidator
//
// This file will be removed after validation in staging environments confirms the new
// LLM-based approach produces better results.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// FKDiscoveryResult contains the results of FK relationship discovery.
type FKDiscoveryResult struct {
	FKRelationships int `json:"fk_relationships"`
}

// PKMatchDiscoveryResult contains the results of pk_match relationship discovery.
type PKMatchDiscoveryResult struct {
	InferredRelationships int `json:"inferred_relationships"`
}

// RelationshipProgressCallback is called to report progress during relationship discovery.
// Parameters: current (items processed), total (total items), message (human-readable status).
type RelationshipProgressCallback func(current, total int, message string)

// DeterministicRelationshipService discovers entity relationships from FK constraints
// and PK-match inference.
type DeterministicRelationshipService interface {
	// DiscoverFKRelationships discovers relationships from database FK constraints.
	// Requires entities to exist before calling.
	// The progressCallback is called to report progress (can be nil).
	DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback RelationshipProgressCallback) (*FKDiscoveryResult, error)

	// DiscoverPKMatchRelationships discovers relationships via pairwise SQL join testing.
	// Requires entities and column enrichment to exist before calling.
	// The progressCallback is called to report progress (can be nil).
	DiscoverPKMatchRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback RelationshipProgressCallback) (*PKMatchDiscoveryResult, error)

	// GetByProject returns all entity relationships for a project.
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error)
}

type deterministicRelationshipService struct {
	datasourceService DatasourceService
	projectService    ProjectService
	adapterFactory    datasource.DatasourceAdapterFactory
	ontologyRepo      repositories.OntologyRepository
	entityRepo        repositories.OntologyEntityRepository
	relationshipRepo  repositories.EntityRelationshipRepository
	schemaRepo        repositories.SchemaRepository
	logger            *zap.Logger
}

// NewDeterministicRelationshipService creates a new DeterministicRelationshipService.
func NewDeterministicRelationshipService(
	datasourceService DatasourceService,
	projectService ProjectService,
	adapterFactory datasource.DatasourceAdapterFactory,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) DeterministicRelationshipService {
	return &deterministicRelationshipService{
		datasourceService: datasourceService,
		projectService:    projectService,
		adapterFactory:    adapterFactory,
		ontologyRepo:      ontologyRepo,
		entityRepo:        entityRepo,
		relationshipRepo:  relationshipRepo,
		schemaRepo:        schemaRepo,
		logger:            logger.Named("relationship-discovery"),
	}
}

// createBidirectionalRelationship creates both forward and reverse relationship rows.
// The forward relationship is the FK direction (source → target).
// The reverse relationship swaps source and target to enable bidirectional navigation.
func (s *deterministicRelationshipService) createBidirectionalRelationship(ctx context.Context, rel *models.EntityRelationship) error {
	// Create forward relationship
	if err := s.relationshipRepo.Create(ctx, rel); err != nil {
		return fmt.Errorf("create forward relationship: %w", err)
	}

	// Create reverse relationship by swapping source and target
	reverse := &models.EntityRelationship{
		OntologyID:         rel.OntologyID,
		SourceEntityID:     rel.TargetEntityID,     // swap
		TargetEntityID:     rel.SourceEntityID,     // swap
		SourceColumnSchema: rel.TargetColumnSchema, // swap
		SourceColumnTable:  rel.TargetColumnTable,  // swap
		SourceColumnName:   rel.TargetColumnName,   // swap
		SourceColumnID:     rel.TargetColumnID,     // swap
		TargetColumnSchema: rel.SourceColumnSchema, // swap
		TargetColumnTable:  rel.SourceColumnTable,  // swap
		TargetColumnName:   rel.SourceColumnName,   // swap
		TargetColumnID:     rel.SourceColumnID,     // swap
		DetectionMethod:    rel.DetectionMethod,
		Confidence:         rel.Confidence,
		Status:             rel.Status,
		Cardinality:        ReverseCardinality(rel.Cardinality), // swap: N:1 ↔ 1:N
		Description:        nil,                                 // reverse direction gets its own description during enrichment
	}

	// Create reverse relationship
	if err := s.relationshipRepo.Create(ctx, reverse); err != nil {
		return fmt.Errorf("create reverse relationship: %w", err)
	}

	return nil
}

func (s *deterministicRelationshipService) DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback RelationshipProgressCallback) (*FKDiscoveryResult, error) {
	// Load tables and columns
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	tableByName := make(map[string]*models.SchemaTable) // "schema.table" → table
	for _, t := range tables {
		tableByID[t.ID] = t
		tableByName[fmt.Sprintf("%s.%s", t.SchemaName, t.TableName)] = t
	}

	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	columnByID := make(map[uuid.UUID]*models.SchemaColumn)
	for _, c := range columns {
		columnByID[c.ID] = c
	}

	// Get datasource to create schema discoverer for cardinality analysis
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Phase 1: Create SchemaRelationship records from ColumnFeatures (pre-resolved FKs from Phase 4)
	// These are data-driven FK discoveries from ColumnFeatureExtraction
	columnFeaturesCount, err := s.discoverSchemaRelationshipsFromColumnFeatures(
		ctx, projectID, columns, tableByID, tableByName, discoverer, progressCallback,
	)
	if err != nil {
		return nil, fmt.Errorf("discover FK relationships from column features: %w", err)
	}
	if columnFeaturesCount > 0 {
		s.logger.Info("Created SchemaRelationships from ColumnFeatures",
			zap.Int("count", columnFeaturesCount))
	}

	// Phase 2: Update existing schema FK relationships with cardinality analysis
	// These relationships were created during schema import with inference_method='foreign_key'.
	// We update them with computed cardinality from join analysis.
	schemaRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list schema relationships: %w", err)
	}

	var schemaFKCount int
	for i, schemaRel := range schemaRels {
		// Only process FK relationships (skip inferred ones from Phase 1)
		if schemaRel.InferenceMethod != nil && *schemaRel.InferenceMethod != models.InferenceMethodForeignKey {
			continue
		}

		// Resolve source column/table
		sourceCol := columnByID[schemaRel.SourceColumnID]
		sourceTable := tableByID[schemaRel.SourceTableID]
		if sourceCol == nil || sourceTable == nil {
			continue
		}

		// Resolve target column/table
		targetCol := columnByID[schemaRel.TargetColumnID]
		targetTable := tableByID[schemaRel.TargetTableID]
		if targetCol == nil || targetTable == nil {
			continue
		}

		// Self-referential relationships are allowed here - they represent hierarchies/trees
		// (e.g., employee.manager_id → employee.id, category.parent_id → category.id).
		// These come from explicit FK constraints in the schema and are intentional.

		// Compute cardinality from actual data using join analysis
		// This determines if the relationship is 1:1, N:1, 1:N, or N:M
		cardinality := models.CardinalityNTo1 // Default fallback
		joinResult, err := discoverer.AnalyzeJoin(ctx,
			sourceTable.SchemaName, sourceTable.TableName, sourceCol.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)
		if err != nil {
			s.logger.Warn("Failed to analyze FK join for cardinality - using default N:1",
				zap.String("source", fmt.Sprintf("%s.%s.%s", sourceTable.SchemaName, sourceTable.TableName, sourceCol.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s.%s", targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)),
				zap.Error(err))
		} else {
			cardinality = InferCardinality(joinResult)
			s.logger.Debug("Computed FK cardinality from data",
				zap.String("source", fmt.Sprintf("%s.%s.%s", sourceTable.SchemaName, sourceTable.TableName, sourceCol.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s.%s", targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)),
				zap.String("cardinality", cardinality),
				zap.Int64("join_count", joinResult.JoinCount),
				zap.Int64("source_matched", joinResult.SourceMatched),
				zap.Int64("target_matched", joinResult.TargetMatched))
		}

		// Report progress to UI
		if progressCallback != nil {
			msg := fmt.Sprintf("Analyzing FK %s.%s → %s.%s (%d/%d)",
				sourceTable.TableName, sourceCol.ColumnName,
				targetTable.TableName, targetCol.ColumnName,
				i+1, len(schemaRels))
			progressCallback(i+1, len(schemaRels), msg)
		}

		// Update the existing SchemaRelationship with computed cardinality
		schemaRel.Cardinality = cardinality
		schemaRel.IsValidated = true

		if err := s.schemaRepo.UpsertRelationship(ctx, schemaRel); err != nil {
			return nil, fmt.Errorf("update FK relationship cardinality: %w", err)
		}

		schemaFKCount++
	}

	// Collect column stats for pk_match discovery
	// This must happen after FK discovery but before pk_match runs
	if err := s.collectColumnStats(ctx, projectID, datasourceID, tables, columns, progressCallback); err != nil {
		return nil, fmt.Errorf("collect column stats: %w", err)
	}

	// Total relationships: ColumnFeatures-derived + schema FK constraints
	// Note: Upsert semantics mean overlapping relationships are counted only once in the DB,
	// but we report both counts for visibility into the sources.
	totalCount := columnFeaturesCount + schemaFKCount
	s.logger.Info("FK relationship discovery complete",
		zap.Int("from_column_features", columnFeaturesCount),
		zap.Int("from_schema_fks", schemaFKCount),
		zap.Int("total", totalCount))

	return &FKDiscoveryResult{
		FKRelationships: totalCount,
	}, nil
}

// discoverSchemaRelationshipsFromColumnFeatures creates SchemaRelationship records from columns
// where ColumnFeatureExtraction Phase 4 has already resolved FK targets.
// This avoids redundant SQL queries for FKs that were already discovered via data overlap analysis.
// Unlike the old entity-based approach, this writes directly to engine_schema_relationships
// with inference_method='column_features'.
func (s *deterministicRelationshipService) discoverSchemaRelationshipsFromColumnFeatures(
	ctx context.Context,
	projectID uuid.UUID,
	columns []*models.SchemaColumn,
	tableByID map[uuid.UUID]*models.SchemaTable,
	tableByName map[string]*models.SchemaTable,
	discoverer datasource.SchemaDiscoverer,
	progressCallback RelationshipProgressCallback,
) (int, error) {
	// Find columns with pre-resolved FK targets from ColumnFeatureExtraction
	var fkColumns []*models.SchemaColumn
	for _, col := range columns {
		features := col.GetColumnFeatures()
		if features == nil || features.IdentifierFeatures == nil {
			continue
		}
		if features.IdentifierFeatures.FKTargetTable == "" {
			continue
		}
		fkColumns = append(fkColumns, col)
	}

	if len(fkColumns) == 0 {
		return 0, nil
	}

	s.logger.Info("Processing pre-resolved FKs from ColumnFeatures",
		zap.Int("count", len(fkColumns)))

	var createdCount int
	for i, col := range fkColumns {
		features := col.GetColumnFeatures()
		idFeatures := features.IdentifierFeatures

		// Get source table
		sourceTable := tableByID[col.SchemaTableID]
		if sourceTable == nil {
			s.logger.Debug("Source table not found for column",
				zap.String("column_id", col.ID.String()))
			continue
		}

		// Resolve target table from FK target info
		// FKTargetTable may be just the table name or "schema.table"
		targetTableKey := idFeatures.FKTargetTable
		if !strings.Contains(targetTableKey, ".") {
			// Assume same schema as source if not specified
			targetTableKey = fmt.Sprintf("%s.%s", sourceTable.SchemaName, idFeatures.FKTargetTable)
		}

		targetTable := tableByName[targetTableKey]
		if targetTable == nil {
			s.logger.Debug("Target table not found",
				zap.String("target_table", targetTableKey),
				zap.String("source_column", col.ColumnName))
			continue
		}

		// Find target column (use FKTargetColumn if specified, otherwise try to find PK)
		var targetCol *models.SchemaColumn
		targetColName := idFeatures.FKTargetColumn
		if targetColName == "" {
			// Default to "id" if not specified (common convention)
			targetColName = "id"
		}

		// Find the target column in the target table
		for _, c := range columns {
			if c.SchemaTableID == targetTable.ID && c.ColumnName == targetColName {
				targetCol = c
				break
			}
		}
		if targetCol == nil {
			s.logger.Debug("Target column not found",
				zap.String("target_table", targetTableKey),
				zap.String("target_column", targetColName))
			continue
		}

		// Report progress
		if progressCallback != nil {
			msg := fmt.Sprintf("Creating FK from ColumnFeatures: %s.%s → %s.%s (%d/%d)",
				sourceTable.TableName, col.ColumnName,
				targetTable.TableName, targetCol.ColumnName,
				i+1, len(fkColumns))
			progressCallback(i+1, len(fkColumns), msg)
		}

		// Compute cardinality from actual data
		cardinality := models.CardinalityNTo1 // Default for FK relationships
		joinResult, err := discoverer.AnalyzeJoin(ctx,
			sourceTable.SchemaName, sourceTable.TableName, col.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)
		if err != nil {
			s.logger.Debug("Failed to analyze join for cardinality - using default N:1",
				zap.String("source", fmt.Sprintf("%s.%s.%s", sourceTable.SchemaName, sourceTable.TableName, col.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s.%s", targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)),
				zap.Error(err))
		} else {
			cardinality = InferCardinality(joinResult)
		}

		// Use the FK confidence from ColumnFeatures
		confidence := idFeatures.FKConfidence
		if confidence == 0 {
			confidence = 0.9 // Default high confidence for data-driven FK discovery
		}

		// Create SchemaRelationship (unidirectional, source→target)
		inferenceMethod := models.InferenceMethodColumnFeatures
		rel := &models.SchemaRelationship{
			ProjectID:        projectID,
			SourceTableID:    sourceTable.ID,
			SourceColumnID:   col.ID,
			TargetTableID:    targetTable.ID,
			TargetColumnID:   targetCol.ID,
			RelationshipType: models.RelationshipTypeInferred,
			Cardinality:      cardinality,
			Confidence:       confidence,
			InferenceMethod:  &inferenceMethod,
			IsValidated:      true,
		}

		if err := s.schemaRepo.UpsertRelationship(ctx, rel); err != nil {
			return createdCount, fmt.Errorf("upsert SchemaRelationship from ColumnFeatures: %w", err)
		}

		createdCount++
		s.logger.Debug("Created SchemaRelationship from ColumnFeatures",
			zap.String("source", fmt.Sprintf("%s.%s.%s", sourceTable.SchemaName, sourceTable.TableName, col.ColumnName)),
			zap.String("target", fmt.Sprintf("%s.%s.%s", targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)),
			zap.Float64("confidence", confidence),
			zap.String("cardinality", cardinality))
	}

	return createdCount, nil
}

// collectColumnStats analyzes column statistics from the target database
// and determines joinability for each column. This is required for pk_match discovery.
func (s *deterministicRelationshipService) collectColumnStats(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	tables []*models.SchemaTable,
	columns []*models.SchemaColumn,
	progressCallback RelationshipProgressCallback,
) error {
	startTime := time.Now()
	s.logger.Info("Starting column stats collection",
		zap.Int("table_count", len(tables)),
		zap.Int("column_count", len(columns)))

	// Get datasource to create schema discoverer
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Build table lookup and group columns by table
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	columnsByTable := make(map[uuid.UUID][]*models.SchemaColumn)
	for _, t := range tables {
		tableByID[t.ID] = t
	}
	for _, c := range columns {
		columnsByTable[c.SchemaTableID] = append(columnsByTable[c.SchemaTableID], c)
	}

	// Process each table
	tableCount := len(columnsByTable)
	processedTables := 0
	failedTables := 0
	columnsWithStats := 0
	for tableID, tableCols := range columnsByTable {
		table := tableByID[tableID]
		if table == nil {
			continue
		}

		processedTables++
		tableStartTime := time.Now()

		// Get row count for the table (needed for joinability classification)
		var tableRowCount int64
		if table.RowCount != nil {
			tableRowCount = *table.RowCount
		}

		// Get column names for stats query
		columnNames := make([]string, len(tableCols))
		for i, c := range tableCols {
			columnNames[i] = c.ColumnName
		}

		// Analyze column stats from target database
		stats, err := discoverer.AnalyzeColumnStats(ctx, table.SchemaName, table.TableName, columnNames)
		if err != nil {
			s.logger.Error("Failed to analyze column stats - table skipped",
				zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
				zap.Int("column_count", len(columnNames)),
				zap.Error(err))
			failedTables++
			continue
		}

		s.logger.Debug("Analyzed table column stats",
			zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
			zap.Int("columns", len(tableCols)),
			zap.Int("progress", processedTables),
			zap.Int("total", tableCount),
			zap.Duration("duration", time.Since(tableStartTime)))

		// Report progress to UI
		if progressCallback != nil {
			msg := fmt.Sprintf("Analyzing table %s.%s (%d/%d)", table.SchemaName, table.TableName, processedTables, tableCount)
			progressCallback(processedTables, tableCount, msg)
		}

		// Build stats lookup
		statsMap := make(map[string]*datasource.ColumnStats)
		for i := range stats {
			statsMap[stats[i].ColumnName] = &stats[i]
		}

		// Classify joinability and update columns
		for _, col := range tableCols {
			st, found := statsMap[col.ColumnName]
			if found {
				s.logger.Debug("Found stats for column",
					zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
					zap.String("column", col.ColumnName),
					zap.Int64("distinct_count", st.DistinctCount))
			} else {
				// Collect available keys for debugging
				availableKeys := make([]string, 0, len(statsMap))
				for k := range statsMap {
					availableKeys = append(availableKeys, k)
				}
				s.logger.Warn("No stats found for column",
					zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
					zap.String("column", col.ColumnName),
					zap.Strings("available_keys", availableKeys))
			}

			isJoinable, reason := classifyJoinability(col, st, tableRowCount)

			// Prepare stats for update
			var rowCount, nonNullCount, distinctCount *int64
			if st != nil {
				rowCount = &st.RowCount
				nonNullCount = &st.NonNullCount
				distinctCount = &st.DistinctCount
				columnsWithStats++
			}

			// Update column joinability in database
			// Debug: log values being passed to update
			var dcVal int64
			if distinctCount != nil {
				dcVal = *distinctCount
			}
			s.logger.Debug("Updating column joinability",
				zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
				zap.String("column", col.ColumnName),
				zap.Int64("distinct_count_value", dcVal),
				zap.Bool("distinct_count_is_nil", distinctCount == nil))

			if err := s.schemaRepo.UpdateColumnJoinability(ctx, col.ID, rowCount, nonNullCount, distinctCount, &isJoinable, &reason); err != nil {
				s.logger.Warn("Failed to update column joinability",
					zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
					zap.String("column", col.ColumnName),
					zap.Error(err))
				continue
			}
		}
	}

	s.logger.Info("Column stats collection complete",
		zap.Int("tables_processed", processedTables),
		zap.Int("tables_failed", failedTables),
		zap.Int("columns_with_stats", columnsWithStats),
		zap.Duration("total_duration", time.Since(startTime)))

	return nil
}

// classifyJoinability determines if a column is suitable for join key consideration.
func classifyJoinability(col *models.SchemaColumn, stats *datasource.ColumnStats, tableRowCount int64) (bool, string) {
	// Primary keys are always joinable
	if col.IsPrimaryKey {
		return true, models.JoinabilityPK
	}

	// Exclude certain data types
	baseType := normalizeTypeForJoin(col.DataType)
	if isExcludedJoinType(baseType) {
		return false, models.JoinabilityTypeExcluded
	}

	// Need stats for further classification
	if stats == nil || tableRowCount == 0 {
		return false, models.JoinabilityNoStats
	}

	// Check for unique values (potential FK target)
	if stats.DistinctCount == stats.NonNullCount && stats.NonNullCount > 0 {
		return true, models.JoinabilityUniqueValues
	}

	// Check for low cardinality (< 1% distinct)
	distinctRatio := float64(stats.DistinctCount) / float64(tableRowCount)
	if distinctRatio < 0.01 {
		return false, models.JoinabilityLowCardinality
	}

	// Default: joinable if it has reasonable cardinality
	return true, models.JoinabilityCardinalityOK
}

// normalizeTypeForJoin normalizes a column type for join type comparison.
func normalizeTypeForJoin(t string) string {
	t = strings.ToLower(t)
	// Strip length/precision info
	if idx := strings.Index(t, "("); idx > 0 {
		t = t[:idx]
	}
	return strings.TrimSpace(t)
}

// isExcludedJoinType checks if a column type should be excluded from join consideration.
func isExcludedJoinType(baseType string) bool {
	excludedTypes := map[string]bool{
		// Temporal types - dates/times don't represent relationships
		"timestamp": true, "timestamptz": true, "date": true,
		"time": true, "timetz": true, "interval": true,
		// Boolean - too few values, causes false positives
		"boolean": true, "bool": true,
		// Binary/LOB types - not comparable
		"bytea": true, "blob": true, "binary": true,
		// Structured data types - not comparable
		"json": true, "jsonb": true, "xml": true,
		// Geometry types - spatial data not suitable for FK inference
		"point": true, "line": true, "polygon": true, "geometry": true,
	}
	return excludedTypes[baseType]
}

func (s *deterministicRelationshipService) DiscoverPKMatchRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback RelationshipProgressCallback) (*PKMatchDiscoveryResult, error) {
	startTime := time.Now()
	s.logger.Info("Starting PK-match relationship discovery")

	// Load tables and columns
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}

	// Build a map of column IDs that already have SchemaRelationships (from FKDiscovery).
	// This avoids running redundant SQL join analysis for columns we've already processed.
	existingSchemaRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get existing schema relationships: %w", err)
	}
	columnsWithRelationships := make(map[uuid.UUID]bool)
	for _, rel := range existingSchemaRels {
		columnsWithRelationships[rel.SourceColumnID] = true
	}
	s.logger.Debug("Loaded existing schema relationships for deduplication",
		zap.Int("existing_count", len(existingSchemaRels)),
		zap.Int("columns_with_relationships", len(columnsWithRelationships)))

	// Get datasource to create schema discoverer for join analysis
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Build list of "target columns" - columns that could be join targets
	// These are PKs, unique columns, or high cardinality columns
	// (No entity dependency - built from schema metadata)
	type targetColumn struct {
		column *models.SchemaColumn
		table  *models.SchemaTable
	}
	var targetColumns []targetColumn

	for _, col := range columns {
		table, ok := tableByID[col.SchemaTableID]
		if !ok {
			continue
		}

		// Include if: PK, unique, or high cardinality (potential join target)
		isCandidate := col.IsPrimaryKey || col.IsUnique ||
			(col.DistinctCount != nil && *col.DistinctCount >= 20)

		if !isCandidate {
			continue
		}

		// Get column features for exclusion checks
		features := col.GetColumnFeatures()

		// Exclude types unlikely to be join keys
		if isPKMatchExcludedType(col) {
			continue
		}

		// Exclude columns based on stored purpose (timestamp, flag, measure, etc.)
		if features != nil {
			switch features.Purpose {
			case models.PurposeTimestamp, models.PurposeFlag, models.PurposeMeasure, models.PurposeEnum:
				continue
			}
		}

		targetColumns = append(targetColumns, targetColumn{
			column: col,
			table:  table,
		})
	}

	// Build list of FK candidate columns (columns that could reference target columns)
	// Priority: Role=foreign_key or Purpose=identifier from ColumnFeatures
	var priorityCandidates []*pkMatchCandidate
	var regularCandidates []*pkMatchCandidate
	var skippedHighConfidence int
	var skippedExistingRelationship int

	for _, col := range columns {
		table, ok := tableByID[col.SchemaTableID]
		if !ok {
			continue
		}

		// Exclude types unlikely to be entity references
		if isPKMatchExcludedType(col) {
			continue
		}

		// Get column features for purpose-based filtering
		features := col.GetColumnFeatures()

		// Skip columns with high FK confidence (>0.8) from Phase 4 - they already have
		// resolved FK targets and relationships were created in FKDiscovery.
		// This avoids redundant SQL join analysis.
		if features != nil && features.IdentifierFeatures != nil {
			if features.IdentifierFeatures.FKConfidence > 0.8 && features.IdentifierFeatures.FKTargetTable != "" {
				s.logger.Debug("Skipping column with high FK confidence from Phase 4",
					zap.String("table", table.TableName),
					zap.String("column", col.ColumnName),
					zap.Float64("fk_confidence", features.IdentifierFeatures.FKConfidence),
					zap.String("fk_target", features.IdentifierFeatures.FKTargetTable))
				skippedHighConfidence++
				continue
			}
		}

		// Skip columns that already have relationships from FKDiscovery
		if columnsWithRelationships[col.ID] {
			s.logger.Debug("Skipping column with existing relationship",
				zap.String("table", table.TableName),
				zap.String("column", col.ColumnName))
			skippedExistingRelationship++
			continue
		}

		// Exclude columns based on stored purpose (timestamp, flag, measure, etc.)
		if features != nil {
			switch features.Purpose {
			case models.PurposeTimestamp, models.PurposeFlag, models.PurposeMeasure, models.PurposeEnum:
				continue
			}
		}

		// Determine if this column is a FK candidate based on ColumnFeatures
		isFKCandidate := false
		isPriorityCandidate := false

		// Priority: Role=foreign_key indicates a likely FK
		if features != nil && features.Role == models.RoleForeignKey {
			isFKCandidate = true
			isPriorityCandidate = true
		}

		// Also include Purpose=identifier columns (likely FKs even without explicit Role)
		if features != nil && features.Purpose == models.PurposeIdentifier {
			isFKCandidate = true
		}

		// For columns without ColumnFeatures or without explicit Role/Purpose,
		// fall back to joinability check
		if !isFKCandidate {
			if col.IsJoinable == nil || !*col.IsJoinable {
				continue // No joinability info or not joinable = skip
			}
			isFKCandidate = true
		}

		// Apply cardinality filters only if stats exist
		// Identifier columns skip cardinality checks since they are often valid FKs
		// even with low cardinality (e.g., user_id with only a few users)
		skipCardinalityCheck := features != nil && (features.Purpose == models.PurposeIdentifier || features.Role == models.RoleForeignKey)
		if col.DistinctCount != nil && !skipCardinalityCheck {
			// Check cardinality threshold
			if *col.DistinctCount < 20 {
				continue
			}
			// Check cardinality ratio if row count available
			if table.RowCount != nil && *table.RowCount > 0 {
				ratio := float64(*col.DistinctCount) / float64(*table.RowCount)
				if ratio < 0.05 {
					continue
				}
			}
		} else if col.DistinctCount == nil && !skipCardinalityCheck {
			// Columns without stats and without FK indicators proceed to validation
			// Join validation will determine actual FK validity via CheckValueOverlap
			s.logger.Debug("Including column with NULL stats for validation",
				zap.String("table", table.TableName),
				zap.String("column", col.ColumnName),
				zap.Bool("is_joinable", col.IsJoinable != nil && *col.IsJoinable))
		}

		candidate := &pkMatchCandidate{
			column: col,
			schema: table.SchemaName,
			table:  table.TableName,
		}

		if isPriorityCandidate {
			priorityCandidates = append(priorityCandidates, candidate)
		} else {
			regularCandidates = append(regularCandidates, candidate)
		}
	}

	// Combine candidates with priority ones first
	allCandidates := append(priorityCandidates, regularCandidates...)

	s.logger.Info("PK-match discovery setup complete",
		zap.Int("target_columns", len(targetColumns)),
		zap.Int("priority_candidates", len(priorityCandidates)),
		zap.Int("regular_candidates", len(regularCandidates)),
		zap.Int("total_candidates", len(allCandidates)),
		zap.Int("skipped_high_confidence", skippedHighConfidence),
		zap.Int("skipped_existing_relationship", skippedExistingRelationship))

	// For each target column, find candidates with compatible types and test joins
	var inferredCount int
	processedTargets := 0
	for _, target := range targetColumns {
		for _, candidate := range allCandidates {
			// Skip if types are incompatible (handles text ↔ uuid, int variants, etc.)
			if !areTypesCompatibleForFK(candidate.column.DataType, target.column.DataType) {
				continue
			}
			// Skip if same table (self-reference handled differently)
			if candidate.column.SchemaTableID == target.column.SchemaTableID {
				continue
			}

			// Skip if source is a PK - PKs are never FK sources, they are FK targets.
			// A PK column references nothing; other columns reference it.
			if candidate.column.IsPrimaryKey {
				continue
			}

			// Run join analysis
			joinResult, err := discoverer.AnalyzeJoin(ctx,
				candidate.schema, candidate.table, candidate.column.ColumnName,
				target.table.SchemaName, target.table.TableName, target.column.ColumnName)
			if err != nil {
				// Skip this candidate on join error
				continue
			}

			// Real FK relationships have 0% orphans - all source values must exist in target
			if joinResult.OrphanCount > 0 {
				continue
			}

			// Bidirectional validation: Check for false positives where source has few values
			// that coincidentally exist in target. Example:
			// - identity_provider has 3 values {1,2,3}, jobs.id has 83 values {1-83}
			// - Source→target: all 3 exist → 0 orphans → would pass above check
			// - Target→source: 80 values (4-83) don't exist in source → 96% reverse orphans
			// Reject if reverse_orphan_count / target_distinct > 0.5 (>50% of target values are orphans)
			if joinResult.TargetMatched > 0 && joinResult.ReverseOrphanCount > 0 {
				reverseOrphanRate := float64(joinResult.ReverseOrphanCount) / float64(joinResult.TargetMatched+joinResult.ReverseOrphanCount)
				if reverseOrphanRate > 0.5 {
					continue // Too many target values don't exist in source - likely coincidental match
				}
			}

			// Semantic validation: If ALL source values are very small integers (1-10),
			// likely not a real FK relationship (e.g., rating, score, level columns)
			if joinResult.MaxSourceValue != nil && *joinResult.MaxSourceValue <= 10 {
				// Only valid if target table also has <= 10 rows (small lookup table)
				if candidate.column.DistinctCount != nil && *candidate.column.DistinctCount > 10 {
					continue // Source values too small for a real FK to this table
				}
			}

			// Semantic validation: If target column has very low cardinality relative to row count,
			// it's likely a status/type column, not a valid FK target
			if target.column.DistinctCount != nil && target.table.RowCount != nil && *target.table.RowCount > 0 {
				ratio := float64(*target.column.DistinctCount) / float64(*target.table.RowCount)
				if ratio < 0.01 { // Less than 1% unique values
					continue // Likely a status/type column, not a valid FK target
				}
			}

			confidence := 0.9

			// Calculate cardinality from join analysis
			cardinality := InferCardinality(joinResult)

			// Create SchemaRelationship (unidirectional, source→target)
			inferenceMethod := models.InferenceMethodPKMatch
			rel := &models.SchemaRelationship{
				ProjectID:        projectID,
				SourceTableID:    candidate.column.SchemaTableID,
				SourceColumnID:   candidate.column.ID,
				TargetTableID:    target.column.SchemaTableID,
				TargetColumnID:   target.column.ID,
				RelationshipType: models.RelationshipTypeInferred,
				Cardinality:      cardinality,
				Confidence:       confidence,
				InferenceMethod:  &inferenceMethod,
				IsValidated:      true,
			}

			// Compute discovery metrics from join analysis
			// SourceDistinct = matched + orphans (total distinct source values)
			// Since we only save relationships with 0 orphans, MatchRate is 1.0
			sourceDistinct := joinResult.SourceMatched + joinResult.OrphanCount
			var matchRate float64
			if sourceDistinct > 0 {
				matchRate = float64(joinResult.SourceMatched) / float64(sourceDistinct)
			}
			metrics := &models.DiscoveryMetrics{
				MatchRate:      matchRate,
				SourceDistinct: sourceDistinct,
				TargetDistinct: joinResult.TargetMatched,
				MatchedCount:   joinResult.SourceMatched,
			}

			if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
				return nil, fmt.Errorf("upsert pk-match SchemaRelationship: %w", err)
			}

			inferredCount++
			s.logger.Debug("Created pk-match SchemaRelationship",
				zap.String("source", fmt.Sprintf("%s.%s.%s", candidate.schema, candidate.table, candidate.column.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s.%s", target.table.SchemaName, target.table.TableName, target.column.ColumnName)),
				zap.Float64("confidence", confidence),
				zap.String("cardinality", cardinality))
		}

		processedTargets++
		if processedTargets%10 == 0 || processedTargets == len(targetColumns) {
			s.logger.Debug("PK-match discovery progress",
				zap.Int("processed", processedTargets),
				zap.Int("total", len(targetColumns)),
				zap.Int("inferred_so_far", inferredCount))
		}

		// Report progress to UI
		if progressCallback != nil {
			msg := fmt.Sprintf("Testing join candidates (%d/%d)", processedTargets, len(targetColumns))
			progressCallback(processedTargets, len(targetColumns), msg)
		}
	}

	s.logger.Info("PK-match relationship discovery complete",
		zap.Int("inferred_relationships", inferredCount),
		zap.Duration("total_duration", time.Since(startTime)))

	return &PKMatchDiscoveryResult{
		InferredRelationships: inferredCount,
	}, nil
}

func (s *deterministicRelationshipService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return s.relationshipRepo.GetByProject(ctx, projectID)
}

// pkMatchCandidate bundles a column with its table location info for PK-match discovery.
type pkMatchCandidate struct {
	column *models.SchemaColumn
	schema string
	table  string
}

// isPKMatchExcludedType returns true for types unlikely to be entity references.
// For text columns, checks if they have uniform length (likely UUIDs or other ID formats).
func isPKMatchExcludedType(col *models.SchemaColumn) bool {
	lower := strings.ToLower(col.DataType)

	// Boolean types
	if strings.Contains(lower, "bool") {
		return true
	}

	// Timestamp/date types
	if strings.Contains(lower, "timestamp") || strings.Contains(lower, "date") {
		return true
	}

	// Text types - only exclude if proven variable length
	if lower == "text" || strings.Contains(lower, "varchar") || strings.Contains(lower, "char") {
		// Only exclude if we have stats proving variable length
		if col.MinLength != nil && col.MaxLength != nil && *col.MinLength != *col.MaxLength {
			return true // Proven variable length - exclude
		}
		// Either uniform length OR unknown stats - include and let join validation decide
		return false
	}

	// JSON types
	if strings.Contains(lower, "json") {
		return true
	}

	return false
}

// NOTE: isLikelyFKColumn has been removed. Column classification is now handled by the
// column_feature_extraction service. Columns with Purpose=identifier are treated as likely FK columns.

// areTypesCompatibleForFK checks if source and target column types are compatible for FK relationships.
// Supports exact match, UUID compatibility (text ↔ uuid ↔ varchar ↔ character varying),
// and integer compatibility (int ↔ integer ↔ bigint ↔ smallint ↔ serial).
func areTypesCompatibleForFK(sourceType, targetType string) bool {
	source := strings.ToLower(sourceType)
	target := strings.ToLower(targetType)

	// Strip length/precision info (e.g., varchar(255) → varchar)
	if idx := strings.Index(source, "("); idx > 0 {
		source = source[:idx]
	}
	if idx := strings.Index(target, "("); idx > 0 {
		target = target[:idx]
	}
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)

	// Exact match
	if source == target {
		return true
	}

	// UUID compatibility: text, uuid, varchar, character varying can all store UUIDs
	uuidTypes := map[string]bool{
		"uuid":              true,
		"text":              true,
		"varchar":           true,
		"character varying": true,
	}
	if uuidTypes[source] && uuidTypes[target] {
		return true
	}

	// Integer compatibility: int, integer, bigint, smallint, serial variants
	intTypes := map[string]bool{
		"int":       true,
		"int2":      true,
		"int4":      true,
		"int8":      true,
		"integer":   true,
		"bigint":    true,
		"smallint":  true,
		"serial":    true,
		"bigserial": true,
	}
	if intTypes[source] && intTypes[target] {
		return true
	}

	return false
}

// NOTE: isPKMatchExcludedName has been removed. Column classification is now handled by the
// column_feature_extraction service. Columns are excluded based on their stored Purpose
// (timestamp, flag, measure, enum) rather than name patterns.

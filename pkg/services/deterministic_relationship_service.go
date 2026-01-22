package services

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
	adapterFactory datasource.DatasourceAdapterFactory,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) DeterministicRelationshipService {
	return &deterministicRelationshipService{
		datasourceService: datasourceService,
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
		TargetColumnSchema: rel.SourceColumnSchema, // swap
		TargetColumnTable:  rel.SourceColumnTable,  // swap
		TargetColumnName:   rel.SourceColumnName,   // swap
		DetectionMethod:    rel.DetectionMethod,
		Confidence:         rel.Confidence,
		Status:             rel.Status,
		Description:        nil, // reverse direction gets its own description during enrichment
	}

	// Create reverse relationship
	if err := s.relationshipRepo.Create(ctx, reverse); err != nil {
		return fmt.Errorf("create reverse relationship: %w", err)
	}

	return nil
}

func (s *deterministicRelationshipService) DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback RelationshipProgressCallback) (*FKDiscoveryResult, error) {
	// Get active ontology for the project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project")
	}

	// Get all entities for this ontology
	entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}
	if len(entities) == 0 {
		return &FKDiscoveryResult{}, nil // No entities, no relationships to discover
	}

	// entityByPrimaryTable: maps "schema.table" to the entity that owns that table
	entityByPrimaryTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		key := fmt.Sprintf("%s.%s", entity.PrimarySchema, entity.PrimaryTable)
		entityByPrimaryTable[key] = entity
	}

	// Get all schema relationships (FKs) for this datasource
	schemaRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list schema relationships: %w", err)
	}

	// Load tables and columns to resolve IDs to names
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
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
	columnByID := make(map[uuid.UUID]*models.SchemaColumn)
	for _, c := range columns {
		columnByID[c.ID] = c
	}

	// Process each FK from schema relationships
	var fkCount int
	for _, schemaRel := range schemaRels {
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

		// Find source entity by primary table
		sourceKey := fmt.Sprintf("%s.%s", sourceTable.SchemaName, sourceTable.TableName)
		sourceEntity := entityByPrimaryTable[sourceKey]
		if sourceEntity == nil {
			continue // No entity owns this table
		}

		// Find target entity by primary table
		targetKey := fmt.Sprintf("%s.%s", targetTable.SchemaName, targetTable.TableName)
		targetEntity := entityByPrimaryTable[targetKey]
		if targetEntity == nil {
			continue // No entity owns this table
		}

		// Self-referential relationships are allowed here - they represent hierarchies/trees
		// (e.g., employee.manager_id → employee.id, category.parent_id → category.id).
		// These come from explicit FK constraints in the schema and are intentional.

		// Determine detection method based on schema relationship type
		detectionMethod := models.DetectionMethodForeignKey
		if schemaRel.RelationshipType == models.RelationshipTypeManual {
			detectionMethod = models.DetectionMethodManual
		}

		// Create bidirectional relationship (both forward and reverse)
		rel := &models.EntityRelationship{
			OntologyID:         ontology.ID,
			SourceEntityID:     sourceEntity.ID,
			TargetEntityID:     targetEntity.ID,
			SourceColumnSchema: sourceTable.SchemaName,
			SourceColumnTable:  sourceTable.TableName,
			SourceColumnName:   sourceCol.ColumnName,
			TargetColumnSchema: targetTable.SchemaName,
			TargetColumnTable:  targetTable.TableName,
			TargetColumnName:   targetCol.ColumnName,
			DetectionMethod:    detectionMethod,
			Confidence:         1.0,
			Status:             models.RelationshipStatusConfirmed,
		}

		err := s.createBidirectionalRelationship(ctx, rel)
		if err != nil {
			return nil, fmt.Errorf("create bidirectional FK relationship: %w", err)
		}

		fkCount++
	}

	// Collect column stats for pk_match discovery
	// This must happen after FK discovery but before pk_match runs
	if err := s.collectColumnStats(ctx, projectID, datasourceID, tables, columns, progressCallback); err != nil {
		return nil, fmt.Errorf("collect column stats: %w", err)
	}

	return &FKDiscoveryResult{
		FKRelationships: fkCount,
	}, nil
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
			s.logger.Warn("Failed to analyze column stats",
				zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
				zap.Error(err))
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
			st := statsMap[col.ColumnName]

			isJoinable, reason := classifyJoinability(col, st, tableRowCount)

			// Prepare stats for update
			var rowCount, nonNullCount, distinctCount *int64
			if st != nil {
				rowCount = &st.RowCount
				nonNullCount = &st.NonNullCount
				distinctCount = &st.DistinctCount
			}

			// Update column joinability in database
			if err := s.schemaRepo.UpdateColumnJoinability(ctx, col.ID, rowCount, nonNullCount, distinctCount, &isJoinable, &reason); err != nil {
				// Log warning but continue
				continue
			}
		}
	}

	s.logger.Info("Column stats collection complete",
		zap.Int("tables_processed", processedTables),
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

	// Get active ontology for the project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project")
	}

	// Get all entities for this ontology
	entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}
	if len(entities) == 0 {
		return &PKMatchDiscoveryResult{}, nil // No entities, no relationships to discover
	}

	// entityByPrimaryTable: maps "schema.table" to the entity that owns that table
	entityByPrimaryTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		key := fmt.Sprintf("%s.%s", entity.PrimarySchema, entity.PrimaryTable)
		entityByPrimaryTable[key] = entity
	}

	// Load tables and columns
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
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

	// Build list of "entity reference columns" - columns in entity tables that could be
	// join targets (PKs, unique columns, *_id naming, high cardinality)
	type entityRefColumn struct {
		entity *models.OntologyEntity
		column *models.SchemaColumn
		schema string
		table  string
	}
	var entityRefColumns []entityRefColumn

	for _, entity := range entities {
		// Find all columns in this entity's primary table
		for _, col := range columns {
			table, ok := tableByID[col.SchemaTableID]
			if !ok {
				continue
			}
			if table.SchemaName != entity.PrimarySchema || table.TableName != entity.PrimaryTable {
				continue
			}

			// Include if: PK, unique, or passes candidate filters
			isCandidate := col.IsPrimaryKey || col.IsUnique ||
				isEntityReferenceName(col.ColumnName) ||
				(col.DistinctCount != nil && *col.DistinctCount >= 20)

			if !isCandidate {
				continue
			}

			// Exclude types unlikely to be join keys
			if isPKMatchExcludedType(col) {
				continue
			}

			// Exclude names unlikely to be join keys (count, rating, score, etc.)
			if isPKMatchExcludedName(col.ColumnName) {
				continue
			}

			entityRefColumns = append(entityRefColumns, entityRefColumn{
				entity: entity,
				column: col,
				schema: table.SchemaName,
				table:  table.TableName,
			})
		}
	}

	// Build list of filtered FK candidate columns (columns that could reference entity columns)
	// Group by type for efficient matching
	candidatesByType := make(map[string][]*pkMatchCandidate)
	for _, col := range columns {
		table, ok := tableByID[col.SchemaTableID]
		if !ok {
			continue
		}

		// Exclude types unlikely to be entity references
		if isPKMatchExcludedType(col) {
			continue
		}

		// Exclude names unlikely to be entity references
		if isPKMatchExcludedName(col.ColumnName) {
			continue
		}

		// Require explicit joinability determination
		if col.IsJoinable == nil || !*col.IsJoinable {
			continue
		}

		// Require stats to exist (fail-fast on missing data)
		// Note: While IsJoinable=true typically implies stats exist (from classifyJoinability),
		// PK columns can be marked joinable without stats. This defensive check prevents
		// nil pointer access during cardinality filtering.
		if col.DistinctCount == nil {
			continue // No stats = cannot evaluate = skip
		}
		// Check cardinality threshold
		if *col.DistinctCount < 20 {
			continue
		}
		// Check cardinality ratio if row count available
		// Skip this check for likely FK columns (ending in _id, _uuid, _key) since they
		// are often valid FK columns even with low cardinality (e.g., 500 unique visitors
		// in 100,000 rows = 0.5%). Let the actual join validation decide.
		if table.RowCount != nil && *table.RowCount > 0 && !isLikelyFKColumn(col.ColumnName) {
			ratio := float64(*col.DistinctCount) / float64(*table.RowCount)
			if ratio < 0.05 {
				continue
			}
		}

		cwt := &pkMatchCandidate{
			column: col,
			schema: table.SchemaName,
			table:  table.TableName,
		}
		candidatesByType[col.DataType] = append(candidatesByType[col.DataType], cwt)
	}

	// Calculate total candidates for progress logging
	totalCandidates := 0
	for _, candidates := range candidatesByType {
		totalCandidates += len(candidates)
	}
	s.logger.Info("PK-match discovery setup complete",
		zap.Int("entity_ref_columns", len(entityRefColumns)),
		zap.Int("total_candidates", totalCandidates))

	// For each entity reference column, find candidates with matching type and test joins
	var inferredCount int
	processedRefs := 0
	for _, ref := range entityRefColumns {
		refType := ref.column.DataType
		candidates := candidatesByType[refType]

		for _, candidate := range candidates {
			// Skip if same table (self-reference)
			if candidate.schema == ref.schema && candidate.table == ref.table {
				continue
			}

			// Skip PK-to-PK matches (both auto-increment)
			if ref.column.IsPrimaryKey && candidate.column.IsPrimaryKey {
				continue
			}

			// Find source entity for candidate's table (by primary table ownership)
			sourceEntity := entityByPrimaryTable[fmt.Sprintf("%s.%s", candidate.schema, candidate.table)]
			if sourceEntity == nil {
				continue
			}

			// Don't create self-referencing entity relationships
			if sourceEntity.ID == ref.entity.ID {
				continue
			}

			// Run join analysis
			joinResult, err := discoverer.AnalyzeJoin(ctx,
				candidate.schema, candidate.table, candidate.column.ColumnName,
				ref.schema, ref.table, ref.column.ColumnName)
			if err != nil {
				// Skip this candidate on join error
				continue
			}

			// Real FK relationships have 0% orphans - all source values must exist in target
			if joinResult.OrphanCount > 0 {
				continue
			}

			// Semantic validation: If ALL source values are very small integers (1-10),
			// likely not a real FK relationship (e.g., rating, score, level columns)
			if joinResult.MaxSourceValue != nil && *joinResult.MaxSourceValue <= 10 {
				// Only valid if target table also has <= 10 rows (small lookup table)
				if candidate.column.DistinctCount != nil && *candidate.column.DistinctCount > 10 {
					continue // Source values too small for a real FK to this table
				}
			}

			// Semantic validation: If source column has very low cardinality relative to row count,
			// it's likely a status/type column, not a FK
			table := tableByID[ref.column.SchemaTableID]
			if ref.column.DistinctCount != nil && table.RowCount != nil && *table.RowCount > 0 {
				ratio := float64(*ref.column.DistinctCount) / float64(*table.RowCount)
				if ratio < 0.01 { // Less than 1% unique values
					continue // Likely a status/type column, not a FK
				}
			}

			status := models.RelationshipStatusConfirmed
			confidence := 0.9

			// Create bidirectional relationship (both forward and reverse)
			rel := &models.EntityRelationship{
				OntologyID:         ontology.ID,
				SourceEntityID:     sourceEntity.ID,
				TargetEntityID:     ref.entity.ID,
				SourceColumnSchema: candidate.schema,
				SourceColumnTable:  candidate.table,
				SourceColumnName:   candidate.column.ColumnName,
				TargetColumnSchema: ref.schema,
				TargetColumnTable:  ref.table,
				TargetColumnName:   ref.column.ColumnName,
				DetectionMethod:    models.DetectionMethodPKMatch,
				Confidence:         confidence,
				Status:             status,
			}

			err = s.createBidirectionalRelationship(ctx, rel)
			if err != nil {
				return nil, fmt.Errorf("create bidirectional pk-match relationship: %w", err)
			}

			inferredCount++
		}

		processedRefs++
		if processedRefs%10 == 0 || processedRefs == len(entityRefColumns) {
			s.logger.Debug("PK-match discovery progress",
				zap.Int("processed", processedRefs),
				zap.Int("total", len(entityRefColumns)),
				zap.Int("inferred_so_far", inferredCount))
		}

		// Report progress to UI
		if progressCallback != nil {
			msg := fmt.Sprintf("Testing join candidates (%d/%d)", processedRefs, len(entityRefColumns))
			progressCallback(processedRefs, len(entityRefColumns), msg)
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

// isLikelyFKColumn returns true for column names that strongly suggest a foreign key relationship.
// These columns skip the cardinality ratio check because they are likely FK columns even with low
// cardinality (e.g., 500 unique visitors in 100,000 rows = 0.5%).
func isLikelyFKColumn(columnName string) bool {
	lower := strings.ToLower(columnName)

	// Explicit FK patterns
	if strings.HasSuffix(lower, "_id") ||
		strings.HasSuffix(lower, "_uuid") ||
		strings.HasSuffix(lower, "_key") {
		return true
	}

	return false
}

// isPKMatchExcludedName returns true for column names unlikely to be entity references.
func isPKMatchExcludedName(columnName string) bool {
	lower := strings.ToLower(columnName)

	// Timestamp patterns
	if strings.HasSuffix(lower, "_at") || strings.HasSuffix(lower, "_date") {
		return true
	}

	// Boolean flag patterns
	if strings.HasPrefix(lower, "is_") || strings.HasPrefix(lower, "has_") {
		return true
	}

	// Status/type/flag patterns (often low-cardinality enums stored as int)
	if strings.HasSuffix(lower, "_status") || strings.HasSuffix(lower, "_type") || strings.HasSuffix(lower, "_flag") {
		return true
	}

	// Count/amount patterns (expanded)
	if strings.HasPrefix(lower, "num_") || // num_users, num_items
		strings.HasPrefix(lower, "total_") || // total_amount
		strings.HasSuffix(lower, "_count") ||
		strings.HasSuffix(lower, "_amount") ||
		strings.HasSuffix(lower, "_total") ||
		strings.HasSuffix(lower, "_sum") ||
		strings.HasSuffix(lower, "_avg") ||
		strings.HasSuffix(lower, "_min") ||
		strings.HasSuffix(lower, "_max") {
		return true
	}

	// Rating/score/level patterns - use suffix without underscore to catch all variants
	// Catches: rating, user_rating, mod_level, credit_score, etc.
	if strings.HasSuffix(lower, "rating") ||
		strings.HasSuffix(lower, "score") ||
		strings.HasSuffix(lower, "level") {
		return true
	}

	return false
}

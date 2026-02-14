package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// ColumnEnrichmentService provides semantic enrichment for database columns.
// It uses LLM to generate descriptions, semantic types, roles, and enum value mappings.
type ColumnEnrichmentService interface {
	// EnrichTable enriches all columns for a single table.
	EnrichTable(ctx context.Context, projectID uuid.UUID, tableName string) error

	// EnrichProject enriches all tables in a project.
	// Returns the enrichment result with success/failure counts.
	// The progressCallback is called after each table to report progress (can be nil).
	EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback dag.ProgressCallback) (*EnrichColumnsResult, error)
}

// EnrichColumnsResult holds the result of a column enrichment operation.
type EnrichColumnsResult struct {
	TablesEnriched []string          `json:"tables_enriched"`
	TablesFailed   map[string]string `json:"tables_failed,omitempty"`
	DurationMs     int64             `json:"duration_ms"`
}

// TableContext provides table information for column enrichment.
// Replaces the entity concept for v1.0.
type TableContext struct {
	TableName    string
	SchemaName   string
	BusinessName string
	Description  string
	DatasourceID uuid.UUID
}

type columnEnrichmentService struct {
	ontologyRepo       repositories.OntologyRepository
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	conversationRepo   repositories.ConversationRepository
	projectRepo        repositories.ProjectRepository
	questionService    OntologyQuestionService
	dsSvc              DatasourceService
	adapterFactory     datasource.DatasourceAdapterFactory
	llmFactory         llm.LLMClientFactory
	workerPool         *llm.WorkerPool
	circuitBreaker     *llm.CircuitBreaker
	getTenantCtx       TenantContextFunc
	logger             *zap.Logger
}

// NewColumnEnrichmentService creates a new column enrichment service.
func NewColumnEnrichmentService(
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	conversationRepo repositories.ConversationRepository,
	projectRepo repositories.ProjectRepository,
	questionService OntologyQuestionService,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	circuitBreaker *llm.CircuitBreaker,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) ColumnEnrichmentService {
	return &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		conversationRepo:   conversationRepo,
		projectRepo:        projectRepo,
		questionService:    questionService,
		dsSvc:              dsSvc,
		adapterFactory:     adapterFactory,
		llmFactory:         llmFactory,
		workerPool:         workerPool,
		circuitBreaker:     circuitBreaker,
		getTenantCtx:       getTenantCtx,
		logger:             logger.Named("column-enrichment"),
	}
}

var _ ColumnEnrichmentService = (*columnEnrichmentService)(nil)

// EnrichProject enriches all specified tables (or all selected tables if empty).
func (s *columnEnrichmentService) EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback dag.ProgressCallback) (*EnrichColumnsResult, error) {
	startTime := time.Now()
	result := &EnrichColumnsResult{
		TablesEnriched: []string{},
		TablesFailed:   make(map[string]string),
	}

	// If no tableNames provided, fetch all selected tables
	if len(tableNames) == 0 {
		tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, uuid.Nil, true)
		if err != nil {
			return nil, fmt.Errorf("fetch selected tables: %w", err)
		}
		if len(tables) == 0 {
			s.logger.Info("No selected tables to enrich",
				zap.String("project_id", projectID.String()))
			result.DurationMs = time.Since(startTime).Milliseconds()
			return result, nil
		}
		for _, t := range tables {
			tableNames = append(tableNames, t.TableName)
		}
		s.logger.Debug("Fetched tables for enrichment",
			zap.String("project_id", projectID.String()),
			zap.Int("table_count", len(tableNames)))
	}

	// Build work items for parallel processing
	var workItems []llm.WorkItem[string]
	for _, tableName := range tableNames {
		name := tableName // Capture for closure
		workItems = append(workItems, llm.WorkItem[string]{
			ID: name,
			Execute: func(ctx context.Context) (string, error) {
				// Acquire a fresh database connection for this work item to avoid
				// concurrent access issues when multiple workers share the same context.
				// Each worker goroutine needs its own connection since pgx connections
				// are not safe for concurrent use.
				var workCtx context.Context
				var cleanup func()
				if s.getTenantCtx != nil {
					var err error
					workCtx, cleanup, err = s.getTenantCtx(ctx, projectID)
					if err != nil {
						return name, fmt.Errorf("acquire tenant context: %w", err)
					}
					defer cleanup()
				} else {
					// For tests that don't use real database connections
					workCtx = ctx
				}

				if err := s.EnrichTable(workCtx, projectID, name); err != nil {
					return name, err
				}
				return name, nil
			},
		})
	}

	// Process all tables with worker pool
	tableResults := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total,
				fmt.Sprintf("Enriching columns (%d/%d tables)...", completed, total))
		}
	})

	// Aggregate results
	for _, r := range tableResults {
		if r.Err != nil {
			s.logTableFailure(r.ID, "Failed to enrich table", r.Err)
			result.TablesFailed[r.ID] = r.Err.Error()
		} else {
			result.TablesEnriched = append(result.TablesEnriched, r.ID)
		}
	}

	result.DurationMs = time.Since(startTime).Milliseconds()
	return result, nil
}

// EnrichTable enriches all columns for a single table.
func (s *columnEnrichmentService) EnrichTable(ctx context.Context, projectID uuid.UUID, tableName string) error {
	s.logger.Debug("Enriching columns for table",
		zap.String("project_id", projectID.String()),
		zap.String("table", tableName))

	// Get table context (schema, business name, description)
	tableCtx, err := s.getTableContext(ctx, projectID, tableName)
	if err != nil {
		return fmt.Errorf("get table context for %s: %w", tableName, err)
	}

	// Get schema columns for this table
	columns, err := s.getColumnsForTable(ctx, projectID, tableName)
	if err != nil {
		return fmt.Errorf("get columns for table %s: %w", tableName, err)
	}

	if len(columns) == 0 {
		s.logger.Debug("No columns found for table", zap.String("table", tableName))
		return nil
	}

	// Fetch column metadata for all columns in this table
	columnIDs := make([]uuid.UUID, len(columns))
	for i, col := range columns {
		columnIDs[i] = col.ID
	}
	metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, columnIDs)
	if err != nil {
		s.logger.Warn("Failed to fetch column metadata, continuing without",
			zap.String("table", tableName),
			zap.Error(err))
	}
	// Build map for quick lookup
	metadataByColumnID := make(map[uuid.UUID]*models.ColumnMetadata)
	for _, meta := range metadataList {
		metadataByColumnID[meta.SchemaColumnID] = meta
	}

	// Extract FK info from column metadata (populated by column_feature_extraction service)
	fkInfo := make(map[string]string)
	for _, col := range columns {
		if meta, ok := metadataByColumnID[col.ID]; ok {
			if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.FKTargetTable != "" {
				fkInfo[col.ColumnName] = idFeatures.FKTargetTable
			}
		}
	}

	// Identify enum candidates
	enumCandidates := s.identifyEnumCandidates(columns, metadataByColumnID)

	// Sample enum values for likely enum columns
	enumSamples, err := s.sampleEnumValues(ctx, projectID, tableCtx, columns, metadataByColumnID)
	if err != nil {
		s.logger.Warn("Failed to sample enum values, continuing without",
			zap.String("table", tableName),
			zap.Error(err))
		enumSamples = make(map[string][]string)
	}

	// Analyze enum value distributions (count, percentage, state semantics)
	enumDistributions, err := s.analyzeEnumDistributions(ctx, projectID, tableCtx, columns, enumCandidates, metadataByColumnID)
	if err != nil {
		s.logger.Warn("Failed to analyze enum distributions, continuing without",
			zap.String("table", tableName),
			zap.Error(err))
		enumDistributions = make(map[string]*datasource.EnumDistributionResult)
	}

	// Enum definitions are no longer loaded from files (cloud service has no project files)
	var enumDefs []models.EnumDefinition

	// Separate columns into those needing LLM enrichment and those already complete
	// from ColumnFeatureExtraction (high confidence threshold: 0.9)
	columnsNeedingLLM, syntheticEnrichments := s.filterColumnsForLLM(columns, metadataByColumnID)

	s.logger.Debug("Filtered columns for LLM enrichment",
		zap.String("table", tableName),
		zap.Int("total_columns", len(columns)),
		zap.Int("skipped_columns", len(syntheticEnrichments)),
		zap.Int("columns_needing_llm", len(columnsNeedingLLM)))

	var enrichments []columnEnrichment

	// Only call LLM if there are columns that need enrichment
	if len(columnsNeedingLLM) > 0 {
		// Filter FK info and enum samples for columns needing LLM
		filteredFKInfo := make(map[string]string)
		filteredEnumSamples := make(map[string][]string)
		for _, col := range columnsNeedingLLM {
			if target, ok := fkInfo[col.ColumnName]; ok {
				filteredFKInfo[col.ColumnName] = target
			}
			if samples, ok := enumSamples[col.ColumnName]; ok {
				filteredEnumSamples[col.ColumnName] = samples
			}
		}

		// Build and send LLM prompt only for columns that need it
		llmEnrichments, err := s.enrichColumnsWithLLM(ctx, projectID, tableCtx, columnsNeedingLLM, filteredFKInfo, filteredEnumSamples)
		if err != nil {
			return fmt.Errorf("LLM enrichment failed: %w", err)
		}
		enrichments = append(enrichments, llmEnrichments...)
	}

	// Add synthetic enrichments for high-confidence columns
	enrichments = append(enrichments, syntheticEnrichments...)

	// Convert enrichments to ColumnDetail and save, merging enum definitions, distributions, and FK info
	columnDetails := s.convertToColumnDetails(tableName, enrichments, columns, fkInfo, enumSamples, enumDefs, enumDistributions, metadataByColumnID)
	if err := s.ontologyRepo.UpdateColumnDetails(ctx, projectID, tableName, columnDetails); err != nil {
		return fmt.Errorf("save column details: %w", err)
	}

	// Persist sample values for columns with low cardinality (≤50 distinct values)
	if err := s.persistSampleValues(ctx, columns, enumSamples); err != nil {
		s.logger.Warn("Failed to persist sample values, continuing",
			zap.String("table", tableName),
			zap.Error(err))
	}

	s.logger.Info("Enriched columns for table",
		zap.String("table", tableName),
		zap.Int("column_count", len(columnDetails)))

	return nil
}

// getTableContext retrieves table context for column enrichment.
// It looks up the SchemaTable to get business name and description.
func (s *columnEnrichmentService) getTableContext(ctx context.Context, projectID uuid.UUID, tableName string) (*TableContext, error) {
	// Get datasources to find the one containing this table
	datasources, err := s.dsSvc.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list datasources: %w", err)
	}
	if len(datasources) == 0 {
		return nil, fmt.Errorf("no datasource found for project %s", projectID)
	}

	// Find the table in schema tables
	for _, dsStatus := range datasources {
		ds := dsStatus.Datasource
		table, err := s.schemaRepo.FindTableByName(ctx, projectID, ds.ID, tableName)
		if err != nil {
			continue // Try next datasource
		}
		if table != nil {
			tableCtx := &TableContext{
				TableName:    table.TableName,
				SchemaName:   table.SchemaName,
				DatasourceID: ds.ID,
			}
			// Note: BusinessName and Description now live in TableMetadata
			// (engine_ontology_table_metadata), not SchemaTable. Future enhancement
			// could fetch this from TableMetadataRepository if needed for enrichment.
			return tableCtx, nil
		}
	}

	// Table not found in any datasource - create minimal context
	// This allows enrichment to proceed even for tables not yet in schema
	return &TableContext{
		TableName:    tableName,
		DatasourceID: datasources[0].Datasource.ID,
	}, nil
}

// getColumnsForTable retrieves schema columns for a given table name.
func (s *columnEnrichmentService) getColumnsForTable(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.SchemaColumn, error) {
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, []string{tableName}, true)
	if err != nil {
		return nil, err
	}
	return columnsByTable[tableName], nil
}

// NOTE: getForeignKeyInfo function has been removed in v1.0.
// FK information is now extracted from ColumnFeatures.IdentifierFeatures
// populated by the column_feature_extraction service in Phase 2.
// See extractFKInfoFromColumns() for the replacement.

// sampleEnumValues samples distinct values for columns likely to be enums.
func (s *columnEnrichmentService) sampleEnumValues(
	ctx context.Context,
	projectID uuid.UUID,
	tableCtx *TableContext,
	columns []*models.SchemaColumn,
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
) (map[string][]string, error) {
	result := make(map[string][]string)

	// Identify columns likely to be enums
	enumCandidates := s.identifyEnumCandidates(columns, metadataByColumnID)
	if len(enumCandidates) == 0 {
		return result, nil
	}

	// Get datasource for this table
	ds, err := s.getDatasource(ctx, projectID, tableCtx)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer
	adapter, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, ds.ID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Sample each enum candidate
	for _, col := range enumCandidates {
		values, err := adapter.GetDistinctValues(ctx, tableCtx.SchemaName, tableCtx.TableName, col.ColumnName, 50)
		if err != nil {
			s.logger.Debug("Failed to sample values for column, skipping",
				zap.String("column", col.ColumnName),
				zap.Error(err))
			continue
		}
		if len(values) > 0 && len(values) < 50 {
			result[col.ColumnName] = values
		}
	}

	return result, nil
}

// persistSampleValues is a no-op. Sample values are no longer persisted to avoid storing
// target datasource data in the engine database. The function signature is preserved for
// backward compatibility but does nothing.
func (s *columnEnrichmentService) persistSampleValues(_ context.Context, _ []*models.SchemaColumn, _ map[string][]string) error {
	return nil
}

// highConfidenceThreshold is the minimum confidence level for ColumnFeatures
// to skip LLM enrichment. Columns with confidence >= this threshold are considered
// complete enough to not require LLM processing.
const highConfidenceThreshold = 0.9

// filterColumnsForLLM separates columns into those needing LLM enrichment and those
// that already have high-confidence ColumnMetadata from the feature extraction pipeline.
// Returns:
//   - columnsNeedingLLM: columns that should be sent to the LLM for enrichment
//   - syntheticEnrichments: pre-built enrichments for high-confidence columns
func (s *columnEnrichmentService) filterColumnsForLLM(columns []*models.SchemaColumn, metadataByColumnID map[uuid.UUID]*models.ColumnMetadata) ([]*models.SchemaColumn, []columnEnrichment) {
	var columnsNeedingLLM []*models.SchemaColumn
	var syntheticEnrichments []columnEnrichment

	for _, col := range columns {
		meta := metadataByColumnID[col.ID]

		// Column needs LLM enrichment if:
		// - No ColumnMetadata available
		// - Confidence below threshold
		// - Missing description (even if other fields are populated)
		if meta == nil || meta.Confidence == nil || *meta.Confidence < highConfidenceThreshold || meta.Description == nil || *meta.Description == "" {
			columnsNeedingLLM = append(columnsNeedingLLM, col)
			continue
		}

		// Create synthetic enrichment from ColumnMetadata
		enrichment := columnEnrichment{
			Name:        col.ColumnName,
			Description: *meta.Description,
		}
		if meta.SemanticType != nil {
			enrichment.SemanticType = *meta.SemanticType
		}
		if meta.Role != nil {
			enrichment.Role = *meta.Role
		}

		// Copy FK association from IdentifierFeatures
		if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.EntityReferenced != "" {
			assoc := idFeatures.EntityReferenced
			enrichment.FKAssociation = &assoc
		}

		// Copy enum values from EnumFeatures
		if enumFeatures := meta.GetEnumFeatures(); enumFeatures != nil && len(enumFeatures.Values) > 0 {
			for _, cev := range enumFeatures.Values {
				ev := models.EnumValue{
					Value: cev.Value,
					Label: cev.Label,
				}
				if cev.Count > 0 {
					count := cev.Count
					ev.Count = &count
				}
				if cev.Percentage > 0 {
					pct := cev.Percentage
					ev.Percentage = &pct
				}
				enrichment.EnumValues = append(enrichment.EnumValues, ev)
			}
		}

		syntheticEnrichments = append(syntheticEnrichments, enrichment)
	}

	return columnsNeedingLLM, syntheticEnrichments
}

// identifyEnumCandidates identifies columns likely to contain enum values using ColumnMetadata
// from the feature extraction pipeline (DAG step 2). Falls back to low-cardinality text heuristic
// when metadata is not available for a column.
func (s *columnEnrichmentService) identifyEnumCandidates(columns []*models.SchemaColumn, metadataByColumnID map[uuid.UUID]*models.ColumnMetadata) []*models.SchemaColumn {
	var candidates []*models.SchemaColumn
	seen := make(map[uuid.UUID]bool)

	for _, col := range columns {
		if seen[col.ID] {
			continue
		}

		// Primary: use ColumnMetadata from feature extraction
		if meta, ok := metadataByColumnID[col.ID]; ok {
			if meta.ClassificationPath != nil && models.ClassificationPath(*meta.ClassificationPath) == models.ClassificationPathEnum {
				candidates = append(candidates, col)
				seen[col.ID] = true
				continue
			}
			if meta.NeedsEnumAnalysis {
				candidates = append(candidates, col)
				seen[col.ID] = true
				continue
			}
		}

		// Backstop: low cardinality text columns (data-driven, not name-based)
		if col.DistinctCount != nil && *col.DistinctCount < 50 && isTextType(col.DataType) {
			candidates = append(candidates, col)
			seen[col.ID] = true
		}
	}

	return candidates
}

// analyzeEnumDistributions retrieves value distribution for enum columns and identifies state semantics.
// This function enriches enum values with count, percentage, and state classification metadata.
func (s *columnEnrichmentService) analyzeEnumDistributions(
	ctx context.Context,
	projectID uuid.UUID,
	tableCtx *TableContext,
	columns []*models.SchemaColumn,
	enumCandidates []*models.SchemaColumn,
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
) (map[string]*datasource.EnumDistributionResult, error) {
	result := make(map[string]*datasource.EnumDistributionResult)

	if len(enumCandidates) == 0 {
		return result, nil
	}

	// Get datasource for this table
	ds, err := s.getDatasource(ctx, projectID, tableCtx)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer
	adapter, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, ds.ID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Find a completion timestamp column if one exists (for state machine detection)
	completionCol := findCompletionTimestampColumn(columns, metadataByColumnID)

	// Analyze distribution for each enum candidate
	for _, col := range enumCandidates {
		dist, err := adapter.GetEnumValueDistribution(ctx, tableCtx.SchemaName, tableCtx.TableName, col.ColumnName, completionCol, 100)
		if err != nil {
			s.logger.Debug("Failed to get enum distribution for column, skipping",
				zap.String("column", col.ColumnName),
				zap.Error(err))
			continue
		}

		// Only store if we have meaningful distribution data
		if dist != nil && len(dist.Distributions) > 0 && len(dist.Distributions) < 50 {
			result[col.ColumnName] = dist
		}
	}

	return result, nil
}

// findCompletionTimestampColumn finds a column whose ColumnMetadata indicates a "completion"
// timestamp purpose (set by the column_feature_extraction LLM in DAG step 2).
// Returns empty string if no such column is found — the adapter gracefully skips
// completion-rate calculation in that case.
func findCompletionTimestampColumn(columns []*models.SchemaColumn, metadataByColumnID map[uuid.UUID]*models.ColumnMetadata) string {
	for _, col := range columns {
		if meta, ok := metadataByColumnID[col.ID]; ok {
			if tsFeatures := meta.GetTimestampFeatures(); tsFeatures != nil {
				if tsFeatures.TimestampPurpose == models.TimestampPurposeCompletion {
					return col.ColumnName
				}
			}
		}
	}
	return ""
}

// NOTE: detectSoftDeletePattern, monetaryColumnPatterns, and detectMonetaryColumnPattern
// have been removed. Column classification is now handled by the column_feature_extraction
// service in Phase 2 of the DAG. Features are stored in SchemaColumn.Metadata and retrieved
// via GetColumnFeatures(). See PLAN-extracting-column-features.md for details.

// NOTE: UUIDTextColumnDescription, uuidPattern, detectUUIDTextColumnPattern,
// TimestampScaleDescription, detectTimestampScalePattern, isBigintType, inferTimestampScale,
// countDigits, and generateTimestampScaleDescription have been removed.
// Column classification is now handled by the column_feature_extraction service.

// isTimestampType checks if a column data type is a timestamp/datetime type.
// This helper is used by column_feature_extraction.go for classification path routing.
func isTimestampType(dataType string) bool {
	dataTypeLower := strings.ToLower(dataType)
	return strings.Contains(dataTypeLower, "timestamp") ||
		strings.Contains(dataTypeLower, "datetime") ||
		strings.Contains(dataTypeLower, "date")
}

// isTextType checks if a column data type is a text/varchar/char type.
// This helper is still used by column_feature_extraction.go for classification path routing.
func isTextType(dataType string) bool {
	dataTypeLower := strings.ToLower(dataType)
	return strings.Contains(dataTypeLower, "char") ||
		strings.Contains(dataTypeLower, "text") ||
		strings.Contains(dataTypeLower, "varchar")
}

// NOTE: Boolean naming pattern detection (BooleanColumnDescription, booleanPrefixes,
// detectBooleanNamingPattern, isBooleanLikeIntegerColumn, humanizeFeatureName,
// generateBooleanDescription) has been removed. Boolean classification is now handled by the
// column_feature_extraction service in Phase 2. Features are stored in SchemaColumn.Metadata
// and retrieved via GetColumnFeatures(). See BooleanFeatures for true/false meanings.

// isBooleanType checks if a column data type is a boolean type.
// This helper is still used by column_feature_extraction.go for classification path routing.
func isBooleanType(dataType string) bool {
	dataTypeLower := strings.ToLower(dataType)
	return dataTypeLower == "boolean" ||
		dataTypeLower == "bool" ||
		strings.HasPrefix(dataTypeLower, "bit")
}

// NOTE: ExternalIDPatternDescription, ExternalIDPattern, externalIDPatterns, and detectExternalIDPattern
// have been removed. External ID detection is now handled by the column_feature_extraction service
// in Phase 2 (ClassificationPathExternalID). See PLAN-extracting-column-features.md for details.

// NOTE: Role detection from column naming patterns (RolePatterns, RoleDescriptions, DetectedRole,
// detectRoleFromColumnName) has been removed. Role detection is now handled by the
// column_feature_extraction service in Phase 2. The EntityReferenced field in IdentifierFeatures
// captures the semantic role (e.g., "host", "visitor", "payer").

// NOTE: FK column pattern detection (FKColumnDescription, detectFKColumnPattern) has been removed.
// FK detection is now handled by the column_feature_extraction service in Phase 4 (FK resolution).
// The IdentifierFeatures fields (FKTargetTable, FKTargetColumn, EntityReferenced) provide FK metadata.

// applyEnumDistributions merges distribution data into EnumValue structs.
func applyEnumDistributions(enumValues []models.EnumValue, dist *datasource.EnumDistributionResult) []models.EnumValue {
	if dist == nil || len(dist.Distributions) == 0 {
		return enumValues
	}

	// Build a map of value -> distribution for quick lookup
	distMap := make(map[string]datasource.EnumValueDistribution)
	for _, d := range dist.Distributions {
		distMap[d.Value] = d
	}

	// Apply distribution data to each enum value
	for i := range enumValues {
		ev := &enumValues[i]
		if d, ok := distMap[ev.Value]; ok {
			count := d.Count
			percentage := d.Percentage
			ev.Count = &count
			ev.Percentage = &percentage

			// Apply state semantics if detected
			if d.IsLikelyInitialState {
				isTrue := true
				ev.IsLikelyInitialState = &isTrue
			}
			if d.IsLikelyTerminalState {
				isTrue := true
				ev.IsLikelyTerminalState = &isTrue
			}
			if d.IsLikelyErrorState {
				isTrue := true
				ev.IsLikelyErrorState = &isTrue
			}
		}
	}

	return enumValues
}

// getDatasource retrieves the datasource for the table's schema.
func (s *columnEnrichmentService) getDatasource(ctx context.Context, projectID uuid.UUID, tableCtx *TableContext) (*models.Datasource, error) {
	// If we have a specific datasource ID from table context, try to get that one
	if tableCtx.DatasourceID != uuid.Nil {
		datasources, err := s.dsSvc.List(ctx, projectID)
		if err != nil {
			return nil, err
		}
		for _, dsStatus := range datasources {
			if dsStatus.Datasource.ID == tableCtx.DatasourceID {
				if dsStatus.DecryptionFailed {
					return nil, fmt.Errorf("datasource credentials were encrypted with a different key")
				}
				return dsStatus.Datasource, nil
			}
		}
	}

	// Fallback: return the first datasource (most projects have one)
	datasources, err := s.dsSvc.List(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if len(datasources) > 0 {
		if datasources[0].DecryptionFailed {
			return nil, fmt.Errorf("datasource credentials were encrypted with a different key")
		}
		return datasources[0].Datasource, nil
	}

	return nil, fmt.Errorf("no datasource found for project %s", projectID)
}

// columnEnrichmentResponse wraps the LLM response for standardization.
type columnEnrichmentResponse struct {
	Columns   []columnEnrichment            `json:"columns"`
	Questions []columnOntologyQuestionInput `json:"questions,omitempty"`
}

// columnOntologyQuestionInput represents a question generated by the LLM during column enrichment.
// These questions identify areas of uncertainty where user clarification would improve accuracy.
type columnOntologyQuestionInput struct {
	Category string `json:"category"` // terminology | enumeration | relationship | business_rules | temporal | data_quality
	Priority int    `json:"priority"` // 1=critical | 2=important | 3=nice-to-have
	Question string `json:"question"` // Clear question for domain expert
	Context  string `json:"context"`  // Relevant schema/data context
}

// columnEnrichment is the LLM response structure for a single column.
type columnEnrichment struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	SemanticType  string             `json:"semantic_type"`
	Role          string             `json:"role"`
	Synonyms      []string           `json:"synonyms,omitempty"`
	EnumValues    []models.EnumValue `json:"enum_values,omitempty"`
	FKAssociation *string            `json:"fk_association"`
}

// enrichColumnsWithLLM uses the LLM to generate semantic metadata for columns.
// Implements chunking for large tables and retry logic for transient failures.
func (s *columnEnrichmentService) enrichColumnsWithLLM(
	ctx context.Context,
	projectID uuid.UUID,
	tableCtx *TableContext,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
) ([]columnEnrichment, error) {
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Chunk columns if table has many columns to avoid context limits
	const maxColumnsPerChunk = 50
	if len(columns) > maxColumnsPerChunk {
		s.logger.Info("Table has many columns, using chunked enrichment",
			zap.String("table", tableCtx.TableName),
			zap.Int("total_columns", len(columns)),
			zap.Int("chunk_size", maxColumnsPerChunk))
		return s.enrichColumnsInChunks(ctx, projectID, llmClient, tableCtx, columns, fkInfo, enumSamples, maxColumnsPerChunk)
	}

	// Single batch enrichment with retry
	return s.enrichColumnBatch(ctx, projectID, llmClient, tableCtx, columns, fkInfo, enumSamples)
}

// chunkWorkItem holds metadata for a chunk work item.
type chunkWorkItem struct {
	Index int
	Start int
	End   int
}

// enrichColumnsInChunks processes columns in chunks to avoid context limits.
// For tables with >50 columns, chunks are processed in parallel using the worker pool.
func (s *columnEnrichmentService) enrichColumnsInChunks(
	ctx context.Context,
	projectID uuid.UUID,
	llmClient llm.LLMClient,
	tableCtx *TableContext,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
	chunkSize int,
) ([]columnEnrichment, error) {
	// Build work items for each chunk
	var workItems []llm.WorkItem[[]columnEnrichment]
	var chunkMetadata []chunkWorkItem

	for i := 0; i < len(columns); i += chunkSize {
		end := i + chunkSize
		if end > len(columns) {
			end = len(columns)
		}
		chunk := columns[i:end]

		// Filter FK info and enum samples to only include columns in this chunk
		chunkFKInfo := make(map[string]string)
		chunkEnumSamples := make(map[string][]string)
		for _, col := range chunk {
			if target, ok := fkInfo[col.ColumnName]; ok {
				chunkFKInfo[col.ColumnName] = target
			}
			if samples, ok := enumSamples[col.ColumnName]; ok {
				chunkEnumSamples[col.ColumnName] = samples
			}
		}

		// Capture loop variables for closure
		chunkIdx := len(workItems)
		chunkCols := chunk
		chunkFK := chunkFKInfo
		chunkEnum := chunkEnumSamples
		start := i
		chunkEnd := end

		chunkMetadata = append(chunkMetadata, chunkWorkItem{
			Index: chunkIdx,
			Start: start,
			End:   chunkEnd,
		})

		workItems = append(workItems, llm.WorkItem[[]columnEnrichment]{
			ID: fmt.Sprintf("%s-chunk-%d", tableCtx.TableName, chunkIdx),
			Execute: func(ctx context.Context) ([]columnEnrichment, error) {
				s.logger.Debug("Enriching column chunk",
					zap.String("table", tableCtx.TableName),
					zap.Int("chunk_start", start),
					zap.Int("chunk_end", chunkEnd),
					zap.Int("total_columns", len(columns)))

				return s.enrichColumnBatch(ctx, projectID, llmClient, tableCtx, chunkCols, chunkFK, chunkEnum)
			},
		})
	}

	// Build ID -> chunk index map for result reassembly
	chunkIndexByID := make(map[string]int)
	for _, meta := range chunkMetadata {
		chunkIndexByID[fmt.Sprintf("%s-chunk-%d", tableCtx.TableName, meta.Index)] = meta.Index
	}

	// Process chunks in parallel using worker pool
	results := llm.Process(ctx, s.workerPool, workItems, nil)

	// Aggregate results in order (by chunk index)
	// Results come back in completion order, so we need to map them back to chunk index
	resultsByChunk := make(map[int][]columnEnrichment)
	for _, result := range results {
		chunkIdx := chunkIndexByID[result.ID]
		if result.Err != nil {
			meta := chunkMetadata[chunkIdx]
			return nil, fmt.Errorf("chunk %d-%d failed: %w", meta.Start, meta.End, result.Err)
		}
		resultsByChunk[chunkIdx] = result.Result
	}

	// Assemble results in order by chunk index
	var allEnrichments []columnEnrichment
	for i := 0; i < len(workItems); i++ {
		allEnrichments = append(allEnrichments, resultsByChunk[i]...)
	}

	return allEnrichments, nil
}

// enrichColumnBatch enriches a batch of columns with retry logic and circuit breaker protection.
func (s *columnEnrichmentService) enrichColumnBatch(
	ctx context.Context,
	projectID uuid.UUID,
	llmClient llm.LLMClient,
	tableCtx *TableContext,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
) ([]columnEnrichment, error) {
	// Check circuit breaker before attempting LLM call
	allowed, err := s.circuitBreaker.Allow()
	if !allowed {
		s.logger.Error("Circuit breaker prevented LLM call",
			zap.String("table", tableCtx.TableName),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Int("consecutive_failures", s.circuitBreaker.ConsecutiveFailures()),
			zap.Error(err))
		return nil, err
	}

	systemMsg := s.columnEnrichmentSystemMessage()
	prompt := s.buildColumnEnrichmentPrompt(tableCtx, columns, fkInfo, enumSamples)

	// Retry LLM call with exponential backoff
	retryConfig := &retry.Config{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}

	var result *llm.GenerateResponseResult
	err = retry.Do(ctx, retryConfig, func() error {
		var retryErr error
		result, retryErr = llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
		if retryErr != nil {
			// Classify error to determine if retryable
			classified := llm.ClassifyError(retryErr)
			if classified.Retryable {
				s.logger.Warn("LLM call failed, retrying",
					zap.String("table", tableCtx.TableName),
					zap.Int("column_count", len(columns)),
					zap.String("error_type", string(classified.Type)),
					zap.Error(retryErr))
				return retryErr
			}
			// Non-retryable error, fail immediately
			s.logger.Error("LLM call failed with non-retryable error",
				zap.String("table", tableCtx.TableName),
				zap.Int("column_count", len(columns)),
				zap.String("error_type", string(classified.Type)),
				zap.Error(retryErr))
			return retryErr
		}
		return nil
	})

	if err != nil {
		// Record failure in circuit breaker
		s.circuitBreaker.RecordFailure()
		s.logger.Error("Circuit breaker recorded failure",
			zap.String("table", tableCtx.TableName),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Int("consecutive_failures", s.circuitBreaker.ConsecutiveFailures()))
		return nil, fmt.Errorf("LLM call failed after retries: %w", err)
	}

	// Record success in circuit breaker
	s.circuitBreaker.RecordSuccess()

	// Parse response (wrapped in object for standardization)
	response, err := llm.ParseJSONResponse[columnEnrichmentResponse](result.Content)
	if err != nil {
		s.logger.Error("Failed to parse LLM response",
			zap.String("table", tableCtx.TableName),
			zap.Int("column_count", len(columns)),
			zap.String("response_preview", truncateString(result.Content, 200)),
			zap.String("conversation_id", result.ConversationID.String()),
			zap.Error(err))

		// Update conversation status for parse failure
		if s.conversationRepo != nil {
			errorMessage := fmt.Sprintf("parse_failure: %s", err.Error())
			if updateErr := s.conversationRepo.UpdateStatus(ctx, result.ConversationID, models.LLMConversationStatusError, errorMessage); updateErr != nil {
				s.logger.Warn("Failed to update conversation status",
					zap.String("conversation_id", result.ConversationID.String()),
					zap.Error(updateErr))
			}
		}
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	// Store questions generated during enrichment
	if len(response.Questions) > 0 {
		s.logger.Info("LLM generated questions during column enrichment",
			zap.String("table", tableCtx.TableName),
			zap.Int("question_count", len(response.Questions)),
			zap.String("project_id", projectID.String()))

		// Get active ontology for question storage
		ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
		if err != nil {
			s.logger.Error("failed to get active ontology for question storage", zap.Error(err))
			// Non-fatal: continue even if we can't store questions
		} else if ontology != nil && s.questionService != nil {
			questionInputs := make([]OntologyQuestionInput, len(response.Questions))
			for i, q := range response.Questions {
				questionInputs[i] = OntologyQuestionInput{
					Category: q.Category,
					Priority: q.Priority,
					Question: q.Question,
					Context:  q.Context,
					Tables:   []string{tableCtx.TableName},
				}
			}
			questionModels := ConvertQuestionInputs(questionInputs, projectID, ontology.ID, nil)
			if len(questionModels) > 0 {
				if err := s.questionService.CreateQuestions(ctx, questionModels); err != nil {
					s.logger.Error("failed to store ontology questions from column enrichment",
						zap.String("table", tableCtx.TableName),
						zap.Int("question_count", len(questionModels)),
						zap.Error(err))
					// Non-fatal: continue even if question storage fails
				} else {
					s.logger.Debug("Stored ontology questions from column enrichment",
						zap.String("table", tableCtx.TableName),
						zap.Int("question_count", len(questionModels)))
				}
			}
		}
	}

	return response.Columns, nil
}

func (s *columnEnrichmentService) columnEnrichmentSystemMessage() string {
	return `You are a database schema expert. Your task is to analyze database columns and provide semantic metadata that helps AI agents write accurate SQL queries.

Consider the business context of the table when determining column purposes, semantic types, and roles.`
}

func (s *columnEnrichmentService) buildColumnEnrichmentPrompt(
	tableCtx *TableContext,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
) string {
	var sb strings.Builder

	// Table context
	sb.WriteString(fmt.Sprintf("# Table: %s\n", tableCtx.TableName))
	if tableCtx.BusinessName != "" {
		sb.WriteString(fmt.Sprintf("Business Name: \"%s\"", tableCtx.BusinessName))
		if tableCtx.Description != "" {
			sb.WriteString(fmt.Sprintf(" - %s", tableCtx.Description))
		}
		sb.WriteString("\n")
	} else if tableCtx.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", tableCtx.Description))
	}
	sb.WriteString("\n")

	// Columns to analyze
	sb.WriteString("## Columns to Analyze\n")
	sb.WriteString("| Column | Type | PK | FK | Sample Values |\n")
	sb.WriteString("|--------|------|----|----|---------------|\n")

	for _, col := range columns {
		pk := "no"
		if col.IsPrimaryKey {
			pk = "yes"
		}

		fk := "no"
		if targetTable, ok := fkInfo[col.ColumnName]; ok {
			fk = fmt.Sprintf("yes->%s", targetTable)
		}

		samples := "-"
		if vals, ok := enumSamples[col.ColumnName]; ok && len(vals) > 0 {
			// Show up to 5 sample values
			showVals := vals
			if len(showVals) > 5 {
				showVals = showVals[:5]
			}
			samples = strings.Join(showVals, ", ")
			if len(vals) > 5 {
				samples += ", ..."
			}
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			col.ColumnName, col.DataType, pk, fk, samples))
	}

	// FK context for role detection
	s.writeFKContext(&sb, fkInfo)

	// Instructions
	sb.WriteString("\n## For Each Column Provide:\n")
	sb.WriteString("1. **description**: 1 sentence explaining business meaning\n")
	sb.WriteString("2. **semantic_type**: identifier, currency_cents, timestamp_utc, status, count, percentage, email, text, boolean_flag, json, etc.\n")
	sb.WriteString("3. **role**: dimension (for grouping/filtering) | measure (for aggregation) | identifier (unique IDs) | attribute (descriptive)\n")
	sb.WriteString("4. **synonyms**: alternative names users might use (optional array)\n")
	sb.WriteString("5. **enum_values**: for status/type/state columns with sampled values:\n")
	sb.WriteString("   - Return as objects: [{\"value\": \"1\", \"label\": \"Started\"}, ...]\n")
	sb.WriteString("   - Infer labels from column context and common patterns\n")
	sb.WriteString("   - For integer enums, infer meaning from column name (e.g., transaction_state [1,2,3] → Started, Ended, Waiting)\n")
	sb.WriteString("   - For string enums, use the value as label if descriptive (e.g., \"active\" → \"Active\")\n")
	sb.WriteString("   - Include description if you can infer the business meaning\n")
	sb.WriteString("6. **fk_association**: for FK columns, what association does this reference represent?\n")
	sb.WriteString("   Examples: \"owner\", \"creator\", \"assignee\", \"payer\", \"payee\", \"host\", \"visitor\"\n")
	sb.WriteString("   Set to null if it's a generic reference with no special association.\n")

	// Questions section
	sb.WriteString("\n## Questions for Clarification\n\n")
	sb.WriteString("Additionally, identify any areas of uncertainty where user clarification would improve accuracy.\n")
	sb.WriteString("For each uncertainty, provide:\n")
	sb.WriteString("- **category**: terminology | enumeration | relationship | business_rules | temporal | data_quality\n")
	sb.WriteString("- **priority**: 1 (critical) | 2 (important) | 3 (nice-to-have)\n")
	sb.WriteString("- **question**: A clear question for the domain expert\n")
	sb.WriteString("- **context**: Relevant schema/data context\n\n")
	sb.WriteString("Examples of good questions:\n")
	sb.WriteString("- \"What do status values 'A', 'P', 'C' represent?\" (enumeration, priority 1)\n")
	sb.WriteString("- \"Does deleted_at=NULL mean active records?\" (temporal, priority 2)\n")
	sb.WriteString("- \"Column phone has 80% NULL - is this expected?\" (data_quality, priority 3)\n\n")

	// Response format
	sb.WriteString("\n## Response Format (JSON object)\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"columns\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"name\": \"column_name\",\n")
	sb.WriteString("      \"description\": \"Business meaning of this column\",\n")
	sb.WriteString("      \"semantic_type\": \"status\",\n")
	sb.WriteString("      \"role\": \"dimension\",\n")
	sb.WriteString("      \"synonyms\": [\"alt_name1\", \"alt_name2\"],\n")
	sb.WriteString("      \"enum_values\": [{\"value\": \"active\", \"label\": \"Active\"}, {\"value\": \"pending\", \"label\": \"Pending\"}],\n")
	sb.WriteString("      \"fk_association\": null\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"questions\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"category\": \"enumeration\",\n")
	sb.WriteString("      \"priority\": 1,\n")
	sb.WriteString("      \"question\": \"What do status values 'A', 'P', 'C' represent?\",\n")
	sb.WriteString("      \"context\": \"Column status has sampled values: A, P, C - these appear to be abbreviations.\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// writeFKContext adds context about FK columns pointing to the same table.
func (s *columnEnrichmentService) writeFKContext(sb *strings.Builder, fkInfo map[string]string) {
	// Group FK columns by target table
	fkByTarget := make(map[string][]string)
	for col, target := range fkInfo {
		fkByTarget[target] = append(fkByTarget[target], col)
	}

	// Find targets with multiple FK columns (need role differentiation)
	var multipleRoles []string
	for target, cols := range fkByTarget {
		if len(cols) > 1 {
			sort.Strings(cols)
			multipleRoles = append(multipleRoles, fmt.Sprintf("%s (%s)", target, strings.Join(cols, ", ")))
		}
	}

	if len(multipleRoles) > 0 {
		sort.Strings(multipleRoles)
		sb.WriteString("\n## FK Role Context\n")
		sb.WriteString("These columns reference the same table - identify what role each FK represents:\n")
		for _, info := range multipleRoles {
			sb.WriteString(fmt.Sprintf("- %s\n", info))
		}
	}
}

// convertToColumnDetails converts LLM enrichments to ColumnDetail structs.
// It merges project-level enum definitions when available, using them to provide
// accurate descriptions for enum values that the LLM cannot infer.
// It also applies enum distribution metadata (count, percentage, state semantics).
func (s *columnEnrichmentService) convertToColumnDetails(
	tableName string,
	enrichments []columnEnrichment,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
	enumDefs []models.EnumDefinition,
	enumDistributions map[string]*datasource.EnumDistributionResult,
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
) []models.ColumnDetail {
	// Build a map for quick lookup
	enrichmentByName := make(map[string]columnEnrichment)
	for _, e := range enrichments {
		enrichmentByName[e.Name] = e
	}

	// Build column details with schema overlay
	details := make([]models.ColumnDetail, 0, len(columns))
	for _, col := range columns {
		detail := models.ColumnDetail{
			Name:         col.ColumnName,
			IsPrimaryKey: col.IsPrimaryKey,
		}

		// Check FK status from fkInfo
		if targetTable, ok := fkInfo[col.ColumnName]; ok {
			detail.IsForeignKey = true
			detail.ForeignTable = targetTable
		}

		// Overlay enrichment data if available
		if enrichment, ok := enrichmentByName[col.ColumnName]; ok {
			detail.Description = enrichment.Description
			detail.SemanticType = enrichment.SemanticType
			detail.Role = enrichment.Role
			detail.Synonyms = enrichment.Synonyms
			detail.EnumValues = enrichment.EnumValues
			if enrichment.FKAssociation != nil {
				detail.FKAssociation = *enrichment.FKAssociation
			}
		}

		// Apply stored column metadata from the feature extraction pipeline (Phase 2+)
		// These features are populated by the column_feature_extraction service
		// and stored in the ontology column metadata table. Metadata takes precedence
		// over LLM-generated values as they are data-driven and more reliable.
		if meta := metadataByColumnID[col.ID]; meta != nil {
			// Use description from metadata if available and LLM didn't provide one
			if meta.Description != nil && *meta.Description != "" && detail.Description == "" {
				detail.Description = *meta.Description
			}
			// Semantic type from metadata takes precedence as it's data-driven
			if meta.SemanticType != nil && *meta.SemanticType != "" {
				detail.SemanticType = *meta.SemanticType
			}
			// Role from metadata takes precedence
			if meta.Role != nil && *meta.Role != "" {
				detail.Role = *meta.Role
			}

			// Copy EnumFeatures.Values to ColumnDetail.EnumValues if available
			// EnumFeatures from Phase 3 include LLM-generated labels and state categories
			if enumFeatures := meta.GetEnumFeatures(); enumFeatures != nil && len(enumFeatures.Values) > 0 {
				enumValues := make([]models.EnumValue, 0, len(enumFeatures.Values))
				for _, cev := range enumFeatures.Values {
					ev := models.EnumValue{
						Value: cev.Value,
						Label: cev.Label,
					}
					// Map state categories to state flags
					if cev.Count > 0 {
						count := cev.Count
						ev.Count = &count
					}
					if cev.Percentage > 0 {
						pct := cev.Percentage
						ev.Percentage = &pct
					}
					switch cev.Category {
					case "initial":
						isTrue := true
						ev.IsLikelyInitialState = &isTrue
					case "terminal", "terminal_success":
						isTrue := true
						ev.IsLikelyTerminalState = &isTrue
					case "terminal_error":
						isTrue := true
						ev.IsLikelyTerminalState = &isTrue
						ev.IsLikelyErrorState = &isTrue
					}
					enumValues = append(enumValues, ev)
				}
				detail.EnumValues = enumValues
			}

			// Copy IdentifierFeatures.EntityReferenced to FKAssociation
			// This provides semantic role information (e.g., "host", "visitor", "payer")
			if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.EntityReferenced != "" {
				// Only set if not already set by LLM
				if detail.FKAssociation == "" {
					detail.FKAssociation = idFeatures.EntityReferenced
				}
			}

			// Copy FK target info from IdentifierFeatures if available
			if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.FKTargetTable != "" {
				detail.IsForeignKey = true
				detail.ForeignTable = idFeatures.FKTargetTable
			}

			// Copy BooleanFeatures to description if not already set
			// BooleanFeatures from Phase 2 include true/false meaning
			if boolFeatures := meta.GetBooleanFeatures(); boolFeatures != nil && detail.Description == "" {
				var descParts []string
				if boolFeatures.TrueMeaning != "" {
					descParts = append(descParts, fmt.Sprintf("True: %s", boolFeatures.TrueMeaning))
				}
				if boolFeatures.FalseMeaning != "" {
					descParts = append(descParts, fmt.Sprintf("False: %s", boolFeatures.FalseMeaning))
				}
				if len(descParts) > 0 {
					detail.Description = strings.Join(descParts, ". ")
				}
			}

			// Copy TimestampFeatures to semantic type if applicable
			if tsFeatures := meta.GetTimestampFeatures(); tsFeatures != nil {
				if tsFeatures.IsSoftDelete {
					detail.SemanticType = "soft_delete"
				} else if tsFeatures.IsAuditField {
					switch tsFeatures.TimestampPurpose {
					case "audit_created":
						detail.SemanticType = "audit_created"
					case "audit_updated":
						detail.SemanticType = "audit_updated"
					}
				}
			}
		}

		// NOTE: detectBooleanNamingPattern and detectFKColumnPattern fallbacks have been removed.
		// All columns now get their features from the column_feature_extraction service in Phase 2+.
		// Boolean features come from BooleanFeatures, FK features from IdentifierFeatures.

		// Merge project-level enum definitions if available
		// This overrides LLM-inferred enum values with explicit definitions
		if sampledValues, hasSamples := enumSamples[col.ColumnName]; hasSamples && len(enumDefs) > 0 {
			if mergedEnums := s.mergeEnumDefinitions(tableName, col.ColumnName, sampledValues, enumDefs); len(mergedEnums) > 0 {
				// Only override if we have meaningful descriptions from definitions
				hasDescriptions := false
				for _, ev := range mergedEnums {
					if ev.Description != "" || ev.Label != "" {
						hasDescriptions = true
						break
					}
				}
				if hasDescriptions {
					detail.EnumValues = mergedEnums
				}
			}
		}

		// Apply enum distribution metadata (count, percentage, state semantics)
		if dist, hasDist := enumDistributions[col.ColumnName]; hasDist && len(detail.EnumValues) > 0 {
			detail.EnumValues = applyEnumDistributions(detail.EnumValues, dist)
		}

		details = append(details, detail)
	}

	return details
}

// logTableFailure logs detailed information about a failed table enrichment.
func (s *columnEnrichmentService) logTableFailure(
	tableName string,
	reason string,
	err error,
) {
	fields := []zap.Field{
		zap.String("table", tableName),
		zap.String("reason", reason),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
	}

	s.logger.Error("Table enrichment failed", fields...)
}

// mergeEnumDefinitions merges project-level enum definitions with sampled values.
// If a matching definition exists for the column, it creates EnumValue objects
// with value and description from the definition. Otherwise, it falls back to
// sampled values without descriptions.
func (s *columnEnrichmentService) mergeEnumDefinitions(
	tableName string,
	columnName string,
	sampledValues []string,
	enumDefs []models.EnumDefinition,
) []models.EnumValue {
	// Find matching definition
	for _, def := range enumDefs {
		if (def.Table == "*" || def.Table == tableName) && def.Column == columnName {
			// Found a matching definition - create EnumValues with descriptions
			var result []models.EnumValue
			for _, v := range sampledValues {
				ev := models.EnumValue{
					Value: v,
				}
				if desc, ok := def.Values[v]; ok {
					ev.Description = desc
					// If description has format "LABEL - Description", extract label
					if parts := splitEnumDescription(desc); parts != nil {
						ev.Label = parts[0]
						ev.Description = parts[1]
					}
				}
				result = append(result, ev)
			}

			s.logger.Debug("Applied enum definitions",
				zap.String("table", tableName),
				zap.String("column", columnName),
				zap.Int("value_count", len(result)))

			return result
		}
	}

	// No matching definition - fall back to sampled values without descriptions
	return toEnumValues(sampledValues)
}

// splitEnumDescription splits a description in format "LABEL - Description" into parts.
// Returns nil if the description doesn't match this format.
func splitEnumDescription(desc string) []string {
	// Look for " - " separator
	idx := strings.Index(desc, " - ")
	if idx == -1 {
		return nil
	}
	label := strings.TrimSpace(desc[:idx])
	description := strings.TrimSpace(desc[idx+3:])
	if label == "" || description == "" {
		return nil
	}
	return []string{label, description}
}

// toEnumValues converts a slice of strings to EnumValue objects without descriptions.
func toEnumValues(values []string) []models.EnumValue {
	if len(values) == 0 {
		return nil
	}
	result := make([]models.EnumValue, len(values))
	for i, v := range values {
		result[i] = models.EnumValue{Value: v}
	}
	return result
}

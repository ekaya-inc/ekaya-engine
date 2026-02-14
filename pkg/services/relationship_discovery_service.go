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
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// LLMRelationshipDiscoveryResult contains the results of LLM-validated relationship discovery.
type LLMRelationshipDiscoveryResult struct {
	CandidatesEvaluated   int   `json:"candidates_evaluated"`
	RelationshipsCreated  int   `json:"relationships_created"`
	RelationshipsRejected int   `json:"relationships_rejected"`
	PreservedDBFKs        int   `json:"preserved_db_fks"`
	PreservedColumnFKs    int   `json:"preserved_column_fks"`
	DurationMs            int64 `json:"duration_ms"`
}

// LLMRelationshipDiscoveryService orchestrates the full LLM-validated relationship discovery pipeline.
// This is the new implementation that uses LLM validation for semantic accuracy.
// It replaces the threshold-based heuristics that produced ~90% incorrect relationships.
type LLMRelationshipDiscoveryService interface {
	// DiscoverRelationships runs the full discovery pipeline:
	// 1. Preserve existing DB-declared FK relationships (skip LLM)
	// 2. Preserve ColumnFeatures FK relationships with high confidence
	// 3. Collect inference candidates for remaining potential relationships
	// 4. Validate candidates in parallel with worker pool
	// 5. Store validated relationships with LLM-provided cardinality and role
	DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*LLMRelationshipDiscoveryResult, error)
}

type llmRelationshipDiscoveryService struct {
	candidateCollector RelationshipCandidateCollector
	validator          RelationshipValidator
	datasourceService  DatasourceService
	adapterFactory     datasource.DatasourceAdapterFactory
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	logger             *zap.Logger
}

// NewLLMRelationshipDiscoveryService creates a new LLMRelationshipDiscoveryService.
func NewLLMRelationshipDiscoveryService(
	candidateCollector RelationshipCandidateCollector,
	validator RelationshipValidator,
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	logger *zap.Logger,
) LLMRelationshipDiscoveryService {
	return &llmRelationshipDiscoveryService{
		candidateCollector: candidateCollector,
		validator:          validator,
		datasourceService:  datasourceService,
		adapterFactory:     adapterFactory,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		logger:             logger.Named("llm-relationship-discovery"),
	}
}

var _ LLMRelationshipDiscoveryService = (*llmRelationshipDiscoveryService)(nil)

// DiscoverRelationships runs the full LLM-validated relationship discovery pipeline.
// Relationships are stored in engine_schema_relationships (not engine_entity_relationships).
func (s *llmRelationshipDiscoveryService) DiscoverRelationships(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (*LLMRelationshipDiscoveryResult, error) {
	startTime := time.Now()

	result := &LLMRelationshipDiscoveryResult{}

	// Load tables and columns for resolving table/column metadata
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	tableByName := make(map[string]*models.SchemaTable) // "schema.table" and "table" → table
	for _, t := range tables {
		tableByID[t.ID] = t
		tableByName[fmt.Sprintf("%s.%s", t.SchemaName, t.TableName)] = t
		tableByName[t.TableName] = t // Also allow lookup by table name only
	}

	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	columnByID := make(map[uuid.UUID]*models.SchemaColumn)
	var allColumnIDs []uuid.UUID
	for _, c := range columns {
		columnByID[c.ID] = c
		allColumnIDs = append(allColumnIDs, c.ID)
	}

	// Fetch column metadata for feature analysis (if repo is available)
	metadataByColumnID := make(map[uuid.UUID]*models.ColumnMetadata)
	if s.columnMetadataRepo != nil && len(allColumnIDs) > 0 {
		metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, allColumnIDs)
		if err != nil {
			s.logger.Warn("Failed to fetch column metadata, continuing without ColumnFeatures FKs",
				zap.Error(err))
		} else {
			for _, meta := range metadataList {
				metadataByColumnID[meta.SchemaColumnID] = meta
			}
		}
	}

	// Get schema discoverer for join analysis
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Phase 1: Count existing DB-declared FK relationships (these are already stored in schema_relationships)
	// DB-declared FKs are created by schema discovery (inference_method = 'fk'), not this service.
	if progressCallback != nil {
		progressCallback(0, 1, "Preserving DB FKs")
	}

	existingDBFKs, err := s.schemaRepo.GetRelationshipsByMethod(ctx, projectID, datasourceID, models.InferenceMethodForeignKey)
	if err != nil {
		return nil, fmt.Errorf("get existing DB FKs: %w", err)
	}
	result.PreservedDBFKs = len(existingDBFKs)

	s.logger.Info("Found existing DB-declared FK relationships",
		zap.Int("count", result.PreservedDBFKs),
		zap.String("project_id", projectID.String()))

	// Phase 2: Process ColumnFeatures FK relationships with high confidence
	if progressCallback != nil {
		progressCallback(0, 1, "Processing ColumnFeatures FKs")
	}

	preservedColumnFKs, err := s.preserveColumnFeaturesFKs(ctx, projectID, columns, tableByID, tableByName, columnByID, metadataByColumnID, discoverer)
	if err != nil {
		return nil, fmt.Errorf("preserve ColumnFeatures FKs: %w", err)
	}
	result.PreservedColumnFKs = preservedColumnFKs

	s.logger.Info("Preserved ColumnFeatures FK relationships",
		zap.Int("count", preservedColumnFKs),
		zap.String("project_id", projectID.String()))

	// Phase 3: Collect inference candidates for remaining potential relationships
	if progressCallback != nil {
		progressCallback(0, 1, "Collecting relationship candidates")
	}

	candidates, err := s.candidateCollector.CollectCandidates(ctx, projectID, datasourceID, func(current, total int, msg string) {
		if progressCallback != nil {
			// Prefix messages from collector to maintain phase context
			progressCallback(current, total, "Collecting candidates: "+msg)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("collect candidates: %w", err)
	}

	s.logger.Info("Collected relationship candidates",
		zap.Int("count", len(candidates)),
		zap.String("project_id", projectID.String()))

	// Phase 4: Filter out candidates that already have schema relationships
	existingRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get existing relationships: %w", err)
	}
	existingRelSet := s.buildExistingSchemaRelationshipSet(existingRels, tableByID, columnByID)

	var newCandidates []*RelationshipCandidate
	for _, c := range candidates {
		key := fmt.Sprintf("%s.%s->%s.%s", c.SourceTable, c.SourceColumn, c.TargetTable, c.TargetColumn)
		if !existingRelSet[key] {
			newCandidates = append(newCandidates, c)
		}
	}

	s.logger.Info("Filtered candidates to new relationships only",
		zap.Int("original_count", len(candidates)),
		zap.Int("new_count", len(newCandidates)),
		zap.Int("existing_count", len(existingRels)))

	result.CandidatesEvaluated = len(newCandidates)

	// Phase 4: Validate candidates with LLM (if any remain)
	if len(newCandidates) > 0 {
		if progressCallback != nil {
			progressCallback(0, len(newCandidates), "Validating relationships")
		}

		validatedResults, err := s.validator.ValidateCandidates(ctx, projectID, newCandidates, func(current, total int, msg string) {
			if progressCallback != nil {
				progressCallback(current, total, msg)
			}
		})
		if err != nil {
			// Log the error but continue - we may have partial results
			s.logger.Warn("Some relationship validations failed",
				zap.Error(err),
				zap.String("project_id", projectID.String()))
		}

		// Phase 5: Store validated relationships in schema_relationships
		if progressCallback != nil {
			progressCallback(0, 1, "Storing results")
		}

		for _, vr := range validatedResults {
			if vr.Result == nil {
				continue
			}

			if vr.Result.IsValidFK {
				// Create the schema relationship
				if err := s.createSchemaRelationshipFromValidation(ctx, projectID, vr, tableByName); err != nil {
					s.logger.Warn("Failed to create relationship",
						zap.String("source", fmt.Sprintf("%s.%s", vr.Candidate.SourceTable, vr.Candidate.SourceColumn)),
						zap.String("target", fmt.Sprintf("%s.%s", vr.Candidate.TargetTable, vr.Candidate.TargetColumn)),
						zap.Error(err))
					continue
				}
				result.RelationshipsCreated++
			} else {
				result.RelationshipsRejected++
			}
		}
	}

	if progressCallback != nil {
		progressCallback(1, 1, "Discovery complete")
	}

	result.DurationMs = time.Since(startTime).Milliseconds()

	s.logger.Info("LLM relationship discovery complete",
		zap.Int("candidates_evaluated", result.CandidatesEvaluated),
		zap.Int("relationships_created", result.RelationshipsCreated),
		zap.Int("relationships_rejected", result.RelationshipsRejected),
		zap.Int("preserved_db_fks", result.PreservedDBFKs),
		zap.Int("preserved_column_fks", result.PreservedColumnFKs),
		zap.Int64("duration_ms", result.DurationMs),
		zap.String("project_id", projectID.String()))

	return result, nil
}

// buildExistingSchemaRelationshipSet creates a set of existing relationship keys for deduplication.
// Uses table/column names resolved from the provided lookups for consistent key formatting.
func (s *llmRelationshipDiscoveryService) buildExistingSchemaRelationshipSet(
	rels []*models.SchemaRelationship,
	tableByID map[uuid.UUID]*models.SchemaTable,
	columnByID map[uuid.UUID]*models.SchemaColumn,
) map[string]bool {
	set := make(map[string]bool)
	for _, r := range rels {
		sourceTable := tableByID[r.SourceTableID]
		sourceCol := columnByID[r.SourceColumnID]
		targetTable := tableByID[r.TargetTableID]
		targetCol := columnByID[r.TargetColumnID]

		if sourceTable == nil || sourceCol == nil || targetTable == nil || targetCol == nil {
			continue
		}

		key := fmt.Sprintf("%s.%s->%s.%s", sourceTable.TableName, sourceCol.ColumnName, targetTable.TableName, targetCol.ColumnName)
		set[key] = true
	}
	return set
}

// preserveColumnFeaturesFKs creates SchemaRelationship records for columns where Phase 4
// (ColumnFeatureExtraction) already resolved FK targets with high confidence.
// Writes to engine_schema_relationships (not engine_entity_relationships).
func (s *llmRelationshipDiscoveryService) preserveColumnFeaturesFKs(
	ctx context.Context,
	projectID uuid.UUID,
	columns []*models.SchemaColumn,
	tableByID map[uuid.UUID]*models.SchemaTable,
	tableByName map[string]*models.SchemaTable,
	_ map[uuid.UUID]*models.SchemaColumn, // columnByID - unused, columns searched via iteration
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
	discoverer datasource.SchemaDiscoverer,
) (int, error) {
	// Find columns with pre-resolved FK targets from ColumnFeatureExtraction (now in ColumnMetadata)
	type fkColumn struct {
		col        *models.SchemaColumn
		idFeatures *models.IdentifierFeatures
	}
	var fkColumns []fkColumn
	for _, col := range columns {
		meta := metadataByColumnID[col.ID]
		if meta == nil {
			continue
		}
		idFeatures := meta.GetIdentifierFeatures()
		if idFeatures == nil {
			continue
		}
		// Only use high-confidence FK resolutions (>= 0.8)
		if idFeatures.FKTargetTable == "" || idFeatures.FKConfidence < 0.8 {
			continue
		}
		fkColumns = append(fkColumns, fkColumn{col: col, idFeatures: idFeatures})
	}

	var createdCount int
	for _, fk := range fkColumns {
		col := fk.col
		idFeatures := fk.idFeatures

		// Get source table
		sourceTable := tableByID[col.SchemaTableID]
		if sourceTable == nil {
			continue
		}

		// Resolve target table
		targetTableKey := idFeatures.FKTargetTable
		if !strings.Contains(targetTableKey, ".") {
			// Assume same schema as source if not specified
			targetTableKey = fmt.Sprintf("%s.%s", sourceTable.SchemaName, idFeatures.FKTargetTable)
		}

		targetTable := tableByName[targetTableKey]
		if targetTable == nil {
			// Try without schema
			targetTable = tableByName[idFeatures.FKTargetTable]
		}
		if targetTable == nil {
			continue
		}

		// Find target column
		targetColName := idFeatures.FKTargetColumn
		if targetColName == "" {
			targetColName = "id" // Default convention
		}

		var targetCol *models.SchemaColumn
		for _, c := range columns {
			if c.SchemaTableID == targetTable.ID && c.ColumnName == targetColName {
				targetCol = c
				break
			}
		}
		if targetCol == nil {
			continue
		}

		// Compute cardinality from actual data
		cardinality := models.CardinalityNTo1 // Default
		joinResult, err := discoverer.AnalyzeJoin(ctx,
			sourceTable.SchemaName, sourceTable.TableName, col.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)
		if err == nil {
			cardinality = InferCardinality(joinResult)
		}

		inferenceMethod := models.InferenceMethodColumnFeatures
		rel := &models.SchemaRelationship{
			ProjectID:        projectID,
			SourceTableID:    sourceTable.ID,
			SourceColumnID:   col.ID,
			TargetTableID:    targetTable.ID,
			TargetColumnID:   targetCol.ID,
			RelationshipType: models.RelationshipTypeInferred,
			Cardinality:      cardinality,
			Confidence:       idFeatures.FKConfidence,
			InferenceMethod:  &inferenceMethod,
			IsValidated:      true,
		}

		// Build metrics for upsert
		var sourceDistinct, targetDistinct int64
		if col.DistinctCount != nil {
			sourceDistinct = *col.DistinctCount
		}
		if targetCol.DistinctCount != nil {
			targetDistinct = *targetCol.DistinctCount
		}
		metrics := &models.DiscoveryMetrics{
			MatchRate:      idFeatures.FKConfidence,
			SourceDistinct: sourceDistinct,
			TargetDistinct: targetDistinct,
		}

		if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
			s.logger.Warn("Failed to create ColumnFeatures FK relationship",
				zap.String("source", fmt.Sprintf("%s.%s", sourceTable.TableName, col.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s", targetTable.TableName, targetCol.ColumnName)),
				zap.Error(err))
			continue
		}
		createdCount++
	}

	return createdCount, nil
}

// createSchemaRelationshipFromValidation creates a SchemaRelationship from a validated candidate.
// Writes to engine_schema_relationships (not engine_entity_relationships).
func (s *llmRelationshipDiscoveryService) createSchemaRelationshipFromValidation(
	ctx context.Context,
	projectID uuid.UUID,
	vr *ValidatedRelationship,
	tableByName map[string]*models.SchemaTable,
) error {
	candidate := vr.Candidate
	result := vr.Result

	// Resolve table IDs from table lookup
	sourceTable := tableByName[candidate.SourceTable]
	targetTable := tableByName[candidate.TargetTable]

	if sourceTable == nil || targetTable == nil {
		return fmt.Errorf("missing table: source=%s (%v), target=%s (%v)",
			candidate.SourceTable, sourceTable != nil,
			candidate.TargetTable, targetTable != nil)
	}

	// Compute cardinality from actual join data, not LLM response.
	// The LLM tends to default to "N:1" without analyzing join statistics.
	// InferCardinality uses the ratio of JoinCount to matched values to determine
	// the actual relationship cardinality (1:1, N:1, 1:N, or N:M).
	joinAnalysis := &datasource.JoinAnalysis{
		JoinCount:     candidate.JoinCount,
		SourceMatched: candidate.SourceMatched,
		TargetMatched: candidate.TargetMatched,
	}
	cardinality := InferCardinality(joinAnalysis)

	inferenceMethod := models.InferenceMethodPKMatch
	rel := &models.SchemaRelationship{
		ProjectID:        projectID,
		SourceTableID:    sourceTable.ID,
		SourceColumnID:   candidate.SourceColumnID,
		TargetTableID:    targetTable.ID,
		TargetColumnID:   candidate.TargetColumnID,
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      cardinality,
		Confidence:       result.Confidence,
		InferenceMethod:  &inferenceMethod,
		IsValidated:      true,
	}

	// Build metrics from candidate join analysis
	metrics := &models.DiscoveryMetrics{
		SourceDistinct: int64(candidate.SourceDistinctCount),
		TargetDistinct: int64(candidate.TargetDistinctCount),
		MatchedCount:   int64(candidate.SourceMatched),
	}
	if candidate.SourceDistinctCount > 0 {
		metrics.MatchRate = float64(candidate.SourceMatched) / float64(candidate.SourceDistinctCount)
	}

	return s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics)
}

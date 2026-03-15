package services

import (
	"context"
	"fmt"
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
	for _, c := range columns {
		columnByID[c.ID] = c
	}

	// Phase 1: Count existing DB-declared FK relationships (these are already stored in schema_relationships)
	// DB-declared FKs are created by schema sync (inference_method = 'fk'), not this service.
	if progressCallback != nil {
		progressCallback(0, 1, "Preserving DB FKs")
	}

	existingDBFKs, err := s.schemaRepo.GetRelationshipsByMethod(ctx, projectID, datasourceID, models.InferenceMethodFK)
	if err != nil {
		return nil, fmt.Errorf("get existing DB FKs: %w", err)
	}
	result.PreservedDBFKs = len(existingDBFKs)

	s.logger.Info("Found existing DB-declared FK relationships",
		zap.Int("count", result.PreservedDBFKs),
		zap.String("project_id", projectID.String()))

	// Phase 2: Count existing ColumnFeatures relationships created during FKDiscovery.
	if progressCallback != nil {
		progressCallback(0, 1, "Loading ColumnFeatures FKs")
	}

	existingColumnFeatureFKs, err := s.schemaRepo.GetRelationshipsByMethod(ctx, projectID, datasourceID, models.InferenceMethodColumnFeatures)
	if err != nil {
		return nil, fmt.Errorf("get existing ColumnFeatures FKs: %w", err)
	}
	result.PreservedColumnFKs = len(existingColumnFeatureFKs)

	s.logger.Info("Loaded existing ColumnFeatures FK relationships",
		zap.Int("count", result.PreservedColumnFKs),
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
				if err := s.createSchemaRelationshipFromValidation(ctx, projectID, vr, tableByName, columnByID); err != nil {
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

// createSchemaRelationshipFromValidation creates a SchemaRelationship from a validated candidate.
// Writes to engine_schema_relationships (not engine_entity_relationships).
func (s *llmRelationshipDiscoveryService) createSchemaRelationshipFromValidation(
	ctx context.Context,
	projectID uuid.UUID,
	vr *ValidatedRelationship,
	tableByName map[string]*models.SchemaTable,
	columnByID map[uuid.UUID]*models.SchemaColumn,
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

	// Use the validator's deterministically computed cardinality.
	// The validator computes cardinality from schema constraints (PK/unique)
	// which is authoritative for FK relationships:
	// - Non-unique, non-PK source → always N:1 (the standard FK pattern)
	// - Unique/PK source with 1:1 mapping → 1:1
	cardinality := result.Cardinality

	inferenceMethod := models.InferenceMethodRelationshipDiscovery
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

	if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
		return err
	}

	sourceColumn := columnByID[candidate.SourceColumnID]
	targetColumn := columnByID[candidate.TargetColumnID]
	return reconcileRelationshipBackedColumnMetadata(
		ctx,
		s.columnMetadataRepo,
		projectID,
		sourceColumn,
		targetTable,
		targetColumn,
		result.Confidence,
	)
}

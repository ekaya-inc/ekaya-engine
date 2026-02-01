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
	ontologyRepo       repositories.OntologyRepository
	entityRepo         repositories.OntologyEntityRepository
	relationshipRepo   repositories.EntityRelationshipRepository
	schemaRepo         repositories.SchemaRepository
	logger             *zap.Logger
}

// NewLLMRelationshipDiscoveryService creates a new LLMRelationshipDiscoveryService.
func NewLLMRelationshipDiscoveryService(
	candidateCollector RelationshipCandidateCollector,
	validator RelationshipValidator,
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) LLMRelationshipDiscoveryService {
	return &llmRelationshipDiscoveryService{
		candidateCollector: candidateCollector,
		validator:          validator,
		datasourceService:  datasourceService,
		adapterFactory:     adapterFactory,
		ontologyRepo:       ontologyRepo,
		entityRepo:         entityRepo,
		relationshipRepo:   relationshipRepo,
		schemaRepo:         schemaRepo,
		logger:             logger.Named("llm-relationship-discovery"),
	}
}

var _ LLMRelationshipDiscoveryService = (*llmRelationshipDiscoveryService)(nil)

// DiscoverRelationships runs the full LLM-validated relationship discovery pipeline.
func (s *llmRelationshipDiscoveryService) DiscoverRelationships(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (*LLMRelationshipDiscoveryResult, error) {
	startTime := time.Now()

	result := &LLMRelationshipDiscoveryResult{}

	// Get active ontology for the project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project %s", projectID)
	}

	// Load entities for entity resolution
	entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}
	entityByTable := s.buildEntityByTableMap(entities)

	// Load tables and columns for resolving table/column metadata
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
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

	// Phase 1: Preserve DB-declared FK relationships (these are always valid)
	if progressCallback != nil {
		progressCallback(0, 100, "Processing DB-declared FK relationships")
	}

	preservedDBFKs, err := s.preserveDBDeclaredFKs(ctx, ontology.ID, projectID, datasourceID, tableByID, columnByID, entityByTable, discoverer)
	if err != nil {
		return nil, fmt.Errorf("preserve DB-declared FKs: %w", err)
	}
	result.PreservedDBFKs = preservedDBFKs

	s.logger.Info("Preserved DB-declared FK relationships",
		zap.Int("count", preservedDBFKs),
		zap.String("project_id", projectID.String()))

	// Phase 2: Preserve ColumnFeatures FK relationships with high confidence
	if progressCallback != nil {
		progressCallback(10, 100, "Processing ColumnFeatures FK relationships")
	}

	preservedColumnFKs, err := s.preserveColumnFeaturesFKs(ctx, ontology.ID, projectID, columns, tableByID, tableByName, columnByID, entityByTable, discoverer)
	if err != nil {
		return nil, fmt.Errorf("preserve ColumnFeatures FKs: %w", err)
	}
	result.PreservedColumnFKs = preservedColumnFKs

	s.logger.Info("Preserved ColumnFeatures FK relationships",
		zap.Int("count", preservedColumnFKs),
		zap.String("project_id", projectID.String()))

	// Phase 3: Collect inference candidates for remaining potential relationships
	if progressCallback != nil {
		progressCallback(20, 100, "Collecting relationship candidates")
	}

	candidates, err := s.candidateCollector.CollectCandidates(ctx, projectID, datasourceID, func(current, total int, msg string) {
		// Map collector progress (0-100%) to 20-50% of overall progress
		overallProgress := 20 + (current * 30 / max(total, 1))
		if progressCallback != nil {
			progressCallback(overallProgress, 100, msg)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("collect candidates: %w", err)
	}

	s.logger.Info("Collected relationship candidates",
		zap.Int("count", len(candidates)),
		zap.String("project_id", projectID.String()))

	// Phase 4: Filter out candidates that already have relationships (from Phase 1 & 2)
	existingRels, err := s.relationshipRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		return nil, fmt.Errorf("get existing relationships: %w", err)
	}
	existingRelSet := s.buildExistingRelationshipSet(existingRels)

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

	// Phase 5: Validate candidates with LLM (if any remain)
	if len(newCandidates) > 0 {
		if progressCallback != nil {
			progressCallback(50, 100, "Validating relationship candidates with LLM")
		}

		validatedResults, err := s.validator.ValidateCandidates(ctx, projectID, newCandidates, func(current, total int, msg string) {
			// Map validator progress (0-100%) to 50-90% of overall progress
			overallProgress := 50 + (current * 40 / max(total, 1))
			if progressCallback != nil {
				progressCallback(overallProgress, 100, msg)
			}
		})
		if err != nil {
			// Log the error but continue - we may have partial results
			s.logger.Warn("Some relationship validations failed",
				zap.Error(err),
				zap.String("project_id", projectID.String()))
		}

		// Phase 6: Store validated relationships
		if progressCallback != nil {
			progressCallback(90, 100, "Storing validated relationships")
		}

		for _, vr := range validatedResults {
			if vr.Result == nil {
				continue
			}

			if vr.Result.IsValidFK {
				// Create the relationship
				if err := s.createRelationshipFromValidation(ctx, ontology.ID, vr, entityByTable, columnByID, tableByName); err != nil {
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
		progressCallback(100, 100, "Relationship discovery complete")
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

// buildEntityByTableMap creates a map from table name to entity for quick lookup.
func (s *llmRelationshipDiscoveryService) buildEntityByTableMap(entities []*models.OntologyEntity) map[string]*models.OntologyEntity {
	entityByTable := make(map[string]*models.OntologyEntity)
	for _, e := range entities {
		if e.PrimaryTable != "" {
			entityByTable[e.PrimaryTable] = e
		}
	}
	return entityByTable
}

// buildExistingRelationshipSet creates a set of existing relationship keys for deduplication.
func (s *llmRelationshipDiscoveryService) buildExistingRelationshipSet(rels []*models.EntityRelationship) map[string]bool {
	set := make(map[string]bool)
	for _, r := range rels {
		key := fmt.Sprintf("%s.%s->%s.%s", r.SourceColumnTable, r.SourceColumnName, r.TargetColumnTable, r.TargetColumnName)
		set[key] = true
	}
	return set
}

// preserveDBDeclaredFKs creates EntityRelationship records for database-declared FK constraints.
// These are always valid (they come from the database schema) and skip LLM validation.
func (s *llmRelationshipDiscoveryService) preserveDBDeclaredFKs(
	ctx context.Context,
	ontologyID, projectID, datasourceID uuid.UUID,
	tableByID map[uuid.UUID]*models.SchemaTable,
	columnByID map[uuid.UUID]*models.SchemaColumn,
	entityByTable map[string]*models.OntologyEntity,
	discoverer datasource.SchemaDiscoverer,
) (int, error) {
	// Get schema relationships with inference_method = 'foreign_key'
	schemaRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return 0, fmt.Errorf("list schema relationships: %w", err)
	}

	var createdCount int
	for _, schemaRel := range schemaRels {
		// Only process FK relationships (skip inferred ones)
		if schemaRel.InferenceMethod == nil || *schemaRel.InferenceMethod != models.InferenceMethodForeignKey {
			continue
		}

		sourceCol := columnByID[schemaRel.SourceColumnID]
		sourceTable := tableByID[schemaRel.SourceTableID]
		targetCol := columnByID[schemaRel.TargetColumnID]
		targetTable := tableByID[schemaRel.TargetTableID]

		if sourceCol == nil || sourceTable == nil || targetCol == nil || targetTable == nil {
			continue
		}

		// Resolve entities
		sourceEntity := entityByTable[sourceTable.TableName]
		targetEntity := entityByTable[targetTable.TableName]

		if sourceEntity == nil || targetEntity == nil {
			s.logger.Debug("Skipping FK - missing entity for table",
				zap.String("source_table", sourceTable.TableName),
				zap.String("target_table", targetTable.TableName),
				zap.Bool("source_entity_found", sourceEntity != nil),
				zap.Bool("target_entity_found", targetEntity != nil))
			continue
		}

		// Compute cardinality from actual data
		cardinality := models.CardinalityNTo1 // Default
		joinResult, err := discoverer.AnalyzeJoin(ctx,
			sourceTable.SchemaName, sourceTable.TableName, sourceCol.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)
		if err == nil {
			cardinality = InferCardinality(joinResult)
		}

		rel := &models.EntityRelationship{
			OntologyID:         ontologyID,
			SourceEntityID:     sourceEntity.ID,
			TargetEntityID:     targetEntity.ID,
			SourceColumnSchema: sourceTable.SchemaName,
			SourceColumnTable:  sourceTable.TableName,
			SourceColumnName:   sourceCol.ColumnName,
			SourceColumnID:     &sourceCol.ID,
			TargetColumnSchema: targetTable.SchemaName,
			TargetColumnTable:  targetTable.TableName,
			TargetColumnName:   targetCol.ColumnName,
			TargetColumnID:     &targetCol.ID,
			DetectionMethod:    models.DetectionMethodForeignKey,
			Confidence:         1.0, // DB-declared FKs have maximum confidence
			Status:             models.RelationshipStatusConfirmed,
			Cardinality:        cardinality,
			Source:             models.SourceInferred.String(),
		}

		if err := s.relationshipRepo.Create(ctx, rel); err != nil {
			s.logger.Warn("Failed to create FK relationship",
				zap.String("source", fmt.Sprintf("%s.%s", sourceTable.TableName, sourceCol.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s", targetTable.TableName, targetCol.ColumnName)),
				zap.Error(err))
			continue
		}
		createdCount++
	}

	return createdCount, nil
}

// preserveColumnFeaturesFKs creates EntityRelationship records for columns where Phase 4
// (ColumnFeatureExtraction) already resolved FK targets with high confidence.
func (s *llmRelationshipDiscoveryService) preserveColumnFeaturesFKs(
	ctx context.Context,
	ontologyID uuid.UUID,
	_ uuid.UUID, // projectID - unused, available if needed for future extensions
	columns []*models.SchemaColumn,
	tableByID map[uuid.UUID]*models.SchemaTable,
	tableByName map[string]*models.SchemaTable,
	_ map[uuid.UUID]*models.SchemaColumn, // columnByID - unused, columns searched via iteration
	entityByTable map[string]*models.OntologyEntity,
	discoverer datasource.SchemaDiscoverer,
) (int, error) {
	// Find columns with pre-resolved FK targets from ColumnFeatureExtraction
	var fkColumns []*models.SchemaColumn
	for _, col := range columns {
		features := col.GetColumnFeatures()
		if features == nil || features.IdentifierFeatures == nil {
			continue
		}
		// Only use high-confidence FK resolutions (>= 0.8)
		if features.IdentifierFeatures.FKTargetTable == "" || features.IdentifierFeatures.FKConfidence < 0.8 {
			continue
		}
		fkColumns = append(fkColumns, col)
	}

	var createdCount int
	for _, col := range fkColumns {
		features := col.GetColumnFeatures()
		idFeatures := features.IdentifierFeatures

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

		// Resolve entities
		sourceEntity := entityByTable[sourceTable.TableName]
		targetEntity := entityByTable[targetTable.TableName]

		if sourceEntity == nil || targetEntity == nil {
			s.logger.Debug("Skipping ColumnFeatures FK - missing entity for table",
				zap.String("source_table", sourceTable.TableName),
				zap.String("target_table", targetTable.TableName))
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

		rel := &models.EntityRelationship{
			OntologyID:         ontologyID,
			SourceEntityID:     sourceEntity.ID,
			TargetEntityID:     targetEntity.ID,
			SourceColumnSchema: sourceTable.SchemaName,
			SourceColumnTable:  sourceTable.TableName,
			SourceColumnName:   col.ColumnName,
			SourceColumnID:     &col.ID,
			TargetColumnSchema: targetTable.SchemaName,
			TargetColumnTable:  targetTable.TableName,
			TargetColumnName:   targetCol.ColumnName,
			TargetColumnID:     &targetCol.ID,
			DetectionMethod:    models.DetectionMethodDataOverlap,
			Confidence:         idFeatures.FKConfidence,
			Status:             models.RelationshipStatusConfirmed,
			Cardinality:        cardinality,
			Source:             models.SourceInferred.String(),
		}

		if err := s.relationshipRepo.Create(ctx, rel); err != nil {
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

// createRelationshipFromValidation creates an EntityRelationship from a validated candidate.
func (s *llmRelationshipDiscoveryService) createRelationshipFromValidation(
	ctx context.Context,
	ontologyID uuid.UUID,
	vr *ValidatedRelationship,
	entityByTable map[string]*models.OntologyEntity,
	_ map[uuid.UUID]*models.SchemaColumn, // columnByID - unused, IDs come from candidate
	tableByName map[string]*models.SchemaTable,
) error {
	candidate := vr.Candidate
	result := vr.Result

	// Resolve entities
	sourceEntity := entityByTable[candidate.SourceTable]
	targetEntity := entityByTable[candidate.TargetTable]

	if sourceEntity == nil || targetEntity == nil {
		return fmt.Errorf("missing entity for table: source=%s (%v), target=%s (%v)",
			candidate.SourceTable, sourceEntity != nil,
			candidate.TargetTable, targetEntity != nil)
	}

	// Resolve schema names from table lookup
	var sourceSchema, targetSchema string
	if t := tableByName[candidate.SourceTable]; t != nil {
		sourceSchema = t.SchemaName
	}
	if t := tableByName[candidate.TargetTable]; t != nil {
		targetSchema = t.SchemaName
	}

	rel := &models.EntityRelationship{
		OntologyID:         ontologyID,
		SourceEntityID:     sourceEntity.ID,
		TargetEntityID:     targetEntity.ID,
		SourceColumnSchema: sourceSchema,
		SourceColumnTable:  candidate.SourceTable,
		SourceColumnName:   candidate.SourceColumn,
		SourceColumnID:     &candidate.SourceColumnID,
		TargetColumnSchema: targetSchema,
		TargetColumnTable:  candidate.TargetTable,
		TargetColumnName:   candidate.TargetColumn,
		TargetColumnID:     &candidate.TargetColumnID,
		DetectionMethod:    models.DetectionMethodPKMatch,
		Confidence:         result.Confidence,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        result.Cardinality,
		Source:             models.SourceInferred.String(),
	}

	// Add role-based description if LLM provided a source role
	if result.SourceRole != "" {
		desc := fmt.Sprintf("The %s in %s represents the %s.",
			candidate.SourceColumn, candidate.SourceTable, result.SourceRole)
		rel.Description = &desc
	}

	return s.relationshipRepo.Create(ctx, rel)
}

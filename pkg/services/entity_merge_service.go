package services

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityMergeService handles merging of entities when name collisions occur.
// This is needed when the LLM suggests renaming an inferred entity (e.g., "accounts" -> "Account")
// but an entity with that name already exists (e.g., created via MCP tools).
type EntityMergeService interface {
	// MergeEntities merges the source entity into the target entity.
	// - Copies primary_table/schema/column from source to target if target lacks them
	// - Transfers aliases, key columns, and relationships
	// - Soft-deletes the source entity
	// Returns the updated target entity.
	MergeEntities(ctx context.Context, source, target *models.OntologyEntity) (*models.OntologyEntity, error)
}

type entityMergeService struct {
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	columnMetaRepo   repositories.ColumnMetadataRepository
	logger           *zap.Logger
}

// NewEntityMergeService creates a new EntityMergeService.
func NewEntityMergeService(
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	columnMetaRepo repositories.ColumnMetadataRepository,
	logger *zap.Logger,
) EntityMergeService {
	return &entityMergeService{
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		columnMetaRepo:   columnMetaRepo,
		logger:           logger.Named("entity-merge"),
	}
}

var _ EntityMergeService = (*entityMergeService)(nil)

// MergeEntities merges the source entity into the target entity.
// The source entity is typically an inferred entity that the LLM wants to rename,
// and the target is an existing entity (e.g., created via MCP) with the desired name.
func (s *entityMergeService) MergeEntities(ctx context.Context, source, target *models.OntologyEntity) (*models.OntologyEntity, error) {
	if source == nil || target == nil {
		return nil, fmt.Errorf("source and target entities are required")
	}

	if source.ID == target.ID {
		return nil, fmt.Errorf("cannot merge entity with itself")
	}

	s.logger.Info("Merging entities",
		zap.String("source_id", source.ID.String()),
		zap.String("source_name", source.Name),
		zap.String("target_id", target.ID.String()),
		zap.String("target_name", target.Name))

	// 1. Copy primary_table/schema/column from source to target if target lacks them
	needsUpdate := false
	if target.PrimaryTable == "" && source.PrimaryTable != "" {
		target.PrimaryTable = source.PrimaryTable
		target.PrimarySchema = source.PrimarySchema
		target.PrimaryColumn = source.PrimaryColumn
		needsUpdate = true
		s.logger.Debug("Copied primary table from source to target",
			zap.String("primary_table", source.PrimaryTable))
	}

	// Copy domain if target lacks it
	if target.Domain == "" && source.Domain != "" {
		target.Domain = source.Domain
		needsUpdate = true
	}

	// Copy description if target lacks it (preserve MCP user intent if they set one)
	if target.Description == "" && source.Description != "" {
		target.Description = source.Description
		needsUpdate = true
	}

	// Update confidence to reflect LLM enrichment
	if source.Confidence > target.Confidence {
		target.Confidence = source.Confidence
		needsUpdate = true
	}

	// Clear stale flag since we're merging
	if target.IsStale {
		target.IsStale = false
		needsUpdate = true
	}

	if needsUpdate {
		if err := s.entityRepo.Update(ctx, target); err != nil {
			return nil, fmt.Errorf("failed to update target entity: %w", err)
		}
	}

	// 2. Transfer aliases from source to target
	aliasCount, err := s.entityRepo.TransferAliasesToEntity(ctx, source.ID, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer aliases: %w", err)
	}
	if aliasCount > 0 {
		s.logger.Debug("Transferred aliases", zap.Int("count", aliasCount))
	}

	// Also add the source entity name as an alias of the target
	// (e.g., if "accounts" is merged into "Account", "accounts" becomes an alias)
	if source.Name != target.Name {
		discoverySource := "merge"
		alias := &models.OntologyEntityAlias{
			EntityID: target.ID,
			Alias:    source.Name,
			Source:   &discoverySource,
		}
		if err := s.entityRepo.CreateAlias(ctx, alias); err != nil {
			// Log but don't fail - alias may already exist
			s.logger.Warn("Failed to create alias for merged entity name",
				zap.String("alias", source.Name),
				zap.Error(err))
		}
	}

	// 3. Transfer key columns from source to target
	keyColCount, err := s.entityRepo.TransferKeyColumnsToEntity(ctx, source.ID, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer key columns: %w", err)
	}
	if keyColCount > 0 {
		s.logger.Debug("Transferred key columns", zap.Int("count", keyColCount))
	}

	// 4. Update relationships - redirect source entity references to target
	srcUpdated, err := s.relationshipRepo.UpdateSourceEntityID(ctx, source.ID, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update source relationships: %w", err)
	}
	if srcUpdated > 0 {
		s.logger.Debug("Updated source relationships", zap.Int("count", srcUpdated))
	}

	tgtUpdated, err := s.relationshipRepo.UpdateTargetEntityID(ctx, source.ID, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update target relationships: %w", err)
	}
	if tgtUpdated > 0 {
		s.logger.Debug("Updated target relationships", zap.Int("count", tgtUpdated))
	}

	// NOTE: Column metadata no longer has entity names as direct fields.
	// Entity associations are tracked via engine_ontology_entity_occurrences.

	// 5. Soft-delete the source entity
	deletionReason := fmt.Sprintf("Merged into entity %s (ID: %s)", target.Name, target.ID.String())
	if err := s.entityRepo.SoftDelete(ctx, source.ID, deletionReason); err != nil {
		return nil, fmt.Errorf("failed to soft-delete source entity: %w", err)
	}

	s.logger.Info("Entity merge completed",
		zap.String("source_id", source.ID.String()),
		zap.String("target_id", target.ID.String()),
		zap.Int("aliases_transferred", aliasCount),
		zap.Int("key_columns_transferred", keyColCount),
		zap.Int("relationships_updated", srcUpdated+tgtUpdated))

	return target, nil
}

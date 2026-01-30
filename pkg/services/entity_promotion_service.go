package services

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityPromotionService evaluates entities and assigns promotion scores.
// Entities with high semantic value (hub in relationship graph, multiple roles, etc.)
// are promoted. Simple 1:1 table-to-entity mappings are demoted.
type EntityPromotionService interface {
	// ScoreAndPromoteEntities evaluates all entities for a project and updates is_promoted status.
	// Returns count of promoted and demoted entities.
	ScoreAndPromoteEntities(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error)
}

type entityPromotionService struct {
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	schemaRepo       repositories.SchemaRepository
	ontologyRepo     repositories.OntologyRepository
	logger           *zap.Logger
}

// NewEntityPromotionService creates a new EntityPromotionService.
func NewEntityPromotionService(
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	logger *zap.Logger,
) EntityPromotionService {
	return &entityPromotionService{
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		schemaRepo:       schemaRepo,
		ontologyRepo:     ontologyRepo,
		logger:           logger.Named("entity-promotion"),
	}
}

var _ EntityPromotionService = (*entityPromotionService)(nil)

// ScoreAndPromoteEntities evaluates all entities for a project and updates is_promoted status.
// Manual promotions/demotions (Source == "manual") are preserved and not overwritten.
func (s *entityPromotionService) ScoreAndPromoteEntities(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error) {
	s.logger.Info("Starting entity promotion scoring",
		zap.String("project_id", projectID.String()))

	// Get the active ontology for this project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}
	if ontology == nil {
		s.logger.Info("No active ontology found, skipping promotion",
			zap.String("project_id", projectID.String()))
		return 0, 0, nil
	}

	// Fetch all entities for the project (non-deleted)
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}
	if len(entities) == 0 {
		s.logger.Info("No entities found for promotion scoring",
			zap.String("project_id", projectID.String()))
		return 0, 0, nil
	}

	// Fetch all relationships for the project
	relationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}

	// Fetch all aliases grouped by entity
	aliasesByEntity, err := s.entityRepo.GetAllAliasesByProject(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}

	// We need schema tables for related tables check
	// Get the datasource ID from relationships or entities
	// For now, we'll skip the allTables check if we can't determine the datasource
	// This is acceptable because the related tables check is just one criterion
	var allTables []*models.SchemaTable

	s.logger.Debug("Scoring entities for promotion",
		zap.String("project_id", projectID.String()),
		zap.Int("entity_count", len(entities)),
		zap.Int("relationship_count", len(relationships)))

	for _, entity := range entities {
		// Skip manual promotions/demotions - they persist across re-extraction
		if entity.Source == models.ProvenanceManual {
			s.logger.Debug("Skipping manually managed entity",
				zap.String("entity_name", entity.Name),
				zap.Bool("is_promoted", entity.IsPromoted))
			// Count it as promoted or demoted based on current status
			if entity.IsPromoted {
				promoted++
			} else {
				demoted++
			}
			continue
		}

		// Build promotion input
		input := PromotionInput{
			TableName:     entity.PrimaryTable,
			SchemaName:    entity.PrimarySchema,
			AllTables:     allTables,
			Relationships: relationships,
			Aliases:       aliasesByEntity[entity.ID],
		}

		// Calculate promotion score
		result := PromotionScore(input)

		// Determine if entity should be promoted
		shouldPromote := result.Score >= PromotionThreshold

		// Log promotion decision
		s.logger.Info("Entity promotion decision",
			zap.String("entity_name", entity.Name),
			zap.String("primary_table", entity.PrimaryTable),
			zap.Int("score", result.Score),
			zap.Bool("is_promoted", shouldPromote),
			zap.Strings("reasons", result.Reasons))

		// Update entity with promotion data
		entity.PromotionScore = &result.Score
		entity.PromotionReasons = result.Reasons
		entity.IsPromoted = shouldPromote

		if err := s.entityRepo.Update(ctx, entity); err != nil {
			s.logger.Error("Failed to update entity promotion status",
				zap.String("entity_id", entity.ID.String()),
				zap.Error(err))
			return promoted, demoted, err
		}

		if shouldPromote {
			promoted++
		} else {
			demoted++
		}
	}

	s.logger.Info("Entity promotion scoring complete",
		zap.String("project_id", projectID.String()),
		zap.Int("promoted", promoted),
		zap.Int("demoted", demoted))

	return promoted, demoted, nil
}

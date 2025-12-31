package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityWithDetails represents an entity with its occurrences and aliases.
type EntityWithDetails struct {
	Entity          *models.OntologyEntity
	Occurrences     []*models.OntologyEntityOccurrence
	Aliases         []*models.OntologyEntityAlias
	OccurrenceCount int
}

// EntityService provides operations for managing entities in the ontology.
type EntityService interface {
	// ListByProject returns all entities for a project (from active ontology).
	// Only returns non-deleted entities.
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]*EntityWithDetails, error)

	// GetByID returns a single entity with occurrences and aliases.
	// Returns deleted entities too (for restore preview).
	GetByID(ctx context.Context, entityID uuid.UUID) (*EntityWithDetails, error)

	// Delete soft-deletes an entity with a reason.
	Delete(ctx context.Context, entityID uuid.UUID, reason string) error

	// Restore restores a soft-deleted entity.
	Restore(ctx context.Context, entityID uuid.UUID) error

	// Update updates an entity's description.
	Update(ctx context.Context, entityID uuid.UUID, description string) error

	// AddAlias adds an alias to an entity.
	AddAlias(ctx context.Context, entityID uuid.UUID, alias, source string) (*models.OntologyEntityAlias, error)

	// RemoveAlias removes an alias from an entity.
	RemoveAlias(ctx context.Context, aliasID uuid.UUID) error
}

type entityService struct {
	entityRepo   repositories.SchemaEntityRepository
	ontologyRepo repositories.OntologyRepository
	logger       *zap.Logger
}

// NewEntityService creates a new EntityService.
func NewEntityService(
	entityRepo repositories.SchemaEntityRepository,
	ontologyRepo repositories.OntologyRepository,
	logger *zap.Logger,
) EntityService {
	return &entityService{
		entityRepo:   entityRepo,
		ontologyRepo: ontologyRepo,
		logger:       logger.Named("entity-service"),
	}
}

var _ EntityService = (*entityService)(nil)

func (s *entityService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*EntityWithDetails, error) {
	// Get active ontology for project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get active ontology: %w", err)
	}

	if ontology == nil {
		// No active ontology - return empty list
		return []*EntityWithDetails{}, nil
	}

	// Get entities for ontology (already filters deleted)
	entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		s.logger.Error("Failed to get entities by ontology",
			zap.String("ontology_id", ontology.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get entities: %w", err)
	}

	// Fetch occurrences and aliases for each entity
	result := make([]*EntityWithDetails, 0, len(entities))
	for _, entity := range entities {
		occurrences, err := s.entityRepo.GetOccurrencesByEntity(ctx, entity.ID)
		if err != nil {
			s.logger.Error("Failed to get occurrences",
				zap.String("entity_id", entity.ID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("get occurrences for entity %s: %w", entity.ID, err)
		}

		aliases, err := s.entityRepo.GetAliasesByEntity(ctx, entity.ID)
		if err != nil {
			s.logger.Error("Failed to get aliases",
				zap.String("entity_id", entity.ID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("get aliases for entity %s: %w", entity.ID, err)
		}

		result = append(result, &EntityWithDetails{
			Entity:          entity,
			Occurrences:     occurrences,
			Aliases:         aliases,
			OccurrenceCount: len(occurrences),
		})
	}

	return result, nil
}

func (s *entityService) GetByID(ctx context.Context, entityID uuid.UUID) (*EntityWithDetails, error) {
	// Get entity (returns deleted entities too for restore preview)
	entity, err := s.entityRepo.GetByID(ctx, entityID)
	if err != nil {
		s.logger.Error("Failed to get entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get entity: %w", err)
	}

	if entity == nil {
		return nil, nil
	}

	// Get occurrences
	occurrences, err := s.entityRepo.GetOccurrencesByEntity(ctx, entityID)
	if err != nil {
		s.logger.Error("Failed to get occurrences",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get occurrences: %w", err)
	}

	// Get aliases
	aliases, err := s.entityRepo.GetAliasesByEntity(ctx, entityID)
	if err != nil {
		s.logger.Error("Failed to get aliases",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get aliases: %w", err)
	}

	return &EntityWithDetails{
		Entity:          entity,
		Occurrences:     occurrences,
		Aliases:         aliases,
		OccurrenceCount: len(occurrences),
	}, nil
}

func (s *entityService) Delete(ctx context.Context, entityID uuid.UUID, reason string) error {
	if err := s.entityRepo.SoftDelete(ctx, entityID, reason); err != nil {
		s.logger.Error("Failed to soft delete entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return fmt.Errorf("soft delete entity: %w", err)
	}

	s.logger.Info("Entity soft deleted",
		zap.String("entity_id", entityID.String()),
		zap.String("reason", reason))

	return nil
}

func (s *entityService) Restore(ctx context.Context, entityID uuid.UUID) error {
	if err := s.entityRepo.Restore(ctx, entityID); err != nil {
		s.logger.Error("Failed to restore entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return fmt.Errorf("restore entity: %w", err)
	}

	s.logger.Info("Entity restored",
		zap.String("entity_id", entityID.String()))

	return nil
}

func (s *entityService) Update(ctx context.Context, entityID uuid.UUID, description string) error {
	// Get entity
	entity, err := s.entityRepo.GetByID(ctx, entityID)
	if err != nil {
		s.logger.Error("Failed to get entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return fmt.Errorf("get entity: %w", err)
	}

	if entity == nil {
		return fmt.Errorf("entity not found")
	}

	// Update description
	entity.Description = description
	if err := s.entityRepo.Update(ctx, entity); err != nil {
		s.logger.Error("Failed to update entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return fmt.Errorf("update entity: %w", err)
	}

	s.logger.Info("Entity updated",
		zap.String("entity_id", entityID.String()))

	return nil
}

func (s *entityService) AddAlias(ctx context.Context, entityID uuid.UUID, alias, source string) (*models.OntologyEntityAlias, error) {
	if alias == "" {
		return nil, fmt.Errorf("alias cannot be empty")
	}

	// Verify entity exists
	entity, err := s.entityRepo.GetByID(ctx, entityID)
	if err != nil {
		s.logger.Error("Failed to get entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get entity: %w", err)
	}

	if entity == nil {
		return nil, fmt.Errorf("entity not found")
	}

	// Create alias
	aliasModel := &models.OntologyEntityAlias{
		EntityID: entityID,
		Alias:    alias,
	}
	if source != "" {
		aliasModel.Source = &source
	}

	if err := s.entityRepo.CreateAlias(ctx, aliasModel); err != nil {
		s.logger.Error("Failed to create alias",
			zap.String("entity_id", entityID.String()),
			zap.String("alias", alias),
			zap.Error(err))
		return nil, fmt.Errorf("create alias: %w", err)
	}

	s.logger.Info("Alias added to entity",
		zap.String("entity_id", entityID.String()),
		zap.String("alias", alias))

	return aliasModel, nil
}

func (s *entityService) RemoveAlias(ctx context.Context, aliasID uuid.UUID) error {
	if err := s.entityRepo.DeleteAlias(ctx, aliasID); err != nil {
		s.logger.Error("Failed to delete alias",
			zap.String("alias_id", aliasID.String()),
			zap.Error(err))
		return fmt.Errorf("delete alias: %w", err)
	}

	s.logger.Info("Alias removed",
		zap.String("alias_id", aliasID.String()))

	return nil
}

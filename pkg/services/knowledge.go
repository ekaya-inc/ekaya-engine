package services

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// KnowledgeService provides operations for project knowledge management.
type KnowledgeService interface {
	// Store creates or updates a knowledge fact.
	Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error)

	// Update modifies an existing knowledge fact by ID.
	Update(ctx context.Context, projectID, id uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error)

	// GetAll retrieves all knowledge facts for a project.
	GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)

	// GetByType retrieves knowledge facts of a specific type.
	GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)

	// Delete removes a knowledge fact.
	Delete(ctx context.Context, id uuid.UUID) error
}

type knowledgeService struct {
	repo         repositories.KnowledgeRepository
	projectRepo  repositories.ProjectRepository
	ontologyRepo repositories.OntologyRepository
	logger       *zap.Logger
}

// NewKnowledgeService creates a new knowledge service.
func NewKnowledgeService(
	repo repositories.KnowledgeRepository,
	projectRepo repositories.ProjectRepository,
	ontologyRepo repositories.OntologyRepository,
	logger *zap.Logger,
) KnowledgeService {
	return &knowledgeService{
		repo:         repo,
		projectRepo:  projectRepo,
		ontologyRepo: ontologyRepo,
		logger:       logger.Named("knowledge"),
	}
}

var _ KnowledgeService = (*knowledgeService)(nil)

func (s *knowledgeService) Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	// Look up active ontology to associate knowledge with it
	var ontologyID *uuid.UUID
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err == nil && ontology != nil {
		ontologyID = &ontology.ID
	}
	// If no active ontology found, ontologyID remains nil (allowed by schema)

	fact := &models.KnowledgeFact{
		ProjectID:  projectID,
		OntologyID: ontologyID,
		FactType:   factType,
		Key:        key,
		Value:      value,
		Context:    contextInfo,
	}

	if err := s.repo.Upsert(ctx, fact); err != nil {
		s.logger.Error("Failed to store knowledge fact",
			zap.String("project_id", projectID.String()),
			zap.String("fact_type", factType),
			zap.String("key", key),
			zap.Error(err))
		return nil, err
	}

	s.logger.Info("Knowledge fact stored",
		zap.String("project_id", projectID.String()),
		zap.String("fact_type", factType),
		zap.String("key", key))

	return fact, nil
}

func (s *knowledgeService) Update(ctx context.Context, projectID, id uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	fact := &models.KnowledgeFact{
		ID:        id,
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
	}

	if err := s.repo.Upsert(ctx, fact); err != nil {
		s.logger.Error("Failed to update knowledge fact",
			zap.String("id", id.String()),
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}

	s.logger.Info("Knowledge fact updated",
		zap.String("id", id.String()),
		zap.String("project_id", projectID.String()),
		zap.String("fact_type", factType),
		zap.String("key", key))

	return fact, nil
}

func (s *knowledgeService) GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	facts, err := s.repo.GetByProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get knowledge facts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return facts, nil
}

func (s *knowledgeService) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	facts, err := s.repo.GetByType(ctx, projectID, factType)
	if err != nil {
		s.logger.Error("Failed to get knowledge facts by type",
			zap.String("project_id", projectID.String()),
			zap.String("fact_type", factType),
			zap.Error(err))
		return nil, err
	}
	return facts, nil
}

func (s *knowledgeService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error("Failed to delete knowledge fact",
			zap.String("id", id.String()),
			zap.Error(err))
		return err
	}
	return nil
}

package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// KnowledgeService provides operations for project knowledge management.
type KnowledgeService interface {
	// Store creates or updates a knowledge fact.
	Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error)

	// StoreWithSource creates or updates a knowledge fact with explicit source provenance.
	// Use source="manual" for user-entered data, source="inferred" for LLM-extracted data.
	// For project_overview facts, ontologyID is set to nil so it survives ontology deletion.
	StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo, source string) (*models.KnowledgeFact, error)

	// Update modifies an existing knowledge fact by ID.
	Update(ctx context.Context, projectID, id uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error)

	// GetAll retrieves all knowledge facts for a project.
	GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)

	// GetByType retrieves knowledge facts of a specific type.
	GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)

	// Delete removes a knowledge fact.
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteAll removes all knowledge facts for a project.
	DeleteAll(ctx context.Context, projectID uuid.UUID) error
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
	fact := &models.KnowledgeFact{
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
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

// StoreWithSource creates or updates a knowledge fact with explicit source provenance.
// The source parameter should be "manual" for user-entered data or "inferred" for LLM-extracted data.
// Knowledge facts have project-lifecycle scope and persist across ontology re-extractions.
func (s *knowledgeService) StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo, source string) (*models.KnowledgeFact, error) {
	// Validate and convert source to ProvenanceSource
	provenanceSource := models.ProvenanceSource(source)
	if !provenanceSource.IsValid() {
		return nil, fmt.Errorf("invalid source %q: must be one of 'manual', 'inferred', 'mcp'", source)
	}

	// Get userID from context (from JWT claims)
	userID, ok := auth.GetUserUUIDFromContext(ctx)
	if !ok {
		// For background operations (like DAG execution), userID might not be in JWT claims
		// but could be passed via provenance context already set by caller
		if prov, provOK := models.GetProvenance(ctx); provOK {
			userID = prov.UserID
		} else {
			userID = uuid.Nil // Allow nil user for system operations
		}
	}

	// Wrap context with appropriate provenance based on source
	var provenanceCtx context.Context
	switch provenanceSource {
	case models.SourceManual:
		provenanceCtx = models.WithManualProvenance(ctx, userID)
	case models.SourceInferred:
		provenanceCtx = models.WithInferredProvenance(ctx, userID)
	case models.SourceMCP:
		provenanceCtx = models.WithMCPProvenance(ctx, userID)
	default:
		return nil, fmt.Errorf("unsupported source: %s", source)
	}

	fact := &models.KnowledgeFact{
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
	}

	if err := s.repo.Upsert(provenanceCtx, fact); err != nil {
		s.logger.Error("Failed to store knowledge fact with source",
			zap.String("project_id", projectID.String()),
			zap.String("fact_type", factType),
			zap.String("key", key),
			zap.String("source", source),
			zap.Error(err))
		return nil, err
	}

	s.logger.Info("Knowledge fact stored with source",
		zap.String("project_id", projectID.String()),
		zap.String("fact_type", factType),
		zap.String("key", key),
		zap.String("source", source))

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

func (s *knowledgeService) DeleteAll(ctx context.Context, projectID uuid.UUID) error {
	if err := s.repo.DeleteByProject(ctx, projectID); err != nil {
		s.logger.Error("Failed to delete all knowledge facts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("Deleted all knowledge facts",
		zap.String("project_id", projectID.String()))

	return nil
}

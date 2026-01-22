package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// KnowledgeService provides operations for project knowledge management.
type KnowledgeService interface {
	// Store creates or updates a knowledge fact.
	Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error)

	// GetAll retrieves all knowledge facts for a project.
	GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)

	// GetByType retrieves knowledge facts of a specific type.
	GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)

	// Delete removes a knowledge fact.
	Delete(ctx context.Context, id uuid.UUID) error

	// SeedKnowledgeFromFile loads knowledge facts from the project's configured seed file path
	// and upserts them into the knowledge repository.
	// Returns the count of facts seeded.
	// If no seed path is configured, returns (0, nil) without error.
	SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error)
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

// SeedKnowledgeFromFile loads knowledge facts from the project's configured seed file path
// and upserts them into the knowledge repository.
func (s *knowledgeService) SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error) {
	// Get the project to check for seed path
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("get project: %w", err)
	}

	if project.Parameters == nil {
		return 0, nil // No parameters, no seed path
	}

	seedPath, ok := project.Parameters["knowledge_seed_path"].(string)
	if !ok || seedPath == "" {
		return 0, nil // No seed path configured
	}

	s.logger.Info("Loading knowledge seed file",
		zap.String("project_id", projectID.String()),
		zap.String("seed_path", seedPath))

	// Load and parse the seed file
	seedFile, err := loadKnowledgeSeedFile(seedPath)
	if err != nil {
		return 0, fmt.Errorf("load knowledge seed file: %w", err)
	}

	// Look up active ontology to associate knowledge with it
	var ontologyID *uuid.UUID
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err == nil && ontology != nil {
		ontologyID = &ontology.ID
	}
	// If no active ontology found, ontologyID remains nil (allowed by schema)

	// Upsert all facts
	count := 0
	for _, fact := range seedFile.AllFacts() {
		knowledgeFact := &models.KnowledgeFact{
			ProjectID:  projectID,
			OntologyID: ontologyID,
			FactType:   fact.FactType,
			Key:        fact.Fact,
			Value:      fact.Fact,
			Context:    fact.Context,
		}

		if err := s.repo.Upsert(ctx, knowledgeFact); err != nil {
			s.logger.Error("Failed to upsert knowledge fact",
				zap.String("project_id", projectID.String()),
				zap.String("fact_type", fact.FactType),
				zap.String("fact", fact.Fact),
				zap.Error(err))
			return count, fmt.Errorf("upsert knowledge fact: %w", err)
		}
		count++
	}

	s.logger.Info("Knowledge seeding complete",
		zap.String("project_id", projectID.String()),
		zap.Int("fact_count", count))

	return count, nil
}

// KnowledgeSeedFile represents the structure of a knowledge seed file.
type KnowledgeSeedFile struct {
	Terminology   []KnowledgeSeedFact `yaml:"terminology" json:"terminology"`
	BusinessRules []KnowledgeSeedFact `yaml:"business_rules" json:"business_rules"`
	Enumerations  []KnowledgeSeedFact `yaml:"enumerations" json:"enumerations"`
	Conventions   []KnowledgeSeedFact `yaml:"conventions" json:"conventions"`
}

// KnowledgeSeedFact represents a single fact in the seed file.
type KnowledgeSeedFact struct {
	Fact    string `yaml:"fact" json:"fact"`
	Context string `yaml:"context" json:"context"`
}

// knowledgeSeedFactWithType is a helper struct that includes the fact type.
type knowledgeSeedFactWithType struct {
	FactType string
	Fact     string
	Context  string
}

// AllFacts returns all facts with their types.
func (f *KnowledgeSeedFile) AllFacts() []knowledgeSeedFactWithType {
	var facts []knowledgeSeedFactWithType

	for _, fact := range f.Terminology {
		facts = append(facts, knowledgeSeedFactWithType{
			FactType: "terminology",
			Fact:     fact.Fact,
			Context:  fact.Context,
		})
	}

	for _, fact := range f.BusinessRules {
		facts = append(facts, knowledgeSeedFactWithType{
			FactType: "business_rule",
			Fact:     fact.Fact,
			Context:  fact.Context,
		})
	}

	for _, fact := range f.Enumerations {
		facts = append(facts, knowledgeSeedFactWithType{
			FactType: "enumeration",
			Fact:     fact.Fact,
			Context:  fact.Context,
		})
	}

	for _, fact := range f.Conventions {
		facts = append(facts, knowledgeSeedFactWithType{
			FactType: "convention",
			Fact:     fact.Fact,
			Context:  fact.Context,
		})
	}

	return facts
}

// loadKnowledgeSeedFile loads and parses a knowledge seed file (YAML or JSON).
func loadKnowledgeSeedFile(path string) (*KnowledgeSeedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed file: %w", err)
	}

	var seedFile KnowledgeSeedFile

	// Determine format by extension
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &seedFile); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &seedFile); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	default:
		// Try YAML first (more permissive), then JSON
		if err := yaml.Unmarshal(data, &seedFile); err != nil {
			if jsonErr := json.Unmarshal(data, &seedFile); jsonErr != nil {
				return nil, fmt.Errorf("parse seed file (tried YAML and JSON): YAML: %v, JSON: %v", err, jsonErr)
			}
		}
	}

	return &seedFile, nil
}

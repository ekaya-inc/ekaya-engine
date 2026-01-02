package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyFinalizationService generates domain-level summary after entity and relationship extraction.
type OntologyFinalizationService interface {
	// Finalize generates domain description and aggregates primary domains from entities.
	Finalize(ctx context.Context, projectID uuid.UUID) error
}

type ontologyFinalizationService struct {
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	llmFactory       llm.LLMClientFactory
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewOntologyFinalizationService creates a new ontology finalization service.
func NewOntologyFinalizationService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) OntologyFinalizationService {
	return &ontologyFinalizationService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		llmFactory:       llmFactory,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("ontology-finalization"),
	}
}

var _ OntologyFinalizationService = (*ontologyFinalizationService)(nil)

func (s *ontologyFinalizationService) Finalize(ctx context.Context, projectID uuid.UUID) error {
	s.logger.Info("Starting ontology finalization", zap.String("project_id", projectID.String()))

	// Get all entities (with domains populated from entity extraction)
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get entities: %w", err)
	}

	if len(entities) == 0 {
		s.logger.Info("No entities found, skipping finalization", zap.String("project_id", projectID.String()))
		return nil
	}

	// Get all relationships
	relationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get relationships: %w", err)
	}

	// Aggregate unique domains from entity.Domain fields
	primaryDomains := s.aggregateUniqueDomains(entities)

	// Build entity name lookup for relationship display
	entityNameByID := make(map[uuid.UUID]string, len(entities))
	for _, e := range entities {
		entityNameByID[e.ID] = e.Name
	}

	// Generate domain description via LLM
	description, err := s.generateDomainDescription(ctx, projectID, entities, relationships, entityNameByID)
	if err != nil {
		return fmt.Errorf("generate domain description: %w", err)
	}

	// Save to domain_summary JSONB
	domainSummary := &models.DomainSummary{
		Description: description,
		Domains:     primaryDomains,
		// RelationshipGraph: nil - redundant, GetDomainContext builds from normalized tables
		// SampleQuestions: nil - not generating per user decision
	}

	if err := s.ontologyRepo.UpdateDomainSummary(ctx, projectID, domainSummary); err != nil {
		return fmt.Errorf("update domain summary: %w", err)
	}

	s.logger.Info("Ontology finalization complete",
		zap.String("project_id", projectID.String()),
		zap.Int("entity_count", len(entities)),
		zap.Int("domain_count", len(primaryDomains)),
	)

	return nil
}

// aggregateUniqueDomains collects unique domain values from entities.
// Returns domains sorted alphabetically for deterministic output.
func (s *ontologyFinalizationService) aggregateUniqueDomains(entities []*models.OntologyEntity) []string {
	domainSet := make(map[string]struct{})
	for _, e := range entities {
		if e.Domain != "" {
			domainSet[e.Domain] = struct{}{}
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

// generateDomainDescription calls the LLM to generate a business description.
func (s *ontologyFinalizationService) generateDomainDescription(
	ctx context.Context,
	projectID uuid.UUID,
	entities []*models.OntologyEntity,
	relationships []*models.EntityRelationship,
	entityNameByID map[uuid.UUID]string,
) (string, error) {
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("create LLM client: %w", err)
	}

	systemMessage := s.domainDescriptionSystemMessage()
	prompt := s.buildDomainDescriptionPrompt(entities, relationships, entityNameByID)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return "", fmt.Errorf("LLM generate response: %w", err)
	}

	s.logger.Debug("LLM response received",
		zap.String("project_id", projectID.String()),
		zap.Int("prompt_tokens", result.PromptTokens),
		zap.Int("completion_tokens", result.CompletionTokens),
	)

	// Parse the response
	parsed, err := s.parseDomainDescriptionResponse(result.Content)
	if err != nil {
		return "", fmt.Errorf("parse domain description response: %w", err)
	}

	return parsed.Description, nil
}

func (s *ontologyFinalizationService) domainDescriptionSystemMessage() string {
	return `You are a data modeling expert. Your task is to analyze a database schema and provide a concise business description of what it represents.`
}

func (s *ontologyFinalizationService) buildDomainDescriptionPrompt(
	entities []*models.OntologyEntity,
	relationships []*models.EntityRelationship,
	entityNameByID map[uuid.UUID]string,
) string {
	var sb strings.Builder

	sb.WriteString("# Database Schema Analysis\n\n")
	sb.WriteString("Based on the following entities and their relationships, provide a 2-3 sentence business description of what this database represents.\n\n")

	sb.WriteString("## Entities\n\n")
	for _, e := range entities {
		domain := e.Domain
		if domain == "" {
			domain = "general"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", e.Name, domain, e.Description))
	}

	if len(relationships) > 0 {
		sb.WriteString("\n## Key Relationships\n\n")
		for _, rel := range relationships {
			sourceName := entityNameByID[rel.SourceEntityID]
			targetName := entityNameByID[rel.TargetEntityID]
			if sourceName == "" || targetName == "" {
				s.logger.Debug("Skipping relationship with missing entity name",
					zap.String("source_entity_id", rel.SourceEntityID.String()),
					zap.String("target_entity_id", rel.TargetEntityID.String()),
					zap.String("source_name", sourceName),
					zap.String("target_name", targetName))
				continue
			}
			if rel.Description != nil && *rel.Description != "" {
				sb.WriteString(fmt.Sprintf("- %s → %s (%s)\n", sourceName, targetName, *rel.Description))
			} else {
				sb.WriteString(fmt.Sprintf("- %s → %s\n", sourceName, targetName))
			}
		}
	}

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("Respond with a JSON object:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"description\": \"A 2-3 sentence business summary of what this database represents.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// domainDescriptionResponse is the expected LLM response structure.
type domainDescriptionResponse struct {
	Description string `json:"description"`
}

func (s *ontologyFinalizationService) parseDomainDescriptionResponse(content string) (*domainDescriptionResponse, error) {
	parsed, err := llm.ParseJSONResponse[domainDescriptionResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse domain description JSON: %w", err)
	}
	return &parsed, nil
}

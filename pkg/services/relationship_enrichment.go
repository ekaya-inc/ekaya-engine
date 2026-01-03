package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// RelationshipEnrichmentService provides semantic enrichment for entity relationships.
// It uses LLM to generate business-meaningful descriptions for relationships.
type RelationshipEnrichmentService interface {
	// EnrichProject enriches all relationships in a project with descriptions.
	EnrichProject(ctx context.Context, projectID uuid.UUID) (*EnrichRelationshipsResult, error)
}

// EnrichRelationshipsResult holds the result of a relationship enrichment operation.
type EnrichRelationshipsResult struct {
	RelationshipsEnriched int   `json:"relationships_enriched"`
	RelationshipsFailed   int   `json:"relationships_failed"`
	DurationMs            int64 `json:"duration_ms"`
}

type relationshipEnrichmentService struct {
	relationshipRepo repositories.EntityRelationshipRepository
	entityRepo       repositories.OntologyEntityRepository
	llmFactory       llm.LLMClientFactory
	logger           *zap.Logger
}

// NewRelationshipEnrichmentService creates a new relationship enrichment service.
func NewRelationshipEnrichmentService(
	relationshipRepo repositories.EntityRelationshipRepository,
	entityRepo repositories.OntologyEntityRepository,
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) RelationshipEnrichmentService {
	return &relationshipEnrichmentService{
		relationshipRepo: relationshipRepo,
		entityRepo:       entityRepo,
		llmFactory:       llmFactory,
		logger:           logger.Named("relationship-enrichment"),
	}
}

var _ RelationshipEnrichmentService = (*relationshipEnrichmentService)(nil)

// EnrichProject enriches all relationships in a project.
func (s *relationshipEnrichmentService) EnrichProject(ctx context.Context, projectID uuid.UUID) (*EnrichRelationshipsResult, error) {
	startTime := time.Now()
	result := &EnrichRelationshipsResult{}

	// Get all relationships for the project
	relationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get relationships: %w", err)
	}

	if len(relationships) == 0 {
		result.DurationMs = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Get all entities for context
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}

	// Build entity lookup
	entityByID := make(map[uuid.UUID]*models.OntologyEntity)
	for _, e := range entities {
		entityByID[e.ID] = e
	}

	// Enrich relationships in batches
	const batchSize = 20
	for i := 0; i < len(relationships); i += batchSize {
		end := i + batchSize
		if end > len(relationships) {
			end = len(relationships)
		}
		batch := relationships[i:end]

		enriched, failed := s.enrichBatch(ctx, projectID, batch, entityByID)
		result.RelationshipsEnriched += enriched
		result.RelationshipsFailed += failed
	}

	result.DurationMs = time.Since(startTime).Milliseconds()

	s.logger.Info("Relationship enrichment complete",
		zap.String("project_id", projectID.String()),
		zap.Int("enriched", result.RelationshipsEnriched),
		zap.Int("failed", result.RelationshipsFailed),
		zap.Int64("duration_ms", result.DurationMs))

	return result, nil
}

// enrichBatch enriches a batch of relationships via LLM.
func (s *relationshipEnrichmentService) enrichBatch(
	ctx context.Context,
	projectID uuid.UUID,
	relationships []*models.EntityRelationship,
	entityByID map[uuid.UUID]*models.OntologyEntity,
) (enriched int, failed int) {
	// Build prompt with relationship context
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to create LLM client", zap.Error(err))
		return 0, len(relationships)
	}

	systemMsg := s.relationshipEnrichmentSystemMessage()
	prompt := s.buildRelationshipEnrichmentPrompt(relationships, entityByID)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		s.logger.Error("LLM call failed", zap.Error(err))
		return 0, len(relationships)
	}

	// Parse response (wrapped in object for standardization)
	response, err := llm.ParseJSONResponse[relationshipEnrichmentResponse](result.Content)
	if err != nil {
		s.logger.Error("Failed to parse LLM response", zap.Error(err))
		return 0, len(relationships)
	}
	enrichments := response.Relationships

	// Build lookup by relationship key
	enrichmentByKey := make(map[string]string)
	for _, e := range enrichments {
		key := fmt.Sprintf("%s.%s->%s.%s", e.SourceTable, e.SourceColumn, e.TargetTable, e.TargetColumn)
		enrichmentByKey[key] = e.Description
	}

	// Update each relationship
	for _, rel := range relationships {
		key := fmt.Sprintf("%s.%s->%s.%s", rel.SourceColumnTable, rel.SourceColumnName, rel.TargetColumnTable, rel.TargetColumnName)
		description, ok := enrichmentByKey[key]
		if !ok || description == "" {
			s.logger.Debug("No enrichment found for relationship", zap.String("key", key))
			failed++
			continue
		}

		if err := s.relationshipRepo.UpdateDescription(ctx, rel.ID, description); err != nil {
			s.logger.Error("Failed to update relationship description",
				zap.String("relationship_id", rel.ID.String()),
				zap.Error(err))
			failed++
			continue
		}

		enriched++
	}

	return enriched, failed
}

// relationshipEnrichmentResponse wraps the LLM response for standardization.
type relationshipEnrichmentResponse struct {
	Relationships []relationshipEnrichment `json:"relationships"`
}

// relationshipEnrichment is the LLM response structure for a single relationship.
type relationshipEnrichment struct {
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column"`
	Description  string `json:"description"`
}

func (s *relationshipEnrichmentService) relationshipEnrichmentSystemMessage() string {
	return `You are a database relationship expert. Your task is to generate clear, business-meaningful descriptions for database relationships that help users understand how entities are connected.

Focus on the business meaning, not technical details. Describe what the relationship represents in terms users would understand.`
}

func (s *relationshipEnrichmentService) buildRelationshipEnrichmentPrompt(
	relationships []*models.EntityRelationship,
	entityByID map[uuid.UUID]*models.OntologyEntity,
) string {
	var sb strings.Builder

	sb.WriteString("# Relationship Description Generation\n\n")
	sb.WriteString("Generate business-meaningful descriptions for each relationship below.\n\n")

	// Entity context
	sb.WriteString("## Entities\n")
	seenEntities := make(map[uuid.UUID]bool)
	for _, rel := range relationships {
		if !seenEntities[rel.SourceEntityID] {
			if e, ok := entityByID[rel.SourceEntityID]; ok {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Name, e.Description))
				seenEntities[rel.SourceEntityID] = true
			}
		}
		if !seenEntities[rel.TargetEntityID] {
			if e, ok := entityByID[rel.TargetEntityID]; ok {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Name, e.Description))
				seenEntities[rel.TargetEntityID] = true
			}
		}
	}

	// Relationships to describe
	sb.WriteString("\n## Relationships\n")
	sb.WriteString("| Source | Target | Detection | Confidence |\n")
	sb.WriteString("|--------|--------|-----------|------------|\n")

	for _, rel := range relationships {
		sourceEntity := ""
		targetEntity := ""
		if e, ok := entityByID[rel.SourceEntityID]; ok {
			sourceEntity = e.Name
		}
		if e, ok := entityByID[rel.TargetEntityID]; ok {
			targetEntity = e.Name
		}

		sb.WriteString(fmt.Sprintf("| %s.%s (%s) | %s.%s (%s) | %s | %.0f%% |\n",
			rel.SourceColumnTable, rel.SourceColumnName, sourceEntity,
			rel.TargetColumnTable, rel.TargetColumnName, targetEntity,
			rel.DetectionMethod, rel.Confidence*100))
	}

	// Instructions
	sb.WriteString("\n## Guidelines\n")
	sb.WriteString("- Write 1-2 sentences describing the business meaning of each relationship\n")
	sb.WriteString("- Use entity names (not table names) when referring to what's connected\n")
	sb.WriteString("- Describe the nature of the relationship (e.g., 'belongs to', 'created by', 'references')\n")
	sb.WriteString("- Include cardinality hints if clear (e.g., 'each order can have many items')\n")

	// Response format
	sb.WriteString("\n## Response Format (JSON object)\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"relationships\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"source_table\": \"orders\",\n")
	sb.WriteString("      \"source_column\": \"user_id\",\n")
	sb.WriteString("      \"target_table\": \"users\",\n")
	sb.WriteString("      \"target_column\": \"id\",\n")
	sb.WriteString("      \"description\": \"Links each order to the user who placed it. A user can place many orders.\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

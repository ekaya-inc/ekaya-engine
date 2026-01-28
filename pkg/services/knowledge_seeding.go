package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// knowledgeSeedingService implements dag.KnowledgeSeedingMethods.
// It extracts domain knowledge facts from the project overview using LLM.
type knowledgeSeedingService struct {
	knowledgeService KnowledgeService
	schemaService    SchemaService
	llmFactory       llm.LLMClientFactory
	logger           *zap.Logger
}

// NewKnowledgeSeedingService creates a new knowledge seeding service.
func NewKnowledgeSeedingService(
	knowledgeService KnowledgeService,
	schemaService SchemaService,
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) dag.KnowledgeSeedingMethods {
	return &knowledgeSeedingService{
		knowledgeService: knowledgeService,
		schemaService:    schemaService,
		llmFactory:       llmFactory,
		logger:           logger.Named("knowledge-seeding"),
	}
}

var _ dag.KnowledgeSeedingMethods = (*knowledgeSeedingService)(nil)

// ExtractKnowledgeFromOverview extracts domain knowledge facts from the project overview.
// Returns the number of facts stored.
func (s *knowledgeSeedingService) ExtractKnowledgeFromOverview(ctx context.Context, projectID, datasourceID uuid.UUID) (int, error) {
	// 1. Get the project overview from knowledge table
	facts, err := s.knowledgeService.GetByType(ctx, projectID, "overview")
	if err != nil {
		return 0, fmt.Errorf("failed to get project overview: %w", err)
	}

	var projectOverview string
	for _, fact := range facts {
		if fact.Key == "project_overview" {
			projectOverview = fact.Value
			break
		}
	}

	if projectOverview == "" {
		s.logger.Info("No project overview provided, skipping knowledge extraction",
			zap.String("project_id", projectID.String()))
		return 0, nil
	}

	// 2. Get existing knowledge facts (excluding project_overview)
	allFacts, err := s.knowledgeService.GetAll(ctx, projectID)
	if err != nil {
		s.logger.Warn("Failed to get existing knowledge facts, continuing without them",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		allFacts = nil
	}

	// Filter out project_overview - it's the input, not learned knowledge
	existingFacts := make([]*models.KnowledgeFact, 0, len(allFacts))
	for _, fact := range allFacts {
		if fact.Key != "project_overview" {
			existingFacts = append(existingFacts, fact)
		}
	}

	// Skip seeding if we already have learned facts (knowledge seeding is one-time initialization)
	if len(existingFacts) > 0 {
		s.logger.Info("Knowledge facts already exist, skipping seeding",
			zap.String("project_id", projectID.String()),
			zap.Int("existing_facts", len(existingFacts)))
		return 0, nil
	}

	// 3. Get schema summary for context
	schemaContext, err := s.schemaService.GetDatasourceSchemaForPrompt(ctx, projectID, datasourceID, true)
	if err != nil {
		s.logger.Warn("Failed to get schema summary, continuing without schema context",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		schemaContext = ""
	}

	// 4. Use LLM to extract knowledge facts
	extractedFacts, err := s.extractKnowledgeFacts(ctx, projectID, projectOverview, schemaContext)
	if err != nil {
		return 0, fmt.Errorf("failed to extract knowledge facts: %w", err)
	}

	// 5. Store extracted facts with source='inferred'
	storedCount := 0
	for _, fact := range extractedFacts {
		if _, err := s.knowledgeService.StoreWithSource(
			ctx, projectID, fact.FactType, fact.Key, fact.Value, fact.Context, "inferred",
		); err != nil {
			s.logger.Warn("Failed to store extracted fact",
				zap.String("key", fact.Key),
				zap.String("fact_type", fact.FactType),
				zap.Error(err))
			// Continue with other facts - don't fail the entire operation
		} else {
			storedCount++
		}
	}

	s.logger.Info("Knowledge extraction complete",
		zap.String("project_id", projectID.String()),
		zap.Int("facts_extracted", len(extractedFacts)),
		zap.Int("facts_stored", storedCount))

	return storedCount, nil
}

// extractedFact represents a knowledge fact extracted from the overview.
type extractedFact struct {
	FactType string `json:"fact_type"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Context  string `json:"context,omitempty"`
}

// llmExtractionResponse represents the expected LLM response structure.
type llmExtractionResponse struct {
	Facts []extractedFact `json:"facts"`
}

// extractKnowledgeFacts uses LLM to extract knowledge facts from the project overview.
func (s *knowledgeSeedingService) extractKnowledgeFacts(
	ctx context.Context,
	projectID uuid.UUID,
	overview string,
	schemaContext string,
) ([]extractedFact, error) {
	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Build prompt
	prompt := s.buildExtractionPrompt(overview, schemaContext)
	systemMessage := s.extractionSystemMessage()

	// Call LLM
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM generate response: %w", err)
	}

	s.logger.Debug("LLM response received",
		zap.String("project_id", projectID.String()),
		zap.Int("prompt_tokens", result.PromptTokens),
		zap.Int("completion_tokens", result.CompletionTokens))

	// Parse response
	facts, err := s.parseExtractionResponse(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	return facts, nil
}

// extractionSystemMessage returns the system message for knowledge extraction.
func (s *knowledgeSeedingService) extractionSystemMessage() string {
	return `You are a data analyst expert who extracts domain knowledge from project descriptions.
Your task is to identify important business facts, conventions, and terminology from the user's project overview.

You must respond with a JSON object containing an array of facts. Each fact should have:
- fact_type: One of "business_rule", "convention", "terminology", "entity_hint"
- key: A unique identifier for the fact (e.g., "currency_format", "user_vs_customer")
- value: The actual fact or rule
- context: Optional additional context about the fact

Focus on extracting:
1. business_rule: Business logic or rules (e.g., "All timestamps are stored in UTC")
2. convention: Data or naming conventions (e.g., "Currency amounts are stored in cents")
3. terminology: Domain-specific terms and their meanings (e.g., "A 'channel' refers to a video creator")
4. entity_hint: Distinctions between similar concepts (e.g., "Users are employee accounts, Customers are external businesses")

Only extract facts that are clearly stated or strongly implied in the overview.
Do not make assumptions or invent facts not supported by the text.
If you cannot extract any meaningful facts, return an empty facts array.

Respond ONLY with valid JSON. No markdown, no explanations.`
}

// buildExtractionPrompt builds the prompt for knowledge extraction.
func (s *knowledgeSeedingService) buildExtractionPrompt(overview string, schemaContext string) string {
	var sb strings.Builder

	sb.WriteString("Extract domain knowledge facts from the following project overview.\n\n")

	sb.WriteString("## Project Overview\n")
	sb.WriteString(overview)
	sb.WriteString("\n\n")

	if schemaContext != "" {
		sb.WriteString("## Database Schema (for context)\n")
		sb.WriteString(schemaContext)
		sb.WriteString("\n\n")
	}

	sb.WriteString(`## Output Format
Respond with a JSON object containing extracted facts:
{
  "facts": [
    {
      "fact_type": "business_rule" | "convention" | "terminology" | "entity_hint",
      "key": "unique_identifier",
      "value": "The actual fact or rule",
      "context": "Optional additional context"
    }
  ]
}

Return {"facts": []} if no clear facts can be extracted from the overview.`)

	return sb.String()
}

// parseExtractionResponse parses the LLM response into extracted facts.
func (s *knowledgeSeedingService) parseExtractionResponse(content string) ([]extractedFact, error) {
	// Clean up the response - remove markdown code blocks if present
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var response llmExtractionResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (content: %s)", err, truncateForLog(content, 200))
	}

	// Validate and filter facts
	validFacts := make([]extractedFact, 0, len(response.Facts))
	for _, fact := range response.Facts {
		// Validate required fields
		if fact.FactType == "" || fact.Key == "" || fact.Value == "" {
			s.logger.Debug("Skipping invalid fact - missing required fields",
				zap.String("fact_type", fact.FactType),
				zap.String("key", fact.Key))
			continue
		}

		// Map fact_type to valid fact types from the model
		mappedType := mapFactTypeToModel(fact.FactType)
		if mappedType == "" {
			s.logger.Debug("Skipping fact with unknown type",
				zap.String("fact_type", fact.FactType),
				zap.String("key", fact.Key))
			continue
		}
		fact.FactType = mappedType

		validFacts = append(validFacts, fact)
	}

	return validFacts, nil
}

// mapFactTypeToModel maps LLM fact types to valid model fact types.
func mapFactTypeToModel(factType string) string {
	switch strings.ToLower(factType) {
	case "business_rule":
		return models.FactTypeBusinessRule
	case "convention":
		return models.FactTypeConvention
	case "terminology", "domain_term":
		return models.FactTypeTerminology
	case "entity_hint":
		// entity_hint maps to terminology since it's about clarifying domain concepts
		return models.FactTypeTerminology
	default:
		return ""
	}
}

// truncateForLog truncates a string to the specified length for logging purposes.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

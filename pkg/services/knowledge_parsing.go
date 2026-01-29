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
)

// KnowledgeParsingService parses free-form text into structured knowledge facts using LLM.
type KnowledgeParsingService interface {
	// ParseAndStore takes a free-form fact string, uses LLM to extract structured data,
	// and stores the resulting fact(s). Returns the created facts.
	ParseAndStore(ctx context.Context, projectID uuid.UUID, freeFormText string) ([]*models.KnowledgeFact, error)
}

type knowledgeParsingService struct {
	knowledgeService KnowledgeService
	llmFactory       llm.LLMClientFactory
	logger           *zap.Logger
}

// NewKnowledgeParsingService creates a new knowledge parsing service.
func NewKnowledgeParsingService(
	knowledgeService KnowledgeService,
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) KnowledgeParsingService {
	return &knowledgeParsingService{
		knowledgeService: knowledgeService,
		llmFactory:       llmFactory,
		logger:           logger.Named("knowledge-parsing"),
	}
}

var _ KnowledgeParsingService = (*knowledgeParsingService)(nil)

// ParseAndStore parses a free-form fact using LLM and stores the result.
func (s *knowledgeParsingService) ParseAndStore(ctx context.Context, projectID uuid.UUID, freeFormText string) ([]*models.KnowledgeFact, error) {
	if strings.TrimSpace(freeFormText) == "" {
		return nil, fmt.Errorf("free-form text cannot be empty")
	}

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Build prompt
	prompt := s.buildParsingPrompt(freeFormText)
	systemMessage := s.parsingSystemMessage()

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
	parsed, err := s.parseResponse(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	// Store facts
	var storedFacts []*models.KnowledgeFact
	for _, pf := range parsed {
		fact, err := s.knowledgeService.StoreWithSource(
			ctx, projectID, pf.FactType, pf.Key, pf.Value, pf.Context, "manual",
		)
		if err != nil {
			s.logger.Error("Failed to store parsed fact",
				zap.String("key", pf.Key),
				zap.Error(err))
			return nil, fmt.Errorf("store fact '%s': %w", pf.Key, err)
		}
		storedFacts = append(storedFacts, fact)
	}

	s.logger.Info("Parsed and stored facts",
		zap.String("project_id", projectID.String()),
		zap.Int("facts_stored", len(storedFacts)))

	return storedFacts, nil
}

// parsedFact represents a knowledge fact extracted from free-form text.
type parsedFact struct {
	FactType string `json:"fact_type"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Context  string `json:"context,omitempty"`
}

// llmParsingResponse represents the expected LLM response structure.
type llmParsingResponse struct {
	Facts []parsedFact `json:"facts"`
}

// parsingSystemMessage returns the system message for parsing free-form facts.
func (s *knowledgeParsingService) parsingSystemMessage() string {
	return `You are a data analyst expert who structures domain knowledge from free-form user input.
Your task is to convert a user's statement about their domain into structured knowledge facts.

You must respond with a JSON object containing an array of facts. Each fact should have:
- fact_type: One of "business_rule", "convention", "terminology"
- key: A unique snake_case identifier (e.g., "timezone_format", "currency_storage")
- value: A clear, complete statement of the fact
- context: Brief context about where this fact applies (optional)

Fact type guidelines:
1. business_rule: Business logic or rules (e.g., "All timestamps are stored in UTC")
2. convention: Data or naming conventions (e.g., "Currency amounts are stored in cents")
3. terminology: Domain-specific terms and their meanings (e.g., "A 'channel' refers to a video creator")

The user's input may be informal - convert it into a professional, clear statement.
Most inputs will result in exactly one fact, but if the user provides multiple facts in one statement, extract them all.

Respond ONLY with valid JSON. No markdown, no explanations.`
}

// buildParsingPrompt builds the prompt for parsing a free-form fact.
func (s *knowledgeParsingService) buildParsingPrompt(freeFormText string) string {
	var sb strings.Builder

	sb.WriteString("Convert the following user statement into structured knowledge fact(s):\n\n")
	sb.WriteString("## User Statement\n")
	sb.WriteString(freeFormText)
	sb.WriteString("\n\n")

	sb.WriteString(`## Output Format
Respond with a JSON object containing the structured fact(s):
{
  "facts": [
    {
      "fact_type": "business_rule" | "convention" | "terminology",
      "key": "snake_case_identifier",
      "value": "Clear statement of the fact",
      "context": "Optional context"
    }
  ]
}`)

	return sb.String()
}

// parseResponse parses the LLM response into parsed facts.
func (s *knowledgeParsingService) parseResponse(content string) ([]parsedFact, error) {
	// Clean up the response - remove markdown code blocks if present
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var response llmParsingResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w (content: %s)", err, truncateForLog(content, 200))
	}

	// Validate facts
	validFacts := make([]parsedFact, 0, len(response.Facts))
	for _, fact := range response.Facts {
		// Validate required fields
		if fact.FactType == "" || fact.Key == "" || fact.Value == "" {
			s.logger.Debug("Skipping invalid fact - missing required fields",
				zap.String("fact_type", fact.FactType),
				zap.String("key", fact.Key))
			continue
		}

		// Map fact_type to valid model types
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

	if len(validFacts) == 0 {
		return nil, fmt.Errorf("no valid facts extracted from input")
	}

	return validFacts, nil
}

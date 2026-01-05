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
)

// OntologyBuilderService provides LLM-based ontology construction.
type OntologyBuilderService interface {
	// ProcessAnswer processes a user's answer and returns any ontology updates.
	ProcessAnswer(ctx context.Context, projectID uuid.UUID, question *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error)
}

// AnswerProcessingResult contains the result of processing a question answer.
type AnswerProcessingResult struct {
	FollowUp       *string
	EntityUpdates  []EntityUpdate
	ColumnUpdates  []ColumnUpdate
	KnowledgeFacts []*models.KnowledgeFact
	ActionsSummary string
	Thinking       string // LLM reasoning extracted from <think> tags
}

// EntityUpdate describes an update to apply to an entity.
type EntityUpdate struct {
	TableName    string
	BusinessName *string
	Description  *string
	Synonyms     []string
	Domain       *string
}

// ColumnUpdate describes an update to apply to a column.
type ColumnUpdate struct {
	TableName    string
	ColumnName   string
	Description  *string
	SemanticType *string
	Synonyms     []string
	Role         *string
}

type ontologyBuilderService struct {
	llmFactory llm.LLMClientFactory
	logger     *zap.Logger
}

// NewOntologyBuilderService creates a new ontology builder service.
func NewOntologyBuilderService(
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) OntologyBuilderService {
	return &ontologyBuilderService{
		llmFactory: llmFactory,
		logger:     logger.Named("ontology-builder"),
	}
}

var _ OntologyBuilderService = (*ontologyBuilderService)(nil)

// ============================================================================
// ProcessAnswer
// ============================================================================

func (s *ontologyBuilderService) ProcessAnswer(ctx context.Context, projectID uuid.UUID, question *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
	startTime := time.Now()
	s.logger.Info("Processing answer",
		zap.String("project_id", projectID.String()),
		zap.String("question_id", question.ID.String()))

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	prompt := s.buildAnswerProcessingPrompt(question, answer)
	systemMsg := s.answerProcessingSystemMessage()

	llmResult, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Extract thinking from response before parsing JSON
	thinking := llm.ExtractThinking(llmResult.Content)

	result, err := s.parseAnswerProcessingResponse(llmResult.Content, projectID)
	if err != nil {
		// If parsing fails, return a basic result with thinking preserved
		s.logger.Warn("Failed to parse answer processing response", zap.Error(err))
		return &AnswerProcessingResult{
			ActionsSummary: "Answer recorded",
			Thinking:       thinking,
		}, nil
	}

	// Preserve thinking in the result
	result.Thinking = thinking

	s.logger.Info("Answer processed",
		zap.Int("entity_updates", len(result.EntityUpdates)),
		zap.Int("knowledge_facts", len(result.KnowledgeFacts)),
		zap.Duration("elapsed", time.Since(startTime)))

	return result, nil
}

func (s *ontologyBuilderService) answerProcessingSystemMessage() string {
	return `You are analyzing a user's answer to a database understanding question.
Extract structured updates to apply to the ontology and knowledge base.

Your response should identify:
1. Entity updates (table-level: business names, descriptions, synonyms, domains)
2. Column updates (column-level: descriptions, semantic types, synonyms, roles)
3. Knowledge facts to store (terminology, business rules, relationships)
4. Whether a follow-up question is needed
5. A brief summary of actions taken

IMPORTANT: If the question is about specific columns (listed in "Affected Columns"),
extract column_updates for those columns based on the user's answer.

Return a JSON object with these fields.`
}

func (s *ontologyBuilderService) buildAnswerProcessingPrompt(question *models.OntologyQuestion, answer string) string {
	var prompt strings.Builder

	prompt.WriteString("## Question\n\n")
	prompt.WriteString(question.Text + "\n\n")

	if question.Affects != nil {
		prompt.WriteString("## Affected Schema Elements\n\n")
		if len(question.Affects.Tables) > 0 {
			prompt.WriteString(fmt.Sprintf("Tables: %s\n", strings.Join(question.Affects.Tables, ", ")))
		}
		if len(question.Affects.Columns) > 0 {
			prompt.WriteString(fmt.Sprintf("Columns: %s\n", strings.Join(question.Affects.Columns, ", ")))
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("## User's Answer\n\n")
	prompt.WriteString(answer + "\n\n")

	prompt.WriteString("## Output Format\n\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(`{
  "follow_up": null,
  "entity_updates": [
    {
      "table_name": "orders",
      "business_name": "Customer Orders",
      "description": "Records of customer purchases",
      "synonyms": ["purchases", "transactions"],
      "domain": "sales"
    }
  ],
  "column_updates": [
    {
      "table_name": "orders",
      "column_name": "status",
      "description": "Current state of the order in the fulfillment process",
      "semantic_type": "status",
      "synonyms": ["order state", "fulfillment status"],
      "role": "state_tracking"
    }
  ],
  "knowledge_facts": [
    {
      "fact_type": "terminology",
      "key": "SKU",
      "value": "Stock Keeping Unit - unique product identifier"
    }
  ],
  "actions_summary": "Updated orders entity and status column with business context"
}
`)
	prompt.WriteString("```\n")

	return prompt.String()
}

func (s *ontologyBuilderService) parseAnswerProcessingResponse(response string, projectID uuid.UUID) (*AnswerProcessingResult, error) {
	type llmResult struct {
		FollowUp      *string `json:"follow_up"`
		EntityUpdates []struct {
			TableName    string   `json:"table_name"`
			BusinessName *string  `json:"business_name"`
			Description  *string  `json:"description"`
			Synonyms     []string `json:"synonyms"`
			Domain       *string  `json:"domain"`
		} `json:"entity_updates"`
		ColumnUpdates []struct {
			TableName    string   `json:"table_name"`
			ColumnName   string   `json:"column_name"`
			Description  *string  `json:"description"`
			SemanticType *string  `json:"semantic_type"`
			Synonyms     []string `json:"synonyms"`
			Role         *string  `json:"role"`
		} `json:"column_updates"`
		KnowledgeFacts []struct {
			FactType string `json:"fact_type"`
			Key      string `json:"key"`
			Value    string `json:"value"`
			Context  string `json:"context"`
		} `json:"knowledge_facts"`
		ActionsSummary string `json:"actions_summary"`
	}

	llmResp, err := llm.ParseJSONResponse[llmResult](response)
	if err != nil {
		return nil, err
	}

	result := &AnswerProcessingResult{
		FollowUp:       llmResp.FollowUp,
		ActionsSummary: llmResp.ActionsSummary,
	}

	for _, eu := range llmResp.EntityUpdates {
		result.EntityUpdates = append(result.EntityUpdates, EntityUpdate{
			TableName:    eu.TableName,
			BusinessName: eu.BusinessName,
			Description:  eu.Description,
			Synonyms:     eu.Synonyms,
			Domain:       eu.Domain,
		})
	}

	for _, cu := range llmResp.ColumnUpdates {
		result.ColumnUpdates = append(result.ColumnUpdates, ColumnUpdate{
			TableName:    cu.TableName,
			ColumnName:   cu.ColumnName,
			Description:  cu.Description,
			SemanticType: cu.SemanticType,
			Synonyms:     cu.Synonyms,
			Role:         cu.Role,
		})
	}

	for _, kf := range llmResp.KnowledgeFacts {
		result.KnowledgeFacts = append(result.KnowledgeFacts, &models.KnowledgeFact{
			ProjectID: projectID,
			FactType:  kf.FactType,
			Key:       kf.Key,
			Value:     kf.Value,
			Context:   kf.Context,
		})
	}

	return result, nil
}

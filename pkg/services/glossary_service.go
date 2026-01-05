package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// GlossaryService provides operations for managing business glossary terms.
type GlossaryService interface {
	// CreateTerm creates a new glossary term for a project.
	CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error

	// UpdateTerm updates an existing glossary term.
	UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error

	// DeleteTerm deletes a glossary term by ID.
	DeleteTerm(ctx context.Context, termID uuid.UUID) error

	// GetTerms returns all glossary terms for a project.
	GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)

	// GetTerm returns a single glossary term by ID.
	GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error)

	// SuggestTerms uses LLM to analyze the ontology and suggest business terms.
	SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
}

type glossaryService struct {
	glossaryRepo repositories.GlossaryRepository
	ontologyRepo repositories.OntologyRepository
	entityRepo   repositories.OntologyEntityRepository
	llmFactory   llm.LLMClientFactory
	logger       *zap.Logger
}

// NewGlossaryService creates a new GlossaryService.
func NewGlossaryService(
	glossaryRepo repositories.GlossaryRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) GlossaryService {
	return &glossaryService{
		glossaryRepo: glossaryRepo,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		llmFactory:   llmFactory,
		logger:       logger.Named("glossary-service"),
	}
}

var _ GlossaryService = (*glossaryService)(nil)

func (s *glossaryService) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	// Validate required fields
	if term.Term == "" {
		return fmt.Errorf("term name is required")
	}
	if term.Definition == "" {
		return fmt.Errorf("term definition is required")
	}

	// Set project ID and default source if not provided
	term.ProjectID = projectID
	if term.Source == "" {
		term.Source = "user"
	}

	if err := s.glossaryRepo.Create(ctx, term); err != nil {
		s.logger.Error("Failed to create glossary term",
			zap.String("project_id", projectID.String()),
			zap.String("term", term.Term),
			zap.Error(err))
		return fmt.Errorf("create glossary term: %w", err)
	}

	s.logger.Info("Created glossary term",
		zap.String("project_id", projectID.String()),
		zap.String("term", term.Term),
		zap.String("term_id", term.ID.String()))

	return nil
}

func (s *glossaryService) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	// Validate required fields
	if term.ID == uuid.Nil {
		return fmt.Errorf("term ID is required")
	}
	if term.Term == "" {
		return fmt.Errorf("term name is required")
	}
	if term.Definition == "" {
		return fmt.Errorf("term definition is required")
	}

	if err := s.glossaryRepo.Update(ctx, term); err != nil {
		s.logger.Error("Failed to update glossary term",
			zap.String("term_id", term.ID.String()),
			zap.Error(err))
		return fmt.Errorf("update glossary term: %w", err)
	}

	s.logger.Info("Updated glossary term",
		zap.String("term_id", term.ID.String()),
		zap.String("term", term.Term))

	return nil
}

func (s *glossaryService) DeleteTerm(ctx context.Context, termID uuid.UUID) error {
	if err := s.glossaryRepo.Delete(ctx, termID); err != nil {
		s.logger.Error("Failed to delete glossary term",
			zap.String("term_id", termID.String()),
			zap.Error(err))
		return fmt.Errorf("delete glossary term: %w", err)
	}

	s.logger.Info("Deleted glossary term",
		zap.String("term_id", termID.String()))

	return nil
}

func (s *glossaryService) GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	terms, err := s.glossaryRepo.GetByProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get glossary terms",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get glossary terms: %w", err)
	}

	return terms, nil
}

func (s *glossaryService) GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	term, err := s.glossaryRepo.GetByID(ctx, termID)
	if err != nil {
		s.logger.Error("Failed to get glossary term",
			zap.String("term_id", termID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get glossary term: %w", err)
	}

	return term, nil
}

func (s *glossaryService) SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	s.logger.Info("Starting term suggestion", zap.String("project_id", projectID.String()))

	// Get active ontology for context
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project")
	}

	// Get entities for context
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}

	if len(entities) == 0 {
		s.logger.Info("No entities found, skipping term suggestion",
			zap.String("project_id", projectID.String()))
		return []*models.BusinessGlossaryTerm{}, nil
	}

	// Build context for LLM
	prompt := s.buildSuggestTermsPrompt(ontology, entities)
	systemMessage := s.suggestTermsSystemMessage()

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

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
	suggestions, err := s.parseSuggestTermsResponse(result.Content, projectID)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	s.logger.Info("Generated term suggestions",
		zap.String("project_id", projectID.String()),
		zap.Int("count", len(suggestions)))

	return suggestions, nil
}

func (s *glossaryService) suggestTermsSystemMessage() string {
	return `You are a business analyst expert. Your task is to analyze a database schema and suggest business metrics and terms that would help executives understand the data.

Focus on:
1. Key Performance Indicators (KPIs) - metrics that measure business success
2. Financial metrics - revenue, costs, margins, etc.
3. User/customer metrics - active users, retention, churn, etc.
4. Transaction metrics - volume, value, conversion rates, etc.

For each term, provide:
- A clear business name
- A definition that explains what it measures
- The SQL pattern or aggregation used to calculate it
- The base table(s) involved
- Any filters that should be applied`
}

func (s *glossaryService) buildSuggestTermsPrompt(ontology *models.TieredOntology, entities []*models.OntologyEntity) string {
	var sb strings.Builder

	sb.WriteString("# Database Schema Analysis for Business Metrics\n\n")

	// Include domain summary if available
	if ontology.DomainSummary != nil && ontology.DomainSummary.Description != "" {
		sb.WriteString("## Domain Overview\n\n")
		sb.WriteString(ontology.DomainSummary.Description)
		sb.WriteString("\n\n")
	}

	// Include conventions if available
	if ontology.DomainSummary != nil && ontology.DomainSummary.Conventions != nil {
		conv := ontology.DomainSummary.Conventions
		sb.WriteString("## Conventions\n\n")

		if conv.SoftDelete != nil && conv.SoftDelete.Enabled {
			sb.WriteString(fmt.Sprintf("- Soft delete: Filter with `%s`\n", conv.SoftDelete.Filter))
		}
		if conv.Currency != nil {
			if conv.Currency.Format == "cents" {
				sb.WriteString("- Currency: Stored in cents, divide by 100 for display\n")
			} else {
				sb.WriteString("- Currency: Stored as dollars/decimal\n")
			}
		}
		sb.WriteString("\n")
	}

	// List entities with their descriptions
	sb.WriteString("## Entities\n\n")
	for _, e := range entities {
		if e.IsDeleted {
			continue // Skip deleted entities
		}
		domain := e.Domain
		if domain == "" {
			domain = "general"
		}
		sb.WriteString(fmt.Sprintf("### %s (%s)\n", e.Name, domain))
		sb.WriteString(fmt.Sprintf("- Table: `%s`\n", e.PrimaryTable))
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("- Description: %s\n", e.Description))
		}
		sb.WriteString("\n")
	}

	// Include column details if available
	if ontology.ColumnDetails != nil && len(ontology.ColumnDetails) > 0 {
		sb.WriteString("## Key Columns\n\n")
		for tableName, columns := range ontology.ColumnDetails {
			// Only show columns with roles (measures, dimensions) or FK associations
			relevantCols := make([]models.ColumnDetail, 0)
			for _, col := range columns {
				if col.Role == "measure" || col.Role == "dimension" || col.FKAssociation != "" {
					relevantCols = append(relevantCols, col)
				}
			}
			if len(relevantCols) > 0 {
				sb.WriteString(fmt.Sprintf("**%s:**\n", tableName))
				for _, col := range relevantCols {
					colInfo := fmt.Sprintf("- `%s`", col.Name)
					if col.Role != "" {
						colInfo += fmt.Sprintf(" [%s]", col.Role)
					}
					if col.Description != "" {
						colInfo += fmt.Sprintf(" - %s", col.Description)
					}
					sb.WriteString(colInfo + "\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("Respond with a JSON array of suggested business terms:\n")
	sb.WriteString("```json\n")
	sb.WriteString(`[
  {
    "term": "Revenue",
    "definition": "Total earned amount from completed transactions after fees",
    "sql_pattern": "SUM(earned_amount) WHERE transaction_state = 'completed'",
    "base_table": "billing_transactions",
    "columns_used": ["earned_amount", "transaction_state"],
    "filters": [
      {"column": "transaction_state", "operator": "=", "values": ["completed"]}
    ],
    "aggregation": "SUM"
  }
]
`)
	sb.WriteString("```\n\n")
	sb.WriteString("Suggest 3-5 key business metrics based on the schema.\n")

	return sb.String()
}

// suggestedTermResponse represents a single term in the LLM response.
type suggestedTermResponse struct {
	Term        string            `json:"term"`
	Definition  string            `json:"definition"`
	SQLPattern  string            `json:"sql_pattern,omitempty"`
	BaseTable   string            `json:"base_table,omitempty"`
	ColumnsUsed []string          `json:"columns_used,omitempty"`
	Filters     []suggestedFilter `json:"filters,omitempty"`
	Aggregation string            `json:"aggregation,omitempty"`
}

type suggestedFilter struct {
	Column   string   `json:"column"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

func (s *glossaryService) parseSuggestTermsResponse(content string, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	suggestions, err := llm.ParseJSONResponse[[]suggestedTermResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse suggestions JSON: %w", err)
	}

	terms := make([]*models.BusinessGlossaryTerm, 0, len(suggestions))
	for _, suggestion := range suggestions {
		// Convert filters
		filters := make([]models.Filter, 0, len(suggestion.Filters))
		for _, f := range suggestion.Filters {
			filters = append(filters, models.Filter{
				Column:   f.Column,
				Operator: f.Operator,
				Values:   f.Values,
			})
		}

		term := &models.BusinessGlossaryTerm{
			ProjectID:   projectID,
			Term:        suggestion.Term,
			Definition:  suggestion.Definition,
			SQLPattern:  suggestion.SQLPattern,
			BaseTable:   suggestion.BaseTable,
			ColumnsUsed: suggestion.ColumnsUsed,
			Filters:     filters,
			Aggregation: suggestion.Aggregation,
			Source:      "suggested",
		}
		terms = append(terms, term)
	}

	return terms, nil
}

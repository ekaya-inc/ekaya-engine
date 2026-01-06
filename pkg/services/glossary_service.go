package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
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

	// GetTermByName returns a single glossary term by name (term or alias).
	GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error)

	// TestSQL validates SQL syntax and captures output columns.
	TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*SQLTestResult, error)

	// CreateAlias adds an alias to an existing glossary term.
	CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error

	// DeleteAlias removes an alias from a glossary term.
	DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error

	// SuggestTerms uses LLM to analyze the ontology and suggest business terms.
	SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)

	// DiscoverGlossaryTerms identifies candidate business terms from ontology.
	// Saves discovered terms to database with source="discovered".
	// Returns count of terms discovered.
	DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error)

	// EnrichGlossaryTerms adds SQL patterns, filters, and aggregations to discovered terms.
	// Processes terms in parallel via LLM calls.
	// Only enriches terms with source="discovered" that lack enrichment.
	EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error
}

// SQLTestResult contains the result of testing a SQL query.
type SQLTestResult struct {
	Valid         bool                   `json:"valid"`
	Error         string                 `json:"error,omitempty"`
	OutputColumns []models.OutputColumn  `json:"output_columns,omitempty"`
	SampleRow     map[string]interface{} `json:"sample_row,omitempty"`
}

type glossaryService struct {
	glossaryRepo   repositories.GlossaryRepository
	ontologyRepo   repositories.OntologyRepository
	entityRepo     repositories.OntologyEntityRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	llmFactory     llm.LLMClientFactory
	logger         *zap.Logger
}

// NewGlossaryService creates a new GlossaryService.
func NewGlossaryService(
	glossaryRepo repositories.GlossaryRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) GlossaryService {
	return &glossaryService{
		glossaryRepo:   glossaryRepo,
		ontologyRepo:   ontologyRepo,
		entityRepo:     entityRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		llmFactory:     llmFactory,
		logger:         logger.Named("glossary-service"),
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
	if term.DefiningSQL == "" {
		return fmt.Errorf("defining_sql is required")
	}

	// Set project ID and default source if not provided
	term.ProjectID = projectID
	if term.Source == "" {
		term.Source = models.GlossarySourceManual
	}

	// Validate SQL and capture output columns
	testResult, err := s.TestSQL(ctx, projectID, term.DefiningSQL)
	if err != nil {
		s.logger.Error("Failed to test SQL during term creation",
			zap.String("project_id", projectID.String()),
			zap.String("term", term.Term),
			zap.Error(err))
		return fmt.Errorf("test SQL: %w", err)
	}

	if !testResult.Valid {
		return fmt.Errorf("SQL validation failed: %s", testResult.Error)
	}

	// Set output columns from test result
	term.OutputColumns = testResult.OutputColumns

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
	if term.DefiningSQL == "" {
		return fmt.Errorf("defining_sql is required")
	}

	// Get existing term to check if SQL changed
	existing, err := s.glossaryRepo.GetByID(ctx, term.ID)
	if err != nil {
		s.logger.Error("Failed to get existing term",
			zap.String("term_id", term.ID.String()),
			zap.Error(err))
		return fmt.Errorf("get existing term: %w", err)
	}

	// If SQL changed, re-validate and update output columns
	if term.DefiningSQL != existing.DefiningSQL {
		testResult, err := s.TestSQL(ctx, term.ProjectID, term.DefiningSQL)
		if err != nil {
			s.logger.Error("Failed to test SQL during term update",
				zap.String("term_id", term.ID.String()),
				zap.String("term", term.Term),
				zap.Error(err))
			return fmt.Errorf("test SQL: %w", err)
		}

		if !testResult.Valid {
			return fmt.Errorf("SQL validation failed: %s", testResult.Error)
		}

		// Update output columns from test result
		term.OutputColumns = testResult.OutputColumns
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

func (s *glossaryService) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	// Try to find by exact term name first
	term, err := s.glossaryRepo.GetByTerm(ctx, projectID, termName)
	if err != nil {
		s.logger.Error("Failed to get glossary term by name",
			zap.String("project_id", projectID.String()),
			zap.String("term_name", termName),
			zap.Error(err))
		return nil, fmt.Errorf("get glossary term by name: %w", err)
	}

	// If not found by term name, try alias lookup
	if term == nil {
		term, err = s.glossaryRepo.GetByAlias(ctx, projectID, termName)
		if err != nil {
			s.logger.Error("Failed to get glossary term by alias",
				zap.String("project_id", projectID.String()),
				zap.String("alias", termName),
				zap.Error(err))
			return nil, fmt.Errorf("get glossary term by alias: %w", err)
		}
	}

	return term, nil
}

func (s *glossaryService) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*SQLTestResult, error) {
	// Extract userID from context (required for datasource adapter)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return &SQLTestResult{
			Valid: false,
			Error: fmt.Sprintf("user ID not found in context: %v", err),
		}, nil
	}

	// Get datasource for the project (assumes one datasource per project)
	datasources, err := s.datasourceSvc.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasources: %w", err)
	}

	if len(datasources) == 0 {
		return &SQLTestResult{
			Valid: false,
			Error: "no datasource configured for project",
		}, nil
	}

	ds := datasources[0] // Use first datasource

	// Create query executor
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, ds.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// First, validate SQL syntax using EXPLAIN (doesn't execute the query)
	// Note: ValidateQuery might not be available on all adapters, so we'll try to execute with LIMIT 1 directly
	// and catch any syntax errors

	// Execute query with LIMIT 1 to capture output columns
	sqlWithLimit := sql
	if !strings.Contains(strings.ToUpper(sql), "LIMIT") {
		sqlWithLimit = fmt.Sprintf("%s LIMIT 1", sql)
	}

	result, err := executor.ExecuteQuery(ctx, sqlWithLimit, 1)
	if err != nil {
		return &SQLTestResult{
			Valid: false,
			Error: err.Error(),
		}, nil
	}

	// Convert ColumnInfo to OutputColumn
	outputColumns := make([]models.OutputColumn, len(result.Columns))
	for i, col := range result.Columns {
		outputColumns[i] = models.OutputColumn{
			Name: col.Name,
			Type: col.Type,
		}
	}

	// Get sample row if available
	var sampleRow map[string]interface{}
	if len(result.Rows) > 0 {
		sampleRow = result.Rows[0]
	}

	return &SQLTestResult{
		Valid:         true,
		OutputColumns: outputColumns,
		SampleRow:     sampleRow,
	}, nil
}

func (s *glossaryService) CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	if err := s.glossaryRepo.CreateAlias(ctx, termID, alias); err != nil {
		s.logger.Error("Failed to create glossary alias",
			zap.String("term_id", termID.String()),
			zap.String("alias", alias),
			zap.Error(err))
		return fmt.Errorf("create glossary alias: %w", err)
	}

	s.logger.Info("Created glossary alias",
		zap.String("term_id", termID.String()),
		zap.String("alias", alias))

	return nil
}

func (s *glossaryService) DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	if alias == "" {
		return fmt.Errorf("alias is required")
	}

	if err := s.glossaryRepo.DeleteAlias(ctx, termID, alias); err != nil {
		s.logger.Error("Failed to delete glossary alias",
			zap.String("term_id", termID.String()),
			zap.String("alias", alias),
			zap.Error(err))
		return fmt.Errorf("delete glossary alias: %w", err)
	}

	s.logger.Info("Deleted glossary alias",
		zap.String("term_id", termID.String()),
		zap.String("alias", alias))

	return nil
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
- A clear business name (term)
- A definition that explains what it measures
- A complete, executable SQL SELECT statement (defining_sql) that computes the metric
- The primary table being queried (base_table)
- Alternative names that business users might use (aliases)

IMPORTANT: The defining_sql must be a complete SELECT statement that can be executed as-is. It should:
- Start with SELECT and include the column aliases that represent the metric
- Include all necessary FROM, JOIN, WHERE, GROUP BY, ORDER BY clauses
- Be ready to execute without modification
- Return meaningful column names that represent the business metric`
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
    "defining_sql": "SELECT SUM(earned_amount) AS revenue\nFROM billing_transactions\nWHERE transaction_state = 'completed'",
    "base_table": "billing_transactions",
    "aliases": ["Total Revenue", "Gross Revenue"]
  }
]
`)
	sb.WriteString("```\n\n")
	sb.WriteString("Suggest 3-5 key business metrics based on the schema.\n")

	return sb.String()
}

// suggestedTermResponse represents a single term in the LLM response.
type suggestedTermResponse struct {
	Term        string   `json:"term"`
	Definition  string   `json:"definition"`
	DefiningSQL string   `json:"defining_sql"`
	BaseTable   string   `json:"base_table,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

func (s *glossaryService) parseSuggestTermsResponse(content string, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	suggestions, err := llm.ParseJSONResponse[[]suggestedTermResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse suggestions JSON: %w", err)
	}

	terms := make([]*models.BusinessGlossaryTerm, 0, len(suggestions))
	for _, suggestion := range suggestions {
		term := &models.BusinessGlossaryTerm{
			ProjectID:   projectID,
			Term:        suggestion.Term,
			Definition:  suggestion.Definition,
			DefiningSQL: suggestion.DefiningSQL,
			BaseTable:   suggestion.BaseTable,
			Aliases:     suggestion.Aliases,
			Source:      models.GlossarySourceInferred,
		}
		terms = append(terms, term)
	}

	return terms, nil
}

func (s *glossaryService) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
	s.logger.Info("Starting glossary term discovery",
		zap.String("project_id", projectID.String()),
		zap.String("ontology_id", ontologyID.String()))

	// Get active ontology for context
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return 0, fmt.Errorf("no active ontology found for project")
	}

	// Get entities for context
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("get entities: %w", err)
	}

	if len(entities) == 0 {
		s.logger.Info("No entities found, skipping term discovery",
			zap.String("project_id", projectID.String()))
		return 0, nil
	}

	// Build context for LLM
	prompt := s.buildSuggestTermsPrompt(ontology, entities)
	systemMessage := s.suggestTermsSystemMessage()

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return 0, fmt.Errorf("LLM generate response: %w", err)
	}

	s.logger.Debug("LLM response received",
		zap.String("project_id", projectID.String()),
		zap.Int("prompt_tokens", result.PromptTokens),
		zap.Int("completion_tokens", result.CompletionTokens))

	// Parse response
	suggestions, err := s.parseSuggestTermsResponse(result.Content, projectID)
	if err != nil {
		return 0, fmt.Errorf("parse LLM response: %w", err)
	}

	// Save discovered terms to database with source="discovered"
	discoveredCount := 0
	for _, term := range suggestions {
		// Check for duplicates
		existing, err := s.glossaryRepo.GetByTerm(ctx, projectID, term.Term)
		if err != nil {
			s.logger.Error("Failed to check for duplicate term",
				zap.String("project_id", projectID.String()),
				zap.String("term", term.Term),
				zap.Error(err))
			continue
		}

		if existing != nil {
			s.logger.Debug("Term already exists, skipping",
				zap.String("term", term.Term))
			continue
		}

		// Set source to "discovered" instead of "suggested"
		term.Source = "discovered"

		// Create the term
		if err := s.glossaryRepo.Create(ctx, term); err != nil {
			s.logger.Error("Failed to create discovered term",
				zap.String("project_id", projectID.String()),
				zap.String("term", term.Term),
				zap.Error(err))
			continue
		}

		discoveredCount++
		s.logger.Debug("Created discovered term",
			zap.String("term", term.Term),
			zap.String("term_id", term.ID.String()))
	}

	s.logger.Info("Completed glossary term discovery",
		zap.String("project_id", projectID.String()),
		zap.Int("terms_discovered", discoveredCount),
		zap.Int("terms_suggested", len(suggestions)))

	return discoveredCount, nil
}

func (s *glossaryService) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	s.logger.Info("Starting glossary term enrichment",
		zap.String("project_id", projectID.String()),
		zap.String("ontology_id", ontologyID.String()))

	// Get terms that need enrichment (discovered terms without sql_pattern)
	allTerms, err := s.glossaryRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get glossary terms: %w", err)
	}

	// Filter for unenriched discovered terms (DefiningSQL is now required, so this is a no-op for new schema)
	var unenrichedTerms []*models.BusinessGlossaryTerm
	for _, term := range allTerms {
		if term.Source == models.GlossarySourceInferred && term.DefiningSQL == "" {
			unenrichedTerms = append(unenrichedTerms, term)
		}
	}

	if len(unenrichedTerms) == 0 {
		s.logger.Info("No unenriched terms found",
			zap.String("project_id", projectID.String()))
		return nil
	}

	s.logger.Info("Found unenriched terms to process",
		zap.Int("count", len(unenrichedTerms)))

	// Get active ontology for context
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology found for project")
	}

	// Get entities for context
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get entities: %w", err)
	}

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Process each term (for now, sequentially - can be parallelized later)
	successCount := 0
	for _, term := range unenrichedTerms {
		prompt := s.buildEnrichTermPrompt(term, ontology, entities)
		systemMessage := s.enrichTermSystemMessage()

		// Call LLM
		result, err := llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.3, false)
		if err != nil {
			s.logger.Error("Failed to enrich term with LLM",
				zap.String("term_id", term.ID.String()),
				zap.String("term", term.Term),
				zap.Error(err))
			continue
		}

		// Parse enrichment response
		enrichment, err := s.parseEnrichTermResponse(result.Content)
		if err != nil {
			s.logger.Error("Failed to parse enrichment response",
				zap.String("term_id", term.ID.String()),
				zap.String("term", term.Term),
				zap.Error(err))
			continue
		}

		// Update term with enrichment (legacy - DefiningSQL should be provided at creation now)
		// This method is deprecated and will be updated in Phase 3
		term.DefiningSQL = enrichment.SQLPattern // Use SQLPattern as DefiningSQL for now
		term.BaseTable = enrichment.BaseTable

		if err := s.glossaryRepo.Update(ctx, term); err != nil {
			s.logger.Error("Failed to update enriched term",
				zap.String("term_id", term.ID.String()),
				zap.String("term", term.Term),
				zap.Error(err))
			continue
		}

		successCount++
		s.logger.Debug("Enriched term",
			zap.String("term", term.Term),
			zap.String("term_id", term.ID.String()))
	}

	s.logger.Info("Completed glossary term enrichment",
		zap.String("project_id", projectID.String()),
		zap.Int("terms_enriched", successCount),
		zap.Int("terms_processed", len(unenrichedTerms)))

	return nil
}

func (s *glossaryService) enrichTermSystemMessage() string {
	return `You are a SQL expert and business analyst. Your task is to generate SQL patterns for calculating business metrics based on database schema context.

For each business term, provide:
1. A SQL pattern showing how to calculate this metric
2. The base table(s) to query
3. The columns involved in the calculation
4. Any filters that should be applied
5. The aggregation function used (if any)

Be specific and use exact table/column names from the provided schema.`
}

func (s *glossaryService) buildEnrichTermPrompt(
	term *models.BusinessGlossaryTerm,
	ontology *models.TieredOntology,
	entities []*models.OntologyEntity,
) string {
	var sb strings.Builder

	sb.WriteString("# Schema Context\n\n")

	// Include domain summary
	if ontology.DomainSummary != nil && ontology.DomainSummary.Description != "" {
		sb.WriteString("## Domain Overview\n\n")
		sb.WriteString(ontology.DomainSummary.Description)
		sb.WriteString("\n\n")
	}

	// Include conventions
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

	// List entities
	sb.WriteString("## Entities\n\n")
	for _, e := range entities {
		if e.IsDeleted {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n", e.Name))
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
			relevantCols := make([]models.ColumnDetail, 0)
			for _, col := range columns {
				if col.Role == "measure" || col.Role == "dimension" {
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

	// Add the term to enrich
	sb.WriteString("## Term to Enrich\n\n")
	sb.WriteString(fmt.Sprintf("**Term:** %s\n", term.Term))
	sb.WriteString(fmt.Sprintf("**Definition:** %s\n\n", term.Definition))

	// Response format
	sb.WriteString("## Response Format\n\n")
	sb.WriteString("Respond with a JSON object containing the enrichment:\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "sql_pattern": "SUM(amount) WHERE status = 'completed'",
  "base_table": "transactions",
  "columns_used": ["amount", "status"],
  "filters": [
    {"column": "status", "operator": "=", "values": ["completed"]}
  ],
  "aggregation": "SUM"
}
`)
	sb.WriteString("```\n")

	return sb.String()
}

type termEnrichment struct {
	SQLPattern  string   `json:"sql_pattern"`
	BaseTable   string   `json:"base_table"`
	ColumnsUsed []string `json:"columns_used"`
	// Filters field removed - not used in DefiningSQL approach
	Aggregation string `json:"aggregation"`
}

func (s *glossaryService) parseEnrichTermResponse(content string) (*termEnrichment, error) {
	enrichment, err := llm.ParseJSONResponse[termEnrichment](content)
	if err != nil {
		return nil, fmt.Errorf("parse enrichment JSON: %w", err)
	}

	return &enrichment, nil
}

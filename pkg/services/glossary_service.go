package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
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
	knowledgeRepo  repositories.KnowledgeRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	llmFactory     llm.LLMClientFactory
	getTenant      TenantContextFunc
	logger         *zap.Logger
}

// NewGlossaryService creates a new GlossaryService.
func NewGlossaryService(
	glossaryRepo repositories.GlossaryRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	getTenant TenantContextFunc,
	logger *zap.Logger,
) GlossaryService {
	return &glossaryService{
		glossaryRepo:   glossaryRepo,
		ontologyRepo:   ontologyRepo,
		entityRepo:     entityRepo,
		knowledgeRepo:  knowledgeRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		llmFactory:     llmFactory,
		getTenant:      getTenant,
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

	if existing == nil {
		return apperrors.ErrNotFound
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
	} else {
		// SQL hasn't changed, preserve existing output columns
		term.OutputColumns = existing.OutputColumns
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

	dsWithStatus := datasources[0] // Use first datasource

	// Check if decryption failed
	if dsWithStatus.DecryptionFailed {
		return &SQLTestResult{
			Valid: false,
			Error: "datasource credentials were encrypted with a different key",
		}, nil
	}

	// Create query executor (empty userID uses shared pool for system operations)
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, dsWithStatus.DatasourceType, dsWithStatus.Config, projectID, dsWithStatus.ID, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Execute query with limit 1 to capture output columns
	// The adapter handles dialect-specific limit wrapping (LIMIT for PostgreSQL, TOP for SQL Server)
	result, err := executor.Query(ctx, sql, 1)
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

	// Fetch project knowledge for domain context
	var knowledgeFacts []*models.KnowledgeFact
	if s.knowledgeRepo != nil {
		knowledgeFacts, err = s.knowledgeRepo.GetByProject(ctx, projectID)
		if err != nil {
			s.logger.Warn("Failed to fetch project knowledge, continuing without it",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without knowledge - don't fail the entire operation
		}
	}

	// Build context for LLM
	prompt := s.buildSuggestTermsPrompt(ontology, entities, knowledgeFacts)
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

	// Filter out inapplicable terms based on entity types
	filtered := filterInapplicableTerms(suggestions, entities)

	s.logger.Info("Generated term suggestions",
		zap.String("project_id", projectID.String()),
		zap.Int("suggested", len(suggestions)),
		zap.Int("filtered", len(filtered)))

	return filtered, nil
}

func (s *glossaryService) suggestTermsSystemMessage() string {
	return `You are a business analyst expert. Your task is to analyze a database schema and identify business metrics and terms SPECIFIC TO THIS DOMAIN.

CRITICAL: Analyze entity names and descriptions to understand the business model BEFORE suggesting terms.

IMPORTANT: Only suggest metrics that apply to the specific business model shown in the schema.
- DO NOT suggest subscription metrics (MRR, ARR, churn, subscriber count) if the model is pay-per-use or transaction-based
- DO NOT suggest inventory metrics (stock, turnover, warehouse) if there is no inventory
- DO NOT suggest e-commerce metrics (AOV, GMV, cart abandonment) if there are no orders/products
- DO NOT suggest generic SaaS metrics unless they are clearly supported by the schema and domain knowledge

When domain knowledge is provided, use it to understand:
- Industry-specific terminology (e.g., "tik" as a billing unit vs generic "session")
- User roles and their meanings (e.g., "host" vs "visitor" vs generic "user")
- Business rules and calculations (e.g., fee structures, revenue splits)
- Platform-specific concepts that differ from generic SaaS patterns

Focus on discovering business terms that are:
1. Directly supported by the schema columns and tables
2. Aligned with the domain knowledge provided
3. Specific to this platform's business model

For each term, provide ONLY:
- term: A clear business name using domain-specific terminology when applicable
- definition: A detailed explanation of what it measures, using the domain's concepts
- aliases: Alternative names that business users might use for this metric

DO NOT include SQL in this response. SQL definitions will be generated separately.
Suggest 5-15 terms that are specific and meaningful for this domain.`
}

func (s *glossaryService) buildSuggestTermsPrompt(ontology *models.TieredOntology, entities []*models.OntologyEntity, knowledgeFacts []*models.KnowledgeFact) string {
	var sb strings.Builder

	sb.WriteString("# Database Schema Analysis for Business Metrics\n\n")

	// Include domain knowledge section if available
	if len(knowledgeFacts) > 0 {
		sb.WriteString("## Domain Knowledge\n\n")
		sb.WriteString("The following domain-specific facts have been captured for this project. Use these to inform your term suggestions:\n\n")

		// Group facts by type for better organization
		factsByType := make(map[string][]*models.KnowledgeFact)
		for _, fact := range knowledgeFacts {
			factsByType[fact.FactType] = append(factsByType[fact.FactType], fact)
		}

		// Order of presentation
		typeOrder := []string{"terminology", "business_rule", "enumeration", "convention"}
		for _, factType := range typeOrder {
			facts, exists := factsByType[factType]
			if !exists || len(facts) == 0 {
				continue
			}

			sb.WriteString(fmt.Sprintf("**%s:**\n", capitalizeWords(factType)))
			for _, fact := range facts {
				sb.WriteString(fmt.Sprintf("- %s", fact.Value))
				if fact.Context != "" {
					sb.WriteString(fmt.Sprintf(" (%s)", fact.Context))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

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

	// Add negative examples section to prevent generic SaaS terms
	sb.WriteString("## What NOT to Suggest\n\n")
	sb.WriteString("Do NOT suggest these generic terms unless the schema clearly supports them:\n")
	sb.WriteString("- \"Active Subscribers\" - only for subscription businesses with recurring billing\n")
	sb.WriteString("- \"Churn Rate\" - only for subscription businesses with membership/plan concepts\n")
	sb.WriteString("- \"Customer Lifetime Value\" - requires purchase history and repeat transactions\n")
	sb.WriteString("- \"Average Order Value\" - requires order/cart system with line items\n")
	sb.WriteString("- \"Inventory Turnover\" - requires inventory management tables\n")
	sb.WriteString("- \"MRR/ARR\" - only for subscription businesses with recurring revenue\n\n")
	sb.WriteString("Instead, look for domain-specific metrics based on:\n")
	sb.WriteString("- What entities actually exist (Engagement, Transaction, Session, etc.)\n")
	sb.WriteString("- What columns track value (amount, fee, revenue, earned_amount)\n")
	sb.WriteString("- What time-based columns exist (duration, start_time, end_time)\n")
	sb.WriteString("- What user roles are distinguished (host, visitor, creator, viewer)\n\n")

	// Add domain-specific hints based on detected patterns
	domainHints := getDomainHints(entities, ontology)
	if len(domainHints) > 0 {
		sb.WriteString("## Domain Analysis\n\n")
		sb.WriteString("Based on the schema structure, the following observations apply to this business:\n\n")
		for _, hint := range domainHints {
			sb.WriteString(fmt.Sprintf("- %s\n", hint))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("Respond with a JSON object containing suggested business terms (NO SQL - just term, definition, aliases):\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "terms": [
    {
      "term": "Revenue",
      "definition": "Total earned amount from completed transactions after deducting platform fees. Calculated by summing the earned_amount field for transactions in 'completed' state.",
      "aliases": ["Total Revenue", "Gross Revenue", "Earnings"]
    },
    {
      "term": "Active Users",
      "definition": "Number of unique users who have engaged with the platform within a defined time frame (typically 30 days). Measured by counting distinct user IDs with recent activity.",
      "aliases": ["MAU", "Monthly Active Users", "Active User Count"]
    }
  ]
}
`)
	sb.WriteString("```\n\n")
	sb.WriteString("Suggest 10-20 key business metrics based on the schema. Be comprehensive!\n")

	return sb.String()
}

// suggestedTermsResponse wraps the array of suggested terms.
type suggestedTermsResponse struct {
	Terms []suggestedTerm `json:"terms"`
}

// suggestedTerm represents a single term in the discovery LLM response.
// Note: DefiningSQL and BaseTable are NOT included - they are generated in the enrichment phase.
type suggestedTerm struct {
	Term       string   `json:"term"`
	Definition string   `json:"definition"`
	Aliases    []string `json:"aliases,omitempty"`
}

func (s *glossaryService) parseSuggestTermsResponse(content string, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	response, err := llm.ParseJSONResponse[suggestedTermsResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse suggestions JSON: %w", err)
	}

	terms := make([]*models.BusinessGlossaryTerm, 0, len(response.Terms))
	for _, suggestion := range response.Terms {
		// Discovery phase only captures term, definition, aliases
		// DefiningSQL and BaseTable are generated in the enrichment phase
		term := &models.BusinessGlossaryTerm{
			ProjectID:  projectID,
			Term:       suggestion.Term,
			Definition: suggestion.Definition,
			Aliases:    suggestion.Aliases,
			Source:     models.GlossarySourceInferred,
			// DefiningSQL is empty - will be filled by enrichment phase
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

	// Fetch project knowledge for domain context
	var knowledgeFacts []*models.KnowledgeFact
	if s.knowledgeRepo != nil {
		knowledgeFacts, err = s.knowledgeRepo.GetByProject(ctx, projectID)
		if err != nil {
			s.logger.Warn("Failed to fetch project knowledge, continuing without it",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without knowledge - don't fail the entire operation
		}
	}

	// Build context for LLM
	prompt := s.buildSuggestTermsPrompt(ontology, entities, knowledgeFacts)
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

	// Filter out inapplicable terms based on entity types
	filtered := filterInapplicableTerms(suggestions, entities)

	s.logger.Debug("Filtered inapplicable terms",
		zap.Int("suggested", len(suggestions)),
		zap.Int("after_filter", len(filtered)))

	// Save discovered terms to database with source="inferred"
	// Note: DefiningSQL is NOT generated in discovery phase - that's done in enrichment
	discoveredCount := 0
	for _, term := range filtered {
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

		// Source is already set to "inferred" in parseSuggestTermsResponse
		// DefiningSQL is empty - will be populated in enrichment phase

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
		zap.Int("terms_suggested", len(suggestions)),
		zap.Int("terms_after_filter", len(filtered)))

	return discoveredCount, nil
}

func (s *glossaryService) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	s.logger.Info("Starting glossary term enrichment",
		zap.String("project_id", projectID.String()),
		zap.String("ontology_id", ontologyID.String()))

	// Get terms that need enrichment (inferred terms without DefiningSQL)
	allTerms, err := s.glossaryRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get glossary terms: %w", err)
	}

	// Filter for unenriched inferred terms
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

	// Process terms in parallel with bounded concurrency
	const maxConcurrency = 5
	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan enrichmentResult, len(unenrichedTerms))

	for _, term := range unenrichedTerms {
		semaphore <- struct{}{} // Acquire
		go func(t *models.BusinessGlossaryTerm) {
			defer func() { <-semaphore }() // Release
			results <- s.enrichSingleTerm(ctx, t, ontology, entities, llmClient, projectID)
		}(term)
	}

	// Collect results
	successCount := 0
	failedCount := 0
	for range unenrichedTerms {
		result := <-results
		if result.err != nil {
			s.logger.Error("Failed to enrich term",
				zap.String("term", result.termName),
				zap.Error(result.err))
			failedCount++
		} else {
			successCount++
			s.logger.Debug("Enriched term",
				zap.String("term", result.termName),
				zap.Int("output_columns", result.outputColumns))
		}
	}

	s.logger.Info("Completed glossary term enrichment",
		zap.String("project_id", projectID.String()),
		zap.Int("terms_enriched", successCount),
		zap.Int("terms_failed", failedCount),
		zap.Int("terms_processed", len(unenrichedTerms)))

	return nil
}

// enrichmentResult holds the result of enriching a single term.
type enrichmentResult struct {
	termName      string
	outputColumns int
	err           error
}

// enrichSingleTerm generates SQL for a single term, validates it, and updates the database.
// Each call acquires its own database connection from the pool to allow parallel execution.
func (s *glossaryService) enrichSingleTerm(
	ctx context.Context,
	term *models.BusinessGlossaryTerm,
	ontology *models.TieredOntology,
	entities []*models.OntologyEntity,
	llmClient llm.LLMClient,
	projectID uuid.UUID,
) enrichmentResult {
	// Acquire own database connection FIRST - this ensures the LLM client's SavePending
	// uses this goroutine's dedicated connection, avoiding "conn busy" errors when
	// multiple goroutines share the parent context's connection.
	tenantCtx, cleanup, err := s.getTenant(ctx, projectID)
	if err != nil {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("get tenant context: %w", err)}
	}
	defer cleanup()

	prompt := s.buildEnrichTermPrompt(term, ontology, entities)
	systemMessage := s.enrichTermSystemMessage()

	// Call LLM to generate SQL (uses tenantCtx so SavePending has its own connection)
	result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("LLM generate: %w", err)}
	}

	// Parse enrichment response
	enrichment, err := s.parseEnrichTermResponse(result.Content)
	if err != nil {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("parse response: %w", err)}
	}

	if enrichment.DefiningSQL == "" {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("LLM returned empty SQL")}
	}

	// Validate SQL and capture output columns (uses tenantCtx for datasource lookup)
	testResult, err := s.TestSQL(tenantCtx, projectID, enrichment.DefiningSQL)
	if err != nil {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("test SQL: %w", err)}
	}

	if !testResult.Valid {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("SQL validation failed: %s", testResult.Error)}
	}

	// Update term with enrichment and output columns
	term.DefiningSQL = enrichment.DefiningSQL
	term.BaseTable = enrichment.BaseTable
	term.OutputColumns = testResult.OutputColumns
	// Merge aliases - keep existing ones and add new ones from enrichment
	if len(enrichment.Aliases) > 0 {
		aliasSet := make(map[string]bool)
		for _, a := range term.Aliases {
			aliasSet[a] = true
		}
		for _, a := range enrichment.Aliases {
			if !aliasSet[a] {
				term.Aliases = append(term.Aliases, a)
			}
		}
	}

	if err := s.glossaryRepo.Update(tenantCtx, term); err != nil {
		return enrichmentResult{termName: term.Term, err: fmt.Errorf("update term: %w", err)}
	}

	return enrichmentResult{termName: term.Term, outputColumns: len(testResult.OutputColumns)}
}

func (s *glossaryService) enrichTermSystemMessage() string {
	return `You are a SQL expert and business analyst. Your task is to generate complete, executable SQL definitions for business metrics based on database schema context.

For each business term, provide:
- A complete, executable SQL SELECT statement (defining_sql) that computes the metric
- The primary table being queried (base_table)
- Alternative names that business users might use (aliases)

IMPORTANT: The defining_sql must be a complete SELECT statement that can be executed as-is. It should:
- Start with SELECT and include column aliases that represent the metric
- Include all necessary FROM, JOIN, WHERE, GROUP BY, ORDER BY clauses
- Be a definition/calculation of the metric, not just a fragment
- Be ready to execute without modification
- Return meaningful column names that business users will understand

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
  "defining_sql": "SELECT SUM(amount) AS total_revenue\nFROM transactions\nWHERE status = 'completed'",
  "base_table": "transactions",
  "aliases": ["Total Revenue", "Gross Revenue"]
}
`)
	sb.WriteString("```\n")

	return sb.String()
}

type termEnrichment struct {
	DefiningSQL string   `json:"defining_sql"`
	BaseTable   string   `json:"base_table"`
	Aliases     []string `json:"aliases"`
}

func (s *glossaryService) parseEnrichTermResponse(content string) (*termEnrichment, error) {
	enrichment, err := llm.ParseJSONResponse[termEnrichment](content)
	if err != nil {
		return nil, fmt.Errorf("parse enrichment JSON: %w", err)
	}

	return &enrichment, nil
}

// capitalizeWords capitalizes the first letter of each word in a string,
// treating underscores as word separators.
func capitalizeWords(s string) string {
	words := strings.Split(strings.ReplaceAll(s, "_", " "), " ")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// getDomainHints analyzes entities and column details to detect domain patterns
// and returns hints to guide the LLM toward domain-specific term suggestions.
func getDomainHints(entities []*models.OntologyEntity, ontology *models.TieredOntology) []string {
	var hints []string

	// Detect patterns from entity names
	hasEngagement := containsEntityByName(entities, "engagement", "session", "meeting", "call", "booking")
	hasSubscription := containsEntityByName(entities, "subscription", "plan", "membership", "tier")
	hasBilling := containsEntityByName(entities, "billing", "transaction", "payment", "invoice", "charge")
	hasInventory := containsEntityByName(entities, "inventory", "product", "stock", "warehouse", "sku")
	hasEcommerce := containsEntityByName(entities, "order", "cart", "checkout", "purchase")

	// Check for distinct user roles in column details
	hasUserRoles := hasRoleDistinctingColumns(ontology)

	// Generate domain-specific hints
	if hasEngagement && !hasSubscription {
		hints = append(hints, "This appears to be an engagement/session-based business, not subscription-based. Focus on per-engagement metrics rather than recurring revenue metrics.")
	}

	if hasBilling && !hasSubscription {
		hints = append(hints, "Focus on transaction-based metrics (revenue per engagement, fees, payouts, transaction volume) rather than subscription metrics (MRR, ARR, churn).")
	}

	if hasUserRoles {
		hints = append(hints, "There are distinct user roles (e.g., host/visitor, creator/viewer, buyer/seller). Consider role-specific metrics for each participant type.")
	}

	if !hasInventory && !hasEcommerce {
		hints = append(hints, "This is not an e-commerce or inventory-based business. Do not suggest inventory metrics (stock levels, turnover) or order-based metrics (AOV, cart abandonment).")
	}

	if hasSubscription {
		hints = append(hints, "This appears to be a subscription-based business. Consider recurring revenue metrics (MRR, ARR, churn, subscriber lifetime value).")
	}

	return hints
}

// containsEntityByName checks if any entity name contains one of the specified keywords.
// The comparison is case-insensitive and matches substrings.
func containsEntityByName(entities []*models.OntologyEntity, keywords ...string) bool {
	for _, entity := range entities {
		if entity.IsDeleted {
			continue
		}
		nameLower := strings.ToLower(entity.Name)
		tableLower := strings.ToLower(entity.PrimaryTable)
		for _, keyword := range keywords {
			keywordLower := strings.ToLower(keyword)
			if strings.Contains(nameLower, keywordLower) || strings.Contains(tableLower, keywordLower) {
				return true
			}
		}
	}
	return false
}

// hasRoleDistinctingColumns checks if the ontology contains columns that indicate
// distinct user roles (e.g., host_id, visitor_id, creator_id, buyer_id).
// These columns suggest the platform has different participant types with potentially
// different metrics for each role.
func hasRoleDistinctingColumns(ontology *models.TieredOntology) bool {
	if ontology == nil || ontology.ColumnDetails == nil {
		return false
	}

	// Role-indicating column patterns (beyond simple user_id)
	rolePatterns := []string{
		"host_id", "visitor_id", "creator_id", "viewer_id",
		"buyer_id", "seller_id", "sender_id", "receiver_id",
		"payer_id", "payee_id", "owner_id", "member_id",
		"author_id", "performer_id", "attendee_id", "participant_id",
	}

	roleCount := 0
	for _, columns := range ontology.ColumnDetails {
		for _, col := range columns {
			colLower := strings.ToLower(col.Name)
			for _, pattern := range rolePatterns {
				if colLower == pattern {
					roleCount++
					if roleCount >= 2 {
						// Need at least 2 distinct role columns to indicate role-based business
						return true
					}
				}
			}
			// Also check FKAssociation for role semantics
			if col.FKAssociation != "" {
				assocLower := strings.ToLower(col.FKAssociation)
				if strings.Contains(assocLower, "host") || strings.Contains(assocLower, "visitor") ||
					strings.Contains(assocLower, "buyer") || strings.Contains(assocLower, "seller") ||
					strings.Contains(assocLower, "payer") || strings.Contains(assocLower, "payee") {
					roleCount++
					if roleCount >= 2 {
						return true
					}
				}
			}
		}
	}
	return false
}

// filterInapplicableTerms removes glossary terms that don't match the domain's
// business model based on detected entity types. This provides a safety net after
// LLM generation to catch generic SaaS terms that slipped through prompt guidance.
func filterInapplicableTerms(
	terms []*models.BusinessGlossaryTerm,
	entities []*models.OntologyEntity,
) []*models.BusinessGlossaryTerm {
	// Terms that require subscription-based business model entities
	subscriptionTerms := []string{
		"subscriber", "subscription", "churn", "mrr", "arr",
		"monthly recurring", "annual recurring", "recurring revenue",
	}

	// Terms that require inventory management entities
	inventoryTerms := []string{
		"inventory", "stock", "warehouse", "turnover",
		"stock level", "reorder", "stockout",
	}

	// Terms that require e-commerce/order entities
	ecommerceTerms := []string{
		"order value", "cart", "checkout", "aov", "gmv",
		"average order", "gross merchandise",
	}

	// Detect what entity types exist in the schema
	hasSubscription := containsEntityByName(entities, "subscription", "plan", "membership", "tier")
	hasInventory := containsEntityByName(entities, "inventory", "product", "stock", "warehouse", "sku")
	hasEcommerce := containsEntityByName(entities, "order", "cart", "checkout", "purchase")

	var filtered []*models.BusinessGlossaryTerm
	for _, term := range terms {
		termLower := strings.ToLower(term.Term)

		// Skip subscription terms if no subscription entities
		if !hasSubscription && matchesAny(termLower, subscriptionTerms) {
			continue
		}
		// Skip inventory terms if no inventory entities
		if !hasInventory && matchesAny(termLower, inventoryTerms) {
			continue
		}
		// Skip e-commerce terms if no e-commerce entities
		if !hasEcommerce && matchesAny(termLower, ecommerceTerms) {
			continue
		}

		filtered = append(filtered, term)
	}
	return filtered
}

// matchesAny checks if the term contains any of the specified patterns.
// The comparison is case-insensitive substring matching.
func matchesAny(term string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(term, pattern) {
			return true
		}
	}
	return false
}

package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ErrTestTermRejected is returned when a term name matches test-like patterns.
var ErrTestTermRejected = errors.New("term name appears to be test data")

// testTermPatterns contains regex patterns to detect test-like term names.
// Terms matching these patterns are rejected to prevent test data from
// being persisted in the glossary.
//
// IMPORTANT: Keep in sync with testTermPatterns in scripts/cleanup-test-data/main.go
var testTermPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^test`),    // Starts with "test"
	regexp.MustCompile(`(?i)test$`),    // Ends with "test"
	regexp.MustCompile(`(?i)^uitest`),  // UI test prefix
	regexp.MustCompile(`(?i)^debug`),   // Debug prefix
	regexp.MustCompile(`(?i)^todo`),    // Todo prefix
	regexp.MustCompile(`(?i)^fixme`),   // Fixme prefix
	regexp.MustCompile(`(?i)^dummy`),   // Dummy prefix
	regexp.MustCompile(`(?i)^sample`),  // Sample prefix
	regexp.MustCompile(`(?i)^example`), // Example prefix
	regexp.MustCompile(`\d{4}$`),       // Ends with 4 digits (e.g., Term2026)
}

// IsTestTerm checks if a term name matches any test-like pattern.
// This is exported for use by MCP tools that need to validate terms
// before calling service methods.
func IsTestTerm(termName string) bool {
	for _, pattern := range testTermPatterns {
		if pattern.MatchString(termName) {
			return true
		}
	}
	return false
}

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
	knowledgeRepo  repositories.KnowledgeRepository
	schemaRepo     repositories.SchemaRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	llmFactory     llm.LLMClientFactory
	getTenant      TenantContextFunc
	logger         *zap.Logger
	env            string
}

// NewGlossaryService creates a new GlossaryService.
// The env parameter specifies the runtime environment (e.g., "local", "dev", "production").
// In production, test-like terms are rejected entirely.
// In non-production environments, test-like terms are allowed but logged as warnings.
func NewGlossaryService(
	glossaryRepo repositories.GlossaryRepository,
	ontologyRepo repositories.OntologyRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	schemaRepo repositories.SchemaRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	getTenant TenantContextFunc,
	logger *zap.Logger,
	env string,
) GlossaryService {
	return &glossaryService{
		glossaryRepo:   glossaryRepo,
		ontologyRepo:   ontologyRepo,
		knowledgeRepo:  knowledgeRepo,
		schemaRepo:     schemaRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		llmFactory:     llmFactory,
		getTenant:      getTenant,
		logger:         logger.Named("glossary-service"),
		env:            env,
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

	// Handle test-like term names based on environment
	if IsTestTerm(term.Term) {
		if s.env == "production" {
			return fmt.Errorf("test data not allowed in production: %s", term.Term)
		}
		s.logger.Warn("Creating test-like glossary term",
			zap.String("term", term.Term),
			zap.String("env", s.env))
	}

	// Set project ID and default source if not provided
	term.ProjectID = projectID
	if term.Source == "" {
		term.Source = models.GlossarySourceManual
	}

	// Get active ontology and set ontology_id for proper CASCADE delete and uniqueness
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology found for project")
	}
	term.OntologyID = &ontology.ID

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

	// Handle test-like term names based on environment
	if IsTestTerm(term.Term) {
		if s.env == "production" {
			return fmt.Errorf("test data not allowed in production: %s", term.Term)
		}
		s.logger.Warn("Updating to test-like glossary term",
			zap.String("term", term.Term),
			zap.String("env", s.env))
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

	// Execute query with limit 5 to check for multi-row results
	// The adapter handles dialect-specific limit wrapping (LIMIT for PostgreSQL, TOP for SQL Server)
	result, err := executor.Query(ctx, sql, 5)
	if err != nil {
		return &SQLTestResult{
			Valid: false,
			Error: err.Error(),
		}, nil
	}

	// Check for single-row result (for aggregate metrics)
	// Glossary terms should define aggregate metrics that return a single row
	if len(result.Rows) > 1 {
		return &SQLTestResult{
			Valid: false,
			Error: "Query returns multiple rows. Aggregate metrics should return a single row.",
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

	// Get tables for context (replaces entity concept)
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, uuid.Nil, true)
	if err != nil {
		return nil, fmt.Errorf("get tables: %w", err)
	}

	if len(tables) == 0 {
		s.logger.Info("No tables found, skipping term suggestion",
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
	prompt := s.buildSuggestTermsPrompt(ontology, tables, knowledgeFacts)
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

	// Filter out inapplicable terms based on table names
	filtered := filterInapplicableTerms(suggestions, tables)

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

func (s *glossaryService) buildSuggestTermsPrompt(ontology *models.TieredOntology, tables []*models.SchemaTable, knowledgeFacts []*models.KnowledgeFact) string {
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

	// List tables with their descriptions
	sb.WriteString("## Tables\n\n")
	for _, t := range tables {
		sb.WriteString(fmt.Sprintf("### %s\n", t.TableName))
		if t.BusinessName != nil && *t.BusinessName != "" {
			sb.WriteString(fmt.Sprintf("- Business Name: %s\n", *t.BusinessName))
		}
		if t.Description != nil && *t.Description != "" {
			sb.WriteString(fmt.Sprintf("- Description: %s\n", *t.Description))
		}
		sb.WriteString("\n")
	}

	// Include column details if available
	if len(ontology.ColumnDetails) > 0 {
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
	sb.WriteString("- What tables actually exist (Engagement, Transaction, Session, etc.)\n")
	sb.WriteString("- What columns track value (amount, fee, revenue, earned_amount)\n")
	sb.WriteString("- What time-based columns exist (duration, start_time, end_time)\n")
	sb.WriteString("- What user roles are distinguished (host, visitor, creator, viewer)\n\n")

	// Add domain-specific hints based on detected patterns
	domainHints := getDomainHints(tables, ontology)
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
	sb.WriteString("Suggest 5-15 domain-specific business terms based on the schema. Focus on quality over quantity.\n")

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

	// Get tables for context (replaces entity concept)
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, uuid.Nil, true)
	if err != nil {
		return 0, fmt.Errorf("get tables: %w", err)
	}

	if len(tables) == 0 {
		s.logger.Info("No tables found, skipping term discovery",
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
	prompt := s.buildSuggestTermsPrompt(ontology, tables, knowledgeFacts)
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

	// Filter out inapplicable terms based on table names
	filtered := filterInapplicableTerms(suggestions, tables)

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

		// Set ontology_id to link term to ontology lifecycle (CASCADE delete)
		term.OntologyID = &ontologyID

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

	// Get tables for context (replaces entity concept)
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, uuid.Nil, true)
	if err != nil {
		return fmt.Errorf("get tables: %w", err)
	}

	// Get schema columns for accurate column reference (prevents LLM hallucinations)
	var schemaColumnsByTable map[string][]*models.SchemaColumn
	if s.schemaRepo != nil {
		// Get all table names
		tableNames := make([]string, 0, len(tables))
		for _, t := range tables {
			tableNames = append(tableNames, t.TableName)
		}
		// Fetch columns for all tables in one query
		schemaColumnsByTable, err = s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames, false)
		if err != nil {
			s.logger.Warn("Failed to get schema columns, continuing without",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			schemaColumnsByTable = nil
		} else {
			s.logger.Debug("Loaded schema columns for enrichment",
				zap.Int("tables", len(schemaColumnsByTable)))
		}
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
			results <- s.enrichSingleTerm(ctx, t, ontology, tables, schemaColumnsByTable, llmClient, projectID)
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
// On failure, it retries with enhanced context (more schema detail, examples) before giving up.
func (s *glossaryService) enrichSingleTerm(
	ctx context.Context,
	term *models.BusinessGlossaryTerm,
	ontology *models.TieredOntology,
	tables []*models.SchemaTable,
	schemaColumnsByTable map[string][]*models.SchemaColumn,
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

	// First attempt: normal enrichment
	result, firstErr := s.tryEnrichTerm(tenantCtx, term, ontology, tables, schemaColumnsByTable, llmClient, projectID, false, "")
	if firstErr == nil {
		return result
	}

	// Retry with enhanced context (includes more schema detail, examples)
	s.logger.Debug("Retrying enrichment with enhanced context",
		zap.String("term", term.Term),
		zap.Error(firstErr))

	result, retryErr := s.tryEnrichTerm(tenantCtx, term, ontology, tables, schemaColumnsByTable, llmClient, projectID, true, firstErr.Error())
	if retryErr == nil {
		s.logger.Info("Enrichment succeeded on retry with enhanced context",
			zap.String("term", term.Term))
		return result
	}

	// Both attempts failed - update term with failure status before returning error
	term.EnrichmentStatus = models.GlossaryEnrichmentFailed
	term.EnrichmentError = retryErr.Error()
	if updateErr := s.glossaryRepo.Update(tenantCtx, term); updateErr != nil {
		s.logger.Error("Failed to save enrichment failure status",
			zap.String("term", term.Term),
			zap.Error(updateErr))
	}

	return enrichmentResult{termName: term.Term, err: fmt.Errorf("enrichment failed after retry: %w", retryErr)}
}

// tryEnrichTerm attempts to enrich a single term with LLM-generated SQL.
// When enhanced=true, the prompt includes ALL columns (not just measures/dimensions),
// additional examples for complex metrics, and context from the previous failure.
func (s *glossaryService) tryEnrichTerm(
	ctx context.Context,
	term *models.BusinessGlossaryTerm,
	ontology *models.TieredOntology,
	tables []*models.SchemaTable,
	schemaColumnsByTable map[string][]*models.SchemaColumn,
	llmClient llm.LLMClient,
	projectID uuid.UUID,
	enhanced bool,
	previousError string,
) (enrichmentResult, error) {
	var prompt string
	if enhanced {
		prompt = s.buildEnhancedEnrichTermPrompt(term, ontology, tables, schemaColumnsByTable, previousError)
	} else {
		prompt = s.buildEnrichTermPrompt(term, ontology, tables, schemaColumnsByTable)
	}
	systemMessage := s.enrichTermSystemMessage()

	// Call LLM to generate SQL (uses tenantCtx so SavePending has its own connection)
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return enrichmentResult{termName: term.Term}, fmt.Errorf("LLM generate: %w", err)
	}

	// Parse enrichment response
	enrichment, err := s.parseEnrichTermResponse(result.Content)
	if err != nil {
		return enrichmentResult{termName: term.Term}, fmt.Errorf("parse response: %w", err)
	}

	if enrichment.DefiningSQL == "" {
		return enrichmentResult{termName: term.Term}, fmt.Errorf("LLM returned empty SQL")
	}

	// Validate column references before executing SQL
	// This catches LLM hallucinations (e.g., "started_at" instead of "created_at")
	// and returns a descriptive error for retry with enhanced context
	if len(schemaColumnsByTable) > 0 {
		if columnErrors := validateColumnReferences(enrichment.DefiningSQL, schemaColumnsByTable); len(columnErrors) > 0 {
			return enrichmentResult{termName: term.Term}, fmt.Errorf("%s", formatColumnValidationError(columnErrors))
		}
	}

	// Validate SQL and capture output columns (uses tenantCtx for datasource lookup)
	testResult, err := s.TestSQL(ctx, projectID, enrichment.DefiningSQL)
	if err != nil {
		return enrichmentResult{termName: term.Term}, fmt.Errorf("test SQL: %w", err)
	}

	if !testResult.Valid {
		return enrichmentResult{termName: term.Term}, fmt.Errorf("SQL validation failed: %s", testResult.Error)
	}

	// Check for potential enum value mismatches (best-effort validation)
	// This logs warnings but doesn't fail the enrichment
	if mismatches := validateEnumValues(enrichment.DefiningSQL, ontology); len(mismatches) > 0 {
		for _, mismatch := range mismatches {
			s.logger.Warn("Potential enum value mismatch in generated SQL",
				zap.String("term", term.Term),
				zap.String("sql_value", mismatch.SQLValue),
				zap.String("table", mismatch.Table),
				zap.String("column", mismatch.Column),
				zap.String("suggested_value", mismatch.BestMatch),
				zap.Strings("actual_values", mismatch.ActualValues))
		}
	}

	// Update term with enrichment and output columns
	term.DefiningSQL = enrichment.DefiningSQL
	term.BaseTable = enrichment.BaseTable
	term.OutputColumns = testResult.OutputColumns
	term.EnrichmentStatus = models.GlossaryEnrichmentSuccess
	term.EnrichmentError = "" // Clear any previous error
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

	if err := s.glossaryRepo.Update(ctx, term); err != nil {
		return enrichmentResult{termName: term.Term}, fmt.Errorf("update term: %w", err)
	}

	return enrichmentResult{termName: term.Term, outputColumns: len(testResult.OutputColumns)}, nil
}

func (s *glossaryService) enrichTermSystemMessage() string {
	return `You are a SQL expert and business analyst. Your task is to generate complete, executable SQL definitions for business metrics based on database schema context.

For each business term, provide:
- A complete, executable SQL SELECT statement (defining_sql) that computes the metric
- The primary table being queried (base_table)
- Alternative names that business users might use (aliases)

CRITICAL REQUIREMENTS:
1. The SQL MUST return exactly ONE row (aggregate/summary metrics)
2. Do NOT use UNION/UNION ALL unless combining results into a single row
3. The formula must match the term name semantically:
   - "Average X Per Y" → SUM(X) / COUNT(Y), not a percentage calculation
   - "X Rate" → X per unit time (e.g., revenue per hour)
   - "X Ratio" → X / Total, typically as a percentage
   - "Total X" → SUM(X), a simple aggregate
   - "X Utilization" → used / authorized, as a percentage (0-100)
   - "X Count" → COUNT of items matching criteria

IMPORTANT: The defining_sql must be a complete SELECT statement that can be executed as-is. It should:
- Start with SELECT and include column aliases that represent the metric
- Include all necessary FROM, JOIN, WHERE, GROUP BY, ORDER BY clauses
- Be a definition/calculation of the metric, not just a fragment
- Be ready to execute without modification
- Return meaningful column names that business users will understand

IMPORTANT: When filtering on enumeration columns, use the EXACT values provided in the schema context.
Do NOT simplify or normalize enum values (e.g., use 'TRANSACTION_STATE_ENDED' not 'ended').

EXAMPLES FOR COMPLEX METRICS:

For utilization/conversion rates (percentage of items in a specific state):
{
  "defining_sql": "SELECT COUNT(*) FILTER (WHERE status = 'used') * 100.0 / NULLIF(COUNT(*), 0) AS utilization_rate FROM offers WHERE created_at >= NOW() - INTERVAL '30 days'",
  "base_table": "offers",
  "aliases": ["usage rate", "redemption rate"]
}

For participation rates (distinct participants vs total eligible):
{
  "defining_sql": "SELECT COUNT(DISTINCT r.referrer_id) * 100.0 / NULLIF((SELECT COUNT(*) FROM users WHERE is_eligible = true), 0) AS participation_rate FROM referrals r WHERE r.bonus_paid = true",
  "base_table": "referrals",
  "aliases": ["adoption rate", "enrollment rate"]
}

For completion rates (successful vs total attempts):
{
  "defining_sql": "SELECT COUNT(*) FILTER (WHERE state = 'COMPLETED') * 100.0 / NULLIF(COUNT(*), 0) AS completion_rate FROM transactions",
  "base_table": "transactions",
  "aliases": ["success rate", "fulfillment rate"]
}

For averages with conditional filtering:
{
  "defining_sql": "SELECT AVG(duration_seconds) FILTER (WHERE state = 'COMPLETED') AS avg_duration FROM sessions",
  "base_table": "sessions",
  "aliases": ["mean duration", "average session length"]
}

For metrics requiring multi-table joins:
{
  "defining_sql": "SELECT u.id AS user_id, COUNT(t.id) AS transaction_count, COALESCE(SUM(t.amount), 0) AS total_amount FROM users u LEFT JOIN transactions t ON u.id = t.user_id GROUP BY u.id",
  "base_table": "users",
  "aliases": ["user transaction summary"]
}

Be specific and use exact table/column names from the provided schema.`
}

func (s *glossaryService) buildEnrichTermPrompt(
	term *models.BusinessGlossaryTerm,
	ontology *models.TieredOntology,
	tables []*models.SchemaTable,
	schemaColumnsByTable map[string][]*models.SchemaColumn,
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

	// List tables
	sb.WriteString("## Tables\n\n")
	for _, t := range tables {
		sb.WriteString(fmt.Sprintf("### %s\n", t.TableName))
		if t.BusinessName != nil && *t.BusinessName != "" {
			sb.WriteString(fmt.Sprintf("- Business Name: %s\n", *t.BusinessName))
		}
		if t.Description != nil && *t.Description != "" {
			sb.WriteString(fmt.Sprintf("- Description: %s\n", *t.Description))
		}
		sb.WriteString("\n")
	}

	// Include actual schema columns - the ground truth for SQL generation
	// This section provides the EXACT column names and types that exist in the database
	if len(schemaColumnsByTable) > 0 {
		sb.WriteString("## Available Columns (EXACT names - use these in your SQL)\n\n")
		sb.WriteString("IMPORTANT: Only use column names listed below. Do NOT invent or guess column names.\n\n")
		for tableName, columns := range schemaColumnsByTable {
			sb.WriteString(fmt.Sprintf("**%s:**\n", tableName))
			for _, col := range columns {
				colInfo := fmt.Sprintf("- `%s` (%s)", col.ColumnName, col.DataType)
				if col.IsPrimaryKey {
					colInfo += " [PK]"
				}
				if col.Description != nil && *col.Description != "" {
					colInfo += fmt.Sprintf(" - %s", *col.Description)
				}
				sb.WriteString(colInfo + "\n")

				// Include sample values for low-cardinality columns (helps LLM use correct values)
				if len(col.SampleValues) > 0 && len(col.SampleValues) <= 10 {
					quotedValues := make([]string, len(col.SampleValues))
					for i, v := range col.SampleValues {
						quotedValues[i] = fmt.Sprintf("'%s'", v)
					}
					sb.WriteString(fmt.Sprintf("  Sample values: %s\n", strings.Join(quotedValues, ", ")))
				}
			}
			sb.WriteString("\n")
		}

		// Add common column confusion warnings based on actual schema
		confusions := s.detectColumnConfusions(schemaColumnsByTable)
		if len(confusions) > 0 {
			sb.WriteString("## IMPORTANT: Common Column Mistakes to Avoid\n\n")
			for _, warning := range confusions {
				sb.WriteString(fmt.Sprintf("- %s\n", warning))
			}
			sb.WriteString("\n")
		}

		// Add type comparison guidance to prevent type mismatch errors
		typeGuidance := generateTypeComparisonGuidance(schemaColumnsByTable)
		if typeGuidance != "" {
			sb.WriteString(typeGuidance)
		}
	}

	// Include semantic column details from ontology (enriched information)
	if len(ontology.ColumnDetails) > 0 {
		sb.WriteString("## Column Semantics\n\n")
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

					// Include enum values if present so LLM uses exact values in SQL
					if len(col.EnumValues) > 0 {
						values := make([]string, len(col.EnumValues))
						for i, v := range col.EnumValues {
							values[i] = fmt.Sprintf("'%s'", v.Value)
						}
						sb.WriteString(fmt.Sprintf("  Allowed values: %s\n", strings.Join(values, ", ")))
					}
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

// buildEnhancedEnrichTermPrompt builds a more detailed prompt for retry attempts.
// It includes ALL columns (not just measures/dimensions), additional examples for
// complex metrics like utilization rates and percentages, and context from the
// previous failure to help the LLM avoid the same mistake.
func (s *glossaryService) buildEnhancedEnrichTermPrompt(
	term *models.BusinessGlossaryTerm,
	ontology *models.TieredOntology,
	tables []*models.SchemaTable,
	schemaColumnsByTable map[string][]*models.SchemaColumn,
	previousError string,
) string {
	var sb strings.Builder

	sb.WriteString("# Schema Context (Enhanced Detail)\n\n")

	// Include previous error context to help LLM avoid the same mistake
	if previousError != "" {
		sb.WriteString("## Previous Attempt Failed\n\n")
		sb.WriteString("The first attempt to generate SQL for this term failed with the following error:\n")
		sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", previousError))
		sb.WriteString("Please analyze this error and generate valid SQL that avoids this issue.\n\n")
	}

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

	// List tables with descriptions
	sb.WriteString("## Tables\n\n")
	for _, t := range tables {
		sb.WriteString(fmt.Sprintf("### %s\n", t.TableName))
		if t.BusinessName != nil && *t.BusinessName != "" {
			sb.WriteString(fmt.Sprintf("- Business Name: %s\n", *t.BusinessName))
		}
		if t.Description != nil && *t.Description != "" {
			sb.WriteString(fmt.Sprintf("- Description: %s\n", *t.Description))
		}
		sb.WriteString("\n")
	}

	// Include actual schema columns - the ground truth for SQL generation (enhanced prompt)
	if len(schemaColumnsByTable) > 0 {
		sb.WriteString("## EXACT Available Columns (use ONLY these column names)\n\n")
		sb.WriteString("CRITICAL: The following is the complete list of columns that exist in the database.\n")
		sb.WriteString("Do NOT use any column name that is not listed here. Column names are case-sensitive.\n\n")
		for tableName, columns := range schemaColumnsByTable {
			sb.WriteString(fmt.Sprintf("**%s:**\n", tableName))
			for _, col := range columns {
				colInfo := fmt.Sprintf("- `%s` (%s)", col.ColumnName, col.DataType)
				if col.IsPrimaryKey {
					colInfo += " [PK]"
				}
				if col.Description != nil && *col.Description != "" {
					colInfo += fmt.Sprintf(" - %s", *col.Description)
				}
				sb.WriteString(colInfo + "\n")

				// Include sample values for low-cardinality columns
				if len(col.SampleValues) > 0 && len(col.SampleValues) <= 10 {
					quotedValues := make([]string, len(col.SampleValues))
					for i, v := range col.SampleValues {
						quotedValues[i] = fmt.Sprintf("'%s'", v)
					}
					sb.WriteString(fmt.Sprintf("  Sample values: %s\n", strings.Join(quotedValues, ", ")))
				}
			}
			sb.WriteString("\n")
		}

		// Add common column confusion warnings
		confusions := s.detectColumnConfusions(schemaColumnsByTable)
		if len(confusions) > 0 {
			sb.WriteString("## CRITICAL: Column Mistakes to Avoid\n\n")
			for _, warning := range confusions {
				sb.WriteString(fmt.Sprintf("- %s\n", warning))
			}
			sb.WriteString("\n")
		}

		// Add type comparison guidance to prevent type mismatch errors
		typeGuidance := generateTypeComparisonGuidance(schemaColumnsByTable)
		if typeGuidance != "" {
			sb.WriteString(typeGuidance)
		}
	}

	// Include semantic column details from ontology
	if len(ontology.ColumnDetails) > 0 {
		sb.WriteString("## Column Semantics and Roles\n\n")
		for tableName, columns := range ontology.ColumnDetails {
			sb.WriteString(fmt.Sprintf("**%s:**\n", tableName))
			for _, col := range columns {
				colInfo := fmt.Sprintf("- `%s`", col.Name)
				if col.Role != "" {
					colInfo += fmt.Sprintf(" [%s]", col.Role)
				}
				if col.IsPrimaryKey {
					colInfo += " (PK)"
				}
				if col.IsForeignKey {
					colInfo += fmt.Sprintf(" (FK→%s)", col.ForeignTable)
				}
				if col.Description != "" {
					colInfo += fmt.Sprintf(" - %s", col.Description)
				}
				sb.WriteString(colInfo + "\n")

				// Include enum values if present so LLM uses exact values in SQL
				if len(col.EnumValues) > 0 {
					values := make([]string, len(col.EnumValues))
					for i, v := range col.EnumValues {
						if v.Description != "" {
							values[i] = fmt.Sprintf("'%s' (%s)", v.Value, v.Description)
						} else {
							values[i] = fmt.Sprintf("'%s'", v.Value)
						}
					}
					sb.WriteString(fmt.Sprintf("  Allowed values: %s\n", strings.Join(values, ", ")))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Add the term to enrich
	sb.WriteString("## Term to Enrich\n\n")
	sb.WriteString(fmt.Sprintf("**Term:** %s\n", term.Term))
	sb.WriteString(fmt.Sprintf("**Definition:** %s\n\n", term.Definition))

	// Add examples for complex metrics
	sb.WriteString("## SQL Pattern Examples\n\n")
	sb.WriteString("For complex metrics like rates, percentages, and multi-table calculations:\n\n")
	sb.WriteString("**Utilization/Conversion Rate (percentage of items in a state):**\n")
	sb.WriteString("```sql\n")
	sb.WriteString("SELECT\n")
	sb.WriteString("    COUNT(*) FILTER (WHERE status = 'used') * 100.0 / NULLIF(COUNT(*), 0) AS utilization_rate\n")
	sb.WriteString("FROM offers\n")
	sb.WriteString("WHERE created_at >= NOW() - INTERVAL '30 days'\n")
	sb.WriteString("```\n\n")
	sb.WriteString("**Participation Rate (distinct participants / total eligible):**\n")
	sb.WriteString("```sql\n")
	sb.WriteString("SELECT\n")
	sb.WriteString("    COUNT(DISTINCT participant_id) * 100.0 / NULLIF((SELECT COUNT(*) FROM users WHERE eligible = true), 0) AS participation_rate\n")
	sb.WriteString("FROM program_participants\n")
	sb.WriteString("```\n\n")
	sb.WriteString("**Multi-table Join for Related Metrics:**\n")
	sb.WriteString("```sql\n")
	sb.WriteString("SELECT\n")
	sb.WriteString("    u.id AS user_id,\n")
	sb.WriteString("    COUNT(t.id) AS transaction_count,\n")
	sb.WriteString("    COALESCE(SUM(t.amount), 0) AS total_amount\n")
	sb.WriteString("FROM users u\n")
	sb.WriteString("LEFT JOIN transactions t ON u.id = t.user_id\n")
	sb.WriteString("GROUP BY u.id\n")
	sb.WriteString("```\n\n")

	// Response format
	sb.WriteString("## Response Format\n\n")
	sb.WriteString("Respond with a JSON object containing the enrichment:\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "defining_sql": "SELECT COUNT(*) FILTER (WHERE status = 'used') * 100.0 / NULLIF(COUNT(*), 0) AS utilization_rate\nFROM offers\nWHERE created_at >= NOW() - INTERVAL '30 days'",
  "base_table": "offers",
  "aliases": ["Usage Rate", "Redemption Rate"]
}
`)
	sb.WriteString("```\n\n")
	sb.WriteString("IMPORTANT: Use ONLY the exact table and column names from the schema above. The SQL must execute successfully.\n")

	return sb.String()
}

// detectColumnConfusions analyzes schema columns to detect common naming patterns
// that LLMs often confuse, and returns warning messages to include in the prompt.
// For example, if a table has 'created_at' but not 'started_at', we warn the LLM
// to avoid the common mistake of using 'started_at'.
func (s *glossaryService) detectColumnConfusions(schemaColumnsByTable map[string][]*models.SchemaColumn) []string {
	var warnings []string

	// Common column name confusions (expected -> often hallucinated)
	confusionPatterns := map[string][]string{
		"created_at":  {"started_at", "start_time", "begin_at"},
		"updated_at":  {"modified_at", "changed_at"},
		"ended_at":    {"finished_at", "end_time", "completed_at"},
		"deleted_at":  {"removed_at"},
		"amount":      {"value", "total"},
		"status":      {"state"},
		"user_id":     {"customer_id", "account_id"},
		"id":          {"uuid", "pk"},
		"name":        {"title", "label"},
		"description": {"desc", "details"},
	}

	// Build a set of all column names across all tables
	allColumns := make(map[string]map[string]bool) // table -> column -> exists
	for tableName, columns := range schemaColumnsByTable {
		allColumns[tableName] = make(map[string]bool)
		for _, col := range columns {
			allColumns[tableName][strings.ToLower(col.ColumnName)] = true
		}
	}

	// Check each table for confusion patterns
	for tableName, columns := range allColumns {
		for actual, hallucinated := range confusionPatterns {
			if columns[actual] {
				// This table has the actual column, warn about hallucinated alternatives
				for _, wrong := range hallucinated {
					if !columns[wrong] {
						warnings = append(warnings,
							fmt.Sprintf("Table '%s' has NO column named '%s'. Use '%s' instead.",
								tableName, wrong, actual))
					}
				}
			}
		}

		// Special case: detect timestamp columns and warn about common confusions
		hasCreatedAt := columns["created_at"]
		hasStartedAt := columns["started_at"]
		if hasCreatedAt && !hasStartedAt {
			// Only add if not already in warnings
			warning := fmt.Sprintf("Table '%s': There is NO 'started_at' column. For start time, use 'created_at'.", tableName)
			if !containsWarning(warnings, warning) {
				warnings = append(warnings, warning)
			}
		}
	}

	return warnings
}

// containsWarning checks if a warning message already exists in the slice.
func containsWarning(warnings []string, warning string) bool {
	for _, w := range warnings {
		if w == warning {
			return true
		}
	}
	return false
}

// generateTypeComparisonGuidance creates guidance for avoiding type mismatch errors.
// Numeric types (bigint, integer, etc.) should be compared with numbers, not strings.
// Text types should be compared with quoted strings.
func generateTypeComparisonGuidance(schemaColumnsByTable map[string][]*models.SchemaColumn) string {
	var sb strings.Builder

	// Collect examples of numeric and text columns for type guidance
	var numericExamples []string
	var textExamples []string

	numericTypes := map[string]bool{
		"bigint": true, "int8": true,
		"integer": true, "int": true, "int4": true,
		"smallint": true, "int2": true,
		"numeric": true, "decimal": true,
		"real": true, "float4": true,
		"double precision": true, "float8": true,
	}

	textTypes := map[string]bool{
		"text": true, "varchar": true, "character varying": true,
		"char": true, "character": true, "bpchar": true,
	}

	// Collect up to 3 examples of each type
	for tableName, columns := range schemaColumnsByTable {
		for _, col := range columns {
			normalizedType := strings.ToLower(col.DataType)
			// Strip size suffixes like varchar(255)
			if idx := strings.Index(normalizedType, "("); idx > 0 {
				normalizedType = normalizedType[:idx]
			}

			if numericTypes[normalizedType] && len(numericExamples) < 3 {
				// Prefer columns with _id suffix as they're commonly misused
				if strings.HasSuffix(strings.ToLower(col.ColumnName), "_id") || len(numericExamples) < 2 {
					numericExamples = append(numericExamples, fmt.Sprintf("`%s.%s` (%s)", tableName, col.ColumnName, col.DataType))
				}
			}
			if textTypes[normalizedType] && len(textExamples) < 3 {
				textExamples = append(textExamples, fmt.Sprintf("`%s.%s` (%s)", tableName, col.ColumnName, col.DataType))
			}
		}
	}

	// Only add guidance if we found relevant columns
	if len(numericExamples) == 0 && len(textExamples) == 0 {
		return ""
	}

	sb.WriteString("## TYPE COMPARISON RULES\n\n")
	sb.WriteString("CRITICAL: PostgreSQL is strictly typed. Type mismatches will cause query failures.\n\n")

	if len(numericExamples) > 0 {
		sb.WriteString("**Numeric columns (compare with integers, NOT quoted strings):**\n")
		for _, ex := range numericExamples {
			sb.WriteString(fmt.Sprintf("- %s\n", ex))
		}
		sb.WriteString("\n")
		sb.WriteString("```\n")
		sb.WriteString("WRONG: WHERE offer_id = 'abc'    -- String comparison on numeric column\n")
		sb.WriteString("WRONG: WHERE offer_id = '123'   -- Still wrong - quoted number is a string\n")
		sb.WriteString("RIGHT: WHERE offer_id = 123     -- Unquoted integer for numeric column\n")
		sb.WriteString("```\n\n")
	}

	if len(textExamples) > 0 {
		sb.WriteString("**Text columns (compare with quoted strings):**\n")
		for _, ex := range textExamples {
			sb.WriteString(fmt.Sprintf("- %s\n", ex))
		}
		sb.WriteString("\n")
		sb.WriteString("```\n")
		sb.WriteString("WRONG: WHERE status = active    -- Unquoted will be treated as column name\n")
		sb.WriteString("RIGHT: WHERE status = 'active'  -- Quoted string for text column\n")
		sb.WriteString("```\n\n")
	}

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

// getDomainHints analyzes tables and column details to detect domain patterns
// and returns hints to guide the LLM toward domain-specific term suggestions.
func getDomainHints(tables []*models.SchemaTable, ontology *models.TieredOntology) []string {
	var hints []string

	// Detect patterns from table names
	hasEngagement := containsTableByName(tables, "engagement", "session", "meeting", "call", "booking")
	hasSubscription := containsTableByName(tables, "subscription", "plan", "membership", "tier")
	hasBilling := containsTableByName(tables, "billing", "transaction", "payment", "invoice", "charge")
	hasInventory := containsTableByName(tables, "inventory", "product", "stock", "warehouse", "sku")
	hasEcommerce := containsTableByName(tables, "order", "cart", "checkout", "purchase")

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

// containsTableByName checks if any table name contains one of the specified keywords.
// The comparison is case-insensitive and matches substrings.
func containsTableByName(tables []*models.SchemaTable, keywords ...string) bool {
	for _, table := range tables {
		tableLower := strings.ToLower(table.TableName)
		for _, keyword := range keywords {
			keywordLower := strings.ToLower(keyword)
			if strings.Contains(tableLower, keywordLower) {
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
// business model based on detected table types. This provides a safety net after
// LLM generation to catch generic SaaS terms that slipped through prompt guidance.
func filterInapplicableTerms(
	terms []*models.BusinessGlossaryTerm,
	tables []*models.SchemaTable,
) []*models.BusinessGlossaryTerm {
	// Terms that require subscription-based business model tables
	subscriptionTerms := []string{
		"subscriber", "subscription", "churn", "mrr", "arr",
		"monthly recurring", "annual recurring", "recurring revenue",
	}

	// Terms that require inventory management tables
	inventoryTerms := []string{
		"inventory", "stock", "warehouse", "turnover",
		"stock level", "reorder", "stockout",
	}

	// Terms that require e-commerce/order tables
	ecommerceTerms := []string{
		"order value", "cart", "checkout", "aov", "gmv",
		"average order", "gross merchandise",
	}

	// Detect what table types exist in the schema
	hasSubscription := containsTableByName(tables, "subscription", "plan", "membership", "tier")
	hasInventory := containsTableByName(tables, "inventory", "product", "stock", "warehouse", "sku")
	hasEcommerce := containsTableByName(tables, "order", "cart", "checkout", "purchase")

	var filtered []*models.BusinessGlossaryTerm
	for _, term := range terms {
		termLower := strings.ToLower(term.Term)

		// Skip subscription terms if no subscription tables
		if !hasSubscription && matchesAny(termLower, subscriptionTerms) {
			continue
		}
		// Skip inventory terms if no inventory tables
		if !hasInventory && matchesAny(termLower, inventoryTerms) {
			continue
		}
		// Skip e-commerce terms if no e-commerce tables
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

// EnumMismatch represents a detected enum value issue in generated SQL.
// It captures when the SQL uses a string literal that appears to be a simplified
// or incorrect version of an actual enum value in the ontology.
type EnumMismatch struct {
	SQLValue      string   // The value found in the SQL
	Column        string   // The column this likely refers to
	Table         string   // The table containing the column
	ActualValues  []string // The actual enum values for this column
	BestMatch     string   // The best matching actual value (if found)
	MatchDistance int      // How different the values are (lower is better match)
}

// enumInfo holds metadata about where an enum value comes from.
type enumInfo struct {
	table    string
	column   string
	original string
}

// validateEnumValues checks generated SQL for potential enum value mismatches.
// It extracts string literals from the SQL and compares them against known enum
// columns in the ontology. Returns a list of potential issues.
//
// This is a best-effort heuristic check - it may produce false positives for
// string literals that aren't meant to be enum values. The caller should use
// these results as warnings/logs rather than hard failures.
func validateEnumValues(sql string, ontology *models.TieredOntology) []EnumMismatch {
	if ontology == nil || len(ontology.ColumnDetails) == 0 {
		return nil
	}

	// Extract string literals from SQL
	literals := extractStringLiterals(sql)
	if len(literals) == 0 {
		return nil
	}

	// Build a map of all known enum values across the ontology
	// key: lowercase enum value, value: struct with table, column, original value
	knownEnums := make(map[string]enumInfo)
	enumColumns := make(map[string][]string) // key: "table.column", values: all enum values

	for tableName, columns := range ontology.ColumnDetails {
		for _, col := range columns {
			if len(col.EnumValues) > 0 {
				key := tableName + "." + col.Name
				for _, ev := range col.EnumValues {
					knownEnums[strings.ToLower(ev.Value)] = enumInfo{
						table:    tableName,
						column:   col.Name,
						original: ev.Value,
					}
					enumColumns[key] = append(enumColumns[key], ev.Value)
				}
			}
		}
	}

	if len(knownEnums) == 0 {
		return nil
	}

	var mismatches []EnumMismatch

	for _, literal := range literals {
		literalLower := strings.ToLower(literal)

		// If the literal exactly matches a known enum, it's fine
		if _, exists := knownEnums[literalLower]; exists {
			continue
		}

		// Check if this literal is a simplified/shortened version of an enum value
		bestMatch, matchInfo, distance := findBestEnumMatch(literalLower, knownEnums)
		if bestMatch != "" && distance <= 3 {
			// This literal is suspiciously close to an enum value but doesn't match
			// Skip if it's too short (might just be a coincidental word)
			if len(literal) < 3 {
				continue
			}

			mismatches = append(mismatches, EnumMismatch{
				SQLValue:      literal,
				Column:        matchInfo.column,
				Table:         matchInfo.table,
				ActualValues:  enumColumns[matchInfo.table+"."+matchInfo.column],
				BestMatch:     matchInfo.original,
				MatchDistance: distance,
			})
		}
	}

	return mismatches
}

// extractStringLiterals extracts all single-quoted string literals from SQL.
// It handles escaped quotes (”) within strings.
// Examples:
//   - SELECT * FROM t WHERE status = 'active' → ["active"]
//   - WHERE name = 'O”Brien' → ["O'Brien"]
func extractStringLiterals(sql string) []string {
	var literals []string
	var current strings.Builder
	inString := false

	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' {
			if inString {
				// Check for escaped quote ('')
				if i+1 < len(sql) && sql[i+1] == '\'' {
					current.WriteByte('\'')
					i++ // Skip the next quote
				} else {
					// End of string literal
					if current.Len() > 0 {
						literals = append(literals, current.String())
					}
					current.Reset()
					inString = false
				}
			} else {
				// Start of string literal
				inString = true
			}
		} else if inString {
			current.WriteByte(sql[i])
		}
	}

	return literals
}

// findBestEnumMatch finds the best matching enum value for a given literal.
// Uses Levenshtein-inspired suffix matching to detect when a literal like 'ended'
// might be a simplified version of 'TRANSACTION_STATE_ENDED'.
//
// Returns the best match, its info, and a distance score (0 = perfect suffix match).
func findBestEnumMatch(literalLower string, knownEnums map[string]enumInfo) (string, enumInfo, int) {
	var bestMatch string
	var bestInfo enumInfo
	bestDistance := 999

	for enumLower, info := range knownEnums {
		// Check if the literal is a suffix of the enum (common pattern)
		// e.g., "ended" matches "TRANSACTION_STATE_ENDED"
		if strings.HasSuffix(enumLower, "_"+literalLower) || strings.HasSuffix(enumLower, literalLower) {
			// Suffix match - very likely a simplified version
			distance := 0
			if distance < bestDistance {
				bestDistance = distance
				bestMatch = enumLower
				bestInfo = info
			}
			continue
		}

		// Check if literal is a prefix stripped of underscores
		// e.g., "waiting" vs "TRANSACTION_STATE_WAITING"
		parts := strings.Split(enumLower, "_")
		for _, part := range parts {
			if part == literalLower {
				distance := 1 // Partial match via part
				if distance < bestDistance {
					bestDistance = distance
					bestMatch = enumLower
					bestInfo = info
				}
				break
			}
		}

		// Check Levenshtein distance for very similar values
		// Only check if the literal and enum have similar lengths
		if absInt(len(literalLower)-len(enumLower)) <= 5 {
			distance := levenshteinDistance(literalLower, enumLower)
			if distance <= 3 && distance < bestDistance {
				bestDistance = distance
				bestMatch = enumLower
				bestInfo = info
			}
		}
	}

	return bestMatch, bestInfo, bestDistance
}

// absInt returns the absolute value of an integer.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// levenshteinDistance calculates the edit distance between two strings.
// This is used to detect near-misses in enum values.
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Use a single row of the DP table for space efficiency
	prev := make([]int, len(s2)+1)
	curr := make([]int, len(s2)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(s1); i++ {
		curr[0] = i
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			curr[j] = minInt(
				curr[j-1]+1,    // insertion
				prev[j]+1,      // deletion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[len(s2)]
}

// minInt returns the minimum of three integers.
func minInt(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// ColumnValidationError represents a column reference that doesn't exist in the schema.
type ColumnValidationError struct {
	Column      string // The column name referenced in SQL
	Table       string // The table name if qualified (e.g., t.column), or empty
	Alias       string // The alias if used (e.g., t in t.column)
	SuggestFrom string // Suggested table containing a similar column, if any
}

// validateColumnReferences extracts column references from SQL and validates they exist
// in the provided schema columns. Returns a list of column references that don't exist.
//
// This is a best-effort heuristic check that:
// 1. Extracts potential column references from SQL (identifiers after SELECT, in WHERE, etc.)
// 2. Checks each reference against the schema columns
// 3. Returns non-existent columns so the LLM can be retried with better context
//
// The function handles:
//   - Qualified references: table.column or alias.column
//   - Unqualified references: just column_name
//   - Common SQL functions are excluded (COUNT, SUM, AVG, etc.)
func validateColumnReferences(sql string, schemaColumnsByTable map[string][]*models.SchemaColumn) []ColumnValidationError {
	if len(schemaColumnsByTable) == 0 {
		return nil // No schema to validate against
	}

	// Build a set of all valid column names (lowercased for case-insensitive matching)
	validColumns := make(map[string]bool)            // column_name -> exists
	tableColumns := make(map[string]map[string]bool) // table -> column -> exists
	for tableName, columns := range schemaColumnsByTable {
		tableColumns[strings.ToLower(tableName)] = make(map[string]bool)
		for _, col := range columns {
			colLower := strings.ToLower(col.ColumnName)
			validColumns[colLower] = true
			tableColumns[strings.ToLower(tableName)][colLower] = true
		}
	}

	// Extract column references from SQL
	refs := extractColumnReferences(sql)
	if len(refs) == 0 {
		return nil
	}

	// Build table alias map from the SQL (e.g., FROM users u -> u maps to users)
	aliases := extractTableAliases(sql, schemaColumnsByTable)

	var errors []ColumnValidationError
	seenErrors := make(map[string]bool) // Dedupe errors by column name

	for _, ref := range refs {
		colLower := strings.ToLower(ref.column)

		// Skip if already reported this column
		errorKey := ref.qualifier + "." + colLower
		if seenErrors[errorKey] {
			continue
		}

		if ref.qualifier != "" {
			// Qualified reference (e.g., t.column or table.column)
			qualLower := strings.ToLower(ref.qualifier)

			// Try to resolve alias to table name
			tableName := qualLower
			if resolved, ok := aliases[qualLower]; ok {
				tableName = resolved
			}

			// Check if the table exists and has the column
			if tableCols, tableExists := tableColumns[tableName]; tableExists {
				if !tableCols[colLower] {
					// Column doesn't exist in this table
					seenErrors[errorKey] = true
					errors = append(errors, ColumnValidationError{
						Column:      ref.column,
						Table:       tableName,
						Alias:       ref.qualifier,
						SuggestFrom: findTableWithColumn(colLower, tableColumns),
					})
				}
			} else if aliases[qualLower] == "" {
				// Qualifier doesn't match any known table - could be a subquery alias
				// Only report if the column doesn't exist anywhere
				if !validColumns[colLower] {
					seenErrors[errorKey] = true
					errors = append(errors, ColumnValidationError{
						Column:      ref.column,
						Alias:       ref.qualifier,
						SuggestFrom: findTableWithColumn(colLower, tableColumns),
					})
				}
			}
		} else {
			// Unqualified reference - check if column exists in any table
			if !validColumns[colLower] {
				seenErrors[errorKey] = true
				errors = append(errors, ColumnValidationError{
					Column:      ref.column,
					SuggestFrom: findSimilarColumn(colLower, validColumns),
				})
			}
		}
	}

	return errors
}

// columnReference represents a parsed column reference from SQL.
type columnReference struct {
	qualifier string // Table name or alias (empty if unqualified)
	column    string // Column name
}

// extractColumnReferences extracts potential column references from SQL.
// This is a heuristic parser that looks for identifier patterns commonly used
// for column references.
func extractColumnReferences(sql string) []columnReference {
	var refs []columnReference

	// SQL keywords and functions to skip (not column names)
	skipWords := map[string]bool{
		// SQL keywords
		"select": true, "from": true, "where": true, "and": true, "or": true,
		"on": true, "join": true, "left": true, "right": true, "inner": true,
		"outer": true, "full": true, "cross": true, "group": true, "by": true,
		"order": true, "having": true, "limit": true, "offset": true, "as": true,
		"case": true, "when": true, "then": true, "else": true, "end": true,
		"null": true, "true": true, "false": true, "not": true, "in": true,
		"is": true, "like": true, "between": true, "exists": true, "distinct": true,
		"all": true, "any": true, "union": true, "except": true, "intersect": true,
		"insert": true, "update": true, "delete": true, "into": true, "values": true,
		"set": true, "create": true, "alter": true, "drop": true, "table": true,
		"index": true, "view": true, "asc": true, "desc": true, "nulls": true,
		"first": true, "last": true, "over": true, "partition": true, "rows": true,
		"range": true, "preceding": true, "following": true, "current": true, "row": true,
		"unbounded": true, "with": true, "recursive": true, "filter": true,
		// SQL aggregate/scalar functions
		"count": true, "sum": true, "avg": true, "min": true, "max": true,
		"coalesce": true, "nullif": true, "cast": true, "extract": true,
		"date_trunc": true, "now": true, "current_date": true, "current_timestamp": true,
		"lower": true, "upper": true, "trim": true, "substring": true, "concat": true,
		"length": true, "abs": true, "round": true, "floor": true, "ceil": true,
		"array_agg": true, "string_agg": true, "jsonb_agg": true, "json_agg": true,
		"row_number": true, "rank": true, "dense_rank": true, "ntile": true,
		"lag": true, "lead": true, "first_value": true, "last_value": true,
		// PostgreSQL date/time functions and their field arguments
		"date_part": true, "age": true, "date_diff": true, "datediff": true,
		"epoch": true, "second": true, "minute": true, "hour": true,
		"day": true, "week": true, "month": true, "year": true, "quarter": true,
		"dow": true, "doy": true, "isodow": true, "isoyear": true,
		"timezone": true, "timezone_hour": true, "timezone_minute": true,
		"millisecond": true, "microsecond": true, "century": true, "decade": true,
		"millennium": true, "julian": true,
		// Common type names used in CAST
		"int": true, "integer": true, "bigint": true, "smallint": true,
		"text": true, "varchar": true, "char": true, "boolean": true, "bool": true,
		"float": true, "double": true, "numeric": true, "decimal": true,
		"date": true, "time": true, "timestamp": true, "interval": true,
		"uuid": true, "json": true, "jsonb": true, "array": true,
	}

	// Regular expression to find identifiers (qualified or unqualified)
	// Matches: word.word or just word (but not inside strings)
	// We'll use a simple state machine approach instead of complex regex
	tokens := tokenizeSQL(sql)

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		// Skip if it's a known keyword or function
		if skipWords[strings.ToLower(token)] {
			continue
		}

		// Skip if it looks like a number
		if isNumeric(token) {
			continue
		}

		// Skip if it's a quoted string (starts with ')
		if strings.HasPrefix(token, "'") {
			continue
		}

		// Skip if it's an operator, punctuation, or wildcard
		if token == "*" || token == "+" || token == "-" || token == "/" ||
			token == "=" || token == "<" || token == ">" || token == "!" ||
			token == "(" || token == ")" || token == "," || token == "." {
			continue
		}

		// Check if this is a qualified reference (next token is . followed by identifier)
		if i+2 < len(tokens) && tokens[i+1] == "." {
			nextToken := tokens[i+2]
			if !skipWords[strings.ToLower(nextToken)] && !isNumeric(nextToken) {
				refs = append(refs, columnReference{
					qualifier: token,
					column:    nextToken,
				})
				i += 2 // Skip the . and column name
				continue
			}
		}

		// Check if previous token was a comma, SELECT, WHERE, AND, OR, ON, =, <>, etc.
		// These contexts usually indicate a column reference
		if i > 0 {
			prev := strings.ToLower(tokens[i-1])
			if prev == "," || prev == "select" || prev == "where" || prev == "and" ||
				prev == "or" || prev == "on" || prev == "=" || prev == "<>" ||
				prev == "!=" || prev == "<" || prev == ">" || prev == "<=" ||
				prev == ">=" || prev == "by" || prev == "having" || prev == "then" ||
				prev == "when" || prev == "else" || prev == "(" {
				refs = append(refs, columnReference{column: token})
			}
		}
	}

	return refs
}

// tokenizeSQL breaks SQL into tokens (identifiers, operators, punctuation).
// This is a simple tokenizer that handles the common cases for column validation.
func tokenizeSQL(sql string) []string {
	var tokens []string
	var current strings.Builder
	inString := false
	inIdentifier := false

	for i := 0; i < len(sql); i++ {
		c := sql[i]

		if inString {
			if c == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++ // Skip escaped quote
				} else {
					inString = false
					if current.Len() > 0 {
						tokens = append(tokens, "'"+current.String()+"'")
						current.Reset()
					}
				}
			} else {
				current.WriteByte(c)
			}
			continue
		}

		if c == '\'' {
			// Start of string literal
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			inString = true
			continue
		}

		if c == '"' {
			// Double-quoted identifier - read until closing quote
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			i++
			for i < len(sql) && sql[i] != '"' {
				current.WriteByte(sql[i])
				i++
			}
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		// Check for identifier characters (letters, digits, underscore)
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' {
			current.WriteByte(c)
			inIdentifier = true
			continue
		}

		// End of identifier
		if inIdentifier && current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
			inIdentifier = false
		}

		// Handle operators and punctuation as separate tokens
		switch c {
		case '.', ',', '(', ')', '+', '-', '*', '/', '=', '<', '>', '!':
			tokens = append(tokens, string(c))
		}
		// Whitespace and other characters are ignored
	}

	// Don't forget the last token
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// isNumeric checks if a string represents a number.
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if c == '.' && i > 0 {
			continue // Allow decimal point
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// extractTableAliases extracts table aliases from the FROM clause.
// Returns a map of alias -> table_name.
func extractTableAliases(sql string, schemaColumnsByTable map[string][]*models.SchemaColumn) map[string]string {
	aliases := make(map[string]string)

	// Build a set of known table names
	knownTables := make(map[string]bool)
	for tableName := range schemaColumnsByTable {
		knownTables[strings.ToLower(tableName)] = true
	}

	tokens := tokenizeSQL(sql)
	inFrom := false
	inJoin := false

	for i := 0; i < len(tokens); i++ {
		tokenLower := strings.ToLower(tokens[i])

		// Track when we're in FROM or JOIN context
		if tokenLower == "from" {
			inFrom = true
			continue
		}
		if tokenLower == "join" {
			inJoin = true
			continue
		}
		if tokenLower == "where" || tokenLower == "group" || tokenLower == "order" ||
			tokenLower == "having" || tokenLower == "limit" || tokenLower == "union" {
			inFrom = false
			inJoin = false
			continue
		}

		// Look for pattern: table_name alias (where alias is not a keyword)
		if (inFrom || inJoin) && knownTables[tokenLower] {
			// Check if next token is an alias (not AS, not a keyword, not punctuation)
			if i+1 < len(tokens) {
				next := tokens[i+1]
				nextLower := strings.ToLower(next)

				// Skip "AS" if present
				if nextLower == "as" && i+2 < len(tokens) {
					next = tokens[i+2]
					nextLower = strings.ToLower(next)
					i++
				}

				// If next token is an identifier (not keyword, not punctuation)
				if !isKeywordOrPunctuation(nextLower) && len(next) > 0 {
					aliases[nextLower] = tokenLower
					i++ // Skip the alias
				}
			}
			inFrom = false // Move on after finding table
			inJoin = false
		}
	}

	return aliases
}

// isKeywordOrPunctuation checks if a token is a SQL keyword or punctuation.
func isKeywordOrPunctuation(token string) bool {
	keywords := map[string]bool{
		"select": true, "from": true, "where": true, "and": true, "or": true,
		"on": true, "join": true, "left": true, "right": true, "inner": true,
		"outer": true, "group": true, "by": true, "order": true, "having": true,
		"limit": true, "as": true, "union": true, "cross": true, ",": true,
		"(": true, ")": true, ".": true,
	}
	return keywords[token]
}

// findTableWithColumn finds a table that contains the given column.
func findTableWithColumn(column string, tableColumns map[string]map[string]bool) string {
	for table, cols := range tableColumns {
		if cols[column] {
			return table
		}
	}
	return ""
}

// findSimilarColumn finds a similar column name that might be what the user meant.
// Uses simple heuristics to detect common typos/variations.
func findSimilarColumn(column string, validColumns map[string]bool) string {
	// Common substitutions LLMs make
	substitutions := map[string]string{
		"started_at":  "created_at",
		"start_time":  "created_at",
		"begin_at":    "created_at",
		"ended_at":    "updated_at", // or completed_at if exists
		"finished_at": "completed_at",
		"modified_at": "updated_at",
		"changed_at":  "updated_at",
	}

	// Check if there's a known substitution
	if suggestion, ok := substitutions[column]; ok {
		if validColumns[suggestion] {
			return suggestion
		}
	}

	// Try removing common prefixes/suffixes
	for validCol := range validColumns {
		// Check if valid column ends with same suffix (e.g., "_at", "_id", "_count")
		if strings.HasSuffix(column, "_at") && strings.HasSuffix(validCol, "_at") {
			// Both are timestamp-like columns - might be a match
			if levenshteinDistance(column, validCol) <= 3 {
				return validCol
			}
		}
	}

	return ""
}

// formatColumnValidationError formats column validation errors into a human-readable message.
func formatColumnValidationError(errors []ColumnValidationError) string {
	if len(errors) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("SQL references non-existent columns: ")

	for i, err := range errors {
		if i > 0 {
			sb.WriteString(", ")
		}
		if err.Alias != "" {
			sb.WriteString(fmt.Sprintf("%s.%s", err.Alias, err.Column))
		} else {
			sb.WriteString(err.Column)
		}
		if err.SuggestFrom != "" {
			sb.WriteString(fmt.Sprintf(" (did you mean column in '%s'?)", err.SuggestFrom))
		}
	}

	return sb.String()
}

// SemanticWarning represents a potential issue in the formula semantics.
// These are advisory warnings, not hard errors - the SQL may still be valid
// but might not match the term's intended meaning.
type SemanticWarning struct {
	Code    string // Short identifier (e.g., "MISSING_COUNT", "UNION_MULTI_ROW")
	Message string // Human-readable description
}

// ValidateFormulaSemantics checks if the SQL formula matches the semantic
// meaning implied by the term name. This is a best-effort heuristic check
// that catches common mismatches between term names and their formulas.
//
// Returns a list of warnings (empty if no issues detected). These warnings
// are advisory - the SQL may still be syntactically valid and executable.
//
// Patterns checked:
//   - "Average X Per Y" should divide by COUNT (not by another SUM)
//   - UNION/UNION ALL typically returns multiple rows (unless wrapped in subquery)
func ValidateFormulaSemantics(termName string, sql string) []SemanticWarning {
	var warnings []SemanticWarning

	termLower := strings.ToLower(termName)
	sqlUpper := strings.ToUpper(sql)

	// Check "Average X Per Y" pattern
	// Terms with "average" and "per" typically mean: SUM(X) / COUNT(Y)
	// Not: SUM(X) / SUM(Y) (which would be a ratio/percentage)
	if strings.Contains(termLower, "average") && strings.Contains(termLower, "per") {
		// Check if SQL contains COUNT - it should for "per" calculations
		if !strings.Contains(sqlUpper, "COUNT(") && !strings.Contains(sqlUpper, "COUNT (") {
			warnings = append(warnings, SemanticWarning{
				Code:    "MISSING_COUNT",
				Message: "Term mentions 'average per' but SQL doesn't divide by COUNT. For 'Average X Per Y', the formula should typically be SUM(X) / COUNT(Y).",
			})
		}
	}

	// Check for UNION/UNION ALL which may return multiple rows
	// This is a warning because UNIONs are sometimes wrapped in subqueries
	// to produce a single row (e.g., SELECT AVG(*) FROM (... UNION ...))
	if strings.Contains(sqlUpper, "UNION") {
		// Check if the UNION is inside a subquery that aggregates the result
		// Simple heuristic: if there's an outer SELECT with AVG/SUM/COUNT over the union
		// This is imperfect but catches the obvious multi-row cases
		if !isUnionInAggregatingSubquery(sql) {
			warnings = append(warnings, SemanticWarning{
				Code:    "UNION_MULTI_ROW",
				Message: "SQL uses UNION which may return multiple rows. Consider wrapping in a subquery with aggregation if a single result is needed.",
			})
		}
	}

	return warnings
}

// isUnionInAggregatingSubquery checks if a UNION is wrapped in a subquery
// that aggregates the results into a single row.
//
// This is a simple heuristic check - it looks for patterns like:
//   - SELECT AVG(*) FROM (... UNION ...)
//   - SELECT SUM(*) FROM (SELECT ... UNION SELECT ...)
//
// Returns true if the UNION appears to be properly aggregated.
func isUnionInAggregatingSubquery(sql string) bool {
	sqlUpper := strings.ToUpper(sql)

	// Find the position of UNION
	unionPos := strings.Index(sqlUpper, "UNION")
	if unionPos == -1 {
		return false
	}

	// Count opening and closing parentheses before UNION
	// If UNION is inside parentheses, it's likely in a subquery
	openCount := strings.Count(sqlUpper[:unionPos], "(")
	closeCount := strings.Count(sqlUpper[:unionPos], ")")

	// If more opens than closes, UNION is inside parentheses (subquery)
	if openCount > closeCount {
		// Check if there's an aggregate function in the outer SELECT
		// Look at the SQL before the first opening paren that contains the UNION
		outerSQL := strings.TrimSpace(sqlUpper[:strings.Index(sqlUpper, "(")])
		aggregateFuncs := []string{"AVG(", "AVG ", "SUM(", "SUM ", "COUNT(", "COUNT ", "MAX(", "MAX ", "MIN(", "MIN "}
		for _, agg := range aggregateFuncs {
			// Check if aggregate appears after the outer SELECT
			selectPos := strings.Index(outerSQL, "SELECT")
			if selectPos != -1 {
				afterSelect := sqlUpper[selectPos:]
				if strings.Contains(afterSelect[:min(len(afterSelect), 100)], agg) {
					return true
				}
			}
		}
	}

	return false
}

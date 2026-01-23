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
	entityRepo     repositories.OntologyEntityRepository
	knowledgeRepo  repositories.KnowledgeRepository
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
	entityRepo repositories.OntologyEntityRepository,
	knowledgeRepo repositories.KnowledgeRepository,
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
		entityRepo:     entityRepo,
		knowledgeRepo:  knowledgeRepo,
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

	// Set ontology_id from active ontology if not already set
	// This enables proper unique constraint enforcement and CASCADE delete
	if term.OntologyID == nil {
		activeOntology, err := s.ontologyRepo.GetActive(ctx, projectID)
		if err != nil {
			s.logger.Warn("Failed to get active ontology for term creation",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without ontology_id - term will still be created
		} else if activeOntology != nil {
			term.OntologyID = &activeOntology.ID
		}
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
// On failure, it retries with enhanced context (more schema detail, examples) before giving up.
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

	// First attempt: normal enrichment
	result, firstErr := s.tryEnrichTerm(tenantCtx, term, ontology, entities, llmClient, projectID, false, "")
	if firstErr == nil {
		return result
	}

	// Retry with enhanced context (includes more schema detail, examples)
	s.logger.Debug("Retrying enrichment with enhanced context",
		zap.String("term", term.Term),
		zap.Error(firstErr))

	result, retryErr := s.tryEnrichTerm(tenantCtx, term, ontology, entities, llmClient, projectID, true, firstErr.Error())
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
	entities []*models.OntologyEntity,
	llmClient llm.LLMClient,
	projectID uuid.UUID,
	enhanced bool,
	previousError string,
) (enrichmentResult, error) {
	var prompt string
	if enhanced {
		prompt = s.buildEnhancedEnrichTermPrompt(term, ontology, entities, previousError)
	} else {
		prompt = s.buildEnrichTermPrompt(term, ontology, entities)
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
	if len(ontology.ColumnDetails) > 0 {
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
	entities []*models.OntologyEntity,
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

	// List entities with descriptions
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

	// Include ALL column details (not just measures/dimensions)
	if len(ontology.ColumnDetails) > 0 {
		sb.WriteString("## Complete Column Reference\n\n")
		sb.WriteString("All available columns by table (use these exact names in your SQL):\n\n")
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
	sb.WriteString("IMPORTANT: Use exact table and column names from the schema above. The SQL must execute successfully.\n")

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

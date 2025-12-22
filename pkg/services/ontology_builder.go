package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// MaxTablesPerBatch limits tables per LLM call for entity summary generation.
const MaxTablesPerBatch = 20

// OntologyBuilderService provides LLM-based ontology construction.
type OntologyBuilderService interface {
	// BuildTieredOntology orchestrates the complete tiered ontology construction.
	BuildTieredOntology(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID) error

	// BuildEntitySummaries generates Tier 1 entity summaries for tables.
	BuildEntitySummaries(ctx context.Context, projectID uuid.UUID, tables []*models.SchemaTable) (map[string]*models.EntitySummary, error)

	// BuildDomainSummary generates the Tier 0 domain summary from entity summaries.
	BuildDomainSummary(ctx context.Context, projectID uuid.UUID, entities map[string]*models.EntitySummary) (*models.DomainSummary, error)

	// AnalyzeEntity analyzes a single entity and may generate clarifying questions.
	// Returns questions generated for this entity (may be empty).
	// Questions with IsRequired=true indicate the entity cannot be completed without user input.
	AnalyzeEntity(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID, tableName string) ([]*models.OntologyQuestion, error)

	// GenerateQuestions generates clarifying questions based on schema analysis.
	// Deprecated: Use AnalyzeEntity for per-entity question generation.
	GenerateQuestions(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID) ([]*models.OntologyQuestion, error)

	// ProcessAnswer processes a user's answer and returns any ontology updates.
	ProcessAnswer(ctx context.Context, projectID uuid.UUID, question *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error)

	// ProcessProjectDescription analyzes user description + schema to produce
	// refined domain context, initial knowledge facts, and clarifying questions.
	// This runs as the first step of extraction (before Tier 1/0).
	ProcessProjectDescription(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID, description string) (*DescriptionProcessingResult, error)
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

// DescriptionProcessingResult contains the output of processing user's project description.
type DescriptionProcessingResult struct {
	DomainContext       *models.DomainContext         `json:"domain_context"`
	KnowledgeFacts      []*models.KnowledgeFact       `json:"knowledge_facts"`
	ClarifyingQuestions []*models.OntologyQuestion    `json:"clarifying_questions"`
	EntityHints         map[string]*models.EntityHint `json:"entity_hints"`
}

type ontologyBuilderService struct {
	ontologyRepo      repositories.OntologyRepository
	schemaRepo        repositories.SchemaRepository
	workflowRepo      repositories.OntologyWorkflowRepository
	knowledgeRepo     repositories.KnowledgeRepository
	workflowStateRepo repositories.WorkflowStateRepository
	llmFactory        llm.LLMClientFactory
	logger            *zap.Logger
}

// NewOntologyBuilderService creates a new ontology builder service.
func NewOntologyBuilderService(
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	workflowRepo repositories.OntologyWorkflowRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	workflowStateRepo repositories.WorkflowStateRepository,
	llmFactory llm.LLMClientFactory,
	logger *zap.Logger,
) OntologyBuilderService {
	return &ontologyBuilderService{
		ontologyRepo:      ontologyRepo,
		schemaRepo:        schemaRepo,
		workflowRepo:      workflowRepo,
		knowledgeRepo:     knowledgeRepo,
		workflowStateRepo: workflowStateRepo,
		llmFactory:        llmFactory,
		logger:            logger.Named("ontology-builder"),
	}
}

var _ OntologyBuilderService = (*ontologyBuilderService)(nil)

// ============================================================================
// BuildTieredOntology - Main Orchestration
// ============================================================================

func (s *ontologyBuilderService) BuildTieredOntology(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID) error {
	startTime := time.Now()

	// Add workflow context for conversation recording
	ctx = llm.WithWorkflowID(ctx, workflowID)

	s.logger.Info("Starting tiered ontology build",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflowID.String()))

	// Get the active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology found")
	}

	// Get workflow to find datasource and description
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("failed to get workflow: %w", err)
	}
	if workflow.Config == nil {
		return fmt.Errorf("workflow has no config")
	}

	// Load domain context from ontology metadata (set by InitializeOntologyTask)
	var domainContext *models.DomainContext
	if ontology.Metadata != nil {
		if dc, ok := ontology.Metadata["domain_context"]; ok {
			if dcMap, ok := dc.(map[string]any); ok {
				domainContext = &models.DomainContext{
					KeyTerminology: make(map[string]string),
				}
				if summary, ok := dcMap["summary"].(string); ok {
					domainContext.Summary = summary
				}
				if domains, ok := dcMap["primary_domains"].([]any); ok {
					for _, d := range domains {
						if ds, ok := d.(string); ok {
							domainContext.PrimaryDomains = append(domainContext.PrimaryDomains, ds)
						}
					}
				}
				if terms, ok := dcMap["key_terminology"].(map[string]any); ok {
					for k, v := range terms {
						if vs, ok := v.(string); ok {
							domainContext.KeyTerminology[k] = vs
						}
					}
				}
			}
		}
	}

	// Load schema tables
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, workflow.Config.DatasourceID)
	if err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}
	tableCount := len(tables)

	s.logger.Info("Loaded schema tables",
		zap.Int("table_count", tableCount))

	// Update workflow progress with table count
	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseTier1Building,
		Message:      fmt.Sprintf("Building entity summaries for %d tables...", tableCount),
		Current:      0,
		Total:        tableCount,
	}); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	// Build Tier 1: Entity summaries (with domain context)
	entitySummaries, err := s.buildEntitySummariesWithContext(ctx, projectID, tables, domainContext)
	if err != nil {
		return fmt.Errorf("failed to build entity summaries: %w", err)
	}

	// Save entity summaries
	if err := s.ontologyRepo.UpdateEntitySummaries(ctx, projectID, entitySummaries); err != nil {
		return fmt.Errorf("failed to save entity summaries: %w", err)
	}

	// Update progress - entity summaries complete
	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseTier0Building,
		Message:      "Building domain summary...",
		Current:      tableCount,
		Total:        tableCount,
	}); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	// Build Tier 0: Domain summary (with domain context)
	domainSummary, err := s.buildDomainSummaryWithContext(ctx, projectID, entitySummaries, domainContext)
	if err != nil {
		return fmt.Errorf("failed to build domain summary: %w", err)
	}

	// Save domain summary
	if err := s.ontologyRepo.UpdateDomainSummary(ctx, projectID, domainSummary); err != nil {
		return fmt.Errorf("failed to save domain summary: %w", err)
	}

	// Update progress - ontology is now ready
	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase:  models.WorkflowPhaseCompleting,
		Message:       "Ontology ready, generating questions...",
		Current:       tableCount,
		Total:         tableCount,
		OntologyReady: true,
	}); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	s.logger.Info("Tiered ontology build completed",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflowID.String()),
		zap.Int("entities", len(entitySummaries)),
		zap.Int("domains", len(domainSummary.Domains)),
		zap.Duration("elapsed", time.Since(startTime)))

	// Mark global entity as complete so orchestrator can finalize workflow
	globalState, err := s.workflowStateRepo.GetByEntity(ctx, workflowID, models.WorkflowEntityTypeGlobal, "")
	if err != nil {
		return fmt.Errorf("get global entity state: %w", err)
	}
	if globalState != nil {
		if err := s.workflowStateRepo.UpdateStatus(ctx, globalState.ID, models.WorkflowEntityStatusComplete, nil); err != nil {
			return fmt.Errorf("update global entity status: %w", err)
		}
	}

	return nil
}

// ============================================================================
// BuildEntitySummaries - Tier 1
// ============================================================================

func (s *ontologyBuilderService) BuildEntitySummaries(ctx context.Context, projectID uuid.UUID, tables []*models.SchemaTable) (map[string]*models.EntitySummary, error) {
	// Delegate to context-aware version with nil context
	return s.buildEntitySummariesWithContext(ctx, projectID, tables, nil)
}

func (s *ontologyBuilderService) buildEntitySummariesWithContext(ctx context.Context, projectID uuid.UUID, tables []*models.SchemaTable, domainContext *models.DomainContext) (map[string]*models.EntitySummary, error) {
	if len(tables) == 0 {
		return map[string]*models.EntitySummary{}, nil
	}

	startTime := time.Now()
	s.logger.Info("Building entity summaries",
		zap.String("project_id", projectID.String()),
		zap.Int("table_count", len(tables)),
		zap.Bool("has_domain_context", domainContext != nil))

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Load relationships for context
	relationships, err := s.loadRelationships(ctx, projectID, tables)
	if err != nil {
		s.logger.Error("Failed to load relationships", zap.Error(err))
	}
	relationshipMap := s.buildRelationshipMap(relationships, tables)

	// Process in batches
	allSummaries := make(map[string]*models.EntitySummary)

	for i := 0; i < len(tables); i += MaxTablesPerBatch {
		end := i + MaxTablesPerBatch
		if end > len(tables) {
			end = len(tables)
		}

		batchTables := tables[i:end]
		batchNum := (i / MaxTablesPerBatch) + 1
		totalBatches := (len(tables) + MaxTablesPerBatch - 1) / MaxTablesPerBatch

		s.logger.Info("Processing entity batch",
			zap.Int("batch", batchNum),
			zap.Int("total_batches", totalBatches),
			zap.Int("tables_in_batch", len(batchTables)))

		batchSummaries, err := s.buildEntityBatchWithContext(ctx, llmClient, batchTables, relationshipMap, domainContext)
		if err != nil {
			return nil, fmt.Errorf("batch %d failed: %w", batchNum, err)
		}

		for tableName, summary := range batchSummaries {
			allSummaries[tableName] = summary
		}
	}

	s.logger.Info("Entity summaries built",
		zap.Int("entity_count", len(allSummaries)),
		zap.Duration("elapsed", time.Since(startTime)))

	return allSummaries, nil
}

func (s *ontologyBuilderService) buildEntityBatch(
	ctx context.Context,
	llmClient llm.LLMClient,
	tables []*models.SchemaTable,
	relationshipMap map[string][]string,
) (map[string]*models.EntitySummary, error) {
	return s.buildEntityBatchWithContext(ctx, llmClient, tables, relationshipMap, nil)
}

func (s *ontologyBuilderService) buildEntityBatchWithContext(
	ctx context.Context,
	llmClient llm.LLMClient,
	tables []*models.SchemaTable,
	relationshipMap map[string][]string,
	domainContext *models.DomainContext,
) (map[string]*models.EntitySummary, error) {
	prompt := s.buildTier1PromptWithContext(tables, relationshipMap, domainContext)
	systemMsg := s.tier1SystemMessage()

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	summaries, err := s.parseTier1Response(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return summaries, nil
}

func (s *ontologyBuilderService) tier1SystemMessage() string {
	return `You are a database ontology expert. Generate concise entity summaries for schema understanding.

For each table, provide:
1. table_name: The exact table name from input
2. business_name: Human-friendly name (e.g., "Customer Orders" not "orders")
3. description: ONE sentence explaining business purpose (max 15 words)
4. domain: One of: sales, finance, operations, customer, product, analytics, hr, inventory, marketing, unknown
5. synonyms: 3-5 alternative names users might search for
6. key_columns: Up to 3 most important columns with their synonyms
7. column_count: Total number of columns
8. relationships: Direct relationships (1-hop only)

CRITICAL: Keep each entity summary under 75 tokens. Be concise.

Return a JSON array of entity summaries.`
}

func (s *ontologyBuilderService) buildTier1Prompt(tables []*models.SchemaTable, relationshipMap map[string][]string) string {
	return s.buildTier1PromptWithContext(tables, relationshipMap, nil)
}

func (s *ontologyBuilderService) buildTier1PromptWithContext(tables []*models.SchemaTable, relationshipMap map[string][]string, domainContext *models.DomainContext) string {
	var prompt strings.Builder

	// Include refined domain context if available (NOT raw user description)
	if domainContext != nil {
		prompt.WriteString("## Domain Context\n\n")
		if domainContext.Summary != "" {
			prompt.WriteString(domainContext.Summary + "\n\n")
		}

		if len(domainContext.PrimaryDomains) > 0 {
			prompt.WriteString("Primary domains: " + strings.Join(domainContext.PrimaryDomains, ", ") + "\n\n")
		}

		if len(domainContext.KeyTerminology) > 0 {
			prompt.WriteString("### Key Terminology\n")
			for term, definition := range domainContext.KeyTerminology {
				prompt.WriteString(fmt.Sprintf("- **%s**: %s\n", term, definition))
			}
			prompt.WriteString("\n")
		}
	}

	prompt.WriteString("Generate entity summaries for the following database tables.\n\n")
	prompt.WriteString("## Tables\n\n")

	for _, table := range tables {
		prompt.WriteString(fmt.Sprintf("### %s\n", table.TableName))
		if table.RowCount != nil {
			prompt.WriteString(fmt.Sprintf("Row count: %d\n", *table.RowCount))
		}

		// Fetch columns for this table (we have them from SchemaTable)
		if len(table.Columns) > 0 {
			prompt.WriteString("Columns:\n")
			for _, col := range table.Columns {
				colInfo := fmt.Sprintf("- %s (%s)", col.ColumnName, col.DataType)
				if col.IsPrimaryKey {
					colInfo += " [PK]"
				}
				prompt.WriteString(colInfo + "\n")
			}
		}

		// Add existing business context if available
		if table.BusinessName != nil && *table.BusinessName != "" {
			prompt.WriteString(fmt.Sprintf("Current business name: %s\n", *table.BusinessName))
		}
		if table.Description != nil && *table.Description != "" {
			prompt.WriteString(fmt.Sprintf("Current description: %s\n", *table.Description))
		}

		// Add relationships
		if rels, ok := relationshipMap[table.TableName]; ok && len(rels) > 0 {
			prompt.WriteString(fmt.Sprintf("Related to: %s\n", strings.Join(rels, ", ")))
		}

		prompt.WriteString("\n")
	}

	prompt.WriteString("## Output Format\n\n")
	prompt.WriteString("Return a JSON array:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(`[
  {
    "table_name": "orders",
    "business_name": "Customer Orders",
    "description": "Records of customer purchase transactions",
    "domain": "sales",
    "synonyms": ["purchases", "transactions", "sales"],
    "key_columns": [
      {"name": "total_amount", "synonyms": ["revenue", "price"]},
      {"name": "order_date", "synonyms": ["purchase_date"]}
    ],
    "column_count": 12,
    "relationships": ["customers", "products"]
  }
]
`)
	prompt.WriteString("```\n")

	return prompt.String()
}

func (s *ontologyBuilderService) parseTier1Response(response string) (map[string]*models.EntitySummary, error) {
	type llmEntitySummary struct {
		TableName     string             `json:"table_name"`
		BusinessName  string             `json:"business_name"`
		Description   string             `json:"description"`
		Domain        string             `json:"domain"`
		Synonyms      []string           `json:"synonyms"`
		KeyColumns    []models.KeyColumn `json:"key_columns"`
		ColumnCount   int                `json:"column_count"`
		Relationships []string           `json:"relationships"`
	}

	summaries, err := llm.ParseJSONResponse[[]llmEntitySummary](response)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*models.EntitySummary)
	for _, entity := range summaries {
		result[entity.TableName] = &models.EntitySummary{
			TableName:     entity.TableName,
			BusinessName:  entity.BusinessName,
			Description:   entity.Description,
			Domain:        entity.Domain,
			Synonyms:      entity.Synonyms,
			KeyColumns:    entity.KeyColumns,
			ColumnCount:   entity.ColumnCount,
			Relationships: entity.Relationships,
		}
	}

	return result, nil
}

// ============================================================================
// BuildDomainSummary - Tier 0
// ============================================================================

func (s *ontologyBuilderService) BuildDomainSummary(ctx context.Context, projectID uuid.UUID, entities map[string]*models.EntitySummary) (*models.DomainSummary, error) {
	return s.buildDomainSummaryWithContext(ctx, projectID, entities, nil)
}

func (s *ontologyBuilderService) buildDomainSummaryWithContext(ctx context.Context, projectID uuid.UUID, entities map[string]*models.EntitySummary, domainContext *models.DomainContext) (*models.DomainSummary, error) {
	if len(entities) == 0 {
		return &models.DomainSummary{
			Description: "Empty database",
			Domains:     []string{},
		}, nil
	}

	startTime := time.Now()
	s.logger.Info("Building domain summary",
		zap.String("project_id", projectID.String()),
		zap.Int("entity_count", len(entities)),
		zap.Bool("has_domain_context", domainContext != nil))

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	prompt := s.buildTier0PromptWithContext(entities, domainContext)
	systemMsg := s.tier0SystemMessage()

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	summary, err := s.parseTier0Response(result.Content, entities)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	s.logger.Info("Domain summary built",
		zap.Int("domain_count", len(summary.Domains)),
		zap.Duration("elapsed", time.Since(startTime)))

	return summary, nil
}

func (s *ontologyBuilderService) tier0SystemMessage() string {
	return `You are a business intelligence expert. Generate a high-level domain summary for this database schema.

Provide:
1. description: 1-2 sentence business summary of the entire database
2. domains: List of business domains present (e.g., ["sales", "finance", "operations"])
3. sample_questions: 3-5 example business questions this database can answer

CRITICAL: Keep total output under 500 tokens. Be extremely concise.

Return a JSON object with these 3 fields.`
}

func (s *ontologyBuilderService) buildTier0Prompt(entities map[string]*models.EntitySummary) string {
	return s.buildTier0PromptWithContext(entities, nil)
}

func (s *ontologyBuilderService) buildTier0PromptWithContext(entities map[string]*models.EntitySummary, domainContext *models.DomainContext) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a domain summary for this database schema.\n\n")

	// Include pre-existing domain context if available
	if domainContext != nil && domainContext.Summary != "" {
		prompt.WriteString("## User-Provided Context\n\n")
		prompt.WriteString(domainContext.Summary + "\n\n")
		if len(domainContext.KeyTerminology) > 0 {
			prompt.WriteString("Key terms:\n")
			for term, def := range domainContext.KeyTerminology {
				prompt.WriteString(fmt.Sprintf("- %s: %s\n", term, def))
			}
			prompt.WriteString("\n")
		}
	}

	prompt.WriteString("## Entities by Domain\n\n")

	// Group entities by domain
	domainEntities := make(map[string][]string)
	for _, entity := range entities {
		domain := entity.Domain
		if domain == "" {
			domain = "unknown"
		}
		domainEntities[domain] = append(domainEntities[domain], entity.TableName)
	}

	for domain, tables := range domainEntities {
		prompt.WriteString(fmt.Sprintf("**%s**: %s\n", domain, strings.Join(tables, ", ")))
	}

	prompt.WriteString("\n## Entity Descriptions\n\n")
	for _, entity := range entities {
		prompt.WriteString(fmt.Sprintf("- **%s**: %s\n", entity.BusinessName, entity.Description))
	}

	prompt.WriteString("\n## Output Format\n\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(`{
  "description": "E-commerce database tracking orders, customers, and inventory",
  "domains": ["sales", "customer", "inventory"],
  "sample_questions": [
    "What was total revenue last quarter?",
    "Which products are low in stock?",
    "Who are our top customers?"
  ]
}
`)
	prompt.WriteString("```\n")

	return prompt.String()
}

func (s *ontologyBuilderService) parseTier0Response(response string, entities map[string]*models.EntitySummary) (*models.DomainSummary, error) {
	type llmDomainSummary struct {
		Description     string   `json:"description"`
		Domains         []string `json:"domains"`
		SampleQuestions []string `json:"sample_questions"`
	}

	llmSummary, err := llm.ParseJSONResponse[llmDomainSummary](response)
	if err != nil {
		return nil, err
	}

	// Build relationship graph from entity relationships
	var relationshipGraph []models.RelationshipEdge
	for tableName, entity := range entities {
		for _, relatedTable := range entity.Relationships {
			relationshipGraph = append(relationshipGraph, models.RelationshipEdge{
				From: tableName,
				To:   relatedTable,
			})
		}
	}

	return &models.DomainSummary{
		Description:       llmSummary.Description,
		Domains:           llmSummary.Domains,
		SampleQuestions:   llmSummary.SampleQuestions,
		RelationshipGraph: relationshipGraph,
	}, nil
}

// ============================================================================
// AnalyzeEntity
// ============================================================================

func (s *ontologyBuilderService) AnalyzeEntity(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID, tableName string) ([]*models.OntologyQuestion, error) {
	startTime := time.Now()

	// Add workflow context for conversation recording
	ctx = llm.WithWorkflowID(ctx, workflowID)

	s.logger.Info("Analyzing entity",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflowID.String()),
		zap.String("table_name", tableName))

	// Get workflow to find datasource
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	if workflow.Config == nil {
		return nil, fmt.Errorf("workflow has no config")
	}

	// Load the table schema (schema-agnostic lookup)
	table, err := s.schemaRepo.FindTableByName(ctx, projectID, workflow.Config.DatasourceID, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get table %s: %w", tableName, err)
	}

	// Load columns for the table (GetTableByName doesn't include columns)
	columnPtrs, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns for table %s: %w", tableName, err)
	}
	// Convert []*SchemaColumn to []SchemaColumn
	table.Columns = make([]models.SchemaColumn, len(columnPtrs))
	for i, col := range columnPtrs {
		table.Columns[i] = *col
	}

	// Load gathered data from workflow state for each column
	columnGatheredData := make(map[string]map[string]any)
	if s.workflowStateRepo != nil {
		for _, col := range table.Columns {
			colEntityKey := models.ColumnEntityKey(tableName, col.ColumnName)
			ws, err := s.workflowStateRepo.GetByEntity(ctx, workflowID, models.WorkflowEntityTypeColumn, colEntityKey)
			if err != nil {
				s.logger.Warn("Failed to load column workflow state",
					zap.String("column", col.ColumnName),
					zap.Error(err))
				continue
			}
			if ws != nil && ws.StateData != nil && ws.StateData.Gathered != nil {
				columnGatheredData[col.ColumnName] = ws.StateData.Gathered
			}
		}
	}

	// Load the existing ontology to get the entity summary and domain context
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ontology: %w", err)
	}

	var entitySummary *models.EntitySummary
	var domainContext *models.DomainContext
	var entityHint *models.EntityHint

	if ontology != nil {
		if ontology.EntitySummaries != nil {
			entitySummary = ontology.EntitySummaries[tableName]
		}

		// Extract domain context from metadata
		if ontology.Metadata != nil {
			if dc, ok := ontology.Metadata["domain_context"]; ok {
				if dcMap, ok := dc.(map[string]any); ok {
					domainContext = &models.DomainContext{}
					if s, ok := dcMap["summary"].(string); ok {
						domainContext.Summary = s
					}
					if pd, ok := dcMap["primary_domains"].([]any); ok {
						for _, d := range pd {
							if ds, ok := d.(string); ok {
								domainContext.PrimaryDomains = append(domainContext.PrimaryDomains, ds)
							}
						}
					}
					if kt, ok := dcMap["key_terminology"].(map[string]any); ok {
						domainContext.KeyTerminology = make(map[string]string)
						for k, v := range kt {
							if vs, ok := v.(string); ok {
								domainContext.KeyTerminology[k] = vs
							}
						}
					}
				}
			}

			// Extract entity hint from metadata
			if hints, ok := ontology.Metadata["entity_hints"]; ok {
				if hintsMap, ok := hints.(map[string]any); ok {
					if hint, ok := hintsMap[tableName]; ok {
						if hintMap, ok := hint.(map[string]any); ok {
							entityHint = &models.EntityHint{}
							if bn, ok := hintMap["business_name"].(string); ok {
								entityHint.BusinessName = bn
							}
							if d, ok := hintMap["domain"].(string); ok {
								entityHint.Domain = d
							}
						}
					}
				}
			}
		}
	}

	// Load relationships for this table
	relationships, err := s.schemaRepo.GetRelationshipDetails(ctx, projectID, workflow.Config.DatasourceID)
	if err != nil {
		s.logger.Warn("Failed to load relationships", zap.Error(err))
		relationships = nil
	}

	// Filter relationships to only those involving this table
	var tableRelationships []*models.RelationshipDetail
	for _, rel := range relationships {
		if rel.SourceTableName == tableName || rel.TargetTableName == tableName {
			tableRelationships = append(tableRelationships, rel)
		}
	}

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Build prompt and call LLM
	prompt := s.buildEntityAnalysisPrompt(table, entitySummary, domainContext, entityHint, tableRelationships, columnGatheredData)
	systemMsg := s.entityAnalysisSystemMessage()

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	questions, entitySummaryFromLLM, err := s.parseEntityAnalysisResponse(result.Content, projectID, workflow.OntologyID, workflowID, tableName)
	if err != nil {
		s.logger.Warn("Failed to parse entity analysis response, treating as no questions",
			zap.String("table_name", tableName),
			zap.Error(err))
		return []*models.OntologyQuestion{}, nil
	}

	// Write initial entity summary immediately if we got one from LLM
	if entitySummaryFromLLM != nil {
		entitySummaryFromLLM.ColumnCount = len(table.Columns)
		if err := s.ontologyRepo.UpdateEntitySummary(ctx, projectID, tableName, entitySummaryFromLLM); err != nil {
			s.logger.Warn("Failed to write initial entity summary",
				zap.String("table_name", tableName),
				zap.Error(err))
			// Non-fatal - continue with questions
		} else {
			s.logger.Debug("Wrote initial entity summary",
				zap.String("table_name", tableName),
				zap.String("business_name", entitySummaryFromLLM.BusinessName))
		}
	}

	s.logger.Info("Entity analysis complete",
		zap.String("table_name", tableName),
		zap.Int("question_count", len(questions)),
		zap.Bool("has_entity_summary", entitySummaryFromLLM != nil),
		zap.Duration("elapsed", time.Since(startTime)))

	return questions, nil
}

func (s *ontologyBuilderService) entityAnalysisSystemMessage() string {
	return `You are a database analyst helping to understand a business domain by examining a single table in detail.
Your goal is to identify any clarifying questions that would help fully understand what this table represents and how it's used.

CRITICAL RULES:
- Focus on THIS SPECIFIC TABLE only
- Reference EXACT column names from the schema
- Focus on BUSINESS understanding, not technical details
- Only ask questions if there is genuine ambiguity or missing context
- Many tables are self-explanatory - it's OK to return zero questions
- Status/type/state columns often encode business rules worth asking about`
}

func (s *ontologyBuilderService) buildEntityAnalysisPrompt(
	table *models.SchemaTable,
	entitySummary *models.EntitySummary,
	domainContext *models.DomainContext,
	entityHint *models.EntityHint,
	relationships []*models.RelationshipDetail,
	columnGatheredData map[string]map[string]any,
) string {
	var sb strings.Builder

	// Include domain context if available (from user's project description)
	if domainContext != nil && domainContext.Summary != "" {
		sb.WriteString("## DOMAIN CONTEXT\n\n")
		sb.WriteString(fmt.Sprintf("%s\n", domainContext.Summary))
		if len(domainContext.KeyTerminology) > 0 {
			sb.WriteString("\nKey Terminology:\n")
			for term, definition := range domainContext.KeyTerminology {
				sb.WriteString(fmt.Sprintf("  - **%s**: %s\n", term, definition))
			}
		}
		sb.WriteString("\n")
	}

	// Include entity hint if available (from user's project description)
	if entityHint != nil && (entityHint.BusinessName != "" || entityHint.Domain != "") {
		sb.WriteString("## USER-PROVIDED CONTEXT FOR THIS TABLE\n\n")
		if entityHint.BusinessName != "" {
			sb.WriteString(fmt.Sprintf("User calls this: %s\n", entityHint.BusinessName))
		}
		if entityHint.Domain != "" {
			sb.WriteString(fmt.Sprintf("Domain: %s\n", entityHint.Domain))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## TABLE SCHEMA\n\n")
	sb.WriteString(fmt.Sprintf("Table: %s\n", table.TableName))
	if table.RowCount != nil {
		sb.WriteString(fmt.Sprintf("Row count: %d\n", *table.RowCount))
	}
	sb.WriteString("\nColumns:\n")

	// Token Budget Management
	// ========================
	// LLM has a ~40k token limit. We need to fit:
	//   - Fixed prompt parts (task description, domain context, relationships): ~16k tokens
	//   - Column data with sample values: remaining ~24k tokens
	//
	// Token-to-char ratio: ~4 chars per token (conservative estimate)
	//   24k tokens × 4 chars/token = 96k chars available for column data
	//   Using 76k chars to leave safety margin for response tokens
	//
	// Budget allocation:
	//   - Base column info (name, type, flags): ~50 chars per column
	//   - Remaining budget divided among columns for sample values
	//   - Tables with many columns get less sample data per column
	//   - Tables with few columns get rich sample data
	const totalSampleBudget = 76000
	numCols := len(table.Columns)
	if numCols == 0 {
		numCols = 1 // avoid division by zero
	}
	// Subtract base info cost, divide remainder among columns
	perColumnBudget := (totalSampleBudget - numCols*50) / numCols
	if perColumnBudget < 0 {
		perColumnBudget = 0
	}
	// Sample limits based on per-column budget:
	//   - Enum candidates: more samples (short values, ~30 chars each including overhead)
	//   - Regular columns: fewer samples (longer values, ~50 chars each)
	//   - Individual sample max length: 1/3 of column budget, capped at 200 chars
	maxSamplesForEnum := min(20, perColumnBudget/30)
	maxSamplesRegular := min(10, perColumnBudget/50)
	maxSampleLen := min(200, perColumnBudget/3)
	if maxSampleLen < 20 {
		maxSampleLen = 20 // minimum useful length to show meaningful data
	}

	for _, col := range table.Columns {
		flags := []string{}
		if col.IsPrimaryKey {
			flags = append(flags, "PK")
		}
		if col.IsNullable {
			flags = append(flags, "nullable")
		}
		flagStr := ""
		if len(flags) > 0 {
			flagStr = " [" + strings.Join(flags, ", ") + "]"
		}
		sb.WriteString(fmt.Sprintf("  - %s: %s%s\n", col.ColumnName, col.DataType, flagStr))

		// Include gathered data if available (sample values, enum candidate info)
		if gathered, ok := columnGatheredData[col.ColumnName]; ok {
			isEnumCandidate := false
			if isEnum, ok := gathered["is_enum_candidate"].(bool); ok {
				isEnumCandidate = isEnum
			}

			// Use budget-based limits for sample values
			if sampleValues, ok := gathered["sample_values"].([]any); ok && len(sampleValues) > 0 && perColumnBudget > 0 {
				// Enum candidates get more samples (they're usually short values)
				maxSamples := maxSamplesRegular
				if isEnumCandidate {
					maxSamples = maxSamplesForEnum
				}
				if maxSamples < 1 {
					maxSamples = 1
				}

				samples := make([]string, 0, min(len(sampleValues), maxSamples))
				for i, v := range sampleValues {
					if i >= maxSamples {
						break
					}
					if str, ok := v.(string); ok {
						// Truncate long sample values
						if len(str) > maxSampleLen {
							str = str[:maxSampleLen] + "..."
						}
						samples = append(samples, fmt.Sprintf("%q", str))
					}
				}
				if len(samples) > 0 {
					sb.WriteString(fmt.Sprintf("      Sample values: [%s]\n", strings.Join(samples, ", ")))
				}
			}

			// Note if column is an enum candidate
			if isEnumCandidate {
				distinctCount := 0
				if dc, ok := gathered["distinct_count"].(int); ok {
					distinctCount = dc
				} else if dc, ok := gathered["distinct_count"].(float64); ok {
					distinctCount = int(dc)
				}
				if distinctCount > 0 {
					sb.WriteString(fmt.Sprintf("      (Enum candidate: %d distinct values)\n", distinctCount))
				}
			}
		}
	}

	// Include relationships
	if len(relationships) > 0 {
		sb.WriteString("\n## RELATIONSHIPS\n\n")
		for _, rel := range relationships {
			if rel.SourceTableName == table.TableName {
				sb.WriteString(fmt.Sprintf("  - %s.%s → %s.%s (%s)\n",
					rel.SourceTableName, rel.SourceColumnName,
					rel.TargetTableName, rel.TargetColumnName,
					rel.RelationshipType))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s.%s ← %s.%s (%s)\n",
					table.TableName, rel.TargetColumnName,
					rel.SourceTableName, rel.SourceColumnName,
					rel.RelationshipType))
			}
		}
	}

	if entitySummary != nil {
		sb.WriteString("\n## CURRENT UNDERSTANDING\n\n")
		sb.WriteString(fmt.Sprintf("Business Name: %s\n", entitySummary.BusinessName))
		sb.WriteString(fmt.Sprintf("Description: %s\n", entitySummary.Description))
		sb.WriteString(fmt.Sprintf("Domain: %s\n", entitySummary.Domain))
		if len(entitySummary.Synonyms) > 0 {
			sb.WriteString(fmt.Sprintf("Synonyms: %s\n", strings.Join(entitySummary.Synonyms, ", ")))
		}
		if len(entitySummary.KeyColumns) > 0 {
			sb.WriteString("Key Columns:\n")
			for _, kc := range entitySummary.KeyColumns {
				sb.WriteString(fmt.Sprintf("  - %s\n", kc.Name))
			}
		}
	}

	sb.WriteString(fmt.Sprintf(`
## TASK

Analyze the table "%s" and determine if there are any questions that would significantly improve understanding of this entity.

Focus on:
1. Status/type/state columns - what do their values mean?
2. Unclear column purposes - what data do they hold?
3. Business rules - any constraints or workflows encoded?
4. Relationships - any unclear foreign key meanings?

IMPORTANT: Only generate questions if there is genuine ambiguity. Many tables are self-explanatory.

## OUTPUT FORMAT

Return ONLY a JSON object:

%sjson
{
  "analysis": "Brief summary of what this table represents and any ambiguities found",
  "entity_summary": {
    "business_name": "Human-readable name for this entity (e.g., 'Customer Orders' for orders table)",
    "description": "1-2 sentence description of what this table represents in business terms",
    "domain": "Business domain classification (e.g., billing, users, inventory, analytics)"
  },
  "questions": [
    {
      "text": "What are the possible values for the 'status' column and what does each represent?",
      "priority": 2,
      "category": "business_rules",
      "reasoning": "Status column likely encodes workflow states",
      "is_required": false,
      "affects_columns": ["status"]
    }
  ]
}
%s

Priority scale: 1=Critical, 2=High, 3=Medium, 4=Low, 5=Optional
Categories: business_rules, relationship, terminology, enumeration, temporal, data_quality

If the table is self-explanatory, return:
%sjson
{
  "analysis": "This table clearly represents X and all columns have obvious purposes.",
  "entity_summary": {
    "business_name": "...",
    "description": "...",
    "domain": "..."
  },
  "questions": []
}
%s
`, table.TableName, "```", "```", "```", "```"))

	return sb.String()
}

func (s *ontologyBuilderService) parseEntityAnalysisResponse(response string, projectID uuid.UUID, ontologyID uuid.UUID, workflowID uuid.UUID, tableName string) ([]*models.OntologyQuestion, *models.EntitySummary, error) {
	type llmResponse struct {
		Analysis      string `json:"analysis"`
		EntitySummary struct {
			BusinessName string `json:"business_name"`
			Description  string `json:"description"`
			Domain       string `json:"domain"`
		} `json:"entity_summary"`
		Questions []struct {
			Text           string   `json:"text"`
			Priority       int      `json:"priority"`
			Category       string   `json:"category"`
			Reasoning      string   `json:"reasoning"`
			IsRequired     bool     `json:"is_required"`
			AffectsColumns []string `json:"affects_columns"`
		} `json:"questions"`
	}

	parsed, err := llm.ParseJSONResponse[llmResponse](response)
	if err != nil {
		return nil, nil, err
	}

	questions := make([]*models.OntologyQuestion, 0, len(parsed.Questions))
	for _, raw := range parsed.Questions {
		if raw.Text == "" {
			continue
		}

		priority := raw.Priority
		if priority < 1 {
			priority = 1
		}
		if priority > 5 {
			priority = 5
		}

		category := raw.Category
		if category == "" {
			category = "general"
		}

		q := &models.OntologyQuestion{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: &workflowID,
			Text:       raw.Text,
			Priority:   priority,
			Category:   category,
			Reasoning:  raw.Reasoning,
			IsRequired: raw.IsRequired,
			Status:     models.QuestionStatusPending,
			Affects: &models.QuestionAffects{
				Tables:  []string{tableName},
				Columns: raw.AffectsColumns,
			},
		}

		questions = append(questions, q)
	}

	// Build entity summary if LLM provided one
	var entitySummary *models.EntitySummary
	if parsed.EntitySummary.BusinessName != "" || parsed.EntitySummary.Description != "" || parsed.EntitySummary.Domain != "" {
		entitySummary = &models.EntitySummary{
			TableName:    tableName,
			BusinessName: parsed.EntitySummary.BusinessName,
			Description:  parsed.EntitySummary.Description,
			Domain:       parsed.EntitySummary.Domain,
		}
	}

	return questions, entitySummary, nil
}

// ============================================================================
// GenerateQuestions (Deprecated)
// ============================================================================

func (s *ontologyBuilderService) GenerateQuestions(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID) ([]*models.OntologyQuestion, error) {
	startTime := time.Now()

	// Add workflow context for conversation recording
	ctx = llm.WithWorkflowID(ctx, workflowID)

	s.logger.Info("Generating questions",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflowID.String()))

	// Get workflow to find datasource
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	if workflow.Config == nil {
		return nil, fmt.Errorf("workflow has no config")
	}

	// Load schema tables
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, workflow.Config.DatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	if len(tables) == 0 {
		s.logger.Warn("No tables found for question generation")
		return []*models.OntologyQuestion{}, nil
	}

	// Load relationships
	relationships, err := s.loadRelationships(ctx, projectID, tables)
	if err != nil {
		s.logger.Error("Failed to load relationships", zap.Error(err))
	}

	// Build schema context
	schemaContext := s.buildSchemaContext(tables, relationships)

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	prompt := s.buildQuestionGenerationPrompt(schemaContext)
	systemMsg := s.questionGenerationSystemMessage()

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	questions, err := s.parseQuestionsResponse(result.Content, projectID, workflow.OntologyID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Sort by priority and limit
	questions = s.sortAndLimitQuestions(questions, 10)

	s.logger.Info("Questions generated",
		zap.Int("question_count", len(questions)),
		zap.Duration("elapsed", time.Since(startTime)))

	return questions, nil
}

func (s *ontologyBuilderService) questionGenerationSystemMessage() string {
	return `You are a database analyst helping to understand a business domain by examining its schema.
Your goal is to identify the most important questions that would help understand what this data represents and how it's used in the business.

CRITICAL RULES:
- ONLY ask questions about tables/columns that ACTUALLY EXIST in the schema provided
- NEVER make up or assume tables/columns that aren't shown
- Reference SPECIFIC table.column names in your questions using the EXACT names from the schema
- Focus on BUSINESS understanding, not technical details
- Prioritize highly-connected tables - they are central to the business
- Status/type columns encode business logic - ask about their meanings`
}

func (s *ontologyBuilderService) buildSchemaContext(tables []*models.SchemaTable, relationships []*models.SchemaRelationship) string {
	var sb strings.Builder

	sb.WriteString("## DATABASE SCHEMA\n\n")
	sb.WriteString(fmt.Sprintf("Total tables: %d\n", len(tables)))
	if len(relationships) > 0 {
		sb.WriteString(fmt.Sprintf("Foreign key relationships: %d\n", len(relationships)))
	}
	sb.WriteString("\n")

	// Build table ID to name lookup for relationships
	tableNameLookup := make(map[string]string)
	for _, t := range tables {
		tableNameLookup[t.ID.String()] = t.TableName
	}

	// Build relationship count for sorting
	relationshipCount := make(map[string]int)
	for _, rel := range relationships {
		relationshipCount[rel.SourceTableID.String()]++
		relationshipCount[rel.TargetTableID.String()]++
	}

	// Sort tables by relationship count
	sortedTables := make([]*models.SchemaTable, len(tables))
	copy(sortedTables, tables)
	sort.Slice(sortedTables, func(i, j int) bool {
		return relationshipCount[sortedTables[i].ID.String()] > relationshipCount[sortedTables[j].ID.String()]
	})

	for _, table := range sortedTables {
		relCount := relationshipCount[table.ID.String()]
		centralityNote := ""
		if relCount >= 5 {
			centralityNote = " (HIGHLY CONNECTED)"
		} else if relCount >= 3 {
			centralityNote = " (well connected)"
		}

		sb.WriteString(fmt.Sprintf("### TABLE: %s%s\n", table.TableName, centralityNote))
		if table.RowCount != nil {
			sb.WriteString(fmt.Sprintf("Row count: %d\n", *table.RowCount))
		}
		sb.WriteString("Columns:\n")

		for _, col := range table.Columns {
			flags := []string{}
			if col.IsPrimaryKey {
				flags = append(flags, "PK")
			}
			if col.IsNullable {
				flags = append(flags, "nullable")
			}

			flagStr := ""
			if len(flags) > 0 {
				flagStr = " [" + strings.Join(flags, ", ") + "]"
			}

			sb.WriteString(fmt.Sprintf("  - %s: %s%s\n", col.ColumnName, col.DataType, flagStr))
		}
		sb.WriteString("\n")
	}

	if len(relationships) > 0 {
		sb.WriteString("## RELATIONSHIPS\n\n")
		for _, rel := range relationships {
			sourceName := tableNameLookup[rel.SourceTableID.String()]
			targetName := tableNameLookup[rel.TargetTableID.String()]
			// Only include relationships where both tables are known
			if sourceName != "" && targetName != "" {
				sb.WriteString(fmt.Sprintf("- %s -> %s\n", sourceName, targetName))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (s *ontologyBuilderService) buildQuestionGenerationPrompt(schemaContext string) string {
	return fmt.Sprintf(`%s

## TASK

Generate 3-10 targeted questions that would SIGNIFICANTLY improve understanding of what this data represents and how it's used.

## CRITICAL REQUIREMENTS

1. **Schema-Grounded**: Every question MUST reference actual table/column names from the schema above.

2. **High Impact**: Focus on questions that would unlock understanding of:
   - Core business entities and their purposes
   - Workflow states and transitions
   - Key relationships between entities
   - Business rules encoded in the data

3. **Prioritize** (in this order):
   - Tables marked as "(HIGHLY CONNECTED)" or "(well connected)"
   - Columns named 'status', 'type', 'state', 'category'
   - Unclear naming conventions or abbreviations

## OUTPUT FORMAT

Return ONLY a JSON array:

%sjson
[
  {
    "text": "The 'orders' table has a 'status' column. What are the possible order statuses?",
    "priority": 1,
    "category": "business_rules",
    "reasoning": "Understanding order lifecycle is critical for reporting",
    "affects": {
      "tables": ["orders"],
      "columns": ["orders.status"]
    },
    "detected_pattern": "status_column"
  }
]
%s

## Priority Scale
- 1 = Critical (blocks understanding of core business)
- 2 = High (affects multiple entities/queries)
- 3 = Medium (clarifies important details)
- 4 = Low (nice to have)
- 5 = Optional (minor clarification)

## Categories
- business_rules: Status columns, state machines, validation rules
- relationship: How entities relate, cardinality, business meaning
- terminology: What terms/abbreviations mean in this business
- enumeration: What specific values represent
- temporal: Date ranges, time-based patterns
- data_quality: Null handling, expected values`, schemaContext, "```", "```")
}

func (s *ontologyBuilderService) parseQuestionsResponse(response string, projectID uuid.UUID, ontologyID uuid.UUID, workflowID uuid.UUID) ([]*models.OntologyQuestion, error) {
	type llmQuestion struct {
		Text      string `json:"text"`
		Priority  int    `json:"priority"`
		Category  string `json:"category"`
		Reasoning string `json:"reasoning"`
		Affects   struct {
			Tables  []string `json:"tables"`
			Columns []string `json:"columns"`
		} `json:"affects"`
		DetectedPattern string `json:"detected_pattern"`
	}

	rawQuestions, err := llm.ParseJSONResponse[[]llmQuestion](response)
	if err != nil {
		return nil, err
	}

	questions := make([]*models.OntologyQuestion, 0, len(rawQuestions))
	for _, raw := range rawQuestions {
		if raw.Text == "" {
			continue
		}

		// Clamp priority
		priority := raw.Priority
		if priority < 1 {
			priority = 1
		}
		if priority > 5 {
			priority = 5
		}

		category := raw.Category
		if category == "" {
			category = "general"
		}

		q := &models.OntologyQuestion{
			ID:              uuid.New(),
			ProjectID:       projectID,
			OntologyID:      ontologyID,
			WorkflowID:      &workflowID,
			Text:            raw.Text,
			Priority:        priority,
			Category:        category,
			Reasoning:       raw.Reasoning,
			DetectedPattern: raw.DetectedPattern,
			Status:          models.QuestionStatusPending,
			Affects: &models.QuestionAffects{
				Tables:  raw.Affects.Tables,
				Columns: raw.Affects.Columns,
			},
		}

		questions = append(questions, q)
	}

	return questions, nil
}

func (s *ontologyBuilderService) sortAndLimitQuestions(questions []*models.OntologyQuestion, maxCount int) []*models.OntologyQuestion {
	sort.Slice(questions, func(i, j int) bool {
		return questions[i].Priority < questions[j].Priority
	})

	if len(questions) > maxCount {
		return questions[:maxCount]
	}
	return questions
}

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

// ============================================================================
// Helper Methods
// ============================================================================

func (s *ontologyBuilderService) loadRelationships(ctx context.Context, projectID uuid.UUID, tables []*models.SchemaTable) ([]*models.SchemaRelationship, error) {
	if len(tables) == 0 {
		return nil, nil
	}

	// Get datasource ID from first table
	datasourceID := tables[0].DatasourceID

	return s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
}

func (s *ontologyBuilderService) buildRelationshipMap(relationships []*models.SchemaRelationship, tables []*models.SchemaTable) map[string][]string {
	relMap := make(map[string][]string)

	if relationships == nil || len(relationships) == 0 {
		return relMap
	}

	// Build lookup from table ID to table name
	tableNameLookup := make(map[string]string)
	for _, t := range tables {
		tableNameLookup[t.ID.String()] = t.TableName
	}

	for _, rel := range relationships {
		sourceName := tableNameLookup[rel.SourceTableID.String()]
		targetName := tableNameLookup[rel.TargetTableID.String()]

		// Skip relationships where either table is not in our lookup
		if sourceName == "" || targetName == "" {
			continue
		}

		relMap[sourceName] = append(relMap[sourceName], targetName)
		relMap[targetName] = append(relMap[targetName], sourceName)
	}

	// Deduplicate
	for key, related := range relMap {
		relMap[key] = uniqueStrings(related)
	}

	return relMap
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(input))

	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// ============================================================================
// ProcessProjectDescription - Kickoff Task
// ============================================================================

func (s *ontologyBuilderService) ProcessProjectDescription(ctx context.Context, projectID uuid.UUID, workflowID uuid.UUID, description string) (*DescriptionProcessingResult, error) {
	if description == "" {
		// No description provided - return empty result
		return &DescriptionProcessingResult{
			EntityHints: make(map[string]*models.EntityHint),
		}, nil
	}

	// Add workflow context for conversation recording
	ctx = llm.WithWorkflowID(ctx, workflowID)

	startTime := time.Now()
	s.logger.Info("Processing project description",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflowID.String()),
		zap.Int("description_len", len(description)))

	// Update workflow progress
	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseDescriptionProcessing,
		Message:      "Analyzing project description...",
	}); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	// Get workflow to find datasource
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	if workflow.Config == nil {
		return nil, fmt.Errorf("workflow has no config")
	}

	// Load schema tables for context
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, workflow.Config.DatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	// Build schema summary for LLM
	schemaSummary := s.buildSchemaSummaryForDescription(tables)

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	prompt := s.buildDescriptionProcessingPrompt(description, schemaSummary)
	systemMsg := s.descriptionProcessingSystemMessage()

	llmResult, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	result, err := s.parseDescriptionProcessingResponse(llmResult.Content, projectID, workflow.OntologyID, workflowID)
	if err != nil {
		s.logger.Warn("Failed to parse description processing response",
			zap.Error(err),
			zap.String("response", llmResult.Content))
		// Return empty result rather than failing
		return &DescriptionProcessingResult{
			EntityHints: make(map[string]*models.EntityHint),
		}, nil
	}

	// Store domain context in ontology metadata for use by subsequent tasks
	if result.DomainContext != nil {
		metadata := map[string]any{
			"domain_context": result.DomainContext,
		}
		if len(result.EntityHints) > 0 {
			metadata["entity_hints"] = result.EntityHints
		}
		if err := s.ontologyRepo.UpdateMetadata(ctx, projectID, metadata); err != nil {
			s.logger.Error("Failed to store domain context in metadata",
				zap.Error(err))
			// Continue - this is non-fatal
		}
	}

	// Persist knowledge facts extracted from the description
	for _, fact := range result.KnowledgeFacts {
		if err := s.knowledgeRepo.Upsert(ctx, fact); err != nil {
			s.logger.Error("Failed to store knowledge fact from description",
				zap.String("fact_type", fact.FactType),
				zap.String("key", fact.Key),
				zap.Error(err))
			// Continue - non-fatal, log and proceed
		}
	}

	// Note: Clarifying questions from description analysis are now returned in the result
	// and should be stored in workflow state by the caller (InitializeOntologyTask)
	if len(result.ClarifyingQuestions) > 0 {
		s.logger.Info("Description processing generated clarifying questions",
			zap.Int("count", len(result.ClarifyingQuestions)))
	}

	s.logger.Info("Project description processed",
		zap.Int("knowledge_facts", len(result.KnowledgeFacts)),
		zap.Int("questions", len(result.ClarifyingQuestions)),
		zap.Int("entity_hints", len(result.EntityHints)),
		zap.Duration("elapsed", time.Since(startTime)))

	return result, nil
}

func (s *ontologyBuilderService) descriptionProcessingSystemMessage() string {
	return `You are a database analyst helping to understand a data source. The user has provided a description of their data. Your job is to:

1. Extract structured knowledge from their description
2. Identify terminology and business rules they've mentioned
3. Generate clarifying questions where the description is ambiguous or incomplete
4. Provide hints for entity naming and domain classification

IMPORTANT:
- Extract only what the user explicitly stated or clearly implied
- Do not invent information not present in the description
- Flag ambiguities as clarifying questions rather than making assumptions
- Your output will seed the ontology, so be precise and conservative
- Be concise - each fact/question should be a single focused item

Return a JSON object with: domain_context, knowledge_facts, clarifying_questions, entity_hints`
}

func (s *ontologyBuilderService) buildSchemaSummaryForDescription(tables []*models.SchemaTable) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Total tables: %d\n\n", len(tables)))

	for _, table := range tables {
		sb.WriteString(fmt.Sprintf("### %s\n", table.TableName))
		if table.RowCount != nil {
			sb.WriteString(fmt.Sprintf("Rows: %d\n", *table.RowCount))
		}
		sb.WriteString("Columns: ")
		colNames := make([]string, 0, len(table.Columns))
		for _, col := range table.Columns {
			colNames = append(colNames, col.ColumnName)
		}
		sb.WriteString(strings.Join(colNames, ", "))
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func (s *ontologyBuilderService) buildDescriptionProcessingPrompt(description, schemaSummary string) string {
	return fmt.Sprintf(`## User's Description
%s

## Database Schema
%s

## Task
Analyze the user's description against the schema and extract:
1. A refined domain context summary (concise, LLM-optimized)
2. Key terminology definitions mentioned
3. Business rules mentioned or implied
4. Clarifying questions where the description is ambiguous
5. Entity hints (business names, domains, synonyms) for specific tables

## Output Format
Return a JSON object:
%sjson
{
  "domain_context": {
    "summary": "Brief description of what this database represents",
    "primary_domains": ["sales", "inventory"],
    "key_terminology": {
      "Customer": "Person who has completed a purchase",
      "SKU": "Stock Keeping Unit"
    }
  },
  "knowledge_facts": [
    {
      "fact_type": "terminology",
      "key": "customer_definition",
      "value": "Person who has completed at least one purchase"
    }
  ],
  "clarifying_questions": [
    {
      "text": "What are the possible values for order status?",
      "priority": 1,
      "category": "business_rules",
      "reasoning": "Understanding order lifecycle is critical"
    }
  ],
  "entity_hints": {
    "orders": {
      "business_name": "Customer Orders",
      "domain": "sales"
    }
  }
}
%s`, description, schemaSummary, "```", "```")
}

func (s *ontologyBuilderService) parseDescriptionProcessingResponse(response string, projectID uuid.UUID, ontologyID uuid.UUID, workflowID uuid.UUID) (*DescriptionProcessingResult, error) {
	type llmResponse struct {
		DomainContext struct {
			Summary        string            `json:"summary"`
			PrimaryDomains []string          `json:"primary_domains"`
			KeyTerminology map[string]string `json:"key_terminology"`
		} `json:"domain_context"`
		KnowledgeFacts []struct {
			FactType string `json:"fact_type"`
			Key      string `json:"key"`
			Value    string `json:"value"`
			Context  string `json:"context"`
		} `json:"knowledge_facts"`
		ClarifyingQuestions []struct {
			Text      string `json:"text"`
			Priority  int    `json:"priority"`
			Category  string `json:"category"`
			Reasoning string `json:"reasoning"`
			Affects   struct {
				Tables  []string `json:"tables"`
				Columns []string `json:"columns"`
			} `json:"affects"`
		} `json:"clarifying_questions"`
		EntityHints map[string]struct {
			BusinessName string   `json:"business_name"`
			Domain       string   `json:"domain"`
			Synonyms     []string `json:"synonyms"`
		} `json:"entity_hints"`
	}

	parsed, err := llm.ParseJSONResponse[llmResponse](response)
	if err != nil {
		return nil, err
	}

	result := &DescriptionProcessingResult{
		DomainContext: &models.DomainContext{
			Summary:        parsed.DomainContext.Summary,
			PrimaryDomains: parsed.DomainContext.PrimaryDomains,
			KeyTerminology: parsed.DomainContext.KeyTerminology,
		},
		KnowledgeFacts:      make([]*models.KnowledgeFact, 0, len(parsed.KnowledgeFacts)),
		ClarifyingQuestions: make([]*models.OntologyQuestion, 0, len(parsed.ClarifyingQuestions)),
		EntityHints:         make(map[string]*models.EntityHint),
	}

	// Convert knowledge facts
	for _, kf := range parsed.KnowledgeFacts {
		if kf.Key == "" || kf.Value == "" {
			continue
		}
		result.KnowledgeFacts = append(result.KnowledgeFacts, &models.KnowledgeFact{
			ProjectID: projectID,
			FactType:  kf.FactType,
			Key:       kf.Key,
			Value:     kf.Value,
			Context:   "Extracted from user description",
		})
	}

	// Convert clarifying questions
	for _, cq := range parsed.ClarifyingQuestions {
		if cq.Text == "" {
			continue
		}
		priority := cq.Priority
		if priority < 1 || priority > 5 {
			priority = 2
		}
		q := &models.OntologyQuestion{
			ID:              uuid.New(),
			ProjectID:       projectID,
			OntologyID:      ontologyID,
			WorkflowID:      &workflowID,
			Text:            cq.Text,
			Priority:        priority,
			Category:        cq.Category,
			Reasoning:       cq.Reasoning,
			DetectedPattern: "user_description",
			Status:          models.QuestionStatusPending,
		}
		if len(cq.Affects.Tables) > 0 || len(cq.Affects.Columns) > 0 {
			q.Affects = &models.QuestionAffects{
				Tables:  cq.Affects.Tables,
				Columns: cq.Affects.Columns,
			}
		}
		result.ClarifyingQuestions = append(result.ClarifyingQuestions, q)
	}

	// Convert entity hints
	for tableName, hint := range parsed.EntityHints {
		result.EntityHints[tableName] = &models.EntityHint{
			BusinessName: hint.BusinessName,
			Domain:       hint.Domain,
			Synonyms:     hint.Synonyms,
		}
	}

	return result, nil
}

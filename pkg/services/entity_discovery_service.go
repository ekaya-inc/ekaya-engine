package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityDiscoveryService provides entity discovery operations for the DAG workflow.
// It contains the core algorithms for identifying and enriching entities from schema metadata.
type EntityDiscoveryService interface {
	// IdentifyEntitiesFromDDL discovers entities from DDL metadata (PK/unique constraints)
	IdentifyEntitiesFromDDL(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID) (int, []*models.SchemaTable, []*models.SchemaColumn, error)

	// EnrichEntitiesWithLLM uses an LLM to generate entity names and descriptions
	EnrichEntitiesWithLLM(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID, tables []*models.SchemaTable, columns []*models.SchemaColumn) error

	// ValidateEnrichment checks that all entities have non-empty descriptions
	ValidateEnrichment(ctx context.Context, projectID, ontologyID uuid.UUID) error
}

type entityDiscoveryService struct {
	entityRepo       repositories.OntologyEntityRepository
	schemaRepo       repositories.SchemaRepository
	ontologyRepo     repositories.OntologyRepository
	conversationRepo repositories.ConversationRepository
	questionService  OntologyQuestionService
	llmFactory       llm.LLMClientFactory
	workerPool       *llm.WorkerPool
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewEntityDiscoveryService creates a new entity discovery service.
func NewEntityDiscoveryService(
	entityRepo repositories.OntologyEntityRepository,
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	conversationRepo repositories.ConversationRepository,
	questionService OntologyQuestionService,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) EntityDiscoveryService {
	return &entityDiscoveryService{
		entityRepo:       entityRepo,
		schemaRepo:       schemaRepo,
		ontologyRepo:     ontologyRepo,
		conversationRepo: conversationRepo,
		questionService:  questionService,
		llmFactory:       llmFactory,
		workerPool:       workerPool,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("entity-discovery"),
	}
}

var _ EntityDiscoveryService = (*entityDiscoveryService)(nil)

// entityCandidate represents a column that may represent an entity.
type entityCandidate struct {
	schemaName string
	tableName  string
	columnName string
	confidence float64 // 1.0 for PK, 0.9 for unique+not null
	reason     string  // "primary_key" or "unique_not_null"
}

// IdentifyEntitiesFromDDL finds entities using DDL metadata (is_primary_key, is_unique)
// from engine_schema_columns instead of running expensive COUNT(DISTINCT) queries.
// Returns the count and the tables/columns for LLM enrichment.
func (s *entityDiscoveryService) IdentifyEntitiesFromDDL(
	ctx context.Context,
	projectID, ontologyID, datasourceID uuid.UUID,
) (int, []*models.SchemaTable, []*models.SchemaColumn, error) {
	return s.identifyEntitiesFromDDL(ctx, projectID, ontologyID, datasourceID)
}

// identifyEntitiesFromDDL is the internal implementation.
func (s *entityDiscoveryService) identifyEntitiesFromDDL(
	ctx context.Context,
	projectID, ontologyID, datasourceID uuid.UUID,
) (int, []*models.SchemaTable, []*models.SchemaColumn, error) {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get selected tables for this datasource (respects is_selected flag to exclude test/sample tables)
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, true)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("list tables: %w", err)
	}

	// Build table lookup by ID
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	// Get all columns for this datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("list columns: %w", err)
	}

	// Find entity candidates from DDL metadata
	// Priority: primary key (100% confidence) > unique+not null (90% confidence)
	var candidates []entityCandidate

	for _, col := range columns {
		table, ok := tableByID[col.SchemaTableID]
		if !ok {
			continue
		}

		// Primary key: 100% confidence
		if col.IsPrimaryKey {
			candidates = append(candidates, entityCandidate{
				schemaName: table.SchemaName,
				tableName:  table.TableName,
				columnName: col.ColumnName,
				confidence: 1.0,
				reason:     "primary_key",
			})
			s.logger.Info("Found primary key column",
				zap.String("column", fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, col.ColumnName)))
			continue
		}

		// Unique + not nullable: 90% confidence
		if col.IsUnique && !col.IsNullable {
			candidates = append(candidates, entityCandidate{
				schemaName: table.SchemaName,
				tableName:  table.TableName,
				columnName: col.ColumnName,
				confidence: 0.9,
				reason:     "unique_not_null",
			})
			s.logger.Info("Found unique non-nullable column",
				zap.String("column", fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, col.ColumnName)),
				zap.Float64("confidence", 0.9))
		}
	}

	// Group candidates by table to select the best one per table
	// (prefer PK over unique+not null)
	bestByTable := make(map[string]entityCandidate)
	for _, c := range candidates {
		tableKey := fmt.Sprintf("%s.%s", c.schemaName, c.tableName)
		if existing, ok := bestByTable[tableKey]; !ok || c.confidence > existing.confidence {
			bestByTable[tableKey] = c
		}
	}

	// Group similar tables by core concept to prevent duplicate entities
	// e.g., "users", "s1_users", "test_users" all map to concept "users"
	tableGroups := groupSimilarTables(tables)

	// Create ONE entity per concept group, using the primary (non-test) table
	entityCount := 0
	for concept, groupTables := range tableGroups {
		// Select the primary table (prefer non-test tables)
		primaryTable := selectPrimaryTable(groupTables)
		tableKey := fmt.Sprintf("%s.%s", primaryTable.SchemaName, primaryTable.TableName)

		// Get the best candidate for the primary table
		c, ok := bestByTable[tableKey]
		if !ok {
			// No PK/unique constraint found for primary table, try alternate tables in group
			for _, altTable := range groupTables {
				if altTable.TableName == primaryTable.TableName {
					continue
				}
				altKey := fmt.Sprintf("%s.%s", altTable.SchemaName, altTable.TableName)
				if altC, found := bestByTable[altKey]; found {
					// Use the alternate table's column info for the entity
					c = altC
					ok = true
					break
				}
			}
			if !ok {
				s.logger.Debug("No entity candidate found for concept group",
					zap.String("concept", concept),
					zap.Int("tables_in_group", len(groupTables)))
				continue
			}
		}

		entity := &models.OntologyEntity{
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          primaryTable.TableName, // Temporary - will be enriched by LLM
			Description:   "",                     // Will be filled by LLM
			PrimarySchema: primaryTable.SchemaName,
			PrimaryTable:  primaryTable.TableName,
			PrimaryColumn: c.columnName,
			// Provenance: DDL-derived entities start with lower confidence,
			// will be increased after LLM enrichment
			Confidence: 0.5,
		}

		if err := s.entityRepo.Create(tenantCtx, entity); err != nil {
			s.logger.Error("Failed to create entity",
				zap.String("table_name", primaryTable.TableName),
				zap.Error(err))
			return 0, nil, nil, fmt.Errorf("create entity for table %s: %w", primaryTable.TableName, err)
		}

		s.logger.Info("Entity created (pending LLM enrichment)",
			zap.String("entity_id", entity.ID.String()),
			zap.String("table_name", primaryTable.TableName),
			zap.String("primary_location", fmt.Sprintf("%s.%s.%s", primaryTable.SchemaName, primaryTable.TableName, c.columnName)),
			zap.Float64("confidence", c.confidence),
			zap.String("reason", c.reason),
			zap.Int("grouped_tables", len(groupTables)))

		// Store alternate tables as aliases for query matching
		aliasSource := "table_grouping"
		for _, altTable := range groupTables {
			if altTable.TableName == primaryTable.TableName {
				continue
			}
			alias := &models.OntologyEntityAlias{
				EntityID: entity.ID,
				Alias:    altTable.TableName,
				Source:   &aliasSource,
			}
			if err := s.entityRepo.CreateAlias(tenantCtx, alias); err != nil {
				s.logger.Error("Failed to create alias for grouped table",
					zap.String("entity_id", entity.ID.String()),
					zap.String("alias", altTable.TableName),
					zap.Error(err))
				// Continue - alias creation failure is not fatal
			} else {
				s.logger.Debug("Created alias for grouped table",
					zap.String("entity_id", entity.ID.String()),
					zap.String("alias", altTable.TableName))
			}
		}

		entityCount++
	}

	return entityCount, tables, columns, nil
}

// entityEnrichment holds LLM-generated entity name, description, and additional metadata.
// keyColumnEnrichment holds LLM-generated metadata for a key column.
type keyColumnEnrichment struct {
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms,omitempty"`
}

type entityEnrichment struct {
	TableName        string                `json:"table_name"`
	EntityName       string                `json:"entity_name"`
	Description      string                `json:"description"`
	Domain           string                `json:"domain"`
	KeyColumns       []keyColumnEnrichment `json:"key_columns"`
	AlternativeNames []string              `json:"alternative_names"`
}

// ontologyQuestionInput represents a question generated by the LLM during enrichment.
// These questions identify areas of uncertainty where user clarification would improve accuracy.
type ontologyQuestionInput struct {
	Category string `json:"category"` // terminology | enumeration | relationship | business_rules | temporal | data_quality
	Priority int    `json:"priority"` // 1=critical | 2=important | 3=nice-to-have
	Question string `json:"question"` // Clear question for domain expert
	Context  string `json:"context"`  // Relevant schema/data context
}

// EnrichEntitiesWithLLM uses an LLM to generate clean entity names and descriptions
// based on the full schema context.
func (s *entityDiscoveryService) EnrichEntitiesWithLLM(
	ctx context.Context,
	projectID, ontologyID, datasourceID uuid.UUID,
	tables []*models.SchemaTable,
	columns []*models.SchemaColumn,
) error {
	return s.enrichEntitiesWithLLM(ctx, projectID, ontologyID, datasourceID, tables, columns)
}

// enrichEntitiesWithLLM is the internal implementation.
func (s *entityDiscoveryService) enrichEntitiesWithLLM(
	ctx context.Context,
	projectID, ontologyID, datasourceID uuid.UUID,
	tables []*models.SchemaTable,
	columns []*models.SchemaColumn,
) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get entities we just created
	entities, err := s.entityRepo.GetByOntology(tenantCtx, ontologyID)
	if err != nil {
		return fmt.Errorf("get entities: %w", err)
	}

	if len(entities) == 0 {
		return nil
	}

	// Build table -> columns map for context
	tableColumns := make(map[string][]string)
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}
	for _, col := range columns {
		if t, ok := tableByID[col.SchemaTableID]; ok {
			key := fmt.Sprintf("%s.%s", t.SchemaName, t.TableName)
			tableColumns[key] = append(tableColumns[key], col.ColumnName)
		}
	}

	// Use batched enrichment when there are many entities to avoid token limits
	const entityBatchSize = 20
	if len(entities) > entityBatchSize && s.workerPool != nil {
		s.logger.Info("Using batched entity enrichment",
			zap.Int("total_entities", len(entities)),
			zap.Int("batch_size", entityBatchSize))
		return s.enrichEntitiesInBatches(ctx, projectID, entities, tableColumns, entityBatchSize)
	}

	// Single batch enrichment for small entity sets or when worker pool unavailable
	return s.enrichEntityBatch(tenantCtx, projectID, entities, tableColumns)
}

// enrichEntitiesInBatches splits entities into batches and processes them in parallel.
// Fails fast if any batch fails.
func (s *entityDiscoveryService) enrichEntitiesInBatches(
	ctx context.Context,
	projectID uuid.UUID,
	entities []*models.OntologyEntity,
	tableColumns map[string][]string,
	batchSize int,
) error {
	// Build work items for each batch
	var workItems []llm.WorkItem[int]

	for i := 0; i < len(entities); i += batchSize {
		end := i + batchSize
		if end > len(entities) {
			end = len(entities)
		}
		batch := entities[i:end]
		batchID := fmt.Sprintf("batch-%d", i/batchSize)
		batchStart := i
		batchEnd := end

		// Capture batch for closure
		batchCopy := batch
		workItems = append(workItems, llm.WorkItem[int]{
			ID: batchID,
			Execute: func(ctx context.Context) (int, error) {
				// Acquire a fresh database connection for this batch to avoid
				// concurrent access issues when multiple batches run in parallel.
				var batchCtx context.Context
				var cleanup func()
				if s.getTenantCtx != nil {
					var err error
					batchCtx, cleanup, err = s.getTenantCtx(ctx, projectID)
					if err != nil {
						s.logger.Error("Failed to acquire tenant context for batch",
							zap.String("batch_id", batchID),
							zap.Error(err))
						return 0, fmt.Errorf("acquire tenant context: %w", err)
					}
					defer cleanup()
				} else {
					batchCtx = ctx
				}

				s.logger.Debug("Enriching entity batch",
					zap.String("batch_id", batchID),
					zap.Int("batch_start", batchStart),
					zap.Int("batch_end", batchEnd),
					zap.Int("batch_size", len(batchCopy)))

				if err := s.enrichEntityBatch(batchCtx, projectID, batchCopy, tableColumns); err != nil {
					return 0, fmt.Errorf("batch %d-%d failed: %w", batchStart, batchEnd, err)
				}
				return len(batchCopy), nil
			},
		})
	}

	// Process all batches with worker pool
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		s.logger.Debug("Entity enrichment progress",
			zap.Int("batches_completed", completed),
			zap.Int("total_batches", total))
	})

	// Check for failures - fail fast on any batch error
	totalEnriched := 0
	for _, r := range results {
		if r.Err != nil {
			return fmt.Errorf("entity enrichment batch failed: %w", r.Err)
		}
		totalEnriched += r.Result
	}

	s.logger.Info("Enriched entities with LLM-generated metadata (batched)",
		zap.Int("total_entities", len(entities)),
		zap.Int("batches", len(workItems)),
		zap.Int("enriched", totalEnriched))

	return nil
}

// enrichEntityBatch enriches a single batch of entities via LLM.
func (s *entityDiscoveryService) enrichEntityBatch(
	ctx context.Context,
	projectID uuid.UUID,
	entities []*models.OntologyEntity,
	tableColumns map[string][]string,
) error {
	// Query for entity names that have already been enriched (have descriptions).
	// This helps prevent duplicate names when processing in batches.
	var existingNames []string
	if len(entities) > 0 {
		allEntities, err := s.entityRepo.GetByOntology(ctx, entities[0].OntologyID)
		if err != nil {
			s.logger.Warn("Failed to query existing entity names for prompt context",
				zap.Error(err))
			// Continue without existing names - not fatal
		} else {
			for _, e := range allEntities {
				// Only include entities that have been enriched (have non-empty descriptions)
				// and whose name differs from their primary table (indicating LLM-generated name)
				if e.Description != "" && e.Name != e.PrimaryTable {
					existingNames = append(existingNames, e.Name)
				}
			}
			if len(existingNames) > 0 {
				s.logger.Debug("Including existing entity names in prompt",
					zap.Int("existing_count", len(existingNames)),
					zap.Strings("names", existingNames))
			}
		}
	}

	// Build the prompt for this batch
	prompt := s.buildEntityEnrichmentPrompt(entities, tableColumns, existingNames)
	systemMsg := s.entityEnrichmentSystemMessage()

	// Get LLM client - must use tenant context for config lookup
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	enrichments, questions, err := s.parseEntityEnrichmentResponse(result.Content)
	if err != nil {
		s.logger.Error("Failed to parse entity enrichment response",
			zap.String("conversation_id", result.ConversationID.String()),
			zap.Error(err))

		// Record parse failure in LLM conversation for troubleshooting
		if s.conversationRepo != nil {
			errorMessage := fmt.Sprintf("parse_failure: %s", err.Error())
			if updateErr := s.conversationRepo.UpdateStatus(ctx, result.ConversationID, models.LLMConversationStatusError, errorMessage); updateErr != nil {
				s.logger.Error("Failed to update conversation status",
					zap.String("conversation_id", result.ConversationID.String()),
					zap.Error(updateErr))
			}
		}
		return fmt.Errorf("entity enrichment parse failure: %w", err)
	}

	// Store questions generated during enrichment
	if len(questions) > 0 {
		s.logger.Info("LLM generated questions during entity enrichment",
			zap.Int("question_count", len(questions)),
			zap.String("project_id", projectID.String()))

		// Get ontology ID for question storage
		ontologyID := entities[0].OntologyID
		questionInputs := make([]OntologyQuestionInput, len(questions))
		for i, q := range questions {
			questionInputs[i] = OntologyQuestionInput{
				Category: q.Category,
				Priority: q.Priority,
				Question: q.Question,
				Context:  q.Context,
			}
		}
		questionModels := ConvertQuestionInputs(questionInputs, projectID, ontologyID, nil)
		if s.questionService != nil && len(questionModels) > 0 {
			if err := s.questionService.CreateQuestions(ctx, questionModels); err != nil {
				s.logger.Error("failed to store ontology questions from entity enrichment",
					zap.Int("question_count", len(questionModels)),
					zap.Error(err))
				// Non-fatal: continue even if question storage fails
			} else {
				s.logger.Debug("Stored ontology questions from entity enrichment",
					zap.Int("question_count", len(questionModels)))
			}
		}
	}

	// Update entities with enriched names, descriptions, and new fields
	enrichmentByTable := make(map[string]entityEnrichment)
	for _, e := range enrichments {
		enrichmentByTable[e.TableName] = e
	}

	// Track entities not found in LLM response
	var unenrichedTables []string

	for _, entity := range entities {
		enrichment, found := enrichmentByTable[entity.PrimaryTable]
		if !found {
			unenrichedTables = append(unenrichedTables, entity.PrimaryTable)
			continue
		}

		entity.Name = enrichment.EntityName
		entity.Description = enrichment.Description
		entity.Domain = enrichment.Domain
		// Provenance: LLM enrichment increases confidence from DDL-based 0.5 to 0.8
		entity.Confidence = 0.8
		// Clear stale flag - entity was successfully re-enriched
		entity.IsStale = false
		if err := s.entityRepo.Update(ctx, entity); err != nil {
			s.logger.Error("Failed to update entity with enrichment",
				zap.String("entity_id", entity.ID.String()),
				zap.Error(err))
			// Continue with other entities
			continue
		}

		// Create key columns with synonyms
		for _, kc := range enrichment.KeyColumns {
			keyCol := &models.OntologyEntityKeyColumn{
				EntityID:   entity.ID,
				ColumnName: kc.Name,
				Synonyms:   kc.Synonyms,
			}
			if err := s.entityRepo.CreateKeyColumn(ctx, keyCol); err != nil {
				s.logger.Error("Failed to create key column",
					zap.String("entity_id", entity.ID.String()),
					zap.String("column_name", kc.Name),
					zap.Error(err))
				// Continue with other key columns
			}
		}

		// Create aliases (alternative names)
		discoverySource := "discovery"
		for _, altName := range enrichment.AlternativeNames {
			alias := &models.OntologyEntityAlias{
				EntityID: entity.ID,
				Alias:    altName,
				Source:   &discoverySource,
			}
			if err := s.entityRepo.CreateAlias(ctx, alias); err != nil {
				s.logger.Error("Failed to create entity alias",
					zap.String("entity_id", entity.ID.String()),
					zap.String("alias", altName),
					zap.Error(err))
				// Continue with other aliases
			}
		}
	}

	// Fail if any entities were not in the LLM response (truncated/incomplete response)
	if len(unenrichedTables) > 0 {
		return fmt.Errorf("entity enrichment incomplete: %d entities not in LLM response: %v", len(unenrichedTables), unenrichedTables)
	}

	s.logger.Info("Enriched entities with LLM-generated metadata",
		zap.Int("entity_count", len(entities)),
		zap.Int("enrichments_applied", len(enrichments)))

	return nil
}

func (s *entityDiscoveryService) entityEnrichmentSystemMessage() string {
	return `You are a data modeling expert. Your task is to convert database table names into clean, human-readable entity names, provide brief descriptions, identify the business domain, key business columns, and alternative names users might use.

Consider the full schema context to understand the domain and make informed guesses about each entity's purpose.`
}

func (s *entityDiscoveryService) buildEntityEnrichmentPrompt(
	entities []*models.OntologyEntity,
	tableColumns map[string][]string,
	existingNames []string,
) string {
	var sb strings.Builder

	// Include existing entity names to prevent duplicates
	if len(existingNames) > 0 {
		sb.WriteString("# IMPORTANT: Existing Entity Names\n\n")
		sb.WriteString("**EXISTING ENTITY NAMES (DO NOT REUSE):** ")
		sb.WriteString(strings.Join(existingNames, ", "))
		sb.WriteString("\n\n")
		sb.WriteString("When naming entities, you MUST:\n")
		sb.WriteString("1. Check if a similar name already exists above\n")
		sb.WriteString("2. Choose a distinct name if the concept is different\n")
		sb.WriteString("3. Merge tables representing the same concept under one name\n\n")
	}

	sb.WriteString("# Schema Context\n\n")
	sb.WriteString("Below are all the tables in this database with their columns. Use this context to understand what domain/industry this database serves.\n\n")

	// List all tables with columns for context
	for tableKey, cols := range tableColumns {
		sb.WriteString(fmt.Sprintf("**%s**: %s\n", tableKey, strings.Join(cols, ", ")))
	}

	sb.WriteString("\n# Task\n\n")
	sb.WriteString("For each table below, provide:\n")
	sb.WriteString("1. **Entity Name**: A clean, singular, Title Case name (e.g., \"users\" → \"User\", \"billing_activities\" → \"Billing Activity\")\n")
	sb.WriteString("2. **Description**: A brief (1-2 sentence) description of what this entity represents in the domain\n")
	sb.WriteString("3. **Domain**: A short, lowercase business domain (e.g., \"billing\", \"hospitality\", \"logistics\", \"customer\", \"analytics\")\n")
	sb.WriteString("4. **Key Columns**: 2-3 important business columns that users typically query on (exclude id, created_at, updated_at). For each column, include synonyms users might use.\n")
	sb.WriteString("5. **Alternative Names**: Synonyms or alternative names users might use to refer to this entity\n\n")

	sb.WriteString("## Examples\n\n")
	sb.WriteString("- `accounts` → **Account** - domain: \"customer\", key_columns: [{name: \"email\", synonyms: [\"e-mail\", \"mail\"]}, {name: \"name\", synonyms: [\"username\", \"full_name\"]}], alternative_names: [\"user\", \"member\"]\n")
	sb.WriteString("- `billing_activities` → **Billing Activity** - domain: \"billing\", key_columns: [{name: \"amount\", synonyms: [\"total\", \"price\"]}, {name: \"status\", synonyms: [\"state\"]}], alternative_names: [\"charge\", \"transaction\"]\n")
	sb.WriteString("- `reservations` → **Reservation** - domain: \"hospitality\", key_columns: [{name: \"check_in_date\", synonyms: [\"arrival\", \"start_date\"]}, {name: \"status\", synonyms: [\"state\"]}], alternative_names: [\"booking\", \"stay\"]\n\n")

	sb.WriteString("## Tables to Process\n\n")
	for _, entity := range entities {
		tableKey := fmt.Sprintf("%s.%s", entity.PrimarySchema, entity.PrimaryTable)
		cols := tableColumns[tableKey]
		sb.WriteString(fmt.Sprintf("- `%s` (columns: %s)\n", entity.PrimaryTable, strings.Join(cols, ", ")))
	}

	sb.WriteString("\n## Questions for Clarification\n\n")
	sb.WriteString("Additionally, identify any areas of uncertainty where user clarification would improve accuracy.\n")
	sb.WriteString("For each uncertainty, provide:\n")
	sb.WriteString("- **category**: terminology | enumeration | relationship | business_rules | temporal | data_quality\n")
	sb.WriteString("- **priority**: 1 (critical) | 2 (important) | 3 (nice-to-have)\n")
	sb.WriteString("- **question**: A clear question for the domain expert\n")
	sb.WriteString("- **context**: Relevant schema/data context\n\n")
	sb.WriteString("Examples of good questions:\n")
	sb.WriteString("- \"What does 'tik' mean in tiks_count?\" (terminology, priority 2)\n")
	sb.WriteString("- \"Is the users table meant to store customers, admins, or both?\" (business_rules, priority 1)\n")
	sb.WriteString("- \"Are multiple accounts allowed per email address?\" (business_rules, priority 2)\n\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("Respond with a JSON object containing an \"entities\" array and an optional \"questions\" array:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"entities\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"table_name\": \"accounts\",\n")
	sb.WriteString("      \"entity_name\": \"Account\",\n")
	sb.WriteString("      \"description\": \"A user account that can access the platform.\",\n")
	sb.WriteString("      \"domain\": \"customer\",\n")
	sb.WriteString("      \"key_columns\": [{\"name\": \"email\", \"synonyms\": [\"e-mail\", \"mail\"]}, {\"name\": \"name\", \"synonyms\": [\"username\"]}],\n")
	sb.WriteString("      \"alternative_names\": [\"user\", \"member\"]\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"questions\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"category\": \"terminology\",\n")
	sb.WriteString("      \"priority\": 2,\n")
	sb.WriteString("      \"question\": \"What does 'tik' mean in tiks_count?\",\n")
	sb.WriteString("      \"context\": \"Column accounts.tiks_count appears to track some kind of count but 'tik' is not a standard term.\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// entityEnrichmentResponse is the object-wrapped response from the LLM.
type entityEnrichmentResponse struct {
	Entities  []entityEnrichment      `json:"entities"`
	Questions []ontologyQuestionInput `json:"questions,omitempty"`
}

func (s *entityDiscoveryService) parseEntityEnrichmentResponse(content string) ([]entityEnrichment, []ontologyQuestionInput, error) {
	// Use the generic ParseJSONResponse helper to unwrap the object format
	response, err := llm.ParseJSONResponse[entityEnrichmentResponse](content)
	if err != nil {
		return nil, nil, fmt.Errorf("parse entity enrichment response: %w", err)
	}
	return response.Entities, response.Questions, nil
}

// testPrefixPatterns defines regex patterns for common test/sample table prefixes.
// These prefixes indicate tables that are likely test data, staging, or sample tables
// rather than production tables.
var testPrefixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^s\d+_`),    // s1_, s2_, s10_, etc. (sample tables)
	regexp.MustCompile(`^test_`),    // test_users, test_orders
	regexp.MustCompile(`^tmp_`),     // tmp_users, tmp_data
	regexp.MustCompile(`^temp_`),    // temp_users, temp_data
	regexp.MustCompile(`^staging_`), // staging_users, staging_orders
	regexp.MustCompile(`^dev_`),     // dev_users, dev_data
	regexp.MustCompile(`^sample_`),  // sample_users, sample_data
	regexp.MustCompile(`^demo_`),    // demo_users, demo_data
	regexp.MustCompile(`^backup_`),  // backup_users, backup_data
	regexp.MustCompile(`^old_`),     // old_users, old_data
	regexp.MustCompile(`^_`),        // _temp_table, _backup
	regexp.MustCompile(`^copy_of_`), // copy_of_users
	regexp.MustCompile(`^archive_`), // archive_users
}

// extractCoreConcept removes common test/sample prefixes from a table name
// to extract the core business concept. For example:
//   - "s1_users" → "users"
//   - "test_orders" → "orders"
//   - "staging_products" → "products"
//   - "users" → "users" (unchanged)
func extractCoreConcept(tableName string) string {
	name := tableName
	for _, pattern := range testPrefixPatterns {
		name = pattern.ReplaceAllString(name, "")
	}
	return strings.ToLower(name)
}

// hasTestPrefix returns true if the table name has a test/sample/staging prefix.
func hasTestPrefix(tableName string) bool {
	for _, pattern := range testPrefixPatterns {
		if pattern.MatchString(tableName) {
			return true
		}
	}
	return false
}

// groupSimilarTables groups tables by their core concept.
// Tables with test prefixes like "s1_users", "s2_users", "test_users" will all
// be grouped under the core concept "users" along with the real "users" table.
// This allows the system to identify that they represent the same business entity.
func groupSimilarTables(tables []*models.SchemaTable) map[string][]*models.SchemaTable {
	groups := make(map[string][]*models.SchemaTable)

	for _, t := range tables {
		concept := extractCoreConcept(t.TableName)
		groups[concept] = append(groups[concept], t)
	}
	return groups
}

// selectPrimaryTable selects the best table to represent an entity from a group.
// It prefers tables without test prefixes (the "real" table).
// If all tables have test prefixes, it returns the first one.
func selectPrimaryTable(tables []*models.SchemaTable) *models.SchemaTable {
	// Prefer tables without test prefixes
	for _, t := range tables {
		if !hasTestPrefix(t.TableName) {
			return t
		}
	}
	// Fallback to first table if all have test prefixes
	return tables[0]
}

// ValidateEnrichment checks that all entities have non-empty descriptions after enrichment.
// This catches any entities that failed to get enriched due to LLM response issues.
func (s *entityDiscoveryService) ValidateEnrichment(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	entities, err := s.entityRepo.GetByOntology(tenantCtx, ontologyID)
	if err != nil {
		return fmt.Errorf("list entities: %w", err)
	}

	var emptyDescriptions []string
	for _, e := range entities {
		if e.Description == "" {
			emptyDescriptions = append(emptyDescriptions, e.Name)
		}
	}

	if len(emptyDescriptions) > 0 {
		return fmt.Errorf("%d entities lack descriptions: %v", len(emptyDescriptions), emptyDescriptions)
	}

	return nil
}

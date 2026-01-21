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

// EntityDiscoveryService provides entity discovery operations for the DAG workflow.
// It contains the core algorithms for identifying and enriching entities from schema metadata.
type EntityDiscoveryService interface {
	// IdentifyEntitiesFromDDL discovers entities from DDL metadata (PK/unique constraints)
	IdentifyEntitiesFromDDL(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID) (int, []*models.SchemaTable, []*models.SchemaColumn, error)

	// EnrichEntitiesWithLLM uses an LLM to generate entity names and descriptions
	EnrichEntitiesWithLLM(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID, tables []*models.SchemaTable, columns []*models.SchemaColumn) error
}

type entityDiscoveryService struct {
	entityRepo       repositories.OntologyEntityRepository
	schemaRepo       repositories.SchemaRepository
	ontologyRepo     repositories.OntologyRepository
	conversationRepo repositories.ConversationRepository
	llmFactory       llm.LLMClientFactory
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewEntityDiscoveryService creates a new entity discovery service.
func NewEntityDiscoveryService(
	entityRepo repositories.OntologyEntityRepository,
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	conversationRepo repositories.ConversationRepository,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) EntityDiscoveryService {
	return &entityDiscoveryService{
		entityRepo:       entityRepo,
		schemaRepo:       schemaRepo,
		ontologyRepo:     ontologyRepo,
		conversationRepo: conversationRepo,
		llmFactory:       llmFactory,
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

	// Create entity records (one per table, using the best candidate)
	// Use table name as temporary name - LLM will enrich with proper names later
	entityCount := 0
	for _, c := range bestByTable {
		entity := &models.OntologyEntity{
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          c.tableName, // Temporary - will be enriched by LLM
			Description:   "",          // Will be filled by LLM
			PrimarySchema: c.schemaName,
			PrimaryTable:  c.tableName,
			PrimaryColumn: c.columnName,
		}

		if err := s.entityRepo.Create(tenantCtx, entity); err != nil {
			s.logger.Error("Failed to create entity",
				zap.String("table_name", c.tableName),
				zap.Error(err))
			return 0, nil, nil, fmt.Errorf("create entity for table %s: %w", c.tableName, err)
		}

		s.logger.Info("Entity created (pending LLM enrichment)",
			zap.String("entity_id", entity.ID.String()),
			zap.String("table_name", c.tableName),
			zap.String("primary_location", fmt.Sprintf("%s.%s.%s", c.schemaName, c.tableName, c.columnName)),
			zap.Float64("confidence", c.confidence),
			zap.String("reason", c.reason))

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

	// Build the prompt
	prompt := s.buildEntityEnrichmentPrompt(entities, tableColumns)
	systemMsg := s.entityEnrichmentSystemMessage()

	// Get LLM client - must use tenant context for config lookup
	llmClient, err := s.llmFactory.CreateForProject(tenantCtx, projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM
	result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	enrichments, err := s.parseEntityEnrichmentResponse(result.Content)
	if err != nil {
		s.logger.Error("Failed to parse entity enrichment response",
			zap.String("conversation_id", result.ConversationID.String()),
			zap.Error(err))

		// Record parse failure in LLM conversation for troubleshooting
		if s.conversationRepo != nil {
			errorMessage := fmt.Sprintf("parse_failure: %s", err.Error())
			if updateErr := s.conversationRepo.UpdateStatus(tenantCtx, result.ConversationID, models.LLMConversationStatusError, errorMessage); updateErr != nil {
				s.logger.Error("Failed to update conversation status",
					zap.String("conversation_id", result.ConversationID.String()),
					zap.Error(updateErr))
			}
		}
		return fmt.Errorf("entity enrichment parse failure: %w", err)
	}

	// Update entities with enriched names, descriptions, and new fields
	enrichmentByTable := make(map[string]entityEnrichment)
	for _, e := range enrichments {
		enrichmentByTable[e.TableName] = e
	}

	for _, entity := range entities {
		if enrichment, ok := enrichmentByTable[entity.PrimaryTable]; ok {
			entity.Name = enrichment.EntityName
			entity.Description = enrichment.Description
			entity.Domain = enrichment.Domain
			if err := s.entityRepo.Update(tenantCtx, entity); err != nil {
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
				if err := s.entityRepo.CreateKeyColumn(tenantCtx, keyCol); err != nil {
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
				if err := s.entityRepo.CreateAlias(tenantCtx, alias); err != nil {
					s.logger.Error("Failed to create entity alias",
						zap.String("entity_id", entity.ID.String()),
						zap.String("alias", altName),
						zap.Error(err))
					// Continue with other aliases
				}
			}
		}
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
) string {
	var sb strings.Builder

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

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("Respond with a JSON object containing an \"entities\" array:\n")
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
	sb.WriteString("    },\n")
	sb.WriteString("    ...\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// entityEnrichmentResponse is the object-wrapped response from the LLM.
type entityEnrichmentResponse struct {
	Entities []entityEnrichment `json:"entities"`
}

func (s *entityDiscoveryService) parseEntityEnrichmentResponse(content string) ([]entityEnrichment, error) {
	// Use the generic ParseJSONResponse helper to unwrap the object format
	response, err := llm.ParseJSONResponse[entityEnrichmentResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse entity enrichment response: %w", err)
	}
	return response.Entities, nil
}

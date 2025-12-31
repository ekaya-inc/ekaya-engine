package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// EntityDiscoveryTask uses an LLM to identify domain entities from candidate columns.
// It takes filtered candidates, excluded columns, graph components, and island tables
// as input and produces a structured list of entities with their occurrences and roles.
type EntityDiscoveryTask struct {
	workqueue.BaseTask
	entityRepo     repositories.SchemaEntityRepository
	schemaRepo     repositories.SchemaRepository
	llmFactory     llm.LLMClientFactory
	adapterFactory datasource.DatasourceAdapterFactory
	dsSvc          DatasourceService
	getTenantCtx   TenantContextFunc
	projectID      uuid.UUID
	workflowID     uuid.UUID
	ontologyID     uuid.UUID
	datasourceID   uuid.UUID
	candidates     []ColumnFilterResult
	excluded       []ColumnFilterResult
	components     []ConnectedComponent
	islands        []string
	statsMap       map[string]datasource.ColumnStats
	logger         *zap.Logger
}

// NewEntityDiscoveryTask creates a new entity discovery task.
func NewEntityDiscoveryTask(
	entityRepo repositories.SchemaEntityRepository,
	schemaRepo repositories.SchemaRepository,
	llmFactory llm.LLMClientFactory,
	adapterFactory datasource.DatasourceAdapterFactory,
	dsSvc DatasourceService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	ontologyID uuid.UUID,
	datasourceID uuid.UUID,
	candidates []ColumnFilterResult,
	excluded []ColumnFilterResult,
	components []ConnectedComponent,
	islands []string,
	statsMap map[string]datasource.ColumnStats,
	logger *zap.Logger,
) *EntityDiscoveryTask {
	return &EntityDiscoveryTask{
		BaseTask:       workqueue.NewBaseTask("Discover entities with LLM", true), // LLM task
		entityRepo:     entityRepo,
		schemaRepo:     schemaRepo,
		llmFactory:     llmFactory,
		adapterFactory: adapterFactory,
		dsSvc:          dsSvc,
		getTenantCtx:   getTenantCtx,
		projectID:      projectID,
		workflowID:     workflowID,
		ontologyID:     ontologyID,
		datasourceID:   datasourceID,
		candidates:     candidates,
		excluded:       excluded,
		components:     components,
		islands:        islands,
		statsMap:       statsMap,
		logger:         logger,
	}
}

// Execute implements workqueue.Task.
// Calls LLM to identify entities and persists them to the database.
func (t *EntityDiscoveryTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Add workflow context for conversation recording
	tenantCtx = llm.WithWorkflowID(tenantCtx, t.workflowID)

	if t.logger != nil {
		t.logger.Info("Starting entity discovery with LLM",
			zap.Int("candidate_columns", len(t.candidates)),
			zap.Int("excluded_columns", len(t.excluded)),
			zap.Int("connected_components", len(t.components)),
			zap.Int("island_tables", len(t.islands)))
	}

	// Get foreign keys to include in prompt
	fks, err := t.getForeignKeys(tenantCtx)
	if err != nil {
		return fmt.Errorf("get foreign keys: %w", err)
	}

	// Build LLM prompt
	prompt := t.buildPrompt(fks)
	systemMessage := t.buildSystemMessage()

	// Call LLM
	llmClient, err := t.llmFactory.CreateForProject(tenantCtx, t.projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}
	if llmClient == nil {
		return fmt.Errorf("LLM client is nil")
	}

	result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return fmt.Errorf("LLM entity discovery failed: %w", err)
	}
	if result == nil {
		return fmt.Errorf("LLM returned nil result")
	}

	if t.logger != nil {
		t.logger.Info("Entity discovery LLM call completed",
			zap.Int("prompt_tokens", result.PromptTokens),
			zap.Int("completion_tokens", result.CompletionTokens),
			zap.Int("total_tokens", result.TotalTokens))
	}

	// Parse LLM output
	output, err := t.parseEntityDiscoveryOutput(result.Content)
	if err != nil {
		return fmt.Errorf("parse LLM output: %w", err)
	}

	// Persist entities and occurrences to database
	if err := t.persistEntities(tenantCtx, output); err != nil {
		return fmt.Errorf("persist entities: %w", err)
	}

	if t.logger != nil {
		t.logger.Info("Entity discovery completed",
			zap.Int("entities_discovered", len(output.Entities)),
			zap.Int("total_occurrences", t.countTotalOccurrences(output.Entities)))
	}

	return nil
}

// getForeignKeys retrieves foreign key relationships for the datasource.
func (t *EntityDiscoveryTask) getForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	// Get datasource
	ds, err := t.dsSvc.Get(ctx, t.projectID, t.datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer adapter
	adapter, err := t.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, t.projectID, t.datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Check if datasource supports foreign keys
	if !adapter.SupportsForeignKeys() {
		if t.logger != nil {
			t.logger.Info("Datasource does not support foreign keys",
				zap.String("datasource_type", string(ds.DatasourceType)))
		}
		return []datasource.ForeignKeyMetadata{}, nil
	}

	// Discover foreign keys
	fks, err := adapter.DiscoverForeignKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover foreign keys: %w", err)
	}

	return fks, nil
}

// buildPrompt constructs the LLM prompt for entity discovery.
func (t *EntityDiscoveryTask) buildPrompt(fks []datasource.ForeignKeyMetadata) string {
	var b strings.Builder

	b.WriteString("# Entity Discovery Task\n\n")
	b.WriteString("You are analyzing a database schema to identify domain entities and their occurrences.\n\n")

	// Section 1: Candidate Columns
	b.WriteString("## Candidate Columns (Entity References)\n\n")
	b.WriteString("These columns were identified as potential entity references based on naming patterns, ")
	b.WriteString("distinct counts, and constraints:\n\n")
	for _, c := range t.candidates {
		b.WriteString(fmt.Sprintf("- `%s.%s.%s` (%s, %d distinct, PK=%v, Unique=%v) - %s\n",
			c.SchemaName, c.TableName, c.ColumnName, c.DataType, c.DistinctCount, c.IsPrimaryKey, c.IsUnique, c.Reason))
	}
	b.WriteString("\n")

	// Section 2: Existing Foreign Keys
	b.WriteString("## Existing Foreign Key Relationships\n\n")
	if len(fks) == 0 {
		b.WriteString("No foreign keys discovered (datasource may not support FKs or none are defined).\n\n")
	} else {
		for _, fk := range fks {
			b.WriteString(fmt.Sprintf("- `%s.%s.%s` → `%s.%s.%s`\n",
				fk.SourceSchema, fk.SourceTable, fk.SourceColumn,
				fk.TargetSchema, fk.TargetTable, fk.TargetColumn))
		}
		b.WriteString("\n")
	}

	// Section 3: Graph Connectivity
	b.WriteString("## Graph Connectivity Analysis\n\n")
	if len(t.components) > 0 {
		b.WriteString(fmt.Sprintf("Found %d connected components:\n\n", len(t.components)))
		for i, comp := range t.components {
			b.WriteString(fmt.Sprintf("**Component %d** (%d tables): %s\n\n",
				i+1, comp.Size, strings.Join(comp.Tables, ", ")))
		}
	} else {
		b.WriteString("No connected components found (no foreign keys).\n\n")
	}

	if len(t.islands) > 0 {
		b.WriteString(fmt.Sprintf("**Island tables** (%d tables with no FK connections): %s\n\n",
			len(t.islands), strings.Join(t.islands, ", ")))
	}

	// Section 4: Excluded Columns (Context)
	b.WriteString("## Excluded Columns (For Context Only)\n\n")
	b.WriteString("These columns were excluded as they appear to be attributes, not entity references:\n\n")
	for _, e := range t.excluded {
		b.WriteString(fmt.Sprintf("- `%s.%s.%s` - %s\n", e.SchemaName, e.TableName, e.ColumnName, e.Reason))
	}
	b.WriteString("\n")

	// Section 5: Task Instructions
	b.WriteString("## Your Task\n\n")
	b.WriteString("Identify domain entities (like `user`, `account`, `order`, `product`) and all their occurrences across tables.\n\n")
	b.WriteString("**Guidelines:**\n")
	b.WriteString("- Each entity should have a **primary location** (the main table where it's defined)\n")
	b.WriteString("- List all **occurrences** where this entity appears in other tables (foreign keys)\n")
	b.WriteString("- Assign **semantic roles** when appropriate (e.g., `visitor`, `host`, `owner`, `buyer`, `seller`)\n")
	b.WriteString("- Use `null` for the role when the occurrence is generic (no special role)\n")
	b.WriteString("- Do NOT include attribute columns (like `email`, `password`, `status`, `created_at`)\n")
	b.WriteString("- Focus on entity references (IDs, UUIDs, keys) that link tables together\n")
	b.WriteString("- For island tables, try to infer which entity they might relate to based on naming\n\n")

	b.WriteString("**Output Format:**\n")
	b.WriteString("Return a JSON object with an `entities` array. Each entity should have:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"entities\": [\n")
	b.WriteString("    {\n")
	b.WriteString("      \"name\": \"user\",\n")
	b.WriteString("      \"description\": \"A person who uses the system\",\n")
	b.WriteString("      \"primary_schema\": \"public\",\n")
	b.WriteString("      \"primary_table\": \"users\",\n")
	b.WriteString("      \"primary_column\": \"id\",\n")
	b.WriteString("      \"occurrences\": [\n")
	b.WriteString("        {\n")
	b.WriteString("          \"schema_name\": \"public\",\n")
	b.WriteString("          \"table_name\": \"orders\",\n")
	b.WriteString("          \"column_name\": \"user_id\",\n")
	b.WriteString("          \"role\": null\n")
	b.WriteString("        },\n")
	b.WriteString("        {\n")
	b.WriteString("          \"schema_name\": \"public\",\n")
	b.WriteString("          \"table_name\": \"visits\",\n")
	b.WriteString("          \"column_name\": \"visitor_id\",\n")
	b.WriteString("          \"role\": \"visitor\"\n")
	b.WriteString("        },\n")
	b.WriteString("        {\n")
	b.WriteString("          \"schema_name\": \"public\",\n")
	b.WriteString("          \"table_name\": \"visits\",\n")
	b.WriteString("          \"column_name\": \"host_id\",\n")
	b.WriteString("          \"role\": \"host\"\n")
	b.WriteString("        }\n")
	b.WriteString("      ]\n")
	b.WriteString("    }\n")
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")

	b.WriteString("Begin your analysis now.\n")

	return b.String()
}

// buildSystemMessage constructs the system message for the LLM.
func (t *EntityDiscoveryTask) buildSystemMessage() string {
	return "You are a database schema analysis expert. Your task is to identify domain entities " +
		"and their occurrences across a database schema. Focus on semantic understanding of the data model. " +
		"Return valid JSON only, with no additional text or explanation."
}

// EntityDiscoveryOutput represents the LLM's response.
type EntityDiscoveryOutput struct {
	Entities []DiscoveredEntity `json:"entities"`
}

// DiscoveredEntity represents a domain entity identified by the LLM.
type DiscoveredEntity struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	PrimarySchema string             `json:"primary_schema"`
	PrimaryTable  string             `json:"primary_table"`
	PrimaryColumn string             `json:"primary_column"`
	Occurrences   []EntityOccurrence `json:"occurrences"`
}

// EntityOccurrence represents a single occurrence of an entity in a table.
type EntityOccurrence struct {
	SchemaName string  `json:"schema_name"`
	TableName  string  `json:"table_name"`
	ColumnName string  `json:"column_name"`
	Role       *string `json:"role"`
}

// parseEntityDiscoveryOutput parses the LLM's JSON response.
func (t *EntityDiscoveryTask) parseEntityDiscoveryOutput(content string) (*EntityDiscoveryOutput, error) {
	// Extract JSON from response (LLM might wrap it in markdown code blocks)
	jsonStr := extractJSON(content)

	var output EntityDiscoveryOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		if t.logger != nil {
			t.logger.Error("Failed to parse entity discovery output",
				zap.Error(err),
				zap.String("content", content))
		}
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	// Validate output
	if len(output.Entities) == 0 {
		return nil, fmt.Errorf("LLM returned no entities")
	}

	for _, entity := range output.Entities {
		if entity.Name == "" {
			return nil, fmt.Errorf("entity missing name")
		}
		if entity.PrimaryTable == "" {
			return nil, fmt.Errorf("entity %s missing primary_table", entity.Name)
		}
		if entity.PrimaryColumn == "" {
			return nil, fmt.Errorf("entity %s missing primary_column", entity.Name)
		}
	}

	return &output, nil
}

// persistEntities saves discovered entities and their occurrences to the database.
func (t *EntityDiscoveryTask) persistEntities(ctx context.Context, output *EntityDiscoveryOutput) error {
	for _, discoveredEntity := range output.Entities {
		// Create entity record
		entity := &models.SchemaEntity{
			ProjectID:     t.projectID,
			OntologyID:    t.ontologyID,
			Name:          discoveredEntity.Name,
			Description:   discoveredEntity.Description,
			PrimarySchema: discoveredEntity.PrimarySchema,
			PrimaryTable:  discoveredEntity.PrimaryTable,
			PrimaryColumn: discoveredEntity.PrimaryColumn,
		}

		if err := t.entityRepo.Create(ctx, entity); err != nil {
			if t.logger != nil {
				t.logger.Error("Failed to create entity",
					zap.String("entity_name", discoveredEntity.Name),
					zap.Error(err))
			}
			return fmt.Errorf("create entity %s: %w", discoveredEntity.Name, err)
		}

		if t.logger != nil {
			t.logger.Info("Entity created",
				zap.String("entity_id", entity.ID.String()),
				zap.String("entity_name", entity.Name),
				zap.String("primary_location", fmt.Sprintf("%s.%s.%s", entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn)))
		}

		// Create occurrence records
		for _, occ := range discoveredEntity.Occurrences {
			occurrence := &models.SchemaEntityOccurrence{
				EntityID:   entity.ID,
				SchemaName: occ.SchemaName,
				TableName:  occ.TableName,
				ColumnName: occ.ColumnName,
				Role:       occ.Role,
				Confidence: 1.0, // Default confidence from LLM discovery
			}

			if err := t.entityRepo.CreateOccurrence(ctx, occurrence); err != nil {
				if t.logger != nil {
					t.logger.Error("Failed to create occurrence",
						zap.String("entity_name", entity.Name),
						zap.String("location", fmt.Sprintf("%s.%s.%s", occ.SchemaName, occ.TableName, occ.ColumnName)),
						zap.Error(err))
				}
				return fmt.Errorf("create occurrence for entity %s: %w", entity.Name, err)
			}

			if t.logger != nil {
				roleStr := "null"
				if occ.Role != nil {
					roleStr = *occ.Role
				}
				t.logger.Info("  Occurrence created",
					zap.String("location", fmt.Sprintf("%s.%s.%s", occ.SchemaName, occ.TableName, occ.ColumnName)),
					zap.String("role", roleStr))
			}
		}
	}

	return nil
}

// countTotalOccurrences counts the total number of occurrences across all entities.
func (t *EntityDiscoveryTask) countTotalOccurrences(entities []DiscoveredEntity) int {
	total := 0
	for _, e := range entities {
		total += len(e.Occurrences)
	}
	return total
}

// extractJSON extracts JSON from text that might be wrapped in markdown code blocks
// or prefixed with LLM reasoning tags like <think>...</think>.
func extractJSON(text string) string {
	text = strings.TrimSpace(text)

	// Remove <think>...</think> tags (LLM reasoning mode output)
	if idx := strings.Index(text, "</think>"); idx != -1 {
		text = strings.TrimSpace(text[idx+len("</think>"):])
	}

	// Remove markdown code blocks if present
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text
}

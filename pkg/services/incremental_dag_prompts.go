package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// LLM response structures for incremental enrichment

// singleEntityEnrichment holds LLM-generated metadata for a single entity.
type singleEntityEnrichment struct {
	EntityName       string                `json:"entity_name"`
	Description      string                `json:"description"`
	Domain           string                `json:"domain"`
	KeyColumns       []keyColumnEnrichment `json:"key_columns,omitempty"`
	AlternativeNames []string              `json:"alternative_names,omitempty"`
}

// singleEntityResponse wraps the LLM response for single entity enrichment.
type singleEntityResponse struct {
	Entity singleEntityEnrichment `json:"entity"`
}

// singleColumnEnrichment holds LLM-generated metadata for a single column.
type singleColumnEnrichment struct {
	Description   string             `json:"description"`
	SemanticType  string             `json:"semantic_type"`
	Role          string             `json:"role"`
	Synonyms      []string           `json:"synonyms,omitempty"`
	EnumValues    []models.EnumValue `json:"enum_values,omitempty"`
	FKAssociation *string            `json:"fk_association,omitempty"`
}

// singleColumnResponse wraps the LLM response for single column enrichment.
type singleColumnResponse struct {
	Column singleColumnEnrichment `json:"column"`
}

// incrementalRelEnrichment holds LLM-generated metadata for a relationship.
type incrementalRelEnrichment struct {
	Description string `json:"description"`
	Association string `json:"association"`
	Label       string `json:"label,omitempty"`
}

// relationshipResponse wraps the LLM response for relationship enrichment.
type relationshipResponse struct {
	Relationship incrementalRelEnrichment `json:"relationship"`
}

// enrichEntityWithLLM generates entity metadata for a single table using LLM.
func (s *incrementalDAGService) enrichEntityWithLLM(
	ctx context.Context,
	llmClient llm.LLMClient,
	tableName string,
	columns []*models.SchemaColumn,
) (*singleEntityEnrichment, error) {
	systemMsg := `You are a data modeling expert. Your task is to analyze a single database table and provide semantic metadata that helps AI agents understand the business entity it represents.

Provide clear, concise business descriptions. Be specific about what the entity represents in the domain.`

	prompt := s.buildSingleEntityPrompt(tableName, columns)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	response, err := llm.ParseJSONResponse[singleEntityResponse](result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &response.Entity, nil
}

func (s *incrementalDAGService) buildSingleEntityPrompt(tableName string, columns []*models.SchemaColumn) string {
	var sb strings.Builder

	sb.WriteString("# Analyze This Table\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** `%s`\n\n", tableName))

	if len(columns) > 0 {
		sb.WriteString("**Columns:**\n")
		sb.WriteString("| Name | Type | PK | Nullable |\n")
		sb.WriteString("|------|------|----|---------|\n")
		for _, col := range columns {
			pk := ""
			if col.IsPrimaryKey {
				pk = "✓"
			}
			nullable := "yes"
			if !col.IsNullable {
				nullable = "no"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				col.ColumnName, col.DataType, pk, nullable))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Task\n\n")
	sb.WriteString("Provide semantic metadata for this entity:\n\n")
	sb.WriteString("1. **Entity Name**: A clean, singular, Title Case name (e.g., \"users\" → \"User\")\n")
	sb.WriteString("2. **Description**: A brief (1-2 sentence) business description\n")
	sb.WriteString("3. **Domain**: A lowercase business domain (e.g., \"billing\", \"customer\", \"inventory\")\n")
	sb.WriteString("4. **Key Columns**: 2-3 important business columns users typically query (exclude id, timestamps)\n")
	sb.WriteString("5. **Alternative Names**: Synonyms users might use to refer to this entity\n\n")

	sb.WriteString("## Response Format (JSON)\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"entity\": {\n")
	sb.WriteString("    \"entity_name\": \"User\",\n")
	sb.WriteString("    \"description\": \"A user account that can access the platform.\",\n")
	sb.WriteString("    \"domain\": \"customer\",\n")
	sb.WriteString("    \"key_columns\": [{\"name\": \"email\", \"synonyms\": [\"e-mail\", \"mail\"]}],\n")
	sb.WriteString("    \"alternative_names\": [\"account\", \"member\"]\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// enrichColumnWithLLM generates column metadata using LLM.
func (s *incrementalDAGService) enrichColumnWithLLM(
	ctx context.Context,
	llmClient llm.LLMClient,
	entity *models.OntologyEntity,
	column *models.SchemaColumn,
) (*singleColumnEnrichment, error) {
	if column == nil {
		return nil, fmt.Errorf("column info is required")
	}

	systemMsg := `You are a database schema expert. Your task is to analyze a single database column and provide semantic metadata that helps AI agents write accurate SQL queries.

Be concise and focus on the business meaning of the column.`

	prompt := s.buildSingleColumnPrompt(entity, column)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	response, err := llm.ParseJSONResponse[singleColumnResponse](result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &response.Column, nil
}

func (s *incrementalDAGService) buildSingleColumnPrompt(entity *models.OntologyEntity, column *models.SchemaColumn) string {
	var sb strings.Builder

	sb.WriteString("# Analyze This Column\n\n")

	// Entity context if available
	if entity != nil {
		sb.WriteString(fmt.Sprintf("**Table:** `%s` (Entity: \"%s\"", entity.PrimaryTable, entity.Name))
		if entity.Description != "" {
			sb.WriteString(fmt.Sprintf(" - %s", entity.Description))
		}
		sb.WriteString(")\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("**Table:** `%s`\n\n", column.ColumnName))
	}

	sb.WriteString("**Column:**\n")
	sb.WriteString(fmt.Sprintf("- Name: `%s`\n", column.ColumnName))
	sb.WriteString(fmt.Sprintf("- Type: `%s`\n", column.DataType))
	if column.IsPrimaryKey {
		sb.WriteString("- Primary Key: yes\n")
	}
	if !column.IsNullable {
		sb.WriteString("- Required: yes\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## Task\n\n")
	sb.WriteString("Provide semantic metadata for this column:\n\n")
	sb.WriteString("1. **description**: 1 sentence explaining business meaning\n")
	sb.WriteString("2. **semantic_type**: identifier, currency_cents, timestamp_utc, status, count, email, etc.\n")
	sb.WriteString("3. **role**: dimension (for grouping) | measure (for aggregation) | identifier | attribute\n")
	sb.WriteString("4. **synonyms**: alternative names users might use (optional)\n")
	sb.WriteString("5. **enum_values**: if status/type column, list possible values (optional)\n\n")

	sb.WriteString("## Response Format (JSON)\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"column\": {\n")
	sb.WriteString("    \"description\": \"The current status of the record\",\n")
	sb.WriteString("    \"semantic_type\": \"status\",\n")
	sb.WriteString("    \"role\": \"dimension\",\n")
	sb.WriteString("    \"synonyms\": [\"state\"],\n")
	sb.WriteString("    \"enum_values\": [{\"value\": \"active\", \"description\": \"Normal active state\"}]\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// enrichRelationshipWithLLM generates relationship metadata using LLM.
func (s *incrementalDAGService) enrichRelationshipWithLLM(
	ctx context.Context,
	llmClient llm.LLMClient,
	sourceEntity *models.OntologyEntity,
	targetEntity *models.OntologyEntity,
	sourceColumn string,
) (*incrementalRelEnrichment, error) {
	systemMsg := `You are a data modeling expert. Your task is to describe the business relationship between two entities based on a foreign key column.

Focus on the business meaning of the relationship, not the technical implementation.`

	prompt := s.buildRelationshipPrompt(sourceEntity, targetEntity, sourceColumn)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	response, err := llm.ParseJSONResponse[relationshipResponse](result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &response.Relationship, nil
}

func (s *incrementalDAGService) buildRelationshipPrompt(
	sourceEntity *models.OntologyEntity,
	targetEntity *models.OntologyEntity,
	sourceColumn string,
) string {
	var sb strings.Builder

	sb.WriteString("# Analyze This Relationship\n\n")

	sb.WriteString("**Source Entity:**\n")
	sb.WriteString(fmt.Sprintf("- Name: %s\n", sourceEntity.Name))
	sb.WriteString(fmt.Sprintf("- Table: %s\n", sourceEntity.PrimaryTable))
	if sourceEntity.Description != "" {
		sb.WriteString(fmt.Sprintf("- Description: %s\n", sourceEntity.Description))
	}
	sb.WriteString("\n")

	sb.WriteString("**Target Entity:**\n")
	sb.WriteString(fmt.Sprintf("- Name: %s\n", targetEntity.Name))
	sb.WriteString(fmt.Sprintf("- Table: %s\n", targetEntity.PrimaryTable))
	if targetEntity.Description != "" {
		sb.WriteString(fmt.Sprintf("- Description: %s\n", targetEntity.Description))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("**FK Column:** `%s.%s` → `%s`\n\n", sourceEntity.PrimaryTable, sourceColumn, targetEntity.PrimaryTable))

	sb.WriteString("## Task\n\n")
	sb.WriteString("Describe the business relationship:\n\n")
	sb.WriteString("1. **description**: A sentence describing what this relationship means in business terms\n")
	sb.WriteString("2. **association**: A short verb/phrase describing the relationship (e.g., \"owns\", \"created_by\", \"assigned_to\")\n")
	sb.WriteString("3. **label**: Optional short label for UI display\n\n")

	sb.WriteString("## Examples\n")
	sb.WriteString("- Order → User via `user_id`: association=\"placed_by\", description=\"The user who placed this order\"\n")
	sb.WriteString("- Task → User via `assignee_id`: association=\"assigned_to\", description=\"The user assigned to work on this task\"\n")
	sb.WriteString("- Comment → Post via `post_id`: association=\"belongs_to\", description=\"The post this comment is attached to\"\n\n")

	sb.WriteString("## Response Format (JSON)\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"relationship\": {\n")
	sb.WriteString("    \"description\": \"The user who owns this account\",\n")
	sb.WriteString("    \"association\": \"owned_by\",\n")
	sb.WriteString("    \"label\": \"owner\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

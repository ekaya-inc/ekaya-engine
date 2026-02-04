package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// LLM response structures for incremental enrichment

// keyColumnEnrichment holds enrichment data for a key column.
type keyColumnEnrichment struct {
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms,omitempty"`
}

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
// Note: entity parameter is deprecated and ignored (kept for API compatibility).
func (s *incrementalDAGService) enrichColumnWithLLM(
	ctx context.Context,
	llmClient llm.LLMClient,
	_ any, // entity parameter removed for v1.0
	column *models.SchemaColumn,
) (*singleColumnEnrichment, error) {
	if column == nil {
		return nil, fmt.Errorf("column info is required")
	}

	systemMsg := `You are a database schema expert. Your task is to analyze a single database column and provide semantic metadata that helps AI agents write accurate SQL queries.

Be concise and focus on the business meaning of the column.`

	prompt := s.buildSingleColumnPrompt(column)

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

func (s *incrementalDAGService) buildSingleColumnPrompt(column *models.SchemaColumn) string {
	var sb strings.Builder

	sb.WriteString("# Analyze This Column\n\n")

	// Get table name from column if available
	tableName := "unknown"
	if column != nil {
		// Column name is used as fallback if table info not available
		tableName = column.ColumnName
	}
	sb.WriteString(fmt.Sprintf("**Table:** `%s`\n\n", tableName))

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

// Note: enrichRelationshipWithLLM and buildRelationshipPrompt have been removed
// for v1.0 simplification. Entity-based relationship enrichment is no longer supported.
// Relationships are now stored at the schema level (SchemaRelationship).

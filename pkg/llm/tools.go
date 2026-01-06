// Package llm provides OpenAI-compatible LLM client functionality.
package llm

// ToolDefinition defines a tool that can be called by the LLM.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ParameterProperty defines a parameter property in JSON Schema format.
type ParameterProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// NewToolDefinition creates a new tool definition with standard JSON Schema parameters.
func NewToolDefinition(name, description string, properties map[string]ParameterProperty, required []string) ToolDefinition {
	props := make(map[string]any)
	for k, v := range properties {
		props[k] = map[string]any{
			"type":        v.Type,
			"description": v.Description,
		}
		if len(v.Enum) > 0 {
			props[k].(map[string]any)["enum"] = v.Enum
		}
	}

	return ToolDefinition{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": props,
			"required":   required,
		},
	}
}

// GetOntologyChatTools returns the tool definitions for ontology chat.
func GetOntologyChatTools() []ToolDefinition {
	return []ToolDefinition{
		NewToolDefinition(
			"query_column_values",
			"Query sample values from a specific column in the database to understand its contents",
			map[string]ParameterProperty{
				"table_name": {
					Type:        "string",
					Description: "The name of the table to query",
				},
				"column_name": {
					Type:        "string",
					Description: "The name of the column to get sample values from",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of sample values to return (default 10)",
				},
			},
			[]string{"table_name", "column_name"},
		),
		NewToolDefinition(
			"query_schema_metadata",
			"Get detailed metadata about tables and columns in the schema",
			map[string]ParameterProperty{
				"table_name": {
					Type:        "string",
					Description: "Optional: specific table name to get metadata for. If not provided, returns all tables.",
				},
			},
			[]string{},
		),
		NewToolDefinition(
			"store_knowledge",
			"Store a piece of business knowledge or terminology learned from the conversation",
			map[string]ParameterProperty{
				"fact_type": {
					Type:        "string",
					Description: "The type of knowledge being stored",
					Enum:        []string{"terminology", "business_rule", "data_relationship", "constraint", "context"},
				},
				"key": {
					Type:        "string",
					Description: "A short key or identifier for this knowledge (e.g., 'SKU', 'order_status_values')",
				},
				"value": {
					Type:        "string",
					Description: "The actual knowledge or definition",
				},
				"context": {
					Type:        "string",
					Description: "Optional additional context about where this knowledge applies",
				},
			},
			[]string{"fact_type", "key", "value"},
		),
		NewToolDefinition(
			"update_entity",
			"Update the business description or properties of a table/entity in the ontology",
			map[string]ParameterProperty{
				"table_name": {
					Type:        "string",
					Description: "The technical name of the table to update",
				},
				"business_name": {
					Type:        "string",
					Description: "The business-friendly name for this entity",
				},
				"description": {
					Type:        "string",
					Description: "A business description of what this entity represents",
				},
				"domain": {
					Type:        "string",
					Description: "The business domain this entity belongs to",
				},
				"synonyms": {
					Type:        "array",
					Description: "Alternative names or terms used to refer to this entity",
				},
			},
			[]string{"table_name"},
		),
		NewToolDefinition(
			"update_column",
			"Update the business description or properties of a column in the ontology",
			map[string]ParameterProperty{
				"table_name": {
					Type:        "string",
					Description: "The name of the table containing the column",
				},
				"column_name": {
					Type:        "string",
					Description: "The technical name of the column to update",
				},
				"business_name": {
					Type:        "string",
					Description: "The business-friendly name for this column",
				},
				"description": {
					Type:        "string",
					Description: "A business description of what this column represents",
				},
				"semantic_type": {
					Type:        "string",
					Description: "The semantic type of this column",
					Enum:        []string{"identifier", "name", "description", "amount", "quantity", "date", "timestamp", "status", "flag", "code", "reference", "other"},
				},
			},
			[]string{"table_name", "column_name"},
		),
		NewToolDefinition(
			"create_domain_entity",
			"Create a new domain entity that represents a business concept spanning one or more tables",
			map[string]ParameterProperty{
				"name": {
					Type:        "string",
					Description: "The name of the domain entity (e.g., 'user', 'campaign', 'order')",
				},
				"description": {
					Type:        "string",
					Description: "A description of what this entity represents in business terms",
				},
				"primary_table": {
					Type:        "string",
					Description: "The main table where this entity is defined (e.g., 'users' for 'user' entity)",
				},
				"primary_column": {
					Type:        "string",
					Description: "The primary column that identifies this entity (e.g., 'id', 'user_id')",
				},
			},
			[]string{"name", "description", "primary_table", "primary_column"},
		),
		NewToolDefinition(
			"create_entity_relationship",
			"Create a relationship between two domain entities based on how they connect in the data",
			map[string]ParameterProperty{
				"source_entity": {
					Type:        "string",
					Description: "The name of the source domain entity",
				},
				"target_entity": {
					Type:        "string",
					Description: "The name of the target domain entity",
				},
				"source_table": {
					Type:        "string",
					Description: "The table containing the source reference",
				},
				"source_column": {
					Type:        "string",
					Description: "The column that references the target entity",
				},
				"target_table": {
					Type:        "string",
					Description: "The table containing the target entity",
				},
				"target_column": {
					Type:        "string",
					Description: "The column being referenced (usually the primary key)",
				},
				"description": {
					Type:        "string",
					Description: "Optional description of the relationship (e.g., 'Users can place multiple orders')",
				},
			},
			[]string{"source_entity", "target_entity", "source_table", "source_column", "target_table", "target_column"},
		),
	}
}

// GetQuestionAnswererTools returns tools for the question answering service.
// This is a subset focused on information gathering and knowledge storage.
func GetQuestionAnswererTools() []ToolDefinition {
	return []ToolDefinition{
		NewToolDefinition(
			"query_column_values",
			"Query sample values from a specific column to help answer questions about data",
			map[string]ParameterProperty{
				"table_name": {
					Type:        "string",
					Description: "The name of the table to query",
				},
				"column_name": {
					Type:        "string",
					Description: "The name of the column to get sample values from",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of sample values to return (default 10)",
				},
			},
			[]string{"table_name", "column_name"},
		),
		NewToolDefinition(
			"query_schema_metadata",
			"Get detailed metadata about tables and columns",
			map[string]ParameterProperty{
				"table_name": {
					Type:        "string",
					Description: "Optional: specific table name to get metadata for",
				},
			},
			[]string{},
		),
		NewToolDefinition(
			"store_knowledge",
			"Store business knowledge learned from the user's answer",
			map[string]ParameterProperty{
				"fact_type": {
					Type:        "string",
					Description: "The type of knowledge being stored",
					Enum:        []string{"terminology", "business_rule", "data_relationship", "constraint", "context"},
				},
				"key": {
					Type:        "string",
					Description: "A short key or identifier for this knowledge",
				},
				"value": {
					Type:        "string",
					Description: "The actual knowledge or definition",
				},
				"context": {
					Type:        "string",
					Description: "Optional additional context",
				},
			},
			[]string{"fact_type", "key", "value"},
		),
	}
}

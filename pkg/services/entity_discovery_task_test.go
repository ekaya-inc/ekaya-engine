package services

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEntityDiscoveryOutput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectCount int
	}{
		{
			name: "valid JSON",
			input: `{
				"entities": [
					{
						"name": "user",
						"description": "A person who uses the system",
						"primary_schema": "public",
						"primary_table": "users",
						"primary_column": "id",
						"occurrences": [
							{
								"schema_name": "public",
								"table_name": "orders",
								"column_name": "user_id",
								"role": null
							}
						]
					}
				]
			}`,
			expectError: false,
			expectCount: 1,
		},
		{
			name: "valid JSON with markdown code blocks",
			input: "```json\n" + `{
				"entities": [
					{
						"name": "user",
						"description": "A person who uses the system",
						"primary_schema": "public",
						"primary_table": "users",
						"primary_column": "id",
						"occurrences": []
					}
				]
			}` + "\n```",
			expectError: false,
			expectCount: 1,
		},
		{
			name: "empty entities array",
			input: `{
				"entities": []
			}`,
			expectError: true,
			expectCount: 0,
		},
		{
			name:        "invalid JSON",
			input:       `{"entities": [{"name": "user"`,
			expectError: true,
			expectCount: 0,
		},
		{
			name: "missing required fields",
			input: `{
				"entities": [
					{
						"name": "user",
						"description": "A person"
					}
				]
			}`,
			expectError: true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &EntityDiscoveryTask{}
			output, err := task.parseEntityDiscoveryOutput(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, output.Entities, tt.expectCount)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with markdown code blocks",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with generic code blocks",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with whitespace",
			input:    "  \n  {\"key\": \"value\"}  \n  ",
			expected: `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildPrompt(t *testing.T) {
	task := &EntityDiscoveryTask{
		candidates: []ColumnFilterResult{
			{
				SchemaName:    "public",
				TableName:     "users",
				ColumnName:    "id",
				DataType:      "bigint",
				DistinctCount: 100,
				IsPrimaryKey:  true,
				IsUnique:      false,
				IsCandidate:   true,
				Reason:        "primary key",
			},
			{
				SchemaName:    "public",
				TableName:     "orders",
				ColumnName:    "user_id",
				DataType:      "bigint",
				DistinctCount: 95,
				IsPrimaryKey:  false,
				IsUnique:      false,
				IsCandidate:   true,
				Reason:        "entity reference name pattern",
			},
		},
		excluded: []ColumnFilterResult{
			{
				SchemaName:   "public",
				TableName:    "orders",
				ColumnName:   "created_at",
				DataType:     "timestamp",
				IsCandidate:  false,
				Reason:       "excluded type (timestamp)",
			},
		},
		components: []ConnectedComponent{
			{
				Tables: []string{"public.users", "public.orders"},
				Size:   2,
			},
		},
		islands: []string{"public.audit_logs"},
	}

	prompt := task.buildPrompt(nil)

	// Check that prompt contains expected sections
	assert.Contains(t, prompt, "# Entity Discovery Task")
	assert.Contains(t, prompt, "## Candidate Columns (Entity References)")
	assert.Contains(t, prompt, "public.users.id")
	assert.Contains(t, prompt, "public.orders.user_id")
	assert.Contains(t, prompt, "## Existing Foreign Key Relationships")
	assert.Contains(t, prompt, "## Graph Connectivity Analysis")
	assert.Contains(t, prompt, "Component 1")
	assert.Contains(t, prompt, "Island tables")
	assert.Contains(t, prompt, "public.audit_logs")
	assert.Contains(t, prompt, "## Excluded Columns (For Context Only)")
	assert.Contains(t, prompt, "created_at")
	assert.Contains(t, prompt, "## Your Task")
	assert.Contains(t, prompt, "Output Format:")
}

func TestCountTotalOccurrences(t *testing.T) {
	task := &EntityDiscoveryTask{}

	entities := []DiscoveredEntity{
		{
			Name: "user",
			Occurrences: []EntityOccurrence{
				{TableName: "orders", ColumnName: "user_id"},
				{TableName: "visits", ColumnName: "visitor_id"},
			},
		},
		{
			Name: "product",
			Occurrences: []EntityOccurrence{
				{TableName: "order_items", ColumnName: "product_id"},
			},
		},
	}

	count := task.countTotalOccurrences(entities)
	assert.Equal(t, 3, count)
}

func TestEntityDiscoveryOutputValidation(t *testing.T) {
	tests := []struct {
		name        string
		entity      DiscoveredEntity
		expectValid bool
	}{
		{
			name: "valid entity",
			entity: DiscoveredEntity{
				Name:          "user",
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
				Occurrences:   []EntityOccurrence{},
			},
			expectValid: true,
		},
		{
			name: "missing name",
			entity: DiscoveredEntity{
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
			expectValid: false,
		},
		{
			name: "missing primary_table",
			entity: DiscoveredEntity{
				Name:          "user",
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryColumn: "id",
			},
			expectValid: false,
		},
		{
			name: "missing primary_column",
			entity: DiscoveredEntity{
				Name:          "user",
				Description:   "A person",
				PrimarySchema: "public",
				PrimaryTable:  "users",
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := EntityDiscoveryOutput{
				Entities: []DiscoveredEntity{tt.entity},
			}

			// Serialize to JSON and back to test validation
			jsonBytes, err := json.Marshal(output)
			require.NoError(t, err)

			task := &EntityDiscoveryTask{}
			_, err = task.parseEntityDiscoveryOutput(string(jsonBytes))

			if tt.expectValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

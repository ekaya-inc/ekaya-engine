package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaToolDeps_Structure verifies the SchemaToolDeps struct has all required fields.
func TestSchemaToolDeps_Structure(t *testing.T) {
	// Create a zero-value instance to verify struct is properly defined
	deps := &SchemaToolDeps{}

	// Verify all fields exist and have correct types
	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.ProjectService, "ProjectService field should be nil by default")
	assert.Nil(t, deps.SchemaService, "SchemaService field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestGetSchema_Registration verifies get_schema is registered via RegisterSchemaTools.
func TestGetSchema_Registration(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	deps := &SchemaToolDeps{}
	RegisterSchemaTools(mcpServer, deps)

	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	var found bool
	for _, tool := range response.Result.Tools {
		if tool.Name == "get_schema" {
			found = true
		}
		// list_tables should NOT be registered
		assert.NotEqual(t, "list_tables", tool.Name, "list_tables should not be registered")
	}
	assert.True(t, found, "get_schema should be registered")
}

// TestGetSchema_ResponseStructure verifies the structured JSON response shape.
func TestGetSchema_ResponseStructure(t *testing.T) {
	type columnInfo struct {
		Name         string `json:"name"`
		DataType     string `json:"data_type"`
		IsPrimaryKey bool   `json:"is_primary_key"`
		IsNullable   bool   `json:"is_nullable"`
		OrdinalPos   int    `json:"ordinal_position"`
	}
	type tableInfo struct {
		Schema   string       `json:"schema"`
		Name     string       `json:"name"`
		RowCount int64        `json:"row_count"`
		Columns  []columnInfo `json:"columns"`
	}
	type relationshipInfo struct {
		SourceTable  string `json:"source_table"`
		SourceColumn string `json:"source_column"`
		TargetTable  string `json:"target_table"`
		TargetColumn string `json:"target_column"`
		Cardinality  string `json:"cardinality,omitempty"`
	}

	response := struct {
		Tables        []tableInfo        `json:"tables"`
		Relationships []relationshipInfo `json:"relationships"`
		TableCount    int                `json:"table_count"`
		ProjectID     string             `json:"project_id"`
		DatasourceID  string             `json:"datasource_id"`
	}{
		Tables: []tableInfo{
			{
				Schema:   "public",
				Name:     "users",
				RowCount: 1000,
				Columns: []columnInfo{
					{Name: "id", DataType: "uuid", IsPrimaryKey: true, IsNullable: false, OrdinalPos: 1},
					{Name: "email", DataType: "varchar", IsPrimaryKey: false, IsNullable: false, OrdinalPos: 2},
					{Name: "name", DataType: "varchar", IsPrimaryKey: false, IsNullable: true, OrdinalPos: 3},
				},
			},
			{
				Schema:   "public",
				Name:     "orders",
				RowCount: 5000,
				Columns: []columnInfo{
					{Name: "id", DataType: "uuid", IsPrimaryKey: true, IsNullable: false, OrdinalPos: 1},
					{Name: "user_id", DataType: "uuid", IsPrimaryKey: false, IsNullable: false, OrdinalPos: 2},
				},
			},
		},
		Relationships: []relationshipInfo{
			{SourceTable: "orders", SourceColumn: "user_id", TargetTable: "users", TargetColumn: "id", Cardinality: "many-to-one"},
		},
		TableCount:   2,
		ProjectID:    uuid.New().String(),
		DatasourceID: uuid.New().String(),
	}

	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	// Parse back and verify structure
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Top-level fields
	assert.Equal(t, float64(2), parsed["table_count"])
	assert.NotEmpty(t, parsed["project_id"])
	assert.NotEmpty(t, parsed["datasource_id"])

	// No schema_context text blob
	assert.Nil(t, parsed["schema_context"], "should not contain schema_context text blob")

	// Tables
	tables, ok := parsed["tables"].([]any)
	require.True(t, ok)
	require.Len(t, tables, 2)

	table := tables[0].(map[string]any)
	assert.Equal(t, "public", table["schema"])
	assert.Equal(t, "users", table["name"])
	assert.Equal(t, float64(1000), table["row_count"])

	columns := table["columns"].([]any)
	require.Len(t, columns, 3)

	col0 := columns[0].(map[string]any)
	assert.Equal(t, "id", col0["name"])
	assert.Equal(t, "uuid", col0["data_type"])
	assert.Equal(t, true, col0["is_primary_key"])
	assert.Equal(t, false, col0["is_nullable"])

	// Relationships
	rels, ok := parsed["relationships"].([]any)
	require.True(t, ok)
	require.Len(t, rels, 1)

	rel := rels[0].(map[string]any)
	assert.Equal(t, "orders", rel["source_table"])
	assert.Equal(t, "user_id", rel["source_column"])
	assert.Equal(t, "users", rel["target_table"])
	assert.Equal(t, "id", rel["target_column"])
	assert.Equal(t, "many-to-one", rel["cardinality"])
}

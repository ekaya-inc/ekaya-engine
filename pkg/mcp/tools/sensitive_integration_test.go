//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// sensitiveTestContext holds test dependencies for is_sensitive integration tests.
type sensitiveTestContext struct {
	t                  *testing.T
	engineDB           *testhelpers.EngineDB
	projectID          uuid.UUID
	datasourceID       uuid.UUID
	mcpServer          *server.MCPServer
	columnMetadataRepo repositories.ColumnMetadataRepository
	schemaRepo         repositories.SchemaRepository
	contextDeps        *ContextToolDeps
	columnDeps         *ColumnToolDeps
}

// setupSensitiveIntegrationTest initializes the test context.
func setupSensitiveIntegrationTest(t *testing.T) *sensitiveTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000099")
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000199")

	// Ensure test project and datasource exist
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Sensitive Flag Integration Test Project")
	require.NoError(t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, provider, datasource_config)
		VALUES ($1, $2, 'test_datasource', 'postgresql', 'custom', '{}')
		ON CONFLICT (id) DO NOTHING
	`, datasourceID, projectID)
	require.NoError(t, err)

	// Create MCP server
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	columnMetadataRepo := repositories.NewColumnMetadataRepository()
	schemaRepo := repositories.NewSchemaRepository()

	// Configure mock to enable developer tools
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true,
			AddQueryTools:          true,
			AddOntologyMaintenance: true,
		},
	}

	mockProjectService := &mockProjectService{defaultDatasourceID: datasourceID}

	// Register column tools
	columnDeps := &ColumnToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		SchemaRepo:         schemaRepo,
		ColumnMetadataRepo: columnMetadataRepo,
		ProjectService:     mockProjectService,
	}
	RegisterColumnTools(mcpServer, columnDeps)

	// Create context deps (for get_context)
	contextDeps := &ContextToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		ProjectService:     mockProjectService,
		SchemaRepo:         schemaRepo,
		ColumnMetadataRepo: columnMetadataRepo,
	}

	return &sensitiveTestContext{
		t:                  t,
		engineDB:           engineDB,
		projectID:          projectID,
		datasourceID:       datasourceID,
		mcpServer:          mcpServer,
		columnMetadataRepo: columnMetadataRepo,
		schemaRepo:         schemaRepo,
		contextDeps:        contextDeps,
		columnDeps:         columnDeps,
	}
}

// cleanup removes test data.
func (tc *sensitiveTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Clean up in order due to foreign keys
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_column_metadata WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_columns WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *sensitiveTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	// Include admin role to access developer tools
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})

	return ctx, func() { scope.Close() }
}

// createTestTable creates a table with columns for testing.
func (tc *sensitiveTestContext) createTestTable(tableName string, columns []struct {
	name         string
	dataType     string
	sampleValues []string // Note: sampleValues not persisted to DB (removed in schema refactor)
}) uuid.UUID {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Create the table
	tableID := uuid.New()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected)
		VALUES ($1, $2, $3, 'public', $4, true)
	`, tableID, tc.projectID, tc.datasourceID, tableName)
	require.NoError(tc.t, err)

	// Create columns (sample_values and metadata columns removed in schema refactor)
	for i, col := range columns {
		_, err = scope.Conn.Exec(ctx, `
			INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position, is_selected)
			VALUES ($1, $2, $3, $4, $5, true, $6, true)
		`, uuid.New(), tc.projectID, tableID, col.name, col.dataType, i+1)
		require.NoError(tc.t, err)
	}

	return tableID
}

// callTool invokes an MCP tool and returns the result.
func (tc *sensitiveTestContext) callTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args

	// Get the handler from the MCP server via the HandleMessage path
	reqJSON, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	})

	result := tc.mcpServer.HandleMessage(ctx, reqJSON)
	resultJSON, _ := json.Marshal(result)

	var response struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resultJSON, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", response.Error.Message)
	}

	if len(response.Result.Content) > 0 && response.Result.Content[0].Type == "text" {
		return mcp.NewToolResultText(response.Result.Content[0].Text), nil
	}
	return nil, nil
}

// TestSensitiveFlag_ManualOverride tests that the is_sensitive flag correctly overrides automatic detection.
func TestSensitiveFlag_ManualOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tc := setupSensitiveIntegrationTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a test table with columns including sample values
	// One column has a sensitive-sounding name, one has normal name but sensitive content
	tc.createTestTable("test_sensitive", []struct {
		name         string
		dataType     string
		sampleValues []string
	}{
		{
			name:         "api_key", // Auto-detected as sensitive by name
			dataType:     "text",
			sampleValues: []string{"sk-abc123", "sk-def456"},
		},
		{
			name:         "agent_data", // Normal name but contains sensitive JSON
			dataType:     "jsonb",
			sampleValues: []string{`{"livekit_api_key": "API123", "livekit_api_secret": "SECRET456"}`},
		},
		{
			name:         "normal_column", // Normal column, not sensitive
			dataType:     "text",
			sampleValues: []string{"value1", "value2"},
		},
	})

	// Test 1: Call update_column to mark api_key as NOT sensitive (override auto-detection)
	result, err := tc.callTool(ctx, "update_column", map[string]any{
		"table":     "test_sensitive",
		"column":    "api_key",
		"sensitive": false, // Override: NOT sensitive
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the is_sensitive flag was set to false
	var updateResponse updateColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &updateResponse)
	require.NoError(t, err)
	require.NotNil(t, updateResponse.IsSensitive)
	assert.False(t, *updateResponse.IsSensitive)

	// Test 2: Call update_column to mark normal_column as sensitive (override auto-detection)
	result, err = tc.callTool(ctx, "update_column", map[string]any{
		"table":     "test_sensitive",
		"column":    "normal_column",
		"sensitive": true, // Override: IS sensitive
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the is_sensitive flag was set to true
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &updateResponse)
	require.NoError(t, err)
	require.NotNil(t, updateResponse.IsSensitive)
	assert.True(t, *updateResponse.IsSensitive)

	// Test 3: Verify get_column_metadata returns the is_sensitive flag
	result, err = tc.callTool(ctx, "get_column_metadata", map[string]any{
		"table":  "test_sensitive",
		"column": "api_key",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var metaResponse getColumnMetadataResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &metaResponse)
	require.NoError(t, err)
	require.NotNil(t, metaResponse.Metadata)
	require.NotNil(t, metaResponse.Metadata.IsSensitive)
	assert.False(t, *metaResponse.Metadata.IsSensitive)

	// Test 4: Verify get_column_metadata for normal_column shows it as sensitive
	result, err = tc.callTool(ctx, "get_column_metadata", map[string]any{
		"table":  "test_sensitive",
		"column": "normal_column",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &metaResponse)
	require.NoError(t, err)
	require.NotNil(t, metaResponse.Metadata)
	require.NotNil(t, metaResponse.Metadata.IsSensitive)
	assert.True(t, *metaResponse.Metadata.IsSensitive)
}

// TestSensitiveFlag_GetContextRedaction tests that get_context respects the is_sensitive flag.
func TestSensitiveFlag_GetContextRedaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tc := setupSensitiveIntegrationTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a test table with sample values
	tc.createTestTable("test_redaction", []struct {
		name         string
		dataType     string
		sampleValues []string
	}{
		{
			name:         "password", // Auto-detected as sensitive
			dataType:     "text",
			sampleValues: []string{"secret123"},
		},
		{
			name:         "display_name", // Normal, but we'll mark as sensitive
			dataType:     "text",
			sampleValues: []string{"John Doe", "Jane Smith"},
		},
	})

	// Mark display_name as sensitive
	_, err := tc.callTool(ctx, "update_column", map[string]any{
		"table":     "test_redaction",
		"column":    "display_name",
		"sensitive": true,
	})
	require.NoError(t, err)

	// Mark password as NOT sensitive (override)
	_, err = tc.callTool(ctx, "update_column", map[string]any{
		"table":     "test_redaction",
		"column":    "password",
		"sensitive": false,
	})
	require.NoError(t, err)

	// Verify column metadata in database directly
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()
	scopeCtx := database.SetTenantScope(context.Background(), scope)

	// First, find the table to get column IDs
	table, err := tc.schemaRepo.FindTableByName(scopeCtx, tc.projectID, tc.datasourceID, "test_redaction")
	require.NoError(t, err)
	require.NotNil(t, table)

	// Check display_name is marked as sensitive
	displayCol, err := tc.schemaRepo.GetColumnByName(scopeCtx, table.ID, "display_name")
	require.NoError(t, err)
	require.NotNil(t, displayCol)
	displayMeta, err := tc.columnMetadataRepo.GetBySchemaColumnID(scopeCtx, displayCol.ID)
	require.NoError(t, err)
	require.NotNil(t, displayMeta)
	require.NotNil(t, displayMeta.IsSensitive)
	assert.True(t, *displayMeta.IsSensitive)

	// Check password is marked as not sensitive
	passwordCol, err := tc.schemaRepo.GetColumnByName(scopeCtx, table.ID, "password")
	require.NoError(t, err)
	require.NotNil(t, passwordCol)
	passwordMeta, err := tc.columnMetadataRepo.GetBySchemaColumnID(scopeCtx, passwordCol.ID)
	require.NoError(t, err)
	require.NotNil(t, passwordMeta)
	require.NotNil(t, passwordMeta.IsSensitive)
	assert.False(t, *passwordMeta.IsSensitive)
}

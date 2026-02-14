//go:build integration

package tools

import (
	"context"
	"encoding/json"
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

// schemaSelectionTestContext holds test dependencies for schema selection enforcement tests.
// These tests verify that MCP tools respect the is_selected flag on tables and columns,
// which is a security boundary: users choose which tables to expose via MCP.
type schemaSelectionTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	projectID    uuid.UUID
	datasourceID uuid.UUID
	mcpServer    *server.MCPServer
	schemaRepo   repositories.SchemaRepository

	// IDs for verification
	selectedTableID   uuid.UUID
	unselectedTableID uuid.UUID
}

// setupSchemaSelectionTest initializes a test context with both selected and unselected tables.
//
// Schema layout:
//   - selected_users (is_selected=true) with columns: id, email, name
//   - unselected_secrets (is_selected=false) with columns: id, api_key, secret_token
//
// The security invariant: MCP tools must NEVER return data about unselected_secrets.
func setupSchemaSelectionTest(t *testing.T) *schemaSelectionTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000088")
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000188")

	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	// Ensure test project exists
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Schema Selection Security Test")
	require.NoError(t, err)

	// Ensure datasource exists
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, provider, datasource_config)
		VALUES ($1, $2, 'test_datasource', 'postgresql', 'custom', '{}')
		ON CONFLICT (id) DO NOTHING
	`, datasourceID, projectID)
	require.NoError(t, err)

	// Create MCP server with all relevant tools registered
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	schemaRepo := repositories.NewSchemaRepository()

	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true,
			AddQueryTools:          true,
			AddOntologyMaintenance: true,
		},
	}
	mockProject := &mockProjectService{defaultDatasourceID: datasourceID}

	// Register search tools
	RegisterSearchTools(mcpServer, &SearchToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		SchemaRepo: schemaRepo,
	})

	// Register probe tools
	RegisterProbeTools(mcpServer, &ProbeToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		SchemaRepo:         schemaRepo,
		OntologyRepo:       repositories.NewOntologyRepository(),
		ColumnMetadataRepo: repositories.NewColumnMetadataRepository(),
		ProjectService:     mockProject,
	})

	// Register column tools
	RegisterColumnTools(mcpServer, &ColumnToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		SchemaRepo:         schemaRepo,
		OntologyRepo:       repositories.NewOntologyRepository(),
		ColumnMetadataRepo: repositories.NewColumnMetadataRepository(),
		ProjectService:     mockProject,
	})

	tc := &schemaSelectionTestContext{
		t:            t,
		engineDB:     engineDB,
		projectID:    projectID,
		datasourceID: datasourceID,
		mcpServer:    mcpServer,
		schemaRepo:   schemaRepo,
	}

	// Create the schema: one selected table, one unselected table
	tc.createTestSchema()

	return tc
}

// createTestSchema creates the two-table schema for testing selection enforcement.
func (tc *schemaSelectionTestContext) createTestSchema() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Table 1: selected_users (is_selected = TRUE) - user should see this
	tc.selectedTableID = uuid.New()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'selected_users', true, 100)
	`, tc.selectedTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	// Columns for selected_users (all selected)
	for i, col := range []struct {
		name     string
		dataType string
	}{
		{"id", "uuid"},
		{"email", "text"},
		{"name", "text"},
	} {
		_, err = scope.Conn.Exec(ctx, `
			INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position, is_selected)
			VALUES ($1, $2, $3, $4, $5, true, $6, true)
		`, uuid.New(), tc.projectID, tc.selectedTableID, col.name, col.dataType, i+1)
		require.NoError(tc.t, err)
	}

	// Table 2: unselected_secrets (is_selected = FALSE) - user should NOT see this
	tc.unselectedTableID = uuid.New()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'unselected_secrets', false, 50)
	`, tc.unselectedTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	// Columns for unselected_secrets (all unselected)
	for i, col := range []struct {
		name     string
		dataType string
	}{
		{"id", "uuid"},
		{"api_key", "text"},
		{"secret_token", "text"},
	} {
		_, err = scope.Conn.Exec(ctx, `
			INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position, is_selected)
			VALUES ($1, $2, $3, $4, $5, true, $6, false)
		`, uuid.New(), tc.projectID, tc.unselectedTableID, col.name, col.dataType, i+1)
		require.NoError(tc.t, err)
	}
}

// cleanup removes all test data for this project.
func (tc *schemaSelectionTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_column_metadata WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_columns WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and admin claims.
func (tc *schemaSelectionTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})
	ctx = models.WithMCPProvenance(ctx, uuid.Nil)

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *schemaSelectionTestContext) callTool(ctx context.Context, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	tc.t.Helper()

	reqBytes, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      1,
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	})
	require.NoError(tc.t, err)

	result := tc.mcpServer.HandleMessage(ctx, reqBytes)
	resultBytes, err := json.Marshal(result)
	require.NoError(tc.t, err)

	var response struct {
		Result *mcp.CallToolResult `json:"result,omitempty"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	err = json.Unmarshal(resultBytes, &response)
	require.NoError(tc.t, err)

	if response.Error != nil {
		return nil, &mcpError{Code: response.Error.Code, Message: response.Error.Message}
	}

	return response.Result, nil
}

// =============================================================================
// Security Tests: MCP tools must respect schema table selection (is_selected)
// =============================================================================

// TestSchemaSelection_SearchSchema_ExcludesUnselectedTables verifies that
// search_schema does NOT return tables where is_selected=false.
//
// BUG: search.go searchTables() uses raw SQL without "AND st.is_selected = true".
func TestSchemaSelection_SearchSchema_ExcludesUnselectedTables(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Search for "secrets" - this is the unselected table name
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "secrets",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// SECURITY ASSERTION: unselected table must NOT appear in results
	assert.Empty(t, searchResp.Tables,
		"search_schema must not return unselected tables - 'unselected_secrets' has is_selected=false")
	assert.Equal(t, 0, searchResp.TotalCount,
		"total_count must be 0 when only unselected tables match the query")
}

// TestSchemaSelection_SearchSchema_ExcludesUnselectedColumns verifies that
// search_schema does NOT return columns from unselected tables.
//
// BUG: search.go searchColumns() uses raw SQL without "AND st.is_selected = true".
func TestSchemaSelection_SearchSchema_ExcludesUnselectedColumns(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Search for "api_key" - a column in the unselected table
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "api_key",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// SECURITY ASSERTION: columns from unselected tables must NOT appear
	assert.Empty(t, searchResp.Columns,
		"search_schema must not return columns from unselected tables - 'api_key' is in unselected_secrets")
	assert.Equal(t, 0, searchResp.TotalCount,
		"total_count must be 0 when only columns from unselected tables match")
}

// TestSchemaSelection_SearchSchema_IncludesSelectedTables verifies that
// search_schema still returns tables where is_selected=true (sanity check).
func TestSchemaSelection_SearchSchema_IncludesSelectedTables(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Search for "selected_users" - the selected table
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "selected_users",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// Sanity check: selected table SHOULD appear
	assert.NotEmpty(t, searchResp.Tables,
		"search_schema must return selected tables")
	assert.Equal(t, "selected_users", searchResp.Tables[0].TableName)
}

// TestSchemaSelection_ProbeColumn_RejectsUnselectedTable verifies that
// probe_column does NOT return data for columns in unselected tables.
func TestSchemaSelection_ProbeColumn_RejectsUnselectedTable(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Attempt to probe a column in the unselected table
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "unselected_secrets",
		"column": "api_key",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response — the handler converts probeColumnResponse errors into ErrorResponse
	var errResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errResp)
	require.NoError(t, err)

	// SECURITY ASSERTION: probing an unselected table should behave as if
	// the table doesn't exist (TABLE_NOT_FOUND)
	assert.True(t, errResp.Error,
		"probe_column must return an error for unselected tables - 'unselected_secrets' has is_selected=false")
	assert.Equal(t, "TABLE_NOT_FOUND", errResp.Code,
		"probe_column should report TABLE_NOT_FOUND for unselected tables")
}

// TestSchemaSelection_ProbeColumn_AllowsSelectedTable verifies that
// probe_column works for columns in selected tables (sanity check).
func TestSchemaSelection_ProbeColumn_AllowsSelectedTable(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Probe a column in the selected table
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "selected_users",
		"column": "email",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var probeResp probeColumnResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResp)
	require.NoError(t, err)

	// Sanity check: selected table should work
	assert.Empty(t, probeResp.Error,
		"probe_column must succeed for selected tables")
	assert.Equal(t, "selected_users", probeResp.Table)
	assert.Equal(t, "email", probeResp.Column)
}

// TestSchemaSelection_ProbeColumns_ExcludesUnselectedTable verifies that
// the batch probe_columns tool also respects selection.
func TestSchemaSelection_ProbeColumns_ExcludesUnselectedTable(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Batch probe: one selected column, one unselected
	result, err := tc.callTool(ctx, "probe_columns", map[string]any{
		"columns": []map[string]any{
			{"table": "selected_users", "column": "email"},
			{"table": "unselected_secrets", "column": "api_key"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var batchResp probeColumnsResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &batchResp)
	require.NoError(t, err)

	// Selected column should succeed
	selectedResult, ok := batchResp.Results["selected_users.email"]
	require.True(t, ok, "should have result for selected_users.email")
	assert.Empty(t, selectedResult.Error, "selected column should succeed")

	// SECURITY ASSERTION: unselected column should fail as TABLE_NOT_FOUND
	unselectedResult, ok := batchResp.Results["unselected_secrets.api_key"]
	require.True(t, ok, "should have result for unselected_secrets.api_key")
	assert.NotEmpty(t, unselectedResult.Error,
		"probe_columns must return error for unselected tables")
	assert.Contains(t, unselectedResult.Error, "TABLE_NOT_FOUND",
		"probe_columns should report TABLE_NOT_FOUND for unselected tables")
}

// TestSchemaSelection_GetColumnMetadata_RejectsUnselectedTable verifies that
// get_column_metadata does NOT return metadata for columns in unselected tables.
func TestSchemaSelection_GetColumnMetadata_RejectsUnselectedTable(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Attempt to get metadata for a column in the unselected table
	result, err := tc.callTool(ctx, "get_column_metadata", map[string]any{
		"table":  "unselected_secrets",
		"column": "api_key",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse the response text
	responseText := result.Content[0].(mcp.TextContent).Text

	// SECURITY ASSERTION: should return an error (column/table not found)
	// rather than successfully returning metadata for an unselected table
	assert.True(t, result.IsError || containsErrorCode(responseText, "not_found") || containsErrorCode(responseText, "TABLE_NOT_FOUND"),
		"get_column_metadata must reject access to columns in unselected tables, got: %s", responseText)
}

// TestSchemaSelection_UpdateColumn_RejectsUnselectedTable verifies that
// update_column does NOT allow modifying metadata for columns in unselected tables.
func TestSchemaSelection_UpdateColumn_RejectsUnselectedTable(t *testing.T) {
	tc := setupSchemaSelectionTest(t)
	tc.cleanup()
	defer tc.cleanup()
	tc.createTestSchema()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Attempt to update metadata for a column in the unselected table
	result, err := tc.callTool(ctx, "update_column", map[string]any{
		"table":       "unselected_secrets",
		"column":      "api_key",
		"description": "This should be rejected",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	responseText := result.Content[0].(mcp.TextContent).Text

	// SECURITY ASSERTION: should reject modification of unselected columns
	assert.True(t, result.IsError || containsErrorCode(responseText, "not_found") || containsErrorCode(responseText, "TABLE_NOT_FOUND"),
		"update_column must reject modifications to columns in unselected tables, got: %s", responseText)
}

// containsErrorCode checks if a JSON response text contains a specific error code.
func containsErrorCode(text string, code string) bool {
	var response map[string]any
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		return false
	}
	if errCode, ok := response["code"].(string); ok {
		return errCode == code
	}
	if errMsg, ok := response["error"].(string); ok {
		return len(errMsg) > 0 && (errMsg == code || json.Valid([]byte(errMsg)))
	}
	return false
}

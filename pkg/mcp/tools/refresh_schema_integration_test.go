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
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// refreshSchemaTestContext holds test dependencies for refresh_schema tool integration tests.
type refreshSchemaTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	projectID uuid.UUID
	mcpServer *server.MCPServer
}

// setupRefreshSchemaIntegrationTest initializes the test context with shared testcontainer.
func setupRefreshSchemaIntegrationTest(t *testing.T) *refreshSchemaTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000067")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Refresh Schema Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with refresh_schema tool
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	// Configure mock to enable developer tools
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:       true,
			AddQueryTools: true,
		},
	}

	// Note: This test uses a mock schema service to test the response logic
	// without needing actual datasource connections.
	// deps struct will be set up in individual tests with mock services.
	_ = mockMCPConfig // Suppress unused variable warning - used in test setup

	return &refreshSchemaTestContext{
		t:         t,
		engineDB:  engineDB,
		projectID: projectID,
		mcpServer: mcpServer,
	}
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *refreshSchemaTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	// Include admin role to access developer tools (refresh_schema requires developer access)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *refreshSchemaTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	tc.t.Helper()

	// Build MCP request
	reqID := 1
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      reqID,
		"params": map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	reqBytes, err := json.Marshal(callReq)
	require.NoError(tc.t, err)

	// Call the tool through MCP server
	result := tc.mcpServer.HandleMessage(ctx, reqBytes)

	// Parse response
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

// TestRefreshSchema_AutoSelectApplied_ReflectsNewTables verifies that
// auto_select_applied in the response reflects whether new tables were discovered.
// Selection is now set at creation time (not via SelectAllTables), so auto_select_applied
// is true when autoSelect=true AND new tables exist.
func TestRefreshSchema_AutoSelectApplied_ReflectsNewTables_Integration(t *testing.T) {
	tc := setupRefreshSchemaIntegrationTest(t)
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000068")

	tests := []struct {
		name                      string
		newTableNames             []string
		expectedAutoSelectApplied bool
	}{
		{
			name:                      "auto_select_applied true when new tables discovered",
			newTableNames:             []string{"public.new_table"},
			expectedAutoSelectApplied: true,
		},
		{
			name:                      "auto_select_applied false when no new tables",
			newTableNames:             []string{},
			expectedAutoSelectApplied: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh MCP server for each test
			mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
			tc.mcpServer = mcpServer

			// Set up mocks - refresh_schema requires ontology maintenance to be enabled
			mockMCPConfig := &mockMCPConfigService{
				config: &models.ToolGroupConfig{
					Enabled:                true,
					AddOntologyMaintenance: true,
				},
			}

			mockSchema := &mockSchemaService{
				refreshResult: &models.RefreshResult{
					NewTableNames:        tt.newTableNames,
					RemovedTableNames:    []string{},
					TablesUpserted:       len(tt.newTableNames),
					ColumnsUpserted:      5,
					RelationshipsCreated: 0,
				},
			}

			mockProject := &mockProjectService{
				defaultDatasourceID: datasourceID,
			}

			deps := &MCPToolDeps{
				BaseMCPToolDeps: BaseMCPToolDeps{
					DB:               tc.engineDB.DB,
					MCPConfigService: mockMCPConfig,
					Logger:           zap.NewNop(),
				},
				SchemaService:  mockSchema,
				ProjectService: mockProject,
			}
			registerRefreshSchemaTool(mcpServer, deps)

			ctx, cleanup := tc.createTestContext()
			defer cleanup()

			// Call refresh_schema tool
			result, err := tc.callTool(ctx, "refresh_schema", map[string]any{
				"auto_select": true,
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			// Parse the tool result content
			require.Len(t, result.Content, 1)
			textContent, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok, "expected text content")

			var refreshResponse struct {
				TablesAdded       []string `json:"tables_added"`
				AutoSelectApplied bool     `json:"auto_select_applied"`
			}
			err = json.Unmarshal([]byte(textContent.Text), &refreshResponse)
			require.NoError(t, err)

			// Verify auto_select_applied reflects whether new tables were discovered
			assert.Equal(t, tt.expectedAutoSelectApplied, refreshResponse.AutoSelectApplied,
				"auto_select_applied should be true when autoSelect=true AND new tables exist")
		})
	}
}

// TestRefreshSchema_ColumnsAdded_ReportsOnlyNewColumns verifies that columns_added
// in the response reports only NEW columns, not total columns upserted.
// This fixes the bug where refresh_schema reported 634 columns on every run.
func TestRefreshSchema_ColumnsAdded_ReportsOnlyNewColumns_Integration(t *testing.T) {
	tc := setupRefreshSchemaIntegrationTest(t)
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000069")

	tests := []struct {
		name                 string
		columnsUpserted      int // Total columns processed (upserted/updated)
		newColumns           []models.RefreshColumnChange
		expectedColumnsAdded int
	}{
		{
			name:                 "reports 0 columns when no new columns exist",
			columnsUpserted:      634,                            // Large number representing all columns in the database
			newColumns:           []models.RefreshColumnChange{}, // No actual new columns
			expectedColumnsAdded: 0,
		},
		{
			name:            "reports only new columns when some are new",
			columnsUpserted: 634, // Total columns processed
			newColumns: []models.RefreshColumnChange{
				{TableName: "public.users", ColumnName: "new_field", DataType: "text"},
				{TableName: "public.orders", ColumnName: "tracking_id", DataType: "varchar"},
			},
			expectedColumnsAdded: 2,
		},
		{
			name:            "reports all columns when all are new (initial schema discovery)",
			columnsUpserted: 50, // All columns are new
			newColumns: func() []models.RefreshColumnChange {
				// Simulate 50 new columns
				cols := make([]models.RefreshColumnChange, 50)
				for i := 0; i < 50; i++ {
					cols[i] = models.RefreshColumnChange{
						TableName:  "public.new_table",
						ColumnName: "col_" + string(rune('a'+i%26)),
						DataType:   "text",
					}
				}
				return cols
			}(),
			expectedColumnsAdded: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh MCP server for each test
			mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
			tc.mcpServer = mcpServer

			// Set up mocks
			mockMCPConfig := &mockMCPConfigService{
				config: &models.ToolGroupConfig{
					Enabled:                true,
					AddOntologyMaintenance: true,
				},
			}

			mockSchema := &mockSchemaService{
				refreshResult: &models.RefreshResult{
					NewTableNames:        []string{},
					RemovedTableNames:    []string{},
					TablesUpserted:       0,
					ColumnsUpserted:      tt.columnsUpserted,
					NewColumns:           tt.newColumns,
					RelationshipsCreated: 0,
				},
			}

			mockProject := &mockProjectService{
				defaultDatasourceID: datasourceID,
			}

			deps := &MCPToolDeps{
				BaseMCPToolDeps: BaseMCPToolDeps{
					DB:               tc.engineDB.DB,
					MCPConfigService: mockMCPConfig,
					Logger:           zap.NewNop(),
				},
				SchemaService:  mockSchema,
				ProjectService: mockProject,
			}
			registerRefreshSchemaTool(mcpServer, deps)

			ctx, cleanup := tc.createTestContext()
			defer cleanup()

			// Call refresh_schema tool
			result, err := tc.callTool(ctx, "refresh_schema", map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, result)

			// Parse the tool result content
			require.Len(t, result.Content, 1)
			textContent, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok, "expected text content")

			var refreshResponse struct {
				ColumnsAdded int `json:"columns_added"`
			}
			err = json.Unmarshal([]byte(textContent.Text), &refreshResponse)
			require.NoError(t, err)

			// Verify columns_added reports only NEW columns, not total upserted
			assert.Equal(t, tt.expectedColumnsAdded, refreshResponse.ColumnsAdded,
				"columns_added should report len(NewColumns), not ColumnsUpserted")
		})
	}
}

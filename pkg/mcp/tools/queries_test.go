package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockQueryService implements services.QueryService for testing.
type mockQueryService struct {
	enabledQueries []*models.Query
	executeResult  *datasource.QueryExecutionResult
	executeError   error
}

func (m *mockQueryService) Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.CreateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Update(ctx context.Context, projectID, queryID uuid.UUID, req *services.UpdateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Delete(ctx context.Context, projectID, queryID uuid.UUID) error {
	return nil
}

func (m *mockQueryService) ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return m.enabledQueries, nil
}

func (m *mockQueryService) SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error {
	return nil
}

func (m *mockQueryService) Execute(ctx context.Context, projectID, queryID uuid.UUID, req *services.ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return m.executeResult, m.executeError
}

func (m *mockQueryService) ExecuteWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any, req *services.ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return m.executeResult, m.executeError
}

func (m *mockQueryService) Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.TestQueryRequest) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}

func (m *mockQueryService) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) error {
	return nil
}

func (m *mockQueryService) ValidateParameterizedQuery(sqlQuery string, params []models.QueryParameter) error {
	return nil
}

func TestRegisterApprovedQueriesTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &QueryToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
		Logger:         zap.NewNop(),
	}

	RegisterApprovedQueriesTools(mcpServer, deps)

	// Verify tools are registered
	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	// Check both tools are registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["list_approved_queries"], "list_approved_queries tool should be registered")
	assert.True(t, toolNames["execute_approved_query"], "execute_approved_query tool should be registered")
}

// Note: Full integration tests for tool execution require a database connection
// and would be covered in integration tests. The tool registration test above
// verifies that both tools are properly registered with the MCP server.

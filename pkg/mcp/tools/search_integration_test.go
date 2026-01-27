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

// searchToolTestContext holds test dependencies for search tool integration tests.
type searchToolTestContext struct {
	t                  *testing.T
	engineDB           *testhelpers.EngineDB
	projectID          uuid.UUID
	mcpServer          *server.MCPServer
	ontologyRepo       repositories.OntologyRepository
	ontologyEntityRepo repositories.OntologyEntityRepository
	schemaRepo         repositories.SchemaRepository
}

// setupSearchToolIntegrationTest initializes the test context with shared testcontainer.
func setupSearchToolIntegrationTest(t *testing.T) *searchToolTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000077")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Search Tool Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with search tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	ontologyRepo := repositories.NewOntologyRepository()
	ontologyEntityRepo := repositories.NewOntologyEntityRepository()
	schemaRepo := repositories.NewSchemaRepository()

	// Configure mock to enable developer tools
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:       true,
			AddQueryTools: true,
		},
	}

	deps := &SearchToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mockMCPConfig,
		SchemaRepo:       schemaRepo,
		OntologyRepo:     ontologyRepo,
		EntityRepo:       ontologyEntityRepo,
		Logger:           zap.NewNop(),
	}

	RegisterSearchTools(mcpServer, deps)

	return &searchToolTestContext{
		t:                  t,
		engineDB:           engineDB,
		projectID:          projectID,
		mcpServer:          mcpServer,
		ontologyRepo:       ontologyRepo,
		ontologyEntityRepo: ontologyEntityRepo,
		schemaRepo:         schemaRepo,
	}
}

// cleanup removes test data.
func (tc *searchToolTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Clean up in order due to foreign keys
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_aliases WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *searchToolTestContext) createTestContext() (context.Context, func()) {
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
func (tc *searchToolTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	tc.t.Helper()

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

// createOntologyWithEntityAndAliases creates test data for search.
func (tc *searchToolTestContext) createOntologyWithEntityAndAliases(ctx context.Context, entityName string, aliases []string) uuid.UUID {
	tc.t.Helper()

	// Create an active ontology
	ontology := &models.TieredOntology{
		ProjectID:       tc.projectID,
		Version:         1,
		IsActive:        true,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
		Metadata:        make(map[string]any),
	}
	err := tc.ontologyRepo.Create(ctx, ontology)
	require.NoError(tc.t, err)

	// Create an entity under this ontology
	ctxWithProv := models.WithInferredProvenance(ctx, uuid.Nil)
	entity := &models.OntologyEntity{
		ProjectID:    tc.projectID,
		OntologyID:   ontology.ID,
		Name:         entityName,
		Description:  "Test entity for search",
		PrimaryTable: entityName + "_table",
	}
	err = tc.ontologyEntityRepo.Create(ctxWithProv, entity)
	require.NoError(tc.t, err)

	// Create aliases directly in the database
	scope, ok := database.GetTenantScope(ctx)
	require.True(tc.t, ok)
	for _, alias := range aliases {
		_, err := scope.Conn.Exec(ctx, `
			INSERT INTO engine_ontology_entity_aliases (project_id, entity_id, alias, source)
			VALUES ($1, $2, $3, 'test')
		`, tc.projectID, entity.ID, alias)
		require.NoError(tc.t, err)
	}

	return ontology.ID
}

// TestSearchSchemaTool_Integration_EntitySearch verifies that search_schema
// correctly searches entities including by alias. This tests the fix for
// ISSUE-search-schema-deleted-at-column-missing.md where the entity search
// query referenced a non-existent a.deleted_at column.
func TestSearchSchemaTool_Integration_EntitySearch(t *testing.T) {
	tc := setupSearchToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an entity with aliases
	tc.createOntologyWithEntityAndAliases(ctx, "SearchableUser", []string{"customer", "client"})

	// Search by entity name
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "searchableuser",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error")

	// Parse response
	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// Verify entity was found
	assert.Len(t, searchResp.Entities, 1, "should find one entity")
	assert.Equal(t, "SearchableUser", searchResp.Entities[0].Name)
	assert.Equal(t, "name", searchResp.Entities[0].MatchType)
}

// TestSearchSchemaTool_Integration_EntitySearchByAlias verifies that entities
// can be found by searching their aliases. This directly tests the fixed query
// that was failing due to a.deleted_at column reference.
func TestSearchSchemaTool_Integration_EntitySearchByAlias(t *testing.T) {
	tc := setupSearchToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an entity with aliases
	tc.createOntologyWithEntityAndAliases(ctx, "HostUser", []string{"creator", "owner"})

	// Search by alias
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "creator",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error")

	// Parse response
	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// Verify entity was found via alias
	assert.Len(t, searchResp.Entities, 1, "should find one entity via alias")
	assert.Equal(t, "HostUser", searchResp.Entities[0].Name)
	assert.Equal(t, "alias", searchResp.Entities[0].MatchType)
	assert.Contains(t, searchResp.Entities[0].Aliases, "creator")
	assert.Contains(t, searchResp.Entities[0].Aliases, "owner")
}

// TestSearchSchemaTool_Integration_NoOntology verifies that search_schema
// gracefully handles the case where no ontology exists.
func TestSearchSchemaTool_Integration_NoOntology(t *testing.T) {
	tc := setupSearchToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Search without any ontology created
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "anything",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error when no ontology exists")

	// Parse response
	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// Verify empty results (no error, just no matches)
	assert.Len(t, searchResp.Entities, 0, "should return empty entities when no ontology")
}

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

// entityToolTestContext holds test dependencies for entity tool integration tests.
type entityToolTestContext struct {
	t                      *testing.T
	engineDB               *testhelpers.EngineDB
	projectID              uuid.UUID
	mcpServer              *server.MCPServer
	ontologyRepo           repositories.OntologyRepository
	ontologyEntityRepo     repositories.OntologyEntityRepository
	entityRelationshipRepo repositories.EntityRelationshipRepository
}

// setupEntityToolIntegrationTest initializes the test context with shared testcontainer.
func setupEntityToolIntegrationTest(t *testing.T) *entityToolTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000066")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Entity Tool Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with entity tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	ontologyRepo := repositories.NewOntologyRepository()
	ontologyEntityRepo := repositories.NewOntologyEntityRepository()
	entityRelationshipRepo := repositories.NewEntityRelationshipRepository()

	// Configure mock to enable entity tools (developer with ontology maintenance)
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true,
			AddQueryTools:          true, // Enables query tools
			AddOntologyMaintenance: true, // Enables update_entity, delete_entity, etc.
		},
	}

	deps := &EntityToolDeps{
		DB:                     engineDB.DB,
		MCPConfigService:       mockMCPConfig,
		OntologyRepo:           ontologyRepo,
		OntologyEntityRepo:     ontologyEntityRepo,
		EntityRelationshipRepo: entityRelationshipRepo,
		Logger:                 zap.NewNop(),
	}

	RegisterEntityTools(mcpServer, deps)

	return &entityToolTestContext{
		t:                      t,
		engineDB:               engineDB,
		projectID:              projectID,
		mcpServer:              mcpServer,
		ontologyRepo:           ontologyRepo,
		ontologyEntityRepo:     ontologyEntityRepo,
		entityRelationshipRepo: entityRelationshipRepo,
	}
}

// cleanup removes test entities and ontologies.
func (tc *entityToolTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Clean up in order due to foreign keys
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_entity_relationships WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_occurrences WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_key_columns WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_aliases WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *entityToolTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{ProjectID: tc.projectID.String()})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *entityToolTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
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

// createOntologyAndEntity creates an ontology and entity directly in the database,
// simulating what happens during ontology extraction.
func (tc *entityToolTestContext) createOntologyAndEntity(ctx context.Context, entityName string) (uuid.UUID, uuid.UUID) {
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

	// Create an entity under this ontology (simulating extraction)
	entity := &models.OntologyEntity{
		ProjectID:    tc.projectID,
		OntologyID:   ontology.ID,
		Name:         entityName,
		Description:  "Test entity created during extraction",
		PrimaryTable: entityName + "_table",
		CreatedBy:    models.ProvenanceInference, // Extraction creates with inference provenance
	}
	err = tc.ontologyEntityRepo.Create(ctx, entity)
	require.NoError(tc.t, err)

	return ontology.ID, entity.ID
}

// ============================================================================
// Integration Tests: get_entity with extraction-created entities
// ============================================================================

// TestGetEntityTool_Integration_FindsExtractionCreatedEntity verifies that get_entity
// can find entities that were created during ontology extraction. This tests the fix
// for Bug 6 where entities visible in get_context were not found by get_entity.
func TestGetEntityTool_Integration_FindsExtractionCreatedEntity(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an ontology and entity directly (simulating extraction)
	_, _ = tc.createOntologyAndEntity(ctx, "Customer")

	// Call get_entity tool - this should find the entity
	result, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "Customer",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error")

	// Parse response
	var response getEntityResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify entity was found
	assert.Equal(t, "Customer", response.Name)
	assert.Equal(t, "Customer_table", response.PrimaryTable)
	assert.Equal(t, "Test entity created during extraction", response.Description)
}

// TestGetEntityTool_Integration_EntityNotFound verifies that get_entity returns
// appropriate error when entity doesn't exist.
func TestGetEntityTool_Integration_EntityNotFound(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call get_entity for a non-existent entity
	result, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "NonExistentEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error for non-existent entity")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "ENTITY_NOT_FOUND", errorResp.Code)
	assert.Contains(t, errorResp.Message, "NonExistentEntity")
}

// ============================================================================
// Integration Tests: update_entity with extraction-created entities
// ============================================================================

// TestUpdateEntityTool_Integration_UpdatesExtractionCreatedEntity verifies that
// update_entity can update entities that were created during ontology extraction.
func TestUpdateEntityTool_Integration_UpdatesExtractionCreatedEntity(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an ontology and entity directly (simulating extraction)
	_, _ = tc.createOntologyAndEntity(ctx, "Order")

	// Call update_entity tool to add description
	result, err := tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "Order",
		"description": "Updated description for Order entity",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error")

	// Parse response
	var response updateEntityResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify entity was updated (not created)
	assert.Equal(t, "Order", response.Name)
	assert.Equal(t, "Updated description for Order entity", response.Description)
	assert.False(t, response.Created, "should update existing entity, not create new one")
}

// TestUpdateEntityTool_Integration_CreatesNewEntity verifies that update_entity
// creates a new entity when one doesn't exist.
func TestUpdateEntityTool_Integration_CreatesNewEntity(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call update_entity tool to create new entity
	result, err := tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "NewProduct",
		"description": "A new product entity",
		"aliases":     []any{"item", "merchandise"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error")

	// Parse response
	var response updateEntityResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify entity was created
	assert.Equal(t, "NewProduct", response.Name)
	assert.Equal(t, "A new product entity", response.Description)
	assert.True(t, response.Created, "should create new entity")
	assert.Len(t, response.Aliases, 2)
	assert.Contains(t, response.Aliases, "item")
	assert.Contains(t, response.Aliases, "merchandise")

	// Verify entity can be retrieved
	getResult, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "NewProduct",
	})
	require.NoError(t, err)
	require.NotNil(t, getResult)
	assert.False(t, getResult.IsError)
}

// ============================================================================
// Integration Tests: delete_entity with extraction-created entities
// ============================================================================

// TestDeleteEntityTool_Integration_DeletesExtractionCreatedEntity verifies that
// delete_entity can delete entities that were created during ontology extraction.
func TestDeleteEntityTool_Integration_DeletesExtractionCreatedEntity(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an ontology and entity directly (simulating extraction)
	_, _ = tc.createOntologyAndEntity(ctx, "DeleteMe")

	// Call delete_entity tool
	result, err := tc.callTool(ctx, "delete_entity", map[string]any{
		"name": "DeleteMe",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error")

	// Parse response
	var response deleteEntityResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify entity was deleted
	assert.Equal(t, "DeleteMe", response.Name)
	assert.True(t, response.Deleted)

	// Verify entity is no longer found
	getResult, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "DeleteMe",
	})
	require.NoError(t, err)
	require.NotNil(t, getResult)
	assert.True(t, getResult.IsError, "should return error for deleted entity")
}

// TestDeleteEntityTool_Integration_EntityNotFound verifies that delete_entity returns
// appropriate error when entity doesn't exist.
func TestDeleteEntityTool_Integration_EntityNotFound(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call delete_entity for a non-existent entity
	result, err := tc.callTool(ctx, "delete_entity", map[string]any{
		"name": "NonExistentEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error for non-existent entity")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "ENTITY_NOT_FOUND", errorResp.Code)
	assert.Contains(t, errorResp.Message, "NonExistentEntity")
}

// ============================================================================
// Integration Tests: Complete Entity Lifecycle Workflow
// ============================================================================

// TestEntityTools_Integration_FullWorkflow tests the complete lifecycle of
// creating, reading, updating, and deleting an entity.
func TestEntityTools_Integration_FullWorkflow(t *testing.T) {
	tc := setupEntityToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create an entity via update_entity
	createResult, err := tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "WorkflowEntity",
		"description": "Initial description",
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	assert.False(t, createResult.IsError)

	var createResponse updateEntityResponse
	err = json.Unmarshal([]byte(createResult.Content[0].(mcp.TextContent).Text), &createResponse)
	require.NoError(t, err)
	assert.True(t, createResponse.Created)

	// Step 2: Read the entity via get_entity
	getResult, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "WorkflowEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, getResult)
	assert.False(t, getResult.IsError)

	var getResponse getEntityResponse
	err = json.Unmarshal([]byte(getResult.Content[0].(mcp.TextContent).Text), &getResponse)
	require.NoError(t, err)
	assert.Equal(t, "WorkflowEntity", getResponse.Name)
	assert.Equal(t, "Initial description", getResponse.Description)

	// Step 3: Update the entity
	updateResult, err := tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "WorkflowEntity",
		"description": "Updated description",
		"aliases":     []any{"workflow", "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	assert.False(t, updateResult.IsError)

	var updateResponse updateEntityResponse
	err = json.Unmarshal([]byte(updateResult.Content[0].(mcp.TextContent).Text), &updateResponse)
	require.NoError(t, err)
	assert.False(t, updateResponse.Created, "should update, not create")
	assert.Len(t, updateResponse.Aliases, 2)

	// Step 4: Read again to verify update
	getResult2, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "WorkflowEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, getResult2)
	assert.False(t, getResult2.IsError)

	var getResponse2 getEntityResponse
	err = json.Unmarshal([]byte(getResult2.Content[0].(mcp.TextContent).Text), &getResponse2)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", getResponse2.Description)
	assert.Len(t, getResponse2.Aliases, 2)

	// Step 5: Delete the entity
	deleteResult, err := tc.callTool(ctx, "delete_entity", map[string]any{
		"name": "WorkflowEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	assert.False(t, deleteResult.IsError)

	var deleteResponse deleteEntityResponse
	err = json.Unmarshal([]byte(deleteResult.Content[0].(mcp.TextContent).Text), &deleteResponse)
	require.NoError(t, err)
	assert.True(t, deleteResponse.Deleted)

	// Step 6: Verify entity is no longer accessible
	getResult3, err := tc.callTool(ctx, "get_entity", map[string]any{
		"name": "WorkflowEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, getResult3)
	assert.True(t, getResult3.IsError, "deleted entity should not be found")
}

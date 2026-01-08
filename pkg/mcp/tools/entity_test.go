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

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestEntityToolDeps verifies EntityToolDeps struct initialization.
func TestEntityToolDeps(t *testing.T) {
	logger := zap.NewNop()

	deps := &EntityToolDeps{
		Logger: logger,
	}

	assert.NotNil(t, deps, "EntityToolDeps should be initialized")
	assert.Equal(t, logger, deps.Logger, "Logger should be set correctly")
}

// TestRegisterEntityTools verifies tools are registered with the MCP server.
func TestRegisterEntityTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &EntityToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterEntityTools(mcpServer, deps)

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

	err = json.Unmarshal(resultBytes, &response)
	require.NoError(t, err)

	// Check that get_entity is in the tool list
	foundGetEntity := false
	for _, tool := range response.Result.Tools {
		if tool.Name == "get_entity" {
			foundGetEntity = true
			assert.Contains(t, tool.Description, "entity")
		}
	}

	assert.True(t, foundGetEntity, "get_entity tool should be registered")
}

// TestGetEntityToolStructure tests the tool definition structure.
func TestGetEntityToolStructure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &EntityToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterEntityTools(mcpServer, deps)

	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Type       string                 `json:"type"`
					Properties map[string]interface{} `json:"properties"`
					Required   []string               `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}

	err = json.Unmarshal(resultBytes, &response)
	require.NoError(t, err)

	// Find get_entity tool
	var getEntityTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "get_entity" {
			getEntityTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, getEntityTool, "get_entity tool should exist")

	// Verify required parameters
	assert.Contains(t, getEntityTool.InputSchema.Required, "name", "name should be required")

	// Verify properties
	assert.Contains(t, getEntityTool.InputSchema.Properties, "name", "should have name parameter")
}

// TestBuildGetEntityResponse tests the response builder.
func TestBuildGetEntityResponse(t *testing.T) {
	// Create test entity
	entityID := uuid.New()
	entity := &models.OntologyEntity{
		ID:           entityID,
		Name:         "User",
		PrimaryTable: "users",
		Description:  "A platform user",
	}

	// Create test aliases
	aliases := []*models.OntologyEntityAlias{
		{Alias: "creator"},
		{Alias: "host"},
	}

	// Create test key columns
	keyColumns := []*models.OntologyEntityKeyColumn{
		{ColumnName: "user_id"},
		{ColumnName: "username"},
	}

	// Create test relationships
	targetEntityID := uuid.New()
	association := "owns"
	sourceRels := []*models.EntityRelationship{
		{
			SourceEntityID:    entityID,
			TargetEntityID:    targetEntityID,
			SourceColumnTable: "users",
			SourceColumnName:  "user_id",
			TargetColumnTable: "accounts",
			TargetColumnName:  "owner_id",
			Association:       &association,
		},
	}

	targetRels := []*models.EntityRelationship{}

	// Create entity map
	entityMap := map[uuid.UUID]string{
		entityID:       "User",
		targetEntityID: "Account",
	}

	// Build response
	response := buildGetEntityResponse(entity, aliases, keyColumns, sourceRels, targetRels, entityMap)

	// Verify response structure
	assert.Equal(t, "User", response.Name)
	assert.Equal(t, "users", response.PrimaryTable)
	assert.Equal(t, "A platform user", response.Description)
	assert.Len(t, response.Aliases, 2)
	assert.Contains(t, response.Aliases, "creator")
	assert.Contains(t, response.Aliases, "host")
	assert.Len(t, response.KeyColumns, 2)
	assert.Contains(t, response.KeyColumns, "user_id")
	assert.Contains(t, response.KeyColumns, "username")

	// Verify occurrences
	assert.Len(t, response.Occurrences, 1)
	assert.Equal(t, "users", response.Occurrences[0].Table)
	assert.Equal(t, "user_id", response.Occurrences[0].Column)
	assert.Equal(t, "owns", response.Occurrences[0].Role)

	// Verify relationships
	assert.Len(t, response.Relationships, 1)
	assert.Equal(t, "to", response.Relationships[0].Direction)
	assert.Equal(t, "Account", response.Relationships[0].Entity)
	assert.Equal(t, "owns", response.Relationships[0].Label)
	assert.Contains(t, response.Relationships[0].Columns, "users.user_id")
	assert.Contains(t, response.Relationships[0].Columns, "accounts.owner_id")
}

// TestBuildGetEntityResponse_EmptyData tests the response builder with empty data.
func TestBuildGetEntityResponse_EmptyData(t *testing.T) {
	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "Product",
		PrimaryTable: "products",
		Description:  "A product in the catalog",
	}

	aliases := []*models.OntologyEntityAlias{}
	keyColumns := []*models.OntologyEntityKeyColumn{}
	sourceRels := []*models.EntityRelationship{}
	targetRels := []*models.EntityRelationship{}
	entityMap := map[uuid.UUID]string{entity.ID: "Product"}

	response := buildGetEntityResponse(entity, aliases, keyColumns, sourceRels, targetRels, entityMap)

	assert.Equal(t, "Product", response.Name)
	assert.Equal(t, "products", response.PrimaryTable)
	assert.Equal(t, "A product in the catalog", response.Description)
	assert.Empty(t, response.Aliases)
	assert.Empty(t, response.KeyColumns)
	assert.Empty(t, response.Occurrences)
	assert.Empty(t, response.Relationships)
}

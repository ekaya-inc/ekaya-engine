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
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// TestOntologyToolDeps_Structure verifies the OntologyToolDeps struct has all required fields.
func TestOntologyToolDeps_Structure(t *testing.T) {
	// Create a zero-value instance to verify struct is properly defined
	deps := &OntologyToolDeps{}

	// Verify all fields exist and have correct types
	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.ProjectService, "ProjectService field should be nil by default")
	assert.Nil(t, deps.OntologyRepo, "OntologyRepo field should be nil by default")
	assert.Nil(t, deps.EntityRepo, "EntityRepo field should be nil by default")
	assert.Nil(t, deps.SchemaRepo, "SchemaRepo field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestOntologyToolDeps_Initialization verifies the struct can be initialized with dependencies.
func TestOntologyToolDeps_Initialization(t *testing.T) {
	// Create mock dependencies (just for compilation check)
	var db *database.DB
	var mcpConfigService services.MCPConfigService
	var projectService services.ProjectService
	var ontologyRepo repositories.OntologyRepository
	var entityRepo repositories.OntologyEntityRepository
	var schemaRepo repositories.SchemaRepository
	logger := zap.NewNop()

	// Verify struct can be initialized with all dependencies
	deps := &OntologyToolDeps{
		DB:               db,
		MCPConfigService: mcpConfigService,
		ProjectService:   projectService,
		OntologyRepo:     ontologyRepo,
		EntityRepo:       entityRepo,
		SchemaRepo:       schemaRepo,
		Logger:           logger,
	}

	assert.NotNil(t, deps, "OntologyToolDeps should be initialized")
	assert.Equal(t, logger, deps.Logger, "Logger should be set correctly")
}

// TestRegisterOntologyTools verifies tools are registered with the MCP server.
func TestRegisterOntologyTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &OntologyToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterOntologyTools(mcpServer, deps)

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

	// Check get_ontology tool is registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["get_ontology"], "get_ontology tool should be registered")
}

// TestCheckOntologyToolsEnabled tests the checkOntologyToolsEnabled function.
// Note: Full integration tests with database would require testhelpers.GetEngineDB(t).
// These tests validate error paths that don't require database access.
func TestCheckOntologyToolsEnabled(t *testing.T) {
	tests := []struct {
		name          string
		setupAuth     bool
		projectID     string
		expectError   bool
		errorContains string
	}{
		{
			name:          "missing auth claims",
			setupAuth:     false,
			expectError:   true,
			errorContains: "authentication required",
		},
		{
			name:          "invalid project ID",
			setupAuth:     true,
			projectID:     "invalid-uuid",
			expectError:   true,
			errorContains: "invalid project ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test context
			ctx := context.Background()

			// Setup auth if required
			if tt.setupAuth {
				claims := &auth.Claims{
					ProjectID: tt.projectID,
				}
				ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
			}

			// Create mock dependencies (minimal for error path testing)
			deps := &OntologyToolDeps{
				Logger: zap.NewNop(),
			}

			// Call checkOntologyToolsEnabled
			projectID, tenantCtx, cleanup, err := checkOntologyToolsEnabled(ctx, deps)

			// Verify results
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Equal(t, uuid.Nil, projectID)
			assert.Nil(t, tenantCtx)
			assert.Nil(t, cleanup)
		})
	}
}

// TestGetOntologyTool_HelperFunctions tests the parameter extraction helpers.
func TestGetOntologyTool_HelperFunctions(t *testing.T) {
	t.Run("getStringSlice", func(t *testing.T) {
		// Test with valid string array
		args1 := map[string]any{
			"tables": []any{"users", "orders", "products"},
		}
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: args1,
			},
		}
		result := getStringSlice(req, "tables")
		assert.Equal(t, []string{"users", "orders", "products"}, result)

		// Test with missing key
		result = getStringSlice(req, "nonexistent")
		assert.Nil(t, result)

		// Test with non-array value
		args2 := map[string]any{
			"tables": "not-an-array",
		}
		req = mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: args2,
			},
		}
		result = getStringSlice(req, "tables")
		assert.Nil(t, result)

		// Test with array containing non-strings
		args3 := map[string]any{
			"mixed": []any{"string", 42, true},
		}
		req = mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: args3,
			},
		}
		result = getStringSlice(req, "mixed")
		assert.Equal(t, []string{"string"}, result, "Should filter out non-string values")
	})

	t.Run("getOptionalBoolWithDefault", func(t *testing.T) {
		// Test with explicit true
		args1 := map[string]any{
			"include_relationships": true,
		}
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: args1,
			},
		}
		result := getOptionalBoolWithDefault(req, "include_relationships", false)
		assert.True(t, result)

		// Test with explicit false
		args2 := map[string]any{
			"include_relationships": false,
		}
		req = mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: args2,
			},
		}
		result = getOptionalBoolWithDefault(req, "include_relationships", true)
		assert.False(t, result)

		// Test with missing key (should use default)
		args3 := map[string]any{}
		req = mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: args3,
			},
		}
		result = getOptionalBoolWithDefault(req, "include_relationships", true)
		assert.True(t, result, "Should return default when key is missing")

		result = getOptionalBoolWithDefault(req, "include_relationships", false)
		assert.False(t, result, "Should return default when key is missing")
	})
}

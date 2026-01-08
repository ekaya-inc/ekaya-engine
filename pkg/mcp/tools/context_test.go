package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/mark3labs/mcp-go/server"
)

// TestContextToolDeps_Structure verifies the ContextToolDeps struct has all required fields.
func TestContextToolDeps_Structure(t *testing.T) {
	// Create a zero-value instance to verify struct is properly defined
	deps := &ContextToolDeps{}

	// Verify all fields exist and have correct types
	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.ProjectService, "ProjectService field should be nil by default")
	assert.Nil(t, deps.OntologyContextService, "OntologyContextService field should be nil by default")
	assert.Nil(t, deps.OntologyRepo, "OntologyRepo field should be nil by default")
	assert.Nil(t, deps.SchemaService, "SchemaService field should be nil by default")
	assert.Nil(t, deps.GlossaryService, "GlossaryService field should be nil by default")
	assert.Nil(t, deps.SchemaRepo, "SchemaRepo field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestContextToolDeps_Initialization verifies the struct can be initialized with dependencies.
func TestContextToolDeps_Initialization(t *testing.T) {
	// Create mock dependencies (just for compilation check)
	var db *database.DB
	var mcpConfigService services.MCPConfigService
	var projectService services.ProjectService
	var ontologyContextService services.OntologyContextService
	var ontologyRepo repositories.OntologyRepository
	var schemaService services.SchemaService
	var glossaryService services.GlossaryService
	var schemaRepo repositories.SchemaRepository
	logger := zap.NewNop()

	// Verify struct can be initialized with all dependencies
	deps := &ContextToolDeps{
		DB:                     db,
		MCPConfigService:       mcpConfigService,
		ProjectService:         projectService,
		OntologyContextService: ontologyContextService,
		OntologyRepo:           ontologyRepo,
		SchemaService:          schemaService,
		GlossaryService:        glossaryService,
		SchemaRepo:             schemaRepo,
		Logger:                 logger,
	}

	assert.NotNil(t, deps, "ContextToolDeps should be initialized")
	assert.Equal(t, logger, deps.Logger, "Logger should be set correctly")
}

// TestRegisterContextTools verifies the get_context tool is registered with the MCP server.
func TestRegisterContextTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ContextToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterContextTools(mcpServer, deps)

	// Verify tool is registered
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

	// Check get_context tool is registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["get_context"], "get_context tool should be registered")
}

// TestCheckContextToolsEnabled tests the checkContextToolEnabled function.
// These tests validate error paths that don't require database access.
func TestCheckContextToolsEnabled(t *testing.T) {
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
			deps := &ContextToolDeps{
				Logger: zap.NewNop(),
			}

			// Call checkContextToolEnabled
			projectID, tenantCtx, cleanup, err := checkContextToolEnabled(ctx, deps, "get_context")

			// Verify results
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Equal(t, uuid.Nil, projectID)
			assert.Nil(t, tenantCtx)
			assert.Nil(t, cleanup)
		})
	}
}

// TestDetermineOntologyStatus tests the determineOntologyStatus function.
func TestDetermineOntologyStatus(t *testing.T) {
	tests := []struct {
		name     string
		ontology *models.TieredOntology
		expected string
	}{
		{
			name:     "nil ontology returns none",
			ontology: nil,
			expected: "none",
		},
		{
			name: "active ontology returns complete",
			ontology: &models.TieredOntology{
				IsActive: true,
			},
			expected: "complete",
		},
		{
			name: "inactive ontology returns extracting",
			ontology: &models.TieredOntology{
				IsActive: false,
			},
			expected: "extracting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineOntologyStatus(tt.ontology)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildGlossaryResponse tests the buildGlossaryResponse function.
func TestBuildGlossaryResponse(t *testing.T) {
	tests := []struct {
		name     string
		terms    []*models.BusinessGlossaryTerm
		expected int // expected number of terms
	}{
		{
			name:     "empty terms returns empty array",
			terms:    []*models.BusinessGlossaryTerm{},
			expected: 0,
		},
		{
			name:     "nil terms returns empty array",
			terms:    nil,
			expected: 0,
		},
		{
			name: "single term",
			terms: []*models.BusinessGlossaryTerm{
				{
					Term:        "Revenue",
					Definition:  "Total revenue",
					DefiningSQL: "SELECT SUM(amount) FROM orders",
					Aliases:     []string{"Total Revenue"},
				},
			},
			expected: 1,
		},
		{
			name: "multiple terms",
			terms: []*models.BusinessGlossaryTerm{
				{
					Term:       "Revenue",
					Definition: "Total revenue",
				},
				{
					Term:       "GMV",
					Definition: "Gross Merchandise Value",
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGlossaryResponse(tt.terms)
			assert.Equal(t, tt.expected, len(result))

			// Verify structure of first term if exists
			if len(result) > 0 && len(tt.terms) > 0 {
				assert.Equal(t, tt.terms[0].Term, result[0]["term"])
				assert.Equal(t, tt.terms[0].Definition, result[0]["definition"])

				// Check optional fields
				if len(tt.terms[0].Aliases) > 0 {
					assert.NotNil(t, result[0]["aliases"])
				}
				if tt.terms[0].DefiningSQL != "" {
					assert.Equal(t, tt.terms[0].DefiningSQL, result[0]["sql_pattern"])
				}
			}
		})
	}
}

// TestFilterDatasourceTables tests the filterDatasourceTables function.
func TestFilterDatasourceTables(t *testing.T) {
	tables := []*models.DatasourceTable{
		{
			SchemaName: "public",
			TableName:  "users",
		},
		{
			SchemaName: "public",
			TableName:  "orders",
		},
		{
			SchemaName: "analytics",
			TableName:  "reports",
		},
	}

	tests := []struct {
		name       string
		tables     []*models.DatasourceTable
		filter     []string
		expectLen  int
		expectName string // name of first table in result
	}{
		{
			name:       "no filter returns all tables",
			tables:     tables,
			filter:     []string{},
			expectLen:  3,
			expectName: "users",
		},
		{
			name:       "filter by table name",
			tables:     tables,
			filter:     []string{"users"},
			expectLen:  1,
			expectName: "users",
		},
		{
			name:       "filter by fully qualified name",
			tables:     tables,
			filter:     []string{"public.orders"},
			expectLen:  1,
			expectName: "orders",
		},
		{
			name:       "filter multiple tables",
			tables:     tables,
			filter:     []string{"users", "analytics.reports"},
			expectLen:  2,
			expectName: "users",
		},
		{
			name:      "filter no match returns empty",
			tables:    tables,
			filter:    []string{"nonexistent"},
			expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDatasourceTables(tt.tables, tt.filter)
			assert.Equal(t, tt.expectLen, len(result))

			if tt.expectLen > 0 {
				assert.Equal(t, tt.expectName, result[0].TableName)
			}
		})
	}
}

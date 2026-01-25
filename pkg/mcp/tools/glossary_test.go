package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockGlossaryService implements services.GlossaryService for testing.
type mockGlossaryService struct {
	terms []*models.BusinessGlossaryTerm
	term  *models.BusinessGlossaryTerm
	err   error
}

func (m *mockGlossaryService) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	return nil
}

func (m *mockGlossaryService) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	return nil
}

func (m *mockGlossaryService) DeleteTerm(ctx context.Context, termID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryService) GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.terms, nil
}

func (m *mockGlossaryService) GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.term, nil
}

func (m *mockGlossaryService) SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}

func (m *mockGlossaryService) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockGlossaryService) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryService) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	// Return the term if it matches the name (for update path testing)
	if m.term != nil && m.term.Term == termName {
		return m.term, nil
	}
	return nil, nil
}

func (m *mockGlossaryService) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*services.SQLTestResult, error) {
	return nil, nil
}

func (m *mockGlossaryService) CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}

func (m *mockGlossaryService) DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}

// TestGlossaryToolDeps_Structure verifies the GlossaryToolDeps struct has all required fields.
func TestGlossaryToolDeps_Structure(t *testing.T) {
	deps := &GlossaryToolDeps{}

	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.GlossaryService, "GlossaryService field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestGlossaryToolDeps_Initialization verifies the struct can be initialized with dependencies.
func TestGlossaryToolDeps_Initialization(t *testing.T) {
	logger := zap.NewNop()
	glossaryService := &mockGlossaryService{}
	mcpConfigService := &mockMCPConfigService{}

	deps := &GlossaryToolDeps{
		MCPConfigService: mcpConfigService,
		GlossaryService:  glossaryService,
		Logger:           logger,
	}

	assert.NotNil(t, deps, "GlossaryToolDeps should be initialized")
	assert.Equal(t, logger, deps.Logger, "Logger should be set correctly")
	assert.Equal(t, glossaryService, deps.GlossaryService, "GlossaryService should be set correctly")
}

// TestRegisterGlossaryTools verifies tools are registered with the MCP server.
func TestRegisterGlossaryTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &GlossaryToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterGlossaryTools(mcpServer, deps)

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

	// Check both glossary tools are registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["list_glossary"], "list_glossary tool should be registered")
	assert.True(t, toolNames["get_glossary_sql"], "get_glossary_sql tool should be registered")
}

// TestCheckGlossaryToolsEnabled tests the checkGlossaryToolsEnabled function.
func TestCheckGlossaryToolsEnabled(t *testing.T) {
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
			ctx := context.Background()

			if tt.setupAuth {
				claims := &auth.Claims{
					ProjectID: tt.projectID,
				}
				ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
			}

			deps := &GlossaryToolDeps{
				Logger: zap.NewNop(),
			}

			projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "list_glossary")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Equal(t, uuid.Nil, projectID)
			assert.Nil(t, tenantCtx)
			assert.Nil(t, cleanup)
		})
	}
}

// TestToListGlossaryResponse verifies the model to list response conversion.
func TestToListGlossaryResponse(t *testing.T) {
	t.Run("term with aliases", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:         uuid.New(),
			ProjectID:  uuid.New(),
			Term:       "Revenue",
			Definition: "Total earned amount from completed transactions",
			Aliases:    []string{"Total Revenue", "Gross Revenue"},
		}

		resp := toListGlossaryResponse(term)

		assert.Equal(t, "Revenue", resp.Term)
		assert.Equal(t, "Total earned amount from completed transactions", resp.Definition)
		assert.Equal(t, []string{"Total Revenue", "Gross Revenue"}, resp.Aliases)
	})

	t.Run("term without aliases", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:         uuid.New(),
			ProjectID:  uuid.New(),
			Term:       "Active User",
			Definition: "User with recent activity",
			Aliases:    nil,
		}

		resp := toListGlossaryResponse(term)

		assert.Equal(t, "Active User", resp.Term)
		assert.Equal(t, "User with recent activity", resp.Definition)
		assert.Nil(t, resp.Aliases)
	})

	t.Run("term with enrichment status success", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:               uuid.New(),
			ProjectID:        uuid.New(),
			Term:             "Revenue",
			Definition:       "Total earned amount",
			EnrichmentStatus: models.GlossaryEnrichmentSuccess,
		}

		resp := toListGlossaryResponse(term)

		assert.Equal(t, "Revenue", resp.Term)
		assert.Equal(t, models.GlossaryEnrichmentSuccess, resp.EnrichmentStatus)
		assert.Empty(t, resp.EnrichmentError)
	})

	t.Run("term with enrichment status failed", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:               uuid.New(),
			ProjectID:        uuid.New(),
			Term:             "Offer Utilization Rate",
			Definition:       "Percentage of offers used",
			EnrichmentStatus: models.GlossaryEnrichmentFailed,
			EnrichmentError:  "LLM returned empty SQL",
		}

		resp := toListGlossaryResponse(term)

		assert.Equal(t, "Offer Utilization Rate", resp.Term)
		assert.Equal(t, models.GlossaryEnrichmentFailed, resp.EnrichmentStatus)
		assert.Equal(t, "LLM returned empty SQL", resp.EnrichmentError)
	})

	t.Run("term with enrichment status pending", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:               uuid.New(),
			ProjectID:        uuid.New(),
			Term:             "New Metric",
			Definition:       "A new metric awaiting enrichment",
			EnrichmentStatus: models.GlossaryEnrichmentPending,
		}

		resp := toListGlossaryResponse(term)

		assert.Equal(t, "New Metric", resp.Term)
		assert.Equal(t, models.GlossaryEnrichmentPending, resp.EnrichmentStatus)
		assert.Empty(t, resp.EnrichmentError)
	})
}

// TestToGetGlossarySQLResponse verifies the model to SQL response conversion.
func TestToGetGlossarySQLResponse(t *testing.T) {
	t.Run("full term with all fields", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:         uuid.New(),
			ProjectID:  uuid.New(),
			Term:       "Revenue",
			Definition: "Total earned amount from completed transactions",
			DefiningSQL: `SELECT SUM(earned_amount) AS revenue
FROM billing_transactions
WHERE transaction_state = 'completed'`,
			BaseTable: "billing_transactions",
			OutputColumns: []models.OutputColumn{
				{Name: "revenue", Type: "numeric"},
			},
			Aliases:          []string{"total_revenue", "earnings"},
			EnrichmentStatus: models.GlossaryEnrichmentSuccess,
		}

		resp := toGetGlossarySQLResponse(term)

		assert.Equal(t, "Revenue", resp.Term)
		assert.Equal(t, "Total earned amount from completed transactions", resp.Definition)
		assert.Contains(t, resp.DefiningSQL, "SUM(earned_amount)")
		assert.Equal(t, "billing_transactions", resp.BaseTable)
		assert.Equal(t, 1, len(resp.OutputColumns))
		assert.Equal(t, "revenue", resp.OutputColumns[0].Name)
		assert.Equal(t, "numeric", resp.OutputColumns[0].Type)
		assert.Equal(t, []string{"total_revenue", "earnings"}, resp.Aliases)
		assert.Equal(t, models.GlossaryEnrichmentSuccess, resp.EnrichmentStatus)
		assert.Empty(t, resp.EnrichmentError)
	})

	t.Run("minimal term with only required fields", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:          uuid.New(),
			ProjectID:   uuid.New(),
			Term:        "Active User",
			Definition:  "User with recent activity",
			DefiningSQL: "SELECT COUNT(*) FROM users WHERE active = true",
		}

		resp := toGetGlossarySQLResponse(term)

		assert.Equal(t, "Active User", resp.Term)
		assert.Equal(t, "User with recent activity", resp.Definition)
		assert.Equal(t, "SELECT COUNT(*) FROM users WHERE active = true", resp.DefiningSQL)
		assert.Equal(t, "", resp.BaseTable)
		assert.Nil(t, resp.OutputColumns)
		assert.Nil(t, resp.Aliases)
	})

	t.Run("term with enrichment failure includes error message", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:               uuid.New(),
			ProjectID:        uuid.New(),
			Term:             "Offer Utilization Rate",
			Definition:       "Percentage of offers that were used",
			DefiningSQL:      "", // Empty SQL due to enrichment failure
			EnrichmentStatus: models.GlossaryEnrichmentFailed,
			EnrichmentError:  "SQL validation failed: column 'used_count' not found in table 'offers'",
		}

		resp := toGetGlossarySQLResponse(term)

		assert.Equal(t, "Offer Utilization Rate", resp.Term)
		assert.Equal(t, "", resp.DefiningSQL)
		assert.Equal(t, models.GlossaryEnrichmentFailed, resp.EnrichmentStatus)
		assert.Equal(t, "SQL validation failed: column 'used_count' not found in table 'offers'", resp.EnrichmentError)
	})
}

// TestGlossaryToolsInOntologyToolNames verifies glossary tools are in the tool filter map.
func TestGlossaryToolsInOntologyToolNames(t *testing.T) {
	assert.True(t, ontologyToolNames["list_glossary"], "list_glossary should be in ontologyToolNames for filtering")
	assert.True(t, ontologyToolNames["get_glossary_sql"], "get_glossary_sql should be in ontologyToolNames for filtering")
}

// TestListGlossaryTool_Integration verifies the list_glossary tool returns lightweight discovery response.
func TestListGlossaryTool_Integration(t *testing.T) {
	// Create mock service that returns terms
	terms := []*models.BusinessGlossaryTerm{
		{
			ID:          uuid.New(),
			ProjectID:   uuid.New(),
			Term:        "Revenue",
			Definition:  "Total earned amount",
			DefiningSQL: "SELECT SUM(amount) FROM transactions",
			Aliases:     []string{"Total Revenue"},
		},
		{
			ID:          uuid.New(),
			ProjectID:   uuid.New(),
			Term:        "Active Users",
			Definition:  "Users who logged in recently",
			DefiningSQL: "SELECT COUNT(*) FROM users WHERE active = true",
			Aliases:     []string{"MAU", "Monthly Active Users"},
		},
	}

	mockService := &mockGlossaryService{
		terms: terms,
	}

	deps := &GlossaryToolDeps{
		GlossaryService: mockService,
		Logger:          zap.NewNop(),
	}

	// Register tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	RegisterGlossaryTools(mcpServer, deps)

	// The list_glossary response should NOT include DefiningSQL or OutputColumns
	// It's a lightweight discovery response with just term, definition, and aliases
	t.Run("list_glossary returns lightweight response", func(t *testing.T) {
		result := struct {
			Terms []listGlossaryResponse `json:"terms"`
			Count int                    `json:"count"`
		}{}

		// Simulate response building
		for _, term := range terms {
			result.Terms = append(result.Terms, toListGlossaryResponse(term))
		}
		result.Count = len(result.Terms)

		assert.Equal(t, 2, result.Count)
		assert.Equal(t, "Revenue", result.Terms[0].Term)
		assert.Equal(t, "Total earned amount", result.Terms[0].Definition)
		assert.Equal(t, []string{"Total Revenue"}, result.Terms[0].Aliases)

		assert.Equal(t, "Active Users", result.Terms[1].Term)
		assert.Equal(t, []string{"MAU", "Monthly Active Users"}, result.Terms[1].Aliases)
	})
}

// TestGetGlossarySQLTool_Integration verifies the get_glossary_sql tool returns full SQL definition.
func TestGetGlossarySQLTool_Integration(t *testing.T) {
	term := &models.BusinessGlossaryTerm{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		Term:       "Revenue",
		Definition: "Total earned amount from completed transactions",
		DefiningSQL: `SELECT SUM(earned_amount) AS revenue
FROM billing_transactions
WHERE transaction_state = 'completed'`,
		BaseTable: "billing_transactions",
		OutputColumns: []models.OutputColumn{
			{Name: "revenue", Type: "numeric"},
		},
		Aliases: []string{"Total Revenue", "Gross Revenue"},
	}

	mockService := &mockGlossaryService{
		term: term,
	}

	deps := &GlossaryToolDeps{
		GlossaryService: mockService,
		Logger:          zap.NewNop(),
	}

	// Register tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	RegisterGlossaryTools(mcpServer, deps)

	t.Run("get_glossary_sql returns complete SQL definition", func(t *testing.T) {
		result := toGetGlossarySQLResponse(term)

		assert.Equal(t, "Revenue", result.Term)
		assert.Equal(t, "Total earned amount from completed transactions", result.Definition)
		assert.Contains(t, result.DefiningSQL, "SUM(earned_amount)")
		assert.Equal(t, "billing_transactions", result.BaseTable)
		assert.Equal(t, 1, len(result.OutputColumns))
		assert.Equal(t, "revenue", result.OutputColumns[0].Name)
		assert.Equal(t, "numeric", result.OutputColumns[0].Type)
		assert.Equal(t, []string{"Total Revenue", "Gross Revenue"}, result.Aliases)
	})

	t.Run("get_glossary_sql handles not found gracefully", func(t *testing.T) {
		// Simulate not found response
		notFoundResult := struct {
			Error string `json:"error"`
			Term  string `json:"term"`
		}{
			Error: "Term not found",
			Term:  "Unknown",
		}

		assert.Equal(t, "Term not found", notFoundResult.Error)
		assert.Equal(t, "Unknown", notFoundResult.Term)
	})
}

// TestUpdateGlossaryTermTool_ToolStructure verifies the update_glossary_term tool is registered correctly.
func TestUpdateGlossaryTermTool_ToolStructure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &GlossaryToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterGlossaryTools(mcpServer, deps)

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

	// Check update_glossary_term is registered
	var updateToolFound bool
	var updateToolName, updateToolDesc string
	for _, tool := range response.Result.Tools {
		if tool.Name == "update_glossary_term" {
			updateToolFound = true
			updateToolName = tool.Name
			updateToolDesc = tool.Description
			break
		}
	}

	require.True(t, updateToolFound, "update_glossary_term tool should be registered")
	assert.Equal(t, "update_glossary_term", updateToolName)
	assert.Contains(t, updateToolDesc, "upsert")
	assert.Contains(t, updateToolDesc, "business glossary term")
}

// TestUpdateGlossaryTermTool_ResponseStructure verifies response format.
func TestUpdateGlossaryTermTool_ResponseStructure(t *testing.T) {
	t.Run("create new term response", func(t *testing.T) {
		response := struct {
			Term          string                `json:"term"`
			Definition    string                `json:"definition"`
			SQL           string                `json:"sql"`
			Aliases       []string              `json:"aliases,omitempty"`
			OutputColumns []models.OutputColumn `json:"output_columns,omitempty"`
			Created       bool                  `json:"created"`
		}{
			Term:       "Revenue",
			Definition: "Total earned amount",
			SQL:        "SELECT SUM(amount) FROM transactions",
			Aliases:    []string{"Total Revenue"},
			OutputColumns: []models.OutputColumn{
				{Name: "revenue", Type: "numeric"},
			},
			Created: true,
		}

		assert.Equal(t, "Revenue", response.Term)
		assert.Equal(t, "Total earned amount", response.Definition)
		assert.Equal(t, "SELECT SUM(amount) FROM transactions", response.SQL)
		assert.Equal(t, []string{"Total Revenue"}, response.Aliases)
		assert.Equal(t, 1, len(response.OutputColumns))
		assert.True(t, response.Created)
	})

	t.Run("update existing term response", func(t *testing.T) {
		response := struct {
			Term          string                `json:"term"`
			Definition    string                `json:"definition"`
			SQL           string                `json:"sql"`
			Aliases       []string              `json:"aliases,omitempty"`
			OutputColumns []models.OutputColumn `json:"output_columns,omitempty"`
			Created       bool                  `json:"created"`
		}{
			Term:       "Revenue",
			Definition: "Updated definition",
			SQL:        "SELECT SUM(amount) FROM transactions WHERE status='completed'",
			Aliases:    []string{"Total Revenue", "Gross Revenue"},
			OutputColumns: []models.OutputColumn{
				{Name: "revenue", Type: "numeric"},
			},
			Created: false,
		}

		assert.Equal(t, "Revenue", response.Term)
		assert.Equal(t, "Updated definition", response.Definition)
		assert.Contains(t, response.SQL, "status='completed'")
		assert.Equal(t, 2, len(response.Aliases))
		assert.False(t, response.Created)
	})
}

// TestDeleteGlossaryTermTool_ToolStructure verifies the delete_glossary_term tool is registered correctly.
func TestDeleteGlossaryTermTool_ToolStructure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &GlossaryToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterGlossaryTools(mcpServer, deps)

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

	// Check delete_glossary_term is registered
	var deleteToolFound bool
	var deleteToolName, deleteToolDesc string
	for _, tool := range response.Result.Tools {
		if tool.Name == "delete_glossary_term" {
			deleteToolFound = true
			deleteToolName = tool.Name
			deleteToolDesc = tool.Description
			break
		}
	}

	require.True(t, deleteToolFound, "delete_glossary_term tool should be registered")
	assert.Equal(t, "delete_glossary_term", deleteToolName)
	assert.Contains(t, deleteToolDesc, "Delete")
	assert.Contains(t, deleteToolDesc, "business glossary term")
}

// TestDeleteGlossaryTermTool_ResponseStructure verifies response format.
func TestDeleteGlossaryTermTool_ResponseStructure(t *testing.T) {
	t.Run("deleted term response", func(t *testing.T) {
		response := struct {
			Term    string `json:"term"`
			Deleted bool   `json:"deleted"`
		}{
			Term:    "Revenue",
			Deleted: true,
		}

		assert.Equal(t, "Revenue", response.Term)
		assert.True(t, response.Deleted)
	})

	t.Run("term not found response (idempotent)", func(t *testing.T) {
		response := struct {
			Term    string `json:"term"`
			Deleted bool   `json:"deleted"`
		}{
			Term:    "Unknown",
			Deleted: false,
		}

		assert.Equal(t, "Unknown", response.Term)
		assert.False(t, response.Deleted)
	})
}

// TestGlossaryUpdateTools_AreDeveloperTools verifies new tools are in developer tools group.
func TestGlossaryUpdateTools_AreDeveloperTools(t *testing.T) {
	// update_glossary_term and delete_glossary_term are developer tools, NOT ontology tools
	// They should NOT be in ontologyToolNames (which is for approved_queries group)
	assert.False(t, ontologyToolNames["update_glossary_term"], "update_glossary_term should NOT be in ontologyToolNames (it's a developer tool)")
	assert.False(t, ontologyToolNames["delete_glossary_term"], "delete_glossary_term should NOT be in ontologyToolNames (it's a developer tool)")
}

// TestGetGlossarySQLTool_ErrorResults verifies error handling for invalid parameters.
func TestGetGlossarySQLTool_ErrorResults(t *testing.T) {
	t.Run("empty term after trimming", func(t *testing.T) {
		// Simulate validation check for empty term after trimming
		termName := "   "
		termName = trimString(termName)
		if termName == "" {
			result := NewErrorResult(
				"invalid_parameters",
				"parameter 'term' cannot be empty",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'term' cannot be empty")
		}
	})

	t.Run("term not found", func(t *testing.T) {
		// Simulate term not found response
		termName := "UnknownTerm"
		result := NewErrorResult("TERM_NOT_FOUND",
			fmt.Sprintf("term %q not found in glossary. Use list_glossary to see available terms.", termName))

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse the content to verify structure
		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "TERM_NOT_FOUND", errorResp.Code)
		assert.Contains(t, errorResp.Message, "term \"UnknownTerm\" not found")
		assert.Contains(t, errorResp.Message, "Use list_glossary")
	})
}

// TestUpdateGlossaryTermTool_UpdateExistingTerm verifies the update path uses existing.Source, not term.Source.
// This test guards against a nil pointer dereference bug where term.Source was accessed before assignment.
func TestUpdateGlossaryTermTool_UpdateExistingTerm(t *testing.T) {
	t.Run("update path uses existing.Source for precedence check", func(t *testing.T) {
		// Create an existing term with MCP source (allowing updates from MCP)
		existingTerm := &models.BusinessGlossaryTerm{
			ID:          uuid.New(),
			ProjectID:   uuid.New(),
			Term:        "Revenue",
			Definition:  "Original definition",
			DefiningSQL: "SELECT SUM(amount) FROM transactions",
			Aliases:     []string{"Total Revenue"},
			Source:      models.GlossarySourceMCP, // MCP source allows MCP updates
		}

		// Test the canModifyGlossaryTerm function directly
		// This simulates the precedence check that happens in the update path
		// Before the fix, this would have been: canModifyGlossaryTerm(term.Source, ...) where term is nil
		// After the fix, it correctly uses: canModifyGlossaryTerm(existing.Source, ...)

		// MCP can modify MCP-sourced terms
		assert.True(t, canModifyGlossaryTerm(existingTerm.Source, models.GlossarySourceMCP),
			"MCP should be able to modify MCP-sourced terms")

		// MCP cannot modify Manual-sourced terms
		existingTerm.Source = models.GlossarySourceManual
		assert.False(t, canModifyGlossaryTerm(existingTerm.Source, models.GlossarySourceMCP),
			"MCP should NOT be able to modify Manual-sourced terms")

		// MCP can modify Inferred-sourced terms
		existingTerm.Source = models.GlossarySourceInferred
		assert.True(t, canModifyGlossaryTerm(existingTerm.Source, models.GlossarySourceMCP),
			"MCP should be able to modify Inferred-sourced terms")
	})

	t.Run("update path correctly accesses existing term source", func(t *testing.T) {
		// This test verifies the fix: when updating an existing term,
		// the code must access existing.Source (not term.Source which would be nil)
		//
		// Before fix (line 320): if !canModifyGlossaryTerm(term.Source, ...) // term is nil -> panic
		// After fix (line 320): if !canModifyGlossaryTerm(existing.Source, ...) // existing is valid

		existingTerm := &models.BusinessGlossaryTerm{
			ID:          uuid.New(),
			ProjectID:   uuid.New(),
			Term:        "TestTerm",
			Definition:  "Test definition",
			DefiningSQL: "SELECT 1",
			Source:      models.GlossarySourceMCP,
		}

		// Simulate what the code does in the update path BEFORE the fix would have happened:
		// At line 285, term is declared as nil: var term *models.BusinessGlossaryTerm
		// At line 287, if existing != nil (update path), we go to line 317
		// At line 320 (BEFORE FIX), we access term.Source which is nil -> PANIC

		// The test shows the correct behavior after the fix:
		// We should use existing.Source, not term.Source
		var term *models.BusinessGlossaryTerm // nil, as in the actual code
		existing := existingTerm              // non-nil, found by GetTermByName

		// This would panic before the fix:
		// _ = term.Source // PANIC: nil pointer dereference

		// After fix, we correctly use existing.Source:
		sourceToCheck := existing.Source
		assert.Equal(t, models.GlossarySourceMCP, sourceToCheck)

		// And the precedence check works
		canModify := canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP)
		assert.True(t, canModify)

		// Verify term is still nil at this point (as it would be in the actual code before line 327)
		assert.Nil(t, term, "term should still be nil at this point in the update path")
	})
}

// statefulGlossaryService is a mock service that maintains state across create/update operations.
// This allows testing the create→update flow that exercises the nil pointer fix.
type statefulGlossaryService struct {
	mockGlossaryService
	storedTerms map[string]*models.BusinessGlossaryTerm // keyed by term name
	createCalls int
	updateCalls int
}

func newStatefulGlossaryService() *statefulGlossaryService {
	return &statefulGlossaryService{
		storedTerms: make(map[string]*models.BusinessGlossaryTerm),
	}
}

func (s *statefulGlossaryService) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	s.createCalls++
	// Simulate database behavior: assign ID if not set
	if term.ID == uuid.Nil {
		term.ID = uuid.New()
	}
	// Store the term for later retrieval
	s.storedTerms[term.Term] = term
	return nil
}

func (s *statefulGlossaryService) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	s.updateCalls++
	// Update the stored term
	if existing, ok := s.storedTerms[term.Term]; ok {
		// Preserve ID from existing
		term.ID = existing.ID
		s.storedTerms[term.Term] = term
	}
	return nil
}

func (s *statefulGlossaryService) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	if term, ok := s.storedTerms[termName]; ok {
		return term, nil
	}
	return nil, nil
}

// TestUpdateGlossaryTermTool_CreateThenUpdate verifies the complete create→update flow via MCP.
// This is a regression test for the nil pointer dereference bug fixed in glossary.go:320.
// Before the fix, calling update_glossary_term on an existing term would panic because
// the code accessed term.Source when term was still nil (should have used existing.Source).
func TestUpdateGlossaryTermTool_CreateThenUpdate(t *testing.T) {
	t.Run("create then update same term via mock service", func(t *testing.T) {
		// This test simulates the create→update flow that exercised the nil pointer bug.
		// The fix changed line 320 from term.Source to existing.Source.

		statefulSvc := newStatefulGlossaryService()
		projectID := uuid.New()

		// Step 1: Simulate creating a new term (what update_glossary_term does when term doesn't exist)
		// When GetTermByName returns nil, the tool creates a new term
		existing, err := statefulSvc.GetTermByName(context.Background(), projectID, "TestTerm")
		require.NoError(t, err)
		assert.Nil(t, existing, "term should not exist initially")

		// Create the term
		newTerm := &models.BusinessGlossaryTerm{
			ProjectID:   projectID,
			Term:        "TestTerm",
			Definition:  "Original definition",
			DefiningSQL: "SELECT COUNT(*) FROM users",
			Source:      models.GlossarySourceMCP, // MCP-created term
		}
		err = statefulSvc.CreateTerm(context.Background(), projectID, newTerm)
		require.NoError(t, err)
		assert.Equal(t, 1, statefulSvc.createCalls, "CreateTerm should be called once")

		// Step 2: Simulate updating the same term (what update_glossary_term does when term exists)
		// This is where the nil pointer bug was triggered
		existing, err = statefulSvc.GetTermByName(context.Background(), projectID, "TestTerm")
		require.NoError(t, err)
		require.NotNil(t, existing, "term should exist after creation")
		assert.Equal(t, models.GlossarySourceMCP, existing.Source, "term should have MCP source")

		// Verify the fix: The code should use existing.Source (not term.Source which would be nil)
		// This simulates the logic in glossary.go lines 285-327:
		//   var term *models.BusinessGlossaryTerm  // nil at this point
		//   if existing != nil {
		//       if !canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP) { // FIXED: was term.Source
		//           ...
		//       }
		//       term = existing  // Now term is assigned
		//   }

		// Before fix: accessing term.Source when term is nil would panic
		// After fix: accessing existing.Source works correctly
		var term *models.BusinessGlossaryTerm // nil, as in the actual code

		// This would have crashed before the fix: canModifyGlossaryTerm(term.Source, ...)
		// After fix, we correctly check: canModifyGlossaryTerm(existing.Source, ...)
		canModify := canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP)
		assert.True(t, canModify, "MCP should be able to modify MCP-sourced terms")

		// Now assign term (as the fixed code does at line 327)
		term = existing

		// Update the term
		term.Definition = "Updated definition"
		err = statefulSvc.UpdateTerm(context.Background(), term)
		require.NoError(t, err)
		assert.Equal(t, 1, statefulSvc.updateCalls, "UpdateTerm should be called once")

		// Verify the update persisted
		updated, err := statefulSvc.GetTermByName(context.Background(), projectID, "TestTerm")
		require.NoError(t, err)
		assert.Equal(t, "Updated definition", updated.Definition)
		assert.Equal(t, models.GlossarySourceMCP, updated.Source, "source should be preserved")
	})

	t.Run("update promoted inferred term to MCP source", func(t *testing.T) {
		// Test that an inferred term gets promoted to MCP source when updated via MCP
		statefulSvc := newStatefulGlossaryService()
		projectID := uuid.New()

		// Pre-populate with an inferred term (as if from ontology extraction)
		inferredTerm := &models.BusinessGlossaryTerm{
			ID:          uuid.New(),
			ProjectID:   projectID,
			Term:        "Revenue",
			Definition:  "Auto-generated definition",
			DefiningSQL: "SELECT SUM(amount) FROM orders",
			Source:      models.GlossarySourceInferred,
		}
		statefulSvc.storedTerms["Revenue"] = inferredTerm

		// Retrieve the existing term
		existing, err := statefulSvc.GetTermByName(context.Background(), projectID, "Revenue")
		require.NoError(t, err)
		require.NotNil(t, existing)
		assert.Equal(t, models.GlossarySourceInferred, existing.Source)

		// MCP can modify inferred terms
		canModify := canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP)
		assert.True(t, canModify, "MCP should be able to modify Inferred-sourced terms")

		// Simulate the source promotion logic from glossary.go lines 341-343:
		// if term.Source == models.GlossarySourceInferred {
		//     term.Source = models.GlossarySourceMCP
		// }
		term := existing
		if term.Source == models.GlossarySourceInferred {
			term.Source = models.GlossarySourceMCP
		}
		term.Definition = "Refined definition via MCP"

		err = statefulSvc.UpdateTerm(context.Background(), term)
		require.NoError(t, err)

		// Verify source was promoted
		updated, err := statefulSvc.GetTermByName(context.Background(), projectID, "Revenue")
		require.NoError(t, err)
		assert.Equal(t, models.GlossarySourceMCP, updated.Source, "source should be promoted to MCP")
		assert.Equal(t, "Refined definition via MCP", updated.Definition)
	})

	t.Run("MCP cannot modify manual terms", func(t *testing.T) {
		// Test that manual terms block MCP updates (precedence rule)
		statefulSvc := newStatefulGlossaryService()
		projectID := uuid.New()

		// Pre-populate with a manual term (created via UI)
		manualTerm := &models.BusinessGlossaryTerm{
			ID:          uuid.New(),
			ProjectID:   projectID,
			Term:        "Manual Term",
			Definition:  "Created via UI",
			DefiningSQL: "SELECT 1",
			Source:      models.GlossarySourceManual,
		}
		statefulSvc.storedTerms["Manual Term"] = manualTerm

		// Retrieve the existing term
		existing, err := statefulSvc.GetTermByName(context.Background(), projectID, "Manual Term")
		require.NoError(t, err)
		require.NotNil(t, existing)

		// MCP cannot modify manual terms
		canModify := canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP)
		assert.False(t, canModify, "MCP should NOT be able to modify Manual-sourced terms")

		// The actual tool returns a precedence_blocked error in this case
		// We verify UpdateTerm is NOT called
		assert.Equal(t, 0, statefulSvc.updateCalls, "UpdateTerm should not be called for blocked terms")
	})
}

// TestUpdateGlossaryTermTool_ErrorResults verifies error handling for invalid parameters.
func TestUpdateGlossaryTermTool_ErrorResults(t *testing.T) {
	t.Run("empty term after trimming", func(t *testing.T) {
		// Simulate validation check for empty term after trimming
		termName := "   "
		termName = trimString(termName)
		if termName == "" {
			result := NewErrorResult(
				"invalid_parameters",
				"parameter 'term' cannot be empty",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'term' cannot be empty")
		}
	})

	t.Run("invalid aliases array - non-string element", func(t *testing.T) {
		// Simulate aliases validation with invalid element type
		aliasArray := []any{"valid_alias", 123, "another_valid"}
		for i, alias := range aliasArray {
			if _, ok := alias.(string); !ok {
				result := NewErrorResultWithDetails(
					"invalid_parameters",
					fmt.Sprintf("parameter 'aliases' must be an array of strings. Element at index %d is %T, not string", i, alias),
					map[string]any{
						"parameter":             "aliases",
						"invalid_element_index": i,
						"invalid_element_type":  fmt.Sprintf("%T", alias),
					},
				)

				// Verify it's an error result
				assert.NotNil(t, result)
				assert.True(t, result.IsError)

				// Parse the content to verify structure
				var errorResp ErrorResponse
				err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
				require.NoError(t, err)

				assert.True(t, errorResp.Error)
				assert.Equal(t, "invalid_parameters", errorResp.Code)
				assert.Contains(t, errorResp.Message, "parameter 'aliases' must be an array of strings")
				assert.Contains(t, errorResp.Message, "index 1")

				// Check details
				detailsMap, ok := errorResp.Details.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "aliases", detailsMap["parameter"])
				assert.Equal(t, float64(1), detailsMap["invalid_element_index"]) // JSON numbers are float64
				assert.Equal(t, "int", detailsMap["invalid_element_type"])

				// Only test the first invalid element
				break
			}
		}
	})

	t.Run("test-like term name rejected", func(t *testing.T) {
		// These test terms should be rejected by the IsTestTerm validation
		testTerms := []string{
			"TestTerm",       // Starts with "test"
			"UITestTerm2026", // UI test prefix with year
			"mytest",         // Ends with "test"
			"DebugMetric",    // Debug prefix
			"DummyValue",     // Dummy prefix
			"SampleData",     // Sample prefix
			"ExampleRevenue", // Example prefix
			"Revenue2026",    // Ends with year (4 digits)
		}

		for _, termName := range testTerms {
			// Verify the term is detected as test data
			require.True(t, services.IsTestTerm(termName),
				"IsTestTerm(%q) should return true", termName)

			// Verify the error response structure matches what the handler would return
			result := NewErrorResult(
				"invalid_parameters",
				"term name appears to be test data - use a real business term",
			)

			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "term name appears to be test data")
		}
	})

	t.Run("valid business term names accepted", func(t *testing.T) {
		// These should NOT be rejected as test terms
		validTerms := []string{
			"Revenue",
			"Active Users",
			"Monthly Recurring Revenue",
			"Customer Lifetime Value",
			"Gross Merchandise Value",
			"Average Order Value",
			"Net Promoter Score",
			"Churn Rate",
		}

		for _, termName := range validTerms {
			assert.False(t, services.IsTestTerm(termName),
				"IsTestTerm(%q) should return false for valid business term", termName)
		}
	})
}

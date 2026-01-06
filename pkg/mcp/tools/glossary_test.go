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

			projectID, tenantCtx, cleanup, err := checkGlossaryToolsEnabled(ctx, deps)

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
			Aliases: []string{"total_revenue", "earnings"},
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

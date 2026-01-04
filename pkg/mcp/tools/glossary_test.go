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

	// Check get_glossary tool is registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["get_glossary"], "get_glossary tool should be registered")
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

// TestToGlossaryTermResponse verifies the model to response conversion.
func TestToGlossaryTermResponse(t *testing.T) {
	t.Run("full term with all fields", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:          uuid.New(),
			ProjectID:   uuid.New(),
			Term:        "Revenue",
			Definition:  "Total earned amount from completed transactions",
			SQLPattern:  "SUM(earned_amount) WHERE transaction_state = 'completed'",
			BaseTable:   "billing_transactions",
			ColumnsUsed: []string{"earned_amount", "transaction_state"},
			Filters: []models.Filter{
				{Column: "transaction_state", Operator: "=", Values: []string{"completed"}},
			},
			Aggregation: "SUM",
			Source:      "user",
		}

		resp := toGlossaryTermResponse(term)

		assert.Equal(t, "Revenue", resp.Term)
		assert.Equal(t, "Total earned amount from completed transactions", resp.Definition)
		assert.Equal(t, "SUM(earned_amount) WHERE transaction_state = 'completed'", resp.SQLPattern)
		assert.Equal(t, "billing_transactions", resp.BaseTable)
		assert.Equal(t, []string{"earned_amount", "transaction_state"}, resp.ColumnsUsed)
		assert.Equal(t, "SUM", resp.Aggregation)
		assert.Equal(t, "user", resp.Source)
		require.Len(t, resp.Filters, 1)
		assert.Equal(t, "transaction_state", resp.Filters[0].Column)
		assert.Equal(t, "=", resp.Filters[0].Operator)
		assert.Equal(t, []string{"completed"}, resp.Filters[0].Values)
	})

	t.Run("minimal term with only required fields", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			ID:         uuid.New(),
			ProjectID:  uuid.New(),
			Term:       "Active User",
			Definition: "User with recent activity",
			Source:     "suggested",
		}

		resp := toGlossaryTermResponse(term)

		assert.Equal(t, "Active User", resp.Term)
		assert.Equal(t, "User with recent activity", resp.Definition)
		assert.Equal(t, "", resp.SQLPattern)
		assert.Equal(t, "", resp.BaseTable)
		assert.Nil(t, resp.ColumnsUsed)
		assert.Nil(t, resp.Filters)
		assert.Equal(t, "", resp.Aggregation)
		assert.Equal(t, "suggested", resp.Source)
	})

	t.Run("term with empty filters slice", func(t *testing.T) {
		term := &models.BusinessGlossaryTerm{
			Term:       "Test",
			Definition: "Test definition",
			Filters:    []models.Filter{}, // Empty slice
			Source:     "user",
		}

		resp := toGlossaryTermResponse(term)

		assert.Nil(t, resp.Filters, "Empty filters should result in nil in response")
	})
}

// TestGlossaryToolInOntologyToolNames verifies get_glossary is in the tool filter map.
func TestGlossaryToolInOntologyToolNames(t *testing.T) {
	assert.True(t, ontologyToolNames["get_glossary"], "get_glossary should be in ontologyToolNames for filtering")
}

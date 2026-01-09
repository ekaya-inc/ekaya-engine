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

// TestCheckContextToolsEnabled tests the AcquireToolAccess function for context tools.
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

			// Call AcquireToolAccess
			projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_context")

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

// TestParseIncludeOptions tests the parseIncludeOptions function.
func TestParseIncludeOptions(t *testing.T) {
	tests := []struct {
		name             string
		values           []string
		expectStatistics bool
		expectSamples    bool
	}{
		{
			name:             "empty values returns false for both",
			values:           []string{},
			expectStatistics: false,
			expectSamples:    false,
		},
		{
			name:             "nil values returns false for both",
			values:           nil,
			expectStatistics: false,
			expectSamples:    false,
		},
		{
			name:             "statistics only",
			values:           []string{"statistics"},
			expectStatistics: true,
			expectSamples:    false,
		},
		{
			name:             "sample_values only",
			values:           []string{"sample_values"},
			expectStatistics: false,
			expectSamples:    true,
		},
		{
			name:             "both statistics and sample_values",
			values:           []string{"statistics", "sample_values"},
			expectStatistics: true,
			expectSamples:    true,
		},
		{
			name:             "unknown values are ignored",
			values:           []string{"unknown", "invalid"},
			expectStatistics: false,
			expectSamples:    false,
		},
		{
			name:             "mixed valid and invalid",
			values:           []string{"statistics", "invalid", "sample_values"},
			expectStatistics: true,
			expectSamples:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIncludeOptions(tt.values)
			assert.Equal(t, tt.expectStatistics, result.Statistics, "Statistics flag should match")
			assert.Equal(t, tt.expectSamples, result.SampleValues, "SampleValues flag should match")
		})
	}
}

// TestAddStatisticsToColumnDetail tests the addStatisticsToColumnDetail function.
func TestAddStatisticsToColumnDetail(t *testing.T) {
	tests := []struct {
		name             string
		schemaCol        *models.SchemaColumn
		datasourceCol    *models.DatasourceColumn
		expectFields     []string // fields expected in the result
		expectNotPresent []string // fields NOT expected in the result
	}{
		{
			name: "all statistics available",
			schemaCol: &models.SchemaColumn{
				DistinctCount:     ptrInt64(100),
				NullCount:         ptrInt64(10),
				RowCount:          ptrInt64(1000),
				IsJoinable:        ptrBool(true),
				JoinabilityReason: ptrString("high_cardinality"),
			},
			datasourceCol:    &models.DatasourceColumn{},
			expectFields:     []string{"distinct_count", "row_count", "null_rate", "cardinality_ratio", "is_joinable", "joinability_reason"},
			expectNotPresent: []string{},
		},
		{
			name: "missing null count",
			schemaCol: &models.SchemaColumn{
				DistinctCount: ptrInt64(100),
				RowCount:      ptrInt64(1000),
			},
			datasourceCol:    &models.DatasourceColumn{},
			expectFields:     []string{"distinct_count", "row_count", "cardinality_ratio"},
			expectNotPresent: []string{"null_rate"},
		},
		{
			name: "missing row count",
			schemaCol: &models.SchemaColumn{
				DistinctCount: ptrInt64(100),
			},
			datasourceCol:    &models.DatasourceColumn{},
			expectFields:     []string{"distinct_count"},
			expectNotPresent: []string{"row_count", "null_rate", "cardinality_ratio"},
		},
		{
			name: "joinability info only",
			schemaCol: &models.SchemaColumn{
				IsJoinable:        ptrBool(false),
				JoinabilityReason: ptrString("low_cardinality"),
			},
			datasourceCol:    &models.DatasourceColumn{},
			expectFields:     []string{"is_joinable", "joinability_reason"},
			expectNotPresent: []string{"distinct_count", "row_count", "null_rate"},
		},
		{
			name:             "no statistics available",
			schemaCol:        &models.SchemaColumn{},
			datasourceCol:    &models.DatasourceColumn{},
			expectFields:     []string{},
			expectNotPresent: []string{"distinct_count", "row_count", "null_rate", "cardinality_ratio", "is_joinable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			colDetail := make(map[string]any)
			addStatisticsToColumnDetail(colDetail, tt.schemaCol, tt.datasourceCol)

			// Check expected fields are present
			for _, field := range tt.expectFields {
				_, exists := colDetail[field]
				assert.True(t, exists, "Expected field %s to be present", field)
			}

			// Check fields that should not be present
			for _, field := range tt.expectNotPresent {
				_, exists := colDetail[field]
				assert.False(t, exists, "Expected field %s to NOT be present", field)
			}

			// Verify calculated values are correct
			if tt.schemaCol.RowCount != nil && tt.schemaCol.NullCount != nil {
				if nullRate, ok := colDetail["null_rate"].(float64); ok {
					expectedRate := float64(*tt.schemaCol.NullCount) / float64(*tt.schemaCol.RowCount)
					assert.InDelta(t, expectedRate, nullRate, 0.0001, "null_rate calculation should be accurate")
				}
			}

			if tt.schemaCol.RowCount != nil && tt.schemaCol.DistinctCount != nil && *tt.schemaCol.RowCount > 0 {
				if cardRatio, ok := colDetail["cardinality_ratio"].(float64); ok {
					expectedRatio := float64(*tt.schemaCol.DistinctCount) / float64(*tt.schemaCol.RowCount)
					assert.InDelta(t, expectedRatio, cardRatio, 0.0001, "cardinality_ratio calculation should be accurate")
				}
			}
		})
	}
}

// Helper functions for creating pointers
func ptrInt64(v int64) *int64 {
	return &v
}

func ptrBool(v bool) *bool {
	return &v
}

func ptrString(v string) *string {
	return &v
}

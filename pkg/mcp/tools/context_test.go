package tools

import (
	"context"
	"encoding/json"
	"strings"
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
	assert.Nil(t, deps.ColumnMetadataRepo, "ColumnMetadataRepo field should be nil by default")
	assert.Nil(t, deps.TableMetadataRepo, "TableMetadataRepo field should be nil by default")
	assert.Nil(t, deps.KnowledgeRepo, "KnowledgeRepo field should be nil by default")
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
	var knowledgeRepo repositories.KnowledgeRepository
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
		KnowledgeRepo:          knowledgeRepo,
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

// TestBuildProjectKnowledgeResponse tests the buildProjectKnowledgeResponse function.
func TestBuildProjectKnowledgeResponse(t *testing.T) {
	projectID := uuid.New()

	tests := []struct {
		name               string
		facts              []*models.KnowledgeFact
		expectedCategories []string // categories expected in result
		expectedFactCounts map[string]int
	}{
		{
			name:               "empty facts returns empty map",
			facts:              []*models.KnowledgeFact{},
			expectedCategories: []string{},
			expectedFactCounts: map[string]int{},
		},
		{
			name:               "nil facts returns empty map",
			facts:              nil,
			expectedCategories: []string{},
			expectedFactCounts: map[string]int{},
		},
		{
			name: "single business_rule fact",
			facts: []*models.KnowledgeFact{
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "business_rule",
					Value:     "sessions table is ephemeral and should not be used for analytics",
					Context:   "Discovered from table structure",
				},
			},
			expectedCategories: []string{"business_rule"},
			expectedFactCounts: map[string]int{"business_rule": 1},
		},
		{
			name: "multiple facts in same category",
			facts: []*models.KnowledgeFact{
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "business_rule",
					Value:     "Use billing_engagements not billing_transactions for engagement counts",
				},
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "business_rule",
					Value:     "Platform fees are ~33% of total_amount",
					Context:   "Verified: tikr_share/total_amount ≈ 0.33",
				},
			},
			expectedCategories: []string{"business_rule"},
			expectedFactCounts: map[string]int{"business_rule": 2},
		},
		{
			name: "facts across multiple categories",
			facts: []*models.KnowledgeFact{
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "terminology",
					Value:     "A tik represents 6 seconds of engagement",
				},
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "business_rule",
					Value:     "sessions table is ephemeral",
				},
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "convention",
					Value:     "All tables use deleted_at for soft deletes",
				},
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "enumeration",
					Value:     "engagement_status: PENDING, COMPLETED, CANCELLED",
				},
			},
			expectedCategories: []string{"terminology", "business_rule", "convention", "enumeration"},
			expectedFactCounts: map[string]int{
				"terminology":   1,
				"business_rule": 1,
				"convention":    1,
				"enumeration":   1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProjectKnowledgeResponse(tt.facts)

			// Check expected categories are present
			assert.Equal(t, len(tt.expectedCategories), len(result), "Number of categories should match")

			for _, category := range tt.expectedCategories {
				facts, exists := result[category]
				assert.True(t, exists, "Category %s should be present", category)
				assert.Equal(t, tt.expectedFactCounts[category], len(facts), "Fact count for category %s should match", category)
			}

			// Verify structure of facts
			if len(result) > 0 && len(tt.facts) > 0 {
				// Find a fact with context to verify context is included
				for _, fact := range tt.facts {
					if fact.Context != "" {
						categoryFacts := result[fact.FactType]
						foundWithContext := false
						for _, f := range categoryFacts {
							if f["fact"] == fact.Value {
								if ctx, ok := f["context"]; ok {
									assert.Equal(t, fact.Context, ctx)
									foundWithContext = true
								}
							}
						}
						if fact.Context != "" {
							assert.True(t, foundWithContext, "Fact with context should have context field")
						}
					}
				}
			}
		})
	}
}

// TestBuildProjectKnowledgeResponse_ContextOmittedWhenEmpty verifies context is omitted when empty.
func TestBuildProjectKnowledgeResponse_ContextOmittedWhenEmpty(t *testing.T) {
	facts := []*models.KnowledgeFact{
		{
			ID:        uuid.New(),
			ProjectID: uuid.New(),
			FactType:  "business_rule",
			Value:     "Test rule value",
			Context:   "", // Empty context
		},
	}

	result := buildProjectKnowledgeResponse(facts)

	require.Len(t, result["business_rule"], 1)
	factData := result["business_rule"][0]

	// Verify fact value is present
	assert.Equal(t, "Test rule value", factData["fact"])

	// Verify context is NOT present when empty
	_, hasContext := factData["context"]
	assert.False(t, hasContext, "Context should not be present when empty")
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
			name: "missing null count but has non_null_count - calculates null_rate",
			schemaCol: &models.SchemaColumn{
				DistinctCount: ptrInt64(100),
				RowCount:      ptrInt64(1000),
				NonNullCount:  ptrInt64(950), // 950 non-null = 50 nulls = 5% null rate
			},
			datasourceCol:    &models.DatasourceColumn{},
			expectFields:     []string{"distinct_count", "row_count", "cardinality_ratio", "null_rate"},
			expectNotPresent: []string{},
		},
		{
			name: "missing both null_count and non_null_count",
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
			if tt.schemaCol.RowCount != nil {
				if nullRate, ok := colDetail["null_rate"].(float64); ok {
					var expectedRate float64
					if tt.schemaCol.NullCount != nil {
						expectedRate = float64(*tt.schemaCol.NullCount) / float64(*tt.schemaCol.RowCount)
					} else if tt.schemaCol.NonNullCount != nil {
						nullCount := *tt.schemaCol.RowCount - *tt.schemaCol.NonNullCount
						expectedRate = float64(nullCount) / float64(*tt.schemaCol.RowCount)
					}
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

// TestSampleValuesRedaction tests that sample values are properly redacted for sensitive data.
// This covers the integration of SensitiveDetector into the get_context tool.
func TestSampleValuesRedaction(t *testing.T) {
	tests := []struct {
		name                    string
		columnName              string
		sampleValues            []string
		expectRedacted          bool
		expectRedactionReason   string
		expectSampleValuesCount int // -1 means sample_values should be absent
	}{
		{
			name:                    "sensitive column name (api_key) - fully redacted",
			columnName:              "api_key",
			sampleValues:            []string{"secret123", "secret456"},
			expectRedacted:          true,
			expectRedactionReason:   "column name matches sensitive pattern",
			expectSampleValuesCount: -1, // no sample_values should be present
		},
		{
			name:                    "sensitive column name (password) - fully redacted",
			columnName:              "password",
			sampleValues:            []string{"hashedpw1", "hashedpw2"},
			expectRedacted:          true,
			expectRedactionReason:   "column name matches sensitive pattern",
			expectSampleValuesCount: -1,
		},
		{
			name:                    "sensitive column name (livekit_api_secret) - fully redacted",
			columnName:              "livekit_api_secret",
			sampleValues:            []string{"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i"},
			expectRedacted:          true,
			expectRedactionReason:   "column name matches sensitive pattern",
			expectSampleValuesCount: -1,
		},
		{
			name:       "non-sensitive column with sensitive JSON content - values redacted",
			columnName: "agent_data",
			sampleValues: []string{
				"",
				`{"livekit_url":"wss://tikragents-xxx.livekit.cloud","livekit_api_key":"API67e2wiyw3KvB","livekit_api_secret":"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i","livekit_agent_id":"kitt"}`,
			},
			expectRedacted:          true,
			expectRedactionReason:   "values contain sensitive patterns (api keys, secrets, etc.)",
			expectSampleValuesCount: 2,
		},
		{
			name:                    "non-sensitive column with clean content - no redaction",
			columnName:              "email",
			sampleValues:            []string{"user@example.com", "test@example.com"},
			expectRedacted:          false,
			expectRedactionReason:   "",
			expectSampleValuesCount: 2,
		},
		{
			name:                    "non-sensitive column with empty values",
			columnName:              "status",
			sampleValues:            []string{"active", "inactive"},
			expectRedacted:          false,
			expectRedactionReason:   "",
			expectSampleValuesCount: 2,
		},
		{
			name:       "JSON with nested password field - values redacted",
			columnName: "config",
			sampleValues: []string{
				`{"database":{"password":"secret123","host":"localhost"}}`,
			},
			expectRedacted:          true,
			expectRedactionReason:   "values contain sensitive patterns (api keys, secrets, etc.)",
			expectSampleValuesCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a column detail map and simulate the sample_values logic
			colDetail := map[string]any{
				"column_name": tt.columnName,
			}
			col := &models.DatasourceColumn{
				ColumnName: tt.columnName,
			}
			// Use the test's sample values directly (SampleValues was removed from SchemaColumn)
			sampleValues := tt.sampleValues

			// Simulate the sample_values logic from buildColumnDetails
			if len(sampleValues) > 0 {
				// Check if column name indicates sensitive data
				if DefaultSensitiveDetector.IsSensitiveColumn(col.ColumnName) {
					colDetail["sample_values_redacted"] = true
					colDetail["redaction_reason"] = "column name matches sensitive pattern"
				} else {
					// Check each sample value for sensitive content and redact if needed
					redactedValues := make([]string, 0, len(sampleValues))
					anyRedacted := false
					for _, val := range sampleValues {
						if DefaultSensitiveDetector.IsSensitiveContent(val) {
							redactedValues = append(redactedValues, DefaultSensitiveDetector.RedactContent(val))
							anyRedacted = true
						} else {
							redactedValues = append(redactedValues, val)
						}
					}
					colDetail["sample_values"] = redactedValues
					if anyRedacted {
						colDetail["sample_values_redacted"] = true
						colDetail["redaction_reason"] = "values contain sensitive patterns (api keys, secrets, etc.)"
					}
				}
			}

			// Verify expectations
			redacted, hasRedactedFlag := colDetail["sample_values_redacted"].(bool)
			reason, _ := colDetail["redaction_reason"].(string)

			if tt.expectRedacted {
				assert.True(t, hasRedactedFlag && redacted, "Expected sample_values_redacted to be true")
				assert.Equal(t, tt.expectRedactionReason, reason, "Redaction reason should match")
			} else {
				assert.False(t, hasRedactedFlag, "Expected sample_values_redacted to not be set")
			}

			// Check sample values count
			if tt.expectSampleValuesCount == -1 {
				_, hasSampleValues := colDetail["sample_values"]
				assert.False(t, hasSampleValues, "Expected sample_values to be absent for fully redacted columns")
			} else {
				sampleValues, hasSampleValues := colDetail["sample_values"].([]string)
				assert.True(t, hasSampleValues, "Expected sample_values to be present")
				assert.Equal(t, tt.expectSampleValuesCount, len(sampleValues), "Sample values count should match")
			}

			// For content redaction, verify values were actually redacted
			if tt.expectRedacted && tt.expectSampleValuesCount > 0 {
				sampleValues := colDetail["sample_values"].([]string)
				foundRedaction := false
				for _, val := range sampleValues {
					// Check if any value contains [REDACTED], indicating redaction occurred
					if strings.Contains(val, "[REDACTED]") {
						foundRedaction = true
						break
					}
				}
				assert.True(t, foundRedaction, "At least one value should contain [REDACTED] after redaction")
			}
		})
	}
}

// TestSampleValuesRedactionPreservesJSONStructure verifies that JSON structure is preserved during redaction.
func TestSampleValuesRedactionPreservesJSONStructure(t *testing.T) {
	// Test case from the issue - LiveKit credentials in agent_data
	originalJSON := `{"livekit_url":"wss://tikragents-xxx.livekit.cloud","livekit_api_key":"API67e2wiyw3KvB","livekit_api_secret":"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i","livekit_agent_id":"kitt"}`

	redacted := DefaultSensitiveDetector.RedactContent(originalJSON)

	// Verify it's still valid JSON
	var parsed map[string]any
	err := json.Unmarshal([]byte(redacted), &parsed)
	assert.NoError(t, err, "Redacted JSON should still be valid JSON")

	// Verify sensitive fields are redacted
	assert.Equal(t, "[REDACTED]", parsed["livekit_api_key"], "API key should be redacted")
	assert.Equal(t, "[REDACTED]", parsed["livekit_api_secret"], "API secret should be redacted")

	// Verify non-sensitive fields are preserved
	assert.Equal(t, "wss://tikragents-xxx.livekit.cloud", parsed["livekit_url"], "Non-sensitive URL should be preserved")
	assert.Equal(t, "kitt", parsed["livekit_agent_id"], "Non-sensitive agent ID should be preserved")
}

// TestColumnMetadataEnrichment tests that column metadata from update_column is merged into
// column details in get_context output.
func TestColumnMetadataEnrichment(t *testing.T) {
	tests := []struct {
		name              string
		columnName        string
		datasourceCol     *models.DatasourceColumn
		columnMeta        *models.ColumnMetadata
		expectedFields    map[string]any
		notExpectedFields []string
	}{
		{
			name:       "no metadata - only datasource values",
			columnName: "user_id",
			datasourceCol: &models.DatasourceColumn{
				ColumnName: "user_id",
				DataType:   "uuid",
				IsNullable: false,
			},
			columnMeta: nil,
			expectedFields: map[string]any{
				"column_name": "user_id",
				"data_type":   "uuid",
				"is_nullable": false,
			},
			notExpectedFields: []string{"description", "entity", "role", "enum_values"},
		},
		{
			name:       "metadata with description overrides datasource",
			columnName: "host_id",
			datasourceCol: &models.DatasourceColumn{
				ColumnName:  "host_id",
				DataType:    "uuid",
				IsNullable:  false,
				Description: "Original datasource description",
			},
			columnMeta: &models.ColumnMetadata{
				Description: ptrString("Use this to find all hosts who had engagements"),
			},
			expectedFields: map[string]any{
				"column_name": "host_id",
				"description": "Use this to find all hosts who had engagements", // Should override
			},
			notExpectedFields: []string{"entity", "role", "enum_values"},
		},
		{
			name:       "metadata with entity (via IdentifierFeatures) and role",
			columnName: "status",
			datasourceCol: &models.DatasourceColumn{
				ColumnName: "status",
				DataType:   "varchar",
				IsNullable: false,
			},
			columnMeta: &models.ColumnMetadata{
				Role: ptrString("dimension"),
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						EntityReferenced: "User",
					},
				},
			},
			expectedFields: map[string]any{
				"column_name": "status",
				"entity":      "User",
				"role":        "dimension",
			},
			notExpectedFields: []string{"description", "enum_values"},
		},
		{
			name:       "metadata with enum_values (via EnumFeatures)",
			columnName: "order_status",
			datasourceCol: &models.DatasourceColumn{
				ColumnName: "order_status",
				DataType:   "varchar",
				IsNullable: false,
			},
			columnMeta: &models.ColumnMetadata{
				Features: models.ColumnMetadataFeatures{
					EnumFeatures: &models.EnumFeatures{
						Values: []models.ColumnEnumValue{
							{Value: "PENDING", Label: "Order is pending"},
							{Value: "COMPLETED", Label: "Order completed"},
						},
					},
				},
			},
			expectedFields: map[string]any{
				"column_name": "order_status",
				"enum_values": []string{"PENDING - Order is pending", "COMPLETED - Order completed"},
			},
			notExpectedFields: []string{"description", "entity", "role"},
		},
		{
			name:       "full metadata enrichment",
			columnName: "amount",
			datasourceCol: &models.DatasourceColumn{
				ColumnName: "amount",
				DataType:   "numeric",
				IsNullable: false,
			},
			columnMeta: &models.ColumnMetadata{
				Description: ptrString("Transaction amount in USD cents"),
				Role:        ptrString("measure"),
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						EntityReferenced: "Transaction",
					},
				},
			},
			expectedFields: map[string]any{
				"column_name": "amount",
				"data_type":   "numeric",
				"is_nullable": false,
				"description": "Transaction amount in USD cents",
				"entity":      "Transaction",
				"role":        "measure",
			},
			notExpectedFields: []string{"enum_values"},
		},
		{
			name:       "empty string values are not included",
			columnName: "test_col",
			datasourceCol: &models.DatasourceColumn{
				ColumnName: "test_col",
				DataType:   "varchar",
				IsNullable: true,
			},
			columnMeta: &models.ColumnMetadata{
				Description: ptrString(""), // Empty string
				Role:        ptrString(""), // Empty string
				Features: models.ColumnMetadataFeatures{
					EnumFeatures: &models.EnumFeatures{
						Values: []models.ColumnEnumValue{}, // Empty slice
					},
				},
			},
			expectedFields: map[string]any{
				"column_name": "test_col",
			},
			notExpectedFields: []string{"description", "entity", "role", "enum_values"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build column detail as done in buildColumnDetails
			colDetail := map[string]any{
				"column_name": tt.datasourceCol.ColumnName,
				"data_type":   tt.datasourceCol.DataType,
				"is_nullable": tt.datasourceCol.IsNullable,
			}
			if tt.datasourceCol.BusinessName != "" {
				colDetail["business_name"] = tt.datasourceCol.BusinessName
			}
			if tt.datasourceCol.Description != "" {
				colDetail["description"] = tt.datasourceCol.Description
			}

			// Apply column metadata enrichment (same logic as context.go)
			// Column metadata now uses Features JSONB with typed sub-features
			if tt.columnMeta != nil {
				// Description from update_column overrides datasource description
				if tt.columnMeta.Description != nil && *tt.columnMeta.Description != "" {
					colDetail["description"] = *tt.columnMeta.Description
				}
				// Entity association is now in Features.IdentifierFeatures.EntityReferenced
				if idFeatures := tt.columnMeta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.EntityReferenced != "" {
					colDetail["entity"] = idFeatures.EntityReferenced
				}
				// Semantic role
				if tt.columnMeta.Role != nil && *tt.columnMeta.Role != "" {
					colDetail["role"] = *tt.columnMeta.Role
				}
				// Enum values are now in Features.EnumFeatures.Values
				if enumFeatures := tt.columnMeta.GetEnumFeatures(); enumFeatures != nil && len(enumFeatures.Values) > 0 {
					enumStrings := make([]string, len(enumFeatures.Values))
					for i, ev := range enumFeatures.Values {
						if ev.Label != "" {
							enumStrings[i] = ev.Value + " - " + ev.Label
						} else {
							enumStrings[i] = ev.Value
						}
					}
					colDetail["enum_values"] = enumStrings
				}
			}

			// Verify expected fields are present with correct values
			for field, expectedVal := range tt.expectedFields {
				actualVal, exists := colDetail[field]
				assert.True(t, exists, "Expected field %s to be present", field)
				assert.Equal(t, expectedVal, actualVal, "Field %s should have expected value", field)
			}

			// Verify fields that should not be present
			for _, field := range tt.notExpectedFields {
				_, exists := colDetail[field]
				assert.False(t, exists, "Field %s should NOT be present", field)
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

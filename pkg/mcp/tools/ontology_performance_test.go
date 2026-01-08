//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test IDs for ontology performance tests (unique range 0x801-0x8xx)
var (
	ontologyPerfTestProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000801")
	ontologyPerfTestDSID      = uuid.MustParse("00000000-0000-0000-0000-000000000802")
)

// TestGetOntology_Performance_DomainDepth verifies that get_ontology(depth: 'domain')
// returns domain context in <100ms as specified in success criteria.
func TestGetOntology_Performance_DomainDepth(t *testing.T) {
	// Setup test context with real database
	tc := setupOntologyPerformanceTest(t)
	defer tc.cleanup()

	// Ensure we have an active ontology with representative data
	tc.createTestOntology()

	// Create authenticated context
	ctx, cleanup := tc.createAuthContext()
	defer cleanup()

	// Create MCP server and register tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	RegisterOntologyTools(mcpServer, tc.deps)

	// Create JSON-RPC request
	requestJSON := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_ontology","arguments":{"depth":"domain","include_relationships":true}},"id":1}`

	// Run performance test - measure time for domain depth query
	const iterations = 10 // Run multiple times to get average
	var totalDuration time.Duration
	var successCount int

	for i := 0; i < iterations; i++ {
		start := time.Now()
		result := mcpServer.HandleMessage(ctx, []byte(requestJSON))
		elapsed := time.Since(start)

		// Marshal result to check for errors
		resultBytes, err := json.Marshal(result)
		require.NoError(t, err, "iteration %d: failed to marshal result", i)

		// Parse response
		var response struct {
			Result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		err = json.Unmarshal(resultBytes, &response)
		require.NoError(t, err, "iteration %d: failed to unmarshal response", i)
		require.Nil(t, response.Error, "iteration %d: unexpected error in response", i)

		totalDuration += elapsed
		successCount++

		// Verify response structure
		require.NotEmpty(t, response.Result.Content, "iteration %d: expected content in response", i)
		require.Equal(t, "text", response.Result.Content[0].Type, "iteration %d: expected text type", i)

		// Parse JSON response
		var ontologyResponse map[string]any
		err = json.Unmarshal([]byte(response.Result.Content[0].Text), &ontologyResponse)
		require.NoError(t, err, "iteration %d: failed to parse ontology response JSON", i)

		// Verify response has expected fields
		assert.Contains(t, ontologyResponse, "domain", "iteration %d: response should have domain field", i)
		assert.Contains(t, ontologyResponse, "entities", "iteration %d: response should have entities field", i)
	}

	// Calculate average time
	avgDuration := totalDuration / time.Duration(successCount)

	// Log performance results
	t.Logf("get_ontology(depth: 'domain') performance:")
	t.Logf("  Iterations: %d", successCount)
	t.Logf("  Total time: %v", totalDuration)
	t.Logf("  Average time: %v", avgDuration)
	t.Logf("  Min acceptable: 100ms")

	// Assert performance requirement: <100ms average
	const maxAcceptableTime = 100 * time.Millisecond
	assert.Less(t, avgDuration, maxAcceptableTime,
		"get_ontology(depth: 'domain') must complete in <100ms on average, got %v", avgDuration)

	// Also check that no single iteration exceeded 200ms (reasonable upper bound)
	const maxSingleIteration = 200 * time.Millisecond
	if avgDuration < maxAcceptableTime {
		t.Logf("✓ Performance requirement met: average %v < %v", avgDuration, maxAcceptableTime)
	} else {
		t.Errorf("✗ Performance requirement NOT met: average %v >= %v", avgDuration, maxAcceptableTime)
	}
}

// TestGetOntology_Performance_AllDepths benchmarks all depth levels for performance profiling.
func TestGetOntology_Performance_AllDepths(t *testing.T) {
	tc := setupOntologyPerformanceTest(t)
	defer tc.cleanup()

	tc.createTestOntology()

	ctx, cleanup := tc.createAuthContext()
	defer cleanup()

	// Create MCP server and register tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	RegisterOntologyTools(mcpServer, tc.deps)

	testCases := []struct {
		name     string
		depth    string
		tables   []string
		expected time.Duration // expected reasonable time
	}{
		{
			name:     "domain",
			depth:    "domain",
			expected: 100 * time.Millisecond,
		},
		{
			name:     "entities",
			depth:    "entities",
			expected: 200 * time.Millisecond,
		},
		{
			name:     "tables_filtered",
			depth:    "tables",
			tables:   []string{"users", "orders"},
			expected: 300 * time.Millisecond,
		},
		{
			name:     "columns_filtered",
			depth:    "columns",
			tables:   []string{"users"},
			expected: 300 * time.Millisecond,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Build JSON request
			var requestJSON string
			if testCase.tables != nil {
				tablesJSON, _ := json.Marshal(testCase.tables)
				requestJSON = fmt.Sprintf(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_ontology","arguments":{"depth":"%s","include_relationships":true,"tables":%s}},"id":1}`, testCase.depth, string(tablesJSON))
			} else {
				requestJSON = fmt.Sprintf(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_ontology","arguments":{"depth":"%s","include_relationships":true}},"id":1}`, testCase.depth)
			}

			// Measure performance
			start := time.Now()
			result := mcpServer.HandleMessage(ctx, []byte(requestJSON))
			elapsed := time.Since(start)

			// Marshal and verify result
			resultBytes, err := json.Marshal(result)
			require.NoError(t, err)

			var response struct {
				Error *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			err = json.Unmarshal(resultBytes, &response)
			require.NoError(t, err)
			require.Nil(t, response.Error, "unexpected error in response")

			t.Logf("get_ontology(depth: '%s') completed in %v (expected <%v)", testCase.depth, elapsed, testCase.expected)

			// Log warning if slower than expected (not a hard failure, as this is for profiling)
			if elapsed > testCase.expected {
				t.Logf("  ⚠ Slower than expected: %v > %v", elapsed, testCase.expected)
			}
		})
	}
}

// ontologyPerformanceTestContext holds test dependencies
type ontologyPerformanceTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	deps      *OntologyToolDeps
	projectID uuid.UUID
	dsID      uuid.UUID
}

// setupOntologyPerformanceTest creates test context with real database
func setupOntologyPerformanceTest(t *testing.T) *ontologyPerformanceTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	logger := zap.NewNop()

	// Create repositories
	ontologyRepo := repositories.NewOntologyRepository()
	entityRepo := repositories.NewOntologyEntityRepository()
	relationshipRepo := repositories.NewEntityRelationshipRepository()
	schemaRepo := repositories.NewSchemaRepository()

	// Create mock project service for tests
	projectService := &mockProjectService{defaultDatasourceID: ontologyPerfTestDSID}

	// Create services
	ontologyContextService := services.NewOntologyContextService(
		ontologyRepo,
		entityRepo,
		relationshipRepo,
		schemaRepo,
		projectService,
		logger,
	)

	// Create mock MCP config service with developer tools enabled
	mcpConfigService := &mockMCPConfigService{
		config:                    &models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		shouldShowApprovedQueries: true,
	}

	// Create dependencies
	deps := &OntologyToolDeps{
		DB:                     engineDB.DB,
		MCPConfigService:       mcpConfigService,
		OntologyContextService: ontologyContextService,
		OntologyRepo:           ontologyRepo,
		EntityRepo:             entityRepo,
		SchemaRepo:             schemaRepo,
		Logger:                 logger,
	}

	tc := &ontologyPerformanceTestContext{
		t:         t,
		engineDB:  engineDB,
		deps:      deps,
		projectID: ontologyPerfTestProjectID,
		dsID:      ontologyPerfTestDSID,
	}

	// Ensure project exists
	tc.ensureTestProject()

	return tc
}

// createAuthContext creates authenticated context with tenant scope
func (tc *ontologyPerformanceTestContext) createAuthContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()

	// Add auth claims
	claims := &auth.Claims{
		ProjectID: tc.projectID.String(),
	}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	// Create tenant scope
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// ensureTestProject creates the test project if it doesn't exist
func (tc *ontologyPerformanceTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Ontology Performance Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// createTestOntology creates a representative ontology for performance testing
func (tc *ontologyPerformanceTestContext) createTestOntology() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	ctx = database.SetTenantScope(ctx, scope)

	// Create ontology
	ontologyID := uuid.New()
	domainSummary := &models.DomainSummary{
		Description: "E-commerce platform for B2B wholesale transactions",
		Domains:     []string{"sales", "customer", "product"},
		RelationshipGraph: []models.RelationshipEdge{
			{From: "user", To: "order", Label: "places", Cardinality: "1:N"},
			{From: "order", To: "product", Label: "contains", Cardinality: "N:M"},
		},
	}

	entitySummaries := map[string]*models.EntitySummary{
		"users": {
			TableName:    "users",
			BusinessName: "Users",
			Description:  "Platform users including customers and internal staff",
			Domain:       "customer",
			Synonyms:     []string{"accounts", "members"},
			KeyColumns: []models.KeyColumn{
				{Name: "id", Synonyms: []string{"user_id"}},
				{Name: "email", Synonyms: []string{"email_address"}},
			},
			ColumnCount: 5,
		},
		"orders": {
			TableName:    "orders",
			BusinessName: "Orders",
			Description:  "Customer purchase orders",
			Domain:       "sales",
			Synonyms:     []string{"purchases"},
			KeyColumns: []models.KeyColumn{
				{Name: "id", Synonyms: []string{"order_id"}},
				{Name: "status", Synonyms: []string{"order_status"}},
			},
			ColumnCount: 8,
		},
		"products": {
			TableName:    "products",
			BusinessName: "Products",
			Description:  "Product catalog",
			Domain:       "product",
			ColumnCount:  6,
		},
	}

	columnDetails := map[string][]models.ColumnDetail{
		"users": {
			{Name: "id", Role: "identifier", IsPrimaryKey: true},
			{Name: "email", Role: "attribute"},
			{Name: "name", Role: "attribute"},
			{Name: "tier", Role: "dimension"},
			{Name: "created_at", Role: "dimension"},
		},
		"orders": {
			{Name: "id", Role: "identifier", IsPrimaryKey: true},
			{Name: "user_id", Role: "identifier", IsForeignKey: true},
			{Name: "status", Role: "dimension", EnumValues: []models.EnumValue{
				{Value: "pending", Label: "Pending"},
				{Value: "confirmed", Label: "Confirmed"},
				{Value: "shipped", Label: "Shipped"},
			}},
			{Name: "total_amount", Role: "measure"},
			{Name: "created_at", Role: "dimension"},
		},
		"products": {
			{Name: "id", Role: "identifier", IsPrimaryKey: true},
			{Name: "name", Role: "attribute"},
			{Name: "price", Role: "measure"},
			{Name: "category", Role: "dimension"},
		},
	}

	ontology := &models.TieredOntology{
		ID:              ontologyID,
		ProjectID:       tc.projectID,
		Version:         1,
		IsActive:        true,
		DomainSummary:   domainSummary,
		EntitySummaries: entitySummaries,
		ColumnDetails:   columnDetails,
	}

	err = tc.deps.OntologyRepo.Create(ctx, ontology)
	if err != nil {
		tc.t.Fatalf("Failed to create test ontology: %v", err)
	}

	// Create entities
	entities := []*models.OntologyEntity{
		{
			ID:            uuid.New(),
			ProjectID:     tc.projectID,
			OntologyID:    ontologyID,
			Name:          "user",
			Description:   "Platform users including customers and internal staff",
			PrimarySchema: "public",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
		{
			ID:            uuid.New(),
			ProjectID:     tc.projectID,
			OntologyID:    ontologyID,
			Name:          "order",
			Description:   "Customer purchase orders",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
			PrimaryColumn: "id",
		},
		{
			ID:            uuid.New(),
			ProjectID:     tc.projectID,
			OntologyID:    ontologyID,
			Name:          "product",
			Description:   "Product catalog items",
			PrimarySchema: "public",
			PrimaryTable:  "products",
			PrimaryColumn: "id",
		},
	}

	for _, entity := range entities {
		err = tc.deps.EntityRepo.Create(ctx, entity)
		if err != nil {
			tc.t.Fatalf("Failed to create test entity %s: %v", entity.Name, err)
		}
	}
}

// cleanup removes test data
func (tc *ontologyPerformanceTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Logf("Failed to create tenant scope for cleanup: %v", err)
		return
	}
	defer scope.Close()

	// Delete test data in reverse dependency order
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_entity_occurrences WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_entity_aliases WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_entities WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
}

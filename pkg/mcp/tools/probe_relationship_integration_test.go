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

// probeRelationshipTestContext holds test dependencies for probe_relationship integration tests.
type probeRelationshipTestContext struct {
	t                      *testing.T
	engineDB               *testhelpers.EngineDB
	projectID              uuid.UUID
	mcpServer              *server.MCPServer
	ontologyRepo           repositories.OntologyRepository
	ontologyEntityRepo     repositories.OntologyEntityRepository
	entityRelationshipRepo repositories.EntityRelationshipRepository
}

// setupProbeRelationshipIntegrationTest initializes the test context.
func setupProbeRelationshipIntegrationTest(t *testing.T) *probeRelationshipTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000067")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Probe Relationship Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with probe tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	ontologyRepo := repositories.NewOntologyRepository()
	ontologyEntityRepo := repositories.NewOntologyEntityRepository()
	entityRelationshipRepo := repositories.NewEntityRelationshipRepository()

	// Configure mock to enable developer tools
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true,
			AddQueryTools:          true,
			AddOntologyMaintenance: true,
		},
	}

	// Register probe tools
	probeDeps := &ProbeToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mockMCPConfig,
		SchemaRepo:       &mockSchemaRepository{}, // Empty schema for relationship-only tests
		OntologyRepo:     ontologyRepo,
		EntityRepo:       ontologyEntityRepo,
		RelationshipRepo: entityRelationshipRepo,
		ProjectService:   &mockProjectService{defaultDatasourceID: uuid.New()},
		Logger:           zap.NewNop(),
	}
	RegisterProbeTools(mcpServer, probeDeps)

	// Register relationship tools (for update_relationship)
	relationshipDeps := &RelationshipToolDeps{
		DB:                     engineDB.DB,
		MCPConfigService:       mockMCPConfig,
		OntologyRepo:           ontologyRepo,
		OntologyEntityRepo:     ontologyEntityRepo,
		EntityRelationshipRepo: entityRelationshipRepo,
		Logger:                 zap.NewNop(),
	}
	RegisterRelationshipTools(mcpServer, relationshipDeps)

	// Register entity tools (for update_entity)
	entityDeps := &EntityToolDeps{
		DB:                     engineDB.DB,
		MCPConfigService:       mockMCPConfig,
		OntologyRepo:           ontologyRepo,
		OntologyEntityRepo:     ontologyEntityRepo,
		EntityRelationshipRepo: entityRelationshipRepo,
		Logger:                 zap.NewNop(),
	}
	RegisterEntityTools(mcpServer, entityDeps)

	return &probeRelationshipTestContext{
		t:                      t,
		engineDB:               engineDB,
		projectID:              projectID,
		mcpServer:              mcpServer,
		ontologyRepo:           ontologyRepo,
		ontologyEntityRepo:     ontologyEntityRepo,
		entityRelationshipRepo: entityRelationshipRepo,
	}
}

// cleanup removes test entities, relationships, and ontologies.
func (tc *probeRelationshipTestContext) cleanup() {
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
func (tc *probeRelationshipTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{ProjectID: tc.projectID.String()})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *probeRelationshipTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
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

// createOntologyAndEntities creates an ontology and entities directly in the database,
// simulating what happens during ontology extraction.
func (tc *probeRelationshipTestContext) createOntologyAndEntities(ctx context.Context, entityNames ...string) uuid.UUID {
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

	// Create entities under this ontology (simulating extraction)
	for _, name := range entityNames {
		entity := &models.OntologyEntity{
			ProjectID:     tc.projectID,
			OntologyID:    ontology.ID,
			Name:          name,
			Description:   "Test entity: " + name,
			PrimaryTable:  name + "_table",
			PrimaryColumn: name + "_id",
			CreatedBy:     models.ProvenanceInferred, // Extraction creates with inference provenance
		}
		err = tc.ontologyEntityRepo.Create(ctx, entity)
		require.NoError(tc.t, err)
	}

	return ontology.ID
}

// ============================================================================
// Integration Tests: probe_relationship finds MCP-created relationships
// ============================================================================

// TestProbeRelationship_Integration_FindsMCPCreatedRelationship verifies that probe_relationship
// can find relationships that were created via update_relationship MCP tool.
// This is the primary test for Bug 3 fix.
func TestProbeRelationship_Integration_FindsMCPCreatedRelationship(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create ontology and entities (simulating extraction)
	_ = tc.createOntologyAndEntities(ctx, "Order", "Customer")

	// Step 2: Create relationship via update_relationship MCP tool
	updateResult, err := tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Order",
		"to_entity":   "Customer",
		"description": "The customer who placed this order",
		"label":       "placed_by",
		"cardinality": "N:1",
	})
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	assert.False(t, updateResult.IsError, "update_relationship should succeed")

	// Step 3: Verify relationship was created
	var updateResponse updateRelationshipResponse
	require.Len(t, updateResult.Content, 1)
	err = json.Unmarshal([]byte(updateResult.Content[0].(mcp.TextContent).Text), &updateResponse)
	require.NoError(t, err)
	assert.True(t, updateResponse.Created, "should create new relationship")
	assert.Equal(t, "Order", updateResponse.FromEntity)
	assert.Equal(t, "Customer", updateResponse.ToEntity)

	// Step 4: Call probe_relationship - this should find the MCP-created relationship
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"from_entity": "Order",
		"to_entity":   "Customer",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError, "probe_relationship should succeed")

	// Step 5: Parse and verify probe_relationship result
	var probeResponse probeRelationshipResponse
	require.Len(t, probeResult.Content, 1)
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// The key assertion: relationship should NOT be empty (this was the bug)
	assert.Len(t, probeResponse.Relationships, 1, "should find the MCP-created relationship")
	if len(probeResponse.Relationships) > 0 {
		rel := probeResponse.Relationships[0]
		assert.Equal(t, "Order", rel.FromEntity)
		assert.Equal(t, "Customer", rel.ToEntity)
		assert.NotNil(t, rel.Description)
		assert.Equal(t, "The customer who placed this order", *rel.Description)
		assert.NotNil(t, rel.Label)
		assert.Equal(t, "placed_by", *rel.Label)
		// Note: Cardinality comes from schema relationships, not entity relationships.
		// MCP-created relationships without schema counterparts will have empty cardinality.
		// This is expected behavior for now.
	}
}

// TestProbeRelationship_Integration_NoFilters returns all relationships.
func TestProbeRelationship_Integration_NoFiltersReturnsAll(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create ontology and entities
	_ = tc.createOntologyAndEntities(ctx, "Account", "User", "Product")

	// Create multiple relationships
	_, err := tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Account",
		"to_entity":   "User",
		"label":       "owned_by",
	})
	require.NoError(t, err)

	_, err = tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Account",
		"to_entity":   "Product",
		"label":       "contains",
	})
	require.NoError(t, err)

	// Probe without filters should return all relationships
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Len(t, probeResponse.Relationships, 2, "should find both relationships")
}

// TestProbeRelationship_Integration_FilterByFromEntity filters by source entity.
func TestProbeRelationship_Integration_FilterByFromEntity(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create ontology and entities
	_ = tc.createOntologyAndEntities(ctx, "Order", "Customer", "Vendor")

	// Create relationships
	_, err := tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Order",
		"to_entity":   "Customer",
	})
	require.NoError(t, err)

	_, err = tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Customer",
		"to_entity":   "Vendor",
	})
	require.NoError(t, err)

	// Probe with from_entity filter
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"from_entity": "Order",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Len(t, probeResponse.Relationships, 1, "should find only Order relationships")
	assert.Equal(t, "Order", probeResponse.Relationships[0].FromEntity)
}

// TestProbeRelationship_Integration_FilterByToEntity filters by target entity.
func TestProbeRelationship_Integration_FilterByToEntity(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create ontology and entities
	_ = tc.createOntologyAndEntities(ctx, "Order", "Customer", "Refund")

	// Create relationships
	_, err := tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Order",
		"to_entity":   "Customer",
	})
	require.NoError(t, err)

	_, err = tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Refund",
		"to_entity":   "Customer",
	})
	require.NoError(t, err)

	// Probe with to_entity filter
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"to_entity": "Customer",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Len(t, probeResponse.Relationships, 2, "should find both relationships to Customer")
	for _, rel := range probeResponse.Relationships {
		assert.Equal(t, "Customer", rel.ToEntity)
	}
}

// ============================================================================
// Edge Case: Relationship created AFTER extraction entities exist
// ============================================================================

// TestProbeRelationship_Integration_RelationshipAfterExtraction verifies probe_relationship
// finds relationships created after entities were created via extraction simulation.
func TestProbeRelationship_Integration_RelationshipAfterExtraction(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Simulate extraction creating entities
	_ = tc.createOntologyAndEntities(ctx, "Invoice", "Payment")

	// Step 2: Later, MCP creates a relationship between these extraction-created entities
	updateResult, err := tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Invoice",
		"to_entity":   "Payment",
		"description": "Payment associated with this invoice",
		"cardinality": "1:N",
	})
	require.NoError(t, err)
	require.NotNil(t, updateResult)
	assert.False(t, updateResult.IsError)

	// Step 3: probe_relationship should find this relationship
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"from_entity": "Invoice",
		"to_entity":   "Payment",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Len(t, probeResponse.Relationships, 1, "should find the relationship")
	assert.Equal(t, "Invoice", probeResponse.Relationships[0].FromEntity)
	assert.Equal(t, "Payment", probeResponse.Relationships[0].ToEntity)
}

// ============================================================================
// Edge Case: Both entities and relationship created via MCP
// ============================================================================

// TestProbeRelationship_Integration_AllMCPCreated verifies probe_relationship works
// when both entities and relationship are created via MCP tools.
func TestProbeRelationship_Integration_AllMCPCreated(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create entities via MCP (update_entity)
	_, err := tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "Subscription",
		"description": "Customer subscription",
	})
	require.NoError(t, err)

	_, err = tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "Plan",
		"description": "Subscription plan",
	})
	require.NoError(t, err)

	// Step 2: Create relationship via MCP
	_, err = tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "Subscription",
		"to_entity":   "Plan",
		"label":       "has_plan",
		"cardinality": "N:1",
	})
	require.NoError(t, err)

	// Step 3: probe_relationship should find the relationship
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"from_entity": "Subscription",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Len(t, probeResponse.Relationships, 1, "should find the relationship")
	assert.Equal(t, "Subscription", probeResponse.Relationships[0].FromEntity)
	assert.Equal(t, "Plan", probeResponse.Relationships[0].ToEntity)
}

// ============================================================================
// Edge Case: Empty result when no relationships match filter
// ============================================================================

// TestProbeRelationship_Integration_NoMatchingRelationships verifies probe_relationship
// returns empty array when no relationships match the filter.
func TestProbeRelationship_Integration_NoMatchingRelationships(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create ontology and entities but no relationships
	_ = tc.createOntologyAndEntities(ctx, "Standalone", "Orphan")

	// probe_relationship should return empty
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"from_entity": "Standalone",
		"to_entity":   "Orphan",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Empty(t, probeResponse.Relationships, "should return empty relationships array")
}

// ============================================================================
// Edge Case: Mixed provenance (extraction-created entity + MCP-created entity)
// ============================================================================

// TestProbeRelationship_Integration_MixedProvenance verifies probe_relationship works
// when relationship connects extraction-created entity to MCP-created entity.
func TestProbeRelationship_Integration_MixedProvenance(t *testing.T) {
	tc := setupProbeRelationshipIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create one entity via extraction simulation
	_ = tc.createOntologyAndEntities(ctx, "ExtractionEntity")

	// Step 2: Create another entity via MCP
	_, err := tc.callTool(ctx, "update_entity", map[string]any{
		"name":        "MCPEntity",
		"description": "Entity created via MCP",
	})
	require.NoError(t, err)

	// Step 3: Create relationship between them
	_, err = tc.callTool(ctx, "update_relationship", map[string]any{
		"from_entity": "ExtractionEntity",
		"to_entity":   "MCPEntity",
		"label":       "connects_to",
	})
	require.NoError(t, err)

	// Step 4: probe_relationship should find this relationship
	probeResult, err := tc.callTool(ctx, "probe_relationship", map[string]any{
		"from_entity": "ExtractionEntity",
		"to_entity":   "MCPEntity",
	})
	require.NoError(t, err)
	require.NotNil(t, probeResult)
	assert.False(t, probeResult.IsError)

	var probeResponse probeRelationshipResponse
	err = json.Unmarshal([]byte(probeResult.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	assert.Len(t, probeResponse.Relationships, 1, "should find the mixed-provenance relationship")
	assert.Equal(t, "ExtractionEntity", probeResponse.Relationships[0].FromEntity)
	assert.Equal(t, "MCPEntity", probeResponse.Relationships[0].ToEntity)
}

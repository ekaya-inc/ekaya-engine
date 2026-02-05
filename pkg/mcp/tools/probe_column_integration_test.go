//go:build ignore

// TODO: This test needs rewrite after column schema refactor.
// ColumnMetadata now uses SchemaColumnID instead of TableName/ColumnName.
// EnumValues moved to ColumnMetadataFeatures JSONB.

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
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// probeColumnTestContext holds test dependencies for probe_column integration tests.
type probeColumnTestContext struct {
	t                  *testing.T
	engineDB           *testhelpers.EngineDB
	projectID          uuid.UUID
	datasourceID       uuid.UUID
	mcpServer          *server.MCPServer
	ontologyRepo       repositories.OntologyRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	schemaRepo         repositories.SchemaRepository
	changeReviewSvc    services.ChangeReviewService
	pendingChangeRepo  repositories.PendingChangeRepository
}

// setupProbeColumnIntegrationTest initializes the test context.
func setupProbeColumnIntegrationTest(t *testing.T) *probeColumnTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000068")
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000168")

	// Ensure test project and datasource exist
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Probe Column Integration Test Project")
	require.NoError(t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, provider, datasource_config)
		VALUES ($1, $2, 'test_datasource', 'postgresql', 'custom', '{}')
		ON CONFLICT (id) DO NOTHING
	`, datasourceID, projectID)
	require.NoError(t, err)

	// Create MCP server with probe tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	ontologyRepo := repositories.NewOntologyRepository()
	columnMetadataRepo := repositories.NewColumnMetadataRepository()
	schemaRepo := repositories.NewSchemaRepository()
	pendingChangeRepo := repositories.NewPendingChangeRepository()

	// Configure mock to enable developer tools
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true,
			AddQueryTools:          true,
			AddOntologyMaintenance: true,
		},
	}

	// Register probe tools with real repositories
	probeDeps := &ProbeToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		SchemaRepo:         schemaRepo,
		OntologyRepo:       ontologyRepo,
		ColumnMetadataRepo: columnMetadataRepo,
		ProjectService:     &mockProjectService{defaultDatasourceID: datasourceID},
	}
	RegisterProbeTools(mcpServer, probeDeps)

	// Create change review service
	changeReviewSvc := services.NewChangeReviewService(&services.ChangeReviewServiceDeps{
		PendingChangeRepo:  pendingChangeRepo,
		ColumnMetadataRepo: columnMetadataRepo,
		OntologyRepo:       ontologyRepo,
		Logger:             zap.NewNop(),
	})

	return &probeColumnTestContext{
		t:                  t,
		engineDB:           engineDB,
		projectID:          projectID,
		datasourceID:       datasourceID,
		mcpServer:          mcpServer,
		ontologyRepo:       ontologyRepo,
		columnMetadataRepo: columnMetadataRepo,
		schemaRepo:         schemaRepo,
		changeReviewSvc:    changeReviewSvc,
		pendingChangeRepo:  pendingChangeRepo,
	}
}

// cleanup removes test data.
func (tc *probeColumnTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Clean up in order due to foreign keys
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_pending_changes WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_column_metadata WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_relationships WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_columns WHERE table_id IN (SELECT id FROM engine_schema_tables WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_entity_relationships WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_occurrences WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_key_columns WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_aliases WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *probeColumnTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	// Include admin role to access developer tools (probe tools require developer access)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *probeColumnTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
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

// createSchemaTable creates a schema table for testing.
func (tc *probeColumnTestContext) createSchemaTable(ctx context.Context, tableName string) uuid.UUID {
	tc.t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		SchemaName:   "public",
		TableName:    tableName,
		IsSelected:   true,
	}
	err := tc.schemaRepo.UpsertTable(ctx, table)
	require.NoError(tc.t, err)

	return table.ID
}

// createSchemaColumn creates a schema column for testing with statistics.
func (tc *probeColumnTestContext) createSchemaColumn(ctx context.Context, tableID uuid.UUID, columnName, dataType string, distinctCount, rowCount int64) uuid.UUID {
	tc.t.Helper()

	isJoinable := distinctCount > 10
	joinabilityReason := "low_cardinality"
	if isJoinable {
		joinabilityReason = "high_cardinality"
	}
	nonNullCount := rowCount

	column := &models.SchemaColumn{
		ProjectID:         tc.projectID,
		SchemaTableID:     tableID,
		ColumnName:        columnName,
		DataType:          dataType,
		IsSelected:        true,
		DistinctCount:     &distinctCount,
		RowCount:          &rowCount,
		NonNullCount:      &nonNullCount,
		IsJoinable:        &isJoinable,
		JoinabilityReason: &joinabilityReason,
	}
	err := tc.schemaRepo.UpsertColumn(ctx, column)
	require.NoError(tc.t, err)

	return column.ID
}

// createActiveOntology creates an active ontology for testing.
func (tc *probeColumnTestContext) createActiveOntology(ctx context.Context) *models.TieredOntology {
	tc.t.Helper()

	ontology := &models.TieredOntology{
		ProjectID:     tc.projectID,
		Version:       1,
		IsActive:      true,
		ColumnDetails: make(map[string][]models.ColumnDetail),
		Metadata:      make(map[string]any),
	}
	err := tc.ontologyRepo.Create(ctx, ontology)
	require.NoError(tc.t, err)

	return ontology
}

// createPendingChange creates a pending change for testing.
func (tc *probeColumnTestContext) createPendingChange(ctx context.Context, tableName, columnName string, enumValues []string) uuid.UUID {
	tc.t.Helper()

	// Convert enum values to []any for payload
	enumAny := make([]any, len(enumValues))
	for i, v := range enumValues {
		enumAny[i] = v
	}

	change := &models.PendingChange{
		ProjectID:       tc.projectID,
		ChangeType:      models.ChangeTypeNewEnumValue, // For detected enum values
		ChangeSource:    models.ChangeSourceDataScan,   // Required field
		TableName:       tableName,
		ColumnName:      columnName,
		SuggestedAction: models.SuggestedActionCreateColumnMetadata,
		SuggestedPayload: map[string]any{
			"enum_values": enumAny,
		},
		Status: models.ChangeStatusPending,
	}
	err := tc.pendingChangeRepo.Create(ctx, change)
	require.NoError(tc.t, err)

	return change.ID
}

// ============================================================================
// Integration Tests: probe_column with column_metadata fallback
// ============================================================================

// TestProbeColumn_Integration_ApproveChangeThenProbe verifies the full flow:
// 1. Create schema with a column
// 2. Create a pending change with enum values
// 3. Approve the change (writes to column_metadata)
// 4. Probe the column and verify enum_labels appear
func TestProbeColumn_Integration_ApproveChangeThenProbe(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "test_tickets")
	tc.createSchemaColumn(ctx, tableID, "ticket_type", "varchar", 5, 1000)

	// Step 2: Create an active ontology (required for probe_column)
	tc.createActiveOntology(ctx)

	// Step 3: Create a pending change with enum values
	changeID := tc.createPendingChange(ctx, "test_tickets", "ticket_type",
		[]string{"BUG", "FEATURE", "SUPPORT", "TASK", "ENHANCEMENT"})

	// Step 4: Approve the change
	_, err := tc.changeReviewSvc.ApproveChange(ctx, changeID, models.ProvenanceMCP)
	require.NoError(t, err)

	// Step 5: Probe the column - should return enum_labels from column_metadata
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "test_tickets",
		"column": "ticket_type",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "probe_column should succeed")

	// Parse and verify response
	var probeResponse probeColumnResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// The key assertion: enum_labels should appear from column_metadata fallback
	assert.NotNil(t, probeResponse.Semantic, "should have semantic section")
	assert.NotNil(t, probeResponse.Semantic.EnumLabels, "should have enum_labels")
	assert.Len(t, probeResponse.Semantic.EnumLabels, 5, "should have 5 enum values")
	assert.Equal(t, "BUG", probeResponse.Semantic.EnumLabels["BUG"])
	assert.Equal(t, "FEATURE", probeResponse.Semantic.EnumLabels["FEATURE"])
}

// TestProbeColumn_Integration_MetadataBothLocations verifies ontology takes precedence
// when metadata exists in BOTH ontology.column_details AND column_metadata.
func TestProbeColumn_Integration_MetadataBothLocations(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "users")
	tc.createSchemaColumn(ctx, tableID, "status", "varchar", 3, 500)

	// Step 2: Create an active ontology WITH column details
	tc.createActiveOntology(ctx)
	columnDetails := []models.ColumnDetail{
		{
			Name:        "status",
			Description: "User account status from ontology",
			Role:        "dimension",
			EnumValues: []models.EnumValue{
				{Value: "ACTIVE", Label: "Active user account"},
				{Value: "SUSPENDED", Label: "Temporarily suspended"},
				{Value: "DELETED", Label: "Permanently removed"},
			},
		},
	}
	err := tc.ontologyRepo.UpdateColumnDetails(ctx, tc.projectID, "users", columnDetails)
	require.NoError(t, err)

	// Step 3: Also create column_metadata (simulating approved change)
	columnMeta := &models.ColumnMetadata{
		ProjectID:  tc.projectID,
		TableName:  "users",
		ColumnName: "status",
		EnumValues: []string{"ACTIVE", "INACTIVE"}, // Different values
		Source:     models.ProvenanceMCP,
	}
	err = tc.columnMetadataRepo.Upsert(ctx, columnMeta)
	require.NoError(t, err)

	// Step 4: Probe the column - ontology should take precedence
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "users",
		"column": "status",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Ontology values should be present (with labels, not raw values)
	assert.NotNil(t, probeResponse.Semantic)
	assert.Len(t, probeResponse.Semantic.EnumLabels, 3, "should have 3 values from ontology")
	assert.Equal(t, "Active user account", probeResponse.Semantic.EnumLabels["ACTIVE"])
	assert.Equal(t, "Temporarily suspended", probeResponse.Semantic.EnumLabels["SUSPENDED"])
	assert.Equal(t, "dimension", probeResponse.Semantic.Role)
}

// TestProbeColumn_Integration_MetadataOnlyColumnMetadata verifies probe_column uses
// column_metadata when ontology has NO column details for this column.
func TestProbeColumn_Integration_MetadataOnlyColumnMetadata(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "orders")
	tc.createSchemaColumn(ctx, tableID, "order_status", "varchar", 4, 1000)

	// Step 2: Create an active ontology WITHOUT column details for this column
	tc.createActiveOntology(ctx)

	// Step 3: Create column_metadata with enum values
	desc := "Current state of the order"
	entity := "Order"
	role := "attribute"
	columnMeta := &models.ColumnMetadata{
		ProjectID:   tc.projectID,
		TableName:   "orders",
		ColumnName:  "order_status",
		Description: &desc,
		Entity:      &entity,
		Role:        &role,
		EnumValues:  []string{"PENDING", "PROCESSING", "SHIPPED", "DELIVERED"},
		Source:      models.ProvenanceMCP,
	}
	err := tc.columnMetadataRepo.Upsert(ctx, columnMeta)
	require.NoError(t, err)

	// Step 4: Probe the column
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "orders",
		"column": "order_status",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Column metadata values should be used
	assert.NotNil(t, probeResponse.Semantic)
	assert.Len(t, probeResponse.Semantic.EnumLabels, 4, "should have 4 values from column_metadata")
	assert.Equal(t, "PENDING", probeResponse.Semantic.EnumLabels["PENDING"])
	assert.Equal(t, "DELIVERED", probeResponse.Semantic.EnumLabels["DELIVERED"])
	assert.Equal(t, "Current state of the order", probeResponse.Semantic.Description)
	assert.Equal(t, "Order", probeResponse.Semantic.Entity)
	assert.Equal(t, "attribute", probeResponse.Semantic.Role)
}

// TestProbeColumn_Integration_MetadataOnlyOntology verifies probe_column uses
// ontology when column_metadata has NO entry for this column.
func TestProbeColumn_Integration_MetadataOnlyOntology(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "products")
	tc.createSchemaColumn(ctx, tableID, "category", "varchar", 10, 5000)

	// Step 2: Create an active ontology WITH column details
	tc.createActiveOntology(ctx)
	columnDetails := []models.ColumnDetail{
		{
			Name:        "category",
			Description: "Product category from extraction",
			Role:        "dimension",
			EnumValues: []models.EnumValue{
				{Value: "ELECTRONICS", Label: "Electronic devices"},
				{Value: "CLOTHING", Label: "Apparel and accessories"},
				{Value: "HOME", Label: "Home and garden"},
			},
		},
	}
	err := tc.ontologyRepo.UpdateColumnDetails(ctx, tc.projectID, "products", columnDetails)
	require.NoError(t, err)

	// NO column_metadata entry

	// Step 3: Probe the column
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "products",
		"column": "category",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Ontology values should be present
	assert.NotNil(t, probeResponse.Semantic)
	assert.Len(t, probeResponse.Semantic.EnumLabels, 3, "should have 3 values from ontology")
	assert.Equal(t, "Electronic devices", probeResponse.Semantic.EnumLabels["ELECTRONICS"])
}

// TestProbeColumn_Integration_NoMetadataAnywhere verifies probe_column works
// when neither ontology nor column_metadata have data for the column.
func TestProbeColumn_Integration_NoMetadataAnywhere(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "logs")
	tc.createSchemaColumn(ctx, tableID, "message", "text", 10000, 50000)

	// Step 2: Create an active ontology (empty, no column details)
	tc.createActiveOntology(ctx)

	// NO column_metadata entry

	// Step 3: Probe the column
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "logs",
		"column": "message",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Statistics should still be present (if available from schema column)
	if probeResponse.Statistics != nil {
		assert.Equal(t, int64(10000), probeResponse.Statistics.DistinctCount)
		assert.Equal(t, int64(50000), probeResponse.Statistics.RowCount)
	}

	// Semantic should be nil (no metadata anywhere)
	assert.Nil(t, probeResponse.Semantic)
}

// TestProbeColumn_Integration_EnumLabelsPersistAcrossSessions verifies that
// enum labels from an approved change persist and are visible in new sessions.
func TestProbeColumn_Integration_EnumLabelsPersistAcrossSessions(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	// Session 1: Create schema and approve a change
	ctx1, cleanup1 := tc.createTestContext()

	tableID := tc.createSchemaTable(ctx1, "payments")
	tc.createSchemaColumn(ctx1, tableID, "payment_method", "varchar", 5, 2000)
	tc.createActiveOntology(ctx1)

	changeID := tc.createPendingChange(ctx1, "payments", "payment_method",
		[]string{"CREDIT_CARD", "DEBIT_CARD", "PAYPAL", "BANK_TRANSFER", "CRYPTO"})

	_, err := tc.changeReviewSvc.ApproveChange(ctx1, changeID, models.ProvenanceMCP)
	require.NoError(t, err)

	// Close session 1 (simulating end of first session)
	cleanup1()

	// Session 2: New context (simulating a new session)
	ctx2, cleanup2 := tc.createTestContext()
	defer cleanup2()

	// Probe the column - should still have enum_labels
	result, err := tc.callTool(ctx2, "probe_column", map[string]any{
		"table":  "payments",
		"column": "payment_method",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Enum labels should persist from the approved change
	assert.NotNil(t, probeResponse.Semantic, "semantic should persist")
	assert.NotNil(t, probeResponse.Semantic.EnumLabels, "enum_labels should persist")
	assert.Len(t, probeResponse.Semantic.EnumLabels, 5, "all 5 enum values should persist")
	assert.Equal(t, "CREDIT_CARD", probeResponse.Semantic.EnumLabels["CREDIT_CARD"])
	assert.Equal(t, "CRYPTO", probeResponse.Semantic.EnumLabels["CRYPTO"])
}

// TestProbeColumn_Integration_PartialMerge verifies that partial metadata from
// different sources is properly merged.
func TestProbeColumn_Integration_PartialMerge(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "invoices")
	tc.createSchemaColumn(ctx, tableID, "invoice_state", "varchar", 4, 3000)

	// Step 2: Create an active ontology with description but NO enum values
	tc.createActiveOntology(ctx)
	columnDetails := []models.ColumnDetail{
		{
			Name:        "invoice_state",
			Description: "Current processing state of the invoice",
			Role:        "attribute",
			EnumValues:  nil, // No enum values in ontology
		},
	}
	err := tc.ontologyRepo.UpdateColumnDetails(ctx, tc.projectID, "invoices", columnDetails)
	require.NoError(t, err)

	// Step 3: Create column_metadata WITH enum values but no description
	columnMeta := &models.ColumnMetadata{
		ProjectID:  tc.projectID,
		TableName:  "invoices",
		ColumnName: "invoice_state",
		EnumValues: []string{"DRAFT", "SENT", "PAID", "OVERDUE"},
		Source:     models.ProvenanceMCP,
	}
	err = tc.columnMetadataRepo.Upsert(ctx, columnMeta)
	require.NoError(t, err)

	// Step 4: Probe the column
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "invoices",
		"column": "invoice_state",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Should have description from ontology and enum values from column_metadata
	assert.NotNil(t, probeResponse.Semantic)
	assert.Equal(t, "Current processing state of the invoice", probeResponse.Semantic.Description, "description from ontology")
	assert.Equal(t, "attribute", probeResponse.Semantic.Role, "role from ontology")

	// Enum labels should come from column_metadata since ontology has none
	assert.Len(t, probeResponse.Semantic.EnumLabels, 4, "enum values from column_metadata")
	assert.Equal(t, "DRAFT", probeResponse.Semantic.EnumLabels["DRAFT"])
	assert.Equal(t, "OVERDUE", probeResponse.Semantic.EnumLabels["OVERDUE"])
}

// TestProbeColumn_Integration_EnumWithDescriptions verifies that probe_column
// returns enum values with their descriptions for integer enum columns.
// This is the key test for BUG-6: Missing Enum Value Extraction.
func TestProbeColumn_Integration_EnumWithDescriptions(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column (billing_transactions.transaction_state)
	tableID := tc.createSchemaTable(ctx, "billing_transactions")
	tc.createSchemaColumn(ctx, tableID, "transaction_state", "integer", 8, 10000)

	// Step 2: Create an active ontology WITH column details that have enum values with descriptions
	// This simulates what happens after column enrichment with project-level enum definitions
	tc.createActiveOntology(ctx)
	columnDetails := []models.ColumnDetail{
		{
			Name:        "transaction_state",
			Description: "State of the billing transaction",
			Role:        "dimension",
			EnumValues: []models.EnumValue{
				{Value: "0", Label: "UNSPECIFIED", Description: "Not set"},
				{Value: "1", Label: "STARTED", Description: "Transaction started"},
				{Value: "2", Label: "ENDED", Description: "Transaction ended"},
				{Value: "3", Label: "WAITING", Description: "Awaiting chargeback period"},
				{Value: "4", Label: "AVAILABLE", Description: "Available for payout"},
				{Value: "5", Label: "PROCESSING", Description: "Processing payout"},
				{Value: "6", Label: "PAYING", Description: "Paying out"},
				{Value: "7", Label: "PAID", Description: "Paid out"},
				{Value: "8", Label: "ERROR", Description: "Error occurred"},
			},
		},
	}
	err := tc.ontologyRepo.UpdateColumnDetails(ctx, tc.projectID, "billing_transactions", columnDetails)
	require.NoError(t, err)

	// Step 3: Probe the column
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "billing_transactions",
		"column": "transaction_state",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// Key assertions: enum_labels should have meaningful labels (not just "1", "2", "3")
	assert.NotNil(t, probeResponse.Semantic, "should have semantic section")
	assert.Equal(t, "dimension", probeResponse.Semantic.Role)
	assert.Equal(t, "State of the billing transaction", probeResponse.Semantic.Description)

	// The critical assertion: enum_labels should have descriptive labels
	assert.NotNil(t, probeResponse.Semantic.EnumLabels, "should have enum_labels")
	assert.Len(t, probeResponse.Semantic.EnumLabels, 9, "should have all 9 transaction states")

	// Label takes precedence over description
	assert.Equal(t, "UNSPECIFIED", probeResponse.Semantic.EnumLabels["0"], "value 0 should have label")
	assert.Equal(t, "STARTED", probeResponse.Semantic.EnumLabels["1"], "value 1 should have label")
	assert.Equal(t, "ENDED", probeResponse.Semantic.EnumLabels["2"], "value 2 should have label")
	assert.Equal(t, "WAITING", probeResponse.Semantic.EnumLabels["3"], "value 3 should have label")
	assert.Equal(t, "AVAILABLE", probeResponse.Semantic.EnumLabels["4"], "value 4 should have label")
	assert.Equal(t, "PROCESSING", probeResponse.Semantic.EnumLabels["5"], "value 5 should have label")
	assert.Equal(t, "PAYING", probeResponse.Semantic.EnumLabels["6"], "value 6 should have label")
	assert.Equal(t, "PAID", probeResponse.Semantic.EnumLabels["7"], "value 7 should have label")
	assert.Equal(t, "ERROR", probeResponse.Semantic.EnumLabels["8"], "value 8 should have label")
}

// TestProbeColumn_Integration_EnumDescriptionFallback verifies that when enum values
// have no Label but have Description, the description is used as the label.
func TestProbeColumn_Integration_EnumDescriptionFallback(t *testing.T) {
	tc := setupProbeColumnIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create schema table and column
	tableID := tc.createSchemaTable(ctx, "activity_logs")
	tc.createSchemaColumn(ctx, tableID, "activity_type", "integer", 5, 5000)

	// Step 2: Create an active ontology with enum values that have Description but no Label
	// This simulates enum values where only descriptions were provided
	tc.createActiveOntology(ctx)
	columnDetails := []models.ColumnDetail{
		{
			Name:        "activity_type",
			Description: "Type of activity logged",
			Role:        "dimension",
			EnumValues: []models.EnumValue{
				{Value: "1", Description: "User logged in"},
				{Value: "2", Description: "User logged out"},
				{Value: "3", Description: "Password changed"},
				{Value: "4", Description: "Profile updated"},
				{Value: "5", Description: "Settings modified"},
			},
		},
	}
	err := tc.ontologyRepo.UpdateColumnDetails(ctx, tc.projectID, "activity_logs", columnDetails)
	require.NoError(t, err)

	// Step 3: Probe the column
	result, err := tc.callTool(ctx, "probe_column", map[string]any{
		"table":  "activity_logs",
		"column": "activity_type",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var probeResponse probeColumnResponse
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &probeResponse)
	require.NoError(t, err)

	// When no Label exists, Description should be used
	assert.NotNil(t, probeResponse.Semantic)
	assert.NotNil(t, probeResponse.Semantic.EnumLabels)
	assert.Len(t, probeResponse.Semantic.EnumLabels, 5)

	// Description is used as the label when Label is empty
	assert.Equal(t, "User logged in", probeResponse.Semantic.EnumLabels["1"])
	assert.Equal(t, "User logged out", probeResponse.Semantic.EnumLabels["2"])
	assert.Equal(t, "Password changed", probeResponse.Semantic.EnumLabels["3"])
	assert.Equal(t, "Profile updated", probeResponse.Semantic.EnumLabels["4"])
	assert.Equal(t, "Settings modified", probeResponse.Semantic.EnumLabels["5"])
}

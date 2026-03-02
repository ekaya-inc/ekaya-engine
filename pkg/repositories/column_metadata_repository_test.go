//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// columnMetadataTestContext holds test dependencies for column metadata repository tests.
type columnMetadataTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	repo         ColumnMetadataRepository
	schemaRepo   SchemaRepository
	projectID    uuid.UUID
	datasourceID uuid.UUID
	testUserID   uuid.UUID
}

func setupColumnMetadataTest(t *testing.T) *columnMetadataTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &columnMetadataTestContext{
		t:            t,
		engineDB:     engineDB,
		repo:         NewColumnMetadataRepository(),
		schemaRepo:   NewSchemaRepository(),
		projectID:    uuid.MustParse("00000000-0000-0000-0000-000000000060"),
		datasourceID: uuid.MustParse("00000000-0000-0000-0000-000000000061"),
		testUserID:   uuid.MustParse("00000000-0000-0000-0000-000000000062"),
	}
	tc.ensureTestProject()
	tc.ensureTestDatasource()
	return tc
}

func (tc *columnMetadataTestContext) ensureTestProject() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Column Metadata Test Project")
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_users (project_id, user_id, role)
		VALUES ($1, $2, 'admin')
		ON CONFLICT (project_id, user_id) DO NOTHING
	`, tc.projectID, tc.testUserID)
	require.NoError(tc.t, err)
}

func (tc *columnMetadataTestContext) ensureTestDatasource() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID, "Column Metadata Test DS", "postgres", "{}")
	require.NoError(tc.t, err)
}

func (tc *columnMetadataTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_column_metadata WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_columns WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
}

func (tc *columnMetadataTestContext) createTestContext() (context.Context, func()) {
	return tc.createTestContextWithSource(models.SourceManual)
}

func (tc *columnMetadataTestContext) createTestContextWithSource(source models.ProvenanceSource) (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	ctx = database.SetTenantScope(ctx, scope)
	ctx = models.WithProvenance(ctx, models.ProvenanceContext{
		Source: source,
		UserID: tc.testUserID,
	})
	return ctx, func() { scope.Close() }
}

func (tc *columnMetadataTestContext) createTestSchemaTable(ctx context.Context, schemaName, tableName string) *models.SchemaTable {
	tc.t.Helper()
	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		SchemaName:   schemaName,
		TableName:    tableName,
	}
	err := tc.schemaRepo.UpsertTable(ctx, table)
	require.NoError(tc.t, err)
	return table
}

func (tc *columnMetadataTestContext) createTestSchemaColumn(ctx context.Context, tableID uuid.UUID, colName string, ordinal int) *models.SchemaColumn {
	tc.t.Helper()
	col := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   tableID,
		ColumnName:      colName,
		DataType:        "text",
		IsNullable:      true,
		OrdinalPosition: ordinal,
	}
	err := tc.schemaRepo.UpsertColumn(ctx, col)
	require.NoError(tc.t, err)
	return col
}

// ============================================================================
// UpsertFromExtraction: Provenance Protection Tests
// ============================================================================

func TestColumnMetadataRepository_UpsertFromExtraction_PreservesMCPDescription(t *testing.T) {
	tc := setupColumnMetadataTest(t)
	tc.cleanup()

	// Step 1: Create schema objects
	ctx, cleanup := tc.createTestContextWithSource(models.SourceMCP)
	defer cleanup()

	table := tc.createTestSchemaTable(ctx, "public", "applications")
	col := tc.createTestSchemaColumn(ctx, table.ID, "name", 1)

	// Step 2: Simulate MCP setting a curated description
	mcpDesc := "Product application name (e.g., 'MCP Server')"
	mcpRole := "attribute"
	mcpMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Description:    &mcpDesc,
		Role:           &mcpRole,
		Source:         models.ProvenanceMCP,
	}
	err := tc.repo.Upsert(ctx, mcpMeta)
	require.NoError(t, err)

	// Verify MCP values are saved with source = 'mcp'
	retrieved, err := tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, mcpDesc, *retrieved.Description)
	assert.Equal(t, mcpRole, *retrieved.Role)
	assert.Equal(t, "mcp", retrieved.Source)

	// Step 3: Simulate re-extraction overwriting with inferred values
	inferredDesc := "Name of a person, company, or entity"
	inferredRole := "dimension"
	classPath := "text"
	extractionMeta := &models.ColumnMetadata{
		ProjectID:          tc.projectID,
		SchemaColumnID:     col.ID,
		Description:        &inferredDesc,
		Role:               &inferredRole,
		ClassificationPath: &classPath,
	}
	err = tc.repo.UpsertFromExtraction(ctx, extractionMeta)
	require.NoError(t, err)

	// Step 4: Verify MCP values are PRESERVED (not overwritten)
	retrieved, err = tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, mcpDesc, *retrieved.Description, "MCP description should be preserved after re-extraction")
	assert.Equal(t, mcpRole, *retrieved.Role, "MCP role should be preserved after re-extraction")
	// Classification path should still be updated (internal field)
	assert.Equal(t, classPath, *retrieved.ClassificationPath, "Classification path should be updated")
}

func TestColumnMetadataRepository_UpsertFromExtraction_PreservesManualDescription(t *testing.T) {
	tc := setupColumnMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContextWithSource(models.SourceManual)
	defer cleanup()

	table := tc.createTestSchemaTable(ctx, "public", "manual_test")
	col := tc.createTestSchemaColumn(ctx, table.ID, "buyer", 1)

	// Simulate manual (admin) edit
	manualDesc := "Target buyer persona for go-to-market"
	manualMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Description:    &manualDesc,
		Source:         models.ProvenanceManual,
	}
	err := tc.repo.Upsert(ctx, manualMeta)
	require.NoError(t, err)

	// Simulate re-extraction
	inferredDesc := "Name of the buyer, which could be a person or a company"
	extractionMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Description:    &inferredDesc,
	}
	err = tc.repo.UpsertFromExtraction(ctx, extractionMeta)
	require.NoError(t, err)

	// Verify manual values are preserved
	retrieved, err := tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, manualDesc, *retrieved.Description, "Manual description should be preserved after re-extraction")
}

func TestColumnMetadataRepository_UpsertFromExtraction_OverwritesInferredValues(t *testing.T) {
	tc := setupColumnMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestSchemaTable(ctx, "public", "inferred_test")
	col := tc.createTestSchemaColumn(ctx, table.ID, "status", 1)

	// Step 1: Initial extraction sets inferred values
	oldDesc := "Status column"
	oldRole := "attribute"
	initial := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Description:    &oldDesc,
		Role:           &oldRole,
	}
	err := tc.repo.UpsertFromExtraction(ctx, initial)
	require.NoError(t, err)

	// Step 2: Re-extraction with better inferred values
	newDesc := "Account status indicator"
	newRole := "dimension"
	updated := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Description:    &newDesc,
		Role:           &newRole,
	}
	err = tc.repo.UpsertFromExtraction(ctx, updated)
	require.NoError(t, err)

	// Step 3: Verify inferred values ARE overwritten (no last_edit_source = mcp/manual)
	retrieved, err := tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, newDesc, *retrieved.Description, "Inferred description should be overwritten by new extraction")
	assert.Equal(t, newRole, *retrieved.Role, "Inferred role should be overwritten by new extraction")
}

func TestColumnMetadataRepository_UpsertFromExtraction_PreservesMCPSemanticType(t *testing.T) {
	tc := setupColumnMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContextWithSource(models.SourceMCP)
	defer cleanup()

	table := tc.createTestSchemaTable(ctx, "public", "semantic_test")
	col := tc.createTestSchemaColumn(ctx, table.ID, "amount", 1)

	// MCP sets semantic_type
	semType := "currency_cents"
	mcpMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		SemanticType:   &semType,
		Source:         models.ProvenanceMCP,
	}
	err := tc.repo.Upsert(ctx, mcpMeta)
	require.NoError(t, err)

	// Re-extraction tries to overwrite
	wrongSemType := "numeric"
	extractionMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		SemanticType:   &wrongSemType,
	}
	err = tc.repo.UpsertFromExtraction(ctx, extractionMeta)
	require.NoError(t, err)

	// Verify MCP value is preserved
	retrieved, err := tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, semType, *retrieved.SemanticType, "MCP semantic_type should be preserved after re-extraction")
}

func TestColumnMetadataRepository_UpsertFromExtraction_PreservesMCPFeatures(t *testing.T) {
	tc := setupColumnMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContextWithSource(models.SourceMCP)
	defer cleanup()

	table := tc.createTestSchemaTable(ctx, "public", "features_test")
	col := tc.createTestSchemaColumn(ctx, table.ID, "status", 1)

	// MCP sets enum features with correct values
	mcpMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Source:         models.ProvenanceMCP,
		Features: models.ColumnMetadataFeatures{
			EnumFeatures: &models.EnumFeatures{
				Values: []models.ColumnEnumValue{
					{Value: "active", Label: "Active account"},
					{Value: "suspended", Label: "Temporarily disabled"},
				},
			},
		},
	}
	err := tc.repo.Upsert(ctx, mcpMeta)
	require.NoError(t, err)

	// Re-extraction tries to overwrite features with wrong values
	extractionMeta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Features: models.ColumnMetadataFeatures{
			EnumFeatures: &models.EnumFeatures{
				Values: []models.ColumnEnumValue{
					{Value: "1", Label: "Value 1"},
					{Value: "2", Label: "Value 2"},
				},
			},
		},
	}
	err = tc.repo.UpsertFromExtraction(ctx, extractionMeta)
	require.NoError(t, err)

	// Verify MCP enum values are preserved
	retrieved, err := tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.NotNil(t, retrieved.Features.EnumFeatures)
	require.Len(t, retrieved.Features.EnumFeatures.Values, 2)
	assert.Equal(t, "active", retrieved.Features.EnumFeatures.Values[0].Value, "MCP enum values should be preserved")
	assert.Equal(t, "Active account", retrieved.Features.EnumFeatures.Values[0].Label, "MCP enum labels should be preserved")
}

func TestColumnMetadataRepository_UpsertFromExtraction_NewColumnGetsInferredValues(t *testing.T) {
	tc := setupColumnMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestSchemaTable(ctx, "public", "new_col_test")
	col := tc.createTestSchemaColumn(ctx, table.ID, "new_column", 1)

	// Brand new column - no existing metadata
	desc := "Inferred description for new column"
	role := "attribute"
	meta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: col.ID,
		Description:    &desc,
		Role:           &role,
	}
	err := tc.repo.UpsertFromExtraction(ctx, meta)
	require.NoError(t, err)

	// Verify inferred values are set correctly
	retrieved, err := tc.repo.GetBySchemaColumnID(ctx, col.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, desc, *retrieved.Description)
	assert.Equal(t, role, *retrieved.Role)
	assert.Equal(t, "inferred", retrieved.Source)
}

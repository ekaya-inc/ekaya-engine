//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// tableMetadataTestContext holds test dependencies for table metadata repository tests.
type tableMetadataTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	repo         TableMetadataRepository
	schemaRepo   SchemaRepository
	projectID    uuid.UUID
	datasourceID uuid.UUID
	testUserID   uuid.UUID
}

// setupTableMetadataTest initializes the test context with shared testcontainer.
func setupTableMetadataTest(t *testing.T) *tableMetadataTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &tableMetadataTestContext{
		t:            t,
		engineDB:     engineDB,
		repo:         NewTableMetadataRepository(),
		schemaRepo:   NewSchemaRepository(),
		projectID:    uuid.MustParse("00000000-0000-0000-0000-000000000050"),
		datasourceID: uuid.MustParse("00000000-0000-0000-0000-000000000051"),
		testUserID:   uuid.MustParse("00000000-0000-0000-0000-000000000052"),
	}
	tc.ensureTestProject()
	tc.ensureTestDatasource()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *tableMetadataTestContext) ensureTestProject() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Table Metadata Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Create test user for provenance FK constraints
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_users (project_id, user_id, role)
		VALUES ($1, $2, 'admin')
		ON CONFLICT (project_id, user_id) DO NOTHING
	`, tc.projectID, tc.testUserID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test user: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *tableMetadataTestContext) ensureTestDatasource() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope for datasource setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID, "Table Metadata Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("failed to ensure test datasource: %v", err)
	}
}

// cleanup removes test table metadata and schema tables.
func (tc *tableMetadataTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_table_metadata WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and manual provenance.
func (tc *tableMetadataTestContext) createTestContext() (context.Context, func()) {
	return tc.createTestContextWithSource(models.SourceManual)
}

// createTestContextWithSource returns a context with tenant scope and specified provenance source.
func (tc *tableMetadataTestContext) createTestContextWithSource(source models.ProvenanceSource) (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	ctx = models.WithProvenance(ctx, models.ProvenanceContext{
		Source: source,
		UserID: tc.testUserID,
	})
	return ctx, func() { scope.Close() }
}

// createTestSchemaTable creates a schema table for testing and returns it.
func (tc *tableMetadataTestContext) createTestSchemaTable(ctx context.Context, schemaName, tableName string) *models.SchemaTable {
	tc.t.Helper()
	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		SchemaName:   schemaName,
		TableName:    tableName,
	}
	err := tc.schemaRepo.UpsertTable(ctx, table)
	if err != nil {
		tc.t.Fatalf("failed to create test schema table: %v", err)
	}
	return table
}

// createTestMetadata creates table metadata for testing.
func (tc *tableMetadataTestContext) createTestMetadata(ctx context.Context, schemaTableID uuid.UUID, description *string) *models.TableMetadata {
	tc.t.Helper()
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTableID,
		Description:   description,
	}
	err := tc.repo.Upsert(ctx, meta)
	if err != nil {
		tc.t.Fatalf("failed to create test metadata: %v", err)
	}
	return meta
}

// ============================================================================
// Upsert Tests (Insert)
// ============================================================================

func TestTableMetadataRepository_Upsert_Insert(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "users")

	desc := "Users table containing all registered users"
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
	}

	err := tc.repo.Upsert(ctx, meta)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if meta.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if meta.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected metadata, got nil")
	}
	if retrieved.Description == nil || *retrieved.Description != desc {
		t.Errorf("expected description %q, got %v", desc, retrieved.Description)
	}
}

func TestTableMetadataRepository_Upsert_InsertWithAllFields(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "sessions")

	desc := "Transient session tracking table"
	notes := "Do not use for analytics. Sessions are deleted after 24 hours."
	alt := "billing_engagements"
	meta := &models.TableMetadata{
		ProjectID:            tc.projectID,
		SchemaTableID:        schemaTable.ID,
		Description:          &desc,
		UsageNotes:           &notes,
		IsEphemeral:          true,
		PreferredAlternative: &alt,
	}

	err := tc.repo.Upsert(ctx, meta)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected metadata, got nil")
	}
	if retrieved.Description == nil || *retrieved.Description != desc {
		t.Errorf("expected description %q, got %v", desc, retrieved.Description)
	}
	if retrieved.UsageNotes == nil || *retrieved.UsageNotes != notes {
		t.Errorf("expected usage_notes %q, got %v", notes, retrieved.UsageNotes)
	}
	if !retrieved.IsEphemeral {
		t.Error("expected is_ephemeral to be true")
	}
	if retrieved.PreferredAlternative == nil || *retrieved.PreferredAlternative != alt {
		t.Errorf("expected preferred_alternative %q, got %v", alt, retrieved.PreferredAlternative)
	}
}

// ============================================================================
// Upsert Tests (Update)
// ============================================================================

func TestTableMetadataRepository_Upsert_Update(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "orders")

	// Create initial metadata
	desc1 := "Original description"
	tc.createTestMetadata(ctx, schemaTable.ID, &desc1)

	// Update with same schema_table_id
	desc2 := "Updated description"
	notes := "Use this for order analytics"
	updated := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc2,
		UsageNotes:    &notes,
	}

	err := tc.repo.Upsert(ctx, updated)
	if err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}

	// Verify updated
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved.Description == nil || *retrieved.Description != desc2 {
		t.Errorf("expected description %q, got %v", desc2, retrieved.Description)
	}
	if retrieved.UsageNotes == nil || *retrieved.UsageNotes != notes {
		t.Errorf("expected usage_notes %q, got %v", notes, retrieved.UsageNotes)
	}
}

func TestTableMetadataRepository_Upsert_UpdatePreservesExistingFields(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "products")

	// Create initial metadata with description and usage_notes
	desc := "Original description"
	notes := "Original notes"
	initial := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
		UsageNotes:    &notes,
	}
	err := tc.repo.Upsert(ctx, initial)
	if err != nil {
		t.Fatalf("Initial upsert failed: %v", err)
	}

	// Update with only is_ephemeral (no description or usage_notes)
	updated := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		IsEphemeral:   true,
		// Description and UsageNotes are nil
	}

	err = tc.repo.Upsert(ctx, updated)
	if err != nil {
		t.Fatalf("Update upsert failed: %v", err)
	}

	// Verify existing fields are preserved via COALESCE
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved.Description == nil || *retrieved.Description != desc {
		t.Errorf("expected description to be preserved as %q, got %v", desc, retrieved.Description)
	}
	if retrieved.UsageNotes == nil || *retrieved.UsageNotes != notes {
		t.Errorf("expected usage_notes to be preserved as %q, got %v", notes, retrieved.UsageNotes)
	}
	if !retrieved.IsEphemeral {
		t.Error("expected is_ephemeral to be updated to true")
	}
}

// ============================================================================
// GetBySchemaTableID Tests
// ============================================================================

func TestTableMetadataRepository_GetBySchemaTableID_Success(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "test_table")

	desc := "Test table"
	tc.createTestMetadata(ctx, schemaTable.ID, &desc)

	meta, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.SchemaTableID != schemaTable.ID {
		t.Errorf("expected schema_table_id %q, got %q", schemaTable.ID, meta.SchemaTableID)
	}
}

func TestTableMetadataRepository_GetBySchemaTableID_NotFound(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	meta, err := tc.repo.GetBySchemaTableID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetBySchemaTableID should not error for not found: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil for not found, got %+v", meta)
	}
}

// ============================================================================
// List Tests
// ============================================================================

func TestTableMetadataRepository_List_Success(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create schema tables first
	table1 := tc.createTestSchemaTable(ctx, "public", "alpha_table")
	table2 := tc.createTestSchemaTable(ctx, "public", "beta_table")
	table3 := tc.createTestSchemaTable(ctx, "public", "gamma_table")

	desc1 := "Table 1"
	desc2 := "Table 2"
	desc3 := "Table 3"
	tc.createTestMetadata(ctx, table1.ID, &desc1)
	tc.createTestMetadata(ctx, table2.ID, &desc2)
	tc.createTestMetadata(ctx, table3.ID, &desc3)

	metas, err := tc.repo.List(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 3 {
		t.Errorf("expected 3 metadata entries, got %d", len(metas))
	}
}

func TestTableMetadataRepository_List_Empty(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	metas, err := tc.repo.List(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 metadata entries, got %d", len(metas))
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestTableMetadataRepository_Delete_Success(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "deleteme")

	desc := "To be deleted"
	tc.createTestMetadata(ctx, schemaTable.ID, &desc)

	err := tc.repo.Delete(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected metadata to be deleted")
	}
}

func TestTableMetadataRepository_Delete_NotFound(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Deleting non-existent should not error (standard SQL DELETE behavior)
	err := tc.repo.Delete(ctx, uuid.New())
	if err != nil {
		t.Errorf("Delete should not error for non-existent: %v", err)
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestTableMetadataRepository_NoTenantScope(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	// First create a schema table with proper context
	ctx, cleanup := tc.createTestContext()
	schemaTable := tc.createTestSchemaTable(ctx, "public", "test")
	cleanup()

	// Now test operations without tenant scope
	ctx = context.Background() // No tenant scope

	desc := "Test"
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
	}

	// Upsert should fail
	err := tc.repo.Upsert(ctx, meta)
	if err == nil {
		t.Error("expected error for Upsert without tenant scope")
	}

	// GetBySchemaTableID should fail
	_, err = tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err == nil {
		t.Error("expected error for GetBySchemaTableID without tenant scope")
	}

	// List should fail
	_, err = tc.repo.List(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for List without tenant scope")
	}

	// Delete should fail
	err = tc.repo.Delete(ctx, schemaTable.ID)
	if err == nil {
		t.Error("expected error for Delete without tenant scope")
	}
}

// ============================================================================
// Provenance Tests
// ============================================================================

func TestTableMetadataRepository_Upsert_Provenance_Create(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	// Test with manual provenance
	ctx, cleanup := tc.createTestContextWithSource(models.SourceManual)
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "provenance_test")

	desc := "Testing provenance on create"
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
	}

	err := tc.repo.Upsert(ctx, meta)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify Source was set from context
	if meta.Source != "manual" {
		t.Errorf("expected Source 'manual', got %q", meta.Source)
	}
	// Verify CreatedBy was set from context
	if meta.CreatedBy == nil {
		t.Error("expected CreatedBy to be set")
	}
	if meta.CreatedBy != nil && *meta.CreatedBy != tc.testUserID {
		t.Errorf("expected CreatedBy to be %v, got %v", tc.testUserID, *meta.CreatedBy)
	}

	// Verify persisted correctly
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved.Source != "manual" {
		t.Errorf("expected persisted Source 'manual', got %q", retrieved.Source)
	}
	if retrieved.CreatedBy == nil || *retrieved.CreatedBy != tc.testUserID {
		t.Errorf("expected persisted CreatedBy %v, got %v", tc.testUserID, retrieved.CreatedBy)
	}
}

func TestTableMetadataRepository_Upsert_Provenance_MCP(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	// Test with MCP provenance
	ctx, cleanup := tc.createTestContextWithSource(models.SourceMCP)
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "mcp_test")

	desc := "Created by MCP tool"
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
	}

	err := tc.repo.Upsert(ctx, meta)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify Source was set from context
	if meta.Source != "mcp" {
		t.Errorf("expected Source 'mcp', got %q", meta.Source)
	}

	// Verify persisted correctly
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved.Source != "mcp" {
		t.Errorf("expected persisted Source 'mcp', got %q", retrieved.Source)
	}
}

func TestTableMetadataRepository_Upsert_NoProvenance(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	// First create a schema table with proper context
	ctx, cleanup := tc.createTestContextWithSource(models.SourceManual)
	schemaTable := tc.createTestSchemaTable(ctx, "public", "no_provenance")
	cleanup()

	// Create context with tenant scope but NO provenance
	ctx = context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)
	// Note: no provenance set

	desc := "No provenance"
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
	}

	err = tc.repo.Upsert(ctx, meta)
	if err == nil {
		t.Error("expected error when creating without provenance context")
	}
	if err != nil && err.Error() != "provenance context required" {
		t.Errorf("expected 'provenance context required' error, got: %v", err)
	}
}

// ============================================================================
// UpsertFromExtraction Tests
// ============================================================================

func TestTableMetadataRepository_UpsertFromExtraction(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a schema table first
	schemaTable := tc.createTestSchemaTable(ctx, "public", "extraction_test")

	desc := "Extracted table description"
	tableType := "transactional"
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTable.ID,
		Description:   &desc,
		TableType:     &tableType,
	}

	err := tc.repo.UpsertFromExtraction(ctx, meta)
	if err != nil {
		t.Fatalf("UpsertFromExtraction failed: %v", err)
	}

	// Verify Source was set to 'inferred'
	if meta.Source != "inferred" {
		t.Errorf("expected Source 'inferred', got %q", meta.Source)
	}

	// Verify persisted correctly
	retrieved, err := tc.repo.GetBySchemaTableID(ctx, schemaTable.ID)
	if err != nil {
		t.Fatalf("GetBySchemaTableID failed: %v", err)
	}
	if retrieved.Source != "inferred" {
		t.Errorf("expected persisted Source 'inferred', got %q", retrieved.Source)
	}
	if retrieved.TableType == nil || *retrieved.TableType != tableType {
		t.Errorf("expected table_type %q, got %v", tableType, retrieved.TableType)
	}
}

// ============================================================================
// ListByTableNames Tests
// ============================================================================

func TestTableMetadataRepository_ListByTableNames(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create schema tables
	table1 := tc.createTestSchemaTable(ctx, "public", "users")
	table2 := tc.createTestSchemaTable(ctx, "public", "orders")
	_ = tc.createTestSchemaTable(ctx, "public", "products") // No metadata for this one

	// Create metadata for some tables
	desc1 := "Users table"
	desc2 := "Orders table"
	tc.createTestMetadata(ctx, table1.ID, &desc1)
	tc.createTestMetadata(ctx, table2.ID, &desc2)

	// Query by table names
	result, err := tc.repo.ListByTableNames(ctx, tc.projectID, []string{"users", "orders", "products"})
	if err != nil {
		t.Fatalf("ListByTableNames failed: %v", err)
	}

	// Should have 2 entries (users and orders have metadata, products does not)
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}

	// Verify users metadata
	if usersMeta, ok := result["users"]; !ok {
		t.Error("expected 'users' in result")
	} else if usersMeta.Description == nil || *usersMeta.Description != desc1 {
		t.Errorf("expected users description %q, got %v", desc1, usersMeta.Description)
	}

	// Verify orders metadata
	if ordersMeta, ok := result["orders"]; !ok {
		t.Error("expected 'orders' in result")
	} else if ordersMeta.Description == nil || *ordersMeta.Description != desc2 {
		t.Errorf("expected orders description %q, got %v", desc2, ordersMeta.Description)
	}

	// Verify products is not in result (no metadata)
	if _, ok := result["products"]; ok {
		t.Error("expected 'products' to not be in result (no metadata)")
	}
}

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

// cleanup removes test table metadata.
func (tc *tableMetadataTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_table_metadata WHERE project_id = $1", tc.projectID)
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

// createTestMetadata creates table metadata for testing.
func (tc *tableMetadataTestContext) createTestMetadata(ctx context.Context, tableName string, description *string) *models.TableMetadata {
	tc.t.Helper()
	meta := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    tableName,
		Description:  description,
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

	desc := "Users table containing all registered users"
	meta := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "users",
		Description:  &desc,
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
	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "users")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
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

	desc := "Transient session tracking table"
	notes := "Do not use for analytics. Sessions are deleted after 24 hours."
	alt := "billing_engagements"
	meta := &models.TableMetadata{
		ProjectID:            tc.projectID,
		DatasourceID:         tc.datasourceID,
		TableName:            "sessions",
		Description:          &desc,
		UsageNotes:           &notes,
		IsEphemeral:          true,
		PreferredAlternative: &alt,
	}

	err := tc.repo.Upsert(ctx, meta)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "sessions")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
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

	// Create initial metadata
	desc1 := "Original description"
	tc.createTestMetadata(ctx, "orders", &desc1)

	// Update with same table name
	desc2 := "Updated description"
	notes := "Use this for order analytics"
	updated := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "orders",
		Description:  &desc2,
		UsageNotes:   &notes,
	}

	err := tc.repo.Upsert(ctx, updated)
	if err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}

	// Verify updated
	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "orders")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
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

	// Create initial metadata with description and usage_notes
	desc := "Original description"
	notes := "Original notes"
	initial := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "products",
		Description:  &desc,
		UsageNotes:   &notes,
	}
	err := tc.repo.Upsert(ctx, initial)
	if err != nil {
		t.Fatalf("Initial upsert failed: %v", err)
	}

	// Update with only is_ephemeral (no description or usage_notes)
	updated := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "products",
		IsEphemeral:  true,
		// Description and UsageNotes are nil
	}

	err = tc.repo.Upsert(ctx, updated)
	if err != nil {
		t.Fatalf("Update upsert failed: %v", err)
	}

	// Verify existing fields are preserved via COALESCE
	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "products")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
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
// Get Tests
// ============================================================================

func TestTableMetadataRepository_Get_Success(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	desc := "Test table"
	tc.createTestMetadata(ctx, "test_table", &desc)

	meta, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "test_table")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.TableName != "test_table" {
		t.Errorf("expected table_name 'test_table', got %q", meta.TableName)
	}
}

func TestTableMetadataRepository_Get_NotFound(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	meta, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "nonexistent")
	if err != nil {
		t.Fatalf("Get should not error for not found: %v", err)
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

	desc1 := "Table 1"
	desc2 := "Table 2"
	desc3 := "Table 3"
	tc.createTestMetadata(ctx, "alpha_table", &desc1)
	tc.createTestMetadata(ctx, "beta_table", &desc2)
	tc.createTestMetadata(ctx, "gamma_table", &desc3)

	metas, err := tc.repo.List(ctx, tc.projectID, tc.datasourceID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 3 {
		t.Errorf("expected 3 metadata entries, got %d", len(metas))
	}

	// Verify ordering by table_name
	if metas[0].TableName != "alpha_table" {
		t.Errorf("expected first table to be 'alpha_table', got %q", metas[0].TableName)
	}
	if metas[1].TableName != "beta_table" {
		t.Errorf("expected second table to be 'beta_table', got %q", metas[1].TableName)
	}
	if metas[2].TableName != "gamma_table" {
		t.Errorf("expected third table to be 'gamma_table', got %q", metas[2].TableName)
	}
}

func TestTableMetadataRepository_List_Empty(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	metas, err := tc.repo.List(ctx, tc.projectID, tc.datasourceID)
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

	desc := "To be deleted"
	tc.createTestMetadata(ctx, "deleteme", &desc)

	err := tc.repo.Delete(ctx, tc.projectID, tc.datasourceID, "deleteme")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "deleteme")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
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
	err := tc.repo.Delete(ctx, tc.projectID, tc.datasourceID, "nonexistent")
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

	ctx := context.Background() // No tenant scope

	desc := "Test"
	meta := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "test",
		Description:  &desc,
	}

	// Upsert should fail
	err := tc.repo.Upsert(ctx, meta)
	if err == nil {
		t.Error("expected error for Upsert without tenant scope")
	}

	// Get should fail
	_, err = tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "test")
	if err == nil {
		t.Error("expected error for Get without tenant scope")
	}

	// List should fail
	_, err = tc.repo.List(ctx, tc.projectID, tc.datasourceID)
	if err == nil {
		t.Error("expected error for List without tenant scope")
	}

	// Delete should fail
	err = tc.repo.Delete(ctx, tc.projectID, tc.datasourceID, "test")
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

	desc := "Testing provenance on create"
	meta := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "provenance_test",
		Description:  &desc,
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
	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "provenance_test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
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

	desc := "Created by MCP tool"
	meta := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "mcp_test",
		Description:  &desc,
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
	retrieved, err := tc.repo.Get(ctx, tc.projectID, tc.datasourceID, "mcp_test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Source != "mcp" {
		t.Errorf("expected persisted Source 'mcp', got %q", retrieved.Source)
	}
}

func TestTableMetadataRepository_Upsert_NoProvenance(t *testing.T) {
	tc := setupTableMetadataTest(t)
	tc.cleanup()

	// Create context with tenant scope but NO provenance
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)
	// Note: no provenance set

	desc := "No provenance"
	meta := &models.TableMetadata{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		TableName:    "no_provenance",
		Description:  &desc,
	}

	err = tc.repo.Upsert(ctx, meta)
	if err == nil {
		t.Error("expected error when creating without provenance context")
	}
	if err != nil && err.Error() != "provenance context required" {
		t.Errorf("expected 'provenance context required' error, got: %v", err)
	}
}

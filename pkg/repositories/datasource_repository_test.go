//go:build integration

package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// datasourceTestContext holds all dependencies for datasource repository integration tests.
type datasourceTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      DatasourceRepository
	projectID uuid.UUID
}

// setupDatasourceTest creates a test context with real database.
func setupDatasourceTest(t *testing.T) *datasourceTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	repo := NewDatasourceRepository()

	// Use fixed ID for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000010")

	tc := &datasourceTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      repo,
		projectID: projectID,
	}

	// Ensure project exists
	tc.ensureTestProject()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *datasourceTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *datasourceTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Datasource Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanup removes all datasources for the test project.
func (tc *datasourceTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE project_id = $1", tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup datasources: %v", err)
	}
}

// createTestDatasource creates a test datasource and returns it.
func (tc *datasourceTestContext) createTestDatasource(ctx context.Context, name, dsType, config string) *models.Datasource {
	tc.t.Helper()

	ds := &models.Datasource{
		ProjectID:      tc.projectID,
		Name:           name,
		DatasourceType: dsType,
	}

	if err := tc.repo.Create(ctx, ds, config); err != nil {
		tc.t.Fatalf("Failed to create test datasource: %v", err)
	}

	return ds
}

// ============================================================================
// Create Tests
// ============================================================================

func TestDatasourceRepository_Create_Success(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	ds := &models.Datasource{
		ProjectID:      tc.projectID,
		Name:           "Test Database",
		DatasourceType: "postgres",
	}
	encryptedConfig := "encrypted_config_data_here"

	err := tc.repo.Create(ctx, ds, encryptedConfig)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if ds.ID == uuid.Nil {
		t.Error("expected ID to be assigned")
	}

	// Verify timestamps were set
	if ds.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if ds.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify data was persisted
	retrieved, config, err := tc.repo.GetByID(ctx, tc.projectID, ds.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.Name != "Test Database" {
		t.Errorf("expected Name 'Test Database', got %q", retrieved.Name)
	}
	if retrieved.DatasourceType != "postgres" {
		t.Errorf("expected DatasourceType 'postgres', got %q", retrieved.DatasourceType)
	}
	if config != encryptedConfig {
		t.Errorf("expected config %q, got %q", encryptedConfig, config)
	}
}

func TestDatasourceRepository_Create_OneDatasourceLimit(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create first datasource - should succeed
	tc.createTestDatasource(ctx, "First DB", "postgres", "config1")

	// Try to create second datasource - should fail with limit error
	ds2 := &models.Datasource{
		ProjectID:      tc.projectID,
		Name:           "Second DB",
		DatasourceType: "postgres",
	}

	err := tc.repo.Create(ctx, ds2, "config2")
	if err == nil {
		t.Fatal("expected error when creating second datasource")
	}

	if !errors.Is(err, apperrors.ErrDatasourceLimitReached) {
		t.Errorf("expected ErrDatasourceLimitReached, got %v", err)
	}
}

func TestDatasourceRepository_Create_DuplicateName(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create first datasource
	tc.createTestDatasource(ctx, "Unique Name", "postgres", "config1")

	// Delete it to allow another create (bypassing one-per-project limit)
	// Actually, the one-per-project limit would trigger first, so this test
	// validates that the limit check happens before the unique constraint.
	// The duplicate name check only matters if we ever allow multiple datasources.

	// For now, let's verify the limit error takes precedence
	ds2 := &models.Datasource{
		ProjectID:      tc.projectID,
		Name:           "Unique Name", // Same name
		DatasourceType: "postgres",
	}

	err := tc.repo.Create(ctx, ds2, "config2")
	if err == nil {
		t.Fatal("expected error")
	}

	// With one-per-project policy, limit error fires first
	if !errors.Is(err, apperrors.ErrDatasourceLimitReached) {
		t.Errorf("expected ErrDatasourceLimitReached (limit check happens first), got %v", err)
	}
}

// ============================================================================
// GetByID Tests
// ============================================================================

func TestDatasourceRepository_GetByID_Success(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestDatasource(ctx, "Get Test DB", "postgres", "encrypted_data")

	retrieved, config, err := tc.repo.GetByID(ctx, tc.projectID, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, retrieved.ID)
	}
	if retrieved.Name != "Get Test DB" {
		t.Errorf("expected Name 'Get Test DB', got %q", retrieved.Name)
	}
	if config != "encrypted_data" {
		t.Errorf("expected config 'encrypted_data', got %q", config)
	}
}

func TestDatasourceRepository_GetByID_NotFound(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.New()

	_, _, err := tc.repo.GetByID(ctx, tc.projectID, nonExistentID)
	if err == nil {
		t.Fatal("expected error for non-existent datasource")
	}

	if err.Error() != "datasource not found" {
		t.Errorf("expected 'datasource not found' error, got %v", err)
	}
}

// ============================================================================
// GetByName Tests
// ============================================================================

func TestDatasourceRepository_GetByName_Success(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestDatasource(ctx, "Named Database", "postgres", "config_by_name")

	retrieved, config, err := tc.repo.GetByName(ctx, tc.projectID, "Named Database")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, retrieved.ID)
	}
	if config != "config_by_name" {
		t.Errorf("expected config 'config_by_name', got %q", config)
	}
}

func TestDatasourceRepository_GetByName_NotFound(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	_, _, err := tc.repo.GetByName(ctx, tc.projectID, "NonExistent Name")
	if err == nil {
		t.Fatal("expected error for non-existent datasource")
	}

	if err.Error() != "datasource not found" {
		t.Errorf("expected 'datasource not found' error, got %v", err)
	}
}

// ============================================================================
// List Tests
// ============================================================================

func TestDatasourceRepository_List_Success(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a datasource
	tc.createTestDatasource(ctx, "List Test DB", "postgres", "list_config")

	datasources, configs, err := tc.repo.List(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(datasources) != 1 {
		t.Errorf("expected 1 datasource, got %d", len(datasources))
	}

	if len(configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(configs))
	}

	if datasources[0].Name != "List Test DB" {
		t.Errorf("expected Name 'List Test DB', got %q", datasources[0].Name)
	}

	if configs[0] != "list_config" {
		t.Errorf("expected config 'list_config', got %q", configs[0])
	}
}

func TestDatasourceRepository_List_Empty(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	datasources, configs, err := tc.repo.List(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(datasources) != 0 {
		t.Errorf("expected 0 datasources, got %d", len(datasources))
	}

	if len(configs) != 0 {
		t.Errorf("expected 0 configs, got %d", len(configs))
	}
}

// ============================================================================
// Update Tests
// ============================================================================

func TestDatasourceRepository_Update_Success(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestDatasource(ctx, "Original Name", "postgres", "original_config")

	// Update all fields
	err := tc.repo.Update(ctx, created.ID, "Updated Name", "clickhouse", "updated_config")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify updates
	retrieved, config, err := tc.repo.GetByID(ctx, tc.projectID, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("expected Name 'Updated Name', got %q", retrieved.Name)
	}
	if retrieved.DatasourceType != "clickhouse" {
		t.Errorf("expected DatasourceType 'clickhouse', got %q", retrieved.DatasourceType)
	}
	if config != "updated_config" {
		t.Errorf("expected config 'updated_config', got %q", config)
	}
}

func TestDatasourceRepository_Update_NotFound(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.New()

	err := tc.repo.Update(ctx, nonExistentID, "Name", "postgres", "config")
	if err == nil {
		t.Fatal("expected error for non-existent datasource")
	}

	if err.Error() != "datasource not found" {
		t.Errorf("expected 'datasource not found' error, got %v", err)
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestDatasourceRepository_Delete_Success(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestDatasource(ctx, "Delete Test DB", "postgres", "delete_config")

	err := tc.repo.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, _, err = tc.repo.GetByID(ctx, tc.projectID, created.ID)
	if err == nil {
		t.Error("expected datasource to be deleted")
	}

	if err.Error() != "datasource not found" {
		t.Errorf("expected 'datasource not found' error, got %v", err)
	}
}

func TestDatasourceRepository_Delete_NotFound(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.New()

	err := tc.repo.Delete(ctx, nonExistentID)
	if err == nil {
		t.Fatal("expected error for non-existent datasource")
	}

	if err.Error() != "datasource not found" {
		t.Errorf("expected 'datasource not found' error, got %v", err)
	}
}

func TestDatasourceRepository_Delete_AllowsNewCreate(t *testing.T) {
	tc := setupDatasourceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create and delete
	first := tc.createTestDatasource(ctx, "First DB", "postgres", "config1")
	err := tc.repo.Delete(ctx, first.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should be able to create again (one-per-project limit should allow)
	second := tc.createTestDatasource(ctx, "Second DB", "postgres", "config2")
	if second.ID == uuid.Nil {
		t.Error("expected second datasource to be created after deletion")
	}
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestDatasourceRepository_NoTenantScope(t *testing.T) {
	tc := setupDatasourceTest(t)

	// Use context WITHOUT tenant scope
	ctx := context.Background()

	_, _, err := tc.repo.List(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error when no tenant scope")
	}

	expectedErr := "no tenant scope in context"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestDatasourceRepository_NoTenantScope_AllMethods(t *testing.T) {
	tc := setupDatasourceTest(t)

	// Use context WITHOUT tenant scope
	ctx := context.Background()
	expectedErr := "no tenant scope in context"

	// Create
	ds := &models.Datasource{ProjectID: tc.projectID, Name: "test", DatasourceType: "postgres"}
	err := tc.repo.Create(ctx, ds, "config")
	if err == nil || err.Error() != expectedErr {
		t.Errorf("Create: expected %q, got %v", expectedErr, err)
	}

	// GetByID
	_, _, err = tc.repo.GetByID(ctx, tc.projectID, uuid.New())
	if err == nil || err.Error() != expectedErr {
		t.Errorf("GetByID: expected %q, got %v", expectedErr, err)
	}

	// GetByName
	_, _, err = tc.repo.GetByName(ctx, tc.projectID, "name")
	if err == nil || err.Error() != expectedErr {
		t.Errorf("GetByName: expected %q, got %v", expectedErr, err)
	}

	// Update
	err = tc.repo.Update(ctx, uuid.New(), "name", "type", "config")
	if err == nil || err.Error() != expectedErr {
		t.Errorf("Update: expected %q, got %v", expectedErr, err)
	}

	// Delete
	err = tc.repo.Delete(ctx, uuid.New())
	if err == nil || err.Error() != expectedErr {
		t.Errorf("Delete: expected %q, got %v", expectedErr, err)
	}
}

//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// queryTestContext holds all dependencies for query repository integration tests.
type queryTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      QueryRepository
	projectID uuid.UUID
	dsID      uuid.UUID // test datasource ID
}

// setupQueryTest creates a test context with real database.
func setupQueryTest(t *testing.T) *queryTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	repo := NewQueryRepository()

	// Use fixed IDs for consistent testing (different from schema tests)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	dsID := uuid.MustParse("00000000-0000-0000-0000-000000000005")

	tc := &queryTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      repo,
		projectID: projectID,
		dsID:      dsID,
	}

	// Ensure project and datasource exist
	tc.ensureTestProject()
	tc.ensureTestDatasource()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *queryTestContext) createTestContext() (context.Context, func()) {
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
func (tc *queryTestContext) ensureTestProject() {
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
	`, tc.projectID, "Query Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *queryTestContext) ensureTestDatasource() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID, "Query Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all query data for the test project.
func (tc *queryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		DELETE FROM engine_queries
		WHERE project_id = $1
	`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup queries: %v", err)
	}
}

// createTestQuery creates a test query and returns it.
func (tc *queryTestContext) createTestQuery(ctx context.Context, prompt, sqlQuery string) *models.Query {
	tc.t.Helper()

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: prompt,
		SQLQuery:              sqlQuery,
		Dialect:               "postgres",
		IsEnabled:             true,
		UsageCount:            0,
	}

	if err := tc.repo.Create(ctx, query); err != nil {
		tc.t.Fatalf("Failed to create test query: %v", err)
	}

	return query
}

// ============================================================================
// CRUD Tests
// ============================================================================

func TestQueryRepository_Create(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Get all users",
		SQLQuery:              "SELECT * FROM users",
		Dialect:               "postgres",
		IsEnabled:             true,
		UsageCount:            0,
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if query.ID == uuid.Nil {
		t.Error("expected ID to be assigned")
	}

	// Verify timestamps were set
	if query.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if query.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify data was persisted
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.NaturalLanguagePrompt != "Get all users" {
		t.Errorf("expected NaturalLanguagePrompt 'Get all users', got %q", retrieved.NaturalLanguagePrompt)
	}
	if retrieved.SQLQuery != "SELECT * FROM users" {
		t.Errorf("expected SQLQuery 'SELECT * FROM users', got %q", retrieved.SQLQuery)
	}
	if retrieved.Dialect != "postgres" {
		t.Errorf("expected Dialect 'postgres', got %q", retrieved.Dialect)
	}
	if !retrieved.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}
	if retrieved.UsageCount != 0 {
		t.Errorf("expected UsageCount 0, got %d", retrieved.UsageCount)
	}
}

func TestQueryRepository_Create_WithAdditionalContext(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	context := "Only return active users"
	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Get active users",
		AdditionalContext:     &context,
		SQLQuery:              "SELECT * FROM users WHERE status = 'active'",
		Dialect:               "postgres",
		IsEnabled:             true,
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.AdditionalContext == nil || *retrieved.AdditionalContext != context {
		t.Errorf("expected AdditionalContext %q, got %v", context, retrieved.AdditionalContext)
	}
}

func TestQueryRepository_GetByID(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestQuery(ctx, "Test query", "SELECT 1")

	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, retrieved.ID)
	}
	if retrieved.NaturalLanguagePrompt != "Test query" {
		t.Errorf("expected NaturalLanguagePrompt 'Test query', got %q", retrieved.NaturalLanguagePrompt)
	}
}

func TestQueryRepository_GetByID_NotFound(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.New()
	_, err := tc.repo.GetByID(ctx, tc.projectID, nonExistentID)
	if err == nil {
		t.Fatal("expected error for non-existent query")
	}
	if err.Error() != "query not found" {
		t.Errorf("expected 'query not found' error, got %q", err.Error())
	}
}

func TestQueryRepository_ListByDatasource(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple queries
	tc.createTestQuery(ctx, "Query 1", "SELECT 1")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	tc.createTestQuery(ctx, "Query 2", "SELECT 2")
	time.Sleep(10 * time.Millisecond)
	tc.createTestQuery(ctx, "Query 3", "SELECT 3")

	queries, err := tc.repo.ListByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListByDatasource failed: %v", err)
	}

	if len(queries) != 3 {
		t.Fatalf("expected 3 queries, got %d", len(queries))
	}

	// Verify ordering (newest first)
	if queries[0].NaturalLanguagePrompt != "Query 3" {
		t.Errorf("expected first query to be 'Query 3', got %q", queries[0].NaturalLanguagePrompt)
	}
	if queries[2].NaturalLanguagePrompt != "Query 1" {
		t.Errorf("expected last query to be 'Query 1', got %q", queries[2].NaturalLanguagePrompt)
	}
}

func TestQueryRepository_Update(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := tc.createTestQuery(ctx, "Original prompt", "SELECT 1")
	originalUpdatedAt := query.UpdatedAt

	time.Sleep(10 * time.Millisecond) // Ensure different timestamp

	// Update the query
	query.NaturalLanguagePrompt = "Updated prompt"
	query.SQLQuery = "SELECT 2"
	context := "New context"
	query.AdditionalContext = &context

	err := tc.repo.Update(ctx, query)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify updated_at changed
	if !query.UpdatedAt.After(originalUpdatedAt) {
		t.Error("expected UpdatedAt to be updated")
	}

	// Verify changes persisted
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.NaturalLanguagePrompt != "Updated prompt" {
		t.Errorf("expected NaturalLanguagePrompt 'Updated prompt', got %q", retrieved.NaturalLanguagePrompt)
	}
	if retrieved.SQLQuery != "SELECT 2" {
		t.Errorf("expected SQLQuery 'SELECT 2', got %q", retrieved.SQLQuery)
	}
	if retrieved.AdditionalContext == nil || *retrieved.AdditionalContext != "New context" {
		t.Errorf("expected AdditionalContext 'New context', got %v", retrieved.AdditionalContext)
	}
}

func TestQueryRepository_Update_NotFound(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := &models.Query{
		ID:                    uuid.New(),
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Non-existent",
		SQLQuery:              "SELECT 1",
		Dialect:               "postgres",
		IsEnabled:             true,
	}

	err := tc.repo.Update(ctx, query)
	if err == nil {
		t.Fatal("expected error for non-existent query")
	}
	if err.Error() != "query not found" {
		t.Errorf("expected 'query not found' error, got %q", err.Error())
	}
}

func TestQueryRepository_SoftDelete(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := tc.createTestQuery(ctx, "To be deleted", "SELECT 1")

	err := tc.repo.SoftDelete(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// Verify query is no longer returned
	_, err = tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err == nil {
		t.Fatal("expected error for soft-deleted query")
	}
	if err.Error() != "query not found" {
		t.Errorf("expected 'query not found' error, got %q", err.Error())
	}

	// Verify not in list
	queries, err := tc.repo.ListByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListByDatasource failed: %v", err)
	}
	for _, q := range queries {
		if q.ID == query.ID {
			t.Error("soft-deleted query should not appear in list")
		}
	}
}

func TestQueryRepository_SoftDelete_NotFound(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.SoftDelete(ctx, tc.projectID, uuid.New())
	if err == nil {
		t.Fatal("expected error for non-existent query")
	}
	if err.Error() != "query not found" {
		t.Errorf("expected 'query not found' error, got %q", err.Error())
	}
}

// ============================================================================
// Filtering Tests
// ============================================================================

func TestQueryRepository_ListEnabled(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create enabled and disabled queries
	enabled1 := tc.createTestQuery(ctx, "Enabled 1", "SELECT 1")
	disabled := tc.createTestQuery(ctx, "Disabled", "SELECT 2")
	enabled2 := tc.createTestQuery(ctx, "Enabled 2", "SELECT 3")

	// Disable one query
	err := tc.repo.UpdateEnabledStatus(ctx, tc.projectID, disabled.ID, false)
	if err != nil {
		t.Fatalf("UpdateEnabledStatus failed: %v", err)
	}

	queries, err := tc.repo.ListEnabled(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListEnabled failed: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("expected 2 enabled queries, got %d", len(queries))
	}

	// Verify only enabled queries returned
	for _, q := range queries {
		if q.ID == disabled.ID {
			t.Error("disabled query should not be in enabled list")
		}
		if q.ID != enabled1.ID && q.ID != enabled2.ID {
			t.Errorf("unexpected query ID %s", q.ID)
		}
	}
}

// ============================================================================
// Status Tests
// ============================================================================

func TestQueryRepository_UpdateEnabledStatus(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := tc.createTestQuery(ctx, "Test", "SELECT 1")

	// Disable
	err := tc.repo.UpdateEnabledStatus(ctx, tc.projectID, query.ID, false)
	if err != nil {
		t.Fatalf("UpdateEnabledStatus failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.IsEnabled {
		t.Error("expected IsEnabled to be false")
	}

	// Re-enable
	err = tc.repo.UpdateEnabledStatus(ctx, tc.projectID, query.ID, true)
	if err != nil {
		t.Fatalf("UpdateEnabledStatus failed: %v", err)
	}

	retrieved, err = tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !retrieved.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}
}

func TestQueryRepository_UpdateEnabledStatus_NotFound(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.UpdateEnabledStatus(ctx, tc.projectID, uuid.New(), false)
	if err == nil {
		t.Fatal("expected error for non-existent query")
	}
	if err.Error() != "query not found" {
		t.Errorf("expected 'query not found' error, got %q", err.Error())
	}
}

// ============================================================================
// Usage Tracking Tests
// ============================================================================

func TestQueryRepository_IncrementUsageCount(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := tc.createTestQuery(ctx, "Test", "SELECT 1")

	// Verify initial state
	if query.UsageCount != 0 {
		t.Errorf("expected initial UsageCount 0, got %d", query.UsageCount)
	}
	if query.LastUsedAt != nil {
		t.Error("expected initial LastUsedAt to be nil")
	}

	// Increment
	err := tc.repo.IncrementUsageCount(ctx, query.ID)
	if err != nil {
		t.Fatalf("IncrementUsageCount failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.UsageCount != 1 {
		t.Errorf("expected UsageCount 1, got %d", retrieved.UsageCount)
	}
	if retrieved.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set")
	}

	// Increment again
	time.Sleep(10 * time.Millisecond)
	firstLastUsedAt := *retrieved.LastUsedAt

	err = tc.repo.IncrementUsageCount(ctx, query.ID)
	if err != nil {
		t.Fatalf("IncrementUsageCount failed: %v", err)
	}

	retrieved, err = tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.UsageCount != 2 {
		t.Errorf("expected UsageCount 2, got %d", retrieved.UsageCount)
	}
	if !retrieved.LastUsedAt.After(firstLastUsedAt) {
		t.Error("expected LastUsedAt to be updated")
	}
}

func TestQueryRepository_IncrementUsageCount_NotFound(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.IncrementUsageCount(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for non-existent query")
	}
	if err.Error() != "query not found" {
		t.Errorf("expected 'query not found' error, got %q", err.Error())
	}
}

// ============================================================================
// RLS Tests
// ============================================================================

func TestQueryRepository_NoTenantScope(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Test",
		SQLQuery:              "SELECT 1",
		Dialect:               "postgres",
		IsEnabled:             true,
	}

	err := tc.repo.Create(ctx, query)
	if err == nil {
		t.Fatal("expected error without tenant scope")
	}
	if err.Error() != "no tenant scope in context" {
		t.Errorf("expected 'no tenant scope in context' error, got %q", err.Error())
	}
}

func TestQueryRepository_ProjectIsolation(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a query in project A
	query := tc.createTestQuery(ctx, "Project A query", "SELECT 1")

	// Try to access from a different project ID
	otherProjectID := uuid.New()

	// GetByID with wrong project should fail
	_, err := tc.repo.GetByID(ctx, otherProjectID, query.ID)
	if err == nil {
		t.Fatal("expected error when accessing query from wrong project")
	}

	// ListByDatasource with wrong project should return empty
	queries, err := tc.repo.ListByDatasource(ctx, otherProjectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListByDatasource failed: %v", err)
	}
	if len(queries) != 0 {
		t.Errorf("expected 0 queries for wrong project, got %d", len(queries))
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestQueryRepository_DuplicatePromptsAllowed(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create two queries with the same prompt
	query1 := tc.createTestQuery(ctx, "Same prompt", "SELECT 1")
	query2 := tc.createTestQuery(ctx, "Same prompt", "SELECT 2")

	// Both should exist
	if query1.ID == query2.ID {
		t.Error("expected different IDs for duplicate prompts")
	}

	queries, err := tc.repo.ListByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListByDatasource failed: %v", err)
	}
	if len(queries) != 2 {
		t.Errorf("expected 2 queries with duplicate prompts, got %d", len(queries))
	}
}

func TestQueryRepository_SoftDeleteDoesNotAffectOthers(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple queries
	query1 := tc.createTestQuery(ctx, "Query 1", "SELECT 1")
	query2 := tc.createTestQuery(ctx, "Query 2", "SELECT 2")
	query3 := tc.createTestQuery(ctx, "Query 3", "SELECT 3")

	// Soft delete the middle one
	err := tc.repo.SoftDelete(ctx, tc.projectID, query2.ID)
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// Verify other queries still exist
	queries, err := tc.repo.ListByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListByDatasource failed: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("expected 2 queries after soft delete, got %d", len(queries))
	}

	foundQuery1, foundQuery3 := false, false
	for _, q := range queries {
		if q.ID == query1.ID {
			foundQuery1 = true
		}
		if q.ID == query3.ID {
			foundQuery3 = true
		}
		if q.ID == query2.ID {
			t.Error("soft-deleted query2 should not be in list")
		}
	}
	if !foundQuery1 {
		t.Error("query1 should still exist")
	}
	if !foundQuery3 {
		t.Error("query3 should still exist")
	}
}

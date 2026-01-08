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
		Parameters:            []models.QueryParameter{}, // Initialize empty parameters
		OutputColumns:         []models.OutputColumn{},   // Initialize empty output columns
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
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{},
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
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{},
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
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{},
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
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{},
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

// ============================================================================
// Parameters Tests
// ============================================================================

func TestQueryRepository_Create_WithParameters(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Get orders by customer",
		SQLQuery:              "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND status = {{status}}",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{
				Name:        "customer_id",
				Type:        "uuid",
				Description: "The customer's unique identifier",
				Required:    true,
				Default:     nil,
			},
			{
				Name:        "status",
				Type:        "string",
				Description: "Order status filter",
				Required:    false,
				Default:     "pending",
			},
		},
		OutputColumns: []models.OutputColumn{},
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify data was persisted with parameters
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if len(retrieved.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(retrieved.Parameters))
	}

	// Verify first parameter
	param1 := retrieved.Parameters[0]
	if param1.Name != "customer_id" {
		t.Errorf("expected parameter name 'customer_id', got %q", param1.Name)
	}
	if param1.Type != "uuid" {
		t.Errorf("expected parameter type 'uuid', got %q", param1.Type)
	}
	if param1.Description != "The customer's unique identifier" {
		t.Errorf("expected parameter description 'The customer's unique identifier', got %q", param1.Description)
	}
	if !param1.Required {
		t.Error("expected parameter to be required")
	}
	if param1.Default != nil {
		t.Errorf("expected parameter default to be nil, got %v", param1.Default)
	}

	// Verify second parameter
	param2 := retrieved.Parameters[1]
	if param2.Name != "status" {
		t.Errorf("expected parameter name 'status', got %q", param2.Name)
	}
	if param2.Type != "string" {
		t.Errorf("expected parameter type 'string', got %q", param2.Type)
	}
	if param2.Required {
		t.Error("expected parameter to not be required")
	}
	if param2.Default != "pending" {
		t.Errorf("expected parameter default 'pending', got %v", param2.Default)
	}
}

func TestQueryRepository_Create_WithoutParameters(t *testing.T) {
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
		Parameters:            []models.QueryParameter{}, // Empty parameters
		OutputColumns:         []models.OutputColumn{},
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify data was persisted without parameters
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if len(retrieved.Parameters) != 0 {
		t.Errorf("expected 0 parameters, got %d", len(retrieved.Parameters))
	}
}

func TestQueryRepository_Update_WithParameters(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create query without parameters
	query := tc.createTestQuery(ctx, "Original", "SELECT * FROM users")

	// Update to add parameters
	query.SQLQuery = "SELECT * FROM users WHERE email = {{email}}"
	query.Parameters = []models.QueryParameter{
		{
			Name:        "email",
			Type:        "string",
			Description: "User email address",
			Required:    true,
			Default:     nil,
		},
	}

	err := tc.repo.Update(ctx, query)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify parameters persisted
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if len(retrieved.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(retrieved.Parameters))
	}

	param := retrieved.Parameters[0]
	if param.Name != "email" {
		t.Errorf("expected parameter name 'email', got %q", param.Name)
	}
	if param.Type != "string" {
		t.Errorf("expected parameter type 'string', got %q", param.Type)
	}
}

func TestQueryRepository_Update_RemoveParameters(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create query with parameters
	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Get user by email",
		SQLQuery:              "SELECT * FROM users WHERE email = {{email}}",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{
				Name:        "email",
				Type:        "string",
				Description: "User email",
				Required:    true,
				Default:     nil,
			},
		},
		OutputColumns: []models.OutputColumn{},
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update to remove parameters
	query.SQLQuery = "SELECT * FROM users"
	query.Parameters = []models.QueryParameter{}

	err = tc.repo.Update(ctx, query)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify parameters removed
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if len(retrieved.Parameters) != 0 {
		t.Errorf("expected 0 parameters after removal, got %d", len(retrieved.Parameters))
	}
}

func TestQueryRepository_ListByDatasource_WithParameters(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create query with parameters
	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Get orders by date range",
		SQLQuery:              "SELECT * FROM orders WHERE created_at BETWEEN {{start_date}} AND {{end_date}}",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{
				Name:        "start_date",
				Type:        "date",
				Description: "Start date",
				Required:    true,
				Default:     nil,
			},
			{
				Name:        "end_date",
				Type:        "date",
				Description: "End date",
				Required:    true,
				Default:     nil,
			},
		},
		OutputColumns: []models.OutputColumn{},
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create another query without parameters
	tc.createTestQuery(ctx, "Simple query", "SELECT 1")

	// List all queries
	queries, err := tc.repo.ListByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListByDatasource failed: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(queries))
	}

	// Find the parameterized query
	var paramQuery *models.Query
	for _, q := range queries {
		if q.ID == query.ID {
			paramQuery = q
			break
		}
	}

	if paramQuery == nil {
		t.Fatal("parameterized query not found in list")
	}

	if len(paramQuery.Parameters) != 2 {
		t.Errorf("expected 2 parameters in listed query, got %d", len(paramQuery.Parameters))
	}
}

func TestQueryRepository_ListEnabled_WithParameters(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create enabled query with parameters
	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Get user by ID",
		SQLQuery:              "SELECT * FROM users WHERE id = {{user_id}}",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{
				Name:        "user_id",
				Type:        "integer",
				Description: "User ID",
				Required:    true,
				Default:     nil,
			},
		},
		OutputColumns: []models.OutputColumn{},
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List enabled queries
	queries, err := tc.repo.ListEnabled(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListEnabled failed: %v", err)
	}

	if len(queries) != 1 {
		t.Fatalf("expected 1 enabled query, got %d", len(queries))
	}

	if len(queries[0].Parameters) != 1 {
		t.Errorf("expected 1 parameter in enabled query, got %d", len(queries[0].Parameters))
	}
}

func TestQueryRepository_Parameters_WithVariousTypes(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Complex query with all types",
		SQLQuery:              "SELECT * FROM table WHERE col1 = {{string_param}} AND col2 = {{int_param}}",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{
				Name:        "string_param",
				Type:        "string",
				Description: "String parameter",
				Required:    true,
				Default:     "default_value",
			},
			{
				Name:        "int_param",
				Type:        "integer",
				Description: "Integer parameter",
				Required:    false,
				Default:     float64(42), // JSON numbers are float64
			},
			{
				Name:        "bool_param",
				Type:        "boolean",
				Description: "Boolean parameter",
				Required:    false,
				Default:     true,
			},
			{
				Name:        "array_param",
				Type:        "string[]",
				Description: "String array parameter",
				Required:    false,
				Default:     []interface{}{"a", "b", "c"},
			},
		},
		OutputColumns: []models.OutputColumn{},
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve and verify all parameter types
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if len(retrieved.Parameters) != 4 {
		t.Fatalf("expected 4 parameters, got %d", len(retrieved.Parameters))
	}

	// Verify each parameter type
	paramMap := make(map[string]models.QueryParameter)
	for _, p := range retrieved.Parameters {
		paramMap[p.Name] = p
	}

	if p := paramMap["string_param"]; p.Default != "default_value" {
		t.Errorf("expected string default 'default_value', got %v", p.Default)
	}

	if p := paramMap["int_param"]; p.Default != float64(42) {
		t.Errorf("expected int default 42, got %v", p.Default)
	}

	if p := paramMap["bool_param"]; p.Default != true {
		t.Errorf("expected bool default true, got %v", p.Default)
	}

	// Array comparison needs to be done carefully since it comes back as []interface{}
	if p := paramMap["array_param"]; p.Default != nil {
		arr, ok := p.Default.([]interface{})
		if !ok {
			t.Errorf("expected array default to be []interface{}, got %T", p.Default)
		} else if len(arr) != 3 {
			t.Errorf("expected array length 3, got %d", len(arr))
		}
	}
}

func TestQueryRepository_OutputColumnsAndConstraints(t *testing.T) {
	tc := setupQueryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	additionalContext := "Returns total revenue by customer"
	constraints := "Only includes completed orders. Excludes refunded amounts."

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Total revenue by customer",
		AdditionalContext:     &additionalContext,
		SQLQuery:              "SELECT u.name, SUM(o.total_amount) as revenue FROM users u JOIN orders o ON u.id = o.user_id WHERE o.status = 'completed' GROUP BY u.name",
		Dialect:               "postgres",
		IsEnabled:             true,
		UsageCount:            0,
		Parameters:            []models.QueryParameter{},
		OutputColumns: []models.OutputColumn{
			{
				Name:        "name",
				Type:        "string",
				Description: "Customer name",
			},
			{
				Name:        "revenue",
				Type:        "decimal",
				Description: "Total revenue from completed orders",
			},
		},
		Constraints: &constraints,
	}

	err := tc.repo.Create(ctx, query)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve and verify output_columns and constraints
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Verify output_columns
	if len(retrieved.OutputColumns) != 2 {
		t.Fatalf("expected 2 output columns, got %d", len(retrieved.OutputColumns))
	}

	if retrieved.OutputColumns[0].Name != "name" {
		t.Errorf("expected first column name 'name', got %s", retrieved.OutputColumns[0].Name)
	}
	if retrieved.OutputColumns[0].Type != "string" {
		t.Errorf("expected first column type 'string', got %s", retrieved.OutputColumns[0].Type)
	}
	if retrieved.OutputColumns[0].Description != "Customer name" {
		t.Errorf("expected first column description 'Customer name', got %s", retrieved.OutputColumns[0].Description)
	}

	if retrieved.OutputColumns[1].Name != "revenue" {
		t.Errorf("expected second column name 'revenue', got %s", retrieved.OutputColumns[1].Name)
	}
	if retrieved.OutputColumns[1].Type != "decimal" {
		t.Errorf("expected second column type 'decimal', got %s", retrieved.OutputColumns[1].Type)
	}

	// Verify constraints
	if retrieved.Constraints == nil {
		t.Fatal("expected constraints to be set, got nil")
	}
	if *retrieved.Constraints != constraints {
		t.Errorf("expected constraints '%s', got '%s'", constraints, *retrieved.Constraints)
	}

	// Test update with new constraints
	newConstraints := "Updated: Only includes orders from last 12 months"
	retrieved.Constraints = &newConstraints

	err = tc.repo.Update(ctx, retrieved)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update persisted
	updated, err := tc.repo.GetByID(ctx, tc.projectID, query.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}

	if updated.Constraints == nil {
		t.Fatal("expected updated constraints to be set, got nil")
	}
	if *updated.Constraints != newConstraints {
		t.Errorf("expected updated constraints '%s', got '%s'", newConstraints, *updated.Constraints)
	}
}

func TestQueryRepository_ListEnabledByTags(t *testing.T) {
	tc := setupQueryTest(t)
	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Clean up any existing queries before test
	tc.cleanup()

	// Create test queries with different tags
	query1 := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Billing revenue report",
		SQLQuery:              "SELECT * FROM billing_transactions WHERE total_amount > 0",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{{Name: "id", Type: "UUID", Description: "Transaction ID"}},
		Tags:                  []string{"billing", "category:finance", "reporting"},
		Status:                "approved",
		UsageCount:            0,
	}

	query2 := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "User engagement metrics",
		SQLQuery:              "SELECT user_id, COUNT(*) as session_count FROM sessions GROUP BY user_id",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{{Name: "user_id", Type: "UUID", Description: "User ID"}},
		Tags:                  []string{"engagement", "category:analytics", "reporting"},
		Status:                "approved",
		UsageCount:            0,
	}

	query3 := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Disabled query with billing tag",
		SQLQuery:              "SELECT * FROM billing_transactions WHERE deleted_at IS NOT NULL",
		Dialect:               "postgres",
		IsEnabled:             false, // Disabled - should not appear in results
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{{Name: "id", Type: "UUID", Description: "Transaction ID"}},
		Tags:                  []string{"billing", "admin"},
		Status:                "approved",
		UsageCount:            0,
	}

	query4 := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: "Query without tags",
		SQLQuery:              "SELECT NOW()",
		Dialect:               "postgres",
		IsEnabled:             true,
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{{Name: "now", Type: "TIMESTAMP", Description: "Current time"}},
		Tags:                  []string{}, // No tags
		Status:                "approved",
		UsageCount:            0,
	}

	// Create all queries
	if err := tc.repo.Create(ctx, query1); err != nil {
		t.Fatalf("Create query1 failed: %v", err)
	}
	if err := tc.repo.Create(ctx, query2); err != nil {
		t.Fatalf("Create query2 failed: %v", err)
	}
	if err := tc.repo.Create(ctx, query3); err != nil {
		t.Fatalf("Create query3 failed: %v", err)
	}
	if err := tc.repo.Create(ctx, query4); err != nil {
		t.Fatalf("Create query4 failed: %v", err)
	}

	// Test 1: Filter by single tag "billing" - should return query1 only (query3 is disabled)
	results, err := tc.repo.ListEnabledByTags(ctx, tc.projectID, tc.dsID, []string{"billing"})
	if err != nil {
		t.Fatalf("ListEnabledByTags failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 query with tag 'billing', got %d", len(results))
	}
	if len(results) > 0 && results[0].NaturalLanguagePrompt != "Billing revenue report" {
		t.Errorf("expected 'Billing revenue report', got '%s'", results[0].NaturalLanguagePrompt)
	}

	// Test 2: Filter by single tag "reporting" - should return query1 and query2 (both have reporting tag)
	results, err = tc.repo.ListEnabledByTags(ctx, tc.projectID, tc.dsID, []string{"reporting"})
	if err != nil {
		t.Fatalf("ListEnabledByTags failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 queries with tag 'reporting', got %d", len(results))
	}

	// Test 3: Filter by multiple tags (OR logic) - should return queries matching ANY tag
	results, err = tc.repo.ListEnabledByTags(ctx, tc.projectID, tc.dsID, []string{"billing", "engagement"})
	if err != nil {
		t.Fatalf("ListEnabledByTags failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 queries with tags 'billing' OR 'engagement', got %d", len(results))
	}

	// Test 4: Filter by tag that doesn't exist - should return empty list
	results, err = tc.repo.ListEnabledByTags(ctx, tc.projectID, tc.dsID, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("ListEnabledByTags failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 queries with tag 'nonexistent', got %d", len(results))
	}

	// Test 5: Empty tag list - should return empty list
	results, err = tc.repo.ListEnabledByTags(ctx, tc.projectID, tc.dsID, []string{})
	if err != nil {
		t.Fatalf("ListEnabledByTags failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 queries with empty tag list, got %d", len(results))
	}

	// Test 6: Verify tags are properly returned in results
	results, err = tc.repo.ListEnabledByTags(ctx, tc.projectID, tc.dsID, []string{"category:finance"})
	if err != nil {
		t.Fatalf("ListEnabledByTags failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 query, got %d", len(results))
	}
	if len(results[0].Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(results[0].Tags))
	}
	expectedTags := map[string]bool{"billing": true, "category:finance": true, "reporting": true}
	for _, tag := range results[0].Tags {
		if !expectedTags[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
	}
}

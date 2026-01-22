//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// glossaryTestContext holds test dependencies for glossary repository tests.
type glossaryTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      GlossaryRepository
	projectID uuid.UUID
}

// setupGlossaryTest initializes the test context with shared testcontainer.
func setupGlossaryTest(t *testing.T) *glossaryTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &glossaryTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      NewGlossaryRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000044"),
	}
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *glossaryTestContext) ensureTestProject() {
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
	`, tc.projectID, "Glossary Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

// cleanup removes test glossary terms and ontologies.
func (tc *glossaryTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete glossary terms first (depends on ontologies)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_business_glossary WHERE project_id = $1", tc.projectID)
	// Delete test ontologies
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *glossaryTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestTerm creates a glossary term for testing.
func (tc *glossaryTestContext) createTestTerm(ctx context.Context, termName, definition string) *models.BusinessGlossaryTerm {
	tc.t.Helper()
	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        termName,
		Definition:  definition,
		DefiningSQL: "SELECT 1", // Minimal valid SQL
		Source:      models.GlossarySourceManual,
	}
	err := tc.repo.Create(ctx, term)
	if err != nil {
		tc.t.Fatalf("failed to create test term: %v", err)
	}
	return term
}

// Fixed ontology IDs for testing unique constraint behavior
var (
	testOntologyID1 = uuid.MustParse("00000000-0000-0000-0000-000000000101")
	testOntologyID2 = uuid.MustParse("00000000-0000-0000-0000-000000000102")
)

// ensureTestOntology creates a test ontology if it doesn't exist and returns its ID.
func (tc *glossaryTestContext) ensureTestOntology(ctx context.Context) uuid.UUID {
	tc.t.Helper()
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		tc.t.Fatal("no tenant scope in context")
	}

	now := time.Now()
	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active, created_at, updated_at)
		VALUES ($1, $2, 1, true, $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, testOntologyID1, tc.projectID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}
	return testOntologyID1
}

// ensureTestOntology2 creates a second test ontology for testing different ontology scenarios.
// Uses version 2 to avoid unique constraint conflict with ensureTestOntology.
func (tc *glossaryTestContext) ensureTestOntology2(ctx context.Context) uuid.UUID {
	tc.t.Helper()
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		tc.t.Fatal("no tenant scope in context")
	}

	now := time.Now()
	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active, created_at, updated_at)
		VALUES ($1, $2, 2, false, $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, testOntologyID2, tc.projectID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology 2: %v", err)
	}
	return testOntologyID2
}

// ============================================================================
// Create Tests
// ============================================================================

func TestGlossaryRepository_Create_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "Revenue",
		Definition:  "Earned amount after fees from completed transactions",
		DefiningSQL: "SELECT SUM(earned_amount) AS revenue FROM billing_transactions WHERE transaction_state = 'completed'",
		BaseTable:   "billing_transactions",
		OutputColumns: []models.OutputColumn{
			{Name: "revenue", Type: "numeric", Description: "Total revenue"},
		},
		Aliases: []string{"Total Revenue", "Earned Revenue"},
		Source:  models.GlossarySourceManual,
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if term.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if term.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if term.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected term, got nil")
	}
	if retrieved.Term != "Revenue" {
		t.Errorf("expected term 'Revenue', got %q", retrieved.Term)
	}
	if retrieved.Definition != "Earned amount after fees from completed transactions" {
		t.Errorf("expected definition, got %q", retrieved.Definition)
	}
	if len(retrieved.OutputColumns) != 1 {
		t.Errorf("expected 1 output column, got %d", len(retrieved.OutputColumns))
	}
	if len(retrieved.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(retrieved.Aliases))
	}
	if retrieved.BaseTable != "billing_transactions" {
		t.Errorf("expected base_table 'billing_transactions', got %q", retrieved.BaseTable)
	}
}

func TestGlossaryRepository_Create_MinimalFields(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "Active User",
		Definition:  "User with recent activity in the last 30 days",
		DefiningSQL: "SELECT COUNT(DISTINCT user_id) FROM users WHERE last_active >= NOW() - INTERVAL '30 days'",
		Source:      models.GlossarySourceInferred,
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.BaseTable != "" {
		t.Errorf("expected empty base_table, got %q", retrieved.BaseTable)
	}
	if len(retrieved.OutputColumns) != 0 {
		t.Errorf("expected empty output_columns, got %v", retrieved.OutputColumns)
	}
	if len(retrieved.Aliases) != 0 {
		t.Errorf("expected empty aliases, got %v", retrieved.Aliases)
	}
}

func TestGlossaryRepository_Create_DuplicateTerm_SameOntology(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an ontology for testing the unique constraint
	ontologyID := tc.ensureTestOntology(ctx)

	// Create first term with ontology_id
	term1 := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		OntologyID:  &ontologyID,
		Term:        "GMV",
		Definition:  "Gross Merchandise Value",
		DefiningSQL: "SELECT 1",
		Source:      models.GlossarySourceManual,
	}
	err := tc.repo.Create(ctx, term1)
	if err != nil {
		t.Fatalf("failed to create first term: %v", err)
	}

	// Attempt to create duplicate (same project_id, ontology_id, term)
	term2 := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		OntologyID:  &ontologyID,
		Term:        "GMV",
		Definition:  "Different definition",
		DefiningSQL: "SELECT 1",
		Source:      models.GlossarySourceManual,
	}

	err = tc.repo.Create(ctx, term2)
	if err == nil {
		t.Error("expected error for duplicate term with same ontology_id")
	}
}

func TestGlossaryRepository_Create_DuplicateTerm_DifferentOntology(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create two ontologies for testing
	ontologyID1 := tc.ensureTestOntology(ctx)
	ontologyID2 := tc.ensureTestOntology2(ctx)

	// Create first term with ontology_id1
	term1 := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		OntologyID:  &ontologyID1,
		Term:        "Revenue",
		Definition:  "First ontology revenue",
		DefiningSQL: "SELECT 1",
		Source:      models.GlossarySourceInferred,
	}
	err := tc.repo.Create(ctx, term1)
	if err != nil {
		t.Fatalf("failed to create first term: %v", err)
	}

	// Create same term with different ontology_id - should succeed
	// This supports ontology refresh where new ontology can have same terms
	term2 := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		OntologyID:  &ontologyID2,
		Term:        "Revenue",
		Definition:  "Second ontology revenue",
		DefiningSQL: "SELECT 1",
		Source:      models.GlossarySourceInferred,
	}

	err = tc.repo.Create(ctx, term2)
	if err != nil {
		t.Errorf("expected success for duplicate term with different ontology_id, got: %v", err)
	}
}

// ============================================================================
// Update Tests
// ============================================================================

func TestGlossaryRepository_Update_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	original := tc.createTestTerm(ctx, "CAC", "Customer Acquisition Cost")
	originalCreatedAt := original.CreatedAt

	// Wait to ensure updated_at will be different (PostgreSQL NOW() has millisecond precision)
	time.Sleep(50 * time.Millisecond)

	// Update the term
	original.Definition = "Updated: Total cost to acquire a new customer"
	original.DefiningSQL = "SELECT SUM(marketing_spend) / COUNT(new_customers) AS cac FROM campaigns"
	original.BaseTable = "campaigns"
	original.Aliases = []string{"Customer Cost", "Acquisition Cost"}
	original.OutputColumns = []models.OutputColumn{
		{Name: "cac", Type: "numeric", Description: "Customer acquisition cost"},
	}

	err := tc.repo.Update(ctx, original)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// UpdatedAt should change
	if original.UpdatedAt.Equal(originalCreatedAt) || original.UpdatedAt.Before(originalCreatedAt) {
		t.Error("expected UpdatedAt to be after CreatedAt")
	}

	// Verify changes
	retrieved, err := tc.repo.GetByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Definition != "Updated: Total cost to acquire a new customer" {
		t.Errorf("expected updated definition, got %q", retrieved.Definition)
	}
	if retrieved.DefiningSQL != "SELECT SUM(marketing_spend) / COUNT(new_customers) AS cac FROM campaigns" {
		t.Errorf("expected updated defining_sql, got %q", retrieved.DefiningSQL)
	}
	if retrieved.BaseTable != "campaigns" {
		t.Errorf("expected base_table 'campaigns', got %q", retrieved.BaseTable)
	}
	if len(retrieved.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(retrieved.Aliases))
	}
}

func TestGlossaryRepository_Update_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ID:          uuid.New(),
		ProjectID:   tc.projectID,
		Term:        "NonExistent",
		Definition:  "Does not exist",
		DefiningSQL: "SELECT 1",
		Source:      models.GlossarySourceManual,
	}

	err := tc.repo.Update(ctx, term)
	if err != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestGlossaryRepository_Delete_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := tc.createTestTerm(ctx, "LTV", "Lifetime Value")

	err := tc.repo.Delete(ctx, term.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected term to be deleted")
	}
}

func TestGlossaryRepository_Delete_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.Delete(ctx, uuid.New())
	if err != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================================
// GetByProject Tests
// ============================================================================

func TestGlossaryRepository_GetByProject_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestTerm(ctx, "Revenue", "Earned amount after fees")
	tc.createTestTerm(ctx, "GMV", "Gross Merchandise Value")
	tc.createTestTerm(ctx, "Active User", "User with recent activity")

	terms, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(terms) != 3 {
		t.Errorf("expected 3 terms, got %d", len(terms))
	}
}

func TestGlossaryRepository_GetByProject_Empty(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	terms, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(terms) != 0 {
		t.Errorf("expected 0 terms, got %d", len(terms))
	}
}

func TestGlossaryRepository_GetByProject_OrderedByTerm(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create in non-alphabetical order
	tc.createTestTerm(ctx, "Zebra Metric", "Z metric")
	tc.createTestTerm(ctx, "Apple Revenue", "A metric")
	tc.createTestTerm(ctx, "Beta Conversion", "B metric")

	terms, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	// Should be ordered alphabetically by term
	if terms[0].Term != "Apple Revenue" {
		t.Errorf("expected first term 'Apple Revenue', got %q", terms[0].Term)
	}
	if terms[1].Term != "Beta Conversion" {
		t.Errorf("expected second term 'Beta Conversion', got %q", terms[1].Term)
	}
	if terms[2].Term != "Zebra Metric" {
		t.Errorf("expected third term 'Zebra Metric', got %q", terms[2].Term)
	}
}

// ============================================================================
// GetByTerm Tests
// ============================================================================

func TestGlossaryRepository_GetByTerm_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestTerm(ctx, "Churn Rate", "Percentage of customers who stop using the service")

	term, err := tc.repo.GetByTerm(ctx, tc.projectID, "Churn Rate")
	if err != nil {
		t.Fatalf("GetByTerm failed: %v", err)
	}
	if term == nil {
		t.Fatal("expected term, got nil")
	}
	if term.Term != "Churn Rate" {
		t.Errorf("expected term 'Churn Rate', got %q", term.Term)
	}
}

func TestGlossaryRepository_GetByTerm_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term, err := tc.repo.GetByTerm(ctx, tc.projectID, "NonExistent")
	if err != nil {
		t.Fatalf("GetByTerm should not error for not found: %v", err)
	}
	if term != nil {
		t.Errorf("expected nil for not found, got %+v", term)
	}
}

func TestGlossaryRepository_GetByTerm_CaseSensitive(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestTerm(ctx, "Revenue", "Earned amount")

	// Search with different case should not find it
	term, err := tc.repo.GetByTerm(ctx, tc.projectID, "revenue")
	if err != nil {
		t.Fatalf("GetByTerm failed: %v", err)
	}
	if term != nil {
		t.Error("expected case-sensitive search to not find 'revenue' when term is 'Revenue'")
	}
}

// ============================================================================
// GetByID Tests
// ============================================================================

func TestGlossaryRepository_GetByID_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestTerm(ctx, "ARPU", "Average Revenue Per User")

	term, err := tc.repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if term == nil {
		t.Fatal("expected term, got nil")
	}
	if term.Term != "ARPU" {
		t.Errorf("expected term 'ARPU', got %q", term.Term)
	}
}

func TestGlossaryRepository_GetByID_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term, err := tc.repo.GetByID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetByID should not error for not found: %v", err)
	}
	if term != nil {
		t.Errorf("expected nil for not found, got %+v", term)
	}
}

// ============================================================================
// Alias Tests
// ============================================================================

func TestGlossaryRepository_GetByAlias_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "Monthly Active Users",
		Definition:  "Users who logged in during the last 30 days",
		DefiningSQL: "SELECT COUNT(DISTINCT user_id) FROM users WHERE last_login >= NOW() - INTERVAL '30 days'",
		Aliases:     []string{"MAU", "Active Users"},
		Source:      models.GlossarySourceManual,
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Lookup by alias
	retrieved, err := tc.repo.GetByAlias(ctx, tc.projectID, "MAU")
	if err != nil {
		t.Fatalf("GetByAlias failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected term, got nil")
	}
	if retrieved.Term != "Monthly Active Users" {
		t.Errorf("expected term 'Monthly Active Users', got %q", retrieved.Term)
	}
	if len(retrieved.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(retrieved.Aliases))
	}
}

func TestGlossaryRepository_GetByAlias_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term, err := tc.repo.GetByAlias(ctx, tc.projectID, "NonExistentAlias")
	if err != nil {
		t.Fatalf("GetByAlias should not error for not found: %v", err)
	}
	if term != nil {
		t.Errorf("expected nil for not found alias, got %+v", term)
	}
}

func TestGlossaryRepository_CreateAlias_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := tc.createTestTerm(ctx, "Revenue", "Total revenue")

	// Add alias
	err := tc.repo.CreateAlias(ctx, term.ID, "Total Rev")
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	// Verify alias was added
	retrieved, err := tc.repo.GetByAlias(ctx, tc.projectID, "Total Rev")
	if err != nil {
		t.Fatalf("GetByAlias failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected term, got nil")
	}
	if retrieved.Term != "Revenue" {
		t.Errorf("expected term 'Revenue', got %q", retrieved.Term)
	}
}

func TestGlossaryRepository_DeleteAlias_Success(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "Churn Rate",
		Definition:  "Percentage of customers who cancel",
		DefiningSQL: "SELECT COUNT(*) FROM cancellations",
		Aliases:     []string{"Attrition", "Cancel Rate"},
		Source:      models.GlossarySourceManual,
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete one alias
	err = tc.repo.DeleteAlias(ctx, term.ID, "Attrition")
	if err != nil {
		t.Fatalf("DeleteAlias failed: %v", err)
	}

	// Verify alias was deleted
	retrieved, err := tc.repo.GetByAlias(ctx, tc.projectID, "Attrition")
	if err != nil {
		t.Fatalf("GetByAlias failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected alias to be deleted")
	}

	// Other alias should still exist
	retrieved, err = tc.repo.GetByAlias(ctx, tc.projectID, "Cancel Rate")
	if err != nil {
		t.Fatalf("GetByAlias failed: %v", err)
	}
	if retrieved == nil {
		t.Error("expected other alias to still exist")
	}
}

func TestGlossaryRepository_DeleteAlias_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := tc.createTestTerm(ctx, "GMV", "Gross Merchandise Value")

	err := tc.repo.DeleteAlias(ctx, term.ID, "NonExistentAlias")
	if err != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================================
// Output Columns Tests
// ============================================================================

func TestGlossaryRepository_OutputColumns(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "User Stats",
		Definition:  "User statistics with multiple metrics",
		DefiningSQL: "SELECT COUNT(*) as total, AVG(age) as avg_age FROM users",
		OutputColumns: []models.OutputColumn{
			{Name: "total", Type: "integer", Description: "Total users"},
			{Name: "avg_age", Type: "numeric", Description: "Average age"},
		},
		Source: models.GlossarySourceManual,
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(retrieved.OutputColumns) != 2 {
		t.Errorf("expected 2 output columns, got %d", len(retrieved.OutputColumns))
	}
	if retrieved.OutputColumns[0].Name != "total" {
		t.Errorf("expected first column name 'total', got %q", retrieved.OutputColumns[0].Name)
	}
	if retrieved.OutputColumns[0].Type != "integer" {
		t.Errorf("expected first column type 'integer', got %q", retrieved.OutputColumns[0].Type)
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestGlossaryRepository_NoTenantScope(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "Test",
		Definition:  "Test definition",
		DefiningSQL: "SELECT 1",
		Source:      models.GlossarySourceManual,
	}

	// Create should fail
	err := tc.repo.Create(ctx, term)
	if err == nil {
		t.Error("expected error for Create without tenant scope")
	}

	// GetByProject should fail
	_, err = tc.repo.GetByProject(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetByProject without tenant scope")
	}

	// GetByTerm should fail
	_, err = tc.repo.GetByTerm(ctx, tc.projectID, "Test")
	if err == nil {
		t.Error("expected error for GetByTerm without tenant scope")
	}

	// GetByAlias should fail
	_, err = tc.repo.GetByAlias(ctx, tc.projectID, "TestAlias")
	if err == nil {
		t.Error("expected error for GetByAlias without tenant scope")
	}

	// GetByID should fail
	_, err = tc.repo.GetByID(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for GetByID without tenant scope")
	}

	// Update should fail
	err = tc.repo.Update(ctx, term)
	if err == nil {
		t.Error("expected error for Update without tenant scope")
	}

	// Delete should fail
	err = tc.repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for Delete without tenant scope")
	}

	// CreateAlias should fail
	err = tc.repo.CreateAlias(ctx, uuid.New(), "Alias")
	if err == nil {
		t.Error("expected error for CreateAlias without tenant scope")
	}

	// DeleteAlias should fail
	err = tc.repo.DeleteAlias(ctx, uuid.New(), "Alias")
	if err == nil {
		t.Error("expected error for DeleteAlias without tenant scope")
	}
}

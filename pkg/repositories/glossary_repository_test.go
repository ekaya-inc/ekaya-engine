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

// cleanup removes test glossary terms.
func (tc *glossaryTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_business_glossary WHERE project_id = $1", tc.projectID)
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
		ProjectID:  tc.projectID,
		Term:       termName,
		Definition: definition,
		Source:     "user",
	}
	err := tc.repo.Create(ctx, term)
	if err != nil {
		tc.t.Fatalf("failed to create test term: %v", err)
	}
	return term
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
		SQLPattern:  "SUM(earned_amount) WHERE transaction_state = 'completed'",
		BaseTable:   "billing_transactions",
		ColumnsUsed: []string{"earned_amount", "transaction_state"},
		Filters: []models.Filter{
			{Column: "transaction_state", Operator: "=", Values: []string{"completed"}},
		},
		Aggregation: "SUM",
		Source:      "user",
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
	if len(retrieved.ColumnsUsed) != 2 {
		t.Errorf("expected 2 columns_used, got %d", len(retrieved.ColumnsUsed))
	}
	if len(retrieved.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(retrieved.Filters))
	}
	if retrieved.Filters[0].Column != "transaction_state" {
		t.Errorf("expected filter column 'transaction_state', got %q", retrieved.Filters[0].Column)
	}
}

func TestGlossaryRepository_Create_MinimalFields(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:  tc.projectID,
		Term:       "Active User",
		Definition: "User with recent activity in the last 30 days",
		Source:     "suggested",
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.SQLPattern != "" {
		t.Errorf("expected empty sql_pattern, got %q", retrieved.SQLPattern)
	}
	if len(retrieved.ColumnsUsed) != 0 {
		t.Errorf("expected empty columns_used, got %v", retrieved.ColumnsUsed)
	}
	if len(retrieved.Filters) != 0 {
		t.Errorf("expected empty filters, got %v", retrieved.Filters)
	}
}

func TestGlossaryRepository_Create_DuplicateTerm(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestTerm(ctx, "GMV", "Gross Merchandise Value")

	// Attempt to create duplicate
	term := &models.BusinessGlossaryTerm{
		ProjectID:  tc.projectID,
		Term:       "GMV",
		Definition: "Different definition",
		Source:     "user",
	}

	err := tc.repo.Create(ctx, term)
	if err == nil {
		t.Error("expected error for duplicate term")
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
	time.Sleep(10 * time.Millisecond)

	// Update the term
	original.Definition = "Updated: Total cost to acquire a new customer"
	original.SQLPattern = "SUM(marketing_spend) / COUNT(new_customers)"
	original.ColumnsUsed = []string{"marketing_spend"}
	original.Aggregation = "RATIO"

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
	if retrieved.SQLPattern != "SUM(marketing_spend) / COUNT(new_customers)" {
		t.Errorf("expected updated sql_pattern, got %q", retrieved.SQLPattern)
	}
	if retrieved.Aggregation != "RATIO" {
		t.Errorf("expected aggregation 'RATIO', got %q", retrieved.Aggregation)
	}
}

func TestGlossaryRepository_Update_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		Term:       "NonExistent",
		Definition: "Does not exist",
		Source:     "user",
	}

	err := tc.repo.Update(ctx, term)
	if err == nil {
		t.Error("expected error for non-existent term")
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
	if err == nil {
		t.Error("expected error for non-existent term")
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
// JSONB Field Tests
// ============================================================================

func TestGlossaryRepository_JSONBFields_ColumnsUsed(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		Term:        "Profit Margin",
		Definition:  "Revenue minus costs divided by revenue",
		ColumnsUsed: []string{"revenue", "costs", "total_amount"},
		Source:      "user",
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(retrieved.ColumnsUsed) != 3 {
		t.Errorf("expected 3 columns, got %d", len(retrieved.ColumnsUsed))
	}
	if retrieved.ColumnsUsed[0] != "revenue" {
		t.Errorf("expected first column 'revenue', got %q", retrieved.ColumnsUsed[0])
	}
}

func TestGlossaryRepository_JSONBFields_Filters(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:  tc.projectID,
		Term:       "Completed GMV",
		Definition: "GMV for completed transactions",
		Filters: []models.Filter{
			{Column: "transaction_state", Operator: "=", Values: []string{"completed"}},
			{Column: "is_refunded", Operator: "=", Values: []string{"false"}},
		},
		Source: "user",
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(retrieved.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(retrieved.Filters))
	}
	if retrieved.Filters[0].Operator != "=" {
		t.Errorf("expected operator '=', got %q", retrieved.Filters[0].Operator)
	}
	if len(retrieved.Filters[0].Values) != 1 || retrieved.Filters[0].Values[0] != "completed" {
		t.Errorf("expected values ['completed'], got %v", retrieved.Filters[0].Values)
	}
}

func TestGlossaryRepository_JSONBFields_FilterWithMultipleValues(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	term := &models.BusinessGlossaryTerm{
		ProjectID:  tc.projectID,
		Term:       "Pending or Processing GMV",
		Definition: "GMV for transactions in progress",
		Filters: []models.Filter{
			{Column: "transaction_state", Operator: "IN", Values: []string{"pending", "processing", "confirming"}},
		},
		Source: "user",
	}

	err := tc.repo.Create(ctx, term)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, term.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(retrieved.Filters[0].Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(retrieved.Filters[0].Values))
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
		ProjectID:  tc.projectID,
		Term:       "Test",
		Definition: "Test definition",
		Source:     "user",
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
}

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

// knowledgeTestContext holds test dependencies for knowledge repository tests.
type knowledgeTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       KnowledgeRepository
	projectID  uuid.UUID
	testUserID uuid.UUID // User ID for provenance context
}

// setupKnowledgeTest initializes the test context with shared testcontainer.
func setupKnowledgeTest(t *testing.T) *knowledgeTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &knowledgeTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewKnowledgeRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000043"),
		testUserID: uuid.MustParse("00000000-0000-0000-0000-000000000046"), // Test user for provenance
	}
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *knowledgeTestContext) ensureTestProject() {
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
	`, tc.projectID, "Knowledge Test Project")
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

// cleanup removes test knowledge facts.
func (tc *knowledgeTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_project_knowledge WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and manual provenance.
func (tc *knowledgeTestContext) createTestContext() (context.Context, func()) {
	return tc.createTestContextWithSource(models.SourceManual)
}

// createTestContextWithSource returns a context with tenant scope and specified provenance source.
func (tc *knowledgeTestContext) createTestContextWithSource(source models.ProvenanceSource) (context.Context, func()) {
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

// createTestFact creates a knowledge fact for testing.
func (tc *knowledgeTestContext) createTestFact(ctx context.Context, factType, value string) *models.KnowledgeFact {
	tc.t.Helper()
	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  factType,
		Value:     value,
		Context:   "Test context",
	}
	err := tc.repo.Create(ctx, fact)
	if err != nil {
		tc.t.Fatalf("failed to create test fact: %v", err)
	}
	return fact
}

// ============================================================================
// Create Tests
// ============================================================================

func TestKnowledgeRepository_Create_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeFiscalYear,
		Value:     "July",
		Context:   "Company follows July-June fiscal year",
	}

	err := tc.repo.Create(ctx, fact)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if fact.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if fact.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching all facts
	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Value != "July" {
		t.Errorf("expected value 'July', got %q", facts[0].Value)
	}
	if facts[0].Context != "Company follows July-June fiscal year" {
		t.Errorf("expected context, got %q", facts[0].Context)
	}
}

func TestKnowledgeRepository_Create_WithoutContext(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Value:     "A person or organization that purchases goods",
		// No context
	}

	err := tc.repo.Create(ctx, fact)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	facts, err := tc.repo.GetByType(ctx, tc.projectID, models.FactTypeTerminology)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Context != "" {
		t.Errorf("expected empty context, got %q", facts[0].Context)
	}
}

// ============================================================================
// Update Tests
// ============================================================================

func TestKnowledgeRepository_Update_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create initial fact
	original := tc.createTestFact(ctx, models.FactTypeBusinessRule, "100")
	originalID := original.ID
	originalCreatedAt := original.CreatedAt

	// Update the fact
	original.Value = "150"
	original.Context = "Updated policy"

	err := tc.repo.Update(ctx, original)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// ID should be unchanged
	if original.ID != originalID {
		t.Errorf("expected same ID %v, got %v", originalID, original.ID)
	}

	// CreatedAt should be preserved
	if !original.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("expected CreatedAt to be preserved")
	}

	// Verify value was updated
	facts, err := tc.repo.GetByType(ctx, tc.projectID, models.FactTypeBusinessRule)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Value != "150" {
		t.Errorf("expected updated value '150', got %q", facts[0].Value)
	}
	if facts[0].Context != "Updated policy" {
		t.Errorf("expected updated context, got %q", facts[0].Context)
	}
}

func TestKnowledgeRepository_Update_NotFound(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Try to update a non-existent ID
	nonExistentID := uuid.New()
	fact := &models.KnowledgeFact{
		ID:        nonExistentID,
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Value:     "Some value",
	}

	err := tc.repo.Update(ctx, fact)
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
}

// ============================================================================
// GetByProject Tests
// ============================================================================

func TestKnowledgeRepository_GetByProject_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestFact(ctx, models.FactTypeFiscalYear, "January")
	tc.createTestFact(ctx, models.FactTypeBusinessRule, "30%")
	tc.createTestFact(ctx, models.FactTypeTerminology, "Stock Keeping Unit")

	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 3 {
		t.Errorf("expected 3 facts, got %d", len(facts))
	}
}

func TestKnowledgeRepository_GetByProject_Empty(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts, got %d", len(facts))
	}
}

func TestKnowledgeRepository_GetByProject_Ordering(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create in non-alphabetical order
	tc.createTestFact(ctx, models.FactTypeTerminology, "Z animal")
	tc.createTestFact(ctx, models.FactTypeBusinessRule, "A rule")
	tc.createTestFact(ctx, models.FactTypeTerminology, "A fruit")

	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	// Should be ordered by fact_type, then value
	// business_rule < terminology (alphabetically)
	if facts[0].FactType != models.FactTypeBusinessRule {
		t.Errorf("expected first fact to be business_rule, got %q", facts[0].FactType)
	}
}

// ============================================================================
// GetByType Tests
// ============================================================================

func TestKnowledgeRepository_GetByType_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestFact(ctx, models.FactTypeTerminology, "Application Programming Interface")
	tc.createTestFact(ctx, models.FactTypeTerminology, "Software Development Kit")
	tc.createTestFact(ctx, models.FactTypeBusinessRule, "100")

	facts, err := tc.repo.GetByType(ctx, tc.projectID, models.FactTypeTerminology)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(facts) != 2 {
		t.Errorf("expected 2 terminology facts, got %d", len(facts))
	}
	for _, f := range facts {
		if f.FactType != models.FactTypeTerminology {
			t.Errorf("expected only terminology facts, got %q", f.FactType)
		}
	}
}

func TestKnowledgeRepository_GetByType_Empty(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestFact(ctx, models.FactTypeBusinessRule, "value")

	facts, err := tc.repo.GetByType(ctx, tc.projectID, models.FactTypeTerminology)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 terminology facts, got %d", len(facts))
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestKnowledgeRepository_Delete_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	fact := tc.createTestFact(ctx, models.FactTypeBusinessRule, "To be deleted")
	factID := fact.ID

	err := tc.repo.Delete(ctx, factID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts after delete, got %d", len(facts))
	}
}

func TestKnowledgeRepository_Delete_NotFound(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Try to delete non-existent fact - returns not found error
	err := tc.repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Error("Delete should error for non-existent ID")
	}
}

// ============================================================================
// DeleteByProject Tests
// ============================================================================

func TestKnowledgeRepository_DeleteByProject_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestFact(ctx, models.FactTypeBusinessRule, "Rule 1")
	tc.createTestFact(ctx, models.FactTypeTerminology, "Term 1")

	err := tc.repo.DeleteByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("DeleteByProject failed: %v", err)
	}

	// Verify all gone
	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts after delete, got %d", len(facts))
	}
}

// ============================================================================
// DeleteBySource Tests
// ============================================================================

func TestKnowledgeRepository_DeleteBySource_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	// Create facts with manual source
	manualCtx, manualCleanup := tc.createTestContextWithSource(models.SourceManual)
	defer manualCleanup()
	tc.createTestFact(manualCtx, models.FactTypeBusinessRule, "Manual rule")

	// Create facts with inferred source
	inferredCtx, inferredCleanup := tc.createTestContextWithSource(models.SourceInferred)
	defer inferredCleanup()
	tc.createTestFact(inferredCtx, models.FactTypeBusinessRule, "Inferred rule")

	// Delete only inferred facts
	err := tc.repo.DeleteBySource(inferredCtx, tc.projectID, models.SourceInferred)
	if err != nil {
		t.Fatalf("DeleteBySource failed: %v", err)
	}

	// Verify only manual fact remains
	facts, err := tc.repo.GetByProject(manualCtx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("expected 1 fact remaining, got %d", len(facts))
	}
	if len(facts) > 0 && facts[0].Source != string(models.SourceManual) {
		t.Errorf("expected manual source, got %q", facts[0].Source)
	}
}

// ============================================================================
// Provenance Tests
// ============================================================================

func TestKnowledgeRepository_Create_WithProvenance(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContextWithSource(models.SourceMCP)
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Value:     "A fact from MCP",
	}

	err := tc.repo.Create(ctx, fact)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify source was set from provenance
	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Source != string(models.SourceMCP) {
		t.Errorf("expected source 'mcp', got %q", facts[0].Source)
	}
}

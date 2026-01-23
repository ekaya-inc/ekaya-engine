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
	ontologyID uuid.UUID
}

// setupKnowledgeTest initializes the test context with shared testcontainer.
func setupKnowledgeTest(t *testing.T) *knowledgeTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &knowledgeTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewKnowledgeRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000043"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000044"),
	}
	tc.ensureTestProject()
	tc.ensureTestOntology()
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
}

// ensureTestOntology creates the test ontology if it doesn't exist.
func (tc *knowledgeTestContext) ensureTestOntology() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create scope for ontology setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
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

// createTestContext returns a context with tenant scope.
func (tc *knowledgeTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestFact creates a knowledge fact for testing.
func (tc *knowledgeTestContext) createTestFact(ctx context.Context, factType, key, value string) *models.KnowledgeFact {
	tc.t.Helper()
	fact := &models.KnowledgeFact{
		ProjectID:  tc.projectID,
		OntologyID: &tc.ontologyID,
		FactType:   factType,
		Key:        key,
		Value:      value,
		Context:    "Test context",
	}
	err := tc.repo.Upsert(ctx, fact)
	if err != nil {
		tc.t.Fatalf("failed to create test fact: %v", err)
	}
	return fact
}

// ============================================================================
// Upsert Tests (Insert)
// ============================================================================

func TestKnowledgeRepository_Upsert_Insert(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeFiscalYear,
		Key:       "start_month",
		Value:     "July",
		Context:   "Company follows July-June fiscal year",
	}

	err := tc.repo.Upsert(ctx, fact)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if fact.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if fact.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeFiscalYear, "start_month")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected fact, got nil")
	}
	if retrieved.Value != "July" {
		t.Errorf("expected value 'July', got %q", retrieved.Value)
	}
	if retrieved.Context != "Company follows July-June fiscal year" {
		t.Errorf("expected context, got %q", retrieved.Context)
	}
}

func TestKnowledgeRepository_Upsert_InsertWithoutContext(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "customer",
		Value:     "A person or organization that purchases goods",
		// No context
	}

	err := tc.repo.Upsert(ctx, fact)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	retrieved, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "customer")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved.Context != "" {
		t.Errorf("expected empty context, got %q", retrieved.Context)
	}
}

// ============================================================================
// Upsert Tests (Update)
// ============================================================================

func TestKnowledgeRepository_Upsert_Update(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create initial fact
	original := tc.createTestFact(ctx, models.FactTypeBusinessRule, "discount_threshold", "100")
	originalID := original.ID

	// Upsert with same key should update
	updated := &models.KnowledgeFact{
		ProjectID:  tc.projectID,
		OntologyID: &tc.ontologyID,
		FactType:   models.FactTypeBusinessRule,
		Key:        "discount_threshold",
		Value:      "150", // Changed value
		Context:    "Updated policy",
	}

	err := tc.repo.Upsert(ctx, updated)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// ID should be preserved (same record)
	if updated.ID != originalID {
		t.Errorf("expected same ID %v, got %v", originalID, updated.ID)
	}

	// CreatedAt should be preserved
	if updated.CreatedAt != original.CreatedAt {
		t.Errorf("expected same CreatedAt, but it changed")
	}

	// Verify value was updated
	retrieved, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeBusinessRule, "discount_threshold")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved.Value != "150" {
		t.Errorf("expected updated value '150', got %q", retrieved.Value)
	}
	if retrieved.Context != "Updated policy" {
		t.Errorf("expected updated context, got %q", retrieved.Context)
	}
}

func TestKnowledgeRepository_Upsert_SameTypesDifferentKeys(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create facts with same type but different keys
	tc.createTestFact(ctx, models.FactTypeEnumeration, "status_active", "Account is active and in good standing")
	tc.createTestFact(ctx, models.FactTypeEnumeration, "status_suspended", "Account is temporarily suspended")
	tc.createTestFact(ctx, models.FactTypeEnumeration, "status_closed", "Account is permanently closed")

	facts, err := tc.repo.GetByType(ctx, tc.projectID, models.FactTypeEnumeration)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(facts) != 3 {
		t.Errorf("expected 3 facts, got %d", len(facts))
	}
}

func TestKnowledgeRepository_Upsert_UpdateByID(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create initial fact
	original := tc.createTestFact(ctx, models.FactTypeTerminology, "original_key", "Original value")
	originalID := original.ID
	originalCreatedAt := original.CreatedAt

	// Update by ID with a NEW key (this was the bug - it would fail with duplicate key error)
	updated := &models.KnowledgeFact{
		ID:        originalID, // Explicitly set ID for update-by-ID
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "updated_key", // Different key
		Value:     "Updated value",
		Context:   "New context",
	}

	err := tc.repo.Upsert(ctx, updated)
	if err != nil {
		t.Fatalf("Upsert with ID failed: %v", err)
	}

	// ID should be unchanged
	if updated.ID != originalID {
		t.Errorf("expected same ID %v, got %v", originalID, updated.ID)
	}

	// CreatedAt should be preserved
	if !updated.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("expected CreatedAt to be preserved, was %v now %v", originalCreatedAt, updated.CreatedAt)
	}

	// Verify old key no longer exists
	oldFact, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "original_key")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if oldFact != nil {
		t.Error("expected old key to not exist after update")
	}

	// Verify new key exists with correct values
	newFact, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "updated_key")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if newFact == nil {
		t.Fatal("expected new key to exist")
	}
	if newFact.Value != "Updated value" {
		t.Errorf("expected value 'Updated value', got %q", newFact.Value)
	}
	if newFact.Context != "New context" {
		t.Errorf("expected context 'New context', got %q", newFact.Context)
	}
}

func TestKnowledgeRepository_Upsert_UpdateByID_NotFound(t *testing.T) {
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
		Key:       "some_key",
		Value:     "Some value",
	}

	err := tc.repo.Upsert(ctx, fact)
	if err == nil {
		t.Error("expected error for non-existent ID")
	}
	if err != nil && err.Error() != "fact with id "+nonExistentID.String()+" not found" {
		t.Errorf("unexpected error message: %v", err)
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

	tc.createTestFact(ctx, models.FactTypeFiscalYear, "start_month", "January")
	tc.createTestFact(ctx, models.FactTypeBusinessRule, "max_discount", "30%")
	tc.createTestFact(ctx, models.FactTypeTerminology, "SKU", "Stock Keeping Unit")

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
	tc.createTestFact(ctx, models.FactTypeTerminology, "zebra", "Z animal")
	tc.createTestFact(ctx, models.FactTypeBusinessRule, "alpha", "A rule")
	tc.createTestFact(ctx, models.FactTypeTerminology, "apple", "A fruit")

	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	// Should be ordered by fact_type, then key
	// business_rule < terminology (alphabetically)
	if facts[0].FactType != models.FactTypeBusinessRule {
		t.Errorf("expected first fact to be business_rule, got %q", facts[0].FactType)
	}
	// Then terminology facts ordered by key
	if len(facts) == 3 {
		if facts[1].Key != "apple" {
			t.Errorf("expected second fact key 'apple', got %q", facts[1].Key)
		}
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

	tc.createTestFact(ctx, models.FactTypeTerminology, "API", "Application Programming Interface")
	tc.createTestFact(ctx, models.FactTypeTerminology, "SDK", "Software Development Kit")
	tc.createTestFact(ctx, models.FactTypeBusinessRule, "max_users", "100")

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

	tc.createTestFact(ctx, models.FactTypeBusinessRule, "rule1", "value")

	facts, err := tc.repo.GetByType(ctx, tc.projectID, models.FactTypeTerminology)
	if err != nil {
		t.Fatalf("GetByType failed: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 terminology facts, got %d", len(facts))
	}
}

// ============================================================================
// GetByKey Tests
// ============================================================================

func TestKnowledgeRepository_GetByKey_Success(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestFact(ctx, models.FactTypeFiscalYear, "end_month", "June")

	fact, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeFiscalYear, "end_month")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if fact == nil {
		t.Fatal("expected fact, got nil")
	}
	if fact.Value != "June" {
		t.Errorf("expected value 'June', got %q", fact.Value)
	}
}

func TestKnowledgeRepository_GetByKey_NotFound(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	fact, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeFiscalYear, "nonexistent")
	if err != nil {
		t.Fatalf("GetByKey should not error for not found: %v", err)
	}
	if fact != nil {
		t.Errorf("expected nil for not found, got %+v", fact)
	}
}

func TestKnowledgeRepository_GetByKey_WrongType(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestFact(ctx, models.FactTypeBusinessRule, "my_key", "value")

	// Same key but different type should not be found
	fact, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "my_key")
	if err != nil {
		t.Fatalf("GetByKey should not error: %v", err)
	}
	if fact != nil {
		t.Errorf("expected nil for wrong type, got %+v", fact)
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

	fact := tc.createTestFact(ctx, models.FactTypeConvention, "date_format", "YYYY-MM-DD")

	err := tc.repo.Delete(ctx, fact.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	retrieved, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeConvention, "date_format")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected fact to be deleted")
	}
}

func TestKnowledgeRepository_Delete_NotFound(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for non-existent fact")
	}
}

// ============================================================================
// All Fact Types Tests
// ============================================================================

func TestKnowledgeRepository_AllFactTypes(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test all supported fact types
	factTypes := []struct {
		factType string
		key      string
		value    string
	}{
		{models.FactTypeFiscalYear, "start", "January"},
		{models.FactTypeBusinessRule, "max_discount", "25%"},
		{models.FactTypeTerminology, "MRR", "Monthly Recurring Revenue"},
		{models.FactTypeConvention, "currency", "USD"},
		{models.FactTypeEnumeration, "order_status_pending", "Order has been placed but not processed"},
		{models.FactTypeRelationship, "user_orders", "Users can have multiple orders"},
	}

	for _, ft := range factTypes {
		tc.createTestFact(ctx, ft.factType, ft.key, ft.value)
	}

	facts, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(facts) != len(factTypes) {
		t.Errorf("expected %d facts, got %d", len(factTypes), len(facts))
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestKnowledgeRepository_NoTenantScope(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "test",
		Value:     "value",
	}

	// Upsert should fail
	err := tc.repo.Upsert(ctx, fact)
	if err == nil {
		t.Error("expected error for Upsert without tenant scope")
	}

	// GetByProject should fail
	_, err = tc.repo.GetByProject(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetByProject without tenant scope")
	}

	// GetByType should fail
	_, err = tc.repo.GetByType(ctx, tc.projectID, models.FactTypeTerminology)
	if err == nil {
		t.Error("expected error for GetByType without tenant scope")
	}

	// GetByKey should fail
	_, err = tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "test")
	if err == nil {
		t.Error("expected error for GetByKey without tenant scope")
	}

	// Delete should fail
	err = tc.repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for Delete without tenant scope")
	}
}

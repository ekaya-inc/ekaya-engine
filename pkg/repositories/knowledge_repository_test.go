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
func (tc *knowledgeTestContext) createTestFact(ctx context.Context, factType, key, value string) *models.KnowledgeFact {
	tc.t.Helper()
	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   "Test context",
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
		ProjectID: tc.projectID,
		FactType:  models.FactTypeBusinessRule,
		Key:       "discount_threshold",
		Value:     "150", // Changed value
		Context:   "Updated policy",
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

// ============================================================================
// Provenance Tests
// ============================================================================

func TestKnowledgeRepository_Upsert_Provenance_Create(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	// Test with manual provenance
	ctx, cleanup := tc.createTestContextWithSource(models.SourceManual)
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "provenance_test",
		Value:     "Testing provenance on create",
	}

	err := tc.repo.Upsert(ctx, fact)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify Source was set from context
	if fact.Source != "manual" {
		t.Errorf("expected Source 'manual', got %q", fact.Source)
	}
	// Verify CreatedBy was set from context
	if fact.CreatedBy == nil {
		t.Error("expected CreatedBy to be set")
	}
	if fact.CreatedBy != nil && *fact.CreatedBy != tc.testUserID {
		t.Errorf("expected CreatedBy to be %v, got %v", tc.testUserID, *fact.CreatedBy)
	}

	// Verify persisted correctly
	retrieved, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "provenance_test")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved.Source != "manual" {
		t.Errorf("expected persisted Source 'manual', got %q", retrieved.Source)
	}
	if retrieved.CreatedBy == nil || *retrieved.CreatedBy != tc.testUserID {
		t.Errorf("expected persisted CreatedBy %v, got %v", tc.testUserID, retrieved.CreatedBy)
	}
}

func TestKnowledgeRepository_Upsert_Provenance_Inference(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	// Test with inference provenance
	ctx, cleanup := tc.createTestContextWithSource(models.SourceInferred)
	defer cleanup()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "inferred_fact",
		Value:     "Created by inference",
	}

	err := tc.repo.Upsert(ctx, fact)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify Source was set from context
	if fact.Source != "inferred" {
		t.Errorf("expected Source 'inference', got %q", fact.Source)
	}

	// Verify persisted correctly
	retrieved, err := tc.repo.GetByKey(ctx, tc.projectID, models.FactTypeTerminology, "inferred_fact")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved.Source != "inferred" {
		t.Errorf("expected persisted Source 'inference', got %q", retrieved.Source)
	}
}

func TestKnowledgeRepository_Upsert_NoProvenance(t *testing.T) {
	tc := setupKnowledgeTest(t)
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

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "no_provenance",
		Value:     "Created without provenance",
	}

	err = tc.repo.Upsert(ctx, fact)
	if err == nil {
		t.Error("expected error when creating without provenance context")
	}
	if err != nil && err.Error() != "provenance context required" {
		t.Errorf("expected 'provenance context required' error, got: %v", err)
	}
}

func TestKnowledgeRepository_Upsert_Provenance_Update(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	// Create fact with inference provenance
	ctxInference, cleanupCreate := tc.createTestContextWithSource(models.SourceInferred)
	defer cleanupCreate()

	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "updatable_fact",
		Value:     "Original value",
	}

	err := tc.repo.Upsert(ctxInference, fact)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify initial state
	if fact.Source != "inferred" {
		t.Errorf("expected initial Source 'inference', got %q", fact.Source)
	}
	if fact.LastEditSource != nil {
		t.Errorf("expected nil LastEditSource initially, got %v", fact.LastEditSource)
	}

	originalID := fact.ID

	// Update by ID with manual provenance
	ctxManual, cleanupUpdate := tc.createTestContextWithSource(models.SourceManual)
	defer cleanupUpdate()

	// Update by ID
	updateFact := &models.KnowledgeFact{
		ID:        originalID,
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "updatable_fact_renamed",
		Value:     "Updated by user",
		Context:   "Manual update",
	}

	err = tc.repo.Upsert(ctxManual, updateFact)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify LastEditSource was set
	if updateFact.LastEditSource == nil || *updateFact.LastEditSource != "manual" {
		t.Errorf("expected LastEditSource 'manual', got %v", updateFact.LastEditSource)
	}
	// Verify UpdatedBy was set
	if updateFact.UpdatedBy == nil || *updateFact.UpdatedBy != tc.testUserID {
		t.Errorf("expected UpdatedBy %v, got %v", tc.testUserID, updateFact.UpdatedBy)
	}

	// Verify persisted correctly
	retrieved, err := tc.repo.GetByKey(ctxManual, tc.projectID, models.FactTypeTerminology, "updatable_fact_renamed")
	if err != nil {
		t.Fatalf("GetByKey failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected fact to be found")
	}
	// Note: Source is now whatever the upsert set (since update doesn't preserve original source)
	// LastEditSource should be set to manual
	if retrieved.LastEditSource == nil || *retrieved.LastEditSource != "manual" {
		t.Errorf("expected persisted LastEditSource 'manual', got %v", retrieved.LastEditSource)
	}
	// UpdatedBy should be set
	if retrieved.UpdatedBy == nil || *retrieved.UpdatedBy != tc.testUserID {
		t.Errorf("expected persisted UpdatedBy %v, got %v", tc.testUserID, retrieved.UpdatedBy)
	}
}

func TestKnowledgeRepository_DeleteBySource(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	// Create facts with inference provenance
	ctxInference, cleanupInference := tc.createTestContextWithSource(models.SourceInferred)
	defer cleanupInference()

	fact1 := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "inferred_fact_1",
		Value:     "Created by inference",
	}
	if err := tc.repo.Upsert(ctxInference, fact1); err != nil {
		t.Fatalf("Create fact1 failed: %v", err)
	}

	fact2 := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeBusinessRule,
		Key:       "inferred_fact_2",
		Value:     "Also created by inference",
	}
	if err := tc.repo.Upsert(ctxInference, fact2); err != nil {
		t.Fatalf("Create fact2 failed: %v", err)
	}

	// Create fact with manual provenance
	ctxManual, cleanupManual := tc.createTestContextWithSource(models.SourceManual)
	defer cleanupManual()

	fact3 := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  models.FactTypeTerminology,
		Key:       "manual_fact",
		Value:     "Created manually",
	}
	if err := tc.repo.Upsert(ctxManual, fact3); err != nil {
		t.Fatalf("Create fact3 failed: %v", err)
	}

	// Delete inference facts
	err := tc.repo.DeleteBySource(ctxManual, tc.projectID, models.SourceInferred)
	if err != nil {
		t.Fatalf("DeleteBySource failed: %v", err)
	}

	// Verify inference facts are deleted
	retrieved1, _ := tc.repo.GetByKey(ctxManual, tc.projectID, models.FactTypeTerminology, "inferred_fact_1")
	if retrieved1 != nil {
		t.Error("expected inferred fact1 to be deleted")
	}

	retrieved2, _ := tc.repo.GetByKey(ctxManual, tc.projectID, models.FactTypeBusinessRule, "inferred_fact_2")
	if retrieved2 != nil {
		t.Error("expected inferred fact2 to be deleted")
	}

	// Verify manual fact still exists
	retrieved3, err := tc.repo.GetByKey(ctxManual, tc.projectID, models.FactTypeTerminology, "manual_fact")
	if err != nil {
		t.Fatalf("GetByKey for manual fact failed: %v", err)
	}
	if retrieved3 == nil {
		t.Error("expected manual fact to still exist")
	}
}

func TestKnowledgeRepository_DeleteBySource_NoTenantScope(t *testing.T) {
	tc := setupKnowledgeTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	err := tc.repo.DeleteBySource(ctx, tc.projectID, models.SourceInferred)
	if err == nil {
		t.Error("expected error for DeleteBySource without tenant scope")
	}
}

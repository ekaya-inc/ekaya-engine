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

// ontologyTestContext holds test dependencies for ontology repository tests.
type ontologyTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      OntologyRepository
	projectID uuid.UUID
}

// setupOntologyTest initializes the test context with shared testcontainer.
func setupOntologyTest(t *testing.T) *ontologyTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &ontologyTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      NewOntologyRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000040"),
	}
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *ontologyTestContext) ensureTestProject() {
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
	`, tc.projectID, "Ontology Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

// cleanup removes test ontologies.
func (tc *ontologyTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and inferred provenance.
func (tc *ontologyTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	// Add provenance context for write operations (using nil UUID since ontology tests don't need user validation)
	ctx = models.WithInferredProvenance(ctx, uuid.Nil)
	return ctx, func() { scope.Close() }
}

// createTestOntology creates an ontology for testing.
func (tc *ontologyTestContext) createTestOntology(ctx context.Context, version int, isActive bool) *models.TieredOntology {
	tc.t.Helper()
	ontology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   version,
		IsActive:  isActive,
		DomainSummary: &models.DomainSummary{
			Description: "Test domain summary",
			Domains:     []string{"sales", "customer"},
		},
		EntitySummaries: map[string]*models.EntitySummary{
			"accounts": {
				TableName:    "accounts",
				BusinessName: "Accounts",
				Description:  "Customer accounts",
				Domain:       "customer",
			},
		},
		Metadata: map[string]any{"test": true},
	}
	err := tc.repo.Create(ctx, ontology)
	if err != nil {
		tc.t.Fatalf("failed to create test ontology: %v", err)
	}
	return ontology
}

// ============================================================================
// Create Tests
// ============================================================================

func TestOntologyRepository_Create_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	ontology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
		DomainSummary: &models.DomainSummary{
			Description: "A test business domain",
			Domains:     []string{"sales", "finance"},
		},
		EntitySummaries: map[string]*models.EntitySummary{
			"orders": {
				TableName:    "orders",
				BusinessName: "Orders",
				Description:  "Customer purchase orders",
				Domain:       "sales",
			},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"orders": {
				{Name: "id", IsPrimaryKey: true},
				{Name: "amount", Role: "measure"},
			},
		},
		Metadata: map[string]any{"tables_analyzed": 10},
	}

	err := tc.repo.Create(ctx, ontology)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if ontology.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if ontology.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved.DomainSummary == nil {
		t.Error("expected DomainSummary to be set")
	}
	if retrieved.DomainSummary.Description != "A test business domain" {
		t.Errorf("expected description 'A test business domain', got %q", retrieved.DomainSummary.Description)
	}
	if len(retrieved.EntitySummaries) != 1 {
		t.Errorf("expected 1 entity summary, got %d", len(retrieved.EntitySummaries))
	}
}

func TestOntologyRepository_Create_WithNilFields(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create with nil optional fields
	ontology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
	}

	err := tc.repo.Create(ctx, ontology)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved.DomainSummary != nil {
		t.Error("expected nil DomainSummary")
	}
	if retrieved.EntitySummaries != nil {
		t.Error("expected nil EntitySummaries")
	}
}

// ============================================================================
// GetActive Tests
// ============================================================================

func TestOntologyRepository_GetActive_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create inactive and active versions
	tc.createTestOntology(ctx, 1, false)
	active := tc.createTestOntology(ctx, 2, true)

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected active ontology, got nil")
	}
	if retrieved.Version != active.Version {
		t.Errorf("expected version %d, got %d", active.Version, retrieved.Version)
	}
	if !retrieved.IsActive {
		t.Error("expected IsActive to be true")
	}
}

func TestOntologyRepository_GetActive_NoActive(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create only inactive versions
	tc.createTestOntology(ctx, 1, false)
	tc.createTestOntology(ctx, 2, false)

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved != nil {
		t.Errorf("expected nil, got ontology version %d", retrieved.Version)
	}
}

func TestOntologyRepository_GetActive_Empty(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for empty project")
	}
}

// ============================================================================
// UpdateDomainSummary Tests
// ============================================================================

func TestOntologyRepository_UpdateDomainSummary_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	newSummary := &models.DomainSummary{
		Description:     "Updated domain description",
		Domains:         []string{"operations", "hr"},
		SampleQuestions: []string{"What are the active users?"},
	}

	err := tc.repo.UpdateDomainSummary(ctx, tc.projectID, newSummary)
	if err != nil {
		t.Fatalf("UpdateDomainSummary failed: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved.DomainSummary.Description != "Updated domain description" {
		t.Errorf("expected updated description, got %q", retrieved.DomainSummary.Description)
	}
	if len(retrieved.DomainSummary.SampleQuestions) != 1 {
		t.Errorf("expected 1 sample question, got %d", len(retrieved.DomainSummary.SampleQuestions))
	}
}

func TestOntologyRepository_UpdateDomainSummary_NoActive(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, false) // Inactive

	err := tc.repo.UpdateDomainSummary(ctx, tc.projectID, &models.DomainSummary{Description: "test"})
	if err == nil {
		t.Error("expected error when no active ontology")
	}
}

// ============================================================================
// UpdateEntitySummary Tests
// ============================================================================

func TestOntologyRepository_UpdateEntitySummary_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	newSummary := &models.EntitySummary{
		TableName:    "orders",
		BusinessName: "Customer Orders",
		Description:  "All customer purchase orders",
		Domain:       "sales",
		Synonyms:     []string{"purchases", "transactions"},
	}

	err := tc.repo.UpdateEntitySummary(ctx, tc.projectID, "orders", newSummary)
	if err != nil {
		t.Fatalf("UpdateEntitySummary failed: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	ordersSummary := retrieved.EntitySummaries["orders"]
	if ordersSummary == nil {
		t.Fatal("expected orders summary")
	}
	if ordersSummary.BusinessName != "Customer Orders" {
		t.Errorf("expected 'Customer Orders', got %q", ordersSummary.BusinessName)
	}
	if len(ordersSummary.Synonyms) != 2 {
		t.Errorf("expected 2 synonyms, got %d", len(ordersSummary.Synonyms))
	}
}

func TestOntologyRepository_UpdateEntitySummary_AddNew(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	// Add a new entity (not in original)
	newSummary := &models.EntitySummary{
		TableName:    "products",
		BusinessName: "Products",
		Description:  "Product catalog",
		Domain:       "product",
	}

	err := tc.repo.UpdateEntitySummary(ctx, tc.projectID, "products", newSummary)
	if err != nil {
		t.Fatalf("UpdateEntitySummary failed: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Should have both original and new
	if len(retrieved.EntitySummaries) != 2 {
		t.Errorf("expected 2 entities, got %d", len(retrieved.EntitySummaries))
	}
	if retrieved.EntitySummaries["products"] == nil {
		t.Error("expected products entity")
	}
	if retrieved.EntitySummaries["accounts"] == nil {
		t.Error("expected original accounts entity to remain")
	}
}

// ============================================================================
// UpdateEntitySummaries (Batch) Tests
// ============================================================================

func TestOntologyRepository_UpdateEntitySummaries_Batch(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	summaries := map[string]*models.EntitySummary{
		"users": {
			TableName:    "users",
			BusinessName: "Users",
			Description:  "Application users",
			Domain:       "customer",
		},
		"roles": {
			TableName:    "roles",
			BusinessName: "User Roles",
			Description:  "Role definitions",
			Domain:       "customer",
		},
	}

	err := tc.repo.UpdateEntitySummaries(ctx, tc.projectID, summaries)
	if err != nil {
		t.Fatalf("UpdateEntitySummaries failed: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Should have original + 2 new
	if len(retrieved.EntitySummaries) != 3 {
		t.Errorf("expected 3 entities, got %d", len(retrieved.EntitySummaries))
	}
}

// ============================================================================
// UpdateColumnDetails Tests
// ============================================================================

func TestOntologyRepository_UpdateColumnDetails_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	columns := []models.ColumnDetail{
		{Name: "id", IsPrimaryKey: true, Role: "identifier"},
		{Name: "name", Description: "Account name", Synonyms: []string{"title", "label"}},
		{Name: "balance", Role: "measure", SemanticType: "currency"},
	}

	err := tc.repo.UpdateColumnDetails(ctx, tc.projectID, "accounts", columns)
	if err != nil {
		t.Fatalf("UpdateColumnDetails failed: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	accountCols := retrieved.ColumnDetails["accounts"]
	if len(accountCols) != 3 {
		t.Errorf("expected 3 columns, got %d", len(accountCols))
	}
}

// ============================================================================
// GetNextVersion Tests
// ============================================================================

func TestOntologyRepository_GetNextVersion_FirstVersion(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	version, err := tc.repo.GetNextVersion(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetNextVersion failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1 for new project, got %d", version)
	}
}

func TestOntologyRepository_GetNextVersion_Increments(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, false)
	tc.createTestOntology(ctx, 2, true)

	version, err := tc.repo.GetNextVersion(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetNextVersion failed: %v", err)
	}
	if version != 3 {
		t.Errorf("expected version 3, got %d", version)
	}
}

// ============================================================================
// Create Deactivates Prior Ontologies Tests
// ============================================================================

func TestOntologyRepository_Create_DeactivatesPriorOntologies(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create first ontology (version 1, active)
	first := tc.createTestOntology(ctx, 1, true)
	if !first.IsActive {
		t.Fatal("first ontology should be active")
	}

	// Create second ontology (version 2, active)
	// This should automatically deactivate the first one
	second := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   2,
		IsActive:  true,
		Metadata:  map[string]any{"test": true},
	}
	err := tc.repo.Create(ctx, second)
	if err != nil {
		t.Fatalf("Create second ontology failed: %v", err)
	}

	// Verify only one active ontology exists
	activeCount := tc.countActiveOntologies(ctx)
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active ontology, got %d", activeCount)
	}

	// Verify the active one is version 2
	active, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if active.Version != 2 {
		t.Errorf("expected active version to be 2, got %d", active.Version)
	}

	// Verify first ontology is now inactive
	firstStatus := tc.getOntologyActiveStatus(ctx, first.ID)
	if firstStatus {
		t.Error("first ontology should have been deactivated")
	}
}

func TestOntologyRepository_Create_DeactivatesPriorOntologies_MultipleExisting(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple active ontologies (simulating the bug state)
	tc.createTestOntology(ctx, 1, true)
	tc.createTestOntology(ctx, 2, true)
	tc.createTestOntology(ctx, 3, true)

	// Count active before fix - should be 3 with current bug
	activeBefore := tc.countActiveOntologies(ctx)
	t.Logf("Active ontologies before new create: %d", activeBefore)

	// Create new ontology - this should deactivate ALL prior ontologies
	newOntology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   4,
		IsActive:  true,
		Metadata:  map[string]any{"test": true},
	}
	err := tc.repo.Create(ctx, newOntology)
	if err != nil {
		t.Fatalf("Create new ontology failed: %v", err)
	}

	// Verify only one active ontology exists
	activeCount := tc.countActiveOntologies(ctx)
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active ontology after create, got %d", activeCount)
	}

	// Verify the active one is the new version
	active, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if active.Version != 4 {
		t.Errorf("expected active version to be 4, got %d", active.Version)
	}
}

// countActiveOntologies counts ontologies with is_active=true for the test project.
func (tc *ontologyTestContext) countActiveOntologies(ctx context.Context) int {
	tc.t.Helper()
	scope, _ := database.GetTenantScope(ctx)
	var count int
	err := scope.Conn.QueryRow(ctx,
		"SELECT COUNT(*) FROM engine_ontologies WHERE project_id = $1 AND is_active = true",
		tc.projectID).Scan(&count)
	if err != nil {
		tc.t.Fatalf("failed to count active ontologies: %v", err)
	}
	return count
}

// getOntologyActiveStatus returns the is_active status for a specific ontology.
func (tc *ontologyTestContext) getOntologyActiveStatus(ctx context.Context, ontologyID uuid.UUID) bool {
	tc.t.Helper()
	scope, _ := database.GetTenantScope(ctx)
	var isActive bool
	err := scope.Conn.QueryRow(ctx,
		"SELECT is_active FROM engine_ontologies WHERE id = $1",
		ontologyID).Scan(&isActive)
	if err != nil {
		tc.t.Fatalf("failed to get ontology status: %v", err)
	}
	return isActive
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestOntologyRepository_NoTenantScope(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	ontology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
	}

	// Create should fail
	err := tc.repo.Create(ctx, ontology)
	if err == nil {
		t.Error("expected error for Create without tenant scope")
	}

	// GetActive should fail
	_, err = tc.repo.GetActive(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetActive without tenant scope")
	}

	// UpdateDomainSummary should fail
	err = tc.repo.UpdateDomainSummary(ctx, tc.projectID, &models.DomainSummary{})
	if err == nil {
		t.Error("expected error for UpdateDomainSummary without tenant scope")
	}

	// UpdateEntitySummary should fail
	err = tc.repo.UpdateEntitySummary(ctx, tc.projectID, "test", &models.EntitySummary{})
	if err == nil {
		t.Error("expected error for UpdateEntitySummary without tenant scope")
	}

	// UpdateEntitySummaries should fail
	err = tc.repo.UpdateEntitySummaries(ctx, tc.projectID, map[string]*models.EntitySummary{})
	if err == nil {
		t.Error("expected error for UpdateEntitySummaries without tenant scope")
	}

	// UpdateColumnDetails should fail
	err = tc.repo.UpdateColumnDetails(ctx, tc.projectID, "test", nil)
	if err == nil {
		t.Error("expected error for UpdateColumnDetails without tenant scope")
	}

	// GetNextVersion should fail
	_, err = tc.repo.GetNextVersion(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetNextVersion without tenant scope")
	}
}

// ============================================================================
// DeleteByProject Tests
// ============================================================================

func TestOntologyRepository_DeleteByProject_CleansUpRelatedData(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an ontology
	ontology := tc.createTestOntology(ctx, 1, true)

	// Create knowledge facts linked to this project and ontology
	knowledgeRepo := NewKnowledgeRepository()
	fact := &models.KnowledgeFact{
		ProjectID:  tc.projectID,
		OntologyID: &ontology.ID,
		FactType:   "terminology",
		Key:        "test_term",
		Value:      "A test term definition",
	}
	err := knowledgeRepo.Upsert(ctx, fact)
	if err != nil {
		t.Fatalf("failed to create knowledge fact: %v", err)
	}

	// Create glossary terms linked to this project and ontology
	glossaryRepo := NewGlossaryRepository()
	term := &models.BusinessGlossaryTerm{
		ProjectID:   tc.projectID,
		OntologyID:  &ontology.ID,
		Term:        "Active Users",
		Definition:  "Users who logged in recently",
		DefiningSQL: "SELECT * FROM users WHERE last_login > NOW() - INTERVAL '30 days'",
		Source:      models.GlossarySourceInferred,
	}
	err = glossaryRepo.Create(ctx, term)
	if err != nil {
		t.Fatalf("failed to create glossary term: %v", err)
	}

	// Verify data exists before delete
	facts, err := knowledgeRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("failed to get knowledge facts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected knowledge facts to exist before delete")
	}

	terms, err := glossaryRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("failed to get glossary terms: %v", err)
	}
	if len(terms) == 0 {
		t.Fatal("expected glossary terms to exist before delete")
	}

	// Delete by project
	err = tc.repo.DeleteByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("DeleteByProject failed: %v", err)
	}

	// Verify ontology is deleted
	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected ontology to be deleted")
	}

	// Verify knowledge facts are deleted
	facts, err = knowledgeRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("failed to get knowledge facts after delete: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 knowledge facts after delete, got %d", len(facts))
	}

	// Verify glossary terms are deleted
	terms, err = glossaryRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("failed to get glossary terms after delete: %v", err)
	}
	if len(terms) != 0 {
		t.Errorf("expected 0 glossary terms after delete, got %d", len(terms))
	}
}

func TestOntologyRepository_DeleteByProject_EmptyProject(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Delete should succeed even if no data exists
	err := tc.repo.DeleteByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("DeleteByProject failed on empty project: %v", err)
	}
}

func TestOntologyRepository_DeleteByProject_NoTenantScope(t *testing.T) {
	tc := setupOntologyTest(t)
	ctx := context.Background() // No tenant scope

	err := tc.repo.DeleteByProject(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for DeleteByProject without tenant scope")
	}
}

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

// createTestContext returns a context with tenant scope.
func (tc *ontologyTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
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
	retrieved, err := tc.repo.GetByVersion(ctx, tc.projectID, 1)
	if err != nil {
		t.Fatalf("GetByVersion failed: %v", err)
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

	retrieved, err := tc.repo.GetByVersion(ctx, tc.projectID, 1)
	if err != nil {
		t.Fatalf("GetByVersion failed: %v", err)
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
// GetByVersion Tests
// ============================================================================

func TestOntologyRepository_GetByVersion_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, false)
	tc.createTestOntology(ctx, 2, true)

	retrieved, err := tc.repo.GetByVersion(ctx, tc.projectID, 1)
	if err != nil {
		t.Fatalf("GetByVersion failed: %v", err)
	}
	if retrieved.Version != 1 {
		t.Errorf("expected version 1, got %d", retrieved.Version)
	}
}

func TestOntologyRepository_GetByVersion_NotFound(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	_, err := tc.repo.GetByVersion(ctx, tc.projectID, 99)
	if err == nil {
		t.Error("expected error for non-existent version")
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
// SetActive Tests
// ============================================================================

func TestOntologyRepository_SetActive_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)
	tc.createTestOntology(ctx, 2, false)

	// Switch active to version 2
	err := tc.repo.SetActive(ctx, tc.projectID, 2)
	if err != nil {
		t.Fatalf("SetActive failed: %v", err)
	}

	// Verify version 2 is now active
	active, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if active.Version != 2 {
		t.Errorf("expected version 2 active, got %d", active.Version)
	}

	// Verify version 1 is now inactive
	v1, err := tc.repo.GetByVersion(ctx, tc.projectID, 1)
	if err != nil {
		t.Fatalf("GetByVersion failed: %v", err)
	}
	if v1.IsActive {
		t.Error("expected version 1 to be inactive")
	}
}

func TestOntologyRepository_SetActive_NotFound(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestOntology(ctx, 1, true)

	err := tc.repo.SetActive(ctx, tc.projectID, 99)
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

// ============================================================================
// DeactivateAll Tests
// ============================================================================

func TestOntologyRepository_DeactivateAll_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create one active ontology
	tc.createTestOntology(ctx, 1, true)

	err := tc.repo.DeactivateAll(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("DeactivateAll failed: %v", err)
	}

	active, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if active != nil {
		t.Error("expected no active ontology after DeactivateAll")
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

	// GetByVersion should fail
	_, err = tc.repo.GetByVersion(ctx, tc.projectID, 1)
	if err == nil {
		t.Error("expected error for GetByVersion without tenant scope")
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

	// SetActive should fail
	err = tc.repo.SetActive(ctx, tc.projectID, 1)
	if err == nil {
		t.Error("expected error for SetActive without tenant scope")
	}

	// DeactivateAll should fail
	err = tc.repo.DeactivateAll(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for DeactivateAll without tenant scope")
	}

	// GetNextVersion should fail
	_, err = tc.repo.GetNextVersion(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetNextVersion without tenant scope")
	}
}

// ============================================================================
// WriteCleanOntology Tests
// ============================================================================

func TestOntologyRepository_WriteCleanOntology_Success(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create ontology with semantic content
	ontology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
		DomainSummary: &models.DomainSummary{
			Description: "Test domain",
			Domains:     []string{"sales"},
		},
		EntitySummaries: map[string]*models.EntitySummary{
			"orders": {
				TableName:    "orders",
				BusinessName: "Customer Orders",
				Description:  "All customer purchase orders",
				Domain:       "sales",
				Synonyms:     []string{"purchases"},
			},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"orders": {
				{
					Name:         "id",
					Description:  "Order identifier",
					IsPrimaryKey: true,
				},
				{
					Name:         "status",
					Description:  "Order status",
					SemanticType: "order_lifecycle",
					EnumValues: []models.EnumValue{
						{Value: "pending", Description: "Awaiting processing"},
						{Value: "shipped", Description: "Order shipped"},
					},
				},
			},
		},
	}

	err := tc.repo.Create(ctx, ontology)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// WriteCleanOntology should succeed when active ontology exists
	err = tc.repo.WriteCleanOntology(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("WriteCleanOntology failed: %v", err)
	}

	// Verify ontology is still accessible
	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Verify semantic content is preserved
	if retrieved.DomainSummary == nil || retrieved.DomainSummary.Description != "Test domain" {
		t.Error("expected DomainSummary to be preserved")
	}

	ordersSummary := retrieved.EntitySummaries["orders"]
	if ordersSummary == nil {
		t.Fatal("expected orders entity summary")
	}
	if ordersSummary.BusinessName != "Customer Orders" {
		t.Errorf("expected BusinessName 'Customer Orders', got %q", ordersSummary.BusinessName)
	}

	ordersCols := retrieved.ColumnDetails["orders"]
	if len(ordersCols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(ordersCols))
	}
}

func TestOntologyRepository_WriteCleanOntology_NoActiveOntology(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// No ontology exists
	err := tc.repo.WriteCleanOntology(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error when no active ontology")
	}
}

func TestOntologyRepository_WriteCleanOntology_EmptyOntology(t *testing.T) {
	tc := setupOntologyTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create empty ontology
	ontology := &models.TieredOntology{
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
	}

	err := tc.repo.Create(ctx, ontology)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// WriteCleanOntology should handle nil maps gracefully
	err = tc.repo.WriteCleanOntology(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("WriteCleanOntology failed for empty ontology: %v", err)
	}

	retrieved, err := tc.repo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Should still be valid with empty maps
	if retrieved.EntitySummaries == nil {
		// Empty map after clean write is acceptable
	}
}

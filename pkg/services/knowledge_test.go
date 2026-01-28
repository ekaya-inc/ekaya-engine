//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// knowledgeServiceTestContext holds test dependencies for knowledge service tests.
type knowledgeServiceTestContext struct {
	t             *testing.T
	engineDB      *testhelpers.EngineDB
	service       KnowledgeService
	knowledgeRepo repositories.KnowledgeRepository
	ontologyRepo  repositories.OntologyRepository
	projectID     uuid.UUID
	ontologyID    uuid.UUID
	testUserID    uuid.UUID
}

// setupKnowledgeServiceTest initializes the test context with shared testcontainer.
func setupKnowledgeServiceTest(t *testing.T) *knowledgeServiceTestContext {
	engineDB := testhelpers.GetEngineDB(t)

	knowledgeRepo := repositories.NewKnowledgeRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	projectRepo := repositories.NewProjectRepository()

	tc := &knowledgeServiceTestContext{
		t:             t,
		engineDB:      engineDB,
		knowledgeRepo: knowledgeRepo,
		ontologyRepo:  ontologyRepo,
		service:       NewKnowledgeService(knowledgeRepo, projectRepo, ontologyRepo, zap.NewNop()),
		projectID:     uuid.MustParse("00000000-0000-0000-0000-000000000044"),
		ontologyID:    uuid.MustParse("00000000-0000-0000-0000-000000000144"),
		testUserID:    uuid.MustParse("00000000-0000-0000-0000-000000000047"),
	}
	tc.ensureTestProject()
	tc.ensureTestOntology()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *knowledgeServiceTestContext) ensureTestProject() {
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
	`, tc.projectID, "Knowledge Service Test Project")
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

// ensureTestOntology creates the test ontology if it doesn't exist.
func (tc *knowledgeServiceTestContext) ensureTestOntology() {
	tc.t.Helper()
	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	scope, _ := database.GetTenantScope(ctx)
	err := scope.Conn.QueryRow(ctx, `
		INSERT INTO engine_ontologies (id, project_id, is_active, domain_summary, entity_summaries, column_details)
		VALUES ($1, $2, true, '{}', '{}', '{}')
		ON CONFLICT (project_id, version) DO UPDATE SET is_active = true
		RETURNING id
	`, tc.ontologyID, tc.projectID).Scan(&tc.ontologyID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}
}

// createTestContext creates a tenant-scoped context for testing with manual provenance.
func (tc *knowledgeServiceTestContext) createTestContext() (context.Context, func()) {
	return tc.createTestContextWithProvenance(models.SourceManual)
}

// createTestContextWithProvenance creates a tenant-scoped context with specified provenance source.
func (tc *knowledgeServiceTestContext) createTestContextWithProvenance(source models.ProvenanceSource) (context.Context, func()) {
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

// cleanup removes test knowledge facts.
func (tc *knowledgeServiceTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		DELETE FROM engine_project_knowledge WHERE project_id = $1
	`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to cleanup test knowledge: %v", err)
	}
}

func TestKnowledgeService_StoreWithSource_Manual(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test storing with manual source
	fact, err := tc.service.StoreWithSource(ctx, tc.projectID, "overview", "project_overview", "Test project overview", "", "manual")
	if err != nil {
		t.Fatalf("StoreWithSource failed: %v", err)
	}

	// Verify the fact was stored
	if fact.ProjectID != tc.projectID {
		t.Errorf("ProjectID = %v, want %v", fact.ProjectID, tc.projectID)
	}
	if fact.FactType != "overview" {
		t.Errorf("FactType = %v, want %v", fact.FactType, "overview")
	}
	if fact.Key != "project_overview" {
		t.Errorf("Key = %v, want %v", fact.Key, "project_overview")
	}
	if fact.Value != "Test project overview" {
		t.Errorf("Value = %v, want %v", fact.Value, "Test project overview")
	}
	if fact.Source != "manual" {
		t.Errorf("Source = %v, want %v", fact.Source, "manual")
	}
}

func TestKnowledgeService_StoreWithSource_Inferred(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test storing with inferred source (non-overview fact)
	fact, err := tc.service.StoreWithSource(ctx, tc.projectID, "business_rule", "test_rule", "Test business rule value", "Some context", "inferred")
	if err != nil {
		t.Fatalf("StoreWithSource failed: %v", err)
	}

	// Verify the fact was stored
	if fact.Source != "inferred" {
		t.Errorf("Source = %v, want %v", fact.Source, "inferred")
	}
}

func TestKnowledgeService_StoreWithSource_MCP(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test storing with MCP source
	fact, err := tc.service.StoreWithSource(ctx, tc.projectID, "domain_term", "channel", "A video content creator", "", "mcp")
	if err != nil {
		t.Fatalf("StoreWithSource failed: %v", err)
	}

	if fact.Source != "mcp" {
		t.Errorf("Source = %v, want %v", fact.Source, "mcp")
	}
}

func TestKnowledgeService_StoreWithSource_InvalidSource(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test with invalid source
	_, err := tc.service.StoreWithSource(ctx, tc.projectID, "test", "key", "value", "", "invalid_source")
	if err == nil {
		t.Error("StoreWithSource should fail with invalid source")
	}
}

func TestKnowledgeService_StoreWithSource_ProjectOverviewNilOntology(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Store a project_overview fact
	fact, err := tc.service.StoreWithSource(ctx, tc.projectID, "overview", "project_overview", "Overview that survives ontology deletion", "", "manual")
	if err != nil {
		t.Fatalf("StoreWithSource failed: %v", err)
	}

	// Verify the fact was stored with the correct values
	if fact.FactType != "overview" {
		t.Errorf("FactType = %v, want %v", fact.FactType, "overview")
	}
	if fact.Key != "project_overview" {
		t.Errorf("Key = %v, want %v", fact.Key, "project_overview")
	}

	// Store a different fact type
	otherFact, err := tc.service.StoreWithSource(ctx, tc.projectID, "convention", "timestamp_format", "UTC timestamps", "", "manual")
	if err != nil {
		t.Fatalf("StoreWithSource for convention failed: %v", err)
	}

	// Verify the convention fact was stored
	if otherFact.FactType != "convention" {
		t.Errorf("FactType = %v, want %v", otherFact.FactType, "convention")
	}
}

func TestKnowledgeService_StoreWithSource_Upsert(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Store initial fact with ontology_id set (non-overview fact for upsert testing)
	fact1, err := tc.service.StoreWithSource(ctx, tc.projectID, "business_rule", "test_key", "Initial value", "", "manual")
	if err != nil {
		t.Fatalf("Initial StoreWithSource failed: %v", err)
	}

	// Update the same fact (upsert behavior)
	fact2, err := tc.service.StoreWithSource(ctx, tc.projectID, "business_rule", "test_key", "Updated value", "Added context", "manual")
	if err != nil {
		t.Fatalf("Update StoreWithSource failed: %v", err)
	}

	// Verify the value was updated
	if fact2.Value != "Updated value" {
		t.Errorf("Value = %v, want %v", fact2.Value, "Updated value")
	}

	// Both should have the same ID (upsert)
	if fact2.ID != fact1.ID {
		t.Errorf("Upsert should update existing fact, got different IDs: %v vs %v", fact1.ID, fact2.ID)
	}

	// Verify context was set
	if fact2.Context != "Added context" {
		t.Errorf("Context = %v, want %v", fact2.Context, "Added context")
	}
}

func TestKnowledgeService_StoreWithSource_WithExistingProvenance(t *testing.T) {
	tc := setupKnowledgeServiceTest(t)
	t.Cleanup(tc.cleanup)

	// Create context with inferred provenance already set (like in DAG execution)
	ctx, cleanup := tc.createTestContextWithProvenance(models.SourceInferred)
	defer cleanup()

	// StoreWithSource should still work - it will use the userID from existing provenance
	fact, err := tc.service.StoreWithSource(ctx, tc.projectID, "business_rule", "test_key", "test_value", "", "inferred")
	if err != nil {
		t.Fatalf("StoreWithSource with existing provenance failed: %v", err)
	}

	// The fact should be stored with inferred source
	if fact.Source != "inferred" {
		t.Errorf("Source = %v, want %v", fact.Source, "inferred")
	}
}

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

// ontologyEntityTestContext holds test dependencies for ontology entity repository tests.
type ontologyEntityTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       OntologyEntityRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
	testUserID uuid.UUID // User ID for provenance context
}

// setupOntologyEntityTest initializes the test context with shared testcontainer.
func setupOntologyEntityTest(t *testing.T) *ontologyEntityTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &ontologyEntityTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewOntologyEntityRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000050"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000051"),
		testUserID: uuid.MustParse("00000000-0000-0000-0000-000000000052"), // Test user for provenance
	}
	tc.ensureTestProjectAndOntology()
	return tc
}

// ensureTestProjectAndOntology creates the test project and ontology if they don't exist.
func (tc *ontologyEntityTestContext) ensureTestProjectAndOntology() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Ontology Entity Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Delete any existing workflow and ontology for this project to start fresh
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_workflows WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)

	// Create ontology
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}
}

// cleanup removes test entities and aliases.
func (tc *ontologyEntityTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Cascading delete will handle aliases
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE ontology_id = $1", tc.ontologyID)
}

// createTestContext returns a context with tenant scope and inference provenance.
func (tc *ontologyEntityTestContext) createTestContext() (context.Context, func()) {
	return tc.createTestContextWithProvenance(models.SourceInference)
}

// createTestContextWithProvenance returns a context with tenant scope and the specified provenance source.
func (tc *ontologyEntityTestContext) createTestContextWithProvenance(source models.ProvenanceSource) (context.Context, func()) {
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

// createTestEntity creates a schema entity for testing using the provenance from context.
func (tc *ontologyEntityTestContext) createTestEntity(ctx context.Context, name string) *models.OntologyEntity {
	tc.t.Helper()
	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          name,
		Description:   "Test " + name + " entity",
		PrimarySchema: "public",
		PrimaryTable:  name + "s",
		PrimaryColumn: "id",
	}
	err := tc.repo.Create(ctx, entity)
	if err != nil {
		tc.t.Fatalf("failed to create test entity: %v", err)
	}
	return entity
}

// createTestEntityWithProvenance creates a schema entity with the specified provenance source.
// This creates a new context with the specified provenance, using the same scope as the input context.
func (tc *ontologyEntityTestContext) createTestEntityWithProvenance(ctx context.Context, name string, sourceStr string) *models.OntologyEntity {
	tc.t.Helper()

	// Map string source to ProvenanceSource
	var source models.ProvenanceSource
	switch sourceStr {
	case models.ProvenanceInference:
		source = models.SourceInference
	case models.ProvenanceMCP:
		source = models.SourceMCP
	case models.ProvenanceManual:
		source = models.SourceManual
	default:
		tc.t.Fatalf("invalid provenance source: %s", sourceStr)
	}

	// Create a new context with the specified provenance
	ctxWithProv := models.WithProvenance(ctx, models.ProvenanceContext{
		Source: source,
		UserID: tc.testUserID,
	})

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          name,
		Description:   "Test " + name + " entity",
		PrimarySchema: "public",
		PrimaryTable:  name + "s",
		PrimaryColumn: "id",
	}
	err := tc.repo.Create(ctxWithProv, entity)
	if err != nil {
		tc.t.Fatalf("failed to create test entity: %v", err)
	}
	return entity
}

// NOTE: createTestOccurrence and createTestOccurrenceDeleted were removed
// because engine_ontology_entity_occurrences table was dropped in migration 030.

// ============================================================================
// Entity Create Tests
// ============================================================================

func TestOntologyEntityRepository_Create_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "A person who uses the system",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	err := tc.repo.Create(ctx, entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if entity.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if entity.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if entity.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByName(ctx, tc.ontologyID, "user")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected entity to be found")
	}
	if retrieved.Name != "user" {
		t.Errorf("expected name 'user', got %q", retrieved.Name)
	}
	if retrieved.Description != "A person who uses the system" {
		t.Errorf("expected description 'A person who uses the system', got %q", retrieved.Description)
	}
}

func TestOntologyEntityRepository_Create_UpsertOnDuplicateName(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	original := tc.createTestEntity(ctx, "account")
	originalID := original.ID

	// Create another entity with the same name - should upsert (update existing)
	updated := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "account",
		Description:   "Updated account description",
		PrimarySchema: "new_schema",
		PrimaryTable:  "new_accounts",
		PrimaryColumn: "new_id",
	}

	err := tc.repo.Create(ctx, updated)
	if err != nil {
		t.Fatalf("Create (upsert) failed: %v", err)
	}

	// Verify the entity struct was updated with the actual ID from database
	if updated.ID != originalID {
		t.Errorf("expected ID to be original %v, got %v", originalID, updated.ID)
	}

	// Verify only one entity exists
	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("expected 1 entity after upsert, got %d", len(entities))
	}

	// Verify the entity was updated
	retrieved, err := tc.repo.GetByName(ctx, tc.ontologyID, "account")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved.Description != "Updated account description" {
		t.Errorf("expected description 'Updated account description', got %q", retrieved.Description)
	}
	if retrieved.PrimarySchema != "new_schema" {
		t.Errorf("expected primary_schema 'new_schema', got %q", retrieved.PrimarySchema)
	}
	if retrieved.PrimaryTable != "new_accounts" {
		t.Errorf("expected primary_table 'new_accounts', got %q", retrieved.PrimaryTable)
	}
}

func TestOntologyEntityRepository_Create_UpsertPreservesExistingDescription(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entity with description
	original := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "A platform user",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}
	err := tc.repo.Create(ctx, original)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Upsert with empty description - should preserve existing
	updated := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "", // Empty description
		PrimarySchema: "app",
		PrimaryTable:  "app_users",
		PrimaryColumn: "user_id",
	}
	err = tc.repo.Create(ctx, updated)
	if err != nil {
		t.Fatalf("Create (upsert) failed: %v", err)
	}

	// Verify original description was preserved
	retrieved, err := tc.repo.GetByName(ctx, tc.ontologyID, "user")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved.Description != "A platform user" {
		t.Errorf("expected description 'A platform user' to be preserved, got %q", retrieved.Description)
	}
	// But other fields should be updated
	if retrieved.PrimarySchema != "app" {
		t.Errorf("expected primary_schema 'app', got %q", retrieved.PrimarySchema)
	}
	if retrieved.PrimaryTable != "app_users" {
		t.Errorf("expected primary_table 'app_users', got %q", retrieved.PrimaryTable)
	}
}

func TestOntologyEntityRepository_Create_UpsertOverwritesWithNewDescription(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entity with description
	original := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "order",
		Description:   "Original order description",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
		PrimaryColumn: "id",
	}
	err := tc.repo.Create(ctx, original)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Upsert with new description - should overwrite
	updated := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "order",
		Description:   "Better order description",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
		PrimaryColumn: "id",
	}
	err = tc.repo.Create(ctx, updated)
	if err != nil {
		t.Fatalf("Create (upsert) failed: %v", err)
	}

	// Verify new description replaced old one
	retrieved, err := tc.repo.GetByName(ctx, tc.ontologyID, "order")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved.Description != "Better order description" {
		t.Errorf("expected description 'Better order description', got %q", retrieved.Description)
	}
}

// ============================================================================
// Entity GetByOntology Tests
// ============================================================================

func TestOntologyEntityRepository_GetByOntology_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")
	tc.createTestEntity(ctx, "account")
	tc.createTestEntity(ctx, "order")

	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 3 {
		t.Errorf("expected 3 entities, got %d", len(entities))
	}

	// Should be sorted by name
	names := []string{entities[0].Name, entities[1].Name, entities[2].Name}
	if names[0] != "account" || names[1] != "order" || names[2] != "user" {
		t.Errorf("expected entities sorted by name (account, order, user), got %v", names)
	}
}

func TestOntologyEntityRepository_GetByOntology_Empty(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(entities))
	}
}

// ============================================================================
// Entity GetByName Tests
// ============================================================================

func TestOntologyEntityRepository_GetByName_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")
	tc.createTestEntity(ctx, "account")

	entity, err := tc.repo.GetByName(ctx, tc.ontologyID, "account")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if entity == nil {
		t.Fatal("expected entity to be found")
	}
	if entity.Name != "account" {
		t.Errorf("expected name 'account', got %q", entity.Name)
	}
}

func TestOntologyEntityRepository_GetByName_NotFound(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")

	entity, err := tc.repo.GetByName(ctx, tc.ontologyID, "nonexistent")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if entity != nil {
		t.Error("expected nil for non-existent entity")
	}
}

// ============================================================================
// Entity GetByProjectAndName Tests
// ============================================================================

func TestOntologyEntityRepository_GetByProjectAndName_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")
	tc.createTestEntity(ctx, "account")

	entity, err := tc.repo.GetByProjectAndName(ctx, tc.projectID, "account")
	if err != nil {
		t.Fatalf("GetByProjectAndName failed: %v", err)
	}
	if entity == nil {
		t.Fatal("expected entity to be found")
	}
	if entity.Name != "account" {
		t.Errorf("expected name 'account', got %q", entity.Name)
	}
}

func TestOntologyEntityRepository_GetByProjectAndName_NotFound(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")

	entity, err := tc.repo.GetByProjectAndName(ctx, tc.projectID, "nonexistent")
	if err != nil {
		t.Fatalf("GetByProjectAndName failed: %v", err)
	}
	if entity != nil {
		t.Error("expected nil for non-existent entity")
	}
}

func TestOntologyEntityRepository_GetByProjectAndName_FilteredFromDeletedEntity(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Soft delete
	err := tc.repo.SoftDelete(ctx, entity.ID, "test")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// GetByProjectAndName should not find deleted entity
	retrieved, err := tc.repo.GetByProjectAndName(ctx, tc.projectID, "user")
	if err != nil {
		t.Fatalf("GetByProjectAndName failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for soft-deleted entity via GetByProjectAndName")
	}
}

func TestOntologyEntityRepository_GetByProjectAndName_OnlyFindsActiveOntology(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entity in the active ontology
	tc.createTestEntity(ctx, "user")

	// Deactivate the ontology
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		t.Fatal("failed to get tenant scope")
	}
	_, err := scope.Conn.Exec(ctx, `UPDATE engine_ontologies SET is_active = false WHERE id = $1`, tc.ontologyID)
	if err != nil {
		t.Fatalf("failed to deactivate ontology: %v", err)
	}

	// GetByProjectAndName should not find entity in inactive ontology
	entity, err := tc.repo.GetByProjectAndName(ctx, tc.projectID, "user")
	if err != nil {
		t.Fatalf("GetByProjectAndName failed: %v", err)
	}
	if entity != nil {
		t.Error("expected nil for entity in inactive ontology")
	}
}

func TestOntologyEntityRepository_GetByProjectAndName_NoTenantScope(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	_, err := tc.repo.GetByProjectAndName(ctx, tc.projectID, "user")
	if err == nil {
		t.Error("expected error for GetByProjectAndName without tenant scope")
	}
}

// ============================================================================
// Entity DeleteByOntology Tests
// ============================================================================

func TestOntologyEntityRepository_DeleteByOntology_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")
	tc.createTestEntity(ctx, "account")

	err := tc.repo.DeleteByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteByOntology failed: %v", err)
	}

	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities after delete, got %d", len(entities))
	}
}

// ============================================================================
// Entity DeleteInferenceEntitiesByOntology Tests
// ============================================================================

func TestOntologyEntityRepository_DeleteInferenceEntitiesByOntology_PreservesManualAndMCP(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entities with different provenance
	tc.createTestEntityWithProvenance(ctx, "inference_entity1", models.ProvenanceInference)
	tc.createTestEntityWithProvenance(ctx, "inference_entity2", models.ProvenanceInference)
	tc.createTestEntityWithProvenance(ctx, "manual_entity", models.ProvenanceManual)
	tc.createTestEntityWithProvenance(ctx, "mcp_entity", models.ProvenanceMCP)

	// Delete only inference entities
	err := tc.repo.DeleteInferenceEntitiesByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteInferenceEntitiesByOntology failed: %v", err)
	}

	// Verify only manual and MCP entities remain
	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities (manual + mcp), got %d", len(entities))
	}

	// Verify the correct entities remain
	entityNames := make(map[string]bool)
	for _, e := range entities {
		entityNames[e.Name] = true
	}
	if !entityNames["manual_entity"] {
		t.Error("expected manual_entity to be preserved")
	}
	if !entityNames["mcp_entity"] {
		t.Error("expected mcp_entity to be preserved")
	}
	if entityNames["inference_entity1"] || entityNames["inference_entity2"] {
		t.Error("expected inference entities to be deleted")
	}
}

func TestOntologyEntityRepository_DeleteInferenceEntitiesByOntology_NoEntities(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// No entities exist - should not error
	err := tc.repo.DeleteInferenceEntitiesByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteInferenceEntitiesByOntology failed: %v", err)
	}
}

func TestOntologyEntityRepository_DeleteInferenceEntitiesByOntology_OnlyManualEntities(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create only manual entities
	tc.createTestEntityWithProvenance(ctx, "manual_entity1", models.ProvenanceManual)
	tc.createTestEntityWithProvenance(ctx, "manual_entity2", models.ProvenanceManual)

	// Delete inference entities (none exist)
	err := tc.repo.DeleteInferenceEntitiesByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteInferenceEntitiesByOntology failed: %v", err)
	}

	// All manual entities should remain
	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(entities))
	}
}

func TestOntologyEntityRepository_DeleteInferenceEntitiesByOntology_CascadesAliases(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create inference entity with alias
	inferenceEntity := tc.createTestEntityWithProvenance(ctx, "inference_entity", models.ProvenanceInference)
	err := tc.repo.CreateAlias(ctx, &models.OntologyEntityAlias{
		EntityID: inferenceEntity.ID,
		Alias:    "inference_alias",
	})
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	// Create manual entity with alias
	manualEntity := tc.createTestEntityWithProvenance(ctx, "manual_entity", models.ProvenanceManual)
	err = tc.repo.CreateAlias(ctx, &models.OntologyEntityAlias{
		EntityID: manualEntity.ID,
		Alias:    "manual_alias",
	})
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	// Delete inference entities
	err = tc.repo.DeleteInferenceEntitiesByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteInferenceEntitiesByOntology failed: %v", err)
	}

	// Verify inference entity alias is deleted (cascaded)
	inferenceAliases, err := tc.repo.GetAliasesByEntity(ctx, inferenceEntity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(inferenceAliases) != 0 {
		t.Errorf("expected 0 aliases for deleted inference entity, got %d", len(inferenceAliases))
	}

	// Verify manual entity alias is preserved
	manualAliases, err := tc.repo.GetAliasesByEntity(ctx, manualEntity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(manualAliases) != 1 {
		t.Errorf("expected 1 alias for manual entity, got %d", len(manualAliases))
	}
}

func TestOntologyEntityRepository_DeleteInferenceEntitiesByOntology_NoTenantScope(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	err := tc.repo.DeleteInferenceEntitiesByOntology(ctx, tc.ontologyID)
	if err == nil {
		t.Error("expected error for DeleteInferenceEntitiesByOntology without tenant scope")
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestOntologyEntityRepository_NoTenantScope(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "Test",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	// Create should fail
	err := tc.repo.Create(ctx, entity)
	if err == nil {
		t.Error("expected error for Create without tenant scope")
	}

	// GetByOntology should fail
	_, err = tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err == nil {
		t.Error("expected error for GetByOntology without tenant scope")
	}

	// GetByName should fail
	_, err = tc.repo.GetByName(ctx, tc.ontologyID, "user")
	if err == nil {
		t.Error("expected error for GetByName without tenant scope")
	}

	// DeleteByOntology should fail
	err = tc.repo.DeleteByOntology(ctx, tc.ontologyID)
	if err == nil {
		t.Error("expected error for DeleteByOntology without tenant scope")
	}
}

// ============================================================================
// Entity GetByID Tests
// ============================================================================

func TestOntologyEntityRepository_GetByID_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected entity to be found")
	}
	if retrieved.Name != "user" {
		t.Errorf("expected name 'user', got %q", retrieved.Name)
	}
	if retrieved.IsDeleted {
		t.Error("expected IsDeleted to be false for new entity")
	}
}

func TestOntologyEntityRepository_GetByID_NotFound(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	retrieved, err := tc.repo.GetByID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for non-existent entity")
	}
}

func TestOntologyEntityRepository_GetByID_ReturnsDeletedEntity(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Soft delete the entity
	err := tc.repo.SoftDelete(ctx, entity.ID, "test deletion")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// GetByID should still return deleted entities (for admin/restore purposes)
	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected deleted entity to be found via GetByID")
	}
	if !retrieved.IsDeleted {
		t.Error("expected IsDeleted to be true")
	}
}

// ============================================================================
// Entity Update Tests
// ============================================================================

func TestOntologyEntityRepository_Update_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")
	originalUpdatedAt := entity.UpdatedAt

	entity.Description = "Updated description"
	entity.PrimaryTable = "app_users"

	err := tc.repo.Update(ctx, entity)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if entity.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("expected UpdatedAt to be updated")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Description != "Updated description" {
		t.Errorf("expected description 'Updated description', got %q", retrieved.Description)
	}
	if retrieved.PrimaryTable != "app_users" {
		t.Errorf("expected primary_table 'app_users', got %q", retrieved.PrimaryTable)
	}
}

// ============================================================================
// Soft Delete Tests
// ============================================================================

func TestOntologyEntityRepository_SoftDelete_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	err := tc.repo.SoftDelete(ctx, entity.ID, "Not relevant to this domain")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// Verify entity is soft deleted
	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !retrieved.IsDeleted {
		t.Error("expected IsDeleted to be true")
	}
	if retrieved.DeletionReason == nil || *retrieved.DeletionReason != "Not relevant to this domain" {
		t.Errorf("expected deletion reason 'Not relevant to this domain', got %v", retrieved.DeletionReason)
	}
}

func TestOntologyEntityRepository_SoftDelete_FilteredFromGetByOntology(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "user")
	account := tc.createTestEntity(ctx, "account")
	tc.createTestEntity(ctx, "order")

	// Soft delete account
	err := tc.repo.SoftDelete(ctx, account.ID, "test")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// GetByOntology should not return deleted entity
	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities (excluding deleted), got %d", len(entities))
	}
	for _, e := range entities {
		if e.Name == "account" {
			t.Error("deleted entity 'account' should not be returned")
		}
	}
}

func TestOntologyEntityRepository_SoftDelete_FilteredFromGetByName(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Soft delete
	err := tc.repo.SoftDelete(ctx, entity.ID, "test")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// GetByName should not find deleted entity
	retrieved, err := tc.repo.GetByName(ctx, tc.ontologyID, "user")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for soft-deleted entity via GetByName")
	}
}

// ============================================================================
// Restore Tests
// ============================================================================

func TestOntologyEntityRepository_Restore_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Soft delete
	err := tc.repo.SoftDelete(ctx, entity.ID, "test deletion")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// Restore
	err = tc.repo.Restore(ctx, entity.ID)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify entity is restored
	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.IsDeleted {
		t.Error("expected IsDeleted to be false after restore")
	}
	if retrieved.DeletionReason != nil {
		t.Error("expected DeletionReason to be nil after restore")
	}

	// Should be visible in GetByName again
	byName, err := tc.repo.GetByName(ctx, tc.ontologyID, "user")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if byName == nil {
		t.Error("expected restored entity to be findable via GetByName")
	}
}

// ============================================================================
// Alias CreateAlias Tests
// ============================================================================

func TestOntologyEntityRepository_CreateAlias_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	source := "user"
	alias := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "customer",
		Source:   &source,
	}

	err := tc.repo.CreateAlias(ctx, alias)
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	if alias.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if alias.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching
	aliases, err := tc.repo.GetAliasesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].Alias != "customer" {
		t.Errorf("expected alias 'customer', got %q", aliases[0].Alias)
	}
	if aliases[0].Source == nil || *aliases[0].Source != "user" {
		t.Errorf("expected source 'user', got %v", aliases[0].Source)
	}
}

func TestOntologyEntityRepository_CreateAlias_DuplicateAlias(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	alias1 := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "customer",
	}
	err := tc.repo.CreateAlias(ctx, alias1)
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	// Try to create duplicate
	alias2 := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "customer",
	}
	err = tc.repo.CreateAlias(ctx, alias2)
	if err == nil {
		t.Error("expected error for duplicate alias")
	}
}

func TestOntologyEntityRepository_CreateAlias_NullSource(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	alias := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "person",
		Source:   nil,
	}

	err := tc.repo.CreateAlias(ctx, alias)
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	aliases, err := tc.repo.GetAliasesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].Source != nil {
		t.Errorf("expected nil source, got %v", aliases[0].Source)
	}
}

// ============================================================================
// Alias GetAliasesByEntity Tests
// ============================================================================

func TestOntologyEntityRepository_GetAliasesByEntity_Multiple(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create multiple aliases
	aliases := []string{"customer", "member", "person"}
	for _, a := range aliases {
		err := tc.repo.CreateAlias(ctx, &models.OntologyEntityAlias{
			EntityID: entity.ID,
			Alias:    a,
		})
		if err != nil {
			t.Fatalf("CreateAlias failed: %v", err)
		}
	}

	retrieved, err := tc.repo.GetAliasesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(retrieved) != 3 {
		t.Fatalf("expected 3 aliases, got %d", len(retrieved))
	}

	// Should be sorted by alias
	if retrieved[0].Alias != "customer" || retrieved[1].Alias != "member" || retrieved[2].Alias != "person" {
		t.Errorf("expected aliases sorted (customer, member, person), got (%s, %s, %s)",
			retrieved[0].Alias, retrieved[1].Alias, retrieved[2].Alias)
	}
}

func TestOntologyEntityRepository_GetAliasesByEntity_Empty(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	aliases, err := tc.repo.GetAliasesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases, got %d", len(aliases))
	}
}

// ============================================================================
// Alias DeleteAlias Tests
// ============================================================================

func TestOntologyEntityRepository_DeleteAlias_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	alias := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "customer",
	}
	err := tc.repo.CreateAlias(ctx, alias)
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	// Delete the alias
	err = tc.repo.DeleteAlias(ctx, alias.ID)
	if err != nil {
		t.Fatalf("DeleteAlias failed: %v", err)
	}

	// Verify alias is deleted
	aliases, err := tc.repo.GetAliasesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestOntologyEntityRepository_DeleteAlias_CascadeOnEntityDelete(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	alias := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "customer",
	}
	err := tc.repo.CreateAlias(ctx, alias)
	if err != nil {
		t.Fatalf("CreateAlias failed: %v", err)
	}

	// Delete entity should cascade to aliases
	err = tc.repo.DeleteByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteByOntology failed: %v", err)
	}

	// Verify aliases are deleted
	aliases, err := tc.repo.GetAliasesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetAliasesByEntity failed: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after entity delete, got %d", len(aliases))
	}
}

// ============================================================================
// CountOccurrencesByEntity and GetOccurrenceTablesByEntity Tests
// ============================================================================
// NOTE: These tests were removed because the engine_ontology_entity_occurrences
// table was dropped in migration 030. The repository methods now return empty
// results for interface compatibility.

// ============================================================================
// Stale Marking Tests (for Incremental Ontology Refresh)
// ============================================================================

func TestOntologyEntityRepository_MarkInferenceEntitiesStale_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entities with different provenance
	inferenceEntity1 := tc.createTestEntityWithProvenance(ctx, "inference1", models.ProvenanceInference)
	inferenceEntity2 := tc.createTestEntityWithProvenance(ctx, "inference2", models.ProvenanceInference)
	manualEntity := tc.createTestEntityWithProvenance(ctx, "manual", models.ProvenanceManual)
	mcpEntity := tc.createTestEntityWithProvenance(ctx, "mcp", models.ProvenanceMCP)

	// Mark inference entities as stale
	err := tc.repo.MarkInferenceEntitiesStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceEntitiesStale failed: %v", err)
	}

	// Verify inference entities are stale
	staleEntities, err := tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetStaleEntities failed: %v", err)
	}
	if len(staleEntities) != 2 {
		t.Errorf("expected 2 stale entities, got %d", len(staleEntities))
	}

	// Verify specific entities
	staleIDs := make(map[uuid.UUID]bool)
	for _, e := range staleEntities {
		staleIDs[e.ID] = true
	}
	if !staleIDs[inferenceEntity1.ID] {
		t.Error("expected inference1 to be stale")
	}
	if !staleIDs[inferenceEntity2.ID] {
		t.Error("expected inference2 to be stale")
	}

	// Verify manual and MCP entities are NOT stale
	retrievedManual, _ := tc.repo.GetByID(ctx, manualEntity.ID)
	if retrievedManual.IsStale {
		t.Error("manual entity should not be stale")
	}
	retrievedMCP, _ := tc.repo.GetByID(ctx, mcpEntity.ID)
	if retrievedMCP.IsStale {
		t.Error("MCP entity should not be stale")
	}
}

func TestOntologyEntityRepository_MarkInferenceEntitiesStale_SkipsDeleted(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an inference entity and soft-delete it
	deletedEntity := tc.createTestEntityWithProvenance(ctx, "deleted_inference", models.ProvenanceInference)
	err := tc.repo.SoftDelete(ctx, deletedEntity.ID, "test deletion")
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// Create an active inference entity
	tc.createTestEntityWithProvenance(ctx, "active_inference", models.ProvenanceInference)

	// Mark inference entities as stale
	err = tc.repo.MarkInferenceEntitiesStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceEntitiesStale failed: %v", err)
	}

	// Only non-deleted entity should be stale
	staleEntities, err := tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetStaleEntities failed: %v", err)
	}
	if len(staleEntities) != 1 {
		t.Errorf("expected 1 stale entity, got %d", len(staleEntities))
	}
}

func TestOntologyEntityRepository_ClearStaleFlag_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create and mark an entity as stale
	entity := tc.createTestEntityWithProvenance(ctx, "inference", models.ProvenanceInference)
	err := tc.repo.MarkInferenceEntitiesStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceEntitiesStale failed: %v", err)
	}

	// Verify it's stale
	staleEntities, _ := tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if len(staleEntities) != 1 {
		t.Fatal("expected entity to be stale before clearing")
	}

	// Clear stale flag
	err = tc.repo.ClearStaleFlag(ctx, entity.ID)
	if err != nil {
		t.Fatalf("ClearStaleFlag failed: %v", err)
	}

	// Verify it's no longer stale
	staleEntities, _ = tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if len(staleEntities) != 0 {
		t.Errorf("expected 0 stale entities after clearing, got %d", len(staleEntities))
	}
}

func TestOntologyEntityRepository_GetStaleEntities_Empty(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entities but don't mark any as stale
	tc.createTestEntityWithProvenance(ctx, "entity1", models.ProvenanceInference)
	tc.createTestEntityWithProvenance(ctx, "entity2", models.ProvenanceManual)

	staleEntities, err := tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetStaleEntities failed: %v", err)
	}
	if len(staleEntities) != 0 {
		t.Errorf("expected 0 stale entities, got %d", len(staleEntities))
	}
}

func TestOntologyEntityRepository_Create_ClearsStaleOnRediscovery(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an inference entity
	original := tc.createTestEntityWithProvenance(ctx, "user", models.ProvenanceInference)

	// Mark it as stale (simulating start of ontology refresh)
	err := tc.repo.MarkInferenceEntitiesStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceEntitiesStale failed: %v", err)
	}

	// Verify it's stale
	staleEntities, _ := tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if len(staleEntities) != 1 {
		t.Fatal("expected entity to be stale")
	}

	// Re-create the entity (simulating re-discovery during refresh)
	recreated := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "", // Empty description simulates DDL discovery
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
		IsStale:       false, // New entities are not stale
	}
	err = tc.repo.Create(ctx, recreated)
	if err != nil {
		t.Fatalf("Create (rediscovery) failed: %v", err)
	}

	// Verify the entity ID was preserved (upsert)
	if recreated.ID != original.ID {
		t.Errorf("expected ID to be preserved on upsert, got different IDs")
	}

	// Verify stale flag is cleared
	staleEntities, _ = tc.repo.GetStaleEntities(ctx, tc.ontologyID)
	if len(staleEntities) != 0 {
		t.Errorf("expected stale flag to be cleared on rediscovery, got %d stale entities", len(staleEntities))
	}

	// Verify the entity is still accessible
	retrieved, err := tc.repo.GetByName(ctx, tc.ontologyID, "user")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected entity to exist after rediscovery")
	}
	if retrieved.IsStale {
		t.Error("expected IsStale to be false after rediscovery")
	}
}

// ============================================================================
// Provenance Context Tests
// ============================================================================

func TestOntologyEntityRepository_Create_RequiresProvenanceContext(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	// Create context WITHOUT provenance
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)
	// Note: NOT adding provenance context

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "Test user",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	err = tc.repo.Create(ctx, entity)
	if err == nil {
		t.Error("expected error when creating entity without provenance context")
	}
	if err != nil && err.Error() != "provenance context required" {
		t.Errorf("expected 'provenance context required' error, got: %v", err)
	}
}

func TestOntologyEntityRepository_Create_SetsProvenanceFields(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContextWithProvenance(models.SourceManual)
	defer cleanup()

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "Test user",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	err := tc.repo.Create(ctx, entity)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify provenance fields were set on entity
	if entity.Source != "manual" {
		t.Errorf("expected Source to be 'manual', got %q", entity.Source)
	}
	if entity.CreatedBy == nil {
		t.Error("expected CreatedBy to be set")
	}
	if entity.CreatedBy != nil && *entity.CreatedBy != tc.testUserID {
		t.Errorf("expected CreatedBy to be %v, got %v", tc.testUserID, *entity.CreatedBy)
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Source != "manual" {
		t.Errorf("expected Source to be 'manual', got %q", retrieved.Source)
	}
	if retrieved.CreatedBy == nil || *retrieved.CreatedBy != tc.testUserID {
		t.Errorf("expected CreatedBy to be %v, got %v", tc.testUserID, retrieved.CreatedBy)
	}
}

func TestOntologyEntityRepository_Update_RequiresProvenanceContext(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	// Create entity first with proper context
	ctx, cleanup := tc.createTestContext()
	defer cleanup()
	entity := tc.createTestEntity(ctx, "user")

	// Now try to update WITHOUT provenance context
	ctxWithoutProv := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctxWithoutProv)
	if err != nil {
		t.Fatalf("failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctxWithoutProv = database.SetTenantScope(ctxWithoutProv, scope)
	// Note: NOT adding provenance context

	entity.Description = "Updated description"
	err = tc.repo.Update(ctxWithoutProv, entity)
	if err == nil {
		t.Error("expected error when updating entity without provenance context")
	}
	if err != nil && err.Error() != "provenance context required" {
		t.Errorf("expected 'provenance context required' error, got: %v", err)
	}
}

func TestOntologyEntityRepository_Update_SetsProvenanceFields(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	// Create entity with inference provenance
	ctx, cleanup := tc.createTestContextWithProvenance(models.SourceInference)
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Update with manual provenance
	ctxManual := models.WithProvenance(ctx, models.ProvenanceContext{
		Source: models.SourceManual,
		UserID: tc.testUserID,
	})

	entity.Description = "Updated by user"
	err := tc.repo.Update(ctxManual, entity)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify provenance fields were set
	if entity.LastEditSource == nil || *entity.LastEditSource != "manual" {
		t.Errorf("expected LastEditSource to be 'manual', got %v", entity.LastEditSource)
	}
	if entity.UpdatedBy == nil || *entity.UpdatedBy != tc.testUserID {
		t.Errorf("expected UpdatedBy to be %v, got %v", tc.testUserID, entity.UpdatedBy)
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByID(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	// Source should still be inference (from creation)
	if retrieved.Source != "inference" {
		t.Errorf("expected Source to still be 'inference', got %q", retrieved.Source)
	}
	// LastEditSource should be manual
	if retrieved.LastEditSource == nil || *retrieved.LastEditSource != "manual" {
		t.Errorf("expected LastEditSource to be 'manual', got %v", retrieved.LastEditSource)
	}
	if retrieved.UpdatedBy == nil || *retrieved.UpdatedBy != tc.testUserID {
		t.Errorf("expected UpdatedBy to be %v, got %v", tc.testUserID, retrieved.UpdatedBy)
	}
}

func TestOntologyEntityRepository_DeleteBySource_Success(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entities with different sources
	tc.createTestEntityWithProvenance(ctx, "inference_entity1", models.ProvenanceInference)
	tc.createTestEntityWithProvenance(ctx, "inference_entity2", models.ProvenanceInference)
	tc.createTestEntityWithProvenance(ctx, "manual_entity", models.ProvenanceManual)
	tc.createTestEntityWithProvenance(ctx, "mcp_entity", models.ProvenanceMCP)

	// Delete inference entities by source
	err := tc.repo.DeleteBySource(ctx, tc.projectID, models.SourceInference)
	if err != nil {
		t.Fatalf("DeleteBySource failed: %v", err)
	}

	// Verify only manual and MCP entities remain
	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities (manual + mcp), got %d", len(entities))
	}

	// Verify the correct entities remain
	entityNames := make(map[string]bool)
	for _, e := range entities {
		entityNames[e.Name] = true
	}
	if !entityNames["manual_entity"] {
		t.Error("expected manual_entity to be preserved")
	}
	if !entityNames["mcp_entity"] {
		t.Error("expected mcp_entity to be preserved")
	}
}

func TestOntologyEntityRepository_DeleteBySource_NoMatchingEntities(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create only manual entities
	tc.createTestEntityWithProvenance(ctx, "manual_entity1", models.ProvenanceManual)
	tc.createTestEntityWithProvenance(ctx, "manual_entity2", models.ProvenanceManual)

	// Delete inference entities (none exist) - should not error
	err := tc.repo.DeleteBySource(ctx, tc.projectID, models.SourceInference)
	if err != nil {
		t.Fatalf("DeleteBySource failed: %v", err)
	}

	// All manual entities should remain
	entities, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(entities))
	}
}

func TestOntologyEntityRepository_DeleteBySource_NoTenantScope(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	err := tc.repo.DeleteBySource(ctx, tc.projectID, models.SourceInference)
	if err == nil {
		t.Error("expected error for DeleteBySource without tenant scope")
	}
}

func TestOntologyEntityRepository_Create_UpsertSetsLastEditSourceOnConflict(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	// Create entity with inference provenance
	ctxInference, cleanup := tc.createTestContextWithProvenance(models.SourceInference)
	defer cleanup()

	original := tc.createTestEntity(ctxInference, "user")

	// Upsert with manual provenance
	ctxManual := models.WithProvenance(ctxInference, models.ProvenanceContext{
		Source: models.SourceManual,
		UserID: tc.testUserID,
	})

	updated := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "Updated description",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	err := tc.repo.Create(ctxManual, updated)
	if err != nil {
		t.Fatalf("Create (upsert) failed: %v", err)
	}

	// Verify the entity was upserted (same ID)
	if updated.ID != original.ID {
		t.Errorf("expected ID to be preserved on upsert, got different IDs")
	}

	// Verify last_edit_source and updated_by were set on upsert
	retrieved, err := tc.repo.GetByID(ctxInference, updated.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	// Source should still be inference (from original creation)
	if retrieved.Source != "inference" {
		t.Errorf("expected Source to still be 'inference', got %q", retrieved.Source)
	}
	// LastEditSource should be manual (from upsert)
	if retrieved.LastEditSource == nil || *retrieved.LastEditSource != "manual" {
		t.Errorf("expected LastEditSource to be 'manual', got %v", retrieved.LastEditSource)
	}
	// UpdatedBy should be set
	if retrieved.UpdatedBy == nil || *retrieved.UpdatedBy != tc.testUserID {
		t.Errorf("expected UpdatedBy to be %v, got %v", tc.testUserID, retrieved.UpdatedBy)
	}
}

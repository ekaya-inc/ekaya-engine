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

// createTestContext returns a context with tenant scope.
func (tc *ontologyEntityTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestEntity creates a schema entity for testing.
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

func (tc *ontologyEntityTestContext) createTestOccurrence(ctx context.Context, entityID uuid.UUID, tableName, columnName string) {
	tc.t.Helper()
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		tc.t.Fatal("failed to get tenant scope from context")
	}
	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_entity_occurrences (id, entity_id, schema_name, table_name, column_name, role, is_deleted)
		VALUES (gen_random_uuid(), $1, 'public', $2, $3, 'identifier', false)
	`, entityID, tableName, columnName)
	if err != nil {
		tc.t.Fatalf("failed to create test occurrence: %v", err)
	}
}

func (tc *ontologyEntityTestContext) createTestOccurrenceDeleted(ctx context.Context, entityID uuid.UUID, tableName, columnName string) {
	tc.t.Helper()
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		tc.t.Fatal("failed to get tenant scope from context")
	}
	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_entity_occurrences (id, entity_id, schema_name, table_name, column_name, role, is_deleted)
		VALUES (gen_random_uuid(), $1, 'public', $2, $3, 'identifier', true)
	`, entityID, tableName, columnName)
	if err != nil {
		tc.t.Fatalf("failed to create test occurrence (deleted): %v", err)
	}
}

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

func TestOntologyEntityRepository_Create_DuplicateName(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "account")

	// Try to create another with same name
	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "account",
		Description:   "Duplicate account",
		PrimarySchema: "public",
		PrimaryTable:  "accounts",
		PrimaryColumn: "id",
	}

	err := tc.repo.Create(ctx, entity)
	if err == nil {
		t.Error("expected error for duplicate entity name")
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
// CountOccurrencesByEntity Tests
// ============================================================================

func TestOntologyEntityRepository_CountOccurrencesByEntity_NoOccurrences(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Entity with no occurrences should return 0
	count, err := tc.repo.CountOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("CountOccurrencesByEntity failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 occurrences, got %d", count)
	}
}

func TestOntologyEntityRepository_CountOccurrencesByEntity_WithOccurrences(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create some occurrences for this entity
	tc.createTestOccurrence(ctx, entity.ID, "users", "id")
	tc.createTestOccurrence(ctx, entity.ID, "accounts", "owner_id")
	tc.createTestOccurrence(ctx, entity.ID, "orders", "customer_id")

	count, err := tc.repo.CountOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("CountOccurrencesByEntity failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 occurrences, got %d", count)
	}
}

func TestOntologyEntityRepository_CountOccurrencesByEntity_ExcludesDeleted(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create occurrences - one active, one deleted
	tc.createTestOccurrence(ctx, entity.ID, "users", "id")
	tc.createTestOccurrenceDeleted(ctx, entity.ID, "accounts", "owner_id")

	count, err := tc.repo.CountOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("CountOccurrencesByEntity failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence (excluding deleted), got %d", count)
	}
}

// ============================================================================
// GetOccurrenceTablesByEntity Tests
// ============================================================================

func TestOntologyEntityRepository_GetOccurrenceTablesByEntity_NoOccurrences(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	tables, err := tc.repo.GetOccurrenceTablesByEntity(ctx, entity.ID, 10)
	if err != nil {
		t.Fatalf("GetOccurrenceTablesByEntity failed: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(tables))
	}
}

func TestOntologyEntityRepository_GetOccurrenceTablesByEntity_WithOccurrences(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create occurrences in different tables
	tc.createTestOccurrence(ctx, entity.ID, "users", "id")
	tc.createTestOccurrence(ctx, entity.ID, "accounts", "owner_id")
	tc.createTestOccurrence(ctx, entity.ID, "orders", "customer_id")

	tables, err := tc.repo.GetOccurrenceTablesByEntity(ctx, entity.ID, 10)
	if err != nil {
		t.Fatalf("GetOccurrenceTablesByEntity failed: %v", err)
	}
	if len(tables) != 3 {
		t.Errorf("expected 3 tables, got %d", len(tables))
	}
}

func TestOntologyEntityRepository_GetOccurrenceTablesByEntity_RespectsLimit(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create occurrences in different tables
	tc.createTestOccurrence(ctx, entity.ID, "users", "id")
	tc.createTestOccurrence(ctx, entity.ID, "accounts", "owner_id")
	tc.createTestOccurrence(ctx, entity.ID, "orders", "customer_id")
	tc.createTestOccurrence(ctx, entity.ID, "invoices", "customer_id")

	tables, err := tc.repo.GetOccurrenceTablesByEntity(ctx, entity.ID, 2)
	if err != nil {
		t.Fatalf("GetOccurrenceTablesByEntity failed: %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("expected 2 tables (limited), got %d", len(tables))
	}
}

func TestOntologyEntityRepository_GetOccurrenceTablesByEntity_ExcludesDeleted(t *testing.T) {
	tc := setupOntologyEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create occurrences - one active, one deleted
	tc.createTestOccurrence(ctx, entity.ID, "users", "id")
	tc.createTestOccurrenceDeleted(ctx, entity.ID, "accounts", "owner_id")

	tables, err := tc.repo.GetOccurrenceTablesByEntity(ctx, entity.ID, 10)
	if err != nil {
		t.Fatalf("GetOccurrenceTablesByEntity failed: %v", err)
	}
	if len(tables) != 1 {
		t.Errorf("expected 1 table (excluding deleted), got %d", len(tables))
	}
	if tables[0] != "users" {
		t.Errorf("expected table 'users', got %q", tables[0])
	}
}

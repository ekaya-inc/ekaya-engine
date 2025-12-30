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

// schemaEntityTestContext holds test dependencies for schema entity repository tests.
type schemaEntityTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       SchemaEntityRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
}

// setupSchemaEntityTest initializes the test context with shared testcontainer.
func setupSchemaEntityTest(t *testing.T) *schemaEntityTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &schemaEntityTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewSchemaEntityRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000050"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000051"),
	}
	tc.ensureTestProjectAndOntology()
	return tc
}

// ensureTestProjectAndOntology creates the test project and ontology if they don't exist.
func (tc *schemaEntityTestContext) ensureTestProjectAndOntology() {
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
	`, tc.projectID, "Schema Entity Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Create ontology
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}
}

// cleanup removes test entities and occurrences.
func (tc *schemaEntityTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Cascading delete will handle occurrences
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_entities WHERE ontology_id = $1", tc.ontologyID)
}

// createTestContext returns a context with tenant scope.
func (tc *schemaEntityTestContext) createTestContext() (context.Context, func()) {
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
func (tc *schemaEntityTestContext) createTestEntity(ctx context.Context, name string) *models.SchemaEntity {
	tc.t.Helper()
	entity := &models.SchemaEntity{
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

// ============================================================================
// Entity Create Tests
// ============================================================================

func TestSchemaEntityRepository_Create_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := &models.SchemaEntity{
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

func TestSchemaEntityRepository_Create_DuplicateName(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestEntity(ctx, "account")

	// Try to create another with same name
	entity := &models.SchemaEntity{
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

func TestSchemaEntityRepository_GetByOntology_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
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

func TestSchemaEntityRepository_GetByOntology_Empty(t *testing.T) {
	tc := setupSchemaEntityTest(t)
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

func TestSchemaEntityRepository_GetByName_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
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

func TestSchemaEntityRepository_GetByName_NotFound(t *testing.T) {
	tc := setupSchemaEntityTest(t)
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
// Entity DeleteByOntology Tests
// ============================================================================

func TestSchemaEntityRepository_DeleteByOntology_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
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
// Occurrence CreateOccurrence Tests
// ============================================================================

func TestSchemaEntityRepository_CreateOccurrence_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	role := "visitor"
	occ := &models.SchemaEntityOccurrence{
		EntityID:   entity.ID,
		SchemaName: "public",
		TableName:  "visits",
		ColumnName: "visitor_id",
		Role:       &role,
		Confidence: 0.95,
	}

	err := tc.repo.CreateOccurrence(ctx, occ)
	if err != nil {
		t.Fatalf("CreateOccurrence failed: %v", err)
	}

	if occ.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if occ.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching
	occurrences, err := tc.repo.GetOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetOccurrencesByEntity failed: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occurrences))
	}
	if occurrences[0].ColumnName != "visitor_id" {
		t.Errorf("expected column_name 'visitor_id', got %q", occurrences[0].ColumnName)
	}
	if occurrences[0].Role == nil || *occurrences[0].Role != "visitor" {
		t.Errorf("expected role 'visitor', got %v", occurrences[0].Role)
	}
	if occurrences[0].Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", occurrences[0].Confidence)
	}
}

func TestSchemaEntityRepository_CreateOccurrence_NullRole(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	occ := &models.SchemaEntityOccurrence{
		EntityID:   entity.ID,
		SchemaName: "public",
		TableName:  "orders",
		ColumnName: "user_id",
		Role:       nil, // No role
		Confidence: 1.0,
	}

	err := tc.repo.CreateOccurrence(ctx, occ)
	if err != nil {
		t.Fatalf("CreateOccurrence failed: %v", err)
	}

	occurrences, err := tc.repo.GetOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetOccurrencesByEntity failed: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(occurrences))
	}
	if occurrences[0].Role != nil {
		t.Errorf("expected nil role, got %v", occurrences[0].Role)
	}
}

func TestSchemaEntityRepository_CreateOccurrence_Duplicate(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	occ := &models.SchemaEntityOccurrence{
		EntityID:   entity.ID,
		SchemaName: "public",
		TableName:  "orders",
		ColumnName: "user_id",
		Confidence: 1.0,
	}

	err := tc.repo.CreateOccurrence(ctx, occ)
	if err != nil {
		t.Fatalf("CreateOccurrence failed: %v", err)
	}

	// Try to create duplicate
	occ2 := &models.SchemaEntityOccurrence{
		EntityID:   entity.ID,
		SchemaName: "public",
		TableName:  "orders",
		ColumnName: "user_id",
		Confidence: 0.8,
	}

	err = tc.repo.CreateOccurrence(ctx, occ2)
	if err == nil {
		t.Error("expected error for duplicate occurrence")
	}
}

// ============================================================================
// Occurrence GetOccurrencesByEntity Tests
// ============================================================================

func TestSchemaEntityRepository_GetOccurrencesByEntity_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	visitor := "visitor"
	host := "host"

	// Create multiple occurrences
	occurrences := []*models.SchemaEntityOccurrence{
		{
			EntityID:   entity.ID,
			SchemaName: "public",
			TableName:  "orders",
			ColumnName: "user_id",
			Confidence: 1.0,
		},
		{
			EntityID:   entity.ID,
			SchemaName: "public",
			TableName:  "visits",
			ColumnName: "visitor_id",
			Role:       &visitor,
			Confidence: 0.95,
		},
		{
			EntityID:   entity.ID,
			SchemaName: "public",
			TableName:  "visits",
			ColumnName: "host_id",
			Role:       &host,
			Confidence: 0.90,
		},
	}

	for _, occ := range occurrences {
		err := tc.repo.CreateOccurrence(ctx, occ)
		if err != nil {
			t.Fatalf("CreateOccurrence failed: %v", err)
		}
	}

	retrieved, err := tc.repo.GetOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetOccurrencesByEntity failed: %v", err)
	}
	if len(retrieved) != 3 {
		t.Fatalf("expected 3 occurrences, got %d", len(retrieved))
	}

	// Should be sorted by schema, table, column
	if retrieved[0].TableName != "orders" {
		t.Errorf("expected first occurrence from orders table, got %s", retrieved[0].TableName)
	}
}

func TestSchemaEntityRepository_GetOccurrencesByEntity_Empty(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	occurrences, err := tc.repo.GetOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetOccurrencesByEntity failed: %v", err)
	}
	if len(occurrences) != 0 {
		t.Errorf("expected 0 occurrences, got %d", len(occurrences))
	}
}

// ============================================================================
// Occurrence GetOccurrencesByTable Tests
// ============================================================================

func TestSchemaEntityRepository_GetOccurrencesByTable_Success(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	user := tc.createTestEntity(ctx, "user")
	account := tc.createTestEntity(ctx, "account")

	visitor := "visitor"
	host := "host"

	// Create occurrences for visits table
	occurrences := []*models.SchemaEntityOccurrence{
		{
			EntityID:   user.ID,
			SchemaName: "public",
			TableName:  "visits",
			ColumnName: "visitor_id",
			Role:       &visitor,
			Confidence: 0.95,
		},
		{
			EntityID:   user.ID,
			SchemaName: "public",
			TableName:  "visits",
			ColumnName: "host_id",
			Role:       &host,
			Confidence: 0.90,
		},
		{
			EntityID:   account.ID,
			SchemaName: "public",
			TableName:  "visits",
			ColumnName: "account_id",
			Confidence: 1.0,
		},
		// Occurrence in different table (should not be returned)
		{
			EntityID:   user.ID,
			SchemaName: "public",
			TableName:  "orders",
			ColumnName: "user_id",
			Confidence: 1.0,
		},
	}

	for _, occ := range occurrences {
		err := tc.repo.CreateOccurrence(ctx, occ)
		if err != nil {
			t.Fatalf("CreateOccurrence failed: %v", err)
		}
	}

	retrieved, err := tc.repo.GetOccurrencesByTable(ctx, tc.ontologyID, "public", "visits")
	if err != nil {
		t.Fatalf("GetOccurrencesByTable failed: %v", err)
	}
	if len(retrieved) != 3 {
		t.Fatalf("expected 3 occurrences for visits table, got %d", len(retrieved))
	}

	// Should be sorted by column name
	if retrieved[0].ColumnName != "account_id" {
		t.Errorf("expected first occurrence to be account_id, got %s", retrieved[0].ColumnName)
	}
	if retrieved[1].ColumnName != "host_id" {
		t.Errorf("expected second occurrence to be host_id, got %s", retrieved[1].ColumnName)
	}
	if retrieved[2].ColumnName != "visitor_id" {
		t.Errorf("expected third occurrence to be visitor_id, got %s", retrieved[2].ColumnName)
	}
}

func TestSchemaEntityRepository_GetOccurrencesByTable_Empty(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	occurrences, err := tc.repo.GetOccurrencesByTable(ctx, tc.ontologyID, "public", "nonexistent")
	if err != nil {
		t.Fatalf("GetOccurrencesByTable failed: %v", err)
	}
	if len(occurrences) != 0 {
		t.Errorf("expected 0 occurrences, got %d", len(occurrences))
	}
}

// ============================================================================
// Cascade Delete Tests
// ============================================================================

func TestSchemaEntityRepository_CascadeDelete_Occurrences(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entity := tc.createTestEntity(ctx, "user")

	// Create occurrence
	occ := &models.SchemaEntityOccurrence{
		EntityID:   entity.ID,
		SchemaName: "public",
		TableName:  "orders",
		ColumnName: "user_id",
		Confidence: 1.0,
	}
	err := tc.repo.CreateOccurrence(ctx, occ)
	if err != nil {
		t.Fatalf("CreateOccurrence failed: %v", err)
	}

	// Delete entity should cascade to occurrence
	err = tc.repo.DeleteByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("DeleteByOntology failed: %v", err)
	}

	// Verify occurrences are deleted
	occurrences, err := tc.repo.GetOccurrencesByEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetOccurrencesByEntity failed: %v", err)
	}
	if len(occurrences) != 0 {
		t.Errorf("expected 0 occurrences after entity delete, got %d", len(occurrences))
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestSchemaEntityRepository_NoTenantScope(t *testing.T) {
	tc := setupSchemaEntityTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	entity := &models.SchemaEntity{
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

	// CreateOccurrence should fail
	occ := &models.SchemaEntityOccurrence{
		EntityID:   uuid.New(),
		SchemaName: "public",
		TableName:  "test",
		ColumnName: "id",
		Confidence: 1.0,
	}
	err = tc.repo.CreateOccurrence(ctx, occ)
	if err == nil {
		t.Error("expected error for CreateOccurrence without tenant scope")
	}

	// GetOccurrencesByEntity should fail
	_, err = tc.repo.GetOccurrencesByEntity(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for GetOccurrencesByEntity without tenant scope")
	}

	// GetOccurrencesByTable should fail
	_, err = tc.repo.GetOccurrencesByTable(ctx, tc.ontologyID, "public", "test")
	if err == nil {
		t.Error("expected error for GetOccurrencesByTable without tenant scope")
	}
}

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

// relationshipTestContext holds test dependencies for relationship repository tests.
type relationshipTestContext struct {
	t              *testing.T
	engineDB       *testhelpers.EngineDB
	repo           EntityRelationshipRepository
	entityRepo     OntologyEntityRepository
	projectID      uuid.UUID
	ontologyID     uuid.UUID
	sourceEntityID uuid.UUID
	targetEntityID uuid.UUID
}

// setupRelationshipTest initializes the test context with shared testcontainer.
func setupRelationshipTest(t *testing.T) *relationshipTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &relationshipTestContext{
		t:              t,
		engineDB:       engineDB,
		repo:           NewEntityRelationshipRepository(),
		entityRepo:     NewOntologyEntityRepository(),
		projectID:      uuid.MustParse("00000000-0000-0000-0000-000000000060"),
		ontologyID:     uuid.MustParse("00000000-0000-0000-0000-000000000061"),
		sourceEntityID: uuid.MustParse("00000000-0000-0000-0000-000000000062"),
		targetEntityID: uuid.MustParse("00000000-0000-0000-0000-000000000063"),
	}
	tc.ensureTestProjectAndOntologyAndEntities()
	return tc
}

// ensureTestProjectAndOntologyAndEntities creates test fixtures.
func (tc *relationshipTestContext) ensureTestProjectAndOntologyAndEntities() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope: %v", err)
	}
	defer scope.Close()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Relationship Repository Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Clear existing data
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_workflows WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)

	// Create ontology
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create test ontology: %v", err)
	}

	// Create source entity
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_entities (id, project_id, ontology_id, name, description, primary_schema, primary_table, primary_column, created_by)
		VALUES ($1, $2, $3, 'Order', 'An order entity', 'public', 'orders', 'id', 'inference')
	`, tc.sourceEntityID, tc.projectID, tc.ontologyID)
	if err != nil {
		tc.t.Fatalf("failed to create source entity: %v", err)
	}

	// Create target entity
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_entities (id, project_id, ontology_id, name, description, primary_schema, primary_table, primary_column, created_by)
		VALUES ($1, $2, $3, 'User', 'A user entity', 'public', 'users', 'id', 'inference')
	`, tc.targetEntityID, tc.projectID, tc.ontologyID)
	if err != nil {
		tc.t.Fatalf("failed to create target entity: %v", err)
	}
}

// cleanup removes test relationships.
func (tc *relationshipTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_entity_relationships WHERE ontology_id = $1", tc.ontologyID)
}

// createTestContext returns a context with tenant scope.
func (tc *relationshipTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestRelationship creates a test relationship with the specified provenance.
func (tc *relationshipTestContext) createTestRelationship(ctx context.Context, sourceCol, targetCol, createdBy string) *models.EntityRelationship {
	tc.t.Helper()
	rel := &models.EntityRelationship{
		OntologyID:         tc.ontologyID,
		SourceEntityID:     tc.sourceEntityID,
		TargetEntityID:     tc.targetEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   sourceCol,
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   targetCol,
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        models.CardinalityNTo1,
		CreatedBy:          createdBy,
	}
	err := tc.repo.Create(ctx, rel)
	if err != nil {
		tc.t.Fatalf("failed to create test relationship: %v", err)
	}
	return rel
}

// ============================================================================
// Stale Marking Tests (for Incremental Ontology Refresh)
// ============================================================================

func TestEntityRelationshipRepository_MarkInferenceRelationshipsStale_Success(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup() // Clean up at end to avoid affecting other tests

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create relationships with different provenance
	tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)
	tc.createTestRelationship(ctx, "created_by", "id", models.ProvenanceInference)
	tc.createTestRelationship(ctx, "owner_id", "id", models.ProvenanceManual)
	tc.createTestRelationship(ctx, "reviewer_id", "id", models.ProvenanceMCP)

	// Mark inference relationships as stale
	err := tc.repo.MarkInferenceRelationshipsStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceRelationshipsStale failed: %v", err)
	}

	// Verify inference relationships are stale
	staleRels, err := tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetStaleRelationships failed: %v", err)
	}
	if len(staleRels) != 2 {
		t.Errorf("expected 2 stale relationships, got %d", len(staleRels))
	}

	// Verify manual and MCP relationships are NOT stale
	allRels, _ := tc.repo.GetByOntology(ctx, tc.ontologyID)
	for _, rel := range allRels {
		if rel.CreatedBy == models.ProvenanceManual || rel.CreatedBy == models.ProvenanceMCP {
			if rel.IsStale {
				t.Errorf("%s relationship should not be stale", rel.CreatedBy)
			}
		}
	}
}

func TestEntityRelationshipRepository_ClearStaleFlag_Success(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create and mark a relationship as stale
	rel := tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)
	err := tc.repo.MarkInferenceRelationshipsStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceRelationshipsStale failed: %v", err)
	}

	// Verify it's stale
	staleRels, _ := tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 1 {
		t.Fatal("expected relationship to be stale before clearing")
	}

	// Clear stale flag
	err = tc.repo.ClearStaleFlag(ctx, rel.ID)
	if err != nil {
		t.Fatalf("ClearStaleFlag failed: %v", err)
	}

	// Verify it's no longer stale
	staleRels, _ = tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 0 {
		t.Errorf("expected 0 stale relationships after clearing, got %d", len(staleRels))
	}
}

func TestEntityRelationshipRepository_GetStaleRelationships_Empty(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create relationships but don't mark any as stale
	tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)
	tc.createTestRelationship(ctx, "owner_id", "id", models.ProvenanceManual)

	staleRels, err := tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetStaleRelationships failed: %v", err)
	}
	if len(staleRels) != 0 {
		t.Errorf("expected 0 stale relationships, got %d", len(staleRels))
	}
}

func TestEntityRelationshipRepository_Create_ClearsStaleOnRediscovery(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create an inference relationship
	original := tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)

	// Mark it as stale (simulating start of ontology refresh)
	err := tc.repo.MarkInferenceRelationshipsStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceRelationshipsStale failed: %v", err)
	}

	// Verify it's stale
	staleRels, _ := tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 1 {
		t.Fatal("expected relationship to be stale")
	}

	// Re-create the same relationship (simulating re-discovery during FK discovery)
	recreated := &models.EntityRelationship{
		OntologyID:         tc.ontologyID,
		SourceEntityID:     tc.sourceEntityID,
		TargetEntityID:     tc.targetEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   "user_id",
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   "id",
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        models.CardinalityNTo1,
		CreatedBy:          models.ProvenanceInference,
		IsStale:            false,
	}
	err = tc.repo.Create(ctx, recreated)
	if err != nil {
		t.Fatalf("Create (rediscovery) failed: %v", err)
	}

	// Verify stale flag is cleared
	staleRels, _ = tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 0 {
		t.Errorf("expected stale flag to be cleared on rediscovery, got %d stale relationships", len(staleRels))
	}

	// Verify the relationship still exists (check by fetching all)
	allRels, err := tc.repo.GetByOntology(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("GetByOntology failed: %v", err)
	}
	if len(allRels) != 1 {
		t.Errorf("expected 1 relationship (upserted), got %d", len(allRels))
	}

	// Verify the original ID was preserved (since it was an upsert on existing)
	_ = original // Note: IDs may differ due to ON CONFLICT behavior - what matters is is_stale is cleared
	if allRels[0].IsStale {
		t.Error("expected IsStale to be false after rediscovery")
	}
}

func TestEntityRelationshipRepository_UpdateDescription_ClearsStaleFlag(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a relationship and mark it stale
	rel := tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)
	err := tc.repo.MarkInferenceRelationshipsStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceRelationshipsStale failed: %v", err)
	}

	// Verify it's stale
	staleRels, _ := tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 1 {
		t.Fatal("expected relationship to be stale")
	}

	// Update description (simulating enrichment)
	err = tc.repo.UpdateDescription(ctx, rel.ID, "Order is placed by a User")
	if err != nil {
		t.Fatalf("UpdateDescription failed: %v", err)
	}

	// Verify stale flag is cleared
	staleRels, _ = tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 0 {
		t.Errorf("expected UpdateDescription to clear stale flag, got %d stale", len(staleRels))
	}

	// Verify description was set
	retrieved, _ := tc.repo.GetByID(ctx, rel.ID)
	if retrieved.Description == nil || *retrieved.Description != "Order is placed by a User" {
		t.Errorf("expected description to be set, got %v", retrieved.Description)
	}
}

func TestEntityRelationshipRepository_UpdateDescriptionAndAssociation_ClearsStaleFlag(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a relationship and mark it stale
	rel := tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)
	err := tc.repo.MarkInferenceRelationshipsStale(ctx, tc.ontologyID)
	if err != nil {
		t.Fatalf("MarkInferenceRelationshipsStale failed: %v", err)
	}

	// Update with description and association (simulating enrichment)
	err = tc.repo.UpdateDescriptionAndAssociation(ctx, rel.ID, "Order is placed by a User", "placed_by")
	if err != nil {
		t.Fatalf("UpdateDescriptionAndAssociation failed: %v", err)
	}

	// Verify stale flag is cleared
	staleRels, _ := tc.repo.GetStaleRelationships(ctx, tc.ontologyID)
	if len(staleRels) != 0 {
		t.Errorf("expected UpdateDescriptionAndAssociation to clear stale flag, got %d stale", len(staleRels))
	}

	// Verify description and association were set
	retrieved, _ := tc.repo.GetByID(ctx, rel.ID)
	if retrieved.Description == nil || *retrieved.Description != "Order is placed by a User" {
		t.Errorf("expected description to be set, got %v", retrieved.Description)
	}
	if retrieved.Association == nil || *retrieved.Association != "placed_by" {
		t.Errorf("expected association to be set, got %v", retrieved.Association)
	}
}

// ============================================================================
// GetByProject Column Types Tests
// ============================================================================

// columnTypesTestContext holds dependencies for column types tests.
type columnTypesTestContext struct {
	*relationshipTestContext
	dsID           uuid.UUID
	sourceTableID  uuid.UUID
	targetTableID  uuid.UUID
	sourceColumnID uuid.UUID
	targetColumnID uuid.UUID
}

// setupColumnTypesTest creates test fixtures including schema columns.
func setupColumnTypesTest(t *testing.T) *columnTypesTestContext {
	tc := setupRelationshipTest(t)
	ctc := &columnTypesTestContext{
		relationshipTestContext: tc,
		dsID:                    uuid.MustParse("00000000-0000-0000-0000-000000000070"),
		sourceTableID:           uuid.MustParse("00000000-0000-0000-0000-000000000071"),
		targetTableID:           uuid.MustParse("00000000-0000-0000-0000-000000000072"),
		sourceColumnID:          uuid.MustParse("00000000-0000-0000-0000-000000000073"),
		targetColumnID:          uuid.MustParse("00000000-0000-0000-0000-000000000074"),
	}
	ctc.ensureSchemaColumns()
	return ctc
}

// ensureSchemaColumns creates datasource, tables, and columns for testing.
func (ctc *columnTypesTestContext) ensureSchemaColumns() {
	ctc.t.Helper()
	ctx := context.Background()
	scope, err := ctc.engineDB.DB.WithTenant(ctx, ctc.projectID)
	if err != nil {
		ctc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	// Create datasource
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, 'Column Types Test DS', 'postgres', '{}')
		ON CONFLICT (id) DO NOTHING
	`, ctc.dsID, ctc.projectID)
	if err != nil {
		ctc.t.Fatalf("failed to create datasource: %v", err)
	}

	// Create source table (orders)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name)
		VALUES ($1, $2, $3, 'public', 'orders')
		ON CONFLICT (id) DO NOTHING
	`, ctc.sourceTableID, ctc.projectID, ctc.dsID)
	if err != nil {
		ctc.t.Fatalf("failed to create source table: %v", err)
	}

	// Create target table (users)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name)
		VALUES ($1, $2, $3, 'public', 'users')
		ON CONFLICT (id) DO NOTHING
	`, ctc.targetTableID, ctc.projectID, ctc.dsID)
	if err != nil {
		ctc.t.Fatalf("failed to create target table: %v", err)
	}

	// Create source column (orders.user_id as bigint)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position)
		VALUES ($1, $2, $3, 'user_id', 'bigint', false, 1)
		ON CONFLICT (id) DO UPDATE SET data_type = EXCLUDED.data_type
	`, ctc.sourceColumnID, ctc.projectID, ctc.sourceTableID)
	if err != nil {
		ctc.t.Fatalf("failed to create source column: %v", err)
	}

	// Create target column (users.id as uuid)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position)
		VALUES ($1, $2, $3, 'id', 'uuid', false, 1)
		ON CONFLICT (id) DO UPDATE SET data_type = EXCLUDED.data_type
	`, ctc.targetColumnID, ctc.projectID, ctc.targetTableID)
	if err != nil {
		ctc.t.Fatalf("failed to create target column: %v", err)
	}
}

// createRelationshipWithColumnIDs creates a relationship with source and target column IDs.
func (ctc *columnTypesTestContext) createRelationshipWithColumnIDs(ctx context.Context) *models.EntityRelationship {
	ctc.t.Helper()
	rel := &models.EntityRelationship{
		OntologyID:         ctc.ontologyID,
		SourceEntityID:     ctc.sourceEntityID,
		TargetEntityID:     ctc.targetEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   "user_id",
		SourceColumnID:     &ctc.sourceColumnID,
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   "id",
		TargetColumnID:     &ctc.targetColumnID,
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        models.CardinalityNTo1,
		CreatedBy:          models.ProvenanceInference,
	}

	// Create uses the repository method which now handles column IDs
	err := ctc.repo.Create(ctx, rel)
	if err != nil {
		ctc.t.Fatalf("failed to create relationship with column IDs: %v", err)
	}
	return rel
}

func TestEntityRelationshipRepository_GetByProject_ColumnTypes(t *testing.T) {
	ctc := setupColumnTypesTest(t)
	ctc.cleanup()
	defer ctc.cleanup()

	ctx, cleanup := ctc.createTestContext()
	defer cleanup()

	// Create a relationship with column IDs
	rel := ctc.createRelationshipWithColumnIDs(ctx)

	// Get by project
	relationships, err := ctc.repo.GetByProject(ctx, ctc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	if len(relationships) == 0 {
		t.Fatal("expected at least one relationship")
	}

	// Find our test relationship
	var found *models.EntityRelationship
	for _, r := range relationships {
		if r.ID == rel.ID {
			found = r
			break
		}
	}

	if found == nil {
		t.Fatal("test relationship not found in results")
	}

	// Verify column types are populated from JOIN
	if found.SourceColumnType != "bigint" {
		t.Errorf("expected SourceColumnType='bigint', got '%s'", found.SourceColumnType)
	}

	if found.TargetColumnType != "uuid" {
		t.Errorf("expected TargetColumnType='uuid', got '%s'", found.TargetColumnType)
	}

	// Verify column IDs are returned
	if found.SourceColumnID == nil || *found.SourceColumnID != ctc.sourceColumnID {
		t.Errorf("expected SourceColumnID=%s, got %v", ctc.sourceColumnID, found.SourceColumnID)
	}

	if found.TargetColumnID == nil || *found.TargetColumnID != ctc.targetColumnID {
		t.Errorf("expected TargetColumnID=%s, got %v", ctc.targetColumnID, found.TargetColumnID)
	}
}

func TestEntityRelationshipRepository_Create_PersistsColumnIDs(t *testing.T) {
	ctc := setupColumnTypesTest(t)
	ctc.cleanup()
	defer ctc.cleanup()

	ctx, cleanup := ctc.createTestContext()
	defer cleanup()

	// Create a relationship WITH column IDs using the Create method
	rel := &models.EntityRelationship{
		OntologyID:         ctc.ontologyID,
		SourceEntityID:     ctc.sourceEntityID,
		TargetEntityID:     ctc.targetEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   "user_id",
		SourceColumnID:     &ctc.sourceColumnID,
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   "id",
		TargetColumnID:     &ctc.targetColumnID,
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        models.CardinalityNTo1,
		CreatedBy:          models.ProvenanceInference,
	}

	// Use the Create method (not raw SQL)
	err := ctc.repo.Create(ctx, rel)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get by project to verify column IDs and types
	relationships, err := ctc.repo.GetByProject(ctx, ctc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	// Find our test relationship
	var found *models.EntityRelationship
	for _, r := range relationships {
		if r.ID == rel.ID {
			found = r
			break
		}
	}

	if found == nil {
		t.Fatal("test relationship not found in results")
	}

	// Verify column IDs are persisted by Create method
	if found.SourceColumnID == nil {
		t.Error("expected SourceColumnID to be persisted, got nil")
	} else if *found.SourceColumnID != ctc.sourceColumnID {
		t.Errorf("expected SourceColumnID=%s, got %s", ctc.sourceColumnID, *found.SourceColumnID)
	}

	if found.TargetColumnID == nil {
		t.Error("expected TargetColumnID to be persisted, got nil")
	} else if *found.TargetColumnID != ctc.targetColumnID {
		t.Errorf("expected TargetColumnID=%s, got %s", ctc.targetColumnID, *found.TargetColumnID)
	}

	// Verify column types are populated from JOIN (only works if column IDs are persisted)
	if found.SourceColumnType != "bigint" {
		t.Errorf("expected SourceColumnType='bigint', got '%s'", found.SourceColumnType)
	}

	if found.TargetColumnType != "uuid" {
		t.Errorf("expected TargetColumnType='uuid', got '%s'", found.TargetColumnType)
	}
}

func TestEntityRelationshipRepository_Upsert_PersistsColumnIDs(t *testing.T) {
	ctc := setupColumnTypesTest(t)
	ctc.cleanup()
	defer ctc.cleanup()

	ctx, cleanup := ctc.createTestContext()
	defer cleanup()

	// Create a relationship WITH column IDs using the Upsert method
	rel := &models.EntityRelationship{
		OntologyID:         ctc.ontologyID,
		SourceEntityID:     ctc.sourceEntityID,
		TargetEntityID:     ctc.targetEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  "orders",
		SourceColumnName:   "user_id",
		SourceColumnID:     &ctc.sourceColumnID,
		TargetColumnSchema: "public",
		TargetColumnTable:  "users",
		TargetColumnName:   "id",
		TargetColumnID:     &ctc.targetColumnID,
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        models.CardinalityNTo1,
		CreatedBy:          models.ProvenanceInference,
	}

	// Use the Upsert method
	err := ctc.repo.Upsert(ctx, rel)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Get by project to verify column IDs and types
	relationships, err := ctc.repo.GetByProject(ctx, ctc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	// Find our test relationship
	var found *models.EntityRelationship
	for _, r := range relationships {
		if r.ID == rel.ID {
			found = r
			break
		}
	}

	if found == nil {
		t.Fatal("test relationship not found in results")
	}

	// Verify column IDs are persisted by Upsert method
	if found.SourceColumnID == nil {
		t.Error("expected SourceColumnID to be persisted, got nil")
	} else if *found.SourceColumnID != ctc.sourceColumnID {
		t.Errorf("expected SourceColumnID=%s, got %s", ctc.sourceColumnID, *found.SourceColumnID)
	}

	if found.TargetColumnID == nil {
		t.Error("expected TargetColumnID to be persisted, got nil")
	} else if *found.TargetColumnID != ctc.targetColumnID {
		t.Errorf("expected TargetColumnID=%s, got %s", ctc.targetColumnID, *found.TargetColumnID)
	}

	// Verify column types are populated from JOIN
	if found.SourceColumnType != "bigint" {
		t.Errorf("expected SourceColumnType='bigint', got '%s'", found.SourceColumnType)
	}

	if found.TargetColumnType != "uuid" {
		t.Errorf("expected TargetColumnType='uuid', got '%s'", found.TargetColumnType)
	}
}

func TestEntityRelationshipRepository_GetByProject_NilColumnIDs(t *testing.T) {
	tc := setupRelationshipTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a relationship WITHOUT column IDs (legacy data scenario)
	rel := tc.createTestRelationship(ctx, "user_id", "id", models.ProvenanceInference)

	// Get by project
	relationships, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	if len(relationships) == 0 {
		t.Fatal("expected at least one relationship")
	}

	// Find our test relationship
	var found *models.EntityRelationship
	for _, r := range relationships {
		if r.ID == rel.ID {
			found = r
			break
		}
	}

	if found == nil {
		t.Fatal("test relationship not found in results")
	}

	// Verify column types are empty (COALESCE returns '')
	if found.SourceColumnType != "" {
		t.Errorf("expected SourceColumnType='', got '%s'", found.SourceColumnType)
	}

	if found.TargetColumnType != "" {
		t.Errorf("expected TargetColumnType='', got '%s'", found.TargetColumnType)
	}

	// Verify column IDs are nil
	if found.SourceColumnID != nil {
		t.Errorf("expected SourceColumnID=nil, got %v", found.SourceColumnID)
	}

	if found.TargetColumnID != nil {
		t.Errorf("expected TargetColumnID=nil, got %v", found.TargetColumnID)
	}
}

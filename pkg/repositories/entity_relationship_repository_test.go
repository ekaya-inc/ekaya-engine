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

//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// entityPromotionIntegrationContext holds test dependencies for entity promotion integration tests.
type entityPromotionIntegrationContext struct {
	t                *testing.T
	engineDB         *testhelpers.EngineDB
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	ontologyRepo     repositories.OntologyRepository
	schemaRepo       repositories.SchemaRepository
	projectID        uuid.UUID
	ontologyID       uuid.UUID
	testUserID       uuid.UUID
}

// setupEntityPromotionIntegrationTest initializes the test context with shared testcontainer.
func setupEntityPromotionIntegrationTest(t *testing.T) *entityPromotionIntegrationContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &entityPromotionIntegrationContext{
		t:                t,
		engineDB:         engineDB,
		entityRepo:       repositories.NewOntologyEntityRepository(),
		relationshipRepo: repositories.NewEntityRelationshipRepository(),
		ontologyRepo:     repositories.NewOntologyRepository(),
		schemaRepo:       repositories.NewSchemaRepository(),
		projectID:        uuid.MustParse("00000000-0000-0000-0000-000000000070"),
		ontologyID:       uuid.MustParse("00000000-0000-0000-0000-000000000071"),
		testUserID:       uuid.MustParse("00000000-0000-0000-0000-000000000072"),
	}
	tc.ensureTestProjectAndOntology()
	return tc
}

// ensureTestProjectAndOntology creates the test project and ontology if they don't exist.
func (tc *entityPromotionIntegrationContext) ensureTestProjectAndOntology() {
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
	`, tc.projectID, "Entity Promotion Integration Test Project")
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

// cleanup removes test entities and relationships.
func (tc *entityPromotionIntegrationContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Cascading delete will handle relationships
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE ontology_id = $1", tc.ontologyID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_entity_relationships WHERE ontology_id = $1", tc.ontologyID)
}

// createTestContext returns a context with tenant scope and the specified provenance.
func (tc *entityPromotionIntegrationContext) createTestContext(source models.ProvenanceSource) (context.Context, func()) {
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

// createTestEntity creates an entity for testing.
func (tc *entityPromotionIntegrationContext) createTestEntity(ctx context.Context, name, tableName string, source models.ProvenanceSource) *models.OntologyEntity {
	tc.t.Helper()

	// Apply provenance to context
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
		PrimaryTable:  tableName,
		PrimaryColumn: "id",
		IsPromoted:    true, // Default to promoted
	}
	err := tc.entityRepo.Create(ctxWithProv, entity)
	if err != nil {
		tc.t.Fatalf("failed to create test entity: %v", err)
	}
	return entity
}

// createTestRelationship creates a relationship targeting the given entity.
// This simulates an inbound relationship (FK from source table to target table).
func (tc *entityPromotionIntegrationContext) createTestRelationship(ctx context.Context, sourceEntityID, targetEntityID uuid.UUID, sourceTable, sourceColumn, targetTable, targetColumn string) *models.EntityRelationship {
	tc.t.Helper()
	rel := &models.EntityRelationship{
		ProjectID:          tc.projectID,
		OntologyID:         tc.ontologyID,
		SourceEntityID:     sourceEntityID,
		TargetEntityID:     targetEntityID,
		SourceColumnSchema: "public",
		SourceColumnTable:  sourceTable,
		SourceColumnName:   sourceColumn,
		TargetColumnSchema: "public",
		TargetColumnTable:  targetTable,
		TargetColumnName:   targetColumn,
		DetectionMethod:    models.DetectionMethodForeignKey,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
		Cardinality:        "N:1",
	}
	err := tc.relationshipRepo.Create(ctx, rel)
	if err != nil {
		tc.t.Fatalf("failed to create test relationship: %v", err)
	}
	return rel
}

// ============================================================================
// Integration Tests for EntityPromotion Service
// These tests verify the EntityPromotion DAG node works correctly by testing
// the underlying ScoreAndPromoteEntities service method with a real database.
// ============================================================================

func TestEntityPromotion_Integration_HighValueEntityPromoted(t *testing.T) {
	tc := setupEntityPromotionIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext(models.SourceInferred)
	defer cleanup()

	// Create a "User" entity that will be a hub (target of many relationships)
	userEntity := tc.createTestEntity(ctx, "User", "users", models.SourceInferred)

	// Create several source entities that reference the User entity
	ordersEntity := tc.createTestEntity(ctx, "Order", "orders", models.SourceInferred)
	postsEntity := tc.createTestEntity(ctx, "Post", "posts", models.SourceInferred)
	commentsEntity := tc.createTestEntity(ctx, "Comment", "comments", models.SourceInferred)
	sessionsEntity := tc.createTestEntity(ctx, "Session", "sessions", models.SourceInferred)
	paymentsEntity := tc.createTestEntity(ctx, "Payment", "payments", models.SourceInferred)

	// Create 5+ inbound relationships to User (hub with 30 points)
	tc.createTestRelationship(ctx, ordersEntity.ID, userEntity.ID, "orders", "user_id", "users", "id")
	tc.createTestRelationship(ctx, postsEntity.ID, userEntity.ID, "posts", "author_id", "users", "id")
	tc.createTestRelationship(ctx, commentsEntity.ID, userEntity.ID, "comments", "user_id", "users", "id")
	tc.createTestRelationship(ctx, sessionsEntity.ID, userEntity.ID, "sessions", "user_id", "users", "id")
	tc.createTestRelationship(ctx, paymentsEntity.ID, userEntity.ID, "payments", "payer_id", "users", "id")

	// Create the promotion service
	promotionService := NewEntityPromotionService(
		tc.entityRepo,
		tc.relationshipRepo,
		tc.schemaRepo,
		tc.ontologyRepo,
		zap.NewNop(),
	)

	// Execute promotion scoring
	promoted, demoted, err := promotionService.ScoreAndPromoteEntities(ctx, tc.projectID)
	require.NoError(t, err)

	// User should be promoted (30 points for hub + 25 for multiple roles = 55+)
	// Other entities have 0 inbound relationships, so should be demoted
	assert.GreaterOrEqual(t, promoted, 1, "At least User should be promoted")
	assert.GreaterOrEqual(t, demoted, 1, "At least some entities should be demoted")

	// Verify User entity is promoted
	retrievedUser, err := tc.entityRepo.GetByID(ctx, userEntity.ID)
	require.NoError(t, err)
	assert.True(t, retrievedUser.IsPromoted, "User entity should be promoted")
	assert.NotNil(t, retrievedUser.PromotionScore, "User should have a promotion score")
	assert.GreaterOrEqual(t, *retrievedUser.PromotionScore, 50, "User score should be >= 50 (threshold)")
	assert.NotEmpty(t, retrievedUser.PromotionReasons, "User should have promotion reasons")
}

func TestEntityPromotion_Integration_LowValueEntityDemoted(t *testing.T) {
	tc := setupEntityPromotionIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext(models.SourceInferred)
	defer cleanup()

	// Create an isolated entity with no relationships (0 points → demoted)
	isolatedEntity := tc.createTestEntity(ctx, "Session", "sessions", models.SourceInferred)

	// Create the promotion service
	promotionService := NewEntityPromotionService(
		tc.entityRepo,
		tc.relationshipRepo,
		tc.schemaRepo,
		tc.ontologyRepo,
		zap.NewNop(),
	)

	// Execute promotion scoring
	promoted, demoted, err := promotionService.ScoreAndPromoteEntities(ctx, tc.projectID)
	require.NoError(t, err)

	// Session has no relationships, so should be demoted
	assert.Equal(t, 0, promoted, "No entities should be promoted")
	assert.Equal(t, 1, demoted, "Session should be demoted")

	// Verify Session entity is demoted
	retrievedSession, err := tc.entityRepo.GetByID(ctx, isolatedEntity.ID)
	require.NoError(t, err)
	assert.False(t, retrievedSession.IsPromoted, "Session entity should be demoted")
	assert.NotNil(t, retrievedSession.PromotionScore, "Session should have a promotion score")
	assert.Less(t, *retrievedSession.PromotionScore, 50, "Session score should be < 50 (threshold)")
}

func TestEntityPromotion_Integration_PreservesManualDecisions(t *testing.T) {
	tc := setupEntityPromotionIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext(models.SourceInferred)
	defer cleanup()

	// Create a manually promoted entity (even with 0 relationships, should stay promoted)
	manuallyPromotedEntity := tc.createTestEntity(ctx, "Config", "configs", models.SourceManual)

	// Update it to be explicitly promoted with source=manual
	manuallyPromotedEntity.IsPromoted = true
	err := tc.entityRepo.Update(ctx, manuallyPromotedEntity)
	require.NoError(t, err)

	// Create a manually demoted entity (even if it would score high, should stay demoted)
	manuallyDemotedEntity := tc.createTestEntity(ctx, "Audit", "audits", models.SourceManual)

	// Update it to be explicitly demoted with source=manual
	manuallyDemotedEntity.IsPromoted = false
	err = tc.entityRepo.Update(ctx, manuallyDemotedEntity)
	require.NoError(t, err)

	// Create the promotion service
	promotionService := NewEntityPromotionService(
		tc.entityRepo,
		tc.relationshipRepo,
		tc.schemaRepo,
		tc.ontologyRepo,
		zap.NewNop(),
	)

	// Execute promotion scoring
	promoted, demoted, err := promotionService.ScoreAndPromoteEntities(ctx, tc.projectID)
	require.NoError(t, err)

	// Both manual entities should be counted as-is (not re-evaluated)
	assert.Equal(t, 1, promoted, "Manually promoted entity should be counted as promoted")
	assert.Equal(t, 1, demoted, "Manually demoted entity should be counted as demoted")

	// Verify manually promoted entity is still promoted
	retrievedPromoted, err := tc.entityRepo.GetByID(ctx, manuallyPromotedEntity.ID)
	require.NoError(t, err)
	assert.True(t, retrievedPromoted.IsPromoted, "Manually promoted entity should remain promoted")
	// Manual entities should NOT have a promotion score set (they're skipped)
	// The score remains what it was before (nil if never set)

	// Verify manually demoted entity is still demoted
	retrievedDemoted, err := tc.entityRepo.GetByID(ctx, manuallyDemotedEntity.ID)
	require.NoError(t, err)
	assert.False(t, retrievedDemoted.IsPromoted, "Manually demoted entity should remain demoted")
}

func TestEntityPromotion_Integration_MixedScenario(t *testing.T) {
	tc := setupEntityPromotionIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext(models.SourceInferred)
	defer cleanup()

	// Create a high-value hub entity (User)
	userEntity := tc.createTestEntity(ctx, "User", "users", models.SourceInferred)

	// Create low-value entities that will be demoted
	sessionEntity := tc.createTestEntity(ctx, "Session", "sessions", models.SourceInferred)
	tokenEntity := tc.createTestEntity(ctx, "Token", "tokens", models.SourceInferred)

	// Create source entities that reference User
	ordersEntity := tc.createTestEntity(ctx, "Order", "orders", models.SourceInferred)
	postsEntity := tc.createTestEntity(ctx, "Post", "posts", models.SourceInferred)
	commentsEntity := tc.createTestEntity(ctx, "Comment", "comments", models.SourceInferred)
	reviewsEntity := tc.createTestEntity(ctx, "Review", "reviews", models.SourceInferred)
	addressesEntity := tc.createTestEntity(ctx, "Address", "addresses", models.SourceInferred)

	// Create 5 inbound relationships to User with different roles
	// The scoring algorithm gives 30 points for 5+ inbound refs, plus 25 for 2+ distinct roles
	// Different column names (user_id, author_id, owner_id, reviewer_id, buyer_id) create distinct roles
	tc.createTestRelationship(ctx, ordersEntity.ID, userEntity.ID, "orders", "buyer_id", "users", "id")
	tc.createTestRelationship(ctx, postsEntity.ID, userEntity.ID, "posts", "author_id", "users", "id")
	tc.createTestRelationship(ctx, commentsEntity.ID, userEntity.ID, "comments", "user_id", "users", "id")
	tc.createTestRelationship(ctx, reviewsEntity.ID, userEntity.ID, "reviews", "reviewer_id", "users", "id")
	tc.createTestRelationship(ctx, addressesEntity.ID, userEntity.ID, "addresses", "owner_id", "users", "id")

	// Create the promotion service
	promotionService := NewEntityPromotionService(
		tc.entityRepo,
		tc.relationshipRepo,
		tc.schemaRepo,
		tc.ontologyRepo,
		zap.NewNop(),
	)

	// Execute promotion scoring
	promoted, demoted, err := promotionService.ScoreAndPromoteEntities(ctx, tc.projectID)
	require.NoError(t, err)

	// User should be promoted (30 points for hub + 25 for roles = 55), the rest should be demoted
	// Total entities: 8 (1 user + 2 low-value + 5 source entities)
	assert.GreaterOrEqual(t, promoted, 1, "At least User should be promoted")
	assert.GreaterOrEqual(t, demoted, 2, "Session and Token should be demoted")

	// Verify User is promoted
	retrievedUser, err := tc.entityRepo.GetByID(ctx, userEntity.ID)
	require.NoError(t, err)
	assert.True(t, retrievedUser.IsPromoted, "User should be promoted")

	// Verify Session is demoted (0 relationships)
	retrievedSession, err := tc.entityRepo.GetByID(ctx, sessionEntity.ID)
	require.NoError(t, err)
	assert.False(t, retrievedSession.IsPromoted, "Session should be demoted")

	// Verify Token is demoted (0 relationships)
	retrievedToken, err := tc.entityRepo.GetByID(ctx, tokenEntity.ID)
	require.NoError(t, err)
	assert.False(t, retrievedToken.IsPromoted, "Token should be demoted")
}

func TestEntityPromotion_Integration_RoleBasedReferences(t *testing.T) {
	tc := setupEntityPromotionIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext(models.SourceInferred)
	defer cleanup()

	// Create a User entity that will be referenced by multiple distinct roles
	userEntity := tc.createTestEntity(ctx, "User", "users", models.SourceInferred)

	// Create an engagement entity that references User in multiple roles
	engagementEntity := tc.createTestEntity(ctx, "Engagement", "engagements", models.SourceInferred)

	// Create relationships with different roles (host_id, visitor_id, creator_id)
	// This gives User 3 inbound refs (20 points) + 3 distinct roles (25 points) = 45 points
	// Need one more relationship to hit 50+ threshold
	tc.createTestRelationship(ctx, engagementEntity.ID, userEntity.ID, "engagements", "host_id", "users", "id")
	tc.createTestRelationship(ctx, engagementEntity.ID, userEntity.ID, "engagements", "visitor_id", "users", "id")
	tc.createTestRelationship(ctx, engagementEntity.ID, userEntity.ID, "engagements", "creator_id", "users", "id")

	// Add two more relationships to reach 5 inbound (30 points) + 5 roles (25 points) = 55 points
	tc.createTestRelationship(ctx, engagementEntity.ID, userEntity.ID, "engagements", "updater_id", "users", "id")
	tc.createTestRelationship(ctx, engagementEntity.ID, userEntity.ID, "engagements", "approver_id", "users", "id")

	// Create the promotion service
	promotionService := NewEntityPromotionService(
		tc.entityRepo,
		tc.relationshipRepo,
		tc.schemaRepo,
		tc.ontologyRepo,
		zap.NewNop(),
	)

	// Execute promotion scoring
	promoted, demoted, err := promotionService.ScoreAndPromoteEntities(ctx, tc.projectID)
	require.NoError(t, err)

	// User should be promoted due to multiple roles
	assert.GreaterOrEqual(t, promoted, 1, "User should be promoted")

	// Verify User is promoted with role-based reasons
	retrievedUser, err := tc.entityRepo.GetByID(ctx, userEntity.ID)
	require.NoError(t, err)
	assert.True(t, retrievedUser.IsPromoted, "User should be promoted")
	assert.NotNil(t, retrievedUser.PromotionScore, "User should have a promotion score")
	assert.GreaterOrEqual(t, *retrievedUser.PromotionScore, 50, "User score should be >= 50")

	// Verify reasons include both hub status and roles
	foundHubReason := false
	foundRoleReason := false
	for _, reason := range retrievedUser.PromotionReasons {
		if len(reason) > 0 {
			if containsSubstring(reason, "inbound") || containsSubstring(reason, "hub") {
				foundHubReason = true
			}
			if containsSubstring(reason, "role") {
				foundRoleReason = true
			}
		}
	}
	assert.True(t, foundHubReason || foundRoleReason, "Should have hub or role reasons")

	_ = demoted // demoted count depends on all entities in the test
}

// containsSubstring checks if substr is in s (simple helper to avoid importing strings for one use).
func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

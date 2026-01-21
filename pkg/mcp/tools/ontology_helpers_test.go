//go:build integration

package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func TestEnsureOntologyExists_CreatesNewOntology(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000301")

	// Clean up any existing data for this project
	cleanupTestProject(t, engineDB, projectID)
	createTestProject(t, engineDB, projectID, "Ensure Ontology Test")

	ontologyRepo := repositories.NewOntologyRepository()
	ctx := context.Background()

	// Get tenant context
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	require.NoError(t, err)
	defer tenantScope.Close()
	tenantCtx := database.SetTenantScope(ctx, tenantScope)

	// Call ensureOntologyExists - should create new ontology
	ontology, err := ensureOntologyExists(tenantCtx, ontologyRepo, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontology)

	// Verify ontology properties
	assert.Equal(t, projectID, ontology.ProjectID)
	assert.Equal(t, 1, ontology.Version)
	assert.True(t, ontology.IsActive)
	assert.NotNil(t, ontology.EntitySummaries)
	assert.NotNil(t, ontology.ColumnDetails)
	assert.Empty(t, ontology.EntitySummaries, "new ontology should have empty entity summaries")
	assert.Empty(t, ontology.ColumnDetails, "new ontology should have empty column details")
}

func TestEnsureOntologyExists_ReturnsExistingOntology(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000302")

	// Clean up and create test project
	cleanupTestProject(t, engineDB, projectID)
	createTestProject(t, engineDB, projectID, "Ensure Ontology Existing Test")

	ontologyRepo := repositories.NewOntologyRepository()
	ctx := context.Background()

	// Get tenant context
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	require.NoError(t, err)
	defer tenantScope.Close()
	tenantCtx := database.SetTenantScope(ctx, tenantScope)

	// Create ontology first time
	ontology1, err := ensureOntologyExists(tenantCtx, ontologyRepo, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontology1)

	// Call ensureOntologyExists again - should return same ontology
	ontology2, err := ensureOntologyExists(tenantCtx, ontologyRepo, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontology2)

	// Should be the same ontology
	assert.Equal(t, ontology1.ID, ontology2.ID)
	assert.Equal(t, ontology1.Version, ontology2.Version)
}

func TestEnsureOntologyExists_Idempotent(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000303")

	// Clean up and create test project
	cleanupTestProject(t, engineDB, projectID)
	createTestProject(t, engineDB, projectID, "Ensure Ontology Idempotent Test")

	ontologyRepo := repositories.NewOntologyRepository()
	ctx := context.Background()

	// Get tenant context
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	require.NoError(t, err)
	defer tenantScope.Close()
	tenantCtx := database.SetTenantScope(ctx, tenantScope)

	// Call multiple times concurrently (simulated sequentially here)
	var ontologies []*uuid.UUID
	for i := 0; i < 3; i++ {
		ontology, err := ensureOntologyExists(tenantCtx, ontologyRepo, projectID)
		require.NoError(t, err)
		require.NotNil(t, ontology)
		ontologies = append(ontologies, &ontology.ID)
	}

	// All should return the same ontology ID
	for i := 1; i < len(ontologies); i++ {
		assert.Equal(t, *ontologies[0], *ontologies[i], "all calls should return the same ontology")
	}
}

// cleanupTestProject removes any existing data for the test project
func cleanupTestProject(t *testing.T, engineDB *testhelpers.EngineDB, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	// Clean up ontology (RLS protected)
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err == nil {
		_, _ = tenantScope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, projectID)
		tenantScope.Close()
	}

	// Clean up project (not RLS protected)
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_projects WHERE id = $1`, projectID)
}

// createTestProject creates a test project
func createTestProject(t *testing.T, engineDB *testhelpers.EngineDB, projectID uuid.UUID, name string) {
	t.Helper()
	ctx := context.Background()

	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
	`, projectID, name)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}
}

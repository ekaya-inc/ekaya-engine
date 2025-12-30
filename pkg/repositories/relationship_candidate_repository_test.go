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

// candidateTestContext holds test dependencies for relationship candidate repository tests.
type candidateTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	repo         RelationshipCandidateRepository
	projectID    uuid.UUID
	datasourceID uuid.UUID
	workflowID   uuid.UUID
	ontologyID   uuid.UUID
	tableID      uuid.UUID
	sourceColID  uuid.UUID
	targetColID  uuid.UUID
}

// setupCandidateTest initializes the test context with shared testcontainer.
func setupCandidateTest(t *testing.T) *candidateTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &candidateTestContext{
		t:            t,
		engineDB:     engineDB,
		repo:         NewRelationshipCandidateRepository(),
		projectID:    uuid.New(),
		datasourceID: uuid.New(),
		workflowID:   uuid.New(),
		ontologyID:   uuid.New(),
		tableID:      uuid.New(),
		sourceColID:  uuid.New(),
		targetColID:  uuid.New(),
	}
	tc.ensureTestData()
	return tc
}

// ensureTestData creates the required test data.
func (tc *candidateTestContext) ensureTestData() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for test data setup: %v", err)
	}
	defer scope.Close()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Candidate Test Project")
	if err != nil {
		tc.t.Fatalf("failed to create test project: %v", err)
	}

	// Create datasource
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, 'Test Datasource', 'postgres', 'encrypted_config')
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create test datasource: %v", err)
	}

	// Create ontology
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create test ontology: %v", err)
	}

	// Create workflow
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_workflows (id, project_id, ontology_id, state, phase, datasource_id)
		VALUES ($1, $2, $3, 'running', 'relationships', $4)
		ON CONFLICT (id) DO NOTHING
	`, tc.workflowID, tc.projectID, tc.ontologyID, tc.datasourceID)
	if err != nil {
		tc.t.Fatalf("failed to create test workflow: %v", err)
	}

	// Create schema table
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name)
		VALUES ($1, $2, $3, 'public', 'test_table')
		ON CONFLICT (id) DO NOTHING
	`, tc.tableID, tc.projectID, tc.datasourceID)
	if err != nil {
		tc.t.Fatalf("failed to create test table: %v", err)
	}

	// Create source column
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position)
		VALUES ($1, $2, $3, 'user_id', 'uuid', false, 1)
		ON CONFLICT (id) DO NOTHING
	`, tc.sourceColID, tc.projectID, tc.tableID)
	if err != nil {
		tc.t.Fatalf("failed to create source column: %v", err)
	}

	// Create target column
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, ordinal_position)
		VALUES ($1, $2, $3, 'id', 'uuid', false, true, 1)
		ON CONFLICT (id) DO NOTHING
	`, tc.targetColID, tc.projectID, tc.tableID)
	if err != nil {
		tc.t.Fatalf("failed to create target column: %v", err)
	}
}

// cleanup removes test data.
func (tc *candidateTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_relationship_candidates WHERE workflow_id = $1", tc.workflowID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_workflows WHERE id = $1", tc.workflowID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_columns WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE id = $1", tc.datasourceID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE id = $1", tc.ontologyID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_projects WHERE id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *candidateTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// TestRelationshipCandidateRepository_Create tests creating a candidate.
func TestRelationshipCandidateRepository_Create(t *testing.T) {
	tc := setupCandidateTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	confidence := 0.92
	reasoning := "Column naming and value overlap suggest FK relationship"
	candidate := &models.RelationshipCandidate{
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.sourceColID,
		TargetColumnID:  tc.targetColID,
		DetectionMethod: models.DetectionMethodValueMatch,
		Confidence:      confidence,
		LLMReasoning:    &reasoning,
		Status:          models.RelCandidateStatusAccepted,
		IsRequired:      false,
	}

	err := tc.repo.Create(ctx, candidate)
	if err != nil {
		t.Fatalf("failed to create candidate: %v", err)
	}

	if candidate.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}

	// Verify created
	retrieved, err := tc.repo.GetByID(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("failed to retrieve candidate: %v", err)
	}

	if retrieved.Confidence != confidence {
		t.Errorf("expected confidence %f, got %f", confidence, retrieved.Confidence)
	}

	if retrieved.DetectionMethod != models.DetectionMethodValueMatch {
		t.Errorf("expected method value_match, got %s", retrieved.DetectionMethod)
	}
}

// TestRelationshipCandidateRepository_GetByWorkflowAndStatus tests retrieving candidates by status.
func TestRelationshipCandidateRepository_GetByWorkflowAndStatus(t *testing.T) {
	tc := setupCandidateTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create accepted candidate
	accepted := &models.RelationshipCandidate{
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.sourceColID,
		TargetColumnID:  tc.targetColID,
		DetectionMethod: models.DetectionMethodLLM,
		Confidence:      0.95,
		Status:          models.RelCandidateStatusAccepted,
		IsRequired:      false,
	}

	err := tc.repo.Create(ctx, accepted)
	if err != nil {
		t.Fatalf("failed to create accepted candidate: %v", err)
	}

	// Retrieve by status
	candidates, err := tc.repo.GetByWorkflowAndStatus(ctx, tc.workflowID, models.RelCandidateStatusAccepted)
	if err != nil {
		t.Fatalf("failed to get candidates by status: %v", err)
	}

	if len(candidates) != 1 {
		t.Errorf("expected 1 accepted candidate, got %d", len(candidates))
	}

	// Verify no rejected candidates
	rejected, err := tc.repo.GetByWorkflowAndStatus(ctx, tc.workflowID, models.RelCandidateStatusRejected)
	if err != nil {
		t.Fatalf("failed to get rejected candidates: %v", err)
	}

	if len(rejected) != 0 {
		t.Errorf("expected 0 rejected candidates, got %d", len(rejected))
	}
}

// TestRelationshipCandidateRepository_GetRequiredPending tests retrieving required pending candidates.
func TestRelationshipCandidateRepository_GetRequiredPending(t *testing.T) {
	tc := setupCandidateTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create required pending candidate
	required := &models.RelationshipCandidate{
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.sourceColID,
		TargetColumnID:  tc.targetColID,
		DetectionMethod: models.DetectionMethodLLM,
		Confidence:      0.70,
		Status:          models.RelCandidateStatusPending,
		IsRequired:      true,
	}

	err := tc.repo.Create(ctx, required)
	if err != nil {
		t.Fatalf("failed to create required candidate: %v", err)
	}

	// Retrieve required pending
	candidates, err := tc.repo.GetRequiredPending(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("failed to get required pending candidates: %v", err)
	}

	if len(candidates) != 1 {
		t.Errorf("expected 1 required pending candidate, got %d", len(candidates))
	}
}

// TestRelationshipCandidateRepository_UpdateStatus tests updating candidate status.
func TestRelationshipCandidateRepository_UpdateStatus(t *testing.T) {
	tc := setupCandidateTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create pending candidate
	candidate := &models.RelationshipCandidate{
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.sourceColID,
		TargetColumnID:  tc.targetColID,
		DetectionMethod: models.DetectionMethodNameInference,
		Confidence:      0.75,
		Status:          models.RelCandidateStatusPending,
		IsRequired:      true,
	}

	err := tc.repo.Create(ctx, candidate)
	if err != nil {
		t.Fatalf("failed to create candidate: %v", err)
	}

	// Update to accepted with user decision
	decision := models.UserDecisionAccepted
	err = tc.repo.UpdateStatus(ctx, candidate.ID, models.RelCandidateStatusAccepted, &decision)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Verify updated
	updated, err := tc.repo.GetByID(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("failed to retrieve updated candidate: %v", err)
	}

	if updated.Status != models.RelCandidateStatusAccepted {
		t.Errorf("expected status accepted, got %s", updated.Status)
	}

	if updated.UserDecision == nil || *updated.UserDecision != models.UserDecisionAccepted {
		t.Error("expected user decision to be accepted")
	}
}

// TestRelationshipCandidateRepository_CountMethods tests count methods.
func TestRelationshipCandidateRepository_CountMethods(t *testing.T) {
	tc := setupCandidateTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple candidates with different statuses
	candidates := []*models.RelationshipCandidate{
		{
			WorkflowID:      tc.workflowID,
			DatasourceID:    tc.datasourceID,
			SourceColumnID:  tc.sourceColID,
			TargetColumnID:  tc.targetColID,
			DetectionMethod: models.DetectionMethodValueMatch,
			Confidence:      0.95,
			Status:          models.RelCandidateStatusAccepted,
			IsRequired:      false,
		},
		{
			WorkflowID:      tc.workflowID,
			DatasourceID:    tc.datasourceID,
			SourceColumnID:  uuid.New(), // Different source
			TargetColumnID:  tc.targetColID,
			DetectionMethod: models.DetectionMethodLLM,
			Confidence:      0.70,
			Status:          models.RelCandidateStatusPending,
			IsRequired:      true,
		},
		{
			WorkflowID:      tc.workflowID,
			DatasourceID:    tc.datasourceID,
			SourceColumnID:  uuid.New(), // Different source
			TargetColumnID:  uuid.New(), // Different target
			DetectionMethod: models.DetectionMethodNameInference,
			Confidence:      0.40,
			Status:          models.RelCandidateStatusRejected,
			IsRequired:      false,
		},
	}

	// Create second source/target columns for unique constraint
	scope, _ := tc.engineDB.DB.WithoutTenant(context.Background())
	defer scope.Close()
	_, _ = scope.Conn.Exec(context.Background(), `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position)
		VALUES ($1, $2, $3, 'col2', 'uuid', false, 2)
		ON CONFLICT (id) DO NOTHING
	`, candidates[1].SourceColumnID, tc.projectID, tc.tableID)
	_, _ = scope.Conn.Exec(context.Background(), `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position)
		VALUES ($1, $2, $3, 'col3', 'uuid', false, 3)
		ON CONFLICT (id) DO NOTHING
	`, candidates[2].SourceColumnID, tc.projectID, tc.tableID)
	_, _ = scope.Conn.Exec(context.Background(), `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, ordinal_position)
		VALUES ($1, $2, $3, 'col4', 'uuid', false, 4)
		ON CONFLICT (id) DO NOTHING
	`, candidates[2].TargetColumnID, tc.projectID, tc.tableID)

	for _, c := range candidates {
		if err := tc.repo.Create(ctx, c); err != nil {
			t.Fatalf("failed to create candidate: %v", err)
		}
	}

	// Test count by status
	acceptedCount, err := tc.repo.CountByWorkflowAndStatus(ctx, tc.workflowID, models.RelCandidateStatusAccepted)
	if err != nil {
		t.Fatalf("failed to count accepted: %v", err)
	}
	if acceptedCount != 1 {
		t.Errorf("expected 1 accepted, got %d", acceptedCount)
	}

	pendingCount, err := tc.repo.CountByWorkflowAndStatus(ctx, tc.workflowID, models.RelCandidateStatusPending)
	if err != nil {
		t.Fatalf("failed to count pending: %v", err)
	}
	if pendingCount != 1 {
		t.Errorf("expected 1 pending, got %d", pendingCount)
	}

	// Test count required pending
	requiredCount, err := tc.repo.CountRequiredPending(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("failed to count required pending: %v", err)
	}
	if requiredCount != 1 {
		t.Errorf("expected 1 required pending, got %d", requiredCount)
	}
}

// TestRelationshipCandidateRepository_DeleteByWorkflow tests deleting all candidates for a workflow.
func TestRelationshipCandidateRepository_DeleteByWorkflow(t *testing.T) {
	tc := setupCandidateTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create candidate
	candidate := &models.RelationshipCandidate{
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.sourceColID,
		TargetColumnID:  tc.targetColID,
		DetectionMethod: models.DetectionMethodLLM,
		Confidence:      0.85,
		Status:          models.RelCandidateStatusAccepted,
		IsRequired:      false,
	}

	err := tc.repo.Create(ctx, candidate)
	if err != nil {
		t.Fatalf("failed to create candidate: %v", err)
	}

	// Delete by workflow
	err = tc.repo.DeleteByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("failed to delete by workflow: %v", err)
	}

	// Verify deleted
	candidates, err := tc.repo.GetByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("failed to get candidates: %v", err)
	}

	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates after delete, got %d", len(candidates))
	}
}

//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// workflowStateTestContext holds all dependencies for workflow state integration tests.
type workflowStateTestContext struct {
	t               *testing.T
	engineDB        *testhelpers.EngineDB
	workflowService OntologyWorkflowService
	stateRepo       repositories.WorkflowStateRepository
	schemaRepo      repositories.SchemaRepository
	workflowRepo    repositories.OntologyWorkflowRepository
	ontologyRepo    repositories.OntologyRepository
	projectID       uuid.UUID
	dsID            uuid.UUID
	logger          *zap.Logger
}

// setupWorkflowStateTest creates a test context with real database.
func setupWorkflowStateTest(t *testing.T) *workflowStateTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	logger := zap.NewNop()

	// Create repositories
	workflowRepo := repositories.NewOntologyWorkflowRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	schemaRepo := repositories.NewSchemaRepository()
	stateRepo := repositories.NewWorkflowStateRepository()

	// Create mock dependencies for the service
	// Pass nil for connection manager since tests use unmanaged pools
	adapterFactory := datasource.NewDatasourceAdapterFactory(nil)

	// Create getTenantCtx function
	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := engineDB.DB.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}

	// Create a minimal builder service (we won't actually use LLM)
	builderService := NewOntologyBuilderService(
		ontologyRepo, schemaRepo, workflowRepo, nil, stateRepo, nil, logger)

	// Create workflow service with all dependencies
	questionRepo := repositories.NewOntologyQuestionRepository()
	workflowService := NewOntologyWorkflowService(
		workflowRepo, ontologyRepo, schemaRepo, stateRepo, questionRepo,
		nil, nil, adapterFactory, builderService, getTenantCtx, logger)

	// Use unique project ID for test isolation
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	dsID := uuid.MustParse("00000000-0000-0000-0000-000000000202")

	tc := &workflowStateTestContext{
		t:               t,
		engineDB:        engineDB,
		workflowService: workflowService,
		stateRepo:       stateRepo,
		schemaRepo:      schemaRepo,
		workflowRepo:    workflowRepo,
		ontologyRepo:    ontologyRepo,
		projectID:       projectID,
		dsID:            dsID,
		logger:          logger,
	}

	// Ensure project and datasource exist
	tc.ensureTestProject()
	tc.ensureTestDatasource()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *workflowStateTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *workflowStateTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Workflow State Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *workflowStateTestContext) ensureTestDatasource() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID, "Workflow State Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all test data.
func (tc *workflowStateTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in order: workflow_state -> workflows -> ontologies -> schema
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_workflow_state WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_workflows WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
}

// ensureCompletedRelationshipWorkflow creates a completed relationship workflow
// to satisfy the prerequisite for ontology extraction.
func (tc *workflowStateTestContext) ensureCompletedRelationshipWorkflow(ctx context.Context) {
	tc.t.Helper()

	// Create a temporary ontology for the workflow
	ontology := &models.TieredOntology{
		ID:              uuid.New(),
		ProjectID:       tc.projectID,
		Version:         0,
		IsActive:        false,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
		Metadata:        make(map[string]any),
	}

	if err := tc.ontologyRepo.Create(ctx, ontology); err != nil {
		tc.t.Fatalf("Failed to create temporary ontology for relationship workflow: %v", err)
	}

	// Create a completed relationship workflow
	workflow := &models.OntologyWorkflow{
		ID:           uuid.New(),
		ProjectID:    tc.projectID,
		OntologyID:   ontology.ID,
		State:        models.WorkflowStateCompleted,
		Phase:        models.WorkflowPhaseRelationships,
		DatasourceID: &tc.dsID,
		Progress: &models.WorkflowProgress{
			Current: 100,
			Total:   100,
			Message: "Complete",
		},
		TaskQueue: []models.WorkflowTask{},
	}

	if err := tc.workflowRepo.Create(ctx, workflow); err != nil {
		tc.t.Fatalf("Failed to create completed relationship workflow: %v", err)
	}
}

// createTestTable creates a test table and returns it.
func (tc *workflowStateTestContext) createTestTable(ctx context.Context, schemaName, tableName string) *models.SchemaTable {
	tc.t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   schemaName,
		TableName:    tableName,
		IsSelected:   true,
	}

	if err := tc.schemaRepo.UpsertTable(ctx, table); err != nil {
		tc.t.Fatalf("Failed to create test table: %v", err)
	}

	return table
}

// createTestColumn creates a test column and returns it.
func (tc *workflowStateTestContext) createTestColumn(ctx context.Context, tableID uuid.UUID, columnName, dataType string, ordinal int) *models.SchemaColumn {
	tc.t.Helper()

	column := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   tableID,
		ColumnName:      columnName,
		DataType:        dataType,
		IsNullable:      true,
		IsPrimaryKey:    columnName == "id",
		IsSelected:      true,
		OrdinalPosition: ordinal,
	}

	if err := tc.schemaRepo.UpsertColumn(ctx, column); err != nil {
		tc.t.Fatalf("Failed to create test column: %v", err)
	}

	return column
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestStartExtraction_InitializesWorkflowState_Integration(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test tables and columns
	usersTable := tc.createTestTable(ctx, "public", "users")
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2)
	tc.createTestColumn(ctx, usersTable.ID, "name", "text", 3)

	ordersTable := tc.createTestTable(ctx, "public", "orders")
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1)
	tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2)
	tc.createTestColumn(ctx, ordersTable.ID, "total", "numeric", 3)

	// Prerequisite: relationship workflow must complete before ontology extraction
	tc.ensureCompletedRelationshipWorkflow(ctx)

	// Start extraction
	config := &models.WorkflowConfig{
		DatasourceID: tc.dsID,
	}

	workflow, err := tc.workflowService.StartExtraction(ctx, tc.projectID, config)
	if err != nil {
		t.Fatalf("StartExtraction failed: %v", err)
	}

	// Give the background goroutine a moment to start, but we don't need to wait for completion
	// The entity state should be created synchronously before StartExtraction returns

	// Verify workflow state rows were created
	states, err := tc.stateRepo.ListByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}

	// Expected: 1 global + 2 tables + 6 columns = 9 entities
	expectedCount := 1 + 2 + 6
	if len(states) != expectedCount {
		t.Errorf("expected %d workflow entities, got %d", expectedCount, len(states))
	}

	// Verify all entities have status='pending'
	for _, state := range states {
		if state.Status != models.WorkflowEntityStatusPending {
			t.Errorf("expected status 'pending' for entity %s:%s, got %s",
				state.EntityType, state.EntityKey, state.Status)
		}
	}

	// Verify entity type counts
	globalCount := 0
	tableCount := 0
	columnCount := 0
	for _, state := range states {
		switch state.EntityType {
		case models.WorkflowEntityTypeGlobal:
			globalCount++
			if state.EntityKey != "" {
				t.Errorf("expected empty entity_key for global, got %s", state.EntityKey)
			}
		case models.WorkflowEntityTypeTable:
			tableCount++
		case models.WorkflowEntityTypeColumn:
			columnCount++
		}
	}

	if globalCount != 1 {
		t.Errorf("expected 1 global entity, got %d", globalCount)
	}
	if tableCount != 2 {
		t.Errorf("expected 2 table entities, got %d", tableCount)
	}
	if columnCount != 6 {
		t.Errorf("expected 6 column entities, got %d", columnCount)
	}

	// Verify column entity key format (table.column)
	columnKeyFound := false
	for _, state := range states {
		if state.EntityType == models.WorkflowEntityTypeColumn {
			if state.EntityKey == "users.email" || state.EntityKey == "orders.total" {
				columnKeyFound = true
				break
			}
		}
	}
	if !columnKeyFound {
		t.Error("expected to find column entity with key like 'users.email' or 'orders.total'")
	}

	// Cancel the workflow to clean up background goroutines
	_ = tc.workflowService.Cancel(ctx, workflow.ID)
}

func TestStartExtraction_EmptyDatasource_Integration(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Don't create any tables - datasource is empty

	// Prerequisite: relationship workflow must complete before ontology extraction
	tc.ensureCompletedRelationshipWorkflow(ctx)

	// Start extraction
	config := &models.WorkflowConfig{
		DatasourceID: tc.dsID,
	}

	workflow, err := tc.workflowService.StartExtraction(ctx, tc.projectID, config)
	if err != nil {
		t.Fatalf("StartExtraction failed: %v", err)
	}

	// Verify workflow state rows were created
	states, err := tc.stateRepo.ListByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}

	// Expected: 1 global only (no tables, no columns)
	if len(states) != 1 {
		t.Errorf("expected 1 workflow entity (global only), got %d", len(states))
	}

	if len(states) > 0 {
		if states[0].EntityType != models.WorkflowEntityTypeGlobal {
			t.Errorf("expected global entity, got %s", states[0].EntityType)
		}
		if states[0].Status != models.WorkflowEntityStatusPending {
			t.Errorf("expected status 'pending', got %s", states[0].Status)
		}
	}

	// Cancel the workflow to clean up background goroutines
	_ = tc.workflowService.Cancel(ctx, workflow.ID)
}

func TestStartExtraction_CleansUpPreviousOntologyWorkflowState_Integration(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test table
	usersTable := tc.createTestTable(ctx, "public", "users")
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2)

	// Prerequisite: relationship workflow must complete before ontology extraction
	tc.ensureCompletedRelationshipWorkflow(ctx)

	config := &models.WorkflowConfig{
		DatasourceID: tc.dsID,
	}

	// === First extraction ===
	workflow1, err := tc.workflowService.StartExtraction(ctx, tc.projectID, config)
	if err != nil {
		t.Fatalf("First StartExtraction failed: %v", err)
	}

	// Get the ontology ID for the first extraction
	ontology1, err := tc.ontologyRepo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to get first active ontology: %v", err)
	}
	if ontology1 == nil {
		t.Fatal("Expected active ontology after first extraction")
	}

	// Verify workflow state exists for first ontology
	states1, err := tc.stateRepo.ListByWorkflow(ctx, workflow1.ID)
	if err != nil {
		t.Fatalf("Failed to list workflow states for first extraction: %v", err)
	}
	if len(states1) == 0 {
		t.Fatal("Expected workflow states after first extraction")
	}
	t.Logf("First extraction: %d workflow states created", len(states1))

	// === Second extraction ===
	// This should clean up workflow_state for ontology1 before creating new ontology
	workflow2, err := tc.workflowService.StartExtraction(ctx, tc.projectID, config)
	if err != nil {
		t.Fatalf("Second StartExtraction failed: %v", err)
	}

	// Verify workflow state for first ontology was deleted
	// Note: The first workflow is still running but its workflow_state should be gone
	// because StartExtraction deletes by ontology_id before deactivating.
	statesOld, err := listWorkflowStatesByOntology(ctx, tc, ontology1.ID)
	if err != nil {
		t.Fatalf("Failed to list old workflow states: %v", err)
	}
	if len(statesOld) != 0 {
		t.Errorf("Expected old workflow states to be deleted by StartExtraction, found %d", len(statesOld))
	}

	// Verify new workflow state exists for second extraction
	states2, err := tc.stateRepo.ListByWorkflow(ctx, workflow2.ID)
	if err != nil {
		t.Fatalf("Failed to list workflow states for second extraction: %v", err)
	}
	if len(states2) == 0 {
		t.Fatal("Expected workflow states for second extraction")
	}
	t.Logf("Second extraction: %d new workflow states created", len(states2))

	// Verify the new ontology is different from the old one
	ontology2, err := tc.ontologyRepo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to get second active ontology: %v", err)
	}
	if ontology2 == nil {
		t.Fatal("Expected active ontology after second extraction")
	}
	if ontology2.ID == ontology1.ID {
		t.Error("Expected new ontology ID after second extraction")
	}
	if ontology2.Version <= ontology1.Version {
		t.Errorf("Expected higher version for second ontology, got %d <= %d",
			ontology2.Version, ontology1.Version)
	}

	// Cleanup - cancel both workflows to stop background goroutines
	// Note: Cancel cascade-deletes workflow_state, but that's fine for cleanup
	_ = tc.workflowService.Cancel(ctx, workflow1.ID)
	_ = tc.workflowService.Cancel(ctx, workflow2.ID)
}

// listWorkflowStatesByOntology queries workflow states by ontology_id directly.
// Used for testing since Cancel deletes the workflow but preserves workflow_state.
func listWorkflowStatesByOntology(ctx context.Context, tc *workflowStateTestContext, ontologyID uuid.UUID) ([]*models.WorkflowEntityState, error) {
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		return nil, err
	}
	defer scope.Close()

	var states []*models.WorkflowEntityState
	rows, err := scope.Conn.Query(ctx, `
		SELECT id, project_id, ontology_id, workflow_id, entity_type, entity_key, status
		FROM engine_workflow_state
		WHERE ontology_id = $1
	`, ontologyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s models.WorkflowEntityState
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.OntologyID, &s.WorkflowID,
			&s.EntityType, &s.EntityKey, &s.Status); err != nil {
			return nil, err
		}
		states = append(states, &s)
	}
	return states, rows.Err()
}

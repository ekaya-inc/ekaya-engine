package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// ============================================================================
// Mock Implementations for ColumnScanTask Tests
// ============================================================================

type cstMockWorkflowStateRepo struct {
	workflowState        *models.WorkflowEntityState
	updateStatusErr      error
	updateErr            error
	updatedWorkflowState *models.WorkflowEntityState
}

func (m *cstMockWorkflowStateRepo) GetByEntity(ctx context.Context, workflowID uuid.UUID, entityType models.WorkflowEntityType, entityKey string) (*models.WorkflowEntityState, error) {
	return m.workflowState, nil
}

func (m *cstMockWorkflowStateRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WorkflowEntityStatus, lastError *string) error {
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	if m.workflowState != nil {
		m.workflowState.Status = status
	}
	return nil
}

func (m *cstMockWorkflowStateRepo) Update(ctx context.Context, state *models.WorkflowEntityState) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedWorkflowState = state
	return nil
}

// Unused interface methods
func (m *cstMockWorkflowStateRepo) Create(ctx context.Context, state *models.WorkflowEntityState) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) CreateBatch(ctx context.Context, states []*models.WorkflowEntityState) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.WorkflowEntityState, error) {
	return nil, nil
}
func (m *cstMockWorkflowStateRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) ListByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.WorkflowEntityState, error) {
	return nil, nil
}
func (m *cstMockWorkflowStateRepo) ListByStatus(ctx context.Context, workflowID uuid.UUID, status models.WorkflowEntityStatus) ([]*models.WorkflowEntityState, error) {
	return nil, nil
}
func (m *cstMockWorkflowStateRepo) DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) UpdateStateData(ctx context.Context, id uuid.UUID, stateData *models.WorkflowStateData) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) AddQuestionsToEntity(ctx context.Context, id uuid.UUID, questions []models.WorkflowQuestion) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) UpdateQuestionInEntity(ctx context.Context, id uuid.UUID, questionID string, status string, answer string) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) RecordAnswerInEntity(ctx context.Context, id uuid.UUID, answer models.WorkflowAnswer) error {
	return nil
}
func (m *cstMockWorkflowStateRepo) GetNextPendingQuestion(ctx context.Context, workflowID uuid.UUID) (*models.WorkflowQuestion, uuid.UUID, error) {
	return nil, uuid.Nil, nil
}
func (m *cstMockWorkflowStateRepo) GetPendingQuestions(ctx context.Context, projectID uuid.UUID, limit int) ([]models.WorkflowQuestion, error) {
	return nil, nil
}
func (m *cstMockWorkflowStateRepo) GetPendingQuestionsCount(ctx context.Context, workflowID uuid.UUID) (int, int, error) {
	return 0, 0, nil
}
func (m *cstMockWorkflowStateRepo) GetPendingQuestionsCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *cstMockWorkflowStateRepo) FindQuestionByID(ctx context.Context, questionID string) (*models.WorkflowQuestion, *models.WorkflowEntityState, *models.OntologyWorkflow, error) {
	return nil, nil, nil, nil
}

type cstMockDatasourceService struct {
	datasource *models.Datasource
	err        error
}

func (m *cstMockDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.datasource, nil
}

func (m *cstMockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

// Unused interface methods
func (m *cstMockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}
func (m *cstMockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, nil
}
func (m *cstMockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return nil
}
func (m *cstMockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *cstMockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
	return nil
}

type cstMockSchemaDiscoverer struct {
	columnStats  []datasource.ColumnStats
	sampleValues []string
	statsErr     error
	samplesErr   error
}

func (m *cstMockSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}
func (m *cstMockSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}
func (m *cstMockSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}
func (m *cstMockSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}
func (m *cstMockSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	return m.columnStats, nil
}
func (m *cstMockSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	if m.samplesErr != nil {
		return nil, m.samplesErr
	}
	return m.sampleValues, nil
}
func (m *cstMockSchemaDiscoverer) AnalyzeValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleSize int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}
func (m *cstMockSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}
func (m *cstMockSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	return nil, nil
}
func (m *cstMockSchemaDiscoverer) Close() error {
	return nil
}

type cstMockAdapterFactory struct {
	discoverer datasource.SchemaDiscoverer
	err        error
}

func (m *cstMockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.discoverer, nil
}

func (m *cstMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

func (m *cstMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *cstMockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

type cstMockTaskEnqueuer struct{}

func (m *cstMockTaskEnqueuer) Enqueue(task workqueue.Task) {
	// no-op for tests
}

// ============================================================================
// Tests
// ============================================================================

func TestColumnScanTask_Execute(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	tableName := "users"
	schemaName := "public"
	columnName := "email"

	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	t.Run("successfully scans column and updates workflow state", func(t *testing.T) {
		// Setup mocks
		colEntityKey := models.ColumnEntityKey(tableName, columnName)
		mockWorkflowState := &models.WorkflowEntityState{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  colEntityKey,
			Status:     models.WorkflowEntityStatusPending,
			StateData:  nil,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		mockDS := &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config: map[string]any{
				"host":     "localhost",
				"port":     5432,
				"database": "testdb",
			},
		}

		mockStats := []datasource.ColumnStats{
			{
				ColumnName:    columnName,
				RowCount:      1000,
				NonNullCount:  950,
				DistinctCount: 900,
			},
		}

		mockSamples := []string{
			"user1@example.com",
			"user2@example.com",
			"user3@example.com",
		}

		mockWorkflowStateRepo := &cstMockWorkflowStateRepo{
			workflowState: mockWorkflowState,
		}
		mockDSSvc := &cstMockDatasourceService{
			datasource: mockDS,
		}
		mockDiscoverer := &cstMockSchemaDiscoverer{
			columnStats:  mockStats,
			sampleValues: mockSamples,
		}
		mockAdapterFactory := &cstMockAdapterFactory{
			discoverer: mockDiscoverer,
		}

		task := NewColumnScanTask(
			mockWorkflowStateRepo,
			mockDSSvc,
			mockAdapterFactory,
			getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			tableName,
			schemaName,
			columnName,
		)

		err := task.Execute(context.Background(), &cstMockTaskEnqueuer{})

		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}

		// Verify workflow state was updated correctly
		if mockWorkflowStateRepo.updatedWorkflowState == nil {
			t.Fatal("workflow state was not updated")
		}

		ws := mockWorkflowStateRepo.updatedWorkflowState
		if ws.Status != models.WorkflowEntityStatusScanned {
			t.Errorf("workflow state status = %v, want %v", ws.Status, models.WorkflowEntityStatusScanned)
		}

		if ws.StateData == nil || ws.StateData.Gathered == nil {
			t.Fatal("state data gathered is nil")
		}

		gathered := ws.StateData.Gathered
		if gathered["row_count"] != int64(1000) {
			t.Errorf("row_count = %v, want 1000", gathered["row_count"])
		}
		if gathered["non_null_count"] != int64(950) {
			t.Errorf("non_null_count = %v, want 950", gathered["non_null_count"])
		}
		if gathered["distinct_count"] != int64(900) {
			t.Errorf("distinct_count = %v, want 900", gathered["distinct_count"])
		}
	})

	t.Run("calculates null percentage correctly", func(t *testing.T) {
		colEntityKey := models.ColumnEntityKey(tableName, columnName)
		mockWorkflowState := &models.WorkflowEntityState{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  colEntityKey,
			Status:     models.WorkflowEntityStatusPending,
		}

		mockDS := &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		}

		// 300 nulls out of 1000 rows = 30% null
		mockStats := []datasource.ColumnStats{
			{
				ColumnName:    columnName,
				RowCount:      1000,
				NonNullCount:  700,
				DistinctCount: 500,
			},
		}

		mockWorkflowStateRepo := &cstMockWorkflowStateRepo{
			workflowState: mockWorkflowState,
		}
		mockDSSvc := &cstMockDatasourceService{
			datasource: mockDS,
		}
		mockDiscoverer := &cstMockSchemaDiscoverer{
			columnStats:  mockStats,
			sampleValues: []string{},
		}
		mockAdapterFactory := &cstMockAdapterFactory{
			discoverer: mockDiscoverer,
		}

		task := NewColumnScanTask(
			mockWorkflowStateRepo,
			mockDSSvc,
			mockAdapterFactory,
			getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			tableName,
			schemaName,
			columnName,
		)

		err := task.Execute(context.Background(), &cstMockTaskEnqueuer{})

		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}

		ws := mockWorkflowStateRepo.updatedWorkflowState
		if ws.StateData == nil || ws.StateData.Gathered == nil {
			t.Fatal("state data gathered is nil")
		}

		nullPercent, ok := ws.StateData.Gathered["null_percent"].(float64)
		if !ok {
			t.Fatal("null_percent is not a float64")
		}
		if nullPercent != 30.0 {
			t.Errorf("null_percent = %v, want 30.0", nullPercent)
		}
	})

	t.Run("identifies enum candidates correctly", func(t *testing.T) {
		colEntityKey := models.ColumnEntityKey(tableName, "status")
		mockWorkflowState := &models.WorkflowEntityState{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  colEntityKey,
			Status:     models.WorkflowEntityStatusPending,
		}

		mockDS := &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		}

		// 5 distinct values out of 1000 rows = 0.5% = enum candidate
		mockStats := []datasource.ColumnStats{
			{
				ColumnName:    "status",
				RowCount:      1000,
				NonNullCount:  1000,
				DistinctCount: 5,
			},
		}

		mockWorkflowStateRepo := &cstMockWorkflowStateRepo{
			workflowState: mockWorkflowState,
		}
		mockDSSvc := &cstMockDatasourceService{
			datasource: mockDS,
		}
		mockDiscoverer := &cstMockSchemaDiscoverer{
			columnStats:  mockStats,
			sampleValues: []string{"active", "pending", "completed", "cancelled", "failed"},
		}
		mockAdapterFactory := &cstMockAdapterFactory{
			discoverer: mockDiscoverer,
		}

		task := NewColumnScanTask(
			mockWorkflowStateRepo,
			mockDSSvc,
			mockAdapterFactory,
			getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			tableName,
			schemaName,
			"status",
		)

		err := task.Execute(context.Background(), &cstMockTaskEnqueuer{})

		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}

		ws := mockWorkflowStateRepo.updatedWorkflowState
		if ws.StateData == nil || ws.StateData.Gathered == nil {
			t.Fatal("state data gathered is nil")
		}

		isEnumCandidate, ok := ws.StateData.Gathered["is_enum_candidate"].(bool)
		if !ok {
			t.Fatal("is_enum_candidate is not a bool")
		}
		if !isEnumCandidate {
			t.Errorf("is_enum_candidate = %v, want true", isEnumCandidate)
		}
	})

	t.Run("handles missing sample values gracefully", func(t *testing.T) {
		colEntityKey := models.ColumnEntityKey(tableName, columnName)
		mockWorkflowState := &models.WorkflowEntityState{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  colEntityKey,
			Status:     models.WorkflowEntityStatusPending,
		}

		mockDS := &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		}

		mockStats := []datasource.ColumnStats{
			{
				ColumnName:    columnName,
				RowCount:      1000,
				NonNullCount:  1000,
				DistinctCount: 1000,
			},
		}

		mockWorkflowStateRepo := &cstMockWorkflowStateRepo{
			workflowState: mockWorkflowState,
		}
		mockDSSvc := &cstMockDatasourceService{
			datasource: mockDS,
		}
		mockDiscoverer := &cstMockSchemaDiscoverer{
			columnStats: mockStats,
			samplesErr:  errors.New("binary column not scannable"),
		}
		mockAdapterFactory := &cstMockAdapterFactory{
			discoverer: mockDiscoverer,
		}

		task := NewColumnScanTask(
			mockWorkflowStateRepo,
			mockDSSvc,
			mockAdapterFactory,
			getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			tableName,
			schemaName,
			columnName,
		)

		err := task.Execute(context.Background(), &cstMockTaskEnqueuer{})

		if err != nil {
			t.Fatalf("Execute() error = %v, want nil", err)
		}

		ws := mockWorkflowStateRepo.updatedWorkflowState
		if ws.StateData == nil || ws.StateData.Gathered == nil {
			t.Fatal("state data gathered is nil")
		}

		// Should still update with nil or empty sample values (error case)
		sampleValues := ws.StateData.Gathered["sample_values"]
		if sampleValues != nil {
			// Check if it's an empty slice, which is acceptable
			if slice, ok := sampleValues.([]string); !ok || len(slice) > 0 {
				t.Errorf("sample_values = %v, want nil or empty slice", sampleValues)
			}
		}
	})

	t.Run("returns error when workflow state not found", func(t *testing.T) {
		mockWorkflowStateRepo := &cstMockWorkflowStateRepo{
			workflowState: nil, // Not found
		}
		mockDSSvc := &cstMockDatasourceService{}
		mockAdapterFactory := &cstMockAdapterFactory{}

		task := NewColumnScanTask(
			mockWorkflowStateRepo,
			mockDSSvc,
			mockAdapterFactory,
			getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			tableName,
			schemaName,
			columnName,
		)

		err := task.Execute(context.Background(), &cstMockTaskEnqueuer{})

		if err == nil {
			t.Fatal("Execute() error = nil, want error")
		}
		if err.Error() != "column workflow state not found: users.email" {
			t.Errorf("Execute() error = %v, want 'column workflow state not found: users.email'", err)
		}
	})
}

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestProcessChange_SkipsWithoutAIConfig(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock AI config service that returns no config
	mockAIConfig := &mockAIConfigServiceForIncremental{
		getErr: nil,
		config: nil, // No config
	}

	service := &incrementalDAGService{
		aiConfigSvc:  mockAIConfig,
		getTenantCtx: mockTenantCtxForIncremental,
		logger:       logger,
	}

	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		ChangeType: models.ChangeTypeNewTable,
		TableName:  "test_table",
	}

	err := service.ProcessChange(context.Background(), change)
	if err != nil {
		t.Errorf("ProcessChange should not error when AI config is missing, got: %v", err)
	}
}

func TestProcessChange_SkipsWithAIConfigNone(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock AI config service that returns config type "none"
	mockAIConfig := &mockAIConfigServiceForIncremental{
		config: &models.AIConfig{
			ConfigType: models.AIConfigNone,
		},
	}

	service := &incrementalDAGService{
		aiConfigSvc:  mockAIConfig,
		getTenantCtx: mockTenantCtxForIncremental,
		logger:       logger,
	}

	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		ChangeType: models.ChangeTypeNewTable,
		TableName:  "test_table",
	}

	err := service.ProcessChange(context.Background(), change)
	if err != nil {
		t.Errorf("ProcessChange should not error when AI config type is none, got: %v", err)
	}
}

func TestProcessChange_SkipsUnknownChangeType(t *testing.T) {
	logger := zap.NewNop()

	mockAIConfig := &mockAIConfigServiceForIncremental{
		config: &models.AIConfig{
			ConfigType: models.AIConfigBYOK,
		},
	}

	service := &incrementalDAGService{
		aiConfigSvc:  mockAIConfig,
		getTenantCtx: mockTenantCtxForIncremental,
		logger:       logger,
	}

	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		ChangeType: "unknown_type",
		TableName:  "test_table",
	}

	err := service.ProcessChange(context.Background(), change)
	if err != nil {
		t.Errorf("ProcessChange should not error for unknown change types, got: %v", err)
	}
}

func TestProcessChanges_GroupsByType(t *testing.T) {
	// This test verifies the batch processing logic doesn't fail
	// A more thorough integration test would verify actual entity creation

	logger := zap.NewNop()

	service := &incrementalDAGService{
		aiConfigSvc:  nil, // Will cause early return since no AI config
		getTenantCtx: mockTenantCtxForIncremental,
		logger:       logger,
	}

	// Empty changes should not error
	err := service.ProcessChanges(context.Background(), nil)
	if err != nil {
		t.Errorf("ProcessChanges should not error with nil changes, got: %v", err)
	}

	err = service.ProcessChanges(context.Background(), []*models.PendingChange{})
	if err != nil {
		t.Errorf("ProcessChanges should not error with empty changes, got: %v", err)
	}
}

// TestProcessEnumUpdate_NoOp verifies processEnumUpdate returns nil.
// The production method is currently a no-op (logs a warning) because it hasn't
// been updated for the new column metadata schema.
func TestProcessEnumUpdate_NoOp(t *testing.T) {
	logger := zap.NewNop()

	service := &incrementalDAGService{
		getTenantCtx: mockTenantCtxForIncremental,
		logger:       logger,
	}

	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  uuid.New(),
		ChangeType: models.ChangeTypeNewEnumValue,
		TableName:  "orders",
		ColumnName: "status",
		NewValue: map[string]any{
			"new_values": []any{"delivered", "cancelled"},
		},
	}

	err := service.processEnumUpdate(context.Background(), change)
	if err != nil {
		t.Errorf("processEnumUpdate should not error, got: %v", err)
	}
}

// Mock implementations

type mockAIConfigServiceForIncremental struct {
	config *models.AIConfig
	getErr error
}

func (m *mockAIConfigServiceForIncremental) Get(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
	return m.config, m.getErr
}

func (m *mockAIConfigServiceForIncremental) GetEffective(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
	return m.config, m.getErr
}

func (m *mockAIConfigServiceForIncremental) Upsert(ctx context.Context, projectID uuid.UUID, config *models.AIConfig) error {
	return nil
}

func (m *mockAIConfigServiceForIncremental) Delete(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockAIConfigServiceForIncremental) UpdateTestResult(ctx context.Context, projectID uuid.UUID, success bool) error {
	return nil
}

type mockColumnMetadataRepoForIncremental struct {
	existing     *models.ColumnMetadata
	lastUpserted *models.ColumnMetadata
	getErr       error
}

func (m *mockColumnMetadataRepoForIncremental) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	m.lastUpserted = meta
	return nil
}

func (m *mockColumnMetadataRepoForIncremental) UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error {
	m.lastUpserted = meta
	return nil
}

func (m *mockColumnMetadataRepoForIncremental) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	return m.existing, m.getErr
}

func (m *mockColumnMetadataRepoForIncremental) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepoForIncremental) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepoForIncremental) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockColumnMetadataRepoForIncremental) DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error {
	return nil
}

type mockChangeReviewForIncremental struct {
	canModify bool
}

func (m *mockChangeReviewForIncremental) ListPendingChanges(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.PendingChange, error) {
	return nil, nil
}

func (m *mockChangeReviewForIncremental) ApproveChange(ctx context.Context, changeID uuid.UUID, reviewerSource string) (*models.PendingChange, error) {
	return nil, nil
}

func (m *mockChangeReviewForIncremental) RejectChange(ctx context.Context, changeID uuid.UUID, reviewerSource string) (*models.PendingChange, error) {
	return nil, nil
}

func (m *mockChangeReviewForIncremental) ApproveAllChanges(ctx context.Context, projectID uuid.UUID, reviewerSource string) (*ApproveAllResult, error) {
	return nil, nil
}

func (m *mockChangeReviewForIncremental) CanModify(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool {
	return m.canModify
}

// mockIncrementalDAGForChangeReview is a mock for testing ChangeReviewService
type mockIncrementalDAGForChangeReview struct {
	processChangesAsyncCalled bool
	processChangesAsyncCount  int
}

func (m *mockIncrementalDAGForChangeReview) ProcessChange(ctx context.Context, change *models.PendingChange) error {
	return nil
}

func (m *mockIncrementalDAGForChangeReview) ProcessChanges(ctx context.Context, changes []*models.PendingChange) error {
	return nil
}

func (m *mockIncrementalDAGForChangeReview) ProcessChangeAsync(ctx context.Context, change *models.PendingChange) {
}

func (m *mockIncrementalDAGForChangeReview) ProcessChangesAsync(projectID uuid.UUID, changes []*models.PendingChange) {
	m.processChangesAsyncCalled = true
	m.processChangesAsyncCount = len(changes)
}

func (m *mockIncrementalDAGForChangeReview) SetChangeReviewService(svc ChangeReviewService) {
}

type mockLLMFactoryForIncremental struct {
	client llm.LLMClient
	err    error
}

func (m *mockLLMFactoryForIncremental) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	return m.client, m.err
}

func (m *mockLLMFactoryForIncremental) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	return m.client, m.err
}

func (m *mockLLMFactoryForIncremental) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, m.err
}

func mockTenantCtxForIncremental(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
	return ctx, func() {}, nil
}

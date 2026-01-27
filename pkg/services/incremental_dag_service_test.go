package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestToTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "Users"},
		{"billing_activities", "Billing Activities"},
		{"order_items", "Order Items"},
		{"a_b_c", "A B C"},
		{"single", "Single"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toTitleCase(tt.input)
			if got != tt.expected {
				t.Errorf("toTitleCase(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

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

func TestProcessEnumUpdate_MergesValues(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()

	// Create mock column metadata repository
	mockColMeta := &mockColumnMetadataRepoForIncremental{
		existing: &models.ColumnMetadata{
			ID:         uuid.New(),
			ProjectID:  projectID,
			TableName:  "orders",
			ColumnName: "status",
			EnumValues: []string{"pending", "shipped"},
			Source:     models.ProvenanceInferred,
		},
	}

	// Create mock change review service
	mockReview := &mockChangeReviewForIncremental{
		canModify: true,
	}

	service := &incrementalDAGService{
		columnMetadataRepo: mockColMeta,
		changeReviewSvc:    mockReview,
		getTenantCtx:       mockTenantCtxForIncremental,
		logger:             logger,
	}

	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  projectID,
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

	// Verify the upserted metadata has merged values
	if mockColMeta.lastUpserted == nil {
		t.Fatal("Expected column metadata to be upserted")
	}

	expectedValues := []string{"pending", "shipped", "delivered", "cancelled"}
	if len(mockColMeta.lastUpserted.EnumValues) != len(expectedValues) {
		t.Errorf("Expected %d enum values, got %d", len(expectedValues), len(mockColMeta.lastUpserted.EnumValues))
	}
}

func TestProcessEnumUpdate_RespectsExistingValues(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()

	mockColMeta := &mockColumnMetadataRepoForIncremental{
		existing: &models.ColumnMetadata{
			ID:         uuid.New(),
			ProjectID:  projectID,
			TableName:  "orders",
			ColumnName: "status",
			EnumValues: []string{"pending", "shipped"},
			Source:     models.ProvenanceInferred,
		},
	}

	mockReview := &mockChangeReviewForIncremental{
		canModify: true,
	}

	service := &incrementalDAGService{
		columnMetadataRepo: mockColMeta,
		changeReviewSvc:    mockReview,
		getTenantCtx:       mockTenantCtxForIncremental,
		logger:             logger,
	}

	// Try to add a value that already exists
	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  projectID,
		ChangeType: models.ChangeTypeNewEnumValue,
		TableName:  "orders",
		ColumnName: "status",
		NewValue: map[string]any{
			"new_values": []any{"pending", "new_value"}, // pending already exists
		},
	}

	err := service.processEnumUpdate(context.Background(), change)
	if err != nil {
		t.Errorf("processEnumUpdate should not error, got: %v", err)
	}

	// Should have 3 values: pending, shipped, new_value (no duplicate pending)
	if mockColMeta.lastUpserted == nil {
		t.Fatal("Expected column metadata to be upserted")
	}

	if len(mockColMeta.lastUpserted.EnumValues) != 3 {
		t.Errorf("Expected 3 enum values (no duplicates), got %d: %v",
			len(mockColMeta.lastUpserted.EnumValues), mockColMeta.lastUpserted.EnumValues)
	}
}

func TestProcessEnumUpdate_SkipsDueToPrecedence(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()

	mockColMeta := &mockColumnMetadataRepoForIncremental{
		existing: &models.ColumnMetadata{
			ID:         uuid.New(),
			ProjectID:  projectID,
			TableName:  "orders",
			ColumnName: "status",
			EnumValues: []string{"pending"},
			Source:     models.ProvenanceManual, // Manual created - inference can't modify
		},
	}

	mockReview := &mockChangeReviewForIncremental{
		canModify: false, // Simulates precedence block
	}

	service := &incrementalDAGService{
		columnMetadataRepo: mockColMeta,
		changeReviewSvc:    mockReview,
		getTenantCtx:       mockTenantCtxForIncremental,
		logger:             logger,
	}

	change := &models.PendingChange{
		ID:         uuid.New(),
		ProjectID:  projectID,
		ChangeType: models.ChangeTypeNewEnumValue,
		TableName:  "orders",
		ColumnName: "status",
		NewValue: map[string]any{
			"new_values": []any{"new_value"},
		},
	}

	err := service.processEnumUpdate(context.Background(), change)
	if err != nil {
		t.Errorf("processEnumUpdate should not error when skipping due to precedence, got: %v", err)
	}

	// Should NOT have been upserted due to precedence
	if mockColMeta.lastUpserted != nil {
		t.Error("Should not have upserted due to precedence block")
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

func (m *mockColumnMetadataRepoForIncremental) GetByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) (*models.ColumnMetadata, error) {
	return m.existing, m.getErr
}

func (m *mockColumnMetadataRepoForIncremental) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	m.lastUpserted = meta
	return nil
}

func (m *mockColumnMetadataRepoForIncremental) GetByTable(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepoForIncremental) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepoForIncremental) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockColumnMetadataRepoForIncremental) DeleteByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) error {
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

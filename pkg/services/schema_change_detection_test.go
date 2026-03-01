package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mock Implementations
// ============================================================================

type mockPendingChangeRepo struct {
	changes       []*models.PendingChange
	createBatchFn func(ctx context.Context, changes []*models.PendingChange) error
}

func newMockPendingChangeRepo() *mockPendingChangeRepo {
	return &mockPendingChangeRepo{
		changes: make([]*models.PendingChange, 0),
	}
}

func (m *mockPendingChangeRepo) Create(ctx context.Context, change *models.PendingChange) error {
	change.ID = uuid.New()
	m.changes = append(m.changes, change)
	return nil
}

func (m *mockPendingChangeRepo) CreateBatch(ctx context.Context, changes []*models.PendingChange) error {
	if m.createBatchFn != nil {
		return m.createBatchFn(ctx, changes)
	}
	for _, c := range changes {
		c.ID = uuid.New()
		m.changes = append(m.changes, c)
	}
	return nil
}

func (m *mockPendingChangeRepo) List(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error) {
	return m.changes, nil
}

func (m *mockPendingChangeRepo) ListByType(ctx context.Context, projectID uuid.UUID, changeType string, limit int) ([]*models.PendingChange, error) {
	var filtered []*models.PendingChange
	for _, c := range m.changes {
		if c.ChangeType == changeType {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

func (m *mockPendingChangeRepo) GetByID(ctx context.Context, changeID uuid.UUID) (*models.PendingChange, error) {
	for _, c := range m.changes {
		if c.ID == changeID {
			return c, nil
		}
	}
	return nil, nil
}

func (m *mockPendingChangeRepo) UpdateStatus(ctx context.Context, changeID uuid.UUID, status, reviewedBy string) error {
	return nil
}

func (m *mockPendingChangeRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockPendingChangeRepo) CountByStatus(ctx context.Context, projectID uuid.UUID) (map[string]int, error) {
	counts := make(map[string]int)
	for _, c := range m.changes {
		counts[c.Status]++
	}
	return counts, nil
}

// ============================================================================
// Tests
// ============================================================================

func TestSchemaChangeDetectionService_DetectChanges_NewTables(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{
		NewTableNames: []string{"public.orders", "public.users"},
	}

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Len(t, changes, 2)

	// Verify first new table change
	assert.Equal(t, models.ChangeTypeNewTable, changes[0].ChangeType)
	assert.Equal(t, "public.orders", changes[0].TableName)
	assert.Equal(t, models.SuggestedActionCreateEntity, changes[0].SuggestedAction)
	assert.Equal(t, "Order", changes[0].SuggestedPayload["name"])

	// Verify second new table change
	assert.Equal(t, models.ChangeTypeNewTable, changes[1].ChangeType)
	assert.Equal(t, "public.users", changes[1].TableName)
	assert.Equal(t, "User", changes[1].SuggestedPayload["name"])
}

func TestSchemaChangeDetectionService_DetectChanges_DroppedTables(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{
		RemovedTableNames: []string{"public.legacy_table"},
	}

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Len(t, changes, 1)

	assert.Equal(t, models.ChangeTypeDroppedTable, changes[0].ChangeType)
	assert.Equal(t, "public.legacy_table", changes[0].TableName)
	assert.Equal(t, models.SuggestedActionReviewEntity, changes[0].SuggestedAction)
	assert.Equal(t, models.ChangeStatusAutoApplied, changes[0].Status, "destructive changes should be auto_applied, not pending")
}

func TestSchemaChangeDetectionService_DetectChanges_NewColumns(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{
		NewColumns: []models.RefreshColumnChange{
			{TableName: "public.users", ColumnName: "email", DataType: "varchar"},
			{TableName: "public.users", ColumnName: "phone", DataType: "varchar"},
		},
	}

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Len(t, changes, 2)

	assert.Equal(t, models.ChangeTypeNewColumn, changes[0].ChangeType)
	assert.Equal(t, "public.users", changes[0].TableName)
	assert.Equal(t, "email", changes[0].ColumnName)
	assert.Equal(t, "varchar", changes[0].NewValue["type"])
	assert.Equal(t, models.SuggestedActionCreateColumnMetadata, changes[0].SuggestedAction)
}

func TestSchemaChangeDetectionService_DetectChanges_DroppedColumns(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{
		RemovedColumns: []models.RefreshColumnChange{
			{TableName: "public.users", ColumnName: "deprecated_field", DataType: "text"},
		},
	}

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Len(t, changes, 1)

	assert.Equal(t, models.ChangeTypeDroppedColumn, changes[0].ChangeType)
	assert.Equal(t, "public.users", changes[0].TableName)
	assert.Equal(t, "deprecated_field", changes[0].ColumnName)
	assert.Equal(t, "text", changes[0].OldValue["type"])
	assert.Equal(t, models.SuggestedActionReviewColumn, changes[0].SuggestedAction)
	assert.Equal(t, models.ChangeStatusAutoApplied, changes[0].Status, "destructive changes should be auto_applied, not pending")
}

func TestSchemaChangeDetectionService_DetectChanges_ModifiedColumns(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{
		ModifiedColumns: []models.RefreshColumnModification{
			{TableName: "public.products", ColumnName: "price", OldType: "integer", NewType: "numeric"},
		},
	}

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Len(t, changes, 1)

	assert.Equal(t, models.ChangeTypeModifiedColumn, changes[0].ChangeType)
	assert.Equal(t, "public.products", changes[0].TableName)
	assert.Equal(t, "price", changes[0].ColumnName)
	assert.Equal(t, "integer", changes[0].OldValue["type"])
	assert.Equal(t, "numeric", changes[0].NewValue["type"])
	assert.Equal(t, models.SuggestedActionUpdateColumnMetadata, changes[0].SuggestedAction)
}

func TestSchemaChangeDetectionService_DetectChanges_NoChanges(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{} // Empty result

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Empty(t, changes)
	assert.Empty(t, repo.changes) // Nothing should have been persisted
}

func TestSchemaChangeDetectionService_DetectChanges_NilResult(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()

	changes, err := svc.DetectChanges(context.Background(), projectID, nil)
	require.NoError(t, err)
	assert.Nil(t, changes)
}

func TestSchemaChangeDetectionService_DetectChanges_MixedChanges(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockPendingChangeRepo()
	svc := NewSchemaChangeDetectionService(repo, logger)

	projectID := uuid.New()
	result := &models.RefreshResult{
		NewTableNames:     []string{"public.new_table"},
		RemovedTableNames: []string{"public.old_table"},
		NewColumns: []models.RefreshColumnChange{
			{TableName: "public.existing", ColumnName: "new_col", DataType: "int"},
		},
		RemovedColumns: []models.RefreshColumnChange{
			{TableName: "public.existing", ColumnName: "old_col", DataType: "text"},
		},
		ModifiedColumns: []models.RefreshColumnModification{
			{TableName: "public.existing", ColumnName: "changed", OldType: "int", NewType: "bigint"},
		},
	}

	changes, err := svc.DetectChanges(context.Background(), projectID, result)
	require.NoError(t, err)
	assert.Len(t, changes, 5) // 1 new table + 1 dropped table + 1 new col + 1 dropped col + 1 modified col

	// Verify all change types are represented and statuses are correct
	changeTypes := make(map[string]int)
	for _, c := range changes {
		changeTypes[c.ChangeType]++
	}
	assert.Equal(t, 1, changeTypes[models.ChangeTypeNewTable])
	assert.Equal(t, 1, changeTypes[models.ChangeTypeDroppedTable])
	assert.Equal(t, 1, changeTypes[models.ChangeTypeNewColumn])
	assert.Equal(t, 1, changeTypes[models.ChangeTypeDroppedColumn])
	assert.Equal(t, 1, changeTypes[models.ChangeTypeModifiedColumn])

	// Verify destructive changes are auto_applied, others are pending
	for _, c := range changes {
		switch c.ChangeType {
		case models.ChangeTypeDroppedTable, models.ChangeTypeDroppedColumn:
			assert.Equal(t, models.ChangeStatusAutoApplied, c.Status, "destructive change %s should be auto_applied", c.ChangeType)
		default:
			assert.Equal(t, models.ChangeStatusPending, c.Status, "non-destructive change %s should be pending", c.ChangeType)
		}
	}
}

func TestToEntityName(t *testing.T) {
	tests := []struct {
		tableName    string
		expectedName string
	}{
		// Regular plurals (simple -s removal)
		{"public.users", "User"},
		{"public.orders", "Order"},
		{"products", "Product"},
		{"user_accounts", "User_account"},

		// -ies → -y (consonant + y plurals)
		{"categories", "Category"},
		{"activities", "Activity"},
		{"companies", "Company"},
		{"s4_categories", "S4_category"},
		{"s5_activities", "S5_activity"},

		// -es plurals (s, x, z, ch, sh endings)
		{"boxes", "Box"},
		{"addresses", "Address"},

		// -ves → -f/-fe
		{"knives", "Knife"},

		// Irregular plurals
		{"people", "Person"},

		// Edge cases
		{"", ""},
		{"data", "Datum"}, // inflection library handles this
	}

	for _, tc := range tests {
		t.Run(tc.tableName, func(t *testing.T) {
			result := toEntityName(tc.tableName)
			assert.Equal(t, tc.expectedName, result)
		})
	}
}

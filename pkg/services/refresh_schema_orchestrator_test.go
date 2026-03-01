package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mocks for RefreshSchemaWithChangeDetection tests
// ============================================================================

type stubSchemaService struct {
	SchemaService
	refreshResult *models.RefreshResult
	refreshErr    error
	refreshCalled bool
}

func (s *stubSchemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID, autoSelect bool) (*models.RefreshResult, error) {
	s.refreshCalled = true
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	return s.refreshResult, nil
}

type stubChangeDetectionService struct {
	changes      []*models.PendingChange
	detectErr    error
	detectCalled bool
}

func (s *stubChangeDetectionService) DetectChanges(ctx context.Context, projectID uuid.UUID, refreshResult *models.RefreshResult) ([]*models.PendingChange, error) {
	s.detectCalled = true
	if s.detectErr != nil {
		return nil, s.detectErr
	}
	return s.changes, nil
}

func (s *stubChangeDetectionService) ListPendingChanges(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error) {
	return nil, nil
}

func (s *stubChangeDetectionService) ResolvePendingChanges(ctx context.Context, projectID uuid.UUID, selectedTableNames map[string]bool, selectedColumnNames map[string]bool) (*ResolvedChangesResult, error) {
	return &ResolvedChangesResult{}, nil
}

// ============================================================================
// Tests
// ============================================================================

func TestRefreshSchemaWithChangeDetection_CallsBothServices(t *testing.T) {
	schemaSvc := &stubSchemaService{
		refreshResult: &models.RefreshResult{
			TablesUpserted: 5,
			NewTableNames:  []string{"public.users"},
		},
	}
	changeDetectionSvc := &stubChangeDetectionService{
		changes: []*models.PendingChange{
			{ID: uuid.New()},
			{ID: uuid.New()},
		},
	}

	result, err := RefreshSchemaWithChangeDetection(
		context.Background(),
		schemaSvc,
		changeDetectionSvc,
		zap.NewNop(),
		uuid.New(), uuid.New(),
		true,
	)

	require.NoError(t, err)
	assert.True(t, schemaSvc.refreshCalled, "RefreshDatasourceSchema should be called")
	assert.True(t, changeDetectionSvc.detectCalled, "DetectChanges should be called")
	assert.Equal(t, 5, result.TablesUpserted)
	assert.Equal(t, 2, result.PendingChangesCreated)
	assert.Equal(t, []string{"public.users"}, result.NewTableNames)
}

func TestRefreshSchemaWithChangeDetection_DetectChangesFailsIsNonFatal(t *testing.T) {
	schemaSvc := &stubSchemaService{
		refreshResult: &models.RefreshResult{
			TablesUpserted: 3,
		},
	}
	changeDetectionSvc := &stubChangeDetectionService{
		detectErr: errors.New("detect failed"),
	}

	result, err := RefreshSchemaWithChangeDetection(
		context.Background(),
		schemaSvc,
		changeDetectionSvc,
		zap.NewNop(),
		uuid.New(), uuid.New(),
		false,
	)

	require.NoError(t, err, "change detection failure should not cause overall failure")
	assert.Equal(t, 3, result.TablesUpserted)
	assert.Equal(t, 0, result.PendingChangesCreated)
}

func TestRefreshSchemaWithChangeDetection_NoChangesStillDetects(t *testing.T) {
	schemaSvc := &stubSchemaService{
		refreshResult: &models.RefreshResult{
			TablesUpserted: 0,
		},
	}
	changeDetectionSvc := &stubChangeDetectionService{
		changes: nil,
	}

	result, err := RefreshSchemaWithChangeDetection(
		context.Background(),
		schemaSvc,
		changeDetectionSvc,
		zap.NewNop(),
		uuid.New(), uuid.New(),
		true,
	)

	require.NoError(t, err)
	assert.True(t, changeDetectionSvc.detectCalled, "DetectChanges should still be called even with no schema changes")
	assert.Equal(t, 0, result.PendingChangesCreated)
}

func TestRefreshSchemaWithChangeDetection_ReturnsRefreshResultAndPendingCount(t *testing.T) {
	schemaSvc := &stubSchemaService{
		refreshResult: &models.RefreshResult{
			TablesUpserted:       10,
			TablesDeleted:        2,
			ColumnsUpserted:      50,
			ColumnsDeleted:       5,
			RelationshipsCreated: 8,
			RelationshipsDeleted: 1,
			NewTableNames:        []string{"public.orders"},
			RemovedTableNames:    []string{"public.old_table"},
		},
	}
	changeDetectionSvc := &stubChangeDetectionService{
		changes: []*models.PendingChange{
			{ID: uuid.New()},
			{ID: uuid.New()},
			{ID: uuid.New()},
		},
	}

	result, err := RefreshSchemaWithChangeDetection(
		context.Background(),
		schemaSvc,
		changeDetectionSvc,
		zap.NewNop(),
		uuid.New(), uuid.New(),
		true,
	)

	require.NoError(t, err)
	assert.Equal(t, 10, result.TablesUpserted)
	assert.Equal(t, int64(2), result.TablesDeleted)
	assert.Equal(t, 50, result.ColumnsUpserted)
	assert.Equal(t, int64(5), result.ColumnsDeleted)
	assert.Equal(t, 8, result.RelationshipsCreated)
	assert.Equal(t, int64(1), result.RelationshipsDeleted)
	assert.Equal(t, 3, result.PendingChangesCreated)
	assert.Equal(t, []string{"public.orders"}, result.NewTableNames)
	assert.Equal(t, []string{"public.old_table"}, result.RemovedTableNames)
}

func TestRefreshSchemaWithChangeDetection_NilChangeDetectionService(t *testing.T) {
	schemaSvc := &stubSchemaService{
		refreshResult: &models.RefreshResult{
			TablesUpserted: 5,
		},
	}

	result, err := RefreshSchemaWithChangeDetection(
		context.Background(),
		schemaSvc,
		nil,
		zap.NewNop(),
		uuid.New(), uuid.New(),
		true,
	)

	require.NoError(t, err)
	assert.Equal(t, 5, result.TablesUpserted)
	assert.Equal(t, 0, result.PendingChangesCreated)
}

func TestRefreshSchemaWithChangeDetection_RefreshFails(t *testing.T) {
	schemaSvc := &stubSchemaService{
		refreshErr: errors.New("database connection failed"),
	}
	changeDetectionSvc := &stubChangeDetectionService{}

	result, err := RefreshSchemaWithChangeDetection(
		context.Background(),
		schemaSvc,
		changeDetectionSvc,
		zap.NewNop(),
		uuid.New(), uuid.New(),
		true,
	)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.False(t, changeDetectionSvc.detectCalled, "DetectChanges should not be called when refresh fails")
}

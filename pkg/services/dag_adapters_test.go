package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Mock GlossaryService for testing adapters
type mockGlossaryService struct {
	discoverFunc func(ctx context.Context, projectID uuid.UUID) (int, error)
	enrichFunc   func(ctx context.Context, projectID uuid.UUID) error
}

func (m *mockGlossaryService) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	return nil
}

func (m *mockGlossaryService) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	return nil
}

func (m *mockGlossaryService) DeleteTerm(ctx context.Context, termID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryService) GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}

func (m *mockGlossaryService) GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	return nil, nil
}

func (m *mockGlossaryService) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	return nil, nil
}

func (m *mockGlossaryService) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*SQLTestResult, error) {
	return nil, nil
}

func (m *mockGlossaryService) CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}

func (m *mockGlossaryService) DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}

func (m *mockGlossaryService) SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}

func (m *mockGlossaryService) DiscoverGlossaryTerms(ctx context.Context, projectID uuid.UUID) (int, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, projectID)
	}
	return 0, nil
}

func (m *mockGlossaryService) EnrichGlossaryTerms(ctx context.Context, projectID uuid.UUID) error {
	if m.enrichFunc != nil {
		return m.enrichFunc(ctx, projectID)
	}
	return nil
}

func (m *mockGlossaryService) GetGenerationStatus(projectID uuid.UUID) *models.GlossaryGenerationStatus {
	return &models.GlossaryGenerationStatus{Status: "idle", Message: "No generation in progress"}
}

func (m *mockGlossaryService) RunAutoGenerate(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// GlossaryDiscoveryAdapter Tests

func TestGlossaryDiscoveryAdapter_DiscoverGlossaryTerms_Success(t *testing.T) {
	projectID := uuid.New()
	expectedCount := 5

	mock := &mockGlossaryService{
		discoverFunc: func(ctx context.Context, pid uuid.UUID) (int, error) {
			assert.Equal(t, projectID, pid)
			return expectedCount, nil
		},
	}

	adapter := NewGlossaryDiscoveryAdapter(mock)
	count, err := adapter.DiscoverGlossaryTerms(context.Background(), projectID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)
}

func TestGlossaryDiscoveryAdapter_DiscoverGlossaryTerms_Error(t *testing.T) {
	projectID := uuid.New()
	expectedErr := errors.New("discovery failed")

	mock := &mockGlossaryService{
		discoverFunc: func(ctx context.Context, pid uuid.UUID) (int, error) {
			return 0, expectedErr
		},
	}

	adapter := NewGlossaryDiscoveryAdapter(mock)
	count, err := adapter.DiscoverGlossaryTerms(context.Background(), projectID)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 0, count)
}

func TestGlossaryDiscoveryAdapter_DiscoverGlossaryTerms_ZeroTerms(t *testing.T) {
	projectID := uuid.New()

	mock := &mockGlossaryService{
		discoverFunc: func(ctx context.Context, pid uuid.UUID) (int, error) {
			return 0, nil
		},
	}

	adapter := NewGlossaryDiscoveryAdapter(mock)
	count, err := adapter.DiscoverGlossaryTerms(context.Background(), projectID)

	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

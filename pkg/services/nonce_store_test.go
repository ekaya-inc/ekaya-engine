package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockNonceRepository struct {
	createFunc             func(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string, expiresAt time.Time) error
	validateAndConsumeFunc func(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string) (bool, error)
	deleteExpiredFunc      func(ctx context.Context) (int64, error)
}

func (m *mockNonceRepository) Create(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string, expiresAt time.Time) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, nonce, action, projectID, appID, expiresAt)
	}
	return nil
}

func (m *mockNonceRepository) ValidateAndConsume(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string) (bool, error) {
	if m.validateAndConsumeFunc != nil {
		return m.validateAndConsumeFunc(ctx, nonce, action, projectID, appID)
	}
	return false, nil
}

func (m *mockNonceRepository) DeleteExpired(ctx context.Context) (int64, error) {
	if m.deleteExpiredFunc != nil {
		return m.deleteExpiredFunc(ctx)
	}
	return 0, nil
}

func TestNonceStore_Generate_ReturnsUniqueNonces(t *testing.T) {
	repo := &mockNonceRepository{}
	store := NewNonceStore(repo, 15*time.Minute)
	seen := make(map[string]bool)
	projectID := uuid.New().String()

	for i := 0; i < 100; i++ {
		nonce, err := store.Generate(context.Background(), "install", projectID, "app-1")
		require.NoError(t, err)
		require.False(t, seen[nonce], "duplicate nonce generated")
		seen[nonce] = true
	}
}

func TestNonceStore_Generate_PersistsExpectedTuple(t *testing.T) {
	projectID := uuid.New()
	var (
		createdNonce     string
		createdAction    string
		createdProjectID uuid.UUID
		createdAppID     string
		createdExpiresAt time.Time
	)
	repo := &mockNonceRepository{
		createFunc: func(_ context.Context, nonce, action string, persistedProjectID uuid.UUID, appID string, expiresAt time.Time) error {
			createdNonce = nonce
			createdAction = action
			createdProjectID = persistedProjectID
			createdAppID = appID
			createdExpiresAt = expiresAt
			return nil
		},
	}
	store := NewNonceStore(repo, 10*time.Minute)

	before := time.Now()
	nonce, err := store.Generate(context.Background(), "activate", projectID.String(), "app-1")
	after := time.Now()

	require.NoError(t, err)
	assert.NotEmpty(t, nonce)
	assert.Equal(t, nonce, createdNonce)
	assert.Equal(t, "activate", createdAction)
	assert.Equal(t, projectID, createdProjectID)
	assert.Equal(t, "app-1", createdAppID)
	assert.WithinDuration(t, before.Add(10*time.Minute), createdExpiresAt, time.Second)
	assert.WithinDuration(t, after.Add(10*time.Minute), createdExpiresAt, time.Second)
}

func TestNonceStore_Generate_RetriesUniqueViolation(t *testing.T) {
	projectID := uuid.New()
	attempts := 0
	repo := &mockNonceRepository{
		createFunc: func(_ context.Context, nonce, action string, persistedProjectID uuid.UUID, appID string, expiresAt time.Time) error {
			attempts++
			if attempts == 1 {
				return &pgconn.PgError{Code: "23505"}
			}
			return nil
		},
	}
	store := NewNonceStore(repo, time.Minute)

	nonce, err := store.Generate(context.Background(), "install", projectID.String(), "app-1")
	require.NoError(t, err)
	assert.NotEmpty(t, nonce)
	assert.Equal(t, 2, attempts)
}

func TestNonceStore_Generate_FailsWithInvalidProjectID(t *testing.T) {
	store := NewNonceStore(&mockNonceRepository{}, time.Minute)

	_, err := store.Generate(context.Background(), "install", "not-a-uuid", "app-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project ID")
}

func TestNonceStore_Validate_SucceedsWithCorrectTuple(t *testing.T) {
	projectID := uuid.New()
	repo := &mockNonceRepository{
		validateAndConsumeFunc: func(_ context.Context, nonce, action string, persistedProjectID uuid.UUID, appID string) (bool, error) {
			assert.Equal(t, "nonce-1", nonce)
			assert.Equal(t, "activate", action)
			assert.Equal(t, projectID, persistedProjectID)
			assert.Equal(t, "app-1", appID)
			return true, nil
		},
	}
	store := NewNonceStore(repo, time.Minute)

	ok, err := store.Validate(context.Background(), "nonce-1", "activate", projectID.String(), "app-1")
	require.NoError(t, err)
	assert.True(t, ok, "first validation should succeed")
}

func TestNonceStore_Validate_PropagatesRepositoryResult(t *testing.T) {
	projectID := uuid.New()
	store := NewNonceStore(&mockNonceRepository{
		validateAndConsumeFunc: func(_ context.Context, nonce, action string, persistedProjectID uuid.UUID, appID string) (bool, error) {
			return false, errors.New("boom")
		},
	}, time.Minute)

	ok, err := store.Validate(context.Background(), "nonce-1", "install", projectID.String(), "app-1")
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, "boom")
}

func TestNonceStore_Validate_FailsWithInvalidProjectID(t *testing.T) {
	store := NewNonceStore(&mockNonceRepository{}, time.Minute)

	_, err := store.Validate(context.Background(), "nonce-1", "install", "not-a-uuid", "app-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project ID")
}

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestWithInferredProvenanceWrapper_AddsProvenance(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()

	inner := func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	wrapped := WithInferredProvenanceWrapper(inner, userID)
	resultCtx, cleanup, err := wrapped(context.Background(), projectID)

	require.NoError(t, err)
	require.NotNil(t, cleanup)

	prov, ok := models.GetProvenance(resultCtx)
	require.True(t, ok, "provenance should be present in context")
	assert.Equal(t, models.SourceInferred, prov.Source)
	assert.Equal(t, userID, prov.UserID)
}

func TestWithInferredProvenanceWrapper_PassesThroughCleanup(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()
	cleanupCalled := false

	inner := func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		return ctx, func() { cleanupCalled = true }, nil
	}

	wrapped := WithInferredProvenanceWrapper(inner, userID)
	_, cleanup, err := wrapped(context.Background(), projectID)

	require.NoError(t, err)
	require.NotNil(t, cleanup)

	cleanup()
	assert.True(t, cleanupCalled, "cleanup from inner function should be called")
}

func TestWithInferredProvenanceWrapper_PropagatesError(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()
	expectedErr := errors.New("tenant connection failed")

	inner := func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		return nil, nil, expectedErr
	}

	wrapped := WithInferredProvenanceWrapper(inner, userID)
	resultCtx, cleanup, err := wrapped(context.Background(), projectID)

	assert.Nil(t, resultCtx)
	assert.Nil(t, cleanup)
	assert.ErrorIs(t, err, expectedErr)
}

func TestWithInferredProvenanceWrapper_ForwardsProjectID(t *testing.T) {
	userID := uuid.New()
	projectID := uuid.New()
	var receivedProjectID uuid.UUID

	inner := func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		receivedProjectID = pid
		return ctx, func() {}, nil
	}

	wrapped := WithInferredProvenanceWrapper(inner, userID)
	_, _, err := wrapped(context.Background(), projectID)

	require.NoError(t, err)
	assert.Equal(t, projectID, receivedProjectID, "projectID should be forwarded to inner function")
}

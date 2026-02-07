package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestQueryHistoryService_PrivacyScoping_UserOnlySeesOwnQueries(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	projectID := uuid.New()
	userA := "user-a"
	userB := "user-b"
	now := time.Now()

	// Add entries for two different users
	repo.entries = append(repo.entries,
		&models.QueryHistoryEntry{
			ID:              uuid.New(),
			ProjectID:       projectID,
			UserID:          userA,
			NaturalLanguage: "User A query 1",
			SQL:             "SELECT 1",
			CreatedAt:       now,
		},
		&models.QueryHistoryEntry{
			ID:              uuid.New(),
			ProjectID:       projectID,
			UserID:          userA,
			NaturalLanguage: "User A query 2",
			SQL:             "SELECT 2",
			CreatedAt:       now,
		},
		&models.QueryHistoryEntry{
			ID:              uuid.New(),
			ProjectID:       projectID,
			UserID:          userB,
			NaturalLanguage: "User B query",
			SQL:             "SELECT 3",
			CreatedAt:       now,
		},
	)

	// User A should only see their 2 queries
	entries, total, err := svc.List(context.Background(), projectID, models.QueryHistoryFilters{
		UserID: userA,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, userA, e.UserID)
	}

	// User B should only see their 1 query
	entries, total, err = svc.List(context.Background(), projectID, models.QueryHistoryFilters{
		UserID: userB,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, userB, entries[0].UserID)
}

func TestQueryHistoryService_PrivacyScoping_FeedbackOnlyOwnEntries(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	projectID := uuid.New()
	entryID := uuid.New()
	ownerUser := "owner"
	otherUser := "other"

	repo.entries = append(repo.entries, &models.QueryHistoryEntry{
		ID:              entryID,
		ProjectID:       projectID,
		UserID:          ownerUser,
		NaturalLanguage: "test query",
		SQL:             "SELECT 1",
		CreatedAt:       time.Now(),
	})

	// Owner can record feedback
	err := svc.RecordFeedback(context.Background(), projectID, entryID, ownerUser, "helpful", nil)
	require.NoError(t, err)

	// Other user cannot record feedback on someone else's entry
	err = svc.RecordFeedback(context.Background(), projectID, entryID, otherUser, "not_helpful", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestQueryHistoryService_PrivacyScoping_CrossProjectIsolation(t *testing.T) {
	repo := newMockQueryHistoryRepo()
	svc := NewQueryHistoryService(repo, zap.NewNop())

	projectA := uuid.New()
	projectB := uuid.New()
	user := "user-1"
	now := time.Now()

	repo.entries = append(repo.entries,
		&models.QueryHistoryEntry{
			ID:              uuid.New(),
			ProjectID:       projectA,
			UserID:          user,
			NaturalLanguage: "Project A query",
			SQL:             "SELECT 1",
			CreatedAt:       now,
		},
		&models.QueryHistoryEntry{
			ID:              uuid.New(),
			ProjectID:       projectB,
			UserID:          user,
			NaturalLanguage: "Project B query",
			SQL:             "SELECT 2",
			CreatedAt:       now,
		},
	)

	// Query for project A should only return project A entries
	entries, total, err := svc.List(context.Background(), projectA, models.QueryHistoryFilters{
		UserID: user,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, projectA, entries[0].ProjectID)

	// Query for project B should only return project B entries
	entries, total, err = svc.List(context.Background(), projectB, models.QueryHistoryFilters{
		UserID: user,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, projectB, entries[0].ProjectID)
}

func TestDefaultRetentionDays(t *testing.T) {
	// Verify the constant is 90 as specified in the plan
	assert.Equal(t, 90, DefaultRetentionDays)
}

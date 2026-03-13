package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

type mockAgentQueryAccessService struct {
	queryIDs     []uuid.UUID
	hasAccess    bool
	getAccessErr error
	hasAccessErr error
}

func (m *mockAgentQueryAccessService) GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error) {
	if m.getAccessErr != nil {
		return nil, m.getAccessErr
	}
	return append([]uuid.UUID(nil), m.queryIDs...), nil
}

func (m *mockAgentQueryAccessService) HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error) {
	if m.hasAccessErr != nil {
		return false, m.hasAccessErr
	}
	return m.hasAccess, nil
}

func TestFilterQueriesForAgent_FiltersToAssignedQueries(t *testing.T) {
	allowedQueryID := uuid.New()
	blockedQueryID := uuid.New()
	agentID := uuid.New()

	ctx := context.WithValue(context.Background(), auth.AgentIDKey, agentID)

	filtered, err := filterQueriesForAgent(ctx, &mockAgentQueryAccessService{
		queryIDs: []uuid.UUID{allowedQueryID},
	}, []*models.Query{
		{ID: allowedQueryID, NaturalLanguagePrompt: "Allowed"},
		{ID: blockedQueryID, NaturalLanguagePrompt: "Blocked"},
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, allowedQueryID, filtered[0].ID)
}

func TestFilterQueriesForAgent_LegacyAgentLeavesQueriesUnchanged(t *testing.T) {
	queryID := uuid.New()

	filtered, err := filterQueriesForAgent(context.Background(), nil, []*models.Query{
		{ID: queryID, NaturalLanguagePrompt: "All queries"},
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, queryID, filtered[0].ID)
}

func TestAgentHasQueryAccess_DeniesUnassignedQuery(t *testing.T) {
	agentID := uuid.New()
	queryID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.AgentIDKey, agentID)

	allowed, err := agentHasQueryAccess(ctx, &mockAgentQueryAccessService{
		hasAccess: false,
	}, queryID)
	require.NoError(t, err)
	assert.False(t, allowed)
}

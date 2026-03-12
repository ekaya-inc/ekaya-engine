//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

type agentRepositoryTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	agentRepo AgentRepository
	queryRepo QueryRepository
	projectID uuid.UUID
	dsID      uuid.UUID
}

func setupAgentRepositoryTest(t *testing.T) *agentRepositoryTestContext {
	t.Helper()

	tc := &agentRepositoryTestContext{
		t:         t,
		engineDB:  testhelpers.GetEngineDB(t),
		agentRepo: NewAgentRepository(),
		queryRepo: NewQueryRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000121"),
		dsID:      uuid.MustParse("00000000-0000-0000-0000-000000000122"),
	}

	tc.ensureTestProject()
	tc.ensureTestDatasource()

	return tc
}

func (tc *agentRepositoryTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	require.NoError(tc.t, err)

	return database.SetTenantScope(ctx, scope), func() {
		scope.Close()
	}
}

func (tc *agentRepositoryTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING`,
		tc.projectID,
		"Agent Repository Test Project",
	)
	require.NoError(tc.t, err)
}

func (tc *agentRepositoryTestContext) ensureTestDatasource() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING`,
		tc.dsID,
		tc.projectID,
		"Agent Test Datasource",
		"postgres",
		"{}",
	)
	require.NoError(tc.t, err)
}

func (tc *agentRepositoryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_agent_queries WHERE agent_id IN (SELECT id FROM engine_agents WHERE project_id = $1)`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_agents WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_queries WHERE project_id = $1`, tc.projectID)
}

func (tc *agentRepositoryTestContext) createApprovedQuery(ctx context.Context, name string) *models.Query {
	tc.t.Helper()

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: name,
		SQLQuery:              "SELECT * FROM users",
		Dialect:               "postgres",
		IsEnabled:             true,
		Status:                "approved",
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{},
	}
	require.NoError(tc.t, tc.queryRepo.Create(ctx, query))
	return query
}

func (tc *agentRepositoryTestContext) createPendingQuery(ctx context.Context, name string) *models.Query {
	tc.t.Helper()

	query := &models.Query{
		ProjectID:             tc.projectID,
		DatasourceID:          tc.dsID,
		NaturalLanguagePrompt: name,
		SQLQuery:              "SELECT * FROM users",
		Dialect:               "postgres",
		IsEnabled:             false,
		Status:                "pending",
		Parameters:            []models.QueryParameter{},
		OutputColumns:         []models.OutputColumn{},
	}
	require.NoError(tc.t, tc.queryRepo.Create(ctx, query))
	return query
}

func TestAgentRepository_CreateListGetDelete(t *testing.T) {
	tc := setupAgentRepositoryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	agent := &models.Agent{
		ProjectID:       tc.projectID,
		Name:            "sales-bot",
		APIKeyEncrypted: "encrypted-key",
	}

	require.NoError(t, tc.agentRepo.Create(ctx, agent))
	require.NotEqual(t, uuid.Nil, agent.ID)

	retrieved, err := tc.agentRepo.GetByID(ctx, tc.projectID, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "sales-bot", retrieved.Name)
	assert.Equal(t, "encrypted-key", retrieved.APIKeyEncrypted)

	agents, err := tc.agentRepo.ListByProject(ctx, tc.projectID)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, agent.ID, agents[0].ID)

	require.NoError(t, tc.agentRepo.Delete(ctx, tc.projectID, agent.ID))
	_, err = tc.agentRepo.GetByID(ctx, tc.projectID, agent.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAgentRepository_SetQueryAccess_OnlyAllowsApprovedQueries(t *testing.T) {
	tc := setupAgentRepositoryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	allowedQuery := tc.createApprovedQuery(ctx, "Allowed query")
	pendingQuery := tc.createPendingQuery(ctx, "Pending query")

	agent := &models.Agent{
		ProjectID:       tc.projectID,
		Name:            "finance-bot",
		APIKeyEncrypted: "encrypted-key",
	}
	require.NoError(t, tc.agentRepo.Create(ctx, agent))

	require.NoError(t, tc.agentRepo.SetQueryAccess(ctx, agent.ID, []uuid.UUID{allowedQuery.ID}))

	queryIDs, err := tc.agentRepo.GetQueryAccess(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{allowedQuery.ID}, queryIDs)

	allowed, err := tc.agentRepo.HasQueryAccess(ctx, agent.ID, allowedQuery.ID)
	require.NoError(t, err)
	assert.True(t, allowed)

	denied, err := tc.agentRepo.HasQueryAccess(ctx, agent.ID, pendingQuery.ID)
	require.NoError(t, err)
	assert.False(t, denied)

	err = tc.agentRepo.SetQueryAccess(ctx, agent.ID, []uuid.UUID{pendingQuery.ID})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

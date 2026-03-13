//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

type auditPageTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      AuditPageRepository
	projectID uuid.UUID
	userID    uuid.UUID
	agentID   uuid.UUID
}

func setupAuditPageTest(t *testing.T) *auditPageTestContext {
	t.Helper()

	return &auditPageTestContext{
		t:         t,
		engineDB:  testhelpers.GetEngineDB(t),
		repo:      NewAuditPageRepository(),
		projectID: uuid.New(),
		userID:    uuid.New(),
		agentID:   uuid.New(),
	}
}

func (tc *auditPageTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func (tc *auditPageTestContext) setupFixtures() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
	`, tc.projectID, "Audit Page Test Project")
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_users (project_id, user_id, role, email)
		VALUES ($1, $2, 'admin', $3)
	`, tc.projectID, tc.userID, "orange.chicken.511@example.com")
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_agents (id, project_id, name, api_key_encrypted)
		VALUES ($1, $2, $3, $4)
	`, tc.agentID, tc.projectID, "support-bot", []byte("encrypted-key"))
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_query_executions (
			id, project_id, query_id, sql, executed_at, row_count,
			execution_time_ms, user_id, source, is_modifying, success
		) VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11),
			($12, $2, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`,
		uuid.New(), tc.projectID, uuid.New(), "SELECT 1", time.Now().UTC().Add(-time.Minute), 1, 5, tc.userID.String(), "mcp", false, true,
		uuid.New(), uuid.New(), "SELECT 2", time.Now().UTC(), 1, 9, "agent:"+tc.agentID.String(), "mcp", false, true,
	)
	require.NoError(tc.t, err)
}

func (tc *auditPageTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_query_executions WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_audit_log WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_agents WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_users WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_projects WHERE id = $1`, tc.projectID)
}

func TestAuditPageRepository_ListQueryExecutions_ResolvesAgentNames(t *testing.T) {
	tc := setupAuditPageTest(t)
	tc.setupFixtures()
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	rows, total, err := tc.repo.ListQueryExecutions(ctx, tc.projectID, models.QueryExecutionFilters{})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, rows, 2)

	var humanRow *models.QueryExecutionRow
	var agentRow *models.QueryExecutionRow
	for _, row := range rows {
		require.NotNil(t, row.UserID)
		switch *row.UserID {
		case tc.userID.String():
			humanRow = row
		case "agent:" + tc.agentID.String():
			agentRow = row
		}
	}

	require.NotNil(t, humanRow)
	require.NotNil(t, agentRow)
	require.NotNil(t, humanRow.UserEmail)
	assert.Equal(t, "orange.chicken.511@example.com", *humanRow.UserEmail)
	require.NotNil(t, agentRow.UserEmail)
	assert.Equal(t, "agent:support-bot", *agentRow.UserEmail)
}

func TestAuditPageRepository_ListQueryExecutions_FiltersByUserSubstring(t *testing.T) {
	tc := setupAuditPageTest(t)
	tc.setupFixtures()
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	humanRows, total, err := tc.repo.ListQueryExecutions(ctx, tc.projectID, models.QueryExecutionFilters{
		UserID: "chicken",
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, humanRows, 1)
	require.NotNil(t, humanRows[0].UserEmail)
	assert.Equal(t, "orange.chicken.511@example.com", *humanRows[0].UserEmail)

	agentRows, total, err := tc.repo.ListQueryExecutions(ctx, tc.projectID, models.QueryExecutionFilters{
		UserID: "sup",
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, agentRows, 1)
	require.NotNil(t, agentRows[0].UserEmail)
	assert.Equal(t, "agent:support-bot", *agentRows[0].UserEmail)
}

func TestAuditPageRepository_ListOntologyChanges_FiltersByUserSubstring(t *testing.T) {
	tc := setupAuditPageTest(t)
	tc.setupFixtures()
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok)

	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_audit_log (
			id, project_id, entity_type, entity_id, action, source, user_id, created_at
		) VALUES
			($1, $2, 'entity', $3, 'update', 'manual', $4, $5),
			($6, $2, 'entity', $7, 'update', 'manual', $8, $9)
	`,
		uuid.New(), tc.projectID, uuid.New(), tc.userID, time.Now().UTC().Add(-time.Minute),
		uuid.New(), uuid.New(), uuid.New(), time.Now().UTC(),
	)
	require.NoError(t, err)

	rows, total, err := tc.repo.ListOntologyChanges(ctx, tc.projectID, models.OntologyChangeFilters{
		UserID: "chicken",
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].UserEmail)
	assert.Equal(t, "orange.chicken.511@example.com", *rows[0].UserEmail)
}

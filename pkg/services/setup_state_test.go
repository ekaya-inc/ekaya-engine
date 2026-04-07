//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func ensureSetupStateTestProject(t *testing.T, engineDB *testhelpers.EngineDB, projectID uuid.UUID) {
	t.Helper()

	scope, err := engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(context.Background(), `
		INSERT INTO engine_projects (id, name, status, parameters)
		VALUES ($1, $2, 'active', '{}'::jsonb)
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Setup State Integration Test Project")
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupScope, cleanupErr := engineDB.DB.WithoutTenant(context.Background())
		require.NoError(t, cleanupErr)
		defer cleanupScope.Close()

		_, cleanupErr = cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_projects WHERE id = $1`, projectID)
		require.NoError(t, cleanupErr)
	})
}

func readSetupStateSteps(t *testing.T, engineDB *testhelpers.EngineDB, projectID uuid.UUID) map[string]bool {
	t.Helper()

	scope, err := engineDB.DB.WithTenant(context.Background(), projectID)
	require.NoError(t, err)
	defer scope.Close()

	parameters, err := loadProjectParameters(context.Background(), scope.Conn, projectID)
	require.NoError(t, err)

	steps, present, err := loadSetupSteps(parameters)
	require.NoError(t, err)
	require.True(t, present)

	return steps
}

func TestSetupStateService_EnsureAppSteps_SeedsGlossaryStepFromLiveState(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	ensureSetupStateTestProject(t, engineDB, projectID)

	glossaryRepo := newMockGlossaryRepo()
	termID := uuid.New()
	glossaryRepo.terms[termID] = &models.BusinessGlossaryTerm{
		ID:        termID,
		ProjectID: projectID,
		Term:      "Revenue",
	}

	installedApps := newMockInstalledAppRepository()
	queryRepo := &mockQueryRepository{}
	service := NewSetupStateService(
		engineDB.DB,
		&mockDatasourceSvc{},
		nil,
		nil,
		nil,
		nil,
		queryRepo,
		glossaryRepo,
		installedApps,
		zap.NewNop(),
	)

	err := service.EnsureInitialized(context.Background(), projectID)
	require.NoError(t, err)

	installedApps.apps[installedApps.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
	}

	err = service.EnsureAppSteps(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)

	steps := readSetupStateSteps(t, engineDB, projectID)
	assert.True(t, steps[SetupStepGlossarySetup])
	assert.False(t, steps[SetupStepADLActivated])
}

func TestSetupStateService_EnsureAppSteps_SeedsAgentQueryStepFromLiveState(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	ensureSetupStateTestProject(t, engineDB, projectID)

	installedApps := newMockInstalledAppRepository()
	queryRepo := &mockQueryRepository{hasApprovedEnabledQueries: true}
	service := NewSetupStateService(
		engineDB.DB,
		&mockDatasourceSvc{},
		nil,
		nil,
		nil,
		nil,
		queryRepo,
		newMockGlossaryRepo(),
		installedApps,
		zap.NewNop(),
	)

	err := service.EnsureInitialized(context.Background(), projectID)
	require.NoError(t, err)

	installedApps.apps[installedApps.key(projectID, models.AppIDAIAgents)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIAgents,
	}

	err = service.EnsureAppSteps(context.Background(), projectID, models.AppIDAIAgents)
	require.NoError(t, err)

	steps := readSetupStateSteps(t, engineDB, projectID)
	assert.True(t, steps[SetupStepAgentsQueriesCreated])
}

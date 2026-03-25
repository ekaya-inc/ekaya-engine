//go:build integration

package database_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver for database/sql in migration test
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	migrationfs "github.com/ekaya-inc/ekaya-engine/migrations"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func Test_019_MCPConfigToolGroupsCleanup(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)

	dbName := fmt.Sprintf("test_mcp_config_cleanup_%d", time.Now().UnixNano())

	_, err := testDB.Pool.Exec(t.Context(), "CREATE DATABASE "+dbName)
	require.NoError(t, err, "Failed to create test database")

	defer func() {
		_, _ = testDB.Pool.Exec(t.Context(), `
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = $1
			AND pid <> pg_backend_pid()
		`, dbName)
		time.Sleep(100 * time.Millisecond)
		_, _ = testDB.Pool.Exec(t.Context(), "DROP DATABASE IF EXISTS "+dbName)
	}()

	host, err := testDB.Container.Host(t.Context())
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(t.Context(), "5432")
	require.NoError(t, err)

	connStr := fmt.Sprintf("postgres://ekaya:test_password@%s:%s/%s?sslmode=disable",
		host, port.Port(), dbName)

	runMigrationsToVersion(t, connStr, 18)

	db := openMigrationTestDB(t, connStr)
	defer db.Close()

	legacyProjectID := uuid.New()
	currentProjectID := uuid.New()
	defaultProjectID := uuid.New()

	for _, projectID := range []uuid.UUID{legacyProjectID, currentProjectID, defaultProjectID} {
		_, err = db.Exec(`
			INSERT INTO engine_projects (id, name, created_at, updated_at)
			VALUES ($1, 'test-project', NOW(), NOW())
		`, projectID)
		require.NoError(t, err, "Failed to create test project")
	}

	_, err = db.Exec(`
		INSERT INTO engine_mcp_config (project_id, tool_groups, created_at, updated_at)
		VALUES ($1, $2::jsonb, NOW(), NOW())
	`, legacyProjectID, `{
		"developer": {"enabled": true},
		"user": {"enabled": true},
		"approved_queries": {"enabled": true},
		"agent_tools": {"enabled": true}
	}`)
	require.NoError(t, err, "Failed to insert legacy MCP config row")

	currentShape := `{
		"tools": {
			"addDirectDatabaseAccess": true,
			"addOntologyMaintenanceTools": false,
			"addOntologySuggestions": true,
			"addApprovalTools": false,
			"addRequestTools": true
		},
		"agent_tools": {
			"enabled": true
		}
	}`
	_, err = db.Exec(`
		INSERT INTO engine_mcp_config (project_id, tool_groups, created_at, updated_at)
		VALUES ($1, $2::jsonb, NOW(), NOW())
	`, currentProjectID, currentShape)
	require.NoError(t, err, "Failed to insert current-shape MCP config row")

	runPendingMigrations(t, connStr)

	var (
		hasTools                bool
		hasAgentTools           bool
		hasDeveloper            bool
		hasUser                 bool
		hasApprovedQueries      bool
		addDirectDatabaseAccess bool
		addOntologyMaintenance  bool
		addOntologySuggestions  bool
		addApprovalTools        bool
		addRequestTools         bool
		agentToolsEnabled       bool
	)
	err = db.QueryRow(`
		SELECT
			tool_groups ? 'tools',
			tool_groups ? 'agent_tools',
			tool_groups ? 'developer',
			tool_groups ? 'user',
			tool_groups ? 'approved_queries',
			(tool_groups->'tools'->>'addDirectDatabaseAccess')::boolean,
			(tool_groups->'tools'->>'addOntologyMaintenanceTools')::boolean,
			(tool_groups->'tools'->>'addOntologySuggestions')::boolean,
			(tool_groups->'tools'->>'addApprovalTools')::boolean,
			(tool_groups->'tools'->>'addRequestTools')::boolean,
			(tool_groups->'agent_tools'->>'enabled')::boolean
		FROM engine_mcp_config
		WHERE project_id = $1
	`, legacyProjectID).Scan(
		&hasTools,
		&hasAgentTools,
		&hasDeveloper,
		&hasUser,
		&hasApprovedQueries,
		&addDirectDatabaseAccess,
		&addOntologyMaintenance,
		&addOntologySuggestions,
		&addApprovalTools,
		&addRequestTools,
		&agentToolsEnabled,
	)
	require.NoError(t, err, "Failed to query rewritten legacy MCP config row")

	assert.True(t, hasTools, "legacy row should be rewritten with tools config")
	assert.True(t, hasAgentTools, "legacy row should retain agent_tools key in new shape")
	assert.False(t, hasDeveloper, "legacy developer key should be removed")
	assert.False(t, hasUser, "legacy user key should be removed")
	assert.False(t, hasApprovedQueries, "legacy approved_queries key should be removed")
	assert.False(t, addDirectDatabaseAccess, "legacy row should be blanked to disabled tools")
	assert.False(t, addOntologyMaintenance, "legacy row should be blanked to disabled tools")
	assert.False(t, addOntologySuggestions, "legacy row should be blanked to disabled tools")
	assert.False(t, addApprovalTools, "legacy row should be blanked to disabled tools")
	assert.False(t, addRequestTools, "legacy row should be blanked to disabled tools")
	assert.False(t, agentToolsEnabled, "legacy row should have agent tools disabled")

	var currentRowUnchanged bool
	err = db.QueryRow(`
		SELECT tool_groups = $2::jsonb
		FROM engine_mcp_config
		WHERE project_id = $1
	`, currentProjectID, currentShape).Scan(&currentRowUnchanged)
	require.NoError(t, err, "Failed to compare current-shape MCP config row")
	assert.True(t, currentRowUnchanged, "current-shape MCP config rows should not be rewritten")

	var defaultRowMatches bool
	err = db.QueryRow(`
		INSERT INTO engine_mcp_config (project_id, created_at, updated_at)
		VALUES ($1, NOW(), NOW())
		RETURNING tool_groups = '{
			"tools": {
				"addDirectDatabaseAccess": true,
				"addOntologyMaintenanceTools": true,
				"addOntologySuggestions": true,
				"addApprovalTools": true,
				"addRequestTools": true
			},
			"agent_tools": {
				"enabled": true
			}
		}'::jsonb
	`, defaultProjectID).Scan(&defaultRowMatches)
	require.NoError(t, err, "Failed to verify MCP config default after migration")
	assert.True(t, defaultRowMatches, "tool_groups default should use the new MCP config shape")
}

func openMigrationTestDB(t *testing.T, connStr string) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err, "Failed to open migration test database")
	require.NoError(t, db.Ping(), "Failed to ping migration test database")
	return db
}

func runMigrationsToVersion(t *testing.T, connStr string, version uint) {
	t.Helper()

	db := openMigrationTestDB(t, connStr)
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "Failed to create postgres migration driver")

	sourceDriver, err := iofs.New(migrationfs.FS, ".")
	require.NoError(t, err, "Failed to create migration source")

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
	require.NoError(t, err, "Failed to create migration instance")
	defer func() {
		srcErr, dbErr := m.Close()
		require.NoError(t, srcErr, "Failed to close migration source")
		require.NoError(t, dbErr, "Failed to close migration database")
	}()

	err = m.Migrate(version)
	if err != nil && err != migrate.ErrNoChange {
		require.NoError(t, err, "Failed to migrate to target version")
	}
}

func runPendingMigrations(t *testing.T, connStr string) {
	t.Helper()

	db := openMigrationTestDB(t, connStr)
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "Failed to create postgres migration driver")

	sourceDriver, err := iofs.New(migrationfs.FS, ".")
	require.NoError(t, err, "Failed to create migration source")

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
	require.NoError(t, err, "Failed to create migration instance")
	defer func() {
		srcErr, dbErr := m.Close()
		require.NoError(t, srcErr, "Failed to close migration source")
		require.NoError(t, dbErr, "Failed to close migration database")
	}()

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		require.NoError(t, err, "Failed to run pending migrations")
	}
}

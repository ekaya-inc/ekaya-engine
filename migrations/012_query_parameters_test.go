//go:build integration

package migrations

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test_012_QueryParameters verifies migration 012 adds the parameters column correctly
func Test_012_QueryParameters(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Verify the parameters column exists and has the correct type
	var columnExists bool
	var dataType string
	var columnDefault string
	err := engineDB.DB.Pool.QueryRow(ctx, `
		SELECT
			true as exists,
			data_type,
			column_default
		FROM information_schema.columns
		WHERE table_name = 'engine_queries'
		AND column_name = 'parameters'
	`).Scan(&columnExists, &dataType, &columnDefault)

	require.NoError(t, err, "Failed to query column information")
	assert.True(t, columnExists, "parameters column should exist")
	assert.Equal(t, "jsonb", dataType, "parameters column should be JSONB type")
	assert.Contains(t, columnDefault, "'[]'::jsonb", "parameters column should default to empty array")

	// Verify the partial index exists
	var indexExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'engine_queries'
			AND indexname = 'idx_engine_queries_has_parameters'
		)
	`).Scan(&indexExists)

	require.NoError(t, err, "Failed to query index information")
	assert.True(t, indexExists, "idx_engine_queries_has_parameters index should exist")

	// Verify the column comment exists
	var comment string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT
			col_description('engine_queries'::regclass,
				(SELECT ordinal_position
				 FROM information_schema.columns
				 WHERE table_name = 'engine_queries'
				 AND column_name = 'parameters'))
	`).Scan(&comment)

	require.NoError(t, err, "Failed to query column comment")
	assert.Contains(t, comment, "parameter definitions", "Column should have descriptive comment")
	assert.Contains(t, comment, "string, integer, decimal, boolean", "Comment should list supported types")
}

// Test_012_QueryParameters_InsertAndQuery verifies we can insert and query parameterized queries
func Test_012_QueryParameters_InsertAndQuery(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()
	projectID := uuid.New()

	// Clean up after test
	defer func() {
		_, _ = engineDB.DB.Pool.Exec(ctx, "DELETE FROM engine_queries WHERE project_id = $1", projectID)
		_, _ = engineDB.DB.Pool.Exec(ctx, "DELETE FROM engine_datasources WHERE project_id = $1", projectID)
		_, _ = engineDB.DB.Pool.Exec(ctx, "DELETE FROM engine_projects WHERE id = $1", projectID)
	}()

	// Create test project
	_, err := engineDB.DB.Pool.Exec(ctx, `
		INSERT INTO engine_projects (id, name, created_at, updated_at)
		VALUES ($1, 'test-project', NOW(), NOW())
	`, projectID)
	require.NoError(t, err, "Failed to create test project")

	// Create test datasource
	var datasourceID string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		INSERT INTO engine_datasources (
			project_id, name, datasource_type, datasource_config, created_at, updated_at
		) VALUES ($1, 'test-ds', 'postgres', 'test-config', NOW(), NOW())
		RETURNING id
	`, projectID).Scan(&datasourceID)
	require.NoError(t, err, "Failed to create test datasource")

	// Test 1: Insert query with empty parameters (default)
	var queryID1 string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		INSERT INTO engine_queries (
			project_id, datasource_id, natural_language_prompt,
			sql_query, dialect
		) VALUES ($1, $2, 'test query 1', 'SELECT * FROM test', 'postgres')
		RETURNING id
	`, projectID, datasourceID).Scan(&queryID1)
	require.NoError(t, err, "Failed to insert query with default empty parameters")

	// Verify it has empty array
	var params1 string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT parameters::text FROM engine_queries WHERE id = $1
	`, queryID1).Scan(&params1)
	require.NoError(t, err)
	assert.Equal(t, "[]", params1, "Default parameters should be empty array")

	// Test 2: Insert query with parameters
	queryID2 := uuid.New()
	_, err = engineDB.DB.Pool.Exec(ctx, `
		INSERT INTO engine_queries (
			id, project_id, datasource_id, natural_language_prompt,
			sql_query, dialect, parameters
		) VALUES ($1, $2, $3, 'test query 2',
			'SELECT * FROM customers WHERE id = {{customer_id}}',
			'postgres',
			$4::jsonb
		)
	`, queryID2, projectID, datasourceID, `[{
		"name": "customer_id",
		"type": "string",
		"description": "Customer identifier",
		"required": true,
		"default": null
	}]`)
	require.NoError(t, err, "Failed to insert query with parameters")

	// Verify we can query and parse the parameters
	var params2 string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT parameters::text FROM engine_queries WHERE id = $1
	`, queryID2).Scan(&params2)
	require.NoError(t, err)
	assert.Contains(t, params2, "customer_id", "Parameters should contain parameter name")
	assert.Contains(t, params2, "string", "Parameters should contain parameter type")
	assert.Contains(t, params2, "required", "Parameters should contain required flag")

	// Test 3: Query using the partial index (has parameters)
	var countWithParams int
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM engine_queries
		WHERE project_id = $1
		AND parameters != '[]'::jsonb
		AND deleted_at IS NULL
	`, projectID).Scan(&countWithParams)
	require.NoError(t, err)
	assert.Equal(t, 1, countWithParams, "Should find 1 query with parameters")

	// Test 4: Update parameters
	_, err = engineDB.DB.Pool.Exec(ctx, `
		UPDATE engine_queries
		SET parameters = $1::jsonb
		WHERE id = $2
	`, `[{
		"name": "customer_id",
		"type": "uuid",
		"description": "Customer UUID",
		"required": true,
		"default": null
	}, {
		"name": "status",
		"type": "string",
		"description": "Order status",
		"required": false,
		"default": "active"
	}]`, queryID2)
	require.NoError(t, err, "Failed to update parameters")

	// Verify multiple parameters
	var paramsUpdated string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT parameters::text FROM engine_queries WHERE id = $1
	`, queryID2).Scan(&paramsUpdated)
	require.NoError(t, err)
	assert.Contains(t, paramsUpdated, "customer_id")
	assert.Contains(t, paramsUpdated, "status")
	assert.Contains(t, paramsUpdated, "uuid")
	assert.Contains(t, paramsUpdated, "active")
}

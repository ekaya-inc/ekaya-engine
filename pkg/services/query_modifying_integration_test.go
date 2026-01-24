//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// These integration tests verify that INSERT/UPDATE/DELETE/CALL statements
// can be executed correctly. This tests the core database execution layer
// that ExecuteModifyingWithParameters depends on.

// TestExecutor_Insert tests direct INSERT execution.
func TestExecutor_Insert(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create test table
	_, err := testDB.Pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_executor_insert;
		CREATE TABLE test_executor_insert (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER DEFAULT 0
		);
	`)
	require.NoError(t, err)
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_executor_insert")
	}()

	// Execute INSERT with RETURNING
	sql := "INSERT INTO test_executor_insert (name, value) VALUES ($1, $2) RETURNING id, name, value"
	rows, err := testDB.Pool.Query(ctx, sql, "test-name", 42)
	require.NoError(t, err)
	defer rows.Close()

	// Collect results
	var resultID int
	var resultName string
	var resultValue int
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&resultID, &resultName, &resultValue))
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())

	// Verify results
	assert.Greater(t, resultID, 0)
	assert.Equal(t, "test-name", resultName)
	assert.Equal(t, 42, resultValue)

	// Verify data was actually inserted
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_executor_insert").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestExecutor_Update tests direct UPDATE execution.
func TestExecutor_Update(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create and populate test table
	_, err := testDB.Pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_executor_update;
		CREATE TABLE test_executor_update (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER DEFAULT 0
		);
		INSERT INTO test_executor_update (name, value) VALUES ('original', 10), ('other', 20);
	`)
	require.NoError(t, err)
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_executor_update")
	}()

	// Execute UPDATE with RETURNING
	sql := "UPDATE test_executor_update SET value = $1 WHERE name = $2 RETURNING name, value"
	rows, err := testDB.Pool.Query(ctx, sql, 99, "original")
	require.NoError(t, err)
	defer rows.Close()

	// Collect results
	var resultName string
	var resultValue int
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&resultName, &resultValue))
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())

	// Verify results
	assert.Equal(t, "original", resultName)
	assert.Equal(t, 99, resultValue)

	// Verify other record was not affected
	var otherValue int
	err = testDB.Pool.QueryRow(ctx, "SELECT value FROM test_executor_update WHERE name = 'other'").Scan(&otherValue)
	require.NoError(t, err)
	assert.Equal(t, 20, otherValue)
}

// TestExecutor_Delete tests direct DELETE execution.
func TestExecutor_Delete(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create and populate test table
	_, err := testDB.Pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_executor_delete;
		CREATE TABLE test_executor_delete (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		INSERT INTO test_executor_delete (name) VALUES ('to-delete'), ('keep');
	`)
	require.NoError(t, err)
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_executor_delete")
	}()

	// Execute DELETE with RETURNING
	sql := "DELETE FROM test_executor_delete WHERE name = $1 RETURNING name"
	rows, err := testDB.Pool.Query(ctx, sql, "to-delete")
	require.NoError(t, err)
	defer rows.Close()

	// Collect results
	var resultName string
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&resultName))
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())

	// Verify results
	assert.Equal(t, "to-delete", resultName)

	// Verify data was actually deleted
	var count int
	err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_executor_delete").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have 1 record remaining")
}

// TestExecutor_Call_StoredProcedure tests CALL execution for stored procedures.
func TestExecutor_Call_StoredProcedure(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create test table and stored procedure
	_, err := testDB.Pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_executor_call;
		CREATE TABLE test_executor_call (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER DEFAULT 0
		);
		INSERT INTO test_executor_call (name, value) VALUES ('proc-test', 10);

		DROP PROCEDURE IF EXISTS test_increment_proc;
		CREATE PROCEDURE test_increment_proc(IN p_name TEXT, IN p_increment INTEGER)
		LANGUAGE plpgsql
		AS $$
		BEGIN
			UPDATE test_executor_call SET value = value + p_increment WHERE name = p_name;
		END;
		$$;
	`)
	require.NoError(t, err)
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, "DROP PROCEDURE IF EXISTS test_increment_proc")
		_, _ = testDB.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_executor_call")
	}()

	// Execute CALL
	_, err = testDB.Pool.Exec(ctx, "CALL test_increment_proc($1, $2)", "proc-test", 5)
	require.NoError(t, err)

	// Verify the procedure modified the data
	var newValue int
	err = testDB.Pool.QueryRow(ctx, "SELECT value FROM test_executor_call WHERE name = 'proc-test'").Scan(&newValue)
	require.NoError(t, err)
	assert.Equal(t, 15, newValue, "value should be incremented from 10 to 15")
}

// TestValidate_ModifyingStatements tests that EXPLAIN can validate modifying statements.
func TestValidate_ModifyingStatements(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create test table
	_, err := testDB.Pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_validate_modify;
		CREATE TABLE test_validate_modify (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER DEFAULT 0
		);
	`)
	require.NoError(t, err)
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_validate_modify")
	}()

	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "INSERT statement",
			sql:  "INSERT INTO test_validate_modify (name, value) VALUES ('test', 1)",
		},
		{
			name: "INSERT with RETURNING",
			sql:  "INSERT INTO test_validate_modify (name) VALUES ('test') RETURNING id, name",
		},
		{
			name: "UPDATE statement",
			sql:  "UPDATE test_validate_modify SET value = 99 WHERE name = 'test'",
		},
		{
			name: "UPDATE with RETURNING",
			sql:  "UPDATE test_validate_modify SET value = 99 WHERE name = 'test' RETURNING name, value",
		},
		{
			name: "DELETE statement",
			sql:  "DELETE FROM test_validate_modify WHERE name = 'test'",
		},
		{
			name: "DELETE with RETURNING",
			sql:  "DELETE FROM test_validate_modify WHERE name = 'test' RETURNING id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use EXPLAIN to validate the statement without executing it
			_, err := testDB.Pool.Exec(ctx, "EXPLAIN "+tt.sql)
			require.NoError(t, err, "EXPLAIN should work for: %s", tt.sql)

			// Verify the statement was NOT actually executed
			var count int
			err = testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_validate_modify").Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 0, count, "table should be empty - statement should not have executed")
		})
	}
}

// TestSQLTypeDetection tests that modifying statement types are detected correctly.
func TestSQLTypeDetection(t *testing.T) {
	tests := []struct {
		sql         string
		isModifying bool
	}{
		{"SELECT * FROM users", false},
		{"INSERT INTO users (name) VALUES ('test')", true},
		{"UPDATE users SET name = 'test' WHERE id = 1", true},
		{"DELETE FROM users WHERE id = 1", true},
		{"CALL process_data()", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", false},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			sqlType := DetectSQLType(tt.sql)
			isModifying := IsModifyingStatement(sqlType)
			assert.Equal(t, tt.isModifying, isModifying, "SQL: %s", tt.sql)
		})
	}
}

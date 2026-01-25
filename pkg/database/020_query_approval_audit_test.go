//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test_020_QueryApprovalAudit verifies migration 020 adds the audit trail columns correctly
func Test_020_QueryApprovalAudit(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Verify audit columns exist with correct types
	auditColumns := map[string]string{
		"reviewed_by":      "character varying",
		"reviewed_at":      "timestamp with time zone",
		"rejection_reason": "text",
		"parent_query_id":  "uuid",
	}

	for colName, expectedType := range auditColumns {
		var dataType string
		err := engineDB.DB.Pool.QueryRow(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_name = 'engine_queries'
			AND column_name = $1
		`, colName).Scan(&dataType)
		require.NoError(t, err, "Column %s should exist", colName)
		assert.Equal(t, expectedType, dataType, "Column %s should have type %s", colName, expectedType)
	}

	// Verify all columns are nullable (no NOT NULL constraint)
	for colName := range auditColumns {
		var isNullable string
		err := engineDB.DB.Pool.QueryRow(ctx, `
			SELECT is_nullable
			FROM information_schema.columns
			WHERE table_name = 'engine_queries'
			AND column_name = $1
		`, colName).Scan(&isNullable)
		require.NoError(t, err)
		assert.Equal(t, "YES", isNullable, "Column %s should be nullable", colName)
	}

	// Verify column comments exist
	columnComments := map[string]string{
		"reviewed_by":      "reviewed the pending query",
		"reviewed_at":      "approved or rejected",
		"rejection_reason": "rejected",
		"parent_query_id":  "original query",
	}

	for colName, expectedSubstring := range columnComments {
		var comment string
		err := engineDB.DB.Pool.QueryRow(ctx, `
			SELECT col_description('engine_queries'::regclass,
				(SELECT ordinal_position
				 FROM information_schema.columns
				 WHERE table_name = 'engine_queries'
				 AND column_name = $1))
		`, colName).Scan(&comment)
		require.NoError(t, err, "Failed to query comment for column %s", colName)
		assert.Contains(t, comment, expectedSubstring, "Column %s should have descriptive comment", colName)
	}

	// Verify partial index for pending queries exists
	var pendingIndexExists bool
	err := engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_indexes
			WHERE tablename = 'engine_queries'
			AND indexname = 'idx_queries_pending_review'
		)
	`).Scan(&pendingIndexExists)
	require.NoError(t, err)
	assert.True(t, pendingIndexExists, "idx_queries_pending_review index should exist")

	// Verify the pending review index is a partial index with correct WHERE clause
	var pendingIndexDef string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT indexdef FROM pg_indexes
		WHERE tablename = 'engine_queries'
		AND indexname = 'idx_queries_pending_review'
	`).Scan(&pendingIndexDef)
	require.NoError(t, err)
	assert.Contains(t, pendingIndexDef, "pending", "Pending review index should filter on pending status")
	assert.Contains(t, pendingIndexDef, "deleted_at IS NULL", "Pending review index should exclude deleted queries")

	// Verify partial index for parent queries exists
	var parentIndexExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_indexes
			WHERE tablename = 'engine_queries'
			AND indexname = 'idx_queries_parent'
		)
	`).Scan(&parentIndexExists)
	require.NoError(t, err)
	assert.True(t, parentIndexExists, "idx_queries_parent index should exist")

	// Verify the parent index is a partial index with correct WHERE clause
	var parentIndexDef string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT indexdef FROM pg_indexes
		WHERE tablename = 'engine_queries'
		AND indexname = 'idx_queries_parent'
	`).Scan(&parentIndexDef)
	require.NoError(t, err)
	assert.Contains(t, parentIndexDef, "parent_query_id IS NOT NULL", "Parent index should filter on non-null parent")
	assert.Contains(t, parentIndexDef, "deleted_at IS NULL", "Parent index should exclude deleted queries")

	// Verify self-referential foreign key exists
	var fkExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			WHERE t.relname = 'engine_queries'
			AND c.contype = 'f'
			AND c.confrelid = 'engine_queries'::regclass
		)
	`).Scan(&fkExists)
	require.NoError(t, err)
	assert.True(t, fkExists, "Self-referential FK from parent_query_id to engine_queries(id) should exist")
}

// Test_020_QueryApprovalAudit_InsertAndQuery verifies we can insert and query queries with audit fields
func Test_020_QueryApprovalAudit_InsertAndQuery(t *testing.T) {
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

	// Test 1: Insert query with null audit fields (pending query, not yet reviewed)
	var parentQueryID string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		INSERT INTO engine_queries (
			project_id, datasource_id, natural_language_prompt,
			sql_query, dialect, status
		) VALUES ($1, $2, 'Get all users', 'SELECT * FROM users', 'postgres', 'approved')
		RETURNING id
	`, projectID, datasourceID).Scan(&parentQueryID)
	require.NoError(t, err, "Failed to insert parent query")

	// Verify null audit fields
	var reviewedBy, rejectionReason *string
	var reviewedAt *time.Time
	var parentID *uuid.UUID
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT reviewed_by, reviewed_at, rejection_reason, parent_query_id
		FROM engine_queries WHERE id = $1
	`, parentQueryID).Scan(&reviewedBy, &reviewedAt, &rejectionReason, &parentID)
	require.NoError(t, err)
	assert.Nil(t, reviewedBy, "reviewed_by should be null for new query")
	assert.Nil(t, reviewedAt, "reviewed_at should be null for new query")
	assert.Nil(t, rejectionReason, "rejection_reason should be null for new query")
	assert.Nil(t, parentID, "parent_query_id should be null for original query")

	// Test 2: Insert a pending update suggestion that references the parent query
	var childQueryID string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		INSERT INTO engine_queries (
			project_id, datasource_id, natural_language_prompt,
			sql_query, dialect, status, suggested_by, parent_query_id
		) VALUES ($1, $2, 'Get active users only',
			'SELECT * FROM users WHERE active = true', 'postgres', 'pending', 'agent', $3)
		RETURNING id
	`, projectID, datasourceID, parentQueryID).Scan(&childQueryID)
	require.NoError(t, err, "Failed to insert child query with parent reference")

	// Verify parent_query_id is set correctly
	var retrievedParentID uuid.UUID
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT parent_query_id FROM engine_queries WHERE id = $1
	`, childQueryID).Scan(&retrievedParentID)
	require.NoError(t, err)
	assert.Equal(t, parentQueryID, retrievedParentID.String(), "parent_query_id should match parent query")

	// Test 3: Simulate approving the query (update with audit fields)
	now := time.Now().UTC().Truncate(time.Microsecond)
	reviewer := "admin@example.com"
	_, err = engineDB.DB.Pool.Exec(ctx, `
		UPDATE engine_queries
		SET reviewed_by = $1,
			reviewed_at = $2,
			status = 'approved'
		WHERE id = $3
	`, reviewer, now, childQueryID)
	require.NoError(t, err, "Failed to update query with approval")

	// Verify approval audit fields
	var approvedReviewedBy string
	var approvedReviewedAt time.Time
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT reviewed_by, reviewed_at FROM engine_queries WHERE id = $1
	`, childQueryID).Scan(&approvedReviewedBy, &approvedReviewedAt)
	require.NoError(t, err)
	assert.Equal(t, reviewer, approvedReviewedBy, "reviewed_by should be set")
	assert.WithinDuration(t, now, approvedReviewedAt, time.Second, "reviewed_at should be set")

	// Test 4: Insert and reject a query
	var rejectedQueryID string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		INSERT INTO engine_queries (
			project_id, datasource_id, natural_language_prompt,
			sql_query, dialect, status, suggested_by
		) VALUES ($1, $2, 'Drop all tables',
			'DROP TABLE users CASCADE', 'postgres', 'pending', 'agent')
		RETURNING id
	`, projectID, datasourceID).Scan(&rejectedQueryID)
	require.NoError(t, err, "Failed to insert query to be rejected")

	// Reject the query
	rejectReason := "Query is destructive and violates safety policy"
	_, err = engineDB.DB.Pool.Exec(ctx, `
		UPDATE engine_queries
		SET reviewed_by = $1,
			reviewed_at = $2,
			status = 'rejected',
			rejection_reason = $3
		WHERE id = $4
	`, reviewer, now, rejectReason, rejectedQueryID)
	require.NoError(t, err, "Failed to reject query")

	// Verify rejection fields
	var rejectedReason string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT rejection_reason FROM engine_queries WHERE id = $1
	`, rejectedQueryID).Scan(&rejectedReason)
	require.NoError(t, err)
	assert.Equal(t, rejectReason, rejectedReason, "rejection_reason should be set")

	// Test 5: Verify partial index on pending queries works efficiently
	var pendingCount int
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_queries
		WHERE project_id = $1
		AND datasource_id = $2
		AND status = 'pending'
		AND deleted_at IS NULL
	`, projectID, datasourceID).Scan(&pendingCount)
	require.NoError(t, err)
	assert.Equal(t, 0, pendingCount, "Should have 0 pending queries after approval/rejection")

	// Test 6: Verify partial index on parent queries works
	var childCount int
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_queries
		WHERE parent_query_id = $1
		AND deleted_at IS NULL
	`, parentQueryID).Scan(&childCount)
	require.NoError(t, err)
	assert.Equal(t, 1, childCount, "Should find 1 child query referencing parent")
}

// Test_020_QueryApprovalAudit_ForeignKeyConstraint verifies FK prevents orphan parent references
func Test_020_QueryApprovalAudit_ForeignKeyConstraint(t *testing.T) {
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

	// Try to insert query with non-existent parent_query_id
	nonExistentParentID := uuid.New()
	_, err = engineDB.DB.Pool.Exec(ctx, `
		INSERT INTO engine_queries (
			project_id, datasource_id, natural_language_prompt,
			sql_query, dialect, status, parent_query_id
		) VALUES ($1, $2, 'Test query', 'SELECT 1', 'postgres', 'pending', $3)
	`, projectID, datasourceID, nonExistentParentID)

	// Should fail due to FK constraint
	require.Error(t, err, "Insert should fail due to FK constraint violation")
	assert.Contains(t, err.Error(), "violates foreign key constraint", "Error should mention FK violation")
}

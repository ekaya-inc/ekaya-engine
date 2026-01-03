//go:build integration

package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test_025_BusinessGlossary verifies migration 025 creates the business glossary table correctly
func Test_025_BusinessGlossary(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Verify the table exists
	var tableExists bool
	err := engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'engine_business_glossary'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "engine_business_glossary table should exist")

	// Verify key columns exist with correct types
	columns := map[string]string{
		"id":           "uuid",
		"project_id":   "uuid",
		"term":         "text",
		"definition":   "text",
		"sql_pattern":  "text",
		"base_table":   "text",
		"columns_used": "jsonb",
		"filters":      "jsonb",
		"aggregation":  "text",
		"source":       "text",
		"created_by":   "uuid",
		"created_at":   "timestamp with time zone",
		"updated_at":   "timestamp with time zone",
	}

	for colName, expectedType := range columns {
		var dataType string
		err := engineDB.DB.Pool.QueryRow(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_name = 'engine_business_glossary'
			AND column_name = $1
		`, colName).Scan(&dataType)
		require.NoError(t, err, "Column %s should exist", colName)
		assert.Equal(t, expectedType, dataType, "Column %s should have type %s", colName, expectedType)
	}

	// Verify RLS is enabled
	var rlsEnabled bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = 'engine_business_glossary'
	`).Scan(&rlsEnabled)
	require.NoError(t, err)
	assert.True(t, rlsEnabled, "Row Level Security should be enabled")

	// Verify project index exists
	var indexExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_indexes
			WHERE tablename = 'engine_business_glossary'
			AND indexname = 'idx_business_glossary_project'
		)
	`).Scan(&indexExists)
	require.NoError(t, err)
	assert.True(t, indexExists, "Project index should exist")

	// Verify unique constraint on (project_id, term)
	var uniqueExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			WHERE t.relname = 'engine_business_glossary'
			AND c.contype = 'u'
		)
	`).Scan(&uniqueExists)
	require.NoError(t, err)
	assert.True(t, uniqueExists, "Unique constraint on (project_id, term) should exist")

	// Verify update timestamp trigger exists
	var triggerExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_trigger
			WHERE tgname = 'update_business_glossary_updated_at'
		)
	`).Scan(&triggerExists)
	require.NoError(t, err)
	assert.True(t, triggerExists, "Update timestamp trigger should exist")

	// Verify policy exists
	var policyExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_policy
			WHERE polrelid = 'engine_business_glossary'::regclass
			AND polname = 'business_glossary_access'
		)
	`).Scan(&policyExists)
	require.NoError(t, err)
	assert.True(t, policyExists, "RLS policy should exist")

	// Verify source column has default value 'user'
	var defaultValue string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_name = 'engine_business_glossary'
		AND column_name = 'source'
	`).Scan(&defaultValue)
	require.NoError(t, err)
	assert.Contains(t, defaultValue, "user", "Source column should default to 'user'")
}

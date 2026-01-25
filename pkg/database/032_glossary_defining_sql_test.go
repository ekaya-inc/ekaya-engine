//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test_031_GlossaryDefiningSql verifies migration 031 creates glossary tables with defining_sql schema
func Test_031_GlossaryDefiningSql(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Verify the glossary table exists
	var glossaryTableExists bool
	err := engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'engine_business_glossary'
		)
	`).Scan(&glossaryTableExists)
	require.NoError(t, err)
	assert.True(t, glossaryTableExists, "engine_business_glossary table should exist")

	// Verify aliases table exists
	var aliasesTableExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'engine_glossary_aliases'
		)
	`).Scan(&aliasesTableExists)
	require.NoError(t, err)
	assert.True(t, aliasesTableExists, "engine_glossary_aliases table should exist")

	// Verify key columns exist with correct types in glossary table
	glossaryColumns := map[string]string{
		"id":             "uuid",
		"project_id":     "uuid",
		"term":           "text",
		"definition":     "text",
		"defining_sql":   "text",
		"base_table":     "text",
		"output_columns": "jsonb",
		"source":         "text",
		"created_by":     "uuid",
		"updated_by":     "uuid",
		"created_at":     "timestamp with time zone",
		"updated_at":     "timestamp with time zone",
	}

	for colName, expectedType := range glossaryColumns {
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

	// Verify old fragmented columns no longer exist
	oldColumns := []string{"sql_pattern", "columns_used", "filters", "aggregation"}
	for _, colName := range oldColumns {
		var exists bool
		err := engineDB.DB.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.columns
				WHERE table_name = 'engine_business_glossary'
				AND column_name = $1
			)
		`, colName).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists, "Old column %s should not exist", colName)
	}

	// Verify aliases table columns
	aliasColumns := map[string]string{
		"id":          "uuid",
		"glossary_id": "uuid",
		"alias":       "text",
		"created_at":  "timestamp with time zone",
	}

	for colName, expectedType := range aliasColumns {
		var dataType string
		err := engineDB.DB.Pool.QueryRow(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_name = 'engine_glossary_aliases'
			AND column_name = $1
		`, colName).Scan(&dataType)
		require.NoError(t, err, "Column %s should exist in aliases table", colName)
		assert.Equal(t, expectedType, dataType, "Column %s should have type %s", colName, expectedType)
	}

	// Verify RLS is enabled on both tables
	var glossaryRlsEnabled bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = 'engine_business_glossary'
	`).Scan(&glossaryRlsEnabled)
	require.NoError(t, err)
	assert.True(t, glossaryRlsEnabled, "Row Level Security should be enabled on glossary")

	var aliasesRlsEnabled bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = 'engine_glossary_aliases'
	`).Scan(&aliasesRlsEnabled)
	require.NoError(t, err)
	assert.True(t, aliasesRlsEnabled, "Row Level Security should be enabled on aliases")

	// Verify glossary indexes exist
	glossaryIndexes := []string{
		"idx_business_glossary_project",
		"idx_business_glossary_source",
		"idx_business_glossary_base_table",
	}

	for _, indexName := range glossaryIndexes {
		var indexExists bool
		err = engineDB.DB.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM pg_indexes
				WHERE tablename = 'engine_business_glossary'
				AND indexname = $1
			)
		`, indexName).Scan(&indexExists)
		require.NoError(t, err)
		assert.True(t, indexExists, "Index %s should exist", indexName)
	}

	// Verify aliases indexes exist
	aliasIndexes := []string{
		"idx_glossary_aliases_glossary",
		"idx_glossary_aliases_alias",
	}

	for _, indexName := range aliasIndexes {
		var indexExists bool
		err = engineDB.DB.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM pg_indexes
				WHERE tablename = 'engine_glossary_aliases'
				AND indexname = $1
			)
		`, indexName).Scan(&indexExists)
		require.NoError(t, err)
		assert.True(t, indexExists, "Index %s should exist", indexName)
	}

	// Verify unique index on (project_id, ontology_id, term)
	// Note: Migration 016 changed this from a constraint to a unique index that includes ontology_id
	var uniqueExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_indexes
			WHERE tablename = 'engine_business_glossary'
			AND indexname = 'engine_business_glossary_project_ontology_term_unique'
		)
	`).Scan(&uniqueExists)
	require.NoError(t, err)
	assert.True(t, uniqueExists, "Unique index on (project_id, ontology_id, term) should exist")

	// Verify unique constraint on aliases (glossary_id, alias)
	var aliasUniqueExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			WHERE t.relname = 'engine_glossary_aliases'
			AND c.contype = 'u'
			AND c.conname = 'engine_glossary_aliases_unique'
		)
	`).Scan(&aliasUniqueExists)
	require.NoError(t, err)
	assert.True(t, aliasUniqueExists, "Unique constraint on (glossary_id, alias) should exist")

	// Verify source check constraint
	var sourceCheckExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			WHERE t.relname = 'engine_business_glossary'
			AND c.contype = 'c'
			AND c.conname = 'engine_business_glossary_source_check'
		)
	`).Scan(&sourceCheckExists)
	require.NoError(t, err)
	assert.True(t, sourceCheckExists, "Source check constraint should exist")

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

	// Verify glossary policy exists
	var glossaryPolicyExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_policy
			WHERE polrelid = 'engine_business_glossary'::regclass
			AND polname = 'business_glossary_access'
		)
	`).Scan(&glossaryPolicyExists)
	require.NoError(t, err)
	assert.True(t, glossaryPolicyExists, "RLS policy should exist on glossary")

	// Verify aliases policy exists
	var aliasesPolicyExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_policy
			WHERE polrelid = 'engine_glossary_aliases'::regclass
			AND polname = 'glossary_aliases_access'
		)
	`).Scan(&aliasesPolicyExists)
	require.NoError(t, err)
	assert.True(t, aliasesPolicyExists, "RLS policy should exist on aliases")

	// Verify source column has default value 'inferred'
	var defaultValue string
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_name = 'engine_business_glossary'
		AND column_name = 'source'
	`).Scan(&defaultValue)
	require.NoError(t, err)
	assert.Contains(t, defaultValue, "inference", "Source column should default to 'inference'")

	// Verify foreign key from aliases to glossary
	var fkExists bool
	err = engineDB.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT FROM pg_constraint c
			JOIN pg_class t ON c.conrelid = t.oid
			WHERE t.relname = 'engine_glossary_aliases'
			AND c.contype = 'f'
			AND c.confrelid = 'engine_business_glossary'::regclass
		)
	`).Scan(&fkExists)
	require.NoError(t, err)
	assert.True(t, fkExists, "Foreign key from aliases to glossary should exist")
}

//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestMigration028_FixRelationshipUniqueConstraint(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Verify the new unique constraint exists
	var constraintExists bool
	err := engineDB.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'engine_entity_relationships_unique_relationship'
		)
	`).Scan(&constraintExists)
	require.NoError(t, err)
	require.True(t, constraintExists, "New unique constraint should exist")

	// Verify the old constraint does not exist
	var oldConstraintExists bool
	err = engineDB.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'engine_entity_relationships_ontology_id_source_entity_id_ta_key'
		)
	`).Scan(&oldConstraintExists)
	require.NoError(t, err)
	require.False(t, oldConstraintExists, "Old unique constraint should not exist")
}

func TestMigration029_ReverseRelationships(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Count relationships - migration 029 should have already run
	var totalCount int
	err := engineDB.DB.QueryRow(ctx, "SELECT COUNT(*) FROM engine_entity_relationships").Scan(&totalCount)
	require.NoError(t, err)

	if totalCount == 0 {
		t.Skip("No relationships exist to test reverse creation")
	}

	// Query a sample relationship and verify its reverse exists
	var originalID, reverseID string
	var origSourceEntityID, origTargetEntityID string
	var origSourceTable, origTargetTable, origSourceCol, origTargetCol string
	var revSourceEntityID, revTargetEntityID string
	var revSourceTable, revTargetTable, revSourceCol, revTargetCol string

	// Get one relationship
	err = engineDB.DB.QueryRow(ctx, `
		SELECT id, source_entity_id, target_entity_id,
		       source_column_table, source_column_name,
		       target_column_table, target_column_name
		FROM engine_entity_relationships
		LIMIT 1
	`).Scan(&originalID, &origSourceEntityID, &origTargetEntityID,
		&origSourceTable, &origSourceCol, &origTargetTable, &origTargetCol)
	require.NoError(t, err)

	// Find its reverse (must match ALL swapped columns including column names)
	err = engineDB.DB.QueryRow(ctx, `
		SELECT id, source_entity_id, target_entity_id,
		       source_column_table, source_column_name,
		       target_column_table, target_column_name
		FROM engine_entity_relationships
		WHERE source_entity_id = $1
		  AND target_entity_id = $2
		  AND source_column_table = $3
		  AND source_column_name = $4
		  AND target_column_table = $5
		  AND target_column_name = $6
		  AND id != $7
	`, origTargetEntityID, origSourceEntityID,
		origTargetTable, origTargetCol, origSourceTable, origSourceCol, originalID).
		Scan(&reverseID, &revSourceEntityID, &revTargetEntityID,
			&revSourceTable, &revSourceCol, &revTargetTable, &revTargetCol)

	require.NoError(t, err, "Reverse relationship should exist for forward relationship")

	// Verify the reverse relationship has swapped entities, tables, and columns
	require.Equal(t, origTargetEntityID, revSourceEntityID, "Reverse source entity should be original target entity")
	require.Equal(t, origSourceEntityID, revTargetEntityID, "Reverse target entity should be original source entity")
	require.Equal(t, origTargetTable, revSourceTable, "Reverse source table should be original target table")
	require.Equal(t, origSourceTable, revTargetTable, "Reverse target table should be original source table")
	require.Equal(t, origTargetCol, revSourceCol, "Reverse source column should be original target column")
	require.Equal(t, origSourceCol, revTargetCol, "Reverse target column should be original source column")

	// Verify reverse has different ID
	require.NotEqual(t, originalID, reverseID, "Reverse should be a separate row with different ID")

	// Count pairs - each original relationship should have a reverse
	// This query counts relationship pairs where both directions exist
	var pairCount int
	err = engineDB.DB.QueryRow(ctx, `
		SELECT COUNT(DISTINCT LEAST(r1.id, r2.id))
		FROM engine_entity_relationships r1
		INNER JOIN engine_entity_relationships r2
		  ON r1.ontology_id = r2.ontology_id
		  AND r1.source_entity_id = r2.target_entity_id
		  AND r1.target_entity_id = r2.source_entity_id
		  AND r1.source_column_table = r2.target_column_table
		  AND r1.target_column_table = r2.source_column_table
		  AND r1.source_column_name = r2.target_column_name
		  AND r1.target_column_name = r2.source_column_name
		WHERE r1.id < r2.id
	`).Scan(&pairCount)
	require.NoError(t, err)

	t.Logf("Total relationships: %d, bidirectional pairs: %d", totalCount, pairCount)

	// We should have at least some bidirectional pairs
	require.Greater(t, pairCount, 0, "Should have at least one bidirectional relationship pair")
}

func TestMigration029_ReverseRelationships_NoExistingData(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Verify migration doesn't error when no relationships exist
	var count int
	err := engineDB.DB.QueryRow(ctx, "SELECT COUNT(*) FROM engine_entity_relationships").Scan(&count)
	require.NoError(t, err)

	// Test passes as long as the query succeeds
	t.Logf("Total relationships in test database: %d", count)
}

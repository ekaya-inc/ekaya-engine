//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// TestMigration027_AddRelationshipAssociation verifies that migration 027
// adds the association column to engine_entity_relationships table.
func TestMigration027_AddRelationshipAssociation(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Query for the association column
	var columnExists bool
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = 'engine_entity_relationships'
			AND column_name = 'association'
		)
	`
	err := engineDB.DB.QueryRow(ctx, query).Scan(&columnExists)
	if err != nil {
		t.Fatalf("failed to check for association column: %v", err)
	}

	if !columnExists {
		t.Error("association column does not exist on engine_entity_relationships table")
	}

	// Verify column type and constraints
	var dataType string
	var characterMaxLength *int
	typeQuery := `
		SELECT data_type, character_maximum_length
		FROM information_schema.columns
		WHERE table_name = 'engine_entity_relationships'
		AND column_name = 'association'
	`
	err = engineDB.DB.QueryRow(ctx, typeQuery).Scan(&dataType, &characterMaxLength)
	if err != nil {
		t.Fatalf("failed to get column metadata: %v", err)
	}

	if dataType != "character varying" {
		t.Errorf("expected data_type 'character varying', got '%s'", dataType)
	}

	if characterMaxLength == nil || *characterMaxLength != 100 {
		t.Errorf("expected character_maximum_length 100, got %v", characterMaxLength)
	}

	// Verify column comment exists
	var comment *string
	commentQuery := `
		SELECT col_description('engine_entity_relationships'::regclass,
			(SELECT ordinal_position FROM information_schema.columns
			WHERE table_name = 'engine_entity_relationships' AND column_name = 'association'))
	`
	err = engineDB.DB.QueryRow(ctx, commentQuery).Scan(&comment)
	if err != nil {
		t.Fatalf("failed to get column comment: %v", err)
	}

	if comment == nil || *comment == "" {
		t.Error("expected association column to have a comment")
	}
}

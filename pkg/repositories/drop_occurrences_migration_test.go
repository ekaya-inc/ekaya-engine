//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// TestMigration030_DropOccurrencesTable verifies that migration 030
// drops the engine_ontology_entity_occurrences table.
func TestMigration030_DropOccurrencesTable(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	ctx := context.Background()

	// Query to check if the table exists
	var tableExists bool
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'engine_ontology_entity_occurrences'
		)
	`
	err := engineDB.DB.QueryRow(ctx, query).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check for occurrences table: %v", err)
	}

	if tableExists {
		t.Error("engine_ontology_entity_occurrences table should not exist after migration 030")
	}
}

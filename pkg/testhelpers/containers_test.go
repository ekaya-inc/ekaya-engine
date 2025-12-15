//go:build integration

package testhelpers

import (
	"context"
	"testing"
)

func TestEngineTestDB_Connection(t *testing.T) {
	testDB := GetTestDB(t)

	ctx := context.Background()

	// Verify we can query the pre-loaded schema
	var tableCount int
	err := testDB.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").
		Scan(&tableCount)
	if err != nil {
		t.Fatalf("failed to count tables: %v", err)
	}

	if tableCount != 38 {
		t.Errorf("expected 38 tables in test schema, got %d", tableCount)
	}
}

func TestEngineTestDB_TableData(t *testing.T) {
	testDB := GetTestDB(t)

	ctx := context.Background()

	// Verify trimmed tables have expected row counts
	tests := []struct {
		table    string
		expected int
	}{
		{"events", 100},
		{"accounts", 95},
		{"users", 95},
	}

	for _, tt := range tests {
		var count int
		err := testDB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+tt.table).Scan(&count)
		if err != nil {
			t.Errorf("failed to count %s: %v", tt.table, err)
			continue
		}
		if count != tt.expected {
			t.Errorf("%s: expected %d rows, got %d", tt.table, tt.expected, count)
		}
	}
}

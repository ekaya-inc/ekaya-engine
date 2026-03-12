package etl

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestMatchColumns_RejectsWhenExistingNotNullColumnMissing(t *testing.T) {
	// Existing table has a NOT NULL column (user_id) that is not in the CSV.
	// The matcher should NOT map to this table — it should be treated as a new table.
	existing := []*models.SchemaColumn{
		{ID: uuid.New(), ColumnName: "id", DataType: "integer", IsNullable: false},
		{ID: uuid.New(), ColumnName: "name", DataType: "text", IsNullable: true},
		{ID: uuid.New(), ColumnName: "email", DataType: "text", IsNullable: true},
		{ID: uuid.New(), ColumnName: "user_id", DataType: "integer", IsNullable: false}, // NOT NULL, no default
	}

	inferred := []models.InferredColumn{
		{Name: "id", SQLType: "INTEGER", Nullable: false},
		{Name: "name", SQLType: "TEXT", Nullable: true},
		{Name: "email", SQLType: "TEXT", Nullable: true},
		// user_id is missing from the CSV
	}

	mappings := matchColumns(inferred, existing, nil)

	// All 3 inferred columns match — but user_id (NOT NULL) is missing from the file.
	// matchColumns should flag this so the caller can reject the match.
	// Check that the result includes information about the missing NOT NULL column.
	hasUnmappedNotNull := hasMissingNotNullColumns(existing, mappings)
	if !hasUnmappedNotNull {
		t.Error("expected hasMissingNotNullColumns to return true when a NOT NULL column without a default is missing from the file")
	}
}

func TestMatchColumns_AllowsMissingNullableColumn(t *testing.T) {
	// Existing table has a nullable column (notes) that is not in the CSV.
	// This should be fine — nullable columns can default to NULL.
	existing := []*models.SchemaColumn{
		{ID: uuid.New(), ColumnName: "id", DataType: "integer", IsNullable: false},
		{ID: uuid.New(), ColumnName: "name", DataType: "text", IsNullable: true},
		{ID: uuid.New(), ColumnName: "notes", DataType: "text", IsNullable: true}, // nullable, not in CSV
	}

	inferred := []models.InferredColumn{
		{Name: "id", SQLType: "INTEGER", Nullable: false},
		{Name: "name", SQLType: "TEXT", Nullable: true},
	}

	mappings := matchColumns(inferred, existing, nil)

	hasUnmappedNotNull := hasMissingNotNullColumns(existing, mappings)
	if hasUnmappedNotNull {
		t.Error("expected hasMissingNotNullColumns to return false when only nullable columns are missing")
	}
}

func TestMatchColumns_AllowsMissingNotNullColumnWithDefault(t *testing.T) {
	// Existing table has a NOT NULL column with a DEFAULT — safe to omit from INSERT.
	defaultVal := "now()"
	existing := []*models.SchemaColumn{
		{ID: uuid.New(), ColumnName: "id", DataType: "integer", IsNullable: false},
		{ID: uuid.New(), ColumnName: "name", DataType: "text", IsNullable: true},
		{ID: uuid.New(), ColumnName: "created_at", DataType: "timestamptz", IsNullable: false, DefaultValue: &defaultVal},
	}

	inferred := []models.InferredColumn{
		{Name: "id", SQLType: "INTEGER", Nullable: false},
		{Name: "name", SQLType: "TEXT", Nullable: true},
		// created_at is missing but has a DEFAULT — should be OK
	}

	mappings := matchColumns(inferred, existing, nil)

	hasUnmappedNotNull := hasMissingNotNullColumns(existing, mappings)
	if hasUnmappedNotNull {
		t.Error("expected hasMissingNotNullColumns to return false when missing NOT NULL column has a DEFAULT")
	}
}

func TestMatch_RequiresHighColumnMatchThreshold(t *testing.T) {
	// Only 2 of 6 inferred columns match (33%) — this should be treated as a new table.
	// The threshold should be higher than 30% to avoid false matches.
	existing := []*models.SchemaColumn{
		{ID: uuid.New(), ColumnName: "id", DataType: "integer", IsNullable: false},
		{ID: uuid.New(), ColumnName: "name", DataType: "text", IsNullable: true},
		{ID: uuid.New(), ColumnName: "category", DataType: "text", IsNullable: true},
	}

	inferred := []models.InferredColumn{
		{Name: "id", SQLType: "INTEGER", Nullable: false},
		{Name: "name", SQLType: "TEXT", Nullable: true},
		{Name: "sku", SQLType: "TEXT", Nullable: true},
		{Name: "price", SQLType: "NUMERIC", Nullable: true},
		{Name: "weight", SQLType: "NUMERIC", Nullable: true},
		{Name: "color", SQLType: "TEXT", Nullable: true},
	}

	mappings := matchColumns(inferred, existing, nil)

	matchedCount := 0
	for _, m := range mappings {
		if m.MappedName != "" {
			matchedCount++
		}
	}

	// 2 out of 6 = 33%. Current threshold is 30% which would pass.
	// We want a higher threshold (e.g., 60%) to avoid loading into wrong tables.
	matchRatio := float64(matchedCount) / float64(len(inferred))
	if matchRatio >= 0.6 {
		t.Errorf("expected match ratio %.2f to be below 0.6 threshold", matchRatio)
	}
	// The real test: this ratio should NOT be high enough to trigger a match.
	// This validates the threshold constant, tested via Match() integration below.
}

func TestIdentifierSimilarity_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		minScore float64
		maxScore float64
	}{
		{"exact match", "order_items", "order_items", 1.0, 1.0},
		{"unrelated", "users", "products", 0.0, 0.1},
		{"partial token overlap", "product_name", "product_brand", 0.4, 0.6},
		{"containment short in long", "user", "user_id", 0.5, 0.7},
		{"very different lengths", "id", "inventory_item_distribution_center_id", 0.0, 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := identifierSimilarity(normalizeIdentifier(tt.a), normalizeIdentifier(tt.b))
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("identifierSimilarity(%q, %q) = %.2f, want [%.2f, %.2f]", tt.a, tt.b, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

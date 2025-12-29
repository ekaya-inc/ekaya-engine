package services

import (
	"strings"
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"go.uber.org/zap"
)

func TestBuildSchemaSummaryForDescription(t *testing.T) {
	// Create a minimal service instance for testing
	service := &ontologyBuilderService{
		logger: zap.NewNop(),
	}

	t.Run("filters out negative row counts", func(t *testing.T) {
		negativeOne := int64(-1)
		validCount := int64(100)

		tables := []*models.SchemaTable{
			{
				TableName: "table_with_negative",
				RowCount:  &negativeOne,
				Columns: []models.SchemaColumn{
					{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				},
			},
			{
				TableName: "table_with_valid",
				RowCount:  &validCount,
				Columns: []models.SchemaColumn{
					{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				},
			},
			{
				TableName: "table_with_nil",
				RowCount:  nil,
				Columns: []models.SchemaColumn{
					{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		// Should NOT contain "Rows: -1"
		if strings.Contains(result, "Rows: -1") {
			t.Errorf("Result should not contain 'Rows: -1', got:\n%s", result)
		}

		// Should contain "Rows: 100"
		if !strings.Contains(result, "Rows: 100") {
			t.Errorf("Result should contain 'Rows: 100', got:\n%s", result)
		}

		// Should NOT contain row count for nil table
		// (just verify the table is present without a Rows line before it)
		if strings.Contains(result, "table_with_nil\nRows:") {
			t.Errorf("table_with_nil should not have a Rows line")
		}
	})

	t.Run("includes column data types", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{
				TableName: "users",
				Columns: []models.SchemaColumn{
					{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
					{ColumnName: "name", DataType: "varchar"},
					{ColumnName: "age", DataType: "integer", IsNullable: true},
				},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		// Should contain data types
		if !strings.Contains(result, "id: uuid") {
			t.Errorf("Result should contain 'id: uuid', got:\n%s", result)
		}
		if !strings.Contains(result, "name: varchar") {
			t.Errorf("Result should contain 'name: varchar', got:\n%s", result)
		}
		if !strings.Contains(result, "age: integer") {
			t.Errorf("Result should contain 'age: integer', got:\n%s", result)
		}
	})

	t.Run("includes PK flags", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{
				TableName: "orders",
				Columns: []models.SchemaColumn{
					{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
					{ColumnName: "user_id", DataType: "uuid", IsPrimaryKey: false},
				},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		// Should have [PK] for id
		if !strings.Contains(result, "id: uuid [PK]") {
			t.Errorf("Result should contain 'id: uuid [PK]', got:\n%s", result)
		}

		// Should NOT have [PK] for user_id
		if strings.Contains(result, "user_id: uuid [PK]") {
			t.Errorf("Result should not contain 'user_id: uuid [PK]', got:\n%s", result)
		}
	})

	t.Run("includes nullable flags", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{
				TableName: "products",
				Columns: []models.SchemaColumn{
					{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, IsNullable: false},
					{ColumnName: "description", DataType: "text", IsNullable: true},
				},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		// Should have [nullable] for description
		if !strings.Contains(result, "description: text [nullable]") {
			t.Errorf("Result should contain 'description: text [nullable]', got:\n%s", result)
		}

		// id should have [PK] but not [nullable]
		if strings.Contains(result, "id: uuid [PK, nullable]") {
			t.Errorf("id should not be marked nullable, got:\n%s", result)
		}
	})

	t.Run("includes combined PK and nullable flags", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{
				TableName: "edge_case",
				Columns: []models.SchemaColumn{
					// Unusual but possible: PK that is nullable (some DBs allow this)
					{ColumnName: "weird_pk", DataType: "varchar", IsPrimaryKey: true, IsNullable: true},
				},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		// Should have both flags
		if !strings.Contains(result, "weird_pk: varchar [PK, nullable]") {
			t.Errorf("Result should contain 'weird_pk: varchar [PK, nullable]', got:\n%s", result)
		}
	})

	t.Run("includes column count", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{
				TableName: "multi_column",
				Columns: []models.SchemaColumn{
					{ColumnName: "a", DataType: "int"},
					{ColumnName: "b", DataType: "int"},
					{ColumnName: "c", DataType: "int"},
				},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		// Should contain column count
		if !strings.Contains(result, "Columns (3):") {
			t.Errorf("Result should contain 'Columns (3):', got:\n%s", result)
		}
	})

	t.Run("includes table count", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "t1", Columns: []models.SchemaColumn{{ColumnName: "a", DataType: "int"}}},
			{TableName: "t2", Columns: []models.SchemaColumn{{ColumnName: "b", DataType: "int"}}},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		if !strings.Contains(result, "Total tables: 2") {
			t.Errorf("Result should contain 'Total tables: 2', got:\n%s", result)
		}
	})

	t.Run("handles empty tables slice", func(t *testing.T) {
		tables := []*models.SchemaTable{}

		result := service.buildSchemaSummaryForDescription(tables)

		if !strings.Contains(result, "Total tables: 0") {
			t.Errorf("Result should contain 'Total tables: 0', got:\n%s", result)
		}
	})

	t.Run("handles table with no columns", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{
				TableName: "empty_table",
				Columns:   []models.SchemaColumn{},
			},
		}

		result := service.buildSchemaSummaryForDescription(tables)

		if !strings.Contains(result, "Columns (0):") {
			t.Errorf("Result should contain 'Columns (0):', got:\n%s", result)
		}
	})
}

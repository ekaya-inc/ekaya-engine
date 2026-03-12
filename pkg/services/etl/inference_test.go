package etl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferSchema_BasicTypes(t *testing.T) {
	headers := []string{"name", "age", "active", "balance", "created"}
	rows := [][]string{
		{"Alice", "30", "true", "1234.56", "2024-01-15"},
		{"Bob", "25", "false", "789.01", "2024-02-20"},
		{"Carol", "35", "yes", "456.78", "2024-03-10"},
	}

	cols := InferSchema(headers, rows, 100)
	require.Len(t, cols, 5)

	assert.Equal(t, "name", cols[0].Name)
	assert.Equal(t, "TEXT", cols[0].SQLType)

	assert.Equal(t, "age", cols[1].Name)
	assert.Equal(t, "INTEGER", cols[1].SQLType)

	assert.Equal(t, "active", cols[2].Name)
	assert.Equal(t, "BOOLEAN", cols[2].SQLType)

	assert.Equal(t, "balance", cols[3].Name)
	assert.Equal(t, "DOUBLE PRECISION", cols[3].SQLType)

	assert.Equal(t, "created", cols[4].Name)
	assert.Equal(t, "DATE", cols[4].SQLType)
}

func TestInferSchema_Nullable(t *testing.T) {
	headers := []string{"value"}
	rows := [][]string{
		{"100"},
		{""},
		{"200"},
	}

	cols := InferSchema(headers, rows, 100)
	require.Len(t, cols, 1)
	assert.Equal(t, "INTEGER", cols[0].SQLType)
	assert.True(t, cols[0].Nullable)
}

func TestInferSchema_BigInt(t *testing.T) {
	headers := []string{"big_id"}
	rows := [][]string{
		{"9999999999999"},
		{"8888888888888"},
	}

	cols := InferSchema(headers, rows, 100)
	require.Len(t, cols, 1)
	assert.Equal(t, "BIGINT", cols[0].SQLType)
}

func TestInferSchema_Timestamps(t *testing.T) {
	headers := []string{"ts"}
	rows := [][]string{
		{"2024-01-15 10:30:00"},
		{"2024-02-20 14:45:00"},
	}

	cols := InferSchema(headers, rows, 100)
	require.Len(t, cols, 1)
	assert.Equal(t, "TIMESTAMPTZ", cols[0].SQLType)
}

func TestInferSchema_EmptyRows(t *testing.T) {
	headers := []string{"col"}
	cols := InferSchema(headers, nil, 100)
	require.Len(t, cols, 1)
	assert.Equal(t, "TEXT", cols[0].SQLType)
	assert.True(t, cols[0].Nullable)
}

func TestInferSchema_NoHeaders(t *testing.T) {
	cols := InferSchema(nil, nil, 100)
	assert.Nil(t, cols)
}

func TestInferSchema_SampleSize(t *testing.T) {
	headers := []string{"val"}
	rows := [][]string{
		{"100"},
		{"200"},
		{"not_a_number"},
		{"300"},
	}

	// With sample size of 2, only first 2 rows sampled (both integers)
	cols := InferSchema(headers, rows, 2)
	require.Len(t, cols, 1)
	assert.Equal(t, "INTEGER", cols[0].SQLType)

	// With full sample, mixed types fall to TEXT
	cols = InferSchema(headers, rows, 0)
	require.Len(t, cols, 1)
	assert.Equal(t, "TEXT", cols[0].SQLType)
}

func TestSanitizeColumnName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"First Name", "first_name"},
		{"Order-ID", "order_id"},
		{"Amount ($)", "amount_"},
		{"123abc", "_123abc"},
		{"", "column"},
		{"hello world/foo", "hello_world_foo"},
		{"CamelCase", "camelcase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeColumnName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

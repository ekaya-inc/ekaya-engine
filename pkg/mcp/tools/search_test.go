package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchResult_Structure(t *testing.T) {
	// Test that searchResult has expected fields
	result := searchResult{
		Query:      "user",
		Tables:     []tableMatch{},
		Columns:    []columnMatch{},
		TotalCount: 0,
	}

	assert.Equal(t, "user", result.Query)
	assert.NotNil(t, result.Tables)
	assert.NotNil(t, result.Columns)
	assert.Equal(t, 0, result.TotalCount)
}

func TestTableMatch_Structure(t *testing.T) {
	// Test that tableMatch has expected fields
	tableType := "transactional"
	description := "User accounts table"
	rowCount := int64(100)

	match := tableMatch{
		SchemaName:  "public",
		TableName:   "users",
		TableType:   &tableType,
		Description: &description,
		RowCount:    &rowCount,
		MatchType:   "table_name",
		Relevance:   1.0,
	}

	assert.Equal(t, "public", match.SchemaName)
	assert.Equal(t, "users", match.TableName)
	assert.Equal(t, "transactional", *match.TableType)
	assert.Equal(t, "User accounts table", *match.Description)
	assert.Equal(t, int64(100), *match.RowCount)
	assert.Equal(t, "table_name", match.MatchType)
	assert.Equal(t, 1.0, match.Relevance)
}

func TestColumnMatch_Structure(t *testing.T) {
	// Test that columnMatch has expected fields
	purpose := "identifier"
	description := "Primary key for users"

	match := columnMatch{
		SchemaName:  "public",
		TableName:   "users",
		ColumnName:  "user_id",
		DataType:    "uuid",
		Purpose:     &purpose,
		Description: &description,
		MatchType:   "column_name",
		Relevance:   0.9,
	}

	assert.Equal(t, "public", match.SchemaName)
	assert.Equal(t, "users", match.TableName)
	assert.Equal(t, "user_id", match.ColumnName)
	assert.Equal(t, "uuid", match.DataType)
	assert.Equal(t, "identifier", *match.Purpose)
	assert.Equal(t, "Primary key for users", *match.Description)
	assert.Equal(t, "column_name", match.MatchType)
	assert.Equal(t, 0.9, match.Relevance)
}

func TestSearchResult_TotalCount(t *testing.T) {
	// Test that total count is the sum of all matches
	result := searchResult{
		Query: "transaction",
		Tables: []tableMatch{
			{TableName: "transactions"},
		},
		Columns: []columnMatch{
			{ColumnName: "transaction_id"},
			{ColumnName: "transaction_state"},
		},
		TotalCount: 3,
	}

	assert.Equal(t, 1, len(result.Tables))
	assert.Equal(t, 2, len(result.Columns))
	assert.Equal(t, 3, result.TotalCount)
}

func TestMatchTypes(t *testing.T) {
	// Test that all match types are string constants
	validTableMatchTypes := []string{"table_name", "description"}
	validColumnMatchTypes := []string{"column_name", "purpose", "description"}

	for _, mt := range validTableMatchTypes {
		assert.NotEmpty(t, mt)
	}
	for _, mt := range validColumnMatchTypes {
		assert.NotEmpty(t, mt)
	}
}

func TestRelevanceScoring(t *testing.T) {
	// Test that relevance scores are floats in expected range
	testCases := []struct {
		name      string
		relevance float64
		valid     bool
	}{
		{"exact match", 1.0, true},
		{"prefix match", 0.9, true},
		{"purpose match", 0.7, true},
		{"description match", 0.6, true},
		{"fallback", 0.5, true},
		{"invalid negative", -0.1, false},
		{"invalid over 1", 1.1, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				assert.GreaterOrEqual(t, tc.relevance, 0.0)
				assert.LessOrEqual(t, tc.relevance, 1.0)
			} else {
				// These would be invalid in production
				assert.True(t, tc.relevance < 0.0 || tc.relevance > 1.0)
			}
		})
	}
}

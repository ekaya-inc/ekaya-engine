package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildRelationshipAnalysisPrompt(t *testing.T) {
	// Create test data
	rowCount := int64(100)
	tables := []TableContext{
		{
			Name:     "users",
			RowCount: &rowCount,
			PKColumn: "id",
			Columns: []ColumnContext{
				{
					Name:         "id",
					DataType:     "uuid",
					IsNullable:   false,
					NullPercent:  0.0,
					IsPrimaryKey: true,
				},
				{
					Name:         "email",
					DataType:     "text",
					IsNullable:   false,
					NullPercent:  0.0,
					IsPrimaryKey: false,
				},
			},
		},
		{
			Name:     "orders",
			RowCount: &rowCount,
			PKColumn: "id",
			Columns: []ColumnContext{
				{
					Name:         "id",
					DataType:     "uuid",
					IsNullable:   false,
					NullPercent:  0.0,
					IsPrimaryKey: true,
				},
				{
					Name:                "user_id",
					DataType:            "uuid",
					IsNullable:          false,
					NullPercent:         0.0,
					IsPrimaryKey:        false,
					LooksLikeForeignKey: true,
				},
			},
		},
	}

	matchRate := 0.95
	orphanRate := 0.05
	cardinality := "N:1"
	sourceRows := int64(100)
	targetRows := int64(50)
	coverage := 0.9

	candidates := []CandidateContext{
		{
			ID:               "test-candidate-1",
			SourceTable:      "orders",
			SourceColumn:     "user_id",
			SourceColumnType: "uuid",
			TargetTable:      "users",
			TargetColumn:     "id",
			TargetColumnType: "uuid",
			DetectionMethod:  "value_match",
			ValueMatchRate:   &matchRate,
			Cardinality:      &cardinality,
			JoinMatchRate:    &matchRate,
			OrphanRate:       &orphanRate,
			TargetCoverage:   &coverage,
			SourceRowCount:   &sourceRows,
			TargetRowCount:   &targetRows,
		},
	}

	prompt := BuildRelationshipAnalysisPrompt(tables, candidates)

	// Verify prompt structure
	assert.Contains(t, prompt, "# Database Relationship Analysis")
	assert.Contains(t, prompt, "## Database Schema")
	assert.Contains(t, prompt, "## Relationship Candidates")
	assert.Contains(t, prompt, "## Analysis Guidelines")
	assert.Contains(t, prompt, "## Interpretation Guide")
	assert.Contains(t, prompt, "## Output Format")

	// Verify table information
	assert.Contains(t, prompt, "### users")
	assert.Contains(t, prompt, "### orders")
	assert.Contains(t, prompt, "Row count: 100")
	assert.Contains(t, prompt, "Primary Key: id")

	// Verify column information
	assert.Contains(t, prompt, "[PK]")
	assert.Contains(t, prompt, "[looks like FK]")
	assert.Contains(t, prompt, "uuid")

	// Verify candidate information
	assert.Contains(t, prompt, "test-candidate-1")
	assert.Contains(t, prompt, "orders.user_id → users.id")
	assert.Contains(t, prompt, "value_match")
	assert.Contains(t, prompt, "N:1")
	assert.Contains(t, prompt, "95.0%") // Match rate

	// Verify guidelines
	assert.Contains(t, prompt, "Strong signals for CONFIRM")
	assert.Contains(t, prompt, "Strong signals for REJECT")
	assert.Contains(t, prompt, "Mark as NEEDS_REVIEW")

	// Verify interpretation guide
	assert.Contains(t, prompt, "Orphan Rate")
	assert.Contains(t, prompt, "Target Coverage")

	// Verify JSON format specification
	assert.Contains(t, prompt, `"decisions"`)
	assert.Contains(t, prompt, `"new_relationships"`)
	assert.Contains(t, prompt, `"candidate_id"`)
	assert.Contains(t, prompt, `"action"`)
	assert.Contains(t, prompt, `"confidence"`)
	assert.Contains(t, prompt, `"reasoning"`)
}

func TestBuildRelationshipAnalysisPrompt_EmptyCandidates(t *testing.T) {
	rowCount := int64(10)
	tables := []TableContext{
		{
			Name:     "users",
			RowCount: &rowCount,
			PKColumn: "id",
			Columns: []ColumnContext{
				{
					Name:         "id",
					DataType:     "uuid",
					IsPrimaryKey: true,
				},
			},
		},
	}

	candidates := []CandidateContext{}

	prompt := BuildRelationshipAnalysisPrompt(tables, candidates)

	// Should still generate a valid prompt
	assert.Contains(t, prompt, "# Database Relationship Analysis")
	assert.Contains(t, prompt, "### users")
	assert.NotContains(t, prompt, "Candidate 1")
}

func TestBuildRelationshipAnalysisPrompt_NullableColumns(t *testing.T) {
	rowCount := int64(100)
	tables := []TableContext{
		{
			Name:     "orders",
			RowCount: &rowCount,
			PKColumn: "id",
			Columns: []ColumnContext{
				{
					Name:         "customer_id",
					DataType:     "uuid",
					IsNullable:   true,
					NullPercent:  15.5,
					IsPrimaryKey: false,
				},
			},
		},
	}

	prompt := BuildRelationshipAnalysisPrompt(tables, []CandidateContext{})

	// Verify nullable information is included
	assert.Contains(t, prompt, "nullable")
	assert.Contains(t, prompt, "15.5% null")
}

func TestBuildRelationshipAnalysisPrompt_ForeignKeyColumns(t *testing.T) {
	rowCount := int64(100)
	tables := []TableContext{
		{
			Name:     "orders",
			RowCount: &rowCount,
			PKColumn: "id",
			Columns: []ColumnContext{
				{
					Name:             "customer_id",
					DataType:         "uuid",
					IsPrimaryKey:     false,
					IsForeignKey:     true,
					ForeignKeyTarget: "customers.id",
				},
			},
		},
	}

	prompt := BuildRelationshipAnalysisPrompt(tables, []CandidateContext{})

	// Verify FK information is included
	assert.Contains(t, prompt, "[FK→customers.id]")
}

func TestBuildRelationshipAnalysisSystemMessage(t *testing.T) {
	message := BuildRelationshipAnalysisSystemMessage()

	assert.NotEmpty(t, message)
	assert.Contains(t, message, "database")
	assert.Contains(t, message, "relationship")
	assert.Contains(t, message, "expert")
}

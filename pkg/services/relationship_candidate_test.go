package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRelationshipCandidate_OrphanRate(t *testing.T) {
	tests := []struct {
		name         string
		candidate    *RelationshipCandidate
		expectedRate float64
	}{
		{
			name: "no orphans",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 100,
				OrphanCount:         0,
			},
			expectedRate: 0.0,
		},
		{
			name: "50% orphan rate",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 100,
				OrphanCount:         50,
			},
			expectedRate: 0.5,
		},
		{
			name: "100% orphan rate",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 100,
				OrphanCount:         100,
			},
			expectedRate: 1.0,
		},
		{
			name: "zero source distinct count",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 0,
				OrphanCount:         0,
			},
			expectedRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := tt.candidate.OrphanRate()
			assert.InDelta(t, tt.expectedRate, rate, 0.001)
		})
	}
}

func TestRelationshipCandidate_MatchRate(t *testing.T) {
	tests := []struct {
		name         string
		candidate    *RelationshipCandidate
		expectedRate float64
	}{
		{
			name: "100% match rate",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 100,
				SourceMatched:       100,
			},
			expectedRate: 1.0,
		},
		{
			name: "80% match rate",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 100,
				SourceMatched:       80,
			},
			expectedRate: 0.8,
		},
		{
			name: "no matches",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 100,
				SourceMatched:       0,
			},
			expectedRate: 0.0,
		},
		{
			name: "zero source distinct count",
			candidate: &RelationshipCandidate{
				SourceDistinctCount: 0,
				SourceMatched:       0,
			},
			expectedRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := tt.candidate.MatchRate()
			assert.InDelta(t, tt.expectedRate, rate, 0.001)
		})
	}
}

func TestRelationshipCandidate_CoverageRate(t *testing.T) {
	tests := []struct {
		name         string
		candidate    *RelationshipCandidate
		expectedRate float64
	}{
		{
			name: "full coverage",
			candidate: &RelationshipCandidate{
				TargetDistinctCount: 100,
				TargetMatched:       100,
			},
			expectedRate: 1.0,
		},
		{
			name: "partial coverage",
			candidate: &RelationshipCandidate{
				TargetDistinctCount: 100,
				TargetMatched:       30,
			},
			expectedRate: 0.3,
		},
		{
			name: "no coverage",
			candidate: &RelationshipCandidate{
				TargetDistinctCount: 100,
				TargetMatched:       0,
			},
			expectedRate: 0.0,
		},
		{
			name: "zero target distinct count",
			candidate: &RelationshipCandidate{
				TargetDistinctCount: 0,
				TargetMatched:       0,
			},
			expectedRate: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rate := tt.candidate.CoverageRate()
			assert.InDelta(t, tt.expectedRate, rate, 0.001)
		})
	}
}

func TestRelationshipCandidate_Fields(t *testing.T) {
	// Test that all fields can be set and retrieved correctly
	sourceColID := uuid.New()
	targetColID := uuid.New()

	candidate := &RelationshipCandidate{
		SourceTable:         "orders",
		SourceColumn:        "user_id",
		SourceDataType:      "uuid",
		SourceIsPK:          false,
		SourceDistinctCount: 1000,
		SourceNullRate:      0.01,
		SourceSamples:       []string{"abc-123", "def-456", "ghi-789"},
		TargetTable:         "users",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetIsPK:          true,
		TargetDistinctCount: 5000,
		TargetNullRate:      0.0,
		TargetSamples:       []string{"abc-123", "xyz-999"},
		JoinCount:           10000,
		OrphanCount:         50,
		ReverseOrphans:      4000,
		SourceMatched:       950,
		TargetMatched:       1000,
		SourcePurpose:       "identifier",
		SourceRole:          "foreign_key",
		TargetPurpose:       "identifier",
		TargetRole:          "primary_key",
		SourceColumnID:      sourceColID,
		TargetColumnID:      targetColID,
	}

	assert.Equal(t, "orders", candidate.SourceTable)
	assert.Equal(t, "user_id", candidate.SourceColumn)
	assert.Equal(t, "uuid", candidate.SourceDataType)
	assert.False(t, candidate.SourceIsPK)
	assert.Equal(t, int64(1000), candidate.SourceDistinctCount)
	assert.InDelta(t, 0.01, candidate.SourceNullRate, 0.001)
	assert.Len(t, candidate.SourceSamples, 3)

	assert.Equal(t, "users", candidate.TargetTable)
	assert.Equal(t, "id", candidate.TargetColumn)
	assert.Equal(t, "uuid", candidate.TargetDataType)
	assert.True(t, candidate.TargetIsPK)

	assert.Equal(t, int64(10000), candidate.JoinCount)
	assert.Equal(t, int64(50), candidate.OrphanCount)
	assert.Equal(t, int64(4000), candidate.ReverseOrphans)
	assert.Equal(t, int64(950), candidate.SourceMatched)
	assert.Equal(t, int64(1000), candidate.TargetMatched)

	assert.Equal(t, "identifier", candidate.SourcePurpose)
	assert.Equal(t, "foreign_key", candidate.SourceRole)

	assert.Equal(t, sourceColID, candidate.SourceColumnID)
	assert.Equal(t, targetColID, candidate.TargetColumnID)
}

func TestRelationshipValidationResult_Fields(t *testing.T) {
	result := &RelationshipValidationResult{
		IsValidFK:   true,
		Confidence:  0.95,
		Cardinality: "N:1",
		Reasoning:   "The user_id column references the users table primary key with high match rate",
		SourceRole:  "owner",
	}

	assert.True(t, result.IsValidFK)
	assert.InDelta(t, 0.95, result.Confidence, 0.001)
	assert.Equal(t, "N:1", result.Cardinality)
	assert.Equal(t, "The user_id column references the users table primary key with high match rate", result.Reasoning)
	assert.Equal(t, "owner", result.SourceRole)
}

func TestValidatedRelationship_Composition(t *testing.T) {
	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	result := &RelationshipValidationResult{
		IsValidFK:   true,
		Confidence:  0.9,
		Cardinality: "N:1",
		Reasoning:   "Valid FK relationship",
	}

	validated := &ValidatedRelationship{
		Candidate: candidate,
		Result:    result,
	}

	assert.Equal(t, "orders", validated.Candidate.SourceTable)
	assert.True(t, validated.Result.IsValidFK)
}

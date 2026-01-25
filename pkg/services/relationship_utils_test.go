package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestReverseCardinality(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "N:1 becomes 1:N",
			input:    models.CardinalityNTo1,
			expected: models.Cardinality1ToN,
		},
		{
			name:     "1:N becomes N:1",
			input:    models.Cardinality1ToN,
			expected: models.CardinalityNTo1,
		},
		{
			name:     "1:1 stays 1:1",
			input:    models.Cardinality1To1,
			expected: models.Cardinality1To1,
		},
		{
			name:     "N:M stays N:M",
			input:    models.CardinalityNToM,
			expected: models.CardinalityNToM,
		},
		{
			name:     "unknown stays unknown",
			input:    models.CardinalityUnknown,
			expected: models.CardinalityUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReverseCardinality(tt.input)
			if result != tt.expected {
				t.Errorf("ReverseCardinality(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInferCardinality(t *testing.T) {
	tests := []struct {
		name          string
		sourceMatched int64
		targetMatched int64
		joinCount     int64
		expected      string
	}{
		{
			name:          "1:1 - both sides unique",
			sourceMatched: 100,
			targetMatched: 100,
			joinCount:     100,
			expected:      models.Cardinality1To1,
		},
		{
			name:          "N:1 - typical FK pattern",
			sourceMatched: 100,
			targetMatched: 10,
			joinCount:     100,
			expected:      models.CardinalityNTo1,
		},
		{
			name:          "1:N - reverse FK pattern",
			sourceMatched: 10,
			targetMatched: 100,
			joinCount:     100,
			expected:      models.Cardinality1ToN,
		},
		{
			name:          "N:M - many-to-many",
			sourceMatched: 50,
			targetMatched: 50,
			joinCount:     200,
			expected:      models.CardinalityNToM,
		},
		{
			name:          "unknown - no source matches",
			sourceMatched: 0,
			targetMatched: 100,
			joinCount:     0,
			expected:      models.CardinalityUnknown,
		},
		{
			name:          "unknown - no target matches",
			sourceMatched: 100,
			targetMatched: 0,
			joinCount:     0,
			expected:      models.CardinalityUnknown,
		},
		{
			name:          "1:1 with slight variance (within threshold)",
			sourceMatched: 100,
			targetMatched: 100,
			joinCount:     105, // ratio = 1.05, within 1.1 threshold
			expected:      models.Cardinality1To1,
		},
		{
			name:          "boundary case - exactly at threshold",
			sourceMatched: 100,
			targetMatched: 100,
			joinCount:     110, // ratio = 1.1, exactly at threshold
			expected:      models.Cardinality1To1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			join := &datasource.JoinAnalysis{
				SourceMatched: tt.sourceMatched,
				TargetMatched: tt.targetMatched,
				JoinCount:     tt.joinCount,
			}
			result := InferCardinality(join)
			if result != tt.expected {
				t.Errorf("InferCardinality() = %s, want %s", result, tt.expected)
			}
		})
	}
}

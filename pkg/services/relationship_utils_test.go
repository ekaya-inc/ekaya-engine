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
		name           string
		sourceIsPK     bool
		sourceIsUnique bool
		sourceMatched  int64
		targetMatched  int64
		expected       string
	}{
		{
			name:           "non-PK non-unique source is always N:1",
			sourceIsPK:     false,
			sourceIsUnique: false,
			sourceMatched:  100,
			targetMatched:  100,
			expected:       models.CardinalityNTo1,
		},
		{
			name:           "unique source with 1:1 mapping",
			sourceIsPK:     false,
			sourceIsUnique: true,
			sourceMatched:  100,
			targetMatched:  100,
			expected:       models.Cardinality1To1,
		},
		{
			name:           "PK source with 1:1 mapping",
			sourceIsPK:     true,
			sourceIsUnique: false,
			sourceMatched:  50,
			targetMatched:  50,
			expected:       models.Cardinality1To1,
		},
		{
			name:           "unique source without 1:1 mapping is N:1",
			sourceIsPK:     false,
			sourceIsUnique: true,
			sourceMatched:  100,
			targetMatched:  200,
			expected:       models.CardinalityNTo1,
		},
		{
			name:           "PK source without 1:1 mapping is N:1",
			sourceIsPK:     true,
			sourceIsUnique: false,
			sourceMatched:  50,
			targetMatched:  100,
			expected:       models.CardinalityNTo1,
		},
		{
			name:           "unique source with zero target matched is N:1",
			sourceIsPK:     false,
			sourceIsUnique: true,
			sourceMatched:  0,
			targetMatched:  0,
			expected:       models.CardinalityNTo1,
		},
		{
			name:           "nil join analysis defaults to N:1",
			sourceIsPK:     true,
			sourceIsUnique: true,
			sourceMatched:  0,
			targetMatched:  0,
			expected:       models.CardinalityNTo1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var join *datasource.JoinAnalysis
			if tt.sourceMatched > 0 || tt.targetMatched > 0 {
				join = &datasource.JoinAnalysis{
					SourceMatched: tt.sourceMatched,
					TargetMatched: tt.targetMatched,
				}
			}
			result := InferCardinality(tt.sourceIsPK, tt.sourceIsUnique, join)
			if result != tt.expected {
				t.Errorf("InferCardinality(isPK=%v, isUnique=%v) = %s, want %s",
					tt.sourceIsPK, tt.sourceIsUnique, result, tt.expected)
			}
		})
	}
}

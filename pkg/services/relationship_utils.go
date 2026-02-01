package services

import (
	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// CardinalityUniqueThreshold allows 10% tolerance for uniqueness detection
// to account for minor data inconsistencies or sampling variance.
// This constant is used by InferCardinality to determine if a relationship
// has a unique side (1:1, N:1, 1:N) or is many-to-many (N:M).
const CardinalityUniqueThreshold = 1.1

// InferCardinality determines the cardinality type (1:1, 1:N, N:1, N:M) from join analysis.
// It uses the ratio of join rows to matched rows on each side to determine the relationship type.
//
// - 1:1: Both sides have unique matches (ratio ≤ 1.1)
// - N:1: Multiple source rows match one target (typical FK - source unique, target not)
// - 1:N: One source matches multiple targets (reverse FK - source not unique, target unique)
// - N:M: Many-to-many relationship (neither side unique)
func InferCardinality(join *datasource.JoinAnalysis) string {
	if join.SourceMatched == 0 || join.TargetMatched == 0 {
		return models.CardinalityUnknown
	}

	// Ratio of join rows to source/target matched
	sourceRatio := float64(join.JoinCount) / float64(join.SourceMatched)
	targetRatio := float64(join.JoinCount) / float64(join.TargetMatched)

	// 1:1 - both sides have unique matches
	if sourceRatio <= CardinalityUniqueThreshold && targetRatio <= CardinalityUniqueThreshold {
		return models.Cardinality1To1
	}

	// N:1 - multiple source rows match one target (typical FK)
	if sourceRatio <= CardinalityUniqueThreshold && targetRatio > CardinalityUniqueThreshold {
		return models.CardinalityNTo1
	}

	// 1:N - one source matches multiple targets (reverse FK)
	if sourceRatio > CardinalityUniqueThreshold && targetRatio <= CardinalityUniqueThreshold {
		return models.Cardinality1ToN
	}

	// N:M - many-to-many
	return models.CardinalityNToM
}

// ReverseCardinality returns the cardinality value for the reverse direction of a relationship.
// N:1 becomes 1:N and vice versa. Symmetric cardinalities (1:1, N:M, unknown) remain unchanged.
func ReverseCardinality(cardinality string) string {
	switch cardinality {
	case models.CardinalityNTo1:
		return models.Cardinality1ToN
	case models.Cardinality1ToN:
		return models.CardinalityNTo1
	default:
		return cardinality // 1:1, N:M, unknown stay the same
	}
}

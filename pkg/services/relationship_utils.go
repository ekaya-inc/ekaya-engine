package services

import (
	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// InferCardinality determines the cardinality of a FK relationship from schema
// constraints and join statistics.
//
// For a FK relationship source.column → target.column, cardinality is determined
// by whether the source column can have duplicate values pointing to the same target:
//
//   - If the source column is NOT a PK and NOT unique, multiple source rows can
//     reference the same target row → N:1 (the standard FK pattern).
//   - If the source column IS a PK or unique, each source row references a distinct
//     target. If every matched target is referenced exactly once (sourceMatched ==
//     targetMatched), it's 1:1. Otherwise N:1.
//   - N:M is never inferred for a single FK column. N:M only applies to junction
//     tables (two N:1 relationships through a bridge table) and should be synthesized
//     at a higher level if needed.
func InferCardinality(sourceIsPK, sourceIsUnique bool, join *datasource.JoinAnalysis) string {
	if sourceIsPK || sourceIsUnique {
		if join != nil && join.TargetMatched > 0 &&
			join.SourceMatched == join.TargetMatched {
			return models.Cardinality1To1
		}
	}
	return models.CardinalityNTo1
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

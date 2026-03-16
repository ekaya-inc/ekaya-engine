package services

import (
	"strings"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// classifyJoinability determines if a column is suitable for join key consideration.
func classifyJoinability(col *models.SchemaColumn, stats *datasource.ColumnStats, tableRowCount int64) (bool, string) {
	if col.IsPrimaryKey {
		return true, models.JoinabilityPK
	}

	baseType := normalizeTypeForJoin(col.DataType)
	if isExcludedJoinType(baseType) {
		return false, models.JoinabilityTypeExcluded
	}

	if stats == nil || tableRowCount == 0 {
		return false, models.JoinabilityNoStats
	}

	if stats.DistinctCount == stats.NonNullCount && stats.NonNullCount > 0 {
		return true, models.JoinabilityUniqueValues
	}

	distinctRatio := float64(stats.DistinctCount) / float64(tableRowCount)
	if distinctRatio < 0.01 {
		return false, models.JoinabilityLowCardinality
	}

	return true, models.JoinabilityCardinalityOK
}

// normalizeTypeForJoin normalizes a column type for join type comparison.
func normalizeTypeForJoin(t string) string {
	t = strings.ToLower(t)
	if idx := strings.Index(t, "("); idx > 0 {
		t = t[:idx]
	}
	return strings.TrimSpace(t)
}

// isExcludedJoinType checks if a column type should be excluded from join consideration.
func isExcludedJoinType(baseType string) bool {
	excludedTypes := map[string]bool{
		"timestamp": true, "timestamptz": true, "date": true,
		"time": true, "timetz": true, "interval": true,
		"boolean": true, "bool": true,
		"bytea": true, "blob": true, "binary": true,
		"json": true, "jsonb": true, "xml": true,
		"point": true, "line": true, "polygon": true, "geometry": true,
	}
	return excludedTypes[baseType]
}

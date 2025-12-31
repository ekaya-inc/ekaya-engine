package services

import (
	"fmt"
	"strings"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"go.uber.org/zap"
)

// ColumnFilterResult holds the filtering decision for a single column.
type ColumnFilterResult struct {
	TableName     string
	SchemaName    string
	ColumnName    string
	DataType      string
	DistinctCount int64
	RowCount      int64
	Ratio         float64
	IsPrimaryKey  bool
	IsUnique      bool
	IsCandidate   bool
	Reason        string
}

// FilterEntityCandidates applies heuristics to identify entity candidate columns.
// Returns separate lists of candidates and excluded columns with reasons.
func FilterEntityCandidates(
	columns []*models.SchemaColumn,
	tableByID map[string]*models.SchemaTable,
	statsByTableColumn map[string]datasource.ColumnStats,
	logger *zap.Logger,
) (candidates []ColumnFilterResult, excluded []ColumnFilterResult) {
	candidates = make([]ColumnFilterResult, 0)
	excluded = make([]ColumnFilterResult, 0)

	for _, col := range columns {
		// Get table info
		table := tableByID[col.SchemaTableID.String()]
		if table == nil {
			logger.Warn("Column references unknown table",
				zap.String("column_id", col.ID.String()),
				zap.String("table_id", col.SchemaTableID.String()))
			continue
		}

		// Get column stats
		statsKey := fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, col.ColumnName)
		stats, hasStats := statsByTableColumn[statsKey]

		// Calculate ratio
		ratio := 0.0
		if hasStats && stats.RowCount > 0 {
			ratio = float64(stats.DistinctCount) / float64(stats.RowCount)
		}

		result := ColumnFilterResult{
			TableName:     table.TableName,
			SchemaName:    table.SchemaName,
			ColumnName:    col.ColumnName,
			DataType:      col.DataType,
			DistinctCount: stats.DistinctCount,
			RowCount:      stats.RowCount,
			Ratio:         ratio,
			IsPrimaryKey:  col.IsPrimaryKey,
			IsUnique:      col.IsUnique,
		}

		// Apply exclusion heuristics first (highest priority)
		if isExcludedType(col.DataType) {
			result.IsCandidate = false
			result.Reason = fmt.Sprintf("excluded type (%s)", col.DataType)
			excluded = append(excluded, result)
			continue
		}

		if isExcludedName(col.ColumnName) {
			result.IsCandidate = false
			result.Reason = fmt.Sprintf("excluded name pattern (%s)", col.ColumnName)
			excluded = append(excluded, result)
			continue
		}

		// Apply inclusion heuristics
		// Priority 1: Primary keys or unique columns
		if col.IsPrimaryKey || col.IsUnique {
			result.IsCandidate = true
			if col.IsPrimaryKey {
				result.Reason = "primary key"
			} else {
				result.Reason = "unique constraint"
			}
			candidates = append(candidates, result)
			continue
		}

		// Priority 2: Name matches entity reference pattern
		if isEntityReferenceName(col.ColumnName) {
			result.IsCandidate = true
			result.Reason = "entity reference name pattern"
			candidates = append(candidates, result)
			continue
		}

		// Priority 3: High distinct count and ratio
		if hasStats && stats.DistinctCount >= 20 && ratio > 0.05 {
			result.IsCandidate = true
			result.Reason = fmt.Sprintf("%d distinct (%.1f%% ratio)", stats.DistinctCount, ratio*100)
			candidates = append(candidates, result)
			continue
		}

		// If none of the above, exclude with reason
		result.IsCandidate = false
		if !hasStats {
			result.Reason = "no statistics available"
		} else if stats.DistinctCount < 20 {
			result.Reason = fmt.Sprintf("low distinct count (%d < 20)", stats.DistinctCount)
		} else {
			result.Reason = fmt.Sprintf("low ratio (%.1f%% < 5%%)", ratio*100)
		}
		excluded = append(excluded, result)
	}

	return candidates, excluded
}

// isExcludedType returns true for types that are unlikely to be entity references.
func isExcludedType(dataType string) bool {
	dataTypeLower := strings.ToLower(dataType)

	// Check for boolean types
	if strings.Contains(dataTypeLower, "bool") {
		return true
	}

	// Check for timestamp/date types
	if strings.Contains(dataTypeLower, "timestamp") ||
		strings.Contains(dataTypeLower, "date") {
		return true
	}

	return false
}

// isExcludedName returns true for column names that are unlikely to be entity references.
func isExcludedName(columnName string) bool {
	lowerName := strings.ToLower(columnName)

	// Timestamp patterns
	if strings.HasSuffix(lowerName, "_at") ||
		strings.HasSuffix(lowerName, "_date") {
		return true
	}

	// Boolean flag patterns
	if strings.HasPrefix(lowerName, "is_") ||
		strings.HasPrefix(lowerName, "has_") {
		return true
	}

	// Status/type/flag patterns
	if strings.HasSuffix(lowerName, "_status") ||
		strings.HasSuffix(lowerName, "_type") ||
		strings.HasSuffix(lowerName, "_flag") {
		return true
	}

	return false
}

// isEntityReferenceName returns true for column names that match entity reference patterns.
func isEntityReferenceName(columnName string) bool {
	lowerName := strings.ToLower(columnName)

	// Exact match for "id"
	if lowerName == "id" {
		return true
	}

	// Suffix patterns for entity references
	if strings.HasSuffix(lowerName, "_id") ||
		strings.HasSuffix(lowerName, "_uuid") ||
		strings.HasSuffix(lowerName, "_key") {
		return true
	}

	return false
}

// LogFilterResults logs the filtering results in a human-readable format.
func LogFilterResults(candidates, excluded []ColumnFilterResult, logger *zap.Logger) {
	logger.Info("Column filtering results:")

	// Log candidates
	if len(candidates) > 0 {
		logger.Info(fmt.Sprintf("  Entity candidates (%d):", len(candidates)))
		for _, c := range candidates {
			logger.Info(fmt.Sprintf("    CANDIDATE: %s.%s (type=%s, distinct=%d, PK=%v, Unique=%v) - %s",
				c.TableName,
				c.ColumnName,
				c.DataType,
				c.DistinctCount,
				c.IsPrimaryKey,
				c.IsUnique,
				c.Reason))
		}
	}

	// Log excluded
	if len(excluded) > 0 {
		logger.Info(fmt.Sprintf("  Excluded columns (%d):", len(excluded)))
		for _, e := range excluded {
			logger.Info(fmt.Sprintf("    EXCLUDED: %s.%s (type=%s, distinct=%d) - %s",
				e.TableName,
				e.ColumnName,
				e.DataType,
				e.DistinctCount,
				e.Reason))
		}
	}

	// Summary
	logger.Info(fmt.Sprintf("Summary: %d candidate columns, %d excluded columns",
		len(candidates), len(excluded)))
}

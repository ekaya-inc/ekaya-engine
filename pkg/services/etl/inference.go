package etl

import (
	"strconv"
	"strings"
	"time"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Common date/time layouts for parsing.
var dateLayouts = []string{
	"2006-01-02",
	"01/02/2006",
	"02/01/2006",
	"1/2/2006",
	"2006/01/02",
	"Jan 2, 2006",
	"January 2, 2006",
	"2-Jan-2006",
	"2006-1-2",
}

var timestampLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.000",
	"01/02/2006 15:04:05",
	"01/02/2006 3:04:05 PM",
	"2006-01-02T15:04:05Z07:00",
}

// InferSchema samples rows and infers SQL column types.
// Type detection priority: bool -> int -> bigint -> float -> date -> timestamp -> text.
func InferSchema(headers []string, rows [][]string, sampleSize int) []models.InferredColumn {
	if len(headers) == 0 {
		return nil
	}

	// Limit sample size
	if sampleSize <= 0 || sampleSize > len(rows) {
		sampleSize = len(rows)
	}
	sample := rows[:sampleSize]

	columns := make([]models.InferredColumn, len(headers))
	for i, name := range headers {
		columns[i] = inferColumn(name, i, sample)
	}
	return columns
}

func inferColumn(name string, colIdx int, sample [][]string) models.InferredColumn {
	col := models.InferredColumn{
		Name:    sanitizeColumnName(name),
		SQLType: "TEXT",
	}

	var values []string
	hasNull := false

	for _, row := range sample {
		if colIdx >= len(row) {
			hasNull = true
			continue
		}
		v := strings.TrimSpace(row[colIdx])
		if v == "" {
			hasNull = true
			continue
		}
		values = append(values, v)
	}

	col.Nullable = hasNull || len(values) == 0

	// Collect sample values (up to 5)
	for i, v := range values {
		if i >= 5 {
			break
		}
		col.SampleValues = append(col.SampleValues, v)
	}

	if len(values) == 0 {
		return col
	}

	// Try each type in priority order
	if allMatch(values, isBool) {
		col.SQLType = "BOOLEAN"
		return col
	}
	if allMatch(values, isInt) {
		if anyExceedsInt32(values) {
			col.SQLType = "BIGINT"
		} else {
			col.SQLType = "INTEGER"
		}
		return col
	}
	if allMatch(values, isFloat) {
		col.SQLType = "DOUBLE PRECISION"
		return col
	}
	if allMatch(values, isDate) {
		col.SQLType = "DATE"
		return col
	}
	if allMatch(values, isTimestamp) {
		col.SQLType = "TIMESTAMPTZ"
		return col
	}

	// Default: TEXT
	return col
}

func allMatch(values []string, pred func(string) bool) bool {
	for _, v := range values {
		if !pred(v) {
			return false
		}
	}
	return true
}

var boolValues = map[string]bool{
	"true": true, "false": true,
	"t": true, "f": true,
	"yes": true, "no": true,
	"y": true, "n": true,
	"1": true, "0": true,
}

func isBool(s string) bool {
	return boolValues[strings.ToLower(s)]
}

func isInt(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func anyExceedsInt32(values []string) bool {
	for _, v := range values {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		if n > 2147483647 || n < -2147483648 {
			return true
		}
	}
	return false
}

func isFloat(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func isDate(s string) bool {
	for _, layout := range dateLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

func isTimestamp(s string) bool {
	for _, layout := range timestampLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

// sanitizeColumnName converts a header into a valid SQL identifier.
func sanitizeColumnName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "column"
	}

	// Lowercase and replace common separators with underscores
	name = strings.ToLower(name)
	replacer := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"(", "",
		")", "",
		"/", "_",
		"\\", "_",
	)
	name = replacer.Replace(name)

	// Remove non-alphanumeric/underscore characters
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		return "column"
	}

	// Ensure starts with letter or underscore
	if result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}

	// Truncate to 63 chars (Postgres identifier limit)
	if len(result) > 63 {
		result = result[:63]
	}

	return result
}

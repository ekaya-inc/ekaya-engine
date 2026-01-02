package sql

import (
	"regexp"
	"strings"
)

// ParsedColumn represents a column extracted from a SELECT statement.
type ParsedColumn struct {
	Name string // The column name or alias
	Expr string // The full expression (e.g., "SUM(amount)")
}

// ParseSelectColumns extracts column names from a SELECT statement.
// This is a simple regex-based parser for common SELECT patterns.
// It handles:
// - Simple columns: SELECT id, name
// - Aliased columns: SELECT name AS customer_name, COUNT(*) AS total
// - Functions: SELECT SUM(amount), MAX(price)
// - Table-qualified columns: SELECT u.name, o.total
//
// Limitations:
// - Does not parse complex subqueries in SELECT list
// - May not handle all edge cases of nested functions
// - Assumes well-formed SQL (should be validated first)
func ParseSelectColumns(sql string) ([]ParsedColumn, error) {
	// Normalize: remove extra whitespace, convert to lowercase for pattern matching
	sql = strings.TrimSpace(sql)
	sqlLower := strings.ToLower(sql)

	// Find SELECT clause (between SELECT and FROM/WHERE/GROUP/ORDER/LIMIT/;)
	selectIdx := strings.Index(sqlLower, "select")
	if selectIdx == -1 {
		return nil, nil // Not a SELECT query
	}

	// Find end of SELECT list (first occurrence of FROM, WHERE, etc.)
	endKeywords := []string{" from ", " where ", " group ", " order ", " limit ", " union ", " intersect ", " except ", ";"}
	endIdx := len(sql)
	for _, keyword := range endKeywords {
		idx := strings.Index(sqlLower[selectIdx:], keyword)
		if idx != -1 && idx < endIdx-selectIdx {
			endIdx = selectIdx + idx
		}
	}

	// Extract the column list portion
	selectClause := sql[selectIdx+6 : endIdx] // +6 to skip "SELECT"
	selectClause = strings.TrimSpace(selectClause)

	// Handle SELECT * - return empty list (can't determine columns without schema)
	if strings.HasPrefix(strings.TrimSpace(selectClause), "*") {
		return nil, nil
	}

	// Split by comma, but be careful of commas inside function calls
	columns := splitSelectColumns(selectClause)

	var result []ParsedColumn
	for _, col := range columns {
		col = strings.TrimSpace(col)
		if col == "" {
			continue
		}

		parsed := parseColumnExpression(col)
		result = append(result, parsed)
	}

	return result, nil
}

// splitSelectColumns splits a SELECT column list by commas, respecting parentheses.
func splitSelectColumns(selectClause string) []string {
	var columns []string
	var current strings.Builder
	parenDepth := 0

	for _, ch := range selectClause {
		switch ch {
		case '(':
			parenDepth++
			current.WriteRune(ch)
		case ')':
			parenDepth--
			current.WriteRune(ch)
		case ',':
			if parenDepth == 0 {
				columns = append(columns, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Add the last column
	if current.Len() > 0 {
		columns = append(columns, current.String())
	}

	return columns
}

// parseColumnExpression parses a single column expression to extract the name/alias.
// Examples:
//   - "name" → name
//   - "u.name" → name
//   - "name AS customer_name" → customer_name
//   - "COUNT(*)" → count
//   - "SUM(amount) AS total" → total
func parseColumnExpression(expr string) ParsedColumn {
	expr = strings.TrimSpace(expr)
	exprLower := strings.ToLower(expr)

	// Check for AS alias
	asPattern := regexp.MustCompile(`\s+as\s+(\w+)\s*$`)
	if matches := asPattern.FindStringSubmatch(exprLower); matches != nil {
		return ParsedColumn{
			Name: matches[1],
			Expr: expr,
		}
	}

	// Check for implicit alias (space before last word, but not inside function)
	// Example: "COUNT(*) total" → total
	// Only check if expression has balanced parens and ends with a word not containing parens
	if strings.Count(expr, "(") == strings.Count(expr, ")") {
		parts := strings.Fields(expr)
		if len(parts) > 1 {
			lastPart := parts[len(parts)-1]
			lastPartLower := strings.ToLower(lastPart)

			// Skip if last part contains parentheses (e.g., "0)" from "COALESCE(..., 0)")
			if strings.Contains(lastPart, "(") || strings.Contains(lastPart, ")") {
				goto extractName
			}

			// Make sure it's not a SQL keyword
			keywords := []string{"from", "where", "group", "order", "limit", "and", "or", "as"}
			isKeyword := false
			for _, kw := range keywords {
				if lastPartLower == kw {
					isKeyword = true
					break
				}
			}
			if !isKeyword {
				return ParsedColumn{
					Name: lastPart,
					Expr: expr,
				}
			}
		}
	}

extractName:

	// Extract column name from various patterns
	name := extractColumnName(expr)

	return ParsedColumn{
		Name: name,
		Expr: expr,
	}
}

// extractColumnName extracts a column name from an expression.
func extractColumnName(expr string) string {
	expr = strings.TrimSpace(expr)

	// Remove table qualifiers (e.g., "users.name" → "name")
	if dotIdx := strings.LastIndex(expr, "."); dotIdx != -1 {
		expr = expr[dotIdx+1:]
	}

	// Handle function calls - extract function name
	// Example: "COUNT(*)" → "count", "SUM(amount)" → "sum"
	funcPattern := regexp.MustCompile(`^(\w+)\s*\(`)
	if matches := funcPattern.FindStringSubmatch(expr); matches != nil {
		return strings.ToLower(matches[1])
	}

	// Handle CASE expressions - use "case_result" as default
	if strings.HasPrefix(strings.ToLower(expr), "case") {
		return "case_result"
	}

	// Clean up the column name (remove quotes, backticks)
	name := strings.Trim(expr, "`\"[]")
	name = strings.TrimSpace(name)

	// Remove any remaining special characters
	name = regexp.MustCompile(`[^\w]`).ReplaceAllString(name, "")

	return strings.ToLower(name)
}

// Package sql provides SQL validation utilities.
package sql

import (
	"errors"
	"strings"
)

var (
	// ErrMultipleStatements indicates the query contains multiple SQL statements.
	ErrMultipleStatements = errors.New("multiple SQL statements not allowed; only single statements are permitted")
)

// ValidationResult contains the normalized SQL and any validation errors.
type ValidationResult struct {
	NormalizedSQL string
	Error         error
}

// ValidateAndNormalize checks SQL for multiple statements and strips the trailing semicolon.
//
// The validation order is:
// 1. Strip trailing semicolon and whitespace (normalize)
// 2. Check for multiple statements (any remaining semicolons outside string literals)
func ValidateAndNormalize(sqlQuery string) ValidationResult {
	// Trim whitespace first
	sqlQuery = strings.TrimSpace(sqlQuery)

	if sqlQuery == "" {
		return ValidationResult{NormalizedSQL: sqlQuery}
	}

	// Strip trailing semicolon first (normalize)
	normalized := stripTrailingSemicolon(sqlQuery)

	// Check for multiple statements (any semicolons remaining after normalization)
	if err := detectMultipleStatements(normalized); err != nil {
		return ValidationResult{Error: err}
	}

	return ValidationResult{NormalizedSQL: normalized}
}

// detectMultipleStatements checks if the SQL contains multiple statements
// by looking for any semicolons outside of string literals.
// Since we've already stripped the trailing semicolon, any remaining semicolon
// indicates multiple statements.
func detectMultipleStatements(sqlQuery string) error {
	if hasSemicolonOutsideStrings(sqlQuery) {
		return ErrMultipleStatements
	}
	return nil
}

// hasSemicolonOutsideStrings returns true if the SQL contains any semicolon
// outside of string literals.
func hasSemicolonOutsideStrings(sqlQuery string) bool {
	const (
		stateNormal = iota
		stateSingleQuote
		stateDoubleQuote
	)

	state := stateNormal
	prevChar := rune(0)

	for _, char := range sqlQuery {
		switch state {
		case stateNormal:
			switch char {
			case ';':
				return true // Found semicolon outside strings
			case '\'':
				state = stateSingleQuote
			case '"':
				state = stateDoubleQuote
			}
		case stateSingleQuote:
			// Exit single quote if we see an unescaped single quote
			// Handle both backslash escape (\') and SQL standard escape ('')
			if char == '\'' && prevChar != '\\' {
				// For SQL standard doubled quote (''), this will exit and immediately
				// re-enter on the next quote, which correctly keeps us in the string
				state = stateNormal
			}
		case stateDoubleQuote:
			// Exit double quote if we see an unescaped double quote
			if char == '"' && prevChar != '\\' {
				state = stateNormal
			}
		}
		prevChar = char
	}

	return false
}

// stripTrailingSemicolon removes a trailing semicolon and any whitespace after it.
func stripTrailingSemicolon(sqlQuery string) string {
	// Trim trailing whitespace first
	sqlQuery = strings.TrimRight(sqlQuery, " \t\n\r")

	// Remove trailing semicolon if present
	if strings.HasSuffix(sqlQuery, ";") {
		sqlQuery = strings.TrimSuffix(sqlQuery, ";")
		// Trim any whitespace that was before the semicolon
		sqlQuery = strings.TrimRight(sqlQuery, " \t\n\r")
	}

	return sqlQuery
}

package services

import (
	"regexp"
	"strings"
)

// SQLStatementType represents the type of SQL statement.
type SQLStatementType string

const (
	SQLTypeSelect  SQLStatementType = "SELECT"
	SQLTypeInsert  SQLStatementType = "INSERT"
	SQLTypeUpdate  SQLStatementType = "UPDATE"
	SQLTypeDelete  SQLStatementType = "DELETE"
	SQLTypeCall    SQLStatementType = "CALL"
	SQLTypeDDL     SQLStatementType = "DDL"     // CREATE, ALTER, DROP, TRUNCATE
	SQLTypeUnknown SQLStatementType = "UNKNOWN" // Unrecognized or blocked statement types
)

// modifyingCTEPattern matches CTEs that contain data-modifying operations.
// Example: WITH deleted AS (DELETE FROM ...) SELECT * FROM deleted
var modifyingCTEPattern = regexp.MustCompile(`(?i)\bAS\s*\(\s*(INSERT|UPDATE|DELETE)\b`)

// DetectSQLType determines the type of SQL statement based on the first keyword.
// Returns SQLTypeDDL for DDL statements (CREATE, ALTER, DROP, TRUNCATE) which are blocked.
// Returns SQLTypeUnknown for unrecognized statements or data-modifying CTEs.
func DetectSQLType(sql string) SQLStatementType {
	// Normalize: trim whitespace and convert to uppercase for prefix matching
	normalized := strings.ToUpper(strings.TrimSpace(sql))

	switch {
	case strings.HasPrefix(normalized, "SELECT"):
		return SQLTypeSelect

	case strings.HasPrefix(normalized, "WITH"):
		// CTEs starting with WITH could be:
		// 1. Pure SELECT: WITH cte AS (SELECT ...) SELECT * FROM cte
		// 2. Data-modifying CTE: WITH deleted AS (DELETE FROM ...) SELECT * FROM deleted
		// Block data-modifying CTEs for safety
		if containsModifyingCTE(sql) {
			return SQLTypeUnknown
		}
		return SQLTypeSelect

	case strings.HasPrefix(normalized, "INSERT"):
		return SQLTypeInsert

	case strings.HasPrefix(normalized, "UPDATE"):
		return SQLTypeUpdate

	case strings.HasPrefix(normalized, "DELETE"):
		return SQLTypeDelete

	case strings.HasPrefix(normalized, "CALL"):
		return SQLTypeCall

	// DDL statements - blocked entirely
	case strings.HasPrefix(normalized, "CREATE"),
		strings.HasPrefix(normalized, "ALTER"),
		strings.HasPrefix(normalized, "DROP"),
		strings.HasPrefix(normalized, "TRUNCATE"):
		return SQLTypeDDL

	// Transaction control - blocked (not supported in pre-approved queries)
	case strings.HasPrefix(normalized, "BEGIN"),
		strings.HasPrefix(normalized, "COMMIT"),
		strings.HasPrefix(normalized, "ROLLBACK"),
		strings.HasPrefix(normalized, "SAVEPOINT"):
		return SQLTypeUnknown

	default:
		return SQLTypeUnknown
	}
}

// containsModifyingCTE checks if a WITH clause contains data-modifying operations.
// This detects CTEs like: WITH deleted AS (DELETE FROM t RETURNING *) SELECT * FROM deleted
func containsModifyingCTE(sql string) bool {
	return modifyingCTEPattern.MatchString(sql)
}

// IsModifyingStatement returns true if the SQL statement type can modify data.
// This includes INSERT, UPDATE, DELETE, and CALL (stored procedures).
func IsModifyingStatement(sqlType SQLStatementType) bool {
	switch sqlType {
	case SQLTypeInsert, SQLTypeUpdate, SQLTypeDelete, SQLTypeCall:
		return true
	default:
		return false
	}
}

// SQLTypeError represents an error related to SQL statement type validation.
type SQLTypeError struct {
	Type    SQLStatementType
	Message string
}

func (e *SQLTypeError) Error() string {
	return e.Message
}

// ValidateSQLType validates the SQL statement type for pre-approved queries.
// Returns an error if the statement type is not allowed.
//
// Rules:
//   - DDL statements (CREATE, ALTER, DROP, TRUNCATE) are never allowed
//   - Unknown statement types are not allowed
//   - Modifying statements (INSERT, UPDATE, DELETE, CALL) require allowsModification=true
//   - SELECT statements do not require allowsModification flag
func ValidateSQLType(sql string, allowsModification bool) (SQLStatementType, error) {
	sqlType := DetectSQLType(sql)

	// Block DDL statements entirely
	if sqlType == SQLTypeDDL {
		return sqlType, &SQLTypeError{
			Type:    sqlType,
			Message: "DDL statements (CREATE, ALTER, DROP, TRUNCATE) are not allowed in pre-approved queries",
		}
	}

	// Block unknown statement types
	if sqlType == SQLTypeUnknown {
		return sqlType, &SQLTypeError{
			Type:    sqlType,
			Message: "unrecognized SQL statement type; only SELECT, INSERT, UPDATE, DELETE, and CALL are allowed",
		}
	}

	// Modifying statements require the allows_modification flag
	if IsModifyingStatement(sqlType) && !allowsModification {
		return sqlType, &SQLTypeError{
			Type:    sqlType,
			Message: "this SQL statement modifies data; enable 'Allows Modification' to save",
		}
	}

	return sqlType, nil
}

// ShouldAutoCorrectAllowsModification returns true if the allowsModification flag
// should be auto-corrected to false for SELECT statements.
// This allows users to toggle the flag off when changing from a modifying query to SELECT.
func ShouldAutoCorrectAllowsModification(sqlType SQLStatementType, allowsModification bool) bool {
	return !IsModifyingStatement(sqlType) && allowsModification
}

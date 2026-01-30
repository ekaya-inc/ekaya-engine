package tools

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mark3labs/mcp-go/mcp"
)

// ErrorResponse represents a structured error in tool results.
// This is used to return actionable error information to Claude
// as a successful tool result, ensuring error details are visible
// rather than being swallowed by the MCP client.
type ErrorResponse struct {
	Error   bool   `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// NewErrorResult creates a tool result containing a structured error.
// Use this for recoverable/actionable errors that Claude should see and
// can potentially fix (e.g., invalid parameters, resource not found).
//
// Do NOT use this for system failures (database connection errors,
// internal server errors) - those should still return Go errors.
//
// Example:
//
//	if entity == nil {
//	    return NewErrorResult("entity_not_found", "no entity named 'User' found"), nil
//	}
func NewErrorResult(code, message string) *mcp.CallToolResult {
	resp := ErrorResponse{
		Error:   true,
		Code:    code,
		Message: message,
	}
	jsonBytes, _ := json.Marshal(resp)
	result := mcp.NewToolResultText(string(jsonBytes))
	result.IsError = true
	return result
}

// NewErrorResultWithDetails creates an error result with additional context.
// The details field can contain any additional information that might help
// Claude understand and respond to the error.
//
// Example:
//
//	return NewErrorResultWithDetails(
//	    "validation_error",
//	    "invalid column names provided",
//	    map[string]any{
//	        "invalid_columns": []string{"foo", "bar"},
//	        "valid_columns": []string{"id", "name", "status"},
//	    },
//	), nil
func NewErrorResultWithDetails(code, message string, details any) *mcp.CallToolResult {
	resp := ErrorResponse{
		Error:   true,
		Code:    code,
		Message: message,
		Details: details,
	}
	jsonBytes, _ := json.Marshal(resp)
	result := mcp.NewToolResultText(string(jsonBytes))
	result.IsError = true
	return result
}

// sqlStateRegex matches PostgreSQL SQLSTATE codes in error messages like "(SQLSTATE 42601)"
var sqlStateRegex = regexp.MustCompile(`\(SQLSTATE ([0-9A-Z]{5})\)`)

// IsSQLUserError returns true if the error is a SQL user error (bad SQL, constraint
// violation, missing table, etc.) rather than a server error (connection failure,
// internal error, etc.).
//
// These errors should be returned as JSON error results, not MCP protocol errors,
// because they are actionable by the user/AI - they can fix their SQL and retry.
//
// PostgreSQL SQLSTATE class codes that indicate user errors:
//   - 22xxx: Data Exception (invalid input, division by zero)
//   - 23xxx: Integrity Constraint Violation (unique, FK, check)
//   - 42xxx: Syntax Error or Access Rule Violation
//   - 44xxx: WITH CHECK OPTION Violation
func IsSQLUserError(err error) bool {
	if err == nil {
		return false
	}

	// Check for pgconn.PgError (structured PostgreSQL error)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return isSQLStateUserError(pgErr.Code)
	}

	// Check for SQLSTATE pattern in error message (for wrapped errors)
	errStr := err.Error()
	if matches := sqlStateRegex.FindStringSubmatch(errStr); len(matches) >= 2 {
		return isSQLStateUserError(matches[1])
	}

	return false
}

// isSQLStateUserError returns true if the SQLSTATE code indicates a user error.
func isSQLStateUserError(code string) bool {
	if len(code) < 2 {
		return false
	}
	class := code[:2]
	switch class {
	case "22", // Data Exception
		"23", // Integrity Constraint Violation
		"42", // Syntax Error or Access Rule Violation
		"44": // WITH CHECK OPTION Violation
		return true
	}
	return false
}

// SQLUserErrorCode returns an appropriate error code for a SQL user error.
// Returns empty string if the error is not a SQL user error.
func SQLUserErrorCode(err error) string {
	if err == nil {
		return ""
	}

	// Check for pgconn.PgError (structured PostgreSQL error)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return mapSQLStateToCode(pgErr.Code)
	}

	// Check for SQLSTATE pattern in error message
	errStr := err.Error()
	if matches := sqlStateRegex.FindStringSubmatch(errStr); len(matches) >= 2 {
		return mapSQLStateToCode(matches[1])
	}

	return ""
}

// mapSQLStateToCode maps a SQLSTATE code to a human-readable error code.
func mapSQLStateToCode(sqlState string) string {
	if len(sqlState) < 2 {
		return "sql_error"
	}

	// Map specific SQLSTATE codes to meaningful error codes
	switch sqlState {
	case "42601": // syntax_error
		return "syntax_error"
	case "42703": // undefined_column
		return "undefined_column"
	case "42P01": // undefined_table
		return "undefined_table"
	case "42P02": // undefined_parameter
		return "undefined_parameter"
	case "23505": // unique_violation
		return "unique_violation"
	case "23503": // foreign_key_violation
		return "foreign_key_violation"
	case "23502": // not_null_violation
		return "not_null_violation"
	case "23514": // check_violation
		return "check_violation"
	case "22001": // string_data_right_truncation (value too long)
		return "value_too_long"
	case "22003": // numeric_value_out_of_range
		return "numeric_out_of_range"
	case "22007": // invalid_datetime_format
		return "invalid_datetime"
	case "22012": // division_by_zero
		return "division_by_zero"
	case "22P02": // invalid_text_representation (invalid input syntax)
		return "invalid_input"
	}

	// Fall back to class-based codes
	class := sqlState[:2]
	switch class {
	case "22":
		return "data_exception"
	case "23":
		return "constraint_violation"
	case "42":
		return "sql_error"
	case "44":
		return "check_option_violation"
	}

	return "sql_error"
}

// ExtractSQLErrorMessage extracts a clean error message from a SQL error.
// Removes the "SQLSTATE XXXXX" suffix and any "ERROR: " prefix for cleaner display.
func ExtractSQLErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	// Check for pgconn.PgError (structured PostgreSQL error)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Message
	}

	// Clean up error message string
	msg := err.Error()

	// Remove SQLSTATE suffix
	if idx := strings.Index(msg, " (SQLSTATE"); idx != -1 {
		msg = msg[:idx]
	}

	// Remove common prefixes from wrapped errors
	prefixes := []string{
		"execution failed: ",
		"query execution failed: ",
		"failed to execute statement: ",
		"error during execution: ",
		"failed to execute query: ",
		"ERROR: ",
	}
	for _, prefix := range prefixes {
		msg = strings.TrimPrefix(msg, prefix)
	}

	return msg
}

// NewSQLErrorResult creates an error result from a SQL error if it's a user error.
// Returns nil if the error is not a SQL user error (caller should return Go error instead).
//
// Example usage:
//
//	result, err := executor.Execute(ctx, sql)
//	if err != nil {
//	    if errResult := NewSQLErrorResult(err); errResult != nil {
//	        return errResult, nil
//	    }
//	    return nil, fmt.Errorf("execution failed: %w", err)
//	}
func NewSQLErrorResult(err error) *mcp.CallToolResult {
	if !IsSQLUserError(err) {
		return nil
	}
	code := SQLUserErrorCode(err)
	message := ExtractSQLErrorMessage(err)
	return NewErrorResult(code, message)
}

// inputErrorPatterns are substrings that indicate an error is due to user input
// rather than a server failure. These errors should be logged at DEBUG/INFO level,
// not ERROR level, because they are expected when users provide invalid input.
var inputErrorPatterns = []string{
	"not found",
	"validation failed",
	"SQL validation failed",
	"output_column_descriptions parameter is required",
	"already exists",
	"invalid input",
	"missing required",
	"cannot be empty",
}

// IsInputError returns true if the error appears to be caused by user input
// rather than a server failure. Input errors include:
//   - SQL user errors (syntax, constraint, missing table)
//   - Validation failures
//   - Resource not found (user provided invalid ID)
//
// These errors should be logged at DEBUG level, not ERROR level.
func IsInputError(err error) bool {
	if err == nil {
		return false
	}

	// SQL user errors are input errors
	if IsSQLUserError(err) {
		return true
	}

	// Check for common input error patterns in the error message
	errStr := strings.ToLower(err.Error())
	for _, pattern := range inputErrorPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

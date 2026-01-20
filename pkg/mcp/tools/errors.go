package tools

import (
	"encoding/json"

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
	return mcp.NewToolResultText(string(jsonBytes))
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
	return mcp.NewToolResultText(string(jsonBytes))
}

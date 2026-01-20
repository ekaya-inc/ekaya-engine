package tools

import "strings"

// trimString removes leading and trailing whitespace from a string.
// This is a common helper used across MCP tool parameter validation.
func trimString(s string) string {
	return strings.TrimSpace(s)
}

package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// trimString removes leading and trailing whitespace from a string.
// This is a common helper used across MCP tool parameter validation.
func trimString(s string) string {
	return strings.TrimSpace(s)
}

// extractArrayParam extracts an array parameter from MCP tool arguments.
// Handles both native JSON arrays and stringified JSON arrays (MCP client bug workaround).
//
// Returns:
//   - (array, nil) on success (native or parsed from string)
//   - (nil, nil) if key is absent from args
//   - (nil, error) if the value exists but cannot be parsed as an array
//
// The optional logger parameter logs a warning when the string fallback is used.
func extractArrayParam(args map[string]any, key string, logger *zap.Logger) ([]any, error) {
	val, exists := args[key]
	if !exists {
		return nil, nil
	}
	// Native JSON array — expected path
	if arr, ok := val.([]any); ok {
		return arr, nil
	}
	// Fallback: MCP client sent stringified JSON array
	if str, ok := val.(string); ok {
		var arr []any
		if err := json.Unmarshal([]byte(str), &arr); err == nil {
			if logger != nil {
				logger.Warn("MCP array parameter received as stringified JSON, used fallback deserialization",
					zap.String("param", key))
			}
			return arr, nil
		}
		return nil, fmt.Errorf(
			"parameter %q: expected a JSON array but received a string that could not be parsed as one — send as a native JSON array, not a stringified one",
			key,
		)
	}
	return nil, fmt.Errorf("parameter %q: expected a JSON array but received %T", key, val)
}

// extractStringSlice extracts a string array parameter from MCP tool arguments.
// Wraps extractArrayParam and validates that all elements are strings.
//
// Returns:
//   - ([]string, nil) on success
//   - (nil, nil) if key is absent from args
//   - (nil, error) if the value exists but is malformed or contains non-string elements
func extractStringSlice(args map[string]any, key string, logger *zap.Logger) ([]string, error) {
	arr, err := extractArrayParam(args, key, logger)
	if err != nil || arr == nil {
		return nil, err
	}
	result := make([]string, 0, len(arr))
	for i, item := range arr {
		str, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("parameter %q: element %d must be a string, got %T", key, i, item)
		}
		result = append(result, str)
	}
	return result, nil
}

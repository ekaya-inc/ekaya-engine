package jsonutil

import (
	"encoding/json"
	"fmt"
)

// FlexibleStringValue converts a json.RawMessage to a string, handling cases where
// LLMs return numbers or booleans instead of strings. Returns empty string for null/empty.
func FlexibleStringValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	// Try string first
	var strVal string
	if err := json.Unmarshal(raw, &strVal); err == nil {
		return strVal
	}

	// Try number
	var numVal float64
	if err := json.Unmarshal(raw, &numVal); err == nil {
		if numVal == float64(int64(numVal)) {
			return fmt.Sprintf("%d", int64(numVal))
		}
		return fmt.Sprintf("%g", numVal)
	}

	// Try boolean
	var boolVal bool
	if err := json.Unmarshal(raw, &boolVal); err == nil {
		return fmt.Sprintf("%t", boolVal)
	}

	// Fallback: return raw string representation
	return string(raw)
}

package sql

import (
	libinjection "github.com/corazawaf/libinjection-go"
)

// InjectionCheckResult contains the result of an injection check on a parameter value.
type InjectionCheckResult struct {
	IsSQLi      bool   // True if SQL injection pattern detected
	Fingerprint string // libinjection fingerprint of the detected pattern
	ParamName   string // Name of the parameter that failed the check
	ParamValue  any    // The value that was checked
}

// CheckParameterForInjection uses libinjection to detect SQL injection patterns
// in a parameter value.
//
// Only string values are checked - numbers, booleans, and other types cannot
// contain SQL injection patterns and will return nil (no injection detected).
//
// Returns nil if no injection is detected, or an InjectionCheckResult with
// details about the detected pattern.
//
// Example:
//
//	// Safe value - no injection
//	result := CheckParameterForInjection("customer_id", "12345")
//	// result == nil
//
//	// Injection attempt detected
//	result := CheckParameterForInjection("search", "'; DROP TABLE users--")
//	// result.IsSQLi == true
//	// result.Fingerprint == "s&1c" (or similar)
//	// result.ParamName == "search"
func CheckParameterForInjection(paramName string, value any) *InjectionCheckResult {
	// Only check string values - numbers/booleans can't contain injection
	strValue, ok := value.(string)
	if !ok {
		return nil
	}

	isSQLi, fingerprint := libinjection.IsSQLi(strValue)
	if isSQLi {
		return &InjectionCheckResult{
			IsSQLi:      true,
			Fingerprint: string(fingerprint),
			ParamName:   paramName,
			ParamValue:  value,
		}
	}

	return nil
}

// CheckAllParameters validates all parameter values for SQL injection attempts.
//
// Returns a slice of InjectionCheckResult for each parameter that failed the
// injection check. Returns an empty slice if all parameters are clean.
//
// Example:
//
//	params := map[string]any{
//	    "customer_id": "12345",                    // clean
//	    "search": "'; DROP TABLE users--",         // injection!
//	    "limit": 100,                              // clean (not a string)
//	}
//	results := CheckAllParameters(params)
//	// len(results) == 1
//	// results[0].ParamName == "search"
//	// results[0].IsSQLi == true
func CheckAllParameters(params map[string]any) []*InjectionCheckResult {
	var results []*InjectionCheckResult
	for name, value := range params {
		if result := CheckParameterForInjection(name, value); result != nil {
			results = append(results, result)
		}
	}
	return results
}

package sql

import (
	"testing"
)

func TestCheckParameterForInjection(t *testing.T) {
	tests := []struct {
		name              string
		paramName         string
		value             any
		expectInjection   bool
		expectFingerprint bool // True if we expect a non-empty fingerprint
	}{
		// Clean values - should pass
		{
			name:            "clean string value",
			paramName:       "customer_id",
			value:           "12345",
			expectInjection: false,
		},
		{
			name:            "clean email address",
			paramName:       "email",
			value:           "user@example.com",
			expectInjection: false,
		},
		{
			name:            "clean date string",
			paramName:       "start_date",
			value:           "2024-01-15",
			expectInjection: false,
		},
		{
			name:            "clean UUID",
			paramName:       "id",
			value:           "550e8400-e29b-41d4-a716-446655440000",
			expectInjection: false,
		},
		{
			name:            "clean search term",
			paramName:       "search",
			value:           "laptop computers",
			expectInjection: false,
		},
		{
			name:            "clean multi-word value",
			paramName:       "description",
			value:           "This is a normal description with spaces",
			expectInjection: false,
		},

		// Non-string values - should pass (can't contain injection)
		{
			name:            "integer value",
			paramName:       "limit",
			value:           100,
			expectInjection: false,
		},
		{
			name:            "float value",
			paramName:       "price",
			value:           99.95,
			expectInjection: false,
		},
		{
			name:            "boolean value",
			paramName:       "is_active",
			value:           true,
			expectInjection: false,
		},
		{
			name:            "nil value",
			paramName:       "optional",
			value:           nil,
			expectInjection: false,
		},

		// Classic SQL injection patterns
		{
			name:              "classic quote injection",
			paramName:         "username",
			value:             "' OR '1'='1",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:              "drop table injection",
			paramName:         "search",
			value:             "'; DROP TABLE users--",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:              "union select injection",
			paramName:         "id",
			value:             "1 UNION SELECT * FROM passwords",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:              "comment injection",
			paramName:         "filter",
			value:             "admin'--",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:              "OR injection",
			paramName:         "password",
			value:             "' OR 1=1--",
			expectInjection:   true,
			expectFingerprint: true,
		},

		// Advanced SQL injection patterns
		{
			name:              "time-based blind injection",
			paramName:         "id",
			value:             "1' AND SLEEP(5)--",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:              "stacked queries",
			paramName:         "name",
			value:             "admin'; DELETE FROM logs; --",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:            "hex encoding attempt",
			paramName:       "value",
			value:           "0x61646D696E",
			expectInjection: false, // libinjection may or may not catch this - depends on context
		},
		{
			name:              "union with null",
			paramName:         "search",
			value:             "' UNION SELECT NULL, NULL--",
			expectInjection:   true,
			expectFingerprint: true,
		},
		{
			name:              "boolean-based blind injection",
			paramName:         "id",
			value:             "1' AND '1'='1",
			expectInjection:   true,
			expectFingerprint: true,
		},

		// Edge cases
		{
			name:            "empty string",
			paramName:       "filter",
			value:           "",
			expectInjection: false,
		},
		{
			name:            "single quote alone (legitimate apostrophe)",
			paramName:       "name",
			value:           "O'Brien",
			expectInjection: false, // Single apostrophe in name is not injection
		},
		{
			name:            "double dash in text",
			paramName:       "note",
			value:           "This is a note -- with dashes",
			expectInjection: false, // Context matters - this is just text
		},
		{
			name:              "SQL keywords without injection context",
			paramName:         "description",
			value:             "SELECT the best option from the menu",
			expectInjection:   false, // Natural language, not injection
			expectFingerprint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckParameterForInjection(tt.paramName, tt.value)

			if tt.expectInjection {
				if result == nil {
					t.Errorf("expected injection detection, got nil")
					return
				}
				if !result.IsSQLi {
					t.Errorf("expected IsSQLi=true, got false")
				}
				if result.ParamName != tt.paramName {
					t.Errorf("expected ParamName=%q, got %q", tt.paramName, result.ParamName)
				}
				if result.ParamValue != tt.value {
					t.Errorf("expected ParamValue=%v, got %v", tt.value, result.ParamValue)
				}
				if tt.expectFingerprint && result.Fingerprint == "" {
					t.Errorf("expected non-empty fingerprint, got empty string")
				}
			} else {
				if result != nil {
					t.Errorf("expected no injection detection (nil), got result: %+v", result)
				}
			}
		})
	}
}

func TestCheckAllParameters(t *testing.T) {
	tests := []struct {
		name                 string
		params               map[string]any
		expectInjectionCount int
		expectParamNames     []string // Names of params expected to fail
	}{
		{
			name: "all clean parameters",
			params: map[string]any{
				"customer_id": "12345",
				"limit":       100,
				"active":      true,
				"email":       "user@example.com",
			},
			expectInjectionCount: 0,
			expectParamNames:     nil,
		},
		{
			name: "single injection attempt",
			params: map[string]any{
				"customer_id": "12345",
				"search":      "'; DROP TABLE users--",
				"limit":       100,
			},
			expectInjectionCount: 1,
			expectParamNames:     []string{"search"},
		},
		{
			name: "multiple injection attempts",
			params: map[string]any{
				"username": "admin'--",
				"password": "' OR '1'='1",
				"email":    "user@example.com",
			},
			expectInjectionCount: 2,
			expectParamNames:     []string{"username", "password"},
		},
		{
			name: "mixed types with injection",
			params: map[string]any{
				"id":      "1 UNION SELECT * FROM passwords",
				"count":   50,
				"enabled": true,
				"filter":  "normal value",
			},
			expectInjectionCount: 1,
			expectParamNames:     []string{"id"},
		},
		{
			name:                 "empty parameters map",
			params:               map[string]any{},
			expectInjectionCount: 0,
			expectParamNames:     nil,
		},
		{
			name: "all non-string parameters",
			params: map[string]any{
				"count":   100,
				"price":   99.95,
				"active":  true,
				"missing": nil,
			},
			expectInjectionCount: 0,
			expectParamNames:     nil,
		},
		{
			name: "complex injection patterns",
			params: map[string]any{
				"a": "normal",
				"b": "' OR 1=1--",
				"c": "1' AND SLEEP(5)--",
				"d": "regular text",
				"e": 12345,
			},
			expectInjectionCount: 2,
			expectParamNames:     []string{"b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := CheckAllParameters(tt.params)

			if len(results) != tt.expectInjectionCount {
				t.Errorf("expected %d injection results, got %d", tt.expectInjectionCount, len(results))
				for _, r := range results {
					t.Logf("  detected: param=%q value=%v fingerprint=%q", r.ParamName, r.ParamValue, r.Fingerprint)
				}
				return
			}

			if tt.expectInjectionCount > 0 {
				// Verify all expected parameter names are present in results
				foundNames := make(map[string]bool)
				for _, result := range results {
					foundNames[result.ParamName] = true
					if !result.IsSQLi {
						t.Errorf("result for %q has IsSQLi=false", result.ParamName)
					}
					if result.Fingerprint == "" {
						t.Errorf("result for %q has empty fingerprint", result.ParamName)
					}
				}

				for _, expectedName := range tt.expectParamNames {
					if !foundNames[expectedName] {
						t.Errorf("expected injection detection for parameter %q, but not found", expectedName)
					}
				}
			}
		})
	}
}

func TestCheckParameterForInjection_RealWorldExamples(t *testing.T) {
	// These are real-world examples of values that might appear in legitimate use
	// and should NOT be flagged as injection attempts
	cleanValues := []struct {
		name      string
		paramName string
		value     string
	}{
		{
			name:      "file path",
			paramName: "path",
			value:     "/usr/local/bin/app",
		},
		{
			name:      "JSON string",
			paramName: "config",
			value:     `{"key": "value", "enabled": true}`,
		},
		{
			name:      "email with plus",
			paramName: "email",
			value:     "user+tag@example.com",
		},
		{
			name:      "phone number",
			paramName: "phone",
			value:     "+1-555-123-4567",
		},
		{
			name:      "currency amount",
			paramName: "amount",
			value:     "$1,234.56",
		},
		{
			name:      "URL",
			paramName: "website",
			value:     "https://example.com/path?query=value&other=123",
		},
		{
			name:      "markdown text",
			paramName: "description",
			value:     "# Header\n\nThis is **bold** and *italic* text.",
		},
		{
			name:      "code snippet",
			paramName: "code",
			value:     "function test() { return true; }",
		},
	}

	for _, tt := range cleanValues {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckParameterForInjection(tt.paramName, tt.value)
			if result != nil {
				t.Errorf("legitimate value %q flagged as injection: fingerprint=%q", tt.value, result.Fingerprint)
			}
		})
	}
}

func TestCheckParameterForInjection_Fingerprints(t *testing.T) {
	// Test that we get consistent fingerprints for known injection patterns
	injectionPatterns := []struct {
		name  string
		value string
	}{
		{"classic OR", "' OR '1'='1"},
		{"union select", "1 UNION SELECT * FROM users"},
		{"drop table", "'; DROP TABLE users--"},
		{"comment injection", "admin'--"},
	}

	for _, tt := range injectionPatterns {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckParameterForInjection("test_param", tt.value)
			if result == nil {
				t.Errorf("expected injection detection for %q, got nil", tt.value)
				return
			}
			if result.Fingerprint == "" {
				t.Errorf("expected non-empty fingerprint for %q", tt.value)
			}
			// Log the fingerprint for documentation purposes
			t.Logf("Pattern %q -> Fingerprint: %q", tt.value, result.Fingerprint)
		})
	}
}

package sql

import (
	"reflect"
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestExtractParameters(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "no parameters",
			sql:      "SELECT * FROM users",
			expected: nil, // Use nil instead of empty slice
		},
		{
			name:     "single parameter",
			sql:      "SELECT * FROM users WHERE id = {{user_id}}",
			expected: []string{"user_id"},
		},
		{
			name:     "multiple parameters",
			sql:      "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}",
			expected: []string{"customer_id", "min_total"},
		},
		{
			name:     "duplicate parameter appears once",
			sql:      "SELECT * FROM transactions WHERE sender_id = {{user_id}} OR receiver_id = {{user_id}}",
			expected: []string{"user_id"},
		},
		{
			name:     "parameter with underscore",
			sql:      "SELECT * FROM orders WHERE created_at >= {{start_date}}",
			expected: []string{"start_date"},
		},
		{
			name:     "parameter starting with underscore",
			sql:      "SELECT * FROM temp WHERE value = {{_private}}",
			expected: []string{"_private"},
		},
		{
			name:     "multiple parameters in complex query",
			sql:      "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND order_date >= {{start_date}} AND order_date < {{end_date}} AND status IN ({{statuses}}) LIMIT {{limit}}",
			expected: []string{"customer_id", "start_date", "end_date", "statuses", "limit"},
		},
		{
			name:     "parameter in WHERE and HAVING",
			sql:      "SELECT category, COUNT(*) FROM products WHERE price > {{min_price}} GROUP BY category HAVING COUNT(*) >= {{min_count}}",
			expected: []string{"min_price", "min_count"},
		},
		{
			name:     "parameter used three times",
			sql:      "SELECT * FROM logs WHERE user_id = {{user_id}} OR created_by = {{user_id}} OR modified_by = {{user_id}}",
			expected: []string{"user_id"},
		},
		{
			name:     "mixed case parameter names",
			sql:      "SELECT * FROM items WHERE userId = {{userId}} AND itemType = {{itemType}}",
			expected: []string{"userId", "itemType"},
		},
		{
			name:     "parameter with numbers",
			sql:      "SELECT * FROM data WHERE field_1 = {{param_1}} AND field_2 = {{param_2}}",
			expected: []string{"param_1", "param_2"},
		},
		{
			name:     "parameter in subquery",
			sql:      "SELECT * FROM orders WHERE customer_id IN (SELECT id FROM customers WHERE status = {{status}})",
			expected: []string{"status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractParameters(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractParameters_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "empty string",
			sql:      "",
			expected: nil,
		},
		{
			name:     "only whitespace",
			sql:      "   \n\t  ",
			expected: nil,
		},
		{
			name:     "malformed placeholder - single brace",
			sql:      "SELECT * FROM users WHERE id = {user_id}",
			expected: nil,
		},
		{
			name:     "malformed placeholder - starts with number",
			sql:      "SELECT * FROM users WHERE id = {{123abc}}",
			expected: nil,
		},
		{
			name:     "malformed placeholder - contains hyphen",
			sql:      "SELECT * FROM users WHERE id = {{user-id}}",
			expected: nil,
		},
		{
			name:     "nested braces in string literal",
			sql:      "SELECT * FROM logs WHERE message = '{{not_a_param}}' AND user_id = {{user_id}}",
			expected: []string{"not_a_param", "user_id"}, // Note: regex doesn't distinguish strings, assumes template is pre-validated
		},
		{
			name:     "parameter in comment (still extracted)",
			sql:      "SELECT * FROM users -- WHERE id = {{user_id}}\nWHERE status = {{status}}",
			expected: []string{"user_id", "status"}, // Note: regex doesn't parse SQL comments
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractParameters(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateParameterDefinitions(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		params    []models.QueryParameter
		expectErr bool
		errMsg    string
	}{
		{
			name: "all parameters defined",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}",
			params: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
				{Name: "min_total", Type: "decimal", Required: false, Default: 0.00},
			},
			expectErr: false,
		},
		{
			name:      "no parameters in SQL, empty definitions",
			sql:       "SELECT * FROM users",
			params:    []models.QueryParameter{},
			expectErr: false,
		},
		{
			name: "missing parameter definition",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}",
			params: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
			},
			expectErr: true,
			errMsg:    "parameter {{min_total}} used in SQL but not defined",
		},
		{
			name: "multiple missing parameter definitions",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}} AND status = {{status}}",
			params: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
			},
			expectErr: true,
			errMsg:    "parameter {{min_total}} used in SQL but not defined", // Returns first missing param
		},
		{
			name: "parameter defined but not used",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}}",
			params: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
				{Name: "unused_param", Type: "string", Required: false},
			},
			expectErr: true,
			errMsg:    "parameter 'unused_param' is defined but not used in SQL",
		},
		{
			name: "duplicate parameter in SQL, single definition (OK)",
			sql:  "SELECT * FROM transactions WHERE sender_id = {{user_id}} OR receiver_id = {{user_id}}",
			params: []models.QueryParameter{
				{Name: "user_id", Type: "uuid", Required: true},
			},
			expectErr: false,
		},
		{
			name: "no parameters in SQL but has definitions",
			sql:  "SELECT * FROM users",
			params: []models.QueryParameter{
				{Name: "filter", Type: "string", Required: false},
			},
			expectErr: true,
			errMsg:    "parameter 'filter' is defined but not used in SQL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParameterDefinitions(tt.sql, tt.params)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("got error %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSubstituteParameters(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		paramDefs      []models.QueryParameter
		suppliedValues map[string]any
		expectedSQL    string
		expectedValues []any
	}{
		{
			name: "single parameter substitution",
			sql:  "SELECT * FROM users WHERE id = {{user_id}}",
			paramDefs: []models.QueryParameter{
				{Name: "user_id", Type: "uuid", Required: true},
			},
			suppliedValues: map[string]any{
				"user_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			expectedSQL:    "SELECT * FROM users WHERE id = $1",
			expectedValues: []any{"550e8400-e29b-41d4-a716-446655440000"},
		},
		{
			name: "multiple parameters in order",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}",
			paramDefs: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
				{Name: "min_total", Type: "decimal", Required: true},
			},
			suppliedValues: map[string]any{
				"customer_id": "550e8400-e29b-41d4-a716-446655440000",
				"min_total":   100.50,
			},
			expectedSQL:    "SELECT * FROM orders WHERE customer_id = $1 AND total > $2",
			expectedValues: []any{"550e8400-e29b-41d4-a716-446655440000", 100.50},
		},
		{
			name: "same parameter used multiple times",
			sql:  "SELECT * FROM transactions WHERE sender_id = {{user_id}} OR receiver_id = {{user_id}}",
			paramDefs: []models.QueryParameter{
				{Name: "user_id", Type: "uuid", Required: true},
			},
			suppliedValues: map[string]any{
				"user_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			expectedSQL:    "SELECT * FROM transactions WHERE sender_id = $1 OR receiver_id = $1",
			expectedValues: []any{"550e8400-e29b-41d4-a716-446655440000"},
		},
		{
			name: "default value applied when not supplied",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}",
			paramDefs: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
				{Name: "min_total", Type: "decimal", Required: false, Default: 0.00},
			},
			suppliedValues: map[string]any{
				"customer_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			expectedSQL:    "SELECT * FROM orders WHERE customer_id = $1 AND total > $2",
			expectedValues: []any{"550e8400-e29b-41d4-a716-446655440000", 0.00},
		},
		{
			name: "multiple parameters with some defaults",
			sql:  "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}} AND status = {{status}} LIMIT {{limit}}",
			paramDefs: []models.QueryParameter{
				{Name: "customer_id", Type: "uuid", Required: true},
				{Name: "min_total", Type: "decimal", Required: false, Default: 0.00},
				{Name: "status", Type: "string", Required: false, Default: "active"},
				{Name: "limit", Type: "integer", Required: false, Default: 100},
			},
			suppliedValues: map[string]any{
				"customer_id": "550e8400-e29b-41d4-a716-446655440000",
				"status":      "pending",
			},
			expectedSQL:    "SELECT * FROM orders WHERE customer_id = $1 AND total > $2 AND status = $3 LIMIT $4",
			expectedValues: []any{"550e8400-e29b-41d4-a716-446655440000", 0.00, "pending", 100},
		},
		{
			name:           "no parameters",
			sql:            "SELECT * FROM users",
			paramDefs:      []models.QueryParameter{},
			suppliedValues: map[string]any{},
			expectedSQL:    "SELECT * FROM users",
			expectedValues: nil, // Use nil instead of empty slice
		},
		{
			name: "complex query with multiple parameter occurrences",
			sql:  "SELECT * FROM logs WHERE (user_id = {{user_id}} OR created_by = {{user_id}}) AND timestamp >= {{start_time}} AND timestamp < {{end_time}} AND level = {{level}}",
			paramDefs: []models.QueryParameter{
				{Name: "user_id", Type: "uuid", Required: true},
				{Name: "start_time", Type: "timestamp", Required: true},
				{Name: "end_time", Type: "timestamp", Required: true},
				{Name: "level", Type: "string", Required: false, Default: "INFO"},
			},
			suppliedValues: map[string]any{
				"user_id":    "550e8400-e29b-41d4-a716-446655440000",
				"start_time": "2024-01-01T00:00:00Z",
				"end_time":   "2024-12-31T23:59:59Z",
			},
			expectedSQL:    "SELECT * FROM logs WHERE (user_id = $1 OR created_by = $1) AND timestamp >= $2 AND timestamp < $3 AND level = $4",
			expectedValues: []any{"550e8400-e29b-41d4-a716-446655440000", "2024-01-01T00:00:00Z", "2024-12-31T23:59:59Z", "INFO"},
		},
		{
			name: "array parameter",
			sql:  "SELECT * FROM products WHERE category IN ({{categories}})",
			paramDefs: []models.QueryParameter{
				{Name: "categories", Type: "string[]", Required: true},
			},
			suppliedValues: map[string]any{
				"categories": []string{"electronics", "books", "toys"},
			},
			expectedSQL:    "SELECT * FROM products WHERE category IN ($1)",
			expectedValues: []any{[]string{"electronics", "books", "toys"}},
		},
		{
			name: "parameter with nil default",
			sql:  "SELECT * FROM users WHERE email LIKE {{email_filter}}",
			paramDefs: []models.QueryParameter{
				{Name: "email_filter", Type: "string", Required: false, Default: nil},
			},
			suppliedValues: map[string]any{},
			expectedSQL:    "SELECT * FROM users WHERE email LIKE $1",
			expectedValues: []any{nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultSQL, resultValues, err := SubstituteParameters(tt.sql, tt.paramDefs, tt.suppliedValues)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if resultSQL != tt.expectedSQL {
				t.Errorf("SQL mismatch:\ngot:  %q\nwant: %q", resultSQL, tt.expectedSQL)
			}
			// Handle nil vs empty slice comparison
			if len(resultValues) == 0 && len(tt.expectedValues) == 0 {
				return
			}
			if !reflect.DeepEqual(resultValues, tt.expectedValues) {
				t.Errorf("values mismatch:\ngot:  %#v\nwant: %#v", resultValues, tt.expectedValues)
			}
		})
	}
}

func TestSubstituteParameters_UndefinedParameter(t *testing.T) {
	// When a parameter is used in SQL but not defined in paramDefs,
	// SubstituteParameters should leave it as-is (this should be caught by ValidateParameterDefinitions first)
	sql := "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND status = {{undefined_param}}"
	paramDefs := []models.QueryParameter{
		{Name: "customer_id", Type: "uuid", Required: true},
	}
	suppliedValues := map[string]any{
		"customer_id": "550e8400-e29b-41d4-a716-446655440000",
	}

	resultSQL, resultValues, err := SubstituteParameters(sql, paramDefs, suppliedValues)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should substitute customer_id but leave undefined_param unchanged
	expectedSQL := "SELECT * FROM orders WHERE customer_id = $1 AND status = {{undefined_param}}"
	if resultSQL != expectedSQL {
		t.Errorf("SQL mismatch:\ngot:  %q\nwant: %q", resultSQL, expectedSQL)
	}

	expectedValues := []any{"550e8400-e29b-41d4-a716-446655440000"}
	if !reflect.DeepEqual(resultValues, expectedValues) {
		t.Errorf("values mismatch:\ngot:  %#v\nwant: %#v", resultValues, expectedValues)
	}
}

func TestSubstituteParameters_ParameterOrdering(t *testing.T) {
	// Test that parameters maintain consistent ordering based on first appearance
	sql := "SELECT * FROM orders WHERE status = {{status}} AND customer_id = {{customer_id}} AND total > {{min_total}}"
	paramDefs := []models.QueryParameter{
		{Name: "customer_id", Type: "uuid", Required: true},
		{Name: "status", Type: "string", Required: true},
		{Name: "min_total", Type: "decimal", Required: true},
	}
	suppliedValues := map[string]any{
		"customer_id": "550e8400-e29b-41d4-a716-446655440000",
		"status":      "active",
		"min_total":   100.00,
	}

	resultSQL, resultValues, err := SubstituteParameters(sql, paramDefs, suppliedValues)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Parameters should be numbered in order of appearance in SQL: status ($1), customer_id ($2), min_total ($3)
	expectedSQL := "SELECT * FROM orders WHERE status = $1 AND customer_id = $2 AND total > $3"
	if resultSQL != expectedSQL {
		t.Errorf("SQL mismatch:\ngot:  %q\nwant: %q", resultSQL, expectedSQL)
	}

	expectedValues := []any{"active", "550e8400-e29b-41d4-a716-446655440000", 100.00}
	if !reflect.DeepEqual(resultValues, expectedValues) {
		t.Errorf("values mismatch:\ngot:  %#v\nwant: %#v", resultValues, expectedValues)
	}
}

func TestFindParametersInStringLiterals(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "no parameters",
			sql:      "SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "parameter outside string - OK",
			sql:      "SELECT * FROM users WHERE name = {{name}}",
			expected: nil,
		},
		{
			name:     "parameter inside string literal - problematic",
			sql:      "SELECT 'Hello {{name}}' FROM users",
			expected: []string{"name"},
		},
		{
			name:     "parameter in greeting string",
			sql:      "SELECT 'hi {{name}}' FROM users",
			expected: []string{"name"},
		},
		{
			name:     "parameter both inside and outside string",
			sql:      "SELECT 'Hello {{name}}' FROM users WHERE id = {{user_id}}",
			expected: []string{"name"},
		},
		{
			name:     "multiple parameters inside string",
			sql:      "SELECT '{{greeting}} {{name}}!' FROM users",
			expected: []string{"greeting", "name"},
		},
		{
			name:     "parameter in WHERE clause string literal",
			sql:      "SELECT * FROM logs WHERE message LIKE '%{{search}}%'",
			expected: []string{"search"},
		},
		{
			name:     "escaped single quotes - parameter still detected",
			sql:      "SELECT 'It''s {{name}}''s turn' FROM users",
			expected: []string{"name"},
		},
		{
			name:     "empty string literal - no parameters",
			sql:      "SELECT '' FROM users WHERE id = {{user_id}}",
			expected: nil,
		},
		{
			name:     "multiple string literals, one with parameter",
			sql:      "SELECT 'static' AS label, 'Hello {{name}}' AS greeting FROM users",
			expected: []string{"name"},
		},
		{
			name:     "parameter in concatenation - OK (outside quotes)",
			sql:      "SELECT 'Hello ' || {{name}} FROM users",
			expected: nil,
		},
		{
			name:     "complex query with mixed usage",
			sql:      "SELECT 'Status: {{status}}' AS label FROM orders WHERE status = {{status}} AND total > {{min_total}}",
			expected: []string{"status"},
		},
		{
			name:     "same parameter inside string appears once in result",
			sql:      "SELECT '{{name}} says hello to {{name}}' FROM users",
			expected: []string{"name"},
		},
		{
			name:     "LIMIT and OFFSET outside strings - OK",
			sql:      "SELECT * FROM users LIMIT {{limit}} OFFSET {{offset}}",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindParametersInStringLiterals(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

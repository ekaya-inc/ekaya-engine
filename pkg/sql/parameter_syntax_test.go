package sql_test

import (
	"regexp"
	"testing"
)

// TestParameterSyntaxDocumentation validates examples from the parameter syntax documentation.
// This ensures that the documented syntax patterns are valid and will work as expected.

// parameterRegex is the pattern used to extract parameter names from SQL templates.
// This matches the documented pattern: \{\{([a-zA-Z_]\w*)\}\}
// Parameters must start with a letter or underscore, followed by word characters.
var parameterRegex = regexp.MustCompile(`\{\{([a-zA-Z_]\w*)\}\}`)

func TestParameterRegexPatternMatching(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "single parameter",
			sql:      "SELECT * FROM customers WHERE id = {{customer_id}}",
			expected: []string{"customer_id"},
		},
		{
			name:     "multiple parameters",
			sql:      "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND date >= {{start_date}} AND date < {{end_date}}",
			expected: []string{"customer_id", "start_date", "end_date"},
		},
		{
			name:     "parameter used twice",
			sql:      "SELECT * FROM transactions WHERE sender_id = {{user_id}} OR receiver_id = {{user_id}}",
			expected: []string{"user_id", "user_id"}, // Same param appears twice
		},
		{
			name:     "array parameter",
			sql:      "SELECT * FROM products WHERE category IN ({{categories}})",
			expected: []string{"categories"},
		},
		{
			name:     "underscore prefix",
			sql:      "SELECT * FROM data WHERE value = {{_private_param}}",
			expected: []string{"_private_param"},
		},
		{
			name:     "mixed case",
			sql:      "SELECT * FROM users WHERE id = {{userId}} OR id = {{userID}}",
			expected: []string{"userId", "userID"}, // Case-sensitive
		},
		{
			name:     "numeric suffix",
			sql:      "SELECT * FROM data WHERE val1 = {{param_1}} AND val2 = {{param_2}}",
			expected: []string{"param_1", "param_2"},
		},
		{
			name:     "no parameters",
			sql:      "SELECT * FROM customers WHERE status = 'active'",
			expected: []string{},
		},
		{
			name:     "multiple parameters on same line",
			sql:      "SELECT * FROM orders WHERE date BETWEEN {{start}} AND {{end}} AND total > {{min}}",
			expected: []string{"start", "end", "min"},
		},
		{
			name:     "parameter in complex query",
			sql:      "SELECT u.name, COUNT(o.id) FROM users u LEFT JOIN orders o ON u.id = o.user_id WHERE u.status = {{status}} GROUP BY u.id HAVING COUNT(o.id) >= {{min_count}}",
			expected: []string{"status", "min_count"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := parameterRegex.FindAllStringSubmatch(tt.sql, -1)
			var extracted []string
			for _, match := range matches {
				if len(match) >= 2 {
					extracted = append(extracted, match[1])
				}
			}

			if len(extracted) != len(tt.expected) {
				t.Errorf("expected %d parameters, got %d", len(tt.expected), len(extracted))
				t.Errorf("expected: %v", tt.expected)
				t.Errorf("got: %v", extracted)
				return
			}

			for i, exp := range tt.expected {
				if extracted[i] != exp {
					t.Errorf("parameter %d: expected %q, got %q", i, exp, extracted[i])
				}
			}
		})
	}
}

func TestParameterRegexInvalidPatterns(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		shouldMatch bool
		reason      string
	}{
		{
			name:        "hyphen in parameter name",
			sql:         "SELECT * FROM users WHERE id = {{user-id}}",
			shouldMatch: false,
			reason:      "hyphens not allowed in parameter names",
		},
		{
			name:        "space in parameter name",
			sql:         "SELECT * FROM users WHERE id = {{user id}}",
			shouldMatch: false,
			reason:      "spaces not allowed in parameter names",
		},
		{
			name:        "dot in parameter name",
			sql:         "SELECT * FROM users WHERE id = {{user.id}}",
			shouldMatch: false,
			reason:      "dots not allowed in parameter names",
		},
		{
			name:        "starts with number",
			sql:         "SELECT * FROM data WHERE val = {{123_param}}",
			shouldMatch: false,
			reason:      "parameter names cannot start with numbers",
		},
		{
			name:        "single braces",
			sql:         "SELECT * FROM data WHERE val = {param}",
			shouldMatch: false,
			reason:      "single braces should not match - double braces required",
		},
		{
			name:        "shell variable syntax",
			sql:         "SELECT * FROM data WHERE val = ${param}",
			shouldMatch: false,
			reason:      "shell variable syntax should not match",
		},
		{
			name:        "postgresql positional parameter",
			sql:         "SELECT * FROM data WHERE val = $1",
			shouldMatch: false,
			reason:      "PostgreSQL positional parameters should not match",
		},
		{
			name:        "empty parameter name",
			sql:         "SELECT * FROM data WHERE val = {{}}",
			shouldMatch: false,
			reason:      "empty parameter names not allowed",
		},
		{
			name:        "valid underscore parameter",
			sql:         "SELECT * FROM data WHERE val = {{valid_param}}",
			shouldMatch: true,
			reason:      "underscores are valid in parameter names",
		},
		{
			name:        "valid number suffix",
			sql:         "SELECT * FROM data WHERE val = {{param123}}",
			shouldMatch: true,
			reason:      "numbers are valid after initial character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := parameterRegex.FindAllStringSubmatch(tt.sql, -1)
			didMatch := len(matches) > 0

			if didMatch != tt.shouldMatch {
				if tt.shouldMatch {
					t.Errorf("expected pattern to match but it didn't: %s", tt.reason)
				} else {
					t.Errorf("expected pattern NOT to match but it did: %s", tt.reason)
					t.Errorf("matched: %v", matches)
				}
			}
		})
	}
}

func TestDocumentationExampleExtraction(t *testing.T) {
	// These are the actual SQL examples from the documentation
	examples := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name: "basic query from docs",
			sql: `SELECT customer_name, email, created_at
FROM customers
WHERE id = {{customer_id}}`,
			expected: []string{"customer_id"},
		},
		{
			name: "multiple parameters from docs",
			sql: `SELECT customer_name, order_total, order_date
FROM orders o
JOIN customers c ON o.customer_id = c.id
WHERE c.id = {{customer_id}}
  AND o.order_date >= {{start_date}}
  AND o.order_date < {{end_date}}
ORDER BY o.order_date DESC
LIMIT {{limit}}`,
			expected: []string{"customer_id", "start_date", "end_date", "limit"},
		},
		{
			name: "array parameter from docs",
			sql: `SELECT product_name, category, price
FROM products
WHERE category IN ({{categories}})
  AND price BETWEEN {{min_price}} AND {{max_price}}
ORDER BY price ASC`,
			expected: []string{"categories", "min_price", "max_price"},
		},
		{
			name: "reused parameter from docs",
			sql: `SELECT *
FROM transactions
WHERE (sender_id = {{user_id}} OR receiver_id = {{user_id}})
  AND amount > {{min_amount}}`,
			expected: []string{"user_id", "user_id", "min_amount"},
		},
		{
			name: "complex query from docs",
			sql: `SELECT
  u.username,
  u.email,
  COUNT(o.id) AS order_count,
  SUM(o.total) AS total_spent
FROM users u
LEFT JOIN orders o ON u.id = o.user_id
WHERE u.status = {{status}}
  AND u.created_at >= {{start_date}}
  AND u.created_at < {{end_date}}
  AND ({{email_filter}} IS NULL OR u.email LIKE {{email_filter}})
GROUP BY u.id
HAVING COUNT(o.id) >= {{min_order_count}}
ORDER BY total_spent DESC
LIMIT {{limit}} OFFSET {{offset}}`,
			expected: []string{"status", "start_date", "end_date", "email_filter", "email_filter", "min_order_count", "limit", "offset"},
		},
	}

	for _, tt := range examples {
		t.Run(tt.name, func(t *testing.T) {
			matches := parameterRegex.FindAllStringSubmatch(tt.sql, -1)
			var extracted []string
			for _, match := range matches {
				if len(match) >= 2 {
					extracted = append(extracted, match[1])
				}
			}

			if len(extracted) != len(tt.expected) {
				t.Errorf("expected %d parameters, got %d", len(tt.expected), len(extracted))
				t.Errorf("expected: %v", tt.expected)
				t.Errorf("got: %v", extracted)
				return
			}

			for i, exp := range tt.expected {
				if extracted[i] != exp {
					t.Errorf("parameter %d: expected %q, got %q", i, exp, extracted[i])
				}
			}
		})
	}
}

func TestParameterNameUniqueness(t *testing.T) {
	// Test that we can correctly identify unique vs. repeated parameter names
	sql := "SELECT * FROM data WHERE a = {{param1}} OR b = {{param2}} OR c = {{param1}}"

	matches := parameterRegex.FindAllStringSubmatch(sql, -1)
	var allParams []string
	for _, match := range matches {
		if len(match) >= 2 {
			allParams = append(allParams, match[1])
		}
	}

	// Should extract 3 total matches (param1, param2, param1)
	if len(allParams) != 3 {
		t.Errorf("expected 3 parameter matches, got %d", len(allParams))
	}

	// Build unique set
	uniqueParams := make(map[string]bool)
	for _, p := range allParams {
		uniqueParams[p] = true
	}

	// Should have 2 unique parameter names (param1, param2)
	if len(uniqueParams) != 2 {
		t.Errorf("expected 2 unique parameters, got %d", len(uniqueParams))
	}

	if !uniqueParams["param1"] || !uniqueParams["param2"] {
		t.Errorf("expected param1 and param2 to be in unique set")
	}
}

func TestSubstitutionExample(t *testing.T) {
	// Test the substitution example from the documentation
	originalSQL := "SELECT * FROM orders WHERE customer_id = {{customer_id}} AND total > {{min_total}}"
	expectedSQL := "SELECT * FROM orders WHERE customer_id = $1 AND total > $2"

	// Extract parameters in order
	matches := parameterRegex.FindAllStringSubmatch(originalSQL, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(matches))
	}

	// Verify parameter names
	if matches[0][1] != "customer_id" {
		t.Errorf("first parameter should be 'customer_id', got %q", matches[0][1])
	}
	if matches[1][1] != "min_total" {
		t.Errorf("second parameter should be 'min_total', got %q", matches[1][1])
	}

	// Simulate substitution
	paramIndex := 1
	substituted := parameterRegex.ReplaceAllStringFunc(originalSQL, func(match string) string {
		replacement := "$" + string(rune('0'+paramIndex))
		paramIndex++
		return replacement
	})

	if substituted != expectedSQL {
		t.Errorf("substitution mismatch:\nexpected: %s\ngot:      %s", expectedSQL, substituted)
	}
}

func TestReusedParameterSubstitution(t *testing.T) {
	// Test that reused parameters get the same $N value
	originalSQL := "SELECT * FROM transactions WHERE (sender_id = {{user_id}} OR receiver_id = {{user_id}}) AND amount > {{min_amount}}"

	// Build parameter position map
	paramPositions := make(map[string]int)
	nextIndex := 1

	substituted := parameterRegex.ReplaceAllStringFunc(originalSQL, func(match string) string {
		paramName := parameterRegex.FindStringSubmatch(match)[1]

		// Check if we've seen this parameter before
		if pos, exists := paramPositions[paramName]; exists {
			// Reuse the same positional parameter
			return "$" + string(rune('0'+pos))
		}

		// New parameter - assign next index
		paramPositions[paramName] = nextIndex
		pos := nextIndex
		nextIndex++
		return "$" + string(rune('0'+pos))
	})

	// user_id should use $1 for both occurrences
	// min_amount should use $2
	expectedSQL := "SELECT * FROM transactions WHERE (sender_id = $1 OR receiver_id = $1) AND amount > $2"

	if substituted != expectedSQL {
		t.Errorf("reused parameter substitution mismatch:\nexpected: %s\ngot:      %s", expectedSQL, substituted)
	}

	// Verify parameter positions
	if paramPositions["user_id"] != 1 {
		t.Errorf("expected user_id at position 1, got %d", paramPositions["user_id"])
	}
	if paramPositions["min_amount"] != 2 {
		t.Errorf("expected min_amount at position 2, got %d", paramPositions["min_amount"])
	}
}

func TestParameterExtractionOrder(t *testing.T) {
	// Parameters should be extracted in order of first appearance
	sql := "SELECT * FROM data WHERE d = {{param4}} AND a = {{param1}} AND c = {{param3}} AND b = {{param1}}"

	matches := parameterRegex.FindAllStringSubmatch(sql, -1)
	var params []string
	for _, match := range matches {
		if len(match) >= 2 {
			params = append(params, match[1])
		}
	}

	// Should extract in order: param4, param1, param3, param1
	expected := []string{"param4", "param1", "param3", "param1"}
	if len(params) != len(expected) {
		t.Fatalf("expected %d parameters, got %d", len(expected), len(params))
	}

	for i, exp := range expected {
		if params[i] != exp {
			t.Errorf("parameter %d: expected %q, got %q", i, exp, params[i])
		}
	}

	// Build unique set in order of first appearance
	seen := make(map[string]bool)
	var unique []string
	for _, p := range params {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}

	// Unique list should be: param4, param1, param3
	expectedUnique := []string{"param4", "param1", "param3"}
	if len(unique) != len(expectedUnique) {
		t.Fatalf("expected %d unique parameters, got %d", len(expectedUnique), len(unique))
	}

	for i, exp := range expectedUnique {
		if unique[i] != exp {
			t.Errorf("unique parameter %d: expected %q, got %q", i, exp, unique[i])
		}
	}
}

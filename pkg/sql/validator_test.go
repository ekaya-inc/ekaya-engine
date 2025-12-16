package sql

import (
	"testing"
)

func TestValidateAndNormalize_ValidQueries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple select without semicolon",
			input:    "SELECT 1",
			expected: "SELECT 1",
		},
		{
			name:     "simple select with trailing semicolon",
			input:    "SELECT 1;",
			expected: "SELECT 1",
		},
		{
			name:     "select with trailing semicolon and whitespace",
			input:    "SELECT 1;  ",
			expected: "SELECT 1",
		},
		{
			name:     "select with leading and trailing whitespace",
			input:    "  SELECT 1  ",
			expected: "SELECT 1",
		},
		{
			name:     "select from table",
			input:    "SELECT * FROM users",
			expected: "SELECT * FROM users",
		},
		{
			name:     "select with where clause",
			input:    "SELECT * FROM users WHERE id = 1;",
			expected: "SELECT * FROM users WHERE id = 1",
		},
		{
			name:     "semicolon inside single quoted string",
			input:    "SELECT * FROM users WHERE name = 'test;test'",
			expected: "SELECT * FROM users WHERE name = 'test;test'",
		},
		{
			name:     "semicolon inside double quoted identifier",
			input:    `SELECT * FROM "table;name"`,
			expected: `SELECT * FROM "table;name"`,
		},
		{
			name:     "SQL standard escaped single quote",
			input:    "SELECT * FROM users WHERE name = 'O''Brien'",
			expected: "SELECT * FROM users WHERE name = 'O''Brien'",
		},
		{
			name:     "semicolon inside string with trailing semicolon",
			input:    "SELECT * FROM users WHERE name = 'test;test';",
			expected: "SELECT * FROM users WHERE name = 'test;test'",
		},
		{
			name:     "complex query with joins",
			input:    "SELECT u.*, o.* FROM users u JOIN orders o ON u.id = o.user_id;",
			expected: "SELECT u.*, o.* FROM users u JOIN orders o ON u.id = o.user_id",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "query with newlines",
			input:    "SELECT *\nFROM users\nWHERE id = 1;",
			expected: "SELECT *\nFROM users\nWHERE id = 1",
		},
		{
			name:     "update query",
			input:    "UPDATE users SET name = 'John' WHERE id = 1;",
			expected: "UPDATE users SET name = 'John' WHERE id = 1",
		},
		{
			name:     "insert query",
			input:    "INSERT INTO users (name) VALUES ('John');",
			expected: "INSERT INTO users (name) VALUES ('John')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAndNormalize(tt.input)
			if result.Error != nil {
				t.Errorf("unexpected error: %v", result.Error)
			}
			if result.NormalizedSQL != tt.expected {
				t.Errorf("got %q, want %q", result.NormalizedSQL, tt.expected)
			}
		})
	}
}

func TestValidateAndNormalize_MultipleStatements(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "two selects with semicolon separator",
			input: "SELECT 1; SELECT 2",
		},
		{
			name:  "two selects with semicolon separator and trailing",
			input: "SELECT 1; SELECT 2;",
		},
		{
			name:  "two selects no space after semicolon",
			input: "SELECT 1;SELECT 2",
		},
		{
			name:  "three statements",
			input: "SELECT 1; SELECT 2; SELECT 3",
		},
		{
			name:  "drop table attempt",
			input: "SELECT 1; DROP TABLE users",
		},
		{
			name:  "delete attempt",
			input: "SELECT * FROM users WHERE 1=1; DELETE FROM users",
		},
		{
			name:  "semicolon mid-statement",
			input: "SELECT 1; SELECT 2; SELECT 3;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAndNormalize(tt.input)
			if result.Error == nil {
				t.Error("expected error for multiple statements, got nil")
			}
			if result.Error != ErrMultipleStatements {
				t.Errorf("expected ErrMultipleStatements, got %v", result.Error)
			}
		})
	}
}

func TestHasSemicolonOutsideStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "no semicolons",
			input:    "SELECT 1",
			expected: false,
		},
		{
			name:     "semicolon in normal position",
			input:    "SELECT 1; SELECT 2",
			expected: true,
		},
		{
			name:     "semicolon in single quoted string",
			input:    "SELECT 'a;b'",
			expected: false,
		},
		{
			name:     "semicolon in double quoted identifier",
			input:    `SELECT "a;b"`,
			expected: false,
		},
		{
			name:     "mixed: semicolon in string and real semicolon",
			input:    "SELECT 'a;b'; SELECT 1",
			expected: true,
		},
		{
			name:     "escaped quote in string with semicolon",
			input:    "SELECT 'it''s;here'",
			expected: false,
		},
		{
			name:     "backslash escaped quote",
			input:    `SELECT 'test\';more'`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSemicolonOutsideStrings(tt.input)
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestStripTrailingSemicolon(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no semicolon",
			input:    "SELECT 1",
			expected: "SELECT 1",
		},
		{
			name:     "trailing semicolon",
			input:    "SELECT 1;",
			expected: "SELECT 1",
		},
		{
			name:     "trailing semicolon with whitespace",
			input:    "SELECT 1;  ",
			expected: "SELECT 1",
		},
		{
			name:     "whitespace before semicolon",
			input:    "SELECT 1 ;",
			expected: "SELECT 1",
		},
		{
			name:     "multiple trailing semicolons only strips one",
			input:    "SELECT 1;;",
			expected: "SELECT 1;",
		},
		{
			name:     "semicolon with tabs and newlines",
			input:    "SELECT 1;\t\n",
			expected: "SELECT 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripTrailingSemicolon(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

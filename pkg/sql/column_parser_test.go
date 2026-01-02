package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSelectColumns(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []ParsedColumn
	}{
		{
			name: "simple columns",
			sql:  "SELECT id, name, email FROM users",
			expected: []ParsedColumn{
				{Name: "id", Expr: "id"},
				{Name: "name", Expr: "name"},
				{Name: "email", Expr: "email"},
			},
		},
		{
			name: "columns with AS aliases",
			sql:  "SELECT id, name AS customer_name, email AS contact_email FROM users",
			expected: []ParsedColumn{
				{Name: "id", Expr: "id"},
				{Name: "customer_name", Expr: "name AS customer_name"},
				{Name: "contact_email", Expr: "email AS contact_email"},
			},
		},
		{
			name: "aggregate functions with aliases",
			sql:  "SELECT COUNT(*) AS total, SUM(amount) AS revenue FROM orders",
			expected: []ParsedColumn{
				{Name: "total", Expr: "COUNT(*) AS total"},
				{Name: "revenue", Expr: "SUM(amount) AS revenue"},
			},
		},
		{
			name: "table-qualified columns",
			sql:  "SELECT u.id, u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id",
			expected: []ParsedColumn{
				{Name: "id", Expr: "u.id"},
				{Name: "name", Expr: "u.name"},
				{Name: "total", Expr: "o.total"},
			},
		},
		{
			name: "mixed expressions",
			sql:  "SELECT u.name, COUNT(*) AS order_count, SUM(o.amount) AS total_amount FROM users u JOIN orders o",
			expected: []ParsedColumn{
				{Name: "name", Expr: "u.name"},
				{Name: "order_count", Expr: "COUNT(*) AS order_count"},
				{Name: "total_amount", Expr: "SUM(o.amount) AS total_amount"},
			},
		},
		{
			name: "function without alias",
			sql:  "SELECT COUNT(*), SUM(amount) FROM orders",
			expected: []ParsedColumn{
				{Name: "count", Expr: "COUNT(*)"},
				{Name: "sum", Expr: "SUM(amount)"},
			},
		},
		{
			name: "columns with implicit alias",
			sql:  "SELECT COUNT(*) total, SUM(amount) revenue FROM orders",
			expected: []ParsedColumn{
				{Name: "total", Expr: "COUNT(*) total"},
				{Name: "revenue", Expr: "SUM(amount) revenue"},
			},
		},
		{
			name:     "SELECT * returns empty",
			sql:      "SELECT * FROM users",
			expected: nil,
		},
		{
			name:     "not a SELECT query",
			sql:      "INSERT INTO users (name) VALUES ('test')",
			expected: nil,
		},
		{
			name: "complex nested functions",
			sql:  "SELECT COALESCE(SUM(amount), 0) AS total_revenue FROM orders",
			expected: []ParsedColumn{
				{Name: "total_revenue", Expr: "COALESCE(SUM(amount), 0) AS total_revenue"},
			},
		},
		{
			name: "with WHERE clause",
			sql:  "SELECT id, name FROM users WHERE status = 'active'",
			expected: []ParsedColumn{
				{Name: "id", Expr: "id"},
				{Name: "name", Expr: "name"},
			},
		},
		{
			name: "with GROUP BY",
			sql:  "SELECT customer_id, COUNT(*) AS orders FROM orders GROUP BY customer_id",
			expected: []ParsedColumn{
				{Name: "customer_id", Expr: "customer_id"},
				{Name: "orders", Expr: "COUNT(*) AS orders"},
			},
		},
		{
			name: "with ORDER BY",
			sql:  "SELECT name, email FROM users ORDER BY name",
			expected: []ParsedColumn{
				{Name: "name", Expr: "name"},
				{Name: "email", Expr: "email"},
			},
		},
		{
			name: "with LIMIT",
			sql:  "SELECT id, name FROM users LIMIT 10",
			expected: []ParsedColumn{
				{Name: "id", Expr: "id"},
				{Name: "name", Expr: "name"},
			},
		},
		{
			name: "lowercase as keyword",
			sql:  "SELECT name as customer_name FROM users",
			expected: []ParsedColumn{
				{Name: "customer_name", Expr: "name as customer_name"},
			},
		},
		{
			name: "mixed case SELECT",
			sql:  "SeLeCt id, NaMe FROM users",
			expected: []ParsedColumn{
				{Name: "id", Expr: "id"},
				{Name: "name", Expr: "NaMe"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSelectColumns(tt.sql)
			require.NoError(t, err)
			assert.Equal(t, len(tt.expected), len(result), "column count mismatch")

			for i, expected := range tt.expected {
				if i < len(result) {
					assert.Equal(t, expected.Name, result[i].Name,
						"column %d name mismatch", i)
				}
			}
		})
	}
}

func TestSplitSelectColumns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple columns",
			input:    "id, name, email",
			expected: []string{"id", " name", " email"},
		},
		{
			name:     "function with comma inside",
			input:    "id, COALESCE(name, 'Unknown'), email",
			expected: []string{"id", " COALESCE(name, 'Unknown')", " email"},
		},
		{
			name:     "nested functions",
			input:    "ROUND(AVG(amount), 2), COUNT(*)",
			expected: []string{"ROUND(AVG(amount), 2)", " COUNT(*)"},
		},
		{
			name:     "single column",
			input:    "id",
			expected: []string{"id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSelectColumns(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseColumnExpression(t *testing.T) {
	tests := []struct {
		name         string
		expr         string
		expectedName string
	}{
		{
			name:         "simple column",
			expr:         "name",
			expectedName: "name",
		},
		{
			name:         "qualified column",
			expr:         "users.name",
			expectedName: "name",
		},
		{
			name:         "AS alias",
			expr:         "name AS customer_name",
			expectedName: "customer_name",
		},
		{
			name:         "lowercase as alias",
			expr:         "name as customer_name",
			expectedName: "customer_name",
		},
		{
			name:         "function with alias",
			expr:         "COUNT(*) AS total",
			expectedName: "total",
		},
		{
			name:         "function without alias",
			expr:         "COUNT(*)",
			expectedName: "count",
		},
		{
			name:         "implicit alias",
			expr:         "COUNT(*) total",
			expectedName: "total",
		},
		{
			name:         "SUM function",
			expr:         "SUM(amount)",
			expectedName: "sum",
		},
		{
			name:         "nested function",
			expr:         "COALESCE(SUM(amount), 0)",
			expectedName: "coalesce",
		},
		{
			name:         "table.column with alias",
			expr:         "u.name AS customer_name",
			expectedName: "customer_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseColumnExpression(tt.expr)
			assert.Equal(t, tt.expectedName, result.Name)
		})
	}
}

func TestExtractColumnName(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "simple column",
			expr:     "name",
			expected: "name",
		},
		{
			name:     "qualified column",
			expr:     "users.name",
			expected: "name",
		},
		{
			name:     "quoted column",
			expr:     "`name`",
			expected: "name",
		},
		{
			name:     "double quoted column",
			expr:     "\"name\"",
			expected: "name",
		},
		{
			name:     "function",
			expr:     "COUNT(*)",
			expected: "count",
		},
		{
			name:     "case expression",
			expr:     "CASE WHEN status = 1 THEN 'active' ELSE 'inactive' END",
			expected: "case_result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractColumnName(tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

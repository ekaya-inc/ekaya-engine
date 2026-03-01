//go:build postgres || all_adapters

package postgres

import "testing"

func TestIsMultiStatement(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{
			name: "single statement no semicolon",
			sql:  "CREATE TABLE foo (id INT)",
			want: false,
		},
		{
			name: "single statement trailing semicolon",
			sql:  "CREATE TABLE foo (id INT);",
			want: false,
		},
		{
			name: "single statement trailing semicolon and whitespace",
			sql:  "CREATE TABLE foo (id INT);  \n\t  ",
			want: false,
		},
		{
			name: "two statements",
			sql:  "CREATE TYPE foo AS ENUM ('a'); CREATE TYPE bar AS ENUM ('b')",
			want: true,
		},
		{
			name: "two statements with trailing semicolon",
			sql:  "CREATE TYPE foo AS ENUM ('a'); CREATE TYPE bar AS ENUM ('b');",
			want: true,
		},
		{
			name: "multiple statements with newlines",
			sql:  "CREATE TABLE a (id INT);\nCREATE TABLE b (id INT);\nCREATE TABLE c (id INT);",
			want: true,
		},
		{
			name: "empty string",
			sql:  "",
			want: false,
		},
		{
			name: "just semicolons",
			sql:  ";;;",
			want: false,
		},
		{
			name: "select with semicolon in string literal (false positive is harmless)",
			sql:  "SELECT 'hello;world'",
			want: true, // false positive, but harmless — tx-wrapping a single statement is safe
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMultiStatement(tt.sql)
			if got != tt.want {
				t.Errorf("isMultiStatement(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

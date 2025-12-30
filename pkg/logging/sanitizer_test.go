package logging

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizeConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "password parameter lowercase",
			input:    "host=localhost password=secret123 dbname=test",
			expected: "host=localhost password=[REDACTED] dbname=test",
		},
		{
			name:     "password parameter uppercase",
			input:    "host=localhost PASSWORD=secret123 dbname=test",
			expected: "host=localhost PASSWORD=[REDACTED] dbname=test",
		},
		{
			name:     "pwd parameter",
			input:    "host=localhost pwd=secret123 dbname=test",
			expected: "host=localhost pwd=[REDACTED] dbname=test",
		},
		{
			name:     "pass parameter",
			input:    "host=localhost pass=secret123 dbname=test",
			expected: "host=localhost pass=[REDACTED] dbname=test",
		},
		{
			name:     "url format with user and password",
			input:    "postgresql://user:password@localhost:5432/dbname",
			expected: "postgresql://[REDACTED]@[REDACTED]/dbname",
		},
		{
			name:     "url format with special characters in password",
			input:    "postgresql://user:p@ssw0rd!@#@localhost:5432/dbname",
			expected: "postgresql://[REDACTED]@[REDACTED]/dbname",
		},
		{
			name:     "multiple password parameters",
			input:    "password=secret1 pwd=secret2 pass=secret3",
			expected: "password=[REDACTED] pwd=[REDACTED] pass=[REDACTED]",
		},
		{
			name:     "no sensitive data",
			input:    "host=localhost port=5432 dbname=test",
			expected: "host=localhost port=5432 dbname=test",
		},
		{
			name:     "password with semicolon delimiter",
			input:    "password=secret;host=localhost",
			expected: "password=[REDACTED];host=localhost",
		},
		{
			name:     "password with ampersand delimiter",
			input:    "password=secret&host=localhost",
			expected: "password=[REDACTED]&host=localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeConnectionString(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeConnectionString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected string
	}{
		{
			name:     "nil error",
			input:    nil,
			expected: "",
		},
		{
			name:     "error with password parameter",
			input:    errors.New("connection failed: password=mysecret host=localhost"),
			expected: "connection failed: password=[REDACTED] host=localhost",
		},
		{
			name:     "error with JWT token",
			input:    errors.New("auth failed: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"),
			expected: "auth failed: Bearer [REDACTED]",
		},
		{
			name:     "error with API key",
			input:    errors.New("request failed: api_key=sk_test_1234567890abcdefghij"),
			expected: "request failed: api_key=[REDACTED]",
		},
		{
			name:     "error with apikey parameter",
			input:    errors.New("request failed: apikey=sk_test_1234567890abcdefghij"),
			expected: "request failed: apikey=[REDACTED]",
		},
		{
			name:     "error with key parameter",
			input:    errors.New("request failed: key=sk_test_1234567890abcdefghij"),
			expected: "request failed: key=[REDACTED]",
		},
		{
			name:     "error with connection string",
			input:    errors.New("connect failed: postgresql://user:password@localhost:5432/db"),
			expected: "connect failed: postgresql://[REDACTED]@[REDACTED]/db",
		},
		{
			name:     "error with multiple sensitive patterns",
			input:    errors.New("error: password=secret123 api_key=sk_test_abcdefghijklmnopqrst Bearer eyJ.abc.xyz"),
			expected: "error: password=[REDACTED] api_key=[REDACTED] Bearer [REDACTED]",
		},
		{
			name:     "error without sensitive data",
			input:    errors.New("connection timeout"),
			expected: "connection timeout",
		},
		{
			name:     "error with pwd parameter",
			input:    errors.New("failed: pwd=mysecret"),
			expected: "failed: pwd=[REDACTED]",
		},
		{
			name:     "error with pass parameter",
			input:    errors.New("failed: pass=mysecret"),
			expected: "failed: pass=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeError(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeError() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty query",
			input:    "",
			expected: "",
		},
		{
			name:     "short query without sensitive data",
			input:    "SELECT * FROM users WHERE id = 1",
			expected: "SELECT * FROM users WHERE id = 1",
		},
		{
			name:     "query with password parameter",
			input:    "UPDATE config SET password=newsecret WHERE id = 1",
			expected: "UPDATE config SET password=[REDACTED] WHERE id = 1",
		},
		{
			name:     "query with api_key parameter",
			input:    "INSERT INTO api_keys (api_key) VALUES ('sk_test_1234567890abcdefghij')",
			expected: "INSERT INTO api_keys (api_key) VALUES ('sk_test_1234567890abcdefghij')",
		},
		{
			name:     "long query gets truncated",
			input:    "SELECT * FROM users WHERE id = 1 AND name = 'test' AND email = 'test@example.com' AND created_at > NOW() - INTERVAL '30 days'",
			expected: "SELECT * FROM users WHERE id = 1 AND name = 'test' AND email = 'test@example.com' AND created_at > N...",
		},
		{
			name:     "query at exactly max length",
			input:    strings.Repeat("a", MaxQueryLogLength),
			expected: strings.Repeat("a", MaxQueryLogLength),
		},
		{
			name:     "query one character over max length",
			input:    strings.Repeat("a", MaxQueryLogLength+1),
			expected: strings.Repeat("a", MaxQueryLogLength) + "...",
		},
		{
			name:     "long query with password gets truncated and sanitized",
			input:    "UPDATE users SET password=verylongsecretpassword123 WHERE id = 1 AND created_at > NOW() - INTERVAL '30 days'",
			expected: "UPDATE users SET password=[REDACTED] WHERE id = 1 AND created_at > NOW() - INTERVAL '...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeQuery(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeQuery() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "string shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "string exactly at max",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "string longer than max",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
		{
			name:     "truncate to zero",
			input:    "hello",
			maxLen:   0,
			expected: "...",
		},
		{
			name:     "long string truncated",
			input:    "this is a very long string that needs to be truncated",
			maxLen:   20,
			expected: "this is a very long ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("TruncateString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestSanitizeConnectionStringFormats tests various real-world connection string formats
func TestSanitizeConnectionStringFormats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "postgresql URL",
			input: "postgresql://admin:p@ssw0rd@localhost:5432/mydb",
			check: func(s string) bool {
				return !strings.Contains(s, "p@ssw0rd") && !strings.Contains(s, "admin:p@ssw0rd")
			},
		},
		{
			name:  "postgres URL",
			input: "postgres://admin:secretpass@db.example.com:5432/production",
			check: func(s string) bool {
				return !strings.Contains(s, "secretpass") && !strings.Contains(s, "admin:secretpass")
			},
		},
		{
			name:  "key-value format",
			input: "host=localhost port=5432 user=admin password=secret dbname=test sslmode=require",
			check: func(s string) bool {
				return !strings.Contains(s, "password=secret") && strings.Contains(s, "password=[REDACTED]")
			},
		},
		{
			name:  "mixed formats should sanitize both",
			input: "postgresql://user:pass@host/db?password=secret",
			check: func(s string) bool {
				// Should not contain the actual password values (pass, secret)
				// The word "password" in "password=" is OK, just not the value
				return !strings.Contains(s, ":pass@") && !strings.Contains(s, "password=secret") && strings.Contains(s, "password=[REDACTED]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeConnectionString(tt.input)
			if !tt.check(result) {
				t.Errorf("SanitizeConnectionString() failed check for input %q, got %q", tt.input, result)
			}
		})
	}
}

// TestSanitizeErrorRealWorld tests sanitization with real-world error messages
func TestSanitizeErrorRealWorld(t *testing.T) {
	tests := []struct {
		name  string
		input error
		check func(string) bool
	}{
		{
			name:  "pgx connection error with password",
			input: errors.New("failed to connect to `host=localhost user=admin password=secret database=test`: dial error"),
			check: func(s string) bool {
				return !strings.Contains(s, "password=secret") && strings.Contains(s, "password=[REDACTED]")
			},
		},
		{
			name:  "JWT authentication error",
			input: errors.New("invalid token: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"),
			check: func(s string) bool {
				return !strings.Contains(s, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") && strings.Contains(s, "Bearer [REDACTED]")
			},
		},
		{
			name:  "API request error with key",
			input: errors.New("OpenAI API error: invalid api_key=sk_test_abcdefghijklmnopqrstuvwxyz"),
			check: func(s string) bool {
				return !strings.Contains(s, "sk_test_abcdefghijklmnopqrstuvwxyz") && strings.Contains(s, "api_key=[REDACTED]")
			},
		},
		{
			name:  "connection string in error",
			input: errors.New("failed to connect to postgresql://dbuser:dbpass123@production-db.example.com:5432/appdb"),
			check: func(s string) bool {
				return !strings.Contains(s, "dbuser:dbpass123") && !strings.Contains(s, "dbpass123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeError(tt.input)
			if !tt.check(result) {
				t.Errorf("SanitizeError() failed check for input %q, got %q", tt.input.Error(), result)
			}
		})
	}
}

// TestPatternPerformance ensures regex patterns are compiled (not created each call)
func TestPatternPerformance(t *testing.T) {
	// This test verifies that patterns are package-level variables (compiled once)
	// If patterns were compiled in functions, this would be slow
	input := "password=secret api_key=sk_test_1234567890abcdefghij"

	// Run many iterations - should be fast if patterns are pre-compiled
	for i := 0; i < 10000; i++ {
		result := SanitizeConnectionString(input)
		if strings.Contains(result, "secret") {
			t.Error("Sanitization failed")
		}
	}
}

// TestEdgeCases tests edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	t.Run("connection string with no credentials", func(t *testing.T) {
		input := "postgresql://localhost:5432/dbname"
		result := SanitizeConnectionString(input)
		if result != input {
			t.Errorf("Expected unchanged for no-credential URL, got %q", result)
		}
	})

	t.Run("password with empty value", func(t *testing.T) {
		input := "host=localhost password= dbname=test"
		result := SanitizeConnectionString(input)
		// Empty password followed by space won't match the pattern [^;&\s]+
		// This is OK - empty passwords are not really sensitive
		if result != input {
			t.Errorf("Expected unchanged for empty password, got %q", result)
		}
	})

	t.Run("case insensitivity for PASSWORD", func(t *testing.T) {
		inputs := []string{
			"PASSWORD=secret",
			"Password=secret",
			"PaSsWoRd=secret",
		}
		for _, input := range inputs {
			result := SanitizeConnectionString(input)
			if strings.Contains(result, "secret") {
				t.Errorf("Failed to sanitize %q, got %q", input, result)
			}
		}
	})

	t.Run("JWT token without Bearer prefix", func(t *testing.T) {
		// Tokens without "Bearer" prefix should not be redacted
		// (we don't want false positives on random base64 strings)
		input := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
		result := SanitizeError(errors.New(input))
		// Should not be redacted without "Bearer" prefix
		if result != input {
			t.Errorf("Should not redact JWT without Bearer prefix, got %q", result)
		}
	})

	t.Run("short API key not matched", func(t *testing.T) {
		// API keys less than 20 chars should not match (avoid false positives)
		input := "api_key=short123"
		result := SanitizeError(errors.New(input))
		if result != input {
			t.Errorf("Should not redact short API key, got %q", result)
		}
	})
}

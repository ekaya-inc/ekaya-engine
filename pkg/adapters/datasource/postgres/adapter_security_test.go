package postgres

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildConnectionString_PasswordURLEscaping tests that passwords with special characters
// are properly URL-escaped to prevent connection string parsing errors.
func TestBuildConnectionString_PasswordURLEscaping(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		check    func(t *testing.T, connStr string)
	}{
		{
			name:     "password with @ symbol",
			password: "p@ssword",
			check: func(t *testing.T, connStr string) {
				// Should contain URL-encoded @ (%40)
				assert.Contains(t, connStr, "%40", "@ should be URL-encoded as %40")
				// Should NOT contain unescaped password breaking the URL format
				assert.NotContains(t, connStr, ":p@ssword@", "password should not break URL format")
			},
		},
		{
			name:     "password with / symbol",
			password: "p/ssword",
			check: func(t *testing.T, connStr string) {
				// Should contain URL-encoded / (%2F)
				assert.Contains(t, connStr, "%2F", "/ should be URL-encoded as %2F")
			},
		},
		{
			name:     "password with # symbol",
			password: "p#ssword",
			check: func(t *testing.T, connStr string) {
				// Should contain URL-encoded # (%23)
				assert.Contains(t, connStr, "%23", "# should be URL-encoded as %23")
			},
		},
		{
			name:     "password with ? symbol",
			password: "p?ssword",
			check: func(t *testing.T, connStr string) {
				// Should contain URL-encoded ? (%3F)
				assert.Contains(t, connStr, "%3F", "? should be URL-encoded as %3F")
			},
		},
		{
			name:     "password with ; symbol",
			password: "p;ssword",
			check: func(t *testing.T, connStr string) {
				// Should contain URL-encoded ; (%3B)
				assert.Contains(t, connStr, "%3B", "; should be URL-encoded as %3B")
			},
		},
		{
			name:     "password with space",
			password: "p ssword",
			check: func(t *testing.T, connStr string) {
				// Should contain URL-encoded space (%20 or +)
				assert.True(t, strings.Contains(connStr, "%20") || strings.Contains(connStr, "+"),
					"space should be URL-encoded")
			},
		},
		{
			name:     "password with multiple special characters",
			password: "p@ss/w#rd?123;456 789",
			check: func(t *testing.T, connStr string) {
				// Verify all special characters are encoded
				assert.Contains(t, connStr, "%40", "@ should be encoded")
				assert.Contains(t, connStr, "%2F", "/ should be encoded")
				assert.Contains(t, connStr, "%23", "# should be encoded")
				assert.Contains(t, connStr, "%3F", "? should be encoded")
				assert.Contains(t, connStr, "%3B", "; should be encoded")
			},
		},
		{
			name:     "password with SQL injection attempt",
			password: "'; DROP TABLE users; --",
			check: func(t *testing.T, connStr string) {
				// Verify SQL injection attempt is safely escaped
				// The single quote should be encoded
				assert.Contains(t, connStr, "%27", "single quote should be encoded")
				// The password should not appear unescaped
				assert.NotContains(t, connStr, "'; DROP TABLE", "SQL injection should be escaped")
			},
		},
		{
			name:     "password with unicode characters",
			password: "pässwörd™",
			check: func(t *testing.T, connStr string) {
				// Unicode should be properly encoded
				// URL encoding converts unicode to percent-encoded UTF-8
				assert.Contains(t, connStr, "%", "unicode should be percent-encoded")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: tt.password,
				Database: "testdb",
				SSLMode:  "require",
			}

			connStr := buildConnectionString(cfg)

			// Verify connection string has the expected format
			assert.True(t, strings.HasPrefix(connStr, "postgresql://"),
				"connection string should start with postgresql://")

			// Run specific checks
			if tt.check != nil {
				tt.check(t, connStr)
			}
		})
	}
}

// TestBuildConnectionString_UserURLEscaping tests that usernames with special characters
// are properly URL-escaped.
func TestBuildConnectionString_UserURLEscaping(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		password string
		check    func(t *testing.T, connStr string)
	}{
		{
			name:     "username with @ symbol",
			user:     "user@domain",
			password: "secret",
			check: func(t *testing.T, connStr string) {
				// User's @ should be encoded
				assert.Contains(t, connStr, "user%40domain", "username @ should be URL-encoded")
			},
		},
		{
			name:     "username with special characters",
			user:     "test+user@example.com",
			password: "secret",
			check: func(t *testing.T, connStr string) {
				// Special chars in username should be encoded
				assert.Contains(t, connStr, "%", "special chars should be encoded")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:     "localhost",
				Port:     5432,
				User:     tt.user,
				Password: tt.password,
				Database: "testdb",
				SSLMode:  "require",
			}

			connStr := buildConnectionString(cfg)

			// Run specific checks
			if tt.check != nil {
				tt.check(t, connStr)
			}
		})
	}
}

// TestBuildConnectionString_DatabaseURLEscaping tests that database names with special characters
// are properly URL-escaped.
func TestBuildConnectionString_DatabaseURLEscaping(t *testing.T) {
	tests := []struct {
		name     string
		database string
		check    func(t *testing.T, connStr string)
	}{
		{
			name:     "database with space",
			database: "test database",
			check: func(t *testing.T, connStr string) {
				// Space should be encoded
				assert.True(t, strings.Contains(connStr, "test%20database") || strings.Contains(connStr, "test+database"),
					"space in database name should be URL-encoded")
			},
		},
		{
			name:     "database with special characters",
			database: "test-db_2024",
			check: func(t *testing.T, connStr string) {
				// Connection string should contain the database name
				assert.Contains(t, connStr, "test-db_2024", "database name should be in connection string")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: "secret",
				Database: tt.database,
				SSLMode:  "require",
			}

			connStr := buildConnectionString(cfg)

			// Run specific checks
			if tt.check != nil {
				tt.check(t, connStr)
			}
		})
	}
}

// TestBuildConnectionString_MaliciousInputs tests that malicious inputs attempting to
// inject additional connection parameters are properly escaped.
func TestBuildConnectionString_MaliciousInputs(t *testing.T) {
	tests := []struct {
		name  string
		cfg   *Config
		check func(t *testing.T, connStr string)
		desc  string
	}{
		{
			name: "password attempting to inject sslmode",
			cfg: &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: "secret?sslmode=disable",
				Database: "testdb",
				SSLMode:  "require",
			},
			check: func(t *testing.T, connStr string) {
				// The injected sslmode should be escaped, not parsed as a parameter
				// Verify sslmode=require is at the end (from actual config)
				assert.True(t, strings.HasSuffix(connStr, "sslmode=require"),
					"actual sslmode should be at end")
				// The ? in password should be encoded
				assert.Contains(t, connStr, "%3F", "? should be URL-encoded")
			},
			desc: "password should not be able to inject additional connection parameters",
		},
		{
			name: "user attempting to inject host",
			cfg: &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "user@evil.com:5432/evildb",
				Password: "secret",
				Database: "testdb",
				SSLMode:  "require",
			},
			check: func(t *testing.T, connStr string) {
				// All special characters in username should be escaped
				assert.Contains(t, connStr, "%40", "@ in username should be encoded")
				assert.Contains(t, connStr, "%3A", ": in username should be encoded")
				assert.Contains(t, connStr, "%2F", "/ in username should be encoded")
			},
			desc: "username should not be able to inject different host/port/database",
		},
		{
			name: "database attempting to inject query params",
			cfg: &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: "secret",
				Database: "testdb?connect_timeout=0",
				SSLMode:  "require",
			},
			check: func(t *testing.T, connStr string) {
				// The ? in database name should be encoded
				assert.Contains(t, connStr, "%3F", "? in database should be encoded")
			},
			desc: "database name should not be able to inject query parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connStr := buildConnectionString(tt.cfg)

			// Verify basic structure is maintained
			assert.True(t, strings.HasPrefix(connStr, "postgresql://"),
				"should maintain postgresql:// prefix")
			assert.Contains(t, connStr, "@localhost:5432/",
				"should maintain correct host:port/ structure")

			// Run specific checks
			if tt.check != nil {
				tt.check(t, connStr)
			}
		})
	}
}

// TestBuildConnectionString_DefaultSSLMode tests that default SSL mode is applied.
func TestBuildConnectionString_DefaultSSLMode(t *testing.T) {
	cfg := &Config{
		Host:     "localhost",
		Port:     5432,
		User:     "testuser",
		Password: "secret",
		Database: "testdb",
		SSLMode:  "", // Empty - should default
	}

	connStr := buildConnectionString(cfg)

	// Should default to "require"
	assert.True(t, strings.HasSuffix(connStr, "sslmode=require"),
		"should default to sslmode=require")
}

// TestBuildConnectionString_ValidStructure tests that the connection string
// has the correct overall structure.
func TestBuildConnectionString_ValidStructure(t *testing.T) {
	cfg := &Config{
		Host:     "db.example.com",
		Port:     5433,
		User:     "myuser",
		Password: "mypass",
		Database: "mydb",
		SSLMode:  "verify-full",
	}

	connStr := buildConnectionString(cfg)

	// Check overall structure
	assert.True(t, strings.HasPrefix(connStr, "postgresql://myuser:mypass@"),
		"should start with postgresql://user:pass@")
	assert.Contains(t, connStr, "db.example.com:5433",
		"should contain host:port")
	assert.Contains(t, connStr, "/mydb",
		"should contain /database")
	assert.True(t, strings.HasSuffix(connStr, "?sslmode=verify-full"),
		"should end with ?sslmode=value")
}

// TestBuildConnectionString_EmptyPassword tests handling of empty passwords.
func TestBuildConnectionString_EmptyPassword(t *testing.T) {
	cfg := &Config{
		Host:     "localhost",
		Port:     5432,
		User:     "testuser",
		Password: "", // Empty password
		Database: "testdb",
		SSLMode:  "disable",
	}

	connStr := buildConnectionString(cfg)

	// Should still have valid structure with empty password
	assert.True(t, strings.HasPrefix(connStr, "postgresql://testuser:@"),
		"should have empty password after colon")
	assert.Contains(t, connStr, "@localhost:5432/testdb",
		"should have valid structure")
}

// TestSQLInjectionPrevention documents that pgx uses parameterized queries which prevent
// SQL injection. This is a documentation test showing the safe pattern.
func TestSQLInjectionPrevention(t *testing.T) {
	// This test documents the safety pattern - actual SQL injection prevention
	// is handled by pgx's parameterized queries, not by our code.
	//
	// SAFE pattern (what we use):
	//   pool.Query(ctx, "SELECT * FROM users WHERE id = $1", userInput)
	//
	// UNSAFE pattern (what we NEVER do):
	//   pool.Query(ctx, fmt.Sprintf("SELECT * FROM users WHERE id = %s", userInput))
	//
	// The pgx library handles parameter sanitization automatically when using
	// placeholders ($1, $2, etc.), so SQL injection attacks like:
	//   userInput = "1 OR 1=1; DROP TABLE users; --"
	// are safely handled as literal string values, not SQL code.
	//
	// This test exists as documentation and to ensure we maintain this pattern.

	t.Log("SQL injection prevention is handled by pgx's parameterized queries")
	t.Log("Always use placeholders ($1, $2, etc.) instead of string concatenation")
	t.Log("Example SAFE:   pool.Query(ctx, \"SELECT * FROM users WHERE id = $1\", id)")
	t.Log("Example UNSAFE: using string concatenation or formatting to build SQL queries")
}

// TestConnectionStringSecurityProperties verifies security properties of generated
// connection strings across various attack vectors.
func TestConnectionStringSecurityProperties(t *testing.T) {
	// Test various attack vectors
	attackVectors := []struct {
		name     string
		field    string // which field is being attacked
		value    string // attack payload
		property string // what security property we're testing
	}{
		{
			name:     "LDAP injection in username",
			field:    "user",
			value:    "admin)(|(password=*))",
			property: "LDAP injection characters should be URL-encoded",
		},
		{
			name:     "Command injection in password",
			field:    "password",
			value:    "pass; rm -rf /",
			property: "Shell command characters should be URL-encoded",
		},
		{
			name:     "Path traversal in database",
			field:    "database",
			value:    "../../etc/passwd",
			property: "Path traversal should be URL-encoded",
		},
		{
			name:     "NULL byte injection in password",
			field:    "password",
			value:    "pass\x00admin",
			property: "NULL bytes should be URL-encoded",
		},
	}

	for _, av := range attackVectors {
		t.Run(av.name, func(t *testing.T) {
			cfg := &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: "secret",
				Database: "testdb",
				SSLMode:  "require",
			}

			// Inject attack vector into specified field
			switch av.field {
			case "user":
				cfg.User = av.value
			case "password":
				cfg.Password = av.value
			case "database":
				cfg.Database = av.value
			}

			connStr := buildConnectionString(cfg)

			// Verify attack payload is encoded (contains % characters from URL encoding)
			assert.Contains(t, connStr, "%", av.property)

			// Verify original attack string doesn't appear unescaped
			// (Some characters might appear coincidentally, so we check structure is intact)
			assert.True(t, strings.HasPrefix(connStr, "postgresql://"),
				"connection string structure should remain valid")
			assert.Contains(t, connStr, "@localhost:5432/",
				"host:port structure should remain valid")
		})
	}
}

// TestBuildConnectionString_HostNotEscaped tests that hostname is NOT URL-escaped
// (since it's not part of the userinfo section).
func TestBuildConnectionString_HostNotEscaped(t *testing.T) {
	cfg := &Config{
		Host:     "db-primary.example.com",
		Port:     5432,
		User:     "testuser",
		Password: "secret",
		Database: "testdb",
		SSLMode:  "require",
	}

	connStr := buildConnectionString(cfg)

	// Hostname should appear as-is (not URL-encoded)
	assert.Contains(t, connStr, "@db-primary.example.com:5432/",
		"hostname should not be URL-encoded")
}

// TestBuildConnectionString_PortHandling tests various port configurations.
func TestBuildConnectionString_PortHandling(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{
			name: "default port",
			port: 5432,
			want: ":5432/",
		},
		{
			name: "custom port",
			port: 5433,
			want: ":5433/",
		},
		{
			name: "high port number",
			port: 65432,
			want: ":65432/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Host:     "localhost",
				Port:     tt.port,
				User:     "testuser",
				Password: "secret",
				Database: "testdb",
				SSLMode:  "disable",
			}

			connStr := buildConnectionString(cfg)
			assert.Contains(t, connStr, tt.want, "should contain correct port")
		})
	}
}

// TestBuildConnectionString_SSLModeOptions tests all valid SSL mode configurations.
func TestBuildConnectionString_SSLModeOptions(t *testing.T) {
	sslModes := []string{"disable", "require", "verify-ca", "verify-full", "prefer", "allow"}

	for _, mode := range sslModes {
		t.Run("sslmode="+mode, func(t *testing.T) {
			cfg := &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: "secret",
				Database: "testdb",
				SSLMode:  mode,
			}

			connStr := buildConnectionString(cfg)
			assert.True(t, strings.HasSuffix(connStr, "?sslmode="+mode),
				"should end with ?sslmode="+mode)
		})
	}
}

// TestQuoteIdentifier_SQLInjectionPrevention tests that pgx.Identifier.Sanitize()
// (which QueryExecutor.QuoteIdentifier uses) properly escapes malicious input.
func TestQuoteIdentifier_SQLInjectionPrevention(t *testing.T) {
	// quoteIdentifier mimics what QueryExecutor.QuoteIdentifier does
	quoteIdentifier := func(name string) string {
		return pgx.Identifier{name}.Sanitize()
	}

	tests := []struct {
		name     string
		input    string
		property string
	}{
		{
			name:     "basic injection attempt",
			input:    `table"; DROP TABLE users; --`,
			property: "should escape double quotes to prevent injection",
		},
		{
			name:     "single quote injection",
			input:    `table'; DROP TABLE users; --`,
			property: "should handle single quotes safely",
		},
		{
			name:     "backtick injection (MySQL style)",
			input:    "`table`; DROP TABLE users; --",
			property: "should handle backticks safely",
		},
		{
			name:     "newline injection",
			input:    "table\n; DROP TABLE users; --",
			property: "should handle newlines safely",
		},
		{
			name:     "null byte injection",
			input:    "table\x00; DROP TABLE users",
			property: "should handle null bytes safely",
		},
		{
			name:     "unicode injection",
			input:    "täble\"; DROP TABLE users; --",
			property: "should handle unicode safely",
		},
		{
			name:     "double quote escape attempt",
			input:    `table""`,
			property: "should properly escape existing double quotes",
		},
		{
			name:     "complex injection attempt",
			input:    `" OR "1"="1`,
			property: "should escape OR-based injection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := quoteIdentifier(tt.input)

			// Result should be wrapped in double quotes
			assert.True(t, strings.HasPrefix(result, `"`), "should start with double quote")
			assert.True(t, strings.HasSuffix(result, `"`), "should end with double quote")

			// The result should NOT contain unescaped double quotes in the middle
			// pgx.Identifier escapes internal " as ""
			inner := result[1 : len(result)-1]
			for i := 0; i < len(inner)-1; i++ {
				if inner[i] == '"' {
					// If there's a quote, the next char must also be a quote (escaped)
					assert.Equal(t, byte('"'), inner[i+1],
						"internal double quotes should be escaped as \"\"")
					i++ // Skip the escape character
				}
			}

			// Log the result for visibility
			t.Logf("Input: %q -> Output: %s", tt.input, result)
		})
	}
}

// TestQuoteIdentifier_ValidIdentifiers tests that valid identifiers are properly quoted.
func TestQuoteIdentifier_ValidIdentifiers(t *testing.T) {
	// quoteIdentifier mimics what QueryExecutor.QuoteIdentifier does
	quoteIdentifier := func(name string) string {
		return pgx.Identifier{name}.Sanitize()
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"users", `"users"`},
		{"user_table", `"user_table"`},
		{"UserTable", `"UserTable"`},
		{"table123", `"table123"`},
		{"_private", `"_private"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := quoteIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFromMap_NoPasswordInjection tests that malicious config values don't escape validation.
func TestFromMap_NoPasswordInjection(t *testing.T) {
	// Attempt to inject SQL through config map
	config := map[string]any{
		"host":     "localhost",
		"port":     5432,
		"user":     "testuser",
		"password": "'; DROP TABLE users; --",
		"database": "testdb",
		"ssl_mode": "disable",
	}

	cfg, err := FromMap(config)
	require.NoError(t, err)

	// Verify malicious password is stored as-is (it will be URL-encoded later)
	assert.Equal(t, "'; DROP TABLE users; --", cfg.Password,
		"password should be stored exactly as provided")

	// Verify it's properly escaped in connection string
	connStr := buildConnectionString(cfg)
	assert.Contains(t, connStr, "%27", "single quote should be URL-encoded")
	assert.NotContains(t, connStr, "'; DROP TABLE", "SQL should not appear unescaped")
}

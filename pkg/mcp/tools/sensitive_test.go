package tools

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Column Name Detection Tests
// ============================================================================

func TestSensitiveDetector_IsSensitiveColumn(t *testing.T) {
	detector := NewSensitiveDetector()

	tests := []struct {
		name       string
		columnName string
		expected   bool
	}{
		// API key patterns
		{"api_key", "api_key", true},
		{"api-key", "api-key", true},
		{"apikey", "apikey", true},
		{"API_KEY uppercase", "API_KEY", true},
		{"user_api_key", "user_api_key", true},
		{"api_key_id", "api_key_id", true},

		// API secret patterns
		{"api_secret", "api_secret", true},
		{"api-secret", "api-secret", true},
		{"apisecret", "apisecret", true},
		{"API_SECRET uppercase", "API_SECRET", true},

		// Password patterns
		{"password", "password", true},
		{"passwd", "passwd", true},
		{"pwd", "pwd", true},
		{"user_password", "user_password", true},
		{"PASSWORD uppercase", "PASSWORD", true},

		// Secret key patterns
		{"secret_key", "secret_key", true},
		{"secret-key", "secret-key", true},
		{"secretkey", "secretkey", true},

		// Access token patterns
		{"access_token", "access_token", true},
		{"access-token", "access-token", true},
		{"accesstoken", "accesstoken", true},

		// Auth token patterns
		{"auth_token", "auth_token", true},
		{"auth-token", "auth-token", true},
		{"authtoken", "authtoken", true},

		// Private key patterns
		{"private_key", "private_key", true},
		{"private-key", "private-key", true},
		{"privatekey", "privatekey", true},

		// Credential patterns
		{"credential", "credential", true},
		{"credentials", "credentials", true},
		{"cred", "cred", true},
		{"user_credentials", "user_credentials", true},

		// Bearer token patterns
		{"bearer_token", "bearer_token", true},
		{"bearer-token", "bearer-token", true},

		// Client secret patterns
		{"client_secret", "client_secret", true},
		{"client-secret", "client-secret", true},

		// Non-sensitive columns
		{"username", "username", false},
		{"email", "email", false},
		{"created_at", "created_at", false},
		{"user_id", "user_id", false},
		{"status", "status", false},
		{"agent_data", "agent_data", false}, // Column name isn't sensitive, content is
		{"description", "description", false},
		{"account_id", "account_id", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.IsSensitiveColumn(tt.columnName)
			assert.Equal(t, tt.expected, result, "IsSensitiveColumn(%q) = %v, want %v",
				tt.columnName, result, tt.expected)
		})
	}
}

func TestSensitiveDetector_IsSensitiveColumn_CaseInsensitive(t *testing.T) {
	detector := NewSensitiveDetector()

	// All of these variations should be detected
	variations := []string{
		"api_key",
		"API_KEY",
		"Api_Key",
		"aPi_KeY",
		"PASSWORD",
		"Password",
		"pAsSWoRd",
	}

	for _, v := range variations {
		t.Run(v, func(t *testing.T) {
			assert.True(t, detector.IsSensitiveColumn(v),
				"IsSensitiveColumn(%q) should be true (case-insensitive)", v)
		})
	}
}

// ============================================================================
// Content Detection Tests
// ============================================================================

func TestSensitiveDetector_IsSensitiveContent(t *testing.T) {
	detector := NewSensitiveDetector()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "LiveKit example from issue",
			content:  `{"livekit_url":"wss://tikragents-xxx.livekit.cloud","livekit_api_key":"API67e2wiyw3KvB","livekit_api_secret":"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i","livekit_agent_id":"kitt"}`,
			expected: true,
		},
		{
			name:     "api_key in JSON",
			content:  `{"api_key": "sk-1234567890"}`,
			expected: true,
		},
		{
			name:     "api_secret in JSON",
			content:  `{"api_secret": "secret123"}`,
			expected: true,
		},
		{
			name:     "password in JSON",
			content:  `{"password": "mypassword"}`,
			expected: true,
		},
		{
			name:     "token in JSON",
			content:  `{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"}`,
			expected: true,
		},
		{
			name:     "secret in JSON",
			content:  `{"secret": "top-secret-value"}`,
			expected: true,
		},
		{
			name:     "credential in JSON",
			content:  `{"credential": "cred123"}`,
			expected: true,
		},
		{
			name:     "private_key in JSON",
			content:  `{"private_key": "-----BEGIN PRIVATE KEY-----"}`,
			expected: true,
		},
		{
			name:     "access_token in JSON",
			content:  `{"access_token": "access123"}`,
			expected: true,
		},
		{
			name:     "auth_token in JSON",
			content:  `{"auth_token": "auth123"}`,
			expected: true,
		},
		{
			name:     "bearer_token in JSON",
			content:  `{"bearer_token": "Bearer xyz"}`,
			expected: true,
		},
		{
			name:     "client_secret in JSON",
			content:  `{"client_secret": "client-secret-value"}`,
			expected: true,
		},
		{
			name:     "no spaces around colon",
			content:  `{"api_key":"no-spaces"}`,
			expected: true,
		},
		{
			name:     "multiple spaces around colon",
			content:  `{"api_key"  :  "many-spaces"}`,
			expected: true,
		},
		{
			name:     "non-sensitive JSON",
			content:  `{"name": "John", "email": "john@example.com"}`,
			expected: false,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "plain text",
			content:  "This is just plain text",
			expected: false,
		},
		{
			name:     "non-JSON with keyword",
			content:  "The password is stored securely",
			expected: false, // Only matches JSON key-value patterns
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.IsSensitiveContent(tt.content)
			assert.Equal(t, tt.expected, result,
				"IsSensitiveContent for %q = %v, want %v", tt.name, result, tt.expected)
		})
	}
}

func TestSensitiveDetector_IsSensitiveContent_CaseInsensitive(t *testing.T) {
	detector := NewSensitiveDetector()

	// All case variations should be detected
	variations := []string{
		`{"api_key": "value"}`,
		`{"API_KEY": "value"}`,
		`{"Api_Key": "value"}`,
		`{"PASSWORD": "value"}`,
		`{"Token": "value"}`,
	}

	for _, v := range variations {
		t.Run(v, func(t *testing.T) {
			assert.True(t, detector.IsSensitiveContent(v),
				"IsSensitiveContent should detect %q (case-insensitive)", v)
		})
	}
}

// ============================================================================
// Redaction Tests
// ============================================================================

func TestSensitiveDetector_RedactContent(t *testing.T) {
	detector := NewSensitiveDetector()

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
			name:     "non-sensitive content",
			input:    `{"name": "John", "email": "john@example.com"}`,
			expected: `{"name": "John", "email": "john@example.com"}`,
		},
		{
			name:     "single api_key",
			input:    `{"api_key": "sk-1234567890"}`,
			expected: `{"api_key":"[REDACTED]"}`,
		},
		{
			name:     "single password",
			input:    `{"password": "secret123"}`,
			expected: `{"password":"[REDACTED]"}`,
		},
		{
			name:     "multiple sensitive fields",
			input:    `{"api_key": "key123", "api_secret": "secret456"}`,
			expected: `{"api_key":"[REDACTED]", "api_secret":"[REDACTED]"}`,
		},
		{
			name:     "mixed sensitive and non-sensitive",
			input:    `{"username": "john", "password": "secret", "email": "john@example.com"}`,
			expected: `{"username": "john", "password":"[REDACTED]", "email": "john@example.com"}`,
		},
		{
			name:     "no spaces around colon",
			input:    `{"token":"abc123"}`,
			expected: `{"token":"[REDACTED]"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.RedactContent(tt.input)
			assert.Equal(t, tt.expected, result,
				"RedactContent(%q) = %q, want %q", tt.input, result, tt.expected)
		})
	}
}

func TestSensitiveDetector_RedactContent_LiveKitExample(t *testing.T) {
	detector := NewSensitiveDetector()

	// The exact example from the issue
	input := `{"livekit_url":"wss://tikragents-xxx.livekit.cloud","livekit_api_key":"API67e2wiyw3KvB","livekit_api_secret":"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i","livekit_agent_id":"kitt"}`

	result := detector.RedactContent(input)

	// Verify the result is valid JSON
	var parsed map[string]any
	err := json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err, "redacted content should be valid JSON")

	// Verify specific fields
	assert.Equal(t, "wss://tikragents-xxx.livekit.cloud", parsed["livekit_url"],
		"livekit_url should not be redacted")
	assert.Equal(t, "[REDACTED]", parsed["livekit_api_key"],
		"livekit_api_key should be redacted")
	assert.Equal(t, "[REDACTED]", parsed["livekit_api_secret"],
		"livekit_api_secret should be redacted")
	assert.Equal(t, "kitt", parsed["livekit_agent_id"],
		"livekit_agent_id should not be redacted")
}

func TestSensitiveDetector_RedactContent_PreservesJSONStructure(t *testing.T) {
	detector := NewSensitiveDetector()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple object",
			input: `{"api_key": "secret123", "name": "test"}`,
		},
		{
			name:  "nested object with secrets",
			input: `{"config": {"api_key": "secret", "timeout": 30}}`,
		},
		{
			name:  "array with objects",
			input: `[{"password": "pass1"}, {"password": "pass2"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.RedactContent(tt.input)

			// Verify result is valid JSON
			var parsed any
			err := json.Unmarshal([]byte(result), &parsed)
			assert.NoError(t, err, "redacted content should be valid JSON: %s", result)
		})
	}
}

// ============================================================================
// Custom Pattern Tests
// ============================================================================

func TestSensitiveDetectorWithPatterns_CustomColumnPatterns(t *testing.T) {
	// Custom pattern that only matches columns ending in "_secret"
	customPatterns := []*regexp.Regexp{
		regexp.MustCompile(`_secret$`),
	}

	detector := NewSensitiveDetectorWithPatterns(customPatterns, nil)

	// Should match custom pattern
	assert.True(t, detector.IsSensitiveColumn("my_secret"))
	assert.True(t, detector.IsSensitiveColumn("another_secret"))

	// Should NOT match default patterns (since we replaced them)
	assert.False(t, detector.IsSensitiveColumn("password"))
	assert.False(t, detector.IsSensitiveColumn("api_key"))
}

func TestSensitiveDetectorWithPatterns_CustomContentPatterns(t *testing.T) {
	// Custom pattern that only matches "custom_secret" key
	customPatterns := []*regexp.Regexp{
		regexp.MustCompile(`"custom_secret"\s*:\s*"[^"]*"`),
	}

	detector := NewSensitiveDetectorWithPatterns(nil, customPatterns)

	// Should match custom pattern
	assert.True(t, detector.IsSensitiveContent(`{"custom_secret": "value"}`))

	// Should NOT match default patterns
	assert.False(t, detector.IsSensitiveContent(`{"api_key": "value"}`))
}

func TestSensitiveDetectorWithPatterns_NilUsesDefaults(t *testing.T) {
	// Passing nil for both should use defaults
	detector := NewSensitiveDetectorWithPatterns(nil, nil)

	// Should use default patterns
	assert.True(t, detector.IsSensitiveColumn("password"))
	assert.True(t, detector.IsSensitiveContent(`{"api_key": "value"}`))
}

// ============================================================================
// Default Singleton Tests
// ============================================================================

func TestDefaultSensitiveDetector(t *testing.T) {
	// Verify the singleton works correctly
	assert.NotNil(t, DefaultSensitiveDetector)
	assert.True(t, DefaultSensitiveDetector.IsSensitiveColumn("password"))
	assert.True(t, DefaultSensitiveDetector.IsSensitiveContent(`{"token": "abc"}`))
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestSensitiveDetector_EdgeCases(t *testing.T) {
	detector := NewSensitiveDetector()

	t.Run("empty column name", func(t *testing.T) {
		assert.False(t, detector.IsSensitiveColumn(""))
	})

	t.Run("whitespace only column name", func(t *testing.T) {
		assert.False(t, detector.IsSensitiveColumn("   "))
	})

	t.Run("unicode in content", func(t *testing.T) {
		content := `{"api_key": "秘密鍵"}`
		result := detector.RedactContent(content)
		assert.Contains(t, result, "[REDACTED]")
	})

	t.Run("escaped quotes in value", func(t *testing.T) {
		// This is a tricky case - the regex may not handle escaped quotes perfectly
		// but it should at least not break
		content := `{"password": "value\"with\"quotes"}`
		// The result might not be perfect, but shouldn't panic
		assert.NotPanics(t, func() {
			_ = detector.RedactContent(content)
		})
		// At minimum it should detect the sensitive pattern
		assert.True(t, detector.IsSensitiveContent(content))
	})

	t.Run("very long value", func(t *testing.T) {
		longValue := make([]byte, 10000)
		for i := range longValue {
			longValue[i] = 'a'
		}
		content := `{"api_key": "` + string(longValue) + `"}`
		result := detector.RedactContent(content)
		assert.Contains(t, result, "[REDACTED]")
	})
}

// ============================================================================
// Comprehensive Pattern Coverage Tests
// ============================================================================

func TestSensitiveDetector_AllColumnPatternsCovered(t *testing.T) {
	detector := NewSensitiveDetector()

	// Every pattern from the plan should be detected
	mustDetect := []string{
		// api_key variations
		"api_key", "api-key", "apikey", "API_KEY",
		// api_secret variations
		"api_secret", "api-secret", "apisecret",
		// password variations
		"password", "passwd", "pwd",
		// secret_key variations
		"secret_key", "secret-key", "secretkey",
		// access_token variations
		"access_token", "access-token", "accesstoken",
		// auth_token variations
		"auth_token", "auth-token", "authtoken",
		// private_key variations
		"private_key", "private-key", "privatekey",
		// credential variations
		"credential", "cred",
		// bearer_token variations
		"bearer_token", "bearer-token",
		// client_secret variations
		"client_secret", "client-secret",
	}

	for _, col := range mustDetect {
		assert.True(t, detector.IsSensitiveColumn(col),
			"pattern should detect column %q", col)
	}
}

func TestSensitiveDetector_AllContentPatternsCovered(t *testing.T) {
	detector := NewSensitiveDetector()

	// Every JSON key pattern from the plan should be detected
	mustDetect := []string{
		`{"api_key": "x"}`,
		`{"api_secret": "x"}`,
		`{"password": "x"}`,
		`{"token": "x"}`,
		`{"secret": "x"}`,
		`{"credential": "x"}`,
		`{"private_key": "x"}`,
		`{"livekit_api_key": "x"}`,
		`{"livekit_api_secret": "x"}`,
		`{"access_token": "x"}`,
		`{"auth_token": "x"}`,
		`{"bearer_token": "x"}`,
		`{"client_secret": "x"}`,
	}

	for _, content := range mustDetect {
		assert.True(t, detector.IsSensitiveContent(content),
			"pattern should detect content %q", content)
	}
}

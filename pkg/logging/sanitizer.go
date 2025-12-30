package logging

import (
	"regexp"
)

const (
	// MaxQueryLogLength is the maximum length of a query to log
	MaxQueryLogLength = 100
	// RedactedText is the replacement text for sensitive data
	RedactedText = "[REDACTED]"
)

var (
	// Pattern to match potential passwords in connection strings
	// Matches: password=xxx, pwd=xxx, pass=xxx (until next delimiter)
	passwordPattern = regexp.MustCompile(`(?i)(password|pwd|pass)=[^;&\s]+`)

	// Pattern to match JWT tokens (three base64 segments separated by dots)
	jwtPattern = regexp.MustCompile(`Bearer\s+[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]*`)

	// Pattern to match potential API keys
	apiKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|apikey|key)=[A-Za-z0-9-_]{20,}`)

	// Pattern to match connection string credentials (user:pass@host format)
	connStringPattern = regexp.MustCompile(`://[^:]+:[^@]+@[^/\s]+`)
)

// SanitizeConnectionString removes sensitive data from connection strings
// Use this before logging any connection string
func SanitizeConnectionString(connStr string) string {
	if connStr == "" {
		return ""
	}

	// Replace password values
	sanitized := passwordPattern.ReplaceAllString(connStr, "${1}="+RedactedText)

	// Replace user:pass@host format
	sanitized = connStringPattern.ReplaceAllString(sanitized, "://"+RedactedText+"@"+RedactedText)

	return sanitized
}

// SanitizeError sanitizes error messages that might contain sensitive data
// Use this before logging any error from database operations
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Remove potential passwords
	sanitized := passwordPattern.ReplaceAllString(errStr, "${1}="+RedactedText)

	// Remove JWT tokens
	sanitized = jwtPattern.ReplaceAllString(sanitized, "Bearer "+RedactedText)

	// Remove API keys
	sanitized = apiKeyPattern.ReplaceAllString(sanitized, "${1}="+RedactedText)

	// Remove connection string details
	sanitized = connStringPattern.ReplaceAllString(sanitized, "://"+RedactedText+"@"+RedactedText)

	return sanitized
}

// SanitizeQuery truncates and sanitizes a SQL query for logging
// Prevents logging very long queries and removes sensitive patterns
func SanitizeQuery(query string) string {
	if query == "" {
		return ""
	}

	// Truncate if too long
	sanitized := query
	if len(sanitized) > MaxQueryLogLength {
		sanitized = sanitized[:MaxQueryLogLength] + "..."
	}

	// Remove potential sensitive data patterns
	sanitized = passwordPattern.ReplaceAllString(sanitized, "${1}="+RedactedText)
	sanitized = apiKeyPattern.ReplaceAllString(sanitized, "${1}="+RedactedText)

	return sanitized
}

// TruncateString truncates a string to maxLen and adds ellipsis if needed
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

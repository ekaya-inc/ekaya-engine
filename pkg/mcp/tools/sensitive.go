package tools

import (
	"regexp"
	"strings"
)

// SensitiveDetector identifies and redacts sensitive data in column names and content.
// It uses configurable regex patterns to detect secrets like API keys, passwords, and tokens.
type SensitiveDetector struct {
	columnPatterns  []*regexp.Regexp // patterns for column names
	contentPatterns []*regexp.Regexp // patterns for JSON keys in content
}

// defaultColumnPatterns returns regex patterns to detect sensitive column names.
// All patterns are case-insensitive.
func defaultColumnPatterns() []*regexp.Regexp {
	patterns := []string{
		`(?i)(api[_-]?key|apikey)`,
		`(?i)(api[_-]?secret|apisecret)`,
		`(?i)(password|passwd|pwd)`,
		`(?i)(secret[_-]?key|secretkey)`,
		`(?i)(access[_-]?token|accesstoken)`,
		`(?i)(auth[_-]?token|authtoken)`,
		`(?i)(private[_-]?key|privatekey)`,
		`(?i)(credential|cred)`,
		`(?i)(bearer[_-]?token)`,
		`(?i)(client[_-]?secret)`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}

// defaultContentPatterns returns regex patterns to detect sensitive keys in JSON content.
// These patterns match JSON key-value pairs where the key indicates sensitive data.
func defaultContentPatterns() []*regexp.Regexp {
	// Pattern matches: "sensitive_key": "value" or "sensitive_key":"value"
	// Captures the key name in group 1 for use in redaction
	patterns := []string{
		`"(api_key|api_secret|password|token|secret|credential|private_key|livekit_api_key|livekit_api_secret|access_token|auth_token|bearer_token|client_secret)"\s*:\s*"[^"]*"`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(`(?i)`+p))
	}
	return compiled
}

// NewSensitiveDetector creates a new detector with default patterns.
func NewSensitiveDetector() *SensitiveDetector {
	return &SensitiveDetector{
		columnPatterns:  defaultColumnPatterns(),
		contentPatterns: defaultContentPatterns(),
	}
}

// NewSensitiveDetectorWithPatterns creates a detector with custom patterns.
// If columnPatterns or contentPatterns is nil, defaults are used.
func NewSensitiveDetectorWithPatterns(columnPatterns, contentPatterns []*regexp.Regexp) *SensitiveDetector {
	d := &SensitiveDetector{}

	if columnPatterns != nil {
		d.columnPatterns = columnPatterns
	} else {
		d.columnPatterns = defaultColumnPatterns()
	}

	if contentPatterns != nil {
		d.contentPatterns = contentPatterns
	} else {
		d.contentPatterns = defaultContentPatterns()
	}

	return d
}

// IsSensitiveColumn checks if a column name matches any sensitive pattern.
func (d *SensitiveDetector) IsSensitiveColumn(columnName string) bool {
	for _, pattern := range d.columnPatterns {
		if pattern.MatchString(columnName) {
			return true
		}
	}
	return false
}

// IsSensitiveContent checks if content contains any sensitive patterns.
// This is useful for detecting secrets embedded in JSON or other structured content.
func (d *SensitiveDetector) IsSensitiveContent(content string) bool {
	for _, pattern := range d.contentPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

// RedactContent replaces sensitive values in content with [REDACTED].
// Preserves JSON structure by only replacing the value portion.
func (d *SensitiveDetector) RedactContent(content string) string {
	if content == "" {
		return content
	}

	result := content

	// Pattern to match sensitive JSON key-value pairs and capture the key
	// This allows us to replace just the value while keeping the key
	sensitiveKeyPattern := regexp.MustCompile(`(?i)"(api_key|api_secret|password|token|secret|credential|private_key|livekit_api_key|livekit_api_secret|access_token|auth_token|bearer_token|client_secret)"\s*:\s*"[^"]*"`)

	result = sensitiveKeyPattern.ReplaceAllStringFunc(result, func(match string) string {
		// Find the colon and replace everything after it with "[REDACTED]"
		colonIdx := strings.Index(match, ":")
		if colonIdx == -1 {
			return match
		}
		// Keep the key and colon, replace the value
		keyPart := match[:colonIdx+1]
		return keyPart + `"[REDACTED]"`
	})

	return result
}

// DefaultSensitiveDetector is a singleton detector instance with default patterns.
// Use this for common cases where custom patterns are not needed.
var DefaultSensitiveDetector = NewSensitiveDetector()

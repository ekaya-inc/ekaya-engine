package auth

import (
	"testing"
)

func TestDeriveCookieSettings_Localhost(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected CookieSettings
	}{
		{
			name:    "localhost with port",
			baseURL: "http://localhost:3443",
			expected: CookieSettings{
				Secure: false,
				Domain: "",
			},
		},
		{
			name:    "localhost without port",
			baseURL: "http://localhost",
			expected: CookieSettings{
				Secure: false,
				Domain: "",
			},
		},
		{
			name:    "127.0.0.1",
			baseURL: "http://127.0.0.1:3443",
			expected: CookieSettings{
				Secure: false,
				Domain: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveCookieSettings(tt.baseURL, "")
			if result.Secure != tt.expected.Secure {
				t.Errorf("Secure: expected %v, got %v", tt.expected.Secure, result.Secure)
			}
			if result.Domain != tt.expected.Domain {
				t.Errorf("Domain: expected %q, got %q", tt.expected.Domain, result.Domain)
			}
		})
	}
}

func TestDeriveCookieSettings_DevEkaya(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected CookieSettings
	}{
		{
			name:    "us.dev.ekaya.ai",
			baseURL: "https://us.dev.ekaya.ai",
			expected: CookieSettings{
				Secure: true,
				Domain: ".dev.ekaya.ai",
			},
		},
		{
			name:    "eu.dev.ekaya.ai",
			baseURL: "https://eu.dev.ekaya.ai",
			expected: CookieSettings{
				Secure: true,
				Domain: ".dev.ekaya.ai",
			},
		},
		{
			name:    "us-central1.dev.ekaya.ai",
			baseURL: "https://us-central1.dev.ekaya.ai",
			expected: CookieSettings{
				Secure: true,
				Domain: ".dev.ekaya.ai",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveCookieSettings(tt.baseURL, "")
			if result.Secure != tt.expected.Secure {
				t.Errorf("Secure: expected %v, got %v", tt.expected.Secure, result.Secure)
			}
			if result.Domain != tt.expected.Domain {
				t.Errorf("Domain: expected %q, got %q", tt.expected.Domain, result.Domain)
			}
		})
	}
}

func TestDeriveCookieSettings_ProdEkaya(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected CookieSettings
	}{
		{
			name:    "us.ekaya.ai",
			baseURL: "https://us.ekaya.ai",
			expected: CookieSettings{
				Secure: true,
				Domain: ".ekaya.ai",
			},
		},
		{
			name:    "eu.ekaya.ai",
			baseURL: "https://eu.ekaya.ai",
			expected: CookieSettings{
				Secure: true,
				Domain: ".ekaya.ai",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveCookieSettings(tt.baseURL, "")
			if result.Secure != tt.expected.Secure {
				t.Errorf("Secure: expected %v, got %v", tt.expected.Secure, result.Secure)
			}
			if result.Domain != tt.expected.Domain {
				t.Errorf("Domain: expected %q, got %q", tt.expected.Domain, result.Domain)
			}
		})
	}
}

func TestDeriveCookieSettings_Internal(t *testing.T) {
	result := DeriveCookieSettings("https://ekaya.internal", "")
	if !result.Secure {
		t.Error("expected Secure to be true for .internal domain")
	}
	if result.Domain != ".internal" {
		t.Errorf("expected Domain '.internal', got %q", result.Domain)
	}
}

func TestDeriveCookieSettings_CustomDomain(t *testing.T) {
	// Unknown custom domain should default to isolated (empty domain)
	result := DeriveCookieSettings("https://ekaya.acme.com", "")
	if !result.Secure {
		t.Error("expected Secure to be true for HTTPS")
	}
	if result.Domain != "" {
		t.Errorf("expected empty Domain for unknown domain, got %q", result.Domain)
	}
}

func TestDeriveCookieSettings_ExplicitOverride(t *testing.T) {
	// Explicit cookie_domain in config should override auto-detection
	result := DeriveCookieSettings("https://us.ekaya.ai", ".custom.domain")
	if !result.Secure {
		t.Error("expected Secure to be true for HTTPS")
	}
	if result.Domain != ".custom.domain" {
		t.Errorf("expected Domain '.custom.domain', got %q", result.Domain)
	}
}

func TestDeriveCookieSettings_InvalidURL(t *testing.T) {
	// Invalid URL should return safe defaults
	result := DeriveCookieSettings("not-a-valid-url", "")
	if !result.Secure {
		t.Error("expected Secure to be true for invalid URL (safe default)")
	}
}

func TestDeriveCookieSettings_EmptyURL(t *testing.T) {
	// Empty URL should return safe defaults
	result := DeriveCookieSettings("", "")
	if !result.Secure {
		t.Error("expected Secure to be true for empty URL (safe default)")
	}
}

func TestIsHTTPS(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com", true},
		{"http://example.com", false},
		{"https://localhost:3443", true},
		{"http://localhost:3443", false},
		{"", true},                  // empty defaults to true (safe)
		{"not-a-url", true},         // invalid defaults to true (safe)
		{"ftp://example.com", true}, // non-http treated as secure
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := isHTTPS(tt.url)
			if result != tt.expected {
				t.Errorf("isHTTPS(%q): expected %v, got %v", tt.url, tt.expected, result)
			}
		})
	}
}

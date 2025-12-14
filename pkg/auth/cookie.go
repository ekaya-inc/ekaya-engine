package auth

import (
	"net/url"
	"strings"
)

// CookieSettings contains cookie security settings derived from base URL.
type CookieSettings struct {
	// Secure indicates whether the cookie should only be sent over HTTPS.
	Secure bool
	// Domain is the cookie domain scope (e.g., ".ekaya.ai" for cross-subdomain sharing).
	Domain string
}

// DeriveCookieSettings automatically determines cookie security settings from base URL.
// This supports multiple hosting scenarios:
//   - Self-hosted localhost (http://localhost:3443) → Secure: false, Domain: ""
//   - Ekaya dev Cloud Run (https://us.dev.ekaya.ai) → Secure: true, Domain: ".dev.ekaya.ai"
//   - Ekaya prod Cloud Run (https://us.ekaya.ai) → Secure: true, Domain: ".ekaya.ai"
//   - Enterprise internal (https://ekaya.internal) → Secure: true, Domain: ".internal"
//
// The configCookieDomain parameter allows explicit override if needed.
func DeriveCookieSettings(baseURL string, configCookieDomain string) CookieSettings {
	// If cookie_domain explicitly set in config, use it with scheme-based Secure
	if configCookieDomain != "" {
		return CookieSettings{
			Secure: isHTTPS(baseURL),
			Domain: configCookieDomain,
		}
	}

	// Auto-derive both Secure and Domain from base_url
	parsedURL, err := url.Parse(baseURL)
	if err != nil || baseURL == "" {
		// Safe defaults for invalid URLs
		return CookieSettings{Secure: true, Domain: ""}
	}

	secure := parsedURL.Scheme != "http"
	hostname := parsedURL.Hostname()

	var domain string
	switch {
	case hostname == "localhost" || hostname == "127.0.0.1":
		// Localhost: no domain restriction, allow HTTP
		domain = ""
	case strings.HasSuffix(hostname, ".dev.ekaya.ai"):
		// Ekaya dev environment: share across dev subdomains
		domain = ".dev.ekaya.ai"
	case strings.HasSuffix(hostname, ".ekaya.ai"):
		// Ekaya production: share across prod subdomains
		domain = ".ekaya.ai"
	case strings.HasSuffix(hostname, ".internal"):
		// Enterprise internal network: share across internal subdomains
		domain = ".internal"
	default:
		// Unknown domain (enterprise custom, etc): isolate to specific hostname
		domain = ""
	}

	return CookieSettings{
		Secure: secure,
		Domain: domain,
	}
}

// isHTTPS determines if the given base URL uses HTTPS protocol.
// Returns true for HTTPS, false for HTTP, true for empty/invalid URLs (safe default).
func isHTTPS(baseURL string) bool {
	if baseURL == "" {
		return true
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return true
	}

	return parsedURL.Scheme != "http"
}

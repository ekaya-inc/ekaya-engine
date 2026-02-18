package config

import (
	"testing"
)

func TestResolveHostForDocker_NotInDocker(t *testing.T) {
	// When not in Docker, host should be returned unchanged
	// Note: This test assumes we're not running in Docker
	// The actual IsRunningInDocker() result depends on the test environment

	tests := []struct {
		input    string
		expected string
	}{
		{"mydb.example.com", "mydb.example.com"},
		{"192.168.1.100", "192.168.1.100"},
		{"host.docker.internal", "host.docker.internal"},
	}

	for _, tt := range tests {
		result := ResolveHostForDocker(tt.input)
		// These hosts should never be modified regardless of Docker status
		if result != tt.expected {
			t.Errorf("ResolveHostForDocker(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestResolveURLForDocker(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"non-localhost URL", "http://api.openai.com/v1", "http://api.openai.com/v1"},
		{"localhost with port", "http://localhost:11434/v1", "http://localhost:11434/v1"},
		{"127.0.0.1 with port", "http://127.0.0.1:8080/v1", "http://127.0.0.1:8080/v1"},
		{"localhost no port", "http://localhost/v1", "http://localhost/v1"},
		{"https localhost", "https://localhost:11434/v1", "https://localhost:11434/v1"},
		{"invalid URL", "://bad", "://bad"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveURLForDocker(tt.input)
			if IsRunningInDocker() {
				// In Docker, localhost URLs should be rewritten
				switch tt.name {
				case "localhost with port":
					if result != "http://host.docker.internal:11434/v1" {
						t.Errorf("ResolveURLForDocker(%q) in Docker = %q, want %q", tt.input, result, "http://host.docker.internal:11434/v1")
					}
				case "127.0.0.1 with port":
					if result != "http://host.docker.internal:8080/v1" {
						t.Errorf("ResolveURLForDocker(%q) in Docker = %q, want %q", tt.input, result, "http://host.docker.internal:8080/v1")
					}
				case "localhost no port":
					if result != "http://host.docker.internal/v1" {
						t.Errorf("ResolveURLForDocker(%q) in Docker = %q, want %q", tt.input, result, "http://host.docker.internal/v1")
					}
				case "https localhost":
					if result != "https://host.docker.internal:11434/v1" {
						t.Errorf("ResolveURLForDocker(%q) in Docker = %q, want %q", tt.input, result, "https://host.docker.internal:11434/v1")
					}
				default:
					if result != tt.expected {
						t.Errorf("ResolveURLForDocker(%q) in Docker = %q, want %q", tt.input, result, tt.expected)
					}
				}
			} else {
				// Not in Docker, all URLs should be unchanged
				if result != tt.input {
					t.Errorf("ResolveURLForDocker(%q) not in Docker = %q, want unchanged", tt.input, result)
				}
			}
		})
	}
}

func TestResolveHostForDocker_LocalhostVariants(t *testing.T) {
	// Test that localhost variants are recognized
	// The actual replacement only happens when IsRunningInDocker() returns true

	localhostVariants := []string{"localhost", "127.0.0.1"}

	for _, host := range localhostVariants {
		result := ResolveHostForDocker(host)
		// If we're in Docker, should be host.docker.internal
		// If we're not in Docker, should be unchanged
		if IsRunningInDocker() {
			if result != "host.docker.internal" {
				t.Errorf("ResolveHostForDocker(%q) in Docker = %q, want %q", host, result, "host.docker.internal")
			}
		} else {
			if result != host {
				t.Errorf("ResolveHostForDocker(%q) not in Docker = %q, want %q", host, result, host)
			}
		}
	}
}

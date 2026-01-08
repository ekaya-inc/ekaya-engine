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

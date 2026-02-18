package config

import (
	"net/url"
	"os"
	"sync"
)

var (
	isDockerOnce   sync.Once
	isDockerResult bool
)

// IsRunningInDocker returns true if the application is running inside a Docker container.
// Detection is based on the presence of /.dockerenv file which exists in all Docker containers.
// The result is cached after the first call.
func IsRunningInDocker() bool {
	isDockerOnce.Do(func() {
		_, err := os.Stat("/.dockerenv")
		isDockerResult = err == nil
	})
	return isDockerResult
}

// ResolveHostForDocker returns the appropriate host address for connecting to external services.
// If running in Docker and the host is "localhost" or "127.0.0.1", it returns "host.docker.internal"
// to allow connections to services running on the host machine.
// Otherwise, returns the original host unchanged.
func ResolveHostForDocker(host string) string {
	if !IsRunningInDocker() {
		return host
	}

	if host == "localhost" || host == "127.0.0.1" {
		return "host.docker.internal"
	}

	return host
}

// ResolveURLForDocker resolves localhost in a full URL for Docker environments.
// If running in Docker and the URL's host is "localhost" or "127.0.0.1",
// it replaces the host with "host.docker.internal".
// Returns the original URL unchanged if not in Docker, not a localhost URL, or if parsing fails.
func ResolveURLForDocker(rawURL string) string {
	if rawURL == "" || !IsRunningInDocker() {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	hostname := parsed.Hostname()
	if hostname != "localhost" && hostname != "127.0.0.1" {
		return rawURL
	}

	port := parsed.Port()
	if port != "" {
		parsed.Host = "host.docker.internal:" + port
	} else {
		parsed.Host = "host.docker.internal"
	}

	return parsed.String()
}

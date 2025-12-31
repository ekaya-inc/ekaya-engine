//go:build !debug

package llm

// debugWriteRequest is a no-op in non-debug builds.
func debugWriteRequest(systemMessage, prompt string, model string) string {
	return ""
}

// debugWriteResponse is a no-op in non-debug builds.
func debugWriteResponse(prefix string, response string, durationMs int64) {
}

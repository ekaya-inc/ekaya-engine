//go:build !debug

package llm

import "github.com/google/uuid"

// debugWriteRequest is a no-op in non-debug builds.
func debugWriteRequest(conversationID uuid.UUID, model, systemMessage, prompt string) string {
	return ""
}

// debugWriteResponse is a no-op in non-debug builds.
func debugWriteResponse(prefix, conversationID, model, response string, durationMs int64) {
}

// debugWriteError is a no-op in non-debug builds.
func debugWriteError(prefix, conversationID, model, errorMessage string, durationMs int64) {
}

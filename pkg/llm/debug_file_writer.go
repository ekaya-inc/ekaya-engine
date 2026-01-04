//go:build debug

package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var debugDir = filepath.Join(os.TempDir(), "ekaya-engine-llm-conversations")

func init() {
	// Ensure directory exists on startup
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to create LLM debug directory %s: %v\n", debugDir, err)
	} else {
		fmt.Fprintf(os.Stderr, "DEBUG: LLM conversations will be written to %s\n", debugDir)
	}
}

// debugWriteRequest writes the request (system message + prompt) before the LLM call.
// Returns the timestamp prefix for matching with the response/error files.
func debugWriteRequest(conversationID uuid.UUID, model, systemMessage, prompt string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05.000")
	prefix := fmt.Sprintf("%s_%s", timestamp, conversationID.String())
	filename := fmt.Sprintf("%s_request.txt", prefix)
	fpath := filepath.Join(debugDir, filename)

	content := fmt.Sprintf(`================================================================================
TIMESTAMP: %s
MODEL: %s
CONVERSATION_ID: %s
TYPE: REQUEST
================================================================================

=== SYSTEM MESSAGE ===
%s

=== PROMPT ===
%s
`,
		time.Now().Format(time.RFC3339),
		model,
		conversationID.String(),
		systemMessage,
		prompt,
	)

	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to write LLM request file %s: %v\n", fpath, err)
	}

	return prefix
}

// debugWriteResponse writes the response after the LLM call completes successfully.
func debugWriteResponse(prefix, conversationID, model, response string, durationMs int64) {
	filename := fmt.Sprintf("%s_response.txt", prefix)
	fpath := filepath.Join(debugDir, filename)

	content := fmt.Sprintf(`================================================================================
TIMESTAMP: %s
MODEL: %s
CONVERSATION_ID: %s
TYPE: RESPONSE
DURATION: %dms
================================================================================

%s
`,
		time.Now().Format(time.RFC3339),
		model,
		conversationID,
		durationMs,
		response,
	)

	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to write LLM response file %s: %v\n", fpath, err)
	}
}

// debugWriteError writes an error file when the LLM call fails.
func debugWriteError(prefix, conversationID, model, errorMessage string, durationMs int64) {
	filename := fmt.Sprintf("%s_error.txt", prefix)
	fpath := filepath.Join(debugDir, filename)

	content := fmt.Sprintf(`================================================================================
TIMESTAMP: %s
MODEL: %s
CONVERSATION_ID: %s
TYPE: ERROR
DURATION: %dms
================================================================================

%s
`,
		time.Now().Format(time.RFC3339),
		model,
		conversationID,
		durationMs,
		errorMessage,
	)

	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to write LLM error file %s: %v\n", fpath, err)
	}
}

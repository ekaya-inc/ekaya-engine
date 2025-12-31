//go:build debug

package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
// Returns the timestamp prefix for matching with the response file.
func debugWriteRequest(systemMessage, prompt string, model string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05.000")
	safeModel := strings.ReplaceAll(model, "/", "_")
	prefix := fmt.Sprintf("%s_%s", timestamp, safeModel)
	filename := fmt.Sprintf("%s_request.txt", prefix)
	fpath := filepath.Join(debugDir, filename)

	content := fmt.Sprintf(`================================================================================
TIMESTAMP: %s
MODEL: %s
TYPE: REQUEST
================================================================================

=== SYSTEM MESSAGE ===
%s

=== PROMPT ===
%s
`,
		time.Now().Format(time.RFC3339),
		model,
		systemMessage,
		prompt,
	)

	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to write LLM request file %s: %v\n", fpath, err)
	}

	return prefix
}

// debugWriteResponse writes the response after the LLM call completes.
func debugWriteResponse(prefix string, response string, durationMs int64) {
	filename := fmt.Sprintf("%s_response.txt", prefix)
	fpath := filepath.Join(debugDir, filename)

	content := fmt.Sprintf(`================================================================================
TIMESTAMP: %s
DURATION: %dms
TYPE: RESPONSE
================================================================================

%s
`,
		time.Now().Format(time.RFC3339),
		durationMs,
		response,
	)

	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to write LLM response file %s: %v\n", fpath, err)
	}
}

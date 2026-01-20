package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// MCPRequestLogger returns middleware that logs MCP JSON-RPC requests/responses.
// It intercepts request/response bodies to extract tool names, parameters, and error details.
// Pass nil logger to disable logging.
func MCPRequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// If no logger provided, pass through without logging
		if logger == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read and restore request body for JSON-RPC parsing
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				logger.Error("Failed to read MCP request body", zap.Error(err))
				next.ServeHTTP(w, r)
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// Parse JSON-RPC request to extract tool name and params
			var rpcReq jsonRPCRequest
			if err := json.Unmarshal(bodyBytes, &rpcReq); err != nil {
				logger.Debug("Failed to parse MCP request JSON", zap.Error(err))
				// Continue anyway - not all requests may be valid JSON
			}

			// Extract tool name and sanitized arguments
			toolName := rpcReq.Params.Name
			sanitizedArgs := sanitizeArguments(rpcReq.Params.Arguments)

			// Log the incoming request
			logger.Debug("MCP request",
				zap.String("method", rpcReq.Method),
				zap.String("tool", toolName),
				zap.Any("arguments", sanitizedArgs),
			)

			// Capture response body using a recorder
			recorder := &mcpResponseRecorder{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
			}
			start := time.Now()

			// Process the request
			next.ServeHTTP(recorder, r)

			duration := time.Since(start)

			// Parse JSON-RPC response to check for errors
			var rpcResp jsonRPCResponse
			if err := json.Unmarshal(recorder.body.Bytes(), &rpcResp); err != nil {
				logger.Debug("Failed to parse MCP response JSON", zap.Error(err))
				return
			}

			// Log based on success/failure
			if rpcResp.Error != nil {
				logger.Debug("MCP response error",
					zap.String("tool", toolName),
					zap.Int("error_code", rpcResp.Error.Code),
					zap.String("error_message", rpcResp.Error.Message),
					zap.Duration("duration", duration),
				)
			} else {
				logger.Debug("MCP response success",
					zap.String("tool", toolName),
					zap.Duration("duration", duration),
				)
			}
		})
	}
}

// jsonRPCRequest represents the structure of a JSON-RPC request for tools/call.
type jsonRPCRequest struct {
	Method string `json:"method"`
	Params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"params"`
}

// jsonRPCResponse represents the structure of a JSON-RPC response.
type jsonRPCResponse struct {
	Result interface{}    `json:"result"`
	Error  *jsonRPCError  `json:"error"`
}

// jsonRPCError represents an error in a JSON-RPC response.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpResponseRecorder is a response writer that captures the response body.
type mcpResponseRecorder struct {
	http.ResponseWriter
	body *bytes.Buffer
}

// Write captures the response body and writes it to the underlying writer.
func (r *mcpResponseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// sanitizeArguments redacts sensitive fields and truncates long values.
func sanitizeArguments(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}

	sensitiveKeywords := []string{"password", "secret", "token", "key", "credential"}
	result := make(map[string]interface{})

	for k, v := range args {
		// Check if key contains sensitive keywords
		lowerKey := strings.ToLower(k)
		isSensitive := false
		for _, keyword := range sensitiveKeywords {
			if strings.Contains(lowerKey, keyword) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			result[k] = "[REDACTED]"
			continue
		}

		// Truncate long strings to prevent log bloat
		if str, ok := v.(string); ok && len(str) > 200 {
			result[k] = str[:200] + "..."
		} else {
			result[k] = v
		}
	}

	return result
}

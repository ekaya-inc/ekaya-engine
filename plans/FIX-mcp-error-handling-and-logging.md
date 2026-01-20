# FIX: MCP Error Handling and Logging

## Context

When debugging MCP integrations, two issues make troubleshooting difficult:

1. **Claude Desktop eats error messages** - Server returns detailed errors, but Claude only sees "Tool execution failed"
2. **Server logs don't show MCP details** - Only HTTP status 200 logged, not the JSON-RPC error inside

This document addresses both issues.

---

## Issue 3: MCP Error Messages Not Reaching Client

### Problem

Our server correctly returns JSON-RPC errors:

```json
{"jsonrpc":"2.0","id":36,"error":{"code":-32603,"message":"failed to get ontology at depth 'columns': failed to get columns context: table names required for columns depth"}}
```

But Claude Desktop logs show this was sent, yet Claude Chat reports only:

```
<error>Tool execution failed</error>
```

The detailed error message is lost, preventing Claude from understanding what went wrong and adjusting its parameters.

### Investigation

**Hypothesis 1:** Claude Desktop has error message length limits or sanitization

The MCP protocol defines error responses as:
```json
{
  "jsonrpc": "2.0",
  "id": 36,
  "error": {
    "code": -32603,  // Internal error
    "message": "...",
    "data": {}  // Optional additional data
  }
}
```

Claude Desktop may be:
- Truncating long error messages
- Only showing errors to user, not passing to Claude
- Treating certain error codes differently

**Hypothesis 2:** The mcp-go library may be doing something unexpected with error returns

When we return `return nil, fmt.Errorf(...)` from a tool handler, mcp-go converts this to a JSON-RPC error. Let's verify the conversion is correct.

### Proposed Solution: Return Errors as Successful Results

Instead of returning Go errors (which become JSON-RPC errors), return a "success" response with error information in the content. This ensures Claude sees the error details.

**Current Pattern:**
```go
func (ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // ...
    if ontology == nil {
        return nil, fmt.Errorf("no active ontology found for project")
    }
    // ...
}
```

**Proposed Pattern:**
```go
func (ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // ...
    if ontology == nil {
        return errorResult("no active ontology found for project", "ontology_not_found"), nil
    }
    // ...
}

// Helper function for structured error responses
func errorResult(message, code string) *mcp.CallToolResult {
    response := map[string]any{
        "error":   true,
        "code":    code,
        "message": message,
    }
    jsonBytes, _ := json.Marshal(response)
    return mcp.NewToolResultText(string(jsonBytes))
}
```

**Claude receives:**
```json
{
  "error": true,
  "code": "ontology_not_found",
  "message": "no active ontology found for project"
}
```

This approach:
- Ensures error details reach Claude
- Allows Claude to understand and adjust its approach
- Distinguishes between "tool worked but found an error condition" vs "tool crashed"

### When to Use Which Pattern

| Situation | Pattern |
|-----------|---------|
| Invalid parameters | Return error result (Claude can fix) |
| Resource not found | Return error result (Claude should know) |
| Business logic failure | Return error result (actionable) |
| Internal server error | Return Go error (truly unexpected) |
| Database connection failed | Return Go error (system issue) |
| Authentication failed | Return Go error (security boundary) |

### Implementation

Create helper in `pkg/mcp/tools/errors.go`:

```go
package tools

import (
    "encoding/json"
    "github.com/mark3labs/mcp-go/mcp"
)

// ErrorResponse represents a structured error in tool results.
type ErrorResponse struct {
    Error   bool   `json:"error"`
    Code    string `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
}

// NewErrorResult creates a tool result containing a structured error.
// Use this for recoverable/actionable errors that Claude should see.
func NewErrorResult(code, message string) *mcp.CallToolResult {
    resp := ErrorResponse{
        Error:   true,
        Code:    code,
        Message: message,
    }
    jsonBytes, _ := json.Marshal(resp)
    return mcp.NewToolResultText(string(jsonBytes))
}

// NewErrorResultWithDetails creates an error result with additional context.
func NewErrorResultWithDetails(code, message string, details any) *mcp.CallToolResult {
    resp := ErrorResponse{
        Error:   true,
        Code:    code,
        Message: message,
        Details: details,
    }
    jsonBytes, _ := json.Marshal(resp)
    return mcp.NewToolResultText(string(jsonBytes))
}
```

Then update tools like `get_ontology`:

```go
// Before
if ontology == nil {
    return nil, fmt.Errorf("no active ontology found for project")
}

// After
if ontology == nil {
    return NewErrorResult("ontology_not_found", "no active ontology found for project"), nil
}
```

### Migration Strategy

1. Add the error helper functions
2. Update high-frequency tools first (get_ontology, update_entity, etc.)
3. Keep Go errors for true system failures
4. Monitor Claude Desktop logs to confirm error visibility

---

## Issue 4: MCP Request/Response Logging

### Problem

Current server logs only show:

```
2026-01-20T13:03:04.046+0200 DEBUG middleware/logging.go:27 HTTP request {"method": "POST", "path": "/mcp/697ebf65-7992-4ebd-b741-1d7e1b2b6c02", "status": 200, "duration": "6.684041ms", "remote_addr": "127.0.0.1:51783"}
```

This doesn't help debug MCP issues because:
- JSON-RPC errors return HTTP 200 (correct per spec)
- Can't see which tool was called
- Can't see request parameters
- Can't see response content
- Must rely on MCP client logs (Claude Desktop) which may be hard to access

### Desired Logging

```
2026-01-20T13:03:04.046+0200 DEBUG mcp/handler.go:XX MCP request {
    "method": "tools/call",
    "tool": "get_ontology",
    "params": {"depth": "columns"},
    "project_id": "697ebf65-..."
}
2026-01-20T13:03:04.050+0200 DEBUG mcp/handler.go:XX MCP response {
    "tool": "get_ontology",
    "success": false,
    "error_code": -32603,
    "error_message": "failed to get ontology at depth 'columns': ...",
    "duration_ms": 4
}
```

### Proposed Solution: MCP Logging Middleware

The mcp-go library's `StreamableHTTPServer` handles the JSON-RPC parsing internally. We need to intercept before and after.

**Option A: Wrap the HTTP handler**

Create middleware that logs request/response bodies:

```go
// pkg/middleware/mcp_logging.go
func MCPRequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Read and restore request body
            bodyBytes, _ := io.ReadAll(r.Body)
            r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

            // Parse JSON-RPC request
            var rpcReq struct {
                Method string `json:"method"`
                Params struct {
                    Name      string         `json:"name"`
                    Arguments map[string]any `json:"arguments"`
                } `json:"params"`
            }
            json.Unmarshal(bodyBytes, &rpcReq)

            // Log request
            logger.Debug("MCP request",
                zap.String("method", rpcReq.Method),
                zap.String("tool", rpcReq.Params.Name),
                zap.Any("arguments", rpcReq.Params.Arguments),
            )

            // Capture response
            recorder := &responseRecorder{ResponseWriter: w, body: &bytes.Buffer{}}
            start := time.Now()

            next.ServeHTTP(recorder, r)

            // Parse JSON-RPC response
            var rpcResp struct {
                Result any `json:"result"`
                Error  *struct {
                    Code    int    `json:"code"`
                    Message string `json:"message"`
                } `json:"error"`
            }
            json.Unmarshal(recorder.body.Bytes(), &rpcResp)

            // Log response
            if rpcResp.Error != nil {
                logger.Debug("MCP response error",
                    zap.String("tool", rpcReq.Params.Name),
                    zap.Int("error_code", rpcResp.Error.Code),
                    zap.String("error_message", rpcResp.Error.Message),
                    zap.Duration("duration", time.Since(start)),
                )
            } else {
                logger.Debug("MCP response success",
                    zap.String("tool", rpcReq.Params.Name),
                    zap.Duration("duration", time.Since(start)),
                )
            }
        })
    }
}

type responseRecorder struct {
    http.ResponseWriter
    body       *bytes.Buffer
    statusCode int
}

func (r *responseRecorder) Write(b []byte) (int, error) {
    r.body.Write(b)
    return r.ResponseWriter.Write(b)
}
```

**Option B: Use mcp-go hooks if available**

Check if mcp-go provides middleware/hook points for logging. If so, use those instead of HTTP-level interception.

### Configuration

Add logging level configuration:

```yaml
# config.yaml
mcp:
  log_requests: true       # Log tool names and params
  log_responses: false     # Log response content (verbose)
  log_errors: true         # Always log errors
```

Or environment variables:
```
MCP_LOG_REQUESTS=true
MCP_LOG_RESPONSES=false
MCP_LOG_ERRORS=true
```

### Sensitive Data Handling

Tool arguments may contain sensitive data. Options:

1. **Redact known sensitive fields:** Mask `password`, `secret`, `token` fields
2. **Truncate large values:** Limit string values to 100 chars in logs
3. **Configurable verbosity:** Full logging in dev, minimal in prod

```go
func sanitizeArguments(args map[string]any) map[string]any {
    sensitive := []string{"password", "secret", "token", "key", "credential"}
    result := make(map[string]any)
    for k, v := range args {
        for _, s := range sensitive {
            if strings.Contains(strings.ToLower(k), s) {
                result[k] = "[REDACTED]"
                continue
            }
        }
        // Truncate long strings
        if str, ok := v.(string); ok && len(str) > 200 {
            result[k] = str[:200] + "..."
        } else {
            result[k] = v
        }
    }
    return result
}
```

---

## Implementation Tasks

### Phase 1: Error Result Helper

1. [x] **COMPLETED** - Create `pkg/mcp/tools/errors.go` with `NewErrorResult()` and `NewErrorResultWithDetails()` helpers
   - Implementation: `pkg/mcp/tools/errors.go` contains two helper functions that return structured error responses as successful MCP tool results
   - Key design: Errors are returned as `*mcp.CallToolResult, nil` rather than `nil, error` to ensure Claude sees error details
   - ErrorResponse struct: `{error: true, code: string, message: string, details?: any}`
   - Test coverage: `pkg/mcp/tools/errors_test.go` includes unit tests and real-world usage examples
   - Pattern established: Use `NewErrorResult()` for actionable errors, reserve Go errors for system failures
   - **UPDATED**: Added `result.IsError = true` to both helper functions to ensure MCP protocol-level error flag is set
   - Next implementer: Apply these helpers to MCP tools starting with `get_ontology` and `update_entity`
2. [x] **COMPLETED** - Update `get_ontology` tool to use error results
   - Implementation: `pkg/mcp/tools/ontology.go` now returns error results for actionable parameter validation errors
   - Changes made:
     - **Invalid depth parameter** (ontology.go:122-125): Returns `NewErrorResult("invalid_parameters", ...)` with specific error message showing valid values
     - **Columns depth validation** (ontology.go:221-236): Pre-validates table requirements in `handleColumnsDepth` before calling service
       - Empty tables list: Returns `NewErrorResult` explaining table names are required
       - Too many tables: Returns `NewErrorResultWithDetails` with `requested_count` and `max_allowed` in details field
   - What was NOT changed:
     - Service-layer errors (no active ontology, database failures) still returned as Go errors - these are system failures, not actionable by Claude
     - Main handler pattern (lines 136-146) for "no active ontology" remains unchanged - returns special-case response with instructions
   - Test coverage: `pkg/mcp/tools/ontology_test.go` includes `TestGetOntologyTool_ErrorResults` with 3 test cases:
     - Invalid depth values (validates error result structure)
     - Columns depth without tables (validates error message)
     - Columns depth with too many tables (validates error details)
   - Design decision: Only parameter validation errors were converted to error results. This is the pattern to follow - catch obvious parameter errors early before calling services
   - Next implementer: Apply same pattern to `update_entity` tool - validate parameters, return error results for actionable errors, keep Go errors for system failures
3. [ ] Update `update_entity` tool to use error results
4. [ ] Test with Claude Desktop - verify error messages are visible

### Phase 2: MCP Logging Middleware

1. [ ] Create `pkg/middleware/mcp_logging.go`
2. [ ] Integrate into MCP handler chain
3. [ ] Add configuration options
4. [ ] Add sensitive data redaction

### Phase 3: Rollout to All Tools

1. [ ] Audit all MCP tools for error handling patterns
2. [ ] Convert actionable errors to error results
3. [ ] Keep system errors as Go errors
4. [ ] Document the error handling pattern

---

## Testing

### Error Result Testing

1. Connect Claude Desktop to local server
2. Call `get_ontology(depth='columns')` without specifying tables
3. Verify Claude receives: `{"error": true, "code": "...", "message": "table names required..."}`
4. Verify Claude can adjust and retry with tables specified

### Logging Testing

1. Enable MCP logging
2. Call various tools
3. Verify logs show:
   - Tool name
   - Parameters (sanitized)
   - Success/failure
   - Error details on failure
   - Duration

### Error Code Reference

Define standard error codes for consistency:

| Code | Meaning |
|------|---------|
| `ontology_not_found` | No active ontology for project |
| `entity_not_found` | Requested entity doesn't exist |
| `invalid_parameters` | Required parameter missing or invalid |
| `permission_denied` | Tool not enabled for this project |
| `resource_conflict` | Resource already exists |
| `validation_error` | Input failed validation |

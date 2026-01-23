# TASK: Add Conversation ID to Model Gateway Requests

**Status:** ✅ Complete (2026-01-23)

## Context

When debugging issues between the ekaya-engine client and the ekaya-model-gateway, there's currently no way to correlate client-side conversation IDs with server-side logs. This makes it difficult to trace errors back to specific conversations.

Example server log (no way to correlate to client conversation):
```
2026-01-23T22:41:53.953+0200  ERROR  proxy/handler.go:141  Chat completion failed  {"error": "backend request failed...", "project_id": "00000000-0000-0000-0000-000000000000"}
```

Example client error (has conversation_id but server doesn't know it):
```
CONVERSATION_ID: f4be08f7-1b61-4021-9a8d-e1a3c755c427
TYPE: ERROR
{"error":"Bad Gateway","message":"Failed to process request"}
```

## Solution

The ekaya-model-gateway uses chi's `middleware.RequestID` which accepts a client-provided `X-Request-Id` header. If the client sends this header, the gateway will log it with all request-related output.

## Implementation

In the ekaya-engine code that makes requests to the model gateway (chat completions, embeddings), add the `X-Request-Id` header set to the conversation ID:

```go
req.Header.Set("X-Request-Id", conversationID)
```

### Files to Update

Search for where HTTP requests are made to the model gateway endpoints:
- `/v1/chat/completions`
- `/v1/embeddings`

Add the header before sending the request.

### Expected Behavior

After implementation:
1. Client sends: `X-Request-Id: f4be08f7-1b61-4021-9a8d-e1a3c755c427`
2. Gateway logs include: `"request_id": "f4be08f7-1b61-4021-9a8d-e1a3c755c427"`
3. Gateway error responses include: `"request_id": "f4be08f7-1b61-4021-9a8d-e1a3c755c427"`

This enables full end-to-end tracing of requests from client conversation to gateway logs and back.

## Notes

- Only send the header if you have a conversation ID; the gateway will only log it if present
- The header value should be the conversation UUID (or any unique identifier for the request context)
- The gateway implementation is complete - this task is client-side only

## Implementation (Completed)

**Files modified:**
- `pkg/llm/context.go` - Added `WithConversationID()` and `GetConversationID()` context helpers
- `pkg/llm/client.go` - Added `contextAwareTransport` that injects `X-Request-Id` header from context
- `pkg/llm/recording_client.go` - Sets conversation ID in context before calling inner client

**Tests added:**
- `pkg/llm/client_test.go` - Tests for `contextAwareTransport`
- `pkg/llm/context_test.go` - Tests for conversation ID context functions

# ISSUE: LLM Errors Not Surfaced to UI

**Status:** FIXED (2026-02-05)

## Summary

When the LLM gateway is unavailable (connection refused), the backend logs errors but the UI shows no indication of failure. Users have no way to know that extraction failed or is experiencing problems.

## Observed Behavior

**Date:** 2026-02-04

**Steps to reproduce:**
1. Stop the AI model gateway (localhost:30000)
2. Start ontology extraction from the UI
3. Observe server logs show repeated LLM connection errors
4. UI shows no errors

**Server logs (saved as `no-ui-errors.txt`):**
```
2026-02-04T20:23:33.929+0200    ERROR    llm    llm/client.go:124    LLM request failed
{"elapsed": "404.042µs", "error": "Post \"http://localhost:30000/v1/chat/completions\": dial tcp [::1]:30000: connect: connection refused"}
```

**UI behavior:**
- No error messages displayed
- No indication that LLM calls are failing
- Progress indicators may show misleading state

## Expected Behavior

1. UI should display an error when LLM requests fail
2. DAG status should show "failed" with error message
3. User should be able to see what went wrong and retry

## Impact

- **User confusion:** Users think extraction is working when it's not
- **Silent failures:** No feedback loop for configuration issues
- **Debugging difficulty:** Users won't know to check server logs

## Potential Root Causes

1. **Error not propagating to DAG:** LLM errors may not be updating the DAG node status
2. **SSE events not sending errors:** The server-sent events stream may not be emitting error events
3. **UI not handling error events:** The frontend may not be rendering error states from the DAG
4. **Retry logic masking errors:** If retries are happening, the error might be logged but execution continues

## Files to Investigate

- `pkg/llm/client.go` - LLM client error handling
- `pkg/services/dag/` - DAG node execution and error propagation
- `pkg/services/ontology_dag_service.go` - DAG orchestration
- `ui/src/components/ontology/OntologyDAG.tsx` - UI DAG display
- `ui/src/components/ontology/WorkQueue.tsx` - Work queue error display

## Priority

**High** - Users cannot diagnose extraction failures without this feedback.

## Root Cause Analysis

**Actual Root Cause:** #1 - Error not propagating to DAG

The first DAG node that uses LLM (`KnowledgeSeedingNode`) was configured to swallow ALL errors including connection errors. This "graceful degradation" pattern was overly broad:

```go
// BEFORE: All errors swallowed
if err != nil {
    n.Logger().Warn("Failed to extract knowledge from overview - continuing without seeded knowledge", ...)
    factsStored = 0
}
```

When the LLM gateway is unavailable:
1. `KnowledgeSeedingNode` swallows the connection error, marks itself "completed"
2. Subsequent nodes that need LLM will also fail, but the first node already completed successfully
3. If subsequent nodes have no data to process (e.g., 0 columns), they complete instantly
4. DAG appears to complete successfully despite LLM being unreachable

## Fix Applied

Modified `pkg/services/dag/knowledge_seeding_node.go` to propagate endpoint and auth errors while still gracefully degrading for other error types:

```go
// AFTER: Connection/auth errors propagate, others degrade gracefully
if err != nil {
    errType := llm.GetErrorType(err)
    if errType == llm.ErrorTypeEndpoint || errType == llm.ErrorTypeAuth {
        return fmt.Errorf("LLM configuration error: %w", err)
    }
    // Other errors - continue without seeded knowledge
    n.Logger().Warn("Failed to extract knowledge from overview - continuing without seeded knowledge", ...)
    factsStored = 0
}
```

This ensures:
- **Connection refused** errors fail the DAG immediately with a clear error message
- **Auth errors** (invalid API key) fail the DAG immediately
- **Other errors** (parsing failures, rate limits, etc.) allow graceful degradation since the LLM is working

## Tests Added

- `TestKnowledgeSeedingNode_Execute_EndpointError_Propagates` - Verifies connection errors fail the DAG
- `TestKnowledgeSeedingNode_Execute_AuthError_Propagates` - Verifies auth errors fail the DAG

## Verification

All tests pass including integration tests (`make check` succeeds).

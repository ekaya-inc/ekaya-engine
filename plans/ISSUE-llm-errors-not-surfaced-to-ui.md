# ISSUE: LLM Errors Not Surfaced to UI

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

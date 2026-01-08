# Plan: Robust Error Handling for Ontology Extraction Pipeline

## Problem Summary

The ontology extraction pipeline "freezes" when encountering transient LLM errors (503 Service Busy, 429 Rate Limited) because:

1. **Dual error classification systems are not aligned** - `llm.IsRetryable()` correctly identifies HTTP errors as retryable, but `retry.IsRetryable()` only recognizes connection-level errors
2. **DAG retry logic uses wrong classifier** - `ontology_dag_service.go:598` calls `retry.DoIfRetryable()` which uses `retry.IsRetryable()` (connection errors only)
3. **Glossary nodes fail fatally** - Unlike `EntityEnrichmentNode` which degrades gracefully, glossary nodes return errors that kill the pipeline
4. **Error messages lose context** - Original HTTP status codes get buried in wrapped errors

## Root Cause Analysis

### Issue 1: Mismatched Error Classification

From `pkg/retry/retry.go:107-119`:
```go
retryablePatterns := []string{
    "connection refused", "connection reset", "broken pipe",
    "no such host", "timeout", "temporary failure",
    // NO HTTP status codes like "503", "429", "500"
}
```

From `pkg/llm/errors.go:86-100`:
```go
// Rate limiting (retryable after backoff)
if strings.Contains(errStr, "429") || strings.Contains(lower, "rate limit") {
    return NewError(ErrorTypeUnknown, "rate limited", true, err)
}
// 5xx server errors (retryable)
if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
    strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
    return NewError(ErrorTypeEndpoint, "server error", true, err)
}
```

The LLM package correctly classifies 503/429 as retryable, but the retry package doesn't recognize them.

### Issue 2: DAG Uses Wrong Retry Function

From `pkg/services/ontology_dag_service.go:596-601`:
```go
retryCfg := retry.DefaultConfig()
err = retry.DoIfRetryable(ctx, retryCfg, func() error {
    return executor.Execute(ctx, dagRecord)
})
```

This uses `retry.IsRetryable()` which misses HTTP errors. Meanwhile, `pkg/services/workqueue/queue.go:236` correctly uses `llm.IsRetryable()`:
```go
if !llm.IsRetryable(err) {
    q.logger.Warn("non-retryable error, failing task immediately"...
```

### Issue 3: Inconsistent Graceful Degradation

**EntityEnrichmentNode** (pkg/services/dag/entity_enrichment_node.go:56-60) - GRACEFUL:
```go
if err := n.entityEnrichment.EnrichEntitiesWithLLM(...); err != nil {
    n.Logger().Warn("Failed to enrich entities... using table names as fallback", zap.Error(err))
}
return nil // Pipeline continues
```

**GlossaryDiscoveryNode** (pkg/services/dag/glossary_discovery_node.go:58-61) - FATAL:
```go
if err != nil {
    return fmt.Errorf("discover glossary terms: %w", err) // Kills pipeline
}
```

### Issue 4: No Jitter in DAG Retry

The workqueue has jitter (pkg/services/workqueue/queue.go:282):
```go
jitter := backoff * 0.1 * (rand.Float64()*2 - 1)
```

But `pkg/retry/retry.go` has no jitter, causing thundering herd when multiple DAGs retry simultaneously.

---

## Implementation Plan

### Phase 1: Unify Error Classification ✅ COMPLETED

**Goal:** Create a single source of truth for retryable error detection that handles both connection and HTTP errors.

**Implementation Notes:**
- Used interface-based approach instead of direct type checking to avoid import cycles
- Added `RetryableError` interface in `pkg/retry/retry.go` with `IsRetryable() bool` method
- Updated `llm.Error` to implement this interface (pkg/llm/errors.go:30-35)
- Extended pattern matching in `retry.IsRetryable()` to include HTTP status codes (429, 500-504), rate limit messages, and GPU/CUDA errors
- Interface check takes precedence over pattern matching for explicit control
- Added comprehensive unit tests in `pkg/retry/retry_test.go` (connection, HTTP, GPU errors)
- Added integration test in `pkg/retry/llm_integration_test.go` verifying LLM errors work correctly with retry logic
- Pattern matching serves as fallback for wrapped errors or non-LLM error types

**Files Modified:**
- `pkg/llm/errors.go` - Added IsRetryable() method
- `pkg/retry/retry.go` - Added RetryableError interface, extended pattern matching
- `pkg/retry/retry_test.go` - Added tests for HTTP/GPU error patterns and interface-based retryability
- `pkg/retry/llm_integration_test.go` (new) - Integration tests with actual llm.Error types

#### Option A: Extend retry.IsRetryable() (Recommended) ✅ USED

**File:** `pkg/retry/retry.go`

Add HTTP status codes to existing patterns:

```go
// In IsRetryable(), add to retryablePatterns slice:
retryablePatterns := []string{
    // Connection errors (existing)
    "connection refused",
    "connection reset",
    "broken pipe",
    "no such host",
    "timeout",
    "temporary failure",
    "too many connections",
    "deadlock",
    "i/o timeout",
    "network is unreachable",
    "connection timed out",
    // HTTP errors (new)
    "429",
    "500",
    "502",
    "503",
    "504",
    "rate limit",
    "service busy",
    "service unavailable",
    "too many requests",
    "cuda error",
    "gpu error",
    "out of memory",
}
```

**Also add check for llm.Error type:**

```go
func IsRetryable(err error) bool {
    if err == nil {
        return false
    }

    // Check if it's an LLM error with explicit retryable flag
    var llmErr *llm.Error
    if errors.As(err, &llmErr) {
        return llmErr.Retryable
    }

    // Fall back to pattern matching
    errStr := strings.ToLower(err.Error())
    // ... existing pattern matching
}
```

This requires adding `"github.com/ekaya-inc/ekaya-engine/pkg/llm"` import to retry package.

#### Option B: Create LLM-specific retry helper

**File:** `pkg/llm/retry.go` (new file)

```go
package llm

import (
    "context"
    "time"
)

// DoIfRetryable executes fn with retry for LLM-retryable errors.
func DoIfRetryable(ctx context.Context, cfg *RetryConfig, fn func() error) error {
    // Similar to retry.DoIfRetryable but uses llm.IsRetryable()
}
```

Then update `ontology_dag_service.go` to use `llm.DoIfRetryable()`.

**Recommendation:** Option A is cleaner - keeps retry logic centralized and avoids code duplication.

---

### Phase 2: Add Jitter to Retry Package ✅ COMPLETED

**Goal:** Prevent thundering herd problem when multiple DAGs retry simultaneously by adding random jitter to retry delays.

**Implementation Notes:**
- Added `JitterFactor` field to `Config` struct (0.0-1.0 range, default 0.1 for +/-10% jitter)
- Implemented `applyJitter()` helper function that adds random variation to delay: `delay +/- (delay * jitterFactor * random(-1 to +1))`
- Applied jitter in all three retry functions: `Do()`, `DoWithResult()`, `DoIfRetryable()`
- Jitter is applied before each wait: `time.After(applyJitter(delay, cfg.JitterFactor))`
- Added comprehensive unit tests verifying jitter bounds and randomness
- Integration tests verify jitter works correctly across all retry functions

**Files Modified:**
- `pkg/retry/retry.go` - Added JitterFactor field, applyJitter() function, applied to all retry logic
- `pkg/retry/retry_test.go` - Added 5 new tests covering jitter functionality:
  - `TestApplyJitter_NoJitter` - Verifies no jitter when factor=0 or negative
  - `TestApplyJitter_WithJitter` - Verifies jitter stays within bounds over 100 iterations
  - `TestDefaultConfig_HasJitter` - Verifies default config has 10% jitter
  - `TestDo_WithJitter` - Integration test with retry.Do() using 20% jitter
  - `TestDoWithResult_WithJitter` - Integration test with retry.DoWithResult()
  - `TestDoIfRetryable_WithJitter` - Integration test with retry.DoIfRetryable()

**Key Design Decisions:**
- Jitter is multiplicative (percentage-based) rather than additive (fixed amount) to scale naturally with delay magnitude
- Jitter factor defaults to 0.1 (10%) which provides meaningful distribution without excessive variation
- Zero or negative jitter factor disables jitter (returns delay unchanged)
- Used standard `math/rand` package (sufficient for jitter purposes, doesn't need crypto/rand)

---

### Phase 3: Make Glossary Nodes Non-Fatal ✅ COMPLETED (2026-01-08)

**Goal:** Glossary discovery/enrichment failures should not kill the pipeline - the ontology is still useful without glossary terms.

**Implementation Summary:**
Both glossary nodes now treat errors as non-fatal, logging warnings instead of failing the pipeline. This follows the same graceful degradation pattern used by `EntityEnrichmentNode`.

**Key Changes:**
1. **GlossaryDiscoveryNode** (line 60):
   - Catches discovery errors and logs warning with context
   - Sets `termCount = 0` to continue with empty glossary
   - Progress reporting continues normally showing "Discovered 0 business terms"

2. **GlossaryEnrichmentNode** (line 59):
   - Catches enrichment errors and logs warning with context
   - Continues without SQL definitions for glossary terms
   - Progress reporting continues normally

3. **Validation still works:**
   - Missing ontology ID still returns error (fail-fast for config issues)
   - Only LLM/service errors are treated as non-fatal

**Test Coverage:**
- Updated `TestGlossaryDiscoveryNode_Execute_DiscoveryError` to verify non-fatal behavior (now expects success, verifies warning logged)
- Created comprehensive test suite in `pkg/services/dag/glossary_enrichment_node_test.go`:
  - Success path with progress tracking
  - Missing ontology ID validation (still fatal)
  - Enrichment errors are non-fatal
  - Progress reporting errors are non-fatal
  - Node name verification
- All existing tests continue to pass

**Files Modified:**
- `pkg/services/dag/glossary_discovery_node.go` (lines 59-65) - Made errors non-fatal
- `pkg/services/dag/glossary_enrichment_node.go` (lines 58-62) - Made errors non-fatal
- `pkg/services/dag/glossary_discovery_node_test.go` (lines 209-232) - Updated test expectations
- `pkg/services/dag/glossary_enrichment_node_test.go` (new, 293 lines) - Added comprehensive test coverage

**Design Pattern:**
This matches the pattern in `EntityEnrichmentNode` (pkg/services/dag/entity_enrichment_node.go:56-60) which logs warnings on LLM failures but continues execution. The glossary is an optional enhancement - core ontology extraction (entities, relationships, columns) can succeed independently.

#### File: `pkg/services/dag/glossary_discovery_node.go`

**Current (lines 57-61):**
```go
termCount, err := n.glossaryDiscovery.DiscoverGlossaryTerms(ctx, dag.ProjectID, *dag.OntologyID)
if err != nil {
    return fmt.Errorf("discover glossary terms: %w", err)
}
```

**Change to:**
```go
termCount, err := n.glossaryDiscovery.DiscoverGlossaryTerms(ctx, dag.ProjectID, *dag.OntologyID)
if err != nil {
    // Log but don't fail - ontology is useful without glossary terms
    n.Logger().Warn("Failed to discover glossary terms - continuing without glossary",
        zap.String("project_id", dag.ProjectID.String()),
        zap.Error(err))
    termCount = 0
}
```

#### File: `pkg/services/dag/glossary_enrichment_node.go`

**Current (lines 57-60):**
```go
if err := n.glossaryEnrichment.EnrichGlossaryTerms(ctx, dag.ProjectID, *dag.OntologyID); err != nil {
    return fmt.Errorf("enrich glossary terms: %w", err)
}
```

**Change to:**
```go
if err := n.glossaryEnrichment.EnrichGlossaryTerms(ctx, dag.ProjectID, *dag.OntologyID); err != nil {
    // Log but don't fail - glossary terms can remain unenriched
    n.Logger().Warn("Failed to enrich glossary terms - terms will lack SQL definitions",
        zap.String("project_id", dag.ProjectID.String()),
        zap.Error(err))
}
```

---

### Phase 4: Improve Error Messages ✅ COMPLETED (2026-01-08)

**Goal:** Error messages should include model, endpoint info, and original status code for debugging.

**Implementation Summary:**
All error messages now include HTTP status code and model information for improved debugging. The Error struct has been enhanced with StatusCode, Model, and Endpoint fields, and ClassifyError() automatically extracts status codes from error strings. The Client's parseError() method enriches all errors with model and endpoint context.

**Key Changes:**
1. **Error struct** (pkg/llm/errors.go:10-18):
   - Added StatusCode, Model, Endpoint fields for enhanced context

2. **Error() method** (pkg/llm/errors.go:21-38):
   - Enhanced formatting to include "HTTP {code}" and "model={name}" in error messages
   - Format: "{Type} HTTP {code} model={model}: {message}: {cause}"
   - Each component is optional - only included if present
   - Example: "ErrorTypeEndpoint HTTP 503 model=gpt-4: server error: <underlying error>"

3. **ClassifyError()** (pkg/llm/errors.go:87-164):
   - Extracts HTTP status codes (400, 401, 403, 404, 429, 500, 502, 503, 504) from error strings
   - Sets StatusCode field on all returned errors where status code is detected
   - Pattern matching looks for status codes in the error string itself

4. **NewErrorWithContext()** (pkg/llm/errors.go:61-72):
   - New constructor for creating errors with full context (model, endpoint, status code)
   - Useful for creating errors from scratch with all debugging information
   - Not currently used but available for future enhancements

5. **Client.parseError()** (pkg/llm/client.go:178-185):
   - Enriches all classified errors with model and endpoint from client instance
   - Ensures every error from the LLM client includes debugging context
   - Model and endpoint are added after ClassifyError returns

**Test Coverage:**
Created comprehensive test suite in pkg/llm/errors_test.go with 13 tests covering:
- Error message formatting with status codes, models, and causes
- ClassifyError() status code extraction for all HTTP codes
- NewErrorWithContext() constructor functionality
- Error interface methods (Unwrap, IsRetryable)
- Preservation of existing *Error instances
- All tests pass successfully

**Files Modified:**
- `pkg/llm/errors.go` - Enhanced Error struct, updated Error() method, updated ClassifyError(), added NewErrorWithContext()
- `pkg/llm/client.go` - Updated parseError() to enrich errors with model and endpoint
- `pkg/llm/errors_test.go` (new, 283 lines) - Comprehensive test coverage

**Design Decisions for Next Session:**
- Status code extraction is string-based (checking for "503", "429", etc. in error text) - works for OpenAI SDK errors
- Model and endpoint are added at the client level, not in ClassifyError, to avoid coupling ClassifyError to client details
- The Error() method builds a compact, readable format: type, HTTP code, model, message, cause
- NewErrorWithContext() exists but is not used yet - future enhancement could use it for more precise error creation

#### File: `pkg/llm/errors.go`

1. Add context fields to Error struct:
```go
type Error struct {
    Type       ErrorType
    Message    string
    Retryable  bool
    Cause      error
    StatusCode int    // HTTP status code if applicable
    Model      string // Model name if known
    Endpoint   string // Endpoint URL if known
}
```

2. Update Error() method to include context:
```go
func (e *Error) Error() string {
    var parts []string
    parts = append(parts, string(e.Type))

    if e.StatusCode > 0 {
        parts = append(parts, fmt.Sprintf("HTTP %d", e.StatusCode))
    }
    if e.Model != "" {
        parts = append(parts, fmt.Sprintf("model=%s", e.Model))
    }

    parts = append(parts, e.Message)

    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", strings.Join(parts, " "), e.Cause)
    }
    return strings.Join(parts, " ")
}
```

3. Update ClassifyError() to extract status codes:
```go
func ClassifyError(err error) *Error {
    // ... existing code ...

    // Extract HTTP status code from error string
    statusCode := 0
    for _, code := range []int{400, 401, 403, 404, 429, 500, 502, 503, 504} {
        if strings.Contains(errStr, fmt.Sprintf("%d", code)) {
            statusCode = code
            break
        }
    }

    // Return error with status code
    llmErr := NewError(errType, message, retryable, err)
    llmErr.StatusCode = statusCode
    return llmErr
}
```

4. Add NewErrorWithContext constructor:
```go
func NewErrorWithContext(errType ErrorType, message string, retryable bool, cause error, model, endpoint string, statusCode int) *Error {
    return &Error{
        Type:       errType,
        Message:    message,
        Retryable:  retryable,
        Cause:      cause,
        Model:      model,
        Endpoint:   endpoint,
        StatusCode: statusCode,
    }
}
```

#### Update LLM Client Error Wrapping

**File:** `pkg/llm/openai_client.go` (or equivalent)

When catching errors from OpenAI SDK, create errors with context:
```go
if err != nil {
    llmErr := ClassifyError(err)
    llmErr.Model = c.modelName
    llmErr.Endpoint = c.baseURL
    return llmErr
}
```

---

### Phase 5: Add Repeated Error Escalation ✅ COMPLETED (2026-01-08)

**Goal:** After N consecutive failures of the same error type, treat as permanent failure to avoid infinite retry loops.

**Implementation Summary:**
DoIfRetryable now tracks consecutive same-type errors and escalates to permanent failure after MaxSameErrorType consecutive failures (default: 5). A new classifyErrorType() helper extracts error categories for comparison.

**Key Changes:**
1. **Config struct** (pkg/retry/retry.go:10-18):
   - Added MaxSameErrorType field (default: 5) to control escalation threshold

2. **classifyErrorType() helper** (pkg/retry/retry.go:186-228):
   - Extracts error categories from error strings for comparison
   - Returns categories like "503", "429", "timeout", "connection", "oom", "gpu", "broken_pipe", "rate_limit", "unknown"
   - Checks HTTP status codes first (503, 502, 504, 500, 429, 404, 403, 401, 400)
   - Then checks connection errors (connection refused/reset)
   - Then checks timeouts, broken pipe, rate limiting, GPU/CUDA errors, OOM
   - Falls back to "unknown" for unclassified errors

3. **DoIfRetryable() tracking** (pkg/retry/retry.go:236-267):
   - Tracks sameErrorCount and lastErrorType across retry attempts
   - Increments count when error type matches previous error
   - Resets count to 1 when error type changes
   - Escalates when sameErrorCount >= MaxSameErrorType (if MaxSameErrorType > 0)
   - Returns formatted error: "repeated error (N times, type=XXX): original error"
   - Escalation check happens BEFORE retrying, preventing unnecessary wait on final attempt

4. **IsRetryable() pattern fix** (pkg/retry/retry.go:153):
   - Added "timed out" pattern to fix bug where "request timed out" wasn't recognized as retryable

**Test Coverage:**
Created comprehensive test suite in pkg/retry/retry_test.go:
- TestClassifyErrorType (31 cases) - Verifies error classification for all error types including HTTP codes, connection errors, timeouts, rate limits, GPU/CUDA errors, OOM
- TestDoIfRetryable_RepeatedErrorEscalation - Verifies escalation after 5 consecutive same-type 503 errors (exactly 5 calls before escalating)
- TestDoIfRetryable_MixedErrorTypes - Verifies counter resets when error type changes (503→502→503→timeout→503 pattern, escalates after 3 consecutive 503s)
- TestDoIfRetryable_EscalationDisabled - Verifies MaxSameErrorType=0 disables escalation (exhausts all retries without escalating)
- TestDoIfRetryable_EscalationAfterSuccess - Verifies success before threshold doesn't trigger escalation (4 errors < 5 threshold)
- TestDefaultConfig_HasMaxSameErrorType - Verifies default config has MaxSameErrorType=5
- All tests pass successfully

**Files Modified:**
- `pkg/retry/retry.go` - Added MaxSameErrorType, classifyErrorType(), tracking logic, "timed out" pattern
- `pkg/retry/retry_test.go` - Added 6 new tests (227 lines) covering escalation behavior plus helper functions

**Design Decisions:**
- MaxSameErrorType=5 provides reasonable default (allows transient issues without infinite loops)
- Setting MaxSameErrorType=0 disables escalation (for backward compatibility or testing)
- Error type classification is coarse-grained (e.g., all 503 errors are same type) to detect systemic issues
- Escalation error message includes count and type for debugging: "repeated error (5 times, type=503): HTTP 503 Service Unavailable"
- classifyErrorType() checks specific HTTP codes before generic patterns to provide more precise error tracking
- Escalation check happens during retry loop (not after exhausting retries) to fail fast when pattern is detected

**Notes for Next Session:**
- The escalation mechanism is now active for all code using retry.DoIfRetryable() including the ontology DAG executor
- If you see "repeated error" messages in logs, it means the same error type occurred 5+ times consecutively - investigate the root cause
- To tune escalation threshold, adjust MaxSameErrorType in retry.Config (higher = more tolerant of repeated failures, lower = faster escalation)
- The classifyErrorType() function can be extended with new error patterns if needed

#### File: `pkg/retry/retry.go`

1. Add escalation config:
```go
type Config struct {
    // ... existing fields ...
    MaxSameErrorType int // After N same-type errors, treat as permanent (default: 5)
}

func DefaultConfig() *Config {
    return &Config{
        // ... existing ...
        MaxSameErrorType: 5,
    }
}
```

2. Track error types in DoIfRetryable:
```go
func DoIfRetryable(ctx context.Context, cfg *Config, fn func() error) error {
    if cfg == nil {
        cfg = DefaultConfig()
    }

    var lastErr error
    delay := cfg.InitialDelay
    sameErrorCount := 0
    var lastErrorType string

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        if err := fn(); err == nil {
            return nil
        } else {
            lastErr = err

            if !IsRetryable(err) {
                return err
            }

            // Check for repeated same error type
            currentErrorType := classifyErrorType(err)
            if currentErrorType == lastErrorType {
                sameErrorCount++
                if sameErrorCount >= cfg.MaxSameErrorType {
                    // Escalate to permanent failure
                    return fmt.Errorf("repeated error (%d times): %w", sameErrorCount, err)
                }
            } else {
                sameErrorCount = 1
                lastErrorType = currentErrorType
            }

            // ... rest of retry logic with jitter ...
        }
    }

    return lastErr
}

// classifyErrorType extracts a category from error for comparison
func classifyErrorType(err error) string {
    errStr := err.Error()
    for _, code := range []string{"503", "429", "500", "502", "504", "timeout", "connection"} {
        if strings.Contains(errStr, code) {
            return code
        }
    }
    return "unknown"
}
```

---

## Testing Plan

### Unit Tests

1. **pkg/retry/retry_test.go**
   - Test `IsRetryable()` recognizes HTTP status codes
   - Test jitter is applied within expected bounds
   - Test repeated error escalation triggers after N same-type errors
   - Test mixed error types reset the counter

2. **pkg/llm/errors_test.go**
   - Test `Error.Error()` includes status code, model, endpoint
   - Test `ClassifyError()` extracts status codes correctly

3. **pkg/services/dag/glossary_*_node_test.go**
   - Test nodes continue on error instead of failing

### Integration Tests

1. **Simulate 503 errors**
   - Mock LLM client to return 503 errors
   - Verify retries occur with jitter
   - Verify escalation after repeated failures

2. **Glossary graceful degradation**
   - Simulate glossary service failure
   - Verify DAG completes successfully
   - Verify warning logged

---

## Files Changed Summary

| File | Changes |
|------|---------|
| `pkg/retry/retry.go` | Add HTTP patterns to IsRetryable, add jitter, add error escalation |
| `pkg/llm/errors.go` | Add StatusCode/Model/Endpoint fields, update Error() format |
| `pkg/services/dag/glossary_discovery_node.go` | Make errors non-fatal (warn + continue) |
| `pkg/services/dag/glossary_enrichment_node.go` | Make errors non-fatal (warn + continue) |

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Retry package import cycle with llm | Option A adds llm import to retry; if circular, use Option B |
| Jitter randomness in tests | Seed random or use deterministic jitter in tests |
| Glossary silent failures | Log at WARN level with full error context |
| Extended retry times | Cap at reasonable MaxRetries (3 for DAG, 24 for workqueue) |

---

## Success Criteria

1. Pipeline continues on 503/429 errors with proper retries
2. Error messages show HTTP status code and model name
3. Glossary failures don't kill the pipeline
4. No thundering herd when multiple DAGs retry
5. Repeated same-error-type failures escalate to permanent after 5 attempts

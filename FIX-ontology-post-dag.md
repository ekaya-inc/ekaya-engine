# Ontology DAG Post-Implementation Fixes

Investigation date: 2026-01-03
Branch: `ddanieli/create-ontology-workflow-dag`

## Status

| Issue | Severity | Status | Notes |
|-------|----------|--------|-------|
| Issue 1: Missing RLS on 5 tables | High | ✅ DONE | Migration 024 created |
| Issue 2: DAG startup error propagation | Medium | ✅ DONE | Error messages now stored on nodes |
| Issue 3: Worker Pool generics | Minor | ✅ DONE | Completed 2026-01-03 |
| Issue 4: Column chunk parallelism | Minor | ✅ DONE | Completed 2026-01-03 |
| Issue 5: Heartbeat goroutine leak | Low | ✅ DONE | Completed 2026-01-03 |
| Issue 6: UI stale closure | Low | 📋 TODO | React best practice |
| Issue 7: LLM circuit breaker | Enhancement | 📋 TODO | Resilience |

---

## Issue 1: Missing RLS on 5 Tables ✅

**Severity:** High (Security)
**Status:** COMPLETED 2026-01-03

### Problem

Five tables related to ontology entities are missing Row Level Security (RLS) policies. This means queries could potentially access data across projects if the `app.current_project_id` session variable is not set.

**Affected Tables:**
- `engine_entity_relationships`
- `engine_ontology_entities`
- `engine_ontology_entity_aliases`
- `engine_ontology_entity_key_columns`
- `engine_ontology_entity_occurrences`

### Implementation Notes

Migration 024 created with both up and down scripts:
- All 5 tables now have RLS enabled
- Tables with direct `ontology_id` filter through `engine_ontologies.project_id`
- Child tables (aliases, key_columns, occurrences) filter via JOIN through parent entity's ontology
- All policies follow the standard pattern: allow when `current_project_id` is NULL OR matches project_id
- Down migration fully reverses changes (drops policies and disables RLS)

### Context

All other `engine_*` tables have RLS enabled with a policy like:

```sql
ALTER TABLE engine_xxx ENABLE ROW LEVEL SECURITY;
CREATE POLICY xxx_access ON engine_xxx
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );
```

For tables that don't have a direct `project_id` column (like aliases, key_columns, occurrences), RLS should filter via the parent table's project_id through a subquery.

### Fix

Create migration `migrations/024_add_missing_rls.up.sql`:

```sql
-- ============================================================================
-- RLS for engine_entity_relationships
-- ============================================================================
ALTER TABLE engine_entity_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_relationships_access ON engine_entity_relationships
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR ontology_id IN (
            SELECT id FROM engine_ontologies
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- ============================================================================
-- RLS for engine_ontology_entities
-- ============================================================================
ALTER TABLE engine_ontology_entities ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_entities_access ON engine_ontology_entities
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR ontology_id IN (
            SELECT id FROM engine_ontologies
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- ============================================================================
-- RLS for engine_ontology_entity_aliases
-- ============================================================================
ALTER TABLE engine_ontology_entity_aliases ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_aliases_access ON engine_ontology_entity_aliases
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR entity_id IN (
            SELECT e.id FROM engine_ontology_entities e
            JOIN engine_ontologies o ON e.ontology_id = o.id
            WHERE o.project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- ============================================================================
-- RLS for engine_ontology_entity_key_columns
-- ============================================================================
ALTER TABLE engine_ontology_entity_key_columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_key_columns_access ON engine_ontology_entity_key_columns
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR entity_id IN (
            SELECT e.id FROM engine_ontology_entities e
            JOIN engine_ontologies o ON e.ontology_id = o.id
            WHERE o.project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- ============================================================================
-- RLS for engine_ontology_entity_occurrences
-- ============================================================================
ALTER TABLE engine_ontology_entity_occurrences ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_occurrences_access ON engine_ontology_entity_occurrences
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR entity_id IN (
            SELECT e.id FROM engine_ontology_entities e
            JOIN engine_ontologies o ON e.ontology_id = o.id
            WHERE o.project_id = current_setting('app.current_project_id', true)::uuid
        )
    );
```

Create `migrations/024_add_missing_rls.down.sql`:

```sql
DROP POLICY IF EXISTS entity_relationships_access ON engine_entity_relationships;
ALTER TABLE engine_entity_relationships DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS ontology_entities_access ON engine_ontology_entities;
ALTER TABLE engine_ontology_entities DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entity_aliases_access ON engine_ontology_entity_aliases;
ALTER TABLE engine_ontology_entity_aliases DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entity_key_columns_access ON engine_ontology_entity_key_columns;
ALTER TABLE engine_ontology_entity_key_columns DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entity_occurrences_access ON engine_ontology_entity_occurrences;
ALTER TABLE engine_ontology_entity_occurrences DISABLE ROW LEVEL SECURITY;
```

### Testing

1. Run migration
2. Connect as non-admin user
3. Verify queries only return data for the current project
4. Run `make test-integration` to ensure existing tests pass

---

## Issue 2: Background DAG Execution Has No Startup Error Propagation ✅

**Severity:** Medium
**Status:** COMPLETED 2026-01-03

### Problem

In `pkg/services/ontology_dag_service.go`, the `Start()` method spawns a goroutine and returns immediately:

```go
// Line ~203
go s.executeDAG(projectID, dagRecord.ID)
return dagRecord, nil  // Returns success even if executeDAG fails early
```

If `executeDAG` fails during initialization (e.g., can't get tenant context, can't claim ownership), the caller already received a success response.

### Impact

- UI shows "extraction started" but DAG silently fails
- User must poll status to discover the failure
- No immediate feedback for configuration errors

### Suggested Fix

**Option A: Write early errors to DAG record (Recommended)**

The current approach is acceptable since the UI polls for status. Ensure that any early failure in `executeDAG` updates the DAG record with `status=failed` and an error message before returning.

Current code already does this in the defer block, but verify the error path handles early failures:

```go
func (s *ontologyDAGService) executeDAG(projectID, dagID uuid.UUID) {
    defer func() {
        // Ensure DAG is marked failed if we exit early
        if r := recover(); r != nil {
            s.dagRepo.UpdateStatus(ctx, dagID, models.DAGStatusFailed, nil)
        }
    }()

    ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
    if err != nil {
        // This error should be written to the DAG record
        s.dagRepo.UpdateStatus(ctx, dagID, models.DAGStatusFailed, nil)
        // Also update error message on DAG or first node
        return
    }
    // ...
}
```

**Option B: Use channel for startup errors**

```go
func (s *ontologyDAGService) Start(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
    // ... create DAG record ...

    startupErr := make(chan error, 1)
    go func() {
        if err := s.executeDAGWithStartupSignal(projectID, dagRecord.ID, startupErr); err != nil {
            // Error already sent to channel
        }
    }()

    // Wait briefly for startup errors
    select {
    case err := <-startupErr:
        if err != nil {
            return nil, err
        }
    case <-time.After(500 * time.Millisecond):
        // Startup succeeded, continue
    }

    return dagRecord, nil
}
```

### Implementation Notes

Implemented **Option A** - Enhanced the `markDAGFailed` function to store error messages on the appropriate node:

1. **Error message storage**: When `markDAGFailed` is called, it now:
   - Fetches the DAG with its nodes using `GetByIDWithNodes`
   - Identifies the appropriate node to mark as failed (current node, or first pending/running node, or first node)
   - Updates that node's status to `failed` with the error message via `UpdateNodeStatus`
   - Updates the DAG status to `failed`

2. **Node selection logic** (in priority order):
   - If `current_node` is set, mark that node as failed
   - Otherwise, mark the first pending or running node
   - As a fallback, mark the first node (EntityDiscovery)

3. **UI visibility**: Error messages are now stored in the `error_message` field on the failed node, which the UI can display when polling DAG status.

4. **Graceful degradation**: If `GetByIDWithNodes` fails, the DAG is still marked as failed (without a specific node error message).

5. **Code cleanup**: Simplified `executeDAG` to delegate node failure marking to `markDAGFailed` rather than duplicating logic.

### Testing

Added 5 comprehensive unit tests covering all scenarios:

1. `TestMarkDAGFailed_StoresErrorOnCurrentNode`: Verifies error stored on current node when `current_node` is set
2. `TestMarkDAGFailed_StoresErrorOnFirstPendingNode`: Verifies fallback to first pending/running node when no current node
3. `TestMarkDAGFailed_WhenGetByIDWithNodesFails_StillMarksDAGFailed`: Ensures graceful degradation if node fetching fails
4. `TestMarkDAGFailed_WithAllNodesCompleted_MarksFirstNode`: Verifies fallback to first node when all nodes are completed
5. `TestMarkDAGFailed_WhenTenantCtxFails_LogsError`: Confirms no updates attempted when tenant context unavailable

### Files Modified

- `pkg/services/ontology_dag_service.go`:
  - Enhanced `markDAGFailed` function with node error storage logic (lines 681-746)
  - Simplified `executeDAG` to use `markDAGFailed` for all failure cases (lines 486-517)
- `pkg/services/ontology_dag_service_test.go`:
  - Added mock support for `getByIDWithNodesFunc` (lines 205, 234-238)
  - Added 5 unit tests for `markDAGFailed` (lines 332-599)

---

## Issue 3: Worker Pool Uses `any` Instead of Generics ✅

**Severity:** Minor
**Status:** COMPLETED 2026-01-03

### Problem

The worker pool design called for generics but implementation uses `any`:

**Current (`pkg/llm/worker_pool.go`):**
```go
type WorkItem struct {
    ID      string
    Execute func(ctx context.Context) (any, error)
}

type WorkResult struct {
    ID     string
    Result any  // Callers must type-assert
    Err    error
}
```

**Design (from FIX-ontology-workflow-dag-issues.md):**
```go
type WorkItem[T any] struct {
    ID      string
    Execute func(ctx context.Context) (T, error)
}

type WorkResult[T any] struct {
    ID     string
    Result T
    Err    error
}
```

### Impact

- Callers lose compile-time type safety
- Must type-assert results, which could panic if wrong type

### Suggested Fix

If Go 1.18+ is the minimum version, update to use generics:

```go
// pkg/llm/worker_pool.go

type WorkItem[T any] struct {
    ID      string
    Execute func(ctx context.Context) (T, error)
}

type WorkResult[T any] struct {
    ID     string
    Result T
    Err    error
}

func (p *WorkerPool) Process[T any](
    ctx context.Context,
    items []WorkItem[T],
    onProgress func(completed, total int),
) []WorkResult[T] {
    // Implementation...
}
```

Update callers in:
- `pkg/services/column_enrichment.go`
- `pkg/services/relationship_enrichment.go`

### Implementation Summary

**Completed:** 2026-01-03

Successfully converted worker pool from `any` to generics, achieving compile-time type safety and eliminating runtime type assertions.

**Key Implementation Decisions:**

1. **Process function is a standalone generic function, not a method**
   - Signature: `func Process[T any](ctx context.Context, pool *WorkerPool, items []WorkItem[T], ...) []WorkResult[T]`
   - **Why:** Go methods cannot have type parameters beyond their receiver type
   - **Migration path:** Change `pool.Process(...)` to `llm.Process(ctx, pool, ...)`

2. **Type parameters propagate through the stack:**
   - `WorkItem[T any]` with typed `Execute` function
   - `WorkResult[T any]` with typed `Result` field
   - Callers specify concrete types: `WorkItem[string]`, `WorkItem[*batchResult]`

3. **Zero value handling in cancellation path:**
   - When context is cancelled before execution, must return zero value of type `T`
   - See `pkg/llm/worker_pool.go:84-85` for pattern

**Migration Pattern for Future Sessions:**

When adding new worker pool usage:
```go
// Build typed work items
items := []llm.WorkItem[YourType]{
    {
        ID: "work-1",
        Execute: func(ctx context.Context) (YourType, error) {
            // Your work here
            return result, nil
        },
    },
}

// Process with explicit type parameter
results := llm.Process(ctx, pool, items, progressCallback)

// Results are typed - no assertions needed
for _, r := range results {
    if r.Err != nil {
        // handle error
    }
    // r.Result is YourType, not any
}
```

**Testing:**
- All 8 worker pool unit tests pass
- All 13 column enrichment integration tests pass
- All 8 relationship enrichment integration tests pass
- No runtime behavior changes - tests unchanged except for types

### Files Modified

- `pkg/llm/worker_pool.go`: Converted to generics (lines 42-115)
- `pkg/services/column_enrichment.go`: Updated to use `WorkItem[string]` (lines 107-145)
- `pkg/services/relationship_enrichment.go`: Updated to use `WorkItem[*batchResult]` (lines 117-144)
- `pkg/llm/worker_pool_test.go`: Updated all test cases to use typed work items

---

## Issue 4: Column Chunk Enrichment Still Serial ✅

**Severity:** Minor
**Status:** COMPLETED 2026-01-03

### Problem

Tables with >50 columns are chunked to avoid LLM context limits, but chunks were processed serially rather than in parallel.

**Before (`pkg/services/column_enrichment.go`):**
- For a table with 120 columns, 3 chunks created (50, 50, 20)
- Each chunk waited for previous chunk to complete before starting
- Result: 3 sequential LLM calls, total time = 3 × single chunk time

### Impact

- Large tables took longer than necessary
- Worker pool benefits didn't apply within a single table
- Wasted idle capacity when fewer than MaxConcurrent tables were being processed

### Implementation Summary

**Completed:** 2026-01-03

Successfully converted `enrichColumnsInChunks` from serial to parallel processing using the generic worker pool.

**Key Implementation Details:**

1. **Work item generation**: Each chunk becomes a `WorkItem[[]columnEnrichment]` with its own execute function that captures:
   - Column slice for that chunk
   - Filtered FK info for only columns in that chunk
   - Filtered enum samples for only columns in that chunk
   - Chunk start/end indices for logging

2. **Parallel execution**: Worker pool processes multiple chunks concurrently (bounded by pool's MaxConcurrent setting)

3. **Result ordering**: Results come back in completion order, but must be reassembled in chunk order:
   - Parse chunk index from result ID using `fmt.Sscanf`
   - Build `resultsByChunk` map to store results by their original chunk index
   - Assemble final slice in order (chunk 0, then chunk 1, then chunk 2, etc.)

4. **Error handling**: Any chunk failure stops the entire operation with descriptive error including chunk range

**Critical Design Decisions:**

- **Closure variable capture**: Loop variables (chunkIdx, start, chunkEnd, chunk, FK info, enum samples) must be captured into new variables before being closed over in the Execute function - otherwise all chunks would reference the same values from the final loop iteration
- **ID format**: Work item IDs follow pattern `{table_name}-chunk-{index}` to enable parsing back to chunk index
- **Fail-fast**: First chunk error terminates processing - no partial results returned
- **Retry logic preserved**: All retry logic remains in `enrichColumnBatch`, so transient failures within a chunk still retry appropriately

**Testing:**

Added 2 comprehensive unit tests in `pkg/services/column_enrichment_test.go`:

1. **`TestColumnEnrichmentService_EnrichColumnsInChunks_ParallelProcessing`** (lines 1303-1391):
   - Creates 120 columns → 3 chunks
   - Tracks concurrent execution using timestamps and mutex
   - Simulates 50ms processing time per chunk to ensure overlap
   - Verifies parallel execution by checking if second call started before first call ended
   - Verifies all 120 columns returned in correct order

2. **`TestColumnEnrichmentService_EnrichColumnsInChunks_ChunkFailure`** (lines 1393-1508):
   - Creates 100 columns → 2 chunks
   - Forces second chunk to fail consistently (even with retries) by detecting presence of `col_51`
   - Verifies error propagates with chunk range in message
   - Verifies no partial results returned

**Performance Impact:**

For tables with >50 columns:
- **Before**: Chunks processed serially (e.g., 200 columns = 4 sequential LLM calls taking ~40 seconds)
- **After**: Chunks processed in parallel up to worker pool limit (e.g., with MaxConcurrent=8, all 4 chunks run simultaneously, taking ~10 seconds)

**Files Modified:**

- `pkg/services/column_enrichment.go` (lines 412-509):
  - Added `chunkWorkItem` struct to hold chunk metadata (Index, Start, End)
  - Replaced serial for-loop with work item generation loop
  - Added parallel processing via `llm.Process(ctx, s.workerPool, workItems, nil)`
  - Added result assembly logic with chunk index parsing and ordering

- `pkg/services/column_enrichment_test.go`:
  - Added `generateFunc` field to `testColEnrichmentLLMClient` for custom test behavior (line 413)
  - Conditional dispatch in `GenerateResponse` to use custom func if provided (lines 416-419)
  - Added `TestColumnEnrichmentService_EnrichColumnsInChunks_ParallelProcessing` test (lines 1303-1391)
  - Added `TestColumnEnrichmentService_EnrichColumnsInChunks_ChunkFailure` test (lines 1393-1508)
  - Added imports: `sort`, `time`

**Context for Future Sessions:**

If you need to modify chunked column enrichment behavior:
- The chunking threshold is 50 columns (hardcoded in `enrichColumnsWithLLM`)
- Chunk size is passed as parameter to `enrichColumnsInChunks` (currently 50)
- Each chunk filters FK info and enum samples to only include relevant columns
- Work items are created in chunk order, but execute in parallel and complete in arbitrary order
- Final result slice must match input column order exactly (tests verify this)
- Any changes to work item ID format must update the `fmt.Sscanf` parsing logic

---

## Issue 5: Potential Heartbeat Goroutine Leak ✅

**Severity:** Low
**Status:** ✅ COMPLETED 2026-01-03

### Problem

In `pkg/services/ontology_dag_service.go`, if `executeDAG` panics before the defer runs, the heartbeat goroutine might not be stopped:

```go
func (s *ontologyDAGService) executeDAG(projectID, dagID uuid.UUID) {
    // Heartbeat started here
    s.startHeartbeat(dagID, projectID)

    defer func() {
        s.activeDAGs.Delete(dagID)
        s.stopHeartbeat(dagID)  // Only called if defer executes
        s.releaseOwnership(projectID, dagID)
    }()

    // If panic occurs here before defer is set up...
}
```

### Impact

- Edge case: only affects panics during very early initialization
- Heartbeat goroutine would continue running until process exit
- Minor resource leak

### Implementation Summary

**Completed:** 2026-01-03

Successfully fixed the potential heartbeat goroutine leak and added panic recovery to ensure proper cleanup and DAG failure tracking.

**Key Changes:**

1. **Moved heartbeat start to after defer establishment** (`ontology_dag_service.go:481`):
   - Previously: `startHeartbeat` was called in `Start()` method before spawning goroutine
   - Now: `startHeartbeat` is called inside `executeDAG` after defer block is established
   - This guarantees `stopHeartbeat` will always be called, even if panic occurs

2. **Added panic recovery in defer block** (`ontology_dag_service.go:463-478`):
   - Recovers from any panic using `recover()`
   - Logs panic with stack trace using `zap.Stack("stack")`
   - Calls `markDAGFailed` with descriptive panic message
   - Ensures all cleanup happens: activeDAGs deletion, heartbeat stop, ownership release

3. **Removed heartbeat start from Start() method** (`ontology_dag_service.go:200-202`):
   - Deleted `s.startHeartbeat(dagRecord.ID, projectID)` call
   - Added comment explaining heartbeat is started inside executeDAG

**Testing:**

Added 2 comprehensive unit tests in `pkg/services/ontology_dag_service_test.go`:

1. **`TestExecuteDAG_PanicRecovery`** (lines 603-713):
   - Simulates panic during `getTenantCtx` call
   - Verifies panic is recovered and doesn't crash process
   - Verifies `markDAGFailed` is called with panic message
   - Verifies all cleanup happens (activeDAGs, heartbeat)
   - Uses mock repository to track DAG failure updates

2. **`TestExecuteDAG_HeartbeatCleanupOrder`** (lines 715-773):
   - Simulates early error (repository failure)
   - Verifies cleanup happens even on non-panic errors
   - Verifies heartbeat and activeDAGs are properly cleaned up
   - Ensures no goroutine leaks

**Error Handling:**

The panic recovery provides detailed logging including:
- DAG ID and Project ID
- Panic value (the error that caused the panic)
- Full stack trace for debugging
- Error stored on DAG node for UI visibility

**Files Modified:**

- `pkg/services/ontology_dag_service.go`:
  - Removed `startHeartbeat` call from `Start()` method (line ~200)
  - Added panic recovery in `executeDAG` defer (lines 462-477)
  - Moved `startHeartbeat` call to after defer (line 480)
  - Added comment explaining execution order

- `pkg/services/ontology_dag_service_test.go`:
  - Added imports: `fmt`, `sync`, `time`
  - Added `TestExecuteDAG_PanicRecovery` test (lines 603-713)
  - Added `TestExecuteDAG_HeartbeatCleanupOrder` test (lines 715-773)

**Context for Future Sessions:**

If you need to modify DAG execution behavior:
- The defer block at the start of `executeDAG` is critical - it handles both normal and panic cleanup
- `markDAGFailed` is safe to call from panic recovery - it handles its own errors gracefully
- The heartbeat is started after defer setup to guarantee cleanup
- All cleanup operations (stopHeartbeat, releaseOwnership, activeDAGs deletion) are in the defer
- Panic messages are formatted with `fmt.Sprintf("panic during execution: %v", r)` for clarity
- **Order matters:** defer → recover → startHeartbeat ensures no leaks even on panic

---

## Issue 6: UI Stale Closure Risk in useEffect

**Severity:** Low

### Problem

In `ui/src/components/ontology/OntologyDAG.tsx`, the initial useEffect checks `dagStatus` which may not have updated yet:

```tsx
// Lines 261-280
useEffect(() => {
    const init = async () => {
        setIsLoading(true);
        await fetchStatus();  // Sets dagStatus
        setIsLoading(false);

        // dagStatus may still be null here due to React's async state updates
        if (dagStatus && !isTerminalStatus(dagStatus.status)) {
            startPolling();
        }
    };

    void init();
    // ...
}, [projectId, datasourceId]);
```

### Impact

- Polling might not start even when DAG is running
- Race condition between state update and check

### Suggested Fix

Move the polling decision into the fetchStatus callback or use the response directly:

```tsx
useEffect(() => {
    const init = async () => {
        setIsLoading(true);

        try {
            const response = await engineApi.getOntologyDAGStatus(projectId, datasourceId);

            if (response.data) {
                setDagStatus(response.data);

                // Use response directly, not state
                if (!isTerminalStatus(response.data.status)) {
                    startPolling();
                }
            } else {
                setDagStatus(null);
            }
        } catch (err) {
            // Handle error
        } finally {
            setIsLoading(false);
        }
    };

    void init();

    return () => {
        isMountedRef.current = false;
        stopPolling();
    };
}, [projectId, datasourceId]);
```

### Files to Modify

- `ui/src/components/ontology/OntologyDAG.tsx`

---

## Issue 7: No Circuit Breaker for LLM Provider Outages

**Severity:** Enhancement

### Problem

If the LLM provider is completely down, all batches will retry 3 times before failing. With 38 tables, that's 114 failed LLM calls before the workflow fails.

### Impact

- Long wait times during outages
- Unnecessary API calls and costs
- Poor user experience

### Suggested Fix

Implement a circuit breaker pattern that trips after N consecutive failures:

```go
// pkg/llm/circuit_breaker.go

type CircuitBreaker struct {
    mu               sync.RWMutex
    consecutiveFails int
    threshold        int           // Trip after this many failures
    resetAfter       time.Duration // Reset after this duration
    lastFailure      time.Time
    state            CircuitState  // closed, open, half-open
}

type CircuitState int

const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()

    if cb.state == CircuitOpen {
        if time.Since(cb.lastFailure) > cb.resetAfter {
            // Allow one request through (half-open)
            return true
        }
        return false
    }
    return true
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.consecutiveFails = 0
    cb.state = CircuitClosed
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    cb.consecutiveFails++
    cb.lastFailure = time.Now()

    if cb.consecutiveFails >= cb.threshold {
        cb.state = CircuitOpen
    }
}
```

### Integration

Add circuit breaker check in worker pool or enrichment services:

```go
func (s *columnEnrichmentService) enrichColumnBatch(...) ([]columnEnrichment, error) {
    if !s.circuitBreaker.Allow() {
        return nil, fmt.Errorf("circuit breaker open: LLM provider appears to be down")
    }

    result, err := llmClient.GenerateResponse(...)
    if err != nil {
        s.circuitBreaker.RecordFailure()
        return nil, err
    }

    s.circuitBreaker.RecordSuccess()
    return result, nil
}
```

### Configuration

```go
// Suggested defaults
circuitBreaker := llm.NewCircuitBreaker(llm.CircuitBreakerConfig{
    Threshold:  5,              // Trip after 5 consecutive failures
    ResetAfter: 30 * time.Second, // Try again after 30 seconds
})
```

### Files to Create/Modify

- `pkg/llm/circuit_breaker.go` (new)
- `pkg/llm/circuit_breaker_test.go` (new)
- `pkg/services/column_enrichment.go`
- `pkg/services/relationship_enrichment.go`
- `main.go` (wire circuit breaker)

---

## Implementation Priority

| Issue | Priority | Effort | Recommendation |
|-------|----------|--------|----------------|
| Issue 1: RLS | **P0** | Low | Fix immediately - security gap |
| Issue 2: Error propagation | P1 | Medium | Good UX improvement |
| Issue 7: Circuit breaker | P2 | Medium | Important for production resilience |
| Issue 3: Generics | P3 | Low | Nice to have for type safety |
| Issue 4: Chunk parallelism | P3 | Low | Performance optimization |
| Issue 5: Heartbeat leak | P4 | Low | Edge case, low risk |
| Issue 6: UI stale closure | P4 | Low | Minor React best practice |

# Ontology DAG Post-Implementation Fixes

Investigation date: 2026-01-03
Branch: `ddanieli/create-ontology-workflow-dag`

## Status

| Issue | Severity | Status | Notes |
|-------|----------|--------|-------|
| Issue 1: Missing RLS on 5 tables | High | ✅ DONE | Migration 024 created |
| Issue 2: DAG startup error propagation | Medium | ✅ DONE | Error messages now stored on nodes |
| Issue 3: Worker Pool generics | Minor | ✅ DONE | Completed 2026-01-03 |
| Issue 4: Column chunk parallelism | Minor | 📋 TODO | Performance |
| Issue 5: Heartbeat goroutine leak | Low | 📋 TODO | Edge case |
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

## Issue 4: Column Chunk Enrichment Still Serial

**Severity:** Minor

### Problem

Tables with >50 columns are chunked, but chunks process serially:

**Current (`pkg/services/column_enrichment.go:395-441`):**
```go
func (s *columnEnrichmentService) enrichColumnsInChunks(...) ([]columnEnrichment, error) {
    var allEnrichments []columnEnrichment

    for i := 0; i < len(columns); i += chunkSize {
        // Each chunk waits for the previous to complete
        enrichments, err := s.enrichColumnBatch(ctx, projectID, llmClient, entity, chunk, ...)
        if err != nil {
            return nil, fmt.Errorf("chunk %d-%d failed: %w", i, end, err)
        }
        allEnrichments = append(allEnrichments, enrichments...)
    }

    return allEnrichments, nil
}
```

For a 200-column table, this means 4 serial LLM calls instead of potentially parallel.

### Impact

- Large tables take longer than necessary
- Worker pool benefits don't apply within a single table

### Suggested Fix

Use the worker pool for chunks within large tables:

```go
func (s *columnEnrichmentService) enrichColumnsInChunks(...) ([]columnEnrichment, error) {
    // Build work items for each chunk
    var workItems []llm.WorkItem
    for i := 0; i < len(columns); i += chunkSize {
        end := i + chunkSize
        if end > len(columns) {
            end = len(columns)
        }
        chunk := columns[i:end]
        chunkIdx := i // Capture for closure

        workItems = append(workItems, llm.WorkItem{
            ID: fmt.Sprintf("chunk-%d", chunkIdx),
            Execute: func(ctx context.Context) (any, error) {
                return s.enrichColumnBatch(ctx, projectID, llmClient, entity, chunk, ...)
            },
        })
    }

    // Process chunks in parallel
    results := s.workerPool.Process(ctx, workItems, nil)

    // Aggregate results in order
    var allEnrichments []columnEnrichment
    for _, r := range results {
        if r.Err != nil {
            return nil, r.Err
        }
        allEnrichments = append(allEnrichments, r.Result.([]columnEnrichment)...)
    }

    return allEnrichments, nil
}
```

### Files to Modify

- `pkg/services/column_enrichment.go`

---

## Issue 5: Potential Heartbeat Goroutine Leak

**Severity:** Low

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

### Suggested Fix

Start heartbeat after defer is established:

```go
func (s *ontologyDAGService) executeDAG(projectID, dagID uuid.UUID) {
    // Set up defer FIRST
    defer func() {
        s.activeDAGs.Delete(dagID)
        s.stopHeartbeat(dagID)
        s.releaseOwnership(projectID, dagID)

        if r := recover(); r != nil {
            s.logger.Error("DAG execution panicked",
                zap.String("dag_id", dagID.String()),
                zap.Any("panic", r))
            // Update DAG status to failed
        }
    }()

    // Now start heartbeat (defer will clean it up)
    s.startHeartbeat(dagID, projectID)

    // ... rest of execution
}
```

### Files to Modify

- `pkg/services/ontology_dag_service.go`

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

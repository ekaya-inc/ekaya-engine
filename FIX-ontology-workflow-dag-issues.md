# DAG Workflow Issues - Observed During Testing

## Test Run Summary
- **Date**: 2026-01-03
- **Total Runtime**: 921 seconds (~15 minutes)
- **Final Status**: Completed
- **Results**:
  - EntityDiscovery: Discovered 38 entities
  - EntityEnrichment: Complete
  - RelationshipDiscovery: Discovered 86 relationships (14 FK, 72 inferred)
  - RelationshipEnrichment: Enriched 19 relationships (67 failed)
  - OntologyFinalization: Complete
  - ColumnEnrichment: Enriched 24 tables (14 failed)

---

## Issue 1: Progress Bar Does Not Update During Processing (UI/UX) ✅ COMPLETE

**Severity**: Moderate
**Status**: ✅ Fixed and committed

**Observed Behavior**:
- During RelationshipEnrichment and ColumnEnrichment, the progress bar stays at "0/100" for the entire duration (several minutes each)
- Progress only jumps from 0/100 to 100/100 when the node completes
- RelationshipEnrichment ran for ~3 minutes showing 0/100, then completed
- ColumnEnrichment ran for ~14 minutes showing 0/100, then completed

**Expected Behavior**:
- Progress should update incrementally as items are processed
- Users should see progress like "15/86", "32/86", etc.

**User Impact**:
- Users may think the workflow is stuck/frozen
- No visibility into actual progress during long-running steps

**Files Modified**:
- `pkg/services/relationship_enrichment.go` - Added ProgressCallback parameter and progress reporting after each batch
- `pkg/services/column_enrichment.go` - Added ProgressCallback parameter and progress reporting after each table
- `pkg/services/dag/relationship_enrichment_node.go` - Added ProgressCallback interface and passes callback to service
- `pkg/services/dag/column_enrichment_node.go` - Added ProgressCallback interface and passes callback to service
- `pkg/services/dag_adapters.go` - Updated adapters to forward progress callbacks
- `pkg/services/ontology_dag_service_test.go` - Updated test mocks to match new signatures

**Implementation Details**:
1. Created `dag.ProgressCallback` type: `func(current, total int, message string)` in `relationship_enrichment_node.go`
2. Modified service interfaces to accept optional progress callback (can be nil for backwards compatibility)
3. Services now call progress callback after each batch (relationships) or table (columns)
4. DAG nodes wrap their `ReportProgress` method and pass it to services as a closure
5. Progress updates are persisted to `engine_dag_nodes.progress` via existing `UpdateNodeProgress` repository method
6. UI polls this field and will now see incremental updates during processing

**Testing Recommendations for Next Session**:
- Watch relationship enrichment progress: should show "15/86", "35/86", etc. updates during processing
- Watch column enrichment progress: should show "5/24 tables", "12/24 tables", etc. updates during processing
- No changes to UI code needed - it already polls the progress field that these services now update

---

## Issue 2: High Failure Rate in RelationshipEnrichment (78%) ✅ COMPLETE

**Severity**: High
**Status**: ✅ Fixed, Tested, and Committed

**Observed Behavior**:
- 67 out of 86 relationships failed to enrich (78% failure rate)
- Only 19 relationships were successfully enriched
- Final message: "Enriched 19 relationships (67 failed)"

**Database State**:
```sql
SELECT COUNT(*) as total_rels, COUNT(description) as with_descriptions
FROM engine_entity_relationships
WHERE ontology_id = '<ontology_id>';
-- Result: total_rels=86, with_descriptions=19
```

**Root Cause Analysis**:
1. No retry logic for transient LLM failures (rate limits, timeouts, server errors)
2. No detailed logging to diagnose specific failure reasons
3. No validation of relationship data before sending to LLM
4. Malformed relationships with missing fields or invalid entity references

**Files Modified**:
- `pkg/services/relationship_enrichment.go` - Enhanced error handling and retry logic
- `pkg/services/relationship_enrichment_test.go` - Comprehensive test coverage

**Implementation Details**:
1. **Detailed Error Logging**: Added `logRelationshipFailure()` function that logs:
   - Relationship ID
   - Source/target table and column
   - Detection method and confidence
   - Specific failure reason
   - Error details if available

2. **Retry Logic with Exponential Backoff**:
   - Uses existing `pkg/retry` package
   - 3 retries with 500ms initial delay, 10s max delay, 2x multiplier
   - Leverages `llm.ClassifyError()` to distinguish retryable vs non-retryable errors
   - Retryable: rate limits (429), timeouts, server errors (5xx), GPU errors
   - Non-retryable: authentication (401), model not found, bad requests

3. **Validation Before LLM Calls**: Added `validateRelationships()` function that filters out:
   - Relationships with missing required fields (table/column names)
   - Relationships referencing non-existent entities
   - Invalid relationships are counted as failed and logged immediately

4. **Rate Limit Handling**:
   - Retry logic automatically handles 429 rate limit responses
   - Exponential backoff provides breathing room between retries
   - LLM error classification marks rate limits as retryable

5. **Enhanced Error Context**:
   - Added `truncateString()` helper to safely log LLM response previews
   - All batch failures now log every individual relationship
   - Error messages include project context and batch size

**Testing**:
- `TestRelationshipEnrichmentService_EnrichProject_Success` - Happy path
- `TestRelationshipEnrichmentService_EnrichProject_WithRetryOnTransientError` - Verifies retry on transient failures
- `TestRelationshipEnrichmentService_EnrichProject_ValidationFiltersInvalid` - Validates pre-LLM filtering
- `TestRelationshipEnrichmentService_EnrichProject_NonRetryableError` - Verifies no retry on permanent errors
- `TestRelationshipEnrichmentService_EnrichProject_ProgressCallback` - Verifies progress reporting
- `TestRelationshipEnrichmentService_EnrichProject_EmptyProject` - Edge case handling

**Commit Date**: 2026-01-03

**What Was Done**:
1. Added comprehensive error logging with `logRelationshipFailure()` to diagnose each failure
2. Implemented retry logic using `pkg/retry` with exponential backoff (3 retries, 500ms→10s delays)
3. Added `validateRelationships()` to filter out malformed data before LLM calls
4. Leveraged `llm.ClassifyError()` to distinguish retryable (rate limits, timeouts) from permanent errors
5. Enhanced batch failure logging with truncated LLM response previews for debugging
6. Created comprehensive test suite covering success, retries, validation, and edge cases

**Expected Impact**:
- Transient LLM failures (rate limits, timeouts) should auto-recover via retry
- Invalid relationships detected early, preventing wasted LLM calls
- Detailed logs will reveal if remaining failures are due to data quality, LLM issues, or other causes

**Next Session Should**:
1. Run a full ontology extraction and monitor server logs for relationship enrichment phase
2. Compare new failure rate against the original 78% baseline
3. Analyze remaining failures using the detailed error logs to identify patterns
4. If failures persist above ~20%, investigate: batch size tuning, LLM prompt adjustments, or data quality issues

---

## Issue 3: Moderate Failure Rate in ColumnEnrichment (37%) ✅ COMPLETE

**Severity**: Moderate
**Status**: ✅ Fixed, Tested, and Committed (2026-01-03)

**Observed Behavior**:
- 14 out of 38 tables failed to enrich (37% failure rate)
- Final message: "Enriched 24 tables (14 failed)"

**Root Cause Analysis**:
1. No retry logic for transient LLM failures (rate limits, timeouts, server errors)
2. No detailed logging to diagnose specific failure reasons
3. No handling for tables with many columns exceeding LLM context limits
4. No chunking strategy for large tables

**Files Modified**:
- `pkg/services/column_enrichment.go` - Enhanced error handling, retry logic, and chunking
- `pkg/services/column_enrichment_test.go` - Comprehensive test coverage

**Implementation Details**:
1. **Detailed Error Logging**: Added `logTableFailure()` function that logs:
   - Table name
   - Specific failure reason
   - Error details if available

2. **Retry Logic with Exponential Backoff**:
   - Uses existing `pkg/retry` package
   - 3 retries with 500ms initial delay, 10s max delay, 2x multiplier
   - Leverages `llm.ClassifyError()` to distinguish retryable vs non-retryable errors
   - Retryable: rate limits (429), timeouts, server errors (5xx), GPU errors
   - Non-retryable: authentication (401), model not found, bad requests

3. **Column Chunking for Large Tables**:
   - Implemented `enrichColumnsInChunks()` to handle tables with > 50 columns
   - Splits columns into chunks of 50 to avoid context limits
   - Each chunk is enriched separately and results are combined
   - Filters FK info and enum samples per chunk for efficiency

4. **Enhanced Error Context**:
   - All table failures now log with table context
   - LLM response previews included in parse error logs
   - Progress callbacks report after each table

**Testing**:
- `TestColumnEnrichmentService_EnrichProject_Success` - Happy path
- `TestColumnEnrichmentService_EnrichProject_WithRetryOnTransientError` - Verifies retry on transient failures
- `TestColumnEnrichmentService_EnrichProject_NonRetryableError` - Verifies behavior on permanent errors
- `TestColumnEnrichmentService_EnrichProject_LargeTable` - Verifies chunking for 60-column table
- `TestColumnEnrichmentService_EnrichProject_ProgressCallback` - Verifies progress reporting
- `TestColumnEnrichmentService_EnrichProject_EmptyProject` - Edge case handling

**Commit Date**: 2026-01-03

**What Was Done**:
1. Added comprehensive error logging with `logTableFailure()` to diagnose each failure
2. Implemented retry logic using `pkg/retry` with exponential backoff (3 retries, 500ms→10s delays)
3. Added column chunking via `enrichColumnsInChunks()` for tables with >50 columns
4. Extracted `enrichColumnBatch()` to handle single batch enrichment with retry
5. Leveraged `llm.ClassifyError()` to distinguish retryable (rate limits, timeouts) from permanent errors
6. Enhanced parse error logging with truncated LLM response previews for debugging
7. Created comprehensive test suite covering success, retries, chunking, and edge cases

**Expected Impact**:
- Transient LLM failures (rate limits, timeouts) should auto-recover via retry
- Large tables (>50 columns) will be processed in chunks instead of failing due to context limits
- Detailed logs will reveal remaining failures for targeted fixes
- Overall failure rate should drop significantly (target: <15%)

**Next Session Should**:
1. Run a full ontology extraction and monitor server logs for column enrichment phase
2. Compare new failure rate against the original 37% baseline
3. Analyze remaining failures using the detailed error logs to identify patterns
4. If failures persist above ~15%, investigate: chunk size tuning, LLM prompt adjustments, or data quality issues

---

## Additional Observations

### Timing Data by Node
| Node | Duration | Notes |
|------|----------|-------|
| EntityDiscovery | Fast | Completed quickly |
| EntityEnrichment | ~2 min | Completed successfully |
| RelationshipDiscovery | ~1.5 min | Found 86 relationships |
| RelationshipEnrichment | ~3 min | 78% failure rate |
| OntologyFinalization | Fast | Completed successfully |
| ColumnEnrichment | ~14 min | Longest step, 37% failure rate |

### Positive Observations
- DAG orchestration works correctly - nodes execute in proper sequence
- UI correctly shows node status transitions (pending → running → completed)
- Workflow completes successfully despite partial failures
- Database state is consistent after completion

---

## Issue 4: "Refresh Ontology" Runs Full Extraction Instead of Incremental Update ✅ COMPLETE

**Severity**: High
**Status**: ✅ Fixed and committed (2026-01-03)

**Observed Behavior**:
- Clicking "Refresh Ontology" starts a completely new 6-step extraction workflow from scratch
- Even immediately after a successful extraction, it re-runs everything

**Expected Behavior**:
- "Refresh Ontology" should perform an **incremental update**:
  1. Check for schema changes (new tables, columns, dropped objects)
  2. Only run the minimal DAG nodes necessary to incorporate changes
  3. If nothing changed, complete in seconds with "No changes detected"
- This should NOT be a full re-extraction

**User Impact**:
- Users expect "refresh" to be fast and non-destructive
- Accidentally clicking this wastes 15+ minutes re-running the entire workflow
- Confusing UX - "refresh" implies a quick update, not a full rebuild

**Files Modified**:
- `ui/src/components/ontology/OntologyDAG.tsx` - Renamed button, added confirmation dialog
- `ui/src/components/ontology/__tests__/OntologyDAG.test.tsx` - Added comprehensive test coverage

**Implementation Details**:
1. **Button Text Updated**: Changed "Refresh Ontology" → "Re-extract Ontology" to accurately reflect the operation
2. **Confirmation Dialog Added**:
   - Shows when user clicks "Re-extract Ontology" after a completed extraction
   - Warns about 10-15 minute duration and full data replacement
   - Explains that incremental refresh is not yet implemented
   - Includes warning banner with context about what re-extraction means
   - Cancel and Confirm buttons for user control
3. **No Confirmation for Retry**: Failed extractions show "Retry Extraction" button without confirmation dialog
4. **Comprehensive Testing**: Created 7 test cases covering:
   - Confirmation dialog display on re-extraction
   - Warning message content validation
   - Successful re-extraction after confirmation
   - Dialog cancellation behavior
   - No confirmation on retry (failed state)
   - Correct button text for completed vs failed states

**Decision Made**:
Chose approach #1 (rename + confirmation) over implementing full incremental refresh because:
- Incremental refresh requires schema fingerprinting, delta detection, and partial DAG execution
- That would be a major architectural change requiring separate planning and implementation
- Renaming the button immediately solves the UX confusion issue
- Confirmation dialog prevents accidental full re-extractions
- Sets clear expectations about what the operation does
- Future incremental refresh can be added as a separate feature

**Testing**:
- All 7 test cases pass
- Full test suite passes (`make check`)
- TypeScript strict mode compliance verified
- ESLint compliance verified

**Commit Date**: 2026-01-03

---

## Issue 5: Missing "Delete Ontology" Functionality ✅ COMPLETE

**Severity**: Moderate
**Status**: ✅ Fixed, Tested, and Committed (2026-01-03)

**Observed Behavior**:
- No way to delete an existing ontology from the UI
- User cannot start fresh without manual database intervention

**Expected Behavior**:
- A "Delete Ontology" button should exist
- Must have a serious confirmation dialog requiring the user to type "delete ontology"
- This is a destructive action and should be treated as such

**Files Modified**:
- `main.go` - Updated OntologyDAGService constructor to include all required repositories
- `pkg/services/ontology_dag_service.go` - Added Delete method to service interface and implementation
- `pkg/services/ontology_dag_service_test.go` - Added test documentation note
- `pkg/handlers/ontology_dag_handler.go` - Added DELETE endpoint and handler method
- `pkg/handlers/ontology_dag_handler_test.go` - Added comprehensive test coverage (4 tests)
- `pkg/repositories/ontology_dag_repository.go` - Added GetActiveByProject method
- `pkg/repositories/ontology_question_repository.go` - Added DeleteByProject method
- `pkg/repositories/knowledge_repository.go` - Added DeleteByProject method
- `ui/src/services/engineApi.ts` - Added deleteOntology API method
- `ui/src/components/ontology/OntologyDAG.tsx` - Added Delete button and confirmation dialog with text input
- `ui/src/components/ontology/__tests__/OntologyDAG.test.tsx` - Added comprehensive test coverage (7 tests)
- `ui/src/components/__tests__/QueryResultsTable.test.tsx` - Minor whitespace cleanup

**Implementation Details**:
1. **Backend Service (ontology_dag_service.go)**:
   - Added `Delete(ctx context.Context, projectID uuid.UUID) error` method to interface
   - Implementation deletes all ontology-related data:
     - DAGs (with cascading delete of DAG nodes)
     - Ontology entities (with cascading delete of entity aliases)
     - Entity relationships
     - Ontology questions
     - Chat messages
     - Project knowledge
     - Ontologies
   - Uses transactional approach with comprehensive error handling and logging
   - Prevents deletion while extraction is running

2. **Backend Handler (ontology_dag_handler.go)**:
   - Registered DELETE endpoint: `DELETE /api/projects/{pid}/datasources/{dsid}/ontology`
   - Handler validates project/datasource IDs and calls service Delete method
   - Returns success message on completion

3. **Frontend UI (OntologyDAG.tsx)**:
   - Added "Delete Ontology" button in header (shown only when DAG exists and is not running)
   - Button styled in red with Trash2 icon to indicate destructive action
   - Opens confirmation dialog when clicked

4. **Confirmation Dialog**:
   - Clear warning about permanent data loss
   - Red warning banner explaining consequences
   - Text input requiring exact string "delete ontology" to enable delete button
   - Delete button disabled until confirmation text matches exactly
   - Shows loading state during deletion
   - Resets state after successful deletion

**Testing**:
- Backend handler tests: 4 tests covering success, service error, running DAG error, and invalid project ID
- Frontend tests: 7 tests covering button visibility, confirmation dialog, text input validation, successful deletion, cancellation, and error handling
- All tests pass ✅

**Commit Date**: 2026-01-03

**Verification Date**: 2026-01-03 - Confirmed implementation is complete with comprehensive test coverage

---

## Issue 6: Canceled Workflow Shows "Running" Status for In-Progress Node ✅ COMPLETE

**Severity**: High
**Status**: ✅ Fixed, Tested, and Committed (2026-01-03)

**Observed Behavior**:
- User cancels workflow while Entity Enrichment is running
- Overall status correctly shows "Extraction failed"
- BUT Entity Enrichment node still shows:
  - Spinner animation
  - "Running" status badge
  - "0/100" progress

**Expected Behavior**:
- When workflow is canceled, ALL nodes should update their state:
  - Running nodes → "Skipped" status
  - Pending nodes → "Skipped" status
  - Spinner should stop
  - Status badge should show "Skipped" (not "Running")

**Root Cause**:
The `Cancel` method in `ontology_dag_service.go` only updated the DAG status to "cancelled" but did not update individual node statuses. This caused the frontend to continue showing running nodes with spinners.

**Files Modified**:
- `pkg/services/ontology_dag_service.go` - Updated Cancel method to mark non-completed nodes as skipped
- `pkg/services/ontology_dag_service_test.go` - Added comprehensive test coverage

**Implementation Details**:
1. **Backend Cancel Method Enhancement**:
   - Added logic to fetch all nodes for the DAG being canceled
   - Iterate through nodes and mark any non-completed, non-failed nodes as "skipped"
   - Preserves completed and failed node statuses (don't override them)
   - Continues processing all nodes even if one fails to update (fail-safe approach)
   - Updates DAG status to "cancelled" after updating node statuses

2. **Frontend Compatibility**:
   - No frontend changes needed
   - UI already handles "skipped" node status with gray circle icon and gray badge styling (lines 60-61, 611-612 in OntologyDAG.tsx)
   - Spinner will stop because "skipped" is not a "running" status
   - UI polling will pick up the updated node statuses automatically

3. **Test Coverage**:
   - Created `TestCancel_MarksNonCompletedNodesAsSkipped` test
   - Verifies completed nodes are NOT marked as skipped
   - Verifies running nodes ARE marked as skipped
   - Verifies pending nodes ARE marked as skipped
   - Uses mock repository to isolate unit test

**Design Decision**:
Used "skipped" status instead of creating a new "cancelled" status because:
- "Skipped" already exists in the DAGNodeStatus enum
- Frontend already handles "skipped" status appropriately
- Semantically correct: these nodes were skipped due to cancellation
- Avoids frontend changes and maintains consistency with existing node states

**Testing**:
- Unit test passes ✅
- Full service test suite passes ✅
- Frontend already supports "skipped" status ✅

**Expected Behavior After Fix**:
1. User cancels running workflow
2. Backend marks all non-completed nodes as "skipped"
3. Frontend polls and receives updated node statuses
4. Spinners stop for running nodes
5. Status badges change from "Running" to "Skipped" (gray)
6. Overall DAG status shows "Cancelled"

**Commit Date**: 2026-01-03

**What Was Done**:
1. Enhanced `Cancel` method in `ontology_dag_service.go` to fetch all DAG nodes
2. Added logic to mark all non-completed, non-failed nodes as "skipped" when workflow is cancelled
3. Preserves completed and failed node statuses (doesn't override them)
4. Uses fail-safe approach: continues processing all nodes even if one update fails
5. Created comprehensive unit test `TestCancel_MarksNonCompletedNodesAsSkipped` with mock repository
6. Test verifies: completed nodes stay completed, running/pending nodes become skipped, DAG status becomes cancelled

**Next Session Should**:
1. Manually test cancellation with a running workflow
2. Verify spinners stop and status badges change to "Skipped" (gray)
3. Confirm UI no longer shows stale "Running" state for cancelled nodes

---

## Issue 7: "How it works" Banner Shown When Ontology Already Exists

**Severity**: Low (UI Polish)

**Observed Behavior**:
- The informational banner at the bottom ("How it works: The extraction process runs automatically through 6 steps...") is always displayed
- Still shows after ontology extraction is complete

**Expected Behavior**:
- Show the banner only when:
  - No ontology exists yet, OR
  - User is about to start their first extraction
- Hide the banner once an ontology exists and workflow is not running

**How to Fix**:
1. Add conditional rendering based on ontology existence
2. `{!hasOntology && <HowItWorksBanner />}`

---

## Architecture Note: Screen Purposes and Incremental Updates

**Important Context for Future Work**:

This screen (`{pid}/ontology`) is the **workflow status screen**, not the entity/relationship viewer.

| Screen | Purpose |
|--------|---------|
| `{pid}/ontology` | View DAG workflow status, trigger extractions |
| `{pid}/entities` | View and edit entities |
| `{pid}/relationships` | View and edit relationships |

**Future Enhancement - Incremental DAG Triggers**:
- Editing an entity on `{pid}/entities` should trigger a minimal DAG (just re-enrich that entity)
- Adding a relationship on `{pid}/relationships` should trigger relationship enrichment for just that relationship
- These partial workflows need representation in the linear DAG UI

**Design Consideration**:
The current UI shows a single linear 6-step progression. When we support incremental/partial DAG runs:
- How do we show a "relationship enrichment only" workflow?
- How do we show multiple concurrent partial workflows?
- Consider: workflow history list, or collapsible workflow cards, or a different visualization

This is noted for future design - no changes needed now.

---

## Issue 8: LLM Calls Are Serialized - Should Use Parallel Worker Pool

**Severity**: High (Performance)

**Current Behavior**:

Both `relationship_enrichment.go` and `column_enrichment.go` process LLM calls **serially**:

```go
// relationship_enrichment.go - Serial batch processing
for i := 0; i < len(validRelationships); i += batchSize {
    batch := validRelationships[i:end]
    enriched, failed := s.enrichBatch(ctx, projectID, batch, entityByID)  // BLOCKS
    // ... next batch waits for this one to complete
}

// column_enrichment.go - Serial table processing
for idx, tableName := range tableNames {
    if err := s.EnrichTable(ctx, projectID, tableName); err != nil {  // BLOCKS
        // ... next table waits for this one to complete
    }
}
```

This means:
- With 86 relationships in batches of 20 = 5 LLM calls, executed one at a time
- With 38 tables = 38 LLM calls, executed one at a time
- Total time = sum of all individual LLM call times

**Expected Behavior**:

Send up to **8 concurrent LLM requests** using a worker pool pattern:
- As responses come back, immediately start new requests
- Maintain 8 outstanding requests at all times until work is exhausted
- LLM providers (OpenAI, Anthropic) can batch requests server-side for efficiency

**Existing Infrastructure**:

The codebase already has a `workqueue` package with concurrency strategies:
- `pkg/services/workqueue/strategy.go` - `ThrottledLLMStrategy(maxConcurrent int)`
- However, enrichment services don't use this - they use simple `for` loops

**Proposed Design: LLMWorkerPool**

Create a reusable, DI-injectable worker pool for parallel LLM execution:

```go
// pkg/llm/worker_pool.go

// WorkerPoolConfig configures the LLM worker pool.
type WorkerPoolConfig struct {
    MaxConcurrent int // Maximum concurrent LLM calls (default: 8)
}

// DefaultWorkerPoolConfig returns sensible defaults.
func DefaultWorkerPoolConfig() WorkerPoolConfig {
    return WorkerPoolConfig{
        MaxConcurrent: 8,
    }
}

// WorkerPool manages concurrent LLM call execution with bounded parallelism.
// It uses a semaphore to limit outstanding requests and processes results
// as they complete, allowing new requests to start immediately.
type WorkerPool struct {
    config WorkerPoolConfig
    logger *zap.Logger
}

// NewWorkerPool creates a new LLM worker pool.
func NewWorkerPool(config WorkerPoolConfig, logger *zap.Logger) *WorkerPool {
    if config.MaxConcurrent < 1 {
        config.MaxConcurrent = 8
    }
    return &WorkerPool{
        config: config,
        logger: logger.Named("llm-worker-pool"),
    }
}

// WorkItem represents a unit of work to be processed.
type WorkItem[T any] struct {
    ID      string                                    // For logging/tracking
    Execute func(ctx context.Context) (T, error)     // The LLM call to make
}

// WorkResult represents the result of a work item.
type WorkResult[T any] struct {
    ID     string
    Result T
    Err    error
}

// Process executes all work items with bounded parallelism.
// Returns results in completion order (not submission order).
// Continues processing all items even if some fail.
func (p *WorkerPool) Process[T any](
    ctx context.Context,
    items []WorkItem[T],
    onProgress func(completed, total int),
) []WorkResult[T] {
    if len(items) == 0 {
        return nil
    }

    results := make([]WorkResult[T], 0, len(items))
    resultsChan := make(chan WorkResult[T], len(items))
    sem := make(chan struct{}, p.config.MaxConcurrent)

    var wg sync.WaitGroup

    // Submit all work items
    for _, item := range items {
        wg.Add(1)
        go func(item WorkItem[T]) {
            defer wg.Done()

            // Acquire semaphore slot (blocks if at max concurrency)
            select {
            case sem <- struct{}{}:
                defer func() { <-sem }() // Release slot when done
            case <-ctx.Done():
                resultsChan <- WorkResult[T]{ID: item.ID, Err: ctx.Err()}
                return
            }

            // Execute the LLM call
            result, err := item.Execute(ctx)
            resultsChan <- WorkResult[T]{
                ID:     item.ID,
                Result: result,
                Err:    err,
            }
        }(item)
    }

    // Close results channel when all work is done
    go func() {
        wg.Wait()
        close(resultsChan)
    }()

    // Collect results and report progress
    completed := 0
    for result := range resultsChan {
        results = append(results, result)
        completed++
        if onProgress != nil {
            onProgress(completed, len(items))
        }
    }

    return results
}
```

**Usage in Relationship Enrichment**:

```go
func (s *relationshipEnrichmentService) EnrichProject(
    ctx context.Context,
    projectID uuid.UUID,
    progressCallback dag.ProgressCallback,
) (*EnrichRelationshipsResult, error) {
    // ... setup code ...

    // Build work items for each batch
    var workItems []llm.WorkItem[*batchResult]
    for i := 0; i < len(validRelationships); i += batchSize {
        batch := validRelationships[i:min(i+batchSize, len(validRelationships))]
        batchID := fmt.Sprintf("batch-%d", i/batchSize)

        workItems = append(workItems, llm.WorkItem[*batchResult]{
            ID: batchID,
            Execute: func(ctx context.Context) (*batchResult, error) {
                return s.enrichBatchInternal(ctx, projectID, batch, entityByID)
            },
        })
    }

    // Process all batches with 8 concurrent LLM calls
    results := s.workerPool.Process(ctx, workItems, func(completed, total int) {
        if progressCallback != nil {
            // Map batch progress to relationship progress
            relProgress := (completed * len(validRelationships)) / total
            progressCallback(relProgress, len(relationships),
                fmt.Sprintf("Enriching relationships (%d/%d)...", relProgress, len(relationships)))
        }
    })

    // Aggregate results
    for _, r := range results {
        if r.Err != nil {
            result.RelationshipsFailed += r.Result.BatchSize
        } else {
            result.RelationshipsEnriched += r.Result.Enriched
            result.RelationshipsFailed += r.Result.Failed
        }
    }

    return result, nil
}
```

**Usage in Column Enrichment**:

```go
func (s *columnEnrichmentService) EnrichProject(
    ctx context.Context,
    projectID uuid.UUID,
    tableNames []string,
    progressCallback dag.ProgressCallback,
) (*EnrichColumnsResult, error) {
    // ... setup code ...

    // Build work items for each table
    var workItems []llm.WorkItem[string] // Returns table name on success
    for _, tableName := range tableNames {
        name := tableName // Capture for closure
        workItems = append(workItems, llm.WorkItem[string]{
            ID: name,
            Execute: func(ctx context.Context) (string, error) {
                if err := s.EnrichTable(ctx, projectID, name); err != nil {
                    return name, err
                }
                return name, nil
            },
        })
    }

    // Process all tables with 8 concurrent LLM calls
    results := s.workerPool.Process(ctx, workItems, func(completed, total int) {
        if progressCallback != nil {
            progressCallback(completed, total,
                fmt.Sprintf("Enriching columns (%d/%d tables)...", completed, total))
        }
    })

    // Aggregate results
    for _, r := range results {
        if r.Err != nil {
            s.logTableFailure(r.ID, "Failed to enrich table", r.Err)
            result.TablesFailed[r.ID] = r.Err.Error()
        } else {
            result.TablesEnriched = append(result.TablesEnriched, r.ID)
        }
    }

    return result, nil
}
```

**Dependency Injection**:

Add `WorkerPool` to service constructors:

```go
// In main.go or service initialization
workerPoolConfig := llm.DefaultWorkerPoolConfig() // MaxConcurrent: 8
llmWorkerPool := llm.NewWorkerPool(workerPoolConfig, logger)

relationshipEnrichmentService := services.NewRelationshipEnrichmentService(
    relationshipRepo,
    entityRepo,
    llmFactory,
    llmWorkerPool,  // NEW: inject worker pool
    logger,
)

columnEnrichmentService := services.NewColumnEnrichmentService(
    // ... existing deps ...
    llmWorkerPool,  // NEW: inject worker pool
    logger,
)
```

**Testing Considerations**:

```go
// For unit tests, use MaxConcurrent: 1 to serialize and make tests deterministic
testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())

// For integration tests, can use full parallelism
integrationPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)
```

**Expected Performance Improvement**:

| Scenario | Serial (current) | Parallel (8 concurrent) |
|----------|------------------|-------------------------|
| 5 relationship batches @ 3s each | 15s | ~3-6s |
| 38 tables @ 20s each | 760s (~12.5 min) | ~100-150s (~2 min) |
| **Total enrichment phase** | ~15+ min | ~3-4 min |

**Implementation Steps**:

1. Create `pkg/llm/worker_pool.go` with the `WorkerPool` type
2. Add `WorkerPool` to `RelationshipEnrichmentService` constructor
3. Refactor `EnrichProject` to use worker pool instead of serial loop
4. Add `WorkerPool` to `ColumnEnrichmentService` constructor
5. Refactor `EnrichProject` to use worker pool instead of serial loop
6. Update `main.go` to create and inject the worker pool
7. Update tests to use `MaxConcurrent: 1` for determinism
8. Integration test with real LLM to verify parallelism works

**Alternative: Use Existing workqueue Package**

The codebase has `pkg/services/workqueue` with `ThrottledLLMStrategy`. However:
- It's designed for task queues with callbacks, not request/response patterns
- Would require significant refactoring to fit the enrichment use case
- The simpler `WorkerPool` above is more appropriate for batch LLM calls

The `workqueue` package could be refactored to expose a simpler `ProcessBatch` API, but creating a focused `WorkerPool` in the `llm` package is cleaner.

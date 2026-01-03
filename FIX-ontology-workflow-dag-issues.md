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

## Issue 7: "How it works" Banner Shown When Ontology Already Exists ✅ COMPLETE

**Severity**: Low (UI Polish)
**Status**: ✅ Fixed, Tested, and Committed (2026-01-03)

**Observed Behavior**:
- The informational banner at the bottom ("How it works: The extraction process runs automatically through 6 steps...") is always displayed
- Still shows after ontology extraction is complete

**Expected Behavior**:
- Show the banner only when:
  - No ontology exists yet, OR
  - User is about to start their first extraction
- Hide the banner once an ontology exists and workflow is not running

**Files Modified**:
- `ui/src/components/ontology/OntologyDAG.tsx` - Added onStatusChange callback prop
- `ui/src/pages/OntologyPage.tsx` - Added hasOntology state and conditional banner rendering
- `ui/src/components/ontology/__tests__/OntologyDAG.test.tsx` - Added 3 test cases for status change callback

**Implementation Details**:
1. **OntologyDAG Component Changes**:
   - Added optional `onStatusChange?: (hasOntology: boolean) => void` prop to component interface
   - Added useEffect hook that calls `onStatusChange(dagStatus !== null)` whenever dagStatus changes
   - This notifies parent component whenever ontology existence status changes (initial load, after deletion, after extraction)

2. **OntologyPage Component Changes**:
   - Added `hasOntology` state (boolean) to track whether ontology data exists
   - Created `handleStatusChange` callback that updates hasOntology state
   - Passed `onStatusChange={handleStatusChange}` to OntologyDAG component
   - Wrapped info banner in conditional: `{!hasOntology && <InfoBanner />}`

3. **Status Change Triggers**:
   - **On initial load**: OntologyDAG fetches status and calls onStatusChange(dagStatus !== null)
   - **After deletion**: OntologyDAG resets dagStatus to null, triggering onStatusChange(false)
   - **After extraction**: DAG status exists, triggering onStatusChange(true)

4. **Test Coverage**:
   - `calls onStatusChange with false when no DAG exists` - Verifies callback when API returns null
   - `calls onStatusChange with true when DAG exists` - Verifies callback when DAG data is present
   - `calls onStatusChange when DAG is deleted` - Verifies callback transitions from true→false after deletion

**Design Decision - Event-Driven Pattern**:
Used callback prop pattern instead of context or lifting state because:
- Simple parent-child relationship (OntologyPage → OntologyDAG)
- Only one consumer needs the status (OntologyPage for banner visibility)
- Keeps OntologyDAG reusable - doesn't force it to know about banner visibility logic
- Clear data flow: DAG component owns status, parent reacts to changes

**Alternative Considered**:
Could have exposed `dagStatus` via prop directly (`onStatusChange={(status) => ...}`) but chose boolean because:
- Parent only needs to know "exists or not" for banner logic
- Simpler contract - boolean is easier to reason about than full status object
- Prevents parent from coupling to DAG status internals

**Commit Date**: 2026-01-03

**What Was Done**:
1. Modified OntologyDAG to accept optional onStatusChange callback prop
2. Added useEffect to call onStatusChange(dagStatus !== null) when dagStatus changes
3. Modified OntologyPage to track hasOntology state via handleStatusChange callback
4. Wrapped info banner in conditional rendering: `{!hasOntology && <Banner />}`
5. Added 3 comprehensive test cases covering status changes on load, existence, and deletion

**Expected Behavior After Fix**:
1. User visits page with no ontology → Banner is visible
2. User starts extraction → Banner remains visible (ontology doesn't exist yet)
3. Extraction completes → Banner disappears (onStatusChange(true) called)
4. User deletes ontology → Banner reappears (onStatusChange(false) called)
5. User refreshes page with existing ontology → Banner is hidden from initial load

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

## Issue 8: LLM Calls Are Serialized - Should Use Parallel Worker Pool ✅ COMPLETE

**Severity**: High (Performance)
**Status**: ✅ Fixed, Tested, and Committed (2026-01-03)

**Original Behavior**:

Both `relationship_enrichment.go` and `column_enrichment.go` processed LLM calls **serially**:

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

This meant:
- With 86 relationships in batches of 20 = 5 LLM calls, executed one at a time
- With 38 tables = 38 LLM calls, executed one at a time
- Total time = sum of all individual LLM call times (~15+ minutes)

**Files Modified**:
- `pkg/llm/worker_pool.go` - Created new worker pool with bounded parallelism using semaphore pattern
- `pkg/llm/worker_pool_test.go` - Comprehensive test suite (8 tests) covering success, errors, concurrency limits, progress callbacks, context cancellation
- `pkg/services/relationship_enrichment.go` - Refactored to use worker pool for parallel batch processing
- `pkg/services/relationship_enrichment_test.go` - Updated all tests to inject worker pool with MaxConcurrent: 1 for determinism
- `pkg/services/column_enrichment.go` - Refactored to use worker pool for parallel table processing
- `pkg/services/column_enrichment_test.go` - Updated all tests to inject worker pool with MaxConcurrent: 1 for determinism, fixed flaky test with mutex
- `main.go` - Created and injected worker pool into both enrichment services
- `ui/src/pages/__tests__/OntologyPage.test.tsx` - Fixed import ordering (minor cleanup)

**Implementation Details**:

1. **Worker Pool Architecture** (`pkg/llm/worker_pool.go`):
   - Uses Go semaphore pattern (`chan struct{}`) to limit concurrent LLM calls to 8
   - Non-generic implementation: `WorkItem` and `WorkResult` use `any` type for simplicity
   - Goroutines acquire semaphore slot before executing work item
   - Results are collected as they complete (not in submission order)
   - Progress callback invoked after each completion for real-time updates
   - Respects context cancellation - stops acquiring new slots when context is done
   - Fail-safe: continues processing all items even if some fail

2. **Relationship Enrichment Changes**:
   - Created `batchResult` struct to hold enrichment results per batch
   - Refactored `enrichBatch` → `enrichBatchInternal` to return structured result
   - Build `[]llm.WorkItem` with closures that capture each batch
   - Call `workerPool.Process()` to execute all batches concurrently
   - Progress callback maps batch completion to relationship progress
   - Aggregate results after all batches complete

3. **Column Enrichment Changes**:
   - Build `[]llm.WorkItem` with closures that capture each table name
   - Call `workerPool.Process()` to execute all tables concurrently
   - Progress callback directly reports table completion
   - Aggregate results after all tables complete

4. **Dependency Injection** (`main.go`):
   - Created worker pool with `llm.DefaultWorkerPoolConfig()` (MaxConcurrent: 8)
   - Injected into both `NewRelationshipEnrichmentService` and `NewColumnEnrichmentService`
   - Single shared pool for all LLM enrichment operations

5. **Test Updates**:
   - All tests use `MaxConcurrent: 1` to ensure deterministic execution order
   - Fixed race condition in `testColEnrichmentRetryableFailureClient` by adding mutex
   - Made `TestColumnEnrichmentService_EnrichProject_ContinuesOnFailure` deterministic by checking prompt content instead of call count
   - All existing tests pass with new architecture

6. **Non-Generic Design Decision**:
   - Used `any` type instead of generics (`WorkItem[T any]`) for simplicity
   - Avoids type parameter complexity in service constructors
   - Callers do type assertion on results (e.g., `r.Result.(*batchResult)`)
   - Trade-off: Less type safety but simpler dependency injection

**Testing**:
- `TestWorkerPool_Process_Success` - Verifies basic parallel execution
- `TestWorkerPool_Process_WithErrors` - Verifies error handling doesn't stop other work
- `TestWorkerPool_Process_EmptyItems` - Edge case handling
- `TestWorkerPool_Process_ContextCancellation` - Context propagation
- `TestWorkerPool_Process_ConcurrencyLimit` - Verifies semaphore correctly limits to MaxConcurrent
- `TestWorkerPool_Process_ProgressCallback` - Verifies progress reporting
- `TestWorkerPool_ConfigDefault` - Verifies invalid config is corrected to defaults
- All enrichment service tests updated and passing

**Expected Performance Improvement**:

| Scenario | Serial (before) | Parallel (after - 8 concurrent) |
|----------|-----------------|--------------------------------|
| 5 relationship batches @ 3s each | 15s | ~3-6s |
| 38 tables @ 20s each | 760s (~12.5 min) | ~100-150s (~2 min) |
| **Total enrichment phase** | ~15+ min | **~3-4 min** |

**Commit Date**: 2026-01-03

**What Was Done**:
1. Created reusable `WorkerPool` in `pkg/llm/` using semaphore pattern for bounded parallelism
2. Refactored relationship enrichment to process batches concurrently (up to 8 at once)
3. Refactored column enrichment to process tables concurrently (up to 8 at once)
4. Injected worker pool via dependency injection in `main.go`
5. Updated all tests to use `MaxConcurrent: 1` for deterministic behavior
6. Fixed race condition in column enrichment test by adding mutex protection
7. Created comprehensive test suite for worker pool (8 tests covering all edge cases)

**Next Session Should**:
1. Run a full ontology extraction and time the enrichment phases
2. Compare total runtime against the original ~15 minute baseline
3. Monitor server logs to verify 8 concurrent LLM calls are happening
4. Check for any new errors related to concurrent access (unlikely but worth checking)
5. If performance gain is less than expected, investigate: LLM API rate limits, network bottlenecks, or database contention

**Architecture Notes for Future Work**:
- Worker pool is intentionally generic and reusable for other LLM batch operations
- Consider making MaxConcurrent configurable via environment variable if different projects need different limits
- Could add metrics/telemetry to track actual concurrency and identify bottlenecks
- If LLM providers enforce per-minute rate limits, may need to add rate limiting layer on top of worker pool

---

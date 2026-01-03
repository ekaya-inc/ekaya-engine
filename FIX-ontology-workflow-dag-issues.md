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

## Issue 2: High Failure Rate in RelationshipEnrichment (78%)

**Severity**: High

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

**Possible Causes**:
1. LLM rate limiting or timeouts
2. Invalid relationship data causing LLM parsing failures
3. Context window limits being exceeded for certain relationships
4. Missing or malformed entity data that relationships reference

**Files to Investigate**:
- `pkg/services/dag/relationship_enrichment_node.go` - Check error handling and logging
- Check server logs for specific error messages during enrichment

**How to Fix**:
1. Add detailed error logging for each failed relationship
2. Implement retry logic with exponential backoff for transient failures
3. Consider batching relationships to avoid rate limits
4. Add validation before LLM calls to catch malformed data early

---

## Issue 3: Moderate Failure Rate in ColumnEnrichment (37%)

**Severity**: Moderate

**Observed Behavior**:
- 14 out of 38 tables failed to enrich (37% failure rate)
- Final message: "Enriched 24 tables (14 failed)"

**Possible Causes**:
1. Similar to relationship enrichment - LLM issues
2. Tables with many columns exceeding context limits
3. Specific table structures causing parsing issues

**Files to Investigate**:
- `pkg/services/dag/column_enrichment_node.go` - Check error handling
- Check if failed tables have common characteristics (many columns, special characters, etc.)

**How to Fix**:
1. Add per-table error logging to identify patterns
2. Implement chunking for tables with many columns
3. Add retry logic for transient failures
4. Consider graceful degradation - partial enrichment instead of full failure

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

## Issue 4: "Refresh Ontology" Runs Full Extraction Instead of Incremental Update

**Severity**: High

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

**How to Fix**:
1. Rename button to "Re-extract Ontology" if full extraction is intended
2. Better: Implement true incremental refresh:
   - Compare current schema fingerprint with stored fingerprint
   - Identify delta (added/removed/modified tables/columns)
   - Run only affected nodes with scope limited to changed entities
3. Add confirmation dialog for full re-extraction

---

## Issue 5: Missing "Delete Ontology" Functionality

**Severity**: Moderate

**Observed Behavior**:
- No way to delete an existing ontology from the UI
- User cannot start fresh without manual database intervention

**Expected Behavior**:
- A "Delete Ontology" button should exist
- Must have a serious confirmation dialog requiring the user to type "delete ontology"
- This is a destructive action and should be treated as such

**How to Fix**:
1. Add "Delete Ontology" button (perhaps in a dropdown menu or settings area)
2. Implement confirmation modal:
   - Warning text explaining data loss
   - Text input requiring exact string "delete ontology"
   - Button disabled until confirmation text matches
3. Backend: `DELETE /api/projects/{pid}/datasources/{did}/ontology`

---

## Issue 6: Canceled Workflow Shows "Running" Status for In-Progress Node

**Severity**: High

**Observed Behavior**:
- User cancels workflow while Entity Enrichment is running
- Overall status correctly shows "Extraction failed"
- BUT Entity Enrichment node still shows:
  - Spinner animation
  - "Running" status badge
  - "0/100" progress

**Expected Behavior**:
- When workflow is canceled, ALL nodes should update their state:
  - Running nodes → "Canceled" status
  - Pending nodes → remain "Pending" or show "Canceled"
  - Spinner should stop
  - Status badge should show "Canceled" (not "Running")

**Files to Investigate**:
- `ui/src/components/ontology/OntologyDAG.tsx` - Check how cancel propagates to node states
- `pkg/services/dag/orchestrator.go` - Ensure cancel updates all node states in DB
- `pkg/handlers/ontology_dag_handler.go` - Cancel endpoint implementation

**How to Fix**:
1. Backend: When workflow is canceled, update all non-completed nodes to "canceled" status
2. Frontend: Poll should pick up the canceled state and update UI accordingly
3. Ensure spinner component checks for canceled state

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

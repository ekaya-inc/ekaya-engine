# PLAN: Move Glossary Generation After Ontology Questions Answered

**Status:** DRAFT
**Branch:** `ddanieli/move-glossary-after-ontology-questions-answered`

## Problem

Glossary discovery and enrichment currently run as DAG nodes 8-9, immediately after OntologyFinalization (node 7). The DAG does not pause for ontology questions to be answered. This means glossary terms are generated using an incomplete ontology — the user hasn't had a chance to answer clarifying questions that provide critical business context. The result is poor glossary quality that reflects badly on the product.

## Solution

Split glossary generation out of the main DAG and make it a user-triggered action on the Glossary page, gated on ontology questions being answered first.

**Key design decisions:**
1. The main DAG ends at OntologyFinalization (node 7) — glossary nodes are removed from the DAG
2. The Glossary page gets an "Auto-Generate Terms" button that calls the existing glossary service methods
3. The button is disabled with a clear message if required ontology questions are still pending
4. Generation runs asynchronously (goroutine) with status tracking so the UI can show progress
5. Existing `GlossaryService.DiscoverGlossaryTerms()` and `EnrichGlossaryTerms()` are reused unchanged

## Architecture

```
BEFORE:
  DAG: [1..7] → GlossaryDiscovery(8) → GlossaryEnrichment(9) → Complete
  Questions: answered independently, no impact on glossary

AFTER:
  DAG: [1..7] → Complete
  User answers questions on Ontology page
  User navigates to Glossary page
  Glossary page: checks question status → shows "Auto-Generate" button → runs glossary discovery+enrichment
```

## Implementation

### Task 1: Remove Glossary Nodes from Main DAG

**Files:**
- `pkg/models/ontology_dag.go` — Remove `GlossaryDiscovery` and `GlossaryEnrichment` from `AllDAGNodes()` and `DAGNodeOrder`
- Keep the constants (`DAGNodeGlossaryDiscovery`, `DAGNodeGlossaryEnrichment`) — they'll be referenced by the new auto-generate code
- Keep the node executor factory cases in `ontology_dag_service.go` — they'll be used by the new auto-generate flow

**Tests to update:**
- Any tests that assert on `AllDAGNodes()` length or content
- Any tests that assert DAG completion includes glossary nodes

### Task 2: Add Glossary Generation Status Tracking

**File:** `pkg/models/glossary.go`

Add a lightweight generation status model:

```go
type GlossaryGenerationStatus struct {
    Status    string    // "idle", "discovering", "enriching", "completed", "failed"
    Message   string    // Human-readable progress message
    Error     string    // Error message if failed
    StartedAt *time.Time
}
```

**File:** `pkg/services/glossary_service.go`

Add in-memory generation status tracking (per-project, using sync.Map):

```go
// Track generation status per project
var generationStatus sync.Map // map[uuid.UUID]*GlossaryGenerationStatus
```

Add methods:
- `GetGenerationStatus(projectID) *GlossaryGenerationStatus`
- `RunAutoGenerate(ctx, projectID, ontologyID) error` — async wrapper that:
  1. Sets status to "discovering"
  2. Calls `DiscoverGlossaryTerms()`
  3. Sets status to "enriching"
  4. Calls `EnrichGlossaryTerms()`
  5. Sets status to "completed" or "failed"

### Task 3: Add Auto-Generate Endpoint

**File:** `pkg/handlers/glossary_handler.go`

New endpoint: `POST /api/projects/{pid}/glossary/auto-generate`

Logic:
1. Check pending required questions via `questionService.GetPendingCounts()` → return error with count if required > 0
2. Check if generation is already running → return error if so
3. Get active ontology → return error if none exists
4. Kick off `glossaryService.RunAutoGenerate()` in a goroutine
5. Return 202 Accepted with initial status

**File:** `pkg/handlers/glossary_handler.go`

Update existing `List` endpoint to include generation status in response:
- Add `generation_status` field to `GlossaryListResponse`

**File:** `pkg/server/routes.go` (or wherever routes are registered)
- Register the new endpoint

### Task 4: Wire Up Dependencies

**File:** `pkg/handlers/glossary_handler.go`

The GlossaryHandler needs access to `OntologyQuestionService` to check pending question counts. Add it to the handler struct and constructor.

**File:** Where handlers are constructed (likely `pkg/server/` or `main.go`)
- Pass `questionService` to GlossaryHandler

### Task 5: Update Glossary Page UI

**File:** `ui/src/pages/GlossaryPage.tsx`

Changes to the empty state (no terms):
1. When ontology is complete but no terms exist:
   - Fetch pending question counts via `GET /api/projects/{pid}/ontology/questions/next` (existing endpoint returns counts)
   - If required questions > 0: Show "Answer N required questions before generating glossary terms" with link to ontology questions page
   - If no required questions pending: Show "Auto-Generate Terms" button
2. "Auto-Generate Terms" button:
   - Calls `POST /api/projects/{pid}/glossary/auto-generate`
   - Shows spinner/progress while generating
   - Polls `GET /api/projects/{pid}/glossary` for updated terms list and generation status
   - On completion, displays the generated terms

Changes to the terms view (terms exist):
- Add a "Regenerate" button (with confirmation) that deletes inferred terms and re-runs auto-generate
- Same question-gating logic applies

**File:** `ui/src/services/engineApi.ts`
- Add `autoGenerateGlossary(projectId)` method
- Add question counts fetching method if not already available

### Task 6: Update Tests

- Update DAG integration tests that expect glossary nodes in the DAG
- Add unit tests for the auto-generate endpoint (question gating, async execution)
- Add unit tests for generation status tracking

## Checklist

- [ ] Task 1: Remove glossary nodes from main DAG (`AllDAGNodes`, `DAGNodeOrder`)
- [ ] Task 2: Add glossary generation status tracking to `GlossaryService`
- [ ] Task 3: Add `POST /api/projects/{pid}/glossary/auto-generate` endpoint
- [ ] Task 4: Wire `OntologyQuestionService` into `GlossaryHandler`
- [ ] Task 5: Update `GlossaryPage.tsx` with question-gating and auto-generate button
- [ ] Task 6: Update tests (DAG tests, new endpoint tests)

## Notes

- Optional questions do NOT block glossary generation — only required questions do
- The in-memory generation status is acceptable because: (a) this is a single-instance service, (b) if the server restarts mid-generation the user can just re-trigger it, (c) keeping it simple avoids another DB table
- The existing `Suggest` endpoint remains unchanged — it serves a different purpose (manual suggestions without saving)
- Glossary terms are linked to ontology via `ontology_id` with CASCADE delete, so deleting/re-extracting the ontology will clean up glossary terms automatically

# Plan: Add "Critical Ontology Questions answered" to MCP Server Checklist

**Status:** DONE

## Context

The MCP Server Setup Checklist currently has 4 items (datasource configured, schema selected, AI configured, ontology extracted). After ontology extraction, the system generates ontology questions — some marked as `is_required=true` (critical). These critical questions must be answered before the ontology is considered production-ready. Adding a 5th checklist item makes this visible to the user.

The item should show as complete when there are zero pending required ontology questions.

## Changes

### 1. Backend: Add counts endpoint to existing handler

**File:** `pkg/handlers/ontology_questions.go`

- Register new route: `GET /api/projects/{pid}/ontology/questions/counts` in `RegisterRoutes()`
- Add `Counts` handler method that calls `h.questionService.GetPendingCounts()` and returns `{required, optional}`
- Reuse existing `QuestionCountsResponse` type (already defined in the file at the response types section)

**Existing code to reuse:**
- `repositories.QuestionCounts` struct (`pkg/repositories/ontology_question_repository.go:16-20`) — has `Required` and `Optional` int fields
- `OntologyQuestionService.GetPendingCounts()` (`pkg/services/ontology_question.go`) — already exists, calls the repository
- `QuestionCountsResponse` response type (`pkg/handlers/ontology_questions.go`) — already maps counts to JSON

### 2. Frontend: Add API method

**File:** `ui/src/services/engineApi.ts`

- Add `getOntologyQuestionCounts(projectId: string)` method returning `ApiResponse<{required: number, optional: number}>`
- Calls `GET /api/projects/{projectId}/ontology/questions/counts`
- Follow the same pattern as existing methods like `getOntologyDAGStatus()`

### 3. Frontend: Add checklist item #5

**File:** `ui/src/pages/MCPServerPage.tsx`

- Add `questionCounts` state: `useState<{required: number, optional: number} | null>(null)`
- Fetch counts in `fetchConfig()` after datasource is loaded (same block as DAG status fetch)
- Add item 5 to `getChecklistItems()` after the ontology item:
  - **id:** `'questions'`
  - **title:** `'Critical Ontology Questions answered'`
  - **Complete when:** `questionCounts !== null && questionCounts.required === 0`
  - **Description when complete:** `'All critical questions about your schema have been answered'`
  - **Description when pending:** `'N critical questions need answers'` (using actual count)
  - **Link:** `/projects/${pid}/ontology-questions`
  - **linkText:** `'Manage'` when complete, `'Answer'` when pending

## Verification

- [ ] `make check` passes (lint, typecheck, unit + integration tests)
- [ ] Navigate to MCP Server page — 5th checklist item appears
- [ ] With pending required questions → shows pending status with count
- [ ] With no pending required questions → shows complete (green check)
- [ ] Checklist header shows "MCP Server is ready" only when all 5 items are complete

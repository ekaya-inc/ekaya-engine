# FIX: Pending Queries tile missing badge count on Project screen

**Status:** Open
**Date:** 2026-03-12

## Problem

The "Pending Queries" tile on the Project dashboard (`ProjectDashboard.tsx`) does not show a badge count when there are pending query suggestions awaiting review. Users have no visual cue that items need attention without clicking into the tile.

## Root Cause

The dashboard already implements a badge pattern for Ontology Questions (amber badge with count), but the same pattern was never applied to the Pending Queries tile. All the backend and frontend plumbing already exists — the tile just doesn't fetch or display the count.

## Existing Infrastructure

- **Backend endpoint:** `GET /api/projects/{projectId}/queries/pending` returns `{ queries: [...], count: N }` — count is already available
- **Frontend API client:** `engineApi.listPendingQueries(projectId)` in `ui/src/services/engineApi.ts` (lines 517-523)
- **Response type:** `ListPendingQueriesResponse` in `ui/src/types/query.ts` includes `count: number`
- **Badge pattern:** Already implemented for Ontology Questions tile in `ui/src/pages/ProjectDashboard.tsx` (lines 326-331) using amber badge with icon and count

## Fix

In `ui/src/pages/ProjectDashboard.tsx`:

- [ ] Add state for pending query count (e.g. `useState` for `pendingQueryCount`)
- [ ] Add `useEffect` to fetch `engineApi.listPendingQueries(pid)` and extract `count` (similar to how ontology questions count is fetched)
- [ ] Render a badge on the Pending Queries tile when `pendingQueryCount > 0`, following the same pattern as the Ontology Questions badge (lines 326-331)
- [ ] Verify badge appears when pending suggestions exist and disappears when all are processed

# PLAN: Unified Project Deletion (ekaya-engine)

## Problem

Currently, there are two separate "Delete Project" options:
1. **ekaya-engine** (`/projects/{pid}/settings`) - Deletes the project from ekaya-engine only
2. **ekaya-central** (`/projects/{projectId}`) - Deletes the project from ekaya-central only

These are disconnected, leading to orphaned data when one is deleted without the other.

## Solution

Make ekaya-engine the **single point of deletion** for projects:

1. ekaya-central's "Delete Project" button redirects to ekaya-engine's delete page
2. ekaya-engine handles the full deletion workflow:
   - First calls ekaya-central's API to delete the project there
   - If successful, deletes the project from ekaya-engine
   - Redirects user back to ekaya-central's projects list
3. ekaya-central's delete endpoint only accepts requests with Admin JWT (service-to-service)

## Implementation Tasks

### Task 1: Create Dedicated Delete Project Page (UI)

**File:** `ui/src/pages/DeleteProjectPage.tsx`

Create a new standalone page at route `/projects/{pid}/delete`:
- Full-page design (not a modal) with clear warnings
- Display project name prominently
- Explain what will be deleted (both ekaya-engine and ekaya-central data)
- Require typing "delete project" to confirm (consistent with ontology deletion)
- Show progress states: confirming → deleting from central → deleting from engine → redirecting
- Handle errors gracefully with retry option
- Back button returns to settings page

### Task 2: Add Route for Delete Page

**File:** `ui/src/App.tsx`

Add route: `<Route path="/projects/:pid/delete" element={<DeleteProjectPage />} />`

### Task 3: Update Settings Page

**File:** `ui/src/pages/SettingsPage.tsx`

- Remove the delete modal dialog
- Change "Delete Project" button to navigate to `/projects/{pid}/delete`

### Task 4: Create Backend Endpoint to Orchestrate Deletion

**File:** `pkg/handlers/projects.go`

Modify `Delete` handler or create new endpoint `DELETE /api/projects/{pid}/full-delete`:
- Accept optional `central_delete_token` in request body or header
- If ekaya-central PAPI URL is configured for the project:
  1. Call ekaya-central's `DELETE /projects/{projectId}` with admin JWT
  2. If ekaya-central deletion fails, return error (do not proceed)
  3. If ekaya-central deletion succeeds, proceed to delete from ekaya-engine
- If no PAPI URL (standalone mode), just delete from ekaya-engine
- Return 204 on success

### Task 5: Create ekaya-central API Client

**File:** `pkg/services/central_client.go` (new)

Service to call ekaya-central APIs:
- `DeleteProject(ctx, projectID, adminToken) error`
- Use PAPI URL from project parameters
- Handle error responses appropriately

### Task 6: Update Frontend API Service

**File:** `ui/src/services/engineApi.ts`

Add method to call the new orchestrated delete endpoint.

## Deletion Flow

```
User clicks "Delete Project" on ekaya-engine settings
         │
         ▼
Navigates to /projects/{pid}/delete
         │
         ▼
User types "delete project" and confirms
         │
         ▼
Frontend calls DELETE /api/projects/{pid}
         │
         ▼
Backend checks for PAPI URL in project params
         │
    ┌────┴────┐
    │         │
    ▼         ▼
Has PAPI   No PAPI (standalone)
    │              │
    ▼              │
Call ekaya-central │
DELETE /projects   │
    │              │
    ▼              │
Success?           │
    │              │
  ┌─┴─┐            │
  │   │            │
  ▼   ▼            │
 Yes  No──────────►Return error
  │
  ▼
Delete from ekaya-engine
         │
         ▼
Return 204 + redirect info
         │
         ▼
Frontend redirects to ekaya-central projects page
```

## Error Handling

- **ekaya-central unreachable:** Show error, suggest retry later
- **ekaya-central returns 403:** Show "unauthorized" error
- **ekaya-central returns 404:** Project already deleted there, proceed with engine deletion
- **ekaya-engine deletion fails:** Show error (central already deleted - mention this)

## Dependencies

- Requires corresponding changes in ekaya-central (see `../ekaya-central/plans/PLAN-delete-project.md`)
- ekaya-central must accept admin JWT for deletion

## Testing

- Unit tests for central client
- Integration test for full deletion flow
- Test standalone mode (no PAPI URL)
- Test error scenarios (central unreachable, 403, 404)

# ISSUE: Engine delete-callback redirects to `/` instead of Central's Projects screen

**Repo:** ekaya-engine
**Status:** FIXED
**Severity:** Low (cosmetic/UX)
**Found during:** Manual testing of delete-project redirect flow

## Observed Behavior

After a successful project deletion via the central redirect flow, ekaya-engine's delete-callback handler redirects the user to `/` (engine's Sign In page). The user has no project context at this point and sees a dead-end Sign In screen.

## Expected Behavior

Engine should redirect to ekaya-central's Projects screen (`{centralUrl}/projects`) so the user lands somewhere useful — their project list.

## Steps to Reproduce

1. Create and provision a project
2. On ekaya-engine Settings page, click Delete Project
3. Engine redirects to central's `ProjectDeleteConfirmPage`
4. Type "delete project" and confirm
5. Central deletes project, redirects browser to engine's callbackUrl
6. Engine completes local deletion
7. **Engine redirects to `/`** — should redirect to central's `/projects` instead

## Relevant Logs

```
Engine callback completed:
  "Project deleted via callback" {"project_id": "ff7c91f1-..."}

Then engine redirects to:
  GET / → 304 (engine Sign In page)
```

## Fix

In engine's delete-callback handler, after successful deletion, redirect to `{centralUrl}/projects` instead of `/`. The central URL is already available in engine's configuration.

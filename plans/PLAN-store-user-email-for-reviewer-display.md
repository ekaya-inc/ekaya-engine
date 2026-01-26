# PLAN: Store User Email for Reviewer Display

**STATUS: COMPLETED**

## Problem

When a query is approved or rejected, the UI shows:
```
Rejected by decf1bc8-5ebc-426b-b649-df5f32a48104 on 1/26/2026
```

This UUID is meaningless to users. We need to display the reviewer's email address instead.

## Current State

1. **JWT Claims** (`pkg/auth/claims.go`): Already contains `Email` field from ekaya-central
2. **engine_users table**: Only stores `user_id` (UUID), `project_id`, and `role` - **no email column**
3. **Query approval/rejection**: Stores `reviewed_by` as the user's UUID
4. **Frontend**: Already displays `reviewed_by` - will show email if backend returns it

## Solution

Store the user's email in the `engine_users` table (populated from JWT on authentication), then use the email when recording who approved/rejected a query.

### Design Decision

The email comes from the JWT. If we later remove email from the JWT, we can fetch it from ekaya-central using the user_id. This is acceptable because:
- Email is currently in the JWT and readily available
- We're storing user_id as well, so we can always look up the email later
- This avoids an extra API call to ekaya-central on every request

## Implementation Tasks

### Task 1: Add email column to engine_users ✓

**Files:**
- `migrations/022_add_user_email.up.sql`
- `migrations/022_add_user_email.down.sql`

### Task 2: Update User model and repository ✓

**Files:**
- `pkg/models/user.go` - Added `Email *string` field
- `pkg/repositories/user_repository.go` - Updated Add, GetByProject, GetByID queries

### Task 3: Update user upsert logic to store email from JWT ✓

**Files:**
- `pkg/services/projects.go` - EnsureProject now stores email from JWT claims
- `pkg/auth/context.go` - Added `GetEmailFromContext()` helper

### Task 4: Store email in reviewed_by instead of UUID ✓

**Files:**
- `pkg/handlers/queries.go` - Approve and Reject handlers now use `auth.GetEmailFromContext()`

### Task 5: No frontend changes needed ✓

The frontend already displays `reviewed_by` directly.

## Notes

- Existing `reviewed_by` values remain as UUIDs (historical data)
- New approvals/rejections use email
- No index on email column (not needed for lookups, just for display)

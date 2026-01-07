# FIX: Project Deletion Flow

## Problem

When a project is deleted from ekaya-engine, ekaya-central is not notified. This leaves ekaya-central in an inconsistent state where it still thinks the project is provisioned.

## Current State

### ekaya-engine
- Project settings page has a delete action that removes the project from Postgres
- No notification is sent to ekaya-central

### ekaya-central
- No DELETE route exists for ekaya-engine to call
- Unaware when projects are deleted from ekaya-engine

## Required Changes

### ekaya-engine

1. After deleting the project from Postgres, call HTTP DELETE on ekaya-central's resource endpoint
2. Redirect user to the ekaya-central project page after successful deletion

### ekaya-central

1. Add a DELETE route for ekaya-engine to notify deletion
2. Handle the deletion notification:
   - Do NOT automatically delete the project from ekaya-central
   - Return project to pre-provisioning state
   - TBD: Should we prompt the user about what to do with the project? Options might include:
     - Re-provision to ekaya-engine
     - Delete from ekaya-central entirely
     - Keep in pre-provisioned state

## Open Questions

- What should the user experience be on the ekaya-central side when a project is "de-provisioned"?
- Should there be a grace period or confirmation flow?

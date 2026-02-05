# DESIGN: Pending Changes Workflow

**Status:** DRAFT
**Author:** Damon Danieli
**Created:** 2026-02-05

## Overview

The ontology extraction pipeline detects data patterns (enum values, FK relationships) during initial extraction. Over time, customer databases evolve—new enum values appear, new tables are added, relationships emerge. The **pending changes workflow** detects these gaps and surfaces them for admin review.

## Current State

Infrastructure exists but is incomplete:
- `engine_ontology_pending_changes` table stores detected changes
- `scan_data_changes` MCP tool triggers detection
- `list_pending_changes`, `approve_change`, `reject_change` MCP tools exist
- `applyUpdateColumnMetadata` is a TODO stub (changes cannot actually be applied)
- No UI exists

## Proposed Workflow

### UI Trigger: Schema Refresh

When an admin clicks **Refresh Schema** on the Schema screen:
1. Schema refresh runs (existing behavior)
2. After schema refresh completes, automatically run `scan_data_changes` for all selected tables
3. Pending changes are written to `engine_ontology_pending_changes`

### UI: Badge Notification

The **Ontology Extraction** tile on the project dashboard displays a badge with the count of pending changes when count > 0.

### UI: Review Screen

Opening the Ontology Extraction screen shows a **Pending Changes** section (or tab) where the admin can:
- See list of pending changes with change type, table, column, old/new values
- Approve individual changes (applies to ontology)
- Reject individual changes (dismisses without applying)
- Bulk approve/reject

### Detection Types

| Change Type | Trigger | Suggested Action |
|-------------|---------|------------------|
| `new_enum_value` | Column has values not in ontology | Update column metadata with new enum values |
| `new_fk_pattern` | Column values match another table's PK | Create relationship |

## MCP Server Considerations

**⚠️ DISCUSS WITH DAMON DANIELI**

When triggered from MCP Server (Claude Code, AI Data Liaison, etc.), the workflow is different:
- The MCP client is acting as a Data Engineer
- May want to trigger full ontology re-extraction rather than incremental approval
- May want autonomous approval vs. queuing for human review
- Precedence rules (Admin > MCP > Inference) affect what MCP can approve

Questions to resolve:
1. Should MCP-triggered scans auto-apply changes or queue them?
2. Should there be an MCP tool to trigger full ontology re-extraction?
3. How do we handle the case where MCP detects a change but lacks precedence to apply it?

## Out of Scope (Future)

- Scheduled/automatic scanning (cron-style)
- Schema change detection (new tables/columns) - currently separate from data change detection
- Anomaly detection (data quality issues, outliers)
- Notifications (email, Slack) for pending changes

## Dependencies

Before implementing:
- [ ] Complete `applyUpdateColumnMetadata` in `change_review_service.go`
- [ ] Complete `applyCreateColumnMetadata` in `change_review_service.go`

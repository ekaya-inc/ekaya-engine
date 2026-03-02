# ISSUE: Ontology extraction misclassifies timestamp columns

**Date:** 2026-03-02
**Status:** TODO
**Priority:** HIGH
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

The ontology extraction incorrectly classifies several types of timestamp columns:

### 1. Nullable event timestamps → classified as `soft_delete`

Nullable `timestamp with time zone` columns that represent lifecycle events are classified as `semantic_type: "soft_delete"` with descriptions like "Records when the row was logically deleted."

| Table | Column | Actual meaning | Inferred semantic_type |
|-------|--------|---------------|----------------------|
| directory_submissions | submitted_at | When submission was sent | `soft_delete` |
| directory_submissions | approved_at | When submission was approved | `soft_delete` |

**Root cause hypothesis:** The extraction sees a nullable timestamp column and assumes it's a soft-delete marker (`deleted_at` pattern). But many nullable timestamps represent events that haven't happened yet (e.g., `submitted_at` is NULL because the submission hasn't been sent yet).

### 2. Date/timestamp columns → classified as `audit_created`

Non-audit date columns are classified as `semantic_type: "audit_created"`:

| Table | Column | Actual meaning | Inferred semantic_type |
|-------|--------|---------------|----------------------|
| paid_placements | start_date | When placement begins running | `audit_created` |
| content_posts | target_date | Scheduled publish date | `audit_created` |
| lead_magnet_leads | followed_up_at | When follow-up was sent | `audit_created` |
| lead_magnet_leads | converted_at | When lead converted | `audit_created` |
| lead_magnet_leads | commented_at | When lead first commented | `audit_created` |

**Root cause hypothesis:** The extraction defaults any timestamp column to `audit_created` unless it matches known patterns like `updated_at`. Column names like `start_date`, `target_date`, `followed_up_at` don't match the `created_at` / `_created` pattern but still get classified that way.

## Impact

**Critical for query generation:**
- An AI agent seeing `soft_delete` would add `WHERE submitted_at IS NULL` to filter "active" records — this would exclude all submitted records, completely inverting the intended query
- An AI agent seeing `audit_created` on `start_date` would not use it for date range filtering

## Suggested Fix

Improve the timestamp classification heuristic:

1. **`soft_delete` should require** the column name to match patterns like `deleted_at`, `removed_at`, `archived_at`, `purged_at` — not just "nullable timestamp"
2. **`audit_created` should require** the column name to match patterns like `created_at`, `created_on`, `date_created`, `creation_time` — not be the default for all timestamps
3. **Default for unrecognized timestamps** should be `event_time` (safe neutral classification)
4. **Date columns** (not timestamps) like `start_date`, `end_date`, `target_date` should default to `scheduled_time` or `event_time`, never `audit_created`

## Files to Investigate

| File | What to check |
|------|---------------|
| Ontology extraction / column classifier | Timestamp classification logic — where does it decide soft_delete vs event_time vs audit_created? |
| Semantic type constants | What semantic_type values are defined and what are their classification rules? |

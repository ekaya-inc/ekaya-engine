# ISSUE: update_column role parameter rejects "foreign_key" value

**Date:** 2026-03-02
**Status:** TODO
**Priority:** LOW
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

The `update_column` MCP tool's `role` parameter only accepts: `dimension`, `measure`, `identifier`, `attribute`. Attempting to set `role: "foreign_key"` returns:

```
parameter 'role' must be one of: dimension, measure, identifier, attribute. Got: "foreign_key"
```

However, the ontology extraction can set columns to `role: "foreign_key"` with `is_foreign_key: true` and `foreign_table` metadata. This means FK classification can only be set by the extraction, never corrected by an MCP client.

### Examples from ekaya_marketing

| Table | Column | Current role | Relationship graph | Issue |
|-------|--------|-------------|-------------------|-------|
| lead_magnet_leads | post_id | `attribute` | Correctly shows FK to content_posts | Column metadata doesn't match |
| paid_placements | channel_id | `attribute` | Correctly shows FK to paid_channels | Column metadata doesn't match |
| paid_placements | task_id | `attribute` | Correctly shows FK to marketing_tasks | Column metadata doesn't match |

The relationship graph (from `get_schema`) correctly identifies these as FKs with N:1 cardinality. But the column-level metadata from `get_ontology` shows `role: "attribute"` with `is_foreign_key: false`.

## Expected Behavior

Either:
1. Add `"foreign_key"` to the allowed values for `role` in `update_column`, or
2. Add separate `is_foreign_key` and `foreign_table` parameters to `update_column`

## Impact

Low — the relationship graph is the primary source for join information. But inconsistency between column metadata and the relationship graph could confuse AI agents that check both.

## Files to Investigate

| File | What to check |
|------|---------------|
| MCP tool handler for `update_column` | Validation logic for `role` parameter |
| Column metadata model | How `is_foreign_key` and `foreign_table` are stored vs `role` |

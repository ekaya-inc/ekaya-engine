# ISSUE: update_column MCP tool doesn't expose semantic_type parameter

**Date:** 2026-03-02
**Status:** TODO
**Priority:** MEDIUM
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

The `update_column` MCP tool accepts `description`, `enum_values`, `entity`, `role`, and `sensitive` — but not `semantic_type`. The ontology extraction infers semantic_type for every column, and frequently gets it wrong. There is no way for an MCP client to correct it.

### Examples from ekaya_marketing

| Table | Column | Inferred semantic_type | Correct semantic_type |
|-------|--------|----------------------|----------------------|
| directory_submissions | submitted_at | `soft_delete` | `event_time` |
| directory_submissions | approved_at | `soft_delete` → fixed to `event_time` by re-extract, but submitted_at still wrong | `event_time` |
| paid_placements | start_date | `audit_created` | `event_time` |
| content_posts | target_date | `audit_created` | `scheduled_time` |
| lead_magnet_leads | followed_up_at | `audit_created` | `event_time` |
| lead_magnet_leads | converted_at | `audit_created` | `event_time` |

The MCP client updated descriptions to be correct (e.g., "Timestamp when the submission was sent to the directory"), but the semantic_type tag remains wrong. An AI agent reading the ontology would see conflicting signals — a correct description but a wrong type classification.

## Expected Behavior

`update_column` should accept an optional `semantic_type` parameter that overrides the inferred value, with MCP provenance (higher precedence than inference).

## Impact

- AI agents may filter out event timestamps if they see `soft_delete` semantic_type
- Query generation may treat `start_date` as a creation timestamp instead of a date range boundary
- The mismatch between description and semantic_type creates ambiguity

## Files to Investigate

| File | What to check |
|------|---------------|
| MCP tool handler for `update_column` | Add `semantic_type` to accepted parameters |
| Column metadata update service | Persist semantic_type with MCP provenance |
| Ontology precedence logic | Ensure MCP-set semantic_type isn't overwritten by re-extraction |

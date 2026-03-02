# ISSUE: Ontology extraction doesn't read Postgres enum types from pg_enum

**Date:** 2026-03-02
**Status:** TODO
**Priority:** HIGH
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

The ekaya_marketing database has 10 Postgres enum types defined via `CREATE TYPE ... AS ENUM`. The ontology extraction does not read these — instead it guesses enum values, producing wrong results.

### Actual enum types (from `pg_enum`)

```
app_billing_type:    free, freemium, paid
app_status:          shipped, hidden, planning
channel_step_status: pending, in_progress, done, skipped
lead_status:         commented, dm_sent, followed_up, converted, unresponsive
placement_status:    planned, booked, live, completed, cancelled
post_priority:       P0, P1, P2
post_status:         planning, drafting, content_ready, posted, boosted
submission_method:   github_pr, web_form, google_form, contact_form, publish_flow
submission_status:   not_submitted, submitted, approved, listed, rejected, blocked
task_status:         not_started, in_progress, done, blocked
task_type:           free, paid
```

### What the extraction produced (examples)

| Column | Actual enum values | Extraction guessed |
|--------|-------------------|-------------------|
| directory_submissions.status | not_submitted, submitted, approved, listed, rejected, blocked | 1=Started, 2=In Progress, 3=Completed, 4=Rejected, 5=Pending Review |
| lead_magnet_leads.status | commented, dm_sent, followed_up, converted, unresponsive | 1=Started, 2=In Progress, 3=Converted, 4=Not Converted |
| post_channel_steps.status | pending, in_progress, done, skipped | 1=Started, 2=In Progress, 3=Completed, 4=Archived |
| content_posts.priority | P0, P1, P2 | low, medium, high |
| content_posts.status | planning, drafting, content_ready, posted, boosted | draft, scheduled, published, archived |

The guessed values are completely wrong — different names, different count, numbered instead of string values.

## Expected Behavior

During schema discovery or ontology extraction, the engine should query actual Postgres enum definitions:

```sql
SELECT t.typname AS enum_name, e.enumlabel AS enum_value, e.enumsortorder
FROM pg_type t
JOIN pg_enum e ON t.oid = e.enumtypid
ORDER BY t.typname, e.enumsortorder
```

Then for any column with `data_type = 'USER-DEFINED'`, look up the column's UDT name and populate enum_values from the actual type definition.

## Impact

- AI agents generate queries with wrong filter values (e.g., `WHERE status = 'Started'` instead of `WHERE status = 'not_submitted'`)
- Every ontology extraction requires manual correction of all 10+ enum columns via MCP update_column calls
- The MCP-set corrections get overwritten on re-extraction (no precedence protection)

## Additional Context

An MCP client (Claude Code) had to manually correct all enum values via `update_column` after each extraction. This took ~10 tool calls per extraction cycle. Reading pg_enum would eliminate this entirely.

## MSSQL Equivalent

For SQL Server, the equivalent would be reading CHECK constraints or looking up values from reference/lookup tables. This issue focuses on PostgreSQL but the same pattern applies.

## Files to Investigate

| File | What to check |
|------|---------------|
| `pkg/adapters/datasource/postgres/schema.go` | Schema discovery — does it query pg_enum? |
| Ontology extraction pipeline | Where enum values are inferred vs discovered |
| Column metadata persistence | Whether discovered enum values would be stored with higher precedence than inferred |

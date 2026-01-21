# FIX: BUG-11 - Wrong Project Knowledge Content

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-11
**Severity:** Medium
**Category:** Knowledge Capture
**Related:** BUG-10 (Stale Data Not Cleaned)

## Problem Summary

Project knowledge facts exist but are from a prior datasource (claude_cowork), not the current Tikr datasource.

**Current (WRONG) Knowledge:**
```
| fact_type   | key |
|-------------|-----|
| terminology | This database is shared across Claude Chat, Claude Code... |
| terminology | This database stores conversation continuity data... |
| terminology | The claude_cowork database is designed for cross-product... |
| convention  | Entries use source field to track origin... |
```

**Expected (CORRECT) Knowledge for Tikr:**
```
| fact_type     | key |
|---------------|-----|
| convention    | All amounts in cents (USD) |
| terminology   | Tik = 6 seconds of engagement |
| business_rule | MinCaptureAmount = 100 cents ($1.00) |
| business_rule | MinPayoutAmount = 2000 cents ($20.00) |
| business_rule | Platform fees = 4.5% |
| business_rule | Tikr share = 30% after platform fees |
| convention    | deleted_at indicates soft delete |
| convention    | accounts.time_zone stores user timezone |
| convention    | Entity IDs are UUIDs stored as text |
```

## Root Cause

This is a **direct consequence of BUG-10**:

1. Old datasource (claude_cowork) had knowledge facts stored
2. Datasource was switched to Tikr
3. `engine_project_knowledge` was NOT cleaned (no `ontology_id` cascade)
4. Old facts persisted, contaminating the current project

**Note:** Discovered via `psql` querying `ekaya_engine.engine_project_knowledge`, not through MCP tools.

## Fix Implementation

### 1. Apply BUG-10 Fix First

BUG-10's fix (adding `ontology_id` FK with CASCADE) will prevent future contamination.

### 2. One-Time Cleanup

Delete stale knowledge for the affected project:

```sql
-- Delete all knowledge for project that references old datasource
DELETE FROM engine_project_knowledge
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
  AND (
    key ILIKE '%claude_cowork%'
    OR key ILIKE '%Claude Chat%'
    OR key ILIKE '%Claude Code%'
    OR key ILIKE '%cross-product continuity%'
  );
```

### 3. Seed Correct Tikr Knowledge

Use MCP tool to add correct facts:

```
update_project_knowledge(
  fact="All monetary amounts are stored in cents (USD)",
  category="convention",
  context="Applies to billing_transactions, engagement_payment_intents, etc."
)

update_project_knowledge(
  fact="A Tik represents 6 seconds of engagement time",
  category="terminology",
  context="billing_helpers.go:413 - DurationPerTik"
)

update_project_knowledge(
  fact="MinCaptureAmount is $1.00 (100 cents)",
  category="business_rule",
  context="Minimum amount to capture from visitor"
)

update_project_knowledge(
  fact="MinPayoutAmount is $20.00 (2000 cents)",
  category="business_rule",
  context="Minimum amount for host payout"
)

update_project_knowledge(
  fact="Platform fees are 4.5% of total transaction",
  category="business_rule",
  context="billing_helpers.go:373"
)

update_project_knowledge(
  fact="Tikr share is 30% after platform fees",
  category="business_rule",
  context="billing_helpers.go:375 - ~66.35% to host"
)

update_project_knowledge(
  fact="Soft delete uses deleted_at column",
  category="convention",
  context="Tables with deleted_at support soft delete"
)

update_project_knowledge(
  fact="User timezone stored in accounts.time_zone",
  category="convention",
  context="IANA timezone string for display"
)

update_project_knowledge(
  fact="Entity IDs are UUIDs stored as text type",
  category="convention",
  context="No database-level FK constraints, requires join validation"
)
```

### 4. Validation After Cleanup

Verify knowledge is correct:

```sql
SELECT fact_type, key, context
FROM engine_project_knowledge
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
ORDER BY fact_type, created_at;
```

Expected output should only show Tikr-related facts.

## Prevention

1. **BUG-10 fix** prevents future contamination via cascade delete
2. **Knowledge seeding** (from BUG-5 fix) allows pre-populating correct knowledge
3. **Datasource-aware queries** filter knowledge by associated datasource

## Acceptance Criteria

- [ ] Stale claude_cowork facts deleted
- [ ] Correct Tikr knowledge seeded
- [ ] No contamination between datasources
- [ ] BUG-10 fix prevents recurrence
- [ ] Knowledge facts visible via MCP tools match actual Tikr business model

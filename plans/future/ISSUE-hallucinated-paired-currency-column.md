# ISSUE: Extraction hallucinates `paired_currency_column` references to non-existent columns

**Date:** 2026-03-02
**Status:** TODO
**Priority:** LOW
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

When the extraction identifies a monetary column, it populates a `paired_currency_column` field that is supposed to reference a sibling column containing the currency code (e.g., `currency_code`, `currency`). In the ekaya_marketing database, no such column exists in any table — all monetary values are in USD with no currency column. Despite this, the extraction invents references to non-existent columns.

### Affected columns

| Table | Column | paired_currency_column | Column exists? |
|-------|--------|----------------------|----------------|
| paid_channels | est_cost_low | `currency_code` | No |
| paid_channels | est_cost_high | `currency_code` | No |
| paid_placements | spend | `currency_code` | No |
| weekly_metrics | paid_spend | `currency_code` | No |
| marketing_tasks | budget_high | `app_id` | Yes, but it's a FK to applications, not a currency column |

## Expected Behavior

The extraction should only populate `paired_currency_column` if a matching column actually exists in the same table. Validation should check:

1. Does a column with the referenced name exist in the same table?
2. If yes, does it plausibly contain currency codes? (e.g., text type, low cardinality, values like 'USD', 'EUR')
3. If no matching column exists, leave `paired_currency_column` empty/null

## Impact

Low — an AI agent reading this metadata might try to join on or filter by a `currency_code` column that doesn't exist, causing a query error. In practice, most agents will use the column in SELECT/aggregate contexts where the paired column is irrelevant.

## Files to Investigate

| File | What to check |
|------|---------------|
| Monetary column detection / feature extraction | Where `paired_currency_column` is set — add validation that the referenced column exists in the table's schema |

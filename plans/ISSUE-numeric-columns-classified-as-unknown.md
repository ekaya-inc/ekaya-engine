# ISSUE: Postgres `numeric` type columns classified as `unknown`

**Date:** 2026-03-02
**Status:** TODO
**Priority:** MEDIUM
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

Columns with Postgres `numeric` data type (arbitrary precision decimal) are classified with `semantic_type: "unknown"`, `purpose: "text"`, and `role: "attribute"`. The extraction handles `integer` columns correctly (assigns `measure`, `count`, `monetary` etc.) but does not recognize `numeric` as a numeric type.

### Affected columns in ekaya_marketing

| Table | Column | data_type | Inferred semantic_type | Inferred role | Correct role |
|-------|--------|-----------|----------------------|---------------|-------------|
| content_posts | engagement_rate | numeric | unknown | attribute | measure |
| weekly_metrics | activation_rate | numeric | unknown | attribute | measure |
| weekly_metrics | linkedin_engagement_rate | numeric | unknown | attribute | measure |
| weekly_metrics | avg_cpa | numeric | unknown | attribute | measure |
| paid_placements | cpa | numeric | unknown | attribute | measure |
| paid_placements | activation_rate | numeric | unknown | attribute | measure |

All 6 columns are rates, percentages, or monetary values that should be classified as measures.

## Root Cause

The column classifier likely checks for `data_type = 'integer'` (or similar integer types) when deciding if a column is numeric. Postgres `numeric` (also known as `decimal`) is a valid numeric type but appears to be unrecognized, causing it to fall through to the default `unknown`/`text` classification.

## Expected Behavior

The extraction should treat `numeric`/`decimal` columns the same as `integer` columns for classification purposes:
- Recognize them as numeric (not text)
- Apply the same heuristics for `measure` vs `attribute` role assignment
- Apply monetary detection (column names like `cpa`, `avg_cpa` with `_cost`, `_spend`, `_cpa` patterns)
- Apply percentage detection (column names like `*_rate`, `activation_rate`, `engagement_rate`)

## Impact

- AI agents see `role: "attribute"` for metric columns and won't aggregate them
- `purpose: "text"` means the extraction treats decimal values as string data
- 6 of ~146 columns affected in this datasource (~4%)
- MCP correction works (we fixed `avg_cpa` via `update_column` and it survived re-extraction) but shouldn't be necessary for basic numeric type recognition

## Postgres numeric types to handle

The full set of Postgres numeric types:
- `integer` / `int4` (currently handled)
- `bigint` / `int8` (likely handled)
- `smallint` / `int2` (likely handled)
- `numeric` / `decimal` (**not handled**)
- `real` / `float4` (unknown if handled)
- `double precision` / `float8` (unknown if handled)

## Files to Investigate

| File | What to check |
|------|---------------|
| Column classifier / feature extraction | How `data_type` is mapped to `purpose` and `classification_path` — add `numeric`, `decimal`, `real`, `double precision` to the numeric type list |
| Role assignment logic | Ensure numeric-type columns get the same measure/attribute heuristics as integer columns |

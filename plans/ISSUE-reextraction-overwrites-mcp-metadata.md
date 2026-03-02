# ISSUE: Full ontology re-extraction overwrites MCP-sourced metadata

**Date:** 2026-03-02
**Status:** FIXED
**Priority:** HIGH
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

When a full ontology extraction is performed (delete + re-extract), all metadata previously set via MCP `update_column` and `update_table` tools is overwritten with lower-quality inferred values. This forces the MCP client to re-apply all corrections after every extraction.

### What gets overwritten

In a single session on ekaya_marketing, an MCP client (Claude Code) made these corrections after extraction:

| Category | Corrections made | Tool used |
|----------|-----------------|-----------|
| Table descriptions | 2 tables (applications, mcp_directories) | `update_table` |
| Table types | 2 tables (transactional → reference) | `update_table` |
| Column descriptions | 15+ columns (generic → domain-specific) | `update_column` |
| Column roles | 6 columns (attribute → measure) | `update_column` |
| Enum values | 10+ columns (guessed → actual Postgres values) | `update_column` |

All of these will be lost on the next full re-extraction.

### Concrete examples

**Table type reverts:**
- `applications` corrected to `reference` (5 static products) → re-extraction would set back to `transactional`
- `mcp_directories` corrected to `reference` → would revert to `transactional`

**Column descriptions revert:**
- `applications.name` corrected to "Product application name (e.g., 'MCP Server')" → would revert to "Name of a person, company, or entity"
- `applications.buyer` corrected to "Target buyer persona for go-to-market" → would revert to "Name of the buyer, which could be a person or a company"
- `marketing_tasks.name` → same pattern

**Column roles revert:**
- `content_posts.engagement_rate` corrected to `role: measure` → would revert to `role: attribute`
- `weekly_metrics.avg_cpa` corrected to `role: measure` → would revert to `role: attribute`
- 4 other measure columns → same pattern

**Enum values revert:**
- All 10+ enum columns corrected with actual Postgres enum values → would revert to guessed/numbered values (separately tracked in ISSUE-extraction-ignores-postgres-enum-types.md)

## Expected Behavior

MCP-sourced metadata should have higher precedence than inference-sourced metadata. On re-extraction:

1. **Preserve MCP-sourced values** — if a column's description, role, enum_values, or semantic_type was set via MCP (provenance = 'mcp'), do not overwrite with inferred values
2. **Only overwrite with higher-precedence sources** — Admin > MCP > Inference (this precedence model already exists for pending changes)
3. **New columns** should get inferred metadata as today (no MCP metadata exists yet)
4. **Schema changes** (column type changed, column renamed) may warrant re-inference, but should be surfaced as pending changes rather than silently applied

## Impact

- Every full re-extraction requires ~30 manual MCP tool calls to re-apply corrections
- Users who don't re-apply corrections get a degraded ontology with generic descriptions and wrong roles
- Creates a perverse incentive to avoid re-extraction, which means the ontology drifts from the actual schema

## Related Issues

- `ISSUE-extraction-ignores-postgres-enum-types.md` — enum values specifically (would be less impactful if enums were read from pg_enum)
- `ISSUE-extraction-timestamp-misclassification.md` — semantic_type specifically (would be less impactful if classification was better)
- `ISSUE-update-column-missing-semantic-type-param.md` — can't set semantic_type at all

## Files to Investigate

| File | What to check |
|------|---------------|
| Ontology extraction pipeline | Does it check provenance before overwriting existing metadata? |
| Column metadata persistence | Is provenance (mcp vs inferred) tracked and respected? |
| `update_column` / `update_table` handlers | Do they set provenance = 'mcp' on updates? |
| Incremental extraction logic | Does the new incremental extraction preserve MCP metadata? (May already be solved there) |

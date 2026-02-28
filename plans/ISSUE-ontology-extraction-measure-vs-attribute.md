# ISSUE: Ontology Extraction Tags Numeric Measure Columns as "attribute"

Status: FIXED

## Observed

After ontology extraction on the `ekaya_marketing` database, some numeric columns that represent calculated metrics are assigned `role: "attribute"` instead of `role: "measure"`:

- `paid_placements.cpa` (NUMERIC) — cost per acquisition, should be `measure`
- `weekly_metrics.avg_cpa` (NUMERIC) — average cost per acquisition, should be `measure`

Other similar columns were correctly tagged as `measure` (e.g. `paid_placements.activation_rate`, `content_posts.engagement_rate`), so the issue is inconsistent.

## Steps to Reproduce

1. Create a table with NUMERIC columns representing calculated metrics (cpa, avg_cpa)
2. Run ontology extraction
3. Call `get_ontology` at `tables` depth
4. Observe `cpa` and `avg_cpa` have `role: "attribute"` while `activation_rate` and `engagement_rate` have `role: "measure"`

## Expected

Columns like `cpa` and `avg_cpa` should be `role: "measure"` — they are aggregatable numeric metrics, not descriptive attributes.

## Likely Cause

The extraction heuristic may not recognize "cpa" or "avg_*" as measure patterns. It likely keys off column name patterns like `*_rate`, `*_count` for measure detection but misses abbreviations and `avg_` prefixes.

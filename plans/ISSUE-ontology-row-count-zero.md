# ISSUE: get_ontology Returns row_count: 0 for All Tables After Extraction

Status: FIXED

## Observed

After running ontology extraction on the `ekaya_marketing` database (which has seeded data), `get_ontology` at `tables` depth shows `row_count: 0` for every table, including tables with data:

- `marketing_tasks` — 25 rows, shows `row_count: 0`
- `content_posts` — 8 rows, shows `row_count: 0`
- `mcp_directories` — 18 rows, shows `row_count: 0`
- `applications` — 5 rows, shows `row_count: 0`

## Steps to Reproduce

1. Create and seed tables with data
2. Run ontology extraction
3. Call `get_ontology` at `tables` depth
4. Observe all tables show `row_count: 0`

## Expected

`row_count` should reflect the actual row count at the time of extraction, or be populated during the extraction process.

## Notes

This may be by design if row counts are only populated by `probe_column` or a separate statistics pass. If so, the extraction process should either include a row count step or the field should not be returned as `0` (which implies "empty table" rather than "not counted").

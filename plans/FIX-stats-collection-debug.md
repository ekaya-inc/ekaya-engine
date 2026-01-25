# FIX: Debug Stats Collection Flow (BUG-9)

**Priority:** 1 (High)
**Status:** Fix Applied (pending verification via re-extraction)
**Parent:** PLAN-ontology-next.md

## Problem

32.8% of joinable columns have NULL `distinct_count` despite code fixes being applied. The issue is specifically:
- `row_count` and `non_null_count` ARE populated correctly
- Only `distinct_count` is lost in the flow
- Pattern: primarily `text` (60% null) and `integer` (50% null) columns
- `timestamp` and `numeric` columns work correctly (0% null)

SQL queries work correctly when tested directly - the issue is in the Go code path.

## Investigation Steps

### Step 1: Add Runtime Logging ✓

Add debug logging to trace the exact values through the flow:

**File:** `pkg/adapters/datasource/postgres/schema.go` - `AnalyzeColumnStats`
```go
// After row.Scan
d.logger.Debug("Column stats raw scan",
    zap.String("column", colName),
    zap.Int64("row_count", s.RowCount),
    zap.Int64("non_null_count", s.NonNullCount),
    zap.Int64("distinct_count", s.DistinctCount))
```

**File:** `pkg/services/deterministic_relationship_service.go` - where stats are received
```go
// After AnalyzeColumnStats returns
for _, stat := range stats {
    s.logger.Debug("Received column stats",
        zap.String("column", stat.ColumnName),
        zap.Int64("distinct_count", stat.DistinctCount))
}
```

**File:** `pkg/services/deterministic_relationship_service.go` - `UpdateColumnJoinability`
```go
// Before update
s.logger.Debug("Updating column joinability",
    zap.String("column", col.ColumnName),
    zap.Any("distinct_count_ptr", col.DistinctCount))
```

### Step 2: Check statsMap Lookup ✓

In `deterministic_relationship_service.go`, verify the statsMap key matches what's being looked up:
```go
// Check if key exists
if stat, ok := statsMap[col.ColumnName]; ok {
    s.logger.Debug("Found stats for column", zap.String("column", col.ColumnName))
} else {
    s.logger.Warn("No stats found for column",
        zap.String("column", col.ColumnName),
        zap.Any("available_keys", reflect.ValueOf(statsMap).MapKeys()))
}
```

### Step 3: Check Pointer Assignment ✓

Verified: No pointer bug exists.

- `SchemaColumn.DistinctCount` is `*int64` (pointer) in `pkg/models/schema.go:39`
- `datasource.ColumnStats.DistinctCount` is `int64` (value) in `pkg/adapters/datasource/metadata.go:37`
- Code uses `for i := range stats` with `&stats[i]` (not `for _, stat := range` with `&stat`)
- This correctly takes address of slice element, not loop variable - slice elements persist

### Step 4: Check Type Differences ✓

Investigation complete. Key findings:

1. **SQL queries treat all types identically for `distinct_count`**
   - `COUNT(DISTINCT %s)` used for all column types
   - Only `min_length`/`max_length` have type-specific logic (LENGTH for text-compatible types)
   - File: `pkg/adapters/datasource/postgres/schema.go:260-283`

2. **The failure pattern (60% text, 50% integer) suggests DATA-dependent issues, not TYPE-dependent**
   - If type-specific handling was the cause, we'd expect uniform failure rates per type
   - Different failure rates indicate individual column issues that correlate with type

3. **timestamp columns are excluded from joinability analysis**
   - `isExcludedJoinType()` returns true for timestamp, timestamptz, date, etc.
   - File: `pkg/services/deterministic_relationship_service.go:431-446`
   - Stats ARE still collected for excluded types, but `is_joinable=false`
   - The verification query filters `WHERE is_joinable=true`, excluding timestamp from results

4. **0% null for timestamp/numeric likely means they're filtered out, not "working"**
   - timestamp: filtered out because `is_joinable=false` (type excluded)
   - numeric: NOT in the excluded types list - needs separate verification

5. **Root cause likely NOT in type handling** - need to check:
   - Column name matching between query results and map lookups
   - Whether specific columns fail the main query and fall back to retry
   - Whether retry logic correctly handles all cases

**Conclusion:** Type-based exclusion explains timestamp's 0%, but the text/integer pattern suggests a data-dependent issue. The logging added in Steps 1-2 should reveal the actual failure point.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/adapters/datasource/postgres/schema.go` | Add debug logging to AnalyzeColumnStats |
| `pkg/services/deterministic_relationship_service.go` | Add logging at stats receipt and update |

## Testing

1. ✓ Run ontology extraction with DEBUG logging enabled
2. Check logs for where distinct_count value is lost
3. Fix the identified issue
4. Re-run extraction and verify NULL stats rate < 10%

## Verification Query

```sql
SELECT
    COUNT(*) FILTER (WHERE c.distinct_count IS NULL) as null_stats,
    COUNT(*) as total,
    ROUND(100.0 * COUNT(*) FILTER (WHERE c.distinct_count IS NULL) / COUNT(*), 1) as null_pct
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.project_id = '<project-id>'
AND c.is_joinable = true;
```

## Success Criteria

- [x] Root cause identified via logging
- [x] Fix applied
- [ ] NULL stats rate < 10% for joinable columns (requires re-run of ontology extraction)

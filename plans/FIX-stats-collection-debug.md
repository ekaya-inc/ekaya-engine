# FIX: Debug Stats Collection Flow (BUG-9)

**Priority:** 1 (High)
**Status:** Not Started
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

### Step 2: Check statsMap Lookup

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

### Step 3: Check Pointer Assignment

Verify the pointer assignment for distinct_count:
```go
// In models/schema.go or wherever DistinctCount is defined
type SchemaColumn struct {
    DistinctCount *int64 // Is this a pointer?
}
```

If it's a pointer, check if the assignment is correct:
```go
// Bad: This creates a pointer to a loop variable
col.DistinctCount = &stat.DistinctCount // Bug if stat is loop var

// Good: Create a copy first
dc := stat.DistinctCount
col.DistinctCount = &dc
```

### Step 4: Check Type Differences

The pattern (text/integer fail, timestamp/numeric succeed) suggests possible type-related issues:
- Check if there's different handling for different column types
- Check if the SQL query returns different types for different columns

## Files to Modify

| File | Change |
|------|--------|
| `pkg/adapters/datasource/postgres/schema.go` | Add debug logging to AnalyzeColumnStats |
| `pkg/services/deterministic_relationship_service.go` | Add logging at stats receipt and update |

## Testing

1. Run ontology extraction with DEBUG logging enabled
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

- [ ] Root cause identified via logging
- [ ] Fix applied
- [ ] NULL stats rate < 10% for joinable columns

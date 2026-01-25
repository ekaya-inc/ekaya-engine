# FIX: BUG-9 - Stats Collection Fails for 27% of Joinable Columns

**Bug Reference:** BUGS-ontology-extraction.md, BUG-9
**Severity:** High
**Type:** Data Collection Bug
**Status:** NOT FIXED - Code changes applied but issue persists (32.8% null stats)

## Problem Summary

Stats collection (`distinct_count`) fails for a significant portion of columns marked as `is_joinable = true`. This prevents relationship discovery from evaluating these columns, causing many entities to have zero relationships.

**Scale of Impact:**
- 134 out of 501 joinable columns (27%) have `distinct_count = NULL`
- 21 out of 31 entities (68%) have zero relationships
- Core business entities like Billing Engagement, Session, Participant are affected

## Root Cause Analysis

### Issue 1: Per-Column Error Aborts Entire Table

**File:** `pkg/adapters/datasource/postgres/schema.go`

**Function:** `AnalyzeColumnStats` (line 231)

```go
for _, colName := range columnNames {
    // ... build query ...
    if err := row.Scan(&s.RowCount, &s.NonNullCount, &s.DistinctCount, &s.MinLength, &s.MaxLength); err != nil {
        return nil, fmt.Errorf("analyze column %s: %w", colName, err)  // ❌ ABORTS ALL
    }
}
```

If ANY column fails to analyze (type error, permissions, etc.), the entire table's stats collection fails and ALL columns in that table get NULL stats.

### Issue 2: Type Cast Failures

The query uses `::text` cast for length calculation:
```sql
MIN(LENGTH(%s::text)) as min_length,
MAX(LENGTH(%s::text)) as max_length
```

This fails for:
- Array columns (cannot cast to text)
- Binary/bytea columns
- Custom types without text cast
- JSON columns in some cases

### Issue 3: Silent Failure Propagation

**File:** `pkg/services/deterministic_relationship_service.go`

**Lines 302-306:**
```go
stats, err := discoverer.AnalyzeColumnStats(ctx, table.SchemaName, table.TableName, columnNames)
if err != nil {
    s.logger.Warn("Failed to analyze column stats", ...)  // ❌ Only logs warning
    continue  // ❌ Skips entire table
}
```

Failure is logged as warning but entire table is skipped, leaving ALL columns with NULL stats.

### Evidence of the Problem

```sql
-- Columns with NULL stats despite having data:
SELECT t.table_name, c.column_name, c.distinct_count, c.is_joinable
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND c.distinct_count IS NULL AND c.is_joinable = true;

-- But the actual data EXISTS:
SELECT COUNT(DISTINCT visitor_id) FROM billing_engagements;
-- Returns: 7
```

## The Fix

### Fix 1: Per-Column Error Isolation

Continue processing other columns when one fails:

**File:** `pkg/adapters/datasource/postgres/schema.go`

```go
func (d *SchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
    if len(columnNames) == 0 {
        return nil, nil
    }

    quotedSchema := pgx.Identifier{schemaName}.Sanitize()
    quotedTable := pgx.Identifier{tableName}.Sanitize()

    var stats []datasource.ColumnStats
    for _, colName := range columnNames {
        quotedCol := pgx.Identifier{colName}.Sanitize()

        query := fmt.Sprintf(`
            SELECT
                COUNT(*) as row_count,
                COUNT(%s) as non_null_count,
                COUNT(DISTINCT %s) as distinct_count,
                MIN(LENGTH(%s::text)) as min_length,
                MAX(LENGTH(%s::text)) as max_length
            FROM %s.%s
        `, quotedCol, quotedCol, quotedCol, quotedCol, quotedSchema, quotedTable)

        var s datasource.ColumnStats
        s.ColumnName = colName

        row := d.pool.QueryRow(ctx, query)
        if err := row.Scan(&s.RowCount, &s.NonNullCount, &s.DistinctCount, &s.MinLength, &s.MaxLength); err != nil {
            // Log warning but continue with other columns
            d.logger.Warn("Failed to analyze column stats",
                zap.String("column", colName),
                zap.Error(err))
            // Still append with zero/nil stats so column is tracked
            s.RowCount = 0
            s.NonNullCount = 0
            s.DistinctCount = 0
        }

        stats = append(stats, s)
    }

    return stats, nil
}
```

### Fix 2: Handle Type Cast Failures Gracefully

Use COALESCE and TRY_CAST pattern for length:

```sql
SELECT
    COUNT(*) as row_count,
    COUNT(%s) as non_null_count,
    COUNT(DISTINCT %s) as distinct_count,
    CASE
        WHEN pg_typeof(%s)::text IN ('text', 'character varying', 'char', 'uuid', 'varchar')
        THEN MIN(LENGTH(%s::text))
        ELSE NULL
    END as min_length,
    CASE
        WHEN pg_typeof(%s)::text IN ('text', 'character varying', 'char', 'uuid', 'varchar')
        THEN MAX(LENGTH(%s::text))
        ELSE NULL
    END as max_length
FROM %s.%s
```

Or use separate queries for length stats (less efficient but more robust).

### Fix 3: Better Error Reporting

**File:** `pkg/services/deterministic_relationship_service.go`

Add detailed logging and metrics:

```go
stats, err := discoverer.AnalyzeColumnStats(ctx, table.SchemaName, table.TableName, columnNames)
if err != nil {
    s.logger.Error("Failed to analyze column stats - table skipped",  // ❌ Change to Error
        zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
        zap.Int("column_count", len(columnNames)),
        zap.Error(err))
    failedTables++
    continue
}
```

Log summary at end:
```go
s.logger.Info("Column stats collection complete",
    zap.Int("tables_processed", processedTables),
    zap.Int("tables_failed", failedTables),  // NEW
    zap.Int("columns_with_stats", columnsWithStats),  // NEW
    zap.Duration("total_duration", time.Since(startTime)))
```

## Implementation Steps

### Step 1: Fix Per-Column Error Handling
- Modify `AnalyzeColumnStats` to continue on per-column errors
- Log warning for each failed column
- Return partial results instead of failing entirely

### Step 2: Fix Type Cast Issues ✓
- Use conditional casting for length stats
- Skip length for non-text types (set to NULL)

### Step 3: Improve Error Logging ✓
- Change warning to error for table failures
- Add failure counts to summary log
- Consider metrics for monitoring

### Step 4: Add Retry Logic (Optional) ✓
- Retry failed columns with simplified query (without length)
- Track which columns needed retry

## Files to Modify

| File | Change | Status |
|------|--------|--------|
| `pkg/adapters/datasource/postgres/schema.go` | Continue on per-column errors, fix type casting | ✅ Done |
| `pkg/adapters/datasource/mssql/schema.go` | Same fixes for MSSQL adapter (uses sys.columns metadata for type detection instead of SQL_VARIANT_PROPERTY) | ✅ Done |
| `pkg/services/deterministic_relationship_service.go` | Better error logging | ✅ Done |
| `pkg/adapters/datasource/postgres/schema_test.go` | Test error handling | ✅ Done |
| `pkg/adapters/datasource/mssql/schema_test.go` | Test error handling (PartialFailure, NonTextTypes, RetryWithSimplifiedQuery) | ✅ Done |

## Testing

### Test Per-Column Error Isolation

```go
func TestAnalyzeColumnStats_PartialFailure(t *testing.T) {
    // Create table with mixed column types
    // Include one column that will fail (e.g., array type)
    // Verify other columns still get stats
}
```

### Verify Stats Collection Completeness

```sql
-- After fix, verify reduced NULL stats:
SELECT
    COUNT(*) FILTER (WHERE c.distinct_count IS NULL) as null_stats,
    COUNT(*) as total
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND c.is_joinable = true;

-- Should show significantly fewer NULL stats
```

### Integration Test

```go
func TestStatsCollection_AllColumnsProcessed(t *testing.T) {
    // Clear stats
    // Run collectColumnStats
    // Verify all columns have stats (or documented reason for NULL)
}
```

## Success Criteria

- [x] Per-column failures don't abort entire table stats collection
- [ ] All text-compatible columns have accurate distinct_count
- [ ] Non-text columns have NULL length but valid distinct_count
- [x] Error logging shows which columns/tables had issues
- [ ] 90%+ of joinable columns have stats (up from 73%)
- [ ] BUG-3, BUG-6, BUG-11 symptoms reduced after fix

## Current Status (2026-01-25)

**Issue persists despite code fixes.** Testing shows:
- 134/409 (32.8%) joinable columns have NULL `distinct_count`
- All affected columns have `row_count` and `non_null_count` populated
- Pattern: primarily `text` columns (60% null) and `integer` columns (50% null)
- `timestamp` and `numeric` columns work correctly (0% null)

**SQL queries work correctly** when tested directly via MCP - the issue is in the Go code path between `AnalyzeColumnStats` returning results and `UpdateColumnJoinability` storing them.

**Investigation needed:**
1. Why does `distinct_count` get lost while `row_count` and `non_null_count` are preserved?
2. Is there a column name mismatch in the statsMap lookup?
3. Is there a type conversion issue with the pointer assignment?
4. Runtime logging needed to trace exact values through the flow

## Connection to Other Bugs

| Bug | Connection |
|-----|------------|
| **BUG-3** | Missing FK relationships due to NULL stats filtering |
| **BUG-6** | Zero occurrence counts because relationships not discovered |
| **BUG-11** | Unknown cardinality because stats not available |

Fixing this bug should automatically improve or resolve these related issues.

## Performance Consideration

The fix adds per-column error handling but shouldn't significantly impact performance since:
- Errors are already thrown, just now handled differently
- No additional queries added
- Partial results still useful

## Notes

The current behavior is overly conservative - failing the entire table when one column has issues. A more resilient approach:
1. Collect stats for all possible columns
2. Log warnings for problematic columns
3. Let downstream code handle missing stats appropriately

This aligns with the "fail-fast on missing data" principle at the relationship discovery level while being resilient at the data collection level.

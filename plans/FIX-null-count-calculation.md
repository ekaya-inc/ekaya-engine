# FIX: NullCount Calculation Bug

## Summary

The `null_count` field is never populated by schema adapters, causing null rate calculations to always return 0%. This breaks timestamp classification (soft delete detection), MCP tool responses, and cross-column analysis.

## Root Cause

Schema adapters (PostgreSQL, MSSQL) collect `non_null_count` via `COUNT(column)`, but code that calculates null rates reads from `null_count` which is always NULL in the database.

**Data flow:**
```
Schema Adapter → COUNT(col) → NonNullCount (populated)
                             → NullCount (never written, always NULL)

Feature Extraction → reads NullCount (nil) → NullRate = 0/rowCount = 0%
```

## Impact

| Symptom | Cause |
|---------|-------|
| `deleted_at` classified as `audit_updated` | LLM sees 0% null rate instead of 95%+ |
| Cross-column analysis "unknown column" warnings | Soft delete columns not flagged for Phase 5 |
| MCP `probe_column` returns null for `null_rate` | Calculation skipped when NullCount is nil |
| MCP `get_context` missing null rates | Same issue |

---

## Fix Locations

### Fix 1: column_feature_extraction.go (CRITICAL)

**File:** `pkg/services/column_feature_extraction.go`
**Lines:** 328-334

**Current code:**
```go
// Set null count and compute null rate
if col.NullCount != nil {
	profile.NullCount = *col.NullCount
}
if rowCount > 0 {
	profile.NullRate = float64(profile.NullCount) / float64(rowCount)
}
```

**Fixed code:**
```go
// Set null count and compute null rate
// Adapters populate NonNullCount via COUNT(col); calculate NullCount from it
if col.NullCount != nil {
	profile.NullCount = *col.NullCount
} else if col.NonNullCount != nil && rowCount > 0 {
	profile.NullCount = rowCount - *col.NonNullCount
}
if rowCount > 0 {
	profile.NullRate = float64(profile.NullCount) / float64(rowCount)
}
```

---

### Fix 2: mcp/tools/context.go

**File:** `pkg/mcp/tools/context.go`
**Lines:** 764-768

**Current code:**
```go
// Calculate null_rate if we have the data
if schemaCol.NullCount != nil {
	nullRate := float64(*schemaCol.NullCount) / float64(*schemaCol.RowCount)
	colDetail["null_rate"] = nullRate
}
```

**Fixed code:**
```go
// Calculate null_rate if we have the data
// NullCount is rarely populated; calculate from NonNullCount when available
if schemaCol.NullCount != nil {
	nullRate := float64(*schemaCol.NullCount) / float64(*schemaCol.RowCount)
	colDetail["null_rate"] = nullRate
} else if schemaCol.NonNullCount != nil {
	nullCount := *schemaCol.RowCount - *schemaCol.NonNullCount
	nullRate := float64(nullCount) / float64(*schemaCol.RowCount)
	colDetail["null_rate"] = nullRate
}
```

---

### Fix 3: mcp/tools/probe.go

**File:** `pkg/mcp/tools/probe.go`
**Lines:** 283-286

**Current code:**
```go
if column.NullCount != nil && column.RowCount != nil && *column.RowCount > 0 {
	nullRate := float64(*column.NullCount) / float64(*column.RowCount)
	stats.NullRate = &nullRate
}
```

**Fixed code:**
```go
if column.RowCount != nil && *column.RowCount > 0 {
	var nullCount int64
	if column.NullCount != nil {
		nullCount = *column.NullCount
	} else if column.NonNullCount != nil {
		nullCount = *column.RowCount - *column.NonNullCount
	}
	if nullCount > 0 || column.NullCount != nil || column.NonNullCount != nil {
		nullRate := float64(nullCount) / float64(*column.RowCount)
		stats.NullRate = &nullRate
	}
}
```

---

## Verification

### 1. Unit Test for column_feature_extraction.go

Add to `pkg/services/column_feature_extraction_test.go`:

```go
func Test_buildColumnProfile_NullRateFromNonNullCount(t *testing.T) {
	// This test verifies the fix for the NullCount bug.
	// In production, adapters populate NonNullCount but not NullCount.
	// The code must calculate NullCount = RowCount - NonNullCount.

	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	rowCount := int64(100)
	nonNullCount := int64(5) // 95 nulls = 95% null rate

	col := &models.SchemaColumn{
		ID:           uuid.New(),
		ColumnName:   "deleted_at",
		DataType:     "timestamp with time zone",
		IsNullable:   true,
		RowCount:     &rowCount,
		NonNullCount: &nonNullCount,
		NullCount:    nil, // Simulates production: never populated
	}

	tableRowCounts := map[uuid.UUID]int64{col.SchemaTableID: rowCount}
	profile := svc.buildColumnProfile(col, tableRowCounts)

	// Verify NullCount was calculated
	expectedNullCount := int64(95)
	if profile.NullCount != expectedNullCount {
		t.Errorf("NullCount = %d, want %d", profile.NullCount, expectedNullCount)
	}

	// Verify NullRate was calculated correctly
	expectedNullRate := 0.95
	if profile.NullRate != expectedNullRate {
		t.Errorf("NullRate = %f, want %f", profile.NullRate, expectedNullRate)
	}
}

func Test_buildColumnProfile_NullCountPreferred(t *testing.T) {
	// If NullCount IS populated (future-proofing), prefer it over calculation
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	rowCount := int64(100)
	nullCount := int64(80)
	nonNullCount := int64(20)

	col := &models.SchemaColumn{
		ID:           uuid.New(),
		ColumnName:   "optional_field",
		DataType:     "text",
		IsNullable:   true,
		RowCount:     &rowCount,
		NullCount:    &nullCount,    // Explicitly set
		NonNullCount: &nonNullCount,
	}

	tableRowCounts := map[uuid.UUID]int64{col.SchemaTableID: rowCount}
	profile := svc.buildColumnProfile(col, tableRowCounts)

	// Should use NullCount directly, not calculate
	if profile.NullCount != nullCount {
		t.Errorf("NullCount = %d, want %d (should use NullCount directly)", profile.NullCount, nullCount)
	}
}
```

### 2. Integration Test

Add to `pkg/services/column_feature_extraction_test.go` or a new integration test file:

```go
func Test_Integration_TimestampSoftDeleteClassification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a real database with test data
	// Setup: Create table with deleted_at column where 95% of rows have NULL

	ctx := context.Background()
	db := testhelpers.GetTestDB(t)

	// Create test table
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_soft_delete (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			deleted_at TIMESTAMP WITH TIME ZONE
		)
	`)
	require.NoError(t, err)

	// Insert 100 rows, only 5 with deleted_at set
	_, err = db.Exec(ctx, `
		TRUNCATE test_soft_delete;
		INSERT INTO test_soft_delete (name, deleted_at)
		SELECT
			'item_' || i,
			CASE WHEN i <= 5 THEN NOW() ELSE NULL END
		FROM generate_series(1, 100) AS i
	`)
	require.NoError(t, err)

	// Run schema discovery (this populates NonNullCount)
	// ... adapter.AnalyzeColumnStats() ...

	// Run column feature extraction
	// ... service.ExtractFeaturesWithProgress() ...

	// Verify deleted_at is classified as soft_delete
	// Query the column features from metadata
	var semanticType string
	err = db.QueryRow(ctx, `
		SELECT metadata->'column_features'->>'semantic_type'
		FROM engine_schema_columns
		WHERE column_name = 'deleted_at'
		  AND schema_table_id = $1
	`, tableID).Scan(&semanticType)
	require.NoError(t, err)

	assert.Equal(t, "soft_delete", semanticType,
		"deleted_at with 95% null rate should be classified as soft_delete")
}
```

### 3. Manual Verification

```sql
-- Before fix: Check that NonNullCount is populated but NullCount is not
SELECT
    column_name,
    row_count,
    non_null_count,
    null_count,
    CASE
        WHEN row_count > 0 AND non_null_count IS NOT NULL
        THEN round((row_count - non_null_count)::numeric / row_count * 100, 1)
        ELSE NULL
    END as calculated_null_pct
FROM engine_schema_columns
WHERE column_name LIKE '%deleted%'
  AND deleted_at IS NULL
LIMIT 10;

-- After fix: Run feature extraction and verify soft_delete classification
SELECT
    c.column_name,
    c.metadata->'column_features'->>'semantic_type' as semantic_type,
    c.metadata->'column_features'->'timestamp_features'->>'is_soft_delete' as is_soft_delete
FROM engine_schema_columns c
WHERE c.column_name LIKE '%deleted%'
  AND c.deleted_at IS NULL
  AND c.metadata->'column_features' IS NOT NULL;
```

### 4. MCP Tool Verification

```bash
# Test probe_column returns null_rate
curl -X POST http://localhost:3443/mcp/<project-id> \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "probe_column",
      "arguments": {"table": "users", "column": "deleted_at"}
    }
  }' | jq '.result.content[0].text | fromjson | .statistics.null_rate'

# Should return a number like 0.95, not null
```

---

## Related Issues Fixed

Once this fix is applied, the following symptoms will be resolved:

1. **Soft delete misclassification** - `deleted_at` columns will be correctly classified as `soft_delete` instead of `audit_updated`

2. **Cross-column "unknown column" warnings** - Soft delete columns will be flagged for Phase 5, so they'll be in the `columnIDByName` map when the LLM references them

3. **MCP null_rate missing** - `probe_column` and `get_context` will return correct null rates

---

## Checklist

```
[x] Apply fix to pkg/services/column_feature_extraction.go:328-334
[x] Apply fix to pkg/mcp/tools/context.go:764-768
[x] Apply fix to pkg/mcp/tools/probe.go:283-286
[x] Add unit tests for NullRate calculation
[x] Run make check - all tests pass
[x] Manual verification: re-run ontology extraction on test project
[x] Verify deleted_at columns now classified as soft_delete
[x] Verify no more "unknown column" warnings in logs
```

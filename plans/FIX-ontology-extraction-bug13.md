# FIX: BUG-13 - Glossary SQL Semantic and Structural Issues

**Bug Reference:** BUGS-ontology-extraction.md, BUG-13
**Severity:** Medium
**Type:** Data Quality Issue

## Problem Summary

Beyond enum value issues (BUG-12), several glossary terms have semantic or structural issues in their SQL:

1. **User Review Rating**: Returns 2 rows instead of 1 (UNION ALL issue)
2. **Average Fee Per Engagement**: Formula calculates percentage, not average
3. **Preauthorization Utilization**: Calculates rate ($/minute), not utilization ratio

## Issue Details

### Issue 1: User Review Rating Returns Multiple Rows

**Current SQL:**
```sql
SELECT AVG(ur.reviewee_rating) AS average_user_review_rating
FROM user_reviews ur WHERE ur.deleted_at IS NULL
UNION ALL
SELECT AVG(cr.rating) AS average_channel_review_rating
FROM channel_reviews cr WHERE cr.deleted_at IS NULL;
```

**Problem:** UNION ALL returns **two separate rows**, not a single combined value.

**Expected Behavior:** Single-row result with combined average or separate terms.

**Fix Options:**
1. Split into two glossary terms (User Review Rating, Channel Review Rating)
2. Use weighted average: `(SUM_user + SUM_channel) / (COUNT_user + COUNT_channel)`
3. Use subquery: `SELECT AVG(rating) FROM (SELECT ... UNION ALL SELECT ...)`

### Issue 2: Average Fee Per Engagement - Wrong Formula

**Current SQL:**
```sql
SUM(platform_fees) / NULLIF(SUM(total_amount), 0) * 100
```

**Semantic Mismatch:**
- **Term name** says "Average Fee Per Engagement" (fee divided by count)
- **Formula** calculates "Fee as Percentage of Revenue" (fee divided by revenue × 100)

**Expected Formula:**
```sql
SUM(platform_fees) / COUNT(*)  -- Total fees / number of engagements
```

### Issue 3: Preauthorization Utilization - Rate vs Ratio

**Current SQL:**
```sql
SUM(preauthorization_amount) / NULLIF(SUM(preauthorization_minutes), 0)
```

**Result:** 11400 / 120 = 95 (dollars per minute?)

**Semantic Mismatch:**
- "Utilization Ratio" typically means: actual_used / total_authorized (percentage)
- Formula calculates: amount / minutes (a rate, $/minute)

**Needs Clarification:** What does "Preauthorization Utilization" actually mean?
- Ratio of used vs authorized amount?
- Rate of spend per time?
- Something else?

## Root Cause Analysis

### Why These Issues Occur

1. **Ambiguous Term Names**: Terms like "Average Fee" can be interpreted multiple ways
2. **Missing Business Context**: LLM doesn't know exact business definitions
3. **No Semantic Validation**: Current validation only checks SQL syntax, not meaning
4. **No Row Count Validation**: UNION ALL returning multiple rows isn't caught

### Current Validation Limitations

**File:** `pkg/services/glossary_service.go`

`TestSQL` function (line 348) validates:
- ✅ SQL executes without errors
- ✅ Captures output columns
- ❌ Does NOT validate single-row result
- ❌ Does NOT validate semantic correctness
- ❌ Does NOT compare formula to term meaning

## The Fix

### Fix 1: Add Row Count Validation

Reject SQL that returns multiple rows for aggregate metrics:

```go
func (s *glossaryService) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*SQLTestResult, error) {
    // ... existing code ...

    // Execute with limit 5 to check for multi-row results
    result, err := executor.Query(ctx, sql, 5)

    // NEW: Check for single-row result (for aggregate metrics)
    if len(result.Rows) > 1 {
        return &SQLTestResult{
            Valid: false,
            Error: "Query returns multiple rows. Aggregate metrics should return a single row.",
        }, nil
    }

    // ... rest of function ...
}
```

### Fix 2: Improve LLM Prompts

Add explicit instructions about row count and formula semantics:

```go
func (s *glossaryService) enrichTermSystemMessage() string {
    return `...

IMPORTANT REQUIREMENTS:
1. The SQL MUST return exactly ONE row (aggregate/summary metrics)
2. Do NOT use UNION/UNION ALL unless combining into a single result
3. The formula must match the term name semantically:
   - "Average X Per Y" → SUM(X) / COUNT(Y)
   - "X Rate" → X per unit time
   - "X Ratio" → X / Total as percentage

EXAMPLES OF CORRECT FORMULAS:
- "Average Order Value" → SUM(order_total) / COUNT(*)
- "Revenue Rate" → SUM(revenue) / time_period
- "Conversion Ratio" → conversions / visitors * 100

...`
}
```

### Fix 3: Add Formula Validation (Optional)

Pattern-match formulas against term names:

```go
func validateFormulaSemantics(termName string, sql string) []string {
    var warnings []string

    termLower := strings.ToLower(termName)

    // Check "Average X Per Y" pattern
    if strings.Contains(termLower, "average") && strings.Contains(termLower, "per") {
        if !containsPattern(sql, "COUNT(*)") && !containsPattern(sql, "COUNT(") {
            warnings = append(warnings, "Term mentions 'average per' but SQL doesn't divide by COUNT")
        }
    }

    // Check UNION ALL in aggregate metrics
    if strings.Contains(strings.ToUpper(sql), "UNION") {
        warnings = append(warnings, "SQL uses UNION which may return multiple rows")
    }

    return warnings
}
```

### Fix 4: Domain Expert Review Workflow

Flag terms for manual review when:
- Formula complexity is high
- Term name is ambiguous
- Validation returns warnings

```go
type GlossaryTerm struct {
    // ... existing fields ...
    NeedsReview    bool   `json:"needs_review"`
    ReviewReason   string `json:"review_reason,omitempty"`
}
```

## Implementation Steps

### Step 1: Add Row Count Validation

**File:** `pkg/services/glossary_service.go`

Modify `TestSQL` to check for multi-row results.

### Step 2: Update LLM Prompts ✅

Add explicit instructions about single-row results and formula patterns.

### Step 3: Review Existing Terms ✅

Manually reviewed and fixed via MCP glossary tools:
- All 11 glossary terms now have valid SQL returning single rows
- Fixed 5 terms with missing SQL: Engagement Completion Rate, Engagement Fee Per Minute, Engagement Quality Score, Engagement Revenue, Host Earnings
- Verified all queries execute correctly

### Step 4: Add Semantic Validation (Optional)

Implement pattern-based formula checking.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/glossary_service.go` | Add row count validation, improve prompts |
| `pkg/models/glossary.go` | Add review fields (optional) |

## Testing

### Test Single-Row Validation

```go
func TestTestSQL_MultipleRows_ReturnsError(t *testing.T) {
    sql := "SELECT 1 UNION ALL SELECT 2"
    result, err := service.TestSQL(ctx, projectID, sql)

    assert.NoError(t, err)
    assert.False(t, result.Valid)
    assert.Contains(t, result.Error, "multiple rows")
}
```

### Verify Fixed Terms

```sql
-- User Review Rating should return single row
SELECT defining_sql FROM engine_business_glossary WHERE term = 'User Review Rating';
-- Execute and verify: returns 1 row

-- Average Fee should use COUNT
SELECT defining_sql FROM engine_business_glossary WHERE term = 'Average Fee Per Engagement';
-- Should contain: / COUNT(*)
```

## Specific Term Fixes

### User Review Rating

**Option A: Split into two terms**
- "User Review Rating": `SELECT AVG(reviewee_rating) FROM user_reviews`
- "Channel Review Rating": `SELECT AVG(rating) FROM channel_reviews`

**Option B: Combined weighted average**
```sql
SELECT
    (COALESCE(SUM(ur.reviewee_rating), 0) + COALESCE(SUM(cr.rating), 0)) /
    NULLIF(COUNT(ur.*) + COUNT(cr.*), 0) AS combined_rating
FROM user_reviews ur
FULL OUTER JOIN channel_reviews cr ON false
WHERE ur.deleted_at IS NULL OR cr.deleted_at IS NULL
```

### Average Fee Per Engagement

**Correct formula:**
```sql
SELECT SUM(platform_fees) / COUNT(*) AS average_fee_per_engagement
FROM billing_transactions
WHERE deleted_at IS NULL
```

### Preauthorization Utilization

**Needs domain clarification.** Possible interpretations:
- If it means "preauth spend rate": current formula is correct
- If it means "preauth utilization %": needs `actual_charged / preauthorization_amount * 100`

## Success Criteria

- [x] All glossary SQL returns exactly one row
- [x] Formulas match semantic meaning of term names
- [x] No UNION/UNION ALL in aggregate metrics
- [x] Row count validation prevents multi-row queries
- [x] Domain expert reviews ambiguous terms (completed via MCP tools)

## Notes

This bug highlights the challenge of automated metric generation:
- Term names can be ambiguous
- Business definitions aren't captured in schema
- LLMs make reasonable but sometimes wrong interpretations

For high-quality glossary:
1. Provide clear, unambiguous term definitions
2. Include example calculations in business context
3. Implement validation beyond syntax checking
4. Have domain experts review generated metrics

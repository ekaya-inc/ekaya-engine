# FIX: BUG-12 - Glossary SQL Uses Wrong Enum Values

**Bug Reference:** BUGS-ontology-extraction.md, BUG-12
**Severity:** Critical
**Type:** Data Quality / Functional Bug

## Problem Summary

5 out of 9 glossary terms with SQL (55%) use incorrect enum values in their WHERE clauses. The LLM generates SQL with guessed enum values (e.g., `'ended'`) instead of actual database values (e.g., `'TRANSACTION_STATE_ENDED'`). This causes all affected queries to **return zero results**.

## Evidence

```sql
-- Actual enum values in database:
SELECT transaction_state, COUNT(*) FROM billing_transactions GROUP BY transaction_state;
-- TRANSACTION_STATE_ENDED   | 70
-- TRANSACTION_STATE_WAITING | 27
-- TRANSACTION_STATE_ERROR   |  3

-- But glossary SQL uses:
WHERE transaction_state = 'ended'  -- Returns 0 rows!
```

**Affected Terms:**
- Payout Amount
- Engagement Revenue
- Average Fee Per Engagement
- Session Duration
- Transaction Volume

## Root Cause

### Enum Values Not Included in LLM Prompt

**File:** `pkg/services/glossary_service.go`

**Function:** `buildEnrichTermPrompt` (line 1058)

The prompt includes column details but **omits enum values**:

```go
// Lines 1118-1126: Column details in prompt
for _, col := range relevantCols {
    colInfo := fmt.Sprintf("- `%s`", col.Name)
    if col.Role != "" {
        colInfo += fmt.Sprintf(" [%s]", col.Role)
    }
    if col.Description != "" {
        colInfo += fmt.Sprintf(" - %s", col.Description)
    }
    sb.WriteString(colInfo + "\n")
    // ❌ col.EnumValues NOT INCLUDED
}
```

The `ColumnDetail` struct HAS enum values (from `pkg/models/ontology.go`):
```go
type ColumnDetail struct {
    Name          string      `json:"name"`
    // ... other fields ...
    EnumValues    []EnumValue `json:"enum_values,omitempty"`  // ✅ Data exists
}
```

But they're never included in the prompt!

## The Fix

### Fix 1: Include Enum Values in Prompt (Recommended)

**File:** `pkg/services/glossary_service.go`

Update the column details section in `buildEnrichTermPrompt`:

```go
for _, col := range relevantCols {
    colInfo := fmt.Sprintf("- `%s`", col.Name)
    if col.Role != "" {
        colInfo += fmt.Sprintf(" [%s]", col.Role)
    }
    if col.Description != "" {
        colInfo += fmt.Sprintf(" - %s", col.Description)
    }
    sb.WriteString(colInfo + "\n")

    // NEW: Include enum values if present
    if len(col.EnumValues) > 0 {
        values := make([]string, len(col.EnumValues))
        for i, v := range col.EnumValues {
            values[i] = fmt.Sprintf("'%s'", v.Value)
        }
        sb.WriteString(fmt.Sprintf("    Allowed values: %s\n", strings.Join(values, ", ")))
    }
}
```

### Fix 2: Add Explicit Enum Section

Add a dedicated section for enum columns:

```go
// Include enum columns with values
enumCols := collectEnumColumns(ontology)
if len(enumCols) > 0 {
    sb.WriteString("## Enumeration Columns\n\n")
    sb.WriteString("Use these exact values in WHERE clauses:\n\n")
    for _, col := range enumCols {
        sb.WriteString(fmt.Sprintf("**%s.%s:**\n", col.Table, col.Name))
        for _, v := range col.EnumValues {
            sb.WriteString(fmt.Sprintf("- `'%s'`", v.Value))
            if v.Description != "" {
                sb.WriteString(fmt.Sprintf(" - %s", v.Description))
            }
            sb.WriteString("\n")
        }
        sb.WriteString("\n")
    }
}
```

### Fix 3: SQL Validation with Enum Check

Add validation that checks generated SQL against known enum values:

```go
func (s *glossaryService) validateEnumValues(sql string, ontology *models.TieredOntology) []string {
    var issues []string

    // Extract string literals from SQL
    literals := extractStringLiterals(sql)

    // Check each literal against known enum columns
    for _, col := range ontology.ColumnDetails {
        for _, enumCol := range col.EnumValues {
            // If SQL uses a value that looks like a shortened form of an enum...
            // (Implementation detail: pattern matching or fuzzy comparison)
        }
    }

    return issues
}
```

## Implementation Steps

### Step 1: Update Prompt Building

**File:** `pkg/services/glossary_service.go`

In `buildEnrichTermPrompt`, add enum values after column description.

### Step 2: Update System Message

Update `enrichTermSystemMessage` to emphasize using exact enum values:

```go
func (s *glossaryService) enrichTermSystemMessage() string {
    return `...

IMPORTANT: When filtering on enumeration columns, use the EXACT values provided in the schema context.
Do NOT simplify or normalize enum values (e.g., use 'TRANSACTION_STATE_ENDED' not 'ended').

...`
}
```

### Step 3: Add Post-Generation Validation (Optional)

Test generated SQL for common enum mistakes before saving.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/glossary_service.go` | Include enum values in prompt |
| `pkg/services/glossary_service_test.go` | Test enum inclusion |

## Testing

### Verify Enum Values in Prompt

```go
func TestBuildEnrichTermPrompt_IncludesEnumValues(t *testing.T) {
    ontology := &models.TieredOntology{
        ColumnDetails: map[string][]models.ColumnDetail{
            "billing_transactions": {
                {
                    Name: "transaction_state",
                    Role: "dimension",
                    EnumValues: []models.EnumValue{
                        {Value: "TRANSACTION_STATE_ENDED"},
                        {Value: "TRANSACTION_STATE_WAITING"},
                    },
                },
            },
        },
    }

    prompt := service.buildEnrichTermPrompt(term, ontology, entities)

    assert.Contains(t, prompt, "TRANSACTION_STATE_ENDED")
    assert.Contains(t, prompt, "TRANSACTION_STATE_WAITING")
}
```

### Verify SQL Uses Correct Values

After fix, regenerate glossary and verify:
```sql
-- Test affected term (Payout Amount)
-- Query should return non-zero results
SELECT defining_sql FROM engine_business_glossary WHERE term = 'Payout Amount';
-- Should contain: WHERE transaction_state = 'TRANSACTION_STATE_ENDED'

-- Execute the SQL and verify results
-- Should return 70 rows (matching actual data)
```

## Success Criteria

- [ ] LLM prompt includes actual enum values for relevant columns
- [ ] Regenerated glossary SQL uses correct enum values
- [ ] All 5 affected terms return non-zero results
- [ ] SQL validation (if added) catches enum mismatches

## Alternative/Complementary Fixes

### A: Pre-populate Schema Context

Ensure column enrichment captures all enum values from the database:

```sql
-- Query to get distinct values for low-cardinality columns
SELECT DISTINCT transaction_state FROM billing_transactions LIMIT 50;
```

Store these in `column.SampleValues` or `column.EnumValues`.

### B: SQL Post-Processing

After LLM generates SQL, attempt to fix obvious enum mismatches:

```go
func normalizeEnumValues(sql string, knownEnums map[string][]string) string {
    // Pattern match: WHERE column = 'value'
    // If 'value' is not in known enums but 'COLUMN_VALUE' is, replace it
}
```

### C: Validation-Driven Regeneration

If SQL validation fails (returns 0 rows), retry with specific feedback:

```go
if testResult.RowCount == 0 && hasEnumFilters(sql) {
    feedback := fmt.Sprintf("SQL returned 0 rows. Check enum values. Known values: %v", enumValues)
    // Regenerate with feedback
}
```

## Notes

This bug highlights the importance of providing complete context to LLMs. The model made reasonable guesses ('ended' for a completed state) but lacked the specific database values.

The fix ensures:
1. LLM has exact enum values available
2. Generated SQL uses correct values
3. Queries return expected results

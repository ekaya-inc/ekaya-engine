# FIX: Improve Glossary SQL Generation (BUG-10)

**Priority:** 4 (Medium)
**Status:** Not Started
**Parent:** PLAN-ontology-next.md

## Problem

4 out of 15 glossary terms have invalid SQL due to LLM hallucinations:

1. **Active Sessions** - column `started_at` does not exist
2. **Content Popularity Score** - column `cl.channel_likes_count` does not exist
3. **Offer Redemption Rate** - operator mismatch (bigint = text)
4. **Payout Conversion Rate** - operator mismatch (bigint = text)

## Root Cause

LLM generates SQL referencing columns that don't exist or uses incorrect type comparisons. The current prompt doesn't provide enough schema context.

## Solution

### Step 1: Enhance LLM Prompt with Column Information [COMPLETE]

**File:** `pkg/services/glossary_service.go`

In `enrichTermSystemMessage` or the enrichment prompt, include:
- Actual column names from the schema
- Column data types
- Sample values for context

Example prompt addition:
```
AVAILABLE COLUMNS for table 'sessions':
- id (uuid)
- user_id (uuid)
- created_at (timestamp)
- ended_at (timestamp)
- status (varchar): ['active', 'ended', 'expired']

NOTE: There is NO column named 'started_at'. Use 'created_at' for session start time.
```

### Step 2: Add Column Name Validation Before Storage [COMPLETE]

Before storing the SQL, extract column references and validate they exist:
```go
func (s *glossaryService) validateColumnReferences(ctx context.Context, projectID uuid.UUID, sql string) error {
    // Parse SQL to extract column references
    // Check each reference against engine_schema_columns
    // Return error listing non-existent columns
}
```

### Step 3: Add Type Information to Prompt Context [COMPLETE]

For type mismatch errors, include column type information:
```
IMPORTANT TYPE INFORMATION:
- offer_id is type 'bigint' - compare with integers, not strings
- status is type 'text' - compare with strings

WRONG: WHERE offer_id = 'abc'
RIGHT: WHERE offer_id = 123
```

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/glossary_service.go` | Enhance `enrichSingleTerm` prompt with column info |
| `pkg/services/glossary_service.go` | Add column name validation before storage |

## Testing

1. Clear existing glossary terms
2. Re-run glossary enrichment
3. Verify all terms have valid SQL

```sql
SELECT term, enrichment_status, enrichment_error
FROM engine_business_glossary
WHERE project_id = '<project-id>'
AND enrichment_status != 'success';
-- Should return 0 rows after fix
```

## Success Criteria

- [ ] All glossary terms have valid, executable SQL
- [ ] No column reference errors in enrichment
- [ ] No type mismatch errors in generated SQL

# FIX: BUG-10 - Glossary Terms Generated Without SQL Definitions

**Bug Reference:** BUGS-ontology-extraction.md, BUG-10
**Severity:** Medium
**Type:** Data Quality Issue

## Problem Summary

Some glossary terms are generated with definitions but empty `defining_sql`. Terms without SQL cannot be used by MCP clients to calculate metrics - they only serve as documentation.

**Affected Terms:**
- Offer Utilization Rate (definition: 291 chars, SQL: empty)
- Referral Bonus Participation (definition: 239 chars, SQL: empty)

## Root Cause Analysis

### Glossary Generation Flow

1. **Discovery Phase** (`discoverTermsWithLLM`): Terms created with `Term`, `Definition`, empty `DefiningSQL`
2. **Enrichment Phase** (`EnrichTerms`): LLM generates SQL for each term
3. **Validation**: SQL tested against database for validity

### Why Terms Have Empty SQL

**File:** `pkg/services/glossary_service.go`

The enrichment process in `enrichSingleTerm` (line 970) can fail at several points:

```go
// Line 991-993: LLM generation failure
result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMessage, 0.3, false)
if err != nil {
    return enrichmentResult{termName: term.Term, err: fmt.Errorf("LLM generate: %w", err)}
}

// Line 997-999: Parse failure
enrichment, err := s.parseEnrichTermResponse(result.Content)
if err != nil {
    return enrichmentResult{termName: term.Term, err: fmt.Errorf("parse response: %w", err)}
}

// Line 1002-1003: Empty SQL
if enrichment.DefiningSQL == "" {
    return enrichmentResult{termName: term.Term, err: fmt.Errorf("LLM returned empty SQL")}
}

// Line 1007-1014: SQL validation failure
testResult, err := s.TestSQL(tenantCtx, projectID, enrichment.DefiningSQL)
if err != nil || !testResult.Valid {
    return enrichmentResult{termName: term.Term, err: ...}
}
```

When enrichment fails (lines 938-950), the error is logged but the term keeps its empty `DefiningSQL`:

```go
if result.err != nil {
    s.logger.Error("Failed to enrich term",
        zap.String("term", result.termName),
        zap.Error(result.err))
    failedCount++
} else {
    successCount++
}
```

### Likely Reasons for Failure

For "Offer Utilization Rate" and "Referral Bonus Participation":
1. **Complex multi-table metrics**: Require joins across multiple tables
2. **Conceptual metrics**: Represent ratios/percentages that are hard to express in a single query
3. **Missing data context**: LLM doesn't have enough schema context to generate valid SQL
4. **SQL validation failure**: Generated SQL doesn't execute (syntax error, missing tables)

## The Fix

### Option A: Retry with Enhanced Context (Recommended)

Add retry logic with more context when enrichment fails:

```go
func (s *glossaryService) enrichSingleTerm(...) enrichmentResult {
    // First attempt
    result, err := s.tryEnrichTerm(ctx, term, ontology, entities, llmClient, projectID, false)
    if err == nil {
        return result
    }

    // Retry with enhanced context (includes more schema detail, examples)
    s.logger.Debug("Retrying enrichment with enhanced context",
        zap.String("term", term.Term),
        zap.Error(err))

    return s.tryEnrichTerm(ctx, term, ontology, entities, llmClient, projectID, true)
}
```

### Option B: Mark Terms as Needing Review

Add a `status` or `needs_review` flag for terms that fail enrichment:

```go
if enrichment.DefiningSQL == "" {
    term.NeedsReview = true
    term.ReviewReason = "LLM failed to generate SQL"
    if err := s.glossaryRepo.Update(tenantCtx, term); err != nil {
        return enrichmentResult{termName: term.Term, err: fmt.Errorf("mark for review: %w", err)}
    }
    return enrichmentResult{termName: term.Term, err: fmt.Errorf("marked for review: empty SQL")}
}
```

### Option C: Exclude Conceptual Terms from Discovery

Some terms like "Utilization Rate" may be too abstract for SQL. Filter them during discovery:

```go
// In discoverTermsWithLLM prompt:
"Only suggest metrics that can be computed with a single SELECT query.
Do NOT suggest abstract ratios or rates that require complex multi-step calculations."
```

### Option D: Generate Placeholder SQL

For complex metrics, generate a placeholder that shows the intent:

```sql
-- Placeholder: This metric requires multiple queries or calculations
-- Offer Utilization Rate = (offers_used / offers_total) * 100
-- Please implement manually based on your business logic
SELECT NULL AS offer_utilization_rate;
```

## Implementation Steps

### Step 1: Add Enhanced Retry

**File:** `pkg/services/glossary_service.go`

Modify `enrichSingleTerm` to retry with more context on failure.

### Step 2: Improve LLM Prompt

Add more examples for complex metrics in `enrichTermSystemMessage`:

```go
func (s *glossaryService) enrichTermSystemMessage() string {
    return `...

EXAMPLES OF COMPLEX METRICS:

For a "Utilization Rate" metric:
{
    "defining_sql": "SELECT
        COUNT(*) FILTER (WHERE status = 'used') * 100.0 / NULLIF(COUNT(*), 0) AS utilization_rate
    FROM offers
    WHERE created_at >= NOW() - INTERVAL '30 days'",
    "base_table": "offers",
    "aliases": ["offer usage rate", "redemption rate"]
}

...`
}
```

### Step 3: Add Status Tracking

**File:** `pkg/models/glossary.go`

Add optional fields:
```go
type BusinessGlossaryTerm struct {
    // ... existing fields ...
    EnrichmentStatus string // "pending", "success", "failed"
    EnrichmentError  string // Error message if failed
}
```

### Step 4: Surface Failures in UI/API

Return enrichment status in API responses so users can see which terms need attention.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/glossary_service.go` | Add retry logic, improve prompts |
| `pkg/models/glossary.go` | Add status fields (optional) |
| `pkg/repositories/glossary_repository.go` | Update schema for status (optional) |
| `migrations/...` | Add enrichment_status column (optional) |

## Testing

### Verify Enrichment Success

```sql
-- Check for terms without SQL after enrichment
SELECT term, LENGTH(definition) as def_len, LENGTH(defining_sql) as sql_len
FROM engine_business_glossary
WHERE project_id = '...'
AND (defining_sql IS NULL OR defining_sql = '');

-- Should return 0 rows after fix
```

### Test Complex Metric Generation

```go
func TestEnrichTerms_ComplexMetrics(t *testing.T) {
    // Create term like "Offer Utilization Rate"
    // Run enrichment
    // Verify SQL is generated and valid
}
```

## Success Criteria

- [ ] All glossary terms have valid `defining_sql`
- [x] Terms that can't have SQL are clearly marked
- [x] `get_glossary_sql` works for all returned terms (returns enrichment_status/error for failed terms)
- [x] MCP clients can calculate any documented metric
- [x] Retry logic improves enrichment success rate
- [x] LLM prompt includes examples for complex metrics (utilization rates, participation rates, etc.)

## Affected Terms Analysis

### "Offer Utilization Rate"
- **Concept**: Percentage of offers that were used/redeemed
- **Challenge**: Requires counting used vs total offers
- **Likely SQL**: `COUNT(*) FILTER (WHERE used) / COUNT(*) * 100 FROM offers`

### "Referral Bonus Participation"
- **Concept**: Percentage/count of users participating in referral program
- **Challenge**: May require joining users and referrals tables
- **Likely SQL**: `COUNT(DISTINCT referrer_id) FROM referrals WHERE bonus_paid`

## Notes

The glossary generation is a two-phase process. Discovery creates conceptual terms, enrichment adds executable SQL. If enrichment consistently fails for certain types of metrics, consider:

1. Improving discovery prompts to suggest more concrete metrics
2. Adding SQL examples for common metric patterns
3. Creating a manual review workflow for complex metrics
4. Documenting which metric types are best suited for auto-generation

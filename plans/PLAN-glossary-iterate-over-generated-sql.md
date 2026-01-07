# PLAN: Iterative SQL Generation with Error Feedback

## Problem Statement

During glossary enrichment, the LLM generates SQL definitions for business terms. These SQL statements frequently fail validation due to:

1. **Type mismatches**: `operator does not exist: bigint = text` - comparing incompatible types
2. **Missing columns**: `column be.billed_duration_s does not exist` - LLM hallucinated or misnamed columns
3. **Function errors**: `function round(double precision, integer) does not exist` - PostgreSQL-specific syntax
4. **Schema misunderstanding**: LLM uses column names/types that don't match actual schema

Currently, failed terms are logged and left without SQL. This is a missed opportunity because:
- The error message tells us exactly what's wrong
- We can feed this back to the LLM for correction
- Persistent errors may indicate ontology inaccuracies that should be fixed

## Proposed Solution

### Phase 1: Retry Loop with Error Feedback

Add an iteration loop to `enrichSingleTerm` that retries SQL generation with error context.

```
For each term:
  1. Generate SQL via LLM
  2. Validate SQL against database
  3. If valid → save and done
  4. If invalid:
     a. Extract error details
     b. Build retry prompt with error context
     c. Retry (max 3 attempts)
  5. After max retries → mark term as "enrichment_failed" with error details
```

#### Retry Prompt Structure

```
The SQL you generated failed validation with this error:

ERROR: operator does not exist: bigint = text (SQLSTATE 42883)

This typically means you're comparing columns of incompatible types.

Here is the actual schema for the relevant tables:
[Include column names and types from column_details]

Please regenerate the SQL, ensuring type compatibility.

Previous SQL that failed:
[Include the failed SQL]
```

#### Error Categorization

| Error Pattern | Category | Retry Strategy |
|--------------|----------|----------------|
| `operator does not exist: X = Y` | Type mismatch | Include actual column types, suggest CAST |
| `column X does not exist` | Missing column | Include actual column list for table |
| `function X does not exist` | Function error | Suggest PostgreSQL-compatible alternative |
| `syntax error at or near` | Syntax error | Include SQL syntax guidance |
| `relation X does not exist` | Missing table | Include actual table list |

### Phase 2: Ontology Correction Suggestions

When errors persist after retries, analyze whether the ontology itself might be wrong.

#### Detection Logic

```
If error is "column X does not exist" AND ontology lists column X:
  → Ontology may have stale/incorrect column information

If error is type mismatch AND ontology shows different type:
  → Ontology column type may be wrong

If error references table that ontology doesn't document:
  → Ontology may be missing tables
```

#### Correction Storage

Add new table `engine_ontology_corrections`:

```sql
CREATE TABLE engine_ontology_corrections (
  id UUID PRIMARY KEY,
  project_id UUID NOT NULL REFERENCES engine_projects(id),
  ontology_id UUID NOT NULL REFERENCES engine_ontologies(id),
  correction_type VARCHAR(50) NOT NULL, -- 'column_type', 'missing_column', 'missing_table'
  table_name VARCHAR(255) NOT NULL,
  column_name VARCHAR(255),
  current_value TEXT,      -- what ontology currently says
  suggested_value TEXT,    -- what we think it should be
  evidence TEXT,           -- the error message that led to this
  status VARCHAR(20) DEFAULT 'pending', -- 'pending', 'applied', 'rejected'
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

#### Integration with Ontology Refresh

When ontology is refreshed:
1. Check for pending corrections
2. Validate corrections against fresh schema
3. Auto-apply corrections that match fresh schema
4. Surface remaining corrections for user review

### Phase 3: Learning from Successful Patterns

Store successful SQL patterns to improve future generation.

#### Pattern Extraction

When SQL validates successfully:
1. Extract patterns: JOIN styles, type casting approaches, aggregation patterns
2. Store as "known good patterns" for this project's schema

#### Pattern Injection

When generating SQL for new terms:
1. Include relevant successful patterns in the prompt
2. Example: "For this schema, successful queries cast user_id to text when joining with string columns"

## Implementation Steps

### Step 1: Add Retry Logic (Immediate)

Modify `enrichSingleTerm` in `glossary_service.go`:

```go
func (s *glossaryService) enrichSingleTerm(...) enrichmentResult {
    const maxRetries = 3
    var lastError string

    for attempt := 0; attempt < maxRetries; attempt++ {
        prompt := s.buildEnrichTermPrompt(term, ontology, entities)
        if attempt > 0 {
            prompt = s.buildRetryPrompt(term, ontology, entities, lastError, previousSQL)
        }

        // ... generate and validate SQL ...

        if testResult.Valid {
            // Success - save and return
            break
        }

        lastError = testResult.Error
        previousSQL = enrichment.DefiningSQL
    }

    if !testResult.Valid {
        // Mark as failed, store error for later analysis
        return enrichmentResult{termName: term.Term, err: fmt.Errorf("failed after %d retries: %s", maxRetries, lastError)}
    }
}
```

### Step 2: Add Error-Aware Retry Prompt

New method `buildRetryPrompt` that includes:
- The error message
- Relevant column types from `column_details`
- The failed SQL
- Specific guidance based on error category

### Step 3: Track Enrichment Failures

Add fields to `engine_business_glossary` or new table:
- `enrichment_attempts INT`
- `last_enrichment_error TEXT`
- `enrichment_status VARCHAR(20)` -- 'pending', 'success', 'failed'

### Step 4: Ontology Correction Detection (Future)

After implementing retry loop, add logic to detect when errors suggest ontology issues.

### Step 5: Pattern Learning (Future)

After gathering data on successful vs failed SQL, implement pattern extraction and injection.

## Success Metrics

1. **Enrichment success rate**: Target 90%+ terms with valid SQL (vs current ~50%)
2. **Retry efficiency**: Most failures resolved in 1-2 retries
3. **Ontology accuracy**: Corrections lead to fewer future failures

## Open Questions

1. Should we surface failed terms in the UI for manual SQL entry?
2. How aggressively should we auto-apply ontology corrections?
3. Should pattern learning be per-project or global across all projects?

## Dependencies

- None for Phase 1 (self-contained in glossary_service.go)
- Phase 2 requires new database table and migration
- Phase 3 requires pattern storage mechanism

## Estimated Effort

- Phase 1 (retry loop): 2-3 hours
- Phase 2 (ontology corrections): 4-6 hours
- Phase 3 (pattern learning): 8-12 hours

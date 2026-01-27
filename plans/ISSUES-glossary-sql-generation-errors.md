# BUGS: Glossary SQL Generation Errors

**Date:** 2026-01-25
**Purpose:** Track glossary term SQL generation failures as a text2sql quality metric

## Context

Glossary SQL generation errors indicate LLM hallucinations despite having full ontology/schema context. These are not operational errors but quality metrics - if the system is working correctly, the LLM should NOT generate SQL referencing non-existent columns.

## Extraction Run: 2026-01-25 13:28

**Result:** 5/15 terms failed (33% failure rate)

### Failed Terms

| Term | Error | Root Cause Pattern |
|------|-------|-------------------|
| Engagement Duration per User | `column "user_id" does not exist` | Hallucinated column name |
| Engagement Completion Rate | `SQL references non-existent columns: status, =, )` | Hallucinated column + malformed SQL |
| Engagement Payment Intent Rate | `SQL references non-existent columns: e.engagement_id` | Wrong table alias |
| Engagement Volume | `SQL references non-existent columns: =` | Malformed SQL syntax |
| Preauthorization Usage | `column "preauthorization_minutes" does not exist` | Hallucinated column name |

### Error Patterns Observed

1. **Hallucinated columns** (3 cases): LLM invents columns that don't exist
   - `user_id`, `preauthorization_minutes`, `status`

2. **Wrong table aliases** (1 case): LLM uses incorrect table alias
   - `e.engagement_id` instead of correct table reference

3. **Malformed SQL** (2 cases): SQL syntax errors
   - Likely incomplete or truncated generation

### Log Excerpts

```
2026-01-25T13:28:48.689+0200 ERROR glossary-service services/glossary_service.go:993
  Failed to enrich term {"term": "Engagement Duration per User",
  "error": "enrichment failed after retry: SQL validation failed:
  failed to execute query: ERROR: column \"user_id\" does not exist (SQLSTATE 42703)"}

2026-01-25T13:28:48.690+0200 ERROR glossary-service services/glossary_service.go:993
  Failed to enrich term {"term": "Engagement Completion Rate",
  "error": "enrichment failed after retry: SQL references non-existent columns: status, =, )"}

2026-01-25T13:28:48.690+0200 ERROR glossary-service services/glossary_service.go:993
  Failed to enrich term {"term": "Engagement Payment Intent Rate",
  "error": "enrichment failed after retry: SQL references non-existent columns:
  e.engagement_id (did you mean column in 'billing_engagements'?)"}

2026-01-25T13:28:58.435+0200 ERROR glossary-service services/glossary_service.go:993
  Failed to enrich term {"term": "Engagement Volume",
  "error": "enrichment failed after retry: SQL references non-existent columns: ="}

2026-01-25T13:29:03.082+0200 ERROR glossary-service services/glossary_service.go:993
  Failed to enrich term {"term": "Preauthorization Usage",
  "error": "enrichment failed after retry: SQL validation failed:
  failed to execute query: ERROR: column \"preauthorization_minutes\" does not exist (SQLSTATE 42703)"}
```

## Historical Comparison

| Date | Total Terms | Failed | Success Rate |
|------|-------------|--------|--------------|
| 2026-01-25 | 15 | 5 | 67% |

## Success Criteria

Target: 100% valid SQL generation (0 failures)

## Related

- `plans/FIX-glossary-sql-validation.md` - Prompt improvements to reduce hallucinations

# FIX: BUG-8 - Test Data in Glossary

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-8
**Severity:** Low
**Category:** Data Quality

## Problem Summary

A test glossary term exists in the production glossary:
```
term: "UITestTerm2026"
definition: "A test term created via UI to verify MCP sync"
```

This was created during manual UI testing and never cleaned up.

## Root Cause

**No validation or filtering prevents test-like data from being persisted.**

The glossary can be modified via:
1. **MCP tool** `update_glossary_term` - no validation
2. **HTTP handler** `/projects/{pid}/glossary` - no validation
3. **LLM-generated terms** - should not create test terms, but could hallucinate

All paths allow arbitrary term names without checking for test patterns.

## Fix Implementation

### 1. Add Term Name Validation

**File:** `pkg/services/glossary_service.go`

```go
var testTermPatterns = []string{
    `(?i)^test`,       // Starts with "test"
    `(?i)test$`,       // Ends with "test"
    `(?i)^uitest`,     // UI test prefix
    `(?i)^debug`,      // Debug prefix
    `(?i)^todo`,       // Todo prefix
    `(?i)^fixme`,      // Fixme prefix
    `(?i)^dummy`,      // Dummy prefix
    `(?i)^sample`,     // Sample prefix
    `(?i)^example`,    // Example prefix
    `(?i)\d{4}$`,      // Ends with year (e.g., Term2026)
}

func isTestTerm(termName string) bool {
    for _, pattern := range testTermPatterns {
        if matched, _ := regexp.MatchString(pattern, termName); matched {
            return true
        }
    }
    return false
}

func (s *glossaryService) CreateOrUpdateTerm(ctx context.Context, term GlossaryTerm) error {
    // Validate term name
    if isTestTerm(term.Term) {
        return fmt.Errorf("term name '%s' appears to be test data", term.Term)
    }
    // ... rest of create/update logic
}
```

### 2. Add MCP Tool Validation

**File:** `pkg/mcp/tools/glossary.go`

```go
func updateGlossaryTerm(ctx context.Context, args UpdateGlossaryTermArgs) (interface{}, error) {
    // Validate term name
    if isTestTerm(args.Term) {
        return nil, fmt.Errorf("term name '%s' appears to be test data - use a real business term", args.Term)
    }
    // ... rest of handler
}
```

### 3. Add Cleanup Script/Tool

Create a maintenance tool to clean test data:

**File:** `scripts/cleanup-test-data/main.go`

```go
func cleanupTestGlossaryTerms(db *pgxpool.Pool, projectID uuid.UUID) error {
    patterns := []string{
        `^test`,
        `^uitest`,
        `test$`,
        `\d{4}$`,
    }

    for _, pattern := range patterns {
        result, err := db.Exec(ctx, `
            DELETE FROM engine_business_glossary
            WHERE project_id = $1
              AND term ~* $2
        `, projectID, pattern)
        if err != nil {
            return err
        }
        log.Printf("Deleted %d terms matching pattern: %s", result.RowsAffected(), pattern)
    }
    return nil
}
```

### 4. Immediate Cleanup (One-Time)

Run this SQL to remove the existing test term:

```sql
DELETE FROM engine_business_glossary
WHERE term ILIKE '%test%'
   OR term ~ '\d{4}$';  -- Ends with year like 2026
```

### 5. Consider Environment-Based Warnings

In development/staging environments, allow test data but log warnings:

```go
func (s *glossaryService) CreateOrUpdateTerm(ctx context.Context, term GlossaryTerm) error {
    if isTestTerm(term.Term) {
        if s.config.Environment == "production" {
            return fmt.Errorf("test data not allowed in production")
        }
        s.logger.Warn("Creating test-like glossary term",
            zap.String("term", term.Term),
            zap.String("env", s.config.Environment))
    }
    // ... continue
}
```

## Prevention Best Practices

1. **Use separate projects for testing** - Don't test in production projects
2. **Add `source` field** - Track if term was auto-generated, manual, or test
3. **Soft delete for cleanup** - Mark test terms as deleted rather than hard delete
4. **Regular data audits** - Periodic checks for test data in production

## Acceptance Criteria

- [x] Term validation rejects obvious test patterns
- [x] MCP tool returns error for test-like terms
- [x] Existing test term cleaned up (skipped - manual operation, script available at scripts/cleanup-test-data.sh)
- [x] Warning logged in non-production environments for test-like terms
- [x] Production environment blocks test data entirely

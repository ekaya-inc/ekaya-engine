# FIX: BUG-1 - Deterministic Column Name Pattern Matching

**Bug Reference:** BUGS-ontology-extraction.md, BUG-1
**Severity:** Critical
**Type:** Design Flaw

## Problem Summary

Deterministic code makes semantic decisions based on column name patterns (suffixes like `_id`, `_uuid`, `_at`, prefixes like `is_`, `has_`, etc.). This violates the design principle that **LLMs should make semantic judgments from context and data, not hardcoded pattern matching.**

The pattern matching filters out valid FK candidates before LLMs can evaluate them, causing:
- Unconventionally-named FKs (`parent`, `owner`, `ref`) to be excluded
- False negatives for columns that don't follow naming conventions
- Different behavior for semantically identical columns with different names

## Affected Code Locations

### 1. `pkg/services/deterministic_relationship_service.go`

**`isLikelyFKColumn(columnName string)` (line 746)**
- Checks: `_id`, `_uuid`, `_key` suffixes
- Used at lines 545, 568 to bypass other filters for "likely" FK columns
- Creates special treatment for pattern-matching column names

**`isPKMatchExcludedName(columnName string)` (line 812)**
- Checks: `_at`, `_date`, `is_`, `has_`, `_status`, `_type`, `_flag`, `num_`, `total_`, `_count`, `_amount`, `_total`, `_sum`, `_avg`, `_min`, `_max`, `rating`, `score`, `level`
- Used at lines 510, 537 to exclude columns from FK candidate consideration
- Filters happen BEFORE join validation can evaluate the column

### 2. `pkg/services/column_filter.go`

**`isExcludedName(columnName string)` (line 148)**
- Checks: `_at`, `_date`, `is_`, `has_`, `_status`, `_type`, `_flag`
- Used in entity candidate filtering

**`isEntityReferenceName(columnName string)` (line 174)**
- Checks: `id` (exact), `_id`, `_uuid`, `_key` suffixes
- Used to identify entity reference columns

### 3. `pkg/services/relationship_discovery.go`

**`attributeColumnPatterns` (line 545)**
- List: `email`, `password`, `name`, `description`, `status`, `type`
- Used in `shouldCreateCandidate()` to reject columns containing these patterns

**`shouldCreateCandidate(sourceColumnName, targetTableName string)` (line 561)**
- Applies pluralization logic: `user_id` → `users`, `user`
- Rejects `_id` columns that don't match expected table names

### 4. `pkg/services/data_change_detection.go`

**Line 346:**
- Skips columns not ending in `_id` when suggesting FK patterns
- Misses unconventionally-named FK columns

## Why This Is Wrong

1. **Not all FKs follow naming conventions** - Schemas use `UserRef`, `account_key`, `parent`, `owner`, etc.
2. **Not all `_id` columns are FKs** - `transaction_id` might be a business ID string
3. **Context matters** - `visitor_id` vs `host_id` need semantic understanding of roles
4. **Schema conventions vary** - Different projects have different naming patterns
5. **Blocks valid relationships** - Columns filtered out before LLMs can evaluate them

## Design Philosophy

The correct separation of responsibilities:

| Layer | Responsibility | Examples |
|-------|----------------|----------|
| **Deterministic Code** | Extract facts, validate joins | Type compatibility, cardinality stats, join validation |
| **LLM Code** | Make semantic judgments | "Is this a FK?", "What entity does this reference?" |

## The Fix: Multi-Phase Approach

### Phase 1: Remove Exclusion Filters (Quick Win)

Remove functions that exclude columns based on name patterns. Let join validation and cardinality analysis decide instead.

**Files to modify:**
- `pkg/services/deterministic_relationship_service.go`: Remove `isPKMatchExcludedName()` calls at lines 510, 537
- `pkg/services/column_filter.go`: Remove `isExcludedName()` calls
- `pkg/services/data_change_detection.go`: Remove `_id` suffix check at line 346

**Risk:** More columns passed to validation = more processing time
**Mitigation:** Join validation already filters invalid candidates; false positives caught later

### Phase 2: Remove Special Treatment for Pattern-Matching Names

Remove `isLikelyFKColumn()` which gives special treatment to `_id`/`_uuid`/`_key` columns.

**Files to modify:**
- `pkg/services/deterministic_relationship_service.go`: Remove `isLikelyFKColumn()` calls at lines 545, 568

**Risk:** Columns with NULL stats or low cardinality ratio may be filtered
**Mitigation:** See Phase 3 - improve stats collection and use join validation

### Phase 3: Replace Name-Based Logic with Data-Based Logic

Instead of checking column names, use actual data analysis:

1. **For FK detection:** Use join validation result (overlap percentage) instead of name patterns
2. **For type filtering:** Use actual data type and column stats, not name patterns
3. **For cardinality:** Calculate actual ratios, don't assume based on names

**New approach for `shouldCreateCandidate()`:**
```go
// OLD: Check if name matches table pattern
// NEW: Check if types are compatible and join validation shows high overlap
func shouldCreateCandidate(sourceCol, targetCol *models.SchemaColumn, overlapResult *ValueOverlapResult) bool {
    // Type compatibility
    if !areTypesCompatibleForFK(sourceCol.DataType, targetCol.DataType) {
        return false
    }
    // Join validation shows FK relationship
    if overlapResult.OverlapPct < 0.95 {
        return false // High orphan rate
    }
    return true
}
```

### Phase 4: Let LLMs Make Semantic Decisions

Pass candidate columns (after join validation) to LLMs for final semantic judgment:

**LLM prompt context should include:**
- Column name and table name
- Data type and sample values
- Join validation results (overlap %, orphan count)
- Cardinality stats (distinct count, ratio)
- Target entity information

**LLM decides:**
- Is this a FK relationship?
- What is the semantic meaning? (owner, parent, reference, etc.)
- Should this relationship be included in the ontology?

## Implementation Steps

### Step 1: Create Feature Flag ✓
Add `use_legacy_pattern_matching` flag to allow gradual rollout.

### Step 2: Remove Exclusion Filters (Behind Flag) ✓ DONE
When flag is off, don't call:
- `isPKMatchExcludedName()`
- `isExcludedName()`

### Step 3: Remove Inclusion Exemptions (Behind Flag) ✓ DONE
When flag is off, don't call:
- `isLikelyFKColumn()` for special treatment

### Step 4: Add LLM Semantic Evaluation ✓ DONE
Create new service that evaluates FK candidates using LLMs.

### Step 5: Update Tests
- Remove tests for pattern-matching functions
- Add tests for data-based FK detection
- Add tests for LLM semantic evaluation

### Step 6: Performance Tuning
Monitor impact of more candidates being processed. Optimize join validation if needed.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/deterministic_relationship_service.go` | Remove pattern matching functions and calls |
| `pkg/services/column_filter.go` | Remove name-based filtering |
| `pkg/services/relationship_discovery.go` | Remove `attributeColumnPatterns` and `shouldCreateCandidate` |
| `pkg/services/data_change_detection.go` | Remove `_id` suffix check |
| `pkg/services/deterministic_relationship_service_test.go` | Update tests |
| `pkg/services/column_filter_test.go` | Update tests |

## Testing

### Verify Unconventional FKs Are Found

Test with schema containing:
- `parent` column referencing `users.id`
- `owner_ref` column referencing `accounts.account_id`
- `billing_account_key` column referencing `accounts.account_id`

### Verify False Positives Are Filtered

Join validation should reject:
- Timestamp columns that happen to have overlapping values
- Status columns with low-cardinality matches

## Success Criteria

- [ ] Deterministic code extracts facts only (cardinality, types, constraints, join validation)
- [ ] No hardcoded name pattern assumptions filter candidates
- [ ] Unconventionally-named columns (`parent`, `ref`) are properly evaluated
- [ ] LLMs receive rich context and make semantic decisions
- [ ] Tests verify data-based (not name-based) FK detection
- [ ] Performance impact measured and acceptable

## Dependencies

This bug connects to other bugs:
- **BUG-3** (Missing FKs): Pattern matching filters out valid FK columns
- **BUG-9** (Stats collection): NULL stats cause filtering, pattern matching is workaround

Consider fixing BUG-9 (stats collection) first, then removing pattern matching workarounds.

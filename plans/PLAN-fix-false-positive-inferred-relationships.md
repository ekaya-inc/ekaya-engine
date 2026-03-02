# PLAN: Fix False-Positive Inferred FK Relationships

**Status:** TODO
**Branch:** TBD
**Created:** 2026-03-01

## Problem

The relationship inference pipeline produces massive false positives when datasources use auto-increment integer PKs and have small integer values in non-FK columns. For example, a marketing datasource with 12 tables produced 47 inferred relationships where only ~8 are real.

### Root Cause

Three compounding failures:

1. **Column Feature Extraction misclassifies non-FK integers as `role: foreign_key`** — The numeric classifier in `column_feature_extraction.go:1604-1709` classifies columns as either `identifier` or `measure`. Any non-PK integer classified as `identifier` is automatically given `IdentifierType: ForeignKey` (lines 1693-1699). This causes columns like `week_number`, `target_week`, `day_offset`, and `task_number` to be tagged as foreign keys when they are ordinal counters or sequence numbers.

2. **Small sequential integers pass the zero-orphan filter** — The candidate collector (`relationship_candidate_collector.go:707`) requires zero orphans. When source values are small integers like {1,2,3,4,5} and every target table has auto-increment IDs starting at 1, the orphan count is always 0 and match rate is always 100%. Every source column matches every target table's PK.

3. **LLM validator prompt lacks coincidental-match guidance** — The validator system prompt (`relationship_validator.go:121-124`) says "be conservative" but doesn't warn about the small-integer overlap pattern. The older `fk_semantic_evaluation.go:266-283` system prompt explicitly warns about "coincidental matches (small integers matching unrelated lookup tables)" but that guidance was never ported to the current validator.

### Concrete Example

`content_posts.week_number` has values {1..10}. Every table's integer PK contains those values. Result: `week_number` gets inferred as FK to `post_channel_steps.id`, `marketing_task_dependencies.id`, `directory_submissions.id`, and `marketing_tasks.id` — all with match_rate=1.0 and confidence=0.9.

## Approach

Fix at all three layers (defense in depth):

### Fix 1: Improve numeric classifier to distinguish counters from FKs

The numeric classifier prompt asks the LLM to pick between `identifier` and `measure` (plus `monetary`, `percentage`, `count`). It needs a finer distinction: an integer can be an identifier that is NOT a foreign key (e.g., `task_number` is a local sequence, `week_number` is an ordinal). The code then auto-promotes all non-PK `identifier` results to `IdentifierType: ForeignKey`.

**Approach:** Add `ordinal` or `sequence` as a numeric classification type alongside `identifier`. Columns classified as ordinal/sequence should get `purpose: identifier` but NOT `role: foreign_key` and NOT `NeedsFKResolution: true`. The LLM prompt should provide examples of ordinals vs FKs (e.g., `week_number` = ordinal, `app_id` = FK reference).

### Fix 2: Add multi-target detection in candidate collector

A real FK column points to exactly one table. If a source column passes the zero-orphan filter for >2 target tables, the matches are almost certainly coincidental (small integer overlap). This is a cheap, deterministic check.

**Approach:** After collecting all valid candidates, group by source column. If a source column has >2 targets, flag all candidates for that source as suspicious. Either reject them outright or raise the LLM confidence threshold for those candidates (e.g., require 0.95 instead of 0.7).

### Fix 3: Improve LLM validator prompt with coincidental-match guidance

Port the wisdom from `fk_semantic_evaluation.go`'s system prompt into `relationship_validator.go`.

**Approach:** Update the system message and/or validation prompt to explicitly warn about:
- Small sequential integers coincidentally matching auto-increment PKs
- Columns named with ordinal/temporal patterns (`week_*`, `*_number`, `*_offset`, `day_*`, `step_*`) are unlikely FKs
- When a column matches many unrelated tables, it's almost certainly coincidental
- Include the reverse-orphan ratio as a signal: if the target has 25 IDs and the source only references 4 of them (16% coverage), that's weak evidence for a FK

## File-by-File Changes

### 1. `pkg/services/column_feature_extraction.go`

**Lines 1610-1651 (numeric classifier prompt):** Update the classification types to include `ordinal` or `sequence`:
```
- `identifier`: Numeric ID that references another entity (e.g., user_id, app_id, parent_id)
- `ordinal`: Sequential/positional number (e.g., week_number, step_number, day_offset, sort_order, position)
- `measure`: Quantitative value (amount, cost, rate, score, duration)
- `monetary`: Money amount
- `percentage`: Percentage or ratio
- `count`: Integer count of items
```

The prompt should include guidance like: "Columns named *_number, *_week, *_offset, *_order, *_position, *_step, *_rank are typically ordinals, not foreign keys. An identifier references a specific row in another table (e.g., user_id references users). An ordinal represents a position or sequence within the current table's context."

**Lines 1667-1699 (FK inference from classification):** Add handling for `ordinal` type — it should set `purpose: identifier` (it IS a numeric identifier) but NOT set `role: foreign_key` and NOT set `NeedsFKResolution: true`:
```go
if response.NumericType == "identifier" {
    purpose = models.PurposeIdentifier
    if profile.IsPrimaryKey {
        role = models.RolePrimaryKey
    }
} else if response.NumericType == "ordinal" {
    purpose = models.PurposeIdentifier
    role = models.RoleAttribute  // NOT foreign_key
    // Do NOT set NeedsFKResolution
}
```

And the existing FK-flagging block (lines 1693-1699) must exclude `ordinal`:
```go
if response.NumericType == "identifier" && !profile.IsPrimaryKey {
    features.NeedsFKResolution = true
    ...
}
// ordinal does NOT get here
```

**Also update the response struct** for the numeric classifier — currently `NumericType` accepts `identifier|measure|monetary|percentage|count`. Add `ordinal` to the valid values. Check the struct definition (likely in `column_feature_extraction.go` or `models/column_features.go`).

### 2. `pkg/services/relationship_candidate_collector.go`

**After line 739 (end of join statistics loop), before returning `validCandidates`:** Add a multi-target filter:

```go
// Filter: If a source column matches >2 targets, the overlap is likely
// coincidental (small integers matching auto-increment PKs everywhere).
// Group candidates by source column and reject if too many targets.
validCandidates = c.filterMultiTargetCandidates(validCandidates)
```

New method `filterMultiTargetCandidates`:
- Group candidates by `(SourceTable, SourceColumn)`
- If a source column has >2 target matches, log a warning and remove all candidates for that source column
- Exception: keep candidates where inference_method is `column_features` (these came from Phase 4 FK resolution which already did LLM-based target selection)

Note: The threshold of 2 is chosen because a legitimate FK column might reasonably reference one table's PK and one table's unique constraint (rare but possible). Matching 3+ targets is virtually always coincidental overlap.

### 3. `pkg/services/relationship_validator.go`

**Lines 121-124 (system message):** Expand with coincidental-match guidance:

```go
func (v *relationshipValidator) systemMessage() string {
    return `You are a database schema analyst. Your task is to determine if a candidate foreign key relationship is valid.
Analyze the column metadata, sample values, and join statistics to make your decision.
Be conservative - only confirm relationships where there is strong evidence the columns represent a true FK-PK relationship.

IMPORTANT - Common false positive patterns to reject:
- Small sequential integers (1, 2, 3, ...) will coincidentally match auto-increment PKs in many tables. This is NOT evidence of a relationship. A column with values {1,2,3,4,5} matching a target with IDs {1..25} is almost certainly coincidental.
- Columns named with ordinal/temporal patterns (week_number, day_offset, step_number, sort_order, position) are counters, not foreign keys.
- Low target coverage (source references <30% of target values) combined with small source distinct count (<20) is weak evidence.
- Column names should semantically relate: app_id -> applications.id makes sense; week_number -> post_channel_steps.id does NOT.

Respond with valid JSON only.`
}
```

**Lines 128-228 (buildValidationPrompt):** Add a "suspicion signal" section when the data looks like small-integer overlap. After the Join Analysis Results section, add:

```go
// Add warning signals for the LLM
if candidate.SourceDistinctCount > 0 && candidate.SourceDistinctCount <= 20 {
    coverageRate := float64(candidate.TargetMatched) / float64(candidate.TargetDistinctCount) * 100
    if coverageRate < 50 {
        sb.WriteString("\n## Warning Signals\n\n")
        sb.WriteString("- Source has very few distinct values (possible small-integer overlap)\n")
        sb.WriteString(fmt.Sprintf("- Source only covers %.1f%% of target values\n", coverageRate))
        sb.WriteString("- This pattern is common for coincidental matches with auto-increment PKs\n")
    }
}
```

### 4. Tests

**`pkg/services/relationship_candidate_collector_test.go`:**
- Add test for `filterMultiTargetCandidates` — verify that a source column matching 3+ targets has all candidates removed
- Add test for a source column matching exactly 1-2 targets being kept
- Add integration-style test with small sequential integers matching multiple PK columns

**`pkg/services/relationship_validator_test.go`:**
- No structural test changes needed (the LLM prompt changes are tested via the existing mock LLM pattern)

**`pkg/services/column_feature_extraction_test.go`:**
- Add test cases for the numeric classifier returning `ordinal` for columns like `week_number`, `day_offset`
- Verify that `ordinal` columns do NOT get `NeedsFKResolution: true`
- Verify that `ordinal` columns do NOT get `role: foreign_key`

## Checklist

- [ ] Add `ordinal` numeric classification type to the LLM prompt in `column_feature_extraction.go`
- [ ] Update numeric classifier response handling to NOT flag ordinals as FK candidates
- [ ] Add `filterMultiTargetCandidates` method to `relationship_candidate_collector.go`
- [ ] Update LLM system prompt in `relationship_validator.go` with coincidental-match guidance
- [ ] Add warning signals section to `buildValidationPrompt` for small-integer patterns
- [ ] Add tests for multi-target filtering in `relationship_candidate_collector_test.go`
- [ ] Add tests for ordinal classification in `column_feature_extraction_test.go`
- [ ] Run full test suite: `go test ./pkg/services/... -run "Relationship|Candidate|ColumnFeature"`

## Notes

- Fix 2 (multi-target filter) is the most impactful and lowest-risk change — it's purely deterministic and catches the exact pattern observed
- Fix 1 (ordinal classification) prevents the problem at the source but requires LLM prompt tuning and may need iteration
- Fix 3 (validator prompt) is defense-in-depth — even if a bad candidate slips through, the LLM should catch it
- The existing `fk_semantic_evaluation.go` is part of a deprecated pipeline and should not be modified
- The `preserveColumnFeaturesFKs` method in `relationship_discovery_service.go:296` has a confidence threshold of 0.8 — candidates from Phase 4 FK resolution that pass this threshold bypass the candidate collector entirely, so Fix 2 won't affect them

# PLAN: Fix Relationship Cardinality Inference

**Date:** 2026-03-02
**Status:** DONE
**Priority:** MEDIUM

## Problem

The relationship validator relies entirely on the LLM to determine cardinality (`1:1`, `N:1`, `1:N`, `N:M`). Despite the prompt hinting "most FKs are N:1", the LLM frequently returns incorrect cardinality. In the Ekaya Marketing datasource, 6 of 9 relationships have wrong cardinality:

| Relationship | LLM Says | Correct | Why Wrong |
|---|---|---|---|
| `content_posts.app_id → applications.id` | N:M | N:1 | Each post has one app |
| `marketing_tasks.app_id → applications.id` | N:M | N:1 | Each task has one app |
| `post_channel_steps.post_id → content_posts.id` | N:M | N:1 | Each step has one post |
| `directory_submissions.directory_id → mcp_directories.id` | 1:1 | N:1 | Multiple submissions per directory |
| `marketing_task_dependencies.blocking_task_id → marketing_tasks.id` | N:M | N:1 | Each dependency row references one task |
| `marketing_task_dependencies.blocked_task_id → marketing_tasks.id` | N:M | N:1 | Each dependency row references one task |

## Root Cause

Cardinality is a **deterministic, data-driven property** but is being delegated to the LLM as a free-text classification. The join statistics already collected during candidate validation contain all the information needed to compute cardinality without LLM involvement.

### How Cardinality Works

For a FK relationship `source.column → target.column`:

- **N:1** (many-to-one): Multiple source rows can reference the same target row. This is the standard FK pattern. Determined by: `source_distinct < target_distinct` or source column is NOT unique.
- **1:1** (one-to-one): Each source row references a unique target row AND each target row is referenced at most once. Determined by: source column has a UNIQUE constraint, or `source_distinct == source_matched` (no duplicates in source referencing same target).
- **1:N** (one-to-many): Reverse of N:1. Rare in FK direction — would mean the FK is on the "one" side. Determined by: `target_matched < source_matched` with duplicates on the target side.
- **N:M** (many-to-many): Only valid for junction/bridge tables where both columns are FKs. Determined by: both source and target have duplicates in the join. Almost never correct for a single FK column.

### The Key Signal

For a FK column pointing at a PK:
- If the **source column is NOT a primary key and NOT unique**, cardinality is always **N:1**. Multiple source rows can have the same FK value pointing to one target row. This covers the vast majority of FK relationships.
- If the source column IS unique (UNIQUE constraint), then it's **1:1**.
- **N:M** should only apply to junction tables where BOTH columns in the relationship are FKs — a single FK column by definition can only hold one value per row, so from source→target it's always N:1 or 1:1.

## Solution

Compute cardinality deterministically from schema constraints and join statistics. Remove it from LLM responsibility.

### Deterministic Cardinality Rules

```
IF source column has UNIQUE constraint or is PK:
    IF target_matched == source_matched (1:1 mapping):
        cardinality = "1:1"
    ELSE:
        cardinality = "N:1"  // unique source but multiple sources per target
ELSE:
    cardinality = "N:1"  // non-unique source = many rows can reference same target
```

N:M is never inferred for a single FK relationship. N:M only makes sense for junction tables, and the system already identifies junction tables via `table_type = 'junction'`. If needed in the future, N:M can be synthesized by combining two N:1 relationships through a junction table.

### Where to Apply

**Option A (recommended): Post-LLM override.** After the LLM returns its validation result, overwrite the `cardinality` field with the deterministic value. This preserves the LLM's `is_valid_fk`, `confidence`, `reasoning`, and `source_role` (which are genuinely useful) while fixing only the cardinality.

**Option B: Remove from LLM prompt.** Remove `cardinality` from the LLM response format entirely and compute it separately. Cleaner but a larger change to the prompt and response parsing.

Option A is recommended because it's minimal, safe, and doesn't require prompt iteration.

## File-by-File Changes

### 1. `pkg/services/relationship_validator.go`

After parsing the LLM response (line ~257, after `parseValidationResponse`), add deterministic cardinality override:

```go
// Override LLM cardinality with deterministic computation.
// The LLM frequently gets cardinality wrong because it's a data property,
// not a semantic judgment. Schema constraints and join stats are authoritative.
if response.IsValidFK {
    response.Cardinality = v.computeCardinality(candidate)
}
```

New method `computeCardinality`:

```go
func (v *relationshipValidator) computeCardinality(candidate *RelationshipCandidate) string {
    // A unique or PK source column means at most one source row per target row
    if candidate.SourceIsPK || candidate.SourceIsUnique {
        // Check if it's truly 1:1 (each target referenced exactly once)
        if candidate.TargetDistinctCount > 0 &&
           candidate.SourceMatched == candidate.TargetMatched {
            return "1:1"
        }
        // Unique source but not all targets referenced — still N:1 from source perspective
        return "N:1"
    }
    // Non-unique, non-PK source column: multiple source rows can reference same target
    return "N:1"
}
```

Also need to ensure `SourceIsUnique` is available on `RelationshipCandidate`. Check if it exists.

### 2. `pkg/services/relationship_candidate_collector.go`

Verify that `RelationshipCandidate` struct includes `SourceIsUnique` (or `SourceIsUniqueConstraint`). If not, add it and populate from `SchemaColumn.IsUnique` during candidate construction.

### 3. Tests

**`pkg/services/relationship_validator_test.go`:**
- Add test: non-PK, non-unique source → cardinality is always N:1
- Add test: unique source with 1:1 mapping → cardinality is 1:1
- Add test: unique source without 1:1 mapping → cardinality is N:1
- Add test: LLM returns N:M but source is non-unique → overridden to N:1

## Checklist

- [x] Add `computeCardinality` method to `relationship_validator.go`
- [x] Add deterministic override after LLM response parsing in `Validate()`
- [x] Ensure `SourceIsUnique` field exists on `RelationshipCandidate` and is populated
- [x] Add unit tests for `computeCardinality` covering N:1, 1:1, and override cases
- [x] Run test suite: `go test ./pkg/services/... -run "Relationship|Validator"`
- [x] Verify against Ekaya Marketing: all 6 wrong cardinalities should now be N:1

## Notes

- This does NOT require re-extracting the ontology for existing projects. A future enhancement could add a one-time migration or MCP tool to recompute cardinality for existing relationships.
- The LLM prompt can optionally be updated to remove the `cardinality` field from the response format (Option B), but this is not required for the fix to work. The override approach is less risky.
- N:M relationships are a higher-level concept (two N:1 relationships through a junction table) and should be synthesized at the API/presentation layer if needed, not inferred per-relationship.

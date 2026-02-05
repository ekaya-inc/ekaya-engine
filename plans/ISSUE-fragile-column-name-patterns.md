# ISSUE: Fragile Column Name Pattern Matching

**Status:** PARTIALLY OBSOLETE (2026-02-05)

Many files referenced have been **removed** (entity_promotion.go, etc.). Remaining patterns:
- `relationship_discovery.go:574` - attributeColumnPatterns (still exists)
- `column_enrichment.go:482` - enumPatterns (still exists)
- `glossary_service.go` - various patterns (still exist)

Lower priority since ColumnFeatures pipeline now handles most classification.

## Summary

Multiple services use string pattern matching on column/table names to classify or make decisions. This is fragile because it relies on naming conventions that vary across databases and teams. The `column_feature_extraction` pipeline was designed to replace this approach, but adoption is incomplete.

## Context

The codebase has a `ColumnFeatures` model and `column_feature_extraction` service that analyze columns using data sampling and LLM classification. Some files have already removed name-based patterns in favor of this approach (see `column_filter.go:155-157` and `deterministic_relationship_service.go:1109-1110`).

Each instance below should be reviewed to determine:
1. Can this be replaced with `ColumnFeatures` data?
2. Is this a reasonable heuristic that should remain (with documentation)?
3. Should this be moved to a centralized location if it must exist?

---

## Instance 1: relationship_discovery.go - Attribute Column Blocklist

**File:** `pkg/services/relationship_discovery.go`
**Lines:** 545-552

```go
var attributeColumnPatterns = []string{
    "email",
    "password",
    "name",
    "description",
    "status",
    "type",
}
```

**What it does:** Excludes columns from FK consideration if their name contains these strings.

**Why it's fragile:** A column named `user_type_id` would be incorrectly excluded. A column named `category` that should be excluded isn't in the list.

---

## Instance 2: relationship_discovery.go - FK Suffix Detection

**File:** `pkg/services/relationship_discovery.go`
**Lines:** 573

```go
strings.HasSuffix(sourceLower, "_id")
```

**What it does:** Identifies potential FK columns by `_id` suffix.

**Why it's fragile:** Not all FKs end in `_id` (e.g., `user_ref`, `account_key`, `parent`). Not all `_id` columns are FKs (e.g., `external_id`, `correlation_id`).

---

## Instance 3: relationship_discovery.go - Plural Conversion

**File:** `pkg/services/relationship_discovery.go`
**Lines:** 586

```go
strings.HasSuffix(entityName, "y")
```

**What it does:** Detects words ending in "y" for plural conversion (category → categories).

**Why it's fragile:** Naive English pluralization. Fails for "key" → "keys", works incorrectly for proper nouns.

---

## Instance 4: glossary_service.go - FK Column Detection

**File:** `pkg/services/glossary_service.go`
**Lines:** 1688

```go
if strings.HasSuffix(strings.ToLower(col.ColumnName), "_id") || len(numericExamples) < 2 {
```

**What it does:** Uses `_id` suffix to determine if a column is likely an FK for glossary processing.

**Why it's fragile:** Same issues as Instance 2.

---

## Instance 5: glossary_service.go - Hardcoded Role Patterns

**File:** `pkg/services/glossary_service.go`
**Lines:** 1829-1834

```go
rolePatterns := []string{
    "host_id", "visitor_id", "creator_id", "viewer_id",
    "buyer_id", "seller_id", "sender_id", "receiver_id",
    "payer_id", "payee_id", "owner_id", "member_id",
    "author_id", "performer_id", "attendee_id", "participant_id",
}
```

**What it does:** Identifies role-indicating columns by exact name match.

**Why it's fragile:** Limited to this specific list. Misses `requester_id`, `approver_id`, `assignee_id`, etc. Also assumes exact naming.

---

## Instance 6: glossary_service.go - Role Keywords in Association

**File:** `pkg/services/glossary_service.go`
**Lines:** 1852-1854

```go
if strings.Contains(assocLower, "host") || strings.Contains(assocLower, "visitor") || ...
```

**What it does:** Checks FKAssociation string for role keywords.

**Why it's fragile:** A table named `ghost_records` would incorrectly match "host".

---

## Instance 7: glossary_service.go - Timestamp Suffix Matching

**File:** `pkg/services/glossary_service.go`
**Lines:** 2603

```go
if strings.HasSuffix(column, "_at") && strings.HasSuffix(validCol, "_at") {
```

**What it does:** Matches timestamp columns by `_at` suffix.

**Why it's fragile:** Not all timestamps end in `_at` (e.g., `last_login`, `birth_date`, `created_timestamp`).

---

## Instance 8: data_change_detection.go - FK Fallback Detection

**File:** `pkg/services/data_change_detection.go`
**Lines:** 355-362

```go
if features == nil && !strings.HasSuffix(col.ColumnName, "_id") {
    continue
}
if strings.HasSuffix(col.ColumnName, "_id") {
    potentialTableBase = strings.TrimSuffix(col.ColumnName, "_id")
}
```

**What it does:** Falls back to `_id` suffix when ColumnFeatures is not available.

**Why it's fragile:** Same issues as Instance 2. Note: This is explicitly a fallback, which may be acceptable.

---

## Instance 9: column_enrichment.go - Enum Column Detection

**File:** `pkg/services/column_enrichment.go`
**Lines:** 470-481

```go
enumPatterns := []string{"status", "state", "type", "kind", "category", "_code", "level", "tier", "role"}
...
if strings.Contains(colNameLower, pattern) {
    candidates = append(candidates, col)
    break
}
```

**What it does:** Identifies potential enum columns by name patterns.

**Why it's fragile:** A column named `description` containing "type" would match. Misses enum columns with other names like `priority`, `severity`, `phase`.

---

## Instance 10: column_enrichment.go - Completion Timestamp Patterns

**File:** `pkg/services/column_enrichment.go`
**Lines:** 562-565, 585

```go
completionPatterns := []string{
    "completed_at", "finished_at", "ended_at", "closed_at",
    "done_at", "resolved_at", "fulfilled_at", "success_at",
}
...
if strings.Contains(nameLower, "complet") || strings.Contains(nameLower, "finish") {
    return col.ColumnName
}
```

**What it does:** Identifies completion timestamp columns by name patterns.

**Why it's fragile:** Limited list. The `Contains("complet")` fallback would match `incomplete_count`.

---

## Instance 11: ontology_finalization.go - Currency Column Detection

**File:** `pkg/services/ontology_finalization.go`
**Lines:** 721-732

```go
currencyPatterns := []string{"_amount", "_price", "_cost", "_total", "_fee"}
...
if strings.HasSuffix(colNameLower, pattern) {
```

**What it does:** Identifies currency/monetary columns by suffix.

**Why it's fragile:** Misses `revenue`, `balance`, `salary`, `wage`. Would incorrectly match `item_count_total`.

---

## Instance 12: ontology_finalization.go - Soft Delete Pattern Detection

**File:** `pkg/services/ontology_finalization.go`
**Lines:** 658-684

```go
patterns := []softDeletePattern{
    {"deleted_at", "timestamp", "deleted_at IS NULL"},
    {"archived_at", "timestamp", "archived_at IS NULL"},
    {"removed_at", "timestamp", "removed_at IS NULL"},
    {"deleted", "boolean", "deleted = false"},
    {"is_deleted", "boolean", "is_deleted = false"},
    {"is_active", "boolean", "is_active = true"},
    {"active", "boolean", "active = true"},
    {"status", "string", "status != 'deleted'"},
}
...
if colNameLower == pattern.column { ... }
```

**What it does:** Identifies soft delete columns by exact name match + data type.

**Why it's fragile:** Limited to exact names. Misses `deactivated_at`, `hidden`, `is_archived`, `record_status`.

---

## Instance 13: entity_promotion.go - FK Suffix for Role Derivation

**File:** `pkg/services/entity_promotion.go`
**Lines:** 173-178

```go
suffixes := []string{"_id", "_uuid", "_fk"}
for _, suffix := range suffixes {
    if strings.HasSuffix(name, suffix) {
        name = strings.TrimSuffix(name, suffix)
        break
    }
}
```

**What it does:** Strips FK suffixes to derive role name from column name.

**Why it's fragile:** Limited suffix list. Would fail on `user_ref`, `account_key`.

---

## Instance 14: entity_promotion.go - Generic Column Name Blocklist

**File:** `pkg/services/entity_promotion.go`
**Lines:** 182-189

```go
genericNames := map[string]bool{
    "id":        true,
    "uuid":      true,
    "key":       true,
    "ref":       true,
    "reference": true,
    "fk":        true,
    "foreign":   true,
}
```

**What it does:** Blocks generic column names from being used as role names.

**Why it's fragile:** This is actually reasonable - these ARE generic names. May be acceptable.

---

## Files Already Cleaned Up (For Reference)

These files have removed name-based patterns and now use ColumnFeatures:

**`pkg/services/column_filter.go:155-157`**
```go
// NOTE: isExcludedName() and isEntityReferenceName() have been removed.
// Column filtering now uses stored ColumnFeatures.Purpose from the feature extraction pipeline
// instead of name-based pattern matching.
```

**`pkg/services/deterministic_relationship_service.go:1109-1110`**
```go
// NOTE: isLikelyFKColumn has been removed. Column classification is now handled by the
// column_feature_extraction service.
```

---

## Recommended Review Process

For each instance:
1. [ ] Is `ColumnFeatures` data available at this point in the pipeline?
2. [ ] If yes, can this be replaced with a `ColumnFeatures` field check?
3. [ ] If no, is this a pre-feature-extraction step that must use heuristics?
4. [ ] If heuristics are required, should they be centralized in one place?
5. [ ] Document the decision (keep/remove/refactor) with rationale.

# FIX: BUG-9 - Missing Real Foreign Key Relationships

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-9
**Severity:** High
**Category:** Relationship Discovery

## Problem Summary

The "Billing Engagement" entity has **ZERO relationships** despite having obvious foreign keys:

| Missing Relationship | From | To |
|---------------------|------|-----|
| ✗ | billing_engagements.visitor_id | users.user_id |
| ✗ | billing_engagements.host_id | users.user_id |
| ✗ | billing_engagements.session_id | sessions.session_id |
| ✗ | billing_engagements.offer_id | offers.offer_id |

**Key Context:** Tikr uses text UUIDs without database-level FK constraints (soft FKs).

## Root Cause Analysis

### 1. Cardinality Filter Too Aggressive

**File:** `pkg/services/deterministic_relationship_service.go:554-564`

```go
// PROBLEM: Requires 5% cardinality ratio
if table.RowCount != nil && *table.RowCount > 0 {
    ratio := float64(*col.DistinctCount) / float64(*table.RowCount)
    if ratio < 0.05 {  // 5% THRESHOLD
        continue  // Skipped!
    }
}
```

**Example:** If `billing_engagements` has 100,000 rows but only 500 unique visitors:
- Cardinality = 500/100,000 = 0.5%
- Below 5% threshold → **visitor_id filtered out**

This makes sense for excluding statuses/enums, but it also rejects valid FK columns.

### 2. IsJoinable=nil Filtering

**File:** `pkg/services/deterministic_relationship_service.go:542-545`

```go
// Require explicit joinability determination
if col.IsJoinable == nil || !*col.IsJoinable {
    continue  // Skipped if not marked joinable!
}
```

For text UUID columns, `IsJoinable` may be nil initially before stats collection.

### 3. Type Matching Requirement

**File:** `pkg/services/deterministic_relationship_service.go:587-588`

```go
refType := ref.column.DataType
candidates := candidatesByType[refType]  // Must be SAME type
```

If `billing_engagements.visitor_id` is `text` but `users.user_id` is `uuid`, no match.

### 4. No Semantic Column Name Parsing

The system doesn't understand that `visitor_id` references the "user" entity:
- `visitor_id` → needs to recognize "visitor" as a role of "user"
- `host_id` → needs to recognize "host" as a role of "user"

No logic parses role-prefixed column names to find their target entity.

## Fix Implementation

### 1. Relax Cardinality Threshold

**File:** `pkg/services/deterministic_relationship_service.go`

```go
// Change from 5% to 0.1% (1 in 1000 ratio)
// Or remove the ratio check entirely for _id columns
if ratio < 0.001 {  // Was 0.05
    continue
}

// Better: Skip ratio check for likely FK columns
if strings.HasSuffix(col.ColumnName, "_id") {
    // _id columns are likely FKs even with low cardinality
    // Let the actual join validation decide
} else if ratio < 0.05 {
    continue
}
```

### 2. Allow IsJoinable=nil for _id Columns

```go
// For _id columns, assume joinable if stats unknown
if col.IsJoinable == nil {
    if strings.HasSuffix(col.ColumnName, "_id") {
        // Likely FK column - include for validation
    } else {
        continue
    }
}
```

### 3. Handle Type Compatibility

**File:** `pkg/services/deterministic_relationship_service.go`

Add type compatibility mapping:

```go
func areTypesCompatibleForFK(sourceType, targetType string) bool {
    source := strings.ToLower(sourceType)
    target := strings.ToLower(targetType)

    // Exact match
    if source == target {
        return true
    }

    // UUID compatibility: text ↔ uuid
    uuidTypes := []string{"uuid", "text", "varchar", "character varying"}
    sourceIsUUID := containsAny(source, uuidTypes)
    targetIsUUID := containsAny(target, uuidTypes)
    if sourceIsUUID && targetIsUUID {
        return true
    }

    // Integer compatibility
    intTypes := []string{"int", "integer", "bigint", "smallint", "serial"}
    sourceIsInt := containsAny(source, intTypes)
    targetIsInt := containsAny(target, intTypes)
    if sourceIsInt && targetIsInt {
        return true
    }

    return false
}
```

Then use this instead of exact type matching:

```go
// Line 587-588: Replace candidatesByType lookup
for _, ref := range entityReferenceColumns {
    for _, cand := range allCandidates {
        if areTypesCompatibleForFK(cand.DataType, ref.column.DataType) {
            // Create relationship candidate
        }
    }
}
```

### 4. Add Semantic Column Name Parsing

**New:** Recognize role-prefixed column names:

```go
// Map column names to potential target entities
func inferTargetEntity(columnName string, entities map[string]*OntologyEntity) *OntologyEntity {
    // Try direct match: user_id -> user
    baseName := strings.TrimSuffix(columnName, "_id")
    if e, ok := entities[baseName]; ok {
        return e
    }
    if e, ok := entities[baseName+"s"]; ok {  // Plural
        return e
    }

    // Try role mapping: visitor_id, host_id -> user
    knownRoles := map[string]string{
        "visitor":  "user",
        "host":     "user",
        "creator":  "user",
        "owner":    "user",
        "assignee": "user",
        "payer":    "user",
        "payee":    "user",
        "buyer":    "user",
        "seller":   "user",
    }

    if targetEntity, ok := knownRoles[baseName]; ok {
        if e, ok := entities[targetEntity]; ok {
            return e
        }
        if e, ok := entities[targetEntity+"s"]; ok {
            return e
        }
    }

    return nil
}
```

### 5. Add FK Candidate Pre-Filtering by Naming Convention

Before cardinality checks, identify likely FK columns by naming:

```go
func isLikelyFKColumn(col *SchemaColumn) bool {
    name := strings.ToLower(col.ColumnName)

    // Explicit FK patterns
    if strings.HasSuffix(name, "_id") ||
       strings.HasSuffix(name, "_uuid") ||
       strings.HasSuffix(name, "_key") {
        return true
    }

    // Role-based patterns
    rolePatterns := []string{"visitor_", "host_", "owner_", "creator_", "payer_", "payee_"}
    for _, p := range rolePatterns {
        if strings.HasPrefix(name, p) {
            return true
        }
    }

    return false
}

// Then in candidate filtering:
if isLikelyFKColumn(col) {
    // Skip cardinality filtering - let join validation decide
    candidates = append(candidates, col)
    continue
}
```

## Testing

Create test with:
```sql
CREATE TABLE users (user_id text PRIMARY KEY);
CREATE TABLE billing_engagements (
    engagement_id text PRIMARY KEY,
    visitor_id text,  -- FK to users.user_id
    host_id text,     -- FK to users.user_id
    session_id text   -- FK to sessions.session_id
);
```

Run relationship discovery and verify:
- ✓ `visitor_id → users.user_id` discovered
- ✓ `host_id → users.user_id` discovered
- ✓ Relationship roles captured (visitor, host)

## Acceptance Criteria

- [x] Text UUID columns treated as FK candidates
- [x] Cardinality filter relaxed for _id columns
- [ ] Type compatibility includes text ↔ uuid
- [ ] Role-prefixed columns (visitor_id, host_id) mapped to target entity
- [ ] Billing Engagement entity has 4+ relationships after extraction
- [ ] FK relationships include role annotations (visitor, host, owner, etc.)

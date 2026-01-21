# FIX: BUG-4 - Spurious Column-to-Column Relationships

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-4
**Severity:** High
**Category:** Relationship Discovery

## Problem Summary

The extraction creates false relationships based on column name matching and value overlap rather than actual FK constraints. Examples:

| Issue | From | To | Problem |
|-------|------|----|---------|
| 4a | accounts.email | account_authentications.email | Not a FK, just stores related data |
| 4b | account_id | channel_id | Different ID types, not related |
| 4c | accounts.account_id | account_password_resets.account_id | Reversed direction |
| 4d | channels.channel_id | account_authentications | Bogus target |

## Root Cause

### 1. Candidate Generation Too Permissive

**File:** `pkg/services/relationship_discovery.go:250-293`

```go
// Creates O(n²) candidates - every joinable column with every PK column
for _, source := range allJoinableColumns {
    for _, pkCols := range pkColumnsByTable {
        for _, targetPK := range pkCols {
            candidates = append(candidates, &relationshipCandidate{...})
        }
    }
}
```

No pre-filtering for:
- Column name semantics (email → email shouldn't be candidate)
- Entity domain matching (account_id → channel_id shouldn't be candidate)

### 2. Validation Thresholds Too Weak

**File:** `pkg/services/relationship_discovery.go:19-40`

```go
const (
    DefaultMatchThreshold      = 0.70  // Only 70% overlap required!
    DefaultOrphanRateThreshold = 0.50  // 50% orphans allowed!
)
```

**Problem:** Coincidental value overlap can exceed 70%. True FKs should have:
- ~100% match (all FK values exist in PK)
- 0% orphan rate (no dangling references)

### 3. No Semantic Column Name Validation

**File:** `pkg/services/relationship_discovery.go:523-531`

```go
func areTypesCompatible(sourceType, targetType string) bool {
    return normalizeType(sourceType) == normalizeType(targetType)
}
```

Only checks type compatibility, not semantic compatibility:
- `email` (text) → `email` (text) = type match ✓ but semantically wrong
- `account_id` → `channel_id` = both integers ✓ but domain mismatch

### 4. No FK Direction Validation

The algorithm doesn't verify:
- Which table owns the FK (the "child" table)
- Which table is the reference target (the "parent" table)
- N:1 cardinality pattern enforcement

### 5. Missing Attribute vs Identifier Distinction

Columns like `email`, `password`, `status`, `description` are **data attributes**, not FK candidates. No exclusion logic exists.

## Fix Implementation

### 1. Tighten Validation Thresholds

**File:** `pkg/services/relationship_discovery.go`

```go
const (
    DefaultMatchThreshold      = 0.95  // 95% match (allow 5% for data issues)
    DefaultOrphanRateThreshold = 0.10  // Max 10% orphans
)
```

### 2. Add Semantic Column Name Validation

```go
// Before creating candidate
func shouldCreateCandidate(source, target ColumnInfo) bool {
    // Rule 1: *_id columns should match their table
    if strings.HasSuffix(source.Name, "_id") {
        expectedTable := strings.TrimSuffix(source.Name, "_id") + "s"  // user_id → users
        if target.TableName != expectedTable && target.TableName != strings.TrimSuffix(source.Name, "_id") {
            return false  // user_id shouldn't point to channels
        }
    }

    // Rule 2: Attribute columns are not FK sources
    attributePatterns := []string{"email", "password", "name", "description", "status", "type"}
    for _, attr := range attributePatterns {
        if strings.Contains(strings.ToLower(source.Name), attr) {
            return false  // emails, passwords aren't FKs
        }
    }

    return true
}
```

### 3. Add FK Direction Validation

Validate cardinality: FK side should have many-to-one relationship.

```go
func validateFKDirection(ctx context.Context, source, target ColumnInfo, ds datasource.Datasource) bool {
    // Count distinct values
    sourceDistinct := ds.CountDistinct(ctx, source.Table, source.Name)
    targetDistinct := ds.CountDistinct(ctx, target.Table, target.Name)
    sourceTotal := ds.CountRows(ctx, source.Table)
    targetTotal := ds.CountRows(ctx, target.Table)

    // FK pattern: source has fewer or equal distinct values than target
    // And source total >= target total (child table has more rows)
    if sourceDistinct > targetDistinct {
        return false  // Wrong direction: source has more distinct = likely the PK side
    }

    // Validate referential integrity: all source values exist in target
    orphanCount := ds.CountOrphans(ctx, source.Table, source.Name, target.Table, target.Name)
    if float64(orphanCount)/float64(sourceTotal) > 0.05 {
        return false  // More than 5% orphans = probably not a real FK
    }

    return true
}
```

### 4. Stricter PK-Only Targets

Only create relationships where target is actually a primary key:

```go
func (s *relationshipDiscoveryService) createCandidates(...) {
    for _, source := range allJoinableColumns {
        // Only consider actual PKs as targets, not just unique columns
        for _, target := range pkColumns {  // Not all "potentially unique" columns
            if !target.IsPrimaryKey {
                continue  // Skip non-PK columns
            }
            // ... validation ...
        }
    }
}
```

### 5. Add Column Semantic Exclusion List

**New file:** `pkg/services/relationship_exclusions.go`

```go
var nonFKColumnPatterns = []string{
    `^email$`,
    `^password$`,
    `^hashed_password$`,
    `^name$`,
    `^first_name$`,
    `^last_name$`,
    `^description$`,
    `^status$`,
    `^type$`,
    `^state$`,
    `^created_at$`,
    `^updated_at$`,
    `^deleted_at$`,
}

func isAttributeColumn(columnName string) bool {
    for _, pattern := range nonFKColumnPatterns {
        if matched, _ := regexp.MatchString(pattern, columnName); matched {
            return true
        }
    }
    return false
}
```

### 6. Validate *_id Naming Convention

```go
func validateIDColumnNaming(sourceName, targetTable string) bool {
    // user_id should reference users table
    // account_id should reference accounts table
    // etc.

    if !strings.HasSuffix(sourceName, "_id") {
        return true  // Not an _id column, other rules apply
    }

    expectedTable := strings.TrimSuffix(sourceName, "_id")
    expectedTablePlural := expectedTable + "s"

    if targetTable != expectedTable && targetTable != expectedTablePlural {
        return false  // account_id → channels is invalid
    }
    return true
}
```

### 7. Zero-Orphan Requirement for True FKs

```go
// For relationships discovered via inference (not DB constraints)
if candidate.Source != DiscoveryTypeConstraint {
    // Inferred FKs must have zero orphans
    if overlap.OrphanCount > 0 {
        s.recordRejectedCandidate(ctx, candidate, RejectionOrphanValues)
        continue
    }
}
```

## Testing

1. Create test case with:
   - `accounts` table with `email`, `account_id` columns
   - `account_authentications` table with `email`, `account_id` columns
   - Actual FK: `account_authentications.account_id → accounts.account_id`

2. Run relationship discovery

3. Verify:
   - ✓ `account_authentications.account_id → accounts.account_id` discovered
   - ✗ `accounts.email → account_authentications.email` NOT discovered
   - ✗ `accounts.account_id → account_authentications.account_id` NOT discovered (reversed)

## Acceptance Criteria

- [x] Match threshold increased to 95%+
- [x] Orphan threshold decreased to 10% or less
- [x] Attribute columns (email, password, etc.) excluded from FK candidates
- [x] *_id columns only match to their expected tables
- [x] FK direction validated via cardinality analysis
- [x] Only PK columns considered as relationship targets (not all unique columns)
- [x] Zero-orphan requirement for inferred (non-constraint) relationships
- [ ] Existing legitimate relationships (actual DB FKs) still discovered

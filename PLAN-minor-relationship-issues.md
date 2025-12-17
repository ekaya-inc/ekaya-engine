# PLAN: Minor Relationship Feature Issues

These are refinements identified during code review of the relationship extraction feature. None are blocking - the feature is functional.

---

## 1. Missing Unit Tests for Discovery Service

**File:** `pkg/services/relationship_discovery.go`

**Issue:** Only has integration tests. Unlike other services (`query.go` → `query_test.go`), there are no unit tests with mocked dependencies.

**Action:** Create `pkg/services/relationship_discovery_test.go` with unit tests mocking:
- `SchemaRepository`
- `DatasourceService`
- `DatasourceAdapterFactory`

---

## 2. Logger Level Inconsistency (CLAUDE.md Violation)

**File:** `pkg/services/relationship_discovery.go`

**Lines:** 151, 178

**Issue:** Uses `logger.Warn` for errors:
```go
s.logger.Warn("Failed to analyze column stats, skipping table", ...)
s.logger.Warn("Failed to update column joinability", ...)
```

**Action:** Change to `logger.Error` per CLAUDE.md: "Always log errors at ERROR level"

---

## 3. Missing Joinability Constants

**File:** `pkg/services/relationship_discovery.go`

**Lines:** 453, 467

**Issue:** Magic strings not defined as constants:
```go
return false, "no_stats"      // line 453
return true, "cardinality_ok" // line 467
```

**Action:** Add to `pkg/models/schema.go`:
```go
const (
    JoinabilityNoStats       = "no_stats"
    JoinabilityCardinalityOK = "cardinality_ok"
)
```

---

## 4. Repository Error Type Consistency

**File:** `pkg/repositories/schema_repository.go`

**Lines:** 1281-1283

**Issue:** Returns plain error instead of `apperrors.ErrNotFound`:
```go
if result.RowsAffected() == 0 {
    return fmt.Errorf("column not found")
}
```

**Action:** Change to:
```go
if result.RowsAffected() == 0 {
    return apperrors.ErrNotFound
}
```

---

## 5. Interface Check Placement

**File:** `pkg/services/relationship_discovery.go`

**Line:** 556-557

**Issue:** Interface compile-time check is at bottom of file. Other services place it after struct definition.

**Action:** Move to after struct definition (around line 74):
```go
type relationshipDiscoveryService struct { ... }

var _ RelationshipDiscoveryService = (*relationshipDiscoveryService)(nil)
```

---

## 6. Unused Inference Method Constants

**File:** `pkg/models/schema.go`

**Issue:** `InferenceMethodNamingPattern` and `InferenceMethodTypeMatch` are defined but never used. Only `InferenceMethodValueOverlap` is used.

**Action:** Either:
- Add comment explaining these are reserved for future use, OR
- Remove unused constants

---

## Priority

| Issue | Severity |
|-------|----------|
| 1. Missing unit tests | Medium |
| 2. Logger level | Low |
| 3. Missing constants | Low |
| 4. Error type | Low |
| 5. Code organization | Low |
| 6. Unused constants | Low |

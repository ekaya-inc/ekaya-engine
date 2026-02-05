# Architectural Review Issues - February 2026

Status: OPEN
Created: 2026-02-05
Last reviewed: 2026-02-05

This document captures architectural violations and code quality issues found during a comprehensive review against the principles established in CLAUDE.md.

---

## Issue 1: MCP Tool Access Control Code Duplication

**Severity:** Medium
**Location:** `pkg/mcp/tools/queries.go:72-114` vs `pkg/mcp/tools/access.go:40-89`

### Problem

The function `checkApprovedQueriesEnabled()` in `queries.go` duplicates nearly all the logic from `CheckToolAccess()` in `access.go`:

```go
// queries.go - DUPLICATE
func checkApprovedQueriesEnabled(ctx context.Context, deps *QueryToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
    claims, ok := auth.GetClaims(ctx)
    if !ok {
        return uuid.Nil, nil, nil, fmt.Errorf("authentication required")
    }
    projectID, err := uuid.Parse(claims.ProjectID)
    // ... same pattern as CheckToolAccess ...
}
```

Meanwhile, `dev_queries.go` correctly uses the shared `AcquireToolAccess()` function.

### Root Cause

`QueryToolDeps` doesn't implement `ToolAccessDeps` interface because it's missing the `GetMCPConfigService()` method, so `queries.go` couldn't use the shared helper.

### Fix

1. Add `GetMCPConfigService()` to `QueryToolDeps`
2. Replace `checkApprovedQueriesEnabled()` calls with `AcquireToolAccess()`
3. Delete the duplicated function

---

## Issue 2: MCP Tool Deps Structs Duplication

**Severity:** Medium
**Location:** All files in `pkg/mcp/tools/`
**Status:** RESOLVED - `BaseMCPToolDeps` created and embedded by 13 structs

### Problem

There are 14 separate `*Deps` structs that all implement the same interface methods:

| Struct | File |
|--------|------|
| `TableToolDeps` | table.go |
| `MCPToolDeps` | developer.go |
| `KnowledgeToolDeps` | knowledge.go |
| `ColumnToolDeps` | column.go |
| `GlossaryToolDeps` | glossary.go |
| `SchemaToolDeps` | schema.go |
| `OntologyToolDeps` | ontology.go |
| `QueryToolDeps` | queries.go |
| `DevQueryToolDeps` | dev_queries.go |
| `ProbeToolDeps` | probe.go |
| `SearchToolDeps` | search.go |
| `ContextToolDeps` | context.go |
| `HealthToolDeps` | health.go |
| `QuestionToolDeps` | questions.go |

Each implements identical boilerplate:

```go
func (d *XxxToolDeps) GetDB() *database.DB { return d.DB }
func (d *XxxToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }
func (d *XxxToolDeps) GetLogger() *zap.Logger { return d.Logger }
```

### Fix

Create a base deps struct that others embed:

```go
// BaseMCPToolDeps provides common dependencies for all MCP tools.
type BaseMCPToolDeps struct {
    DB               *database.DB
    MCPConfigService services.MCPConfigService
    Logger           *zap.Logger
}

func (d *BaseMCPToolDeps) GetDB() *database.DB { return d.DB }
func (d *BaseMCPToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }
func (d *BaseMCPToolDeps) GetLogger() *zap.Logger { return d.Logger }

// TableToolDeps embeds base and adds tool-specific deps.
type TableToolDeps struct {
    BaseMCPToolDeps
    SchemaRepo   repositories.SchemaRepository
    // ...
}
```

### Resolution

`BaseMCPToolDeps` was created in `pkg/mcp/tools/access.go` with the three common interface methods. The following 13 structs now embed it:

- TableToolDeps, MCPToolDeps, KnowledgeToolDeps, ColumnToolDeps, GlossaryToolDeps
- SchemaToolDeps, OntologyToolDeps, QueryToolDeps, DevQueryToolDeps, ProbeToolDeps
- SearchToolDeps, ContextToolDeps, QuestionToolDeps

**Note:** `HealthToolDeps` was excluded because it doesn't implement `ToolAccessDeps` - it lacks `MCPConfigService` since the health tool is always available without access control.

---

## Issue 3: Column Name Pattern Heuristics in Relationship Discovery

**Severity:** High
**Location:** `pkg/services/relationship_discovery.go:585-630`
**Status:** RESOLVED - Already addressed by architecture transition

### Problem

The `shouldCreateCandidate()` function uses column name patterns to make FK decisions, directly violating CLAUDE.md Rule #5:

```go
// VIOLATION: Using column name suffix to identify FK
if strings.HasSuffix(sourceLower, "_id") {
    entityName := strings.TrimSuffix(sourceLower, "_id")
    // ... assumes *_id columns are foreign keys
}
```

This also includes an `attributeColumnPatterns` list that checks for patterns like "email", "password", "status" by substring matching.

### Why This Is Fragile

- Column naming conventions vary between databases
- `user_id` could be a local identifier, not an FK
- `email_address_id` has `_id` but also contains `email`
- Many legitimate FKs don't follow the `*_id` pattern

### Resolution

This issue has been resolved by the architecture transition already in place:

1. **The file is deprecated** - `relationship_discovery.go` is marked `// DEPRECATED: This file is scheduled for removal` and directs developers to use `LLMRelationshipDiscoveryService` instead.

2. **The replacement properly uses ColumnMetadata** - `relationship_candidate_collector.go` explicitly states at line 72: `// Per CLAUDE.md rule #5: We do NOT filter by column name patterns (e.g., _id suffix).` It uses `ColumnMetadata.Role` and `ColumnMetadata.Purpose` for FK source identification.

3. **The new service is live** - `LLMRelationshipDiscoveryService` uses `RelationshipCandidateCollector` for all production FK discovery.

**Remaining work:** Complete the deprecation by removing:
- `pkg/services/relationship_discovery.go` (deprecated file)
- `pkg/services/relationship_discovery_test.go` (deprecated tests)
- `pkg/services/deterministic_relationship_service.go` (also deprecated)
- The unused `POST /api/.../relationships/discover` endpoint in `schema.go`
- The `discoveryService` wiring in `main.go`

---

## Issue 4: MCP Tool Validation Errors Return Go Errors Instead of JSON

**Severity:** Medium
**Location:** Multiple files in `pkg/mcp/tools/`

### Problem

Per CLAUDE.md Rule #6, actionable MCP tool errors (validation, invalid parameters, not found) should use `NewErrorResult()` to return JSON success responses, not Go errors.

Several tool access functions return Go errors for actionable cases:

**pkg/mcp/tools/access.go:**
```go
// Line 48 - actionable (caller can fix auth)
return nil, fmt.Errorf("authentication required")

// Line 53 - actionable (invalid input)
return nil, fmt.Errorf("invalid project ID: %w", err)

// Line 88 - actionable (tool not enabled, caller should choose different tool)
return nil, fmt.Errorf("%s tool is not enabled for this project", toolName)
```

**pkg/mcp/tools/queries.go:**
```go
// Lines 76, 81, 113 - same pattern
```

### Impact

When MCP clients receive protocol-level errors (code -32603), some clients:
- Flash an error on screen
- Swallow the error entirely
- Don't pass the error message to the LLM

This makes Ekaya appear broken when the tool simply received invalid input.

### Fix

Use `NewErrorResult()` for actionable errors:

```go
// BEFORE
if !ok {
    return nil, fmt.Errorf("authentication required")
}

// AFTER
if !ok {
    return NewErrorResult("authentication_required", "authentication required"), nil
}
```

Keep Go errors only for system failures (DB connection errors, internal errors).

---

## Issue 5: DAG Node OntologyID Validation Inconsistency

**Severity:** Low
**Location:** `pkg/services/dag/`

### Problem

CLAUDE.md Rule #4 states DAG nodes must "Validate required fields (e.g., `dag.OntologyID`) at the start of `Execute()`".

Only 2 of 10 nodes validate `dag.OntologyID`:
- `glossary_discovery_node.go:53` ✓
- `glossary_enrichment_node.go:53` ✓

The following do NOT validate it:
- `ontology_finalization_node.go`
- `column_feature_extraction_node.go`
- `column_enrichment_node.go`
- `table_feature_extraction_node.go`
- `knowledge_seeding_node.go`
- `fk_discovery_node.go`
- `pk_match_discovery_node.go`
- `relationship_discovery_node.go`

### Note

Some nodes may not require `OntologyID` if they operate only on schema data. The issue is the inconsistency - there should be a documented decision about which nodes require which fields.

### Fix

1. Document which DAG nodes require `dag.OntologyID`
2. Add validation to those that need it
3. For nodes that don't need it, add a comment explaining why

---

## Issue 6: Logger.Warn Used for Errors (Minor)

**Severity:** Low
**Location:** Various files

### Problem

CLAUDE.md states "Always log errors at ERROR level - never use `logger.Warn` for errors".

Found instances of `logger.Warn` with error arguments that might be actual errors:

- `pkg/services/ontology_builder.go:99` - "Failed to parse answer processing response"
- `pkg/services/ontology_dag_service.go:750` - "Failed to get tenant context for heartbeat"
- `pkg/services/ontology_dag_service.go:755` - "Failed to update heartbeat"
- `pkg/handlers/auth.go:70` - "Invalid request body"

### Evaluation Needed

Some of these may be intentional (e.g., invalid user input is a warning, not an error). Each should be evaluated:
- If it indicates a bug or system failure → `logger.Error`
- If it's expected bad input from users → `logger.Warn` is OK

---

## Issue 7: QueryToolDeps Missing ToolAccessDeps Compliance

**Severity:** Low
**Location:** `pkg/mcp/tools/queries.go`

### Problem

`QueryToolDeps` has custom interface methods but doesn't fully implement `ToolAccessDeps`:

```go
type QueryToolDeps struct {
    DB               *database.DB
    MCPConfigService services.MCPConfigService  // ← Has the field
    // ...
}

// Implements QueryLoggingDeps but NOT ToolAccessDeps
func (d *QueryToolDeps) GetLogger() *zap.Logger { return d.Logger }
func (d *QueryToolDeps) GetAuditor() *audit.SecurityAuditor { return d.Auditor }
func (d *QueryToolDeps) GetDB() *database.DB { return d.DB }
// MISSING: GetMCPConfigService()
```

This is why Issue #1 exists - the shared `CheckToolAccess()` can't be used.

### Fix

Add the missing method:
```go
func (d *QueryToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }
```

---

## Summary Table

| # | Issue | Severity | Principle Violated | Status |
|---|-------|----------|-------------------|--------|
| 1 | Tool access control duplication | Medium | DRY | Open |
| 2 | Deps structs duplication | Medium | DRY | [x] **Resolved** |
| 3 | Column name pattern heuristics | High | Rule #5 | [x] **Resolved** (via architecture transition) |
| 4 | MCP validation errors as Go errors | Medium | Rule #6 | Open |
| 5 | DAG node validation inconsistency | Low | Rule #4 | Open |
| 6 | Logger.Warn for errors | Low | Fail-fast philosophy | Open |
| 7 | QueryToolDeps missing interface method | Low | Code consistency | Open |

---

## Recommended Fix Order

1. ~~**Issue 3** (High) - Column name heuristics is the most architecturally significant~~ **RESOLVED** - See Issue 3 above
2. **Issue 4** (Medium) - MCP error handling affects user experience
3. **Issues 1, 2, 7** (Medium) - Code duplication refactoring (can be done together)
4. **Issues 5, 6** (Low) - Consistency improvements

---

## Notes

- This review focused on violations of principles in CLAUDE.md
- Test files were excluded from most checks (test SQL is expected)
- The SQL in `pkg/services/query.go` and `pkg/services/glossary_service.go` appears to be for customer datasources via adapters (allowed), not engine database

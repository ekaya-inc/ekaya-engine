# PLAN: Enable Data-Modifying Pre-Approved Queries

## Purpose

Allow admins to create pre-approved SQL statements that modify data (INSERT, UPDATE, DELETE, CALL) while maintaining the safety and parameter validation of existing read-only queries.

**Key Principle:** Keep it to a single SQL statement. Complex logic belongs in stored procedures.

---

## Current State

| Aspect | Current Behavior |
|--------|------------------|
| Pre-Approved Queries | SELECT-only (enforced by hint, not runtime) |
| `execute_approved_query` tool | Has `ReadOnlyHint: true` annotation |
| Query model | No field to distinguish read vs modify |
| UI | No visual distinction for query types |

---

## Design Decisions

### Single Statement Only
- One SQL statement per pre-approved query
- No multi-statement batches or transactions
- Complex logic → use stored procedures (`CALL my_procedure({{param}})`)

### Terminology
- Keep "Pre-Approved Queries" name (not "Pre-Approved SQL")
- Visual indicators communicate the risk, not terminology

### Allowed Statement Types (when enabled)
- `INSERT ... VALUES/SELECT ... RETURNING`
- `UPDATE ... SET ... WHERE ... RETURNING`
- `DELETE FROM ... WHERE ... RETURNING`
- `CALL procedure_name(...)`
- `SELECT function_name(...)` (for functions with side effects)

### Blocked Statement Types
- DDL (CREATE, ALTER, DROP, TRUNCATE)
- Multi-statement batches
- Transaction control (BEGIN, COMMIT, ROLLBACK)

---

## Phase 1: Data Model Changes ✅

### 1.1 Add `allows_modification` to Query Model

**File:** `pkg/models/query.go`

```go
type Query struct {
    // ... existing fields ...

    // AllowsModification indicates this query can modify data (INSERT/UPDATE/DELETE/CALL)
    // When false (default), only SELECT statements are allowed
    AllowsModification bool `json:"allows_modification" db:"allows_modification"`
}
```

### 1.2 Database Migration

**File:** `migrations/018_query_allows_modification.up.sql`

```sql
-- Add allows_modification flag to engine_queries
-- Default false to preserve existing behavior (SELECT-only)
ALTER TABLE engine_queries
ADD COLUMN allows_modification BOOLEAN NOT NULL DEFAULT FALSE;

-- Add comment for documentation
COMMENT ON COLUMN engine_queries.allows_modification IS
    'When true, this query can execute INSERT/UPDATE/DELETE/CALL statements. When false, only SELECT is allowed.';
```

**File:** `migrations/018_query_allows_modification.down.sql`

```sql
ALTER TABLE engine_queries DROP COLUMN allows_modification;
```

---

## Phase 2: Backend Validation ✅

### 2.1 SQL Statement Type Detection

**File:** `pkg/services/query_validation.go`

```go
type SQLStatementType string

const (
    SQLTypeSelect   SQLStatementType = "SELECT"
    SQLTypeInsert   SQLStatementType = "INSERT"
    SQLTypeUpdate   SQLStatementType = "UPDATE"
    SQLTypeDelete   SQLStatementType = "DELETE"
    SQLTypeCall     SQLStatementType = "CALL"
    SQLTypeDDL      SQLStatementType = "DDL"      // CREATE, ALTER, DROP, TRUNCATE
    SQLTypeUnknown  SQLStatementType = "UNKNOWN"
)

// DetectSQLType determines the type of SQL statement
func DetectSQLType(sql string) SQLStatementType {
    normalized := strings.ToUpper(strings.TrimSpace(sql))

    switch {
    case strings.HasPrefix(normalized, "SELECT"),
         strings.HasPrefix(normalized, "WITH"):
        // Check for data-modifying CTEs
        if containsModifyingCTE(normalized) {
            return SQLTypeUnknown // Block data-modifying CTEs
        }
        return SQLTypeSelect
    case strings.HasPrefix(normalized, "INSERT"):
        return SQLTypeInsert
    case strings.HasPrefix(normalized, "UPDATE"):
        return SQLTypeUpdate
    case strings.HasPrefix(normalized, "DELETE"):
        return SQLTypeDelete
    case strings.HasPrefix(normalized, "CALL"):
        return SQLTypeCall
    case strings.HasPrefix(normalized, "CREATE"),
         strings.HasPrefix(normalized, "ALTER"),
         strings.HasPrefix(normalized, "DROP"),
         strings.HasPrefix(normalized, "TRUNCATE"):
        return SQLTypeDDL
    default:
        return SQLTypeUnknown
    }
}

// IsModifyingStatement returns true for INSERT/UPDATE/DELETE/CALL
func IsModifyingStatement(sqlType SQLStatementType) bool {
    switch sqlType {
    case SQLTypeInsert, SQLTypeUpdate, SQLTypeDelete, SQLTypeCall:
        return true
    default:
        return false
    }
}
```

### 2.2 Query Validation on Save

**File:** `pkg/services/query.go`

```go
func (s *queryService) Create(ctx context.Context, projectID uuid.UUID, req *CreateQueryRequest) (*Query, error) {
    sqlType := DetectSQLType(req.SQLQuery)

    // Block DDL statements entirely
    if sqlType == SQLTypeDDL {
        return nil, fmt.Errorf("DDL statements (CREATE, ALTER, DROP, TRUNCATE) are not allowed in pre-approved queries")
    }

    // Block unknown statement types
    if sqlType == SQLTypeUnknown {
        return nil, fmt.Errorf("unrecognized SQL statement type; only SELECT, INSERT, UPDATE, DELETE, and CALL are allowed")
    }

    // Validate consistency: modifying SQL requires allows_modification flag
    if IsModifyingStatement(sqlType) && !req.AllowsModification {
        return nil, fmt.Errorf("this SQL statement modifies data; enable 'Allows Modification' to save")
    }

    // Validate consistency: SELECT doesn't need allows_modification
    if !IsModifyingStatement(sqlType) && req.AllowsModification {
        // Allow but warn? Or auto-correct? Decision: auto-correct silently
        req.AllowsModification = false
    }

    // ... rest of creation logic
}
```

### 2.3 Execution Validation

**File:** `pkg/mcp/tools/queries.go`

Update `execute_approved_query` to validate at runtime:

```go
// In execute_approved_query handler:

// Detect statement type
sqlType := DetectSQLType(query.SQLQuery)

// Validate: modifying statements require allows_modification flag
if IsModifyingStatement(sqlType) && !query.AllowsModification {
    return NewErrorResult("QUERY_NOT_AUTHORIZED",
        fmt.Sprintf("query %q is a %s statement but is not authorized for data modification",
            query.NaturalLanguagePrompt, sqlType)), nil
}

// For modifying statements, use execute path instead of query path
if IsModifyingStatement(sqlType) {
    return executeModifyingQuery(ctx, deps, query, params, limit)
}

// For SELECT, use existing read-only path
return executeReadOnlyQuery(ctx, deps, query, params, limit)
```

---

## Phase 3: UI Changes ✅

### 3.1 Query Editor - Add Checkbox

**File:** `ui/src/pages/QueriesPage.tsx`

Add a checkbox in the query editor form:

```tsx
{/* Below the SQL editor */}
<div className="flex items-center gap-2 mt-4">
  <Checkbox
    id="allows-modification"
    checked={allowsModification}
    onCheckedChange={setAllowsModification}
  />
  <Label htmlFor="allows-modification" className="text-sm">
    Allow data modification (INSERT, UPDATE, DELETE, CALL)
  </Label>
</div>

{/* Warning when checked */}
{allowsModification && (
  <Alert variant="warning" className="mt-2">
    <AlertTriangle className="h-4 w-4" />
    <AlertDescription>
      This query will be able to modify or delete data. Ensure the SQL
      and parameters are thoroughly reviewed before enabling.
    </AlertDescription>
  </Alert>
)}
```

### 3.2 Query List - Visual Indicators

**File:** `ui/src/pages/QueriesPage.tsx`

Add badges to query list items:

```tsx
{/* In query list item */}
<div className="flex items-center gap-2">
  <span className="font-medium">{query.naturalLanguagePrompt}</span>
  {query.allowsModification && (
    <Badge variant="destructive" className="text-xs">
      <Pencil className="h-3 w-3 mr-1" />
      Modifies Data
    </Badge>
  )}
</div>
```

### 3.3 Filter for Query Types

Add a filter dropdown to the query list:

```tsx
<Select value={filter} onValueChange={setFilter}>
  <SelectTrigger className="w-40">
    <SelectValue placeholder="All queries" />
  </SelectTrigger>
  <SelectContent>
    <SelectItem value="all">All queries</SelectItem>
    <SelectItem value="read-only">Read-only</SelectItem>
    <SelectItem value="modifying">Modifies data</SelectItem>
  </SelectContent>
</Select>
```

### 3.4 Test Query Button Behavior

When testing a modifying query:

```tsx
{/* Show different warning for modifying queries */}
{allowsModification && (
  <Alert variant="destructive" className="mb-4">
    <AlertTriangle className="h-4 w-4" />
    <AlertDescription>
      <strong>Warning:</strong> Testing this query will execute it against your
      live database and may modify data. Use test parameters that won't cause harm.
    </AlertDescription>
  </Alert>
)}

<Button
  onClick={handleTestQuery}
  variant={allowsModification ? "destructive" : "default"}
>
  {allowsModification ? "Test Query (Modifies Data)" : "Test Query"}
</Button>
```

---

## Phase 4: MCP Tool Updates ✅

### 4.1 Update Tool Annotations

**File:** `pkg/mcp/tools/queries.go`

The `execute_approved_query` tool should dynamically report its capabilities:

```go
// Tool annotations should reflect that SOME queries may modify data
mcp.WithReadOnlyHintAnnotation(false),  // Changed from true
mcp.WithDestructiveHintAnnotation(false), // Individual queries may be destructive
```

### 4.2 Response Includes Modification Flag

Update the response to indicate if the query modified data:

```go
type ExecuteApprovedQueryResponse struct {
    // ... existing fields ...

    // ModifiedData indicates this execution may have changed data
    ModifiedData bool `json:"modified_data,omitempty"`

    // RowsAffected for INSERT/UPDATE/DELETE
    RowsAffected *int `json:"rows_affected,omitempty"`
}
```

### 4.3 list_approved_queries Shows Modification Flag

```go
// In list_approved_queries response
type ApprovedQueryInfo struct {
    ID                  string   `json:"id"`
    Name                string   `json:"name"`
    Description         string   `json:"description"`
    Parameters          []Param  `json:"parameters"`
    AllowsModification  bool     `json:"allows_modification"`  // NEW
    Tags                []string `json:"tags,omitempty"`
}
```

---

## Phase 5: Safety Considerations ✅

### 5.1 Audit Logging

All modifying query executions are logged with:
- Full SQL with parameters substituted
- User who executed
- Timestamp
- Rows affected
- Success/failure

### 5.2 No Destructive Defaults

- `allows_modification` defaults to `false`
- Existing queries remain SELECT-only
- Must explicitly enable for each query

### 5.3 Parameter Validation Still Applies

- All parameter injection protection remains
- Type checking still enforced
- SQL injection detection still active

### 5.4 RETURNING Clause Recommendation

For INSERT/UPDATE/DELETE, recommend RETURNING to show what was affected:

```sql
-- Good: Shows what was deleted
DELETE FROM tasks WHERE id = {{task_id}} RETURNING id, title

-- Less ideal: Only shows row count
DELETE FROM tasks WHERE id = {{task_id}}
```

UI suggests adding RETURNING if not present.

---

## File Changes Summary

| Phase | File | Change |
|-------|------|--------|
| 1 | `pkg/models/query.go` | Add `AllowsModification` field |
| 1 | `migrations/018_*.sql` | Add column to database |
| 2 | `pkg/services/query_validation.go` | New file for SQL type detection |
| 2 | `pkg/services/query.go` | Validation on create/update |
| 2 | `pkg/mcp/tools/queries.go` | Runtime validation in execute |
| 3 | `ui/src/pages/QueriesPage.tsx` | Checkbox, badges, warnings |
| 3 | `ui/src/types/query.ts` | Add `allowsModification` to type |
| 4 | `pkg/mcp/tools/queries.go` | Update annotations and response |

---

## Testing Strategy

### Unit Tests
- SQL type detection for all statement types
- Validation rejects DDL
- Validation requires flag for modifying SQL
- Validation auto-corrects flag for SELECT

### Integration Tests
- Create modifying query → verify flag persisted
- Execute modifying query → verify data changed
- Execute modifying query without flag → verify rejected
- List queries → verify flag returned

### Manual Testing
1. Create SELECT query → verify no checkbox needed
2. Create INSERT query without checkbox → verify error
3. Create INSERT query with checkbox → verify warning shown
4. Test INSERT query → verify warning about live data
5. Execute via MCP → verify rows_affected returned
6. Verify badge shows in query list
7. Verify filter works

---

## Open Questions

1. **Should we require RETURNING clause for modifying queries?**
   - Pro: Better visibility into what changed
   - Con: Some use cases don't need it
   - Recommendation: Suggest but don't require

2. **Should there be a separate permission for approving modifying queries?**
   - Pro: Separation of duties
   - Con: Complexity for small teams
   - Recommendation: Future enhancement, not v1

3. **Should modifying queries have stricter timeout?**
   - Current: Same timeout as SELECT
   - Recommendation: Keep same for now, monitor usage

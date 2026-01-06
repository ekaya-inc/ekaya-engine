# Design: MCP Client-Driven Glossary Updates

## Overview

This document describes the design for exposing glossary CRUD operations as MCP tools, enabling MCP clients (like Claude Desktop) to dynamically create, update, and delete business glossary terms during query composition.

## Context

Currently, glossary terms are created through:
1. **Inferred** (`source=inferred`) - LLM discovers terms during ontology extraction
2. **Manual** (`source=manual`) - Users create/edit via UI

This design adds a third path:
3. **Client-driven** (`source=client`) - MCP clients create terms dynamically via MCP tools

## Goals

- Enable MCP clients to contribute to the business glossary programmatically
- Maintain source attribution (track which terms came from clients vs LLM vs humans)
- Preserve SQL validation guarantees (all client-provided SQL must be valid)
- Support full CRUD lifecycle (create, update, delete) from MCP clients

## Non-Goals

- Authorization/permissions beyond project-level access (future work)
- Versioning/history of client updates (future work)
- Bulk operations (create many terms at once)
- Entity associations (linking terms to ontology entities)

---

## Current Implementation Status

### Repository Layer (pkg/repositories/glossary_repository.go)

**Already supports all required operations:**

```go
type GlossaryRepository interface {
    Create(ctx context.Context, term *models.BusinessGlossaryTerm) error
    Update(ctx context.Context, term *models.BusinessGlossaryTerm) error
    Delete(ctx context.Context, termID uuid.UUID) error
    GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
    GetByTerm(ctx context.Context, projectID uuid.UUID, term string) (*models.BusinessGlossaryTerm, error)
    GetByAlias(ctx context.Context, projectID uuid.UUID, alias string) (*models.BusinessGlossaryTerm, error)
    GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error)
    CreateAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error
    DeleteAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error
}
```

**Key capabilities:**
- Source field is part of `models.BusinessGlossaryTerm` struct
- No hard-coded source restrictions in repository layer
- All CRUD operations properly handle aliases
- RLS enforces tenant isolation at database level

### Service Layer (pkg/services/glossary_service.go)

**Already supports client operations with validation:**

```go
type GlossaryService interface {
    CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error
    UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error
    DeleteTerm(ctx context.Context, termID uuid.UUID) error
    TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*SQLTestResult, error)
    CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error
    DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error
    GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error)
}
```

**Key behaviors:**
- `CreateTerm` validates SQL via `TestSQL` before persistence (fail-fast)
- `UpdateTerm` re-validates SQL if it changed
- `TestSQL` executes SQL with LIMIT 1 to verify syntax and capture output columns
- Default source is `models.GlossarySourceManual` but can be overridden
- All validation errors return structured errors (not panic/abort)

### Database Schema (migrations/031_glossary_defining_sql.up.sql)

**Schema ready for client updates:**

```sql
CREATE TABLE engine_business_glossary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    term TEXT NOT NULL,
    definition TEXT NOT NULL,
    defining_sql TEXT NOT NULL,
    base_table TEXT,
    output_columns JSONB,

    source TEXT NOT NULL DEFAULT 'inferred',  -- 'inferred', 'manual', 'client'

    created_by UUID,   -- User who created (null for inferred/client)
    updated_by UUID,   -- User who last updated
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT engine_business_glossary_project_term_unique UNIQUE (project_id, term),
    CONSTRAINT engine_business_glossary_source_check CHECK (source IN ('inferred', 'manual', 'client'))
);
```

**Key constraints:**
- Source must be one of: `inferred`, `manual`, `client`
- Unique constraint on (project_id, term) - clients cannot create duplicate terms
- RLS enforces project isolation - clients can only access their project's terms

---

## Proposed MCP Tools

### Tool 1: `create_glossary_term`

**Purpose:** Add a new business term to the glossary with its SQL definition.

**Input Schema:**
```json
{
  "term": "string (required)",
  "definition": "string (required)",
  "defining_sql": "string (required)",
  "base_table": "string (optional)",
  "aliases": ["string"] (optional)
}
```

**Implementation:**
```go
// MCP tool handler in pkg/mcp/tools/glossary.go
func createGlossaryTerm(ctx context.Context, glossarySvc services.GlossaryService, projectID uuid.UUID, input CreateGlossaryTermInput) (*CreateGlossaryTermResponse, error) {
    term := &models.BusinessGlossaryTerm{
        Term:        input.Term,
        Definition:  input.Definition,
        DefiningSQL: input.DefiningSQL,
        BaseTable:   input.BaseTable,
        Aliases:     input.Aliases,
        Source:      models.GlossarySourceClient,  // Always set to 'client'
        CreatedBy:   nil,  // No user association for client-created terms
        UpdatedBy:   nil,
    }

    // Service layer validates SQL via TestSQL before persisting
    err := glossarySvc.CreateTerm(ctx, projectID, term)
    if err != nil {
        return &CreateGlossaryTermResponse{
            Success: false,
            Error:   err.Error(),
        }, nil  // Return structured error, not Go error
    }

    return &CreateGlossaryTermResponse{
        Success: true,
        Term:    toGetGlossarySQLResponse(term),
    }, nil
}
```

**Response Schema:**
```json
{
  "success": true,
  "term": {
    "term": "Active Users",
    "definition": "...",
    "defining_sql": "SELECT ...",
    "output_columns": [...],
    "aliases": [...]
  }
}
```

Or on failure:
```json
{
  "success": false,
  "error": "SQL validation failed: syntax error at position 42"
}
```

**Validation Rules:**
1. Term name must be unique within project (database constraint enforces)
2. SQL must be valid and executable (service layer validates via TestSQL)
3. Definition must be non-empty
4. Output columns are auto-captured from SQL execution

**Error Scenarios:**
- Duplicate term name → `error: "term 'Active Users' already exists"`
- Invalid SQL → `error: "SQL validation failed: <database error>"`
- Missing required fields → `error: "field 'term' is required"`

### Tool 2: `update_glossary_term`

**Purpose:** Modify an existing glossary term (definition and/or SQL).

**Input Schema:**
```json
{
  "term": "string (required - lookup key, can be term name or alias)",
  "definition": "string (optional)",
  "defining_sql": "string (optional)",
  "base_table": "string (optional)",
  "aliases": ["string"] (optional)
}
```

**Implementation:**
```go
// MCP tool handler in pkg/mcp/tools/glossary.go
func updateGlossaryTerm(ctx context.Context, glossarySvc services.GlossaryService, projectID uuid.UUID, input UpdateGlossaryTermInput) (*UpdateGlossaryTermResponse, error) {
    // Lookup existing term by name or alias
    existingTerm, err := glossarySvc.GetTermByName(ctx, projectID, input.Term)
    if err != nil {
        return &UpdateGlossaryTermResponse{
            Success: false,
            Error:   fmt.Sprintf("term lookup failed: %v", err),
        }, nil
    }
    if existingTerm == nil {
        return &UpdateGlossaryTermResponse{
            Success: false,
            Error:   fmt.Sprintf("term '%s' not found", input.Term),
        }, nil
    }

    // Apply updates (only non-nil fields)
    if input.Definition != nil {
        existingTerm.Definition = *input.Definition
    }
    if input.DefiningSQL != nil {
        existingTerm.DefiningSQL = *input.DefiningSQL
    }
    if input.BaseTable != nil {
        existingTerm.BaseTable = *input.BaseTable
    }
    if input.Aliases != nil {
        existingTerm.Aliases = input.Aliases
    }

    // Update source to 'client' if it wasn't already
    // (Preserves source=client if already client-created, overrides inferred/manual)
    existingTerm.Source = models.GlossarySourceClient
    existingTerm.UpdatedBy = nil  // No user association

    // Service layer re-validates SQL if it changed
    err = glossarySvc.UpdateTerm(ctx, existingTerm)
    if err != nil {
        return &UpdateGlossaryTermResponse{
            Success: false,
            Error:   err.Error(),
        }, nil
    }

    return &UpdateGlossaryTermResponse{
        Success: true,
        Term:    toGetGlossarySQLResponse(existingTerm),
    }, nil
}
```

**Response Schema:**
```json
{
  "success": true,
  "term": {
    "term": "Active Users",
    "definition": "Updated definition...",
    "defining_sql": "SELECT ...",
    "output_columns": [...],
    "aliases": [...]
  }
}
```

**Validation Rules:**
1. Term must exist (lookup by name or alias)
2. If SQL changes, new SQL must be valid (service layer validates)
3. Updates overwrite aliases (not merge) - caller must provide full alias list

**Error Scenarios:**
- Term not found → `error: "term 'Unknown' not found"`
- Invalid new SQL → `error: "SQL validation failed: <database error>"`
- No fields provided → Succeeds as no-op (not an error)

**Design Decision: Source Overwrite**
When a client updates ANY term (even inferred/manual), the source changes to `client`. This provides clear attribution: "last touched by client". Alternative considered: preserve original source and add separate `last_modified_by_source` field. Rejected for simplicity.

### Tool 3: `delete_glossary_term`

**Purpose:** Remove a glossary term from the project.

**Input Schema:**
```json
{
  "term": "string (required - can be term name or alias)"
}
```

**Implementation:**
```go
// MCP tool handler in pkg/mcp/tools/glossary.go
func deleteGlossaryTerm(ctx context.Context, glossarySvc services.GlossaryService, projectID uuid.UUID, input DeleteGlossaryTermInput) (*DeleteGlossaryTermResponse, error) {
    // Lookup term by name or alias
    existingTerm, err := glossarySvc.GetTermByName(ctx, projectID, input.Term)
    if err != nil {
        return &DeleteGlossaryTermResponse{
            Success: false,
            Error:   fmt.Sprintf("term lookup failed: %v", err),
        }, nil
    }
    if existingTerm == nil {
        return &DeleteGlossaryTermResponse{
            Success: false,
            Error:   fmt.Sprintf("term '%s' not found", input.Term),
        }, nil
    }

    // Delete the term (cascades to aliases)
    err = glossarySvc.DeleteTerm(ctx, existingTerm.ID)
    if err != nil {
        return &DeleteGlossaryTermResponse{
            Success: false,
            Error:   err.Error(),
        }, nil
    }

    return &DeleteGlossaryTermResponse{
        Success: true,
        Message: fmt.Sprintf("Term '%s' deleted successfully", existingTerm.Term),
    }, nil
}
```

**Response Schema:**
```json
{
  "success": true,
  "message": "Term 'Active Users' deleted successfully"
}
```

**Validation Rules:**
1. Term must exist (lookup by name or alias)
2. No restriction on source - clients can delete any term (inferred/manual/client)

**Error Scenarios:**
- Term not found → `error: "term 'Unknown' not found"`

**Design Decision: No Source Restriction**
Clients can delete ANY term regardless of source. Alternative considered: restrict deletion to client-created terms only. Rejected because clients may need to "clean up" LLM-discovered terms that are incorrect. The UI can implement stricter rules if needed (e.g., require confirmation for deleting manual terms).

---

## Tool Registration and Filtering

**Location:** `pkg/mcp/tools/glossary.go`

**Registration Pattern:**
```go
func RegisterGlossaryTools(server *server.MCPServer, services MCPToolServices) {
    // Read-only tools (always available)
    server.AddTool(listGlossaryTool, makeListGlossaryHandler(services.GlossarySvc))
    server.AddTool(getGlossarySQLTool, makeGetGlossarySQLHandler(services.GlossarySvc))

    // Write tools (gated by developer tools toggle)
    server.AddTool(createGlossaryTermTool, makeCreateGlossaryTermHandler(services.GlossarySvc))
    server.AddTool(updateGlossaryTermTool, makeUpdateGlossaryTermHandler(services.GlossarySvc))
    server.AddTool(deleteGlossaryTermTool, makeDeleteGlossaryTermHandler(services.GlossarySvc))
}
```

**Tool Filtering (pkg/mcp/tools/developer.go):**
```go
var glossaryToolNames = map[string]struct{}{
    "list_glossary":          {},  // Read-only, always available
    "get_glossary_sql":       {},  // Read-only, always available
    "create_glossary_term":   {},  // Write, requires developer tools
    "update_glossary_term":   {},  // Write, requires developer tools
    "delete_glossary_term":   {},  // Write, requires developer tools
}

func (f *toolFilter) FilterTools(projectID uuid.UUID, tools []mcp.Tool) ([]mcp.Tool, error) {
    config := f.getConfig(projectID)

    var filtered []mcp.Tool
    for _, tool := range tools {
        if _, isGlossary := glossaryToolNames[tool.Name]; isGlossary {
            // Read-only glossary tools always allowed
            if tool.Name == "list_glossary" || tool.Name == "get_glossary_sql" {
                filtered = append(filtered, tool)
                continue
            }

            // Write glossary tools require developer tools toggle
            if config.DeveloperToolsEnabled {
                filtered = append(filtered, tool)
            }
        }
    }
    return filtered, nil
}
```

**UI Toggle:** Projects can enable/disable write tools via `/projects/:pid/mcp-server` settings page. Read-only tools (`list_glossary`, `get_glossary_sql`) are always available regardless of toggle.

---

## Authentication and Authorization

### Current State

**Project-level isolation:**
- RLS policies enforce project_id scoping at database level
- MCP server URL includes project ID: `/mcp/{project-id}`
- No term-level permissions (all users with project access can see all terms)

**User context:**
- Service layer uses `auth.RequireUserIDFromContext(ctx)` for datasource operations
- `created_by` and `updated_by` fields track user IDs for UI-created terms
- Client-created terms have `created_by=NULL, updated_by=NULL` (no user association)

### Future Considerations (Out of Scope)

**Source-based restrictions:**
- Allow users to configure "client can only modify client-created terms"
- Require approval workflow for client updates to manual terms
- Version history for term changes

**Role-based access:**
- Read-only vs read-write MCP access per project
- Per-user API keys for MCP server authentication
- Audit log of client modifications

---

## Error Handling

### Validation Errors

**SQL validation failures are NOT Go errors:**
```go
// Service layer returns structured result, not error
testResult, err := glossarySvc.TestSQL(ctx, projectID, sql)
if err != nil {
    // This is a system error (datasource unreachable, etc)
    return systemError(err)
}
if !testResult.Valid {
    // This is a validation error (user-fixable)
    return userError(testResult.Error)
}
```

**MCP tool pattern:**
```go
// GOOD: Return structured response with error field
return &Response{Success: false, Error: "SQL validation failed: ..."}

// BAD: Return Go error (causes MCP protocol error, not user-facing)
return nil, fmt.Errorf("SQL validation failed")
```

### Duplicate Term Handling

**Database constraint enforcement:**
```sql
CONSTRAINT engine_business_glossary_project_term_unique UNIQUE (project_id, term)
```

**Repository layer converts to apperrors.ErrConflict:**
```go
if err := r.Create(ctx, term); err != nil {
    if isUniqueViolation(err) {
        return apperrors.ErrConflict
    }
    return err
}
```

**MCP tool detects and returns user-friendly message:**
```go
if errors.Is(err, apperrors.ErrConflict) {
    return &Response{
        Success: false,
        Error:   fmt.Sprintf("term '%s' already exists", term.Term),
    }
}
```

---

## Testing Strategy

### Unit Tests (pkg/mcp/tools/glossary_test.go)

**Test coverage for each tool:**
1. Success path with all fields
2. Success path with minimal fields
3. Term not found (for update/delete)
4. Invalid SQL (for create/update)
5. Duplicate term name (for create)
6. Lookup by alias (for update/delete)

**Mock setup:**
```go
type mockGlossaryService struct {
    createFn       func(ctx, projectID, term) error
    updateFn       func(ctx, term) error
    deleteFn       func(ctx, termID) error
    getTermByNameFn func(ctx, projectID, name) (*models.BusinessGlossaryTerm, error)
}
```

### Integration Tests (pkg/handlers/glossary_integration_test.go)

**Test scenarios:**
1. Client creates term → Verify source=client, SQL validated, output_columns captured
2. Client updates inferred term → Verify source changes to client
3. Client updates invalid SQL → Verify error response, no data change
4. Client deletes term → Verify cascades to aliases
5. Duplicate term creation → Verify conflict error

**Test helpers:**
```go
// Create term via API (simulates UI or direct API call)
func createTermViaAPI(t *testing.T, projectID uuid.UUID, term *models.BusinessGlossaryTerm) uuid.UUID

// Create term via MCP tool (simulates client)
func createTermViaMCP(t *testing.T, projectID uuid.UUID, input CreateGlossaryTermInput) (*CreateGlossaryTermResponse, error)
```

### Manual Testing

**Test with Claude Desktop as MCP client:**
1. Connect Claude Desktop to local MCP server: `http://localhost:3443/mcp/{project-id}`
2. Enable developer tools in UI
3. Ask Claude: "Create a glossary term for 'Daily Active Users' defined as SELECT COUNT(DISTINCT user_id) FROM events WHERE event_date = CURRENT_DATE"
4. Verify term appears in UI with source=client
5. Ask Claude: "Update the Daily Active Users definition to include WHERE deleted_at IS NULL"
6. Verify SQL changes and source remains client
7. Ask Claude: "Delete the Daily Active Users term"
8. Verify term removed from UI

---

## Migration Path

### Phase 1: MCP Tool Implementation (This Design)

**Deliverables:**
- Three new MCP tools: create, update, delete
- Tool registration and filtering logic
- Comprehensive unit tests
- Integration tests with mock MCP client
- Updated tool filter map in developer.go

**No Schema Changes:** Database already supports source=client (constraint includes 'client' value)

**No Service Changes:** Service layer already validates SQL for all sources

### Phase 2: UI Enhancements (Future)

**Source badges:**
- Display source value (inferred/manual/client) on each term in UI
- Different colors/icons for each source
- Filter terms by source

**Conflict resolution:**
- When client updates manual term, show warning in UI
- "This term was modified by an MCP client on [date]"
- Allow user to "reclaim" term (set source back to manual)

**Audit log:**
- Show history of client modifications
- Link to MCP conversation ID if available
- Show before/after SQL diffs

### Phase 3: Advanced Features (Future)

**Approval workflow:**
- Client updates go to "pending" state
- User reviews and approves/rejects
- Rejected terms don't appear in list_glossary

**Versioning:**
- Keep history of SQL changes
- Allow rollback to previous versions
- Show diff between versions

**Bulk operations:**
- `import_glossary_terms` tool accepting array
- Transaction rollback on any failure
- Progress reporting for large imports

---

## Open Questions

1. **Should clients be able to delete inferred terms?**
   - Current design: Yes (no restrictions)
   - Concern: Client might delete important LLM-discovered terms
   - Mitigation: UI can warn before allowing manual deletion of client-modified terms

2. **Should source change when client updates inferred/manual terms?**
   - Current design: Yes (source becomes 'client')
   - Alternative: Add `last_modified_by_source` separate field
   - Decision: Keep simple, use single source field for "last touch"

3. **Should we log which MCP client made the change?**
   - Current design: No (created_by/updated_by are NULL for client terms)
   - Alternative: Add `client_id` field to track which MCP session
   - Deferred to Phase 2 (audit log)

4. **Should term updates require testing SQL every time?**
   - Current design: Yes if SQL changed, no if only definition/aliases changed
   - Service layer already implements this optimization
   - Trade-off: Safety vs performance (current design favors safety)

---

## Implementation Checklist

When implementing this design:

- [ ] Create MCP tool definitions in `pkg/mcp/tools/glossary.go`
  - [ ] `create_glossary_term` tool with input schema
  - [ ] `update_glossary_term` tool with input schema
  - [ ] `delete_glossary_term` tool with input schema

- [ ] Implement tool handlers
  - [ ] `makeCreateGlossaryTermHandler` function
  - [ ] `makeUpdateGlossaryTermHandler` function
  - [ ] `makeDeleteGlossaryTermHandler` function

- [ ] Update tool registration
  - [ ] Add write tools to `RegisterGlossaryTools`
  - [ ] Update `glossaryToolNames` map in `developer.go`
  - [ ] Add filtering logic for developer tools toggle

- [ ] Write unit tests (`pkg/mcp/tools/glossary_test.go`)
  - [ ] Test create with valid SQL
  - [ ] Test create with invalid SQL
  - [ ] Test create with duplicate term
  - [ ] Test update existing term
  - [ ] Test update by alias
  - [ ] Test update with invalid SQL
  - [ ] Test delete existing term
  - [ ] Test delete by alias
  - [ ] Test delete non-existent term

- [ ] Write integration tests (`pkg/handlers/glossary_integration_test.go`)
  - [ ] Full CRUD cycle via MCP tools
  - [ ] Source attribution (verify source=client)
  - [ ] SQL validation integration
  - [ ] Alias handling

- [ ] Manual testing
  - [ ] Test with Claude Desktop
  - [ ] Verify developer tools toggle
  - [ ] Verify UI displays client-created terms
  - [ ] Test update/delete from UI after client creation

- [ ] Documentation
  - [ ] Update MCP server documentation
  - [ ] Add examples to CLAUDE.md if needed
  - [ ] Update PLAN-glossary-finalize.md to mark task complete

---

## References

- **Plan File:** `PLAN-glossary-finalize.md` (Phase 4, Task 4.2)
- **Database Schema:** `migrations/031_glossary_defining_sql.up.sql`
- **Repository:** `pkg/repositories/glossary_repository.go`
- **Service:** `pkg/services/glossary_service.go`
- **Existing MCP Tools:** `pkg/mcp/tools/glossary.go` (list_glossary, get_glossary_sql)
- **Tool Filtering:** `pkg/mcp/tools/developer.go`
- **Models:** `pkg/models/glossary.go`

---

## Approval

This design is ready for implementation when:
1. Client-driven glossary updates are prioritized
2. Developer tools toggle UI is mature
3. MCP protocol stability is confirmed

Until then, this document serves as reference for future work. The existing service and repository layers already support these operations - only MCP tool exposure remains.

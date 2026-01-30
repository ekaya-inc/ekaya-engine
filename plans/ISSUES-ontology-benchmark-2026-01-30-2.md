# ISSUES: Ontology Benchmark - Additional Findings - 2026-01-30

Issues discovered during continued MCP API usage after initial testing.

---

## Feature Requests

### 7. Missing `update_table` Tool for Table-Level Metadata

**Severity**: MEDIUM (Feature Request)
**Status**: Open - Split into subtasks

**Description**: There is no tool to update table-level metadata. Currently available update tools are:
- `update_column` - column metadata
- `update_entity` - entity metadata
- `update_relationship` - relationships
- `update_project_knowledge` - general facts
- `update_glossary_term` - business terms

**Use Case**: Need to mark tables as "ephemeral" or "not for analytics". For example, the `sessions` table is transient and should not be used for analytics queries - `billing_engagements` should be used instead.

**Current Workaround**: Using `update_project_knowledge` to capture table-level guidance, but this is less discoverable than table metadata would be.

---

#### [x] 7.1 Add Database Schema and Repository for Table Metadata

**Goal**: Create the database schema and data access layer for storing table-level metadata.

**Database Schema** (`migrations/` - create new migration file):
Create table `engine_table_metadata` with columns:
- `id` (UUID, PK)
- `project_id` (UUID, FK to engine_projects, for RLS)
- `datasource_id` (UUID, FK to engine_datasources)
- `table_name` (VARCHAR, the table this metadata describes)
- `description` (TEXT, what the table represents)
- `usage_notes` (TEXT, when to use/not use this table)
- `is_ephemeral` (BOOLEAN, default false, for transient tables)
- `preferred_alternative` (VARCHAR, table to use instead if ephemeral)
- `created_at`, `updated_at` timestamps
- `provenance` (VARCHAR, 'admin'/'mcp'/'inference' like other ontology tables)

Add unique constraint on `(project_id, datasource_id, table_name)`.
Apply RLS policy using `app.current_project_id` pattern matching other ontology tables.

**Repository** (`pkg/repositories/table_metadata_repository.go`):
Create `TableMetadataRepository` interface and implementation with methods:
- `Upsert(ctx, projectID, datasourceID, tableName string, metadata TableMetadata) error`
- `Get(ctx, projectID, datasourceID, tableName string) (*TableMetadata, error)`
- `List(ctx, projectID, datasourceID string) ([]TableMetadata, error)`
- `Delete(ctx, projectID, datasourceID, tableName string) error`

Follow existing repository patterns in `pkg/repositories/` for error handling and connection management.

**Model** (`pkg/models/table_metadata.go`):
```go
type TableMetadata struct {
    ID                   string
    ProjectID            string
    DatasourceID         string
    TableName            string
    Description          *string
    UsageNotes           *string
    IsEphemeral          bool
    PreferredAlternative *string
    Provenance           string
    CreatedAt            time.Time
    UpdatedAt            time.Time
}
```

---

#### [x] 7.2 Implement update_table MCP Tool

**Goal**: Create the MCP tool that allows setting table-level metadata.

**Tool Registration** (`pkg/mcp/tools/ontology_tools.go` or new file `pkg/mcp/tools/table_tools.go`):
Register `update_table` tool following the pattern of `update_column`, `update_entity`, etc.

**Tool Definition**:
```
Name: update_table
Description: Add or update metadata about a table. Use this to document table purpose, mark tables as ephemeral/transient, or indicate preferred alternatives. The table name is the upsert key - if metadata exists for this table, it will be updated; otherwise, new metadata is created.

Parameters:
- table (string, required): Table name to update (e.g., 'sessions', 'billing_transactions')
- description (string, optional): What this table represents and contains
- usage_notes (string, optional): When to use or not use this table for queries
- is_ephemeral (boolean, optional): Mark as transient/temporary table not suitable for analytics
- preferred_alternative (string, optional): Table to use instead if this one is ephemeral or deprecated
```

**Handler** (`pkg/mcp/tools/handlers/` or inline):
- Validate table exists in schema (query `engine_schema_tables`)
- If `preferred_alternative` is set, validate that table also exists
- Call repository `Upsert` method
- Return success message with what was updated

**Wire up**:
- Add to tool filter in `pkg/mcp/tools/developer.go` (requires Developer Tools enabled)
- Add to tool registry in server initialization

**Example usage the tool should support**:
```
update_table(
  table='sessions',
  description='Transient session tracking table',
  usage_notes='Do not use for analytics. Sessions are deleted after 24 hours.',
  is_ephemeral=true,
  preferred_alternative='billing_engagements'
)
```

---

#### [x] 7.3 Add delete_table_metadata MCP Tool

**Goal**: Create companion tool to remove table metadata (matching the pattern of other ontology tools like `delete_column_metadata`).

**Tool Definition**:
```
Name: delete_table_metadata
Description: Clear custom metadata for a table, removing semantic enrichment. Use this to remove incorrect or outdated table annotations.

Parameters:
- table (string, required): Table name to clear metadata for
```

**Handler**:
- Call repository `Delete` method
- Return success/not-found message

**Wire up**: Same pattern as 7.2 - add to tool filter and registry.

---

#### [ ] 7.4 Surface Table Metadata in get_context

**Goal**: Make table metadata visible in the primary discovery workflow so agents see it without having to probe each table.

**Modify `get_context`** (`pkg/mcp/tools/context_tools.go` or equivalent):

At `depth='tables'` and `depth='columns'`, include table metadata in the output. For each table, if metadata exists, add:
```json
{
  "table": "sessions",
  "row_count": 15000,
  "description": "Transient session tracking table",
  "usage_notes": "Do not use for analytics...",
  "is_ephemeral": true,
  "preferred_alternative": "billing_engagements",
  "columns": [...]
}
```

**Implementation**:
1. In the context builder, after fetching schema tables, also fetch table metadata via repository
2. Create a map of `tableName -> TableMetadata` for efficient lookup
3. When building table output objects, merge in metadata fields if present
4. Null/missing metadata fields should be omitted from output (don't show `"description": null`)

**Also update `get_ontology`** if it has separate table display logic - ensure consistency.

---

### 8. `update_project_knowledge` Fact Field Has 255 Character Limit

**Severity**: LOW (UX)
**Status**: Open

**Description**: The `fact` parameter in `update_project_knowledge` is limited to 255 characters, which is too short for complex business rules.

**Reproduction**:
```
update_project_knowledge(
  fact="billing_engagements tracks ALL engagements (including free ones with no charges). billing_transactions tracks only engagements where money was actually charged. An engagement can exist without billing if: (1) it's a free engagement, (2) it ended before the first tik (15 seconds), or (3) the host set a $0 fee_per_minute.",
  category="business_rule"
)
```

**Observed error**:
```
ERROR: value too long for type character varying(255) (SQLSTATE 22001)
```

**Recommendation**:
- Increase `fact` column to TEXT or VARCHAR(1000)
- Or provide a clearer error message indicating the character limit

---

### 9. `create_approved_query` Requires Undiscoverable datasource_id

**Severity**: MEDIUM (UX)
**Status**: Open

**Description**: The `create_approved_query` tool requires a `datasource_id` parameter, but there's no obvious way to discover this UUID. The `health` endpoint returns `project_id` but not `datasource_id`.

**Reproduction**:
```
create_approved_query(
  name="Test Query",
  description="Test",
  sql="SELECT 1",
  datasource_id="???"  -- Where do I get this?
)
```

**Workaround**: Use `suggest_approved_query` instead, which auto-detects the datasource and doesn't require the parameter. Then approve the suggestion.

**Recommendation**: Either:
1. Add `datasource_id` to the `health` endpoint response
2. Make `datasource_id` optional in `create_approved_query` (auto-detect like `suggest_approved_query` does)
3. Add a `list_datasources` tool

### 10. `get_context` Does Not Surface Enriched Column Metadata

**Severity**: HIGH
**Status**: Open

**Description**: Column descriptions added via `update_column` are stored but not included in `get_context` output at the columns depth level. This defeats the purpose of metadata enrichment.

**Reproduction**:
```
# Add enriched metadata
update_column(table='billing_engagements', column='host_id',
  description='Use this to find all hosts who had engagements')

# Check get_context - description is missing
get_context(depth='columns', tables=['billing_engagements'])
# Returns columns with data types but NO descriptions

# Only probe_column shows the metadata
probe_column(table='billing_engagements', column='host_id')
# Returns: {"description": "Use this to find all hosts..."}
```

**Impact**: A fresh agent using the recommended progressive disclosure workflow (`get_context` at increasing depths) will never see enriched column metadata. They'd have to probe each column individually.

**Recommendation**: Include column descriptions, entity, and role in `get_context` columns output when available.

---

### 11. `get_context` Does Not Include Project Knowledge

**Severity**: HIGH
**Status**: Open

**Description**: Project knowledge (business rules added via `update_project_knowledge`) is not surfaced in `get_context`. Critical guidance like "sessions table is ephemeral" or "use billing_engagements not billing_transactions for engagement counts" is invisible to agents using the primary discovery workflow.

**Current depth levels**:
```
domain → entities → tables → columns
```

**Recommendation**: Add project knowledge to `get_context`. Options:
1. New depth level: `domain → project → entities → tables → columns`
2. Include at domain level as a `project_knowledge` section
3. Add as an `include` option: `include: ['project_knowledge']`

Option 2 (include at domain level) seems cleanest since project knowledge is high-level guidance about how to use the database.

---

## Summary

| Issue # | Type | Severity | Description |
|---------|------|----------|-------------|
| 7 | Feature Request | MEDIUM | Missing `update_table` tool for table-level metadata (split into 7.1-7.4) |
| 8 | UX | LOW | `update_project_knowledge` fact limited to 255 chars |
| 9 | UX | MEDIUM | `create_approved_query` requires undiscoverable datasource_id |
| 10 | Bug | HIGH | `get_context` doesn't surface enriched column metadata |
| 11 | Feature Gap | HIGH | `get_context` doesn't include project knowledge |

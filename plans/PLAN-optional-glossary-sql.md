# PLAN: Make Glossary DefiningSQL Optional

**Status:** IMPLEMENTED

## Context

Glossary terms currently **require** a SQL statement at every layer (DB, service, MCP, UI). This was the original design when glossary was part of the DAG pipeline where every term went through discovery+enrichment. Now that glossary generation runs independently (post-questions), we see terms that are schema-derived business concepts without direct SQL representation. These are distinct from project knowledge: glossary terms are tied to the schema and inferred from ontology analysis, but not every term maps to an executable SQL statement.

The provenance bug fix (adding `WithInferredProvenance` to `RunAutoGenerate`) exposed this design issue: 15 terms were discovered but all had empty SQL because enrichment hadn't run yet. Even after enrichment, some terms legitimately don't have SQL definitions.

## Changes

### 1. Database Migration (`migrations/029_glossary_optional_sql.up.sql`)

- `ALTER TABLE engine_business_glossary ALTER COLUMN defining_sql DROP NOT NULL;`
- `ALTER TABLE engine_business_glossary ALTER COLUMN defining_sql SET DEFAULT '';`
- Update column comment to reflect optionality

Create corresponding `.down.sql` that restores `NOT NULL` (with `UPDATE` to set empty strings to `''` first to avoid constraint violation).

### 2. Service Layer (`pkg/services/glossary_service.go`)

**Remove** the `defining_sql is required` validation from:
- `CreateTerm()` (line ~171-172) - remove the `if term.DefiningSQL == ""` check
- `UpdateTerm()` (line ~245-246) - remove the `if term.DefiningSQL == ""` check

**Keep** existing SQL validation logic for when SQL **is** provided:
- SQL testing/validation in `CreateTerm` still runs when `DefiningSQL != ""`
- SQL testing/validation in `UpdateTerm` still runs when SQL changes
- Output column capture still happens when SQL is present

**Enrichment phase** (`enrichSingleTerm`, line ~1100): Keep treating empty LLM SQL response as a retry-worthy failure. If enrichment ultimately fails, the term persists with `enrichment_status=failed` and empty SQL - this is now acceptable.

### 3. Handler Layer (`pkg/handlers/glossary_handler.go`)

- Line ~172: Remove `"defining_sql is required"` from the 400-error string matching in create handler
- Line ~263: Remove `"defining_sql is required"` from the 400-error string matching in update handler

These are just error message routing — they'll naturally stop matching since the service no longer returns that error.

### 4. MCP Tools (`pkg/mcp/tools/glossary.go`)

**`create_glossary_term` tool** (line ~217):
- Change `defining_sql` from `mcp.Required()` to optional
- Update tool description to indicate SQL is optional

**`upsert_glossary_term` tool** (line ~394-396):
- Remove the `if sql == "" { "sql is required when creating a new term" }` check
- SQL remains optional for both create and update paths

### 5. UI: GlossaryTermEditor (`ui/src/components/GlossaryTermEditor.tsx`)

**Remove SQL as a required field:**
- Line ~167-170: Remove the `if (!definingSql.trim())` save-blocking validation
- Line ~172-175: Change the `sqlTested` validation to only apply when SQL is non-empty
- Line ~247-252: Update `canSave` to not require `definingSql.trim()` or `sqlTested`

**Conditional SQL section:**
- Keep the SQL editor visible (users can still add SQL optionally)
- "Test SQL" button stays, but only gates saving when SQL is present
- If SQL is empty, skip validation entirely and allow save

### 6. UI: GlossaryPage (`ui/src/pages/GlossaryPage.tsx`)

- Line ~548: The `hasSqlDetails` check already handles empty `defining_sql` - no change needed
- Line ~617: SQL display already conditional on `term.defining_sql` - no change needed
- Consider showing enrichment_status badge for inferred terms (e.g., "Pending SQL", "SQL Failed") so users know which terms haven't been enriched yet - this is optional/nice-to-have

### 7. Migration Test (`pkg/database/032_glossary_defining_sql_test.go`)

- Update the column type assertion at line ~50 to verify `defining_sql` is nullable (check `is_nullable = 'YES'`)

## Files to Modify

| File | Change |
|------|--------|
| `migrations/029_glossary_optional_sql.up.sql` | New migration: DROP NOT NULL |
| `migrations/029_glossary_optional_sql.down.sql` | New migration: restore NOT NULL |
| `pkg/database/032_glossary_defining_sql_test.go` | Update nullability assertion |
| `pkg/services/glossary_service.go` | Remove required checks in Create/UpdateTerm |
| `pkg/handlers/glossary_handler.go` | Remove error string matching for defining_sql |
| `pkg/mcp/tools/glossary.go` | Make defining_sql optional in create + upsert tools |
| `ui/src/components/GlossaryTermEditor.tsx` | Make SQL field optional, conditional validation |
| `ui/src/pages/GlossaryPage.tsx` | No changes required (already handles empty SQL) |

## Verification

1. `make check` - all tests pass (backend + frontend)
2. Run `make dev-server` + `make dev-ui`, navigate to glossary page
3. Create a term manually via UI **without** SQL - should save successfully
4. Create a term manually via UI **with** SQL - should still require Test SQL before save
5. Run "Auto-Generate Terms" - terms should be created (with or without SQL after enrichment)
6. Via MCP `create_glossary_term` tool - create term without `defining_sql` parameter
7. Via MCP `upsert_glossary_term` tool - create term without `sql` parameter

# BUGS: Ontology Testing (2026-01-21)

Discovered during systematic testing of ontology extraction after completing BUGS-ontology-mix.md fixes.

## Summary

| Bug | Description | Severity | Status |
|-----|-------------|----------|--------|
| 1 | Entity name singularization still incorrect | Medium | **CLOSED** (stale data) |
| 2 | update_glossary_term nil pointer dereference | High | Open → FIX-mcp-update-bugs.md |
| 3 | update_project_knowledge duplicate key on fact_id update | Medium | Open → FIX-mcp-update-bugs.md |

**Previous bugs verified as FIXED:**
- Bug 1 (refresh_schema auto_select): UI correctly shows selected tables
- Bug 3 (probe_relationship MCP-created): Returns MCP-created relationships
- Bug 4 (approved column metadata): probe_column returns MCP-added enums
- Bug 5 (Entity Discovery preserves manual): Manual entities survive extraction
- Bug 6 (get_entity extraction entities): Works for extraction-created entities
- Bug 7 (self-referential FK): Schema detects s4_employees.manager_id -> id
- Bug 8 (junction table FK): Schema detects s3_enrollments FKs to both tables

---

## Bug 1: Entity Name Singularization - CLOSED (Stale Data)

**Severity:** Medium
**Component:** Schema Refresh / Entity Naming
**Status:** CLOSED
**Resolution:** The bad singularization data was from BEFORE the fix in FIX-bug2-singularization-errors.md was applied. The `toEntityName()` function now works correctly.

**Evidence of Fix Working:**
```bash
# Tests pass for all patterns including numbered prefixes:
go test -v -run TestToEntityName ./pkg/services/...
# s4_categories → S4_category (PASS)
# s5_activities → S5_activity (PASS)

# Current pending changes show correct singularization:
# S1_customer, S1_order, S10_event, S10_user_preference, etc.
```

**Original Issue:**
The bad entity names (`S4_categorie`, `S5_activitie`) were created before the inflection library fix and persisted in the database. New pending changes use correct singularization.

**Action:** Clear stale pending changes if needed, or let them be rejected/approved naturally.

---

## Testing Notes

### Extraction Progress (3/9 steps complete)
- Entity Discovery: Completed
- Entity Enrichment: Completed (entities now have semantic names like "Employee", "Category")
- Foreign Key Discovery: Completed
- Column Enrichment: Running (2/59 tables)
- Remaining: Primary Key Match Discovery, Relationship Enrichment, Ontology Finalization, Glossary Discovery, Glossary Enrichment

### Manual Entity Preservation (Bug 5) - VERIFIED
Created `FreshManualTest2026` entity via MCP before extraction. After Entity Discovery AND Entity Enrichment AND FK Discovery completed, the entity is still present with its description and aliases intact. This confirms the provenance-aware deletion is working correctly.

### Self-Referential FK (Bug 7) - VERIFIED AT ONTOLOGY LEVEL
After FK Discovery completed, entity occurrences show self-referential relationships:
- **Employee entity**: `occurrences: [{"table":"s4_employees","column":"id"},{"table":"s4_employees","column":"manager_id"}]`
- **Category entity**: `occurrences: [{"table":"s4_categories","column":"id"},{"table":"s4_categories","column":"parent_category_id"}]`

### Schema-Level FK Detection (Bug 8)
Junction table FKs are correctly detected at the schema level:
- `s3_enrollments.student_id -> s3_students.student_id (N:1)`
- `s3_enrollments.course_code -> s3_courses.course_code (N:1)`

### Junction Table Entity
Note: `s3_enrollments` does not appear as a separate entity in the ontology. The Student and Course entities don't show occurrences in the junction table. This appears to be by design - junction tables are treated as relationship bridges rather than domain entities. The FKs are detected at schema level but no entity is created for pure junction tables without additional business columns.

---

## Bug 2: update_glossary_term Nil Pointer Dereference

**Severity:** High
**Component:** MCP Tools / Glossary
**Status:** Open
**File:** `pkg/mcp/tools/glossary.go:320`

**Description:**
When updating an existing glossary term, the code dereferences `term.Source` but `term` is nil at that point. It should be `existing.Source`.

**Code Location:**
```go
// Line 317-320 in glossary.go
} else {
    // Update existing term
    // Check precedence: can MCP modify this term?
    if !canModifyGlossaryTerm(term.Source, models.GlossarySourceMCP) {  // BUG: term is nil here!
```

**Expected Behavior:**
Should use `existing.Source` instead of `term.Source`.

**Actual Behavior:**
Causes nil pointer dereference (fetch failed error from MCP client due to server panic).

**Fix:**
Change line 320 from:
```go
if !canModifyGlossaryTerm(term.Source, models.GlossarySourceMCP) {
```
to:
```go
if !canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP) {
```

---

## Bug 3: update_project_knowledge Duplicate Key Error

**Severity:** Medium
**Component:** MCP Tools / Project Knowledge
**Status:** Open

**Description:**
When calling `update_project_knowledge` with a `fact_id` to update an existing fact, the operation fails with a duplicate key constraint violation.

**Error:**
```
MCP error -32603: failed to upsert project knowledge: failed to upsert knowledge fact: ERROR: duplicate key value violates unique constraint "engine_project_knowledge_pkey" (SQLSTATE 23505)
```

**Steps to Reproduce:**
1. Create a fact: `update_project_knowledge(fact="Test fact", category="business_rule")`
2. Get the returned `fact_id`
3. Try to update: `update_project_knowledge(fact_id="<uuid>", fact="Updated fact")`
4. Error occurs

**Expected Behavior:**
When `fact_id` is provided, the existing record should be updated (not inserted).

**Actual Behavior:**
The operation attempts to insert a new record, violating the primary key constraint.

---

## MCP↔UI Integration Test Results (2026-01-21)

### Passing Tests
| Feature | Create | Read | Update | Delete |
|---------|--------|------|--------|--------|
| Entity | PASS | PASS | PASS | PASS |
| Relationship | PASS | PASS | PASS | PASS |
| Column Metadata | PASS | PASS | N/A | PASS |
| Glossary Term | PASS | PASS | **BUG** | PASS |
| Project Knowledge | PASS | N/A | **BUG** | PASS |
| Pending Changes | N/A | PASS | PASS (approve/reject) | N/A |
| Approved Queries | PASS (suggest) | PASS | N/A | N/A |
| get_context | N/A | PASS (all depths) | N/A | N/A |

### Notes
- Column enum values are stored correctly via MCP but not displayed in Schema Selection UI (design choice, not bug)
- Approved queries require UI/admin approval before execution (expected behavior)
- Ontology questions: none available to test (extraction complete)

# ISSUES: Ontology Enrichment MCP Tools - 2026-01-31

**Status:** MOSTLY COMPLETE (2026-02-05) - Most issues fixed or N/A due to entity removal. Only low-priority items remain.

Issues observed during comprehensive MCP tool testing on tikr_production.

---

## Issue 1: Server Crash on `sample` Tool (FIXED)

**Severity:** Critical → **FIXED**
**Status:** Verified fixed in latest build

**What Happened (Before Fix):**
- Calling `sample(table="users", limit=3)` crashed the entire MCP server
- All subsequent calls failed with "Unable to connect"
- Required server restart to recover

**Sequence That Triggered Crash:**
```
1. health() ✓
2. echo() ✓
3. get_schema() ✓
4. get_context() ✓
5. get_ontology() ✓
6. search_schema() ✓
7. query(sql="SELECT COUNT(*)...") ✓
8. sample(table="users", limit=3) ✗ CRASH
```

**Fix Verified:** After fix applied, same sequence completes successfully.

---

## Issue 2: `get_context` Returns Only 2 Promoted Entities

**Severity:** Medium
**Status:** N/A - Entities Removed

**What Happened:** Entity promotion logic was too conservative.

**Resolution:** Entities have been removed from the ontology model. `get_context(depth='entities')` now returns tables directly from the schema, not promoted entities. This issue is no longer applicable.

---

## Issue 3: `table_count` Wrong in Domain Context

**Severity:** Low
**Status:** FIXED (entity removal)

**What Happened:** `table_count` showed 2 instead of 38+ tables.

**Resolution:** This was caused by the ontology entity promotion logic only showing 2 "promoted" entities. With entities removed, `table_count` now correctly uses `len(schema.Tables)` which returns all selected tables. See `pkg/mcp/tools/context.go:289`.

---

## Issue 4: `relationships` Always Null in get_context

**Severity:** Medium
**Status:** N/A - Entity-based relationships removed

**What Happened:** `include_relationships=true` returned null in get_context.

**Resolution:** Entity-to-entity relationships have been removed. FK relationships are now part of the schema model and surfaced through `get_schema` and column metadata. The `include_relationships` parameter behavior changed with the entity removal.

---

## Issue 5: Duplicate Glossary Terms

**Severity:** Low
**Observed:** Near-duplicate glossary terms with identical SQL

**What Happened:**
```
- "Engagement Quality Score" → AVG(reviewee_rating)
- "Engagement Review Score" → AVG(reviewee_rating)
```

Both calculate the same thing with nearly identical SQL.

**Expected:**
- Glossary terms should be deduplicated
- Or clearly differentiate their purpose

---

## Issue 6: `probe_relationship` Returns Empty for Valid Entity

**Severity:** Medium
**Status:** N/A - Tool Removed

**What Happened:** `probe_relationship(from_entity='User')` returned empty.

**Resolution:** The `probe_relationship` tool was removed as part of the entity removal refactor. FK relationships are now discovered through the schema introspection pipeline and available via `get_schema` and column metadata.

---

## Issue 7: `probe_column` Shows Wrong Purpose for Numeric

**Severity:** Low
**Observed:** `avg_rating` column shows purpose "text"

**What Happened:**
```
probe_column(table='users', column='avg_rating')
→ "purpose": "text", "semantic_type": "unknown", "confidence": 0.5
```

The column is `numeric` type containing decimal ratings (0.0-5.0).

**Expected:**
- purpose: "measure" or "rating"
- semantic_type: "rating" or "numeric"

---

## Issue 8: `marker_at` Semantic Role Incorrect

**Severity:** Low
**Observed:** marker_at described as "primary_key"

**What Happened:**
```
probe_columns → users.marker_at
→ "role": "primary_key", "description": "A numeric identifier for tracking user markers"
```

marker_at is a nanosecond Unix timestamp for cursor-based pagination, not a primary key.

**Expected:**
- role: "dimension" or "timestamp"
- description: "Nanosecond Unix timestamp for cursor-based pagination"

---

## Issue 9: Project Knowledge Has Duplicate Entries

**Severity:** Low
**Observed:** Same fact appears twice in project_overview

**What Happened:**
```
get_context(depth='domain') → project_knowledge.project_overview:
[
  {"fact": "Tikr is a social media application..."},
  {"fact": "Tikr is a social media application..."} // exact duplicate
]
```

**Expected:**
- Deduplicate project knowledge entries

---

## Issue 10: No Bulk resolve_ontology_question

**Severity:** Medium
**Observed:** Had to call resolve 124 times individually

**What Happened:**
- Each question requires separate API call
- No batch resolution option
- For pattern-based answers (17 soft-delete questions), same answer repeated 17 times

**Expected:**
- `resolve_ontology_questions` (plural) for bulk operations
- Or resolve by pattern/category

---

## Issue 11: Question Priority Filter Not Default

**Severity:** Low
**Observed:** All priorities mixed in results

**What Happened:**
- Questions have priority 1-3 (1=critical, 3=nice-to-have)
- list_ontology_questions returns all priorities mixed
- No way to get only P1 questions by default

**Note:** Priority filter IS available but not used by default.

---

## Issue 12: deleted_at Semantic Type is "audit_updated"

**Severity:** Low
**Observed:** deleted_at labeled as audit_updated instead of soft_delete

**What Happened:**
```
get_column_metadata(table='users', column='deleted_at')
→ "semantic_type": "audit_updated"
```

**Expected:**
- semantic_type: "soft_delete" or "deletion_timestamp"
- Should recognize GORM soft-delete pattern

---

## Tools Tested Successfully

| Tool | Status | Notes |
|------|--------|-------|
| `health` | ✓ OK | |
| `echo` | ✓ OK | |
| `get_schema` | ✓ OK | |
| `get_context` | ✓ OK | relationships null issue |
| `get_ontology` | ✓ OK | table_count wrong |
| `search_schema` | ✓ OK | Finds entities not in get_context |
| `query` | ✓ OK | |
| `sample` | ✓ OK | **Fixed** - was crashing |
| `validate` | ✓ OK | Returns proper error for invalid SQL |
| `explain_query` | ✓ OK | Returns plan + hints |
| `execute` | ✓ OK | |
| `get_column_metadata` | ✓ OK | semantic_type issues |
| `get_entity` | ✓ OK | |
| `probe_column` | ✓ OK | purpose/type issues |
| `probe_columns` | ✓ OK | Batch works |
| `probe_relationship` | ✓ OK | Returns empty for User |
| `list_glossary` | ✓ OK | 10 terms, 100% valid |
| `get_glossary_sql` | ✓ OK | Returns SQL + metadata |
| `list_ontology_questions` | ✓ OK | Pagination needed >100 |
| `list_pending_changes` | ✓ OK | Returns empty (none pending) |
| `list_approved_queries` | ✓ OK | 1 query found |
| `execute_approved_query` | ✓ OK | Returns results correctly |
| `get_query_history` | ✓ OK | Returns empty (none recent) |
| `scan_data_changes` | ✓ OK | Returns empty (no changes) |
| `list_query_suggestions` | ✓ OK | Returns empty |
| `refresh_schema` | ✓ OK | No changes detected |

---

## Tools Not Tested (Write Operations)

Skipped to avoid modifying production data:

| Tool | Reason |
|------|--------|
| `update_column` | Write operation |
| `update_entity` | Write operation |
| `update_relationship` | Write operation |
| `update_table` | Write operation |
| `update_project_knowledge` | Write operation |
| `update_glossary_term` | Write operation |
| `create_glossary_term` | Write operation |
| `create_approved_query` | Write operation |
| `delete_*` tools | Write operation |
| `approve_change` | Write operation |
| `reject_change` | Write operation |

---

---

## Issue 13: Server Crash on `approve_all_changes` (FIXED)

**Severity:** Critical
**Observed:** 2026-01-31 on test_data
**Status:** FIXED

**What Happened:**
Calling `approve_all_changes()` crashed the server with a nil pointer dereference.

**Stack Trace:**
```
ERROR  incremental-dag  services/incremental_dag_service.go:171
  Failed to process new table
  {"table_name": "public.mcp_test_users",
   "error": "get active ontology: failed to scan ontology: timeout: context already done: context canceled"}

panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x2 addr=0x38 pc=0x10339622c]

goroutine 4699 [running]:
github.com/jackc/pgx/v5/pgxpool.(*Conn).QueryRow(...)
github.com/ekaya-inc/ekaya-engine/pkg/repositories.(*ontologyRepository).GetActive(...)
    pkg/repositories/ontology_repository.go:134
github.com/ekaya-inc/ekaya-engine/pkg/services.(*incrementalDAGService).processNewTable(...)
    pkg/services/incremental_dag_service.go:220
github.com/ekaya-inc/ekaya-engine/pkg/services.(*incrementalDAGService).ProcessChanges(...)
    pkg/services/incremental_dag_service.go:170
github.com/ekaya-inc/ekaya-engine/pkg/services.(*changeReviewService).ApproveAllChanges.func1()
    pkg/services/change_review_service.go:192
```

**Root Cause Analysis:**
1. `ApproveAllChanges` spawns a goroutine at `change_review_service.go:191`
2. The goroutine calls `ProcessChanges` which tries to process a "new_table" change
3. `processNewTable` at line 220 calls `ontologyRepository.GetActive()`
4. The context is already canceled by the time the goroutine runs
5. The canceled context causes the connection pool to return a nil connection
6. Code at `ontology_repository.go:134` doesn't check for nil before using the connection

**Key Files:**
- `pkg/services/change_review_service.go:191-192` - spawns goroutine with potentially stale context
- `pkg/services/incremental_dag_service.go:170,220` - ProcessChanges and processNewTable
- `pkg/repositories/ontology_repository.go:134` - nil pointer dereference location

**Fix Needed:**
1. Don't use request context in background goroutines - create a new context with timeout
2. Add nil check for connection before use
3. Consider making incremental DAG processing synchronous or properly async with worker pattern

**Sequence That Triggered Crash:**
```
1-45. [Various MCP tool calls] ✓
46. approve_all_changes() ✗ CRASH
    → Approved 89 changes (returned success to client)
    → Background goroutine triggered for "new_table" processing
    → Context canceled, nil connection, SIGSEGV
```

**Fix Applied:**
1. Added `ProcessChangesAsync(projectID, changes)` method to `IncrementalDAGService` interface
2. This method creates its own `context.Background()` and gets a fresh tenant context
3. Updated `ApproveAllChanges` to call `ProcessChangesAsync` instead of inline goroutine
4. Added test `TestApproveAllChanges_UsesCancelSafeAsync` to verify the fix

**Files Changed:**
- `pkg/services/incremental_dag_service.go` - Added ProcessChangesAsync method
- `pkg/services/change_review_service.go` - Use ProcessChangesAsync instead of inline goroutine
- `pkg/services/change_review_service_test.go` - Added regression test
- `pkg/services/incremental_dag_service_test.go` - Updated mock

---

## Issue 14: `create_glossary_term` Rejects Test-Like Names (Low)

**Severity:** Low (UX)
**Observed:** 2026-01-31 on test_data

**What Happened:**
```
create_glossary_term(term="Test Metric", ...)
→ {"error": "term name appears to be test data - use a real business term"}
```

The tool rejected "Test Metric" as a term name, requiring use of a realistic business term like "Active User Count".

**Expected:**
- On test databases, allow test data creation
- Or document this validation requirement

**Workaround:** Use realistic business term names even in test scenarios.

---

## Issue 14: `create_approved_query` Requires output_column_descriptions

**Severity:** Low (Documentation)
**Observed:** 2026-01-31 on test_data

**What Happened:**
```
create_approved_query(name="User Activity Report", sql="SELECT id...", ...)
→ {"error": "output_column_descriptions parameter is required"}
```

The tool requires `output_column_descriptions` even though it's marked as optional in the schema.

**Expected:**
- Either make the parameter truly optional (auto-detect from SQL)
- Or mark it as required in the tool description

**Workaround:** Always provide output_column_descriptions:
```json
{"id": "User ID", "username": "Username", "created_at": "Creation timestamp"}
```

---

## Issue 15: `update_relationship` Requires Pre-existing Entities

**Severity:** Low (Documentation)
**Status:** N/A - Tool Removed

**What Happened:** `update_relationship` required entities to exist first.

**Resolution:** The `update_relationship` tool was removed as part of the entity removal refactor. Relationships are now discovered automatically through FK introspection.

---

## Write Operations Test Results (test_data 2026-01-31)

All write operations now tested on test_data database:

| Tool | Status | Notes |
|------|--------|-------|
| `update_entity` | ✓ OK | Create, update, verify all work |
| `delete_entity` | ✓ OK | Soft delete works |
| `update_relationship` | ✓ OK | Requires entities to exist first |
| `delete_relationship` | ✓ OK | Hard delete works |
| `update_table` | ✓ OK | Description, usage_notes work |
| `delete_table_metadata` | ✓ OK | Clears custom metadata |
| `update_column` | ✓ OK | Description, semantic_type work |
| `delete_column_metadata` | ✓ OK | Clears custom metadata |
| `update_project_knowledge` | ✓ OK | Create with fact_id returned |
| `delete_project_knowledge` | ✓ OK | Delete by fact_id works |
| `create_glossary_term` | ✓ OK | Rejects test-like names (Issue 13) |
| `update_glossary_term` | ✓ OK | Aliases work correctly |
| `delete_glossary_term` | ✓ OK | Removes term and aliases |
| `create_approved_query` | ✓ OK | Requires output_column_descriptions (Issue 14) |
| `update_approved_query` | ✓ OK | Updates description, tags |
| `delete_approved_query` | ✓ OK | Soft delete, rejects pending suggestions |
| `suggest_approved_query` | ✓ OK | Creates pending suggestion |
| `suggest_query_update` | ✓ OK | Creates update suggestion |
| `approve_query_suggestion` | ✓ OK | Approves and enables query |
| `reject_query_suggestion` | ✓ OK | Rejects with reason |
| `approve_change` | ✓ OK | Applies single change |
| `reject_change` | ✓ OK | Rejects single change |
| `approve_all_changes` | ✓ FIXED | **Issue 13** - was nil pointer, fixed with ProcessChangesAsync |

**Not Tested (no data on test_data):**
| Tool | Reason |
|------|--------|
| `skip_ontology_question` | No ontology/questions on test_data |
| `dismiss_ontology_question` | No ontology/questions on test_data |
| `escalate_ontology_question` | No ontology/questions on test_data |
| `resolve_ontology_question` | No ontology/questions on test_data |

---

## Issue 16: Entity Enrichment Fails on Name Collision

**Severity:** High
**Status:** N/A - Entities Removed

**What Happened:** Entity enrichment failed when LLM tried to rename an entity to a name that already existed.

**Resolution:** Entities have been completely removed from the ontology model. There is no longer entity discovery, entity enrichment, or entity naming. This issue is no longer applicable.

---

## Summary (Updated 2026-02-05)

| Severity | Count | Status |
|----------|-------|--------|
| Critical | 2 | 2 FIXED (sample crash, approve_all_changes crash) |
| High | 1 | N/A (entity name collision - entities removed) |
| Medium | 4 | 3 N/A (entity-related), 1 OPEN (bulk question resolution) |
| Low | 10 | Mixed - see below |

**Status by Issue:**

| Issue | Status |
|-------|--------|
| 1. sample crash | ✅ FIXED |
| 2. Only 2 entities promoted | N/A (entities removed) |
| 3. table_count wrong | ✅ FIXED (entities removed) |
| 4. relationships null | N/A (entities removed) |
| 5. Duplicate glossary terms | Low priority (data issue) |
| 6. probe_relationship empty | N/A (tool removed) |
| 7. probe_column wrong purpose | Low (column classification) |
| 8. marker_at semantic role | Low (column classification) |
| 9. Project knowledge duplicates | Low (data issue) |
| 10. No bulk question resolution | **OPEN** (feature request) |
| 11. Question priority filter | Low (UX) |
| 12. deleted_at semantic type | May be fixed (timestamp work) |
| 13. approve_all_changes crash | ✅ FIXED |
| 14a. create_glossary_term rejects test names | Design choice |
| 14b. output_column_descriptions | Documented behavior |
| 15. update_relationship entities | N/A (tool removed) |
| 16. Entity name collision | N/A (entities removed) |

**Remaining Open Issues:**
- Issue 10: No bulk `resolve_ontology_questions` (feature request)
- Issues 5, 7-9, 11-12: Low priority metadata/classification issues

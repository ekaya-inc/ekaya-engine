# ISSUES: Ontology Enrichment MCP Tools - 2026-01-31

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
**Observed:** `get_context(depth='entities')` returns only 2 entities

**What Happened:**
```json
get_context(depth='entities')
→ entities: {"Account Authentication": {...}, "Invitation": {...}}
```

But `get_schema` shows 38+ entity descriptions, and `search_schema(query='user')` finds User, Account, User Follow, etc.

**Expected:**
- Core business entities (User, Account, Engagement, Channel, Media) should be promoted
- Entity promotion logic appears too conservative

**Impact:**
- AI agents using get_context miss important entities
- Must use search_schema to discover entities

---

## Issue 3: `table_count` Wrong in Domain Context

**Severity:** Low
**Observed:** `get_context(depth='domain')` shows `table_count: 2`

**What Happened:**
```json
get_context(depth='domain')
→ "table_count": 2, "column_count": 634
```

There are 38+ tables in the database, not 2. The 634 columns is correct.

**Expected:**
- table_count should reflect actual selected tables (~38)

---

## Issue 4: `relationships` Always Null in get_context

**Severity:** Medium
**Observed:** `include_relationships=true` returns `"relationships": null`

**What Happened:**
```
get_context(depth='entities', include_relationships=true)
→ "relationships": null
```

But `get_ontology(depth='domain')` DOES return relationships:
```json
"relationships": [
  {"from": "Account Authentication", "to": "Invitation", ...},
  {"from": "Invitation", "to": "Account Authentication", ...}
]
```

**Expected:**
- get_context with include_relationships=true should include relationships
- Inconsistent behavior between get_context and get_ontology

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
**Observed:** `probe_relationship(from_entity='User')` returns empty

**What Happened:**
```
probe_relationship(from_entity='User')
→ {"relationships": []}
```

User entity exists and has foreign keys to other tables.

**Expected:**
- Should return User's relationships to Account, Engagement, etc.

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
**Observed:** 2026-01-31 on test_data

**What Happened:**
```
update_relationship(from_entity="User", to_entity="Account", ...)
→ {"error": "from_entity \"User\" not found"}
```

Cannot create relationships between entities that don't exist yet. Must create entities first.

**Expected:**
- This is likely correct behavior, but should be documented
- Error message is clear

**Workaround:** Create entities first with `update_entity`, then create relationships.

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
**Observed:** 2026-01-31 on test_data

**What Happened:**
Entity enrichment fails when LLM tries to rename an entity to a name that already exists.

**Error:**
```
Failed to update entity with enrichment
  {"entity_id": "ab2635a1-62ee-4498-a448-9b1f050c702d",
   "error": "failed to update entity: ERROR: duplicate key value violates unique constraint
   \"engine_ontology_entities_ontology_id_name_key\" (SQLSTATE 23505)"}
```

**Sequence:**
1. Entity discovery creates `accounts` entity (id: ab2635a1, primary_table: accounts, source: inferred)
2. MCP tool previously created `Account` entity (id: da60b85d, no primary_table, source: mcp)
3. LLM enrichment tries to rename `accounts` → `Account`
4. Update fails because `Account` name already exists

**Data State Showing Conflict:**
```
| Name     | Primary Table | Source   |
|----------|---------------|----------|
| Account  | (none)        | mcp      |
| accounts | accounts      | inferred |
| User     | (none)        | mcp      |
| users    | users         | inferred |
```

**Root Cause:**
- `Create()` uses `ON CONFLICT (ontology_id, name) DO UPDATE` - handles duplicates
- `Update()` uses plain `UPDATE ... WHERE id = $1` - **no conflict handling for name changes**

**Key Files:**
- `pkg/repositories/ontology_entity_repository.go:485` - `Update()` method
- `pkg/services/entity_discovery_service.go:577-584` - sets name and calls Update

**Potential Fixes:**
1. **Check before rename:** In enrichment, check if target name exists, skip rename or merge
2. **Merge entities:** If names collide, merge the inferred entity into the MCP one (preserving primary_table)
3. **Prevent MCP duplicates:** MCP `update_entity` should check for existing inferred entities by table

**Recommended:** Option 2 (merge entities) - the MCP-created `Account` and inferred `accounts` represent the same business concept. Merging preserves user intent while maintaining schema linkage.

---

## Summary

| Severity | Count | Status |
|----------|-------|--------|
| Critical | 2 | 2 FIXED (sample crash, approve_all_changes crash) |
| High | 1 | Open (entity name collision) |
| Medium | 4 | Open |
| Low | 10 | Open (3 new from write tests) |

**Key Issues to Address:**
1. ~~Server crash on sample~~ **FIXED**
2. ~~Server crash on approve_all_changes~~ **FIXED** - Issue 13
3. **Entity enrichment fails on name collision** - Issue 16
4. Only 2 entities promoted (should be more)
5. relationships null in get_context
6. table_count wrong in domain context
7. No bulk question resolution
8. create_approved_query output_column_descriptions should be optional or documented as required

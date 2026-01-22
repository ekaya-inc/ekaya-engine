# BUGS: Ontology Extraction Issues - Round 2

**Date:** 2026-01-22
**Project:** Tikr test database (`2bb984fc-a677-45e9-94ba-9f65712ade70`)
**MCP:** `mcp__test_data__*`

This document captures issues found during re-testing after applying fixes from BUGS-ontology-extraction.md.

---

## Summary

| Bug | Original Issue | Status |
|-----|----------------|--------|
| BUG-1 | Sample tables as entities | **NEEDS RETEST** (tables were selected in import) |
| BUG-2 | Empty entity descriptions | **FIXED** |
| BUG-3 | Duplicate User entities | **FIXED** |
| BUG-4 | Spurious relationships | **FIXED** |
| BUG-5 | Missing domain knowledge | **REQUIRES CODE REMOVAL** (file-based design flaw) |
| BUG-6 | Missing enum values | **FIXED** |
| BUG-7 | Generic SaaS glossary | **FIXED** |
| BUG-8 | Test data in glossary | **FIXED** |
| BUG-9 | Missing FK relationships | **NOT FIXED** - root cause: missing `_id` exemption in NULL stats check |
| BUG-10 | Stale data on delete | **FIXED** (cascade works) |
| BUG-11 | Wrong project knowledge | **FIXED** (no stale data) |

---

## BUG-1: Sample Tables as Entities

**Severity:** Critical
**Status:** NEEDS RETEST

### Observation

23 sample/test tables (s1_* through s10_*) were extracted as entities.

### Clarification

These tables were **explicitly selected** when the schema was imported - they have `is_selected = true`. The fix filters by `is_selected`, so this is **working as designed**.

### Verification Needed

To properly test BUG-1, need to:
1. **Deselect sample tables** in the UI or via SQL
2. **Re-run ontology extraction**
3. **Verify** deselected tables are NOT extracted as entities

```sql
-- To test: Mark sample tables as unselected, then re-extract
UPDATE engine_schema_tables
SET is_selected = false
WHERE table_name ~ '^s\d+_'
AND project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';
```

---

## BUG-5: File-Based Knowledge Loading Must Be Removed

**Severity:** CRITICAL
**Status:** NOT FIXED - REQUIRES CODE REMOVAL

### Problem

The KnowledgeSeeding step and related code expect to load files from disk. This is a **fundamental design flaw** - ekaya-engine is a cloud service with no project-specific files on disk.

The design principle is:
1. **Infer** knowledge from database schema and data
2. **Refine** via MCP tools (agents with access to code/docs can update ontology)
3. **Clarify** via ontology questions that MCP tools can answer

**There should be ZERO file loading for project-specific data.**

### Code to DELETE

#### 1. Delete Entire File: `pkg/services/knowledge_discovery.go` (734 lines)
Scans Go/TypeScript/Markdown files from disk - completely inappropriate for a cloud service.

```bash
rm pkg/services/knowledge_discovery.go
rm pkg/services/knowledge_discovery_test.go  # if exists
```

#### 2. Remove from `pkg/services/knowledge.go`:
- `SeedKnowledgeFromFile` method (~60 lines)
- `loadKnowledgeSeedFile` function (~30 lines)
- `KnowledgeSeedFile` struct
- `KnowledgeSeedFact` struct
- `knowledgeSeedFactWithType` struct
- `AllFacts` method

#### 3. Remove from `pkg/services/dag/knowledge_seeding_node.go`:
- `KnowledgeSeedingMethods` interface (references `SeedKnowledgeFromFile`)
- Update `Execute` to be a no-op or remove the node entirely

#### 4. Remove from `pkg/services/dag_adapters.go`:
- `SeedKnowledgeFromFile` method on `KnowledgeSeedingAdapter`

#### 5. Remove from `pkg/models/project.go`:
- `ParseEnumFile` function (~20 lines)
- Keep `ParseEnumFileContent` if useful for parsing content stored in DB

#### 6. Remove from `pkg/services/column_enrichment.go`:
- Code that loads enums via `project.Parameters["enums_path"]` (~20 lines)

### Tests to DELETE/UPDATE

```bash
rm pkg/services/knowledge_discovery_test.go
# Update these to remove file-based test cases:
# - pkg/services/knowledge_test.go
# - pkg/services/knowledge_seeding_integration_test.go
# - pkg/services/dag/knowledge_seeding_node_test.go
```

### Verification After Removal

```bash
# Should find NO results:
grep -r "os.ReadFile\|ioutil.ReadFile" pkg/services/ --include="*.go" | grep -v "_test.go"
grep -r "knowledge_seed_path\|enums_path" pkg/ --include="*.go"
```

### Future: Auto-Inference (Separate Task)

After removing file-based code, knowledge should be **inferred from schema patterns** during OntologyFinalization:
- Detect `deleted_at` columns → soft delete convention
- Detect `*_amount` columns with large integers → currency in cents
- Detect UUID text columns → entity ID format

This is a **separate enhancement**, not part of this cleanup.

---

## BUG-9: Billing Engagement Has Zero Relationships

**Severity:** High
**Status:** NOT FIXED - Root cause identified

### Problem

The "Billing Engagement" entity has 0 relationships despite having obvious FK columns:
- `visitor_id` → users.user_id
- `host_id` → users.user_id
- `session_id` → sessions.session_id
- `offer_id` → offers.offer_id

### Evidence

```sql
-- FK columns have NULL distinct_count:
SELECT column_name, distinct_count, is_joinable FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.table_name = 'billing_engagements'
AND column_name IN ('visitor_id', 'host_id', 'session_id', 'offer_id');

-- Result:
-- visitor_id  | NULL | t
-- host_id     | NULL | t
-- offer_id    | NULL | t
-- session_id  | 69   | t   <-- only this one has stats
```

### Root Cause: Missing `_id` Exemption in NULL Stats Check

**File:** `pkg/services/deterministic_relationship_service.go:553-555`

```go
// BUG: No exemption for _id columns!
if col.DistinctCount == nil {
    continue // No stats = cannot evaluate = skip
}
```

The FIX-9 plan added `_id` exemptions to:
- ✅ `IsJoinable == nil` check (line 543-549)
- ✅ Cardinality ratio check (line 563-570)
- ❌ **`DistinctCount == nil` check (line 553-555)** - MISSED!

Columns `visitor_id`, `host_id`, `offer_id` have `DistinctCount = NULL` and are filtered out at line 553 **before** the cardinality ratio bypass can help.

### Fix Required

Add `_id` exemption to the NULL stats check:

```go
// Line 553-555: Add _id exemption
if col.DistinctCount == nil {
    if !isLikelyFKColumn(col.ColumnName) {
        continue // No stats and not likely FK = skip
    }
    // _id columns with nil DistinctCount included for join validation
}
```

### Why Stats Are NULL

The `distinct_count` is populated by stats collection. For these columns, stats collection either:
1. Wasn't run
2. Failed silently
3. Has a bug for certain column patterns

This is a secondary issue - the primary fix is to exempt `_id` columns from the NULL check.

---

## BUG-4: Spurious Column-to-Column Relationships

**Severity:** Medium
**Status:** FIXED (core issues)

### Original Issues - All Fixed

The original BUG-4 issues were:
- **4a**: `accounts.email → account_authentications.email` - **FIXED** (no email relationships)
- **4b**: `account_id → channel_id` mapping - **FIXED** (stricter type matching)
- **4c**: Reversed FK direction - **FIXED** (direction validation added)
- **4d**: Bogus FK targets - **FIXED** (PK-only targets)

### Verification

```sql
-- No email/password relationships exist
SELECT * FROM engine_entity_relationships r
JOIN engine_ontologies o ON r.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND (r.source_column_name LIKE '%email%' OR r.source_column_name LIKE '%password%');
-- Returns: 0 rows
```

### Potential Follow-up

Some relationships from LLM semantic inference may warrant review:
- `Account → Channel` via `default_user_id → channel_id`

These are **not** the original name-matching bug but may be LLM inference that could be improved. Lower priority.

---

## New Issues Discovered

### NEW-1: Domain Summary Polluted by Sample Data

**Severity:** Medium

The domain summary includes sample table domains, making it less useful:

```json
"primary_domains": ["activity_log", "analytics", "billing", "categorization",
  "communications", "content", "customer_service", "document_management",
  "ecommerce", "education", "finance", "geography", "hospitality", "hr",
  "infrastructure", "inventory", "legal", "marketing", ...]
```

Many of these (ecommerce, education, geography, hospitality, hr, inventory, legal) are from sample tables, not actual Tikr business.

### NEW-2: Entity Occurrence Counts Seem Low

Several real Tikr entities have `occurrence_count: 0`:
- Billing Engagement: 0
- Billing Transaction: 0
- Billing Activity Message: 0
- Engagement Payment Intent: 0
- Session: 0
- Offer: 0

This suggests these entities aren't being linked to relationships or other entities properly.

---

## Recommendations

1. **BUG-5**: Remove ALL file-based knowledge/enum loading code (see detailed removal plan above)
2. **BUG-1**: Re-run schema discovery to apply table exclusion patterns to sample tables
3. **BUG-9**: Investigate FK discovery for text UUID columns to fix Billing Engagement relationships
4. **Future**: Add auto-inference of conventions from schema patterns (soft delete, currency, UUIDs)

---

## Verification Commands

### Check Sample Tables Selected
```sql
SELECT table_name, is_selected FROM engine_schema_tables
WHERE table_name ~ '^s\d+_' ORDER BY table_name;
```

### Check Billing Engagement Columns
```sql
SELECT c.column_name, c.is_joinable, c.distinct_count
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.table_name = 'billing_engagements';
```

### Check Project Knowledge
```sql
SELECT * FROM engine_project_knowledge
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';
```

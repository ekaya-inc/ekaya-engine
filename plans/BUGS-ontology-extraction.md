# BUGS: Ontology Extraction Issues

**Date:** 2026-01-23
**Project:** Tikr test database (`2bb984fc-a677-45e9-94ba-9f65712ade70`)
**Context:** Issues discovered during ontology extraction testing and code review

This document tracks outstanding bugs in the ontology extraction workflow. Each bug has enough context for a researcher to investigate and create a detailed FIX plan.

---

## Summary

| Bug | Issue | Severity | Status |
|-----|-------|----------|--------|
| BUG-0 | MCP get_schema ignores selected_only for entities | High | Confirmed - blocks testing |
| BUG-8 | Ontology extraction includes deselected columns | Critical | Confirmed - PII/security risk |
| BUG-7 | Relationship discovery processes deselected tables | Medium | Confirmed - performance issue |
| BUG-1 | Deterministic column name pattern matching | Critical | Design flaw - LLMs should decide, not code |
| BUG-2 | File-based knowledge loading | Critical | Architectural flaw - cloud service shouldn't load files |
| BUG-3 | Missing FK relationships in Billing Engagement | High | Root cause: NULL stats filtering |
| BUG-4 | Sample tables extracted as entities | Medium | ✅ RESOLVED - entity discovery filters by is_selected |
| BUG-5 | Domain summary pollution from sample data | Medium | ✅ RESOLVED - domain summary now clean |
| BUG-6 | Low/zero entity occurrence counts | Medium | Investigation needed |
| BUG-9 | Stats collection fails for 27% of joinable columns | High | Root cause of BUG-3 and missing relationships |
| BUG-10 | Glossary terms generated without SQL definitions | Medium | 2/11 terms have empty defining_sql |
| BUG-11 | All relationships have cardinality='unknown' | Medium | 48/48 relationships missing cardinality |
| BUG-12 | Glossary SQL uses wrong enum values | Critical | 5/9 terms use 'ended' instead of 'TRANSACTION_STATE_ENDED' |
| BUG-13 | Glossary SQL semantic/structural issues | Medium | User Review Rating returns 2 rows; Average Fee formula incorrect |

---

## BUG-0: MCP get_schema Ignores selected_only for Entities

**Severity:** High
**Type:** MCP Tool Bug
**Status:** Confirmed - Blocks Testing

### Problem Statement

The MCP tool `get_schema` with `selected_only=true` correctly filters **tables** but does **not** filter the **entity list**. It returns entities from deselected tables (e.g., s1_*, s2_*, etc.) in the "DOMAIN ENTITIES:" section, even though those tables don't appear in the schema output below.

### Evidence

```sql
-- Database shows sample tables are deselected:
SELECT table_name, is_selected
FROM engine_schema_tables
WHERE table_name ~ '^s\d+_' AND project_id = '...'
-- Result: All 27 tables have is_selected = false
```

```javascript
// But MCP still returns their entities:
mcp__test_data__get_schema({ selected_only: true })
// Response includes:
//   - Activity (from s5_activities)
//   - Address (from s9_addresses)
//   - Category (from s4_categories)
//   ... etc
```

### Root Cause

**File:** `pkg/services/schema.go`

**Lines 1090-1103:**
```go
if selectedOnly {
    schema, err = s.GetSelectedDatasourceSchema(ctx, projectID, datasourceID)
    // ✅ This correctly filters tables by IsSelected (line 971)
} else {
    schema, err = s.GetDatasourceSchema(ctx, projectID, datasourceID)
}

// ❌ BUG: Gets ALL entities, ignoring selectedOnly
entities, err := s.entityRepo.GetByProject(ctx, projectID)
```

**Lines 1126-1133:**
```go
// Outputs ALL entities, even if their PrimaryTable is deselected
if len(entities) > 0 {
    sb.WriteString("DOMAIN ENTITIES:\n")
    for _, entity := range entities {
        sb.WriteString(fmt.Sprintf("  - %s: %s\n", entity.Name, entity.Description))
        sb.WriteString(fmt.Sprintf("    Primary location: %s.%s.%s\n",
            entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn))
    }
}
```

### The Fix

After retrieving entities (line 1100), filter them when `selectedOnly=true`:

```go
entities, err := s.entityRepo.GetByProject(ctx, projectID)
if err != nil {
    return "", fmt.Errorf("failed to get entities: %w", err)
}

// NEW: Filter entities to only include those from selected tables
if selectedOnly {
    selectedTableNames := make(map[string]bool)
    for _, table := range schema.Tables {
        selectedTableNames[table.TableName] = true
    }

    filteredEntities := make([]*models.OntologyEntity, 0)
    for _, entity := range entities {
        if selectedTableNames[entity.PrimaryTable] {
            filteredEntities = append(filteredEntities, entity)
        }
    }
    entities = filteredEntities
}
```

### Impact

**Blocks testing:** Cannot verify that deselecting sample tables excludes their entities from ontology extraction, because the MCP tool incorrectly reports them as present.

**User confusion:** MCP clients see entities that don't correspond to any tables in the schema output.

### Success Criteria

- `get_schema(selected_only=true)` returns only entities whose `PrimaryTable` is in the selected tables
- Deselecting sample tables results in their entities being excluded from the entity list
- Entity list matches the tables shown in schema output

---

## BUG-8: Ontology Extraction Includes Deselected Columns

**Severity:** Critical
**Type:** Security/Privacy Issue
**Status:** Confirmed

### Problem Statement

Ontology extraction processes **all columns** (including deselected ones) even though admins explicitly deselect columns to exclude PII, legacy data, or irrelevant fields. Deselected columns appear in the ontology with full semantic enrichment (descriptions, synonyms, enum values).

This is a **critical security issue** because:
1. Admins deselect columns containing PII (emails, passwords, SSNs, etc.)
2. The ontology extraction still processes and stores metadata about these columns
3. LLM prompts may include deselected column data during enrichment
4. MCP clients can query information about columns that should be hidden

### Evidence

**Database shows column is deselected:**
```sql
SELECT c.column_name, c.is_selected
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.table_name = 'accounts' AND c.column_name = 'email';

-- Result: is_selected = false
```

**But MCP returns the column in the ontology:**
```javascript
mcp__test_data__get_context({ depth: 'columns', tables: ['accounts'] })
// Response includes:
{
  "name": "email",
  "description": "Primary email address of the account",
  "semantic_type": "email",
  "role": "attribute"
}
```

### Root Cause Investigation Needed

Need to check where columns are loaded during ontology extraction. Likely locations:

1. **Column enrichment** - `pkg/services/column_enrichment.go`
   - Does it filter by `is_selected`?

2. **Schema context building** - `pkg/services/schema.go`
   - Functions like `GetDatasourceSchema` vs `GetSelectedDatasourceSchema`
   - Which version is used during extraction?

3. **DAG nodes** - `pkg/services/dag/column_enrichment_node.go`
   - Does the DAG node request selected columns only?

### Comparison to Tables

**Tables work correctly:**
- Entity discovery uses `ListTablesByDatasource(ctx, projectID, datasourceID, true)` ✅
- Only selected tables are extracted as entities

**Columns are broken:**
- Column enrichment likely uses all columns regardless of `is_selected` ❌
- Need to verify and fix

### Impact

**Security:** PII columns (email, SSN, phone) are processed and stored in ontology despite deselection

**Privacy:** User expectations violated - deselecting a column should completely exclude it

**Compliance:** May violate data handling policies (GDPR, HIPAA, etc.) if sensitive columns are processed

**Trust:** Admins lose confidence in the selection mechanism if it doesn't work

### The Fix

Similar to entity discovery, column enrichment must filter by `is_selected`:

1. Find where columns are loaded for enrichment
2. Add `selectedOnly=true` parameter (or equivalent filtering)
3. Verify DAG nodes request selected columns only
4. Update schema context functions to respect selection

### Success Criteria

- Deselected columns do NOT appear in the ontology
- `get_context(depth='columns')` only returns selected columns
- Column enrichment only processes selected columns
- MCP clients cannot query deselected column metadata
- LLM prompts do not include deselected column information

---

## BUG-7: Relationship Discovery Processes Deselected Tables

**Severity:** Medium
**Type:** Performance/Efficiency Issue
**Status:** Confirmed

### Problem Statement

Relationship discovery processes **all tables** (including deselected ones) even though entities are only created from selected tables. This wastes processing time and may cause errors when trying to create relationships to non-existent entities.

### Evidence

While entity discovery correctly filters to selected tables:

```go
// ✅ pkg/services/entity_discovery_service.go:97
// Get selected tables for this datasource (respects is_selected flag)
tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, true)
```

Relationship discovery processes **all** tables:

```go
// ❌ pkg/services/relationship_discovery.go:138
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
```

### Affected Code Locations

All these services load tables with `selectedOnly=false`:

1. **`pkg/services/relationship_discovery.go:138`**
   - FK discovery via join validation
   - Processes all tables, finds joinable columns to non-existent entities

2. **`pkg/services/deterministic_relationship_service.go`**
   - Line 146: `DiscoverRelationships` - FK discovery
   - Line 448: `DiscoverPKMatches` - PK matching
   - Analyzes deselected tables, can't create relationships (entities don't exist)

3. **`pkg/services/data_change_detection.go`**
   - Line 88: `DetectEnumChanges` - Enum value changes
   - Line 134: `DetectSchemaChanges` - New FK patterns
   - Scans deselected tables for changes that won't be used

### Impact

**Performance:** Wastes time analyzing deselected tables (especially problematic for databases with many sample/test tables)

**Error logs:** May log warnings/errors when trying to create relationships where target entities don't exist

**Resource usage:** Consumes LLM tokens and database queries for tables that won't be in the ontology

### Example Scenario

1. Admin deselects 27 sample tables (s1_* through s10_*)
2. Entity discovery creates 0 entities from those tables ✅
3. Relationship discovery analyzes all 27 sample tables ❌
4. Tries to find relationships like `s1_orders.customer_id → s1_customers.id`
5. Can't create relationship (s1_customers entity doesn't exist)
6. Logs errors and wastes processing time

### The Fix

Change all relationship discovery services to use `selectedOnly=true`:

```go
// pkg/services/relationship_discovery.go:138
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)

// pkg/services/deterministic_relationship_service.go:146, 448
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)

// pkg/services/data_change_detection.go:88, 134
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
```

### Why This Doesn't Break Things

**Database constraints:** Relationships require both entities to exist:
- `source_entity_id uuid NOT NULL` → FK to `engine_ontology_entities`
- `target_entity_id uuid NOT NULL` → FK to `engine_ontology_entities`

**Result:** Even with the current bug, relationships to deselected tables **cannot be created** because the entities don't exist. The bug only causes wasted processing.

### Research Questions for FIX Plan

1. Are there any legitimate use cases for analyzing deselected tables during relationship discovery?
   - Could a relationship exist between a selected table and a deselected table?
   - Answer: No, because the deselected table won't have an entity

2. Should we add validation to fail fast when trying to create relationships to non-existent entities?
   - Or is the database FK constraint sufficient?

3. What about data_change_detection - should it monitor deselected tables?
   - Current behavior: Yes (monitors enum changes, FK pattern changes)
   - Proposed: No (changes in deselected tables are irrelevant)

### Success Criteria

- Relationship discovery only processes selected tables
- Processing time proportional to selected tables, not total tables
- No errors/warnings about missing entities from deselected tables
- Tests verify that deselecting tables reduces relationship discovery workload

---

## BUG-1: Deterministic Column Name Pattern Matching

**Severity:** Critical
**Type:** Design Flaw

### Problem Statement

The deterministic code makes hardcoded decisions based on column name patterns (suffixes like `_id`, `_uuid`, `_at`, prefixes like `is_`, `has_`, etc.). This violates the design principle that **LLMs should make semantic judgments from context and data, not deterministic code.**

### Design Philosophy Violation

The system should work as:
- **Deterministic code:** Extract statistics, identify constraints, validate joins
- **LLM code:** Make semantic judgments about what columns represent based on data and context

Currently, deterministic code is making semantic judgments like "this is a foreign key because it ends in `_id`".

### Why This Is Wrong

1. **Not all FKs end in `_id`** - Schemas use `UserRef`, `account_key`, `parent`, etc.
2. **Not all `_id` columns are FKs** - `transaction_id` might be a business ID string, not a FK
3. **Context matters** - `visitor_id` vs `host_id` need semantic understanding of roles
4. **Schema conventions vary** - Different projects have different naming patterns
5. **Blocks valid relationships** - Columns without conventional names get filtered out before LLMs can evaluate them

### Affected Code Locations

#### 1. `pkg/services/deterministic_relationship_service.go`

**`isLikelyFKColumn(columnName string)` (line 746)**
```go
// Checks: _id, _uuid, _key suffixes
// Used in: Lines 545, 568 (exemptions from various filters)
```

**`isPKMatchExcludedName(columnName string)` (line 812)**
```go
// Checks: _at, _date, is_, has_, _status, _type, _flag,
//         num_, total_, _count, _amount, _total, _sum, _avg, _min, _max,
//         rating, score, level
// Used in: Lines 510, 537 (exclusion from PK match candidates)
```

#### 2. `pkg/services/column_filter.go`

**`isExcludedName(columnName string)` (line 148)**
```go
// Checks: _at, _date, is_, has_, _status, _type, _flag
// Used in: Line 78 (filtering entity candidates)
```

**`isEntityReferenceName(columnName string)` (line 174)**
```go
// Checks: id (exact match), _id, _uuid, _key suffixes
// Used in: Lines 99, 497 (identifying entity references)
```

#### 3. `pkg/services/relationship_discovery.go`

**`shouldCreateCandidate(sourceColumnName, targetTableName string)` (line 561)**
```go
// Checks: _id suffix + plural table matching logic
// Pattern list: attributeColumnPatterns = ["email", "password", "name", "description", "status", "type"]
// Used in: Line 287 (FK candidate validation)
```

Also performs pluralization logic:
```go
// user_id → users, user
// category_id → categories, category (drops y, adds ies)
```

#### 4. `pkg/services/data_change_detection.go`

**Line 346:**
```go
if !strings.HasSuffix(col.ColumnName, "_id") {
    continue // Skip columns not ending in _id when suggesting FK patterns
}
```

### Connection to Other Bugs

- **BUG-3** (Missing FK relationships): The proposed "fix" adds another `_id` exemption, which perpetuates this design flaw
- **BUG-9** in previous round was caused by this pattern matching filtering out valid FK columns

### Research Questions for FIX Plan

1. What data/statistics should deterministic code extract and pass to LLMs?
2. How should LLMs make semantic decisions about:
   - Which columns are likely FKs?
   - Which columns are timestamps/booleans/metrics?
   - Which columns represent entity references?
3. Should we keep ANY pattern matching or remove all of it?
4. How does this affect performance (more LLM calls vs fewer filtered candidates)?
5. What's the migration path - can we do this incrementally or needs full refactor?

### Success Criteria

- Deterministic code extracts facts (cardinality, types, constraints, join validation results)
- LLMs receive rich context and make semantic decisions
- Unconventionally-named columns (e.g., `parent`, `UserRef`) are properly identified as FKs
- No hardcoded name pattern assumptions in deterministic code

---

## BUG-2: File-Based Knowledge Loading

**Severity:** Critical
**Type:** Architectural Flaw

### Problem Statement

The KnowledgeSeeding step and related code expect to load project-specific knowledge from files on disk. This is a fundamental architectural flaw - ekaya-engine is a **cloud service** with no project-specific files on disk.

### Design Principle

The system should work as:
1. **Infer** knowledge from database schema and data analysis
2. **Refine** via MCP tools (agents with access to code/docs can update ontology)
3. **Clarify** via ontology questions that MCP tools can answer

**There should be ZERO file loading for project-specific data.**

### Code to Delete

This bug has a detailed removal plan. The following files/code must be deleted:

#### 1. Delete Entire File
- `pkg/services/knowledge_discovery.go` (734 lines) - Scans Go/TypeScript/Markdown files from disk

#### 2. Remove from `pkg/services/knowledge.go`
- `SeedKnowledgeFromFile` method (~60 lines)
- `loadKnowledgeSeedFile` function (~30 lines)
- `KnowledgeSeedFile` struct
- `KnowledgeSeedFact` struct
- `knowledgeSeedFactWithType` struct
- `AllFacts` method

#### 3. Remove from `pkg/services/dag/knowledge_seeding_node.go`
- `KnowledgeSeedingMethods` interface (references `SeedKnowledgeFromFile`)
- Update `Execute` to be a no-op or remove the node entirely from DAG

#### 4. Remove from `pkg/services/dag_adapters.go`
- `SeedKnowledgeFromFile` method on `KnowledgeSeedingAdapter`

#### 5. Remove from `pkg/models/project.go`
- `ParseEnumFile` function (~20 lines)
- Keep `ParseEnumFileContent` if useful for parsing content stored in DB

#### 6. Remove from `pkg/services/column_enrichment.go`
- Code that loads enums via `project.Parameters["enums_path"]` (~20 lines)

#### 7. Tests to Delete/Update
- `pkg/services/knowledge_discovery_test.go` (delete entire file)
- `pkg/services/knowledge_test.go` (remove file-based test cases)
- `pkg/services/knowledge_seeding_integration_test.go` (remove file-based test cases)
- `pkg/services/dag/knowledge_seeding_node_test.go` (update to remove file loading tests)

### Verification After Removal

```bash
# Should find NO results after cleanup:
grep -r "os.ReadFile\|ioutil.ReadFile" pkg/services/ --include="*.go" | grep -v "_test.go"
grep -r "knowledge_seed_path\|enums_path" pkg/ --include="*.go"
```

### Research Questions for FIX Plan

1. How should the KnowledgeSeeding DAG node work after file loading is removed?
   - Remove the node entirely?
   - Make it a no-op placeholder?
   - Repurpose it for schema pattern inference?

2. Should we add auto-inference of conventions during OntologyFinalization?
   - Detect `deleted_at` columns → soft delete convention
   - Detect `*_amount` columns with large integers → currency in cents
   - Detect UUID text columns → entity ID format

3. How do we handle existing projects that might have references to file paths in `project.Parameters`?

### Success Criteria

- No file I/O for project-specific data
- DAG completes without file loading step
- Knowledge comes only from schema inference or MCP tool updates
- Tests pass without file fixtures

---

## BUG-3: Missing FK Relationships in Billing Engagement

**Severity:** High
**Type:** Data Quality Issue

### Problem Statement

The "Billing Engagement" entity has 0 relationships despite having obvious FK columns:
- `visitor_id` → users.user_id
- `host_id` → users.user_id
- `session_id` → sessions.session_id
- `offer_id` → offers.offer_id

### Evidence

```sql
-- FK columns have NULL distinct_count:
SELECT column_name, distinct_count, is_joinable
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.table_name = 'billing_engagements'
AND column_name IN ('visitor_id', 'host_id', 'session_id', 'offer_id');

-- Result:
-- visitor_id  | NULL | t
-- host_id     | NULL | t
-- offer_id    | NULL | t
-- session_id  | 69   | t   <-- only this one has stats
```

### Root Cause

**File:** `pkg/services/deterministic_relationship_service.go:557-559`

```go
// BUG: No exemption for _id columns!
if col.DistinctCount == nil {
    continue // No stats = cannot evaluate = skip
}
```

Columns `visitor_id`, `host_id`, `offer_id` have `DistinctCount = NULL` and are filtered out **before** any other logic can evaluate them.

### Connection to BUG-1

The code already has `_id` exemptions in two other places:
1. Line 544-548: If `IsJoinable` is NULL but column ends in `_id`, include it anyway
2. Line 568-572: If cardinality ratio is low but column ends in `_id`, skip ratio check

The previous "fix" was to add another `_id` exemption to the NULL stats check. However, **this perpetuates BUG-1** - we're adding more pattern matching instead of removing it.

### Why Stats Are NULL

The `distinct_count` is populated by stats collection. For these columns, stats either:
1. Weren't collected
2. Failed silently during collection
3. Have a bug for certain column patterns (e.g., text UUID columns)

This is a secondary issue worth investigating.

### Research Questions for FIX Plan

1. **Primary:** How do we fix this WITHOUT adding more `_id` pattern matching?
   - Can we pass columns with NULL stats to LLM for semantic evaluation?
   - Should we always attempt join validation regardless of stats?

2. **Secondary:** Why are stats NULL for these specific columns?
   - Check stats collection code in `pkg/services/schema.go`
   - Are text UUID columns handled differently?
   - Should we improve stats collection reliability?

3. **Design:** How does this interact with BUG-1 refactoring?
   - Should we fix stats collection first, then remove pattern matching?
   - Or remove pattern matching first and rely on LLM judgment for NULL-stats columns?

### Success Criteria

- Billing Engagement entity shows 4 relationships (visitor, host, session, offer)
- Fix works for ANY column name pattern, not just `_id` suffix
- Stats collection is reliable or NULL stats don't block relationship discovery

---

## BUG-4: Sample Tables Extracted as Entities

**Severity:** Medium
**Type:** Retest Required
**Status:** ✅ RESOLVED (2026-01-23)

### Resolution

**Verified fixed via testing.** Entity discovery correctly filters by `is_selected`:
- `pkg/services/entity_discovery_service.go:97` uses `selectedOnly=true`
- After deselecting sample tables and re-extracting ontology: **0 sample entities created**

```sql
-- Verification query returned 0 rows:
SELECT e.name, e.primary_table FROM engine_ontology_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true AND e.primary_table ~ '^s[0-9]+_';
-- Result: 0 rows
```

### Original Problem

Sample/test tables (s1_* through s10_*) were extracted as entities because they were `is_selected = true` when schema was imported. This was **working as designed** - the fix was to deselect the tables before extraction.

### No Code Changes Required

Entity discovery already correctly filters by `is_selected`. The issue was data state, not code.

---

## BUG-5: Domain Summary Pollution from Sample Data

**Severity:** Medium
**Type:** Investigation Required
**Status:** ✅ RESOLVED (2026-01-23)

### Resolution

**Verified fixed via testing.** After re-extracting ontology with sample tables deselected, domain summary is clean:

```sql
SELECT domain_summary->'domains' FROM engine_ontologies
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70' AND is_active = true;

-- Result (all relevant to Tikr):
["analytics", "billing", "customer", "engagement", "feedback",
 "infrastructure", "marketing", "media", "moderation", "notifications",
 "security", "social", "user"]
```

**Polluting domains removed:** ecommerce, education, geography, hospitality, hr, inventory, legal - all gone.

### Original Problem

Domain summary included domains from sample tables (ecommerce, education, geography, etc.) making it less useful.

### Root Cause

Domain inference processes entities from selected tables. When sample tables were selected, their domains polluted the summary. Deselecting sample tables (BUG-4 fix) automatically resolved this.

### No Code Changes Required

Domain inference already correctly uses only selected tables/entities.

---

## BUG-6: Low/Zero Entity Occurrence Counts

**Severity:** Medium
**Type:** Investigation Required

### Problem Statement

Several real Tikr entities have `occurrence_count: 0`:
- Billing Engagement: 0
- Billing Transaction: 0
- Billing Activity Message: 0
- Engagement Payment Intent: 0
- Session: 0
- Offer: 0

This suggests these entities aren't being linked to relationships or other entities properly.

### What is `occurrence_count`?

Based on the schema, `occurrence_count` in `engine_ontology_entities` likely tracks:
- How many times this entity appears across the schema (as table, in relationships, in column roles)
- Or how many occurrences are in `engine_ontology_entity_occurrences` table

### Research Questions for FIX Plan

1. What does `occurrence_count` represent exactly?
   - Check schema definition in migrations
   - Check where it's calculated/updated

2. Is it calculated correctly?
   - Check `pkg/services/entity_discovery*.go`
   - Check `pkg/services/ontology_finalization.go`

3. Why do some entities have 0 count?
   - Are these entities without relationships?
   - Are occurrences not being recorded?
   - Is there a bug in the counting logic?

4. Connection to BUG-3?
   - If Billing Engagement has 0 relationships (BUG-3), does that cause 0 occurrences?
   - Check if fixing BUG-3 also fixes this

### Verification Commands

```sql
-- Check occurrence counts
SELECT e.name, e.occurrence_count, COUNT(r.*) as relationship_count
FROM engine_ontology_entities e
LEFT JOIN engine_entity_relationships r ON (r.from_entity_id = e.id OR r.to_entity_id = e.id)
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
GROUP BY e.id, e.name, e.occurrence_count
HAVING e.occurrence_count = 0
ORDER BY e.name;

-- Check entity occurrences table
SELECT entity_id, COUNT(*) as occurrences
FROM engine_ontology_entity_occurrences
WHERE ontology_id = (
  SELECT id FROM engine_ontologies
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
  AND is_active = true
)
GROUP BY entity_id;
```

### Success Criteria

- All real entities have accurate occurrence counts
- Occurrence count reflects actual usage across schema
- Entities with relationships have count > 0

---

## BUG-9: Stats Collection Fails for 27% of Joinable Columns

**Severity:** High
**Type:** Data Collection Bug
**Status:** Confirmed - Root cause of missing relationships

### Problem Statement

Stats collection (`distinct_count`) fails for a significant portion of columns marked as `is_joinable = true`. This prevents relationship discovery from evaluating these columns, causing many entities to have zero relationships.

**Scale of Impact:**
- 134 out of 501 joinable columns (27%) have `distinct_count = NULL`
- 21 out of 31 entities (68%) have zero relationships
- Core business entities like Billing Engagement, Session, Participant are affected

### Evidence

```sql
-- Columns marked joinable but with NULL stats
SELECT t.table_name, c.column_name, c.distinct_count, c.is_joinable
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND c.distinct_count IS NULL AND c.is_joinable = true
ORDER BY t.table_name LIMIT 10;

-- Results include:
-- billing_engagements | host_id     | NULL | t
-- billing_engagements | visitor_id  | NULL | t
-- billing_engagements | offer_id    | NULL | t
-- billing_transactions | payer_user_id | NULL | t
-- etc.
```

```sql
-- But the actual data EXISTS:
SELECT COUNT(DISTINCT visitor_id), COUNT(DISTINCT host_id)
FROM billing_engagements;
-- Result: 7 distinct visitors, 4 distinct hosts
```

### Root Cause Investigation

The stats collection happens in schema services. Need to investigate:

1. **Where stats are collected:**
   - `pkg/services/schema.go` - `CollectColumnStats` or similar
   - Check if UUID text columns are handled differently

2. **Why some columns fail:**
   - Are there specific data types that fail?
   - Are there errors during collection that are silently ignored?
   - Is there a timeout or limit that causes some columns to be skipped?

3. **Why `is_joinable` is set even with NULL stats:**
   - `is_joinable` might be inferred from column names (the _id suffix pattern)
   - Stats collection runs separately and fails for some columns

### Affected Entities

Entities with zero relationships due to NULL stats on their FK columns:

| Entity | Missing FK Columns | Actual Data |
|--------|-------------------|-------------|
| Billing Engagement | visitor_id, host_id, offer_id | 100 rows, 7 visitors, 4 hosts |
| Billing Transaction | payer_user_id, payee_user_id, offer_id | Has data |
| Session | host_id | Empty table (expected 0 relationships) |
| Participant | user_id, session_id | Needs verification |
| Media | user_id | Needs verification |
| Event | user_id | Needs verification |

### Connection to Other Bugs

- **BUG-3** (Missing FKs in Billing Engagement): This is the specific case; BUG-9 is the systemic cause
- **BUG-6** (Zero occurrence counts): Occurrences are derived from relationships; fixing BUG-9 may resolve this
- **BUG-1** (Pattern matching): The workaround in BUG-3's proposed fix (adding `_id` exemption) is a symptom of trying to work around this bug

### Research Questions for FIX Plan

1. **Stats collection code location:**
   - Where is `distinct_count` calculated?
   - What error handling exists?

2. **Pattern analysis:**
   - Do failed columns share characteristics (data type, table size, column name pattern)?
   - Are UUID text columns vs actual UUID types handled differently?

3. **Timing analysis:**
   - Does stats collection have timeouts?
   - Is it running in parallel with potential race conditions?

4. **Error logging:**
   - Are stats collection failures logged?
   - Can we add better error handling/retry logic?

5. **Alternative approach:**
   - Should relationship discovery work WITHOUT stats?
   - Can we do join validation directly instead of relying on stats?

### Verification Commands

```sql
-- Count impact
SELECT
  COUNT(*) FILTER (WHERE c.distinct_count IS NULL) as null_stats,
  COUNT(*) as total,
  ROUND(100.0 * COUNT(*) FILTER (WHERE c.distinct_count IS NULL) / COUNT(*), 1) as pct_null
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND c.is_joinable = true;

-- Find patterns in affected columns
SELECT c.data_type, COUNT(*) as affected_count
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND c.distinct_count IS NULL AND c.is_joinable = true
GROUP BY c.data_type
ORDER BY affected_count DESC;
```

### Success Criteria

- Stats collection populates `distinct_count` for all columns with data
- Zero joinable columns have NULL `distinct_count` (unless table is empty)
- All FK columns are evaluated for relationships regardless of stats availability
- Entities with valid FK columns have corresponding relationships discovered

---

## BUG-10: Glossary Terms Generated Without SQL Definitions

**Severity:** Medium
**Type:** Data Quality Issue
**Status:** Confirmed

### Problem Statement

Some glossary terms are generated with definitions but empty `defining_sql`. Terms without SQL are not useful for query generation - they can't be used by MCP clients to calculate metrics.

### Evidence

```sql
SELECT term, LENGTH(definition) as def_len, LENGTH(defining_sql) as sql_len
FROM engine_business_glossary
WHERE ontology_id IN (
  SELECT id FROM engine_ontologies
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70' AND is_active = true
);

-- Results:
-- Offer Utilization Rate       | 291 |   0  <-- No SQL!
-- Referral Bonus Participation | 239 |   0  <-- No SQL!
-- (9 other terms have SQL)
```

### Affected Terms

| Term | Has Definition | Has SQL |
|------|---------------|---------|
| Offer Utilization Rate | Yes (291 chars) | **No** |
| Referral Bonus Participation | Yes (239 chars) | **No** |

### Why This Matters

Glossary terms serve two purposes:
1. **Documentation** - Explain what a metric means (definition)
2. **Query generation** - Provide SQL for calculating the metric (defining_sql)

Without SQL, terms can only serve as documentation. MCP tools like `get_glossary_sql` will fail for these terms.

### Root Cause Investigation

Need to check glossary generation in:
1. `pkg/services/glossary_generation.go` or similar
2. LLM prompts for glossary generation
3. DAG node that generates glossary terms

Possible causes:
- LLM failed to generate valid SQL for complex metrics
- Validation rejected the SQL but kept the definition
- These terms represent calculated metrics that span multiple tables (harder to express in SQL)

### Research Questions for FIX Plan

1. Where is glossary generated and SQL validated?
2. Why are terms stored without SQL - is this intentional?
3. Should terms without SQL be:
   - Rejected entirely?
   - Marked with a flag (e.g., `needs_review`)?
   - Regenerated with different prompts?
4. Are these terms conceptually harder to express in SQL (e.g., "utilization rate" requires complex joins)?

### Verification Commands

```sql
-- Find terms without SQL
SELECT term, definition
FROM engine_business_glossary
WHERE ontology_id IN (
  SELECT id FROM engine_ontologies
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70' AND is_active = true
)
AND (defining_sql IS NULL OR defining_sql = '');

-- Check term quality
SELECT
  term,
  LENGTH(definition) as def_chars,
  LENGTH(defining_sql) as sql_chars,
  base_table
FROM engine_business_glossary
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
ORDER BY sql_chars ASC;
```

### Success Criteria

- All glossary terms have valid `defining_sql`
- Terms that can't have SQL are either excluded or marked appropriately
- `get_glossary_sql` works for all returned terms
- MCP clients can calculate any documented metric

---

## BUG-11: All Relationships Have Unknown Cardinality

**Severity:** Medium
**Type:** Data Quality Issue
**Status:** Confirmed

### Problem Statement

All discovered relationships have `cardinality = 'unknown'` instead of proper cardinality values (1:1, 1:N, N:1, N:M). Cardinality information is essential for:
- Query optimization hints
- Understanding data relationships
- Generating correct JOIN patterns
- Validating data integrity

### Evidence

```sql
SELECT cardinality, COUNT(*) as count
FROM engine_entity_relationships r
JOIN engine_ontologies o ON r.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true
GROUP BY cardinality;

-- Result:
-- cardinality | count
-- unknown     |    48
```

### Expected Cardinality Values

Based on the schema, relationships should have determined cardinality:

| From Entity | To Entity | Expected Cardinality |
|-------------|-----------|---------------------|
| Account | User | 1:N (one account has many users) |
| User | Account | N:1 (many users belong to one account) |
| Channel | User | N:1 (many channels owned by one user) |
| Follow | User (followee) | N:1 (many follows point to one user) |
| Follow | User (follower) | N:1 (many follows from one user) |

### Why Cardinality Matters

1. **Query Generation**: JOIN direction depends on cardinality
2. **Performance Hints**: N:1 suggests index on FK column
3. **Data Validation**: Cardinality violations indicate data issues
4. **Schema Understanding**: Helps developers understand the domain model

### Root Cause Investigation

Cardinality calculation requires:
1. Stats on the source column (distinct count)
2. Stats on the target column (distinct count)
3. Join validation to determine relationship type

Possible causes:
1. **BUG-9 connection**: NULL stats on FK columns prevent cardinality calculation
2. Cardinality calculation code is missing or disabled
3. Cardinality is calculated but not saved to database

Need to check:
- `pkg/services/relationship_discovery.go` - does it calculate cardinality?
- `pkg/services/deterministic_relationship_service.go` - cardinality logic
- Database migration - is cardinality column populated correctly?

### Connection to Other Bugs

- **BUG-9** (Stats collection): NULL stats may prevent cardinality calculation
- Fixing BUG-9 might automatically fix this

### Verification Commands

```sql
-- Check cardinality distribution
SELECT cardinality, COUNT(*) as count
FROM engine_entity_relationships
WHERE ontology_id = (
  SELECT id FROM engine_ontologies
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70' AND is_active = true
)
GROUP BY cardinality;

-- Check specific relationships
SELECT e1.name as from_entity, e2.name as to_entity, r.cardinality
FROM engine_entity_relationships r
JOIN engine_ontology_entities e1 ON r.source_entity_id = e1.id
JOIN engine_ontology_entities e2 ON r.target_entity_id = e2.id
WHERE r.ontology_id = (
  SELECT id FROM engine_ontologies
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70' AND is_active = true
)
LIMIT 10;
```

### Success Criteria

- All relationships have calculated cardinality (1:1, 1:N, N:1, N:M)
- Cardinality matches expected values based on schema constraints
- No relationships have `cardinality = 'unknown'` unless truly ambiguous

---

## BUG-12: Glossary SQL Uses Wrong Enum Values

**Severity:** Critical
**Type:** Data Quality / Functional Bug
**Status:** Confirmed

### Problem Statement

5 out of 9 glossary terms with SQL (55%) use incorrect enum values in their WHERE clauses. The SQL checks for `transaction_state = 'ended'` but the actual database values are `'TRANSACTION_STATE_ENDED'`. This causes all affected queries to **return zero results**.

### Evidence

```sql
-- Actual enum values in database:
SELECT transaction_state, COUNT(*) FROM billing_transactions GROUP BY transaction_state;
-- TRANSACTION_STATE_ENDED   | 70
-- TRANSACTION_STATE_WAITING | 27
-- TRANSACTION_STATE_ERROR   |  3

-- But glossary SQL uses:
WHERE transaction_state = 'ended'  -- Returns 0 rows!
```

### Affected Terms

| Term | SQL Filter | Result |
|------|-----------|--------|
| Payout Amount | `transaction_state = 'ended'` | **0 rows** (should be 70) |
| Engagement Revenue | `transaction_state = 'ended'` | **0 rows** (should be 70) |
| Average Fee Per Engagement | `transaction_state = 'ended'` | **NULL** (no matching rows) |
| Session Duration | `transaction_state = 'ended'` | **0** (should sum 70 rows) |
| Transaction Volume | `transaction_state = 'ended'` | **0** (should be 70) |

### Root Cause

The LLM generating glossary SQL either:
1. Wasn't provided with actual enum values from the database
2. Inferred "ended" from the column name `transaction_state` without checking data
3. Normalized the enum value without knowing the actual format

### Research Questions for FIX Plan

1. Where is glossary SQL generated? Check LLM prompts for enum handling
2. Does the system sample actual enum values and provide them to the LLM?
3. Should there be SQL validation that runs the query against sample data?
4. How should enum values be normalized (store mapping, or fix at generation)?

### Verification Commands

```sql
-- Test a glossary query with wrong enum
SELECT COUNT(*) AS transaction_volume
FROM billing_transactions
WHERE transaction_state = 'ended';  -- Returns 0

-- Same query with correct enum
SELECT COUNT(*) AS transaction_volume
FROM billing_transactions
WHERE transaction_state = 'TRANSACTION_STATE_ENDED';  -- Returns 70
```

### Success Criteria

- All glossary SQL uses correct enum values from actual database data
- Glossary queries return non-zero results when data exists
- Enum values are validated during glossary generation
- SQL is tested against sample data before being stored

---

## BUG-13: Glossary SQL Semantic and Structural Issues

**Severity:** Medium
**Type:** Data Quality Issue
**Status:** Confirmed

### Problem Statement

Beyond the enum value bug (BUG-12), several glossary terms have semantic or structural issues in their SQL that make them incorrect or confusing.

### Issue 1: User Review Rating Returns Multiple Rows

**Term:** User Review Rating

**Problem:** The SQL uses `UNION ALL` which returns **two separate rows** instead of a single combined average.

```sql
-- Current SQL returns TWO rows:
SELECT AVG(ur.reviewee_rating) AS average_user_review_rating
FROM user_reviews ur WHERE ur.deleted_at IS NULL
UNION ALL
SELECT AVG(cr.rating) AS average_channel_review_rating
FROM channel_reviews cr WHERE cr.deleted_at IS NULL;

-- Result:
-- average_user_review_rating
-- 5.0000  (from user_reviews)
-- NULL    (from channel_reviews - no data)
```

**Expected:** A single combined average, or separate terms for each type.

**Fix Options:**
1. Split into two separate glossary terms (User Review Rating, Channel Review Rating)
2. Use weighted average combining both tables
3. Use subquery to calculate single average

### Issue 2: Average Fee Per Engagement Formula

**Term:** Average Fee Per Engagement

**Problem:** The definition says "mean fee charged per engagement" but the formula calculates "platform fees as a percentage of total amount".

```sql
-- Current formula:
SUM(platform_fees) / NULLIF(SUM(total_amount), 0) * 100
-- This is: (total fees / total revenue) * 100 = fee percentage

-- Expected for "average fee per engagement":
SUM(platform_fees) / COUNT(*)
-- This is: total fees / number of engagements = average fee
```

**Semantic Mismatch:** The SQL doesn't match what the term name implies.

### Issue 3: Preauthorization Utilization Ratio

**Term:** Preauthorization Utilization

**Problem:** The formula divides amount by minutes, which gives a "rate" (dollars per minute), not a "utilization ratio" (percentage used).

```sql
-- Current formula:
SUM(preauthorization_amount) / NULLIF(SUM(preauthorization_minutes), 0)
-- Result: 11400 / 120 = 95 (dollars per minute?)

-- For a "utilization ratio" you'd expect:
-- actual_used / total_authorized (a percentage)
```

**Question:** Is this the intended metric? May need domain expert input.

### Summary of Issues

| Term | Issue | Severity |
|------|-------|----------|
| User Review Rating | Returns 2 rows instead of 1 | High |
| Average Fee Per Engagement | Formula doesn't match name | Medium |
| Preauthorization Utilization | Rate vs ratio semantic mismatch | Low (needs clarification) |

### Research Questions for FIX Plan

1. Should User Review Rating be split into separate terms?
2. What should "Average Fee Per Engagement" actually calculate?
3. Is "Preauthorization Utilization" correctly defined by domain experts?
4. Should glossary SQL be validated to return single-row results?
5. Should there be semantic validation comparing SQL output to term definition?

### Verification Commands

```sql
-- Test User Review Rating (returns 2 rows)
SELECT AVG(ur.reviewee_rating) FROM user_reviews ur WHERE ur.deleted_at IS NULL
UNION ALL
SELECT AVG(cr.rating) FROM channel_reviews cr WHERE cr.deleted_at IS NULL;

-- Test preauthorization ratio
SELECT
  SUM(preauthorization_amount) as total_amount,
  SUM(preauthorization_minutes) as total_minutes,
  SUM(preauthorization_amount) / NULLIF(SUM(preauthorization_minutes), 0) as ratio
FROM billing_engagements WHERE deleted_at IS NULL;
```

### Success Criteria

- All glossary SQL returns exactly one row
- SQL formulas match the semantic meaning of the term name
- Ratio/rate/percentage terms use appropriate mathematical formulas
- Domain expert review for ambiguous metrics

---

## General Verification Commands

### Check Current Ontology State
```sql
-- Active ontology
SELECT id, version, is_active, created_at
FROM engine_ontologies
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
ORDER BY created_at DESC;

-- Entity count
SELECT COUNT(*) FROM engine_ontology_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true;

-- Relationship count
SELECT COUNT(*) FROM engine_entity_relationships r
JOIN engine_ontologies o ON r.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true;
```

### Clear Ontology for Retest
```sql
-- Clear ALL ontology data for project (CASCADE handles child tables)
DELETE FROM engine_ontologies WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';
DELETE FROM engine_ontology_dag WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';
DELETE FROM engine_llm_conversations WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';
DELETE FROM engine_project_knowledge WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';
```

---

## Recommended Fix Order

1. **BUG-8** (Deselected columns in ontology) - CRITICAL SECURITY - PII exposure
2. **BUG-12** (Glossary wrong enums) - CRITICAL FUNCTIONAL - 55% of glossary SQL broken
3. **BUG-0** (MCP get_schema) - Quick fix, code bug still exists
4. **BUG-7** (Relationship discovery) - Quick fix, improves performance
5. **BUG-9** (Stats collection) - ROOT CAUSE of 68% missing relationships
6. **BUG-2** (File-based loading) - Architectural cleanup, clear removal plan
7. **BUG-1** (Pattern matching) - Design refactor, affects all other bugs
8. **BUG-3** (Missing FKs) - May auto-resolve from BUG-9; otherwise solved with BUG-1
9. **BUG-6** (Occurrence counts) - May auto-resolve from BUG-3/BUG-9
10. **BUG-11** (Relationship cardinality) - May auto-resolve from BUG-9
11. **BUG-10** (Glossary missing SQL) - Data quality
12. **BUG-13** (Glossary semantic issues) - Data quality, needs domain review

**Resolved Bugs (no action needed):**
- ~~BUG-4~~ (Sample tables as entities) - Entity discovery correctly filters by is_selected
- ~~BUG-5~~ (Domain pollution) - Resolved when sample tables deselected

**Priority Notes:**
- **BUG-8** is a CRITICAL security/privacy issue - PII columns exposed despite deselection
- **BUG-12** is a CRITICAL functional issue - glossary SQL returns wrong results
- BUG-0 code bug exists but not visible with current data; BUG-7 is quick fix
- BUG-9 is the root cause of most missing relationships (fixing it may resolve BUG-3, BUG-6, BUG-11)
- BUG-2 and BUG-1 are architectural blockers

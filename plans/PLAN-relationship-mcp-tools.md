# PLAN: Relationship MCP Tools - Surface Verified Joins to Clients

> **Updated 2026-01-30**: Revised after codebase review. Much of the original plan already exists.
> Key changes: Removed duplicate tasks, updated references to existing tables/services, focused on actual gaps.
>
> **Renamed from PLAN-fix-relationships.md**: This plan adds MCP tools that READ existing relationship
> data. It does NOT modify the relationship detection pipeline (which already works).

## Problem Statement

MCP clients (AI agents) are currently forced to **guess** at table relationships:

```
❌ Column naming conventions (guessing)
   host_id → probably joins to users.user_id

❌ Trial and error
   SELECT * FROM billing_engagements be
   JOIN users u ON be.host_id = u.user_id  -- hope this is right

❌ Manual exploration
   SELECT DISTINCT host_id FROM billing_engagements LIMIT 1;
   SELECT * FROM users WHERE user_id = 'that-uuid';
```

This leads to:
- Hallucinated columns/joins
- Wasted tokens on exploratory queries
- Incorrect query results
- Poor user experience

## Solution

**Ekaya should do the hard work during ontology extraction**, not the MCP client at query time.

This plan follows the established Column Features pattern:
1. **Deterministic data collection** (no LLM, no string matching)
2. **Data-driven FK candidate discovery** (overlap analysis between columns)
3. **LLM-assisted semantic classification** (determine role, meaning)
4. **Verification with actual data** (match rate, cardinality)
5. **Store verified relationships**
6. **Surface to MCP clients as facts, not guesses**

---

## What Already Exists (Codebase Review 2026-01-30)

### Architecture Pattern ✅ ALREADY IMPLEMENTED

The codebase already follows the deterministic → LLM → deterministic pattern:

```
ColumnFeatureExtraction (DAG Node 2)
    ├─ Phase 4: FK Resolution via data overlap
    └─ Output: IdentifierFeatures with FKTargetTable/FKTargetColumn

FKDiscovery (DAG Node 5) - `deterministic_relationship_service.go`
    ├─ Phase 1: Uses ColumnFeatures (data overlap results)
    ├─ Phase 2: Uses database FK constraints
    └─ Output: EntityRelationships in engine_entity_relationships

PKMatchDiscovery (DAG Node 6) - `deterministic_relationship_service.go`
    ├─ Tests joins via SQL for columns without explicit FKs
    └─ Output: More EntityRelationships

RelationshipEnrichment (DAG Node 8) - `relationship_enrichment.go`
    ├─ LLM generates descriptions and associations
    ├─ Includes project knowledge for role context (host/visitor/etc)
    └─ Output: Updated EntityRelationships with descriptions
```

### Relationship Storage ✅ ALREADY EXISTS

Two tables store relationship data:

1. **`engine_entity_relationships`** - Entity-level relationships
   - source/target entity IDs, cardinality, detection_method, confidence
   - description, association (from LLM enrichment)
   - provenance tracking (source, created_by, updated_by)

2. **`engine_schema_relationships`** - Column-level with metrics
   - source/target column IDs, cardinality
   - match_rate, source_distinct, target_distinct, matched_count
   - rejection_reason for candidates that didn't pass validation

### MCP Tools ✅ ALREADY IMPLEMENTED

1. **`probe_relationship`** (`pkg/mcp/tools/probe.go:580`)
   - Returns relationships with entity names, cardinality, description, label
   - Includes data quality metrics (match_rate, orphan_count, source_distinct, target_distinct)
   - Shows rejected candidates with rejection reasons

2. **`get_context`** (`pkg/mcp/tools/context.go:722`)
   - At `depth='columns'`, FK columns show:
     - `fk_target_table`, `fk_target_column`, `fk_confidence`, `entity_referenced`

3. **`update_relationship` / `delete_relationship`** (`pkg/mcp/tools/relationship.go`)
   - Entity-based CRUD with provenance tracking

### Repository Layer ✅ ALREADY EXISTS

- `EntityRelationshipRepository` in `pkg/repositories/entity_relationship_repository.go`
- Methods: `Create`, `Upsert`, `GetByProject`, `GetByOntology`, `GetByEntityPair`, `UpdateDescription`, `UpdateDescriptionAndAssociation`, `Delete`

---

## What's Actually Missing (Real Gaps)

### Gap 1: `get_join_path` Tool - NOT IMPLEMENTED

Given two tables, find verified paths to join them. This is the **highest value missing feature**.

```
get_join_path(from_table='billing_engagements', to_table='accounts')
```

Response:
```json
{
  "from_table": "billing_engagements",
  "to_table": "accounts",
  "paths": [
    {
      "description": "Via host user",
      "hops": [
        {
          "from": "billing_engagements.host_id",
          "to": "users.user_id",
          "cardinality": "N:1",
          "role": "host"
        },
        {
          "from": "users.account_id",
          "to": "accounts.account_id",
          "cardinality": "N:1"
        }
      ],
      "total_hops": 2,
      "sql_hint": "JOIN users ON host_id = users.user_id JOIN accounts ON users.account_id = accounts.account_id"
    }
  ]
}
```

### Gap 2: Role Storage on Relationships - PARTIAL

Current state:
- Roles are extracted during column feature extraction (`EntityReferenced` field)
- Relationship enrichment generates descriptions with role context
- But `source_role` is not a dedicated field on `engine_entity_relationships`

Impact: Low. Roles are accessible via ColumnFeatures and relationship descriptions.

### Gap 3: `validate` Tool JOIN Verification - NOT IMPLEMENTED

Enhance `validate` tool to check JOINs against known relationships:

```
validate(sql="SELECT * FROM billing_engagements be JOIN users u ON be.host_id = u.user_id")
```

Response includes relationship validation:
```json
{
  "syntax_valid": true,
  "joins_valid": true,
  "join_details": [
    {
      "join": "billing_engagements.host_id = users.user_id",
      "verified": true,
      "cardinality": "N:1",
      "match_rate": 99.8
    }
  ]
}
```

---

## Implementation Tasks (Revised)

### Task 1: Implement `get_join_path` Tool

**File:** `pkg/mcp/tools/join_path.go` (new)

**Implementation approach:**
1. Build graph from `engine_entity_relationships` (or query via `EntityRelationshipRepository`)
2. BFS/DFS to find all paths up to 3 hops
3. Return SQL hints for each path
4. Handle multiple paths (via host vs via visitor)

**Acceptance criteria:**
- Query relationships by project
- Build adjacency list from source/target tables
- Find all paths between from_table and to_table
- Generate SQL JOIN hints
- Handle bidirectional relationships (relationships are stored both directions)

**Dependencies:**
- `EntityRelationshipRepository.GetByProject()`
- Register tool in `RegisterProbeTools()` or create new `RegisterJoinPathTools()`
- Add to `LoadoutQuery` and `AllToolsOrdered` in `mcp_tool_loadouts.go`

---

### Task 2: Enhance `validate` Tool with JOIN Verification

**File:** `pkg/mcp/tools/query_tools.go` (existing)

**Implementation approach:**
1. Parse SQL to extract JOIN conditions (use sqlparser)
2. For each JOIN, look up relationship in `engine_entity_relationships`
3. Add `join_details` array to validation response
4. Mark each join as verified/unverified with metrics

**Acceptance criteria:**
- Extract JOIN clauses from SQL
- Match JOINs to known relationships
- Include cardinality and match_rate in response
- Warn (not fail) for unverified JOINs

---

### Task 3 (Optional): Add `source_role` Field to Relationships

**File:** `migrations/XXX_add_role_to_relationships.sql`

Add dedicated role field to `engine_entity_relationships`:

```sql
ALTER TABLE engine_entity_relationships
ADD COLUMN source_role VARCHAR(100);

COMMENT ON COLUMN engine_entity_relationships.source_role IS
'Semantic role of source column (e.g., host, visitor, payer). Extracted from ColumnFeatures or LLM enrichment.';
```

**File:** `pkg/services/relationship_enrichment.go`

During enrichment, extract role from column name or LLM response and store in `source_role`.

**Impact:** Low priority. Current system already captures roles in descriptions and ColumnFeatures.

---

## Removed Tasks (Already Implemented)

The following tasks from the original plan are **not needed**:

| Original Task | Why Not Needed |
|---------------|----------------|
| Task 1: Enhance Phase 4 FK Resolution | Already implemented in `column_feature_extraction.go` |
| Task 2: Create `engine_relationships` table | Already exists as `engine_entity_relationships` + `engine_schema_relationships` |
| Task 3: Implement Relationship Repository | Already exists as `EntityRelationshipRepository` |
| Task 4: Enhance RelationshipEnrichment DAG Node | Already implemented in `relationship_enrichment.go` |
| Task 5: Update `get_context` to Include FK References | Already includes `fk_target_table`, `fk_target_column` via ColumnFeatures |
| Task 6: Fix `probe_relationship` | Already works - returns relationships with metrics |

---

## Success Metrics (Updated)

| Metric | Current State | After |
|--------|---------------|-------|
| `probe_relationship` returns data | ✅ Yes | ✅ Yes |
| FK columns show references in `get_context` | ✅ Yes (via ColumnFeatures) | ✅ Yes |
| Join paths discoverable | ❌ No | ✅ Yes (new `get_join_path` tool) |
| `validate` checks JOINs | ❌ No | ✅ Yes |
| MCP client needs to guess joins | ⚠️ Sometimes | ✅ No |
| Column name pattern matching | ✅ Not used | ✅ Not used |

---

## Key Design Decisions (Unchanged)

### Why No Column Name Pattern Matching?

1. **Unreliable**: Not all FKs follow naming conventions
2. **False positives**: `status_id` might be an enum, not a FK to `statuses` table
3. **Misses valid FKs**: `creator` column → `users.user_id` wouldn't be found
4. **LLM already classifies**: Phase 2 determines `identifier` vs other types
5. **Data is ground truth**: If values match, it's a relationship - regardless of name

### Why Use Data Overlap?

1. **Deterministic**: 98% match rate is a fact, not a guess
2. **Universal**: Works regardless of naming conventions
3. **Verifiable**: MCP clients can trust the match_rate metric
4. **Efficient**: Sample-based overlap is fast (50 values per column)
5. **Scalable**: Full verification only runs on high-confidence candidates

---

## Dependencies

- **Column Features implemented** (DAG Node 2) ✅
- **Entity relationships implemented** ✅
- **Relationship enrichment implemented** ✅
- Only new work: `get_join_path` tool + `validate` enhancement

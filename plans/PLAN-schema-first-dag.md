# Plan: Schema-First DAG Refactor

## Overview

Refactor the ontology extraction DAG to focus on schema metadata and table-to-table relationships first, without entity dependencies. Entities become an optional convenience layer added later.

## Goals

1. Extract rich schema metadata (columns + tables) before relationship discovery
2. Store all relationships in `engine_schema_relationships` (not `engine_entity_relationships`)
3. Remove entity dependencies from relationship discovery
4. Fix the bidirectional join validation bug (false positives like `identity_provider` → `jobs.id`)
5. Provide granular UI progress updates (one LLM call per table, not batch)

## Target DAG Order

```
1. KnowledgeSeeding         (existing - no changes)
2. ColumnFeatureExtraction  (existing - no changes)
3. FKDiscovery              (refactor - write to SchemaRelationship)
4. TableFeatureExtraction   (NEW - one LLM call per table)
5. PKMatchDiscovery         (refactor - write to SchemaRelationship, fix validation)
```

Steps after this (EntityDiscovery, EntityEnrichment, etc.) remain commented out for manual testing.

---

## Step 1: Add TableFeatureExtraction DAG Node

### 1.1 Create TableFeatureExtraction service

**File:** `pkg/services/table_feature_extraction.go`

**Purpose:** Generate table-level descriptions based on column features already extracted.

**Inputs per table:**
- Table name, schema
- All columns with their ColumnFeatures (PKs, FKs, enums, semantic types, purposes)
- Any declared FK relationships from schema introspection
- Row count

**Outputs per table (stored in `engine_table_metadata`):**
- `description`: What this table represents
- `usage_notes`: When to use/not use this table
- `is_ephemeral`: Whether it's transient/temp data

**LLM call pattern:**
- One LLM call per table (not batched)
- Parallel execution via worker pool
- Progress callback updates UI after each table completes

**System prompt focus:**
- Synthesize column features into table purpose
- Identify if table is transactional vs reference vs logging
- Note key columns and their roles

### 1.2 Create DAG node wrapper

**File:** `pkg/services/dag/table_feature_extraction_node.go`

- Wraps the service
- Reports progress: "Analyzing table X (5/30)"
- Follows BaseNode pattern

### 1.3 Register node in DAG service

**File:** `pkg/services/ontology_dag_service.go`

- Add to node registry
- Wire up dependencies

### 1.4 Update DAG order

**File:** `pkg/models/ontology_dag.go`

- Add `DAGNodeTableFeatureExtraction` constant
- Update `DAGNodeOrder` map
- Update `AllDAGNodes()` function

---

## Step 2: Refactor FKDiscovery to Write to SchemaRelationship

### 2.1 Update FKDiscovery service

**File:** `pkg/services/deterministic_relationship_service.go`

**Current behavior:**
- Creates `EntityRelationship` records
- Requires entities to exist (returns early if none)
- Uses `entityByPrimaryTable` lookup

**New behavior:**
- Create `SchemaRelationship` records instead
- No entity dependency
- Set `inference_method = 'column_features'` for ColumnFeature-derived FKs
- Set `inference_method = 'fk'` for schema-declared FKs (already done by schema discovery)

**Key changes:**
- Remove entity lookup/requirement
- Change `relationshipRepo.Create()` → `schemaRepo.CreateRelationship()` or similar
- Remove bidirectional EntityRelationship creation (SchemaRelationship is unidirectional?)

### 2.2 Verify SchemaRelationship storage

**Check:** Does `engine_schema_relationships` support the fields we need?
- `inference_method` - yes, exists
- `confidence` - yes, exists
- `cardinality` - yes, exists
- `validation_results` - yes, exists

**May need:** Repository method to create/upsert schema relationships from discovery (not just schema introspection).

---

## Step 3: Refactor PKMatchDiscovery

### 3.1 Remove entity dependency

**File:** `pkg/services/deterministic_relationship_service.go`

**Current behavior (lines 711-717):**
```go
entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
if len(entities) == 0 {
    return &PKMatchDiscoveryResult{}, nil // Returns empty
}
```

**New behavior:**
- Don't require entities
- Build candidate columns from schema (PKs, unique columns, high cardinality)
- Build reference columns from ColumnFeatures (Role=foreign_key, Purpose=identifier)

### 3.2 Write to SchemaRelationship

- Same as FKDiscovery - write to `engine_schema_relationships`
- Set `inference_method = 'pk_match'`
- Store validation metrics (match_rate, source_distinct, target_distinct, etc.)

### 3.3 Fix bidirectional join validation

**File:** `pkg/adapters/datasource/postgres/schema.go` (AnalyzeJoin method)

**Current bug:** Only checks orphans in source→target direction.

**Example of false positive:**
- `identity_provider` has 3 distinct values: {1, 2, 3}
- `jobs.id` has 83 distinct values: {1, 2, ..., 83}
- Source→Target: All 3 values exist in jobs.id → 0 orphans → PASSES
- Target→Source: 80 values (4-83) don't exist in identity_provider → NEVER CHECKED

**Fix:** Add reverse orphan check to AnalyzeJoin:
```sql
-- Add to existing CTE
reverse_orphans AS (
    SELECT COUNT(DISTINCT t.target_col) as reverse_orphan_count
    FROM target_table t
    LEFT JOIN source_table s ON t.target_col = s.source_col
    WHERE s.source_col IS NULL
)
```

**Rejection logic:**
- If source has few distinct values and target has many more, require high match rate in BOTH directions
- Reject if reverse_orphan_count / target_distinct > threshold (e.g., 0.5)

### 3.4 Use table descriptions in validation prompt

After TableFeatureExtraction runs, PKMatchDiscovery can use table descriptions in the join validation prompt:
- "Does it make sense for `identity_provider` (enum column in account_authentications, which tracks authentication methods) to reference `jobs.id` (PK of jobs table, which tracks background processing tasks)?"
- LLM can immediately see this is nonsensical

---

## Step 4: Update Repository Layer

### 4.1 Add SchemaRelationship creation method

**File:** `pkg/repositories/schema_repository.go`

Add method for creating inferred relationships (not just from schema introspection):
```go
CreateInferredRelationship(ctx context.Context, rel *models.SchemaRelationship) error
UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error
```

### 4.2 Add methods to query by inference_method

```go
GetRelationshipsByMethod(ctx context.Context, projectID uuid.UUID, method string) ([]*models.SchemaRelationship, error)
```

---

## Step 5: Testing

### 5.1 Unit tests for TableFeatureExtraction service

Write unit tests for the TableFeatureExtraction service that generates table-level descriptions based on column features.

**File to create:** `pkg/services/table_feature_extraction_test.go`

**Service under test:** `pkg/services/table_feature_extraction.go` - generates table-level metadata (description, usage_notes, is_ephemeral) by synthesizing column features via LLM calls.

**Test cases to implement:**

1. [ ] **Happy path**: Given a table with columns that have ColumnFeatures (PKs, FKs, enums, semantic types), verify:
   - LLM is called with correct prompt containing column metadata
   - Results are stored in `engine_table_metadata` via repository
   - Returned TableMetadata contains correct description, usage_notes, is_ephemeral

2. [ ] **Empty columns**: Table with no columns should handle gracefully (not error, return minimal metadata)

3. [ ] **LLM error handling**: When LLM client returns error, verify error is propagated immediately (fail-fast pattern per project guidelines)

4. [ ] **Progress callback invocation**: Verify progress callback is invoked after each table completes with correct progress fraction

5. [ ] **Parallel execution via worker pool**: Verify multiple tables are processed concurrently (mock multiple tables, verify concurrent calls)

6. [ ] **Output validation**: Verify the service correctly parses LLM response and populates:
   - `description`: What this table represents
   - `usage_notes`: When to use/not use this table
   - `is_ephemeral`: Whether it's transient/temp data

**Dependencies to mock:**
- `TableMetadataRepository` interface - for storing results
- `SchemaRepository` interface - for fetching columns with features
- LLM client interface (likely `pkg/llm` package)
- Progress callback function

**Reference for test patterns:** See `pkg/services/column_feature_extraction_test.go` for similar service test patterns in this codebase.

---

### 5.2 Unit tests for FKDiscovery and PKMatchDiscovery SchemaRelationship writing ✓

Write unit tests verifying that both FKDiscovery and PKMatchDiscovery correctly write to `engine_schema_relationships` without entity dependencies.

**File to update:** `pkg/services/deterministic_relationship_service_test.go`

**Service under test:** `pkg/services/deterministic_relationship_service.go` - the refactored service that discovers relationships and writes to SchemaRelationship instead of EntityRelationship.

**FKDiscovery test cases:**

1. [x] **FK from ColumnFeatures**: When a column has `Role=foreign_key` in its ColumnFeatures, verify:
   - SchemaRelationship is created with `inference_method='column_features'`
   - Correct fields: `source_table`, `source_column`, `target_table`, `target_column`, `confidence`

2. [x] **No entity dependency**: Verify the service does NOT require entities to exist - should NOT return early when no entities found

3. [x] **Upsert behavior**: If relationship already exists, verify it's updated not duplicated

4. [x] **Error propagation**: Verify repository errors are propagated immediately (fail-fast)

**PKMatchDiscovery test cases:**

1. [x] **No entities exist**: When `entityRepo.GetByOntology()` returns empty, PKMatchDiscovery should still proceed (not return empty result as it did before the refactor at lines 711-717)

2. [x] **Build candidates from schema**: Verify candidate columns are built from PKs, unique columns, and high-cardinality columns from schema metadata (not from entities)

3. [x] **Build references from ColumnFeatures**: Verify reference columns are built from ColumnFeatures with `Role=foreign_key` or `Purpose=identifier`

4. [x] **Write to SchemaRelationship**: Verify discovered matches are written with `inference_method='pk_match'`

5. [x] **Validation metrics stored**: Verify `validation_results` field contains match_rate, source_distinct, target_distinct, orphan_count

**Dependencies to mock:**
- `SchemaRepository` interface (methods: `CreateInferredRelationship`, `UpsertRelationship`, `GetRelationshipsByMethod`)
- `ColumnFeatureRepository` (to return columns with FK role)
- `EntityRepository` (to verify it's called but empty result doesn't stop processing)
- `DatasourceAdapter` (for AnalyzeJoin validation in PKMatchDiscovery)

---

### 5.3 Unit tests for bidirectional join validation ✓

Write unit tests for the bidirectional join validation fix in the PostgreSQL adapter's AnalyzeJoin method.

**File to update:** `pkg/adapters/datasource/postgres/schema_test.go`

**Code under test:** `pkg/adapters/datasource/postgres/schema.go` - the `AnalyzeJoin` method that validates FK candidates.

**Bug context:** The previous implementation only checked orphans in source→target direction. This caused false positives like:
- `identity_provider` (3 distinct values: {1,2,3}) appearing to reference `jobs.id` (83 distinct values)
- All 3 source values exist in target → 0 orphans → incorrectly PASSED
- But 80 target values (4-83) don't exist in source → NEVER CHECKED

**Test cases to implement:**

1. [x] **Reject asymmetric cardinality**:
   - Setup: Source column has 3 distinct values {1,2,3}, target has 83 values {1..83}
   - Source→target: 0 orphans (all 3 exist in 83)
   - Target→source: 80 orphans (values 4-83 don't exist in source)
   - Expected: REJECT this relationship

2. [x] **Accept valid FK**:
   - Setup: Source column references target PK with high match rate in both directions
   - Expected: ACCEPT

3. [x] **Accept partial FK based on thresholds**:
   - Setup: Source has some orphans but reverse check passes (target values mostly exist in source)
   - Expected: ACCEPT based on configurable thresholds

4. [x] **Reverse orphan threshold enforcement**:
   - Verify: If `reverse_orphan_count / target_distinct > 0.5`, should reject
   - Test boundary conditions around the threshold

5. [x] **Verify SQL includes reverse_orphans CTE**:
   - The fix added a CTE like:
   ```sql
   reverse_orphans AS (
       SELECT COUNT(DISTINCT t.target_col) as reverse_orphan_count
       FROM target_table t
       LEFT JOIN source_table s ON t.target_col = s.source_col
       WHERE s.source_col IS NULL
   )
   ```
   - Verify this CTE is present and results are used in rejection logic

**Test setup requirements:**
- These tests need a test database connection
- Use `testhelpers.GetTestDB(t)` from `pkg/testhelpers/containers.go` to get containerized PostgreSQL
- Create test tables with controlled data to verify validation logic
- Example setup SQL:
  ```sql
  CREATE TABLE test_target (id INT PRIMARY KEY);
  INSERT INTO test_target SELECT generate_series(1, 83);

  CREATE TABLE test_source (identity_provider INT);
  INSERT INTO test_source VALUES (1), (2), (3), (1), (2);
  ```

---

### 5.4 Integration tests for full DAG relationship discovery ✓

Write integration tests that run the DAG through PKMatchDiscovery and verify correct relationship discovery without false positives.

**File to create:** `pkg/services/dag/relationship_discovery_integration_test.go` (or add to existing integration test file)

**Test scope:** Full DAG execution from KnowledgeSeeding through PKMatchDiscovery to verify the refactored pipeline works end-to-end.

**Test cases:**

1. [x] **Full DAG run through PKMatchDiscovery**:
   - Setup: Create a test project with schema containing:
     - A clear FK relationship (e.g., `orders.customer_id` → `customers.id`)
     - A false positive candidate (e.g., `settings.identity_provider` with values {1,2,3} and a `jobs` table with id 1-100)
   - Execute: Run DAG nodes in order: KnowledgeSeeding → ColumnFeatureExtraction → FKDiscovery → TableFeatureExtraction → PKMatchDiscovery
   - Verify: DAG completes successfully

2. [x] **Verify no false positives like identity_provider → jobs.id**:
   - Query `engine_schema_relationships` after DAG completion
   - Assert: No relationship exists between `identity_provider` column and `jobs.id`
   - This validates the bidirectional join fix is working

3. [x] **Verify legitimate relationships discovered**:
   - Assert: The `orders.customer_id` → `customers.id` relationship IS discovered
   - Verify: `inference_method` is correctly set ('fk', 'column_features', or 'pk_match' depending on discovery path)
   - Verify: `validation_results` contains expected metrics

4. [x] **Verify SchemaRelationship not EntityRelationship**:
   - Assert: Relationships are in `engine_schema_relationships` table
   - Assert: `engine_entity_relationships` is NOT populated by these discovery steps

**Test setup:**
- Use `testhelpers.GetEngineDB(t)` for engine database with migrations
- Use `testhelpers.GetTestDB(t)` for test datasource with controlled schema
- Create isolated test project with unique `project_id`
- Cleanup after test completes

**Reference:** Check existing integration tests in `pkg/services/dag/` for patterns on running DAG nodes in tests.

---

### 5.5 Manual testing ✓

- [x] Run DAG on test project
- [x] Review relationships in UI
- [x] Verify table descriptions are generated and useful

---

## Implementation Order

1. [x] **Step 4** - Add repository methods (foundation, including query by inference_method)
2. [x] **Step 3.3** - Fix bidirectional validation (critical bug)
3. [x] **Step 2** - Refactor FKDiscovery to use SchemaRelationship
4. [x] **Step 3.1-3.2** - Refactor PKMatchDiscovery to use SchemaRelationship
5. [x] **Step 1** - Add TableFeatureExtraction (new node)
6. [x] **Step 3.4** - Use table descriptions in validation
7. [x] **Step 5** - Testing

---

## Out of Scope (Future Work)

- EntityDiscovery - grouping tables into higher-level concepts
- EntityEnrichment - adding descriptions to entities
- EntityRelationships - multi-hop join paths between entities
- RelationshipEnrichment - LLM descriptions for relationships
- OntologyFinalization - building tiered summaries
- GlossaryDiscovery/Enrichment

These remain commented out in the DAG until schema-level extraction is solid.

---

## Success Criteria

1. DAG runs through PKMatchDiscovery without requiring entities
2. All relationships stored in `engine_schema_relationships`
3. No false positives like `identity_provider` → `jobs.id`
4. Table descriptions generated for all selected tables
5. UI shows granular progress (per-table updates)
6. MCP tools can query schema relationships without entity layer

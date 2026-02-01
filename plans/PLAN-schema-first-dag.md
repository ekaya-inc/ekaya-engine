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

### 5.1 Unit tests

- TableFeatureExtraction service tests
- FKDiscovery writing to SchemaRelationship
- PKMatchDiscovery without entities
- Bidirectional join validation logic

### 5.2 Integration tests

- Full DAG run through PKMatchDiscovery
- Verify no false positives like `identity_provider` → `jobs.id`
- Verify legitimate relationships are discovered

### 5.3 Manual testing

- Run DAG on test project
- Review relationships in UI
- Verify table descriptions are generated and useful

---

## Implementation Order

1. [x] **Step 4.1** - Add repository methods (foundation)
2. [ ] **Step 3.3** - Fix bidirectional validation (critical bug)
3. [ ] **Step 2** - Refactor FKDiscovery to use SchemaRelationship
4. [ ] **Step 3.1-3.2** - Refactor PKMatchDiscovery to use SchemaRelationship
5. [ ] **Step 1** - Add TableFeatureExtraction (new node)
6. [ ] **Step 3.4** - Use table descriptions in validation
7. [ ] **Step 5** - Testing

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

# PLAN: Incremental Ontology Updates

**Date:** 2026-03-02
**Status:** TODO
**Priority:** HIGH

## Problem

Today, refreshing the ontology requires a full Delete + Re-extract cycle. This wipes all user corrections (answered questions, MCP-enriched descriptions, manually-set enum values, relationship approvals) and forces a complete re-extraction from scratch. This is not viable as a product — users lose hours of curation work every time the underlying database schema changes.

## Solution: ChangeSet-Driven Incremental DAG

Instead of wiping and rebuilding, compute a `ChangeSet` of what changed in the schema since the last successful extraction, then run all DAG nodes but scope each node's work to only the affected items.

### Design Decisions

- **UI:** Two buttons — "Delete Ontology" (full wipe) and "Refresh Ontology" (incremental). A full re-extract is Delete + Refresh.
- **Change detection:** Compare `updated_at` / `created_at` / `deleted_at` timestamps on schema tables/columns against the last DAG's `completed_at`.
- **User edits:** Columns/tables with `last_edit_source IS NOT NULL` (user or MCP edited) are skipped entirely during re-extraction.
- **Deleted items:** Ontology metadata for deleted tables/columns is cleaned up automatically.
- **DAG execution:** All nodes always run. Each node internally processes only the changed subset. Nodes with no work complete instantly with "0 items processed."
- **Backward compatible:** If no previous DAG exists, falls through to full extraction.

## Architecture

### ChangeSet Computation

```
last_built_at = completed_at from most recent successful DAG

Changed tables:  WHERE updated_at > built_at OR created_at > built_at
Deleted tables:  WHERE deleted_at IS NOT NULL AND deleted_at > built_at
Changed columns: WHERE updated_at > built_at OR created_at > built_at
Deleted columns: WHERE deleted_at IS NOT NULL AND deleted_at > built_at

User-edited (skip): column_metadata/table_metadata WHERE last_edit_source IS NOT NULL
```

### ChangeSet Struct

```go
type ChangeSet struct {
    BuiltAt time.Time  // completed_at from last successful DAG

    // Tables
    AddedTables    []SchemaTable   // created_at > built_at
    ModifiedTables []SchemaTable   // updated_at > built_at, created_at <= built_at
    DeletedTables  []SchemaTable   // deleted_at > built_at

    // Columns
    AddedColumns    []SchemaColumn  // created_at > built_at
    ModifiedColumns []SchemaColumn  // updated_at > built_at, created_at <= built_at
    DeletedColumns  []SchemaColumn  // deleted_at > built_at

    // Computed helpers
    AffectedTableIDs map[uuid.UUID]bool  // union of all table IDs that need re-processing
    UserEditedIDs    map[uuid.UUID]bool  // column/table metadata IDs to skip (last_edit_source != NULL)
}

func (cs *ChangeSet) IsEmpty() bool  // true if no changes detected
```

### How Each Node Uses the ChangeSet

| Node | Full Extraction (ChangeSet nil) | Incremental (ChangeSet present) |
|------|-------------------------------|-------------------------------|
| KnowledgeSeeding | Extract from overview | Skip (knowledge has project-lifecycle scope) |
| ColumnFeatureExtraction | All selected columns | Only added/modified columns, skip user-edited |
| FKDiscovery | All FK constraints | All FK constraints (cheap, no LLM, always full) |
| TableFeatureExtraction | All selected tables | Only affected tables, skip user-edited |
| RelationshipDiscovery | All candidate pairs | Only pairs involving changed columns |
| ColumnEnrichment | All columns | Added/modified columns + columns with changed relationships, skip user-edited |
| OntologyFinalization | Full domain summary | Always re-generate (single LLM call) |

### Pre-Processing: Cleanup for Deleted Items

Before DAG nodes run, a cleanup step removes ontology artifacts for deleted tables/columns:

1. Delete `engine_ontology_column_metadata` rows where `schema_column_id` references a deleted column
2. Delete `engine_ontology_table_metadata` rows where `schema_table_id` references a deleted table
3. Soft-delete `engine_schema_relationships` where source or target column was deleted
4. Delete questions whose `affects` references only deleted tables/columns

### Provenance-Based Skip Rules

A column is **skipped** during re-extraction if:
- `engine_ontology_column_metadata.last_edit_source IS NOT NULL`
- AND the column is NOT in `AddedColumns` (new columns always get extracted)

A table is **skipped** during re-extraction if:
- `engine_ontology_table_metadata.last_edit_source IS NOT NULL`
- AND the table is NOT in `AddedTables`

Relationships:
- DB FK relationships (`inference_method = 'fk'`): Always re-discovered from constraints
- Inferred relationships: Only re-evaluate for changed columns; existing inferred relationships for unchanged columns preserved
- Manual relationships: Never touched by extraction

Questions:
- Existing questions preserved (`content_hash` deduplication prevents duplicates)
- New questions generated for new/modified items
- Questions referencing only deleted tables/columns cleaned up

### API Changes

**New endpoint:** `GET /api/projects/{pid}/datasources/{dsid}/ontology/status`

Returns:
```json
{
  "has_ontology": true,
  "last_built_at": "2026-03-01T10:00:00Z",
  "schema_changed_since_build": true,
  "change_summary": {
    "tables_added": 1,
    "tables_deleted": 1,
    "columns_added": 3,
    "columns_modified": 2,
    "columns_deleted": 1
  }
}
```

Query for dirty check:
```sql
SELECT EXISTS(
  SELECT 1 FROM engine_schema_tables
  WHERE project_id = $1 AND (updated_at > $2 OR (deleted_at IS NOT NULL AND deleted_at > $2))
  UNION ALL
  SELECT 1 FROM engine_schema_columns
  WHERE project_id = $1 AND (updated_at > $2 OR (deleted_at IS NOT NULL AND deleted_at > $2))
)
```

**Modified endpoint:** `POST /api/projects/{pid}/datasources/{dsid}/ontology/extract`
- Computes ChangeSet before creating DAG
- Returns `is_incremental: true/false` in response
- When no previous extraction exists, does full extraction

**Enhanced DAG status response:**
```json
{
  "dag_id": "...",
  "is_incremental": true,
  "change_summary": { "tables_added": 1, "columns_modified": 2 }
}
```

**Unchanged:** `DELETE /api/projects/{pid}/datasources/{dsid}/ontology` — continues full wipe

### Node Interface Change

```go
// Before:
type DAGNodeExecutor interface {
    Execute(ctx context.Context, dag *models.OntologyDAG,
            progressCallback func(current, total int, message string)) error
}

// After:
type DAGNodeExecutor interface {
    Execute(ctx context.Context, dag *models.OntologyDAG,
            changeSet *models.ChangeSet, // nil = full extraction
            progressCallback func(current, total int, message string)) error
}
```

## Acceptance Criteria

| Change Made | Expected Incremental Behavior |
|-------------|-------------------------------|
| Add a new table with columns | ChangeSet has AddedTables + AddedColumns. Only new items processed by ColumnFeatureExtraction, TableFeatureExtraction, RelationshipDiscovery, ColumnEnrichment. |
| New enum value in existing column | `scan_data_changes` updates column → `updated_at` bumps. ChangeSet has ModifiedColumns. ColumnEnrichment re-enriches with new enum values. |
| New foreign key constraint | Schema refresh detects new FK → `updated_at` bumps. FKDiscovery picks up new constraint. RelationshipDiscovery validates it. |
| Delete a column | Schema refresh soft-deletes column. Pre-processing cleanup removes column_metadata, affected relationships, orphaned questions. |
| Delete a table | Schema refresh soft-deletes table + columns. Pre-processing cleanup removes all metadata, relationships, and orphaned questions. |

### What Is Preserved Across Refresh

- Answered/skipped/dismissed questions (content_hash deduplication)
- User/MCP-edited column metadata (descriptions, enum values, roles, semantic types)
- User/MCP-edited table metadata (descriptions, types, usage notes)
- Project knowledge (project-lifecycle scope, never deleted by refresh)
- Manual/MCP glossary terms
- DB FK relationships
- `is_sensitive` user overrides
- Chat message history

## Implementation Checklist

### Phase 1: ChangeSet Infrastructure

- [x] 1. Create `pkg/models/change_set.go` — ChangeSet struct with `IsEmpty()`, `HasChangedColumns()`, `HasChangedTables()`, `ShouldSkipColumn(id)`, `ShouldSkipTable(id)` helper methods
- [x] 2. Create `pkg/services/ontology_dag_incremental.go` — `ComputeChangeSet(ctx, projectID, builtAt)` method that queries schema tables/columns by timestamp and column/table metadata for user-edited exclusions
- [x] 3. Add `IsIncremental` (bool) and `ChangeSummary` (JSONB) fields to `engine_ontology_dag` table via migration — stores whether this was an incremental run and a summary of what changed
- [x] 4. Add `CleanupDeletedItems(ctx, projectID, changeSet)` method to `ontology_dag_incremental.go` — removes column_metadata, table_metadata, relationships, and orphaned questions for deleted items

### Phase 2: Node Interface & Passthrough

- [x] 5. Update `NodeExecutor` interface in `pkg/services/dag/node_executor.go` to accept `*models.ChangeSet` parameter
- [x] 6. Update `executeNode()` in `ontology_dag_service.go` to pass the ChangeSet to each node
- [x] 7. Update all existing node implementations to accept the new parameter — initially ignore it (full extraction behavior preserved)

### Phase 3: Incremental Start Flow

- [x] 8. Modify `ontology_dag_service.Start()` to compute ChangeSet when a previous completed DAG exists
- [ ] 9. Add integration test: start extraction with no prior DAG → full extraction (backward compatible)
- [ ] 10. Add integration test: start extraction with no schema changes → returns "no changes", no DAG created

### Phase 4: Incremental Node Implementations

- [x] 11. Update `KnowledgeSeedingNode` — skip entirely when ChangeSet is present (knowledge has project-lifecycle scope)
- [x] 12. Update `ColumnFeatureExtractionNode` — skip when no columns changed during incremental
- [x] 13. Update `FKDiscoveryNode` — always run full (cheap, no LLM). No changes needed beyond accepting the parameter.
- [x] 14. Update `TableFeatureExtractionNode` — skip when no tables affected during incremental
- [x] 15. Update `RelationshipDiscoveryNode` — accepts ChangeSet param; always runs full (preserves existing relationships)
- [x] 16. Update `ColumnEnrichmentNode` — filter to affected table names during incremental
- [x] 17. Update `OntologyFinalizationNode` — always re-generate domain summary (single LLM call, always runs)

### Phase 5: API & Handler Changes

- [x] 18. Create `GET /ontology/status` endpoint in `ontology_dag_handler.go` — returns dirty check result with change summary
- [x] 19. Update DAG status response (`toDAGResponse`) to include `is_incremental` and `change_summary` fields
- [x] 20. Update `StartExtraction` handler to return incremental status in response

### Phase 6: Frontend Changes

- [x] 21. Update Ontology screen to call `GET /ontology/status` on load — show badge/indicator when `schema_changed_since_build` is true
- [x] 22. Replace "Re-extract Ontology" button with "Refresh Ontology" — calls same `POST /ontology/extract` endpoint
- [x] 23. Update DAG progress display to show incremental context (e.g., "Processing 1 new table, 2 modified columns" instead of "Extracting ontology")

### Phase 7: Integration Testing

- [ ] 24. Integration test: add new table → refresh → only new table/columns processed, existing metadata unchanged
- [ ] 25. Integration test: modify column data (new enum value via scan_data) → refresh → only modified column re-enriched
- [ ] 26. Integration test: delete column → refresh → column metadata and affected relationships cleaned up
- [ ] 27. Integration test: delete table → refresh → all metadata for table and its columns cleaned up
- [ ] 28. Integration test: user-edited column → refresh → column skipped, user edits preserved
- [ ] 29. Integration test: mixed changes (add table + delete column + modify column) → correct subset processed

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/models/change_set.go` | **NEW** — ChangeSet struct and helpers |
| `pkg/models/ontology_dag.go` | Add `IsIncremental`, `ChangeSummary` fields |
| `pkg/services/incremental_dag_service.go` | **NEW** — ComputeChangeSet, CleanupDeletedItems |
| `pkg/services/dag/node_executor.go` | Update DAGNodeExecutor interface |
| `pkg/services/dag/knowledge_seeding_node.go` | Accept ChangeSet, skip when incremental |
| `pkg/services/dag/column_feature_extraction_node.go` | Accept ChangeSet, filter to changed columns |
| `pkg/services/dag/fk_discovery_node.go` | Accept ChangeSet (no filtering — always full) |
| `pkg/services/dag/table_feature_extraction_node.go` | Accept ChangeSet, filter to affected tables |
| `pkg/services/dag/relationship_discovery_node.go` | Accept ChangeSet, filter candidate pairs |
| `pkg/services/dag/column_enrichment_node.go` | Accept ChangeSet, filter to changed columns |
| `pkg/services/dag/ontology_finalization_node.go` | Accept ChangeSet (always runs) |
| `pkg/services/ontology_dag_service.go` | Modify Start() for incremental flow, update executeNode() |
| `pkg/handlers/ontology_dag_handler.go` | New status endpoint, update responses |
| `migrations/NNN_incremental_ontology.up.sql` | Add `is_incremental`, `change_summary` to DAG table |
| Frontend ontology screen | Status check, button changes, progress display |

## Dependencies

- Schema refresh and `updated_at` triggers must be working correctly (they are — managed by PostgreSQL triggers)
- `scan_data_changes` must update column `updated_at` when it detects new enum values (verify this)
- Questions `content_hash` deduplication must work (it does — existing implementation)

## Out of Scope

- Glossary incremental refresh (GlossaryDiscovery/GlossaryEnrichment nodes) — handled separately per PLAN-glossary-after-questions.md
- Scheduled/automatic refresh — this plan covers manual "Refresh Ontology" only
- MCP `update_ontology` tool — separate enhancement

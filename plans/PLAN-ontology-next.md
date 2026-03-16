# PLAN: Ontology System - Next Phase

**Date:** 2026-01-25 (updated 2026-03-16)
**Status:** In Progress

## Overview

This plan is now partially implemented.

Completed since the original version:
- glossary SQL validation (BUG-10)
- `create_glossary_term` MCP tool
- relationship API now includes source/target column types
- incremental ontology extraction now exists in the DAG at a coarse level (`is_incremental`, `change_summary`, `ComputeChangeSet`, cleanup of deleted items, and affected-table scoping for some stages)

Still remaining:
- verify the distinct-count fix against a fresh extraction
- finish the incremental-refresh design so user-curated ontology data is reliably preserved during re-extraction

## Current State

### 1. Stats Collection Bug (BUG-9) - IMPLEMENTED BUT NOT VERIFIED

**Original problem:** 32.8% of joinable columns had NULL `distinct_count` despite `row_count` and `non_null_count` being populated correctly. Primarily affected `text` (60% null) and `integer` (50% null) columns.

**What is done now:**
- stats refresh/writeback lives in `pkg/services/column_feature_extraction.go`
- datasource adapters populate `distinct_count`
- repository persistence for column stats is in place
- adapter tests now assert that populated columns do not lose `distinct_count`

**What is still missing:** a fresh extraction run that proves the production metadata path now meets the target.

**Current verification status:** not yet verified against a real extracted project. The shared local metadata store currently does not have a completed ontology DAG run or any joinable columns for the shared test project, so this criterion cannot be closed from current local state alone.

**To verify:** run a fresh ontology extraction and check:
```sql
SELECT COUNT(*) AS total,
       COUNT(distinct_count) AS has_distinct,
       ROUND(100.0 * COUNT(distinct_count) / COUNT(*), 1) AS pct
FROM engine_schema_columns
WHERE is_joinable = true AND project_id = '<project-id>';
```

### 2. Ontology Refresh (Incremental) - PARTIALLY IMPLEMENTED

**Original problem:** "Re-extract Ontology" did a full wipe and rebuild, losing all user corrections and MCP feedback.

**What exists today:**
- provenance infrastructure in `pkg/models/provenance.go` (`SourceInferred`, `SourceMCP`, `SourceManual`)
- provenance fields on `engine_ontology_column_metadata` and `engine_ontology_table_metadata`
- incremental DAG tracking in `engine_ontology_dag` via `is_incremental` and `change_summary`
- `ComputeChangeSet(...)` and `CleanupDeletedItems(...)`
- no-op refresh behavior when no schema changes are detected
- some selective DAG behavior:
  - changed-table scoping for column enrichment
  - node-level skipping when no relevant table/column changes exist

**What is still missing or incomplete:**
1. Event queue table for schema changes, user answers, MCP feedback
2. Schema fingerprint computation/use as the primary refresh contract
3. Fine-grained selective enrichment that actually uses user-curated skip decisions (`ShouldSkipColumn` / `ShouldSkipTable`) during runtime re-extraction
4. MCP `update_ontology` tool for ad-hoc corrections

**Important nuance:** incremental extraction exists, but the full success criterion for "refresh without losing user corrections" should not be considered closed until item-level user-edit preservation is wired through the active runtime paths, not just computed in `ChangeSet`.

### 3. Column Types in Entity Relationships - COMPLETED

**Original problem:** relationship responses did not include denormalized source/target column types.

**What is done now:**
- `RelationshipDetail` includes `source_column_type` and `target_column_type`
- repository queries populate those fields by joining through `engine_schema_columns`
- handler/UI types already consume those fields

This item is complete.

## Remaining Work

| Priority | Task | Status | Notes |
|----------|------|--------|-------|
| 1 | Verify BUG-9 stats collection fix | REMAINING | Needs fresh extraction evidence, not just unit/integration tests |
| 2 | Finish incremental refresh design | REMAINING | Incremental DAG exists, but event-queue/fingerprint/item-level skip work is still incomplete |
| 3 | Column types in relationship responses | DONE | Implemented in model and repository query path |

## Success Criteria

- [ ] Stats collection: <10% joinable columns with NULL `distinct_count` after fresh extraction
- [ ] Refresh: Can refresh ontology without losing user corrections
- [x] Relationship API includes column types without extra lookups

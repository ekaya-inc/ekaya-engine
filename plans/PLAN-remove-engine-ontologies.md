# PLAN: Remove `engine_ontologies` Table — Distribute Data to Relational Tables

**Status:** TODO
**Created:** 2026-03-01

## Context

The `engine_ontologies` table stores all ontology data in a single row per project with monolithic JSONB blobs:
- `column_details` (up to 130KB per project, growing with schema size) — all column-level semantic data serialized as one JSON document
- `domain_summary` (~600-900 bytes) — project-level domain context
- `metadata` — currently always `{}`

Every `UpdateColumnDetails` call deserializes, mutates, and reserializes the entire blob. This creates scaling issues and data staleness risks because the same data also lives in relational tables (`engine_ontology_column_metadata`) that were introduced as a replacement but never fully migrated to.

The relational replacement tables **already exist**:
- `engine_ontology_column_metadata` — per-column metadata with provenance tracking
- `engine_ontology_table_metadata` — per-table metadata with provenance tracking
- `engine_project_knowledge` — domain-level facts

This plan completes the migration by making `engine_ontology_column_metadata` the sole source of truth, moving `domain_summary` to `engine_projects`, and dropping `engine_ontologies` entirely.

**No backward compatibility or data migration needed — no users yet.**

## Files Affected

### Delete Entirely
| File | Reason |
|------|--------|
| `pkg/repositories/ontology_repository.go` | Entire repo being removed |
| `pkg/repositories/ontology_repository_test.go` | Tests for removed repo |
| `pkg/mcp/tools/ontology_helpers.go` | `ensureOntologyExists()` no longer needed |
| `pkg/mcp/tools/ontology_helpers_test.go` | Tests for removed helpers |

### Modify (Heavy)
| File | Change |
|------|--------|
| `main.go` | Remove `ontologyRepo` creation and ~20 injection sites |
| `pkg/services/ontology_context.go` | Rewrite 3 methods to read from relational tables |
| `pkg/services/ontology_context_test.go` | Rewrite tests |
| `pkg/services/column_enrichment.go` | Remove ontology blob writes, write to ColumnMetadata |
| `pkg/services/column_enrichment_test.go` | Update tests |
| `pkg/services/ontology_finalization.go` | Write DomainSummary to project, not ontology |
| `pkg/services/ontology_finalization_test.go` | Update tests |
| `pkg/services/glossary_service.go` | Replace 5+ `GetActive()` calls with ColumnMetadata reads |
| `pkg/services/glossary_service_test.go` | Update tests |
| `pkg/mcp/tools/column.go` | Remove ontology blob reads/writes, keep ColumnMetadata path |
| `pkg/mcp/tools/ontology_batch.go` | Remove ontology blob reads/writes |
| `pkg/mcp/tools/context.go` | Remove ontology blob reads, single code path |
| `pkg/mcp/tools/ontology.go` | Remove ontology blob reads |
| `pkg/mcp/tools/probe.go` | Remove ontology blob reads, use ColumnMetadata only |

### Modify (Light)
| File | Change |
|------|--------|
| `pkg/models/ontology.go` | Remove `TieredOntology`, `ColumnDetail` structs; keep `DomainSummary`, `EnumValue`, constants |
| `pkg/models/ontology_test.go` | Remove tests for deleted structs |
| `pkg/models/column_metadata.go` | Add `Synonyms`, `FKAssociation` fields |
| `pkg/models/column_features.go` | Add `FKAssociation` to `IdentifierFeatures`; add `Synonyms` field |
| `pkg/models/project.go` | Add `DomainSummary` field |
| `pkg/models/ontology_question.go` | Remove `OntologyID` field |
| `pkg/models/ontology_chat.go` | Remove `OntologyID` field if present |
| `pkg/models/glossary.go` | Remove `OntologyID` field |
| `pkg/repositories/project_repository.go` | Add `UpdateDomainSummary()` method |
| `pkg/repositories/ontology_question_repository.go` | Remove `ontology_id` from queries |
| `pkg/services/ontology_question.go` | Remove ontology blob reads/writes |
| `pkg/services/ontology_chat.go` | Read DomainSummary from project |
| `pkg/services/ontology_dag_service.go` | Remove `getOrCreateOntology()`, remove ontology repo |
| `pkg/services/data_change_detection.go` | Read enum values from ColumnMetadata |
| `pkg/services/column_feature_extraction.go` | Remove ontology ID references |
| `pkg/llm/tool_executor.go` | Remove ontology blob reads/writes |
| `pkg/handlers/mocks_test.go` | Remove ontology repo mocks |

## Implementation

### Task 1: Database Migration

Create `migrations/011_remove_ontologies.up.sql`:

- [ ] Add `domain_summary JSONB` column to `engine_projects`
- [ ] Drop `ontology_id` column from `engine_ontology_questions` (drop FK, drop column, update unique constraints)
- [ ] Drop `ontology_id` column from `engine_business_glossary` (update unique constraint from `(project_id, ontology_id, term)` to `(project_id, term)`)
- [ ] Drop `ontology_id` from any other tables that FK to `engine_ontologies` (check `engine_ontology_dag`, chat messages)
- [ ] Drop `engine_ontologies` table (with CASCADE)

### Task 2: Add Missing Fields to ColumnMetadata

Add fields to `ColumnMetadata` that exist in `ColumnDetail` but not yet in `ColumnMetadata`:

- [ ] Add `Synonyms []string` to `ColumnMetadataFeatures` JSONB (in `pkg/models/column_metadata.go`)
- [ ] Add `FKAssociation string` to `IdentifierFeatures` in `pkg/models/column_features.go`
- [ ] No database migration needed — both go into existing `features` JSONB column

### Task 3: Add DomainSummary to Project Model + Repository

- [ ] Add `DomainSummary *DomainSummary` field to Project struct in `pkg/models/project.go`
- [ ] Add `UpdateDomainSummary(ctx, projectID, *DomainSummary) error` to ProjectRepository interface
- [ ] Implement the method — UPDATE `engine_projects` SET `domain_summary` = $2
- [ ] Add `GetDomainSummary` or ensure existing `GetByID` scans the new column
- [ ] Update `pkg/services/projects.go` CreateProject to initialize empty DomainSummary

### Task 4: Refactor OntologyContextService

The core of the refactor — 3 methods that build all ontology context for MCP tools.

`pkg/services/ontology_context.go`:

- [ ] Remove `ontologyRepo` from constructor, add `columnMetadataRepo`, `tableMetadataRepo`
- [ ] `GetDomainContext()`: Read `project.DomainSummary` instead of `ontology.DomainSummary`. Get table/column counts from schema repo.
- [ ] `GetTablesContext()`: Build column overviews from `ColumnMetadata` (via `columnMetadataRepo.GetBySchemaColumnIDs()`) instead of `ontology.ColumnDetails`. Get table descriptions from `TableMetadata`.
- [ ] `GetColumnsContext()`: Build column details from `ColumnMetadata` + `SchemaColumn` instead of `ontology.ColumnDetails`. Create a new response type to replace `[]ColumnDetail`.
- [ ] Update `pkg/services/ontology_context_test.go` and integration tests

### Task 5: Refactor Column Enrichment Service

`pkg/services/column_enrichment.go`:

- [ ] Remove `ontologyRepo` from constructor
- [ ] Replace `convertToColumnDetails()` + `ontologyRepo.UpdateColumnDetails()` (line 302) with writing to `columnMetadataRepo.Upsert()` for each column
- [ ] Map enrichment results (Description, Role, SemanticType, EnumValues, Synonyms, FKAssociation) to ColumnMetadata fields
- [ ] Remove `convertToColumnDetails()` method entirely (line 1169+)
- [ ] Update tests

### Task 6: Refactor Ontology Finalization Service

`pkg/services/ontology_finalization.go`:

- [ ] Remove `ontologyRepo` from constructor, add project repo
- [ ] Line 60: Remove `ontologyRepo.GetActive()`. Use schema repo to check if tables exist.
- [ ] Line 146: Replace `ontologyRepo.UpdateDomainSummary()` with `projectRepo.UpdateDomainSummary()`
- [ ] The `extractColumnFeatureInsights()` method (line 398) already reads from `columnMetadataRepo` — no changes needed there
- [ ] Update tests

### Task 7: Refactor MCP Tools

**`pkg/mcp/tools/column.go` (update_column, get_column_metadata, delete_column_metadata):**
- [ ] Remove `OntologyRepo` from deps
- [ ] `update_column`: Remove the entire ontology blob read-modify-write block. Keep only `ColumnMetadataRepo.Upsert()` path.
- [ ] `get_column_metadata`: Remove the "Fallback: check ontology JSONB" block. Only use `ColumnMetadataRepo`.
- [ ] `delete_column_metadata`: Remove ontology blob deletion. Only delete from `ColumnMetadataRepo`.

**`pkg/mcp/tools/ontology_batch.go`:**
- [ ] Remove all ontology blob reads/writes. Only write via `ColumnMetadataRepo`.

**`pkg/mcp/tools/probe.go`:**
- [ ] Remove ontology blob reads. `ColumnMetadataRepo` path (already exists) becomes the only path.

**`pkg/mcp/tools/ontology.go` (get_ontology):**
- [ ] Remove `OntologyRepo` from deps. Delegates to `OntologyContextService` (refactored in Task 4).

**`pkg/mcp/tools/context.go` (get_context):**
- [ ] Remove `OntologyRepo` from deps and the `GetActive()` call.
- [ ] Remove the `handleContextWithOntology`/`handleContextWithoutOntology` split — single code path.
- [ ] Remove `determineOntologyStatus()` — replace with a check on whether column metadata exists.

**`pkg/mcp/tools/ontology_helpers.go`:**
- [ ] Delete entirely. `ensureOntologyExists()` is no longer needed.

### Task 8: Refactor Remaining Services

**`pkg/services/glossary_service.go`** (heaviest caller — 5+ GetActive calls):
- [ ] Remove `ontologyRepo` from constructor
- [ ] Replace all `ontologyRepo.GetActive()` + `ontology.ColumnDetails` reads with `columnMetadataRepo.GetBySchemaColumnIDs()` or `GetByProject()`
- [ ] Remove `ontology.ID` references for glossary term creation
- [ ] Update LLM prompt building that iterates `ontology.ColumnDetails` to iterate ColumnMetadata instead

**`pkg/services/data_change_detection.go`:**
- [ ] Remove `ontologyRepo` from constructor
- [ ] Line 134: Replace `ontologyRepo.GetActive()` + reading `ontology.ColumnDetails[tableName]` for existing enum values with reading from `ColumnMetadataRepo` (`EnumFeatures.Values`)

**`pkg/services/ontology_question.go`:**
- [ ] Remove `ontologyRepo` from constructor
- [ ] Remove `applyColumnUpdates()` method (lines 244-336) which writes to ontology blob. Replace with `ColumnMetadataRepo.Upsert()`.
- [ ] Remove `OntologyID` from question creation

**`pkg/services/ontology_chat.go`:**
- [ ] Remove `ontologyRepo` from constructor
- [ ] Replace `ontology.DomainSummary` reads with `project.DomainSummary`
- [ ] Replace `ontology.TableCount()` with schema table count

**`pkg/services/ontology_dag_service.go`:**
- [ ] Remove `ontologyRepo` from constructor
- [ ] Remove `getOrCreateOntology()` method entirely
- [ ] Remove `OntologyID` from DAG record creation
- [ ] In `Delete()`: Remove `ontologyRepo.DeleteByProject()` call

**`pkg/services/column_feature_extraction.go`:**
- [ ] Remove ontology ID references in question creation

**`pkg/llm/tool_executor.go`:**
- [ ] Remove `ontologyRepo` from struct
- [ ] Replace `ontologyRepo.GetActive()` + `ontology.ColumnDetails` read/write with `ColumnMetadataRepo` operations

### Task 9: Remove OntologyID from Models

- [ ] `pkg/models/ontology_question.go` — Remove `OntologyID` field
- [ ] `pkg/models/glossary.go` — Remove `OntologyID` field
- [ ] `pkg/models/ontology_dag.go` — Remove `OntologyID` field if present
- [ ] Update all repository files that read/write `ontology_id` column
- [ ] Update unique constraints in repository queries

### Task 10: Remove TieredOntology and ColumnDetail

- [ ] `pkg/models/ontology.go` — Remove `TieredOntology` struct, `ColumnDetail` struct, helper methods (`GetColumnDetails`, `TableCount`, `ColumnCount`, `TotalEntityCount`). **Keep:** `DomainSummary`, `ProjectConventions`, `RelationshipEdge`, `EnumValue`, `DomainContext`, `EntityHint`, constants.
- [ ] Remove `pkg/models/ontology_test.go` tests for deleted structs

### Task 11: Remove OntologyRepository

- [ ] Delete `pkg/repositories/ontology_repository.go`
- [ ] Delete `pkg/repositories/ontology_repository_test.go`
- [ ] `main.go` — Remove `ontologyRepo` creation and all ~20 injection sites
- [ ] Remove all mock implementations of `OntologyRepository` across test files

### Task 12: Update Tests and Final Cleanup

- [ ] Update all mock files (`pkg/handlers/mocks_test.go`, `pkg/mcp/tools/mocks_test.go`, etc.)
- [ ] Update integration tests (`ontology_context_integration_test.go`, `schema_integration_test.go`, etc.)
- [ ] Run full test suite: `go test ./...`
- [ ] Run `go vet ./...` and ensure clean compilation

## Verification

1. `go test ./...` — all tests pass
2. Start the engine and connect to ekaya_marketing project
3. MCP tools verification:
   - `get_context` at all depth levels (domain, entities, tables, columns) returns correct data
   - `get_ontology` at all depth levels returns correct data
   - `update_column` writes to column metadata and is readable
   - `probe_column` returns semantic info from column metadata
   - `refresh_schema` + `scan_data_changes` work without ontology
4. Verify `engine_ontologies` table no longer exists in database
5. Verify `engine_projects` has `domain_summary` column
6. Verify `engine_ontology_questions` no longer has `ontology_id` column

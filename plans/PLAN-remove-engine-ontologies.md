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

- [x] Add `domain_summary JSONB` column to `engine_projects`
- [x] Drop `ontology_id` column from `engine_ontology_questions` (drop FK, drop column, update unique constraints)
- [x] Drop `ontology_id` column from `engine_business_glossary` (update unique constraint from `(project_id, ontology_id, term)` to `(project_id, term)`)
- [x] Drop `ontology_id` from any other tables that FK to `engine_ontologies` (check `engine_ontology_dag`, chat messages)
- [x] Drop `engine_ontologies` table (with CASCADE)

### Task 2: Add Missing Fields to ColumnMetadata

Add fields to `ColumnMetadata` that exist in `ColumnDetail` but not yet in `ColumnMetadata`:

- [x] Add `Synonyms []string` to `ColumnMetadataFeatures` JSONB (in `pkg/models/column_metadata.go`)
- [x] Add `FKAssociation string` to `IdentifierFeatures` in `pkg/models/column_features.go`
- [x] No database migration needed — both fields go into existing `features` JSONB column

### Task 3: Add DomainSummary to Project Model + Repository

- [x] Add `DomainSummary *DomainSummary` field to Project struct in `pkg/models/project.go`
- [x] Add `UpdateDomainSummary(ctx, projectID, *DomainSummary) error` to ProjectRepository interface
- [x] Implement the method — UPDATE `engine_projects` SET `domain_summary` = $2
- [x] Add `GetDomainSummary` or ensure existing `GetByID` scans the new column
- [x] Update `pkg/services/projects.go` CreateProject to initialize empty DomainSummary

### Task 4: Refactor OntologyContextService

The core of the refactor — 3 methods that build all ontology context for MCP tools.

`pkg/services/ontology_context.go`:

- [x] Remove `ontologyRepo` from constructor, add `columnMetadataRepo`, `tableMetadataRepo`
- [x] `GetDomainContext()`: Read `project.DomainSummary` instead of `ontology.DomainSummary`. Get table/column counts from schema repo.
- [x] `GetTablesContext()`: Build column overviews from `ColumnMetadata` (via `columnMetadataRepo.GetBySchemaColumnIDs()`) instead of `ontology.ColumnDetails`. Get table descriptions from `TableMetadata`.
- [x] `GetColumnsContext()`: Build column details from `ColumnMetadata` + `SchemaColumn` instead of `ontology.ColumnDetails`. Create a new response type to replace `[]ColumnDetail`.
- [x] Update `pkg/services/ontology_context_test.go` and integration tests

### Task 5: Refactor Column Enrichment Service

`pkg/services/column_enrichment.go`:

- [x] Remove `ontologyRepo` from constructor
- [x] Replace `convertToColumnDetails()` + `ontologyRepo.UpdateColumnDetails()` (line 302) with writing to `columnMetadataRepo.Upsert()` for each column
- [x] Map enrichment results (Description, Role, SemanticType, EnumValues, Synonyms, FKAssociation) to ColumnMetadata fields
- [x] Remove `convertToColumnDetails()` method entirely (line 1169+)
- [x] Update tests

### Task 6: Refactor Ontology Finalization Service

`pkg/services/ontology_finalization.go`:

- [x] Remove `ontologyRepo` from constructor, add project repo
- [x] Line 60: Remove `ontologyRepo.GetActive()`. Use schema repo to check if tables exist.
- [x] Line 146: Replace `ontologyRepo.UpdateDomainSummary()` with `projectRepo.UpdateDomainSummary()`
- [x] The `extractColumnFeatureInsights()` method (line 398) already reads from `columnMetadataRepo` — no changes needed there
- [x] Update tests

### Task 7: Refactor MCP Tools

**`pkg/mcp/tools/column.go` (update_column, get_column_metadata, delete_column_metadata):**
- [x] Remove `OntologyRepo` from deps
- [x] `update_column`: Remove the entire ontology blob read-modify-write block. Keep only `ColumnMetadataRepo.Upsert()` path.
- [x] `get_column_metadata`: Remove the "Fallback: check ontology JSONB" block. Only use `ColumnMetadataRepo`.
- [x] `delete_column_metadata`: Remove ontology blob deletion. Only delete from `ColumnMetadataRepo`.

**`pkg/mcp/tools/ontology_batch.go`:**
- [x] Remove all ontology blob reads/writes. Only write via `ColumnMetadataRepo`.

**`pkg/mcp/tools/probe.go`:**
- [x] Remove ontology blob reads. `ColumnMetadataRepo` path (already exists) becomes the only path.

**`pkg/mcp/tools/ontology.go` (get_ontology):**
- [x] Remove `OntologyRepo` from deps. Delegates to `OntologyContextService` (refactored in Task 4).

**`pkg/mcp/tools/context.go` (get_context):**
- [x] Remove `OntologyRepo` from deps and the `GetActive()` call.
- [x] Remove the `handleContextWithOntology`/`handleContextWithoutOntology` split — single code path.
- [x] Remove `determineOntologyStatus()` — replace with a check on whether column metadata exists.

**`pkg/mcp/tools/ontology_helpers.go`:**
- [x] Delete entirely. `ensureOntologyExists()` is no longer needed.

### Task 8: Refactor Remaining Services

**`pkg/services/glossary_service.go`** (heaviest caller — 5+ GetActive calls):
- [x] Remove `ontologyRepo` from constructor
- [x] Replace all `ontologyRepo.GetActive()` + `ontology.ColumnDetails` reads with `columnMetadataRepo.GetBySchemaColumnIDs()` or `GetByProject()`
- [x] Remove `ontology.ID` references for glossary term creation
- [x] Update LLM prompt building that iterates `ontology.ColumnDetails` to iterate ColumnMetadata instead

**`pkg/services/data_change_detection.go`:**
- [x] Remove `ontologyRepo` from constructor
- [x] Line 134: Replace `ontologyRepo.GetActive()` + reading `ontology.ColumnDetails[tableName]` for existing enum values with reading from `ColumnMetadataRepo` (`EnumFeatures.Values`)

**`pkg/services/ontology_question.go`:**
- [x] Remove `ontologyRepo` from constructor
- [x] Remove `applyColumnUpdates()` method (lines 244-336) which writes to ontology blob. Replace with `ColumnMetadataRepo.Upsert()`.
- [x] Remove `OntologyID` from question creation

**`pkg/services/ontology_chat.go`:**
- [x] Remove `ontologyRepo` from constructor
- [x] Replace `ontology.DomainSummary` reads with `project.DomainSummary`
- [x] Replace `ontology.TableCount()` with schema table count

**`pkg/services/ontology_dag_service.go`:**
- [x] Remove `ontologyRepo` from constructor
- [x] Remove `getOrCreateOntology()` method entirely
- [x] Remove `OntologyID` from DAG record creation
- [x] In `Delete()`: Remove `ontologyRepo.DeleteByProject()` call

**`pkg/services/column_feature_extraction.go`:**
- [x] Remove ontology ID references in question creation

**`pkg/llm/tool_executor.go`:**
- [x] Remove `ontologyRepo` from struct
- [x] Replace `ontologyRepo.GetActive()` + `ontology.ColumnDetails` read/write with `ColumnMetadataRepo` operations

### Task 9: Remove OntologyID from Models

- [x] `pkg/models/ontology_question.go` — Remove `OntologyID` field
- [x] `pkg/models/glossary.go` — Remove `OntologyID` field
- [x] `pkg/models/ontology_dag.go` — Remove `OntologyID` field if present
- [x] `pkg/models/ontology_chat.go` — Remove `OntologyID` field
- [x] Update all repository files that read/write `ontology_id` column
- [x] Update unique constraints in repository queries

### Task 10: Remove TieredOntology and ColumnDetail

- [x] `pkg/models/ontology.go` — Remove `TieredOntology` struct, `ColumnDetail` struct, helper methods (`GetColumnDetails`, `TableCount`, `ColumnCount`, `TotalEntityCount`). **Keep:** `DomainSummary`, `ProjectConventions`, `RelationshipEdge`, `EnumValue`, `DomainContext`, `EntityHint`, constants.
- [x] Remove `pkg/models/ontology_test.go` tests for deleted structs

### Task 11: Remove OntologyRepository and All Remaining References

Remove `ontologyRepo` from all services that still accept/store it. Some have active calls that must be deleted, others are dead dependencies.

**Active calls (must remove logic, not just the dependency):**
- [x] `pkg/services/projects.go` — Remove `createEmptyOntology()` method and its call from `CreateProject()`. Remove `ontologyRepo` from struct/constructor.
- [x] `pkg/services/datasource.go` — Remove `ontologyRepo.DeleteByProject()` call in datasource deletion. Remove `ontologyRepo` from struct/constructor.

**Dead dependencies (stored but never called — remove from struct/constructor):**
- [x] `pkg/services/schema.go` — Remove `ontologyRepo` from struct/constructor
- [x] `pkg/services/knowledge.go` — Remove `ontologyRepo` from struct/constructor
- [x] `pkg/services/deterministic_relationship_service.go` — Remove `ontologyRepo` from struct/constructor
- [x] `pkg/services/change_review_service.go` — Remove `ontologyRepo` from struct and `ChangeReviewServiceDeps`
- [x] `pkg/services/incremental_dag_service.go` — Remove `ontologyRepo` from struct/constructor
- [x] `pkg/mcp/tools/knowledge.go` — Remove `OntologyRepository` from `KnowledgeToolDeps`

**Delete repository and wiring:**
- [x] Delete `pkg/repositories/ontology_repository.go`
- [x] Delete `pkg/repositories/ontology_repository_test.go`
- [x] `main.go` — Remove `ontologyRepo` creation and all injection sites
- [x] Remove all mock implementations of `OntologyRepository` across test files

**Stale comments:**
- [x] `pkg/services/ontology_chat.go:160` — Fix comment "Get DAG first to get ontologyID for all messages"

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

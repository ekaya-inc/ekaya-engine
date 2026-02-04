# PLAN: Remove Entity Concept from v1.0

> **🟢 DVX ESCALATION BYPASS: LARGE DELETIONS AUTHORIZED**
>
> This plan is intentionally deletion-heavy. DVX's safeguard that blocks tasks with high deletion ratios is sound for most plans, but this plan's entire purpose is to remove the Entity concept from the codebase. For this plan only, the large-deletion escalation can be bypassed—these deletions are intentional and authorized.
>
> — Damon Danieli (author of dvx and this project)

> **⚠️ IMPORTANT: BREAKING MIGRATION ON SEPARATE BRANCH ⚠️**
>
> This work is being done on a separate branch (`dvx/remove-entity`). Migration 021 will **break the existing schema** and is incompatible with other branches.
>
> **Do not worry about testing.** Make rough changes and the user will test when merging back to the main development branch.

## Context

The Entity concept (domain entities like "User", "Account", "Order" discovered from schema analysis) is being deferred to post-v1.0. This includes:
- **Entities** - Domain concepts discovered from tables (e.g., User, Account, Order)
- **Entity Relationships** - Connections between entities derived from FK constraints or PK matching
- **Entity Occurrences** - Where entities appear across schema columns with role semantics (visitor, host, etc.)
- **Entity Aliases** - Alternative names for entities (for query matching)
- **Entity Key Columns** - Important business columns per entity

The v1.0 focus is on schema-based column and table enrichment without the entity abstraction layer.

## Project Assumptions

This is a greenfield project with no users, no backward compatibility requirements, and the database can be dropped/recreated at will. No data migrations needed.

---

## Phase 1: Database Migration

Create a migration to drop all entity-related tables and columns.

### 1.1 Create migration file

- [x] Created as `migrations/022_remove_entity_concept.up.sql` (021 was taken)

Create `migrations/021_remove_entity_concept.up.sql`:

```sql
-- 021_remove_entity_concept.up.sql
-- Remove all entity-related tables for v1.0 launch

-- Drop tables in dependency order (children first)
DROP TABLE IF EXISTS engine_ontology_entity_key_columns CASCADE;
DROP TABLE IF EXISTS engine_ontology_entity_aliases CASCADE;
DROP TABLE IF EXISTS engine_entity_relationships CASCADE;
DROP TABLE IF EXISTS engine_ontology_entities CASCADE;

-- Remove entity_summaries column from engine_ontologies
ALTER TABLE engine_ontologies DROP COLUMN IF EXISTS entity_summaries;
```

Create `migrations/021_remove_entity_concept.down.sql`:

```sql
-- 021_remove_entity_concept.down.sql
-- This is a one-way migration - entity concept is removed
-- Re-add would require running 005_ontology_core.up.sql entity portions

-- Add entity_summaries column back
ALTER TABLE engine_ontologies ADD COLUMN IF NOT EXISTS entity_summaries jsonb;

-- Note: Tables would need to be recreated from 005_ontology_core.up.sql
-- This down migration intentionally does NOT recreate them
```

### 1.2 Update 019_entity_promotion migration

Delete these files (no longer needed):
- [x] `migrations/019_entity_promotion.up.sql`
- [x] `migrations/019_entity_promotion.down.sql`

---

## Phase 2: Delete Entity Models

### 2.1 Delete model files

- [x] `pkg/models/ontology_entity.go` - Contains `OntologyEntity`, `EntityAlias`, `EntityKeyColumn`, `EntityOccurrence`
- [x] `pkg/models/entity_relationship.go` - Contains `EntityRelationship` struct

### 2.2 Update ontology.go

Remove from `pkg/models/ontology.go`:
- [x] `EntitySummary` struct (lines ~69-79)
- [x] `EntitySummaries` field from `Ontology` struct
- [x] `RelationshipEdge` struct (only if not used elsewhere - check first) - KEPT: Used for table-to-table FK relationships

### 2.3 Update ontology_context.go

Remove from `pkg/models/ontology_context.go`:
- [x] `EntityBrief` struct
- [x] `EntityDetail` struct
- [x] `EntityOccurrence` struct
- [x] `KeyColumnInfo` struct
- [x] `OntologyEntityRelationship` struct
- [x] `OntologyEntitiesContext` struct
- [x] `OntologyDomainContext.Entities` field
- [x] `OntologyDomainContext.Relationships` field (if only used for entity relationships)
- [x] `ColumnOverview.Entity` field
- [x] `ColumnOverview.EntityAssociation` field

### 2.4 Update ontology_dag.go

Remove from `pkg/models/ontology_dag.go`:
- [x] `DAGNodeEntityDiscovery` constant
- [x] `DAGNodeEntityEnrichment` constant
- [x] `DAGNodeEntityPromotion` constant
- [x] `DAGNodeRelationshipEnrichment` constant (if only for entity relationships)
- [x] Remove these from `DAGNodeOrder` map
- [x] Remove these from `AllDAGNodes()` function

---

## Phase 3: Delete Entity Repositories

### 3.1 Delete repository files

- [x] `pkg/repositories/ontology_entity_repository.go`
- [x] `pkg/repositories/ontology_entity_repository_test.go`
- [x] `pkg/repositories/entity_relationship_repository.go`
- [x] `pkg/repositories/entity_relationship_repository_test.go`
- [x] `pkg/repositories/entity_relationship_migration_test.go`
- [x] `pkg/repositories/drop_occurrences_migration_test.go` (if entity-specific)
- [x] `pkg/repositories/reverse_relationships_migration_test.go` (if entity-specific)

---

## Phase 4: Delete Entity Services

### 4.1 Delete entity service files

Delete all entity-related service files from `pkg/services/`:
- [x] `pkg/services/entity_service.go`
- [x] `pkg/services/entity_service_test.go`
- [x] `pkg/services/entity_discovery_service.go`
- [x] `pkg/services/entity_discovery_service_test.go`
- [x] `pkg/services/entity_discovery_task.go`
- [x] `pkg/services/entity_discovery_task_test.go`
- [x] `pkg/services/entity_merge_service.go`
- [x] `pkg/services/entity_promotion.go`
- [x] `pkg/services/entity_promotion_test.go`
- [x] `pkg/services/entity_promotion_service.go`
- [x] `pkg/services/entity_promotion_service_test.go`
- [x] `pkg/services/entity_promotion_integration_test.go`

These files contain the entity discovery, promotion, and merge logic that is being removed for v1.0. Simply delete these files - they have no remaining dependencies after the earlier phases removed the entity models and repositories.

### 4.2 Remove entity references from ontology_context.go and its tests

Update `pkg/services/ontology_context.go` to remove all entity-related code:
- [x] Remove any imports related to entity types
- [x] Remove entity fetching/formatting logic
- [x] Remove any methods that build entity context
- [x] Remove entity-related fields from service structs
- [x] Remove entity repository dependencies

Also update the corresponding test files:
- [x] `pkg/services/ontology_context_test.go`
- [x] `pkg/services/ontology_context_integration_test.go`

The OntologyContextService builds context for LLM prompts. After this change, it should only handle table/column metadata without the entity abstraction layer.

### 4.3 Remove entity references from ontology_finalization.go and ontology_dag_service.go

- [x] Update `pkg/services/ontology_finalization.go`:
  - Remove entity summary building logic
  - Remove any entity-related imports
  - The finalization service should no longer write entity_summaries to the ontology

- [x] Update `pkg/services/ontology_finalization_test.go` to remove entity-related test cases.

- [x] Update `pkg/services/ontology_dag_service.go`:
  - Remove entity node wiring (SetEntityDiscoveryMethods, SetEntityEnrichmentMethods, SetEntityPromotionMethods, etc.)
  - Remove entity-related DAG node creation
  - The DAG service orchestrates the ontology pipeline; it should no longer include entity discovery, enrichment, or promotion nodes

- [x] Update `pkg/services/ontology_dag_service_test.go` to remove entity-related test cases.

### 4.4 Remove entity references from relationship_enrichment.go and column_enrichment.go

- [x] Deleted `pkg/services/relationship_enrichment.go` and test - only handled entity relationships
- [x] Deleted `pkg/services/dag/relationship_enrichment_node.go` - DAG node for entity relationships
- [x] Updated `pkg/services/column_enrichment.go` - removed entity column references
- [x] Updated `pkg/services/column_enrichment_test.go` accordingly

### 4.5 Remove entity references from schema.go and projects.go

Update `pkg/services/schema.go`:
- [ ] Search for and remove any entity references
- [ ] This service handles schema discovery and metadata; it should not reference entity types

Update `pkg/services/schema_test.go` accordingly.

Update `pkg/services/projects.go`:
- [ ] Remove entity deletion in project cleanup logic
- [ ] When a project is deleted, the code currently deletes associated entities; remove this cleanup since entity tables no longer exist

Update `pkg/services/projects_test.go` accordingly.

---

## Phase 5: Delete Entity DAG Nodes

### 5.1 Delete DAG node files

- [x] `pkg/services/dag/entity_discovery_node.go`
- [x] `pkg/services/dag/entity_enrichment_node.go`
- [x] `pkg/services/dag/entity_enrichment_node_test.go`
- [x] `pkg/services/dag/entity_promotion_node.go`
- [x] `pkg/services/dag/entity_promotion_node_test.go`

### 5.2 Update remaining DAG nodes

Review and remove entity references from:
- [x] `pkg/services/dag/relationship_enrichment_node.go` - May need significant changes or deletion
- [x] `pkg/services/dag/column_enrichment_node.go` - Remove entity column handling
- [x] `pkg/services/dag/knowledge_seeding_node.go` - Remove entity references if any
- [x] `pkg/services/dag/column_feature_extraction_node.go` - Remove entity references if any

---

## Phase 6: Delete Entity Handlers

### 6.1 Delete handler files

- [x] `pkg/handlers/entity_handler.go`
- [x] `pkg/handlers/entity_integration_test.go`
- [x] `pkg/handlers/entity_relationship_handler.go`
- [x] `pkg/handlers/entity_relationship_handler_test.go`

### 6.2 Update remaining handlers

Review and remove entity references from:
- [x] `pkg/handlers/ontology_enrichment_handler.go` - Remove entity-related endpoints/logic
- [x] `pkg/handlers/params.go` - Remove entity params if any

---

## Phase 7: Delete Entity MCP Tools

### 7.1 Delete MCP tool files

- [ ] `pkg/mcp/tools/entity.go` - Contains `update_entity`, `get_entity`, `delete_entity`
- [ ] `pkg/mcp/tools/entity_test.go`
- [ ] `pkg/mcp/tools/entity_integration_test.go`

### 7.2 Update remaining MCP tools

Review and remove entity references from:
- [ ] `pkg/mcp/tools/context.go` - Remove entity context building
- [ ] `pkg/mcp/tools/context_test.go`
- [ ] `pkg/mcp/tools/ontology.go` - Remove entity-related tools
- [ ] `pkg/mcp/tools/ontology_test.go`
- [ ] `pkg/mcp/tools/ontology_helpers.go` - Remove entity helpers
- [ ] `pkg/mcp/tools/ontology_helpers_test.go`
- [ ] `pkg/mcp/tools/ontology_batch.go` - Remove entity batch operations
- [ ] `pkg/mcp/tools/ontology_batch_test.go`
- [ ] `pkg/mcp/tools/probe.go` - Remove entity probing
- [ ] `pkg/mcp/tools/probe_test.go`
- [ ] `pkg/mcp/tools/probe_relationship_integration_test.go`
- [ ] `pkg/mcp/tools/relationship.go` - Remove entity relationship tools
- [ ] `pkg/mcp/tools/relationship_test.go`
- [ ] `pkg/mcp/tools/search.go` - Remove entity search
- [ ] `pkg/mcp/tools/search_test.go`
- [ ] `pkg/mcp/tools/search_integration_test.go`
- [ ] `pkg/mcp/tools/column.go` - Remove entity column references
- [ ] `pkg/mcp/tools/column_test.go`
- [ ] `pkg/mcp/tools/questions.go` - Remove entity question handling
- [ ] `pkg/mcp/tools/questions_test.go`
- [ ] `pkg/mcp/tools/schema.go` - Remove entity schema references

### 7.3 Update MCP tool registry

- [ ] `pkg/services/mcp_tools_registry.go` - Remove entity tool registration
- [ ] `pkg/services/mcp_tools_registry_test.go`
- [ ] `pkg/services/mcp_tool_loadouts.go` - Remove entity tool loadouts
- [ ] `pkg/services/mcp_tool_loadouts_test.go`

---

## Phase 8: Update main.go

Remove entity wiring from `main.go`:
- [ ] Remove `entityRepo` creation
- [ ] Remove `entityRelationshipRepo` creation
- [ ] Remove `entityService` creation
- [ ] Remove `entityDiscoveryService` creation
- [ ] Remove `entityPromotionService` creation
- [ ] Remove `entityHandler` creation and route registration
- [ ] Remove `entityRelationshipHandler` creation and route registration
- [ ] Remove entity-related DAG node wiring (SetEntityDiscoveryMethods, SetEntityEnrichmentMethods, etc.)

---

## Phase 9: Update UI

### 9.1 Delete UI files

- [ ] `ui/src/types/entity.ts`
- [ ] `ui/src/pages/EntitiesPage.tsx`

### 9.2 Update UI files

- [ ] `ui/src/App.tsx` - Remove `EntitiesPage` import and route (`path="entities"`)
- [ ] `ui/src/types/index.ts` - Remove entity type exports
- [ ] `ui/src/types/ontology.ts` - Remove entity references if any
- [ ] `ui/src/services/engineApi.ts` - Remove `listEntities()` method and `EntitiesListResponse` import
- [ ] `ui/src/pages/EnrichmentPage.tsx` - Remove entity references
- [ ] `ui/src/components/ontology/AIAnsweringGuide.tsx` - Remove entity references
- [ ] `ui/src/components/ontology/WorkQueue.tsx` - Remove entity references
- [ ] `ui/src/components/ontology/ChatPane.tsx` - Remove entity references
- [ ] `ui/src/components/ontology/RelationshipsView.tsx` - Remove entity references
- [ ] `ui/src/components/ontology/TablesView.tsx` - Remove entity references
- [ ] `ui/src/components/DatasourceConfiguration.tsx` - Remove entity references if any

### 9.3 Update navigation

- [ ] Remove any "Entities" links from navigation/sidebar components

---

## Phase 10: Delete Test Files

### 10.1 Delete MCP test prompts

- [ ] `tests/claude-mcp/prompts/200-entity-create.md`
- [ ] `tests/claude-mcp/prompts/201-entity-update.md`
- [ ] `tests/claude-mcp/prompts/300-entity-delete.md`
- [ ] `tests/claude-mcp/prompts/111-ontology-entities.md`

### 10.2 Update remaining tests

Review and update tests that reference entities in:
- [ ] `pkg/mcp/tools/mcp_tools_scenario_test.go`
- [ ] `pkg/mcp/tools/mcp_tools_integration_test.go`
- [ ] `pkg/mcp/tools/ontology_performance_test.go`
- [ ] `pkg/handlers/datasources_integration_test.go`
- [ ] `pkg/handlers/glossary_integration_test.go`
- [ ] `pkg/handlers/ontology_dag_handler_test.go`

---

## Phase 11: Update Documentation

### 11.1 Update CLAUDE.md

Remove from `CLAUDE.md`:
- [ ] Entity table references in "Ontology Tables" section
- [ ] Entity tables from TRUNCATE command in "Clear Tables Before Testing"
- [ ] "Monitor Entities and Relationships" section
- [ ] "Check Entity Summaries Written" section
- [ ] Entity references in "What to Watch For" section
- [ ] DAG step references: EntityDiscovery, EntityEnrichment, RelationshipEnrichment, EntityPromotion

### 11.2 Delete outdated plan files

- [ ] `plans/PLAN-add-cardinality-to-engine_entity_relationships-and-ui.md`
- [ ] `plans/PLAN-add-non-entity-tables.md`
- [ ] `plans/PLAN-show-enhanced-columns-in-entities.md`

---

## Phase 12: Final Cleanup

### 12.1 Search for remaining references

Run these searches to find any missed references:
```bash
grep -r "entity" --include="*.go" pkg/
grep -r "Entity" --include="*.go" pkg/
grep -r "entity" --include="*.ts" --include="*.tsx" ui/src/
grep -r "Entity" --include="*.ts" --include="*.tsx" ui/src/
```

### 12.2 Verify compilation

```bash
go build ./...
cd ui && npm run typecheck
```

### 12.3 Run tests

```bash
make check
```

---

## Summary of Files to Delete

### Go files (40+ files)
```
pkg/models/ontology_entity.go
pkg/models/entity_relationship.go
pkg/repositories/ontology_entity_repository.go
pkg/repositories/ontology_entity_repository_test.go
pkg/repositories/entity_relationship_repository.go
pkg/repositories/entity_relationship_repository_test.go
pkg/repositories/entity_relationship_migration_test.go
pkg/services/entity_service.go
pkg/services/entity_service_test.go
pkg/services/entity_discovery_service.go
pkg/services/entity_discovery_service_test.go
pkg/services/entity_discovery_task.go
pkg/services/entity_discovery_task_test.go
pkg/services/entity_merge_service.go
pkg/services/entity_promotion.go
pkg/services/entity_promotion_test.go
pkg/services/entity_promotion_service.go
pkg/services/entity_promotion_service_test.go
pkg/services/entity_promotion_integration_test.go
pkg/services/dag/entity_discovery_node.go
pkg/services/dag/entity_enrichment_node.go
pkg/services/dag/entity_enrichment_node_test.go
pkg/services/dag/entity_promotion_node.go
pkg/services/dag/entity_promotion_node_test.go
pkg/handlers/entity_handler.go
pkg/handlers/entity_integration_test.go
pkg/handlers/entity_relationship_handler.go
pkg/handlers/entity_relationship_handler_test.go
pkg/mcp/tools/entity.go
pkg/mcp/tools/entity_test.go
pkg/mcp/tools/entity_integration_test.go
migrations/019_entity_promotion.up.sql
migrations/019_entity_promotion.down.sql
```

### UI files
```
ui/src/types/entity.ts
ui/src/pages/EntitiesPage.tsx
```

### Test files
```
tests/claude-mcp/prompts/200-entity-create.md
tests/claude-mcp/prompts/201-entity-update.md
tests/claude-mcp/prompts/300-entity-delete.md
tests/claude-mcp/prompts/111-ontology-entities.md
```

### Plan files to delete
```
plans/PLAN-add-cardinality-to-engine_entity_relationships-and-ui.md
plans/PLAN-add-non-entity-tables.md
plans/PLAN-show-enhanced-columns-in-entities.md
```

---

## Notes

- This is a v1.0 launch decision - Entity concept will be re-implemented post-launch
- The relationship discovery (FK, PK matching) services may remain but should output to a simpler structure without the entity abstraction
- After removal, the ontology focuses on: table metadata, column metadata, glossary terms, and project knowledge
- No backward compatibility or data migration needed - database will be dropped/recreated

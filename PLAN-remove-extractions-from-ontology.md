# PLAN: Remove Extractions from Ontology - Refocus on Combination Layer

## Status: ✅ COMPLETE

All phases implemented in PR #19 on branch `ddanieli/update-ontology`.

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Prerequisites check (backend + frontend) | ✅ Complete |
| 2 | Remove redundant scanning from workflow | ✅ Complete |
| 3 | Refocus analysis on domain entities/relationships | ✅ Complete |
| 4 | Q&A enhancement for entity/relationship creation | ✅ Complete |

---

## Context

The mental model has evolved. We now have three separate screens:

| Screen | Purpose | Workflow Phase |
|--------|---------|----------------|
| **Relationships** | Discover/manage table-to-table relationships (FK, inferred, manual) | `relationships` |
| **Entities** | Discover/manage domain entities (user, account, order) | `entities` |
| **Ontology** | ??? (currently duplicates entity/relationship work) | `ontology` |

**The Problem:** The current Ontology extraction workflow duplicates work:
- It scans tables and columns (but Relationships already did column stats)
- It treats each table as an "entity" (but domain entities are now separate)
- It builds entity summaries per-table (but Entities screen has domain entities)

**The New Vision:** Ontology is the **combination layer** that takes:
- Schema (tables, columns, types)
- Entities (domain concepts discovered)
- Relationships (how tables/entities connect)

And produces:
- **Business Logic** - Rules, constraints, domain knowledge
- **Column Semantics** - What each column means, enum values, synonyms
- **Project Knowledge** - Facts learned from Q&A
- **Entity-Relationship Mapping** - How domain entities relate to each other

---

## Prerequisites Model

**Ontology requires BOTH Entities AND Relationships to exist.**

Similar to how Datasource and Schema are required before other features, the Dashboard should show:

```
┌─────────────────────────────────────────────────────────────────┐
│  Dashboard Tiles with Prerequisites                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  [Entities]     [Relationships]     [Ontology]                   │
│   ✓ 5 found      ✓ 12 defined       ⚠️ Not started              │
│                                                                  │
│  Comment under Ontology tile when prerequisites missing:         │
│  "Requires Entities and Relationships"                           │
│                                                                  │
│  Comment under Ontology tile when prerequisites met:             │
│  "Ready to build" or status of current ontology                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Ontology is NOT required for most features.** The only place that depends on Ontology is the **MCP Server application**. The MCP Server page should indicate which features require Ontology:
- Ontology tools (schema context with semantics) → Requires Ontology
- Pre-approved queries → Does NOT require Ontology
- Developer tools → Does NOT require Ontology (uses raw schema)

---

## Current Ontology Workflow (What Exists)

```
┌─────────────────────────────────────────────────────────────────┐
│                 Current Ontology Extraction                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Initialize                                                   │
│     └── Load tables/columns from schema                          │
│     └── Create workflow_state for each table and column          │
│                                                                  │
│  2. Process Project Description (LLM)                            │
│     └── Extract domain context from user description             │
│     └── Store in project_knowledge                               │
│                                                                  │
│  3. Scan Phase (SQL) ← DUPLICATES RELATIONSHIPS WORK             │
│     └── Gather column statistics                                 │
│     └── Sample values, distinct counts, null rates               │
│                                                                  │
│  4. Analyze Phase (LLM) ← TREATS TABLES AS ENTITIES              │
│     └── Analyze each table as an "entity"                        │
│     └── Generate questions per table/column                      │
│                                                                  │
│  5. Build Tier 1 (LLM)                                           │
│     └── Create entity_summaries map (table → summary)            │
│     └── Business names, descriptions, synonyms                   │
│                                                                  │
│  6. Build Tier 0 (LLM)                                           │
│     └── Create domain_summary from entity summaries              │
│     └── High-level domain description                            │
│                                                                  │
│  7. Question Resolution                                          │
│     └── User answers clarifying questions                        │
│                                                                  │
│  8. Chat Refinement                                              │
│     └── Ongoing Q&A to refine ontology                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Files Involved:**
- `pkg/services/ontology_workflow.go` - Orchestration
- `pkg/services/ontology_builder.go` - LLM integration
- `pkg/services/ontology_tasks.go` - Task definitions
- `pkg/services/workflow_orchestrator.go` - Entity state machine
- `ui/src/pages/OntologyPage.tsx` - UI
- `ui/src/components/ontology/*` - Components

---

## New Vision: Ontology as Combination Layer

```
┌─────────────────────────────────────────────────────────────────┐
│                     New Ontology Model                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  INPUTS (from other phases - REQUIRED):                          │
│  ├── Schema (tables, columns, types, stats) ← from Schema view   │
│  ├── Relationships (FK, inferred, entity-rel) ← from Rel page    │
│  └── Domain Entities (user, account, order) ← from Entities page │
│                                                                  │
│  ONTOLOGY EXTRACTION (new focus):                                │
│  ├── 1. Project Context (user description → domain understanding)│
│  ├── 2. Entity Analysis (understand each domain entity's role)   │
│  ├── 3. Relationship Analysis (understand entity connections)    │
│  ├── 4. Column Semantic Enrichment (meanings, synonyms, enums)   │
│  ├── 5. Business Rule Extraction (via Q&A)                       │
│  └── 6. Knowledge Consolidation (tier 0 + tier 1 summaries)      │
│                                                                  │
│  OUTPUTS:                                                        │
│  ├── domain_summary (tier 0) - Business domain overview          │
│  ├── entity_summaries (tier 1) - Per-entity semantic info        │
│  ├── column_details (tier 2) - Column semantics, enums           │
│  └── project_knowledge - Business rules, terminology, facts      │
│                                                                  │
│  Q&A CAPABILITIES:                                               │
│  ├── Clarify column meanings                                     │
│  ├── Define enum value meanings                                  │
│  ├── Add NEW entities (conceptual, not in schema)                │
│  ├── Add/optimize relationships                                  │
│  └── Record business rules                                       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## What to Remove/Change

### Backend Changes

#### 1. Remove Table/Column Scanning from Ontology Phase

**Current:** `ontology_tasks.go` has `InitializeOntologyTask` that creates workflow_state entries for every table and column, then scans them.

**Change:**
- Remove the per-table/column workflow_state creation
- Don't re-scan columns (data already gathered in relationships phase)
- Use existing schema data + relationship data as input

**Files:**
- `pkg/services/ontology_tasks.go` - Remove scanning logic
- `pkg/services/workflow_orchestrator.go` - Simplify entity state machine

#### 2. Refocus Analysis on Domain Entities AND Relationships

**Current:** Analyzes each TABLE as an entity.

**Change:**
- Analyze each DOMAIN ENTITY (from `engine_ontology_entities`)
- Analyze each ENTITY RELATIONSHIP (from `engine_entity_relationships`)
- For each entity, understand its occurrences across tables and its role
- For each relationship, understand the business meaning of the connection
- Generate questions at the entity and relationship level, not table level

**Files:**
- `pkg/services/ontology_builder.go` - Change analysis focus

#### 3. Require Prerequisites: Entities AND Relationships

**Current:** Checks if relationships phase completed.

**Change:**
- Check if BOTH entities AND relationships phases completed
- Ontology extraction requires: schema + relationships + entities
- Return clear error if prerequisites not met

**Files:**
- `pkg/services/ontology_workflow.go` - Add prerequisite checks

#### 4. Simplify Workflow State

**Current:** Creates `workflow_state` entries for `global`, `table`, `column`.

**Change:**
- Simplify to `global` and `entity` (domain entities)
- Or remove workflow_state entirely and use a simpler progress model

**Files:**
- `pkg/services/ontology_workflow.go`
- `pkg/repositories/workflow_state_repository.go`

---

### Frontend Changes

#### 1. Keep WorkQueue UI Element (Empty Until Work Defined)

**Current:** Shows scanning/analyzing progress for each table and column.

**Change:**
- Keep the WorkQueue component in place
- Show it empty or with placeholder until we define the new work items
- This preserves the UI layout while we redesign the workflow

**Files:**
- `ui/src/pages/OntologyPage.tsx` - Keep layout, change data source
- `ui/src/components/ontology/WorkQueue.tsx` - No changes needed

#### 2. Update Idle State with Prerequisite Check

**Current:** Shows project description input and "Start Extraction" button.

**Change:**
- Check prerequisites (BOTH entities AND relationships must exist)
- If missing, show guidance to complete those first (similar to Schema/Datasource pattern)
- Project description remains for domain context

**Files:**
- `ui/src/pages/OntologyPage.tsx`

#### 3. Update Dashboard Tile

**Current:** Ontology tile shows status without prerequisite indication.

**Change:**
- Show "Requires Entities and Relationships" comment when prerequisites not met
- Disable tile click or show guidance when clicked
- Pattern matches Datasource → Schema dependency

**Files:**
- `ui/src/pages/ProjectDashboard.tsx`

#### 4. Enhance Q&A Capabilities

**Current:** Q&A can update ontology summaries and store knowledge.

**Change:**
- Add ability to suggest/create new entities through chat
- Add ability to suggest/create new relationships through chat
- These would create entries in the respective tables

**Files:**
- `ui/src/components/ontology/ChatPane.tsx`
- `pkg/services/ontology_chat.go`

---

## MCP Server Integration

**Ontology is only required for MCP Server ontology tools.** Update the MCP Server page to show this:

```
┌─────────────────────────────────────────────────────────────────┐
│  MCP Server Page - Tool Groups                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ☑ Developer Tools                                               │
│    Raw schema access, query execution                            │
│                                                                  │
│  ☑ Pre-Approved Queries                                          │
│    Execute admin-approved parameterized queries                  │
│                                                                  │
│  ☐ Ontology Tools                          ⚠️ Requires Ontology  │
│    Schema context with semantic information                      │
│    (Ontology not yet built - click to set up)                    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**Files:**
- `ui/src/pages/MCPServerPage.tsx` - Show ontology requirement for ontology tools

---

## Database Schema Changes

### No Backward Compatibility Required

The database can be dropped and recreated from scratch. No data migration path needed for existing data.

### Tables to Keep As-Is

| Table | Purpose |
|-------|---------|
| `engine_ontology_entities` | Domain entities (discovered or created) |
| `engine_ontology_entity_occurrences` | Where entities appear in schema |
| `engine_ontology_entity_aliases` | Alternative names for entities |
| `engine_entity_relationships` | Entity-to-entity relationships |
| `engine_schema_relationships` | Table-to-table relationships |
| `engine_ontology_workflows` | Workflow lifecycle (all phases) |
| `engine_ontology_questions` | Q&A questions |
| `engine_ontology_chat_messages` | Chat history |
| `engine_project_knowledge` | Business rules, facts |

### Tables to Migrate/Restructure

| Table | Current State | Change |
|-------|---------------|--------|
| `engine_ontologies` | Contains `entity_summaries` keyed by TABLE name | Restructure: key by ENTITY name |
| `engine_workflow_state` | Per-table/column state during extraction | Simplify: remove table/column types, keep only global/entity |

### Migration Plan

1. **Alter `engine_ontologies.entity_summaries`**
   - Currently: `{"users": {...}, "orders": {...}}` (keyed by table)
   - After: `{"user": {...}, "order": {...}}` (keyed by domain entity)
   - Migration: Drop and let new extraction repopulate

2. **Simplify `engine_workflow_state`**
   - Remove rows where `entity_type` = 'table' or 'column'
   - Keep only 'global' and new 'entity' types
   - Or: drop table entirely if we switch to simpler progress model

3. **Drop redundant columns if any**
   - Review `engine_ontologies` for columns that duplicate entity/relationship data

---

## Phased Implementation

### Phase 1: Add Prerequisites Check ✅ COMPLETE

1. **Backend: Check for entities AND relationships** ✅
   - `ontology_workflow.go`: Added check for both `entities` and `relationships` phases completed
   - Returns clear error message if prerequisites not met

2. **Frontend: Show prerequisite status** ✅
   - `OntologyPage.tsx`: Added `PrerequisitesStatus` state and UI
   - Shows "Prerequisites Required" when entities or relationships missing
   - Shows "Ready to Build Ontology" when prerequisites met

### Phase 2: Remove Redundant Scanning ✅ COMPLETE

1. **Skip column scanning in ontology workflow** ✅
   - Ontology now reads from existing schema data, not re-scans

2. **Simplify workflow_state** ✅
   - `initializeWorkflowEntities` now only creates `global` entity type
   - Removed `table` and `column` entity types

3. **Update UI** ✅
   - Progress shows "Building ontology from entities and relationships..."
   - Simpler flow without per-table/column progress

### Phase 3: Refocus on Entities AND Relationships ✅ COMPLETE

1. **Load entities and relationships as input** ✅
   - `BuildTieredOntology` loads domain entities from `engine_ontology_entities`
   - Loads entity relationships from `engine_entity_relationships`
   - Loads occurrences for each entity via `GetOccurrencesByEntity`

2. **Change analysis focus** ✅
   - `buildEntitySummariesFromDomainEntities` creates summaries per domain entity
   - `buildDomainSummaryFromEntities` includes relationship graph
   - No LLM calls needed - assembles from prerequisite data

3. **Update entity_summaries structure** ✅
   - Keyed by entity name (not table name)
   - Includes relationships field with related entity names

### Phase 4: Q&A Enhancement ✅ COMPLETE

1. **Entity creation through chat** ✅
   - New `create_domain_entity` tool in `pkg/llm/tools.go`
   - Creates entries in `engine_ontology_entities`

2. **Relationship suggestions through chat** ✅
   - New `create_entity_relationship` tool in `pkg/llm/tools.go`
   - Creates entries in `engine_entity_relationships`
   - Added `DetectionMethodManual` constant for chat-created relationships

3. **Business rule extraction** ✅
   - Existing `store_knowledge` tool already supports this

---

## UI Flow After Changes

```
┌─────────────────────────────────────────────────────────────────┐
│                     Ontology Page Flow                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  IF entities OR relationships missing:                           │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ ⚠️ Prerequisites Required                                    ││
│  │                                                              ││
│  │ Before building the ontology, complete these steps:          ││
│  │                                                              ││
│  │ 1. ❌ Discover entities → [Go to Entities]                   ││
│  │ 2. ❌ Define relationships → [Go to Relationships]           ││
│  │                                                              ││
│  │ The ontology combines your schema, entities, and             ││
│  │ relationships into a unified business understanding.         ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│  IF prerequisites met but ontology not started:                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ 🧠 Ready to Build Ontology                                   ││
│  │                                                              ││
│  │ Found: 5 entities, 8 entity relationships                    ││
│  │                                                              ││
│  │ Describe your application: [textarea]                        ││
│  │                                                              ││
│  │ [Start Building Ontology]                                    ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│  DURING extraction:                                              │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Progress: Analyzing entity relationships... (Step 2/4)       ││
│  ├─────────────────────────────────────────────────────────────┤│
│  │ Work Queue (empty)     │  Questions Panel                    ││
│  │                        │                                     ││
│  │ [placeholder or        │  Q: What does "status" mean         ││
│  │  simple progress]      │     for the user entity?            ││
│  │                        │                                     ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│  AFTER completion:                                               │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ ✓ Ontology Complete                                          ││
│  │                                                              ││
│  │ Chat to refine, ask questions, or add new entities           ││
│  │                                                              ││
│  │ [Chat Panel - full width]                                    ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Mental Model Summary

**Before (Current):**
- Ontology = Schema extraction + business analysis
- Entities = Domain entities (separate, but not used by ontology)
- Relationships = Table relationships (prerequisite, some data reuse)

**After (New):**
- Ontology = **Combination** of Schema + Entities + Relationships + Business Knowledge
- Ontology extraction REQUIRES and CONSUMES entities and relationships as input
- Ontology focuses on semantic enrichment and Q&A, not data scanning
- Analyzes domain entities AND their relationships, not tables

---

## File Changes Summary

| File | Change |
|------|--------|
| `pkg/services/ontology_workflow.go` | Require BOTH entities AND relationships as prerequisites |
| `pkg/services/ontology_tasks.go` | Remove scanning tasks, add entity+relationship loading |
| `pkg/services/ontology_builder.go` | Refocus analysis on domain entities AND relationships |
| `pkg/services/workflow_orchestrator.go` | Simplify state tracking |
| `ui/src/pages/OntologyPage.tsx` | Add prerequisite check for both entities and relationships |
| `ui/src/pages/ProjectDashboard.tsx` | Add prerequisite comment under Ontology tile |
| `ui/src/pages/MCPServerPage.tsx` | Show "Requires Ontology" for ontology tools |
| `ui/src/components/ontology/WorkQueue.tsx` | Keep as-is (will show empty until work defined) |
| `pkg/services/ontology_chat.go` | Add entity/relationship creation capabilities |

---

## Resolved Questions

1. **Should we keep WorkQueue at all?**
   - **Answer:** Yes, keep all UI elements. WorkQueue will be empty until we define the new work items.

2. **Should ontology extraction be required?**
   - **Answer:** No. Ontology is only required for MCP Server ontology tools. Other features work without it.

3. **Migration path for existing ontologies?**
   - **Answer:** No migration needed. Database can be dropped and recreated. New extraction will repopulate.

---

## Open Questions

1. **What about tables with no entity mapping?**
   - Some tables might be junction/mapping tables with no domain entity
   - How do we include their column semantics in the ontology?

2. **How do we handle schema changes after ontology is built?**
   - If new tables appear, how does ontology update?
   - Incremental refresh vs full rebuild

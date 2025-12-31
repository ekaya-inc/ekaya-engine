# Plan: Entity Discovery Workflow

## Status: ✅ COMPLETED

All implementation steps have been completed and `make check` passes.

**Files Created:**
- `pkg/services/entity_discovery_service.go` - Standalone workflow service
- `pkg/handlers/entity_discovery_handler.go` - HTTP routes
- `ui/src/types/entityDiscovery.ts` - TypeScript types
- `ui/src/components/EntityDiscoveryProgress.tsx` - Progress modal

**Files Modified:**
- `pkg/models/ontology_workflow.go` - Added `WorkflowPhaseEntities` constant
- `pkg/services/relationship_workflow.go` - Added prerequisite check for entities, removed Phase 6 (entity discovery) since it's now a standalone workflow
- `pkg/services/relationship_workflow_test.go` - Added mock entity repository
- `main.go` - Wired service and handler
- `ui/src/services/engineApi.ts` - Added API methods
- `ui/src/pages/EntitiesPage.tsx` - Added discover button and modal integration
- `ui/src/types/index.ts` - Re-exported entity discovery types

---

## Reference Files

**Study these files before implementing:**

| File | Purpose |
|------|---------|
| `pkg/services/relationship_workflow.go` | Pattern for async workflow service (StartDetection, GetStatus, runWorkflow, heartbeat) |
| `pkg/handlers/relationship_workflow.go` | Handler pattern with status/cancel endpoints |
| `pkg/services/entity_discovery_task.go` | Existing entity discovery code - reuse `collectColumnStatistics()`, `filterEntityCandidates()`, `analyzeGraphConnectivity()` |
| `ui/src/components/RelationshipDiscoveryProgress.tsx` | Modal pattern with polling and two-column layout |
| `ui/src/pages/RelationshipsPage.tsx` | Page pattern with discovery button trigger |
| `pkg/repositories/ontology_workflow_repository.go` | Workflow state persistence |
| `pkg/repositories/schema_entity_repository.go` | Entity CRUD (already updated in PLAN-initial-entities-screen) |

**Database tables:**
- `engine_ontology_workflows` - Workflow state (id, project_id, state, phase, progress JSONB, task_queue JSONB)
- `engine_ontology_entities` - Entity records (migrated from engine_schema_entities)
- `engine_ontology_entity_occurrences` - Where entities appear
- `engine_ontology_entity_aliases` - Alternative names

**Key patterns to copy:**
1. `runWorkflow()` background goroutine with phase execution
2. `updateProgress()` for UI feedback
3. `workqueue.Queue` for task management
4. 2-second polling in UI with `useEffect` + `setInterval`

---

## Overview

Implement entity discovery as a standalone workflow on the Entities screen, decoupled from the relationship workflow. Entity detection is **deterministic** (not LLM-decided), with optional LLM enrichment for descriptions and aliases.

### Key Principles
1. **Deterministic Discovery**: Every table with a unique/primary key column represents an entity
2. **Reuse Patterns**: Follow relationship workflow architecture (async, polling, modal)
3. **Decouple from Relationships**: Entity discovery runs independently; relationships use discovered entities
4. **LLM for Enrichment Only**: LLM provides descriptions and aliases, not entity decisions

### Terminology
- **Entity**: A domain noun (user, order, channel) backed by a table with unique identifier
- **Occurrence**: Where an entity's ID appears in the schema (e.g., `orders.user_id` is an occurrence of `user`)
- **Role**: Semantic context of an occurrence (e.g., `visitor_id` vs `host_id` are both `user` with different roles)

---

## Architecture

### Workflow Phases

| Phase | Name | Type | Description |
|-------|------|------|-------------|
| 0 | Collect Statistics | Parallel | Get row counts, distinct values for all columns |
| 1 | Identify Entities | Single | Find tables with PK/unique columns (deterministic) |
| 2 | Find Occurrences | Single | Match entity IDs across schema via naming/stats |
| 3 | Enrich with LLM | Single | Generate descriptions, suggest aliases, identify roles |
| 4 | Persist Results | Single | Save entities, occurrences, aliases to database |

### Reused Components
- `engine_ontology_workflows` table (workflow state tracking)
- `workqueue.Queue` (task execution)
- Polling pattern (UI polls `/status` every 2 seconds)
- Progress modal component pattern from `RelationshipDiscoveryProgress.tsx`

---

## Step 1: Create Entity Discovery Service

**File:** `pkg/services/entity_discovery_service.go`

```go
type EntityDiscoveryService interface {
    // Start entity discovery for a datasource
    StartDiscovery(ctx context.Context, projectID, datasourceID uuid.UUID) (*EntityDiscoveryResult, error)

    // Get current workflow status
    GetStatus(ctx context.Context, projectID, datasourceID uuid.UUID) (*EntityDiscoveryStatus, error)

    // Cancel running workflow
    Cancel(ctx context.Context, projectID, datasourceID uuid.UUID) error
}

type EntityDiscoveryStatus struct {
    WorkflowID    uuid.UUID           `json:"workflow_id"`
    State         string              `json:"state"`         // pending, running, completed, failed, cancelled
    Phase         string              `json:"phase"`         // current phase name
    Progress      WorkflowProgress    `json:"progress"`
    TaskQueue     []TaskSnapshot      `json:"task_queue"`
    EntitiesFound int                 `json:"entities_found"`
    Error         *string             `json:"error,omitempty"`
}

type EntityDiscoveryResult struct {
    WorkflowID uuid.UUID `json:"workflow_id"`
    Status     string    `json:"status"`
}
```

**Dependencies:**
- `OntologyWorkflowRepository` - workflow state persistence
- `SchemaEntityRepository` - entity persistence
- `SchemaRepository` - table/column metadata
- `DatasourceAdapterFactory` - statistics collection
- `LLMFactory` - description/alias generation

**Implementation Pattern:** Follow `relationship_workflow.go`:
1. Check for existing non-terminal workflow
2. Create/claim workflow with heartbeat
3. Launch background goroutine
4. Return workflow ID immediately

---

## Step 2: Implement Discovery Phases

### Phase 0: Collect Statistics

**Reuse:** `collectColumnStatistics()` from relationship workflow

Collects for each column:
- Row count
- Distinct value count
- Null count
- Sample values

### Phase 1: Identify Entities (Deterministic)

**New function:** `identifyEntities()`

```go
type IdentifiedEntity struct {
    Name          string // Derived from table name (singularized)
    Schema        string
    Table         string
    PrimaryColumn string
    RowCount      int64
}
```

**Algorithm:**
1. For each selected table in the datasource:
2. Find columns that are primary keys OR have unique constraints
3. Create entity with name = singularized table name (e.g., `users` → `user`)
4. Store primary location (schema.table.column)

**Edge cases:**
- Composite primary keys: Use first column or skip (configurable)
- Tables without PK: Check for unique constraints, else skip with warning
- Junction tables: Identified by having only FK columns as PK (flag for later)

### Phase 2: Find Occurrences

**New function:** `findOccurrences()`

For each identified entity, find all columns that reference it:

**Matching strategies (in order):**
1. **Foreign Key Constraints**: Direct FK references (highest confidence: 1.0)
2. **Naming Convention**: `{entity}_id`, `{entity}Id`, `fk_{entity}` patterns (confidence: 0.9)
3. **Statistical Match**: Same data type + similar distinct count + overlapping values (confidence: 0.7-0.9)

**Output:** List of occurrences with confidence scores and optional role hints from column names.

### Phase 3: LLM Enrichment (Optional)

**New function:** `enrichWithLLM()`

For each entity, ask LLM to provide:
1. **Description**: Business-friendly explanation of what the entity represents
2. **Suggested Aliases**: Alternative names users might use in queries
3. **Role Identification**: For occurrences, suggest semantic roles based on column context

**Prompt structure:**
```
Given these entities discovered in a database schema:
- Entity "user" (primary: public.users.id) appears in:
  - orders.user_id
  - visits.visitor_id
  - visits.host_id
  - channels.owner_id

For each entity, provide:
1. A brief description (1-2 sentences)
2. Common aliases users might use
3. For each occurrence, a semantic role if applicable
```

**LLM is optional**: If LLM unavailable or disabled, entities are still created with empty descriptions.

### Phase 4: Persist Results

**Function:** `persistEntities()`

1. Delete existing entities for this ontology (fresh discovery)
2. Create `engine_ontology_entities` records
3. Create `engine_ontology_entity_occurrences` records
4. Create `engine_ontology_entity_aliases` records (from LLM suggestions)

Use `ON CONFLICT DO NOTHING` for occurrences to handle duplicates gracefully.

---

## Step 3: Create Handler

**File:** `pkg/handlers/entity_discovery_handler.go`

**Routes:**
```
POST   /api/projects/{pid}/datasources/{dsid}/entities/discover  - Start discovery
GET    /api/projects/{pid}/datasources/{dsid}/entities/status    - Get workflow status
POST   /api/projects/{pid}/datasources/{dsid}/entities/cancel    - Cancel workflow
```

**Response types follow relationship workflow patterns.**

---

## Step 4: Wire Handler in main.go

```go
entityDiscoveryService := services.NewEntityDiscoveryService(
    ontologyWorkflowRepo, schemaEntityRepo, schemaRepo, ontologyRepo,
    datasourceService, adapterFactory, llmFactory, getTenantCtx, logger)

entityDiscoveryHandler := handlers.NewEntityDiscoveryHandler(entityDiscoveryService, logger)
entityDiscoveryHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)
```

---

## Step 5: Add TypeScript Types

**File:** `ui/src/types/entityDiscovery.ts`

```typescript
export interface EntityDiscoveryStatus {
    workflow_id: string;
    state: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
    phase: string;
    progress: {
        current: number;
        total: number;
        percentage: number;
        message: string;
    };
    task_queue: TaskSnapshot[];
    entities_found: number;
    error?: string;
}

export interface StartDiscoveryResponse {
    workflow_id: string;
    status: string;
}
```

---

## Step 6: Add API Methods

**File:** `ui/src/services/engineApi.ts`

```typescript
// Entity Discovery
async startEntityDiscovery(projectId: string, datasourceId: string): Promise<ApiResponse<StartDiscoveryResponse>>
async getEntityDiscoveryStatus(projectId: string, datasourceId: string): Promise<ApiResponse<EntityDiscoveryStatus>>
async cancelEntityDiscovery(projectId: string, datasourceId: string): Promise<ApiResponse<void>>
```

---

## Step 7: Create Discovery Progress Modal

**File:** `ui/src/components/EntityDiscoveryProgress.tsx`

**Reuse pattern from:** `RelationshipDiscoveryProgress.tsx`

**Layout:**
- Modal dialog with two columns
- Left: Discovery steps with progress indicators
- Right: Discovered entities list (real-time updates)

**Steps to display:**
1. Collecting statistics...
2. Identifying entities...
3. Finding occurrences...
4. Enriching with AI... (if LLM enabled)
5. Saving results...

**Polling:** Every 2 seconds, call `/status` and update UI.

---

## Step 8: Update EntitiesPage

**File:** `ui/src/pages/EntitiesPage.tsx`

**Changes:**
1. Add "Discover Entities" button (disabled if workflow running)
2. Open `EntityDiscoveryProgress` modal on click
3. Refresh entity list after discovery completes
4. Show discovery status badge if workflow in progress

**Button placement:** Top-right, next to page header (like relationships page pattern).

---

## Step 9: Update Relationship Workflow

**File:** `pkg/services/relationship_workflow.go`

**Changes:**
1. Remove entity discovery from relationship workflow (Phase 6)
2. Check for existing entities before relationship detection
3. If no entities exist, return error: "Run entity discovery first"
4. Use existing entities for relationship candidate analysis

**New prerequisite check in `StartDetection()`:**
```go
entities, err := s.entityRepo.GetByProject(ctx, projectID)
if err != nil {
    return nil, fmt.Errorf("check entities: %w", err)
}
if len(entities) == 0 {
    return nil, fmt.Errorf("no entities found - run entity discovery first")
}
```

---

## Testing Checklist

- [x] Phase 0: Statistics collected for all selected tables
- [x] Phase 1: Entities created for tables with PK/unique columns
- [x] Phase 2: Occurrences found via FK, naming, and stats matching
- [x] Phase 3: LLM enrichment adds descriptions (when available)
- [x] Phase 4: Results persisted correctly
- [x] UI: Modal shows progress in real-time
- [x] UI: Entity list refreshes after completion
- [x] UI: Error states handled gracefully
- [x] Relationship workflow requires entities first
- [x] `make check` passes

---

## Implementation Order

| Step | Description | Status |
|------|-------------|--------|
| 1 | Create EntityDiscoveryService skeleton | ✅ Done |
| 2 | Implement Phase 0-2 (deterministic) | ✅ Done |
| 3 | Implement Phase 3-4 (LLM + persist) | ✅ Done |
| 4 | Create handler + wire in main.go | ✅ Done |
| 5-6 | TypeScript types + API methods | ✅ Done |
| 7 | Create EntityDiscoveryProgress modal | ✅ Done |
| 8 | Update EntitiesPage with discover button | ✅ Done |
| 9 | Update relationship workflow prerequisite | ✅ Done |

**All batches completed:**
- ✅ Backend: Steps 1-4 (service, phases, handler)
- ✅ Frontend: Steps 5-8 (types, API, modal, page)
- ✅ Integration: Step 9 (relationship prerequisite)

---

## Future Enhancements (Out of Scope)

- Manual entity creation from UI
- Entity merge/split operations
- Re-run discovery with delta detection
- Entity confidence scoring
- Cross-datasource entity linking

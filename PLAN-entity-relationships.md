# PLAN: Entity-Based Relationship Discovery

## Goal
Replace the current candidate-based relationship detection (891 candidates, token limit exceeded) with entity-based discovery that builds a connected graph of tables using domain entities (user, account, order, etc.) with role semantics (visitor, host, owner).

## Testing Protocol
Each phase is tested by clicking **[Find Relationships]** in the UI. Phases are complete when the expected output is visible in either:
- Server logs (terminal running `make dev-server`)
- UI display
- Database tables (query with `psql`)

---

## Phase 1: Column Statistics Collection ✅

**Goal:** Collect distinct counts for all columns across all tables BEFORE any LLM calls.

**Implementation:**
- Modify the relationship workflow to call `AnalyzeColumnStats` for every table
- Log the results: `INFO: Column stats collected for 21 tables, 156 columns`
- Store stats in workflow state or pass through pipeline

**Test:** Click [Find Relationships], check server logs for:
```
Column statistics: public.users (8 columns)
  - id: 100 distinct / 100 rows (100.0%)
  - email: 100 distinct / 100 rows (100.0%)
  - status: 4 distinct / 100 rows (4.0%)
  - created_at: 98 distinct / 100 rows (98.0%)
Column statistics: public.orders (6 columns)
  - id: 500 distinct / 500 rows (100.0%)
  - user_id: 95 distinct / 500 rows (19.0%)
  ...
Summary: Collected stats for 156 columns across 21 tables
```

**Success Criteria:** All tables/columns logged with distinct counts and ratios.

**Files to modify:**
- `pkg/services/relationship_workflow.go` - Add stats collection step
- May need new task type or inline in workflow

---

## Phase 2: Column Filtering (Entity Candidates) ✅

**Goal:** Filter columns to identify entity candidates using heuristics. No LLM yet.

**Heuristics:**
- Include if: `distinct_count >= 20 AND distinct_count / row_count > 0.05`
- Include if: `IsPrimaryKey = true` OR `IsUnique = true`
- Include if: column name matches `*_id`, `*_uuid`, `*_key`, or is `id`
- Exclude if: type is BOOLEAN, TIMESTAMP, DATE
- Exclude if: name matches `*_at`, `*_date`, `is_*`, `has_*`, `*_status`, `*_type`, `*_flag`

**Data Flow:** Returns `(candidates []ColumnFilterResult, excluded []ColumnFilterResult, error)` for use by Phase 6.

**Test:** Click [Find Relationships], check server logs for:
```
Column filtering results:
  CANDIDATE: public.orders.user_id (bigint, 95 distinct, PK=false, Unique=false)
  CANDIDATE: public.orders.product_id (uuid, 200 distinct, PK=false, Unique=false)
  CANDIDATE: public.visits.visitor_id (uuid, 80 distinct, PK=false, Unique=false)
  CANDIDATE: public.visits.host_id (uuid, 45 distinct, PK=false, Unique=false)
  EXCLUDED: public.orders.status (4 distinct - below threshold)
  EXCLUDED: public.orders.created_at (timestamp type)
  EXCLUDED: public.users.is_active (boolean type)

Summary: 47 candidate columns, 109 excluded columns
```

**Success Criteria:** Candidates are reasonable entity references, excluded columns are attributes/enums.

**Files modified:**
- `pkg/services/column_filter.go` - Filtering logic with `ColumnFilterResult` struct
- `pkg/services/relationship_workflow.go` - `filterEntityCandidates()` returns data for Phase 6

---

## Phase 3: Connected Components (Graph Analysis) ✅

**Goal:** Identify table islands using FK relationships. Pure Go, no SQL, no LLM.

**Implementation:**
- Build adjacency graph from `DiscoverForeignKeys()` results
- Run DFS/BFS to find connected components
- Log component membership

**Data Flow:** Returns `(components []ConnectedComponent, islands []string, error)` for use by Phase 6.

**Test:** Click [Find Relationships], check server logs for:
```
Graph connectivity analysis:
  Foreign keys: 14 relationships

  Component 1 (9 tables): users, orders, order_items, products, categories, ...
  Component 2 (3 tables): audit_logs, audit_events, audit_users
  Component 3 (1 table): standalone_config
  Island tables (8): visits, sessions, notifications, ...

Summary: 3 connected components, 8 island tables need bridging
```

**Success Criteria:** Components correctly identified based on FK edges.

**Files modified:**
- `pkg/services/graph.go` - `TableGraph`, `ConnectedComponent` structs and DFS algorithm
- `pkg/services/graph_test.go` - Unit tests for graph algorithms
- `pkg/services/relationship_workflow.go` - `analyzeGraphConnectivity()` returns data for Phase 6

---

## Phase 4: Database Tables for Entities

**Goal:** Create tables to store discovered entities and their occurrences.

**Tables:**
```sql
CREATE TABLE engine_schema_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id),
    name TEXT NOT NULL,                    -- "user", "account", "order"
    description TEXT,                      -- LLM explanation
    primary_schema TEXT NOT NULL,
    primary_table TEXT NOT NULL,
    primary_column TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(ontology_id, name)
);

CREATE TABLE engine_schema_entity_occurrences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_schema_entities(id) ON DELETE CASCADE,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    column_name TEXT NOT NULL,
    role TEXT,                             -- "visitor", "host", "owner", NULL for generic
    confidence FLOAT DEFAULT 1.0,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(entity_id, schema_name, table_name, column_name)
);

CREATE INDEX idx_entity_occurrences_table
    ON engine_schema_entity_occurrences(schema_name, table_name);
```

**Test:** After migration, verify with:
```sql
\d engine_schema_entities
\d engine_schema_entity_occurrences
```

**Success Criteria:** Tables exist with correct schema.

**Files to modify:**
- `migrations/XXXXXX_create_schema_entities.up.sql` (new)
- `migrations/XXXXXX_create_schema_entities.down.sql` (new)

---

## Phase 5: Entity Repository

**Goal:** Create repository layer for entity CRUD operations.

**Interface:**
```go
type SchemaEntityRepository interface {
    Create(ctx context.Context, entity *models.SchemaEntity) error
    GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.SchemaEntity, error)
    GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.SchemaEntity, error)
    DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error

    CreateOccurrence(ctx context.Context, occ *models.SchemaEntityOccurrence) error
    GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.SchemaEntityOccurrence, error)
    GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.SchemaEntityOccurrence, error)
}
```

**Test:** Unit tests pass for repository methods.

**Success Criteria:** `make test` passes with new repository tests.

**Files to modify:**
- `pkg/models/schema_entity.go` (new)
- `pkg/repositories/schema_entity_repository.go` (new)
- `pkg/repositories/postgres/schema_entity_repository.go` (new)
- Tests for repository

---

## Transition Strategy: Old → New Workflow

### Current Code Structure (`runWorkflow` in `relationship_workflow.go`)

The code currently has TWO workflow paths interleaved:

```
NEW ENTITY-BASED (PLAN Phases 1-3):
├── Phase 0:    collectColumnStatistics()      → statsMap           [PLAN Phase 1] ✅
├── Phase 0.5:  filterEntityCandidates()       → candidates, excluded [PLAN Phase 2] ✅
├── Phase 0.75: analyzeGraphConnectivity()     → components, islands  [PLAN Phase 3] ✅
│
│   ← INSERT ENTITY DISCOVERY HERE (PLAN Phase 6)
│
OLD CANDIDATE-BASED (to be removed):
├── Phase 1: enqueueColumnScans()              → workflow_state samples
├── Phase 2: ValueMatchTask                    → relationship candidates
├── Phase 3: NameInferenceTask                 → relationship candidates
├── Phase 4: enqueueTestJoins()                → candidate cardinality
└── Phase 5: AnalyzeRelationshipsTask (LLM)    → confirmed candidates
```

### Transition Instructions for Phase 6

**Step 1: Create `EntityDiscoveryTask`** (new file)
- Input: `candidates`, `excluded`, `components`, `islands`, `statsMap`
- Output: Discovered entities persisted to `engine_schema_entities`

**Step 2: Wire into `runWorkflow`**

Replace the TODO placeholder:
```go
// TODO(Phase 6): Pass candidates, excluded, components, islands to EntityDiscoveryTask
_, _, _, _ = candidates, excluded, components, islands
```

With:
```go
// Phase 6: Entity Discovery (LLM) - replaces old candidate-based analysis
entityTask := NewEntityDiscoveryTask(
    s.entityRepo,        // new repository from Phase 5
    s.schemaRepo,
    s.llmFactory,
    s.getTenantCtx,
    projectID,
    workflowID,
    datasourceID,
    candidates,
    excluded,
    components,
    islands,
    statsMap,
    s.logger,
)
queue.Enqueue(entityTask)

if err := queue.Wait(ctx); err != nil {
    s.logger.Error("Entity discovery failed", zap.Error(err))
    s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("entity discovery: %v", err))
    return
}
```

**Step 3: Remove old phases** (after entity discovery works)

Delete the old candidate-based code (code Phases 1-5):
- `enqueueColumnScans()` call and wait
- `ValueMatchTask` creation and wait
- `NameInferenceTask` creation and wait
- `enqueueTestJoins()` call and wait
- `AnalyzeRelationshipsTask` creation and wait

**Step 4: Update `finalizeWorkflow`**

Change from counting `requiredPending` candidates to counting discovered entities.

### Why This Order?

1. Phases 4-5 (DB + Repository) must complete first — `EntityDiscoveryTask` needs to persist entities
2. Phase 6 can run alongside old phases during development (both paths execute)
3. Old phases are removed only after Phase 6 is verified working
4. This allows incremental testing without breaking the existing workflow

---

## Phase 6: Entity Discovery Task (LLM)

**Goal:** Create LLM task that identifies entities from candidate columns.

**Input from earlier phases:**
- `candidates []ColumnFilterResult` - Entity candidate columns (from Phase 2)
- `excluded []ColumnFilterResult` - Excluded columns with reasons (from Phase 2)
- `components []ConnectedComponent` - FK-connected table groups (from Phase 3)
- `islands []string` - Tables with no FK connections (from Phase 3)
- `statsMap map[string]ColumnStats` - Column statistics (from Phase 1)

**Prompt design:**
- Send schema summary: table names + candidate column names with stats
- Send existing FKs (derived from components)
- Send excluded columns list (context only, marked as excluded)
- Send island tables (may need bridging relationships)
- Ask: "Identify domain entities and their occurrences with roles"

**Output:** JSON with entities, their primary location, and occurrences with roles

**Test:** Click [Find Relationships], check server logs for:
```
Entity discovery LLM call:
  Input tokens: ~8,000
  Output tokens: ~2,000

Discovered entities:
  - user (primary: public.users.id)
    - public.orders.user_id (role: null)
    - public.visits.visitor_id (role: visitor)
    - public.visits.host_id (role: host)
    - public.properties.owner_id (role: owner)
  - account (primary: public.accounts.id)
    - public.users.account_id (role: null)
    - public.invoices.account_id (role: null)
  - product (primary: public.products.id)
    - public.order_items.product_id (role: null)
    - public.inventory.product_id (role: null)

Summary: 8 entities discovered, 32 total occurrences
```

**Success Criteria:** Entities make semantic sense, roles are identified correctly, no attribute columns (email, password) appear.

**Files to modify:**
- `pkg/services/entity_discovery_task.go` (new)
- `pkg/services/relationship_workflow.go` - Wire up task

---

## Phase 7: Store Entities in Database

**Goal:** Persist discovered entities and occurrences to database.

**Test:** Click [Find Relationships], then query:
```sql
SELECT e.name, e.primary_table, e.primary_column, e.description
FROM engine_schema_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE o.is_active = true;

SELECT e.name, o.table_name, o.column_name, o.role
FROM engine_schema_entities e
JOIN engine_schema_entity_occurrences o ON o.entity_id = e.id
ORDER BY e.name, o.table_name;
```

**Success Criteria:** Entities and occurrences are persisted correctly.

**Files to modify:**
- `pkg/services/relationship_workflow.go` - Save entities after LLM discovery

---

## Phase 8: API Endpoints

**Goal:** Expose entities via REST API, adapting the existing workflow API pattern.

### Existing API Structure (to adapt)

Current endpoints in `relationshipWorkflowApi.ts`:
```
POST /api/projects/{pid}/datasources/{dsid}/relationships/detect  → Start workflow
GET  /api/projects/{pid}/datasources/{dsid}/relationships/status  → Workflow status
GET  /api/projects/{pid}/datasources/{dsid}/relationships/candidates → Get candidates
PUT  /api/projects/{pid}/datasources/{dsid}/relationships/candidates/{cid} → Accept/reject
POST /api/projects/{pid}/datasources/{dsid}/relationships/save    → Save accepted
POST /api/projects/{pid}/datasources/{dsid}/relationships/cancel  → Cancel workflow
```

### New/Modified Endpoints

**Keep unchanged:**
- `POST .../detect` - Start workflow (backend changes, API same)
- `GET .../status` - Workflow status (add entity counts)
- `POST .../save` - Save (now saves entities → relationships)
- `POST .../cancel` - Cancel workflow

**Replace candidates with entities:**
- ~~`GET .../candidates`~~ → `GET .../entities` - Returns discovered entities with occurrences
- ~~`PUT .../candidates/{cid}`~~ → Not needed (no accept/reject per candidate)

**New entity endpoints (for future editing, deferred):**
- `GET /api/projects/{pid}/entities` - List all entities for project
- `GET /api/projects/{pid}/entities/{eid}` - Get entity with occurrences

### Response Types

**EntityResponse:**
```typescript
interface EntityResponse {
  id: string;
  name: string;                    // "user", "account"
  description: string;             // LLM explanation
  primary_schema: string;
  primary_table: string;
  primary_column: string;
  occurrences: EntityOccurrenceResponse[];
}

interface EntityOccurrenceResponse {
  id: string;
  schema_name: string;
  table_name: string;
  column_name: string;
  role: string | null;             // "visitor", "host", "owner", or null
  confidence: number;
}
```

**Modified RelationshipWorkflowStatusResponse:**
```typescript
interface RelationshipWorkflowStatusResponse {
  workflow_id: string;
  phase: 'stats' | 'filtering' | 'graph' | 'entity_discovery' | 'complete';
  state: RelationshipWorkflowState;
  progress?: WorkflowProgress;
  task_queue?: TaskProgressResponse[];
  // NEW: entity counts instead of candidate counts
  entity_count: number;
  occurrence_count: number;
  island_count: number;            // tables not connected
  can_save: boolean;
}
```

**Test:** After [Find Relationships] completes:
```bash
curl http://localhost:3443/api/projects/{pid}/datasources/{dsid}/relationships/entities | jq
```

Expected response:
```json
{
  "entities": [
    {
      "id": "...",
      "name": "user",
      "description": "A person who uses the system",
      "primary_table": "users",
      "primary_column": "id",
      "occurrences": [
        {"table_name": "orders", "column_name": "user_id", "role": null},
        {"table_name": "visits", "column_name": "visitor_id", "role": "visitor"},
        {"table_name": "visits", "column_name": "host_id", "role": "host"}
      ]
    }
  ]
}
```

**Success Criteria:** API returns entities with occurrences and roles.

**Files to modify:**
- `pkg/handlers/relationship_workflow.go` - Modify status, add entities endpoint
- `main.go` - Register new routes

---

## Phase 9: UI Display

**Goal:** Replace candidate-based UI with entity-based display in the discovery dialog.

### Conceptual Shift

| Current (Candidates) | New (Entities) |
|---------------------|----------------|
| 891 individual relationships | ~10-20 semantic entities |
| User accepts/rejects each | User reviews entity definitions |
| `needs_review` / `confirmed` / `rejected` | Just display entities |
| CandidateCard with confidence scores | EntityCard with occurrences list |
| Manual curation required | Automatic semantic discovery |

### Current UI Files (from analysis)

```
ui/src/
├── pages/
│   └── RelationshipsPage.tsx           # Main page - KEEP, minor changes
├── components/
│   ├── RelationshipDiscoveryProgress.tsx  # Discovery dialog - MAJOR changes
│   ├── AddRelationshipDialog.tsx          # Manual add - KEEP unchanged
│   ├── RemoveRelationshipDialog.tsx       # Manual remove - KEEP unchanged
│   └── relationships/
│       ├── CandidateCard.tsx              # REPLACE with EntityCard
│       └── CandidateList.tsx              # REPLACE with EntityList
├── services/
│   └── relationshipWorkflowApi.ts         # MODIFY for entities
└── types/
    └── relationshipWorkflow.ts            # MODIFY for entities
```

### New UI Components

**1. EntityCard.tsx** (replaces CandidateCard.tsx)
```tsx
// Displays a single entity with expandable occurrences
interface EntityCardProps {
  entity: EntityResponse;
  defaultExpanded?: boolean;
}

// Layout:
// ┌─────────────────────────────────────────────┐
// │ 👤 user                                      │
// │ "A person who uses the system"              │
// │ Primary: users.id                           │
// │ ▼ 4 occurrences                             │
// │   ├─ orders.user_id                         │
// │   ├─ visits.visitor_id (visitor)            │
// │   ├─ visits.host_id (host)                  │
// │   └─ properties.owner_id (owner)            │
// └─────────────────────────────────────────────┘
```

**2. EntityList.tsx** (replaces CandidateList.tsx)
```tsx
// Displays all discovered entities
interface EntityListProps {
  entities: EntityResponse[];
  isLoading?: boolean;
}

// No needs_review/confirmed/rejected sections
// Just a list of EntityCards
```

**3. Modify RelationshipDiscoveryProgress.tsx**

Current layout (2 panes):
```
┌──────────────────┬────────────────────────────┐
│ Work Queue       │ Relationship Candidates    │
│ ○ Task 1         │ [CandidateCard] Accept/Rej │
│ ● Task 2         │ [CandidateCard] Accept/Rej │
│ ○ Task 3         │ [CandidateCard] Accept/Rej │
│                  │ ... 891 more ...           │
└──────────────────┴────────────────────────────┘
```

New layout (2 panes):
```
┌──────────────────┬────────────────────────────┐
│ Discovery Steps  │ Discovered Entities        │
│ ✓ Column stats   │ [EntityCard] user          │
│ ✓ Filtering      │ [EntityCard] account       │
│ ✓ Graph analysis │ [EntityCard] product       │
│ ● Entity LLM     │ [EntityCard] order         │
│                  │                            │
│ ──────────────── │ 8 entities, 32 occurrences │
│ Connectivity:    │                            │
│ ✓ All connected  │                            │
└──────────────────┴────────────────────────────┘
```

**4. Modify types/relationshipWorkflow.ts**

Remove:
```typescript
// DELETE these
export type DetectionMethod = 'value_match' | 'name_inference' | 'llm' | 'hybrid';
export interface CandidateResponse { ... }
export interface CandidatesResponse { ... }
export interface CandidateDecisionRequest { ... }
```

Add:
```typescript
// ADD these
export interface EntityResponse {
  id: string;
  name: string;
  description: string;
  primary_schema: string;
  primary_table: string;
  primary_column: string;
  occurrences: EntityOccurrenceResponse[];
}

export interface EntityOccurrenceResponse {
  id: string;
  schema_name: string;
  table_name: string;
  column_name: string;
  role: string | null;
  confidence: number;
}

export interface EntitiesResponse {
  entities: EntityResponse[];
  island_tables: string[];  // Tables not connected to any entity
}

export type DiscoveryPhase = 'stats' | 'filtering' | 'graph' | 'entity_discovery' | 'complete';
```

**5. Modify relationshipWorkflowApi.ts**

```typescript
// REMOVE
async getCandidates(...): Promise<CandidatesResponse>
async updateCandidate(...): Promise<CandidateResponse>

// ADD
async getEntities(projectId: string, datasourceId: string): Promise<EntitiesResponse> {
  return this.makeRequest<EntitiesResponse>(
    `/${projectId}/datasources/${datasourceId}/relationships/entities`
  );
}
```

### Test Protocol

**Test:** Click [Find Relationships], verify:

1. **Left pane shows discovery phases** (not individual table tasks):
   - ✓ Collecting column statistics
   - ✓ Filtering entity candidates
   - ✓ Analyzing graph connectivity
   - ● Discovering entities (LLM)

2. **Right pane shows entities** (not 891 candidates):
   - Entity cards with name, description
   - Expandable occurrence list with roles
   - No Accept/Reject buttons

3. **Footer shows**:
   - "8 entities discovered, 32 column mappings"
   - "Save Entities" button (not "Save X Relationships")

4. **After save**:
   - Entities persisted to `engine_schema_entities`
   - Relationships derived from entity occurrences
   - RelationshipsPage shows relationships as before

**Success Criteria:**
- No "891 candidates" or token limit errors
- Entities display with roles (visitor, host, owner)
- Workflow completes in <30 seconds (one LLM call vs many)

**Files to modify:**
- `ui/src/components/RelationshipDiscoveryProgress.tsx` - Major rewrite
- `ui/src/components/relationships/EntityCard.tsx` (new)
- `ui/src/components/relationships/EntityList.tsx` (new)
- `ui/src/components/relationships/CandidateCard.tsx` - DELETE or keep for legacy
- `ui/src/components/relationships/CandidateList.tsx` - DELETE or keep for legacy
- `ui/src/services/relationshipWorkflowApi.ts` - Replace candidates with entities
- `ui/src/types/relationshipWorkflow.ts` - Replace candidate types with entity types

---

## Summary

| Phase | Description | Test Method | Output |
|-------|-------------|-------------|--------|
| 1 | Column stats | Server logs | Stats per column |
| 2 | Column filtering | Server logs | Candidates vs excluded |
| 3 | Connected components | Server logs | Component membership |
| 4 | Database tables | psql \d | Tables exist |
| 5 | Repository | make test | Tests pass |
| 6 | LLM entity discovery | Server logs | Entities discovered |
| 7 | Persist entities | psql query | Data in tables |
| 8 | API endpoints | curl | JSON response |
| 9 | UI display | Browser | Entities shown |

## Phase 10: MCP/Ontology Integration (Future)

**Goal:** Expose entity/role information to MCP clients for intelligent query generation.

**Why this matters:** When an MCP client asks "show me top 5 hosts by booking count," the LLM needs to know:
- `visits.host_id` represents entity "user" with role "host"
- `visits.visitor_id` represents entity "user" with role "visitor"
- These are different join paths to the same `users` table

**Integration points:**
- Ontology export should include entity definitions with roles
- Schema context for text2sql should include role semantics
- MCP tool descriptions should expose entity/role metadata

**Deferred to future work** - entities must exist first (Phases 1-9).

---

## Cleanup (After All Phases)

- Remove or deprecate `engine_relationship_candidates` table
- Remove `TestJoinTask` (or repurpose for validation)
- Remove `AnalyzeRelationshipsTask` (replaced by `EntityDiscoveryTask`)
- Consolidate migrations before launch
- Update MCP schema tools to expose entity/role information

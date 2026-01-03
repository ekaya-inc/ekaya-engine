# Ontology Workflow DAG Implementation Plan

## Executive Summary

### Problem Statement
Currently, Ekaya Engine has **three separate workflows** for ontology extraction, each triggered independently:

1. **Entity Discovery** - Discovers entities from schema metadata
2. **Relationship Detection** - Detects FK relationships
3. **Enrichment** - Finalization and column enrichment

**Problems:**
- Manual orchestration required - users must trigger workflows in correct order
- No single view of what's running or what needs to run next
- Duplicate state management across services
- No way to handle partial updates or resume after failure

### Proposed Solution
A **unified DAG-based workflow** that:
- Runs all extraction steps automatically from a single trigger
- Provides real-time progress visibility in the Ontology page
- Supports incremental updates (re-run only affected nodes)
- Handles failures gracefully with per-node retry logic
- Allows users to leave and come back to check progress

### User Experience
- User clicks **[Start Extraction]** (first time) or **[Refresh Ontology]** (subsequent)
- DAG runs automatically to completion (no user interaction required during execution)
- User can leave the page and return to check progress
- Optional: User can verify/edit results after workflow completes

---

## Section 1: DAG Architecture

### DAG Nodes (Linear Pipeline)

The workflow is a simple linear pipeline - no parallel branches, no user interaction during execution:

```
┌─────────────────────────────────────────────────────────────────┐
│                     ONTOLOGY EXTRACTION DAG                      │
└─────────────────────────────────────────────────────────────────┘

[1] Entity Discovery (DDL-based, deterministic)
           │
           ▼
[2] Entity Enrichment (LLM: names, descriptions)
           │
           ▼
[3] Relationship Discovery (FK-based, deterministic)
           │
           ▼
[4] Relationship Enrichment (LLM: relationship descriptions)
           │
           ▼
[5] Ontology Finalization (LLM: domain summary, conventions)
           │
           ▼
[6] Column Enrichment (LLM: descriptions, semantic types, enums)
           │
           ▼
       [Complete]
```

### Node Descriptions

| Node | Name | Type | What It Does | Service Method |
|------|------|------|--------------|----------------|
| 1 | `EntityDiscovery` | Data | Identify entities from PKs/unique constraints | `EntityDiscoveryService.identifyEntitiesFromDDL()` |
| 2 | `EntityEnrichment` | LLM | Generate entity names, descriptions | `EntityDiscoveryService.enrichEntitiesWithLLM()` |
| 3 | `RelationshipDiscovery` | Data | Discover FK relationships, save to schema | `DeterministicRelationshipService.DiscoverRelationships()` |
| 4 | `RelationshipEnrichment` | LLM | Generate relationship descriptions via LLM | `RelationshipEnrichmentService.EnrichProject()` |
| 5 | `OntologyFinalization` | LLM | Generate domain summary, detect conventions | `OntologyFinalizationService.Finalize()` |
| 6 | `ColumnEnrichment` | LLM | Generate column descriptions, semantic types, enum values | `ColumnEnrichmentService.EnrichProject()` |

### Execution Strategy
- Nodes execute sequentially (simple linear pipeline)
- Each node completes fully before the next starts
- Failed nodes can be retried without re-running completed nodes
- Progress persisted to database after each node completes
- **Retry logic**: Use `pkg/retry.DoWithResult[T]()` for node-level retries (default: 3 retries, 100ms initial, 5s max, 2x multiplier)
- **LLM timeout**: 5 minutes per request (see `pkg/llm/client.go:DefaultRequestTimeout`)

---

## Section 2: State Detection Logic

### Determining What Needs to Run

The DAG orchestrator inspects current state to determine which nodes need to run:

```go
func DetermineStartNode(ctx, projectID, datasourceID) string {
    ontology := GetActiveOntology(projectID)

    // No ontology or no entities → start from beginning
    entities := entityRepo.ListByOntology(ctx, ontology.ID)
    if len(entities) == 0 {
        return "EntityDiscovery"
    }

    // Entities exist but not enriched
    if CountEntitiesWithoutDescription(ontology.ID) > 0 {
        return "EntityEnrichment"
    }

    // No relationships discovered
    relationships := schemaRepo.GetRelationships(ctx, projectID, datasourceID)
    if len(relationships) == 0 {
        return "RelationshipDiscovery"
    }

    // Relationships not enriched (missing descriptions)
    if CountRelationshipsWithoutDescription(projectID) > 0 {
        return "RelationshipEnrichment"
    }

    // No domain summary
    if ontology.DomainSummary == nil {
        return "OntologyFinalization"
    }

    // Columns not enriched
    if CountTablesWithoutColumnDetails(ontology.ID) > 0 {
        return "ColumnEnrichment"
    }

    // Everything complete
    return "Complete"
}
```

### Schema Change Detection

When user clicks **[Refresh Ontology]**, compare schema fingerprints:

```sql
SELECT md5(string_agg(table_name || '.' || column_name || ':' || data_type,
       ',' ORDER BY table_name, ordinal_position))
FROM engine_schema_columns
WHERE project_id = $1 AND datasource_id = $2
```

If fingerprint changed → start from `EntityDiscovery` (full re-extraction).

---

## Section 3: Database Schema

### New Table: `engine_ontology_dag`

Stores the DAG execution state:

```sql
CREATE TABLE engine_ontology_dag (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,
    ontology_id UUID REFERENCES engine_ontologies(id) ON DELETE CASCADE,

    -- Execution state
    status VARCHAR(30) NOT NULL DEFAULT 'pending',  -- pending, running, completed, failed, cancelled
    current_node VARCHAR(50),  -- Which node is currently executing

    -- Schema tracking
    schema_fingerprint TEXT,

    -- Ownership (multi-server support)
    owner_id UUID,
    last_heartbeat TIMESTAMPTZ,

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One active DAG per datasource
    CONSTRAINT engine_ontology_dag_unique_active
        UNIQUE(datasource_id) WHERE status IN ('pending', 'running')
);

CREATE INDEX idx_engine_ontology_dag_project ON engine_ontology_dag(project_id);
CREATE INDEX idx_engine_ontology_dag_status ON engine_ontology_dag(status);
CREATE INDEX idx_engine_ontology_dag_datasource ON engine_ontology_dag(datasource_id);
```

### New Table: `engine_dag_nodes`

Stores per-node execution state:

```sql
CREATE TABLE engine_dag_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dag_id UUID NOT NULL REFERENCES engine_ontology_dag(id) ON DELETE CASCADE,

    -- Node identification
    node_name VARCHAR(50) NOT NULL,  -- EntityDiscovery, EntityEnrichment, etc.
    node_order INT NOT NULL,          -- Execution order (1, 2, 3, 4, 5)

    -- Execution state
    status VARCHAR(30) NOT NULL DEFAULT 'pending',  -- pending, running, completed, failed, skipped
    progress JSONB,  -- {current: 5, total: 10, message: "Processing table X"}

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_ms INT,

    -- Error handling
    error_message TEXT,
    retry_count INT NOT NULL DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(dag_id, node_name)
);

CREATE INDEX idx_engine_dag_nodes_dag ON engine_dag_nodes(dag_id);
CREATE INDEX idx_engine_dag_nodes_status ON engine_dag_nodes(dag_id, status);
```

### Tables to Remove

These tables are no longer needed with the new DAG system:

- `engine_ontology_workflows` - Replaced by `engine_ontology_dag`
- `engine_relationship_candidates` - No user review step; relationships saved directly
- `engine_workflow_state` - Node state now in `engine_dag_nodes`

---

## Section 4: Service Layer

### New Service: `OntologyDAGService`

```go
// pkg/services/ontology_dag_service.go

type OntologyDAGService interface {
    // Start initiates a new DAG execution (or returns existing running DAG)
    Start(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyDAG, error)

    // GetStatus returns the current DAG status with all node states
    GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)

    // Cancel cancels a running DAG
    Cancel(ctx context.Context, dagID uuid.UUID) error

    // Shutdown gracefully stops all DAGs owned by this server
    Shutdown(ctx context.Context) error
}
```

### Node Executor Interface

```go
// pkg/services/dag/node_executor.go

type NodeExecutor interface {
    // Name returns the node name (e.g., "EntityDiscovery")
    Name() string

    // Execute runs the node's work
    Execute(ctx context.Context, dag *models.OntologyDAG) error

    // OnProgress is called to report progress updates
    OnProgress(current, total int, message string)
}
```

### Node Implementations

Each node wraps existing service methods:

| Node | Wraps |
|------|-------|
| `EntityDiscoveryNode` | `entity_discovery_service.identifyEntitiesFromDDL()` |
| `EntityEnrichmentNode` | `entity_discovery_service.enrichEntitiesWithLLM()` |
| `RelationshipDiscoveryNode` | `deterministic_relationship_service.DiscoverRelationships()` |
| `RelationshipEnrichmentNode` | `relationship_enrichment_service.EnrichProject()` |
| `OntologyFinalizationNode` | `ontology_finalization.Finalize()` |
| `ColumnEnrichmentNode` | `column_enrichment.EnrichProject()` |

---

## Section 5: Existing Code to Leverage

### Service Layer

The DAG implementation should reuse existing services rather than reimplementing logic:

#### Entity Discovery
- **File**: `pkg/services/entity_discovery_service.go`
- **What to use**: Split `IdentifyAndEnrichEntities()` into two phases:
  - Phase 1 (deterministic): `identifyEntitiesFromDDL()` - parse schema for PKs/unique constraints
  - Phase 2 (LLM): `enrichEntitiesWithLLM()` - generate names and descriptions
- **Note**: Current implementation combines both phases; DAG nodes should call them separately

#### Relationship Discovery
- **File**: `pkg/services/deterministic_relationship_service.go`
- **What to use**: `DiscoverRelationships(ctx, projectID, datasourceID)` - discovers FK relationships and PK matches
- **Output**: Saves relationships to `engine_schema_relationships` table

#### Relationship Enrichment
- **File**: `pkg/services/relationship_enrichment.go`
- **What to use**: `EnrichProject(ctx, projectID)` - generates LLM descriptions for all relationships
- **Repository**: `EntityRelationshipRepository.UpdateDescription(ctx, id, description)`
- **LLM prompt**: Returns object-wrapped response: `{"relationships": [...]}`

#### Ontology Finalization
- **File**: `pkg/services/ontology_finalization.go`
- **What to use**: `Finalize(ctx, projectID, datasourceID)` - generates domain summary and detects conventions
- **Output**: Updates `engine_ontologies.domain_summary` JSONB field

#### Column Enrichment
- **File**: `pkg/services/column_enrichment.go`
- **What to use**: `EnrichProject(ctx, projectID)` - generates column metadata (descriptions, semantic types, enums)
- **LLM prompt**: Returns object-wrapped response: `{"columns": [...]}`
- **Output**: Updates `engine_ontologies.column_details` JSONB field

### LLM Utilities

#### Response Parsing
- **File**: `pkg/llm/json.go`
- **Functions**:
  - `ParseJSONResponse[T any](response string) (T, error)` - Unmarshal JSON to typed struct
  - `ExtractJSON(text string) (string, error)` - Extract JSON from markdown code blocks or surrounding text

#### Error Handling
- **File**: `pkg/llm/errors.go`
- **Functions**:
  - `IsRetryable(err error) bool` - Classifies errors as retryable or permanent
  - Retryable: deadline exceeded, context canceled, HTTP 5xx, rate limits
  - Permanent: invalid JSON, malformed responses, HTTP 4xx (except 429)

#### Client Configuration
- **File**: `pkg/llm/client.go`
- **Constants**:
  - `DefaultRequestTimeout = 5 * time.Minute` - Prevents infinite hangs when LLM server crashes
  - HTTP client has timeout set to prevent deadlocks

### Retry Utilities

- **File**: `pkg/retry/retry.go`
- **Function**: `DoWithResult[T any](ctx, fn func() (T, error), opts ...Option) (T, error)`
- **Default config**:
  - `DefaultMaxRetries = 3`
  - `DefaultInitialInterval = 100 * time.Millisecond`
  - `DefaultMaxInterval = 5 * time.Second`
  - `DefaultMultiplier = 2.0`
- **Usage**: Wrap node execution in retry logic for transient failures

### Repository Methods

#### EntityRelationshipRepository
- **File**: `pkg/repositories/entity_relationship_repository.go`
- **Methods**:
  - `GetByProject(ctx, projectID) ([]*models.EntityRelationship, error)` - Get all relationships for a project
  - `UpdateDescription(ctx, id, description string) error` - Update relationship description

#### SchemaRepository
- **File**: `pkg/repositories/schema_repository.go`
- **Methods**:
  - `GetRelationships(ctx, projectID, datasourceID) ([]*models.SchemaRelationship, error)` - Get FK relationships from schema

### Service Wiring Pattern

To avoid circular dependencies, use setter methods for cross-service dependencies:

```go
// From main.go (lines 280-281)
relationshipWorkflowService.SetColumnEnrichmentService(columnEnrichmentService)
relationshipWorkflowService.SetRelationshipEnrichmentService(relationshipEnrichmentService)
```

This pattern allows services to reference each other without creating import cycles.

### LLM Response Format Standard

**All LLM prompts should request object-wrapped responses** (not raw arrays):

```json
// ✅ CORRECT - Object-wrapped
{
  "columns": [...],
  "entities": [...],
  "relationships": [...]
}

// ❌ WRONG - Raw array
[...]
```

**Why**: Object wrappers make responses easier to extend (e.g., adding metadata) and more consistent to parse.

**Files using this pattern**:
- `pkg/services/relationship_enrichment.go` - `{"relationships": [...]}`
- `pkg/services/column_enrichment.go` - `{"columns": [...]}`

---

## Section 6: API

### Endpoints

**Start/Refresh Extraction:**
```
POST /api/projects/{pid}/datasources/{dsid}/ontology/extract
```

Response:
```json
{
  "dag_id": "uuid",
  "status": "running",
  "current_node": "EntityDiscovery",
  "nodes": [
    {"name": "EntityDiscovery", "status": "running", "progress": {"current": 3, "total": 15}},
    {"name": "EntityEnrichment", "status": "pending"},
    {"name": "RelationshipDiscovery", "status": "pending"},
    {"name": "RelationshipEnrichment", "status": "pending"},
    {"name": "OntologyFinalization", "status": "pending"},
    {"name": "ColumnEnrichment", "status": "pending"}
  ]
}
```

**Get Status (for polling):**
```
GET /api/projects/{pid}/datasources/{dsid}/ontology/dag
```

Returns same structure as above.

**Cancel:**
```
POST /api/projects/{pid}/datasources/{dsid}/ontology/dag/cancel
```

---

## Section 7: UI Changes

### Ontology Page

Replace current workflow status with DAG visualization:

```
┌────────────────────────────────────────────────────────────────┐
│ Ontology Extraction                          [Refresh Ontology]│
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [✓] Entity Discovery ────────────────────────────────────────│
│      Found 15 entities from schema                             │
│                                                                 │
│  [✓] Entity Enrichment ───────────────────────────────────────│
│      Generated names and descriptions                          │
│                                                                 │
│  [▶] Relationship Discovery ──────────────────────────────────│
│      Discovering FK relationships... (5/25)                    │
│                                                                 │
│  [○] Relationship Enrichment                                    │
│                                                                 │
│  [○] Ontology Finalization                                     │
│                                                                 │
│  [○] Column Enrichment                                         │
│                                                                 │
│  ──────────────────────────────────────────────────────────── │
│  Status: Running (2/6 nodes complete)         [Cancel]         │
└────────────────────────────────────────────────────────────────┘

Legend: [✓] Complete  [▶] Running  [○] Pending  [✗] Failed
```

### Button States

| State | Button Text | Action |
|-------|-------------|--------|
| No ontology | **Start Extraction** | Start DAG |
| DAG running | **Cancel** | Cancel DAG |
| DAG completed | **Refresh Ontology** | Start new DAG |
| DAG failed | **Retry** | Resume from failed node |

### Polling

- Poll `/ontology/dag` every 2 seconds while DAG is running
- Stop polling when status is `completed`, `failed`, or `cancelled`

### Remove from UI

- **Entities page**: Remove "Discover Entities" button (just show results)
- **Relationships page**: Remove "Detect Relationships" button (just show results)
- Both pages link to Ontology page for refresh

---

## Section 8: Implementation Phases

### Phase 1: Database & Models ✅ COMPLETED

**Tasks:**
1. ✅ Create migration for `engine_ontology_dag` and `engine_dag_nodes` tables
2. ✅ Create migration to drop `engine_ontology_workflows`, `engine_relationship_candidates`, `engine_workflow_state`
3. ✅ Create Go models in `pkg/models/ontology_dag.go`
4. ✅ Create repository in `pkg/repositories/ontology_dag_repository.go`
5. ✅ Create integration tests in `pkg/repositories/ontology_dag_repository_test.go`

**Files Created:**
```
migrations/
  023_create_ontology_dag.up.sql    # Creates engine_ontology_dag and engine_dag_nodes tables, drops legacy tables
  023_create_ontology_dag.down.sql  # Rollback: drops DAG tables, recreates legacy tables
pkg/models/
  ontology_dag.go                   # OntologyDAG, DAGNode models with status types and node names
pkg/repositories/
  ontology_dag_repository.go        # Full CRUD + ownership + node operations
  ontology_dag_repository_test.go   # Integration tests (build tag: integration)
```

**Implementation Notes for Future Sessions:**

1. **Model Constants**: The `pkg/models/ontology_dag.go` file defines:
   - `DAGStatus` enum: pending, running, completed, failed, cancelled
   - `DAGNodeStatus` enum: pending, running, completed, failed, skipped
   - `DAGNodeName` constants for all 6 nodes with `DAGNodeOrder` map
   - `AllDAGNodes()` helper returns nodes in execution order
   - `DAGNodeProgress` struct for tracking current/total/message

2. **Repository Interface**: The `OntologyDAGRepository` interface provides:
   - DAG CRUD: Create, GetByID, GetByIDWithNodes, GetLatestByDatasource, GetActiveByDatasource, Update, UpdateStatus, Delete, DeleteByProject
   - Ownership: ClaimOwnership, UpdateHeartbeat, ReleaseOwnership (for multi-server robustness)
   - Nodes: CreateNodes, GetNodesByDAG, UpdateNodeStatus, UpdateNodeProgress, IncrementNodeRetryCount, GetNextPendingNode

3. **Database Schema**:
   - Partial unique index ensures only one active DAG per datasource
   - RLS policies applied to both tables
   - Trigger updates `updated_at` automatically
   - Heartbeat index for efficient stale DAG detection

4. **Test Pattern**: Tests use `testhelpers.GetEngineDB(t)` shared container pattern. Run with `go test -tags=integration ./pkg/repositories/...`

### Phase 2: DAG Service & Nodes ✅ COMPLETED

**Tasks:**
1. ✅ Create `OntologyDAGService` interface and implementation
2. ✅ Create `NodeExecutor` interface with `BaseNode` for common functionality
3. ✅ Create 6 node executors wrapping existing service methods
4. ✅ Wire into `main.go` using setter pattern for cross-service dependencies
5. ✅ Create adapter layer to bridge services and dag package (avoids import cycles)
6. ✅ Add unit tests for node ordering, interface contracts, and execution context

**Files Created:**
```
pkg/services/
  ontology_dag_service.go      # Main DAG orchestrator (Start, GetStatus, Cancel, Shutdown)
  ontology_dag_service_test.go # Unit tests for node ordering and interface contracts
  dag_adapters.go              # Adapter layer bridging services to dag package interfaces
  dag/
    node_executor.go                   # NodeExecutor interface + BaseNode with progress reporting
    entity_discovery_node.go           # Wraps identifyEntitiesFromDDL()
    entity_enrichment_node.go          # Wraps enrichEntitiesWithLLM()
    relationship_discovery_node.go     # Wraps DeterministicRelationshipService.DiscoverRelationships()
    relationship_enrichment_node.go    # Wraps RelationshipEnrichmentService.EnrichProject()
    ontology_finalization_node.go      # Wraps OntologyFinalizationService.Finalize()
    column_enrichment_node.go          # Wraps ColumnEnrichmentService.EnrichProject()
```

**Implementation Notes for Future Sessions:**

1. **Adapter Pattern**: The `dag_adapters.go` file contains adapters that convert between the services package types and the dag package interfaces. This allows the dag package to remain independent of the services package, avoiding import cycles.

2. **Service Method Setters**: `OntologyDAGService` uses setter methods (e.g., `SetEntityDiscoveryMethods()`) to receive dependencies. These are called in `main.go` after all services are constructed.

3. **DAG Execution Flow**:
   - `Start()` → Creates DAG + 6 nodes → Claims ownership → Starts heartbeat → Spawns goroutine
   - Goroutine executes nodes sequentially with retry logic via `pkg/retry`
   - Each node updates its status (pending → running → completed/failed)
   - `GetStatus()` returns current DAG with all node states for UI polling

4. **Ownership & Heartbeat**: The service tracks a `serverInstanceID` and maintains a heartbeat (every 30s) for multi-server robustness. This allows detecting stale DAGs if a server crashes.

5. **Progress Reporting**: Each node has `SetCurrentNodeID()` which enables `ReportProgress()` calls to update the database with current/total/message for UI visibility.

6. **Retry Integration**: Node execution wraps calls in `retry.DoIfRetryable()` which uses `llm.IsRetryable()` to determine if errors should trigger retry.

7. **main.go Wiring**: The DAG service is created and all adapters are wired but marked with `_ = ontologyDAGService` since handlers are added in Phase 3.

### Phase 3: API & Handler

**Tasks:**
1. Create handler for DAG endpoints
2. Register routes in `main.go`
3. Remove old workflow endpoints

**Files:**
```
pkg/handlers/
  ontology_dag_handler.go
```

### Phase 4: UI

**Tasks:**
1. Create DAG visualization component
2. Update OntologyPage to use new API
3. Remove workflow buttons from Entities/Relationships pages
4. Add polling for real-time updates

**Files:**
```
ui/src/
  components/
    OntologyDAG.tsx
  pages/
    OntologyPage.tsx  (update)
    EntitiesPage.tsx  (update)
    RelationshipsPage.tsx  (update)
```

### Phase 5: Cleanup

**Tasks:**
1. Remove old service files no longer needed
2. Remove old handler files
3. Update tests

**Files to remove:**
```
pkg/services/
  entity_discovery_service.go  (keep methods, remove workflow logic)
  relationship_workflow.go     (remove entirely)
  ontology_workflow.go         (remove entirely)
pkg/handlers/
  entity_discovery_handler.go  (remove workflow endpoints)
  relationship_workflow_handler.go  (remove entirely)
```

---

## Section 9: Execution Flow Example

### Fresh Extraction

```
User clicks [Start Extraction]
    │
    ▼
POST /ontology/extract
    │
    ▼
Create DAG record (status: running)
Create 5 node records (status: pending)
    │
    ▼
[1] EntityDiscovery starts
    - Query engine_schema_columns for PKs
    - Create OntologyEntity records
    - Node status: completed
    │
    ▼
[2] EntityEnrichment starts
    - Call LLM for each entity
    - Update entity names/descriptions
    - Node status: completed
    │
    ▼
[3] RelationshipDiscovery starts
    - Query engine_schema_columns for FKs
    - Create engine_schema_relationships records
    - Node status: completed
    │
    ▼
[4] RelationshipEnrichment starts
    - Call LLM for relationship descriptions
    - Update engine_entity_relationships.description
    - Node status: completed
    │
    ▼
[5] OntologyFinalization starts
    - Call LLM for domain summary
    - Detect conventions (soft delete, audit columns)
    - Update engine_ontologies.domain_summary
    - Node status: completed
    │
    ▼
[6] ColumnEnrichment starts
    - For each table, call LLM for column metadata
    - Update engine_ontologies.column_details
    - Node status: completed
    │
    ▼
DAG status: completed
User sees success in UI
```

### Resume After Failure

```
ColumnEnrichment failed at table 25/38
DAG status: failed
    │
    ▼
User clicks [Retry]
    │
    ▼
POST /ontology/extract
    │
    ▼
Find existing failed DAG
Skip nodes 1-5 (already completed)
Resume ColumnEnrichment from table 26
    │
    ▼
ColumnEnrichment completes
DAG status: completed
```

---

## Appendix A: Database Queries

### Get DAG status with nodes

```sql
SELECT
    d.id, d.status, d.current_node, d.started_at,
    json_agg(json_build_object(
        'name', n.node_name,
        'status', n.status,
        'progress', n.progress,
        'error', n.error_message
    ) ORDER BY n.node_order) AS nodes
FROM engine_ontology_dag d
LEFT JOIN engine_dag_nodes n ON d.id = n.dag_id
WHERE d.datasource_id = $1
ORDER BY d.created_at DESC
LIMIT 1
GROUP BY d.id;
```

### Find next node to execute

```sql
SELECT node_name
FROM engine_dag_nodes
WHERE dag_id = $1
  AND status = 'pending'
ORDER BY node_order
LIMIT 1;
```

---

## Appendix B: Code Structure

**New files:**
```
pkg/
  models/
    ontology_dag.go              # OntologyDAG, DAGNode models
  repositories/
    ontology_dag_repository.go   # CRUD for dag tables
  services/
    ontology_dag_service.go      # Main orchestrator
    dag/
      node_executor.go              # Interface
      entity_discovery_node.go
      entity_enrichment_node.go
      relationship_discovery_node.go
      relationship_enrichment_node.go
      ontology_finalization_node.go
      column_enrichment_node.go
  handlers/
    ontology_dag_handler.go      # HTTP endpoints

ui/src/
  components/
    OntologyDAG.tsx              # DAG visualization
```

**Files to delete:**
```
pkg/services/
  relationship_workflow.go
  ontology_workflow.go
pkg/handlers/
  relationship_workflow_handler.go
```

**Files to modify:**
```
pkg/services/
  entity_discovery_service.go    # Extract methods, remove workflow
  relationship_enrichment.go     # Keep as-is (already suitable)
  column_enrichment.go           # Keep as-is (already suitable)
  ontology_finalization.go       # Keep as-is (already suitable)
main.go                          # Wire new service using setter pattern
```

---

## Appendix C: Session Learnings (2026-01-03)

This appendix documents key implementation learnings that should guide DAG development.

### Node 4: Relationship Enrichment Added

**Why**: Relationships discovered by node 3 are deterministic (FK/PK matching) and lack semantic meaning. Node 4 enriches them with LLM-generated descriptions.

**Implementation**:
- Service: `pkg/services/relationship_enrichment.go`
- Method: `EnrichProject(ctx, projectID)`
- Repository: `EntityRelationshipRepository.UpdateDescription(ctx, id, description)`
- LLM response format: `{"relationships": [{"id": "uuid", "description": "..."}]}`

**Node order updated**:
1. EntityDiscovery (deterministic)
2. EntityEnrichment (LLM)
3. RelationshipDiscovery (deterministic)
4. **RelationshipEnrichment (LLM)** ← NEW
5. OntologyFinalization (LLM)
6. ColumnEnrichment (LLM)

### LLM Timeout Configuration

**Problem**: LLM server crashes caused infinite hangs in HTTP requests.

**Solution**: `pkg/llm/client.go` now has `DefaultRequestTimeout = 5 * time.Minute` to prevent deadlocks.

**Error handling**: `pkg/llm/errors.go` classifies "deadline exceeded" and "context canceled" as retryable errors via `IsRetryable(err error) bool`.

### Retry Strategy

**Package**: `pkg/retry/retry.go`

**Function**: `DoWithResult[T any](ctx, fn func() (T, error), opts ...Option) (T, error)`

**Default configuration**:
- `DefaultMaxRetries = 3`
- `DefaultInitialInterval = 100ms`
- `DefaultMaxInterval = 5s`
- `DefaultMultiplier = 2.0` (exponential backoff)

**Usage**: DAG nodes should wrap execution in retry logic for transient failures (network issues, LLM timeouts, rate limits).

### LLM Response Format Standard

**Rule**: All LLM prompts must request object-wrapped responses (not raw arrays).

**Rationale**:
- Easier to extend with metadata
- More consistent parsing
- Clearer structure

**Examples**:
```json
// ✅ CORRECT
{"relationships": [...], "entities": [...], "columns": [...]}

// ❌ WRONG
[...]
```

**Files already using this pattern**:
- `pkg/services/relationship_enrichment.go`
- `pkg/services/column_enrichment.go`

### Service Wiring Pattern

**Problem**: Circular dependencies when services reference each other.

**Solution**: Use setter methods for cross-service dependencies.

**Example from `main.go`**:
```go
relationshipWorkflowService.SetColumnEnrichmentService(columnEnrichmentService)
relationshipWorkflowService.SetRelationshipEnrichmentService(relationshipEnrichmentService)
```

**Why**: Allows services to reference each other without creating import cycles.

### Repository Methods for DAG Implementation

**EntityRelationshipRepository** (`pkg/repositories/entity_relationship_repository.go`):
- `GetByProject(ctx, projectID) ([]*models.EntityRelationship, error)` - Fetch all relationships for a project
- `UpdateDescription(ctx, id, description string) error` - Update relationship description after LLM enrichment

**SchemaRepository** (`pkg/repositories/schema_repository.go`):
- `GetRelationships(ctx, projectID, datasourceID) ([]*models.SchemaRelationship, error)` - Get FK relationships from schema

### LLM Utility Functions

**JSON parsing** (`pkg/llm/json.go`):
- `ParseJSONResponse[T any](response string) (T, error)` - Unmarshal JSON to typed struct with error handling
- `ExtractJSON(text string) (string, error)` - Extract JSON from markdown code blocks or surrounding text

**Error classification** (`pkg/llm/errors.go`):
- `IsRetryable(err error) bool` - Determines if error should trigger retry
- Retryable: deadline exceeded, context canceled, HTTP 5xx, rate limits (429)
- Permanent: invalid JSON, malformed responses, HTTP 4xx (except 429)

### State Detection Logic Update

**New check** for node 4 (RelationshipEnrichment):
```go
// Relationships not enriched (missing descriptions)
if CountRelationshipsWithoutDescription(projectID) > 0 {
    return "RelationshipEnrichment"
}
```

This check runs after RelationshipDiscovery but before OntologyFinalization.

### Implementation Checklist

When implementing the DAG system, ensure:

1. **All 6 nodes are created** (not 5) - RelationshipEnrichment is mandatory
2. **Retry logic uses `pkg/retry`** - Don't reimplement exponential backoff
3. **LLM responses are object-wrapped** - Update prompts to match standard format
4. **Services are wired with setters** - Avoid circular dependency issues
5. **LLM timeout is configured** - Use `DefaultRequestTimeout` constant
6. **Error classification uses `IsRetryable`** - Don't guess which errors to retry
7. **JSON parsing uses `pkg/llm/json.go`** - Don't use raw `json.Unmarshal`
```

# PLAN: Continue Ontology Processing After Server Restart

## Context

When the server crashes or restarts during ontology extraction, the DAG remains stuck in "running" status with no process to continue it. All state is persisted to the database, but there's no recovery logic on server startup to resume interrupted workflows.

### Current Architecture

- **State persistence**: Complete - all DAG/node state in `engine_ontology_dag` and `engine_dag_nodes` tables
- **Execution model**: Per-DAG goroutine spawned on-demand, dies with server
- **Ownership tracking**: `owner_id` and `last_heartbeat` prevent concurrent execution but don't enable recovery
- **Node skip logic**: Already exists (`pkg/services/ontology_dag_service.go:533-535`) - completed nodes are skipped if execution restarts

### What's Lost on Crash
- The goroutine running the DAG execution
- In-progress LLM calls and partial node results not yet committed
- Cancellation contexts stored in `activeDAGs` sync.Map

### What Survives
- DAG status and current_node
- All node statuses (pending/running/completed/failed)
- All ontology data written during completed nodes (entities, relationships, etc.)
- Retry counts, error messages, timing information

## Implementation Steps

### Step 1: Define Orphan Detection Logic

Add method to `OntologyDAGRepository` to find orphaned DAGs:

```go
// FindOrphanedDAGs returns DAGs that are "running" but have stale heartbeats
// A DAG is orphaned if:
// - status = 'running'
// - last_heartbeat < (now - threshold) OR last_heartbeat IS NULL
func (r *OntologyDAGRepository) FindOrphanedDAGs(ctx context.Context, heartbeatThreshold time.Duration) ([]models.OntologyDAG, error)
```

**Threshold**: Should be 2-3x the heartbeat interval (currently 30s), so ~90 seconds.

**File**: `pkg/repositories/ontology_dag_repository.go`

### Step 2: Add Reclaim Ownership Method

Add method to atomically reclaim ownership of an orphaned DAG:

```go
// ReclaimOwnership attempts to take ownership of an orphaned DAG
// Uses optimistic locking via last_heartbeat to prevent race conditions
// Returns true if ownership was successfully claimed
func (r *OntologyDAGRepository) ReclaimOwnership(ctx context.Context, dagID, newOwnerID uuid.UUID, staleThreshold time.Duration) (bool, error)
```

**SQL pattern**:
```sql
UPDATE engine_ontology_dag
SET owner_id = $1, last_heartbeat = NOW()
WHERE id = $2
  AND status = 'running'
  AND (last_heartbeat < NOW() - $3 OR last_heartbeat IS NULL)
```

**File**: `pkg/repositories/ontology_dag_repository.go`

### Step 3: Reset Current Node to Pending

When resuming, the node that was "running" at crash time needs to be retried:

```go
// ResetRunningNodeToPending resets any node with status='running' back to 'pending'
// Called before resuming a reclaimed DAG to ensure clean retry
func (r *OntologyDAGRepository) ResetRunningNodeToPending(ctx context.Context, dagID uuid.UUID) error
```

**Rationale**: The running node may have partially executed. Resetting to pending allows the existing retry logic to handle it cleanly.

**File**: `pkg/repositories/ontology_dag_repository.go`

### Step 4: Add Resume Method to DAG Service

Add method to resume an orphaned DAG:

```go
// ResumeOrphanedDAG attempts to reclaim and resume an orphaned DAG
// Returns error if DAG cannot be reclaimed (already reclaimed by another server)
func (s *OntologyDAGService) ResumeOrphanedDAG(ctx context.Context, projectID, dagID uuid.UUID) error
```

**Logic**:
1. Attempt to reclaim ownership (fail if already reclaimed)
2. Reset any "running" node to "pending"
3. Spawn goroutine with `executeDAG()` (existing method)
4. The existing node-skip logic handles completed nodes automatically

**File**: `pkg/services/ontology_dag_service.go`

### Step 5: Add Startup Recovery Scan

Add recovery logic that runs once on server startup:

```go
// RecoverOrphanedDAGs scans for and resumes any orphaned DAGs
// Called once during server initialization
func (s *OntologyDAGService) RecoverOrphanedDAGs(ctx context.Context) error
```

**Logic**:
1. Query `FindOrphanedDAGs()` with threshold
2. For each orphaned DAG:
   - Log recovery attempt
   - Call `ResumeOrphanedDAG()`
   - Log success/failure
3. Return aggregate error if any recoveries failed

**File**: `pkg/services/ontology_dag_service.go`

### Step 6: Wire Recovery into Server Startup

Call recovery during server initialization in `main.go`:

```go
// After services are initialized, before HTTP server starts
if err := ontologyDAGService.RecoverOrphanedDAGs(ctx); err != nil {
    logger.Error("Failed to recover orphaned DAGs", "error", err)
    // Don't fail startup - just log the error
}
```

**File**: `main.go` (in initialization sequence)

## Idempotency Considerations

### LLM Calls
- LLM calls during Column Enrichment, Entity Enrichment, etc. are not idempotent
- Duplicate calls will result in duplicate API charges but not data corruption
- Ontology data writes use upserts (ON CONFLICT), so re-running is safe

### Node-Level Idempotency
- Each node should be designed to be safely re-runnable
- Completed work is tracked at node granularity, not sub-task
- If a node was 15/38 through processing, it restarts at 0/38 on resume

### Future Enhancement: Sub-Task Tracking
For expensive nodes (like Column Enrichment with many tables), could track per-table completion:
- Add `completed_items JSONB` column to `engine_dag_nodes`
- Store which tables/entities have been processed
- Skip already-processed items on resume

This is optional and adds complexity - the simpler approach (re-run entire node) works correctly, just with potential duplicate LLM calls.

## Testing

1. **Unit tests**: Mock repository, verify recovery logic
2. **Integration test**:
   - Start DAG extraction
   - Kill server mid-execution (or simulate by clearing `activeDAGs` map)
   - Restart server
   - Verify DAG resumes and completes
3. **Multi-server test**:
   - Verify only one server reclaims an orphaned DAG
   - Verify heartbeat prevents false orphan detection

## Files to Modify/Create

| File | Changes |
|------|---------|
| `pkg/repositories/ontology_dag_repository.go` | Add `FindOrphanedDAGs`, `ReclaimOwnership`, `ResetRunningNodeToPending` |
| `pkg/services/ontology_dag_service.go` | Add `ResumeOrphanedDAG`, `RecoverOrphanedDAGs` |
| `main.go` | Call `RecoverOrphanedDAGs` during startup |
| `pkg/repositories/ontology_dag_repository_test.go` | Tests for new repository methods |
| `pkg/services/ontology_dag_service_test.go` | Tests for recovery logic |

## Configuration

Consider making these configurable via environment/config:

| Setting | Default | Description |
|---------|---------|-------------|
| `ORPHAN_HEARTBEAT_THRESHOLD` | 90s | Time since last heartbeat to consider DAG orphaned |
| `ENABLE_ORPHAN_RECOVERY` | true | Feature flag to disable recovery if needed |

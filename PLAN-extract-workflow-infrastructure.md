# Plan: Extract Shared Workflow Infrastructure

## Problem

Three workflow services (`ontologyWorkflowService`, `relationshipWorkflowService`, `entityDiscoveryService`) each implement nearly identical infrastructure code for:

1. **Heartbeat management** - Periodic updates to maintain workflow ownership
2. **Task queue writer** - Debounced, batched persistence of task queue state
3. **Queue setup with callbacks** - Converting workqueue status to model status

This results in ~400 lines of duplicated code across the three services.

## Duplication Evidence

### Heartbeat Pattern (identical across all 3)

```go
// ontology_workflow.go:896-943
func (s *ontologyWorkflowService) startHeartbeat(workflowID, projectID uuid.UUID) {
    stop := make(chan struct{})
    info := &heartbeatInfo{projectID: projectID, stop: stop}
    s.heartbeatStop.Store(workflowID, info)
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-stop:
                return
            case <-ticker.C:
                // Update heartbeat...
            }
        }
    }()
}

// relationship_workflow.go:1629-1676 - IDENTICAL
// entity_discovery_service.go:867-910 - IDENTICAL
```

### TaskQueueWriter Pattern (identical across all 3)

```go
// ontology_workflow.go:64-67, 668-737
type taskQueueWriter struct {
    updates chan taskQueueUpdate
    done    chan struct{}
}

func (s *ontologyWorkflowService) runTaskQueueWriter(writer *taskQueueWriter) {
    defer close(writer.done)
    for {
        update, ok := <-writer.updates
        if !ok { return }
        // Drain and persist latest...
    }
}

// relationship_workflow.go:1501-1569 - IDENTICAL
// entity_discovery_service.go:795-863 - IDENTICAL
```

### Struct Fields (identical across all 3)

```go
// All three services have these fields:
serverInstanceID uuid.UUID
activeQueues     sync.Map // workflowID -> *workqueue.Queue
taskQueueWriters sync.Map // workflowID -> *taskQueueWriter
heartbeatStop    sync.Map // workflowID -> *heartbeatInfo
```

## Solution

Create a new `pkg/services/workflow` package with shared infrastructure that all three services can embed or compose.

## Design Decisions

### Option A: Composition (Recommended)

Services embed a `WorkflowInfra` struct that provides the shared functionality:

```go
type ontologyWorkflowService struct {
    // ... service-specific fields
    infra *workflow.WorkflowInfra
}
```

**Pros:**
- Clear separation of concerns
- Easy to understand
- Services remain independent
- Minimal refactoring of existing logic

**Cons:**
- Need to pass infra to methods or access via s.infra

### Option B: Interface Embedding

Define interface that services implement, extract shared logic.

**Pros:**
- More Go-idiomatic

**Cons:**
- More complex
- Harder to understand
- Services become more tightly coupled

**Decision: Use Option A (Composition)**

## Implementation Steps

### Step 1: Create the workflow infrastructure package

**File:** `pkg/services/workflow/infra.go`

```go
package workflow

import (
    "context"
    "sync"
    "time"

    "github.com/google/uuid"
    "go.uber.org/zap"

    "github.com/ekaya-inc/ekaya-engine/pkg/models"
    "github.com/ekaya-inc/ekaya-engine/pkg/repositories"
    "github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// TenantContextFunc is the function type for acquiring tenant-scoped contexts.
type TenantContextFunc func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error)

// TaskQueueUpdate holds a pending task queue update.
type TaskQueueUpdate struct {
    ProjectID  uuid.UUID
    WorkflowID uuid.UUID
    Tasks      []models.WorkflowTask
}

// taskQueueWriter manages serialized writes for a single workflow.
type taskQueueWriter struct {
    updates chan TaskQueueUpdate
    done    chan struct{}
}

// heartbeatInfo holds info needed for heartbeat goroutine.
type heartbeatInfo struct {
    projectID uuid.UUID
    stop      chan struct{}
}

// WorkflowInfra provides shared infrastructure for workflow services.
// It handles heartbeat management, task queue persistence, and queue lifecycle.
type WorkflowInfra struct {
    workflowRepo     repositories.OntologyWorkflowRepository
    getTenantCtx     TenantContextFunc
    logger           *zap.Logger
    serverInstanceID uuid.UUID

    activeQueues     sync.Map // workflowID -> *workqueue.Queue
    taskQueueWriters sync.Map // workflowID -> *taskQueueWriter
    heartbeatStop    sync.Map // workflowID -> *heartbeatInfo
}

// NewWorkflowInfra creates a new workflow infrastructure instance.
func NewWorkflowInfra(
    workflowRepo repositories.OntologyWorkflowRepository,
    getTenantCtx TenantContextFunc,
    logger *zap.Logger,
) *WorkflowInfra {
    serverID := uuid.New()
    logger.Info("Workflow infrastructure initialized",
        zap.String("server_instance_id", serverID.String()))

    return &WorkflowInfra{
        workflowRepo:     workflowRepo,
        getTenantCtx:     getTenantCtx,
        logger:           logger,
        serverInstanceID: serverID,
    }
}

// ServerInstanceID returns this server's unique instance ID.
func (w *WorkflowInfra) ServerInstanceID() uuid.UUID {
    return w.serverInstanceID
}

// ============================================================================
// Heartbeat Management
// ============================================================================

// StartHeartbeat launches a background goroutine that periodically updates
// the workflow's last_heartbeat timestamp to maintain ownership.
func (w *WorkflowInfra) StartHeartbeat(workflowID, projectID uuid.UUID) {
    stop := make(chan struct{})
    info := &heartbeatInfo{
        projectID: projectID,
        stop:      stop,
    }
    w.heartbeatStop.Store(workflowID, info)

    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-stop:
                w.logger.Debug("Heartbeat stopped",
                    zap.String("workflow_id", workflowID.String()))
                return
            case <-ticker.C:
                ctx, cleanup, err := w.getTenantCtx(context.Background(), projectID)
                if err != nil {
                    w.logger.Error("Failed to acquire DB connection for heartbeat",
                        zap.String("workflow_id", workflowID.String()),
                        zap.Error(err))
                    continue
                }
                if err := w.workflowRepo.UpdateHeartbeat(ctx, workflowID, w.serverInstanceID); err != nil {
                    w.logger.Error("Failed to update heartbeat",
                        zap.String("workflow_id", workflowID.String()),
                        zap.Error(err))
                }
                cleanup()
            }
        }
    }()

    w.logger.Debug("Heartbeat started",
        zap.String("workflow_id", workflowID.String()),
        zap.String("server_instance_id", w.serverInstanceID.String()))
}

// StopHeartbeat stops the heartbeat goroutine for a workflow.
func (w *WorkflowInfra) StopHeartbeat(workflowID uuid.UUID) {
    if infoVal, ok := w.heartbeatStop.LoadAndDelete(workflowID); ok {
        info := infoVal.(*heartbeatInfo)
        close(info.stop)
    }
}

// ============================================================================
// Task Queue Writer
// ============================================================================

// StartTaskQueueWriter creates and starts a single writer goroutine for a workflow.
func (w *WorkflowInfra) StartTaskQueueWriter(workflowID uuid.UUID) {
    writer := &taskQueueWriter{
        updates: make(chan TaskQueueUpdate, 100),
        done:    make(chan struct{}),
    }
    w.taskQueueWriters.Store(workflowID, writer)
    go w.runTaskQueueWriter(writer)
}

// StopTaskQueueWriter closes the channel and waits for the writer to finish.
func (w *WorkflowInfra) StopTaskQueueWriter(workflowID uuid.UUID) {
    if writerVal, ok := w.taskQueueWriters.LoadAndDelete(workflowID); ok {
        writer := writerVal.(*taskQueueWriter)
        close(writer.updates)
        <-writer.done // Wait for writer to finish
    }
}

// SendTaskQueueUpdate sends a task queue update to the writer.
// Returns false if no writer exists for the workflow.
func (w *WorkflowInfra) SendTaskQueueUpdate(update TaskQueueUpdate) bool {
    writerVal, ok := w.taskQueueWriters.Load(update.WorkflowID)
    if !ok {
        return false
    }
    writer := writerVal.(*taskQueueWriter)
    select {
    case writer.updates <- update:
        return true
    default:
        w.logger.Warn("Task queue update buffer full, dropping update",
            zap.String("workflow_id", update.WorkflowID.String()))
        return false
    }
}

// runTaskQueueWriter is the single writer goroutine that processes updates.
// It drains all pending updates and only persists the latest one (debounce).
func (w *WorkflowInfra) runTaskQueueWriter(writer *taskQueueWriter) {
    defer close(writer.done)

    for {
        // Wait for at least one update
        update, ok := <-writer.updates
        if !ok {
            return // Channel closed, exit
        }

        // Drain any additional pending updates, keeping only the latest
        for {
            select {
            case newer, ok := <-writer.updates:
                if !ok {
                    // Channel closed while draining - persist what we have and exit
                    w.persistTaskQueue(update.ProjectID, update.WorkflowID, update.Tasks)
                    return
                }
                update = newer // Keep the newer update
            default:
                // No more pending updates, persist the latest
                goto persist
            }
        }

    persist:
        w.persistTaskQueue(update.ProjectID, update.WorkflowID, update.Tasks)
    }
}

// persistTaskQueue saves the task queue to the database.
func (w *WorkflowInfra) persistTaskQueue(projectID, workflowID uuid.UUID, tasks []models.WorkflowTask) {
    // Acquire a fresh DB connection since this runs in a goroutine
    ctx, cleanup, err := w.getTenantCtx(context.Background(), projectID)
    if err != nil {
        w.logger.Error("Failed to acquire DB connection for task queue update",
            zap.String("workflow_id", workflowID.String()),
            zap.Error(err))
        return
    }
    defer cleanup()

    if err := w.workflowRepo.UpdateTaskQueue(ctx, workflowID, tasks); err != nil {
        w.logger.Error("Failed to persist task queue",
            zap.String("workflow_id", workflowID.String()),
            zap.Error(err))
    }
}

// ============================================================================
// Queue Management
// ============================================================================

// StoreQueue stores a workqueue for a workflow.
func (w *WorkflowInfra) StoreQueue(workflowID uuid.UUID, queue *workqueue.Queue) {
    w.activeQueues.Store(workflowID, queue)
}

// LoadQueue retrieves a workqueue for a workflow.
func (w *WorkflowInfra) LoadQueue(workflowID uuid.UUID) (*workqueue.Queue, bool) {
    val, ok := w.activeQueues.Load(workflowID)
    if !ok {
        return nil, false
    }
    return val.(*workqueue.Queue), true
}

// DeleteQueue removes a workqueue for a workflow.
func (w *WorkflowInfra) DeleteQueue(workflowID uuid.UUID) {
    w.activeQueues.Delete(workflowID)
}

// ============================================================================
// Status Mapping
// ============================================================================

// MapTaskStatus converts a workqueue.TaskStatus to a models.WorkflowTaskStatus.
func MapTaskStatus(status workqueue.TaskStatus) string {
    switch status {
    case workqueue.TaskStatusPending:
        return "pending"
    case workqueue.TaskStatusRunning:
        return "running"
    case workqueue.TaskStatusCompleted:
        return "completed"
    case workqueue.TaskStatusFailed, workqueue.TaskStatusCancelled:
        return "failed"
    case workqueue.TaskStatusPaused:
        return "paused"
    default:
        return "unknown"
    }
}

// ============================================================================
// Graceful Shutdown
// ============================================================================

// ShutdownFunc is called for each active workflow during shutdown.
type ShutdownFunc func(workflowID, projectID uuid.UUID, queue *workqueue.Queue)

// Shutdown gracefully stops all active workflows.
// The provided shutdownFn is called for each active workflow to handle
// service-specific shutdown logic (e.g., marking workflow as failed).
func (w *WorkflowInfra) Shutdown(ctx context.Context, shutdownFn ShutdownFunc) error {
    w.activeQueues.Range(func(key, value any) bool {
        workflowID := key.(uuid.UUID)
        queue := value.(*workqueue.Queue)

        // Get project ID from heartbeat info if available
        var projectID uuid.UUID
        if infoVal, ok := w.heartbeatStop.Load(workflowID); ok {
            info := infoVal.(*heartbeatInfo)
            projectID = info.projectID
        }

        // Call service-specific shutdown logic
        if shutdownFn != nil {
            shutdownFn(workflowID, projectID, queue)
        }

        // Stop infrastructure
        w.StopTaskQueueWriter(workflowID)
        w.StopHeartbeat(workflowID)
        w.activeQueues.Delete(workflowID)

        return true
    })

    w.logger.Info("Workflow infrastructure shutdown complete")
    return nil
}
```

### Step 2: Update ontologyWorkflowService

**File:** `pkg/services/ontology_workflow.go`

#### 2a. Remove duplicated types

Remove (lines 55-73):
- `type taskQueueUpdate struct`
- `type taskQueueWriter struct`
- `type heartbeatInfo struct`

#### 2b. Update struct to use infra

```go
// BEFORE
type ontologyWorkflowService struct {
    workflowRepo   repositories.OntologyWorkflowRepository
    // ... other repos
    getTenantCtx   TenantContextFunc
    logger         *zap.Logger
    activeQueues   sync.Map
    taskQueueWriters sync.Map
    serverInstanceID uuid.UUID
    heartbeatStop    sync.Map
}

// AFTER
type ontologyWorkflowService struct {
    workflowRepo   repositories.OntologyWorkflowRepository
    // ... other repos (keep these)
    logger         *zap.Logger
    infra          *workflow.WorkflowInfra
}
```

#### 2c. Update constructor

```go
// BEFORE
func NewOntologyWorkflowService(...) OntologyWorkflowService {
    serverID := uuid.New()
    // ...
    return &ontologyWorkflowService{
        // ...
        serverInstanceID: serverID,
    }
}

// AFTER
func NewOntologyWorkflowService(...) OntologyWorkflowService {
    infra := workflow.NewWorkflowInfra(workflowRepo, getTenantCtx, logger.Named("ontology-workflow"))
    return &ontologyWorkflowService{
        // ... keep repos
        logger: logger.Named("ontology-workflow"),
        infra:  infra,
    }
}
```

#### 2d. Update method calls

Replace throughout the file:
```go
// BEFORE
s.startHeartbeat(workflowID, projectID)
s.stopHeartbeat(workflowID)
s.startTaskQueueWriter(workflowID)
s.stopTaskQueueWriter(workflowID)
s.activeQueues.Store(workflowID, queue)
s.serverInstanceID

// AFTER
s.infra.StartHeartbeat(workflowID, projectID)
s.infra.StopHeartbeat(workflowID)
s.infra.StartTaskQueueWriter(workflowID)
s.infra.StopTaskQueueWriter(workflowID)
s.infra.StoreQueue(workflowID, queue)
s.infra.ServerInstanceID()
```

#### 2e. Update queue.SetOnUpdate callback

```go
// BEFORE
queue.SetOnUpdate(func(snapshots []workqueue.TaskSnapshot) {
    tasks := make([]models.WorkflowTask, 0, len(snapshots))
    for _, snap := range snapshots {
        var status string
        switch snap.Status {
        case workqueue.TaskStatusPending:
            status = "pending"
        // ... etc
        }
        tasks = append(tasks, models.WorkflowTask{...})
    }
    // Send to writer...
})

// AFTER
queue.SetOnUpdate(func(snapshots []workqueue.TaskSnapshot) {
    tasks := make([]models.WorkflowTask, 0, len(snapshots))
    for _, snap := range snapshots {
        tasks = append(tasks, models.WorkflowTask{
            Name:   snap.Name,
            Status: workflow.MapTaskStatus(snap.Status),
        })
    }
    s.infra.SendTaskQueueUpdate(workflow.TaskQueueUpdate{
        ProjectID:  projectID,
        WorkflowID: workflowID,
        Tasks:      tasks,
    })
})
```

#### 2f. Remove duplicated methods

Remove (lines 668-743, 896-943):
- `func (s *ontologyWorkflowService) startTaskQueueWriter(...)`
- `func (s *ontologyWorkflowService) stopTaskQueueWriter(...)`
- `func (s *ontologyWorkflowService) runTaskQueueWriter(...)`
- `func (s *ontologyWorkflowService) persistTaskQueue(...)`
- `func (s *ontologyWorkflowService) startHeartbeat(...)`
- `func (s *ontologyWorkflowService) stopHeartbeat(...)`

#### 2g. Update Shutdown method

```go
// BEFORE (complex, duplicated)
func (s *ontologyWorkflowService) Shutdown(ctx context.Context) error {
    s.activeQueues.Range(func(key, value any) bool {
        workflowID := key.(uuid.UUID)
        queue := value.(*workqueue.Queue)
        // ... service-specific logic
        s.stopTaskQueueWriter(workflowID)
        s.stopHeartbeat(workflowID)
        return true
    })
    return nil
}

// AFTER (simple delegation)
func (s *ontologyWorkflowService) Shutdown(ctx context.Context) error {
    return s.infra.Shutdown(ctx, func(workflowID, projectID uuid.UUID, queue *workqueue.Queue) {
        // Service-specific shutdown logic
        queue.Cancel()
        if projectID != uuid.Nil {
            ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
            if err == nil {
                _ = s.workflowRepo.MarkFailed(ctx, workflowID, "server shutdown")
                cleanup()
            }
        }
    })
}
```

### Step 3: Update relationshipWorkflowService

**File:** `pkg/services/relationship_workflow.go`

Apply the same pattern as Step 2:

1. Remove duplicated types (`relationshipHeartbeatInfo`, `taskQueueWriter`, `taskQueueUpdate`)
2. Add `infra *workflow.WorkflowInfra` field
3. Update constructor to create infra
4. Replace method calls with `s.infra.*`
5. Remove duplicated methods (lines 1501-1676)
6. Update Shutdown method

### Step 4: Update entityDiscoveryService

**File:** `pkg/services/entity_discovery_service.go`

Apply the same pattern as Step 2:

1. Remove duplicated types (`entityHeartbeatInfo`, `entityTaskQueueWriter`, `entityTaskQueueUpdate`)
2. Add `infra *workflow.WorkflowInfra` field
3. Update constructor to create infra
4. Replace method calls with `s.infra.*`
5. Remove duplicated methods (lines 795-910)
6. Update Shutdown method

### Step 5: Update TenantContextFunc type

Move `TenantContextFunc` type definition to the workflow package since it's now used there.

Check where it's currently defined and update imports as needed.

### Step 6: Add tests for workflow infrastructure

**File:** `pkg/services/workflow/infra_test.go`

Create unit tests for:
- `StartHeartbeat` / `StopHeartbeat`
- `StartTaskQueueWriter` / `StopTaskQueueWriter` / `SendTaskQueueUpdate`
- `MapTaskStatus`
- `Shutdown`

## Testing

1. Run `make check` to ensure everything compiles and passes
2. All existing workflow tests should pass unchanged
3. The existing integration tests for each workflow should continue to pass

## Files Changed

| File | Change |
|------|--------|
| `pkg/services/workflow/infra.go` | **NEW** - Shared workflow infrastructure |
| `pkg/services/workflow/infra_test.go` | **NEW** - Tests for shared infrastructure |
| `pkg/services/ontology_workflow.go` | Use infra, remove ~150 duplicated lines |
| `pkg/services/relationship_workflow.go` | Use infra, remove ~150 duplicated lines |
| `pkg/services/entity_discovery_service.go` | Use infra, remove ~120 duplicated lines |

## Estimated Scope

- ~250 lines of new infra code
- ~420 lines removed across 3 service files
- Net reduction of ~170 lines
- Medium risk - structural refactor but no behavior change

## Risk Mitigation

1. **Keep service-specific logic in services** - Only extract truly identical code
2. **Don't change external interfaces** - All service interfaces remain unchanged
3. **Incremental migration** - Update one service at a time, run tests after each
4. **Preserve shutdown behavior** - Use callback pattern to maintain service-specific shutdown logic

## Order of Operations

1. Create `pkg/services/workflow/infra.go`
2. Create `pkg/services/workflow/infra_test.go` with basic tests
3. Update `ontologyWorkflowService` (most stable, good test candidate)
4. Run `make check`
5. Update `relationshipWorkflowService`
6. Run `make check`
7. Update `entityDiscoveryService`
8. Run `make check`
9. Remove any now-unused imports

## Future Considerations

Once this refactoring is complete, consider:
- Adding metrics/observability to the shared infra
- Adding configurable heartbeat interval
- Adding graceful drain logic for task queue writer

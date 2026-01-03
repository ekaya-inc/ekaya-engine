// Package workflow provides shared infrastructure for workflow services.
// It handles common patterns like heartbeat management, task queue persistence,
// and queue lifecycle that are used across ontology, relationship, and entity
// discovery workflows.
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
// Returns the scoped context, a cleanup function (MUST be called), and any error.
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
// Note: This is deprecated infrastructure from the old workflow system.
// The DAG-based workflow has its own heartbeat management.
type WorkflowInfra struct {
	dagRepo          repositories.OntologyDAGRepository
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
	serverInstanceID uuid.UUID

	activeQueues     sync.Map // workflowID -> *workqueue.Queue
	taskQueueWriters sync.Map // workflowID -> *taskQueueWriter
	heartbeatStop    sync.Map // workflowID -> *heartbeatInfo
}

// NewWorkflowInfra creates a new workflow infrastructure instance.
// Deprecated: Use the DAG-based workflow system instead.
func NewWorkflowInfra(
	dagRepo repositories.OntologyDAGRepository,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) *WorkflowInfra {
	serverID := uuid.New()
	logger.Info("Workflow infrastructure initialized",
		zap.String("server_instance_id", serverID.String()))

	return &WorkflowInfra{
		dagRepo:          dagRepo,
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
				if err := w.dagRepo.UpdateHeartbeat(ctx, workflowID, w.serverInstanceID); err != nil {
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
		updates: make(chan TaskQueueUpdate, 100), // Buffer to avoid blocking the workqueue
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
// Deprecated: Task queue persistence is no longer supported in the DAG-based workflow.
// The DAG system uses node-level state tracking instead.
func (w *WorkflowInfra) persistTaskQueue(_, workflowID uuid.UUID, _ []models.WorkflowTask) {
	w.logger.Warn("Task queue persistence is deprecated in DAG-based workflow",
		zap.String("workflow_id", workflowID.String()))
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

// MapTaskStatus converts a workqueue.TaskStatus to a models.WorkflowTaskStatus string.
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
// It receives the workflow ID, project ID, and queue for service-specific cleanup.
type ShutdownFunc func(workflowID, projectID uuid.UUID, queue *workqueue.Queue)

// Shutdown gracefully stops all active workflows in parallel with timeout handling.
// The provided shutdownFn is called for each active workflow to handle
// service-specific shutdown logic (e.g., marking workflow as failed).
// Returns ctx.Err() if the context times out before all workflows are cancelled.
func (w *WorkflowInfra) Shutdown(ctx context.Context, shutdownFn ShutdownFunc) error {
	var wg sync.WaitGroup

	w.activeQueues.Range(func(key, value any) bool {
		workflowID := key.(uuid.UUID)
		queue := value.(*workqueue.Queue)

		// Get project ID from heartbeat info if available
		var projectID uuid.UUID
		if infoVal, ok := w.heartbeatStop.Load(workflowID); ok {
			info := infoVal.(*heartbeatInfo)
			projectID = info.projectID
		}

		wg.Add(1)
		go func(wfID uuid.UUID, q *workqueue.Queue, pID uuid.UUID) {
			defer wg.Done()

			// Call service-specific shutdown logic
			if shutdownFn != nil {
				shutdownFn(wfID, pID, q)
			}

			// Stop infrastructure
			w.StopTaskQueueWriter(wfID)
			w.StopHeartbeat(wfID)
			w.activeQueues.Delete(wfID)
		}(workflowID, queue, projectID)

		return true
	})

	// Wait for all cancellations with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.logger.Info("Workflow infrastructure shutdown complete")
		return nil
	case <-ctx.Done():
		w.logger.Warn("Shutdown timed out, some workflows may not have been cleaned up")
		return ctx.Err()
	}
}

package workqueue

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusPaused    TaskStatus = "paused"
)

// Task is the interface that all work queue tasks must implement.
type Task interface {
	// ID returns a unique identifier for this task.
	ID() string

	// Name returns a human-readable name for display in UI.
	Name() string

	// RequiresLLM returns true if this task makes LLM API calls.
	// Only one LLM task can run at a time.
	RequiresLLM() bool

	// Execute runs the task. It receives:
	// - ctx: context for cancellation
	// - enqueuer: allows the task to enqueue follow-up tasks
	// Returns an error if the task fails.
	Execute(ctx context.Context, enqueuer TaskEnqueuer) error
}

// TaskEnqueuer allows tasks to enqueue follow-up tasks.
type TaskEnqueuer interface {
	Enqueue(task Task)
}

// TaskState holds the runtime state of a task.
type TaskState struct {
	Task        Task
	Status      TaskStatus
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       error

	mu sync.RWMutex
}

// NewTaskState creates a new TaskState wrapping a task.
func NewTaskState(task Task) *TaskState {
	return &TaskState{
		Task:   task,
		Status: TaskStatusPending,
	}
}

// GetStatus returns the current status (thread-safe).
func (ts *TaskState) GetStatus() TaskStatus {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.Status
}

// SetStatus updates the status and timestamps (thread-safe).
func (ts *TaskState) SetStatus(status TaskStatus) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.Status = status
	now := time.Now()

	switch status {
	case TaskStatusRunning:
		ts.StartedAt = &now
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled, TaskStatusPaused:
		ts.CompletedAt = &now
	}
}

// SetError sets the error (thread-safe).
func (ts *TaskState) SetError(err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.Error = err
}

// GetError returns the error (thread-safe).
func (ts *TaskState) GetError() error {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.Error
}

// Snapshot returns an immutable copy of the task state.
func (ts *TaskState) Snapshot() TaskSnapshot {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var errMsg string
	if ts.Error != nil {
		errMsg = ts.Error.Error()
	}

	return TaskSnapshot{
		ID:          ts.Task.ID(),
		Name:        ts.Task.Name(),
		RequiresLLM: ts.Task.RequiresLLM(),
		Status:      ts.Status,
		StartedAt:   ts.StartedAt,
		CompletedAt: ts.CompletedAt,
		Error:       errMsg,
	}
}

// TaskSnapshot is an immutable view of task state for serialization.
type TaskSnapshot struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	RequiresLLM bool       `json:"requires_llm"`
	Status      TaskStatus `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// BaseTask provides common task functionality.
// Embed this in concrete task implementations.
type BaseTask struct {
	id          string
	name        string
	requiresLLM bool
}

// NewBaseTask creates a new base task.
func NewBaseTask(name string, requiresLLM bool) BaseTask {
	return BaseTask{
		id:          uuid.New().String(),
		name:        name,
		requiresLLM: requiresLLM,
	}
}

// ID returns the task ID.
func (t BaseTask) ID() string {
	return t.id
}

// Name returns the task name.
func (t BaseTask) Name() string {
	return t.name
}

// RequiresLLM returns whether this task needs exclusive LLM access.
func (t BaseTask) RequiresLLM() bool {
	return t.requiresLLM
}

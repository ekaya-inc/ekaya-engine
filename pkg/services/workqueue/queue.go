package workqueue

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"go.uber.org/zap"
)

// RetryConfig configures retry behavior for failed tasks.
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts (0 = no retries)
	InitialBackoff time.Duration // Initial backoff duration
	MaxBackoff     time.Duration // Maximum backoff duration (cap)
	BackoffFactor  float64       // Multiplier for exponential backoff
}

// DefaultRetryConfig returns sensible defaults for retry behavior.
// Backoff schedule: 2s, 4s, 8s, 16s, then 30s (capped) for remaining retries.
// With 24 retries: ~30s for first 4 quick retries + ~10min for 20 retries at 30s = ~10.5min max.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     24,
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
	}
}

// Queue manages task execution with configurable concurrency control.
// The concurrency strategy determines how tasks are allowed to run:
// - SerializedStrategy: one LLM task at a time, one data task at a time (default)
// - ParallelLLMStrategy: unlimited parallel LLM tasks
// - ThrottledLLMStrategy: up to N concurrent LLM tasks
type Queue struct {
	mu        sync.Mutex
	tasks     []*TaskState
	cancelled bool
	paused    bool // distinguishes pause from cancel for task status

	// Concurrency control strategy
	strategy ConcurrencyStrategy

	// Retry configuration for transient errors
	retryConfig RetryConfig

	// done is closed when all tasks complete
	done chan struct{}
	// wg tracks running goroutines
	wg sync.WaitGroup

	// Cancellation context for running tasks
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	onUpdate func([]TaskSnapshot)

	logger *zap.Logger
}

// QueueOption configures a Queue.
type QueueOption func(*Queue)

// WithStrategy sets the concurrency strategy.
func WithStrategy(strategy ConcurrencyStrategy) QueueOption {
	return func(q *Queue) {
		if strategy != nil {
			q.strategy = strategy
		}
	}
}

// WithRetryConfig sets the retry configuration.
func WithRetryConfig(config RetryConfig) QueueOption {
	return func(q *Queue) {
		q.retryConfig = config
	}
}

// NewQueue creates a new work queue with the default serialized strategy.
func NewQueue(logger *zap.Logger) *Queue {
	return NewQueueWithStrategy(logger, nil)
}

// NewQueueWithStrategy creates a new work queue with a custom concurrency strategy.
// If strategy is nil, defaults to SerializedStrategy (original behavior).
// Deprecated: Use New() with WithStrategy() option instead.
func NewQueueWithStrategy(logger *zap.Logger, strategy ConcurrencyStrategy) *Queue {
	return New(logger, WithStrategy(strategy))
}

// New creates a new work queue with the given options.
func New(logger *zap.Logger, opts ...QueueOption) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		tasks:       make([]*TaskState, 0),
		strategy:    NewSerializedStrategy(),
		retryConfig: DefaultRetryConfig(),
		done:        make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger.Named("workqueue"),
	}

	for _, opt := range opts {
		opt(q)
	}

	return q
}

// SetOnUpdate sets the callback invoked when task state changes.
// The callback receives a snapshot of all tasks.
//
// WARNING: The callback is invoked while holding the queue's internal lock.
// Do NOT call any Queue methods from within the callback or it will deadlock.
// The callback should be fast and non-blocking (e.g., send to a channel).
func (q *Queue) SetOnUpdate(callback func([]TaskSnapshot)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onUpdate = callback
}

// Enqueue adds a task to the queue and attempts to start eligible tasks.
func (q *Queue) Enqueue(task Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cancelled {
		q.logger.Warn("queue cancelled, ignoring enqueue",
			zap.String("task_id", task.ID()),
			zap.String("task_name", task.Name()))
		return
	}

	// Reset done channel if it was closed from a previous batch
	q.resetDoneLocked()

	state := NewTaskState(task)
	q.tasks = append(q.tasks, state)

	q.logger.Info("task enqueued",
		zap.String("task_id", task.ID()),
		zap.String("task_name", task.Name()),
		zap.Bool("requires_llm", task.RequiresLLM()))

	q.notifyUpdateLocked()
	q.tryStartTasksLocked()
}

// tryStartTasksLocked checks constraints and starts eligible tasks.
// Uses the configured concurrency strategy to determine which tasks can start.
// Must be called with lock held.
func (q *Queue) tryStartTasksLocked() {
	if q.cancelled {
		return
	}

	for _, ts := range q.tasks {
		if ts.GetStatus() != TaskStatusPending {
			continue
		}

		isLLMTask := ts.Task.RequiresLLM()

		// Check with strategy if task can start
		if isLLMTask && !q.strategy.CanStartLLM() {
			continue
		}
		if !isLLMTask && !q.strategy.CanStartData() {
			continue
		}

		// Notify strategy that task is starting
		if isLLMTask {
			q.strategy.OnStartLLM()
		} else {
			q.strategy.OnStartData()
		}
		ts.SetStatus(TaskStatusRunning)
		q.notifyUpdateLocked()

		q.logger.Info("starting task",
			zap.String("task_id", ts.Task.ID()),
			zap.String("task_name", ts.Task.Name()))

		q.wg.Add(1)
		go q.runTask(ts)
	}
}

// runTask executes a task with retry logic for transient errors.
func (q *Queue) runTask(ts *TaskState) {
	defer q.wg.Done()

	var lastErr error

	// Retry loop
	for attempt := 0; attempt <= q.retryConfig.MaxRetries; attempt++ {
		// Wait before retry (skip on first attempt)
		if attempt > 0 {
			backoff := q.calculateBackoff(attempt)
			q.logger.Info("retrying task after backoff",
				zap.String("task_id", ts.Task.ID()),
				zap.String("task_name", ts.Task.Name()),
				zap.Int("attempt", attempt),
				zap.Int("max_retries", q.retryConfig.MaxRetries),
				zap.Duration("backoff", backoff))

			select {
			case <-q.ctx.Done():
				// Context cancelled during backoff - exit immediately
				q.completeTaskFailure(ts, q.ctx.Err())
				return
			case <-time.After(backoff):
				// Continue with retry
			}
		}

		// Execute the task
		err := ts.Task.Execute(q.ctx, q)

		if err == nil {
			// Success - complete the task
			q.completeTaskSuccess(ts)
			return
		}

		lastErr = err

		// Check for context cancellation (not retryable)
		if errors.Is(err, context.Canceled) {
			break
		}

		// Check if error is retryable
		if !llm.IsRetryable(err) {
			q.logger.Warn("non-retryable error, failing task immediately",
				zap.String("task_id", ts.Task.ID()),
				zap.String("task_name", ts.Task.Name()),
				zap.Error(err))
			break
		}

		// Increment retry count
		retryCount := ts.IncrementRetryCount()

		// Check if we've exhausted retries
		if attempt >= q.retryConfig.MaxRetries {
			q.logger.Error("task failed after max retries",
				zap.String("task_id", ts.Task.ID()),
				zap.String("task_name", ts.Task.Name()),
				zap.Int("retry_count", retryCount),
				zap.Error(err))
			break
		}

		q.logger.Warn("retryable error encountered",
			zap.String("task_id", ts.Task.ID()),
			zap.String("task_name", ts.Task.Name()),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", q.retryConfig.MaxRetries),
			zap.Error(err))
	}

	// Task failed after all retries (or non-retryable error)
	q.completeTaskFailure(ts, lastErr)
}

// calculateBackoff computes the backoff duration for a retry attempt.
// Uses exponential backoff with jitter.
func (q *Queue) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: initial * factor^(attempt-1)
	backoff := float64(q.retryConfig.InitialBackoff) *
		math.Pow(q.retryConfig.BackoffFactor, float64(attempt-1))

	// Cap at max backoff
	if backoff > float64(q.retryConfig.MaxBackoff) {
		backoff = float64(q.retryConfig.MaxBackoff)
	}

	// Add jitter (±10%) to prevent thundering herd
	jitter := backoff * 0.1 * (rand.Float64()*2 - 1)

	return time.Duration(backoff + jitter)
}

// completeTaskSuccess marks a task as successfully completed.
func (q *Queue) completeTaskSuccess(ts *TaskState) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Notify strategy that task completed
	if ts.Task.RequiresLLM() {
		q.strategy.OnCompleteLLM()
	} else {
		q.strategy.OnCompleteData()
	}

	ts.SetStatus(TaskStatusCompleted)
	q.logger.Info("task completed",
		zap.String("task_id", ts.Task.ID()),
		zap.String("task_name", ts.Task.Name()),
		zap.Int("retry_count", ts.GetRetryCount()))

	q.notifyUpdateLocked()

	if q.allTasksDoneLocked() {
		q.closeDoneLocked()
		return
	}

	q.tryStartTasksLocked()
}

// completeTaskFailure marks a task as failed or cancelled/paused.
func (q *Queue) completeTaskFailure(ts *TaskState, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Notify strategy that task completed
	if ts.Task.RequiresLLM() {
		q.strategy.OnCompleteLLM()
	} else {
		q.strategy.OnCompleteData()
	}

	// Distinguish between pause, cancellation, and actual failures
	if errors.Is(err, context.Canceled) {
		if q.paused {
			ts.SetStatus(TaskStatusPaused)
			q.logger.Info("task paused",
				zap.String("task_id", ts.Task.ID()),
				zap.String("task_name", ts.Task.Name()))
		} else {
			ts.SetStatus(TaskStatusCancelled)
			q.logger.Info("task cancelled",
				zap.String("task_id", ts.Task.ID()),
				zap.String("task_name", ts.Task.Name()))
		}
	} else {
		ts.SetStatus(TaskStatusFailed)
		ts.SetError(err)
		q.logger.Error("task failed",
			zap.String("task_id", ts.Task.ID()),
			zap.String("task_name", ts.Task.Name()),
			zap.Int("retry_count", ts.GetRetryCount()),
			zap.Error(err))
	}

	q.notifyUpdateLocked()

	if q.allTasksDoneLocked() {
		q.closeDoneLocked()
		return
	}

	q.tryStartTasksLocked()
}

// allTasksDoneLocked returns true if all tasks are in a terminal state.
// Must be called with lock held.
func (q *Queue) allTasksDoneLocked() bool {
	for _, ts := range q.tasks {
		status := ts.GetStatus()
		if status == TaskStatusPending || status == TaskStatusRunning {
			return false
		}
	}
	return true
}

// closeDoneLocked safely closes the done channel.
// Must be called with lock held.
func (q *Queue) closeDoneLocked() {
	select {
	case <-q.done:
		// Already closed
	default:
		close(q.done)
	}
}

// resetDoneLocked recreates the done channel if it was closed.
// This allows the queue to be reused for multiple batches of work.
// Must be called with lock held.
func (q *Queue) resetDoneLocked() {
	select {
	case <-q.done:
		// Channel was closed, create a new one
		q.done = make(chan struct{})
	default:
		// Channel is still open, nothing to do
	}
}

// notifyUpdateLocked calls the update callback with a snapshot of all tasks.
// Must be called with lock held.
func (q *Queue) notifyUpdateLocked() {
	if q.onUpdate == nil {
		return
	}

	snapshots := make([]TaskSnapshot, len(q.tasks))
	for i, ts := range q.tasks {
		snapshots[i] = ts.Snapshot()
	}
	q.onUpdate(snapshots)
}

// GetTasks returns a snapshot of all tasks.
func (q *Queue) GetTasks() []TaskSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()

	snapshots := make([]TaskSnapshot, len(q.tasks))
	for i, ts := range q.tasks {
		snapshots[i] = ts.Snapshot()
	}
	return snapshots
}

// Wait blocks until all tasks complete or the context is cancelled.
// Returns nil if all tasks completed successfully or queue is empty.
// Returns the first task error if any task failed.
// Returns ctx.Err() if the context was cancelled.
func (q *Queue) Wait(ctx context.Context) error {
	// Handle empty queue - nothing to wait for
	q.mu.Lock()
	if len(q.tasks) == 0 {
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()

	select {
	case <-q.done:
		// Check for failed tasks
		q.mu.Lock()
		defer q.mu.Unlock()
		for _, ts := range q.tasks {
			if ts.GetStatus() == TaskStatusFailed {
				return ts.GetError()
			}
		}
		return nil
	case <-ctx.Done():
		q.Cancel()
		return ctx.Err()
	}
}

// Cancel marks the queue as cancelled, signals running tasks to stop,
// and stops accepting new tasks.
func (q *Queue) Cancel() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cancelled {
		return
	}

	q.cancelled = true
	q.logger.Info("queue cancelled, signaling running tasks to stop")

	// Signal all running tasks to stop via context cancellation
	q.cancel()

	// Mark pending tasks as cancelled
	for _, ts := range q.tasks {
		if ts.GetStatus() == TaskStatusPending {
			ts.SetStatus(TaskStatusCancelled)
		}
	}

	q.notifyUpdateLocked()

	// If no tasks are running, close done channel
	if q.allTasksDoneLocked() {
		q.closeDoneLocked()
	}
}

// Pause marks the queue as paused, signals running tasks to stop,
// and marks pending/running tasks as paused for later resumption.
// Unlike Cancel, paused tasks can be resumed.
func (q *Queue) Pause() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cancelled {
		return
	}

	q.cancelled = true // Reuse to prevent new enqueues
	q.paused = true    // Mark as paused (not cancelled)
	q.logger.Info("queue paused, signaling running tasks to stop")

	// Signal all running tasks to stop via context cancellation
	q.cancel()

	// Mark pending tasks as paused (running tasks will be marked in runTask)
	for _, ts := range q.tasks {
		if ts.GetStatus() == TaskStatusPending {
			ts.SetStatus(TaskStatusPaused)
		}
	}

	q.notifyUpdateLocked()

	// If no tasks are running, close done channel
	if q.allTasksDoneLocked() {
		q.closeDoneLocked()
	}
}

// IsPaused returns true if the queue was paused (vs cancelled).
func (q *Queue) IsPaused() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.paused
}

// IsComplete returns true if all tasks have completed (success or failure).
func (q *Queue) IsComplete() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.allTasksDoneLocked()
}

// HasFailures returns true if any task failed.
func (q *Queue) HasFailures() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, ts := range q.tasks {
		if ts.GetStatus() == TaskStatusFailed {
			return true
		}
	}
	return false
}

// TaskCount returns the total number of tasks.
func (q *Queue) TaskCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

// PendingCount returns the number of pending tasks.
func (q *Queue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, ts := range q.tasks {
		if ts.GetStatus() == TaskStatusPending {
			count++
		}
	}
	return count
}

// CompletedCount returns the number of completed tasks.
func (q *Queue) CompletedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, ts := range q.tasks {
		if ts.GetStatus() == TaskStatusCompleted {
			count++
		}
	}
	return count
}

// Progress returns a progress summary.
func (q *Queue) Progress() Progress {
	q.mu.Lock()
	defer q.mu.Unlock()

	p := Progress{Total: len(q.tasks)}
	for _, ts := range q.tasks {
		switch ts.GetStatus() {
		case TaskStatusPending:
			p.Pending++
		case TaskStatusRunning:
			p.Running++
		case TaskStatusCompleted:
			p.Completed++
		case TaskStatusFailed:
			p.Failed++
		case TaskStatusCancelled:
			p.Cancelled++
		case TaskStatusPaused:
			p.Paused++
		}
	}
	return p
}

// Progress holds queue progress statistics.
type Progress struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
	Paused    int `json:"paused"`
}

// Percentage returns the completion percentage (0-100).
func (p Progress) Percentage() int {
	if p.Total == 0 {
		return 100
	}
	done := p.Completed + p.Failed + p.Cancelled + p.Paused
	return (done * 100) / p.Total
}

package workqueue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// testTask is a simple task for testing.
type testTask struct {
	BaseTask
	executeFunc func(ctx context.Context, enqueuer TaskEnqueuer) error
}

func newTestTask(name string, requiresLLM bool, fn func(ctx context.Context, enqueuer TaskEnqueuer) error) *testTask {
	return &testTask{
		BaseTask:    NewBaseTask(name, requiresLLM),
		executeFunc: fn,
	}
}

func (t *testTask) Execute(ctx context.Context, enqueuer TaskEnqueuer) error {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, enqueuer)
	}
	return nil
}

func TestQueue_EnqueueAndComplete(t *testing.T) {
	q := NewQueue(zap.NewNop())

	executed := false
	task := newTestTask("test-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		executed = true
		return nil
	})

	q.Enqueue(task)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executed {
		t.Error("task was not executed")
	}

	if q.CompletedCount() != 1 {
		t.Errorf("expected 1 completed, got %d", q.CompletedCount())
	}
}

func TestQueue_TaskFailure(t *testing.T) {
	q := NewQueue(zap.NewNop())

	expectedErr := errors.New("task failed")
	task := newTestTask("failing-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		return expectedErr
	})

	q.Enqueue(task)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}

	if !q.HasFailures() {
		t.Error("expected HasFailures to return true")
	}
}

func TestQueue_LLMSerialization(t *testing.T) {
	q := NewQueue(zap.NewNop())

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	// Create 3 LLM tasks that track concurrent execution
	for i := 0; i < 3; i++ {
		task := newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	if mc > 1 {
		t.Errorf("LLM tasks ran concurrently: max concurrent was %d", mc)
	}
}

func TestQueue_DataTasksSerialized(t *testing.T) {
	q := NewQueue(zap.NewNop())

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	// Create 3 data (non-LLM) tasks - they should serialize (only 1 at a time)
	for i := 0; i < 3; i++ {
		task := newTestTask("data-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	// Data tasks should serialize (max 1 concurrent)
	if mc > 1 {
		t.Errorf("expected data tasks to serialize, but max concurrent was %d", mc)
	}
}

func TestQueue_TwoLaneParallelism(t *testing.T) {
	q := NewQueue(zap.NewNop())

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	started := make(chan struct{})
	proceed := make(chan struct{})

	// Enqueue an LLM task that waits
	q.Enqueue(newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		current := atomic.AddInt32(&running, 1)
		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		started <- struct{}{} // Signal that LLM task started
		<-proceed             // Wait for signal to continue
		atomic.AddInt32(&running, -1)
		return nil
	}))

	// Enqueue a data task
	q.Enqueue(newTestTask("data-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		current := atomic.AddInt32(&running, 1)
		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		started <- struct{}{} // Signal that data task started
		<-proceed             // Wait for signal to continue
		atomic.AddInt32(&running, -1)
		return nil
	}))

	// Wait for both tasks to start (they should run in parallel)
	<-started
	<-started

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	// Both tasks should be running in parallel (2 concurrent)
	if mc != 2 {
		t.Errorf("expected LLM and data tasks to run in parallel, but max concurrent was %d", mc)
	}

	// Let tasks complete
	close(proceed)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueue_MixedTasks(t *testing.T) {
	q := NewQueue(zap.NewNop())

	var executionOrder []string
	var mu sync.Mutex

	record := func(name string) {
		mu.Lock()
		executionOrder = append(executionOrder, name)
		mu.Unlock()
	}

	// Enqueue: non-LLM, LLM, non-LLM, LLM
	q.Enqueue(newTestTask("non-llm-1", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		record("non-llm-1")
		time.Sleep(10 * time.Millisecond)
		return nil
	}))
	q.Enqueue(newTestTask("llm-1", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		record("llm-1")
		time.Sleep(10 * time.Millisecond)
		return nil
	}))
	q.Enqueue(newTestTask("non-llm-2", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		record("non-llm-2")
		time.Sleep(10 * time.Millisecond)
		return nil
	}))
	q.Enqueue(newTestTask("llm-2", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		record("llm-2")
		time.Sleep(10 * time.Millisecond)
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All tasks should complete
	if q.CompletedCount() != 4 {
		t.Errorf("expected 4 completed, got %d", q.CompletedCount())
	}

	// Verify both LLM tasks executed
	mu.Lock()
	hasLLM1 := contains(executionOrder, "llm-1")
	hasLLM2 := contains(executionOrder, "llm-2")
	mu.Unlock()

	if !hasLLM1 || !hasLLM2 {
		t.Error("not all LLM tasks executed")
	}
}

func TestQueue_TaskEnqueuesMoreTasks(t *testing.T) {
	q := NewQueue(zap.NewNop())

	var executed []string
	var mu sync.Mutex

	// First task enqueues a second task
	task1 := newTestTask("task-1", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		mu.Lock()
		executed = append(executed, "task-1")
		mu.Unlock()

		enqueuer.Enqueue(newTestTask("task-2", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			mu.Lock()
			executed = append(executed, "task-2")
			mu.Unlock()
			return nil
		}))
		return nil
	})

	q.Enqueue(task1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(executed) != 2 {
		t.Errorf("expected 2 tasks executed, got %d", len(executed))
	}
	if !contains(executed, "task-1") || !contains(executed, "task-2") {
		t.Errorf("expected task-1 and task-2, got %v", executed)
	}
}

func TestQueue_Cancel(t *testing.T) {
	q := NewQueue(zap.NewNop())

	// Use an LLM task to block other LLM tasks
	started := make(chan struct{})
	task := newTestTask("slow-llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		close(started)
		time.Sleep(5 * time.Second)
		return nil
	})

	q.Enqueue(task)

	// Wait for task to start
	<-started

	// Enqueue another LLM task - it must wait since LLM is serialized
	q.Enqueue(newTestTask("pending-llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		return nil
	}))

	// Give the queue a moment to process the enqueue
	time.Sleep(10 * time.Millisecond)

	// Cancel the queue
	q.Cancel()

	// The pending LLM task should be marked as cancelled (it couldn't start)
	tasks := q.GetTasks()
	for _, ts := range tasks {
		if ts.Name == "pending-llm-task" && ts.Status != TaskStatusCancelled {
			t.Errorf("expected pending-llm-task to be cancelled, got %s", ts.Status)
		}
	}
}

func TestQueue_ContextCancellation(t *testing.T) {
	q := NewQueue(zap.NewNop())

	task := newTestTask("slow-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		time.Sleep(10 * time.Second)
		return nil
	})

	q.Enqueue(task)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := q.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestQueue_CancelRunningTasks(t *testing.T) {
	q := NewQueue(zap.NewNop())

	taskStarted := make(chan struct{})
	taskCancelled := make(chan struct{})

	// Task that respects context cancellation
	task := newTestTask("cancellable-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		close(taskStarted)
		select {
		case <-ctx.Done():
			close(taskCancelled)
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	q.Enqueue(task)

	// Wait for task to start
	<-taskStarted

	// Cancel the queue
	q.Cancel()

	// Task should receive cancellation
	select {
	case <-taskCancelled:
		// Good - task was cancelled
	case <-time.After(1 * time.Second):
		t.Fatal("task did not receive cancellation signal")
	}

	// Wait for queue to finish
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = q.Wait(ctx)

	// Task should be marked as cancelled, not failed
	tasks := q.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != TaskStatusCancelled {
		t.Errorf("expected task status Cancelled, got %s", tasks[0].Status)
	}
}

func TestQueue_CancelMultipleRunningTasks(t *testing.T) {
	q := NewQueue(zap.NewNop())

	// With two-lane serialization, we can have at most 2 tasks running:
	// 1 LLM task + 1 data task
	const numTasks = 2
	tasksStarted := make(chan struct{}, numTasks)
	var cancelledCount int32

	// Create 1 LLM task and 1 data task that can run in parallel
	llmTask := newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		tasksStarted <- struct{}{}
		select {
		case <-ctx.Done():
			atomic.AddInt32(&cancelledCount, 1)
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})
	dataTask := newTestTask("data-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		tasksStarted <- struct{}{}
		select {
		case <-ctx.Done():
			atomic.AddInt32(&cancelledCount, 1)
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})
	q.Enqueue(llmTask)
	q.Enqueue(dataTask)

	// Wait for both tasks to start (they run in parallel - different lanes)
	for i := 0; i < numTasks; i++ {
		select {
		case <-tasksStarted:
		case <-time.After(1 * time.Second):
			t.Fatal("tasks did not start in time")
		}
	}

	// Cancel the queue
	q.Cancel()

	// Wait for queue to finish
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = q.Wait(ctx)

	// All tasks should have been cancelled
	if atomic.LoadInt32(&cancelledCount) != numTasks {
		t.Errorf("expected %d tasks cancelled, got %d", numTasks, cancelledCount)
	}

	// All tasks should be marked as cancelled
	tasks := q.GetTasks()
	for _, ts := range tasks {
		if ts.Status != TaskStatusCancelled {
			t.Errorf("expected task %s status Cancelled, got %s", ts.Name, ts.Status)
		}
	}
}

func TestQueue_CancelWithPendingAndRunning(t *testing.T) {
	q := NewQueue(zap.NewNop())

	llmStarted := make(chan struct{})

	// First: LLM task that blocks (only one LLM task runs at a time)
	q.Enqueue(newTestTask("running-llm", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		close(llmStarted)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}))

	// Second: Another LLM task that will be pending (can't run while first is running)
	q.Enqueue(newTestTask("pending-llm", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		return nil
	}))

	// Wait for first task to start
	<-llmStarted

	// Give queue time to process - second task should still be pending
	time.Sleep(10 * time.Millisecond)

	// Cancel the queue
	q.Cancel()

	// Wait for queue to finish
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = q.Wait(ctx)

	// Check final states
	tasks := q.GetTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Both should be cancelled
	for _, ts := range tasks {
		if ts.Status != TaskStatusCancelled {
			t.Errorf("expected task %s status Cancelled, got %s", ts.Name, ts.Status)
		}
	}
}

func TestQueue_OnUpdateCallback(t *testing.T) {
	q := NewQueue(zap.NewNop())

	var updates [][]TaskSnapshot
	var mu sync.Mutex

	q.SetOnUpdate(func(snapshots []TaskSnapshot) {
		mu.Lock()
		// Make a copy
		cp := make([]TaskSnapshot, len(snapshots))
		copy(cp, snapshots)
		updates = append(updates, cp)
		mu.Unlock()
	})

	task := newTestTask("test-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		return nil
	})

	q.Enqueue(task)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have multiple updates: enqueue, start, complete
	if len(updates) < 2 {
		t.Errorf("expected at least 2 updates, got %d", len(updates))
	}

	// First update should have task in pending or running
	if len(updates) > 0 {
		first := updates[0]
		if len(first) != 1 {
			t.Errorf("expected 1 task in first update, got %d", len(first))
		}
	}

	// Last update should have task completed
	last := updates[len(updates)-1]
	if len(last) != 1 || last[0].Status != TaskStatusCompleted {
		t.Error("expected final update to show completed task")
	}
}

func TestQueue_Progress(t *testing.T) {
	q := NewQueue(zap.NewNop())

	blocker := make(chan struct{})
	// Use 1 LLM task and 1 data task so they can run in parallel
	q.Enqueue(newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		<-blocker
		return nil
	}))
	q.Enqueue(newTestTask("data-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		<-blocker
		return nil
	}))

	// Give tasks time to start
	time.Sleep(50 * time.Millisecond)

	p := q.Progress()
	if p.Total != 2 {
		t.Errorf("expected total 2, got %d", p.Total)
	}
	if p.Running != 2 {
		t.Errorf("expected running 2, got %d", p.Running)
	}
	if p.Percentage() != 0 {
		t.Errorf("expected 0%%, got %d%%", p.Percentage())
	}

	// Unblock tasks
	close(blocker)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p = q.Progress()
	if p.Completed != 2 {
		t.Errorf("expected completed 2, got %d", p.Completed)
	}
	if p.Percentage() != 100 {
		t.Errorf("expected 100%%, got %d%%", p.Percentage())
	}
}

func TestQueue_EmptyQueue(t *testing.T) {
	q := NewQueue(zap.NewNop())

	// Empty queue should be complete by default
	if !q.IsComplete() {
		t.Error("empty queue should be complete")
	}
	if q.TaskCount() != 0 {
		t.Errorf("expected 0 tasks, got %d", q.TaskCount())
	}

	// Wait on empty queue should return immediately
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Errorf("expected nil error for empty queue, got %v", err)
	}
}

func TestQueue_GetTasks(t *testing.T) {
	q := NewQueue(zap.NewNop())

	q.Enqueue(newTestTask("task-1", false, nil))
	q.Enqueue(newTestTask("task-2", true, nil))

	tasks := q.GetTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Verify task properties are captured
	found := false
	for _, ts := range tasks {
		if ts.Name == "task-2" && ts.RequiresLLM {
			found = true
		}
	}
	if !found {
		t.Error("expected to find task-2 with RequiresLLM=true")
	}
}

func TestTaskSnapshot(t *testing.T) {
	task := newTestTask("test-task", true, nil)
	ts := NewTaskState(task)

	snapshot := ts.Snapshot()
	if snapshot.ID != task.ID() {
		t.Errorf("expected ID %s, got %s", task.ID(), snapshot.ID)
	}
	if snapshot.Name != "test-task" {
		t.Errorf("expected name 'test-task', got %s", snapshot.Name)
	}
	if !snapshot.RequiresLLM {
		t.Error("expected RequiresLLM to be true")
	}
	if snapshot.Status != TaskStatusPending {
		t.Errorf("expected status pending, got %s", snapshot.Status)
	}
	if snapshot.StartedAt != nil {
		t.Error("expected StartedAt to be nil")
	}
}

func TestTaskState_SetStatus(t *testing.T) {
	task := newTestTask("test-task", false, nil)
	ts := NewTaskState(task)

	ts.SetStatus(TaskStatusRunning)
	if ts.GetStatus() != TaskStatusRunning {
		t.Errorf("expected running, got %s", ts.GetStatus())
	}

	snapshot := ts.Snapshot()
	if snapshot.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}

	ts.SetStatus(TaskStatusCompleted)
	snapshot = ts.Snapshot()
	if snapshot.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestProgress_Percentage(t *testing.T) {
	tests := []struct {
		name     string
		progress Progress
		expected int
	}{
		{"empty", Progress{Total: 0}, 100},
		{"none complete", Progress{Total: 10, Pending: 10}, 0},
		{"half complete", Progress{Total: 10, Completed: 5, Pending: 5}, 50},
		{"all complete", Progress{Total: 10, Completed: 10}, 100},
		{"mixed terminal states", Progress{Total: 10, Completed: 5, Failed: 3, Cancelled: 2}, 100},
		{"partial with failures", Progress{Total: 10, Completed: 3, Failed: 2, Running: 5}, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.progress.Percentage()
			if got != tt.expected {
				t.Errorf("expected %d%%, got %d%%", tt.expected, got)
			}
		})
	}
}

func TestQueue_MultipleBatchesWait(t *testing.T) {
	q := NewQueue(zap.NewNop())

	// Batch 1: Enqueue and wait
	executed := make([]string, 0)
	var mu sync.Mutex

	q.Enqueue(newTestTask("batch1-task1", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		mu.Lock()
		executed = append(executed, "batch1-task1")
		mu.Unlock()
		return nil
	}))

	ctx := context.Background()
	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("batch 1 wait failed: %v", err)
	}

	// Verify batch 1 completed
	mu.Lock()
	if !contains(executed, "batch1-task1") {
		t.Fatalf("batch 1 task was not executed")
	}
	mu.Unlock()

	// Batch 2: Enqueue a SLOW task to the SAME queue and wait again
	// The task takes 100ms to complete - if Wait() returns immediately
	// (due to closed done channel), it won't be executed yet
	q.Enqueue(newTestTask("batch2-task1", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		executed = append(executed, "batch2-task1")
		mu.Unlock()
		return nil
	}))

	// This Wait() should block until batch 2 completes
	// BUG: Currently returns immediately because done channel is closed
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = q.Wait(ctxWithTimeout)
	if err != nil {
		t.Fatalf("batch 2 wait failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(executed) != 2 {
		t.Errorf("expected 2 tasks executed, got %d: %v", len(executed), executed)
	}
	if !contains(executed, "batch2-task1") {
		t.Errorf("batch 2 task was not executed: %v", executed)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ============================================================================
// Strategy Tests
// ============================================================================

func TestParallelLLMStrategy_AllowsConcurrentLLM(t *testing.T) {
	q := NewQueueWithStrategy(zap.NewNop(), NewParallelLLMStrategy())

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	// Create 5 LLM tasks that track concurrent execution
	for i := 0; i < 5; i++ {
		task := newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	// With parallel strategy, all 5 should run concurrently
	if mc < 3 {
		t.Errorf("ParallelLLMStrategy should allow concurrent LLM tasks: max concurrent was %d, expected >= 3", mc)
	}
}

func TestParallelLLMStrategy_StillSerializesDataTasks(t *testing.T) {
	q := NewQueueWithStrategy(zap.NewNop(), NewParallelLLMStrategy())

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	// Create 3 data tasks - they should still serialize
	for i := 0; i < 3; i++ {
		task := newTestTask("data-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	// Data tasks should still serialize (max 1 at a time)
	if mc > 1 {
		t.Errorf("Data tasks should serialize even with ParallelLLMStrategy: max concurrent was %d", mc)
	}
}

func TestThrottledLLMStrategy_RespectsLimit(t *testing.T) {
	maxConcurrent := 3
	q := NewQueueWithStrategy(zap.NewNop(), NewThrottledLLMStrategy(maxConcurrent))

	var running int32
	var observedMax int32
	var mu sync.Mutex

	// Create 10 LLM tasks
	for i := 0; i < 10; i++ {
		task := newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > observedMax {
				observedMax = current
			}
			mu.Unlock()

			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	om := observedMax
	mu.Unlock()

	// Should respect the limit
	if om > int32(maxConcurrent) {
		t.Errorf("ThrottledLLMStrategy exceeded limit: observed max %d, limit was %d", om, maxConcurrent)
	}
	// Should actually use concurrency (not serialize to 1)
	if om < 2 {
		t.Errorf("ThrottledLLMStrategy should allow some concurrency: observed max was %d", om)
	}
}

func TestThrottledLLMStrategy_StillSerializesDataTasks(t *testing.T) {
	q := NewQueueWithStrategy(zap.NewNop(), NewThrottledLLMStrategy(10))

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	// Create 3 data tasks - they should still serialize
	for i := 0; i < 3; i++ {
		task := newTestTask("data-task", false, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	if mc > 1 {
		t.Errorf("Data tasks should serialize even with ThrottledLLMStrategy: max concurrent was %d", mc)
	}
}

func TestSerializedStrategy_SerializesLLM(t *testing.T) {
	// Explicitly test SerializedStrategy (same as default NewQueue behavior)
	q := NewQueueWithStrategy(zap.NewNop(), NewSerializedStrategy())

	var running int32
	var maxConcurrent int32
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		task := newTestTask("llm-task", true, func(ctx context.Context, enqueuer TaskEnqueuer) error {
			current := atomic.AddInt32(&running, 1)
			mu.Lock()
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&running, -1)
			return nil
		})
		q.Enqueue(task)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := q.Wait(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	mc := maxConcurrent
	mu.Unlock()

	if mc > 1 {
		t.Errorf("SerializedStrategy should serialize LLM tasks: max concurrent was %d", mc)
	}
}

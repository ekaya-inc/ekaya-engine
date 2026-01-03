package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestWorkerPool_Process_Success(t *testing.T) {
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())

	items := []WorkItem{
		{ID: "task1", Execute: func(ctx context.Context) (any, error) { return "result1", nil }},
		{ID: "task2", Execute: func(ctx context.Context) (any, error) { return "result2", nil }},
		{ID: "task3", Execute: func(ctx context.Context) (any, error) { return "result3", nil }},
	}

	results := pool.Process(context.Background(), items, nil)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify all results are present (order may vary)
	resultsByID := make(map[string]string)
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("task %s failed: %v", r.ID, r.Err)
		}
		resultsByID[r.ID] = r.Result.(string)
	}

	if resultsByID["task1"] != "result1" || resultsByID["task2"] != "result2" || resultsByID["task3"] != "result3" {
		t.Errorf("unexpected results: %v", resultsByID)
	}
}

func TestWorkerPool_Process_WithErrors(t *testing.T) {
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())

	expectedErr := errors.New("task failed")
	items := []WorkItem{
		{ID: "task1", Execute: func(ctx context.Context) (any, error) { return "result1", nil }},
		{ID: "task2", Execute: func(ctx context.Context) (any, error) { return "", expectedErr }},
		{ID: "task3", Execute: func(ctx context.Context) (any, error) { return "result3", nil }},
	}

	results := pool.Process(context.Background(), items, nil)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify task2 failed and others succeeded
	resultsByID := make(map[string]WorkResult)
	for _, r := range results {
		resultsByID[r.ID] = r
	}

	if resultsByID["task1"].Err != nil {
		t.Errorf("task1 should succeed, got error: %v", resultsByID["task1"].Err)
	}
	if resultsByID["task2"].Err != expectedErr {
		t.Errorf("task2 should fail with expectedErr, got: %v", resultsByID["task2"].Err)
	}
	if resultsByID["task3"].Err != nil {
		t.Errorf("task3 should succeed, got error: %v", resultsByID["task3"].Err)
	}
}

func TestWorkerPool_Process_EmptyItems(t *testing.T) {
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())

	items := []WorkItem{}
	results := pool.Process(context.Background(), items, nil)

	if results != nil {
		t.Errorf("expected nil results for empty items, got %v", results)
	}
}

func TestWorkerPool_Process_ContextCancellation(t *testing.T) {
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())

	items := []WorkItem{
		{ID: "task1", Execute: func(ctx context.Context) (any, error) {
			// Cancel after starting first task
			cancel()
			// Wait a moment for cancellation to propagate
			time.Sleep(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return "result1", nil
			}
		}},
		{ID: "task2", Execute: func(ctx context.Context) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return "result2", nil
			}
		}},
	}

	results := pool.Process(ctx, items, nil)

	// At least one task should detect cancellation
	foundCancellation := false
	for _, r := range results {
		if r.Err == context.Canceled {
			foundCancellation = true
		}
	}
	if !foundCancellation {
		t.Error("expected at least one task to detect context cancellation")
	}
}

func TestWorkerPool_Process_ConcurrencyLimit(t *testing.T) {
	maxConcurrent := 3
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: maxConcurrent}, zap.NewNop())

	var currentConcurrent atomic.Int32
	var maxObservedConcurrent atomic.Int32

	items := make([]WorkItem, 10)
	for i := 0; i < 10; i++ {
		taskID := fmt.Sprintf("task%d", i)
		items[i] = WorkItem{
			ID: taskID,
			Execute: func(ctx context.Context) (any, error) {
				current := currentConcurrent.Add(1)
				defer currentConcurrent.Add(-1)

				// Update max observed concurrent
				for {
					max := maxObservedConcurrent.Load()
					if current <= max || maxObservedConcurrent.CompareAndSwap(max, current) {
						break
					}
				}

				// Simulate work
				time.Sleep(50 * time.Millisecond)
				return "done", nil
			},
		}
	}

	results := pool.Process(context.Background(), items, nil)

	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	maxObserved := maxObservedConcurrent.Load()
	if maxObserved > int32(maxConcurrent) {
		t.Errorf("concurrency limit violated: observed %d concurrent tasks, limit was %d", maxObserved, maxConcurrent)
	}

	// Should have had some concurrency (at least 2 tasks running at once)
	if maxObserved < 2 {
		t.Errorf("expected some concurrency, but max observed was %d", maxObserved)
	}
}

func TestWorkerPool_Process_ProgressCallback(t *testing.T) {
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())

	items := []WorkItem{
		{ID: "task1", Execute: func(ctx context.Context) (any, error) { return "result1", nil }},
		{ID: "task2", Execute: func(ctx context.Context) (any, error) { return "result2", nil }},
		{ID: "task3", Execute: func(ctx context.Context) (any, error) { return "result3", nil }},
	}

	var mu sync.Mutex
	progressUpdates := []int{}

	results := pool.Process(context.Background(), items, func(completed, total int) {
		mu.Lock()
		defer mu.Unlock()
		progressUpdates = append(progressUpdates, completed)

		if total != 3 {
			t.Errorf("expected total=3, got total=%d", total)
		}
	})

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify progress callback was called 3 times with increasing values
	mu.Lock()
	defer mu.Unlock()
	if len(progressUpdates) != 3 {
		t.Errorf("expected 3 progress updates, got %d", len(progressUpdates))
	}

	// Progress should reach 3 eventually
	foundThree := false
	for _, p := range progressUpdates {
		if p == 3 {
			foundThree = true
		}
	}
	if !foundThree {
		t.Errorf("expected final progress of 3, got updates: %v", progressUpdates)
	}
}

func TestWorkerPool_ConfigDefault(t *testing.T) {
	// Test that invalid config is corrected
	pool := NewWorkerPool(WorkerPoolConfig{MaxConcurrent: 0}, zap.NewNop())
	if pool.config.MaxConcurrent != 8 {
		t.Errorf("expected default MaxConcurrent=8, got %d", pool.config.MaxConcurrent)
	}

	pool = NewWorkerPool(WorkerPoolConfig{MaxConcurrent: -1}, zap.NewNop())
	if pool.config.MaxConcurrent != 8 {
		t.Errorf("expected default MaxConcurrent=8, got %d", pool.config.MaxConcurrent)
	}
}

func TestDefaultWorkerPoolConfig(t *testing.T) {
	config := DefaultWorkerPoolConfig()
	if config.MaxConcurrent != 8 {
		t.Errorf("expected MaxConcurrent=8, got %d", config.MaxConcurrent)
	}
}

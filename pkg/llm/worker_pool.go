package llm

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// WorkerPoolConfig configures the LLM worker pool.
type WorkerPoolConfig struct {
	MaxConcurrent int // Maximum concurrent LLM calls (default: 8)
}

// DefaultWorkerPoolConfig returns sensible defaults.
func DefaultWorkerPoolConfig() WorkerPoolConfig {
	return WorkerPoolConfig{
		MaxConcurrent: 8,
	}
}

// WorkerPool manages concurrent LLM call execution with bounded parallelism.
// It uses a semaphore to limit outstanding requests and processes results
// as they complete, allowing new requests to start immediately.
type WorkerPool struct {
	config WorkerPoolConfig
	logger *zap.Logger
}

// NewWorkerPool creates a new LLM worker pool.
func NewWorkerPool(config WorkerPoolConfig, logger *zap.Logger) *WorkerPool {
	if config.MaxConcurrent < 1 {
		config.MaxConcurrent = 8
	}
	return &WorkerPool{
		config: config,
		logger: logger.Named("llm-worker-pool"),
	}
}

// WorkItem represents a unit of work to be processed.
type WorkItem struct {
	ID      string                                 // For logging/tracking
	Execute func(ctx context.Context) (any, error) // The LLM call to make
}

// WorkResult represents the result of a work item.
type WorkResult struct {
	ID     string
	Result any
	Err    error
}

// Process executes all work items with bounded parallelism.
// Returns results in completion order (not submission order).
// Continues processing all items even if some fail.
func (p *WorkerPool) Process(
	ctx context.Context,
	items []WorkItem,
	onProgress func(completed, total int),
) []WorkResult {
	if len(items) == 0 {
		return nil
	}

	results := make([]WorkResult, 0, len(items))
	resultsChan := make(chan WorkResult, len(items))
	sem := make(chan struct{}, p.config.MaxConcurrent)

	var wg sync.WaitGroup

	// Submit all work items
	for _, item := range items {
		wg.Add(1)
		go func(item WorkItem) {
			defer wg.Done()

			// Acquire semaphore slot (blocks if at max concurrency)
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }() // Release slot when done
			case <-ctx.Done():
				resultsChan <- WorkResult{ID: item.ID, Err: ctx.Err()}
				return
			}

			// Execute the LLM call
			result, err := item.Execute(ctx)
			resultsChan <- WorkResult{
				ID:     item.ID,
				Result: result,
				Err:    err,
			}
		}(item)
	}

	// Close results channel when all work is done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results and report progress
	completed := 0
	for result := range resultsChan {
		results = append(results, result)
		completed++
		if onProgress != nil {
			onProgress(completed, len(items))
		}
	}

	return results
}

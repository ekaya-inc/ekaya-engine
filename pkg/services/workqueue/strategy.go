package workqueue

import "sync"

// ConcurrencyStrategy controls how tasks are allowed to start concurrently.
// The strategy is responsible for tracking running tasks and determining
// if a new task can start based on the current state.
type ConcurrencyStrategy interface {
	// CanStartLLM returns true if an LLM task can start given current state
	CanStartLLM() bool
	// CanStartData returns true if a data task can start given current state
	CanStartData() bool
	// OnStartLLM is called when an LLM task starts
	OnStartLLM()
	// OnStartData is called when a data task starts
	OnStartData()
	// OnCompleteLLM is called when an LLM task completes
	OnCompleteLLM()
	// OnCompleteData is called when a data task completes
	OnCompleteData()
}

// ============================================================================
// SerializedStrategy - Original behavior (1 LLM at a time, 1 data at a time)
// ============================================================================

// SerializedStrategy serializes both LLM and data tasks.
// Only one LLM task and one data task can run at a time,
// but an LLM task and a data task can run in parallel.
type SerializedStrategy struct {
	mu          sync.Mutex
	llmRunning  bool
	dataRunning bool
}

// NewSerializedStrategy creates a strategy that serializes LLM tasks
// (only one at a time) and serializes data tasks (only one at a time).
func NewSerializedStrategy() *SerializedStrategy {
	return &SerializedStrategy{}
}

func (s *SerializedStrategy) CanStartLLM() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.llmRunning
}

func (s *SerializedStrategy) CanStartData() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.dataRunning
}

func (s *SerializedStrategy) OnStartLLM() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmRunning = true
}

func (s *SerializedStrategy) OnStartData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataRunning = true
}

func (s *SerializedStrategy) OnCompleteLLM() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmRunning = false
}

func (s *SerializedStrategy) OnCompleteData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataRunning = false
}

// ============================================================================
// ParallelLLMStrategy - Unlimited parallel LLM tasks
// ============================================================================

// ParallelLLMStrategy allows unlimited parallel LLM tasks.
// Data tasks are still serialized (only one at a time).
type ParallelLLMStrategy struct {
	mu          sync.Mutex
	dataRunning bool
}

// NewParallelLLMStrategy creates a strategy that allows unlimited
// parallel LLM tasks while serializing data tasks.
func NewParallelLLMStrategy() *ParallelLLMStrategy {
	return &ParallelLLMStrategy{}
}

func (s *ParallelLLMStrategy) CanStartLLM() bool {
	return true // Always allow LLM tasks to start
}

func (s *ParallelLLMStrategy) CanStartData() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.dataRunning
}

func (s *ParallelLLMStrategy) OnStartLLM() {
	// No-op: we don't track LLM tasks
}

func (s *ParallelLLMStrategy) OnStartData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataRunning = true
}

func (s *ParallelLLMStrategy) OnCompleteLLM() {
	// No-op: we don't track LLM tasks
}

func (s *ParallelLLMStrategy) OnCompleteData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataRunning = false
}

// ============================================================================
// ThrottledLLMStrategy - Up to N parallel LLM tasks
// ============================================================================

// ThrottledLLMStrategy allows up to maxConcurrent LLM tasks to run in parallel.
// Data tasks are still serialized (only one at a time).
type ThrottledLLMStrategy struct {
	mu            sync.Mutex
	maxConcurrent int
	llmRunning    int
	dataRunning   bool
}

// NewThrottledLLMStrategy creates a strategy that allows up to maxConcurrent
// LLM tasks to run in parallel while serializing data tasks.
func NewThrottledLLMStrategy(maxConcurrent int) *ThrottledLLMStrategy {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &ThrottledLLMStrategy{
		maxConcurrent: maxConcurrent,
	}
}

func (s *ThrottledLLMStrategy) CanStartLLM() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.llmRunning < s.maxConcurrent
}

func (s *ThrottledLLMStrategy) CanStartData() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.dataRunning
}

func (s *ThrottledLLMStrategy) OnStartLLM() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmRunning++
}

func (s *ThrottledLLMStrategy) OnStartData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataRunning = true
}

func (s *ThrottledLLMStrategy) OnCompleteLLM() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.llmRunning > 0 {
		s.llmRunning--
	}
}

func (s *ThrottledLLMStrategy) OnCompleteData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataRunning = false
}

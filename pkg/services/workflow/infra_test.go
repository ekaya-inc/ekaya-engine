package workflow

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// mockWorkflowRepo is a minimal mock for testing infrastructure behavior.
type mockWorkflowRepo struct {
	heartbeatCalls   int32
	taskQueueUpdates [][]models.WorkflowTask
	mu               sync.Mutex
}

func (m *mockWorkflowRepo) UpdateHeartbeat(ctx context.Context, workflowID, ownerID uuid.UUID) error {
	atomic.AddInt32(&m.heartbeatCalls, 1)
	return nil
}

func (m *mockWorkflowRepo) UpdateTaskQueue(ctx context.Context, id uuid.UUID, tasks []models.WorkflowTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskQueueUpdates = append(m.taskQueueUpdates, tasks)
	return nil
}

// Implement remaining interface methods as no-ops for testing
func (m *mockWorkflowRepo) Create(ctx context.Context, workflow *models.OntologyWorkflow) error {
	return nil
}
func (m *mockWorkflowRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}
func (m *mockWorkflowRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}
func (m *mockWorkflowRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}
func (m *mockWorkflowRepo) GetLatestByDatasourceAndPhase(ctx context.Context, datasourceID uuid.UUID, phase models.WorkflowPhaseType) (*models.OntologyWorkflow, error) {
	return nil, nil
}
func (m *mockWorkflowRepo) Update(ctx context.Context, workflow *models.OntologyWorkflow) error {
	return nil
}
func (m *mockWorkflowRepo) UpdateState(ctx context.Context, id uuid.UUID, state models.WorkflowState, errorMsg string) error {
	return nil
}
func (m *mockWorkflowRepo) UpdateProgress(ctx context.Context, id uuid.UUID, progress *models.WorkflowProgress) error {
	return nil
}
func (m *mockWorkflowRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockWorkflowRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockWorkflowRepo) ClaimOwnership(ctx context.Context, workflowID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockWorkflowRepo) ReleaseOwnership(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}

func (m *mockWorkflowRepo) getTaskQueueUpdates() [][]models.WorkflowTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.taskQueueUpdates
}

// mockTenantCtxFunc returns a simple context function for testing.
func mockTenantCtxFunc() TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}
}

func TestNewWorkflowInfra(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}

	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	if infra == nil {
		t.Fatal("expected non-nil infra")
	}
	if infra.ServerInstanceID() == uuid.Nil {
		t.Error("expected non-nil server instance ID")
	}
}

func TestServerInstanceID_Uniqueness(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}

	infra1 := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)
	infra2 := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	if infra1.ServerInstanceID() == infra2.ServerInstanceID() {
		t.Error("expected different server instance IDs for different infra instances")
	}
}

func TestHeartbeat_StartStop(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()

	// Start heartbeat
	infra.StartHeartbeat(workflowID, projectID)

	// Verify it's stored
	_, ok := infra.heartbeatStop.Load(workflowID)
	if !ok {
		t.Error("expected heartbeat info to be stored")
	}

	// Stop heartbeat
	infra.StopHeartbeat(workflowID)

	// Verify it's removed
	_, ok = infra.heartbeatStop.Load(workflowID)
	if ok {
		t.Error("expected heartbeat info to be removed after stop")
	}
}

func TestHeartbeat_StopNonExistent(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Should not panic when stopping non-existent heartbeat
	infra.StopHeartbeat(uuid.New())
}

func TestTaskQueueWriter_StartStop(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()

	// Start writer
	infra.StartTaskQueueWriter(workflowID)

	// Verify it's stored
	_, ok := infra.taskQueueWriters.Load(workflowID)
	if !ok {
		t.Error("expected task queue writer to be stored")
	}

	// Stop writer
	infra.StopTaskQueueWriter(workflowID)

	// Verify it's removed
	_, ok = infra.taskQueueWriters.Load(workflowID)
	if ok {
		t.Error("expected task queue writer to be removed after stop")
	}
}

func TestTaskQueueWriter_StopNonExistent(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Should not panic when stopping non-existent writer
	infra.StopTaskQueueWriter(uuid.New())
}

func TestSendTaskQueueUpdate_Success(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()

	// Start writer
	infra.StartTaskQueueWriter(workflowID)

	// Send update
	sent := infra.SendTaskQueueUpdate(TaskQueueUpdate{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Tasks:      []models.WorkflowTask{{Name: "test-task", Status: "pending"}},
	})

	if !sent {
		t.Error("expected update to be sent successfully")
	}

	// Give the writer time to process
	time.Sleep(50 * time.Millisecond)

	// Stop writer to flush
	infra.StopTaskQueueWriter(workflowID)

	// Verify update was persisted
	updates := repo.getTaskQueueUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if len(updates[0]) != 1 || updates[0][0].Name != "test-task" {
		t.Error("expected task to be persisted with correct name")
	}
}

func TestSendTaskQueueUpdate_NoWriter(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Try to send update without starting writer
	sent := infra.SendTaskQueueUpdate(TaskQueueUpdate{
		ProjectID:  uuid.New(),
		WorkflowID: uuid.New(),
		Tasks:      []models.WorkflowTask{},
	})

	if sent {
		t.Error("expected update to fail when no writer exists")
	}
}

func TestTaskQueueWriter_Debouncing(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()

	infra.StartTaskQueueWriter(workflowID)

	// Send multiple updates rapidly - only the last should be persisted
	for i := 0; i < 10; i++ {
		infra.SendTaskQueueUpdate(TaskQueueUpdate{
			ProjectID:  projectID,
			WorkflowID: workflowID,
			Tasks:      []models.WorkflowTask{{Name: "task", Status: "pending"}},
		})
	}

	// Give writer time to drain and process
	time.Sleep(100 * time.Millisecond)
	infra.StopTaskQueueWriter(workflowID)

	// Due to debouncing, we should have fewer persists than sends
	updates := repo.getTaskQueueUpdates()
	if len(updates) >= 10 {
		t.Errorf("expected debouncing to reduce persists, got %d updates", len(updates))
	}
}

func TestQueueManagement(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	queue := workqueue.New(logger)

	// Store
	infra.StoreQueue(workflowID, queue)

	// Load
	loaded, ok := infra.LoadQueue(workflowID)
	if !ok {
		t.Error("expected queue to be found")
	}
	if loaded != queue {
		t.Error("expected same queue to be returned")
	}

	// Delete
	infra.DeleteQueue(workflowID)

	// Load again - should not exist
	_, ok = infra.LoadQueue(workflowID)
	if ok {
		t.Error("expected queue to be deleted")
	}
}

func TestLoadQueue_NonExistent(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	_, ok := infra.LoadQueue(uuid.New())
	if ok {
		t.Error("expected queue not to be found")
	}
}

func TestMapTaskStatus(t *testing.T) {
	tests := []struct {
		input    workqueue.TaskStatus
		expected string
	}{
		{workqueue.TaskStatusPending, "pending"},
		{workqueue.TaskStatusRunning, "running"},
		{workqueue.TaskStatusCompleted, "completed"},
		{workqueue.TaskStatusFailed, "failed"},
		{workqueue.TaskStatusCancelled, "failed"},
		{workqueue.TaskStatusPaused, "paused"},
		{workqueue.TaskStatus("unknown-status"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := MapTaskStatus(tt.input)
			if result != tt.expected {
				t.Errorf("MapTaskStatus(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Set up active workflows
	workflowID1 := uuid.New()
	workflowID2 := uuid.New()
	projectID := uuid.New()

	queue1 := workqueue.New(logger)
	queue2 := workqueue.New(logger)

	infra.StoreQueue(workflowID1, queue1)
	infra.StoreQueue(workflowID2, queue2)
	infra.StartHeartbeat(workflowID1, projectID)
	infra.StartHeartbeat(workflowID2, projectID)
	infra.StartTaskQueueWriter(workflowID1)
	infra.StartTaskQueueWriter(workflowID2)

	// Track shutdown callback invocations
	var callbackCalls int32
	shutdownFn := func(wfID, pID uuid.UUID, q *workqueue.Queue) {
		atomic.AddInt32(&callbackCalls, 1)
	}

	// Shutdown
	err := infra.Shutdown(context.Background(), shutdownFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify callbacks were called
	if atomic.LoadInt32(&callbackCalls) != 2 {
		t.Errorf("expected 2 shutdown callbacks, got %d", callbackCalls)
	}

	// Verify all resources cleaned up
	_, ok := infra.LoadQueue(workflowID1)
	if ok {
		t.Error("expected queue1 to be deleted")
	}
	_, ok = infra.LoadQueue(workflowID2)
	if ok {
		t.Error("expected queue2 to be deleted")
	}
}

func TestShutdown_NilCallback(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()
	queue := workqueue.New(logger)

	infra.StoreQueue(workflowID, queue)
	infra.StartHeartbeat(workflowID, projectID)

	// Should not panic with nil callback
	err := infra.Shutdown(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShutdown_EmptyInfra(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Should not error with no active workflows
	err := infra.Shutdown(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShutdown_WithTimeout(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()
	queue := workqueue.New(logger)

	infra.StoreQueue(workflowID, queue)
	infra.StartHeartbeat(workflowID, projectID)
	infra.StartTaskQueueWriter(workflowID)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Create a blocking shutdown function that would take too long
	shutdownFn := func(wfID, pID uuid.UUID, q *workqueue.Queue) {
		// Simulate slow cleanup by sleeping longer than context timeout
		time.Sleep(100 * time.Millisecond)
	}

	// Shutdown should return context error due to timeout
	err := infra.Shutdown(ctx, shutdownFn)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestSendTaskQueueUpdate_BufferFull(t *testing.T) {
	logger := zap.NewNop()
	// Create a blocking repo that waits on a channel before returning
	blockingRepo := &blockingMockWorkflowRepo{
		unblock: make(chan struct{}),
	}
	infra := NewWorkflowInfra(blockingRepo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()

	infra.StartTaskQueueWriter(workflowID)

	// Send first update which will block in the repo
	infra.SendTaskQueueUpdate(TaskQueueUpdate{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Tasks:      []models.WorkflowTask{{Name: "blocking-task", Status: "pending"}},
	})

	// Give writer time to pick up the first update and start blocking
	time.Sleep(50 * time.Millisecond)

	// Now fill the buffer (buffer size is 100)
	var sentCount int
	for i := 0; i < 150; i++ {
		sent := infra.SendTaskQueueUpdate(TaskQueueUpdate{
			ProjectID:  projectID,
			WorkflowID: workflowID,
			Tasks:      []models.WorkflowTask{{Name: "task", Status: "pending"}},
		})
		if sent {
			sentCount++
		}
	}

	// The first 100 should succeed (buffer size), rest should fail
	if sentCount < 100 {
		t.Errorf("expected at least 100 successful sends, got %d", sentCount)
	}
	if sentCount >= 150 {
		t.Error("expected some sends to fail due to buffer full")
	}

	// Unblock the repo and clean up
	close(blockingRepo.unblock)
	infra.StopTaskQueueWriter(workflowID)
}

// blockingMockWorkflowRepo blocks on UpdateTaskQueue until unblock channel is closed
type blockingMockWorkflowRepo struct {
	mockWorkflowRepo
	unblock chan struct{}
}

func (m *blockingMockWorkflowRepo) UpdateTaskQueue(ctx context.Context, id uuid.UUID, tasks []models.WorkflowTask) error {
	<-m.unblock // Block until unblock is closed
	return m.mockWorkflowRepo.UpdateTaskQueue(ctx, id, tasks)
}

func TestConcurrentQueueOperations(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Perform concurrent store/load/delete operations
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				workflowID := uuid.New()
				queue := workqueue.New(logger)

				// Store
				infra.StoreQueue(workflowID, queue)

				// Load
				loaded, ok := infra.LoadQueue(workflowID)
				if ok && loaded != queue {
					t.Errorf("loaded queue doesn't match stored queue")
				}

				// Delete
				infra.DeleteQueue(workflowID)

				// Verify deleted
				_, ok = infra.LoadQueue(workflowID)
				if ok {
					t.Errorf("queue should be deleted")
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentHeartbeatOperations(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	// Start and stop heartbeats concurrently
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			workflowID := uuid.New()
			projectID := uuid.New()

			// Start
			infra.StartHeartbeat(workflowID, projectID)

			// Small delay to let heartbeat goroutine start
			time.Sleep(10 * time.Millisecond)

			// Stop
			infra.StopHeartbeat(workflowID)
		}()
	}

	wg.Wait()
}

func TestTaskQueueWriter_PersistsOnClose(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockWorkflowRepo{}
	infra := NewWorkflowInfra(repo, mockTenantCtxFunc(), logger)

	workflowID := uuid.New()
	projectID := uuid.New()

	infra.StartTaskQueueWriter(workflowID)

	// Send an update
	infra.SendTaskQueueUpdate(TaskQueueUpdate{
		ProjectID:  projectID,
		WorkflowID: workflowID,
		Tasks:      []models.WorkflowTask{{Name: "final-task", Status: "running"}},
	})

	// Stop writer - should persist the pending update
	infra.StopTaskQueueWriter(workflowID)

	// Verify the update was persisted
	updates := repo.getTaskQueueUpdates()
	if len(updates) == 0 {
		t.Error("expected update to be persisted on close")
	}

	// Check the final task was persisted
	found := false
	for _, update := range updates {
		for _, task := range update {
			if task.Name == "final-task" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected final-task to be persisted")
	}
}

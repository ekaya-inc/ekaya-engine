package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockConversationRepo tracks saved and updated conversations for testing.
type mockConversationRepo struct {
	mu        sync.Mutex
	saved     []*models.LLMConversation
	updated   []*models.LLMConversation
	saveErr   error
	updateErr error
}

func (m *mockConversationRepo) Save(ctx context.Context, conv *models.LLMConversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, conv)
	return nil
}

func (m *mockConversationRepo) Update(ctx context.Context, conv *models.LLMConversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updated = append(m.updated, conv)
	return nil
}

func (m *mockConversationRepo) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (m *mockConversationRepo) GetByContext(ctx context.Context, projectID uuid.UUID, key, value string) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (m *mockConversationRepo) GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (m *mockConversationRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockConversationRepo) getSaved() []*models.LLMConversation {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saved
}

func (m *mockConversationRepo) getUpdated() []*models.LLMConversation {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updated
}

func TestAsyncConversationRecorder_RecordsSingleConversation(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	// Simple tenant context function that just returns the context
	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 10)

	conv := &models.LLMConversation{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Model:     "gpt-4",
		Status:    models.LLMConversationStatusSuccess,
	}

	recorder.Record(conv)
	recorder.Close() // Close waits for pending records

	saved := repo.getSaved()
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved conversation, got %d", len(saved))
	}
	if saved[0].ID != conv.ID {
		t.Errorf("expected ID %s, got %s", conv.ID, saved[0].ID)
	}
}

func TestAsyncConversationRecorder_RecordsMultipleConversations(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 10)

	// Record multiple conversations
	for i := 0; i < 5; i++ {
		recorder.Record(&models.LLMConversation{
			ID:        uuid.New(),
			ProjectID: uuid.New(),
			Model:     "gpt-4",
			Status:    models.LLMConversationStatusSuccess,
		})
	}

	recorder.Close()

	saved := repo.getSaved()
	if len(saved) != 5 {
		t.Errorf("expected 5 saved conversations, got %d", len(saved))
	}
}

func TestAsyncConversationRecorder_DropsWhenQueueFull(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	// Use a blocking tenant context to simulate slow saves
	blockCh := make(chan struct{})
	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		<-blockCh // Block until we're ready
		return ctx, func() {}, nil
	}

	// Very small queue size
	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 2)

	// Fill the queue
	for i := 0; i < 5; i++ {
		recorder.Record(&models.LLMConversation{
			ID:        uuid.New(),
			ProjectID: uuid.New(),
		})
	}

	// Unblock and close
	close(blockCh)
	recorder.Close()

	// We should have at most 2 saved (the queue size) plus 1 that might have been processing
	saved := repo.getSaved()
	if len(saved) > 3 {
		t.Errorf("expected at most 3 saved conversations (queue was full), got %d", len(saved))
	}
}

func TestAsyncConversationRecorder_DefaultQueueSize(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	// Pass 0 for queue size to use default
	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 0)

	// Should be able to queue many items without blocking
	for i := 0; i < 50; i++ {
		recorder.Record(&models.LLMConversation{
			ID:        uuid.New(),
			ProjectID: uuid.New(),
		})
	}

	recorder.Close() // Close waits for all records

	saved := repo.getSaved()
	if len(saved) != 50 {
		t.Errorf("expected 50 saved conversations, got %d", len(saved))
	}
}

func TestAsyncConversationRecorder_HandlesRepoError(t *testing.T) {
	repo := &mockConversationRepo{
		saveErr: errors.New("database connection failed"),
	}
	logger := zap.NewNop()

	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 10)

	recorder.Record(&models.LLMConversation{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
	})

	recorder.Close()

	// Should not panic or hang, just log the error
	saved := repo.getSaved()
	if len(saved) != 0 {
		t.Errorf("expected 0 saved (error), got %d", len(saved))
	}
}

func TestAsyncConversationRecorder_HandlesTenantContextError(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return nil, nil, errors.New("failed to acquire connection")
	}

	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 10)

	recorder.Record(&models.LLMConversation{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
	})

	recorder.Close()

	// Should not panic or hang, just log the error
	saved := repo.getSaved()
	if len(saved) != 0 {
		t.Errorf("expected 0 saved (context error), got %d", len(saved))
	}
}

func TestAsyncConversationRecorder_CloseWaitsForPendingRecords(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	var saveCount int
	var mu sync.Mutex

	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		// Small delay to simulate real save
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		saveCount++
		mu.Unlock()
		return ctx, func() {}, nil
	}

	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 10)

	// Queue records
	for i := 0; i < 5; i++ {
		recorder.Record(&models.LLMConversation{
			ID:        uuid.New(),
			ProjectID: uuid.New(),
		})
	}

	// Close should wait for all pending records
	recorder.Close()

	mu.Lock()
	count := saveCount
	mu.Unlock()

	if count != 5 {
		t.Errorf("expected all 5 records to be processed before Close returns, got %d", count)
	}
}

func TestAsyncConversationRecorder_CallsCleanupOnTenantContext(t *testing.T) {
	repo := &mockConversationRepo{}
	logger := zap.NewNop()

	cleanupCalled := false
	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() { cleanupCalled = true }, nil
	}

	recorder := NewAsyncConversationRecorder(repo, getTenantCtx, logger, 10)

	recorder.Record(&models.LLMConversation{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
	})

	recorder.Close()

	if !cleanupCalled {
		t.Error("expected cleanup function to be called")
	}
}

func TestAsyncConversationRecorder_ImplementsInterface(t *testing.T) {
	// Compile-time check that AsyncConversationRecorder implements ConversationRecorder
	var _ ConversationRecorder = (*AsyncConversationRecorder)(nil)
}

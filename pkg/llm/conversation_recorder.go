package llm

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// TenantContextFunc acquires a tenant-scoped database connection for background work.
// Returns the scoped context, a cleanup function (MUST be called), and any error.
type TenantContextFunc func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error)

// ConversationRecorder records LLM conversations to the database.
type ConversationRecorder interface {
	// Record queues a completed conversation for async persistence (legacy, for backwards compat).
	Record(conv *models.LLMConversation)

	// SavePending synchronously inserts a pending record before the LLM call starts.
	// This enables tracking in-flight requests. Returns error if insert fails.
	SavePending(ctx context.Context, conv *models.LLMConversation) error

	// RecordCompletion queues an update for a pending record after the LLM call completes.
	// This is async to avoid blocking the response.
	RecordCompletion(conv *models.LLMConversation)
}

// recordOp represents a database operation for conversation recording.
type recordOp struct {
	conv     *models.LLMConversation
	isUpdate bool // true = update existing record, false = insert new record
}

// AsyncConversationRecorder records conversations asynchronously to avoid blocking LLM calls.
type AsyncConversationRecorder struct {
	repo         repositories.ConversationRepository
	getTenantCtx TenantContextFunc
	logger       *zap.Logger
	queue        chan recordOp
	done         chan struct{}
}

// NewAsyncConversationRecorder creates a new async recorder.
// queueSize controls the buffer size - if full, records are dropped with a warning.
func NewAsyncConversationRecorder(
	repo repositories.ConversationRepository,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
	queueSize int,
) *AsyncConversationRecorder {
	if queueSize <= 0 {
		queueSize = 100
	}

	r := &AsyncConversationRecorder{
		repo:         repo,
		getTenantCtx: getTenantCtx,
		logger:       logger.Named("conversation-recorder"),
		queue:        make(chan recordOp, queueSize),
		done:         make(chan struct{}),
	}

	go r.processQueue()

	return r
}

// Record queues a conversation for async persistence.
// Non-blocking - if queue is full, the record is dropped with a warning.
func (r *AsyncConversationRecorder) Record(conv *models.LLMConversation) {
	select {
	case r.queue <- recordOp{conv: conv, isUpdate: false}:
		// Queued successfully
	default:
		r.logger.Warn("Conversation record queue full, dropping entry",
			zap.String("project_id", conv.ProjectID.String()),
			zap.String("model", conv.Model))
	}
}

// SavePending synchronously inserts a pending record before the LLM call starts.
// This enables tracking in-flight requests by querying for status='pending'.
func (r *AsyncConversationRecorder) SavePending(ctx context.Context, conv *models.LLMConversation) error {
	// Ensure status is pending
	conv.Status = models.LLMConversationStatusPending

	// Synchronous save - we need the record to exist before the LLM call
	if err := r.repo.Save(ctx, conv); err != nil {
		r.logger.Error("Failed to save pending LLM conversation",
			zap.String("project_id", conv.ProjectID.String()),
			zap.String("model", conv.Model),
			zap.Error(err))
		return err
	}

	r.logger.Debug("Saved pending LLM conversation",
		zap.String("id", conv.ID.String()),
		zap.String("project_id", conv.ProjectID.String()),
		zap.String("model", conv.Model))

	return nil
}

// RecordCompletion queues an update for a pending record after the LLM call completes.
// Non-blocking - if queue is full, the update is dropped with a warning.
func (r *AsyncConversationRecorder) RecordCompletion(conv *models.LLMConversation) {
	select {
	case r.queue <- recordOp{conv: conv, isUpdate: true}:
		// Queued successfully
	default:
		r.logger.Warn("Conversation completion queue full, dropping update",
			zap.String("id", conv.ID.String()),
			zap.String("project_id", conv.ProjectID.String()))
	}
}

// Close stops the recorder and waits for pending records to be saved.
func (r *AsyncConversationRecorder) Close() {
	close(r.queue)
	<-r.done
}

// processQueue processes queued conversation operations (saves and updates).
func (r *AsyncConversationRecorder) processQueue() {
	defer close(r.done)

	for op := range r.queue {
		if op.isUpdate {
			r.updateConversation(op.conv)
		} else {
			r.saveConversation(op.conv)
		}
	}
}

// saveConversation persists a single conversation record.
func (r *AsyncConversationRecorder) saveConversation(conv *models.LLMConversation) {
	// Acquire tenant-scoped context
	ctx, cleanup, err := r.getTenantCtx(context.Background(), conv.ProjectID)
	if err != nil {
		r.logger.Error("Failed to acquire tenant context for conversation save",
			zap.String("project_id", conv.ProjectID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	// Save to database
	if err := r.repo.Save(ctx, conv); err != nil {
		r.logger.Error("Failed to save LLM conversation",
			zap.String("project_id", conv.ProjectID.String()),
			zap.String("model", conv.Model),
			zap.Error(err))
		return
	}

	r.logger.Debug("Saved LLM conversation",
		zap.String("project_id", conv.ProjectID.String()),
		zap.String("model", conv.Model),
		zap.Int("duration_ms", conv.DurationMs))
}

// updateConversation updates an existing conversation record with completion data.
func (r *AsyncConversationRecorder) updateConversation(conv *models.LLMConversation) {
	// Acquire tenant-scoped context
	ctx, cleanup, err := r.getTenantCtx(context.Background(), conv.ProjectID)
	if err != nil {
		r.logger.Error("Failed to acquire tenant context for conversation update",
			zap.String("id", conv.ID.String()),
			zap.String("project_id", conv.ProjectID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	// Update in database
	if err := r.repo.Update(ctx, conv); err != nil {
		r.logger.Error("Failed to update LLM conversation",
			zap.String("id", conv.ID.String()),
			zap.String("project_id", conv.ProjectID.String()),
			zap.String("status", conv.Status),
			zap.Error(err))
		return
	}

	r.logger.Debug("Updated LLM conversation",
		zap.String("id", conv.ID.String()),
		zap.String("project_id", conv.ProjectID.String()),
		zap.String("status", conv.Status),
		zap.Int("duration_ms", conv.DurationMs))
}

var _ ConversationRecorder = (*AsyncConversationRecorder)(nil)

package llm

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// RecordingClient wraps an LLMClient to record all conversations to the database.
type RecordingClient struct {
	inner     LLMClient
	recorder  ConversationRecorder
	projectID uuid.UUID
}

// NewRecordingClient creates a new recording wrapper around an LLMClient.
func NewRecordingClient(inner LLMClient, recorder ConversationRecorder, projectID uuid.UUID) *RecordingClient {
	return &RecordingClient{
		inner:     inner,
		recorder:  recorder,
		projectID: projectID,
	}
}

// GenerateResponse calls the inner client and records the conversation.
// It first inserts a "pending" record, then updates it with the response.
func (c *RecordingClient) GenerateResponse(
	ctx context.Context,
	prompt string,
	systemMessage string,
	temperature float64,
	thinking bool,
) (*GenerateResponseResult, error) {
	// Build request messages for recording (verbatim)
	requestMessages := []any{
		map[string]string{"role": "system", "content": systemMessage},
		map[string]string{"role": "user", "content": prompt},
	}

	// Build the pending conversation record
	conv := &models.LLMConversation{
		ID:              uuid.New(),
		ProjectID:       c.projectID,
		Context:         GetContext(ctx),
		Iteration:       1,
		Endpoint:        c.inner.GetEndpoint(),
		Model:           c.inner.GetModel(),
		RequestMessages: requestMessages,
		Temperature:     &temperature,
		Status:          models.LLMConversationStatusPending,
	}

	// Write request file before LLM call (debug builds only)
	debugPrefix := debugWriteRequest(conv.ID, conv.Model, systemMessage, prompt)

	// Insert pending record synchronously (enables in-flight tracking)
	// If this fails, we still proceed with the LLM call - recording is best-effort
	pendingSaved := c.recorder.SavePending(ctx, conv) == nil

	start := time.Now()

	// Call the inner client
	result, err := c.inner.GenerateResponse(ctx, prompt, systemMessage, temperature, thinking)

	duration := time.Since(start)

	// Update the conversation record with response data
	conv.DurationMs = int(duration.Milliseconds())

	if err != nil {
		conv.Status = models.LLMConversationStatusError
		conv.ErrorMessage = err.Error()
		// Write error file (debug builds only)
		debugWriteError(debugPrefix, conv.ID.String(), conv.Model, err.Error(), int64(conv.DurationMs))
		// Return partial result with conversation ID for debugging
		result = &GenerateResponseResult{ConversationID: conv.ID}
	} else {
		conv.Status = models.LLMConversationStatusSuccess
		if result != nil {
			result.ConversationID = conv.ID
			conv.ResponseContent = result.Content
			conv.PromptTokens = &result.PromptTokens
			conv.CompletionTokens = &result.CompletionTokens
			conv.TotalTokens = &result.TotalTokens
			// Write response file (debug builds only)
			debugWriteResponse(debugPrefix, conv.ID.String(), conv.Model, result.Content, int64(conv.DurationMs))
		}
	}

	// Record completion asynchronously
	if pendingSaved {
		// Update the existing pending record
		c.recorder.RecordCompletion(conv)
	} else {
		// Fallback: insert as a new record (legacy behavior)
		c.recorder.Record(conv)
	}

	return result, err
}

// CreateEmbedding delegates to the inner client (not recorded).
func (c *RecordingClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return c.inner.CreateEmbedding(ctx, input, model)
}

// CreateEmbeddings delegates to the inner client (not recorded).
func (c *RecordingClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return c.inner.CreateEmbeddings(ctx, inputs, model)
}

// GetModel returns the inner client's model.
func (c *RecordingClient) GetModel() string {
	return c.inner.GetModel()
}

// GetEndpoint returns the inner client's endpoint.
func (c *RecordingClient) GetEndpoint() string {
	return c.inner.GetEndpoint()
}

var _ LLMClient = (*RecordingClient)(nil)

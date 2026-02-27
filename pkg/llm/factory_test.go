package llm

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mock AIConfigProvider
// ============================================================================

type mockAIConfigProvider struct {
	getEffectiveFunc func(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error)
}

func (m *mockAIConfigProvider) GetEffective(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
	if m.getEffectiveFunc != nil {
		return m.getEffectiveFunc(ctx, projectID)
	}
	return &models.AIConfig{
		LLMBaseURL: "http://localhost:8080",
		LLMModel:   "test-model",
		LLMAPIKey:  "test-key",
	}, nil
}

// ============================================================================
// CreateForProject tests
// ============================================================================

func TestClientFactory_CreateForProject_ConfigProviderError(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{
		getEffectiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
			return nil, assert.AnError
		},
	}, zap.NewNop())

	_, err := factory.CreateForProject(context.Background(), uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get effective config")
}

func TestClientFactory_CreateForProject_ValidConfig(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{}, zap.NewNop())

	client, err := factory.CreateForProject(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NotNil(t, client)
	// Should be a plain *Client (not wrapped)
	_, isRecording := client.(*RecordingClient)
	assert.False(t, isRecording, "should not be a RecordingClient when no recorder is set")
}

func TestClientFactory_CreateForProject_RecorderWrapsClient(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{}, zap.NewNop())
	factory.SetRecorder(&mockConversationRecorder{})

	client, err := factory.CreateForProject(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NotNil(t, client)
	_, isRecording := client.(*RecordingClient)
	assert.True(t, isRecording, "should be a RecordingClient when recorder is set")
}

// ============================================================================
// CreateEmbeddingClient tests
// ============================================================================

func TestClientFactory_CreateEmbeddingClient_UsesEmbeddingConfig(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{
		getEffectiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
			return &models.AIConfig{
				LLMBaseURL:       "http://llm-host:8080",
				LLMAPIKey:        "llm-key",
				LLMModel:         "llm-model",
				EmbeddingBaseURL: "http://embed-host:8080",
				EmbeddingAPIKey:  "embed-key",
				EmbeddingModel:   "embed-model",
			}, nil
		},
	}, zap.NewNop())

	client, err := factory.CreateEmbeddingClient(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NotNil(t, client)
	// Verify it uses the embedding endpoint, not LLM endpoint
	assert.Equal(t, "http://embed-host:8080", client.GetEndpoint())
	assert.Equal(t, "embed-model", client.GetModel())
}

func TestClientFactory_CreateEmbeddingClient_FallsBackToLLMConfig(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{
		getEffectiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
			return &models.AIConfig{
				LLMBaseURL:     "http://llm-host:8080",
				LLMAPIKey:      "llm-key",
				LLMModel:       "llm-model",
				EmbeddingModel: "embed-model",
				// EmbeddingBaseURL and EmbeddingAPIKey empty — should fall back
			}, nil
		},
	}, zap.NewNop())

	client, err := factory.CreateEmbeddingClient(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NotNil(t, client)
	// Effective URL/key fall back to LLM config
	assert.Equal(t, "http://llm-host:8080", client.GetEndpoint())
}

// ============================================================================
// CreateStreamingClient tests
// ============================================================================

func TestClientFactory_CreateStreamingClient_ConfigProviderError(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{
		getEffectiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
			return nil, assert.AnError
		},
	}, zap.NewNop())

	_, err := factory.CreateStreamingClient(context.Background(), uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get effective config")
}

func TestClientFactory_CreateStreamingClient_ValidConfig(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{}, zap.NewNop())

	client, err := factory.CreateStreamingClient(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NotNil(t, client)
	assert.IsType(t, &StreamingClient{}, client)
}

// ============================================================================
// SetRecorder tests
// ============================================================================

func TestClientFactory_SetRecorder_NilDisablesWrapping(t *testing.T) {
	factory := NewClientFactory(&mockAIConfigProvider{}, zap.NewNop())
	factory.SetRecorder(&mockConversationRecorder{})
	factory.SetRecorder(nil) // Disable

	client, err := factory.CreateForProject(context.Background(), uuid.New())

	require.NoError(t, err)
	_, isRecording := client.(*RecordingClient)
	assert.False(t, isRecording, "should not wrap when recorder is nil")
}

// ============================================================================
// Mock ConversationRecorder
// ============================================================================

type mockConversationRecorder struct{}

func (m *mockConversationRecorder) Record(conv *models.LLMConversation) {}
func (m *mockConversationRecorder) SavePending(ctx context.Context, conv *models.LLMConversation) error {
	return nil
}
func (m *mockConversationRecorder) RecordCompletion(conv *models.LLMConversation) {}

package llm

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AIConfigProvider provides AI configuration for projects.
// This interface breaks the import cycle between llm and services packages.
// The services.AIConfigService implements this interface.
type AIConfigProvider interface {
	GetEffective(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error)
}

// LLMClientFactory is the interface for creating LLM clients.
// Use this interface for dependency injection and testing.
type LLMClientFactory interface {
	CreateForProject(ctx context.Context, projectID uuid.UUID) (LLMClient, error)
	CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (LLMClient, error)
	CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*StreamingClient, error)
}

// ClientFactory creates LLM clients based on project AI configuration.
type ClientFactory struct {
	aiConfigProvider AIConfigProvider
	recorder         ConversationRecorder // Optional: if set, wraps clients to record conversations
	logger           *zap.Logger
}

// SetRecorder enables conversation recording for all clients created by this factory.
// Pass nil to disable recording.
func (f *ClientFactory) SetRecorder(recorder ConversationRecorder) {
	f.recorder = recorder
}

// NewClientFactory creates a new factory.
func NewClientFactory(
	aiConfigProvider AIConfigProvider,
	logger *zap.Logger,
) *ClientFactory {
	return &ClientFactory{
		aiConfigProvider: aiConfigProvider,
		logger:           logger,
	}
}

// CreateForProject creates an LLM client configured for a project.
// Resolves project config with server defaults for community/embedded.
// Returns LLMClient interface to enable dependency injection of mocks.
// If a recorder is set, the client is wrapped to record all conversations.
func (f *ClientFactory) CreateForProject(ctx context.Context, projectID uuid.UUID) (LLMClient, error) {
	effectiveConfig, err := f.aiConfigProvider.GetEffective(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get effective config: %w", err)
	}

	client, err := NewClient(&Config{
		Endpoint:  effectiveConfig.LLMBaseURL,
		Model:     effectiveConfig.LLMModel,
		APIKey:    effectiveConfig.LLMAPIKey,
		ProjectID: projectID.String(),
	}, f.logger)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Wrap with recording if enabled
	if f.recorder != nil {
		return NewRecordingClient(client, f.recorder, projectID), nil
	}

	return client, nil
}

// CreateEmbeddingClient creates a client specifically for embeddings.
// Uses embedding-specific config if available, falls back to LLM config.
// Returns LLMClient interface to enable dependency injection of mocks.
func (f *ClientFactory) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (LLMClient, error) {
	effectiveConfig, err := f.aiConfigProvider.GetEffective(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get effective config: %w", err)
	}

	client, err := NewClient(&Config{
		Endpoint:  effectiveConfig.EffectiveEmbeddingBaseURL(),
		Model:     effectiveConfig.EmbeddingModel,
		APIKey:    effectiveConfig.EffectiveEmbeddingAPIKey(),
		ProjectID: projectID.String(),
	}, f.logger)
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}

	return client, nil
}

// CreateStreamingClient creates a streaming-capable LLM client for a project.
// Use this for chat and tool-calling scenarios that require streaming responses.
func (f *ClientFactory) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*StreamingClient, error) {
	effectiveConfig, err := f.aiConfigProvider.GetEffective(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get effective config: %w", err)
	}

	client, err := NewStreamingClient(&Config{
		Endpoint:  effectiveConfig.LLMBaseURL,
		Model:     effectiveConfig.LLMModel,
		APIKey:    effectiveConfig.LLMAPIKey,
		ProjectID: projectID.String(),
	}, f.logger)
	if err != nil {
		return nil, fmt.Errorf("create streaming client: %w", err)
	}

	return client, nil
}

// Ensure ClientFactory implements LLMClientFactory at compile time.
var _ LLMClientFactory = (*ClientFactory)(nil)

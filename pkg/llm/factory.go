package llm

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ClientFactory creates LLM clients based on project AI configuration.
type ClientFactory struct {
	aiConfigService services.AIConfigService
	logger          *zap.Logger
}

// NewClientFactory creates a new factory.
func NewClientFactory(
	aiConfigService services.AIConfigService,
	logger *zap.Logger,
) *ClientFactory {
	return &ClientFactory{
		aiConfigService: aiConfigService,
		logger:          logger,
	}
}

// CreateForProject creates an LLM client configured for a project.
// Resolves project config with server defaults for community/embedded.
// Returns LLMClient interface to enable dependency injection of mocks.
func (f *ClientFactory) CreateForProject(ctx context.Context, projectID uuid.UUID) (LLMClient, error) {
	effectiveConfig, err := f.aiConfigService.GetEffective(ctx, projectID)
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

	return client, nil
}

// CreateEmbeddingClient creates a client specifically for embeddings.
// Uses embedding-specific config if available, falls back to LLM config.
// Returns LLMClient interface to enable dependency injection of mocks.
func (f *ClientFactory) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (LLMClient, error) {
	effectiveConfig, err := f.aiConfigService.GetEffective(ctx, projectID)
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

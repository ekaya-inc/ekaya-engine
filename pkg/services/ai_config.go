package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AIConfigService defines the interface for AI configuration operations.
type AIConfigService interface {
	// Get retrieves the AI config for a project. Returns nil, nil if no config exists.
	Get(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error)

	// Upsert creates or updates the AI config for a project.
	// Replaces any prior selection - only one config_type can be active at a time.
	Upsert(ctx context.Context, projectID uuid.UUID, config *models.AIConfig) error

	// Delete removes the AI config for a project (sets config_type to "none").
	Delete(ctx context.Context, projectID uuid.UUID) error

	// UpdateTestResult updates the last_tested_at and last_test_success fields.
	UpdateTestResult(ctx context.Context, projectID uuid.UUID, success bool) error

	// GetEffective returns the resolved config merging project config with server defaults.
	// Used by LLM client factory to get credentials for a project.
	GetEffective(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error)
}

// aiConfigService implements AIConfigService.
type aiConfigService struct {
	repo            repositories.AIConfigRepository
	communityConfig *config.CommunityAIConfig
	embeddedConfig  *config.EmbeddedAIConfig
	logger          *zap.Logger
}

// NewAIConfigService creates a new AI config service with dependencies.
func NewAIConfigService(
	repo repositories.AIConfigRepository,
	communityConfig *config.CommunityAIConfig,
	embeddedConfig *config.EmbeddedAIConfig,
	logger *zap.Logger,
) AIConfigService {
	return &aiConfigService{
		repo:            repo,
		communityConfig: communityConfig,
		embeddedConfig:  embeddedConfig,
		logger:          logger,
	}
}

// Get retrieves the AI config for a project.
func (s *aiConfigService) Get(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
	return s.repo.Get(ctx, projectID)
}

// Upsert creates or updates the AI config for a project.
func (s *aiConfigService) Upsert(ctx context.Context, projectID uuid.UUID, cfg *models.AIConfig) error {
	// Validate config_type
	switch cfg.ConfigType {
	case models.AIConfigNone:
		// Valid - no config
	case models.AIConfigBYOK:
		if cfg.LLMBaseURL == "" {
			return fmt.Errorf("BYOK requires llm_base_url")
		}
		if cfg.LLMModel == "" {
			return fmt.Errorf("BYOK requires llm_model")
		}
	case models.AIConfigCommunity:
		if s.communityConfig == nil || !s.communityConfig.IsAvailable() {
			return fmt.Errorf("community AI not configured on server")
		}
	case models.AIConfigEmbedded:
		if s.embeddedConfig == nil || !s.embeddedConfig.IsAvailable() {
			return fmt.Errorf("embedded AI not configured on server")
		}
	default:
		return fmt.Errorf("invalid config_type: %s", cfg.ConfigType)
	}

	if err := s.repo.Upsert(ctx, projectID, cfg); err != nil {
		return err
	}

	s.logger.Info("AI config saved",
		zap.String("project_id", projectID.String()),
		zap.String("config_type", string(cfg.ConfigType)),
	)

	return nil
}

// Delete removes the AI config for a project.
func (s *aiConfigService) Delete(ctx context.Context, projectID uuid.UUID) error {
	if err := s.repo.Delete(ctx, projectID); err != nil {
		return err
	}

	s.logger.Info("AI config deleted",
		zap.String("project_id", projectID.String()),
	)

	return nil
}

// UpdateTestResult updates the test metadata fields.
func (s *aiConfigService) UpdateTestResult(ctx context.Context, projectID uuid.UUID, success bool) error {
	return s.repo.UpdateTestResult(ctx, projectID, success)
}

// GetEffective returns the resolved config merging project config with server defaults.
func (s *aiConfigService) GetEffective(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
	projectConfig, err := s.repo.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if projectConfig == nil || projectConfig.ConfigType == models.AIConfigNone {
		return nil, fmt.Errorf("AI not configured for project")
	}

	switch projectConfig.ConfigType {
	case models.AIConfigCommunity:
		if s.communityConfig == nil || !s.communityConfig.IsAvailable() {
			return nil, fmt.Errorf("community AI not configured on server")
		}
		return &models.AIConfig{
			ConfigType:       models.AIConfigCommunity,
			LLMBaseURL:       s.communityConfig.LLMBaseURL,
			LLMModel:         s.communityConfig.LLMModel,
			EmbeddingBaseURL: s.communityConfig.EmbeddingURL,
			EmbeddingModel:   s.communityConfig.EmbeddingModel,
		}, nil

	case models.AIConfigEmbedded:
		if s.embeddedConfig == nil || !s.embeddedConfig.IsAvailable() {
			return nil, fmt.Errorf("embedded AI not configured on server")
		}
		return &models.AIConfig{
			ConfigType:       models.AIConfigEmbedded,
			LLMBaseURL:       s.embeddedConfig.LLMBaseURL,
			LLMModel:         s.embeddedConfig.LLMModel,
			EmbeddingBaseURL: s.embeddedConfig.EmbeddingURL,
			EmbeddingModel:   s.embeddedConfig.EmbeddingModel,
		}, nil

	case models.AIConfigBYOK:
		if !projectConfig.HasLLMConfig() {
			return nil, fmt.Errorf("BYOK config incomplete")
		}
		return projectConfig, nil

	default:
		return nil, fmt.Errorf("unknown config type: %s", projectConfig.ConfigType)
	}
}

// Ensure aiConfigService implements AIConfigService at compile time.
var _ AIConfigService = (*aiConfigService)(nil)

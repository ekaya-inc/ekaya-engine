package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AgentAPIKeyService manages agent API keys for MCP authentication.
type AgentAPIKeyService interface {
	// GenerateKey creates a new random API key for a project.
	GenerateKey(ctx context.Context, projectID uuid.UUID) (string, error)

	// GetKey retrieves the decrypted API key for a project.
	GetKey(ctx context.Context, projectID uuid.UUID) (string, error)

	// RegenerateKey invalidates the old key and generates a new one.
	RegenerateKey(ctx context.Context, projectID uuid.UUID) (string, error)

	// ValidateKey checks if the provided key matches the project's key.
	ValidateKey(ctx context.Context, projectID uuid.UUID, providedKey string) (bool, error)
}

type agentAPIKeyService struct {
	repo      repositories.MCPConfigRepository
	encryptor *crypto.CredentialEncryptor
	logger    *zap.Logger
}

// NewAgentAPIKeyService creates a new agent API key service.
func NewAgentAPIKeyService(
	repo repositories.MCPConfigRepository,
	logger *zap.Logger,
) (AgentAPIKeyService, error) {
	// Get encryption key from environment
	encKey := os.Getenv("PROJECT_CREDENTIALS_KEY")
	if encKey == "" {
		return nil, fmt.Errorf("PROJECT_CREDENTIALS_KEY not set")
	}

	encryptor, err := crypto.NewCredentialEncryptor(encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	return &agentAPIKeyService{
		repo:      repo,
		encryptor: encryptor,
		logger:    logger,
	}, nil
}

// GenerateKey creates a new random 32-byte API key (64 hex chars).
func (s *agentAPIKeyService) GenerateKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	// Generate 32 random bytes
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	// Encode as hex (64 characters)
	plainKey := hex.EncodeToString(keyBytes)

	// Encrypt and store
	encrypted, err := s.encryptor.Encrypt(plainKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt key: %w", err)
	}

	if err := s.repo.SetAgentAPIKey(ctx, projectID, encrypted); err != nil {
		return "", fmt.Errorf("failed to store key: %w", err)
	}

	s.logger.Info("Generated agent API key",
		zap.String("project_id", projectID.String()),
	)

	return plainKey, nil
}

// GetKey retrieves the decrypted API key for a project.
func (s *agentAPIKeyService) GetKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	encrypted, err := s.repo.GetAgentAPIKey(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get key: %w", err)
	}

	if encrypted == "" {
		return "", nil // No key exists
	}

	plainKey, err := s.encryptor.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt key: %w", err)
	}

	return plainKey, nil
}

// RegenerateKey invalidates the old key and generates a new one.
func (s *agentAPIKeyService) RegenerateKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	s.logger.Info("Regenerating agent API key",
		zap.String("project_id", projectID.String()),
	)

	return s.GenerateKey(ctx, projectID)
}

// ValidateKey checks if the provided key matches the project's key.
// Uses constant-time comparison to prevent timing attacks.
func (s *agentAPIKeyService) ValidateKey(ctx context.Context, projectID uuid.UUID, providedKey string) (bool, error) {
	storedKey, err := s.GetKey(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("failed to get stored key: %w", err)
	}

	if storedKey == "" {
		return false, nil // No key configured
	}

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(storedKey), []byte(providedKey)) == 1, nil
}

// Ensure agentAPIKeyService implements AgentAPIKeyService at compile time.
var _ AgentAPIKeyService = (*agentAPIKeyService)(nil)

package services

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AgentKeyValidator validates agent API keys for MCP authentication.
type AgentKeyValidator interface {
	ValidateKey(ctx context.Context, projectID uuid.UUID, key string) (*models.Agent, error)
	RecordAccess(ctx context.Context, agentID uuid.UUID) error
}

// AgentQueryAccessService exposes the agent query-access operations needed by MCP tools.
type AgentQueryAccessService interface {
	GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error)
	HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error)
}

// AgentService manages named AI agents and scoped query access.
type AgentService interface {
	AgentKeyValidator
	AgentQueryAccessService

	Create(ctx context.Context, projectID uuid.UUID, name string, queryIDs []uuid.UUID) (*models.Agent, string, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*AgentWithQueries, error)
	Get(ctx context.Context, projectID, agentID uuid.UUID) (*AgentWithQueries, error)
	EnsureExists(ctx context.Context, projectID, agentID uuid.UUID) error
	GetKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error)
	UpdateQueryAccess(ctx context.Context, projectID, agentID uuid.UUID, queryIDs []uuid.UUID) error
	RotateKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error)
	Delete(ctx context.Context, projectID, agentID uuid.UUID) error
	RecordAccess(ctx context.Context, agentID uuid.UUID) error
}

// AgentWithQueries represents an agent plus its allowed query IDs.
type AgentWithQueries struct {
	models.Agent
	QueryIDs []uuid.UUID `json:"query_ids"`
}

// AgentValidationError represents a request that violates agent business rules.
type AgentValidationError struct {
	Message string
}

func (e *AgentValidationError) Error() string {
	return e.Message
}

type agentService struct {
	repo      repositories.AgentRepository
	encryptor *crypto.CredentialEncryptor
	logger    *zap.Logger
}

// NewAgentService creates a new AgentService.
func NewAgentService(
	repo repositories.AgentRepository,
	encryptor *crypto.CredentialEncryptor,
	logger *zap.Logger,
) AgentService {
	return &agentService{
		repo:      repo,
		encryptor: encryptor,
		logger:    logger,
	}
}

func (s *agentService) Create(ctx context.Context, projectID uuid.UUID, name string, queryIDs []uuid.UUID) (*models.Agent, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", &AgentValidationError{Message: "name is required"}
	}
	if len(queryIDs) == 0 {
		return nil, "", &AgentValidationError{Message: "at least one query must be selected"}
	}

	plainKey, encryptedKey, err := s.generateEncryptedKey()
	if err != nil {
		return nil, "", err
	}

	agent := &models.Agent{
		ProjectID:       projectID,
		Name:            name,
		APIKeyEncrypted: encryptedKey,
	}

	if err := s.repo.Create(ctx, agent, uniqueQueryIDs(queryIDs)); err != nil {
		return nil, "", err
	}

	return agent, plainKey, nil
}

func (s *agentService) List(ctx context.Context, projectID uuid.UUID) ([]*AgentWithQueries, error) {
	agents, err := s.repo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	agentIDs := make([]uuid.UUID, 0, len(agents))
	for _, agent := range agents {
		agentIDs = append(agentIDs, agent.ID)
	}

	queryAccessByAgentID, err := s.repo.GetQueryAccessByAgentIDs(ctx, agentIDs)
	if err != nil {
		return nil, err
	}

	result := make([]*AgentWithQueries, 0, len(agents))
	for _, agent := range agents {
		result = append(result, &AgentWithQueries{
			Agent:    *agent,
			QueryIDs: queryAccessByAgentID[agent.ID],
		})
	}

	return result, nil
}

func (s *agentService) Get(ctx context.Context, projectID, agentID uuid.UUID) (*AgentWithQueries, error) {
	agent, err := s.repo.GetByID(ctx, projectID, agentID)
	if err != nil {
		return nil, err
	}

	queryIDs, err := s.repo.GetQueryAccess(ctx, agentID)
	if err != nil {
		return nil, err
	}

	return &AgentWithQueries{
		Agent:    *agent,
		QueryIDs: queryIDs,
	}, nil
}

func (s *agentService) EnsureExists(ctx context.Context, projectID, agentID uuid.UUID) error {
	_, err := s.repo.GetByID(ctx, projectID, agentID)
	return err
}

func (s *agentService) GetKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error) {
	agent, err := s.repo.GetByID(ctx, projectID, agentID)
	if err != nil {
		return "", err
	}

	plainKey, err := s.encryptor.Decrypt(agent.APIKeyEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt agent key: %w", err)
	}

	return plainKey, nil
}

func (s *agentService) UpdateQueryAccess(ctx context.Context, projectID, agentID uuid.UUID, queryIDs []uuid.UUID) error {
	if len(queryIDs) == 0 {
		return &AgentValidationError{Message: "at least one query must be selected"}
	}

	if _, err := s.repo.GetByID(ctx, projectID, agentID); err != nil {
		return err
	}

	return s.repo.SetQueryAccess(ctx, agentID, uniqueQueryIDs(queryIDs))
}

func (s *agentService) RotateKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error) {
	if _, err := s.repo.GetByID(ctx, projectID, agentID); err != nil {
		return "", err
	}

	plainKey, encryptedKey, err := s.generateEncryptedKey()
	if err != nil {
		return "", err
	}

	if err := s.repo.UpdateAPIKey(ctx, agentID, encryptedKey); err != nil {
		return "", err
	}

	return plainKey, nil
}

func (s *agentService) Delete(ctx context.Context, projectID, agentID uuid.UUID) error {
	return s.repo.Delete(ctx, projectID, agentID)
}

func (s *agentService) ValidateKey(ctx context.Context, projectID uuid.UUID, key string) (*models.Agent, error) {
	agents, err := s.repo.FindByAPIKey(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if len(agents) > 0 {
		for _, agent := range agents {
			plainKey, err := s.encryptor.Decrypt(agent.APIKeyEncrypted)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt agent key: %w", err)
			}
			if subtle.ConstantTimeCompare([]byte(plainKey), []byte(key)) == 1 {
				return agent, nil
			}
		}
		return nil, nil
	}

	return nil, nil
}

func (s *agentService) GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.GetQueryAccess(ctx, agentID)
}

func (s *agentService) HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error) {
	return s.repo.HasQueryAccess(ctx, agentID, queryID)
}

func (s *agentService) RecordAccess(ctx context.Context, agentID uuid.UUID) error {
	return s.repo.RecordAccess(ctx, agentID)
}

func (s *agentService) generateEncryptedKey() (string, string, error) {
	plainKey, err := generateAPIKey()
	if err != nil {
		return "", "", err
	}

	encryptedKey, err := s.encryptor.Encrypt(plainKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encrypt key: %w", err)
	}

	return plainKey, encryptedKey, nil
}

func uniqueQueryIDs(queryIDs []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(queryIDs))
	result := make([]uuid.UUID, 0, len(queryIDs))
	for _, queryID := range queryIDs {
		if _, exists := seen[queryID]; exists {
			continue
		}
		seen[queryID] = struct{}{}
		result = append(result, queryID)
	}
	return result
}

var _ AgentService = (*agentService)(nil)

package services

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

const agentServiceTestEncryptionKey = "0123456789abcdef0123456789abcdef"

type mockAgentRepository struct {
	agents           map[uuid.UUID]*models.Agent
	agentsByProj     map[uuid.UUID][]*models.Agent
	findByProj       map[uuid.UUID][]*models.Agent
	queryAccess      map[uuid.UUID][]uuid.UUID
	batchQueryAccess map[uuid.UUID][]uuid.UUID
	hasAccess        map[uuid.UUID]map[uuid.UUID]bool
	createErr        error
	getErr           error
	listErr          error
	updateAPIKeyErr  error
	deleteErr        error
	setAccessErr     error
	getAccessErr     error
	batchAccessErr   error
	hasAccessErr     error
	findErr          error
	getAccessCalls   int
	batchAccessCalls int
	findCalls        int
}

func newMockAgentRepository() *mockAgentRepository {
	return &mockAgentRepository{
		agents:           make(map[uuid.UUID]*models.Agent),
		agentsByProj:     make(map[uuid.UUID][]*models.Agent),
		findByProj:       make(map[uuid.UUID][]*models.Agent),
		queryAccess:      make(map[uuid.UUID][]uuid.UUID),
		batchQueryAccess: make(map[uuid.UUID][]uuid.UUID),
		hasAccess:        make(map[uuid.UUID]map[uuid.UUID]bool),
	}
}

func (m *mockAgentRepository) Create(ctx context.Context, agent *models.Agent, queryIDs []uuid.UUID) error {
	if m.createErr != nil {
		return m.createErr
	}

	cloned := *agent
	m.agents[agent.ID] = &cloned
	m.agentsByProj[agent.ProjectID] = append(m.agentsByProj[agent.ProjectID], &cloned)
	m.findByProj[agent.ProjectID] = append(m.findByProj[agent.ProjectID], &cloned)
	if len(queryIDs) > 0 {
		clonedQueryIDs := append([]uuid.UUID(nil), queryIDs...)
		m.queryAccess[agent.ID] = clonedQueryIDs
		m.batchQueryAccess[agent.ID] = clonedQueryIDs
		lookup := make(map[uuid.UUID]bool, len(queryIDs))
		for _, queryID := range queryIDs {
			lookup[queryID] = true
		}
		m.hasAccess[agent.ID] = lookup
	}
	return nil
}

func (m *mockAgentRepository) GetByID(ctx context.Context, projectID, agentID uuid.UUID) (*models.Agent, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}

	agent, ok := m.agents[agentID]
	if !ok || agent.ProjectID != projectID {
		return nil, errors.New("agent not found")
	}

	cloned := *agent
	return &cloned, nil
}

func (m *mockAgentRepository) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	return cloneAgentList(m.agentsByProj[projectID]), nil
}

func (m *mockAgentRepository) UpdateAPIKey(ctx context.Context, agentID uuid.UUID, encryptedKey string) error {
	if m.updateAPIKeyErr != nil {
		return m.updateAPIKeyErr
	}

	agent, ok := m.agents[agentID]
	if !ok {
		return errors.New("agent not found")
	}
	agent.APIKeyEncrypted = encryptedKey

	for _, agents := range m.agentsByProj {
		for _, listed := range agents {
			if listed.ID == agentID {
				listed.APIKeyEncrypted = encryptedKey
			}
		}
	}
	for _, agents := range m.findByProj {
		for _, listed := range agents {
			if listed.ID == agentID {
				listed.APIKeyEncrypted = encryptedKey
			}
		}
	}

	return nil
}

func (m *mockAgentRepository) Delete(ctx context.Context, projectID, agentID uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}

	delete(m.agents, agentID)
	delete(m.queryAccess, agentID)
	delete(m.hasAccess, agentID)

	agents := m.agentsByProj[projectID]
	filtered := make([]*models.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.ID != agentID {
			filtered = append(filtered, agent)
		}
	}
	m.agentsByProj[projectID] = filtered

	findAgents := m.findByProj[projectID]
	filteredFind := make([]*models.Agent, 0, len(findAgents))
	for _, agent := range findAgents {
		if agent.ID != agentID {
			filteredFind = append(filteredFind, agent)
		}
	}
	m.findByProj[projectID] = filteredFind

	return nil
}

func (m *mockAgentRepository) SetQueryAccess(ctx context.Context, agentID uuid.UUID, queryIDs []uuid.UUID) error {
	if m.setAccessErr != nil {
		return m.setAccessErr
	}

	cloned := append([]uuid.UUID(nil), queryIDs...)
	m.queryAccess[agentID] = cloned
	m.batchQueryAccess[agentID] = cloned
	lookup := make(map[uuid.UUID]bool, len(queryIDs))
	for _, queryID := range queryIDs {
		lookup[queryID] = true
	}
	m.hasAccess[agentID] = lookup
	return nil
}

func (m *mockAgentRepository) GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error) {
	if m.getAccessErr != nil {
		return nil, m.getAccessErr
	}

	m.getAccessCalls++
	return append([]uuid.UUID(nil), m.queryAccess[agentID]...), nil
}

func (m *mockAgentRepository) GetQueryAccessByAgentIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if m.batchAccessErr != nil {
		return nil, m.batchAccessErr
	}

	m.batchAccessCalls++

	result := make(map[uuid.UUID][]uuid.UUID, len(agentIDs))
	for _, agentID := range agentIDs {
		result[agentID] = append([]uuid.UUID(nil), m.batchQueryAccess[agentID]...)
	}
	return result, nil
}

func (m *mockAgentRepository) HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error) {
	if m.hasAccessErr != nil {
		return false, m.hasAccessErr
	}

	return m.hasAccess[agentID][queryID], nil
}

func (m *mockAgentRepository) FindByAPIKey(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}

	m.findCalls++
	return cloneAgentList(m.findByProj[projectID]), nil
}

func (m *mockAgentRepository) RecordAccess(_ context.Context, _ uuid.UUID) error {
	return nil
}

func setupAgentServiceTest(t *testing.T) (AgentService, *mockAgentRepository) {
	t.Helper()

	encryptor, err := crypto.NewCredentialEncryptor(agentServiceTestEncryptionKey)
	require.NoError(t, err)

	repo := newMockAgentRepository()

	svc := NewAgentService(repo, encryptor, zap.NewNop())
	return svc, repo
}

func TestAgentServiceCreateRequiresAtLeastOneQuery(t *testing.T) {
	svc, _ := setupAgentServiceTest(t)

	agent, key, err := svc.Create(context.Background(), uuid.New(), "sales-bot", nil)
	require.Error(t, err)
	assert.Nil(t, agent)
	assert.Empty(t, key)
	var validationErr *AgentValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "at least one query must be selected", validationErr.Error())
}

func TestAgentServiceCreateRejectsIneligibleQueries(t *testing.T) {
	svc, repo := setupAgentServiceTest(t)
	repo.createErr = apperrors.ErrNotFound

	agent, key, err := svc.Create(context.Background(), uuid.New(), "sales-bot", []uuid.UUID{uuid.New()})
	require.Error(t, err)
	assert.Nil(t, agent)
	assert.Empty(t, key)

	var validationErr *AgentValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, agentIneligibleQuerySelectionMessage, validationErr.Error())
}

func TestAgentServiceCreateAndValidateKey(t *testing.T) {
	svc, repo := setupAgentServiceTest(t)
	projectID := uuid.New()
	queryIDs := []uuid.UUID{uuid.New(), uuid.New()}

	agent, key, err := svc.Create(context.Background(), projectID, "sales-bot", queryIDs)
	require.NoError(t, err)
	require.NotNil(t, agent)
	assertGeneratedAgentKeyFormat(t, key)
	assert.Equal(t, "sales-bot", agent.Name)
	assert.NotEqual(t, key, agent.APIKeyEncrypted)

	storedQueries, err := repo.GetQueryAccess(context.Background(), agent.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, queryIDs, storedQueries)

	validated, err := svc.ValidateKey(context.Background(), projectID, key)
	require.NoError(t, err)
	require.NotNil(t, validated)
	assert.Equal(t, agent.ID, validated.ID)
	assert.Equal(t, "sales-bot", validated.Name)
}

func TestAgentServiceValidateKey_NoLegacyFallback(t *testing.T) {
	svc, _ := setupAgentServiceTest(t)

	agent, err := svc.ValidateKey(context.Background(), uuid.New(), "legacy-project-key")
	require.NoError(t, err)
	assert.Nil(t, agent)
}

func TestAgentServiceValidateKeyUsesFindByAPIKeyResults(t *testing.T) {
	svc, repo := setupAgentServiceTest(t)
	projectID := uuid.New()

	plainKey := agentAPIKeyPrefix + strings.Repeat("a", agentAPIKeyEntropyBytes*2)
	encryptedKey, err := svc.(*agentService).encryptor.Encrypt(plainKey)
	require.NoError(t, err)

	agent := &models.Agent{
		ID:              uuid.New(),
		ProjectID:       projectID,
		Name:            "auth-bot",
		APIKeyEncrypted: encryptedKey,
	}
	repo.findByProj[projectID] = []*models.Agent{agent}
	repo.listErr = errors.New("list should not be used for key validation")

	validated, err := svc.ValidateKey(context.Background(), projectID, plainKey)
	require.NoError(t, err)
	require.NotNil(t, validated)
	assert.Equal(t, agent.ID, validated.ID)
	assert.Equal(t, 1, repo.findCalls)
}

func TestAgentServiceGetAndRotateKey(t *testing.T) {
	svc, _ := setupAgentServiceTest(t)
	projectID := uuid.New()
	queryIDs := []uuid.UUID{uuid.New()}

	agent, originalKey, err := svc.Create(context.Background(), projectID, "finance-bot", queryIDs)
	require.NoError(t, err)

	revealedKey, err := svc.GetKey(context.Background(), projectID, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, originalKey, revealedKey)

	rotatedKey, err := svc.RotateKey(context.Background(), projectID, agent.ID)
	require.NoError(t, err)
	assertGeneratedAgentKeyFormat(t, rotatedKey)
	assert.NotEqual(t, originalKey, rotatedKey)

	validated, err := svc.ValidateKey(context.Background(), projectID, rotatedKey)
	require.NoError(t, err)
	require.NotNil(t, validated)
	assert.Equal(t, agent.ID, validated.ID)
}

func TestAgentServiceUpdateQueryAccessRequiresAtLeastOneQuery(t *testing.T) {
	svc, _ := setupAgentServiceTest(t)

	err := svc.UpdateQueryAccess(context.Background(), uuid.New(), uuid.New(), nil)
	require.Error(t, err)
	var validationErr *AgentValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "at least one query must be selected", validationErr.Error())
}

func TestAgentServiceUpdateQueryAccessRejectsIneligibleQueries(t *testing.T) {
	svc, repo := setupAgentServiceTest(t)
	projectID := uuid.New()
	agentID := uuid.New()

	repo.agents[agentID] = &models.Agent{ID: agentID, ProjectID: projectID, Name: "sales-bot"}
	repo.setAccessErr = apperrors.ErrNotFound

	err := svc.UpdateQueryAccess(context.Background(), projectID, agentID, []uuid.UUID{uuid.New()})
	require.Error(t, err)

	var validationErr *AgentValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, agentIneligibleQuerySelectionMessage, validationErr.Error())
}

func TestAgentServiceCreateRequiresName(t *testing.T) {
	svc, _ := setupAgentServiceTest(t)

	agent, key, err := svc.Create(context.Background(), uuid.New(), "   ", []uuid.UUID{uuid.New()})
	require.Error(t, err)
	assert.Nil(t, agent)
	assert.Empty(t, key)

	var validationErr *AgentValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "name is required", validationErr.Error())
}

func TestAgentServiceListUsesBatchQueryAccess(t *testing.T) {
	svc, repo := setupAgentServiceTest(t)
	projectID := uuid.New()
	agentA := &models.Agent{ID: uuid.New(), ProjectID: projectID, Name: "a"}
	agentB := &models.Agent{ID: uuid.New(), ProjectID: projectID, Name: "b"}
	repo.agents[agentA.ID] = agentA
	repo.agents[agentB.ID] = agentB
	repo.agentsByProj[projectID] = []*models.Agent{agentA, agentB}

	queryA := uuid.New()
	queryB := uuid.New()
	repo.batchQueryAccess[agentA.ID] = []uuid.UUID{queryA}
	repo.batchQueryAccess[agentB.ID] = []uuid.UUID{queryB}

	agents, err := svc.List(context.Background(), projectID)
	require.NoError(t, err)
	require.Len(t, agents, 2)
	assert.Equal(t, 1, repo.batchAccessCalls)
	assert.Equal(t, 0, repo.getAccessCalls)
	assert.Equal(t, []uuid.UUID{queryA}, agents[0].QueryIDs)
	assert.Equal(t, []uuid.UUID{queryB}, agents[1].QueryIDs)
}

func assertGeneratedAgentKeyFormat(t *testing.T, key string) {
	t.Helper()

	require.True(t, strings.HasPrefix(key, agentAPIKeyPrefix))

	suffix := strings.TrimPrefix(key, agentAPIKeyPrefix)
	require.Len(t, suffix, agentAPIKeyEntropyBytes*2)

	_, err := hex.DecodeString(suffix)
	require.NoError(t, err)
}

func cloneAgentList(agents []*models.Agent) []*models.Agent {
	result := make([]*models.Agent, 0, len(agents))
	for _, agent := range agents {
		cloned := *agent
		result = append(result, &cloned)
	}
	return result
}

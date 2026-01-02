package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mock Implementations for Finalization Tests
// ============================================================================

type mockOntologyRepoForFinalization struct {
	activeOntology       *models.TieredOntology
	updatedDomainSummary *models.DomainSummary
	getActiveErr         error
	updateSummaryErr     error
}

func (m *mockOntologyRepoForFinalization) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveErr != nil {
		return nil, m.getActiveErr
	}
	return m.activeOntology, nil
}

func (m *mockOntologyRepoForFinalization) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
}

func (m *mockOntologyRepoForFinalization) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	if m.updateSummaryErr != nil {
		return m.updateSummaryErr
	}
	m.updatedDomainSummary = summary
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepoForFinalization) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

type mockEntityRepoForFinalization struct {
	entities        []*models.OntologyEntity
	getByProjectErr error
}

func (m *mockEntityRepoForFinalization) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockEntityRepoForFinalization) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

func (m *mockEntityRepoForFinalization) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByProjectErr != nil {
		return nil, m.getByProjectErr
	}
	return m.entities, nil
}

func (m *mockEntityRepoForFinalization) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockEntityRepoForFinalization) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (m *mockEntityRepoForFinalization) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) CreateOccurrence(ctx context.Context, occ *models.OntologyEntityOccurrence) error {
	return nil
}

func (m *mockEntityRepoForFinalization) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (m *mockEntityRepoForFinalization) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (m *mockEntityRepoForFinalization) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

type mockRelationshipRepoForFinalization struct {
	relationships   []*models.EntityRelationship
	getByProjectErr error
}

func (m *mockRelationshipRepoForFinalization) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.relationships, nil
}

func (m *mockRelationshipRepoForFinalization) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	if m.getByProjectErr != nil {
		return nil, m.getByProjectErr
	}
	return m.relationships, nil
}

func (m *mockRelationshipRepoForFinalization) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	return m.relationships, nil
}

func (m *mockRelationshipRepoForFinalization) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

type mockLLMClient struct {
	responseContent string
	generateErr     error
}

func (m *mockLLMClient) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	return &llm.GenerateResponseResult{
		Content:          m.responseContent,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}, nil
}

func (m *mockLLMClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (m *mockLLMClient) GetModel() string {
	return "test-model"
}

func (m *mockLLMClient) GetEndpoint() string {
	return "https://test.endpoint"
}

type mockLLMFactoryForFinalization struct {
	client    llm.LLMClient
	createErr error
}

func (m *mockLLMFactoryForFinalization) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForFinalization) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForFinalization) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

// ============================================================================
// Tests
// ============================================================================

func TestOntologyFinalization_AggregatesDomains(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()
	entityID2 := uuid.New()
	entityID3 := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer"},
		{ID: entityID2, ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales"},
		{ID: entityID3, ProjectID: projectID, OntologyID: ontologyID, Name: "Invoice", Domain: "sales"}, // Duplicate domain
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{relationships: []*models.EntityRelationship{}}

	llmClient := &mockLLMClient{
		responseContent: `{"description": "An e-commerce platform managing users and sales."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify domain summary was updated
	require.NotNil(t, ontologyRepo.updatedDomainSummary)

	// Verify domains are aggregated and unique
	domains := ontologyRepo.updatedDomainSummary.Domains
	assert.Len(t, domains, 2) // "customer" and "sales"
	assert.Contains(t, domains, "customer")
	assert.Contains(t, domains, "sales")
}

func TestOntologyFinalization_GeneratesDomainDescription(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()
	entityID2 := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Description: "Platform users", Domain: "customer"},
		{ID: entityID2, ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Description: "Customer orders", Domain: "sales"},
	}

	placesOrdersDesc := "places orders"
	relationships := []*models.EntityRelationship{
		{SourceEntityID: entityID1, TargetEntityID: entityID2, Description: &placesOrdersDesc},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{relationships: relationships}

	expectedDescription := "This is an e-commerce platform that tracks users and their orders."
	llmClient := &mockLLMClient{
		responseContent: `{"description": "This is an e-commerce platform that tracks users and their orders."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify domain summary was updated with LLM-generated description
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	assert.Equal(t, expectedDescription, ontologyRepo.updatedDomainSummary.Description)
}

func TestOntologyFinalization_SkipsIfNoEntities(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFinalization{}
	entityRepo := &mockEntityRepoForFinalization{entities: []*models.OntologyEntity{}}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	llmFactory := &mockLLMFactoryForFinalization{}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify no domain summary was updated (skipped)
	assert.Nil(t, ontologyRepo.updatedDomainSummary)
}

func TestOntologyFinalization_HandlesEmptyDomains(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()

	// Entity with empty domain
	entities := []*models.OntologyEntity{
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Description: "Platform users", Domain: ""},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}

	llmClient := &mockLLMClient{
		responseContent: `{"description": "A user management system."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify domain summary was updated
	require.NotNil(t, ontologyRepo.updatedDomainSummary)

	// Domains should be empty (no valid domains from entities)
	assert.Empty(t, ontologyRepo.updatedDomainSummary.Domains)
}

func TestOntologyFinalization_HandlesRelationshipDisplay(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()
	entityID2 := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "Host", Description: "Property hosts", Domain: "hospitality"},
		{ID: entityID2, ProjectID: projectID, OntologyID: ontologyID, Name: "Guest", Description: "Property guests", Domain: "hospitality"},
	}

	hostsDesc := "hosts"
	relationships := []*models.EntityRelationship{
		{SourceEntityID: entityID1, TargetEntityID: entityID2, Description: &hostsDesc},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{relationships: relationships}

	llmClient := &mockLLMClient{
		responseContent: `{"description": "A hospitality platform connecting hosts with guests."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	assert.Equal(t, "A hospitality platform connecting hosts with guests.", ontologyRepo.updatedDomainSummary.Description)
	assert.Contains(t, ontologyRepo.updatedDomainSummary.Domains, "hospitality")
}

func TestOntologyFinalization_LLMFailure(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Description: "Platform users", Domain: "customer"},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}

	llmClient := &mockLLMClient{
		generateErr: errors.New("LLM unavailable"),
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)

	// Verify error is propagated
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM unavailable")

	// Verify domain summary was NOT updated
	assert.Nil(t, ontologyRepo.updatedDomainSummary)
}

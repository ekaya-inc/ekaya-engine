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
// Mock Implementations for Glossary Service Tests
// ============================================================================

type mockGlossaryRepo struct {
	terms        map[uuid.UUID]*models.BusinessGlossaryTerm
	createErr    error
	updateErr    error
	deleteErr    error
	getByIDErr   error
	getByProjErr error
}

func newMockGlossaryRepo() *mockGlossaryRepo {
	return &mockGlossaryRepo{
		terms: make(map[uuid.UUID]*models.BusinessGlossaryTerm),
	}
}

func (m *mockGlossaryRepo) Create(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	if m.createErr != nil {
		return m.createErr
	}
	term.ID = uuid.New()
	m.terms[term.ID] = term
	return nil
}

func (m *mockGlossaryRepo) Update(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, exists := m.terms[term.ID]; !exists {
		return errors.New("term not found")
	}
	m.terms[term.ID] = term
	return nil
}

func (m *mockGlossaryRepo) Delete(ctx context.Context, termID uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, exists := m.terms[termID]; !exists {
		return errors.New("term not found")
	}
	delete(m.terms, termID)
	return nil
}

func (m *mockGlossaryRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	if m.getByProjErr != nil {
		return nil, m.getByProjErr
	}
	var result []*models.BusinessGlossaryTerm
	for _, term := range m.terms {
		if term.ProjectID == projectID {
			result = append(result, term)
		}
	}
	return result, nil
}

func (m *mockGlossaryRepo) GetByTerm(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	for _, term := range m.terms {
		if term.ProjectID == projectID && term.Term == termName {
			return term, nil
		}
	}
	return nil, nil
}

func (m *mockGlossaryRepo) GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	return m.terms[termID], nil
}

type mockOntologyRepoForGlossary struct {
	activeOntology *models.TieredOntology
	getActiveErr   error
}

func (m *mockOntologyRepoForGlossary) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveErr != nil {
		return nil, m.getActiveErr
	}
	return m.activeOntology, nil
}

func (m *mockOntologyRepoForGlossary) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
}

func (m *mockOntologyRepoForGlossary) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepoForGlossary) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyRepoForGlossary) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

type mockEntityRepoForGlossary struct {
	entities        []*models.OntologyEntity
	getByProjectErr error
}

func (m *mockEntityRepoForGlossary) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockEntityRepoForGlossary) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForGlossary) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

func (m *mockEntityRepoForGlossary) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByProjectErr != nil {
		return nil, m.getByProjectErr
	}
	return m.entities, nil
}

func (m *mockEntityRepoForGlossary) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForGlossary) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForGlossary) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockEntityRepoForGlossary) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (m *mockEntityRepoForGlossary) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForGlossary) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (m *mockEntityRepoForGlossary) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepoForGlossary) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForGlossary) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepoForGlossary) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (m *mockEntityRepoForGlossary) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockEntityRepoForGlossary) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

type mockLLMClientForGlossary struct {
	responseContent string
	generateErr     error
}

func (m *mockLLMClientForGlossary) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
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

func (m *mockLLMClientForGlossary) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForGlossary) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForGlossary) GetModel() string {
	return "test-model"
}

func (m *mockLLMClientForGlossary) GetEndpoint() string {
	return "https://test.endpoint"
}

type mockLLMFactoryForGlossary struct {
	client    llm.LLMClient
	createErr error
}

func (m *mockLLMFactoryForGlossary) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForGlossary) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForGlossary) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

// ============================================================================
// Tests - CRUD Operations
// ============================================================================

func TestGlossaryService_CreateTerm(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		Term:       "Revenue",
		Definition: "Total earned amount from completed transactions",
		SQLPattern: "SUM(earned_amount)",
		BaseTable:  "billing_transactions",
	}

	err := svc.CreateTerm(ctx, projectID, term)
	require.NoError(t, err)

	// Verify term was created
	assert.NotEqual(t, uuid.Nil, term.ID)
	assert.Equal(t, projectID, term.ProjectID)
	assert.Equal(t, "user", term.Source) // Default source
}

func TestGlossaryService_CreateTerm_MissingName(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		Definition: "Some definition",
	}

	err := svc.CreateTerm(ctx, projectID, term)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "term name is required")
}

func TestGlossaryService_CreateTerm_MissingDefinition(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		Term: "Revenue",
	}

	err := svc.CreateTerm(ctx, projectID, term)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "term definition is required")
}

func TestGlossaryService_UpdateTerm(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	// Create initial term
	term := &models.BusinessGlossaryTerm{
		Term:       "Revenue",
		Definition: "Original definition",
	}
	err := svc.CreateTerm(ctx, projectID, term)
	require.NoError(t, err)

	// Update the term
	term.Definition = "Updated definition"
	err = svc.UpdateTerm(ctx, term)
	require.NoError(t, err)

	// Verify update
	updated, err := svc.GetTerm(ctx, term.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated definition", updated.Definition)
}

func TestGlossaryService_UpdateTerm_NotFound(t *testing.T) {
	ctx := context.Background()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		ID:         uuid.New(),
		Term:       "Revenue",
		Definition: "Definition",
	}

	err := svc.UpdateTerm(ctx, term)
	require.Error(t, err)
}

func TestGlossaryService_DeleteTerm(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	// Create term
	term := &models.BusinessGlossaryTerm{
		Term:       "Revenue",
		Definition: "Definition",
	}
	err := svc.CreateTerm(ctx, projectID, term)
	require.NoError(t, err)

	// Delete term
	err = svc.DeleteTerm(ctx, term.ID)
	require.NoError(t, err)

	// Verify deleted
	deleted, err := svc.GetTerm(ctx, term.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestGlossaryService_GetTerms(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	// Create terms
	term1 := &models.BusinessGlossaryTerm{Term: "Revenue", Definition: "Revenue def"}
	term2 := &models.BusinessGlossaryTerm{Term: "GMV", Definition: "GMV def"}
	err := svc.CreateTerm(ctx, projectID, term1)
	require.NoError(t, err)
	err = svc.CreateTerm(ctx, projectID, term2)
	require.NoError(t, err)

	// Get terms
	terms, err := svc.GetTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, terms, 2)
}

// ============================================================================
// Tests - SuggestTerms
// ============================================================================

func TestGlossaryService_SuggestTerms(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			ProjectID:    projectID,
			OntologyID:   ontologyID,
			Name:         "Transaction",
			Description:  "Financial transactions",
			Domain:       "billing",
			PrimaryTable: "billing_transactions",
		},
		{
			ID:           uuid.New(),
			ProjectID:    projectID,
			OntologyID:   ontologyID,
			Name:         "User",
			Description:  "Platform users",
			Domain:       "customer",
			PrimaryTable: "users",
		},
	}

	llmResponse := `[
		{
			"term": "Revenue",
			"definition": "Total earned amount from completed transactions",
			"sql_pattern": "SUM(earned_amount) WHERE state = 'completed'",
			"base_table": "billing_transactions",
			"columns_used": ["earned_amount", "state"],
			"filters": [{"column": "state", "operator": "=", "values": ["completed"]}],
			"aggregation": "SUM"
		},
		{
			"term": "Active Users",
			"definition": "Users with recent activity",
			"sql_pattern": "COUNT(DISTINCT id) WHERE last_active_at > NOW() - INTERVAL '30 days'",
			"base_table": "users",
			"columns_used": ["id", "last_active_at"],
			"filters": [],
			"aggregation": "COUNT"
		}
	]`

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
			DomainSummary: &models.DomainSummary{
				Description: "E-commerce platform",
			},
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{responseContent: llmResponse}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	suggestions, err := svc.SuggestTerms(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, suggestions, 2)

	// Verify first suggestion
	assert.Equal(t, "Revenue", suggestions[0].Term)
	assert.Equal(t, "Total earned amount from completed transactions", suggestions[0].Definition)
	assert.Equal(t, "billing_transactions", suggestions[0].BaseTable)
	assert.Equal(t, "suggested", suggestions[0].Source)
	assert.Len(t, suggestions[0].Filters, 1)
	assert.Equal(t, "state", suggestions[0].Filters[0].Column)

	// Verify second suggestion
	assert.Equal(t, "Active Users", suggestions[1].Term)
	assert.Equal(t, "users", suggestions[1].BaseTable)
}

func TestGlossaryService_SuggestTerms_NoOntology(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{activeOntology: nil}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	_, err := svc.SuggestTerms(ctx, projectID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active ontology found")
}

func TestGlossaryService_SuggestTerms_NoEntities(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: []*models.OntologyEntity{}}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	suggestions, err := svc.SuggestTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

func TestGlossaryService_SuggestTerms_LLMError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			ProjectID:    projectID,
			OntologyID:   ontologyID,
			Name:         "Transaction",
			PrimaryTable: "transactions",
		},
	}

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{generateErr: errors.New("LLM unavailable")}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	_, err := svc.SuggestTerms(ctx, projectID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM unavailable")
}

func TestGlossaryService_SuggestTerms_WithConventions(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			ProjectID:    projectID,
			OntologyID:   ontologyID,
			Name:         "Transaction",
			PrimaryTable: "transactions",
		},
	}

	llmResponse := `[{"term": "Revenue", "definition": "Total revenue", "base_table": "transactions"}]`

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
			DomainSummary: &models.DomainSummary{
				Description: "E-commerce platform",
				Conventions: &models.ProjectConventions{
					SoftDelete: &models.SoftDeleteConvention{
						Enabled: true,
						Column:  "deleted_at",
						Filter:  "deleted_at IS NULL",
					},
					Currency: &models.CurrencyConvention{
						Format:    "cents",
						Transform: "divide_by_100",
					},
				},
			},
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{responseContent: llmResponse}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	suggestions, err := svc.SuggestTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, suggestions, 1)
}

func TestGlossaryService_SuggestTerms_WithColumnDetails(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			ProjectID:    projectID,
			OntologyID:   ontologyID,
			Name:         "Transaction",
			PrimaryTable: "transactions",
		},
	}

	llmResponse := `[{"term": "Revenue", "definition": "Total revenue", "base_table": "transactions"}]`

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
			ColumnDetails: map[string][]models.ColumnDetail{
				"transactions": {
					{Name: "amount", Role: "measure", Description: "Transaction amount in cents"},
					{Name: "user_id", Role: "dimension", FKRole: "payer"},
				},
			},
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{responseContent: llmResponse}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, llmFactory, logger)

	suggestions, err := svc.SuggestTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, suggestions, 1)
}

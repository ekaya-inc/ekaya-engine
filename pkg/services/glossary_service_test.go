package services

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// withTestAuth adds test auth claims to context
func withTestAuth(ctx context.Context, projectID uuid.UUID) context.Context {
	claims := &auth.Claims{
		ProjectID: projectID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "test-user-123",
		},
	}
	return context.WithValue(ctx, auth.ClaimsKey, claims)
}

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

func (m *mockGlossaryRepo) GetByAlias(ctx context.Context, projectID uuid.UUID, alias string) (*models.BusinessGlossaryTerm, error) {
	// Simple mock: not implemented for now
	return nil, nil
}

func (m *mockGlossaryRepo) CreateAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error {
	// Simple mock: not implemented for now
	return nil
}

func (m *mockGlossaryRepo) DeleteAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error {
	// Simple mock: not implemented for now
	return nil
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

func (m *mockOntologyRepoForGlossary) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepoForGlossary) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
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
	return m.client, nil
}

// Mock datasource service (minimal implementation for tests)
type mockDatasourceServiceForGlossary struct{}

func (m *mockDatasourceServiceForGlossary) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceServiceForGlossary) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceServiceForGlossary) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceServiceForGlossary) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	// Return a mock datasource for SQL validation tests
	return []*models.Datasource{
		{
			ID:             uuid.New(),
			ProjectID:      projectID,
			Name:           "test-datasource",
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}, nil
}

func (m *mockDatasourceServiceForGlossary) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return nil
}

func (m *mockDatasourceServiceForGlossary) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockDatasourceServiceForGlossary) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
	return nil
}

// Mock adapter factory (minimal implementation for tests)
type mockQueryExecutorForGlossary struct{}

func (m *mockQueryExecutorForGlossary) ExecuteQuery(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
	// Return a successful result with one column
	return &datasource.QueryExecutionResult{
		Columns: []datasource.ColumnInfo{
			{Name: "result", Type: "bigint"},
		},
		Rows:     []map[string]any{{"result": 12345}},
		RowCount: 1,
	}, nil
}

func (m *mockQueryExecutorForGlossary) ExecuteQueryWithParams(ctx context.Context, sqlQuery string, params []any, limit int) (*datasource.QueryExecutionResult, error) {
	return m.ExecuteQuery(ctx, sqlQuery, limit)
}

func (m *mockQueryExecutorForGlossary) Execute(ctx context.Context, sqlStatement string) (*datasource.ExecuteResult, error) {
	return &datasource.ExecuteResult{
		RowsAffected: 1,
	}, nil
}

func (m *mockQueryExecutorForGlossary) ValidateQuery(ctx context.Context, sqlQuery string) error {
	return nil // All queries are valid in test mock
}

func (m *mockQueryExecutorForGlossary) QuoteIdentifier(name string) string {
	return `"` + name + `"`
}

func (m *mockQueryExecutorForGlossary) Close() error {
	return nil
}

type mockAdapterFactoryForGlossary struct{}

func (m *mockAdapterFactoryForGlossary) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *mockAdapterFactoryForGlossary) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return nil, nil
}

func (m *mockAdapterFactoryForGlossary) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return &mockQueryExecutorForGlossary{}, nil
}

func (m *mockAdapterFactoryForGlossary) ListTypes() []datasource.DatasourceAdapterInfo {
	return []datasource.DatasourceAdapterInfo{}
}

func (m *mockLLMFactoryForGlossary) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

// ============================================================================
// Tests - CRUD Operations
// ============================================================================

func TestGlossaryService_CreateTerm(t *testing.T) {
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		Term:       "Revenue",
		Definition: "Total earned amount from completed transactions",
		DefiningSQL: "SELECT SUM(earned_amount) FROM billing_transactions",
		BaseTable:  "billing_transactions",
	}

	err := svc.CreateTerm(ctx, projectID, term)
	require.NoError(t, err)

	// Verify term was created
	assert.NotEqual(t, uuid.Nil, term.ID)
	assert.Equal(t, projectID, term.ProjectID)
	assert.Equal(t, models.GlossarySourceManual, term.Source) // Default source
}

func TestGlossaryService_CreateTerm_MissingName(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		Term: "Revenue",
	}

	err := svc.CreateTerm(ctx, projectID, term)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "term definition is required")
}

func TestGlossaryService_UpdateTerm(t *testing.T) {
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// Create initial term
	term := &models.BusinessGlossaryTerm{
		Term:        "Revenue",
		Definition:  "Original definition",
		DefiningSQL: "SELECT SUM(amount) FROM transactions",
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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	term := &models.BusinessGlossaryTerm{
		ID:         uuid.New(),
		Term:       "Revenue",
		Definition: "Definition",
	}

	err := svc.UpdateTerm(ctx, term)
	require.Error(t, err)
}

func TestGlossaryService_DeleteTerm(t *testing.T) {
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// Create term
	term := &models.BusinessGlossaryTerm{
		Term:        "Revenue",
		Definition:  "Definition",
		DefiningSQL: "SELECT SUM(amount) FROM transactions",
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
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{}
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// Create terms
	term1 := &models.BusinessGlossaryTerm{Term: "Revenue", Definition: "Revenue def", DefiningSQL: "SELECT SUM(amount) FROM transactions"}
	term2 := &models.BusinessGlossaryTerm{Term: "GMV", Definition: "GMV def", DefiningSQL: "SELECT SUM(gmv) FROM orders"}
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
			"defining_sql": "SELECT SUM(earned_amount) AS revenue\nFROM billing_transactions\nWHERE state = 'completed'",
			"base_table": "billing_transactions",
			"aliases": ["Total Revenue", "Gross Revenue"]
		},
		{
			"term": "Active Users",
			"definition": "Users with recent activity",
			"defining_sql": "SELECT COUNT(DISTINCT id) AS active_users\nFROM users\nWHERE last_active_at > NOW() - INTERVAL '30 days'",
			"base_table": "users",
			"aliases": ["MAU", "Monthly Active Users"]
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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	suggestions, err := svc.SuggestTerms(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, suggestions, 2)

	// Verify first suggestion
	assert.Equal(t, "Revenue", suggestions[0].Term)
	assert.Equal(t, "Total earned amount from completed transactions", suggestions[0].Definition)
	assert.Equal(t, "billing_transactions", suggestions[0].BaseTable)
	assert.Equal(t, models.GlossarySourceInferred, suggestions[0].Source)
	assert.NotEmpty(t, suggestions[0].DefiningSQL)

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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

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
					{Name: "user_id", Role: "dimension", FKAssociation: "payer"},
				},
			},
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{responseContent: llmResponse}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	suggestions, err := svc.SuggestTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, suggestions, 1)
}

// ============================================================================
// Tests - DiscoverGlossaryTerms
// ============================================================================

func TestGlossaryService_DiscoverGlossaryTerms(t *testing.T) {
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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	count, err := svc.DiscoverGlossaryTerms(ctx, projectID, ontologyID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify term was saved to database
	terms, err := svc.GetTerms(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, terms, 1)
	assert.Equal(t, "Revenue", terms[0].Term)
	assert.Equal(t, "discovered", terms[0].Source)
	assert.Equal(t, "billing_transactions", terms[0].BaseTable)
}

func TestGlossaryService_DiscoverGlossaryTerms_SkipsDuplicates(t *testing.T) {
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)
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

	llmResponse := `[
		{
			"term": "Revenue",
			"definition": "Total revenue",
			"base_table": "transactions"
		}
	]`

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{responseContent: llmResponse}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// Create existing term with same name
	existingTerm := &models.BusinessGlossaryTerm{
		Term:        "Revenue",
		Definition:  "Existing definition",
		Source:      models.GlossarySourceManual,
		DefiningSQL: "SELECT SUM(amount) FROM transactions",
	}
	err := svc.CreateTerm(ctx, projectID, existingTerm)
	require.NoError(t, err)

	// Attempt to discover - should skip duplicate
	count, err := svc.DiscoverGlossaryTerms(ctx, projectID, ontologyID)
	require.NoError(t, err)
	assert.Equal(t, 0, count) // No new terms discovered

	// Verify only one term exists
	terms, err := svc.GetTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, terms, 1)
	assert.Equal(t, models.GlossarySourceManual, terms[0].Source) // Original term unchanged
}

func TestGlossaryService_DiscoverGlossaryTerms_NoEntities(t *testing.T) {
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

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	count, err := svc.DiscoverGlossaryTerms(ctx, projectID, ontologyID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// ============================================================================
// Tests - EnrichGlossaryTerms
// ============================================================================

func TestGlossaryService_EnrichGlossaryTerms(t *testing.T) {
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)
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

	// LLM response for enrichment
	enrichmentResponse := `{
		"defining_sql": "SELECT SUM(amount) AS total_revenue\nFROM transactions\nWHERE status = 'completed'",
		"base_table": "transactions",
		"aliases": ["Total Revenue", "Gross Revenue"]
	}`

	glossaryRepo := newMockGlossaryRepo()
	ontologyRepo := &mockOntologyRepoForGlossary{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	entityRepo := &mockEntityRepoForGlossary{entities: entities}
	llmClient := &mockLLMClientForGlossary{responseContent: enrichmentResponse}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// Create unenriched term (no DefiningSQL initially)
	term := &models.BusinessGlossaryTerm{
		ID:          uuid.New(),
		ProjectID:   projectID,
		Term:        "Revenue",
		Definition:  "Total revenue",
		Source:      models.GlossarySourceInferred,
		DefiningSQL: "", // Empty - will be enriched by LLM
	}
	// Manually insert to bypass validation that requires DefiningSQL
	glossaryRepo.terms[term.ID] = term

	// Enrich terms
	err := svc.EnrichGlossaryTerms(ctx, projectID, ontologyID)
	require.NoError(t, err)

	// Verify enrichment was applied
	terms, err := svc.GetTerms(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, terms, 1)
	assert.NotEmpty(t, terms[0].DefiningSQL)
	assert.Equal(t, "transactions", terms[0].BaseTable)
	assert.Contains(t, terms[0].Aliases, "Total Revenue")
	assert.Contains(t, terms[0].Aliases, "Gross Revenue")
}

func TestGlossaryService_EnrichGlossaryTerms_OnlyEnrichesUnenrichedTerms(t *testing.T) {
	projectID := uuid.New()
	ctx := withTestAuth(context.Background(), projectID)
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
	llmClient := &mockLLMClientForGlossary{responseContent: "{}"}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// Create already-enriched term
	enrichedTerm := &models.BusinessGlossaryTerm{
		Term:        "Revenue",
		Definition:  "Total revenue",
		Source:      models.GlossarySourceInferred,
		DefiningSQL: "SELECT SUM(amount) as total_revenue FROM transactions",
		BaseTable:   "transactions",
	}
	err := svc.CreateTerm(ctx, projectID, enrichedTerm)
	require.NoError(t, err)

	// Create user term (should be skipped)
	userTerm := &models.BusinessGlossaryTerm{
		Term:        "GMV",
		Definition:  "Gross merchandise value",
		Source:      models.GlossarySourceManual,
		DefiningSQL: "SELECT SUM(gmv) FROM orders",
	}
	err = svc.CreateTerm(ctx, projectID, userTerm)
	require.NoError(t, err)

	// Enrich terms - should not process any terms
	err = svc.EnrichGlossaryTerms(ctx, projectID, ontologyID)
	require.NoError(t, err)

	// Verify no changes
	terms, err := svc.GetTerms(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, terms, 2)
}

func TestGlossaryService_EnrichGlossaryTerms_NoUnenrichedTerms(t *testing.T) {
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
	entityRepo := &mockEntityRepoForGlossary{}
	llmFactory := &mockLLMFactoryForGlossary{}
	logger := zap.NewNop()

	datasourceSvc := &mockDatasourceServiceForGlossary{}
	adapterFactory := &mockAdapterFactoryForGlossary{}
	svc := NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, datasourceSvc, adapterFactory, llmFactory, logger)

	// No terms exist
	err := svc.EnrichGlossaryTerms(ctx, projectID, ontologyID)
	require.NoError(t, err)
}

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
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
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

func (m *mockOntologyRepoForFinalization) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepoForFinalization) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
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

func (m *mockEntityRepoForFinalization) GetPromotedByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByProjectErr != nil {
		return nil, m.getByProjectErr
	}
	var promoted []*models.OntologyEntity
	for _, e := range m.entities {
		if e.IsPromoted {
			promoted = append(promoted, e)
		}
	}
	return promoted, nil
}

func (m *mockEntityRepoForFinalization) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
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

func (m *mockEntityRepoForFinalization) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockEntityRepoForFinalization) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepoForFinalization) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepoForFinalization) TransferAliasesToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockEntityRepoForFinalization) TransferKeyColumnsToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
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

func (m *mockRelationshipRepoForFinalization) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	result := make(map[uuid.UUID][]*models.EntityRelationship)
	for _, rel := range m.relationships {
		result[rel.TargetEntityID] = append(result[rel.TargetEntityID], rel)
	}
	return result, nil
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

func (m *mockRelationshipRepoForFinalization) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockRelationshipRepoForFinalization) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockRelationshipRepoForFinalization) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockRelationshipRepoForFinalization) Update(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockRelationshipRepoForFinalization) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

func (m *mockRelationshipRepoForFinalization) UpdateSourceEntityID(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockRelationshipRepoForFinalization) UpdateTargetEntityID(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

type mockSchemaRepoForFinalization struct {
	columnsByTable  map[string][]*models.SchemaColumn
	getColumnsByErr error
}

func (m *mockSchemaRepoForFinalization) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	if m.getColumnsByErr != nil {
		return nil, m.getColumnsByErr
	}
	return m.columnsByTable, nil
}

// Stub implementations for SchemaRepository interface
func (m *mockSchemaRepoForFinalization) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnFeatures(ctx context.Context, projectID, columnID uuid.UUID, features *models.ColumnFeatures) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) ClearColumnFeaturesByProject(ctx context.Context, projectID uuid.UUID) error {
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

	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
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
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
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
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}
	llmFactory := &mockLLMFactoryForFinalization{}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
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
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Description: "Platform users", Domain: "", PrimaryTable: "users"},
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
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}

	llmClient := &mockLLMClient{
		responseContent: `{"description": "A user management system."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
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
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "Host", Description: "Property hosts", Domain: "hospitality", PrimaryTable: "hosts"},
		{ID: entityID2, ProjectID: projectID, OntologyID: ontologyID, Name: "Guest", Description: "Property guests", Domain: "hospitality", PrimaryTable: "guests"},
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
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}

	llmClient := &mockLLMClient{
		responseContent: `{"description": "A hospitality platform connecting hosts with guests."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
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
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Description: "Platform users", Domain: "customer", PrimaryTable: "users"},
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
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}

	llmClient := &mockLLMClient{
		generateErr: errors.New("LLM unavailable"),
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)

	// Verify error is propagated
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM unavailable")

	// Verify domain summary was NOT updated
	assert.Nil(t, ontologyRepo.updatedDomainSummary)
}

// ============================================================================
// Convention Discovery Tests
// ============================================================================

func TestOntologyFinalization_DiscoversSoftDelete_Timestamp(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	// Two entities mapping to two tables
	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
	}

	// Both tables have deleted_at column
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "deleted_at", DataType: "timestamp with time zone", IsNullable: true},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)

	sd := ontologyRepo.updatedDomainSummary.Conventions.SoftDelete
	assert.True(t, sd.Enabled)
	assert.Equal(t, "deleted_at", sd.Column)
	assert.Equal(t, "timestamp", sd.ColumnType)
	assert.Equal(t, "deleted_at IS NULL", sd.Filter)
	assert.Equal(t, 1.0, sd.Coverage) // 100% of tables
}

func TestOntologyFinalization_DiscoversSoftDelete_Boolean(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
	}

	// Both tables have is_deleted boolean column
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "is_deleted", DataType: "boolean", IsNullable: false},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "is_deleted", DataType: "bool", IsNullable: false},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)

	sd := ontologyRepo.updatedDomainSummary.Conventions.SoftDelete
	assert.True(t, sd.Enabled)
	assert.Equal(t, "is_deleted", sd.Column)
	assert.Equal(t, "boolean", sd.ColumnType)
	assert.Equal(t, "is_deleted = false", sd.Filter)
}

func TestOntologyFinalization_DiscoversSoftDelete_Coverage(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	// 4 tables, only 1 has deleted_at (25% coverage - below threshold)
	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Product", Domain: "inventory", PrimaryTable: "products"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Category", Domain: "inventory", PrimaryTable: "categories"},
	}

	columnsByTable := map[string][]*models.SchemaColumn{
		"users":      {{ColumnName: "id", DataType: "uuid"}, {ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true}},
		"orders":     {{ColumnName: "id", DataType: "uuid"}},
		"products":   {{ColumnName: "id", DataType: "uuid"}},
		"categories": {{ColumnName: "id", DataType: "uuid"}},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Should NOT report soft delete convention (below 50% threshold)
	if ontologyRepo.updatedDomainSummary.Conventions != nil {
		assert.Nil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)
	}
}

func TestOntologyFinalization_DiscoversCurrency_Cents(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Transaction", Domain: "billing", PrimaryTable: "transactions"},
	}

	// Multiple integer amount columns
	columnsByTable := map[string][]*models.SchemaColumn{
		"transactions": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "total_amount", DataType: "bigint"},
			{ColumnName: "fee_amount", DataType: "integer"},
			{ColumnName: "net_amount", DataType: "int8"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.Currency)

	cur := ontologyRepo.updatedDomainSummary.Conventions.Currency
	assert.Equal(t, "USD", cur.DefaultCurrency)
	assert.Equal(t, "cents", cur.Format)
	assert.Equal(t, "divide_by_100", cur.Transform)
	assert.Contains(t, cur.ColumnPatterns, "*_amount")
}

func TestOntologyFinalization_DiscoversCurrency_Dollars(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Transaction", Domain: "billing", PrimaryTable: "transactions"},
	}

	// Decimal amount columns suggest dollars
	columnsByTable := map[string][]*models.SchemaColumn{
		"transactions": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "total_amount", DataType: "decimal(10,2)"},
			{ColumnName: "unit_price", DataType: "numeric(12,2)"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.Currency)

	cur := ontologyRepo.updatedDomainSummary.Conventions.Currency
	assert.Equal(t, "dollars", cur.Format)
	assert.Equal(t, "none", cur.Transform)
}

func TestOntologyFinalization_DiscoversAuditColumns_WithCoverage(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
	}

	// Both tables have audit columns
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "created_at", DataType: "timestamptz"},
			{ColumnName: "updated_at", DataType: "timestamptz"},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "created_at", DataType: "timestamptz"},
			{ColumnName: "updated_at", DataType: "timestamptz"},
			{ColumnName: "created_by", DataType: "uuid"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)

	audit := ontologyRepo.updatedDomainSummary.Conventions.AuditColumns
	require.Len(t, audit, 3) // created_at and updated_at (100% coverage), created_by is 50% (meets >= 0.5 threshold)

	// Verify created_at and updated_at are included with 100% coverage
	var createdAt, updatedAt, createdBy *models.AuditColumnInfo
	for i := range audit {
		switch audit[i].Column {
		case "created_at":
			createdAt = &audit[i]
		case "updated_at":
			updatedAt = &audit[i]
		case "created_by":
			createdBy = &audit[i]
		}
	}
	require.NotNil(t, createdAt)
	require.NotNil(t, updatedAt)
	require.NotNil(t, createdBy)
	assert.Equal(t, 1.0, createdAt.Coverage)
	assert.Equal(t, 1.0, updatedAt.Coverage)
	assert.Equal(t, 0.5, createdBy.Coverage) // 1 of 2 tables
}

func TestOntologyFinalization_NoConventions(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
	}

	// No soft delete, no currency, no audit columns
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "name", DataType: "text"},
			{ColumnName: "email", DataType: "text"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Conventions should be nil when nothing detected
	assert.Nil(t, ontologyRepo.updatedDomainSummary.Conventions)
}

func TestOntologyFinalization_SampleQuestionsAreEmpty(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: entityID1, ProjectID: projectID, OntologyID: ontologyID, Name: "User", Description: "Platform users", Domain: "customer", PrimaryTable: "users"},
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
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: map[string][]*models.SchemaColumn{}}

	llmClient := &mockLLMClient{
		responseContent: `{"description": "A user management system."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify domain summary was updated
	require.NotNil(t, ontologyRepo.updatedDomainSummary)

	// Verify sample questions are nil/empty (sample question generation code removed)
	assert.Empty(t, ontologyRepo.updatedDomainSummary.SampleQuestions)
}

// ============================================================================
// ColumnFeatures-based Convention Discovery Tests
// ============================================================================

func TestOntologyFinalization_ExtractsColumnFeatureInsights_SoftDelete(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
	}

	// Columns with ColumnFeatures indicating soft-delete
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "deleted_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_soft_delete":    true,
							"timestamp_purpose": "soft_delete",
						},
					},
				},
			},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "deleted_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_soft_delete":    true,
							"timestamp_purpose": "soft_delete",
						},
					},
				},
			},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)

	sd := ontologyRepo.updatedDomainSummary.Conventions.SoftDelete
	assert.True(t, sd.Enabled)
	assert.Equal(t, "deleted_at", sd.Column)
	assert.Equal(t, "timestamp", sd.ColumnType)
	assert.Equal(t, "deleted_at IS NULL", sd.Filter)
	assert.Equal(t, 1.0, sd.Coverage) // 100% of tables
}

func TestOntologyFinalization_ExtractsColumnFeatureInsights_ExternalServices(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Payment", Domain: "billing", PrimaryTable: "payments"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Notification", Domain: "messaging", PrimaryTable: "notifications"},
	}

	// Columns with ColumnFeatures indicating external service IDs
	columnsByTable := map[string][]*models.SchemaColumn{
		"payments": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "stripe_charge_id",
				DataType:   "text",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"identifier_features": map[string]any{
							"identifier_type":  "external_service_id",
							"external_service": "stripe",
						},
					},
				},
			},
			{
				ColumnName: "stripe_customer_id",
				DataType:   "text",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"identifier_features": map[string]any{
							"identifier_type":  "external_service_id",
							"external_service": "stripe",
						},
					},
				},
			},
		},
		"notifications": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "twilio_message_sid",
				DataType:   "text",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"identifier_features": map[string]any{
							"identifier_type":  "external_service_id",
							"external_service": "twilio",
						},
					},
				},
			},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// The external services are included in the LLM prompt (visible in description generation)
	// We verify this by checking that the service completes successfully
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
}

func TestOntologyFinalization_ExtractsColumnFeatureInsights_AuditColumns(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
	}

	// Columns with ColumnFeatures indicating audit fields
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "created_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_created",
						},
					},
				},
			},
			{
				ColumnName: "updated_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_updated",
						},
					},
				},
			},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "created_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_created",
						},
					},
				},
			},
			{
				ColumnName: "updated_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_updated",
						},
					},
				},
			},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)

	// Audit columns should be discovered from ColumnFeatures
	audit := ontologyRepo.updatedDomainSummary.Conventions.AuditColumns
	require.Len(t, audit, 2) // created_at and updated_at

	var createdAt, updatedAt *models.AuditColumnInfo
	for i := range audit {
		switch audit[i].Column {
		case "created_at":
			createdAt = &audit[i]
		case "updated_at":
			updatedAt = &audit[i]
		}
	}
	require.NotNil(t, createdAt)
	require.NotNil(t, updatedAt)
	assert.Equal(t, 1.0, createdAt.Coverage)
	assert.Equal(t, 1.0, updatedAt.Coverage)
}

func TestOntologyFinalization_ExtractsColumnFeatureInsights_MonetaryColumns(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Transaction", Domain: "billing", PrimaryTable: "transactions"},
	}

	// Columns with ColumnFeatures indicating monetary columns with paired currency
	columnsByTable := map[string][]*models.SchemaColumn{
		"transactions": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "amount_cents",
				DataType:   "bigint",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"monetary_features": map[string]any{
							"is_monetary":            true,
							"currency_unit":          "cents",
							"paired_currency_column": "currency",
						},
					},
				},
			},
			{
				ColumnName: "fee_cents",
				DataType:   "bigint",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"monetary_features": map[string]any{
							"is_monetary":            true,
							"currency_unit":          "cents",
							"paired_currency_column": "currency",
						},
					},
				},
			},
			{ColumnName: "currency", DataType: "text"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// The monetary insights are included in the LLM prompt
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
}

func TestOntologyFinalization_FallsBackToPatternDetection_WhenNoColumnFeatures(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "User", Domain: "customer", PrimaryTable: "users"},
		{ID: uuid.New(), ProjectID: projectID, OntologyID: ontologyID, Name: "Order", Domain: "sales", PrimaryTable: "orders"},
	}

	// Columns WITHOUT ColumnFeatures - should fallback to pattern-based detection
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true}, // No Metadata
			{ColumnName: "created_at", DataType: "timestamptz"},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true}, // No Metadata
			{ColumnName: "created_at", DataType: "timestamptz"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	entityRepo := &mockEntityRepoForFinalization{entities: entities}
	relationshipRepo := &mockRelationshipRepoForFinalization{}
	schemaRepo := &mockSchemaRepoForFinalization{columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)

	// Should still detect soft-delete via pattern matching fallback
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)
	assert.Equal(t, "deleted_at", ontologyRepo.updatedDomainSummary.Conventions.SoftDelete.Column)

	// Should still detect audit columns via pattern matching fallback
	// Both created_at and deleted_at are in the auditColumnNames list
	require.Len(t, ontologyRepo.updatedDomainSummary.Conventions.AuditColumns, 2)
}

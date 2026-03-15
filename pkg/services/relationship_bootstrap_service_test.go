package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

func TestRelationshipBootstrapService_BootstrapCreatesColumnFeaturesRelationships(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	paymentsTableID := uuid.New()
	usersTableID := uuid.New()
	paymentUserIDColID := uuid.New()
	userIDColID := uuid.New()
	sourceDistinct := int64(90)
	targetDistinct := int64(95)

	mockSchemaRepo := &mockSchemaRepoForBootstrap{
		tables: []*models.SchemaTable{
			{ID: paymentsTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "payments"},
			{ID: usersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "users"},
		},
		columns: []*models.SchemaColumn{
			{ID: paymentUserIDColID, ProjectID: projectID, SchemaTableID: paymentsTableID, ColumnName: "user_id", DataType: "uuid", DistinctCount: &sourceDistinct},
			{ID: userIDColID, ProjectID: projectID, SchemaTableID: usersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, DistinctCount: &targetDistinct},
		},
	}

	mockColumnMetadataRepo := &mockColumnMetadataRepoForBootstrap{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			paymentUserIDColID: {
				SchemaColumnID: paymentUserIDColID,
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						FKTargetTable:  "users",
						FKTargetColumn: "id",
						FKConfidence:   0.95,
					},
				},
			},
		},
	}

	mockDatasourceSvc := &mockDatasourceServiceForBootstrap{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForBootstrap{
		schemaDiscoverer: &mockSchemaDiscovererForBootstrap{
			joinResults: map[string]*datasource.JoinAnalysis{
				"public.payments.user_id->public.users.id": {
					JoinCount:          100,
					SourceMatched:      90,
					TargetMatched:      80,
					OrphanCount:        10,
					ReverseOrphanCount: 15,
				},
			},
		},
	}

	svc := NewRelationshipBootstrapService(
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		mockColumnMetadataRepo,
		logger,
	)

	result, err := svc.Bootstrap(context.Background(), projectID, datasourceID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ColumnFeatureRelationships)
	assert.Equal(t, 0, result.DeclaredFKRelationships)
	assert.Equal(t, 1, result.FKRelationships)
	require.Len(t, mockSchemaRepo.upsertedRelationshipsWithMetrics, 1)
	assert.Empty(t, mockSchemaRepo.upsertedRelationships)

	rel := mockSchemaRepo.upsertedRelationshipsWithMetrics[0]
	require.NotNil(t, rel.InferenceMethod)
	assert.Equal(t, models.InferenceMethodColumnFeatures, *rel.InferenceMethod)
	assert.Equal(t, paymentUserIDColID, rel.SourceColumnID)
	assert.Equal(t, userIDColID, rel.TargetColumnID)
	assert.Equal(t, models.CardinalityNTo1, rel.Cardinality)
}

func TestRelationshipBootstrapService_BootstrapRecomputesDeclaredFKCardinality(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	accountsTableID := uuid.New()
	accountIDColID := uuid.New()
	ordersAccountIDColID := uuid.New()
	fkMethod := models.InferenceMethodFK

	mockSchemaRepo := &mockSchemaRepoForBootstrap{
		tables: []*models.SchemaTable{
			{ID: ordersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "orders"},
			{ID: accountsTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "accounts"},
		},
		columns: []*models.SchemaColumn{
			{ID: ordersAccountIDColID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "account_id", DataType: "uuid", IsUnique: true},
			{ID: accountIDColID, ProjectID: projectID, SchemaTableID: accountsTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
		},
		relationships: []*models.SchemaRelationship{
			{
				ID:              uuid.New(),
				ProjectID:       projectID,
				SourceTableID:   ordersTableID,
				SourceColumnID:  ordersAccountIDColID,
				TargetTableID:   accountsTableID,
				TargetColumnID:  accountIDColID,
				Cardinality:     models.CardinalityUnknown,
				InferenceMethod: &fkMethod,
			},
		},
	}

	mockDatasourceSvc := &mockDatasourceServiceForBootstrap{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForBootstrap{
		schemaDiscoverer: &mockSchemaDiscovererForBootstrap{
			joinResults: map[string]*datasource.JoinAnalysis{
				"public.orders.account_id->public.accounts.id": {
					JoinCount:     75,
					SourceMatched: 75,
					TargetMatched: 75,
				},
			},
		},
	}

	svc := NewRelationshipBootstrapService(
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		nil,
		logger,
	)

	result, err := svc.Bootstrap(context.Background(), projectID, datasourceID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ColumnFeatureRelationships)
	assert.Equal(t, 1, result.DeclaredFKRelationships)
	assert.Equal(t, 1, result.FKRelationships)
	assert.Empty(t, mockSchemaRepo.upsertedRelationshipsWithMetrics)
	require.Len(t, mockSchemaRepo.upsertedRelationships, 1)
	assert.Equal(t, models.Cardinality1To1, mockSchemaRepo.upsertedRelationships[0].Cardinality)
	assert.True(t, mockSchemaRepo.upsertedRelationships[0].IsValidated)
}

func TestRelationshipBootstrapService_BootstrapContinuesWhenColumnMetadataReadFails(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	accountsTableID := uuid.New()
	accountIDColID := uuid.New()
	ordersAccountIDColID := uuid.New()
	fkMethod := models.InferenceMethodFK

	mockSchemaRepo := &mockSchemaRepoForBootstrap{
		tables: []*models.SchemaTable{
			{ID: ordersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "orders"},
			{ID: accountsTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "accounts"},
		},
		columns: []*models.SchemaColumn{
			{ID: ordersAccountIDColID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "account_id", DataType: "uuid", IsUnique: true},
			{ID: accountIDColID, ProjectID: projectID, SchemaTableID: accountsTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
		},
		relationships: []*models.SchemaRelationship{
			{
				ID:              uuid.New(),
				ProjectID:       projectID,
				SourceTableID:   ordersTableID,
				SourceColumnID:  ordersAccountIDColID,
				TargetTableID:   accountsTableID,
				TargetColumnID:  accountIDColID,
				Cardinality:     models.CardinalityUnknown,
				InferenceMethod: &fkMethod,
			},
		},
	}

	mockDatasourceSvc := &mockDatasourceServiceForBootstrap{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForBootstrap{
		schemaDiscoverer: &mockSchemaDiscovererForBootstrap{
			joinResults: map[string]*datasource.JoinAnalysis{
				"public.orders.account_id->public.accounts.id": {
					JoinCount:     75,
					SourceMatched: 75,
					TargetMatched: 75,
				},
			},
		},
	}

	mockColumnMetadataRepo := &mockColumnMetadataRepoForBootstrap{
		getBySchemaColumnIDsErr: fmt.Errorf("metadata temporarily unavailable"),
	}

	svc := NewRelationshipBootstrapService(
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		mockColumnMetadataRepo,
		logger,
	)

	result, err := svc.Bootstrap(context.Background(), projectID, datasourceID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ColumnFeatureRelationships)
	assert.Equal(t, 1, result.DeclaredFKRelationships)
	assert.Equal(t, 1, result.FKRelationships)
	require.Len(t, mockSchemaRepo.upsertedRelationships, 1)
}

func TestRelationshipBootstrapService_BootstrapSkipsAmbiguousUnqualifiedTargetTable(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	paymentsTableID := uuid.New()
	adminUsersTableID := uuid.New()
	authUsersTableID := uuid.New()
	paymentUserIDColID := uuid.New()
	adminUserIDColID := uuid.New()
	authUserIDColID := uuid.New()

	mockSchemaRepo := &mockSchemaRepoForBootstrap{
		tables: []*models.SchemaTable{
			{ID: paymentsTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "payments"},
			{ID: adminUsersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "admin", TableName: "users"},
			{ID: authUsersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "auth", TableName: "users"},
		},
		columns: []*models.SchemaColumn{
			{ID: paymentUserIDColID, ProjectID: projectID, SchemaTableID: paymentsTableID, ColumnName: "user_id", DataType: "uuid"},
			{ID: adminUserIDColID, ProjectID: projectID, SchemaTableID: adminUsersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			{ID: authUserIDColID, ProjectID: projectID, SchemaTableID: authUsersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
		},
	}

	mockColumnMetadataRepo := &mockColumnMetadataRepoForBootstrap{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			paymentUserIDColID: {
				SchemaColumnID: paymentUserIDColID,
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						FKTargetTable:  "users",
						FKTargetColumn: "id",
						FKConfidence:   0.95,
					},
				},
			},
		},
	}

	mockDatasourceSvc := &mockDatasourceServiceForBootstrap{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForBootstrap{
		schemaDiscoverer: &mockSchemaDiscovererForBootstrap{},
	}

	svc := NewRelationshipBootstrapService(
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		mockColumnMetadataRepo,
		logger,
	)

	result, err := svc.Bootstrap(context.Background(), projectID, datasourceID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ColumnFeatureRelationships)
	assert.Equal(t, 0, result.DeclaredFKRelationships)
	assert.Equal(t, 0, result.FKRelationships)
	assert.Empty(t, mockSchemaRepo.upsertedRelationshipsWithMetrics)
}

type mockSchemaRepoForBootstrap struct {
	repositories.SchemaRepository
	tables                           []*models.SchemaTable
	columns                          []*models.SchemaColumn
	relationships                    []*models.SchemaRelationship
	upsertedRelationships            []*models.SchemaRelationship
	upsertedRelationshipsWithMetrics []*models.SchemaRelationship
}

func (m *mockSchemaRepoForBootstrap) ListTablesByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockSchemaRepoForBootstrap) ListColumnsByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}

func (m *mockSchemaRepoForBootstrap) ListRelationshipsByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaRelationship, error) {
	return m.relationships, nil
}

func (m *mockSchemaRepoForBootstrap) UpsertRelationship(_ context.Context, rel *models.SchemaRelationship) error {
	cloned := *rel
	m.upsertedRelationships = append(m.upsertedRelationships, &cloned)
	return nil
}

func (m *mockSchemaRepoForBootstrap) UpsertRelationshipWithMetrics(_ context.Context, rel *models.SchemaRelationship, _ *models.DiscoveryMetrics) error {
	cloned := *rel
	m.upsertedRelationshipsWithMetrics = append(m.upsertedRelationshipsWithMetrics, &cloned)
	return nil
}

type mockDatasourceServiceForBootstrap struct {
	DatasourceService
	datasource *models.Datasource
}

func (m *mockDatasourceServiceForBootstrap) Get(_ context.Context, _, _ uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

type mockAdapterFactoryForBootstrap struct {
	datasource.DatasourceAdapterFactory
	schemaDiscoverer datasource.SchemaDiscoverer
}

func (m *mockAdapterFactoryForBootstrap) NewSchemaDiscoverer(_ context.Context, _ string, _ map[string]any, _, _ uuid.UUID, _ string) (datasource.SchemaDiscoverer, error) {
	return m.schemaDiscoverer, nil
}

type mockSchemaDiscovererForBootstrap struct {
	datasource.SchemaDiscoverer
	joinResults map[string]*datasource.JoinAnalysis
}

func (m *mockSchemaDiscovererForBootstrap) AnalyzeJoin(_ context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	key := fmt.Sprintf("%s.%s.%s->%s.%s.%s", sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn)
	if joinResult, ok := m.joinResults[key]; ok {
		return joinResult, nil
	}
	return nil, fmt.Errorf("missing join analysis for %s", key)
}

func (m *mockSchemaDiscovererForBootstrap) Close() error {
	return nil
}

type mockColumnMetadataRepoForBootstrap struct {
	metadataByColumnID      map[uuid.UUID]*models.ColumnMetadata
	getBySchemaColumnIDsErr error
}

func (m *mockColumnMetadataRepoForBootstrap) Upsert(_ context.Context, _ *models.ColumnMetadata) error {
	return nil
}

func (m *mockColumnMetadataRepoForBootstrap) UpsertFromExtraction(_ context.Context, _ *models.ColumnMetadata) error {
	return nil
}

func (m *mockColumnMetadataRepoForBootstrap) GetBySchemaColumnID(_ context.Context, id uuid.UUID) (*models.ColumnMetadata, error) {
	return m.metadataByColumnID[id], nil
}

func (m *mockColumnMetadataRepoForBootstrap) GetByProject(_ context.Context, _ uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepoForBootstrap) GetBySchemaColumnIDs(_ context.Context, ids []uuid.UUID) ([]*models.ColumnMetadata, error) {
	if m.getBySchemaColumnIDsErr != nil {
		return nil, m.getBySchemaColumnIDsErr
	}
	result := make([]*models.ColumnMetadata, 0, len(ids))
	for _, id := range ids {
		if meta, ok := m.metadataByColumnID[id]; ok {
			result = append(result, meta)
		}
	}
	return result, nil
}

func (m *mockColumnMetadataRepoForBootstrap) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockColumnMetadataRepoForBootstrap) DeleteBySchemaColumnID(_ context.Context, _ uuid.UUID) error {
	return nil
}

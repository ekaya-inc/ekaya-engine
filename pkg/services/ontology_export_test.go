package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

type mockOntologyExportProjectRepo struct {
	project *models.Project
}

func (m *mockOntologyExportProjectRepo) Get(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return m.project, nil
}

type mockOntologyExportDatasourceService struct {
	datasource *models.Datasource
}

func (m *mockOntologyExportDatasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

type mockOntologyExportSchemaRepo struct {
	tables        []*models.SchemaTable
	columns       []*models.SchemaColumn
	relationships []*models.SchemaRelationship
}

func (m *mockOntologyExportSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockOntologyExportSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}

func (m *mockOntologyExportSchemaRepo) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return m.relationships, nil
}

type mockOntologyExportTableMetadataRepo struct {
	items []*models.TableMetadata
}

func (m *mockOntologyExportTableMetadataRepo) List(ctx context.Context, projectID uuid.UUID) ([]*models.TableMetadata, error) {
	return m.items, nil
}

type mockOntologyExportColumnMetadataRepo struct {
	items []*models.ColumnMetadata
}

func (m *mockOntologyExportColumnMetadataRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return m.items, nil
}

type mockOntologyExportQuestionRepo struct {
	items []*models.OntologyQuestion
}

func (m *mockOntologyExportQuestionRepo) List(ctx context.Context, projectID uuid.UUID, filters repositories.QuestionListFilters) (*repositories.QuestionListResult, error) {
	start := filters.Offset
	if start > len(m.items) {
		start = len(m.items)
	}
	end := start + filters.Limit
	if filters.Limit <= 0 || end > len(m.items) {
		end = len(m.items)
	}

	return &repositories.QuestionListResult{
		Questions:      m.items[start:end],
		TotalCount:     len(m.items),
		CountsByStatus: map[models.QuestionStatus]int{},
	}, nil
}

type mockOntologyExportKnowledgeRepo struct {
	items []*models.KnowledgeFact
}

func (m *mockOntologyExportKnowledgeRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	return m.items, nil
}

type mockOntologyExportGlossaryRepo struct {
	items []*models.BusinessGlossaryTerm
}

func (m *mockOntologyExportGlossaryRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return m.items, nil
}

type mockOntologyExportQueryRepo struct {
	items []*models.Query
}

func (m *mockOntologyExportQueryRepo) ListByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return m.items, nil
}

type mockOntologyExportAgentRepo struct {
	agents        []*models.Agent
	queryAccessBy map[uuid.UUID][]uuid.UUID
}

func (m *mockOntologyExportAgentRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error) {
	return m.agents, nil
}

func (m *mockOntologyExportAgentRepo) GetQueryAccessByAgentIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return m.queryAccessBy, nil
}

func TestOntologyExportService_BuildBundle(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()
	ordersIDColumnID := uuid.New()
	ordersUserIDColumnID := uuid.New()
	ordersStatusColumnID := uuid.New()
	usersIDColumnID := uuid.New()
	approvedEnabledQueryID := uuid.New()
	approvedDisabledQueryID := uuid.New()
	agentID := uuid.New()

	tableType := models.TableTypeTransactional
	tableDescription := "Orders placed by shoppers"
	columnRole := models.ColumnRoleDimension
	columnDescription := "Business order status"
	source := models.ProvenanceManual

	project := &models.Project{
		ID:           projectID,
		Name:         "The Look Demo",
		IndustryType: models.IndustryMarketplace,
		Parameters: map[string]any{
			"ai_api_key":      "engine-secret",
			"auth_server_url": "https://auth.example.com",
		},
		DomainSummary: &models.DomainSummary{
			Description: "Marketplace analytics ontology",
		},
	}

	datasource := &models.Datasource{
		ID:             datasourceID,
		ProjectID:      projectID,
		Name:           "The Look",
		DatasourceType: "postgres",
		Provider:       "supabase",
		Config: map[string]any{
			"host":     "db.example.com",
			"port":     5432,
			"name":     "the_look",
			"ssl_mode": "require",
			"user":     "readonly",
			"password": "dbpass",
			"extra": map[string]any{
				"schema": "public",
				"token":  "nested-secret",
			},
		},
	}

	questions := []*models.OntologyQuestion{
		{
			ID:         uuid.New(),
			ProjectID:  projectID,
			Text:       "What does orders.status represent?",
			Priority:   1,
			IsRequired: true,
			Category:   models.QuestionCategoryEnumeration,
			Affects: &models.QuestionAffects{
				Tables:  []string{"orders"},
				Columns: []string{"orders.status"},
			},
			Status: models.QuestionStatusAnswered,
			Answer: "It is the business order state.",
		},
		{
			ID:        uuid.New(),
			ProjectID: projectID,
			Text:      "Deleted question",
			Status:    models.QuestionStatusDeleted,
		},
	}

	service := NewOntologyExportService(
		&mockOntologyExportProjectRepo{project: project},
		&mockOntologyExportDatasourceService{datasource: datasource},
		&mockOntologyExportSchemaRepo{
			tables: []*models.SchemaTable{
				{ID: ordersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "orders", IsSelected: true},
				{ID: usersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "users", IsSelected: true},
			},
			columns: []*models.SchemaColumn{
				{ID: ordersIDColumnID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, IsSelected: true, OrdinalPosition: 1},
				{ID: ordersUserIDColumnID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "user_id", DataType: "uuid", IsSelected: true, OrdinalPosition: 2},
				{ID: ordersStatusColumnID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "status", DataType: "text", IsSelected: true, OrdinalPosition: 3, EnumValues: []string{"pending", "complete"}},
				{ID: usersIDColumnID, ProjectID: projectID, SchemaTableID: usersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, IsSelected: true, OrdinalPosition: 1},
			},
			relationships: []*models.SchemaRelationship{
				{
					ID:               uuid.New(),
					ProjectID:        projectID,
					SourceTableID:    ordersTableID,
					SourceColumnID:   ordersUserIDColumnID,
					TargetTableID:    usersTableID,
					TargetColumnID:   usersIDColumnID,
					RelationshipType: models.RelationshipTypeFK,
					Cardinality:      models.CardinalityNTo1,
					Confidence:       1,
				},
			},
		},
		&mockOntologyExportTableMetadataRepo{
			items: []*models.TableMetadata{
				{
					ProjectID:     projectID,
					SchemaTableID: ordersTableID,
					TableType:     &tableType,
					Description:   &tableDescription,
					IsEphemeral:   false,
					Source:        source,
				},
			},
		},
		&mockOntologyExportColumnMetadataRepo{
			items: []*models.ColumnMetadata{
				{
					ProjectID:      projectID,
					SchemaColumnID: ordersStatusColumnID,
					Role:           &columnRole,
					Description:    &columnDescription,
					Features: models.ColumnMetadataFeatures{
						Synonyms: []string{"state"},
					},
					Source: source,
				},
			},
		},
		&mockOntologyExportQuestionRepo{items: questions},
		&mockOntologyExportKnowledgeRepo{
			items: []*models.KnowledgeFact{
				{ProjectID: projectID, FactType: models.FactTypeBusinessRule, Value: "Only completed orders count toward GMV", Context: "Derived from finance review", Source: source},
			},
		},
		&mockOntologyExportGlossaryRepo{
			items: []*models.BusinessGlossaryTerm{
				{
					ProjectID:     projectID,
					Term:          "GMV",
					Definition:    "Gross merchandise volume",
					DefiningSQL:   "SELECT SUM(total_amount) AS gmv FROM orders",
					BaseTable:     "orders",
					OutputColumns: []models.OutputColumn{{Name: "gmv", Type: "decimal", Description: "Gross merchandise volume"}},
					Aliases:       []string{"gross merchandise volume"},
					Source:        source,
				},
			},
		},
		&mockOntologyExportQueryRepo{
			items: []*models.Query{
				{
					ID:                    approvedEnabledQueryID,
					ProjectID:             projectID,
					DatasourceID:          datasourceID,
					NaturalLanguagePrompt: "Find orders",
					SQLQuery:              "SELECT * FROM orders",
					Dialect:               "PostgreSQL",
					IsEnabled:             true,
					Status:                "approved",
					Parameters:            []models.QueryParameter{{Name: "status", Type: "string", Description: "Status", Required: false}},
					OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid", Description: "Order ID"}},
				},
				{
					ID:                    approvedDisabledQueryID,
					ProjectID:             projectID,
					DatasourceID:          datasourceID,
					NaturalLanguagePrompt: "Find users",
					SQLQuery:              "SELECT * FROM users",
					Dialect:               "PostgreSQL",
					IsEnabled:             false,
					Status:                "approved",
				},
				{
					ID:                    uuid.New(),
					ProjectID:             projectID,
					DatasourceID:          datasourceID,
					NaturalLanguagePrompt: "Rejected query",
					SQLQuery:              "DELETE FROM users",
					Dialect:               "PostgreSQL",
					IsEnabled:             false,
					Status:                "rejected",
				},
			},
		},
		&mockOntologyExportAgentRepo{
			agents: []*models.Agent{
				{
					ID:              agentID,
					ProjectID:       projectID,
					Name:            "demo-agent",
					APIKeyEncrypted: "agent-secret",
				},
			},
			queryAccessBy: map[uuid.UUID][]uuid.UUID{
				agentID: {approvedEnabledQueryID},
			},
		},
		zap.NewNop(),
	)

	bundle, err := service.BuildBundle(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	require.Equal(t, models.OntologyExportFormat, bundle.Format)
	require.Equal(t, models.OntologyExportVersion, bundle.Version)
	require.Equal(t, []string{
		models.AppIDOntologyForge,
		models.AppIDAIDataLiaison,
		models.AppIDAIAgents,
	}, bundle.RequiredApps)

	require.Len(t, bundle.Datasources, 1)
	exportedDatasource := bundle.Datasources[0]
	require.Equal(t, "The Look", exportedDatasource.Name)
	require.Equal(t, "db.example.com", exportedDatasource.Config["host"])
	require.NotContains(t, exportedDatasource.Config, "user")
	require.NotContains(t, exportedDatasource.Config, "password")
	require.Equal(t, map[string]any{"schema": "public"}, exportedDatasource.Config["extra"])

	require.Len(t, exportedDatasource.SelectedSchema.Tables, 2)
	require.Equal(t, "orders", exportedDatasource.SelectedSchema.Tables[0].TableName)
	require.Len(t, exportedDatasource.SelectedSchema.Tables[0].Columns, 3)
	require.Len(t, exportedDatasource.SelectedSchema.Relationships, 1)
	require.Equal(t, "orders", exportedDatasource.SelectedSchema.Relationships[0].Source.Table.TableName)
	require.Equal(t, "user_id", exportedDatasource.SelectedSchema.Relationships[0].Source.ColumnName)
	require.Equal(t, "users", exportedDatasource.SelectedSchema.Relationships[0].Target.Table.TableName)

	require.Len(t, bundle.Ontology.TableMetadata, 1)
	require.Equal(t, "orders", bundle.Ontology.TableMetadata[0].Table.TableName)
	require.Len(t, bundle.Ontology.ColumnMetadata, 1)
	require.Equal(t, "status", bundle.Ontology.ColumnMetadata[0].Column.ColumnName)
	require.Len(t, bundle.Ontology.Questions, 1)
	require.Equal(t, models.QuestionStatusAnswered, bundle.Ontology.Questions[0].Status)
	require.Equal(t, "orders", bundle.Ontology.Questions[0].Affects.Tables[0].TableName)
	require.Len(t, bundle.Ontology.ProjectKnowledge, 1)
	require.Len(t, bundle.Ontology.GlossaryTerms, 1)

	require.Len(t, bundle.ApprovedQueries, 2)
	require.Equal(t, "query_001", bundle.ApprovedQueries[0].Key)
	require.Equal(t, "Find orders", bundle.ApprovedQueries[0].NaturalLanguagePrompt)
	require.Equal(t, "query_002", bundle.ApprovedQueries[1].Key)
	require.Equal(t, "Find users", bundle.ApprovedQueries[1].NaturalLanguagePrompt)

	require.Len(t, bundle.Agents, 1)
	require.Equal(t, "demo-agent", bundle.Agents[0].Name)
	require.Equal(t, []string{"query_001"}, bundle.Agents[0].QueryKeys)

	payload, err := service.MarshalBundle(bundle)
	require.NoError(t, err)
	payloadText := string(payload)
	require.Contains(t, payloadText, "\n  \"format\":")
	require.NotContains(t, payloadText, "engine-secret")
	require.NotContains(t, payloadText, "auth_server_url")
	require.NotContains(t, payloadText, "dbpass")
	require.NotContains(t, payloadText, "nested-secret")
	require.NotContains(t, payloadText, "agent-secret")
}

func TestOntologyExportService_BuildBundle_DeterministicOrdering(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()
	ordersIDColumnID := uuid.New()
	ordersUserIDColumnID := uuid.New()
	ordersStatusColumnID := uuid.New()
	usersIDColumnID := uuid.New()
	queryAID := uuid.New()
	queryZID := uuid.New()
	agentAID := uuid.New()
	agentZID := uuid.New()

	service := NewOntologyExportService(
		&mockOntologyExportProjectRepo{
			project: &models.Project{
				ID:   projectID,
				Name: "The Look Demo",
			},
		},
		&mockOntologyExportDatasourceService{
			datasource: &models.Datasource{
				ID:             datasourceID,
				ProjectID:      projectID,
				Name:           "The Look",
				DatasourceType: "postgres",
			},
		},
		&mockOntologyExportSchemaRepo{
			tables: []*models.SchemaTable{
				{ID: usersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "users", IsSelected: true},
				{ID: ordersTableID, ProjectID: projectID, DatasourceID: datasourceID, SchemaName: "public", TableName: "orders", IsSelected: true},
			},
			columns: []*models.SchemaColumn{
				{ID: ordersStatusColumnID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "status", DataType: "text", IsSelected: true, OrdinalPosition: 3},
				{ID: usersIDColumnID, ProjectID: projectID, SchemaTableID: usersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, IsSelected: true, OrdinalPosition: 1},
				{ID: ordersUserIDColumnID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "user_id", DataType: "uuid", IsSelected: true, OrdinalPosition: 2},
				{ID: ordersIDColumnID, ProjectID: projectID, SchemaTableID: ordersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, IsSelected: true, OrdinalPosition: 1},
			},
			relationships: nil,
		},
		&mockOntologyExportTableMetadataRepo{},
		&mockOntologyExportColumnMetadataRepo{},
		&mockOntologyExportQuestionRepo{},
		&mockOntologyExportKnowledgeRepo{},
		&mockOntologyExportGlossaryRepo{},
		&mockOntologyExportQueryRepo{
			items: []*models.Query{
				{
					ID:                    queryZID,
					ProjectID:             projectID,
					DatasourceID:          datasourceID,
					NaturalLanguagePrompt: "Z prompt",
					SQLQuery:              "SELECT * FROM users",
					Dialect:               "PostgreSQL",
					Status:                "approved",
				},
				{
					ID:                    queryAID,
					ProjectID:             projectID,
					DatasourceID:          datasourceID,
					NaturalLanguagePrompt: "A prompt",
					SQLQuery:              "SELECT * FROM orders",
					Dialect:               "PostgreSQL",
					Status:                "approved",
				},
			},
		},
		&mockOntologyExportAgentRepo{
			agents: []*models.Agent{
				{ID: agentZID, ProjectID: projectID, Name: "z-agent"},
				{ID: agentAID, ProjectID: projectID, Name: "a-agent"},
			},
			queryAccessBy: map[uuid.UUID][]uuid.UUID{
				agentZID: {queryZID, queryAID},
				agentAID: {queryAID},
			},
		},
		zap.NewNop(),
	)

	bundle, err := service.BuildBundle(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	require.Len(t, bundle.Datasources, 1)
	require.Equal(t, "orders", bundle.Datasources[0].SelectedSchema.Tables[0].TableName)
	require.Equal(t, "users", bundle.Datasources[0].SelectedSchema.Tables[1].TableName)
	require.Equal(t, []string{"id", "user_id", "status"}, []string{
		bundle.Datasources[0].SelectedSchema.Tables[0].Columns[0].ColumnName,
		bundle.Datasources[0].SelectedSchema.Tables[0].Columns[1].ColumnName,
		bundle.Datasources[0].SelectedSchema.Tables[0].Columns[2].ColumnName,
	})

	require.Equal(t, []string{"A prompt", "Z prompt"}, []string{
		bundle.ApprovedQueries[0].NaturalLanguagePrompt,
		bundle.ApprovedQueries[1].NaturalLanguagePrompt,
	})
	require.Equal(t, []string{"query_001", "query_002"}, []string{
		bundle.ApprovedQueries[0].Key,
		bundle.ApprovedQueries[1].Key,
	})

	require.Equal(t, []string{"a-agent", "z-agent"}, []string{
		bundle.Agents[0].Name,
		bundle.Agents[1].Name,
	})
	require.Equal(t, []string{"query_001", "query_002"}, bundle.Agents[1].QueryKeys)

	firstPayload, err := service.MarshalBundle(bundle)
	require.NoError(t, err)
	secondPayload, err := service.MarshalBundle(bundle)
	require.NoError(t, err)
	require.Equal(t, string(firstPayload), string(secondPayload))
}

func TestOntologyExportService_SuggestedFilename(t *testing.T) {
	service := &ontologyExportService{}

	require.Equal(t, "the-look-demo-export.json", service.SuggestedFilename(&models.OntologyExportBundle{
		Project: models.OntologyExportProject{Name: "The Look Demo"},
	}))
	require.Equal(t, "ontology-export.json", service.SuggestedFilename(nil))
	require.True(t, strings.HasSuffix(service.SuggestedFilename(&models.OntologyExportBundle{
		Project: models.OntologyExportProject{Name: "!!!"},
	}), ".json"))
}

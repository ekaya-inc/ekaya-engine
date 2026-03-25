package services

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

type mockOntologyImportProjectRepo struct {
	project *models.Project
}

func (m *mockOntologyImportProjectRepo) Get(_ context.Context, _ uuid.UUID) (*models.Project, error) {
	return m.project, nil
}

type mockOntologyImportDatasourceLookup struct {
	datasources map[uuid.UUID]*models.Datasource
}

func (m *mockOntologyImportDatasourceLookup) Get(_ context.Context, _ uuid.UUID, id uuid.UUID) (*models.Datasource, error) {
	ds, ok := m.datasources[id]
	if !ok {
		return nil, fmt.Errorf("datasource %s not found", id)
	}
	return ds, nil
}

type mockOntologyImportDAGRepo struct {
	latestByDatasource map[uuid.UUID]*models.OntologyDAG
}

func (m *mockOntologyImportDAGRepo) GetLatestByDatasource(_ context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	if m.latestByDatasource == nil {
		return nil, nil
	}
	return m.latestByDatasource[datasourceID], nil
}

type mockOntologyImportInstalledApps struct{}

func (m *mockOntologyImportInstalledApps) IsInstalled(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return true, nil
}

func TestGetOntologyStatus_CompletionStateScopedByDatasource(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	cleanupOntologyImportTestState(t, tc)
	defer cleanupOntologyImportTestState(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	otherDatasource := ensureAdditionalImportTestDatasource(t, tc, "Secondary Datasource")

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok)

	importedAt := mustParseTime(t, "2026-03-24T10:00:00Z")
	require.NoError(t, storeOntologyCompletionState(
		ctx,
		scope.Conn,
		tc.projectID,
		tc.dsID,
		models.OntologyCompletionProvenanceImported,
		importedAt,
	))

	dagSvc := NewOntologyDAGService(nil, tc.repo, nil, nil, nil, nil, nil, zap.NewNop())

	statusForImportedDatasource, err := dagSvc.GetOntologyStatus(ctx, tc.projectID, tc.dsID)
	require.NoError(t, err)
	require.True(t, statusForImportedDatasource.HasOntology)
	require.Equal(t, models.OntologyCompletionProvenanceImported, statusForImportedDatasource.CompletionProvenance)

	statusForOtherDatasource, err := dagSvc.GetOntologyStatus(ctx, tc.projectID, otherDatasource.ID)
	require.NoError(t, err)
	assert.False(t, statusForOtherDatasource.HasOntology)
	assert.Equal(t, models.OntologyCompletionProvenance(""), statusForOtherDatasource.CompletionProvenance)
	assert.Nil(t, statusForOtherDatasource.LastBuiltAt)
}

func TestOntologyImportService_PrepareImportPlan_IgnoresOtherDatasourceCompletionState(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	cleanupOntologyImportTestState(t, tc)
	defer cleanupOntologyImportTestState(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	sourceDatasource := &models.Datasource{
		ID:             tc.dsID,
		ProjectID:      tc.projectID,
		Name:           "Primary Datasource",
		DatasourceType: "postgres",
	}
	targetDatasource := ensureAdditionalImportTestDatasource(t, tc, "Import Target")
	usersTable := createImportTestTable(t, tc, ctx, targetDatasource.ID, "public", "users", false)
	createImportTestColumn(t, tc, ctx, usersTable.ID, "id", "uuid", 1, false)

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok)
	require.NoError(t, storeOntologyCompletionState(
		ctx,
		scope.Conn,
		tc.projectID,
		sourceDatasource.ID,
		models.OntologyCompletionProvenanceImported,
		mustParseTime(t, "2026-03-24T10:00:00Z"),
	))

	svc := newTestOntologyImportService(tc.projectID, tc.repo, sourceDatasource, targetDatasource)
	_, err := svc.prepareImportPlan(ctx, tc.projectID, targetDatasource.ID, mustMarshalOntologyImportBundle(t, targetDatasource, []models.OntologyExportTable{
		{
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.OntologyExportColumn{
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
			},
		},
	}))
	require.NoError(t, err)
}

func TestOntologyImportService_PrepareImportPlan_UsesAvailableDatasourceSchema(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	cleanupOntologyImportTestState(t, tc)
	defer cleanupOntologyImportTestState(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	datasource := &models.Datasource{
		ID:             tc.dsID,
		ProjectID:      tc.projectID,
		Name:           "Primary Datasource",
		DatasourceType: "postgres",
	}
	usersTable := createImportTestTable(t, tc, ctx, datasource.ID, "public", "users", false)
	usersIDColumn := createImportTestColumn(t, tc, ctx, usersTable.ID, "id", "uuid", 1, false)
	usersEmailColumn := createImportTestColumn(t, tc, ctx, usersTable.ID, "email", "text", 2, false)
	ordersTable := createImportTestTable(t, tc, ctx, datasource.ID, "public", "orders", true)
	createImportTestColumn(t, tc, ctx, ordersTable.ID, "id", "uuid", 1, true)

	svc := newTestOntologyImportService(tc.projectID, tc.repo, datasource)
	plan, err := svc.prepareImportPlan(ctx, tc.projectID, datasource.ID, mustMarshalOntologyImportBundle(t, datasource, []models.OntologyExportTable{
		{
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.OntologyExportColumn{
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
				{ColumnName: "email", DataType: "text", OrdinalPosition: 2},
			},
		},
	}))
	require.NoError(t, err)

	require.Len(t, plan.tableIDs, 1)
	require.Equal(t, usersTable.ID, plan.tableIDs[0])
	require.ElementsMatch(t, []uuid.UUID{usersIDColumn.ID, usersEmailColumn.ID}, plan.columnIDs)
	require.NotContains(t, plan.tableIDs, ordersTable.ID)
}

func TestOntologyImportService_ImportBundle_PreservesExistingAgents(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	cleanupOntologyImportTestState(t, tc)
	defer cleanupOntologyImportTestState(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	datasource := &models.Datasource{
		ID:             tc.dsID,
		ProjectID:      tc.projectID,
		Name:           "Primary Datasource",
		DatasourceType: "postgres",
	}
	usersTable := createImportTestTable(t, tc, ctx, datasource.ID, "public", "users", false)
	createImportTestColumn(t, tc, ctx, usersTable.ID, "id", "uuid", 1, false)

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok)

	agentID := uuid.New()
	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_agents (id, project_id, name, api_key_encrypted, created_at, updated_at, last_access_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), NULL)
	`, agentID, tc.projectID, "existing-agent", "encrypted")
	require.NoError(t, err)

	svc := newTestOntologyImportService(tc.projectID, tc.repo, datasource)
	_, err = svc.ImportBundle(ctx, tc.projectID, datasource.ID, mustMarshalOntologyImportBundle(t, datasource, []models.OntologyExportTable{
		{
			SchemaName: "public",
			TableName:  "users",
			Columns: []models.OntologyExportColumn{
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
			},
		},
	}))
	require.NoError(t, err)

	var count int
	err = scope.Conn.QueryRow(ctx, `SELECT COUNT(*) FROM engine_agents WHERE project_id = $1`, tc.projectID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOntologyImportService_ImportBundle_DefaultsLegacyRelationshipProvenanceToInferred(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	cleanupOntologyImportTestState(t, tc)
	defer cleanupOntologyImportTestState(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	datasource := &models.Datasource{
		ID:             tc.dsID,
		ProjectID:      tc.projectID,
		Name:           "Primary Datasource",
		DatasourceType: "postgres",
	}
	usersTable := createImportTestTable(t, tc, ctx, datasource.ID, "public", "users", false)
	createImportTestColumn(t, tc, ctx, usersTable.ID, "id", "uuid", 1, false)
	ordersTable := createImportTestTable(t, tc, ctx, datasource.ID, "public", "orders", false)
	createImportTestColumn(t, tc, ctx, ordersTable.ID, "id", "uuid", 1, false)
	createImportTestColumn(t, tc, ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	bundle := models.OntologyExportBundle{
		Format:  models.OntologyExportFormat,
		Version: models.OntologyExportVersion,
		Project: models.OntologyExportProject{
			Name: "Ontology Import Test Project",
		},
		Datasources: []models.OntologyExportDatasource{
			{
				Key:            ontologyImportDatasourceKey,
				Name:           datasource.Name,
				DatasourceType: datasource.DatasourceType,
				SelectedSchema: models.OntologyExportSelectedSchema{
					Tables: []models.OntologyExportTable{
						{
							SchemaName: "public",
							TableName:  "users",
							Columns: []models.OntologyExportColumn{
								{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
							},
						},
						{
							SchemaName: "public",
							TableName:  "orders",
							Columns: []models.OntologyExportColumn{
								{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
								{ColumnName: "user_id", DataType: "uuid", OrdinalPosition: 2},
							},
						},
					},
					Relationships: []models.OntologyExportRelationship{
						{
							Source: models.OntologyExportColumnRef{
								Table:      models.OntologyExportTableRef{SchemaName: "public", TableName: "orders"},
								ColumnName: "user_id",
							},
							Target: models.OntologyExportColumnRef{
								Table:      models.OntologyExportTableRef{SchemaName: "public", TableName: "users"},
								ColumnName: "id",
							},
							RelationshipType: models.RelationshipTypeInferred,
							Cardinality:      models.CardinalityNTo1,
							Confidence:       0.97,
							IsValidated:      true,
						},
					},
				},
			},
		},
		Security: models.OntologyExportSecurity{},
	}

	payload, err := json.Marshal(bundle)
	require.NoError(t, err)

	svc := newTestOntologyImportService(tc.projectID, tc.repo, datasource)
	_, err = svc.ImportBundle(ctx, tc.projectID, datasource.ID, payload)
	require.NoError(t, err)

	relationships, err := tc.repo.ListRelationshipsByDatasource(ctx, tc.projectID, datasource.ID)
	require.NoError(t, err)
	require.Len(t, relationships, 1)
	assert.Equal(t, models.ProvenanceInferred, relationships[0].Source)
	assert.Nil(t, relationships[0].LastEditSource)
}

func newTestOntologyImportService(projectID uuid.UUID, schemaRepo ontologyImportSchemaRepository, datasources ...*models.Datasource) *ontologyImportService {
	project := &models.Project{
		ID:   projectID,
		Name: "Ontology Import Test Project",
	}

	datasourceByID := make(map[uuid.UUID]*models.Datasource, len(datasources))
	for _, datasource := range datasources {
		datasourceByID[datasource.ID] = datasource
	}

	return &ontologyImportService{
		projectRepo:         &mockOntologyImportProjectRepo{project: project},
		datasourceService:   &mockOntologyImportDatasourceLookup{datasources: datasourceByID},
		schemaRepo:          schemaRepo,
		dagRepo:             &mockOntologyImportDAGRepo{},
		installedAppService: &mockOntologyImportInstalledApps{},
		logger:              zap.NewNop(),
	}
}

func mustMarshalOntologyImportBundle(t *testing.T, datasource *models.Datasource, tables []models.OntologyExportTable) []byte {
	t.Helper()

	bundle := models.OntologyExportBundle{
		Format:  models.OntologyExportFormat,
		Version: models.OntologyExportVersion,
		Project: models.OntologyExportProject{
			Name: "Ontology Import Test Project",
		},
		Datasources: []models.OntologyExportDatasource{
			{
				Key:            ontologyImportDatasourceKey,
				Name:           datasource.Name,
				DatasourceType: datasource.DatasourceType,
				SelectedSchema: models.OntologyExportSelectedSchema{
					Tables: tables,
				},
			},
		},
		Security: models.OntologyExportSecurity{},
	}

	payload, err := json.Marshal(bundle)
	require.NoError(t, err)
	return payload
}

func ensureAdditionalImportTestDatasource(t *testing.T, tc *schemaServiceTestContext, name string) *models.Datasource {
	t.Helper()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok)

	datasource := &models.Datasource{
		ID:             uuid.New(),
		ProjectID:      tc.projectID,
		Name:           name,
		DatasourceType: "postgres",
	}
	_, err := scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
	`, datasource.ID, datasource.ProjectID, datasource.Name, datasource.DatasourceType, "{}")
	require.NoError(t, err)

	return datasource
}

func createImportTestTable(
	t *testing.T,
	tc *schemaServiceTestContext,
	ctx context.Context,
	datasourceID uuid.UUID,
	schemaName, tableName string,
	isSelected bool,
) *models.SchemaTable {
	t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: datasourceID,
		SchemaName:   schemaName,
		TableName:    tableName,
		IsSelected:   isSelected,
	}
	require.NoError(t, tc.repo.UpsertTable(ctx, table))

	return table
}

func createImportTestColumn(
	t *testing.T,
	tc *schemaServiceTestContext,
	ctx context.Context,
	tableID uuid.UUID,
	columnName, dataType string,
	ordinal int,
	isSelected bool,
) *models.SchemaColumn {
	t.Helper()

	column := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   tableID,
		ColumnName:      columnName,
		DataType:        dataType,
		IsNullable:      true,
		IsPrimaryKey:    columnName == "id",
		IsSelected:      isSelected,
		OrdinalPosition: ordinal,
	}
	require.NoError(t, tc.repo.UpsertColumn(ctx, column))

	return column
}

func cleanupOntologyImportTestState(t *testing.T, tc *schemaServiceTestContext) {
	t.Helper()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok)

	statements := []struct {
		sql  string
		args []any
	}{
		{
			sql: `DELETE FROM engine_agent_queries WHERE agent_id IN (SELECT id FROM engine_agents WHERE project_id = $1)`,
			args: []any{
				tc.projectID,
			},
		},
		{
			sql:  `DELETE FROM engine_agents WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_queries WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql: `DELETE FROM engine_dag_nodes WHERE dag_id IN (SELECT id FROM engine_ontology_dag WHERE project_id = $1)`,
			args: []any{
				tc.projectID,
			},
		},
		{
			sql:  `DELETE FROM engine_ontology_dag WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_ontology_questions WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_project_knowledge WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_business_glossary WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_ontology_column_metadata WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_ontology_table_metadata WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_schema_relationships WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_schema_columns WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_schema_tables WHERE project_id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `UPDATE engine_projects SET parameters = NULL, domain_summary = NULL WHERE id = $1`,
			args: []any{tc.projectID},
		},
		{
			sql:  `DELETE FROM engine_datasources WHERE project_id = $1 AND id <> $2`,
			args: []any{tc.projectID, tc.dsID},
		},
	}

	for _, statement := range statements {
		_, err := scope.Conn.Exec(ctx, statement.sql, statement.args...)
		require.NoError(t, err)
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed.UTC()
}

package handlers

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockProjectService is a configurable mock for all handler tests.
type mockProjectService struct {
	project             *models.Project
	provisionResult     *services.ProvisionResult
	defaultDatasourceID uuid.UUID
	err                 error
}

func (m *mockProjectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.project != nil {
		return m.project, nil
	}
	return &models.Project{
		ID:   id,
		Name: "Test Project",
	}, nil
}

func (m *mockProjectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return m.GetByID(ctx, id)
}

func (m *mockProjectService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockProjectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*services.ProvisionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.provisionResult != nil {
		return m.provisionResult, nil
	}
	return &services.ProvisionResult{
		ProjectID: projectID,
		Name:      name,
		Created:   false,
	}, nil
}

func (m *mockProjectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*services.ProvisionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.provisionResult != nil {
		return m.provisionResult, nil
	}
	projectID, _ := uuid.Parse(claims.ProjectID)
	return &services.ProvisionResult{
		ProjectID: projectID,
		Name:      claims.Email,
		Created:   false,
	}, nil
}

func (m *mockProjectService) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if m.err != nil {
		return uuid.Nil, m.err
	}
	return m.defaultDatasourceID, nil
}

func (m *mockProjectService) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	if m.err != nil {
		return m.err
	}
	m.defaultDatasourceID = datasourceID
	return nil
}

func (m *mockProjectService) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	// No-op for tests
}

// mockAuthService is a mock AuthService for integration testing.
type mockAuthService struct {
	claims *auth.Claims
	token  string
}

func (m *mockAuthService) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	return m.claims, m.token, nil
}

func (m *mockAuthService) RequireProjectID(claims *auth.Claims) error {
	if claims.ProjectID == "" {
		return auth.ErrMissingProjectID
	}
	return nil
}

func (m *mockAuthService) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	if urlProjectID != "" && claims.ProjectID != urlProjectID {
		return auth.ErrProjectIDMismatch
	}
	return nil
}

// mockDatasourceService is a configurable mock for datasource handler tests.
type mockDatasourceService struct {
	datasource  *models.Datasource
	datasources []*models.Datasource
	err         error
}

func (m *mockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.datasource != nil {
		return m.datasource, nil
	}
	return &models.Datasource{
		ID:             uuid.New(),
		ProjectID:      projectID,
		Name:           name,
		DatasourceType: dsType,
		Config:         config,
	}, nil
}

func (m *mockDatasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.datasource != nil {
		return m.datasource, nil
	}
	return &models.Datasource{
		ID:             id,
		ProjectID:      projectID,
		Name:           "Test DB",
		DatasourceType: "postgres",
		Config:         map[string]any{"host": "localhost"},
	}, nil
}

func (m *mockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.datasource != nil {
		return m.datasource, nil
	}
	return &models.Datasource{
		ID:             uuid.New(),
		ProjectID:      projectID,
		Name:           name,
		DatasourceType: "postgres",
		Config:         map[string]any{"host": "localhost"},
	}, nil
}

func (m *mockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.datasources != nil {
		return m.datasources, nil
	}
	return []*models.Datasource{}, nil
}

func (m *mockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return m.err
}

func (m *mockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
	return m.err
}

// mockSchemaService is a configurable mock for schema handler tests.
type mockSchemaService struct {
	schema        *models.DatasourceSchema
	table         *models.DatasourceTable
	relationships []*models.SchemaRelationship
	relationship  *models.SchemaRelationship
	refreshResult *models.RefreshResult
	prompt        string
	err           error
}

func (m *mockSchemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RefreshResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.refreshResult != nil {
		return m.refreshResult, nil
	}
	return &models.RefreshResult{
		TablesUpserted:       5,
		ColumnsUpserted:      20,
		RelationshipsCreated: 3,
	}, nil
}

func (m *mockSchemaService) GetDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.schema != nil {
		return m.schema, nil
	}
	return &models.DatasourceSchema{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		Tables:       []*models.DatasourceTable{},
	}, nil
}

func (m *mockSchemaService) GetDatasourceTable(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.DatasourceTable, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.table != nil {
		return m.table, nil
	}
	return &models.DatasourceTable{
		ID:         uuid.New(),
		SchemaName: "public",
		TableName:  tableName,
		IsSelected: true,
		Columns:    []*models.DatasourceColumn{},
	}, nil
}

func (m *mockSchemaService) AddManualRelationship(ctx context.Context, projectID, datasourceID uuid.UUID, req *models.AddRelationshipRequest) (*models.SchemaRelationship, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.relationship != nil {
		return m.relationship, nil
	}
	return &models.SchemaRelationship{
		ID:               uuid.New(),
		ProjectID:        projectID,
		SourceTableID:    uuid.New(),
		SourceColumnID:   uuid.New(),
		TargetTableID:    uuid.New(),
		TargetColumnID:   uuid.New(),
		RelationshipType: "manual",
		Cardinality:      "N:1",
		Confidence:       1.0,
	}, nil
}

func (m *mockSchemaService) RemoveRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return m.err
}

func (m *mockSchemaService) GetRelationshipsForDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.relationships != nil {
		return m.relationships, nil
	}
	return []*models.SchemaRelationship{}, nil
}

func (m *mockSchemaService) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return m.err
}

func (m *mockSchemaService) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return m.err
}

func (m *mockSchemaService) SaveSelections(ctx context.Context, projectID, datasourceID uuid.UUID, tableSelections map[uuid.UUID]bool, columnSelections map[uuid.UUID][]uuid.UUID) error {
	return m.err
}

func (m *mockSchemaService) GetSelectedDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.schema != nil {
		return m.schema, nil
	}
	return &models.DatasourceSchema{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		Tables:       []*models.DatasourceTable{},
	}, nil
}

func (m *mockSchemaService) GetDatasourceSchemaForPrompt(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.prompt != "" {
		return m.prompt, nil
	}
	return "Schema prompt for datasource", nil
}

func (m *mockSchemaService) GetDatasourceSchemaWithEntities(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.prompt != "" {
		return m.prompt, nil
	}
	return "Schema with entities for datasource", nil
}

func (m *mockSchemaService) GetRelationshipsResponse(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RelationshipsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &models.RelationshipsResponse{
		Relationships: []*models.RelationshipDetail{},
		TotalCount:    0,
	}, nil
}

func (m *mockSchemaService) GetRelationshipCandidates(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.LegacyRelationshipCandidatesResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &models.LegacyRelationshipCandidatesResponse{
		Candidates: []models.LegacyRelationshipCandidate{},
		Summary:    models.CandidatesSummary{},
	}, nil
}

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
	project         *models.Project
	provisionResult *services.ProvisionResult
	err             error
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

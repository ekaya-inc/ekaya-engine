package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// noopTenantMiddleware is a passthrough tenant middleware for unit tests.
func noopTenantMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return next
}

// rbacTestCase defines a reusable RBAC test scenario.
type rbacTestCase struct {
	name           string
	method         string
	path           string
	roles          []string
	expectedStatus int
}

// setupRBACMux creates a mux with registered routes and auth middleware
// using the given claims. The claims' Roles field is set per test case.
func setupRBACMux(t *testing.T, projectID uuid.UUID, registerFn func(*http.ServeMux, *auth.Middleware, TenantMiddleware), roles []string) *http.ServeMux {
	t.Helper()

	claims := &auth.Claims{
		ProjectID: projectID.String(),
		Roles:     roles,
		Email:     "test@example.com",
	}
	claims.Subject = uuid.New().String()

	authService := &mockAuthService{claims: claims, token: "test-token"}
	authMiddleware := auth.NewMiddleware(authService, zap.NewNop())

	mux := http.NewServeMux()
	registerFn(mux, authMiddleware, noopTenantMiddleware)
	return mux
}

// runRBACTest runs a single RBAC test case against a registered handler.
func runRBACTest(t *testing.T, projectID uuid.UUID, registerFn func(*http.ServeMux, *auth.Middleware, TenantMiddleware), tc rbacTestCase) {
	t.Helper()

	mux := setupRBACMux(t, projectID, registerFn, tc.roles)

	req := httptest.NewRequest(tc.method, tc.path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, tc.expectedStatus, rec.Code, "role=%v method=%s path=%s", tc.roles, tc.method, tc.path)

	if tc.expectedStatus == http.StatusForbidden {
		var errResp map[string]string
		err := json.Unmarshal(rec.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Equal(t, "forbidden", errResp["error"])
	}
}

// =============================================================================
// Mock services needed for RBAC tests
// =============================================================================

// mockUserService implements services.UserService for RBAC tests.
type mockUserService struct{}

func (m *mockUserService) Add(ctx context.Context, projectID, userID uuid.UUID, role string) error {
	return nil
}
func (m *mockUserService) Remove(ctx context.Context, projectID, userID uuid.UUID) error {
	return nil
}
func (m *mockUserService) Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error {
	return nil
}
func (m *mockUserService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error) {
	return nil, nil
}

// mockMCPConfigServiceForRBAC implements services.MCPConfigService for RBAC tests.
type mockMCPConfigServiceForRBAC struct{}

func (m *mockMCPConfigServiceForRBAC) Get(ctx context.Context, projectID uuid.UUID) (*services.MCPConfigResponse, error) {
	return &services.MCPConfigResponse{}, nil
}
func (m *mockMCPConfigServiceForRBAC) Update(ctx context.Context, projectID uuid.UUID, req *services.UpdateMCPConfigRequest) (*services.MCPConfigResponse, error) {
	return &services.MCPConfigResponse{}, nil
}
func (m *mockMCPConfigServiceForRBAC) IsToolGroupEnabled(ctx context.Context, projectID uuid.UUID, toolGroup string) (bool, error) {
	return false, nil
}
func (m *mockMCPConfigServiceForRBAC) GetToolGroupConfig(ctx context.Context, projectID uuid.UUID, toolGroup string) (*models.ToolGroupConfig, error) {
	return nil, nil
}
func (m *mockMCPConfigServiceForRBAC) GetToolGroupsState(ctx context.Context, projectID uuid.UUID) (map[string]*models.ToolGroupConfig, error) {
	return nil, nil
}
func (m *mockMCPConfigServiceForRBAC) ShouldShowApprovedQueries(ctx context.Context, projectID uuid.UUID) (bool, error) {
	return false, nil
}
func (m *mockMCPConfigServiceForRBAC) ShouldShowApprovedQueriesTools(ctx context.Context, projectID uuid.UUID) (bool, error) {
	return false, nil
}

// mockInstalledAppService implements services.InstalledAppService for RBAC tests.
type mockInstalledAppService struct{}

func (m *mockInstalledAppService) ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error) {
	return nil, nil
}
func (m *mockInstalledAppService) IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error) {
	return false, nil
}
func (m *mockInstalledAppService) Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*services.AppActionResult, error) {
	return &services.AppActionResult{}, nil
}
func (m *mockInstalledAppService) Activate(ctx context.Context, projectID uuid.UUID, appID string) (*services.AppActionResult, error) {
	return &services.AppActionResult{}, nil
}
func (m *mockInstalledAppService) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) (*services.AppActionResult, error) {
	return &services.AppActionResult{}, nil
}
func (m *mockInstalledAppService) CompleteCallback(ctx context.Context, projectID uuid.UUID, appID, action, status, nonce, userID string) error {
	return nil
}
func (m *mockInstalledAppService) GetSettings(ctx context.Context, projectID uuid.UUID, appID string) (map[string]any, error) {
	return nil, nil
}
func (m *mockInstalledAppService) UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error {
	return nil
}
func (m *mockInstalledAppService) GetApp(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error) {
	return nil, nil
}

// =============================================================================
// Users Handler RBAC Tests
// =============================================================================

func TestRBAC_UsersHandler_AdminOnly(t *testing.T) {
	projectID := uuid.New()
	mockService := &mockUserService{}
	handler := NewUsersHandler(mockService, zap.NewNop())

	basePath := "/api/projects/" + projectID.String() + "/users"

	tests := []rbacTestCase{
		// Admin allowed (400 = past RBAC, no request body)
		{name: "POST_admin_allowed", method: http.MethodPost, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "DELETE_admin_allowed", method: http.MethodDelete, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "PUT_admin_allowed", method: http.MethodPut, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		// User denied
		{name: "POST_user_denied", method: http.MethodPost, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "DELETE_user_denied", method: http.MethodDelete, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "PUT_user_denied", method: http.MethodPut, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		// Data denied (user management is admin only)
		{name: "POST_data_denied", method: http.MethodPost, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "DELETE_data_denied", method: http.MethodDelete, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "PUT_data_denied", method: http.MethodPut, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

// =============================================================================
// Agent API Key Handler RBAC Tests
// =============================================================================

func TestRBAC_AgentAPIKeyHandler_AdminOnly(t *testing.T) {
	projectID := uuid.New()
	mockService := &mockAgentAPIKeyService{key: "test-key"}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	basePath := "/api/projects/" + projectID.String() + "/mcp/agent-key"

	tests := []rbacTestCase{
		// Admin allowed
		{name: "GET_admin_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "POST_regen_admin_allowed", method: http.MethodPost, path: basePath + "/regenerate", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		// User denied
		{name: "GET_user_denied", method: http.MethodGet, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "POST_regen_user_denied", method: http.MethodPost, path: basePath + "/regenerate", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		// Data denied
		{name: "GET_data_denied", method: http.MethodGet, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "POST_regen_data_denied", method: http.MethodPost, path: basePath + "/regenerate", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

// =============================================================================
// MCP Config Handler RBAC Tests
// =============================================================================

func TestRBAC_MCPConfigHandler_PatchAdminOnly(t *testing.T) {
	projectID := uuid.New()
	mockService := &mockMCPConfigServiceForRBAC{}
	handler := NewMCPConfigHandler(mockService, zap.NewNop())

	basePath := "/api/projects/" + projectID.String() + "/mcp/config"

	tests := []rbacTestCase{
		// GET - any authenticated user
		{name: "GET_user_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},
		{name: "GET_admin_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		// PATCH - admin only (400 = past RBAC, bad body)
		{name: "PATCH_admin_allowed", method: http.MethodPatch, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "PATCH_user_denied", method: http.MethodPatch, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "PATCH_data_denied", method: http.MethodPatch, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

// =============================================================================
// Installed App Handler RBAC Tests
// =============================================================================

func TestRBAC_InstalledAppHandler(t *testing.T) {
	projectID := uuid.New()
	mockService := &mockInstalledAppService{}
	handler := NewInstalledAppHandler(mockService, zap.NewNop())

	basePath := "/api/projects/" + projectID.String() + "/apps"
	appPath := basePath + "/test-app"

	tests := []rbacTestCase{
		// GET list/get - any authenticated user
		{name: "GET_list_user_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},
		{name: "GET_detail_user_allowed", method: http.MethodGet, path: appPath, roles: []string{models.RoleUser}, expectedStatus: http.StatusOK}, // mock returns nil app, handler writes it as JSON

		// POST install - admin only
		{name: "POST_install_admin_allowed", method: http.MethodPost, path: appPath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusCreated}, // mock returns empty result (no redirect)
		{name: "POST_install_user_denied", method: http.MethodPost, path: appPath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "POST_install_data_denied", method: http.MethodPost, path: appPath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},

		// POST activate - admin only
		{name: "POST_activate_admin_allowed", method: http.MethodPost, path: appPath + "/activate", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "POST_activate_user_denied", method: http.MethodPost, path: appPath + "/activate", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// DELETE uninstall - admin only
		{name: "DELETE_admin_allowed", method: http.MethodDelete, path: appPath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "DELETE_user_denied", method: http.MethodDelete, path: appPath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "DELETE_data_denied", method: http.MethodDelete, path: appPath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},

		// PATCH settings - admin+data (400 = past RBAC, bad body)
		{name: "PATCH_settings_admin_allowed", method: http.MethodPatch, path: appPath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "PATCH_settings_data_allowed", method: http.MethodPatch, path: appPath, roles: []string{models.RoleData}, expectedStatus: http.StatusBadRequest},
		{name: "PATCH_settings_user_denied", method: http.MethodPatch, path: appPath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// POST callback - admin only (400 = past RBAC, bad body)
		{name: "POST_callback_admin_allowed", method: http.MethodPost, path: appPath + "/callback", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_callback_user_denied", method: http.MethodPost, path: appPath + "/callback", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

// =============================================================================
// Projects Handler RBAC Tests
// =============================================================================

func TestRBAC_ProjectsHandler(t *testing.T) {
	projectID := uuid.New()
	mockService := &mockProjectService{
		project: &models.Project{ID: projectID, Name: "Test"},
	}
	handler := NewProjectsHandler(mockService, testConfig(), zap.NewNop())

	basePath := "/api/projects/" + projectID.String()

	tests := []rbacTestCase{
		// GET - any authenticated user
		{name: "GET_user_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},
		{name: "GET_admin_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},

		// DELETE - admin only
		{name: "DELETE_admin_allowed", method: http.MethodDelete, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusNoContent},
		{name: "DELETE_user_denied", method: http.MethodDelete, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
		{name: "DELETE_data_denied", method: http.MethodDelete, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},

		// PATCH auth-server-url - admin only (400 = past RBAC, bad body)
		{name: "PATCH_auth_url_admin_allowed", method: http.MethodPatch, path: basePath + "/auth-server-url", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "PATCH_auth_url_user_denied", method: http.MethodPatch, path: basePath + "/auth-server-url", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// POST sync-server-url - admin only
		{name: "POST_sync_url_admin_allowed", method: http.MethodPost, path: basePath + "/sync-server-url", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest}, // past RBAC, no PAPI in claims
		{name: "POST_sync_url_user_denied", method: http.MethodPost, path: basePath + "/sync-server-url", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// POST delete-callback - admin only (400 = past RBAC, bad body)
		{name: "POST_delete_cb_admin_allowed", method: http.MethodPost, path: basePath + "/delete-callback", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_delete_cb_user_denied", method: http.MethodPost, path: basePath + "/delete-callback", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// Data denied on admin-only project endpoints
		{name: "DELETE_data_denied", method: http.MethodDelete, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "PATCH_auth_url_data_denied", method: http.MethodPatch, path: basePath + "/auth-server-url", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "POST_sync_url_data_denied", method: http.MethodPost, path: basePath + "/sync-server-url", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "POST_delete_cb_data_denied", method: http.MethodPost, path: basePath + "/delete-callback", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

// =============================================================================
// Queries Handler RBAC Tests
// =============================================================================

// mockQueryServiceForRBAC implements services.QueryService for RBAC tests.
type mockQueryServiceForRBAC struct{}

func (m *mockQueryServiceForRBAC) Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.CreateQueryRequest) (*models.Query, error) {
	return &models.Query{}, nil
}
func (m *mockQueryServiceForRBAC) Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	return &models.Query{}, nil
}
func (m *mockQueryServiceForRBAC) List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}
func (m *mockQueryServiceForRBAC) Update(ctx context.Context, projectID, queryID uuid.UUID, req *services.UpdateQueryRequest) (*models.Query, error) {
	return &models.Query{}, nil
}
func (m *mockQueryServiceForRBAC) Delete(ctx context.Context, projectID, queryID uuid.UUID) error {
	return nil
}
func (m *mockQueryServiceForRBAC) ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}
func (m *mockQueryServiceForRBAC) ListEnabledByTags(ctx context.Context, projectID, datasourceID uuid.UUID, tags []string) ([]*models.Query, error) {
	return nil, nil
}
func (m *mockQueryServiceForRBAC) HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error) {
	return false, nil
}
func (m *mockQueryServiceForRBAC) SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error {
	return nil
}
func (m *mockQueryServiceForRBAC) Execute(ctx context.Context, projectID, queryID uuid.UUID, req *services.ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return &datasource.QueryExecutionResult{}, nil
}
func (m *mockQueryServiceForRBAC) ExecuteWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any, req *services.ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return &datasource.QueryExecutionResult{}, nil
}
func (m *mockQueryServiceForRBAC) ExecuteModifyingWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any) (*datasource.ExecuteResult, error) {
	return &datasource.ExecuteResult{}, nil
}
func (m *mockQueryServiceForRBAC) Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.TestQueryRequest) (*datasource.QueryExecutionResult, error) {
	return &datasource.QueryExecutionResult{}, nil
}
func (m *mockQueryServiceForRBAC) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) (*services.ValidationResult, error) {
	return &services.ValidationResult{}, nil
}
func (m *mockQueryServiceForRBAC) ValidateParameterizedQuery(sqlQuery string, params []models.QueryParameter) error {
	return nil
}
func (m *mockQueryServiceForRBAC) SuggestUpdate(ctx context.Context, projectID uuid.UUID, req *services.SuggestUpdateRequest) (*models.Query, error) {
	return &models.Query{}, nil
}
func (m *mockQueryServiceForRBAC) DirectCreate(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.CreateQueryRequest) (*models.Query, error) {
	return &models.Query{}, nil
}
func (m *mockQueryServiceForRBAC) DirectUpdate(ctx context.Context, projectID, queryID uuid.UUID, req *services.UpdateQueryRequest) (*models.Query, error) {
	return &models.Query{}, nil
}
func (m *mockQueryServiceForRBAC) DeleteWithPendingRejection(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) (int, error) {
	return 0, nil
}
func (m *mockQueryServiceForRBAC) ApproveQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) error {
	return nil
}
func (m *mockQueryServiceForRBAC) RejectQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string, reason string) error {
	return nil
}
func (m *mockQueryServiceForRBAC) MoveToPending(ctx context.Context, projectID, queryID uuid.UUID) error {
	return nil
}
func (m *mockQueryServiceForRBAC) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}

func TestRBAC_QueriesHandler(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	queryID := uuid.New()
	mockService := &mockQueryServiceForRBAC{}
	handler := NewQueriesHandler(mockService, zap.NewNop())

	dsBase := "/api/projects/" + projectID.String() + "/datasources/" + datasourceID.String() + "/queries"
	projBase := "/api/projects/" + projectID.String() + "/queries"

	tests := []rbacTestCase{
		// ---------------------------------------------------------------
		// Read endpoints - all authenticated users
		// ---------------------------------------------------------------
		{name: "GET_list_admin_allowed", method: http.MethodGet, path: dsBase, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "GET_list_data_allowed", method: http.MethodGet, path: dsBase, roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "GET_list_user_allowed", method: http.MethodGet, path: dsBase, roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},
		{name: "GET_detail_user_allowed", method: http.MethodGet, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},
		{name: "GET_enabled_user_allowed", method: http.MethodGet, path: dsBase + "/enabled", roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},

		// ---------------------------------------------------------------
		// Create/Update/Delete - admin and data only
		// ---------------------------------------------------------------
		{name: "POST_create_admin_allowed", method: http.MethodPost, path: dsBase, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_create_data_allowed", method: http.MethodPost, path: dsBase, roles: []string{models.RoleData}, expectedStatus: http.StatusBadRequest},
		{name: "POST_create_user_denied", method: http.MethodPost, path: dsBase, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "PUT_update_admin_allowed", method: http.MethodPut, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "PUT_update_data_allowed", method: http.MethodPut, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleData}, expectedStatus: http.StatusBadRequest},
		{name: "PUT_update_user_denied", method: http.MethodPut, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// Queries Delete returns 200 with JSON (not 204)
		{name: "DELETE_admin_allowed", method: http.MethodDelete, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "DELETE_data_allowed", method: http.MethodDelete, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "DELETE_user_denied", method: http.MethodDelete, path: dsBase + "/" + queryID.String(), roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// ---------------------------------------------------------------
		// Execute - all authenticated users
		// ---------------------------------------------------------------
		// Execute body is optional; mock returns empty result → 200
		{name: "POST_execute_admin_allowed", method: http.MethodPost, path: dsBase + "/" + queryID.String() + "/execute", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "POST_execute_data_allowed", method: http.MethodPost, path: dsBase + "/" + queryID.String() + "/execute", roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "POST_execute_user_allowed", method: http.MethodPost, path: dsBase + "/" + queryID.String() + "/execute", roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},

		// ---------------------------------------------------------------
		// Test/Validate - admin and data only
		// ---------------------------------------------------------------
		{name: "POST_test_admin_allowed", method: http.MethodPost, path: dsBase + "/test", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_test_data_allowed", method: http.MethodPost, path: dsBase + "/test", roles: []string{models.RoleData}, expectedStatus: http.StatusBadRequest},
		{name: "POST_test_user_denied", method: http.MethodPost, path: dsBase + "/test", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "POST_validate_admin_allowed", method: http.MethodPost, path: dsBase + "/validate", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_validate_user_denied", method: http.MethodPost, path: dsBase + "/validate", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "POST_validate_params_admin_allowed", method: http.MethodPost, path: dsBase + "/validate-parameters", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_validate_params_user_denied", method: http.MethodPost, path: dsBase + "/validate-parameters", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// ---------------------------------------------------------------
		// Pending list - admin and data only
		// ---------------------------------------------------------------
		{name: "GET_pending_admin_allowed", method: http.MethodGet, path: projBase + "/pending", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "GET_pending_data_allowed", method: http.MethodGet, path: projBase + "/pending", roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "GET_pending_user_denied", method: http.MethodGet, path: projBase + "/pending", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// ---------------------------------------------------------------
		// Approve/Reject/MoveToPending - admin only
		// ---------------------------------------------------------------
		{name: "POST_approve_admin_allowed", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/approve", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "POST_approve_data_denied", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/approve", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "POST_approve_user_denied", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/approve", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "POST_reject_admin_allowed", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/reject", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_reject_data_denied", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/reject", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "POST_reject_user_denied", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/reject", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "POST_move_pending_admin_allowed", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/move-to-pending", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "POST_move_pending_data_denied", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/move-to-pending", roles: []string{models.RoleData}, expectedStatus: http.StatusForbidden},
		{name: "POST_move_pending_user_denied", method: http.MethodPost, path: projBase + "/" + queryID.String() + "/move-to-pending", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

// =============================================================================
// Glossary Handler RBAC Tests
// =============================================================================

// mockGlossaryServiceForRBAC implements services.GlossaryService for RBAC tests.
type mockGlossaryServiceForRBAC struct{}

func (m *mockGlossaryServiceForRBAC) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	return nil
}
func (m *mockGlossaryServiceForRBAC) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	return nil
}
func (m *mockGlossaryServiceForRBAC) DeleteTerm(ctx context.Context, termID uuid.UUID) error {
	return nil
}
func (m *mockGlossaryServiceForRBAC) GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForRBAC) GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForRBAC) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForRBAC) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*services.SQLTestResult, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForRBAC) CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}
func (m *mockGlossaryServiceForRBAC) DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}
func (m *mockGlossaryServiceForRBAC) SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForRBAC) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockGlossaryServiceForRBAC) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockGlossaryServiceForRBAC) GetGenerationStatus(projectID uuid.UUID) *models.GlossaryGenerationStatus {
	return &models.GlossaryGenerationStatus{Status: "idle"}
}
func (m *mockGlossaryServiceForRBAC) RunAutoGenerate(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// mockQuestionServiceForRBAC implements services.OntologyQuestionService for RBAC tests.
type mockQuestionServiceForRBAC struct{}

func (m *mockQuestionServiceForRBAC) GetNextQuestion(ctx context.Context, projectID uuid.UUID, includeSkipped bool) (*models.OntologyQuestion, error) {
	return nil, nil
}
func (m *mockQuestionServiceForRBAC) GetPendingQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	return nil, nil
}
func (m *mockQuestionServiceForRBAC) GetPendingCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockQuestionServiceForRBAC) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	return &repositories.QuestionCounts{}, nil
}
func (m *mockQuestionServiceForRBAC) AnswerQuestion(ctx context.Context, questionID uuid.UUID, answer string, userID string) (*models.AnswerResult, error) {
	return nil, nil
}
func (m *mockQuestionServiceForRBAC) SkipQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}
func (m *mockQuestionServiceForRBAC) DeleteQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}
func (m *mockQuestionServiceForRBAC) CreateQuestions(ctx context.Context, questions []*models.OntologyQuestion) error {
	return nil
}

func TestRBAC_GlossaryHandler(t *testing.T) {
	projectID := uuid.New()
	termID := uuid.New()
	mockGlossary := &mockGlossaryServiceForRBAC{}
	mockQuestions := &mockQuestionServiceForRBAC{}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	basePath := "/api/projects/" + projectID.String() + "/glossary"

	tests := []rbacTestCase{
		// ---------------------------------------------------------------
		// Read endpoints - all authenticated users
		// ---------------------------------------------------------------
		{name: "GET_list_admin_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "GET_list_data_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "GET_list_user_allowed", method: http.MethodGet, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusOK},
		// GET detail returns 404 when mock returns nil term (past RBAC)
		{name: "GET_detail_user_allowed", method: http.MethodGet, path: basePath + "/" + termID.String(), roles: []string{models.RoleUser}, expectedStatus: http.StatusNotFound},

		// POST test-sql - all authenticated users (read-only validation)
		{name: "POST_test_sql_admin_allowed", method: http.MethodPost, path: basePath + "/test-sql", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_test_sql_user_allowed", method: http.MethodPost, path: basePath + "/test-sql", roles: []string{models.RoleUser}, expectedStatus: http.StatusBadRequest},

		// ---------------------------------------------------------------
		// Create/Update/Delete - admin and data only
		// ---------------------------------------------------------------
		{name: "POST_create_admin_allowed", method: http.MethodPost, path: basePath, roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "POST_create_data_allowed", method: http.MethodPost, path: basePath, roles: []string{models.RoleData}, expectedStatus: http.StatusBadRequest},
		{name: "POST_create_user_denied", method: http.MethodPost, path: basePath, roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "PUT_update_admin_allowed", method: http.MethodPut, path: basePath + "/" + termID.String(), roles: []string{models.RoleAdmin}, expectedStatus: http.StatusBadRequest},
		{name: "PUT_update_data_allowed", method: http.MethodPut, path: basePath + "/" + termID.String(), roles: []string{models.RoleData}, expectedStatus: http.StatusBadRequest},
		{name: "PUT_update_user_denied", method: http.MethodPut, path: basePath + "/" + termID.String(), roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// Glossary Delete returns 200 with JSON (not 204)
		{name: "DELETE_admin_allowed", method: http.MethodDelete, path: basePath + "/" + termID.String(), roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "DELETE_data_allowed", method: http.MethodDelete, path: basePath + "/" + termID.String(), roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "DELETE_user_denied", method: http.MethodDelete, path: basePath + "/" + termID.String(), roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		// ---------------------------------------------------------------
		// Suggest/Auto-generate - admin and data only
		// ---------------------------------------------------------------
		// Suggest returns 200 with empty suggestions (mock returns nil)
		{name: "POST_suggest_admin_allowed", method: http.MethodPost, path: basePath + "/suggest", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusOK},
		{name: "POST_suggest_data_allowed", method: http.MethodPost, path: basePath + "/suggest", roles: []string{models.RoleData}, expectedStatus: http.StatusOK},
		{name: "POST_suggest_user_denied", method: http.MethodPost, path: basePath + "/suggest", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},

		{name: "POST_autogen_admin_allowed", method: http.MethodPost, path: basePath + "/auto-generate", roles: []string{models.RoleAdmin}, expectedStatus: http.StatusAccepted},
		{name: "POST_autogen_data_allowed", method: http.MethodPost, path: basePath + "/auto-generate", roles: []string{models.RoleData}, expectedStatus: http.StatusAccepted},
		{name: "POST_autogen_user_denied", method: http.MethodPost, path: basePath + "/auto-generate", roles: []string{models.RoleUser}, expectedStatus: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRBACTest(t, projectID, handler.RegisterRoutes, tc)
		})
	}
}

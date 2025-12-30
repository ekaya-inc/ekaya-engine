//go:build integration

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres"
	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// parameterizedQueriesTestContext holds dependencies for parameterized query tests.
type parameterizedQueriesTestContext struct {
	t              *testing.T
	testDB         *testhelpers.TestDB
	engineDB       *testhelpers.EngineDB
	queriesHandler *QueriesHandler
	datasourcesSvc services.DatasourceService
	projectID      uuid.UUID
	createdDsID    uuid.UUID
}

// setupParameterizedQueriesTest creates a test context with real database and services.
func setupParameterizedQueriesTest(t *testing.T) *parameterizedQueriesTestContext {
	t.Helper()

	testDB := testhelpers.GetTestDB(t)
	engineDB := testhelpers.GetEngineDB(t)

	encryptor, err := crypto.NewCredentialEncryptor(testEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	adapterFactory := datasource.NewDatasourceAdapterFactory(nil)
	dsRepo := repositories.NewDatasourceRepository()
	dsSvc := services.NewDatasourceService(dsRepo, encryptor, adapterFactory, nil, zap.NewNop())

	auditor := audit.NewSecurityAuditor(zap.NewNop())
	queryRepo := repositories.NewQueryRepository()
	querySvc := services.NewQueryService(queryRepo, dsSvc, adapterFactory, auditor, zap.NewNop())

	handler := NewQueriesHandler(querySvc, zap.NewNop())

	// Use unique project ID to avoid conflicts
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	return &parameterizedQueriesTestContext{
		t:              t,
		testDB:         testDB,
		engineDB:       engineDB,
		queriesHandler: handler,
		datasourcesSvc: dsSvc,
		projectID:      projectID,
	}
}

// makeRequest creates an HTTP request with proper context (tenant scope + auth claims).
func (tc *parameterizedQueriesTestContext) makeRequest(method, path string, body any) *http.Request {
	tc.t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			tc.t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	ctx := req.Context()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "test-user-parameterized",
		},
		ProjectID: tc.projectID.String(),
	}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	req = req.WithContext(ctx)

	tc.t.Cleanup(func() {
		scope.Close()
	})

	return req
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *parameterizedQueriesTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Parameterized Queries Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates a datasource pointing to the test_data database.
func (tc *parameterizedQueriesTestContext) ensureTestDatasource() {
	tc.t.Helper()

	tc.ensureTestProject()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE project_id = $1", tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup datasources: %v", err)
	}

	host, err := tc.testDB.Container.Host(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to get container host: %v", err)
	}
	port, err := tc.testDB.Container.MappedPort(ctx, "5432")
	if err != nil {
		tc.t.Fatalf("Failed to get container port: %v", err)
	}

	dsConfig := map[string]any{
		"host":     host,
		"port":     port.Int(),
		"user":     "ekaya",
		"password": "test_password",
		"database": "test_data",
		"ssl_mode": "disable",
	}

	ds, err := tc.datasourcesSvc.Create(database.SetTenantScope(ctx, scope), tc.projectID, "Test Data DB", "postgres", dsConfig)
	if err != nil {
		tc.t.Fatalf("Failed to create test datasource: %v", err)
	}

	tc.createdDsID = ds.ID
}

// cleanupQueries removes all queries for the test datasource.
func (tc *parameterizedQueriesTestContext) cleanupQueries() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, "DELETE FROM engine_queries WHERE project_id = $1", tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup queries: %v", err)
	}
}

// TestCreateQueryWithParameters tests creating queries with parameter definitions.
func TestCreateQueryWithParameters(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	tests := []struct {
		name          string
		request       CreateQueryRequest
		expectStatus  int
		expectSuccess bool
	}{
		{
			name: "create query with valid parameters",
			request: CreateQueryRequest{
				NaturalLanguagePrompt: "Get user by ID",
				SQLQuery:              "SELECT id, name FROM users WHERE id = :user_id",
				IsEnabled:             true,
				Parameters: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Description: "User ID", Required: true},
				},
			},
			expectStatus:  http.StatusCreated,
			expectSuccess: true,
		},
		{
			name: "create query with multiple parameters",
			request: CreateQueryRequest{
				NaturalLanguagePrompt: "Search users by email and status",
				SQLQuery:              "SELECT id, name, email FROM users WHERE email LIKE :email AND status = :status",
				IsEnabled:             true,
				Parameters: []models.QueryParameter{
					{Name: "email", Type: "string", Description: "Email pattern", Required: true},
					{Name: "status", Type: "string", Description: "User status", Required: false, Default: "active"},
				},
			},
			expectStatus:  http.StatusCreated,
			expectSuccess: true,
		},
		{
			name: "create query without parameters (backward compatibility)",
			request: CreateQueryRequest{
				NaturalLanguagePrompt: "Count all users",
				SQLQuery:              "SELECT COUNT(*) as count FROM users",
				IsEnabled:             true,
			},
			expectStatus:  http.StatusCreated,
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tc.makeRequest(http.MethodPost,
				"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
				tt.request)
			req.SetPathValue("pid", tc.projectID.String())
			req.SetPathValue("did", tc.createdDsID.String())

			rec := httptest.NewRecorder()
			tc.queriesHandler.Create(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code, "Response body: %s", rec.Body.String())

			var resp ApiResponse
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			assert.Equal(t, tt.expectSuccess, resp.Success)

			if tt.expectSuccess {
				data, ok := resp.Data.(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, data["query_id"])

				// Verify parameters are returned
				if len(tt.request.Parameters) > 0 {
					params, ok := data["parameters"].([]any)
					require.True(t, ok)
					assert.Len(t, params, len(tt.request.Parameters))
				}
			}
		})
	}
}

// TestExecuteQueryWithParameters tests executing queries with parameter values.
func TestExecuteQueryWithParameters(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Create a parameterized query
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Get user by ID",
		SQLQuery:              "SELECT id, name FROM users WHERE id = :user_id",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{Name: "user_id", Type: "integer", Description: "User ID", Required: true},
		},
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp ApiResponse
	err := json.Unmarshal(createRec.Body.Bytes(), &createResp)
	require.NoError(t, err)

	createData := createResp.Data.(map[string]any)
	queryID := createData["query_id"].(string)

	tests := []struct {
		name          string
		parameters    map[string]any
		expectStatus  int
		expectSuccess bool
		expectRows    bool
	}{
		{
			name:          "execute with valid parameters",
			parameters:    map[string]any{"user_id": 1},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
			expectRows:    true,
		},
		{
			name:          "execute with missing required parameter",
			parameters:    map[string]any{},
			expectStatus:  http.StatusInternalServerError,
			expectSuccess: false,
			expectRows:    false,
		},
		{
			name:          "execute with SQL injection attempt",
			parameters:    map[string]any{"user_id": "1; DROP TABLE users--"},
			expectStatus:  http.StatusInternalServerError,
			expectSuccess: false,
			expectRows:    false,
		},
		{
			name:          "execute with type coercion (string to int)",
			parameters:    map[string]any{"user_id": "1"},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
			expectRows:    true,
		},
		{
			name:          "execute with invalid type",
			parameters:    map[string]any{"user_id": "not-a-number"},
			expectStatus:  http.StatusInternalServerError,
			expectSuccess: false,
			expectRows:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execBody := ExecuteQueryRequest{
				Limit:      10,
				Parameters: tt.parameters,
			}

			execReq := tc.makeRequest(http.MethodPost,
				"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID+"/execute",
				execBody)
			execReq.SetPathValue("pid", tc.projectID.String())
			execReq.SetPathValue("did", tc.createdDsID.String())
			execReq.SetPathValue("qid", queryID)

			execRec := httptest.NewRecorder()
			tc.queriesHandler.Execute(execRec, execReq)

			assert.Equal(t, tt.expectStatus, execRec.Code, "Response body: %s", execRec.Body.String())

			var execResp ApiResponse
			err := json.Unmarshal(execRec.Body.Bytes(), &execResp)
			require.NoError(t, err)

			assert.Equal(t, tt.expectSuccess, execResp.Success)

			if tt.expectSuccess && tt.expectRows {
				execData := execResp.Data.(map[string]any)
				assert.Greater(t, int(execData["row_count"].(float64)), 0)
			}
		})
	}
}

// TestExecuteQueryBackwardCompatibility tests that queries without parameters still work.
func TestExecuteQueryBackwardCompatibility(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Create a non-parameterized query
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Count all users",
		SQLQuery:              "SELECT COUNT(*) as count FROM users",
		IsEnabled:             true,
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp ApiResponse
	err := json.Unmarshal(createRec.Body.Bytes(), &createResp)
	require.NoError(t, err)

	createData := createResp.Data.(map[string]any)
	queryID := createData["query_id"].(string)

	// Execute without parameters
	execBody := ExecuteQueryRequest{Limit: 10}
	execReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID+"/execute",
		execBody)
	execReq.SetPathValue("pid", tc.projectID.String())
	execReq.SetPathValue("did", tc.createdDsID.String())
	execReq.SetPathValue("qid", queryID)

	execRec := httptest.NewRecorder()
	tc.queriesHandler.Execute(execRec, execReq)

	require.Equal(t, http.StatusOK, execRec.Code)

	var execResp ApiResponse
	err = json.Unmarshal(execRec.Body.Bytes(), &execResp)
	require.NoError(t, err)

	assert.True(t, execResp.Success)

	execData := execResp.Data.(map[string]any)
	assert.Equal(t, 1, int(execData["row_count"].(float64)))
}

// TestTestQueryWithParameters tests the test endpoint with parameters.
func TestTestQueryWithParameters(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()

	tests := []struct {
		name          string
		request       TestQueryRequest
		expectStatus  int
		expectSuccess bool
	}{
		{
			name: "test with valid parameters",
			request: TestQueryRequest{
				SQLQuery: "SELECT id, name FROM users WHERE id = :user_id",
				Limit:    10,
				ParameterDefinitions: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Required: true},
				},
				ParameterValues: map[string]any{"user_id": 1},
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
		{
			name: "test with injection attempt",
			request: TestQueryRequest{
				SQLQuery: "SELECT id, name FROM users WHERE id = :user_id",
				Limit:    10,
				ParameterDefinitions: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Required: true},
				},
				ParameterValues: map[string]any{"user_id": "1; DROP TABLE users--"},
			},
			expectStatus:  http.StatusOK,
			expectSuccess: false,
		},
		{
			name: "test with missing required parameter",
			request: TestQueryRequest{
				SQLQuery: "SELECT id, name FROM users WHERE id = :user_id",
				Limit:    10,
				ParameterDefinitions: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Required: true},
				},
				ParameterValues: map[string]any{},
			},
			expectStatus:  http.StatusOK,
			expectSuccess: false,
		},
		{
			name: "test without parameters (backward compatibility)",
			request: TestQueryRequest{
				SQLQuery: "SELECT COUNT(*) FROM users",
				Limit:    10,
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
		{
			name: "test with parameter definition but no values",
			request: TestQueryRequest{
				SQLQuery: "SELECT id, name FROM users WHERE status = :status",
				Limit:    10,
				ParameterDefinitions: []models.QueryParameter{
					{Name: "status", Type: "string", Required: false, Default: "active"},
				},
				ParameterValues: map[string]any{},
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tc.makeRequest(http.MethodPost,
				"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
				tt.request)
			req.SetPathValue("pid", tc.projectID.String())
			req.SetPathValue("did", tc.createdDsID.String())

			rec := httptest.NewRecorder()
			tc.queriesHandler.Test(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code, "Response body: %s", rec.Body.String())

			var resp ApiResponse
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			assert.Equal(t, tt.expectSuccess, resp.Success)
		})
	}
}

// TestValidateParametersEndpoint tests the parameter validation endpoint.
func TestValidateParametersEndpoint(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()

	tests := []struct {
		name         string
		request      ValidateParametersRequest
		expectStatus int
		expectValid  bool
	}{
		{
			name: "valid parameter definitions match SQL",
			request: ValidateParametersRequest{
				SQLQuery: "SELECT id, name FROM users WHERE id = :user_id AND status = :status",
				Parameters: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Required: true},
					{Name: "status", Type: "string", Required: false, Default: "active"},
				},
			},
			expectStatus: http.StatusOK,
			expectValid:  true,
		},
		{
			name: "missing parameter in definitions",
			request: ValidateParametersRequest{
				SQLQuery: "SELECT id, name FROM users WHERE id = :user_id AND status = :status",
				Parameters: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Required: true},
				},
			},
			expectStatus: http.StatusOK,
			expectValid:  false,
		},
		{
			name: "extra parameter not in SQL",
			request: ValidateParametersRequest{
				SQLQuery: "SELECT id, name FROM users WHERE id = :user_id",
				Parameters: []models.QueryParameter{
					{Name: "user_id", Type: "integer", Required: true},
					{Name: "extra", Type: "string", Required: false},
				},
			},
			expectStatus: http.StatusOK,
			expectValid:  false,
		},
		{
			name: "empty SQL query",
			request: ValidateParametersRequest{
				SQLQuery:   "",
				Parameters: []models.QueryParameter{},
			},
			expectStatus: http.StatusBadRequest,
			expectValid:  false,
		},
		{
			name: "no parameters needed",
			request: ValidateParametersRequest{
				SQLQuery:   "SELECT COUNT(*) as count FROM users",
				Parameters: []models.QueryParameter{},
			},
			expectStatus: http.StatusOK,
			expectValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tc.makeRequest(http.MethodPost,
				"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate-parameters",
				tt.request)
			req.SetPathValue("pid", tc.projectID.String())
			req.SetPathValue("did", tc.createdDsID.String())

			rec := httptest.NewRecorder()
			tc.queriesHandler.ValidateParameters(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code, "Response body: %s", rec.Body.String())

			if tt.expectStatus == http.StatusOK {
				var resp ApiResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)

				assert.True(t, resp.Success)

				data, ok := resp.Data.(map[string]any)
				require.True(t, ok)

				assert.Equal(t, tt.expectValid, data["valid"])
				assert.NotEmpty(t, data["message"])
			}
		})
	}
}

// TestQueryResponseWithParameters tests that query responses include parameters.
func TestQueryResponseWithParameters(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Create a parameterized query
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Search users by status",
		SQLQuery:              "SELECT id, name FROM users WHERE status = :status",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{Name: "status", Type: "string", Description: "User status", Required: false, Default: "active"},
		},
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp ApiResponse
	err := json.Unmarshal(createRec.Body.Bytes(), &createResp)
	require.NoError(t, err)

	createData := createResp.Data.(map[string]any)
	queryID := createData["query_id"].(string)

	// Get the query back
	getReq := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID,
		nil)
	getReq.SetPathValue("pid", tc.projectID.String())
	getReq.SetPathValue("did", tc.createdDsID.String())
	getReq.SetPathValue("qid", queryID)

	getRec := httptest.NewRecorder()
	tc.queriesHandler.Get(getRec, getReq)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp ApiResponse
	err = json.Unmarshal(getRec.Body.Bytes(), &getResp)
	require.NoError(t, err)

	getData := getResp.Data.(map[string]any)

	// Verify parameters are present
	params, ok := getData["parameters"].([]any)
	require.True(t, ok)
	require.Len(t, params, 1)

	param := params[0].(map[string]any)
	assert.Equal(t, "status", param["name"])
	assert.Equal(t, "string", param["type"])
	assert.Equal(t, "User status", param["description"])
	assert.Equal(t, false, param["required"])
	assert.Equal(t, "active", param["default"])
}

// TestEndToEndParameterizedQueryFlow tests the complete workflow.
func TestEndToEndParameterizedQueryFlow(t *testing.T) {
	tc := setupParameterizedQueriesTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Step 1: Validate parameters before creating
	validateReq := ValidateParametersRequest{
		SQLQuery: "SELECT id, name FROM users WHERE id > :min_id AND status = :status ORDER BY id LIMIT :limit",
		Parameters: []models.QueryParameter{
			{Name: "min_id", Type: "integer", Required: true},
			{Name: "status", Type: "string", Required: false, Default: "active"},
			{Name: "limit", Type: "integer", Required: false, Default: 10},
		},
	}

	valReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate-parameters",
		validateReq)
	valReq.SetPathValue("pid", tc.projectID.String())
	valReq.SetPathValue("did", tc.createdDsID.String())

	valRec := httptest.NewRecorder()
	tc.queriesHandler.ValidateParameters(valRec, valReq)
	require.Equal(t, http.StatusOK, valRec.Code)

	var valResp ApiResponse
	err := json.Unmarshal(valRec.Body.Bytes(), &valResp)
	require.NoError(t, err)

	valData := valResp.Data.(map[string]any)
	assert.True(t, valData["valid"].(bool))

	// Step 2: Test the query with sample parameters
	testBody := TestQueryRequest{
		SQLQuery: "SELECT id, name FROM users WHERE id > :min_id AND status = :status ORDER BY id LIMIT :limit",
		Limit:    100,
		ParameterDefinitions: []models.QueryParameter{
			{Name: "min_id", Type: "integer", Required: true},
			{Name: "status", Type: "string", Required: false, Default: "active"},
			{Name: "limit", Type: "integer", Required: false, Default: 10},
		},
		ParameterValues: map[string]any{
			"min_id": 0,
			"status": "active",
			"limit":  5,
		},
	}

	testReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("did", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)
	require.Equal(t, http.StatusOK, testRec.Code)

	var testResp ApiResponse
	err = json.Unmarshal(testRec.Body.Bytes(), &testResp)
	require.NoError(t, err)
	assert.True(t, testResp.Success)

	// Step 3: Create the query
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Get users by minimum ID and status",
		SQLQuery:              "SELECT id, name FROM users WHERE id > :min_id AND status = :status ORDER BY id LIMIT :limit",
		IsEnabled:             true,
		Parameters: []models.QueryParameter{
			{Name: "min_id", Type: "integer", Description: "Minimum user ID", Required: true},
			{Name: "status", Type: "string", Description: "User status", Required: false, Default: "active"},
			{Name: "limit", Type: "integer", Description: "Max results", Required: false, Default: 10},
		},
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp ApiResponse
	err = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	require.NoError(t, err)

	createData := createResp.Data.(map[string]any)
	queryID := createData["query_id"].(string)

	// Step 4: Execute with valid parameters
	execBody := ExecuteQueryRequest{
		Limit: 100,
		Parameters: map[string]any{
			"min_id": 0,
			"status": "active",
			"limit":  5,
		},
	}

	execReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID+"/execute",
		execBody)
	execReq.SetPathValue("pid", tc.projectID.String())
	execReq.SetPathValue("did", tc.createdDsID.String())
	execReq.SetPathValue("qid", queryID)

	execRec := httptest.NewRecorder()
	tc.queriesHandler.Execute(execRec, execReq)
	require.Equal(t, http.StatusOK, execRec.Code)

	var execResp ApiResponse
	err = json.Unmarshal(execRec.Body.Bytes(), &execResp)
	require.NoError(t, err)
	assert.True(t, execResp.Success)

	execData := execResp.Data.(map[string]any)
	rows := execData["rows"].([]any)
	assert.LessOrEqual(t, len(rows), 5) // Should respect limit parameter

	// Step 5: Attempt injection
	injectionBody := ExecuteQueryRequest{
		Limit: 100,
		Parameters: map[string]any{
			"min_id": "0; DROP TABLE users--",
			"status": "active",
			"limit":  5,
		},
	}

	injectionReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID+"/execute",
		injectionBody)
	injectionReq.SetPathValue("pid", tc.projectID.String())
	injectionReq.SetPathValue("did", tc.createdDsID.String())
	injectionReq.SetPathValue("qid", queryID)

	injectionRec := httptest.NewRecorder()
	tc.queriesHandler.Execute(injectionRec, injectionReq)

	// Should fail with internal error (not succeed)
	assert.Equal(t, http.StatusInternalServerError, injectionRec.Code)

	var injectionResp ApiResponse
	err = json.Unmarshal(injectionRec.Body.Bytes(), &injectionResp)
	require.NoError(t, err)
	assert.False(t, injectionResp.Success)

	// Step 6: Verify query is in list with parameters
	listReq := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		nil)
	listReq.SetPathValue("pid", tc.projectID.String())
	listReq.SetPathValue("did", tc.createdDsID.String())

	listRec := httptest.NewRecorder()
	tc.queriesHandler.List(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp ApiResponse
	err = json.Unmarshal(listRec.Body.Bytes(), &listResp)
	require.NoError(t, err)

	listData := listResp.Data.(map[string]any)
	queries := listData["queries"].([]any)
	assert.Len(t, queries, 1)

	query := queries[0].(map[string]any)
	params := query["parameters"].([]any)
	assert.Len(t, params, 3)
}

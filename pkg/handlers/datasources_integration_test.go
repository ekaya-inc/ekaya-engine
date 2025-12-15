//go:build integration

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test encryption key (32 bytes base64 encoded, generated with: openssl rand -base64 32)
const testEncryptionKey = "dux2otOLmF8mbcGKm/hk4+WBVT05FmorIokpgrypt9Y="

// integrationMockAdapterFactory is a mock implementation for integration tests.
// Named differently to avoid conflict with mockAdapterFactory in config_test.go.
type integrationMockAdapterFactory struct{}

func (f *integrationMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any) (datasource.ConnectionTester, error) {
	return &integrationMockConnectionTester{}, nil
}

func (f *integrationMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return []datasource.DatasourceAdapterInfo{
		{Type: "postgres", DisplayName: "PostgreSQL", Description: "PostgreSQL database"},
	}
}

type integrationMockConnectionTester struct{}

func (t *integrationMockConnectionTester) TestConnection(ctx context.Context) error {
	return nil
}

func (t *integrationMockConnectionTester) Close() error {
	return nil
}

// integrationTestContext holds all dependencies for integration tests.
type integrationTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	handler   *DatasourcesHandler
	projectID uuid.UUID
}

// setupIntegrationTest creates a test context with real database and services.
func setupIntegrationTest(t *testing.T) *integrationTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	// Create encryptor
	encryptor, err := crypto.NewCredentialEncryptor(testEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// Create real repository
	repo := repositories.NewDatasourceRepository()

	// Create service with mock adapter factory
	service := services.NewDatasourceService(
		repo,
		encryptor,
		&integrationMockAdapterFactory{},
		zap.NewNop(),
	)

	// Create handler
	handler := NewDatasourcesHandler(service, zap.NewNop())

	// Use a fixed project ID for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	return &integrationTestContext{
		t:         t,
		engineDB:  engineDB,
		handler:   handler,
		projectID: projectID,
	}
}

// makeRequest creates an HTTP request with proper context (tenant scope + auth claims).
func (tc *integrationTestContext) makeRequest(method, path string, body any) *http.Request {
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

	// Set up tenant scope
	ctx := req.Context()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	// Set up auth claims
	claims := &auth.Claims{ProjectID: tc.projectID.String()}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	req = req.WithContext(ctx)

	// Clean up tenant scope after test
	tc.t.Cleanup(func() {
		scope.Close()
	})

	return req
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *integrationTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	// Insert project if it doesn't exist
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanupDatasources removes all datasources for the test project.
func (tc *integrationTestContext) cleanupDatasources() {
	tc.t.Helper()

	// Ensure project exists first
	tc.ensureTestProject()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE project_id = $1", tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup datasources: %v", err)
	}
}

func TestDatasourcesIntegration_CreateSuccess(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	body := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "Test Database",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": "secret",
			"database": "testdb",
			"ssl_mode": "disable",
		},
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", body)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success to be true, got error: %s", resp.Error)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}

	if data["datasource_id"] == "" {
		t.Error("expected datasource_id to be set")
	}

	if data["type"] != "postgres" {
		t.Errorf("expected type 'postgres', got %v", data["type"])
	}

	// Verify name is returned as explicit field
	if data["name"] != "Test Database" {
		t.Errorf("expected name 'Test Database', got %v", data["name"])
	}

	// Verify password is returned (not masked) - UI handles display masking
	config := data["config"].(map[string]any)
	if config["password"] != "secret" {
		t.Errorf("expected real password 'secret', got %v", config["password"])
	}
}

func TestDatasourcesIntegration_CreateDuplicateName(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	// Create first datasource
	body := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "Duplicate Test DB",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": "secret",
			"database": "duplicate_test_db",
			"ssl_mode": "disable",
		},
	}

	req1 := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", body)
	req1.SetPathValue("pid", tc.projectID.String())

	rec1 := httptest.NewRecorder()
	tc.handler.Create(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("first create failed with status %d: %s", rec1.Code, rec1.Body.String())
	}

	// Try to create second datasource with same name
	// Note: With one-datasource-per-project policy, the limit check happens before
	// the duplicate name check, so we get datasource_limit_reached instead of duplicate_name
	req2 := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", body)
	req2.SetPathValue("pid", tc.projectID.String())

	rec2 := httptest.NewRecorder()
	tc.handler.Create(rec2, req2)

	// Second create should return 409 Conflict (due to datasource limit, not duplicate name)
	if rec2.Code != http.StatusConflict {
		t.Errorf("expected status 409 Conflict, got %d: %s", rec2.Code, rec2.Body.String())
	}

	// Verify error response format - with one-datasource-per-project, the limit check fires first
	var resp map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if resp["error"] != "datasource_limit_reached" {
		t.Errorf("expected error 'datasource_limit_reached', got %q", resp["error"])
	}

	if resp["message"] != "Only one datasource per project is currently supported" {
		t.Errorf("expected message about datasource limit, got %q", resp["message"])
	}
}

func TestDatasourcesIntegration_ListAfterCreate(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	// Create a datasource first
	createBody := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "List Test DB",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": "secret",
			"database": "list_test_db",
			"ssl_mode": "disable",
		},
	}

	createReq := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", createBody)
	createReq.SetPathValue("pid", tc.projectID.String())

	createRec := httptest.NewRecorder()
	tc.handler.Create(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create failed with status %d: %s", createRec.Code, createRec.Body.String())
	}

	// Now list datasources
	listReq := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/datasources", nil)
	listReq.SetPathValue("pid", tc.projectID.String())

	listRec := httptest.NewRecorder()
	tc.handler.List(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success to be true")
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}

	datasources, ok := data["datasources"].([]any)
	if !ok {
		t.Fatalf("expected datasources to be an array")
	}

	if len(datasources) != 1 {
		t.Errorf("expected 1 datasource, got %d", len(datasources))
	}

	ds := datasources[0].(map[string]any)
	if ds["name"] != "List Test DB" {
		t.Errorf("expected name 'List Test DB', got %v", ds["name"])
	}
}

func TestDatasourcesIntegration_CreateDeleteCreate(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	// Create first datasource
	firstBody := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "First DB",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": "secret",
			"database": "first_db",
			"ssl_mode": "disable",
		},
	}

	req1 := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", firstBody)
	req1.SetPathValue("pid", tc.projectID.String())

	rec1 := httptest.NewRecorder()
	tc.handler.Create(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("first create failed with status %d: %s", rec1.Code, rec1.Body.String())
	}

	// Extract datasource ID from response
	var createResp ApiResponse
	if err := json.Unmarshal(rec1.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to parse create response: %v", err)
	}
	data := createResp.Data.(map[string]any)
	datasourceID := data["datasource_id"].(string)

	// Delete the datasource
	deleteReq := tc.makeRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/datasources/"+datasourceID, nil)
	deleteReq.SetPathValue("pid", tc.projectID.String())
	deleteReq.SetPathValue("id", datasourceID)

	deleteRec := httptest.NewRecorder()
	tc.handler.Delete(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete failed with status %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	// Create again - should succeed since we deleted the first one
	secondBody := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "Second DB",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "newhost",
			"port":     5432,
			"user":     "newuser",
			"password": "newsecret",
			"database": "second_db",
			"ssl_mode": "disable",
		},
	}

	req2 := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", secondBody)
	req2.SetPathValue("pid", tc.projectID.String())

	rec2 := httptest.NewRecorder()
	tc.handler.Create(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Errorf("second create after delete failed with status %d: %s", rec2.Code, rec2.Body.String())
	}

	// Verify the new datasource was created
	var secondResp ApiResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("failed to parse second create response: %v", err)
	}

	if !secondResp.Success {
		t.Error("expected success to be true for second create")
	}

	secondData := secondResp.Data.(map[string]any)
	if secondData["name"] != "Second DB" {
		t.Errorf("expected name 'Second DB', got %v", secondData["name"])
	}
}

func TestDatasourcesIntegration_PasswordNotMasked(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	testPassword := "my_secret_password_123"

	// Create a datasource
	createBody := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "Password Test DB",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": testPassword,
			"database": "password_test_db",
			"ssl_mode": "disable",
		},
	}

	createReq := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", createBody)
	createReq.SetPathValue("pid", tc.projectID.String())

	createRec := httptest.NewRecorder()
	tc.handler.Create(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create failed with status %d: %s", createRec.Code, createRec.Body.String())
	}

	// Parse create response to get datasource ID
	var createResp ApiResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to parse create response: %v", err)
	}

	// Verify password is NOT masked in create response
	createData := createResp.Data.(map[string]any)
	createConfig := createData["config"].(map[string]any)
	if createConfig["password"] != testPassword {
		t.Errorf("CREATE: expected real password %q, got %q", testPassword, createConfig["password"])
	}

	datasourceID := createData["datasource_id"].(string)

	// Get the datasource - password should NOT be masked
	getReq := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/datasources/"+datasourceID, nil)
	getReq.SetPathValue("pid", tc.projectID.String())
	getReq.SetPathValue("id", datasourceID)

	getRec := httptest.NewRecorder()
	tc.handler.Get(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get failed with status %d: %s", getRec.Code, getRec.Body.String())
	}

	var getResp ApiResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse get response: %v", err)
	}

	getData := getResp.Data.(map[string]any)
	getConfig := getData["config"].(map[string]any)
	if getConfig["password"] != testPassword {
		t.Errorf("GET: expected real password %q, got %q", testPassword, getConfig["password"])
	}

	// List datasources - password should NOT be masked
	listReq := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/datasources", nil)
	listReq.SetPathValue("pid", tc.projectID.String())

	listRec := httptest.NewRecorder()
	tc.handler.List(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list failed with status %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp ApiResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse list response: %v", err)
	}

	listData := listResp.Data.(map[string]any)
	datasources := listData["datasources"].([]any)
	if len(datasources) != 1 {
		t.Fatalf("expected 1 datasource, got %d", len(datasources))
	}

	listDs := datasources[0].(map[string]any)
	listConfig := listDs["config"].(map[string]any)
	if listConfig["password"] != testPassword {
		t.Errorf("LIST: expected real password %q, got %q", testPassword, listConfig["password"])
	}
}

func TestDatasourcesIntegration_ExplicitNameField(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	// Create with explicit name field (not derived from config)
	createBody := map[string]any{
		"project_id": tc.projectID.String(),
		"name":       "My Production Database",
		"type":       "postgres",
		"config": map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": "secret",
			"database": "testdb", // Note: "database" not "name" - database name is separate from datasource name
			"ssl_mode": "disable",
		},
	}

	createReq := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", createBody)
	createReq.SetPathValue("pid", tc.projectID.String())

	createRec := httptest.NewRecorder()
	tc.handler.Create(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create failed with status %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp ApiResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to parse create response: %v", err)
	}

	createData := createResp.Data.(map[string]any)

	// Verify name is returned as a separate field in response
	if createData["name"] != "My Production Database" {
		t.Errorf("CREATE: expected name 'My Production Database', got %v", createData["name"])
	}

	datasourceID := createData["datasource_id"].(string)

	// Update with explicit name field
	updateBody := map[string]any{
		"name": "Renamed Database",
		"type": "postgres",
		"config": map[string]any{
			"host":     "newhost",
			"port":     5432,
			"user":     "newuser",
			"password": "newsecret",
			"database": "newdb",
			"ssl_mode": "disable",
		},
	}

	updateReq := tc.makeRequest(http.MethodPut, "/api/projects/"+tc.projectID.String()+"/datasources/"+datasourceID, updateBody)
	updateReq.SetPathValue("pid", tc.projectID.String())
	updateReq.SetPathValue("id", datasourceID)

	updateRec := httptest.NewRecorder()
	tc.handler.Update(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Errorf("update failed with status %d: %s", updateRec.Code, updateRec.Body.String())
	}

	var updateResp ApiResponse
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("failed to parse update response: %v", err)
	}

	updateData := updateResp.Data.(map[string]any)

	// Verify name is returned as a separate field in update response
	if updateData["name"] != "Renamed Database" {
		t.Errorf("UPDATE: expected name 'Renamed Database', got %v", updateData["name"])
	}

	// Get and verify the name was persisted
	getReq := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/datasources/"+datasourceID, nil)
	getReq.SetPathValue("pid", tc.projectID.String())
	getReq.SetPathValue("id", datasourceID)

	getRec := httptest.NewRecorder()
	tc.handler.Get(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get failed with status %d: %s", getRec.Code, getRec.Body.String())
	}

	var getResp ApiResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse get response: %v", err)
	}

	getData := getResp.Data.(map[string]any)
	if getData["name"] != "Renamed Database" {
		t.Errorf("GET: expected name 'Renamed Database', got %v", getData["name"])
	}
}

func TestDatasourcesIntegration_OneDatasourcePerProject(t *testing.T) {
	tc := setupIntegrationTest(t)
	tc.cleanupDatasources()

	// Create first datasource - should succeed
	firstBody := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "First DB",
		Type:      "postgres",
		Config: map[string]any{
			"host":     "localhost",
			"port":     5432,
			"user":     "test",
			"password": "secret",
			"database": "first_db",
			"ssl_mode": "disable",
		},
	}

	req1 := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", firstBody)
	req1.SetPathValue("pid", tc.projectID.String())

	rec1 := httptest.NewRecorder()
	tc.handler.Create(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("first create failed with status %d: %s", rec1.Code, rec1.Body.String())
	}

	// Create second datasource with DIFFERENT name - should fail (one datasource per project policy)
	secondBody := CreateDatasourceRequest{
		ProjectID: tc.projectID.String(),
		Name:      "Second DB", // Different name
		Type:      "postgres",
		Config: map[string]any{
			"host":     "otherhost",
			"port":     5432,
			"user":     "other",
			"password": "othersecret",
			"database": "second_db",
			"ssl_mode": "disable",
		},
	}

	req2 := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/datasources", secondBody)
	req2.SetPathValue("pid", tc.projectID.String())

	rec2 := httptest.NewRecorder()
	tc.handler.Create(rec2, req2)

	// Second create should return 409 Conflict due to one-datasource-per-project policy
	if rec2.Code != http.StatusConflict {
		t.Errorf("expected status 409 Conflict, got %d: %s", rec2.Code, rec2.Body.String())
	}

	// Verify error response format
	var resp map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if resp["error"] != "datasource_limit_reached" {
		t.Errorf("expected error 'datasource_limit_reached', got %q", resp["error"])
	}

	if resp["message"] != "Only one datasource per project is currently supported" {
		t.Errorf("expected message about datasource limit, got %q", resp["message"])
	}
}

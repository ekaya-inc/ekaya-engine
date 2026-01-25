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
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// queriesIntegrationTestContext holds all dependencies for query integration tests.
type queriesIntegrationTestContext struct {
	t              *testing.T
	testDB         *testhelpers.TestDB
	engineDB       *testhelpers.EngineDB
	queriesHandler *QueriesHandler
	datasourcesSvc services.DatasourceService
	projectID      uuid.UUID
	createdDsID    uuid.UUID
}

// setupQueriesIntegrationTest creates a test context with real database and services.
func setupQueriesIntegrationTest(t *testing.T) *queriesIntegrationTestContext {
	t.Helper()

	// Get both databases from the test container
	testDB := testhelpers.GetTestDB(t)
	engineDB := testhelpers.GetEngineDB(t)

	// Create encryptor for datasource credentials
	encryptor, err := crypto.NewCredentialEncryptor(testEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// Create the real adapter factory (not a mock) for query execution
	// Pass nil for connection manager since tests use unmanaged pools
	adapterFactory := datasource.NewDatasourceAdapterFactory(nil)

	// Create datasource repository and service
	dsRepo := repositories.NewDatasourceRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	dsSvc := services.NewDatasourceService(dsRepo, ontologyRepo, encryptor, adapterFactory, nil, zap.NewNop())

	// Create security auditor
	auditor := audit.NewSecurityAuditor(zap.NewNop())

	// Create query repository and service
	queryRepo := repositories.NewQueryRepository()
	querySvc := services.NewQueryService(queryRepo, dsSvc, adapterFactory, auditor, zap.NewNop())

	// Create handler
	handler := NewQueriesHandler(querySvc, zap.NewNop())

	// Use a unique project ID for this test run to avoid conflicts
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	return &queriesIntegrationTestContext{
		t:              t,
		testDB:         testDB,
		engineDB:       engineDB,
		queriesHandler: handler,
		datasourcesSvc: dsSvc,
		projectID:      projectID,
	}
}

// makeRequest creates an HTTP request with proper context (tenant scope + auth claims).
// Returns the request and a cleanup function that MUST be called after the handler returns.
// This releases the database connection back to the pool immediately.
func (tc *queriesIntegrationTestContext) makeRequest(method, path string, body any) (*http.Request, func()) {
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

	// Set up auth claims with user ID (Subject) for connection pooling
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "test-user-integration",
		},
		ProjectID: tc.projectID.String(),
	}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	req = req.WithContext(ctx)

	// Return cleanup function to release connection immediately after handler returns
	return req, func() { scope.Close() }
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *queriesIntegrationTestContext) ensureTestProject() {
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
	`, tc.projectID, "Queries Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates a datasource pointing to the test_data database.
func (tc *queriesIntegrationTestContext) ensureTestDatasource() {
	tc.t.Helper()

	tc.ensureTestProject()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	// Clean up any existing datasources for this project
	_, err = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE project_id = $1", tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup datasources: %v", err)
	}

	// Get the connection info from the test container
	host, err := tc.testDB.Container.Host(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to get container host: %v", err)
	}
	port, err := tc.testDB.Container.MappedPort(ctx, "5432")
	if err != nil {
		tc.t.Fatalf("Failed to get container port: %v", err)
	}

	// Create datasource pointing to test_data database
	dsConfig := map[string]any{
		"host":     host,
		"port":     port.Int(),
		"user":     "ekaya",
		"password": "test_password",
		"database": "test_data",
		"ssl_mode": "disable",
	}

	ds, err := tc.datasourcesSvc.Create(database.SetTenantScope(ctx, scope), tc.projectID, "Test Data DB", "postgres", "", dsConfig)
	if err != nil {
		tc.t.Fatalf("Failed to create test datasource: %v", err)
	}

	tc.createdDsID = ds.ID
}

// cleanupQueries removes all queries for the test datasource.
func (tc *queriesIntegrationTestContext) cleanupQueries() {
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

func TestQueriesIntegration_CreateAndTestSimpleQuery(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// First, test the query before saving (using Test endpoint)
	// Note: No trailing semicolon because when limit is applied, query gets wrapped in subquery
	testBody := TestQueryRequest{
		SQLQuery: "SELECT 1 AS result",
		Limit:    10,
	}

	testReq, testCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("dsid", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)
	testCleanup()

	if testRec.Code != http.StatusOK {
		t.Fatalf("Test query failed with status %d: %s", testRec.Code, testRec.Body.String())
	}

	var testResp ApiResponse
	if err := json.Unmarshal(testRec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("Failed to parse test response: %v", err)
	}

	if !testResp.Success {
		t.Errorf("Expected test to succeed, got error: %s", testResp.Error)
	}

	testData, ok := testResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("Expected data to be a map, got %T", testResp.Data)
	}

	// Verify we got one row with result=1
	if int(testData["row_count"].(float64)) != 1 {
		t.Errorf("Expected 1 row, got %v", testData["row_count"])
	}

	// Now create the query
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Return the number 1",
		SQLQuery:              "SELECT 1 AS result",
		IsEnabled:             true,
		OutputColumns:         []models.OutputColumn{{Name: "result", Type: "INT4"}},
	}

	createReq, createCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("dsid", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	createCleanup()

	if createRec.Code != http.StatusCreated {
		t.Fatalf("Create query failed with status %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp ApiResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	if !createResp.Success {
		t.Errorf("Expected create to succeed, got error: %s", createResp.Error)
	}

	createData := createResp.Data.(map[string]any)
	queryID := createData["query_id"].(string)

	if queryID == "" {
		t.Error("Expected query_id to be set")
	}

	if createData["natural_language_prompt"] != "Return the number 1" {
		t.Errorf("Expected prompt 'Return the number 1', got %v", createData["natural_language_prompt"])
	}

	if createData["sql_query"] != "SELECT 1 AS result" {
		t.Errorf("Expected sql_query 'SELECT 1 AS result', got %v", createData["sql_query"])
	}

	if createData["dialect"] != "postgres" {
		t.Errorf("Expected dialect 'postgres', got %v", createData["dialect"])
	}

	if createData["is_enabled"] != true {
		t.Errorf("Expected is_enabled true, got %v", createData["is_enabled"])
	}
}

func TestQueriesIntegration_ExecuteSavedQuery(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Create a query
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Add two numbers",
		SQLQuery:              "SELECT 1 + 1 AS sum",
		IsEnabled:             true,
		OutputColumns:         []models.OutputColumn{{Name: "sum", Type: "INT4"}},
	}

	createReq, createCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("dsid", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	createCleanup()

	if createRec.Code != http.StatusCreated {
		t.Fatalf("Create query failed with status %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp ApiResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	createData := createResp.Data.(map[string]any)
	queryID := createData["query_id"].(string)

	// Execute the saved query
	execBody := ExecuteQueryRequest{Limit: 100}

	execReq, execCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID+"/execute",
		execBody)
	execReq.SetPathValue("pid", tc.projectID.String())
	execReq.SetPathValue("dsid", tc.createdDsID.String())
	execReq.SetPathValue("qid", queryID)

	execRec := httptest.NewRecorder()
	tc.queriesHandler.Execute(execRec, execReq)
	execCleanup()

	if execRec.Code != http.StatusOK {
		t.Fatalf("Execute query failed with status %d: %s", execRec.Code, execRec.Body.String())
	}

	var execResp ApiResponse
	if err := json.Unmarshal(execRec.Body.Bytes(), &execResp); err != nil {
		t.Fatalf("Failed to parse execute response: %v", err)
	}

	if !execResp.Success {
		t.Errorf("Expected execute to succeed, got error: %s", execResp.Error)
	}

	execData := execResp.Data.(map[string]any)

	// Verify result
	if int(execData["row_count"].(float64)) != 1 {
		t.Errorf("Expected 1 row, got %v", execData["row_count"])
	}

	columns := execData["columns"].([]any)
	if len(columns) != 1 {
		t.Errorf("Expected 1 column, got %v", columns)
	} else {
		col := columns[0].(map[string]any)
		if col["name"] != "sum" {
			t.Errorf("Expected column name 'sum', got %v", col["name"])
		}
	}

	rows := execData["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0].(map[string]any)
	// PostgreSQL returns int, JSON unmarshals as float64
	if int(row["sum"].(float64)) != 2 {
		t.Errorf("Expected sum=2, got %v", row["sum"])
	}
}

func TestQueriesIntegration_ValidateQuery(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()

	// Test valid query
	validBody := ValidateQueryRequest{
		SQLQuery: "SELECT * FROM users LIMIT 1",
	}

	validReq, validCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate",
		validBody)
	validReq.SetPathValue("pid", tc.projectID.String())
	validReq.SetPathValue("dsid", tc.createdDsID.String())

	validRec := httptest.NewRecorder()
	tc.queriesHandler.Validate(validRec, validReq)
	validCleanup()

	if validRec.Code != http.StatusOK {
		t.Fatalf("Validate query failed with status %d: %s", validRec.Code, validRec.Body.String())
	}

	var validResp ApiResponse
	if err := json.Unmarshal(validRec.Body.Bytes(), &validResp); err != nil {
		t.Fatalf("Failed to parse validate response: %v", err)
	}

	validData := validResp.Data.(map[string]any)
	if validData["valid"] != true {
		t.Errorf("Expected valid=true for valid SQL, got %v (message: %v)", validData["valid"], validData["message"])
	}

	// Test invalid query (syntax error)
	invalidBody := ValidateQueryRequest{
		SQLQuery: "SELEC * FROM users", // Missing 'T' in SELECT
	}

	invalidReq, invalidCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate",
		invalidBody)
	invalidReq.SetPathValue("pid", tc.projectID.String())
	invalidReq.SetPathValue("dsid", tc.createdDsID.String())

	invalidRec := httptest.NewRecorder()
	tc.queriesHandler.Validate(invalidRec, invalidReq)
	invalidCleanup()

	if invalidRec.Code != http.StatusOK {
		t.Fatalf("Validate query failed with status %d: %s", invalidRec.Code, invalidRec.Body.String())
	}

	var invalidResp ApiResponse
	if err := json.Unmarshal(invalidRec.Body.Bytes(), &invalidResp); err != nil {
		t.Fatalf("Failed to parse validate response: %v", err)
	}

	invalidData := invalidResp.Data.(map[string]any)
	if invalidData["valid"] != false {
		t.Errorf("Expected valid=false for invalid SQL, got %v", invalidData["valid"])
	}

	if invalidData["message"] == "" {
		t.Error("Expected error message for invalid SQL")
	}
}

func TestQueriesIntegration_QueryAgainstTestData(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Query the actual test data (users table has 95 rows as per containers_test.go)
	testBody := TestQueryRequest{
		SQLQuery: "SELECT COUNT(*) AS user_count FROM users",
		Limit:    10,
	}

	testReq, testCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("dsid", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)
	testCleanup()

	if testRec.Code != http.StatusOK {
		t.Fatalf("Test query failed with status %d: %s", testRec.Code, testRec.Body.String())
	}

	var testResp ApiResponse
	if err := json.Unmarshal(testRec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("Failed to parse test response: %v", err)
	}

	if !testResp.Success {
		t.Fatalf("Expected test to succeed, got error: %s", testResp.Error)
	}

	testData := testResp.Data.(map[string]any)
	rows := testData["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(rows))
	}

	row := rows[0].(map[string]any)
	userCount := int(row["user_count"].(float64))

	// The test container has 95 users (as verified in containers_test.go)
	if userCount != 95 {
		t.Errorf("Expected 95 users in test data, got %d", userCount)
	}
}

func TestQueriesIntegration_TrailingSemicolonNormalization(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Test that trailing semicolon is stripped when testing a query
	testBody := TestQueryRequest{
		SQLQuery: "SELECT 1 AS result;", // Note the trailing semicolon
		Limit:    10,
	}

	testReq, testCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("dsid", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)
	testCleanup()

	if testRec.Code != http.StatusOK {
		t.Fatalf("Test query with semicolon failed with status %d: %s", testRec.Code, testRec.Body.String())
	}

	var testResp ApiResponse
	if err := json.Unmarshal(testRec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("Failed to parse test response: %v", err)
	}

	if !testResp.Success {
		t.Errorf("Expected test to succeed, got error: %s", testResp.Error)
	}

	// Create a query with trailing semicolon - it should be normalized on save
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Return the number 1",
		SQLQuery:              "SELECT 1 AS result;", // Note the trailing semicolon
		IsEnabled:             true,
		OutputColumns:         []models.OutputColumn{{Name: "result", Type: "INT4"}},
	}

	createReq, createCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("dsid", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	createCleanup()

	if createRec.Code != http.StatusCreated {
		t.Fatalf("Create query failed with status %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp ApiResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	createData := createResp.Data.(map[string]any)

	// Verify the semicolon was stripped
	if createData["sql_query"] != "SELECT 1 AS result" {
		t.Errorf("Expected sql_query 'SELECT 1 AS result' (without semicolon), got %v", createData["sql_query"])
	}
}

func TestQueriesIntegration_RejectMultipleStatements(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Test that multiple statements are rejected
	testBody := TestQueryRequest{
		SQLQuery: "SELECT 1; SELECT 2", // Multiple statements
		Limit:    10,
	}

	testReq, testCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("dsid", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)
	testCleanup()

	// Should return 400 Bad Request for multiple statements
	if testRec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for multiple statements, got %d: %s", testRec.Code, testRec.Body.String())
	}

	var testResp ApiResponse
	if err := json.Unmarshal(testRec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("Failed to parse test response: %v", err)
	}

	if testResp.Success {
		t.Error("Expected test to fail for multiple statements")
	}

	if testResp.Error == "" {
		t.Error("Expected error message for multiple statements")
	}

	// Also verify Create rejects multiple statements
	createBody := CreateQueryRequest{
		NaturalLanguagePrompt: "Bad query",
		SQLQuery:              "SELECT 1; DROP TABLE users",
		IsEnabled:             true,
		OutputColumns:         []models.OutputColumn{{Name: "col", Type: "INT4"}},
	}

	createReq, createCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("dsid", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)
	createCleanup()

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for multiple statements on create, got %d: %s", createRec.Code, createRec.Body.String())
	}

	// Verify Validate also rejects multiple statements
	validateBody := ValidateQueryRequest{
		SQLQuery: "SELECT 1; SELECT 2; SELECT 3",
	}

	validateReq, validateCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate",
		validateBody)
	validateReq.SetPathValue("pid", tc.projectID.String())
	validateReq.SetPathValue("dsid", tc.createdDsID.String())

	validateRec := httptest.NewRecorder()
	tc.queriesHandler.Validate(validateRec, validateReq)
	validateCleanup()

	// Validate endpoint returns 200 with valid=false, not 400
	if validateRec.Code != http.StatusOK {
		t.Fatalf("Validate failed with status %d: %s", validateRec.Code, validateRec.Body.String())
	}

	var validateResp ApiResponse
	if err := json.Unmarshal(validateRec.Body.Bytes(), &validateResp); err != nil {
		t.Fatalf("Failed to parse validate response: %v", err)
	}

	validateData := validateResp.Data.(map[string]any)
	if validateData["valid"] != false {
		t.Errorf("Expected valid=false for multiple statements, got %v", validateData["valid"])
	}
}

func TestQueriesIntegration_ListQueries(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Create a few queries
	queries := []CreateQueryRequest{
		{NaturalLanguagePrompt: "Query 1", SQLQuery: "SELECT 1", IsEnabled: true, OutputColumns: []models.OutputColumn{{Name: "col", Type: "INT4"}}},
		{NaturalLanguagePrompt: "Query 2", SQLQuery: "SELECT 2", IsEnabled: true, OutputColumns: []models.OutputColumn{{Name: "col", Type: "INT4"}}},
		{NaturalLanguagePrompt: "Query 3", SQLQuery: "SELECT 3", IsEnabled: false, OutputColumns: []models.OutputColumn{{Name: "col", Type: "INT4"}}},
	}

	for _, q := range queries {
		req, cleanup := tc.makeRequest(http.MethodPost,
			"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
			q)
		req.SetPathValue("pid", tc.projectID.String())
		req.SetPathValue("dsid", tc.createdDsID.String())

		rec := httptest.NewRecorder()
		tc.queriesHandler.Create(rec, req)
		cleanup()

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create query failed with status %d: %s", rec.Code, rec.Body.String())
		}
	}

	// List all queries
	listReq, listCleanup := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		nil)
	listReq.SetPathValue("pid", tc.projectID.String())
	listReq.SetPathValue("dsid", tc.createdDsID.String())

	listRec := httptest.NewRecorder()
	tc.queriesHandler.List(listRec, listReq)
	listCleanup()

	if listRec.Code != http.StatusOK {
		t.Fatalf("List queries failed with status %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp ApiResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	listData := listResp.Data.(map[string]any)
	queriesResult := listData["queries"].([]any)

	if len(queriesResult) != 3 {
		t.Errorf("Expected 3 queries, got %d", len(queriesResult))
	}

	// List only enabled queries
	enabledReq, enabledCleanup := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/enabled",
		nil)
	enabledReq.SetPathValue("pid", tc.projectID.String())
	enabledReq.SetPathValue("dsid", tc.createdDsID.String())

	enabledRec := httptest.NewRecorder()
	tc.queriesHandler.ListEnabled(enabledRec, enabledReq)
	enabledCleanup()

	if enabledRec.Code != http.StatusOK {
		t.Fatalf("List enabled queries failed with status %d: %s", enabledRec.Code, enabledRec.Body.String())
	}

	var enabledResp ApiResponse
	if err := json.Unmarshal(enabledRec.Body.Bytes(), &enabledResp); err != nil {
		t.Fatalf("Failed to parse enabled response: %v", err)
	}

	enabledData := enabledResp.Data.(map[string]any)
	enabledQueries := enabledData["queries"].([]any)

	if len(enabledQueries) != 2 {
		t.Errorf("Expected 2 enabled queries, got %d", len(enabledQueries))
	}
}

func TestQueriesIntegration_ListPending(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	// Insert pending queries directly into the database
	// (We need to set status='pending' which isn't possible via normal Create API)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_queries (
			id, project_id, datasource_id, natural_language_prompt, sql_query,
			dialect, is_enabled, status, suggested_by, parameters, output_columns, tags
		) VALUES
		($1, $2, $3, 'Pending Query 1', 'SELECT 1', 'postgres', false, 'pending', 'agent', '[]'::jsonb, '[{"name":"col","type":"INT4","description":""}]'::jsonb, '{}'::text[]),
		($4, $2, $3, 'Pending Query 2', 'SELECT 2', 'postgres', false, 'pending', 'user', '[]'::jsonb, '[{"name":"col","type":"INT4","description":""}]'::jsonb, '{}'::text[]),
		($5, $2, $3, 'Approved Query', 'SELECT 3', 'postgres', true, 'approved', 'admin', '[]'::jsonb, '[{"name":"col","type":"INT4","description":""}]'::jsonb, '{}'::text[])
	`, uuid.New(), tc.projectID, tc.createdDsID, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("Failed to insert test queries: %v", err)
	}

	// List pending queries
	listReq, listCleanup := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/queries/pending",
		nil)
	listReq.SetPathValue("pid", tc.projectID.String())

	listRec := httptest.NewRecorder()
	tc.queriesHandler.ListPending(listRec, listReq)
	listCleanup()

	if listRec.Code != http.StatusOK {
		t.Fatalf("ListPending failed with status %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp ApiResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	if !listResp.Success {
		t.Fatalf("Expected success, got error: %s", listResp.Error)
	}

	listData := listResp.Data.(map[string]any)

	// Verify count field
	count := int(listData["count"].(float64))
	if count != 2 {
		t.Errorf("Expected count=2, got %d", count)
	}

	// Verify queries array
	pendingQueries := listData["queries"].([]any)
	if len(pendingQueries) != 2 {
		t.Errorf("Expected 2 pending queries, got %d", len(pendingQueries))
	}

	// Verify all queries have pending status
	for _, q := range pendingQueries {
		query := q.(map[string]any)
		if query["status"] != "pending" {
			t.Errorf("Expected status 'pending', got '%v'", query["status"])
		}
	}
}

func TestQueriesIntegration_ListPending_Empty(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// List pending queries (should be empty)
	listReq, listCleanup := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/queries/pending",
		nil)
	listReq.SetPathValue("pid", tc.projectID.String())

	listRec := httptest.NewRecorder()
	tc.queriesHandler.ListPending(listRec, listReq)
	listCleanup()

	if listRec.Code != http.StatusOK {
		t.Fatalf("ListPending failed with status %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp ApiResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	if !listResp.Success {
		t.Fatalf("Expected success, got error: %s", listResp.Error)
	}

	listData := listResp.Data.(map[string]any)

	// Verify count is 0
	count := int(listData["count"].(float64))
	if count != 0 {
		t.Errorf("Expected count=0, got %d", count)
	}

	// Verify queries array is empty
	pendingQueries := listData["queries"].([]any)
	if len(pendingQueries) != 0 {
		t.Errorf("Expected 0 pending queries, got %d", len(pendingQueries))
	}
}

func TestQueriesIntegration_ApproveNewQuery(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	// Insert a pending query directly into the database
	queryID := uuid.New()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_queries (
			id, project_id, datasource_id, natural_language_prompt, sql_query,
			dialect, is_enabled, status, suggested_by, parameters, output_columns, tags
		) VALUES ($1, $2, $3, 'Pending Query', 'SELECT 1', 'postgres', false, 'pending', 'agent', '[]'::jsonb, '[{"name":"col","type":"INT4","description":""}]'::jsonb, '{}'::text[])
	`, queryID, tc.projectID, tc.createdDsID)
	if err != nil {
		t.Fatalf("Failed to insert test query: %v", err)
	}

	// Approve the query
	approveReq, approveCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/queries/"+queryID.String()+"/approve",
		nil)
	approveReq.SetPathValue("pid", tc.projectID.String())
	approveReq.SetPathValue("qid", queryID.String())

	approveRec := httptest.NewRecorder()
	tc.queriesHandler.Approve(approveRec, approveReq)
	approveCleanup()

	if approveRec.Code != http.StatusOK {
		t.Fatalf("Approve failed with status %d: %s", approveRec.Code, approveRec.Body.String())
	}

	var approveResp ApiResponse
	if err := json.Unmarshal(approveRec.Body.Bytes(), &approveResp); err != nil {
		t.Fatalf("Failed to parse approve response: %v", err)
	}

	if !approveResp.Success {
		t.Fatalf("Expected success, got error: %s", approveResp.Error)
	}

	approveData := approveResp.Data.(map[string]any)

	// Verify success message
	if approveData["success"] != true {
		t.Errorf("Expected success=true, got %v", approveData["success"])
	}

	if approveData["message"] != "Query approved and enabled" {
		t.Errorf("Expected message 'Query approved and enabled', got '%v'", approveData["message"])
	}

	// Verify query is returned
	query := approveData["query"].(map[string]any)
	if query["query_id"] != queryID.String() {
		t.Errorf("Expected query_id=%s, got %v", queryID.String(), query["query_id"])
	}

	// Verify query is now enabled by checking the database
	var isEnabled bool
	var status string
	err = scope.Conn.QueryRow(ctx, "SELECT is_enabled, status FROM engine_queries WHERE id = $1", queryID).Scan(&isEnabled, &status)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if !isEnabled {
		t.Error("Expected query to be enabled after approval")
	}
	if status != "approved" {
		t.Errorf("Expected status='approved', got '%s'", status)
	}
}

func TestQueriesIntegration_ApproveNotFound(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	// Try to approve a non-existent query
	nonExistentID := uuid.New()
	approveReq, approveCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/queries/"+nonExistentID.String()+"/approve",
		nil)
	approveReq.SetPathValue("pid", tc.projectID.String())
	approveReq.SetPathValue("qid", nonExistentID.String())

	approveRec := httptest.NewRecorder()
	tc.queriesHandler.Approve(approveRec, approveReq)
	approveCleanup()

	if approveRec.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d: %s", approveRec.Code, approveRec.Body.String())
	}

	var approveResp ApiResponse
	if err := json.Unmarshal(approveRec.Body.Bytes(), &approveResp); err != nil {
		t.Fatalf("Failed to parse approve response: %v", err)
	}

	if approveResp.Success {
		t.Error("Expected success=false for non-existent query")
	}
}

func TestQueriesIntegration_ApproveAlreadyApproved(t *testing.T) {
	tc := setupQueriesIntegrationTest(t)
	tc.ensureTestDatasource()
	tc.cleanupQueries()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	// Insert an already approved query
	queryID := uuid.New()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_queries (
			id, project_id, datasource_id, natural_language_prompt, sql_query,
			dialect, is_enabled, status, suggested_by, parameters, output_columns, tags
		) VALUES ($1, $2, $3, 'Approved Query', 'SELECT 1', 'postgres', true, 'approved', 'admin', '[]'::jsonb, '[{"name":"col","type":"INT4","description":""}]'::jsonb, '{}'::text[])
	`, queryID, tc.projectID, tc.createdDsID)
	if err != nil {
		t.Fatalf("Failed to insert test query: %v", err)
	}

	// Try to approve it again
	approveReq, approveCleanup := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/queries/"+queryID.String()+"/approve",
		nil)
	approveReq.SetPathValue("pid", tc.projectID.String())
	approveReq.SetPathValue("qid", queryID.String())

	approveRec := httptest.NewRecorder()
	tc.queriesHandler.Approve(approveRec, approveReq)
	approveCleanup()

	// Should return 400 Bad Request since query is not pending
	if approveRec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", approveRec.Code, approveRec.Body.String())
	}

	var approveResp ApiResponse
	if err := json.Unmarshal(approveRec.Body.Bytes(), &approveResp); err != nil {
		t.Fatalf("Failed to parse approve response: %v", err)
	}

	if approveResp.Success {
		t.Error("Expected success=false for already approved query")
	}
}

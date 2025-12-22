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
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
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
	adapterFactory := datasource.NewDatasourceAdapterFactory()

	// Create datasource repository and service
	dsRepo := repositories.NewDatasourceRepository()
	dsSvc := services.NewDatasourceService(dsRepo, encryptor, adapterFactory, nil, zap.NewNop())

	// Create query repository and service
	queryRepo := repositories.NewQueryRepository()
	querySvc := services.NewQueryService(queryRepo, dsSvc, adapterFactory, zap.NewNop())

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
func (tc *queriesIntegrationTestContext) makeRequest(method, path string, body any) *http.Request {
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

	ds, err := tc.datasourcesSvc.Create(database.SetTenantScope(ctx, scope), tc.projectID, "Test Data DB", "postgres", dsConfig)
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

	testReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("did", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)

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
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)

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
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)

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

	execReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/"+queryID+"/execute",
		execBody)
	execReq.SetPathValue("pid", tc.projectID.String())
	execReq.SetPathValue("did", tc.createdDsID.String())
	execReq.SetPathValue("qid", queryID)

	execRec := httptest.NewRecorder()
	tc.queriesHandler.Execute(execRec, execReq)

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
	if len(columns) != 1 || columns[0] != "sum" {
		t.Errorf("Expected columns ['sum'], got %v", columns)
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

	validReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate",
		validBody)
	validReq.SetPathValue("pid", tc.projectID.String())
	validReq.SetPathValue("did", tc.createdDsID.String())

	validRec := httptest.NewRecorder()
	tc.queriesHandler.Validate(validRec, validReq)

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

	invalidReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate",
		invalidBody)
	invalidReq.SetPathValue("pid", tc.projectID.String())
	invalidReq.SetPathValue("did", tc.createdDsID.String())

	invalidRec := httptest.NewRecorder()
	tc.queriesHandler.Validate(invalidRec, invalidReq)

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

	testReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("did", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)

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

	testReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("did", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)

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
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)

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

	testReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/test",
		testBody)
	testReq.SetPathValue("pid", tc.projectID.String())
	testReq.SetPathValue("did", tc.createdDsID.String())

	testRec := httptest.NewRecorder()
	tc.queriesHandler.Test(testRec, testReq)

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
	}

	createReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		createBody)
	createReq.SetPathValue("pid", tc.projectID.String())
	createReq.SetPathValue("did", tc.createdDsID.String())

	createRec := httptest.NewRecorder()
	tc.queriesHandler.Create(createRec, createReq)

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for multiple statements on create, got %d: %s", createRec.Code, createRec.Body.String())
	}

	// Verify Validate also rejects multiple statements
	validateBody := ValidateQueryRequest{
		SQLQuery: "SELECT 1; SELECT 2; SELECT 3",
	}

	validateReq := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/validate",
		validateBody)
	validateReq.SetPathValue("pid", tc.projectID.String())
	validateReq.SetPathValue("did", tc.createdDsID.String())

	validateRec := httptest.NewRecorder()
	tc.queriesHandler.Validate(validateRec, validateReq)

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
		{NaturalLanguagePrompt: "Query 1", SQLQuery: "SELECT 1", IsEnabled: true},
		{NaturalLanguagePrompt: "Query 2", SQLQuery: "SELECT 2", IsEnabled: true},
		{NaturalLanguagePrompt: "Query 3", SQLQuery: "SELECT 3", IsEnabled: false},
	}

	for _, q := range queries {
		req := tc.makeRequest(http.MethodPost,
			"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
			q)
		req.SetPathValue("pid", tc.projectID.String())
		req.SetPathValue("did", tc.createdDsID.String())

		rec := httptest.NewRecorder()
		tc.queriesHandler.Create(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("Create query failed with status %d: %s", rec.Code, rec.Body.String())
		}
	}

	// List all queries
	listReq := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries",
		nil)
	listReq.SetPathValue("pid", tc.projectID.String())
	listReq.SetPathValue("did", tc.createdDsID.String())

	listRec := httptest.NewRecorder()
	tc.queriesHandler.List(listRec, listReq)

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
	enabledReq := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/datasources/"+tc.createdDsID.String()+"/queries/enabled",
		nil)
	enabledReq.SetPathValue("pid", tc.projectID.String())
	enabledReq.SetPathValue("did", tc.createdDsID.String())

	enabledRec := httptest.NewRecorder()
	tc.queriesHandler.ListEnabled(enabledRec, enabledReq)

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

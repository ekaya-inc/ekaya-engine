//go:build ignore

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// glossaryTestContext holds all dependencies for glossary integration tests.
type glossaryTestContext struct {
	t              *testing.T
	testDB         *testhelpers.TestDB
	engineDB       *testhelpers.EngineDB
	handler        *GlossaryHandler
	service        services.GlossaryService
	datasourceSvc  services.DatasourceService
	glossaryRepo   repositories.GlossaryRepository
	ontologyRepo   repositories.OntologyRepository
	projectID      uuid.UUID
	ontologyID     uuid.UUID
	datasourceID   uuid.UUID
	adapterFactory datasource.DatasourceAdapterFactory
}

// setupGlossaryTest creates a test context with real database and services.
func setupGlossaryTest(t *testing.T) *glossaryTestContext {
	t.Helper()

	// Get both databases from the test container
	testDB := testhelpers.GetTestDB(t)
	engineDB := testhelpers.GetEngineDB(t)

	// Create encryptor for datasource credentials
	encryptor, err := crypto.NewCredentialEncryptor(testEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// Create the real adapter factory (not a mock) for SQL validation
	adapterFactory := datasource.NewDatasourceAdapterFactory(nil)

	// Create repositories
	glossaryRepo := repositories.NewGlossaryRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	dsRepo := repositories.NewDatasourceRepository()

	// Create datasource service
	datasourceSvc := services.NewDatasourceService(dsRepo, ontologyRepo, encryptor, adapterFactory, nil, zap.NewNop())

	// Create a mock LLM factory that returns nil (SuggestTerms needs ontology first)
	mockLLMFactory := &mockLLMClientFactory{}

	// Create service with real dependencies
	service := services.NewGlossaryService(glossaryRepo, ontologyRepo, nil, nil, datasourceSvc, adapterFactory, mockLLMFactory, nil, zap.NewNop(), "test")

	// Create handler
	questionService := services.NewOntologyQuestionService(
		repositories.NewOntologyQuestionRepository(), ontologyRepo, nil,
		nil, zap.NewNop())
	handler := NewGlossaryHandler(service, questionService, zap.NewNop())

	// Use a unique project ID for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	return &glossaryTestContext{
		t:              t,
		testDB:         testDB,
		engineDB:       engineDB,
		handler:        handler,
		service:        service,
		datasourceSvc:  datasourceSvc,
		glossaryRepo:   glossaryRepo,
		ontologyRepo:   ontologyRepo,
		projectID:      projectID,
		adapterFactory: adapterFactory,
	}
}

// mockLLMClientFactory is a mock LLM factory for testing.
type mockLLMClientFactory struct{}

func (f *mockLLMClientFactory) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return &mockLLMClient{}, nil
}

func (f *mockLLMClientFactory) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return &mockLLMClient{}, nil
}

func (f *mockLLMClientFactory) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

// mockLLMClient returns canned responses for testing.
type mockLLMClient struct{}

func (c *mockLLMClient) GenerateResponse(_ context.Context, _ string, _ string, _ float64, _ bool) (*llm.GenerateResponseResult, error) {
	// Return a valid JSON object with suggested terms (discovery phase - no SQL)
	return &llm.GenerateResponseResult{
		Content: `{"terms": [
			{
				"term": "Revenue",
				"definition": "Total earned amount from completed transactions",
				"aliases": ["Total Revenue", "Gross Revenue"]
			}
		]}`,
		PromptTokens:     100,
		CompletionTokens: 200,
	}, nil
}

func (c *mockLLMClient) CreateEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (c *mockLLMClient) CreateEmbeddings(_ context.Context, _ []string, _ string) ([][]float32, error) {
	return nil, nil
}

func (c *mockLLMClient) GetModel() string {
	return "test-model"
}

func (c *mockLLMClient) GetEndpoint() string {
	return "https://test.endpoint"
}

// doRequest creates an HTTP request with proper context and executes the handler.
func (tc *glossaryTestContext) doRequest(method, path string, body any, handler http.HandlerFunc, pathValues map[string]string) *httptest.ResponseRecorder {
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

	// Set path values
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}

	// Set up tenant scope
	ctx := req.Context()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	ctx = database.SetTenantScope(ctx, scope)

	// Set up auth claims with user ID (Subject) for connection pooling
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "test-user-glossary",
		},
		ProjectID: tc.projectID.String(),
	}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	// Add provenance context for write operations (simulates what auth middleware does)
	// Using uuid.Nil since we don't have a real user - the repository handles nil UUIDs
	ctx = models.WithManualProvenance(ctx, uuid.Nil)

	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler(rec, req)

	return rec
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *glossaryTestContext) ensureTestProject() {
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
	`, tc.projectID, "Glossary Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestOntology creates an active ontology for the test project.
func (tc *glossaryTestContext) ensureTestOntology() {
	tc.t.Helper()

	// Generate a stable ontology ID for this test context
	tc.ontologyID = uuid.MustParse("00000000-0000-0000-0000-000000000103")

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	// Use ON CONFLICT on the unique constraint (project_id, version) - version defaults to 1
	// Don't change the ID if an ontology already exists (would violate FK constraints)
	err = scope.Conn.QueryRow(ctx, `
		INSERT INTO engine_ontologies (id, project_id, is_active, domain_summary, entity_summaries, column_details)
		VALUES ($1, $2, true, '{}', '{}', '{}')
		ON CONFLICT (project_id, version) DO UPDATE SET is_active = true
		RETURNING id
	`, tc.ontologyID, tc.projectID).Scan(&tc.ontologyID)
	if err != nil {
		tc.t.Fatalf("Failed to ensure test ontology: %v", err)
	}
}

// ensureTestDatasource creates a datasource pointing to the test_data database.
func (tc *glossaryTestContext) ensureTestDatasource() {
	tc.t.Helper()

	tc.ensureTestProject()
	tc.ensureTestOntology()

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

	ds, err := tc.datasourceSvc.Create(database.SetTenantScope(ctx, scope), tc.projectID, "Test Data DB", "postgres", "postgres", dsConfig)
	if err != nil {
		tc.t.Fatalf("Failed to create test datasource: %v", err)
	}

	tc.datasourceID = ds.ID
}

// createTestOntology creates an active ontology for testing.
func (tc *glossaryTestContext) createTestOntology() {
	tc.t.Helper()

	tc.ensureTestProject()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)

	tc.ontologyID = uuid.New()
	ontology := &models.TieredOntology{
		ID:            tc.ontologyID,
		ProjectID:     tc.projectID,
		Version:       1,
		IsActive:      true,
		ColumnDetails: make(map[string][]models.ColumnDetail),
		Metadata:      make(map[string]any),
	}

	if err := tc.ontologyRepo.Create(ctx, ontology); err != nil {
		tc.t.Fatalf("Failed to create ontology: %v", err)
	}
}

// cleanup removes test data.
func (tc *glossaryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in order respecting foreign keys
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_glossary_aliases WHERE glossary_id IN (SELECT id FROM engine_business_glossary WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_business_glossary WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE project_id = $1", tc.projectID)
}

// ============================================================================
// Integration Tests - New Schema
// ============================================================================

func TestGlossaryIntegration_TestSQL_Valid(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	// Test with valid SQL (using users table that exists in test_data)
	req := TestSQLRequest{
		SQL: "SELECT id, COUNT(*) as total FROM users GROUP BY id LIMIT 1",
	}

	rec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary/test-sql",
		req, tc.handler.TestSQL, map[string]string{"pid": tc.projectID.String()})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[TestSQLResponse](t, rec.Body.Bytes())

	if !resp.Valid {
		t.Errorf("expected valid=true, got false with error: %s", resp.Error)
	}

	if len(resp.OutputColumns) == 0 {
		t.Error("expected output columns to be captured")
	}

	// Verify output columns have names and types
	for _, col := range resp.OutputColumns {
		if col.Name == "" {
			t.Error("expected column name to be populated")
		}
		if col.Type == "" {
			t.Error("expected column type to be populated")
		}
	}
}

func TestGlossaryIntegration_TestSQL_Invalid(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	// Test with invalid SQL (syntax error)
	req := TestSQLRequest{
		SQL: "SELECT * FROM nonexistent_table_xyz",
	}

	rec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary/test-sql",
		req, tc.handler.TestSQL, map[string]string{"pid": tc.projectID.String()})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[TestSQLResponse](t, rec.Body.Bytes())

	if resp.Valid {
		t.Error("expected valid=false for invalid SQL")
	}

	if resp.Error == "" {
		t.Error("expected error message for invalid SQL")
	}
}

func TestGlossaryIntegration_TestSQL_NoDatasource(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()
	// Don't create datasource

	req := TestSQLRequest{
		SQL: "SELECT 1",
	}

	rec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary/test-sql",
		req, tc.handler.TestSQL, map[string]string{"pid": tc.projectID.String()})

	// TestSQL returns 200 with valid=false when no datasource configured (structured error response)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[TestSQLResponse](t, rec.Body.Bytes())
	if resp.Valid {
		t.Error("expected valid=false when no datasource configured")
	}
	if resp.Error == "" {
		t.Error("expected error message when no datasource configured")
	}
}

func TestGlossaryIntegration_CreateWithValidation(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	createReq := CreateGlossaryTermRequest{
		Term:        "Total Users",
		Definition:  "Count of all users in the system",
		DefiningSQL: "SELECT COUNT(*) AS total_users FROM users",
		BaseTable:   "users",
		Aliases:     []string{"User Count", "All Users"},
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())
	if createResp.Term != "Total Users" {
		t.Errorf("expected term 'Total Users', got %q", createResp.Term)
	}
	if createResp.Source != models.GlossarySourceManual {
		t.Errorf("expected source 'manual', got %q", createResp.Source)
	}
	if createResp.DefiningSQL != createReq.DefiningSQL {
		t.Errorf("expected defining_sql to match request")
	}
	if len(createResp.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(createResp.Aliases))
	}
	if len(createResp.OutputColumns) == 0 {
		t.Error("expected output columns to be captured after validation")
	}
}

func TestGlossaryIntegration_CreateWithInvalidSQL(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	createReq := CreateGlossaryTermRequest{
		Term:        "Invalid Term",
		Definition:  "This should fail",
		DefiningSQL: "SELECT * FROM table_that_does_not_exist",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", createRec.Code, createRec.Body.String())
	}
}

func TestGlossaryIntegration_CreateDuplicate(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.createTestOntology() // Create ontology so terms get ontology_id for unique constraint
	tc.ensureTestDatasource()

	createReq := CreateGlossaryTermRequest{
		Term:        "Revenue",
		Definition:  "Total revenue",
		DefiningSQL: "SELECT SUM(id) AS revenue FROM users",
	}

	// Create first term
	createRec1 := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec1.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", createRec1.Code, createRec1.Body.String())
	}

	// Try to create duplicate
	createRec2 := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	// Database constraint error returns 500 - that's acceptable for constraint violations
	// In a future enhancement, we could add explicit duplicate checking in the service layer
	if createRec2.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 (database constraint error), got %d: %s", createRec2.Code, createRec2.Body.String())
	}

	// Verify it's the constraint error we expect
	if !strings.Contains(createRec2.Body.String(), "duplicate key value") {
		t.Errorf("expected duplicate key error, got: %s", createRec2.Body.String())
	}
}

func TestGlossaryIntegration_UpdateWithSQLChange(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	// Create term first
	createReq := CreateGlossaryTermRequest{
		Term:        "Active Users",
		Definition:  "Users who logged in",
		DefiningSQL: "SELECT COUNT(*) AS active_users FROM users",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())

	// Update WITHOUT changing SQL (no re-validation needed)
	updateReq := UpdateGlossaryTermRequest{
		Term:        "Active Users",
		Definition:  "Users who have logged in at least once",
		DefiningSQL: "SELECT COUNT(*) AS active_users FROM users", // Same SQL, no revalidation
		BaseTable:   "users",
		Aliases:     []string{"MAU", "Monthly Active Users"},
	}

	updateRec := tc.doRequest(http.MethodPut, "/api/projects/"+tc.projectID.String()+"/glossary/"+createResp.ID.String(),
		updateReq, tc.handler.Update, map[string]string{"pid": tc.projectID.String(), "tid": createResp.ID.String()})

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	updateResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, updateRec.Body.Bytes())
	if updateResp.Definition != updateReq.Definition {
		t.Errorf("expected updated definition, got %q", updateResp.Definition)
	}
	if updateResp.DefiningSQL != updateReq.DefiningSQL {
		t.Errorf("expected updated defining_sql, got %q", updateResp.DefiningSQL)
	}
	if len(updateResp.Aliases) != 2 {
		t.Errorf("expected 2 aliases after update, got %d", len(updateResp.Aliases))
	}
	// Output columns should still be present from initial creation
	if len(updateResp.OutputColumns) == 0 {
		t.Error("expected output columns to be present")
	}
}

func TestGlossaryIntegration_UpdateWithInvalidSQL(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	// Create term first
	createReq := CreateGlossaryTermRequest{
		Term:        "Test Term",
		Definition:  "Test definition",
		DefiningSQL: "SELECT COUNT(*) FROM users",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())

	// Try to update with invalid SQL
	updateReq := UpdateGlossaryTermRequest{
		Term:        "Test Term",
		Definition:  "Test definition",
		DefiningSQL: "SELECT * FROM invalid_table_name",
	}

	updateRec := tc.doRequest(http.MethodPut, "/api/projects/"+tc.projectID.String()+"/glossary/"+createResp.ID.String(),
		updateReq, tc.handler.Update, map[string]string{"pid": tc.projectID.String(), "tid": createResp.ID.String()})

	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
}

func TestGlossaryIntegration_DeleteCascadesToAliases(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	// Create term with aliases
	createReq := CreateGlossaryTermRequest{
		Term:        "GMV",
		Definition:  "Gross Merchandise Value",
		DefiningSQL: "SELECT SUM(id) AS gmv FROM users",
		Aliases:     []string{"Gross Revenue", "Total GMV"},
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())

	// Verify aliases exist in database
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()

	var aliasCount int
	err = scope.Conn.QueryRow(ctx, "SELECT COUNT(*) FROM engine_glossary_aliases WHERE glossary_id = $1", createResp.ID).Scan(&aliasCount)
	if err != nil {
		t.Fatalf("Failed to query aliases: %v", err)
	}
	if aliasCount != 2 {
		t.Errorf("expected 2 aliases in database, got %d", aliasCount)
	}

	// Delete term
	deleteRec := tc.doRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/glossary/"+createResp.ID.String(),
		nil, tc.handler.Delete, map[string]string{"pid": tc.projectID.String(), "tid": createResp.ID.String()})

	// The handler returns 200 with a success response, not 204
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	// Verify aliases were cascaded
	err = scope.Conn.QueryRow(ctx, "SELECT COUNT(*) FROM engine_glossary_aliases WHERE glossary_id = $1", createResp.ID).Scan(&aliasCount)
	if err != nil {
		t.Fatalf("Failed to query aliases after delete: %v", err)
	}
	if aliasCount != 0 {
		t.Errorf("expected 0 aliases after cascade delete, got %d", aliasCount)
	}
}

func TestGlossaryIntegration_ListWithAliases(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestDatasource()

	// Create multiple terms with aliases
	terms := []CreateGlossaryTermRequest{
		{
			Term:        "Revenue",
			Definition:  "Total revenue",
			DefiningSQL: "SELECT SUM(id) AS revenue FROM users",
			Aliases:     []string{"Total Revenue", "Gross Revenue"},
		},
		{
			Term:        "Active Users",
			Definition:  "Monthly active users",
			DefiningSQL: "SELECT COUNT(DISTINCT id) AS active_users FROM users",
			Aliases:     []string{"MAU"},
		},
	}

	for _, req := range terms {
		rec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
			req, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})
		if rec.Code != http.StatusCreated {
			t.Fatalf("failed to create term %q: %d - %s", req.Term, rec.Code, rec.Body.String())
		}
	}

	// List all terms
	listRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	listResp := parseAPIResponse[GlossaryListResponse](t, listRec.Body.Bytes())
	if listResp.Total != 2 {
		t.Fatalf("expected 2 terms, got %d", listResp.Total)
	}

	// Verify aliases are included
	for _, term := range listResp.Terms {
		if len(term.Aliases) == 0 {
			t.Errorf("expected term %q to have aliases", term.Term)
		}
	}
}

func TestGlossaryIntegration_GetByID_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	nonExistentID := uuid.New()
	getRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary/"+nonExistentID.String(),
		nil, tc.handler.Get, map[string]string{"pid": tc.projectID.String(), "tid": nonExistentID.String()})

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", getRec.Code, getRec.Body.String())
	}
}

func TestGlossaryIntegration_Update_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	nonExistentID := uuid.New()
	updateReq := UpdateGlossaryTermRequest{
		Term:        "Test",
		Definition:  "Test",
		DefiningSQL: "SELECT 1",
	}

	updateRec := tc.doRequest(http.MethodPut, "/api/projects/"+tc.projectID.String()+"/glossary/"+nonExistentID.String(),
		updateReq, tc.handler.Update, map[string]string{"pid": tc.projectID.String(), "tid": nonExistentID.String()})

	if updateRec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
}

func TestGlossaryIntegration_Delete_NotFound(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	nonExistentID := uuid.New()
	deleteRec := tc.doRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/glossary/"+nonExistentID.String(),
		nil, tc.handler.Delete, map[string]string{"pid": tc.projectID.String(), "tid": nonExistentID.String()})

	if deleteRec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestGlossaryIntegration_Create_ValidationError(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	// Missing term name
	createReq := CreateGlossaryTermRequest{
		Definition:  "Some definition",
		DefiningSQL: "SELECT 1",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", createRec.Code, createRec.Body.String())
	}

	// Missing definition
	createReq2 := CreateGlossaryTermRequest{
		Term:        "Some Term",
		DefiningSQL: "SELECT 1",
	}

	createRec2 := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq2, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec2.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", createRec2.Code, createRec2.Body.String())
	}

	// Missing defining_sql
	createReq3 := CreateGlossaryTermRequest{
		Term:       "Some Term",
		Definition: "Some definition",
	}

	createRec3 := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq3, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec3.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", createRec3.Code, createRec3.Body.String())
	}
}

func TestGlossaryIntegration_Suggest(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.createTestOntology()
	tc.ensureTestDatasource() // Need datasource for SQL validation

	suggestRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary/suggest",
		nil, tc.handler.Suggest, map[string]string{"pid": tc.projectID.String()})

	if suggestRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", suggestRec.Code, suggestRec.Body.String())
	}

	suggestResp := parseAPIResponse[GlossaryListResponse](t, suggestRec.Body.Bytes())
	if suggestResp.Total != 1 {
		t.Fatalf("expected 1 suggested term, got %d", suggestResp.Total)
	}

	term := suggestResp.Terms[0]
	if term.Term != "Revenue" {
		t.Errorf("expected term 'Revenue', got %q", term.Term)
	}
	if term.Source != models.GlossarySourceInferred {
		t.Errorf("expected source 'inferred', got %q", term.Source)
	}
	// In two-phase workflow, DefiningSQL is empty from SuggestTerms (filled in enrichment)
	if term.DefiningSQL != "" {
		t.Errorf("expected defining_sql to be empty (filled in enrichment phase), got %q", term.DefiningSQL)
	}
	if len(term.Aliases) == 0 {
		t.Error("expected aliases to be populated")
	}
}

func TestGlossaryIntegration_Suggest_NoOntology(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()
	// Don't create ontology

	suggestRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary/suggest",
		nil, tc.handler.Suggest, map[string]string{"pid": tc.projectID.String()})

	if suggestRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", suggestRec.Code, suggestRec.Body.String())
	}
}

func TestGlossaryIntegration_InvalidTermID(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	getRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary/not-a-uuid",
		nil, tc.handler.Get, map[string]string{"pid": tc.projectID.String(), "tid": "not-a-uuid"})

	if getRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", getRec.Code, getRec.Body.String())
	}
}

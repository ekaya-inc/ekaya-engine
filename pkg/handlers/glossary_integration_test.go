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

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// glossaryTestContext holds all dependencies for glossary integration tests.
type glossaryTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	handler      *GlossaryHandler
	service      services.GlossaryService
	glossaryRepo repositories.GlossaryRepository
	ontologyRepo repositories.OntologyRepository
	entityRepo   repositories.OntologyEntityRepository
	projectID    uuid.UUID
	ontologyID   uuid.UUID
}

// setupGlossaryTest creates a test context with real database and services.
func setupGlossaryTest(t *testing.T) *glossaryTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	// Create repositories
	glossaryRepo := repositories.NewGlossaryRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	entityRepo := repositories.NewOntologyEntityRepository()

	// Create a mock LLM factory that returns nil (SuggestTerms needs ontology/entities first)
	mockLLMFactory := &mockLLMClientFactory{}

	// Create service
	service := services.NewGlossaryService(glossaryRepo, ontologyRepo, entityRepo, mockLLMFactory, zap.NewNop())

	// Create handler
	handler := NewGlossaryHandler(service, zap.NewNop())

	// Use a fixed project ID for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	return &glossaryTestContext{
		t:            t,
		engineDB:     engineDB,
		handler:      handler,
		service:      service,
		glossaryRepo: glossaryRepo,
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		projectID:    projectID,
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
	// Return a valid JSON array of suggested terms
	return &llm.GenerateResponseResult{
		Content: `[
			{
				"term": "Revenue",
				"definition": "Total earned amount from completed transactions",
				"sql_pattern": "SUM(earned_amount) WHERE transaction_state = 'completed'",
				"base_table": "billing_transactions",
				"columns_used": ["earned_amount", "transaction_state"],
				"aggregation": "SUM"
			}
		]`,
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

	// Set up auth claims
	claims := &auth.Claims{ProjectID: tc.projectID.String()}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

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
		ID:              tc.ontologyID,
		ProjectID:       tc.projectID,
		Version:         1,
		IsActive:        true,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
		Metadata:        make(map[string]any),
	}

	if err := tc.ontologyRepo.Create(ctx, ontology); err != nil {
		tc.t.Fatalf("Failed to create ontology: %v", err)
	}
}

// createTestEntity creates an entity for SuggestTerms testing.
func (tc *glossaryTestContext) createTestEntity() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "billing_transaction",
		Description:   "A billing transaction",
		PrimarySchema: "public",
		PrimaryTable:  "billing_transactions",
		PrimaryColumn: "id",
	}

	if err := tc.entityRepo.Create(ctx, entity); err != nil {
		tc.t.Fatalf("Failed to create entity: %v", err)
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
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_business_glossary WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_aliases WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	// Note: engine_ontology_entity_occurrences table was dropped in migration 030
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

func TestGlossaryIntegration_ListEmpty(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	rec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[GlossaryListResponse](t, rec.Body.Bytes())

	if resp.Total != 0 {
		t.Errorf("expected 0 terms, got %d", resp.Total)
	}

	if len(resp.Terms) != 0 {
		t.Errorf("expected empty terms list, got %d", len(resp.Terms))
	}
}

func TestGlossaryIntegration_CreateAndList(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	createReq := CreateGlossaryTermRequest{
		Term:        "Revenue",
		Definition:  "Total earned amount from completed transactions",
		SQLPattern:  "SUM(earned_amount) WHERE transaction_state = 'completed'",
		BaseTable:   "billing_transactions",
		ColumnsUsed: []string{"earned_amount", "transaction_state"},
		Filters: []models.Filter{
			{Column: "transaction_state", Operator: "=", Values: []string{"completed"}},
		},
		Aggregation: "SUM",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())
	if createResp.Term != "Revenue" {
		t.Errorf("expected term 'Revenue', got %q", createResp.Term)
	}
	if createResp.Source != "user" {
		t.Errorf("expected source 'user', got %q", createResp.Source)
	}
	if createResp.ID == uuid.Nil {
		t.Error("expected non-nil term ID")
	}

	// Verify via list
	listRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	listResp := parseAPIResponse[GlossaryListResponse](t, listRec.Body.Bytes())
	if listResp.Total != 1 {
		t.Fatalf("expected 1 term, got %d", listResp.Total)
	}

	term := listResp.Terms[0]
	if term.Term != "Revenue" {
		t.Errorf("expected term 'Revenue', got %q", term.Term)
	}
	if term.BaseTable != "billing_transactions" {
		t.Errorf("expected base_table 'billing_transactions', got %q", term.BaseTable)
	}
	if len(term.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(term.Filters))
	}
}

func TestGlossaryIntegration_GetByID(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	// Create term first
	createReq := CreateGlossaryTermRequest{
		Term:       "GMV",
		Definition: "Gross Merchandise Value",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())

	// Get by ID
	getRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary/"+createResp.ID.String(),
		nil, tc.handler.Get, map[string]string{"pid": tc.projectID.String(), "tid": createResp.ID.String()})

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	getResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, getRec.Body.Bytes())
	if getResp.Term != "GMV" {
		t.Errorf("expected term 'GMV', got %q", getResp.Term)
	}
	if getResp.Definition != "Gross Merchandise Value" {
		t.Errorf("expected definition 'Gross Merchandise Value', got %q", getResp.Definition)
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

func TestGlossaryIntegration_Update(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	// Create term first
	createReq := CreateGlossaryTermRequest{
		Term:       "Active User",
		Definition: "A user who logged in",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())

	// Update
	updateReq := UpdateGlossaryTermRequest{
		Term:        "Active User",
		Definition:  "A user who logged in within the last 30 days",
		SQLPattern:  "WHERE last_login_at > NOW() - INTERVAL '30 days'",
		BaseTable:   "users",
		ColumnsUsed: []string{"last_login_at"},
	}

	updateRec := tc.doRequest(http.MethodPut, "/api/projects/"+tc.projectID.String()+"/glossary/"+createResp.ID.String(),
		updateReq, tc.handler.Update, map[string]string{"pid": tc.projectID.String(), "tid": createResp.ID.String()})

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	updateResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, updateRec.Body.Bytes())
	if updateResp.Definition != "A user who logged in within the last 30 days" {
		t.Errorf("expected updated definition, got %q", updateResp.Definition)
	}
	if updateResp.SQLPattern != "WHERE last_login_at > NOW() - INTERVAL '30 days'" {
		t.Errorf("expected updated sql_pattern, got %q", updateResp.SQLPattern)
	}
	if updateResp.BaseTable != "users" {
		t.Errorf("expected base_table 'users', got %q", updateResp.BaseTable)
	}
}

func TestGlossaryIntegration_Delete(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.ensureTestProject()

	// Create term first
	createReq := CreateGlossaryTermRequest{
		Term:       "Churn Rate",
		Definition: "Percentage of users who leave",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	createResp := parseAPIResponse[*models.BusinessGlossaryTerm](t, createRec.Body.Bytes())

	// Delete
	deleteRec := tc.doRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/glossary/"+createResp.ID.String(),
		nil, tc.handler.Delete, map[string]string{"pid": tc.projectID.String(), "tid": createResp.ID.String()})

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	// Verify via list - should be empty
	listRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/glossary", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	listResp := parseAPIResponse[GlossaryListResponse](t, listRec.Body.Bytes())
	if listResp.Total != 0 {
		t.Errorf("expected 0 terms after delete, got %d", listResp.Total)
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
		Definition: "Some definition",
	}

	createRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", createRec.Code, createRec.Body.String())
	}

	// Missing definition
	createReq2 := CreateGlossaryTermRequest{
		Term: "Some Term",
	}

	createRec2 := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/glossary",
		createReq2, tc.handler.Create, map[string]string{"pid": tc.projectID.String()})

	if createRec2.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", createRec2.Code, createRec2.Body.String())
	}
}

func TestGlossaryIntegration_Suggest(t *testing.T) {
	tc := setupGlossaryTest(t)
	tc.cleanup()
	tc.createTestOntology()
	tc.createTestEntity()

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
	if term.Source != "suggested" {
		t.Errorf("expected source 'suggested', got %q", term.Source)
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

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
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// aiConfigIntegrationTestContext holds all dependencies for AI config handler integration tests.
type aiConfigIntegrationTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	handler    *AIConfigHandler
	service    services.AIConfigService
	mockTester *mockConnectionTester
	projectID  uuid.UUID
}

// mockConnectionTester is a configurable mock for ConnectionTester.
type mockConnectionTester struct {
	TestFunc func(ctx context.Context, cfg *llm.TestConfig) *llm.TestResult
}

func (m *mockConnectionTester) Test(ctx context.Context, cfg *llm.TestConfig) *llm.TestResult {
	if m.TestFunc != nil {
		return m.TestFunc(ctx, cfg)
	}
	return &llm.TestResult{
		Success:    true,
		Message:    "Mock connection successful",
		LLMSuccess: true,
		LLMMessage: "LLM OK",
	}
}

// setupAIConfigHandlerTest creates a test context with real database and handler.
func setupAIConfigHandlerTest(t *testing.T) *aiConfigIntegrationTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	encryptor, err := crypto.NewCredentialEncryptor(testEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	repo := repositories.NewAIConfigRepository(encryptor)
	service := services.NewAIConfigService(repo, nil, nil, zap.NewNop())
	mockTester := &mockConnectionTester{}

	cfg := &config.Config{
		CommunityAI: config.CommunityAIConfig{},
		EmbeddedAI:  config.EmbeddedAIConfig{},
	}

	handler := NewAIConfigHandler(service, mockTester, cfg, zap.NewNop())

	// Use a unique project ID for AI config handler tests
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000006")

	return &aiConfigIntegrationTestContext{
		t:          t,
		engineDB:   engineDB,
		handler:    handler,
		service:    service,
		mockTester: mockTester,
		projectID:  projectID,
	}
}

// makeRequest creates an HTTP request with proper context (tenant scope + auth claims).
func (tc *aiConfigIntegrationTestContext) makeRequest(method, path string, body any) *http.Request {
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

	claims := &auth.Claims{ProjectID: tc.projectID.String()}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	req = req.WithContext(ctx)

	tc.t.Cleanup(func() {
		scope.Close()
	})

	return req
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *aiConfigIntegrationTestContext) ensureTestProject() {
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
	`, tc.projectID, "AI Config Handler Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanupAIConfig removes AI config for the test project.
func (tc *aiConfigIntegrationTestContext) cleanupAIConfig() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		UPDATE engine_projects
		SET parameters = parameters - 'ai_config'
		WHERE id = $1
	`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup AI config: %v", err)
	}
}

func TestAIConfigHandler_GetDefault(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	req := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/ai-config",
		nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Get failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var apiResp struct {
		Success bool             `json:"success"`
		Data    AIConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Fatal("Expected success to be true")
	}

	if apiResp.Data.ConfigType != "none" {
		t.Errorf("Expected config_type 'none', got %q", apiResp.Data.ConfigType)
	}

	if apiResp.Data.ProjectID != tc.projectID.String() {
		t.Errorf("Expected project_id %q, got %q", tc.projectID.String(), apiResp.Data.ProjectID)
	}
}

func TestAIConfigHandler_UpsertBYOK(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	body := AIConfigRequest{
		ConfigType: "byok",
		LLMBaseURL: "https://api.openai.com/v1",
		LLMAPIKey:  "sk-test-key-abcdef",
		LLMModel:   "gpt-4o",
	}

	req := tc.makeRequest(http.MethodPut,
		"/api/projects/"+tc.projectID.String()+"/ai-config",
		body)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.Upsert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Upsert failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var apiResp struct {
		Success bool             `json:"success"`
		Data    AIConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Fatal("Expected success to be true")
	}

	if apiResp.Data.ConfigType != "byok" {
		t.Errorf("Expected config_type 'byok', got %q", apiResp.Data.ConfigType)
	}

	if apiResp.Data.LLMBaseURL != "https://api.openai.com/v1" {
		t.Errorf("Expected llm_base_url, got %q", apiResp.Data.LLMBaseURL)
	}

	// API key should be masked in response
	if apiResp.Data.LLMAPIKey != "sk-t...cdef" {
		t.Errorf("Expected masked API key 'sk-t...cdef', got %q", apiResp.Data.LLMAPIKey)
	}
}

func TestAIConfigHandler_Delete(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	// First create a config
	body := AIConfigRequest{
		ConfigType: "byok",
		LLMBaseURL: "https://api.openai.com/v1",
		LLMModel:   "gpt-4o",
	}

	createReq := tc.makeRequest(http.MethodPut,
		"/api/projects/"+tc.projectID.String()+"/ai-config",
		body)
	createReq.SetPathValue("pid", tc.projectID.String())

	createRec := httptest.NewRecorder()
	tc.handler.Upsert(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("Create failed with status %d", createRec.Code)
	}

	// Now delete
	deleteReq := tc.makeRequest(http.MethodDelete,
		"/api/projects/"+tc.projectID.String()+"/ai-config",
		nil)
	deleteReq.SetPathValue("pid", tc.projectID.String())

	deleteRec := httptest.NewRecorder()
	tc.handler.Delete(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("Delete failed with status %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	// Verify deleted
	getReq := tc.makeRequest(http.MethodGet,
		"/api/projects/"+tc.projectID.String()+"/ai-config",
		nil)
	getReq.SetPathValue("pid", tc.projectID.String())

	getRec := httptest.NewRecorder()
	tc.handler.Get(getRec, getReq)

	var apiResp struct {
		Success bool             `json:"success"`
		Data    AIConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if apiResp.Data.ConfigType != "none" {
		t.Errorf("Expected config_type 'none' after delete, got %q", apiResp.Data.ConfigType)
	}
}

func TestAIConfigHandler_TestConnection(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	// Configure mock to return success
	tc.mockTester.TestFunc = func(ctx context.Context, cfg *llm.TestConfig) *llm.TestResult {
		return &llm.TestResult{
			Success:           true,
			Message:           "LLM and embedding connections successful",
			LLMSuccess:        true,
			LLMMessage:        "LLM connection successful (model: gpt-4o, 150ms)",
			LLMResponseTimeMs: 150,
			EmbeddingSuccess:  true,
			EmbeddingMessage:  "Embedding successful",
		}
	}

	body := AIConfigRequest{
		ConfigType: "byok",
		LLMBaseURL: "https://api.openai.com/v1",
		LLMAPIKey:  "sk-test-key",
		LLMModel:   "gpt-4o",
	}

	req := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/ai-config/test",
		body)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.TestConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("TestConnection failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var apiResp struct {
		Success bool           `json:"success"`
		Data    llm.TestResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Fatal("Expected API success to be true")
	}

	if !apiResp.Data.Success {
		t.Errorf("Expected success, got failure: %s", apiResp.Data.Message)
	}

	if !apiResp.Data.LLMSuccess {
		t.Errorf("Expected LLM success")
	}

	if apiResp.Data.LLMResponseTimeMs != 150 {
		t.Errorf("Expected response time 150ms, got %d", apiResp.Data.LLMResponseTimeMs)
	}
}

func TestAIConfigHandler_TestConnection_Failure(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	// Configure mock to return failure
	tc.mockTester.TestFunc = func(ctx context.Context, cfg *llm.TestConfig) *llm.TestResult {
		return &llm.TestResult{
			Success:      false,
			Message:      "LLM: Invalid API key",
			LLMSuccess:   false,
			LLMMessage:   "LLM: Invalid API key",
			LLMErrorType: llm.ErrorTypeAuth,
		}
	}

	body := AIConfigRequest{
		ConfigType: "byok",
		LLMBaseURL: "https://api.openai.com/v1",
		LLMAPIKey:  "invalid-key",
		LLMModel:   "gpt-4o",
	}

	req := tc.makeRequest(http.MethodPost,
		"/api/projects/"+tc.projectID.String()+"/ai-config/test",
		body)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.TestConnection(rec, req)

	// Should still return 200 with failure in body
	if rec.Code != http.StatusOK {
		t.Fatalf("TestConnection failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var apiResp struct {
		Success bool           `json:"success"`
		Error   string         `json:"error"`
		Data    llm.TestResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if apiResp.Success {
		t.Fatal("Expected API success to be false")
	}

	if apiResp.Error != "LLM: Invalid API key" {
		t.Errorf("Expected error 'LLM: Invalid API key', got %q", apiResp.Error)
	}

	if apiResp.Data.LLMErrorType != llm.ErrorTypeAuth {
		t.Errorf("Expected error type 'auth', got %q", apiResp.Data.LLMErrorType)
	}
}

func TestAIConfigHandler_InvalidConfigType(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)
	tc.ensureTestProject()

	body := AIConfigRequest{
		ConfigType: "invalid",
		LLMBaseURL: "https://api.openai.com/v1",
		LLMModel:   "gpt-4o",
	}

	req := tc.makeRequest(http.MethodPut,
		"/api/projects/"+tc.projectID.String()+"/ai-config",
		body)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.Upsert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAIConfigHandler_InvalidProjectID(t *testing.T) {
	tc := setupAIConfigHandlerTest(t)

	req := tc.makeRequest(http.MethodGet,
		"/api/projects/invalid-uuid/ai-config",
		nil)
	req.SetPathValue("pid", "invalid-uuid")

	rec := httptest.NewRecorder()
	tc.handler.Get(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rec.Code)
	}
}

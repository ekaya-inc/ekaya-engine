//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test encryption key (base64 encoded 32-byte key for AES-256)
const aiConfigTestEncryptionKey = "dux2otOLmF8mbcGKm/hk4+WBVT05FmorIokpgrypt9Y="

// aiConfigTestContext holds all dependencies for AI config service integration tests.
type aiConfigTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	service   AIConfigService
	projectID uuid.UUID
}

// setupAIConfigServiceTest creates a test context with real database and services.
func setupAIConfigServiceTest(t *testing.T) *aiConfigTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	encryptor, err := crypto.NewCredentialEncryptor(aiConfigTestEncryptionKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	repo := repositories.NewAIConfigRepository(encryptor)

	// No community/embedded config for tests
	service := NewAIConfigService(repo, nil, nil, zap.NewNop())

	// Use a unique project ID for AI config tests
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000005")

	return &aiConfigTestContext{
		t:         t,
		engineDB:  engineDB,
		service:   service,
		projectID: projectID,
	}
}

// withTenantScope creates a context with tenant scope for the test project.
func (tc *aiConfigTestContext) withTenantScope(ctx context.Context) context.Context {
	tc.t.Helper()

	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	tc.t.Cleanup(func() {
		scope.Close()
	})

	return database.SetTenantScope(ctx, scope)
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *aiConfigTestContext) ensureTestProject() {
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
	`, tc.projectID, "AI Config Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanupAIConfig removes AI config for the test project.
func (tc *aiConfigTestContext) cleanupAIConfig() {
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

func TestAIConfigService_UpsertAndGet(t *testing.T) {
	tc := setupAIConfigServiceTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	ctx := tc.withTenantScope(context.Background())

	// Create BYOK config
	cfg := &models.AIConfig{
		ConfigType:     models.AIConfigBYOK,
		LLMBaseURL:     "https://api.openai.com/v1",
		LLMAPIKey:      "sk-test-key-12345",
		LLMModel:       "gpt-4o",
		EmbeddingModel: "text-embedding-3-small",
	}

	err := tc.service.Upsert(ctx, tc.projectID, cfg)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := tc.service.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected config, got nil")
	}

	if retrieved.ConfigType != models.AIConfigBYOK {
		t.Errorf("Expected config_type 'byok', got %q", retrieved.ConfigType)
	}

	if retrieved.LLMBaseURL != "https://api.openai.com/v1" {
		t.Errorf("Expected llm_base_url 'https://api.openai.com/v1', got %q", retrieved.LLMBaseURL)
	}

	// API key should be decrypted
	if retrieved.LLMAPIKey != "sk-test-key-12345" {
		t.Errorf("Expected decrypted API key, got %q", retrieved.LLMAPIKey)
	}

	if retrieved.LLMModel != "gpt-4o" {
		t.Errorf("Expected llm_model 'gpt-4o', got %q", retrieved.LLMModel)
	}
}

func TestAIConfigService_GetEffective_BYOK(t *testing.T) {
	tc := setupAIConfigServiceTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	ctx := tc.withTenantScope(context.Background())

	cfg := &models.AIConfig{
		ConfigType:       models.AIConfigBYOK,
		LLMBaseURL:       "https://custom.llm.api/v1",
		LLMAPIKey:        "custom-api-key",
		LLMModel:         "custom-model",
		EmbeddingBaseURL: "https://custom.embedding.api/v1",
		EmbeddingAPIKey:  "custom-embedding-key",
		EmbeddingModel:   "custom-embedding-model",
	}

	err := tc.service.Upsert(ctx, tc.projectID, cfg)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	effective, err := tc.service.GetEffective(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetEffective failed: %v", err)
	}

	if effective.LLMBaseURL != "https://custom.llm.api/v1" {
		t.Errorf("Expected custom LLM base URL, got %q", effective.LLMBaseURL)
	}

	if effective.EmbeddingBaseURL != "https://custom.embedding.api/v1" {
		t.Errorf("Expected custom embedding base URL, got %q", effective.EmbeddingBaseURL)
	}
}

func TestAIConfigService_Delete(t *testing.T) {
	tc := setupAIConfigServiceTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	ctx := tc.withTenantScope(context.Background())

	// Create config
	cfg := &models.AIConfig{
		ConfigType: models.AIConfigBYOK,
		LLMBaseURL: "https://api.openai.com/v1",
		LLMModel:   "gpt-4o",
	}

	err := tc.service.Upsert(ctx, tc.projectID, cfg)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Delete
	err = tc.service.Delete(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted (Get returns config with type "none" or nil)
	retrieved, err := tc.service.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}

	// After delete, config_type should be "none"
	if retrieved != nil && retrieved.ConfigType != models.AIConfigNone {
		t.Errorf("Expected config_type 'none' after delete, got %q", retrieved.ConfigType)
	}
}

func TestAIConfigService_Validation_BYOKRequiresFields(t *testing.T) {
	tc := setupAIConfigServiceTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	ctx := tc.withTenantScope(context.Background())

	// BYOK without llm_base_url should fail
	cfg := &models.AIConfig{
		ConfigType: models.AIConfigBYOK,
		LLMModel:   "gpt-4o",
		// Missing LLMBaseURL
	}

	err := tc.service.Upsert(ctx, tc.projectID, cfg)
	if err == nil {
		t.Error("Expected error for BYOK without llm_base_url")
	}

	// BYOK without llm_model should fail
	cfg2 := &models.AIConfig{
		ConfigType: models.AIConfigBYOK,
		LLMBaseURL: "https://api.openai.com/v1",
		// Missing LLMModel
	}

	err = tc.service.Upsert(ctx, tc.projectID, cfg2)
	if err == nil {
		t.Error("Expected error for BYOK without llm_model")
	}
}

func TestAIConfigService_UpdateTestResult(t *testing.T) {
	tc := setupAIConfigServiceTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	ctx := tc.withTenantScope(context.Background())

	// Create config first
	cfg := &models.AIConfig{
		ConfigType: models.AIConfigBYOK,
		LLMBaseURL: "https://api.openai.com/v1",
		LLMModel:   "gpt-4o",
	}

	err := tc.service.Upsert(ctx, tc.projectID, cfg)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Update test result
	err = tc.service.UpdateTestResult(ctx, tc.projectID, true)
	if err != nil {
		t.Fatalf("UpdateTestResult failed: %v", err)
	}

	// Verify test result was saved
	retrieved, err := tc.service.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.LastTestSuccess == nil || !*retrieved.LastTestSuccess {
		t.Error("Expected last_test_success to be true")
	}

	if retrieved.LastTestedAt == nil {
		t.Error("Expected last_tested_at to be set")
	}
}

func TestAIConfigService_GetEffective_NotConfigured(t *testing.T) {
	tc := setupAIConfigServiceTest(t)
	tc.ensureTestProject()
	tc.cleanupAIConfig()

	ctx := tc.withTenantScope(context.Background())

	// GetEffective should fail when no config exists
	_, err := tc.service.GetEffective(ctx, tc.projectID)
	if err == nil {
		t.Error("Expected error when getting effective config without any config")
	}
}

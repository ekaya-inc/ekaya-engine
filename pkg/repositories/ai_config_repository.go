package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AIConfigRepository defines the interface for AI configuration data access.
// AI config is stored as JSONB within engine_projects.parameters.ai_config.
// API keys are encrypted before storage and decrypted after retrieval.
type AIConfigRepository interface {
	// Get retrieves the AI config for a project. Returns nil, nil if no config exists.
	Get(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error)

	// Upsert creates or updates the AI config for a project.
	Upsert(ctx context.Context, projectID uuid.UUID, config *models.AIConfig) error

	// Delete removes the AI config for a project (sets config_type to "none").
	Delete(ctx context.Context, projectID uuid.UUID) error

	// UpdateTestResult updates the last_tested_at and last_test_success fields.
	UpdateTestResult(ctx context.Context, projectID uuid.UUID, success bool) error
}

// aiConfigRepository implements AIConfigRepository using PostgreSQL JSONB.
type aiConfigRepository struct {
	encryptor *crypto.CredentialEncryptor
}

// NewAIConfigRepository creates a new AI config repository.
func NewAIConfigRepository(encryptor *crypto.CredentialEncryptor) AIConfigRepository {
	return &aiConfigRepository{encryptor: encryptor}
}

// Get retrieves the AI config for a project.
func (r *aiConfigRepository) Get(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `SELECT parameters->'ai_config' FROM engine_projects WHERE id = $1`

	var jsonData []byte
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&jsonData)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No project found
		}
		return nil, fmt.Errorf("query ai_config: %w", err)
	}

	// No AI config set yet
	if jsonData == nil || string(jsonData) == "null" {
		return nil, nil
	}

	var stored models.AIConfigStored
	if err := json.Unmarshal(jsonData, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal ai_config: %w", err)
	}

	// Decrypt API keys
	llmKey, err := r.encryptor.Decrypt(stored.LLMAPIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt llm key: %w", err)
	}

	embeddingKey, err := r.encryptor.Decrypt(stored.EmbeddingAPIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt embedding key: %w", err)
	}

	config := &models.AIConfig{
		ConfigType:       models.AIConfigType(stored.ConfigType),
		LLMBaseURL:       stored.LLMBaseURL,
		LLMAPIKey:        llmKey,
		LLMModel:         stored.LLMModel,
		EmbeddingBaseURL: stored.EmbeddingBaseURL,
		EmbeddingAPIKey:  embeddingKey,
		EmbeddingModel:   stored.EmbeddingModel,
		LastTestSuccess:  stored.LastTestSuccess,
	}

	if stored.LastTestedAt != nil {
		t, err := time.Parse(time.RFC3339, *stored.LastTestedAt)
		if err == nil {
			config.LastTestedAt = &t
		}
	}

	return config, nil
}

// Upsert creates or updates the AI config for a project.
func (r *aiConfigRepository) Upsert(ctx context.Context, projectID uuid.UUID, config *models.AIConfig) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Encrypt API keys
	llmKeyEnc, err := r.encryptor.Encrypt(config.LLMAPIKey)
	if err != nil {
		return fmt.Errorf("encrypt llm key: %w", err)
	}

	embKeyEnc, err := r.encryptor.Encrypt(config.EmbeddingAPIKey)
	if err != nil {
		return fmt.Errorf("encrypt embedding key: %w", err)
	}

	stored := models.AIConfigStored{
		ConfigType:               string(config.ConfigType),
		LLMBaseURL:               config.LLMBaseURL,
		LLMAPIKeyEncrypted:       llmKeyEnc,
		LLMModel:                 config.LLMModel,
		EmbeddingBaseURL:         config.EmbeddingBaseURL,
		EmbeddingAPIKeyEncrypted: embKeyEnc,
		EmbeddingModel:           config.EmbeddingModel,
		LastTestSuccess:          config.LastTestSuccess,
	}

	if config.LastTestedAt != nil {
		ts := config.LastTestedAt.Format(time.RFC3339)
		stored.LastTestedAt = &ts
	}

	jsonData, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("marshal ai_config: %w", err)
	}

	query := `
		UPDATE engine_projects
		SET parameters = jsonb_set(
			COALESCE(parameters, '{}'::jsonb),
			'{ai_config}',
			$2::jsonb
		),
		updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, projectID, jsonData)
	if err != nil {
		return fmt.Errorf("update ai_config: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}

	return nil
}

// Delete removes the AI config by setting config_type to "none" and clearing credentials.
func (r *aiConfigRepository) Delete(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Set config_type to "none" and clear all fields
	stored := models.AIConfigStored{
		ConfigType: string(models.AIConfigNone),
	}

	jsonData, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("marshal ai_config: %w", err)
	}

	query := `
		UPDATE engine_projects
		SET parameters = jsonb_set(
			COALESCE(parameters, '{}'::jsonb),
			'{ai_config}',
			$2::jsonb
		),
		updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, projectID, jsonData)
	if err != nil {
		return fmt.Errorf("delete ai_config: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}

	return nil
}

// UpdateTestResult updates the test metadata fields.
func (r *aiConfigRepository) UpdateTestResult(ctx context.Context, projectID uuid.UUID, success bool) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	testedAt := time.Now().Format(time.RFC3339)

	// Update only the test result fields within ai_config
	query := `
		UPDATE engine_projects
		SET parameters = jsonb_set(
			jsonb_set(
				COALESCE(parameters, '{}'::jsonb),
				'{ai_config,last_tested_at}',
				to_jsonb($2::text)
			),
			'{ai_config,last_test_success}',
			to_jsonb($3::boolean)
		),
		updated_at = NOW()
		WHERE id = $1
		AND parameters->'ai_config' IS NOT NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, testedAt, success)
	if err != nil {
		return fmt.Errorf("update test result: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no ai_config to update or project not found")
	}

	return nil
}

// Ensure aiConfigRepository implements AIConfigRepository at compile time.
var _ AIConfigRepository = (*aiConfigRepository)(nil)

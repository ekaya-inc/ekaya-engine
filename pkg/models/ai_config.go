package models

import (
	"time"
)

// AIConfigType represents the type of AI configuration.
type AIConfigType string

const (
	AIConfigNone      AIConfigType = "none"
	AIConfigBYOK      AIConfigType = "byok"
	AIConfigCommunity AIConfigType = "community"
	AIConfigEmbedded  AIConfigType = "embedded"
)

// AIConfig represents a project's AI configuration (decrypted form).
// This is the in-memory representation - API keys are decrypted.
type AIConfig struct {
	ConfigType AIConfigType `json:"config_type"`

	// LLM settings
	LLMBaseURL string `json:"llm_base_url,omitempty"`
	LLMAPIKey  string `json:"llm_api_key,omitempty"` // Decrypted
	LLMModel   string `json:"llm_model,omitempty"`

	// Embedding settings (empty = inherit from LLM)
	EmbeddingBaseURL string `json:"embedding_base_url,omitempty"`
	EmbeddingAPIKey  string `json:"embedding_api_key,omitempty"` // Decrypted
	EmbeddingModel   string `json:"embedding_model,omitempty"`

	// Test metadata
	LastTestedAt    *time.Time `json:"last_tested_at,omitempty"`
	LastTestSuccess *bool      `json:"last_test_success,omitempty"`
}

// AIConfigStored is the JSONB storage format with encrypted keys.
// This is used for database persistence.
type AIConfigStored struct {
	ConfigType               string  `json:"config_type"`
	LLMBaseURL               string  `json:"llm_base_url,omitempty"`
	LLMAPIKeyEncrypted       string  `json:"llm_api_key_encrypted,omitempty"`
	LLMModel                 string  `json:"llm_model,omitempty"`
	EmbeddingBaseURL         string  `json:"embedding_base_url,omitempty"`
	EmbeddingAPIKeyEncrypted string  `json:"embedding_api_key_encrypted,omitempty"`
	EmbeddingModel           string  `json:"embedding_model,omitempty"`
	LastTestedAt             *string `json:"last_tested_at,omitempty"`
	LastTestSuccess          *bool   `json:"last_test_success,omitempty"`
}

// EffectiveEmbeddingBaseURL returns embedding URL, falling back to LLM URL.
func (c *AIConfig) EffectiveEmbeddingBaseURL() string {
	if c.EmbeddingBaseURL != "" {
		return c.EmbeddingBaseURL
	}
	return c.LLMBaseURL
}

// EffectiveEmbeddingAPIKey returns embedding key, falling back to LLM key.
func (c *AIConfig) EffectiveEmbeddingAPIKey() string {
	if c.EmbeddingAPIKey != "" {
		return c.EmbeddingAPIKey
	}
	return c.LLMAPIKey
}

// HasLLMConfig returns true if LLM is configured.
func (c *AIConfig) HasLLMConfig() bool {
	return c.LLMBaseURL != "" && c.LLMModel != ""
}

// HasEmbeddingConfig returns true if embedding is configured.
func (c *AIConfig) HasEmbeddingConfig() bool {
	return c.EffectiveEmbeddingBaseURL() != "" && c.EmbeddingModel != ""
}

// MaskedAPIKey returns masked version: "sk-a...xyz".
func MaskedAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

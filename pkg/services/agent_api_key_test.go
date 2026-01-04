package services

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// agentKeyTestEncryptionKey is a 32-byte test key for agent API key tests.
// This is only used for tests; real key should come from environment.
const agentKeyTestEncryptionKey = "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3Q="

func setupAgentAPIKeyTest(t *testing.T) (AgentAPIKeyService, *mockMCPConfigRepository) {
	t.Helper()

	// Set environment variable for encryption key
	originalKey := os.Getenv("PROJECT_CREDENTIALS_KEY")
	if err := os.Setenv("PROJECT_CREDENTIALS_KEY", agentKeyTestEncryptionKey); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	t.Cleanup(func() {
		if originalKey == "" {
			os.Unsetenv("PROJECT_CREDENTIALS_KEY")
		} else {
			os.Setenv("PROJECT_CREDENTIALS_KEY", originalKey)
		}
	})

	repo := &mockMCPConfigRepository{
		agentAPIKeyByProject: make(map[uuid.UUID]string),
	}

	svc, err := NewAgentAPIKeyService(repo, zap.NewNop())
	require.NoError(t, err)

	return svc, repo
}

func TestAgentAPIKeyService_GenerateKey(t *testing.T) {
	svc, repo := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	key, err := svc.GenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Key should be 64 hex characters (32 bytes)
	assert.Len(t, key, 64, "generated key should be 64 hex characters")

	// Key should be hex-encoded
	for _, c := range key {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"key should only contain hex characters, got: %c", c)
	}

	// Key should be stored encrypted (not the same as plaintext)
	storedEncrypted := repo.agentAPIKeyByProject[projectID]
	assert.NotEmpty(t, storedEncrypted, "encrypted key should be stored")
	assert.NotEqual(t, key, storedEncrypted, "stored key should be encrypted")
}

func TestAgentAPIKeyService_GenerateKey_Unique(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)

	// Generate multiple keys and ensure they're unique
	keys := make(map[string]bool)
	for i := 0; i < 10; i++ {
		projectID := uuid.New()
		key, err := svc.GenerateKey(context.Background(), projectID)
		require.NoError(t, err)

		assert.False(t, keys[key], "generated key should be unique")
		keys[key] = true
	}
}

func TestAgentAPIKeyService_GetKey(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Generate a key first
	generatedKey, err := svc.GenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Get should return the same key
	retrievedKey, err := svc.GetKey(context.Background(), projectID)
	require.NoError(t, err)
	assert.Equal(t, generatedKey, retrievedKey, "retrieved key should match generated key")
}

func TestAgentAPIKeyService_GetKey_NotExists(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Get a key that doesn't exist
	key, err := svc.GetKey(context.Background(), projectID)
	require.NoError(t, err)
	assert.Empty(t, key, "non-existent key should return empty string")
}

func TestAgentAPIKeyService_RegenerateKey(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Generate initial key
	initialKey, err := svc.GenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Regenerate
	newKey, err := svc.RegenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// New key should be different
	assert.NotEqual(t, initialKey, newKey, "regenerated key should be different")
	assert.Len(t, newKey, 64, "regenerated key should be 64 hex characters")

	// Get should return the new key
	retrievedKey, err := svc.GetKey(context.Background(), projectID)
	require.NoError(t, err)
	assert.Equal(t, newKey, retrievedKey, "get should return regenerated key")
}

func TestAgentAPIKeyService_ValidateKey_Valid(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Generate a key
	key, err := svc.GenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Validate with correct key
	valid, err := svc.ValidateKey(context.Background(), projectID, key)
	require.NoError(t, err)
	assert.True(t, valid, "correct key should be valid")
}

func TestAgentAPIKeyService_ValidateKey_Invalid(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Generate a key
	_, err := svc.GenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Validate with wrong key
	valid, err := svc.ValidateKey(context.Background(), projectID, "wrongkey")
	require.NoError(t, err)
	assert.False(t, valid, "wrong key should be invalid")
}

func TestAgentAPIKeyService_ValidateKey_NoKey(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Validate without generating a key first
	valid, err := svc.ValidateKey(context.Background(), projectID, "anykey")
	require.NoError(t, err)
	assert.False(t, valid, "should be invalid when no key exists")
}

func TestAgentAPIKeyService_ValidateKey_AfterRegenerate(t *testing.T) {
	svc, _ := setupAgentAPIKeyTest(t)
	projectID := uuid.New()

	// Generate initial key
	oldKey, err := svc.GenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Regenerate
	newKey, err := svc.RegenerateKey(context.Background(), projectID)
	require.NoError(t, err)

	// Old key should now be invalid
	validOld, err := svc.ValidateKey(context.Background(), projectID, oldKey)
	require.NoError(t, err)
	assert.False(t, validOld, "old key should be invalid after regeneration")

	// New key should be valid
	validNew, err := svc.ValidateKey(context.Background(), projectID, newKey)
	require.NoError(t, err)
	assert.True(t, validNew, "new key should be valid after regeneration")
}

func TestAgentAPIKeyService_NewService_MissingEnvVar(t *testing.T) {
	// Remove the environment variable
	originalKey := os.Getenv("PROJECT_CREDENTIALS_KEY")
	os.Unsetenv("PROJECT_CREDENTIALS_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("PROJECT_CREDENTIALS_KEY", originalKey)
		}
	}()

	repo := &mockMCPConfigRepository{}
	_, err := NewAgentAPIKeyService(repo, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PROJECT_CREDENTIALS_KEY not set")
}

func TestAgentAPIKeyService_NewService_InvalidKey(t *testing.T) {
	originalKey := os.Getenv("PROJECT_CREDENTIALS_KEY")
	os.Setenv("PROJECT_CREDENTIALS_KEY", "not-valid-base64!")
	defer func() {
		if originalKey != "" {
			os.Setenv("PROJECT_CREDENTIALS_KEY", originalKey)
		} else {
			os.Unsetenv("PROJECT_CREDENTIALS_KEY")
		}
	}()

	repo := &mockMCPConfigRepository{}
	_, err := NewAgentAPIKeyService(repo, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create encryptor")
}

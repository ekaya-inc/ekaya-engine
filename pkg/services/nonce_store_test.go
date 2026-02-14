package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNonceStore_Generate_ReturnsUniqueNonces(t *testing.T) {
	store := NewNonceStore()
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		nonce := store.Generate("install", "proj-1", "app-1")
		require.False(t, seen[nonce], "duplicate nonce generated")
		seen[nonce] = true
	}
}

func TestNonceStore_Validate_SucceedsWithCorrectTuple(t *testing.T) {
	store := NewNonceStore()
	nonce := store.Generate("activate", "proj-1", "app-1")

	ok := store.Validate(nonce, "activate", "proj-1", "app-1")
	assert.True(t, ok)
}

func TestNonceStore_Validate_FailsWithWrongAction(t *testing.T) {
	store := NewNonceStore()
	nonce := store.Generate("install", "proj-1", "app-1")

	ok := store.Validate(nonce, "uninstall", "proj-1", "app-1")
	assert.False(t, ok)
}

func TestNonceStore_Validate_FailsWithWrongProjectID(t *testing.T) {
	store := NewNonceStore()
	nonce := store.Generate("install", "proj-1", "app-1")

	ok := store.Validate(nonce, "install", "proj-2", "app-1")
	assert.False(t, ok)
}

func TestNonceStore_Validate_FailsWithWrongAppID(t *testing.T) {
	store := NewNonceStore()
	nonce := store.Generate("install", "proj-1", "app-1")

	ok := store.Validate(nonce, "install", "proj-1", "app-2")
	assert.False(t, ok)
}

func TestNonceStore_Validate_SingleUse(t *testing.T) {
	store := NewNonceStore()
	nonce := store.Generate("activate", "proj-1", "app-1")

	ok := store.Validate(nonce, "activate", "proj-1", "app-1")
	assert.True(t, ok, "first validation should succeed")

	ok = store.Validate(nonce, "activate", "proj-1", "app-1")
	assert.False(t, ok, "second validation should fail (single-use)")
}

func TestNonceStore_Validate_UnknownNonce(t *testing.T) {
	store := NewNonceStore()

	ok := store.Validate("nonexistent-nonce", "install", "proj-1", "app-1")
	assert.False(t, ok)
}

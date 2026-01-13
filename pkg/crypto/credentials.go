// Package crypto provides encryption utilities for project credentials.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	// ErrInvalidKey is returned when the encryption key is empty.
	ErrInvalidKey = errors.New("invalid encryption key: must not be empty")
	// ErrDecryptionFailed is returned when decryption fails due to invalid ciphertext or wrong key.
	ErrDecryptionFailed = errors.New("decryption failed: invalid ciphertext or wrong key")
)

// CredentialEncryptor provides AES-256-GCM encryption for sensitive credential data.
// It uses authenticated encryption to ensure both confidentiality and integrity.
type CredentialEncryptor struct {
	gcm cipher.AEAD
}

// NewCredentialEncryptor creates a new encryptor from a key string.
// The key can be:
//   - A base64-encoded 32-byte key (e.g., from: openssl rand -base64 32)
//   - Any passphrase (will be hashed to 32 bytes with SHA-256)
//
// If the input is valid base64 and decodes to exactly 32 bytes, it's used directly.
// Otherwise, the input is treated as a passphrase and hashed with SHA-256.
func NewCredentialEncryptor(keyInput string) (*CredentialEncryptor, error) {
	if keyInput == "" {
		return nil, ErrInvalidKey
	}

	var key []byte

	// Try base64 decode first
	decoded, err := base64.StdEncoding.DecodeString(keyInput)
	if err == nil && len(decoded) == 32 {
		// Valid base64 that decodes to exactly 32 bytes - use directly
		key = decoded
	} else {
		// Not valid base64 or wrong length - hash the input to get 32 bytes
		hash := sha256.Sum256([]byte(keyInput))
		key = hash[:]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &CredentialEncryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns base64(nonce || ciphertext || tag).
// Empty strings are returned as-is (not encrypted).
func (e *CredentialEncryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// Generate random nonce (12 bytes for GCM)
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends ciphertext and tag to nonce
	// Result: nonce || ciphertext || tag
	ciphertext := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64(nonce || ciphertext || tag) and returns plaintext.
// Empty strings are returned as-is (not decrypted).
func (e *CredentialEncryptor) Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("%w: base64 decode failed", ErrDecryptionFailed)
	}

	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize+e.gcm.Overhead() {
		return "", fmt.Errorf("%w: ciphertext too short", ErrDecryptionFailed)
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: authentication failed", ErrDecryptionFailed)
	}

	return string(plaintext), nil
}

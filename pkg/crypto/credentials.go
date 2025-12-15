// Package crypto provides encryption utilities for project credentials.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	// ErrInvalidKey is returned when the encryption key is not valid (must be 32 bytes base64 encoded).
	ErrInvalidKey = errors.New("invalid encryption key: must be 32 bytes base64 encoded")
	// ErrDecryptionFailed is returned when decryption fails due to invalid ciphertext or wrong key.
	ErrDecryptionFailed = errors.New("decryption failed: invalid ciphertext or wrong key")
)

// CredentialEncryptor provides AES-256-GCM encryption for sensitive credential data.
// It uses authenticated encryption to ensure both confidentiality and integrity.
type CredentialEncryptor struct {
	gcm cipher.AEAD
}

// NewCredentialEncryptor creates a new encryptor from a base64-encoded 32-byte key.
// The key should be generated with: openssl rand -base64 32
func NewCredentialEncryptor(base64Key string) (*CredentialEncryptor, error) {
	if base64Key == "" {
		return nil, ErrInvalidKey
	}

	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode failed", ErrInvalidKey)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes, need 32", ErrInvalidKey, len(key))
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

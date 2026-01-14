package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Test key generated with: openssl rand -base64 32
const testKey = "dGVzdC1rZXktZm9yLXVuaXQtdGVzdHMtMzItYnl0ZXM=" // "test-key-for-unit-tests-32-bytes"

func TestNewCredentialEncryptor(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid 32-byte base64 key",
			key:     testKey,
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
			errMsg:  "invalid encryption key",
		},
		{
			name:    "passphrase (not base64) - hashed to 32 bytes",
			key:     "my-simple-passphrase",
			wantErr: false,
		},
		{
			name:    "short base64 key - hashed to 32 bytes",
			key:     base64.StdEncoding.EncodeToString([]byte("sixteen-byte-key")),
			wantErr: false,
		},
		{
			name:    "long base64 key - hashed to 32 bytes",
			key:     base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 64))),
			wantErr: false,
		},
		{
			name:    "quickstart demo key",
			key:     "quickstart-demo-key",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewCredentialEncryptor(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if enc == nil {
				t.Error("expected non-nil encryptor")
			}
		})
	}
}

func TestPassphraseKeyConsistency(t *testing.T) {
	// Same passphrase should produce same encryption/decryption behavior
	passphrase := "my-consistent-passphrase"

	enc1, err := NewCredentialEncryptor(passphrase)
	if err != nil {
		t.Fatalf("failed to create first encryptor: %v", err)
	}

	enc2, err := NewCredentialEncryptor(passphrase)
	if err != nil {
		t.Fatalf("failed to create second encryptor: %v", err)
	}

	plaintext := "secret-data"
	encrypted, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Second encryptor with same passphrase should decrypt successfully
	decrypted, err := enc2.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("failed to decrypt with same passphrase: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewCredentialEncryptor(testKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "empty string",
			plaintext: "",
		},
		{
			name:      "simple API key",
			plaintext: "sk-abc123xyz789",
		},
		{
			name:      "long API key",
			plaintext: "sk-ant-api03-" + strings.Repeat("x", 100),
		},
		{
			name:      "unicode content",
			plaintext: "API密钥-テスト-키-🔑",
		},
		{
			name:      "special characters",
			plaintext: "key!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
		{
			name:      "newlines and whitespace",
			plaintext: "key with\nnewlines\tand\r\nwhitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := enc.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Empty strings should not be encrypted
			if tt.plaintext == "" {
				if encrypted != "" {
					t.Errorf("empty string should remain empty, got %q", encrypted)
				}
				return
			}

			// Encrypted value should be different from plaintext
			if encrypted == tt.plaintext {
				t.Error("encrypted value should differ from plaintext")
			}

			// Encrypted value should be valid base64
			if _, err := base64.StdEncoding.DecodeString(encrypted); err != nil {
				t.Errorf("encrypted value should be valid base64: %v", err)
			}

			// Decrypt should recover original
			decrypted, err := enc.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("decrypted value mismatch: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	enc, err := NewCredentialEncryptor(testKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	plaintext := "same-plaintext"
	seen := make(map[string]bool)

	// Encrypt same value multiple times - should produce different ciphertexts
	for i := 0; i < 100; i++ {
		encrypted, err := enc.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		if seen[encrypted] {
			t.Error("encryption produced duplicate ciphertext (nonce reuse)")
		}
		seen[encrypted] = true
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	enc1, err := NewCredentialEncryptor(testKey)
	if err != nil {
		t.Fatalf("failed to create first encryptor: %v", err)
	}

	// Different 32-byte key (exactly 32 characters)
	differentKey := base64.StdEncoding.EncodeToString([]byte("different-key-for-testing-32-b!!"))
	enc2, err := NewCredentialEncryptor(differentKey)
	if err != nil {
		t.Fatalf("failed to create second encryptor: %v", err)
	}

	plaintext := "secret-api-key"
	encrypted, err := enc1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// Decrypting with wrong key should fail
	_, err = enc2.Decrypt(encrypted)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
	if !strings.Contains(err.Error(), "decryption failed") {
		t.Errorf("expected 'decryption failed' error, got: %v", err)
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	enc, _ := NewCredentialEncryptor(testKey)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "empty string returns empty",
			input:   "",
			wantErr: "", // No error for empty
		},
		{
			name:    "invalid base64",
			input:   "not-valid-base64!!!",
			wantErr: "base64 decode failed",
		},
		{
			name:    "too short ciphertext",
			input:   base64.StdEncoding.EncodeToString([]byte("short")),
			wantErr: "ciphertext too short",
		},
		{
			name:    "corrupted ciphertext",
			input:   base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 50))),
			wantErr: "authentication failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := enc.Decrypt(tt.input)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.input == "" && result != "" {
					t.Error("empty input should return empty result")
				}
				return
			}

			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestRoundTripWithRealKey(t *testing.T) {
	// Generate a real 32-byte key
	realKey := base64.StdEncoding.EncodeToString([]byte("this-is-a-real-32-byte-test-key!"))

	enc, err := NewCredentialEncryptor(realKey)
	if err != nil {
		t.Fatalf("failed to create encryptor with real key: %v", err)
	}

	secrets := []string{
		"sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"sk-proj-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"postgres://user:password@localhost:5432/db",
	}

	for _, secret := range secrets {
		encrypted, err := enc.Encrypt(secret)
		if err != nil {
			t.Errorf("failed to encrypt %q: %v", secret[:20]+"...", err)
			continue
		}

		decrypted, err := enc.Decrypt(encrypted)
		if err != nil {
			t.Errorf("failed to decrypt: %v", err)
			continue
		}

		if decrypted != secret {
			t.Errorf("round-trip failed: got %q, want %q", decrypted[:20]+"...", secret[:20]+"...")
		}
	}
}

package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func generateAPIKey() (string, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	return hex.EncodeToString(keyBytes), nil
}

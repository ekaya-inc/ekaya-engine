package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const (
	agentAPIKeyPrefix       = "ekai-"
	agentAPIKeyEntropyBytes = 32
)

func generateAPIKey() (string, error) {
	keyBytes := make([]byte, agentAPIKeyEntropyBytes)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}

	return agentAPIKeyPrefix + hex.EncodeToString(keyBytes), nil
}

package services

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// NonceStore manages single-use nonces for callback validation.
// Nonces are tied to a specific (action, projectID, appID) tuple.
type NonceStore interface {
	// Generate creates a new nonce tied to the given action, project, and app.
	Generate(action, projectID, appID string) string
	// Validate checks if the nonce is valid for the given action, project, and app.
	// Returns true and deletes the nonce if valid (single-use).
	Validate(nonce, action, projectID, appID string) bool
}

type nonceEntry struct {
	action    string
	projectID string
	appID     string
}

type nonceStore struct {
	mu     sync.Mutex
	nonces map[string]nonceEntry
}

// NewNonceStore creates a new in-memory nonce store.
func NewNonceStore() NonceStore {
	return &nonceStore{
		nonces: make(map[string]nonceEntry),
	}
}

func (s *nonceStore) Generate(action, projectID, appID string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random nonce: " + err.Error())
	}
	nonce := hex.EncodeToString(b)

	s.mu.Lock()
	s.nonces[nonce] = nonceEntry{
		action:    action,
		projectID: projectID,
		appID:     appID,
	}
	s.mu.Unlock()

	return nonce
}

func (s *nonceStore) Validate(nonce, action, projectID, appID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.nonces[nonce]
	if !ok {
		return false
	}

	if entry.action != action || entry.projectID != projectID || entry.appID != appID {
		return false
	}

	// Single-use: delete on successful validation
	delete(s.nonces, nonce)
	return true
}

var _ NonceStore = (*nonceStore)(nil)

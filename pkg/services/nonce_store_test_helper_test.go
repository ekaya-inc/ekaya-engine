package services

import (
	"context"
	"fmt"
	"sync"
)

type testNonceEntry struct {
	action    string
	projectID string
	appID     string
}

type testNonceStore struct {
	mu      sync.Mutex
	nextID  int
	entries map[string]testNonceEntry
}

func newTestNonceStore() *testNonceStore {
	return &testNonceStore{
		entries: make(map[string]testNonceEntry),
	}
}

func (s *testNonceStore) Generate(_ context.Context, action, projectID, appID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	nonce := fmt.Sprintf("test-nonce-%d", s.nextID)
	s.entries[nonce] = testNonceEntry{
		action:    action,
		projectID: projectID,
		appID:     appID,
	}

	return nonce, nil
}

func (s *testNonceStore) Validate(_ context.Context, nonce, action, projectID, appID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[nonce]
	if !ok {
		return false, nil
	}

	if entry.action != action || entry.projectID != projectID || entry.appID != appID {
		return false, nil
	}

	delete(s.entries, nonce)
	return true, nil
}

var _ NonceStore = (*testNonceStore)(nil)

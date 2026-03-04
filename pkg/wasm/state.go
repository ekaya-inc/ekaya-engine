package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// StateStore persists app state with CAS (compare-and-swap) versioning.
// Each app has isolated state identified by appID.
type StateStore interface {
	// Get returns the current state data and version for the given app.
	// If no state exists, returns (nil, 0, nil).
	Get(ctx context.Context, appID string) (data []byte, version int64, err error)

	// Set writes state data if expectedVersion matches the current version.
	// Returns the new version on success or ErrVersionMismatch on conflict.
	Set(ctx context.Context, appID string, data []byte, expectedVersion int64) (newVersion int64, err error)
}

// ErrVersionMismatch is returned when a CAS write fails because the
// expected version doesn't match the current version.
type ErrVersionMismatch struct {
	Expected int64
	Actual   int64
}

func (e *ErrVersionMismatch) Error() string {
	return fmt.Sprintf("version mismatch: expected %d, got %d", e.Expected, e.Actual)
}

type appState struct {
	data    []byte
	version int64
}

// MemoryStateStore is an in-memory StateStore suitable for testing.
// Thread-safe via mutex.
type MemoryStateStore struct {
	mu    sync.Mutex
	state map[string]*appState
}

// NewMemoryStateStore creates a new in-memory state store.
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		state: make(map[string]*appState),
	}
}

func (s *MemoryStateStore) Get(_ context.Context, appID string) ([]byte, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[appID]
	if !ok {
		return nil, 0, nil
	}
	// Return a copy to prevent external mutation.
	out := make([]byte, len(st.data))
	copy(out, st.data)
	return out, st.version, nil
}

func (s *MemoryStateStore) Set(_ context.Context, appID string, data []byte, expectedVersion int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[appID]
	if !ok {
		st = &appState{}
		s.state[appID] = st
	}

	if st.version != expectedVersion {
		return 0, &ErrVersionMismatch{Expected: expectedVersion, Actual: st.version}
	}

	stored := make([]byte, len(data))
	copy(stored, data)
	st.data = stored
	st.version++
	return st.version, nil
}

// StateHostFuncs returns host functions for state_get and state_set,
// scoped to the given appID. The WASM module doesn't need to know its own ID.
func StateHostFuncs(store StateStore, appID string) []HostFunc {
	return []HostFunc{
		{
			Name: "state_get",
			Fn: func(ctx context.Context, _ []byte) ([]byte, error) {
				data, version, err := store.Get(ctx, appID)
				if err != nil {
					return nil, err
				}
				return json.Marshal(stateGetResponse{
					Data:    json.RawMessage(data),
					Version: version,
				})
			},
		},
		{
			Name: "state_set",
			Fn: func(ctx context.Context, input []byte) ([]byte, error) {
				var req stateSetRequest
				if err := json.Unmarshal(input, &req); err != nil {
					return nil, fmt.Errorf("invalid state_set input: %w", err)
				}
				newVersion, err := store.Set(ctx, appID, req.Data, req.Version)
				if err != nil {
					// Return CAS errors as structured JSON so the guest can
					// distinguish them from unexpected failures.
					return json.Marshal(stateSetResponse{Error: err.Error()})
				}
				return json.Marshal(stateSetResponse{Version: &newVersion})
			},
		},
	}
}

type stateGetResponse struct {
	Data    json.RawMessage `json:"data"`
	Version int64           `json:"version"`
}

type stateSetRequest struct {
	Data    json.RawMessage `json:"data"`
	Version int64           `json:"version"`
}

type stateSetResponse struct {
	Version *int64 `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

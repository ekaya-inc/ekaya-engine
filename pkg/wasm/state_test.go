package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestMemoryStateStore_InitialEmpty(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	data, version, err := store.Get(ctx, "app1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data, got %q", data)
	}
	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}
}

func TestMemoryStateStore_WriteReadRoundTrip(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	blob := []byte(`{"key":"value"}`)
	newVersion, err := store.Set(ctx, "app1", blob, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if newVersion != 1 {
		t.Errorf("expected version 1, got %d", newVersion)
	}

	data, version, err := store.Get(ctx, "app1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != string(blob) {
		t.Errorf("got data %q, want %q", data, blob)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
}

func TestMemoryStateStore_CASConflict(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	// Write version 0 → 1.
	_, err := store.Set(ctx, "app1", []byte(`"first"`), 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Try to write with wrong version (0 instead of 1).
	_, err = store.Set(ctx, "app1", []byte(`"stale"`), 0)
	if err == nil {
		t.Fatal("expected CAS error, got nil")
	}

	var mismatch *ErrVersionMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected ErrVersionMismatch, got %T: %v", err, err)
	}
	if mismatch.Expected != 0 || mismatch.Actual != 1 {
		t.Errorf("expected mismatch {0, 1}, got {%d, %d}", mismatch.Expected, mismatch.Actual)
	}

	// Verify original data is unchanged.
	data, _, _ := store.Get(ctx, "app1")
	if string(data) != `"first"` {
		t.Errorf("data corrupted after CAS failure: got %q", data)
	}
}

func TestMemoryStateStore_SequentialVersions(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	for i := range int64(3) {
		newVersion, err := store.Set(ctx, "app1", []byte(`"v"`), i)
		if err != nil {
			t.Fatalf("Set at version %d failed: %v", i, err)
		}
		expected := i + 1
		if newVersion != expected {
			t.Errorf("after Set with version %d: got %d, want %d", i, newVersion, expected)
		}
	}

	_, version, _ := store.Get(ctx, "app1")
	if version != 3 {
		t.Errorf("final version: got %d, want 3", version)
	}
}

func TestMemoryStateStore_IsolatedPerApp(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	_, err := store.Set(ctx, "app1", []byte(`"data1"`), 0)
	if err != nil {
		t.Fatalf("Set app1 failed: %v", err)
	}
	_, err = store.Set(ctx, "app2", []byte(`"data2"`), 0)
	if err != nil {
		t.Fatalf("Set app2 failed: %v", err)
	}

	d1, v1, _ := store.Get(ctx, "app1")
	d2, v2, _ := store.Get(ctx, "app2")

	if string(d1) != `"data1"` {
		t.Errorf("app1 data: got %q, want %q", d1, `"data1"`)
	}
	if string(d2) != `"data2"` {
		t.Errorf("app2 data: got %q, want %q", d2, `"data2"`)
	}
	if v1 != 1 || v2 != 1 {
		t.Errorf("versions: app1=%d app2=%d, want both 1", v1, v2)
	}

	// Writing to app1 shouldn't affect app2's version.
	_, err = store.Set(ctx, "app1", []byte(`"updated"`), 1)
	if err != nil {
		t.Fatalf("Set app1 v1 failed: %v", err)
	}
	_, v2after, _ := store.Get(ctx, "app2")
	if v2after != 1 {
		t.Errorf("app2 version changed after app1 write: got %d", v2after)
	}
}

func TestStateHostFuncs_GetEmpty(t *testing.T) {
	store := NewMemoryStateStore()
	funcs := StateHostFuncs(store, "app1")
	ctx := context.Background()

	// Find state_get.
	var getFn func(context.Context, []byte) ([]byte, error)
	for _, f := range funcs {
		if f.Name == "state_get" {
			getFn = f.Fn
		}
	}
	if getFn == nil {
		t.Fatal("state_get host function not found")
	}

	out, err := getFn(ctx, nil)
	if err != nil {
		t.Fatalf("state_get failed: %v", err)
	}

	var resp stateGetResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if string(resp.Data) != "null" {
		t.Errorf("expected null data, got %s", resp.Data)
	}
	if resp.Version != 0 {
		t.Errorf("expected version 0, got %d", resp.Version)
	}
}

func TestStateHostFuncs_SetAndGet(t *testing.T) {
	store := NewMemoryStateStore()
	funcs := StateHostFuncs(store, "app1")
	ctx := context.Background()

	var getFn, setFn func(context.Context, []byte) ([]byte, error)
	for _, f := range funcs {
		switch f.Name {
		case "state_get":
			getFn = f.Fn
		case "state_set":
			setFn = f.Fn
		}
	}

	// Set state.
	setInput, _ := json.Marshal(stateSetRequest{
		Data:    json.RawMessage(`{"key":"value"}`),
		Version: 0,
	})
	out, err := setFn(ctx, setInput)
	if err != nil {
		t.Fatalf("state_set failed: %v", err)
	}

	var setResp stateSetResponse
	if err := json.Unmarshal(out, &setResp); err != nil {
		t.Fatalf("unmarshal set response failed: %v", err)
	}
	if setResp.Error != "" {
		t.Fatalf("state_set returned error: %s", setResp.Error)
	}
	if setResp.Version == nil || *setResp.Version != 1 {
		t.Fatalf("expected version 1, got %v", setResp.Version)
	}

	// Get state back.
	out, err = getFn(ctx, nil)
	if err != nil {
		t.Fatalf("state_get failed: %v", err)
	}

	var getResp stateGetResponse
	if err := json.Unmarshal(out, &getResp); err != nil {
		t.Fatalf("unmarshal get response failed: %v", err)
	}
	if string(getResp.Data) != `{"key":"value"}` {
		t.Errorf("got data %s, want %s", getResp.Data, `{"key":"value"}`)
	}
	if getResp.Version != 1 {
		t.Errorf("expected version 1, got %d", getResp.Version)
	}
}

func TestStateHostFuncs_CASError(t *testing.T) {
	store := NewMemoryStateStore()
	funcs := StateHostFuncs(store, "app1")
	ctx := context.Background()

	var setFn func(context.Context, []byte) ([]byte, error)
	for _, f := range funcs {
		if f.Name == "state_set" {
			setFn = f.Fn
		}
	}

	// Write version 0 → 1.
	input, _ := json.Marshal(stateSetRequest{
		Data:    json.RawMessage(`"first"`),
		Version: 0,
	})
	_, err := setFn(ctx, input)
	if err != nil {
		t.Fatalf("first set failed: %v", err)
	}

	// Try stale write with version 0 (current is 1).
	staleInput, _ := json.Marshal(stateSetRequest{
		Data:    json.RawMessage(`"stale"`),
		Version: 0,
	})
	out, err := setFn(ctx, staleInput)
	if err != nil {
		t.Fatalf("state_set returned Go error instead of JSON error: %v", err)
	}

	var resp stateSetResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected CAS error in response, got none")
	}
	if resp.Version != nil {
		t.Errorf("expected no version on error, got %d", *resp.Version)
	}
}

func TestStateGuestWASMRoundTrip(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("state_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read state_guest.wasm: %v", err)
	}

	store := NewMemoryStateStore()
	hostFuncs := StateHostFuncs(store, "test-app")

	rt := NewRuntime()
	ctx := context.Background()

	out, err := rt.LoadAndRun(ctx, wasmBytes, "run", nil, hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	if string(out) != "PASS" {
		t.Fatalf("guest returned %q, want PASS", string(out))
	}

	// Verify state was actually persisted in the store.
	data, version, err := store.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}
	if version != 1 {
		t.Errorf("store version: got %d, want 1", version)
	}
	if string(data) != `{"key":"value"}` {
		t.Errorf("store data: got %q, want %q", data, `{"key":"value"}`)
	}
}

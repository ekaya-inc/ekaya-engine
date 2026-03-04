package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func echoInvoker() *MapToolInvoker {
	return &MapToolInvoker{
		Handlers: map[string]func(ctx context.Context, arguments map[string]any) ([]byte, bool, error){
			"echo_tool": func(_ context.Context, args map[string]any) ([]byte, bool, error) {
				msg, _ := args["msg"].(string)
				result, _ := json.Marshal(map[string]string{"echo": msg})
				return result, false, nil
			},
			"error_tool": func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
				result, _ := json.Marshal(map[string]string{"message": "something went wrong"})
				return result, true, nil
			},
			"system_error_tool": func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
				return nil, false, fmt.Errorf("internal failure")
			},
		},
	}
}

func TestToolInvokeHostFunc_Success(t *testing.T) {
	hf := ToolInvokeHostFunc(echoInvoker())
	ctx := context.Background()

	input, _ := json.Marshal(toolInvokeRequest{
		Tool:      "echo_tool",
		Arguments: map[string]any{"msg": "hello"},
	})

	out, err := hf.Fn(ctx, input)
	if err != nil {
		t.Fatalf("host func returned error: %v", err)
	}

	var resp toolInvokeResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.IsError {
		t.Error("expected is_error=false")
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result failed: %v", err)
	}
	if result["echo"] != "hello" {
		t.Errorf("got echo=%q, want %q", result["echo"], "hello")
	}
}

func TestToolInvokeHostFunc_ToolError(t *testing.T) {
	hf := ToolInvokeHostFunc(echoInvoker())
	ctx := context.Background()

	input, _ := json.Marshal(toolInvokeRequest{
		Tool: "error_tool",
	})

	out, err := hf.Fn(ctx, input)
	if err != nil {
		t.Fatalf("host func returned error: %v", err)
	}

	var resp toolInvokeResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !resp.IsError {
		t.Error("expected is_error=true")
	}
}

func TestToolInvokeHostFunc_UnknownTool(t *testing.T) {
	hf := ToolInvokeHostFunc(echoInvoker())
	ctx := context.Background()

	input, _ := json.Marshal(toolInvokeRequest{
		Tool: "nonexistent",
	})

	out, err := hf.Fn(ctx, input)
	if err != nil {
		t.Fatalf("host func returned error: %v", err)
	}

	var resp toolInvokeErrorResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected error message for unknown tool")
	}
}

func TestToolInvokeHostFunc_SystemError(t *testing.T) {
	hf := ToolInvokeHostFunc(echoInvoker())
	ctx := context.Background()

	input, _ := json.Marshal(toolInvokeRequest{
		Tool: "system_error_tool",
	})

	out, err := hf.Fn(ctx, input)
	if err != nil {
		t.Fatalf("host func returned error: %v", err)
	}

	var resp toolInvokeErrorResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected error message for system error")
	}
}

func TestToolInvokeHostFunc_InvalidJSON(t *testing.T) {
	hf := ToolInvokeHostFunc(echoInvoker())
	ctx := context.Background()

	out, err := hf.Fn(ctx, []byte("not json"))
	if err != nil {
		t.Fatalf("host func returned error: %v", err)
	}

	var resp toolInvokeErrorResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected error for invalid JSON input")
	}
}

func TestToolInvokeHostFunc_EmptyToolName(t *testing.T) {
	hf := ToolInvokeHostFunc(echoInvoker())
	ctx := context.Background()

	input, _ := json.Marshal(toolInvokeRequest{
		Tool: "",
	})

	out, err := hf.Fn(ctx, input)
	if err != nil {
		t.Fatalf("host func returned error: %v", err)
	}

	var resp toolInvokeErrorResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected error for empty tool name")
	}
}

func TestMapToolInvoker_UnknownTool(t *testing.T) {
	invoker := &MapToolInvoker{
		Handlers: map[string]func(ctx context.Context, arguments map[string]any) ([]byte, bool, error){},
	}

	_, _, err := invoker.InvokeTool(context.Background(), "missing", nil)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

// --- WASM integration tests ---

func TestToolGuestWASM_EchoSuccess(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("tool_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read tool_guest.wasm: %v", err)
	}

	invoker := echoInvoker()
	hostFuncs := []HostFunc{ToolInvokeHostFunc(invoker)}

	rt := NewRuntime()
	out, err := rt.LoadAndRun(context.Background(), wasmBytes, "run", []byte("test_echo"), hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}
	if string(out) != "PASS" {
		t.Fatalf("guest returned %q, want PASS", string(out))
	}
}

func TestToolGuestWASM_ErrorTool(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("tool_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read tool_guest.wasm: %v", err)
	}

	invoker := echoInvoker()
	hostFuncs := []HostFunc{ToolInvokeHostFunc(invoker)}

	rt := NewRuntime()
	out, err := rt.LoadAndRun(context.Background(), wasmBytes, "run", []byte("test_error_tool"), hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}
	if string(out) != "PASS" {
		t.Fatalf("guest returned %q, want PASS", string(out))
	}
}

func TestToolGuestWASM_UnknownTool(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("tool_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read tool_guest.wasm: %v", err)
	}

	invoker := echoInvoker()
	hostFuncs := []HostFunc{ToolInvokeHostFunc(invoker)}

	rt := NewRuntime()
	out, err := rt.LoadAndRun(context.Background(), wasmBytes, "run", []byte("test_unknown_tool"), hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}
	if string(out) != "PASS" {
		t.Fatalf("guest returned %q, want PASS", string(out))
	}
}

func TestToolStateGuestWASM_ComposedHostFuncs(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("tool_state_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read tool_state_guest.wasm: %v", err)
	}

	invoker := echoInvoker()
	store := NewMemoryStateStore()

	hostFuncs := append([]HostFunc{ToolInvokeHostFunc(invoker)}, StateHostFuncs(store, "test-app")...)

	rt := NewRuntime()
	out, err := rt.LoadAndRun(context.Background(), wasmBytes, "run", nil, hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}
	if string(out) != "PASS" {
		t.Fatalf("guest returned %q, want PASS", string(out))
	}

	// Verify state was persisted by the guest.
	data, version, err := store.Get(context.Background(), "test-app")
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}
	if version != 1 {
		t.Errorf("store version: got %d, want 1", version)
	}
	if string(data) != `{"echo":"composed"}` {
		t.Errorf("store data: got %q, want %q", data, `{"echo":"composed"}`)
	}
}

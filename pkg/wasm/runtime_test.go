package wasm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func TestRoundTrip(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("echo_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read wasm file: %v", err)
	}

	hostEcho := HostFunc{
		Name: "host_echo",
		Fn: func(ctx context.Context, input []byte) ([]byte, error) {
			return append([]byte("echo: "), input...), nil
		},
	}

	rt := NewRuntime()
	ctx := context.Background()

	out, err := rt.LoadAndRun(ctx, wasmBytes, "run", []byte("hello wasm"), []HostFunc{hostEcho})
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	expected := "echo: hello wasm"
	if string(out) != expected {
		t.Errorf("got %q, want %q", string(out), expected)
	}
}

func TestModuleLoadsSuccessfully(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("echo_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read wasm file: %v", err)
	}

	// Just verify the module loads and runs without error.
	rt := NewRuntime()
	ctx := context.Background()

	_, err = rt.LoadAndRun(ctx, wasmBytes, "run", []byte("test"), []HostFunc{
		{Name: "host_echo", Fn: func(ctx context.Context, input []byte) ([]byte, error) {
			return input, nil
		}},
	})
	if err != nil {
		t.Fatalf("module failed to load and run: %v", err)
	}
}

func TestHostFunctionReceivesInput(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("echo_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read wasm file: %v", err)
	}

	var receivedInput []byte
	hostEcho := HostFunc{
		Name: "host_echo",
		Fn: func(ctx context.Context, input []byte) ([]byte, error) {
			receivedInput = make([]byte, len(input))
			copy(receivedInput, input)
			return input, nil
		},
	}

	rt := NewRuntime()
	ctx := context.Background()

	_, err = rt.LoadAndRun(ctx, wasmBytes, "run", []byte("capture this"), []HostFunc{hostEcho})
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	if string(receivedInput) != "capture this" {
		t.Errorf("host function received %q, want %q", string(receivedInput), "capture this")
	}
}

func TestEmptyInput(t *testing.T) {
	wasmBytes, err := os.ReadFile(testdataPath("echo_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read wasm file: %v", err)
	}

	rt := NewRuntime()
	ctx := context.Background()

	out, err := rt.LoadAndRun(ctx, wasmBytes, "run", []byte(""), []HostFunc{
		{Name: "host_echo", Fn: func(ctx context.Context, input []byte) ([]byte, error) {
			return []byte("got empty"), nil
		}},
	})
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	if string(out) != "got empty" {
		t.Errorf("got %q, want %q", string(out), "got empty")
	}
}

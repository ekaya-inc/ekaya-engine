// Package wasm provides the WASM application runtime for Ekaya Engine.
// It loads WASM modules (Extism plugins) and invokes their exported functions
// with access to host-provided capabilities.
package wasm

import (
	"context"
	"fmt"

	extism "github.com/extism/go-sdk"
)

// HostFunc defines a host function that a WASM module can call.
// The function receives input bytes and returns output bytes.
type HostFunc struct {
	Name string
	Fn   func(ctx context.Context, input []byte) ([]byte, error)
}

// Runtime manages WASM plugin lifecycle.
type Runtime struct{}

// NewRuntime creates a new WASM runtime.
func NewRuntime() *Runtime {
	return &Runtime{}
}

// LoadAndRun loads a WASM module, registers host functions, calls the named
// export with the given input, and returns the output.
func (r *Runtime) LoadAndRun(ctx context.Context, wasmBytes []byte, exportName string, input []byte, hostFuncs []HostFunc) ([]byte, error) {
	extismHostFuncs := make([]extism.HostFunction, len(hostFuncs))
	for i, hf := range hostFuncs {
		hf := hf // capture for closure
		f := extism.NewHostFunctionWithStack(
			hf.Name,
			func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
				inBytes, err := p.ReadBytes(stack[0])
				if err != nil {
					// Signal error by returning empty output.
					offset, _ := p.WriteBytes(nil)
					stack[0] = offset
					return
				}

				out, err := hf.Fn(ctx, inBytes)
				if err != nil {
					offset, _ := p.WriteBytes([]byte(err.Error()))
					stack[0] = offset
					return
				}

				offset, err := p.WriteBytes(out)
				if err != nil {
					return
				}
				stack[0] = offset
			},
			[]extism.ValueType{extism.ValueTypePTR},
			[]extism.ValueType{extism.ValueTypePTR},
		)
		f.SetNamespace("extism:host/user")
		extismHostFuncs[i] = f
	}

	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
	}

	config := extism.PluginConfig{
		EnableWasi: true,
	}

	plugin, err := extism.NewPlugin(ctx, manifest, config, extismHostFuncs)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}
	defer plugin.Close(ctx)

	exit, out, err := plugin.Call(exportName, input)
	if err != nil {
		return nil, fmt.Errorf("plugin call %q failed (exit=%d): %w", exportName, exit, err)
	}

	return out, nil
}

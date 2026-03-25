package servercontrol

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	state := &State{
		Status: Status{
			PID:        4321,
			StartedAt:  time.Unix(1700000000, 0).UTC(),
			Version:    "v1.2.3",
			ConfigPath: configPath,
			BaseURL:    "http://localhost:3443",
		},
		ControlURL: "http://127.0.0.1:40123",
		Token:      "secret-token",
	}

	if err := WriteState(configPath, state); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	got, err := ReadState(configPath)
	if err != nil {
		t.Fatalf("ReadState() failed: %v", err)
	}

	if got.PID != state.PID {
		t.Fatalf("PID = %d, want %d", got.PID, state.PID)
	}
	if !got.StartedAt.Equal(state.StartedAt) {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, state.StartedAt)
	}
	if got.Version != state.Version {
		t.Fatalf("Version = %q, want %q", got.Version, state.Version)
	}
	if got.ConfigPath != state.ConfigPath {
		t.Fatalf("ConfigPath = %q, want %q", got.ConfigPath, state.ConfigPath)
	}
	if got.BaseURL != state.BaseURL {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, state.BaseURL)
	}
	if got.ControlURL != state.ControlURL {
		t.Fatalf("ControlURL = %q, want %q", got.ControlURL, state.ControlURL)
	}
	if got.Token != state.Token {
		t.Fatalf("Token = %q, want %q", got.Token, state.Token)
	}
}

func TestWriteStateCreatesMissingStateDir(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "env-only", "config.yaml")
	state := &State{
		Status: Status{
			PID:        5555,
			StartedAt:  time.Unix(1700000050, 0).UTC(),
			Version:    "v1.2.3",
			ConfigPath: configPath,
			BaseURL:    "http://localhost:3443",
		},
		ControlURL: "http://127.0.0.1:40124",
		Token:      "secret-token",
	}

	if err := WriteState(configPath, state); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	if _, err := os.Stat(StatePath(configPath)); err != nil {
		t.Fatalf("runtime state file missing: %v", err)
	}

	got, err := ReadState(configPath)
	if err != nil {
		t.Fatalf("ReadState() failed: %v", err)
	}
	if got.ConfigPath != configPath {
		t.Fatalf("ConfigPath = %q, want %q", got.ConfigPath, configPath)
	}
}

func TestControlServerStatusAndShutdown(t *testing.T) {
	t.Parallel()

	shutdownRequested := make(chan struct{}, 1)
	status := Status{
		PID:        9876,
		StartedAt:  time.Unix(1700000100, 0).UTC(),
		Version:    "test-version",
		ConfigPath: "/tmp/config.yaml",
		BaseURL:    "http://localhost:3443",
	}

	server, controlURL, err := Start(status, "test-token", func() {
		select {
		case shutdownRequested <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer server.Shutdown(context.Background())

	state := &State{
		Status:     status,
		ControlURL: controlURL,
		Token:      "test-token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := GetStatus(ctx, state)
	if err != nil {
		t.Fatalf("GetStatus() failed: %v", err)
	}
	if got.PID != status.PID {
		t.Fatalf("PID = %d, want %d", got.PID, status.PID)
	}
	if got.Version != status.Version {
		t.Fatalf("Version = %q, want %q", got.Version, status.Version)
	}

	if err := RequestShutdown(ctx, state); err != nil {
		t.Fatalf("RequestShutdown() failed: %v", err)
	}

	select {
	case <-shutdownRequested:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown callback was not invoked")
	}
}

func TestControlServerRejectsInvalidToken(t *testing.T) {
	t.Parallel()

	server, controlURL, err := Start(Status{}, "expected-token", func() {})
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer server.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state := &State{
		ControlURL: controlURL,
		Token:      "wrong-token",
	}

	if _, err := GetStatus(ctx, state); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("GetStatus() error = %v, want ErrUnauthorized", err)
	}

	if err := RequestShutdown(ctx, state); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("RequestShutdown() error = %v, want ErrUnauthorized", err)
	}
}

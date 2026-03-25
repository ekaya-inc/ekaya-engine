package runtimectl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ekaya-inc/ekaya-engine/pkg/servercontrol"
)

func TestReadStatusAndStopWithSyntheticConfigPath(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "env-only", "config.yaml")
	status := servercontrol.Status{
		PID:        2468,
		StartedAt:  time.Unix(1700000200, 0).UTC(),
		Version:    "test-version",
		ConfigPath: configPath,
		BaseURL:    "http://localhost:3443",
	}

	var controlServer *servercontrol.Server
	controlServer, controlURL, err := servercontrol.Start(status, "test-token", func() {
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = servercontrol.RemoveState(configPath)
		}()
	})
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = controlServer.Shutdown(context.Background())
		_ = servercontrol.RemoveState(configPath)
	})

	if err := servercontrol.WriteState(configPath, &servercontrol.State{
		Status:     status,
		ControlURL: controlURL,
		Token:      "test-token",
	}); err != nil {
		t.Fatalf("WriteState() failed: %v", err)
	}

	_, gotStatus, stale, err := ReadStatus(configPath)
	if err != nil {
		t.Fatalf("ReadStatus() failed: %v", err)
	}
	if stale {
		t.Fatal("expected ReadStatus() to report a live runtime state")
	}
	if gotStatus == nil {
		t.Fatal("expected ReadStatus() to return status")
	}
	if gotStatus.PID != status.PID {
		t.Fatalf("PID = %d, want %d", gotStatus.PID, status.PID)
	}

	result, err := Stop(configPath, 2*time.Second)
	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	if !result.WasRunning {
		t.Fatal("expected Stop() to report a running server")
	}
	if result.RemovedStale {
		t.Fatal("expected Stop() not to report stale state")
	}
	if result.Status == nil || result.Status.PID != status.PID {
		t.Fatalf("Stop() returned status = %#v, want PID %d", result.Status, status.PID)
	}
	if _, err := os.Stat(result.StatePath); !os.IsNotExist(err) {
		t.Fatalf("expected runtime state file to be removed, stat err = %v", err)
	}
}

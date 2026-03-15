package runtimectl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ekaya-inc/ekaya-engine/pkg/servercontrol"
)

const (
	controlRequestTimeout = 3 * time.Second
	stopPollInterval      = 250 * time.Millisecond

	DefaultStopTimeout = 35 * time.Second
)

type StopResult struct {
	RemovedStale bool
	StatePath    string
	Status       *servercontrol.Status
	WasRunning   bool
}

func EnsureNotRunning(configPath string) (bool, error) {
	_, status, stale, err := ReadStatus(configPath)
	if err != nil {
		return false, err
	}

	if status != nil {
		return false, fmt.Errorf(
			"ekaya-engine is already running for %s (PID %d, base URL %s). Stop it with `ekaya-engine stop` first",
			status.ConfigPath, status.PID, status.BaseURL,
		)
	}

	if stale {
		if err := servercontrol.RemoveState(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("remove stale runtime state: %w", err)
		}
		return true, nil
	}

	return false, nil
}

func ReadStatus(configPath string) (*servercontrol.State, *servercontrol.Status, bool, error) {
	state, err := servercontrol.ReadState(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("read runtime state: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), controlRequestTimeout)
	defer cancel()

	status, err := servercontrol.GetStatus(ctx, state)
	if err != nil {
		// The state file is local, so any control-plane failure means this snapshot is no longer trustworthy.
		return state, nil, true, nil
	}

	return state, status, false, nil
}

func Stop(configPath string, timeout time.Duration) (*StopResult, error) {
	result := &StopResult{
		StatePath: servercontrol.StatePath(configPath),
	}

	state, status, stale, err := ReadStatus(configPath)
	if err != nil {
		return nil, err
	}

	if stale {
		if err := servercontrol.RemoveState(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale runtime state: %w", err)
		}
		result.RemovedStale = true
		return result, nil
	}

	if state == nil || status == nil {
		return result, nil
	}

	result.WasRunning = true
	result.Status = status

	ctx, cancel := context.WithTimeout(context.Background(), controlRequestTimeout)
	defer cancel()

	if err := servercontrol.RequestShutdown(ctx, state); err != nil {
		return nil, fmt.Errorf("request shutdown: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(result.StatePath); errors.Is(err, os.ErrNotExist) {
			return result, nil
		} else if err != nil {
			return nil, fmt.Errorf("check runtime state: %w", err)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for ekaya-engine to stop")
		}

		time.Sleep(stopPollInterval)
	}
}

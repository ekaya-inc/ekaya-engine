package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ekaya-inc/ekaya-engine/internal/runtimectl"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/servercontrol"
)

func Run(args []string, version string, serve func() error) error {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "", "serve":
		return serve()
	case "status":
		return runStatusCommand(version)
	case "stop":
		return runStopCommand(version)
	case "restart":
		return runRestartCommand(version, serve)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", command)
	}
}

func runStatusCommand(version string) error {
	cfg, err := loadResolvedConfig(version)
	if err != nil {
		return err
	}

	_, status, stale, err := runtimectl.ReadStatus(cfg.ConfigPath)
	if err != nil {
		return err
	}

	if status == nil {
		if stale {
			return fmt.Errorf("ekaya-engine is not running (stale runtime state at %s)", servercontrol.StatePath(cfg.ConfigPath))
		}
		return fmt.Errorf("ekaya-engine is not running")
	}

	fmt.Println("ekaya-engine is running")
	fmt.Printf("PID: %d\n", status.PID)
	fmt.Printf("Version: %s\n", status.Version)
	fmt.Printf("Config: %s\n", status.ConfigPath)
	fmt.Printf("Base URL: %s\n", status.BaseURL)
	fmt.Printf("Started: %s\n", status.StartedAt.Format(time.RFC3339))
	return nil
}

func runStopCommand(version string) error {
	cfg, err := loadResolvedConfig(version)
	if err != nil {
		return err
	}

	result, err := runtimectl.Stop(cfg.ConfigPath, runtimectl.DefaultStopTimeout)
	if err != nil {
		return err
	}

	switch {
	case result.WasRunning:
		fmt.Printf("Stopping ekaya-engine (PID %d)...\n", result.Status.PID)
		fmt.Println("ekaya-engine stopped")
	case result.RemovedStale:
		fmt.Printf("Removed stale runtime state at %s\n", result.StatePath)
		fmt.Println("ekaya-engine is not running")
	default:
		fmt.Println("ekaya-engine is not running")
	}

	return nil
}

func runRestartCommand(version string, serve func() error) error {
	cfg, err := loadResolvedConfig(version)
	if err != nil {
		return err
	}

	result, err := runtimectl.Stop(cfg.ConfigPath, runtimectl.DefaultStopTimeout)
	if err != nil {
		return err
	}

	switch {
	case result.WasRunning:
		fmt.Printf("Stopping ekaya-engine (PID %d)...\n", result.Status.PID)
		fmt.Println("ekaya-engine stopped")
	case result.RemovedStale:
		fmt.Printf("Removed stale runtime state at %s\n", result.StatePath)
	default:
		fmt.Println("No running ekaya-engine instance found; starting a new server")
	}

	return serve()
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ekaya-engine             Start the server")
	fmt.Fprintln(w, "  ekaya-engine serve       Start the server")
	fmt.Fprintln(w, "  ekaya-engine status      Show status for the resolved config")
	fmt.Fprintln(w, "  ekaya-engine stop        Stop the running server for the resolved config")
	fmt.Fprintln(w, "  ekaya-engine restart     Stop then start the server for the resolved config")
}

func loadResolvedConfig(version string) (*config.Config, error) {
	cfg, err := config.Load(version)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

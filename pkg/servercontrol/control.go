package servercontrol

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stateFileName = ".ekaya-engine-runtime.json"
	tokenHeader   = "X-Ekaya-Control-Token"
)

var ErrUnauthorized = errors.New("server control unauthorized")

type Status struct {
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	Version    string    `json:"version"`
	ConfigPath string    `json:"config_path"`
	BaseURL    string    `json:"base_url"`
}

type State struct {
	Status
	ControlURL string `json:"control_url"`
	Token      string `json:"token"`
}

type Server struct {
	listener net.Listener
	server   *http.Server
}

func StatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), stateFileName)
}

func ReadState(configPath string) (*State, error) {
	data, err := os.ReadFile(StatePath(configPath))
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode runtime state: %w", err)
	}

	return &state, nil
}

func WriteState(configPath string, state *State) error {
	statePath := StatePath(configPath)
	dir := filepath.Dir(statePath)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create runtime state directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".ekaya-engine-runtime-*.tmp")
	if err != nil {
		return fmt.Errorf("create runtime state temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := tmpFile.Chmod(0600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod runtime state temp file: %w", err)
	}

	enc := json.NewEncoder(tmpFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		tmpFile.Close()
		return fmt.Errorf("encode runtime state: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close runtime state temp file: %w", err)
	}

	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove previous runtime state: %w", err)
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		return fmt.Errorf("replace runtime state: %w", err)
	}

	return nil
}

func RemoveState(configPath string) error {
	return os.Remove(StatePath(configPath))
}

func GenerateToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate control token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func Start(status Status, token string, onShutdown func()) (*Server, string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("listen for control server: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(w, r, token) {
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(w, r, token) {
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		onShutdown()
		w.WriteHeader(http.StatusAccepted)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = srv.Serve(listener)
	}()

	return &Server{
		listener: listener,
		server:   srv,
	}, "http://" + listener.Addr().String(), nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func GetStatus(ctx context.Context, state *State) (*Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(state.ControlURL, "/status"), nil)
	if err != nil {
		return nil, fmt.Errorf("create control status request: %w", err)
	}
	req.Header.Set(tokenHeader, state.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := expectOK(resp); err != nil {
		return nil, err
	}

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode control status response: %w", err)
	}

	return &status, nil
}

func RequestShutdown(ctx context.Context, state *State) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(state.ControlURL, "/shutdown"), nil)
	if err != nil {
		return fmt.Errorf("create shutdown request: %w", err)
	}
	req.Header.Set(tokenHeader, state.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := expectAccepted(resp); err != nil {
		return err
	}

	return nil
}

func authorize(w http.ResponseWriter, r *http.Request, token string) bool {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(tokenHeader)), []byte(token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func expectOK(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus(resp)
	}
	return nil
}

func expectAccepted(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusAccepted {
		return unexpectedStatus(resp)
	}
	return nil
}

func unexpectedStatus(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("unexpected control response: %s", resp.Status)
	}
	return fmt.Errorf("unexpected control response: %s: %s", resp.Status, msg)
}

func joinURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + path
}

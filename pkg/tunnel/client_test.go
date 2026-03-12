package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockTunnelServer creates a test WebSocket server that simulates ekaya-tunnel.
// It accepts registration, then relays requests and receives responses.
type mockTunnelServer struct {
	t          *testing.T
	server     *httptest.Server
	mu         sync.Mutex
	conn       *websocket.Conn
	registered chan *RegisterMessage
}

func newMockTunnelServer(t *testing.T) *mockTunnelServer {
	m := &mockTunnelServer{
		t:          t,
		registered: make(chan *RegisterMessage, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel/connect", m.handleConnect)
	m.server = httptest.NewServer(mux)

	return m
}

func (m *mockTunnelServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		m.t.Logf("WebSocket accept error: %v", err)
		return
	}

	m.mu.Lock()
	m.conn = conn
	m.mu.Unlock()

	conn.SetReadLimit(10 * 1024 * 1024)

	// Read the register message
	_, data, err := conn.Read(r.Context())
	if err != nil {
		m.t.Logf("Read register error: %v", err)
		return
	}

	var reg RegisterMessage
	if err := json.Unmarshal(data, &reg); err != nil {
		m.t.Logf("Unmarshal register error: %v", err)
		return
	}

	// Send registered confirmation
	registered := RegisteredMessage{
		Type:      TypeRegistered,
		ProjectID: reg.ProjectID,
		PublicURL: fmt.Sprintf("https://mcp.ekaya.ai/mcp/%s", reg.ProjectID),
	}
	regData, _ := json.Marshal(registered)
	if err := conn.Write(r.Context(), websocket.MessageText, regData); err != nil {
		m.t.Logf("Write registered error: %v", err)
		return
	}

	m.registered <- &reg

	// Keep the connection open until context is done
	<-r.Context().Done()
}

func (m *mockTunnelServer) sendRequest(ctx context.Context, req *RequestMessage) error {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("no connection")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func (m *mockTunnelServer) readMessage(ctx context.Context) (any, error) {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("no connection")
	}

	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	return ParseMessage(data)
}

func (m *mockTunnelServer) close() {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()

	if conn != nil {
		conn.Close(websocket.StatusNormalClosure, "test done")
	}
	m.server.Close()
}

func (m *mockTunnelServer) url() string {
	return m.server.URL
}

// mockLocalMCPServer creates a local HTTP server that simulates the engine's /mcp/{pid} endpoint.
func newMockLocalMCPServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/{pid}", handler)
	return httptest.NewServer(mux)
}

func TestClient_ConnectAndRegister(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)

	client := NewClient(projectID, mockTunnel.url(), "http://localhost:3443", "test-api-key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Start client in background
	done := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(done)
	}()

	// Wait for registration
	select {
	case reg := <-mockTunnel.registered:
		assert.Equal(t, projectID.String(), reg.ProjectID)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Poll for connected status — the registered channel fires before the
	// client finishes processing the registered response and setting status.
	require.Eventually(t, func() bool {
		return client.Status() == StatusConnected
	}, 5*time.Second, 10*time.Millisecond, "expected status to become connected")

	assert.Contains(t, client.PublicURL(), projectID.String())
	assert.NotNil(t, client.ConnectedSince())

	// Clean shutdown
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for client shutdown")
	}

	assert.Equal(t, StatusDisconnected, client.Status())
}

func TestClient_RelayRequest(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	// Create a local MCP server that returns a known response
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify headers are forwarded
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Read the body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"method":"test"}`, string(body))

		// Return a response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"result":"success"}`)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "test-api-key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(done)
	}()

	// Wait for registration
	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send a request through the tunnel
	reqBody := base64.StdEncoding.EncodeToString([]byte(`{"method":"test"}`))
	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:   TypeRequest,
		ID:     "req-1",
		Method: "POST",
		Headers: map[string]string{
			"content-type": "application/json",
		},
		Body: reqBody,
	})
	require.NoError(t, err)

	// Read the response messages from the tunnel connection
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	// Read response_start
	msg, err := mockTunnel.readMessage(readCtx)
	require.NoError(t, err)
	respStart, ok := msg.(*ResponseStartMessage)
	require.True(t, ok, "expected response_start, got %T", msg)
	assert.Equal(t, "req-1", respStart.ID)
	assert.Equal(t, 200, respStart.Status)
	assert.Equal(t, "application/json", respStart.Headers["content-type"])

	// Read response chunks (may be one or more)
	var responseBody []byte
	for {
		msg, err = mockTunnel.readMessage(readCtx)
		require.NoError(t, err)

		switch m := msg.(type) {
		case *ResponseChunkMessage:
			assert.Equal(t, "req-1", m.ID)
			decoded, err := base64.StdEncoding.DecodeString(m.Data)
			require.NoError(t, err)
			responseBody = append(responseBody, decoded...)
		case *ResponseEndMessage:
			assert.Equal(t, "req-1", m.ID)
			goto done_reading
		default:
			t.Fatalf("unexpected message type: %T", msg)
		}
	}

done_reading:
	assert.Equal(t, `{"result":"success"}`, string(responseBody))
}

func TestClient_RelayStreaming(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	// Create a local MCP server that returns a streaming SSE response
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		for i := range 3 {
			fmt.Fprintf(w, "data: chunk-%d\n\n", i)
			flusher.Flush()
		}
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "test-api-key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(done)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send request
	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:    TypeRequest,
		ID:      "sse-1",
		Method:  "POST",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    base64.StdEncoding.EncodeToString([]byte(`{}`)),
	})
	require.NoError(t, err)

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	// Read response_start
	msg, err := mockTunnel.readMessage(readCtx)
	require.NoError(t, err)
	respStart, ok := msg.(*ResponseStartMessage)
	require.True(t, ok)
	assert.Equal(t, "sse-1", respStart.ID)
	assert.Equal(t, 200, respStart.Status)
	assert.Equal(t, "text/event-stream", respStart.Headers["content-type"])

	// Read chunks until response_end
	var fullBody []byte
	for {
		msg, err = mockTunnel.readMessage(readCtx)
		require.NoError(t, err)

		switch m := msg.(type) {
		case *ResponseChunkMessage:
			decoded, _ := base64.StdEncoding.DecodeString(m.Data)
			fullBody = append(fullBody, decoded...)
		case *ResponseEndMessage:
			goto done_reading
		default:
			t.Fatalf("unexpected: %T", msg)
		}
	}

done_reading:
	body := string(fullBody)
	assert.Contains(t, body, "data: chunk-0")
	assert.Contains(t, body, "data: chunk-1")
	assert.Contains(t, body, "data: chunk-2")
}

func TestClient_HeaderForwarding(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	// Track what headers the local server receives
	var receivedHeaders http.Header
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "test-api-key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(done)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send a request with Authorization and X-Forwarded headers
	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:   TypeRequest,
		ID:     "auth-1",
		Method: "POST",
		Headers: map[string]string{
			"content-type":      "application/json",
			"authorization":     "Bearer jwt-token-123",
			"x-forwarded-proto": "https",
			"x-forwarded-host":  "mcp.ekaya.ai",
		},
		Body: base64.StdEncoding.EncodeToString([]byte(`{}`)),
	})
	require.NoError(t, err)

	// Wait for the response to come back
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	// Drain all response messages
	for {
		msg, err := mockTunnel.readMessage(readCtx)
		if err != nil {
			break
		}
		if _, ok := msg.(*ResponseEndMessage); ok {
			break
		}
	}

	// Verify headers were forwarded to the local server
	require.NotNil(t, receivedHeaders)
	assert.Equal(t, "Bearer jwt-token-123", receivedHeaders.Get("Authorization"))
	assert.Equal(t, "https", receivedHeaders.Get("X-Forwarded-Proto"))
	assert.Equal(t, "mcp.ekaya.ai", receivedHeaders.Get("X-Forwarded-Host"))
}

func TestClient_DefaultForwardedHeaders(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	var receivedHeaders http.Header
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	// Use a tunnel URL with a known host — tunnelHost is set at construction time
	client := NewClient(projectID, "https://mcp.ekaya.ai", localServer.URL, "test-api-key", ClientConfig{}, logger)
	// Override the connection URL but tunnelHost retains "mcp.ekaya.ai"
	client.tunnelURL = mockTunnel.url()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneC := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(doneC)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send request WITHOUT X-Forwarded headers — client should add defaults
	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:    TypeRequest,
		ID:      "default-1",
		Method:  "POST",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    base64.StdEncoding.EncodeToString([]byte(`{}`)),
	})
	require.NoError(t, err)

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	for {
		msg, err := mockTunnel.readMessage(readCtx)
		if err != nil {
			break
		}
		if _, ok := msg.(*ResponseEndMessage); ok {
			break
		}
	}

	require.NotNil(t, receivedHeaders)
	// Should default to https and the tunnel host (extracted at construction, not from overridden tunnelURL)
	assert.Equal(t, "https", receivedHeaders.Get("X-Forwarded-Proto"))
	assert.Equal(t, "mcp.ekaya.ai", receivedHeaders.Get("X-Forwarded-Host"))
}

func TestClient_AgentAPIKeyFallback(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	var receivedHeaders http.Header
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "my-agent-key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneC := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(doneC)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send request WITHOUT Authorization header — client should use agent API key
	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:    TypeRequest,
		ID:      "agent-1",
		Method:  "POST",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    base64.StdEncoding.EncodeToString([]byte(`{}`)),
	})
	require.NoError(t, err)

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	for {
		msg, err := mockTunnel.readMessage(readCtx)
		if err != nil {
			break
		}
		if _, ok := msg.(*ResponseEndMessage); ok {
			break
		}
	}

	require.NotNil(t, receivedHeaders)
	assert.Equal(t, "Bearer my-agent-key", receivedHeaders.Get("Authorization"))
}

func TestClient_LocalServerError(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	// Local server returns 500
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"internal error"}`)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "test-key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneC := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(doneC)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:    TypeRequest,
		ID:      "err-1",
		Method:  "POST",
		Headers: map[string]string{},
		Body:    base64.StdEncoding.EncodeToString([]byte(`{}`)),
	})
	require.NoError(t, err)

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	// Should still get response_start with 500 status
	msg, err := mockTunnel.readMessage(readCtx)
	require.NoError(t, err)
	respStart, ok := msg.(*ResponseStartMessage)
	require.True(t, ok)
	assert.Equal(t, 500, respStart.Status)

	// Read remaining messages
	for {
		msg, err = mockTunnel.readMessage(readCtx)
		if err != nil {
			break
		}
		if _, ok := msg.(*ResponseEndMessage); ok {
			break
		}
	}
}

func TestClient_Reconnection(t *testing.T) {
	// Create a server that accepts one connection then closes it
	connCount := 0
	var mu sync.Mutex
	registered := make(chan struct{}, 5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}

		mu.Lock()
		connCount++
		count := connCount
		mu.Unlock()

		conn.SetReadLimit(10 * 1024 * 1024)

		// Read register
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var reg RegisterMessage
		json.Unmarshal(data, &reg)

		// Send registered
		resp := RegisteredMessage{Type: TypeRegistered, ProjectID: reg.ProjectID, PublicURL: "https://mcp.ekaya.ai/mcp/" + reg.ProjectID}
		respData, _ := json.Marshal(resp)
		conn.Write(r.Context(), websocket.MessageText, respData)

		registered <- struct{}{}

		if count == 1 {
			// Close the first connection after a short delay
			time.Sleep(100 * time.Millisecond)
			conn.Close(websocket.StatusGoingAway, "test disconnect")
			return
		}

		// Keep second connection open
		<-r.Context().Done()
	}))
	defer server.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, server.URL, "http://localhost:3443", "key", ClientConfig{}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(done)
	}()

	// Wait for first registration
	select {
	case <-registered:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for first registration")
	}

	// Wait for reconnection (second registration)
	select {
	case <-registered:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for reconnection")
	}

	mu.Lock()
	assert.GreaterOrEqual(t, connCount, 2, "Expected at least 2 connections (reconnection)")
	mu.Unlock()

	cancel()
	<-done
}

func TestClient_Stop(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), "http://localhost:3443", "key", ClientConfig{}, logger)

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(done)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	require.Eventually(t, func() bool {
		return client.Status() == StatusConnected
	}, 5*time.Second, 10*time.Millisecond, "expected status to become connected")

	// Stop should complete quickly
	client.Stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for Stop to complete")
	}

	assert.Equal(t, StatusDisconnected, client.Status())
}

func TestClient_ConcurrentRequests(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	// Local server that adds a delay
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // Simulate work
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneC := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(doneC)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send 5 concurrent requests
	numRequests := 5
	for i := range numRequests {
		err := mockTunnel.sendRequest(ctx, &RequestMessage{
			Type:    TypeRequest,
			ID:      fmt.Sprintf("concurrent-%d", i),
			Method:  "POST",
			Headers: map[string]string{},
			Body:    base64.StdEncoding.EncodeToString([]byte(`{}`)),
		})
		require.NoError(t, err)
	}

	// Collect all response_end messages
	readCtx, readCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readCancel()

	endCount := 0
	seenIDs := make(map[string]bool)
	for endCount < numRequests {
		msg, err := mockTunnel.readMessage(readCtx)
		require.NoError(t, err)

		if end, ok := msg.(*ResponseEndMessage); ok {
			seenIDs[end.ID] = true
			endCount++
		}
	}

	assert.Equal(t, numRequests, endCount, "Should receive all response_end messages")
	for i := range numRequests {
		assert.True(t, seenIDs[fmt.Sprintf("concurrent-%d", i)], "Missing response for request %d", i)
	}
}

func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		attempt int
		minSec  float64
		maxSec  float64
	}{
		{1, 0.5, 1.5},    // ~1s ±25%
		{2, 1.5, 2.5},    // ~2s ±25%
		{3, 3.0, 5.0},    // ~4s ±25%
		{7, 45.0, 75.0},  // capped at ~60s ±25%
		{10, 45.0, 75.0}, // still capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			d := backoffDuration(tt.attempt)
			sec := d.Seconds()
			assert.GreaterOrEqual(t, sec, tt.minSec, "backoff too low for attempt %d", tt.attempt)
			assert.LessOrEqual(t, sec, tt.maxSec, "backoff too high for attempt %d", tt.attempt)
		})
	}
}

func TestClient_InvalidRequestBody(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)
	client := NewClient(projectID, mockTunnel.url(), localServer.URL, "key", ClientConfig{}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneC := make(chan struct{})
	go func() {
		client.Start(ctx)
		close(doneC)
	}()

	select {
	case <-mockTunnel.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for registration")
	}

	// Send a request with invalid base64 body
	err := mockTunnel.sendRequest(ctx, &RequestMessage{
		Type:    TypeRequest,
		ID:      "bad-body-1",
		Method:  "POST",
		Headers: map[string]string{},
		Body:    "not-valid-base64!!!",
	})
	require.NoError(t, err)

	// Should receive an error message back
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	msg, err := mockTunnel.readMessage(readCtx)
	require.NoError(t, err)
	errMsg, ok := msg.(*ErrorMessage)
	require.True(t, ok, "expected error message, got %T", msg)
	assert.Equal(t, "bad-body-1", errMsg.ID)
	assert.Contains(t, errMsg.Message, "decode request body")
}

func TestClient_StatusTransitions(t *testing.T) {
	client := &Client{
		status: StatusDisconnected,
	}

	assert.Equal(t, StatusDisconnected, client.Status())

	client.setStatus(StatusConnecting)
	assert.Equal(t, StatusConnecting, client.Status())
	assert.Nil(t, client.ConnectedSince())

	client.setStatus(StatusConnected)
	// Note: connectedSince is only set in connectAndServe, not setStatus
	assert.Equal(t, StatusConnected, client.Status())

	client.setStatus(StatusReconnecting)
	assert.Equal(t, StatusReconnecting, client.Status())
	assert.Nil(t, client.ConnectedSince())
}

func TestClient_XForwardedHostExtraction(t *testing.T) {
	mockTunnel := newMockTunnelServer(t)
	defer mockTunnel.close()

	var receivedHost string
	localServer := newMockLocalMCPServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Header.Get("X-Forwarded-Host")
		w.WriteHeader(http.StatusOK)
	})
	defer localServer.Close()

	projectID := uuid.New()
	logger := zaptest.NewLogger(t)

	// Test with various tunnel URL formats — tunnelHost is set at construction
	tests := []struct {
		tunnelURL    string
		expectedHost string
	}{
		{"https://mcp.ekaya.ai", "mcp.ekaya.ai"},
		{"http://mcp.dev.ekaya.ai", "mcp.dev.ekaya.ai"},
	}

	for _, tt := range tests {
		t.Run(tt.tunnelURL, func(t *testing.T) {
			client := NewClient(projectID, tt.tunnelURL, localServer.URL, "key", ClientConfig{}, logger)
			// Override connection URL; tunnelHost retains the original host
			client.tunnelURL = mockTunnel.url()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			doneC := make(chan struct{})
			go func() {
				client.Start(ctx)
				close(doneC)
			}()

			select {
			case <-mockTunnel.registered:
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for registration")
			}

			receivedHost = ""

			err := mockTunnel.sendRequest(ctx, &RequestMessage{
				Type:    TypeRequest,
				ID:      "host-1",
				Method:  "POST",
				Headers: map[string]string{},
				Body:    base64.StdEncoding.EncodeToString([]byte(`{}`)),
			})
			require.NoError(t, err)

			readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
			defer readCancel()
			for {
				msg, err := mockTunnel.readMessage(readCtx)
				if err != nil {
					break
				}
				if _, ok := msg.(*ResponseEndMessage); ok {
					break
				}
			}

			assert.Equal(t, tt.expectedHost, receivedHost)

			cancel()
			<-doneC
		})
	}
}

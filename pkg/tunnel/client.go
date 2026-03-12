package tunnel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TunnelStatus represents the current state of a tunnel connection.
type TunnelStatus string

const (
	StatusDisconnected TunnelStatus = "disconnected"
	StatusConnecting   TunnelStatus = "connecting"
	StatusConnected    TunnelStatus = "connected"
	StatusReconnecting TunnelStatus = "reconnecting"
)

// ClientConfig holds configuration for a tunnel client.
type ClientConfig struct {
	// TLSCertFile is the client TLS certificate for mTLS (Phase 3, empty for Phase 1).
	TLSCertFile string
	// TLSKeyFile is the client TLS private key for mTLS (Phase 3, empty for Phase 1).
	TLSKeyFile string
}

// Client maintains a persistent WebSocket connection to the ekaya-tunnel relay server
// for a single project. It receives relayed MCP requests and forwards them to the
// local MCP handler via internal HTTP requests.
type Client struct {
	projectID    uuid.UUID
	tunnelURL    string // base URL of the tunnel server (e.g., https://mcp.ekaya.ai)
	tunnelHost   string // hostname extracted from tunnelURL for X-Forwarded-Host default
	localBaseURL string // base URL of the local engine (e.g., http://127.0.0.1:3443)
	agentAPIKey  string // API key for authenticating local MCP requests
	config       ClientConfig
	logger       *zap.Logger
	httpClient   *http.Client

	mu             sync.RWMutex
	status         TunnelStatus
	publicURL      string
	connectedSince *time.Time
	cancel         context.CancelFunc
	done           chan struct{}
}

// NewClient creates a new tunnel client for a project.
func NewClient(
	projectID uuid.UUID,
	tunnelURL string,
	localBaseURL string,
	agentAPIKey string,
	config ClientConfig,
	logger *zap.Logger,
) *Client {
	// Extract the host from the tunnel URL for X-Forwarded-Host default.
	host := strings.TrimRight(tunnelURL, "/")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	return &Client{
		projectID:    projectID,
		tunnelURL:    strings.TrimRight(tunnelURL, "/"),
		tunnelHost:   host,
		localBaseURL: strings.TrimRight(localBaseURL, "/"),
		agentAPIKey:  agentAPIKey,
		config:       config,
		logger:       logger.With(zap.String("project_id", projectID.String())),
		httpClient:   &http.Client{
			// No timeout on the client — individual requests use context deadlines.
			// Streaming MCP responses (SSE) can be long-lived.
		},
		status: StatusDisconnected,
	}
}

// Start begins the tunnel connection loop. It connects to the tunnel server,
// registers, and enters the message relay loop. On disconnection, it reconnects
// with exponential backoff. Blocks until ctx is cancelled.
func (c *Client) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.done = make(chan struct{})
	c.mu.Unlock()

	defer func() {
		c.setStatus(StatusDisconnected)
		close(c.done)
	}()

	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}

		if attempt == 0 {
			c.setStatus(StatusConnecting)
		} else {
			c.setStatus(StatusReconnecting)
		}

		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return // context cancelled, clean shutdown
		}

		c.setStatus(StatusReconnecting)
		attempt++

		// Exponential backoff with jitter: 1s, 2s, 4s, ..., 60s max
		backoff := backoffDuration(attempt)
		c.logger.Warn("Tunnel disconnected, reconnecting",
			zap.Error(err),
			zap.Int("attempt", attempt),
			zap.Duration("backoff", backoff),
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// Stop gracefully shuts down the tunnel client.
func (c *Client) Stop() {
	c.mu.RLock()
	cancel := c.cancel
	done := c.done
	c.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Status returns the current tunnel status.
func (c *Client) Status() TunnelStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// PublicURL returns the public URL assigned by the tunnel server.
func (c *Client) PublicURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.publicURL
}

// ConnectedSince returns when the current connection was established.
func (c *Client) ConnectedSince() *time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectedSince
}

func (c *Client) setStatus(s TunnelStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
	if s != StatusConnected {
		c.connectedSince = nil
	}
}

// connectAndServe establishes a single WebSocket connection, registers, and
// handles messages until the connection drops or context is cancelled.
func (c *Client) connectAndServe(ctx context.Context) error {
	wsURL := c.tunnelURL + "/tunnel/connect"

	// TODO (Phase 3): Configure tls.Config with client certificates here
	// when c.config.TLSCertFile and c.config.TLSKeyFile are set.
	// The ALB validates the client cert against a TrustConfig containing
	// the Ekaya CA certificate.

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to tunnel server: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "shutting down")

	// Disable the default read limit for large MCP request bodies
	conn.SetReadLimit(10 * 1024 * 1024) // 10MB

	c.logger.Info("WebSocket connection established (unauthenticated Phase 1)",
		zap.String("tunnel_url", wsURL),
	)

	// Send register message
	regMsg := RegisterMessage{
		Type:      TypeRegister,
		ProjectID: c.projectID.String(),
	}
	regData, err := json.Marshal(regMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal register message: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, regData); err != nil {
		return fmt.Errorf("failed to send register message: %w", err)
	}

	// Wait for registered confirmation
	_, data, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read registered response: %w", err)
	}

	msg, err := ParseMessage(data)
	if err != nil {
		return fmt.Errorf("failed to parse registered response: %w", err)
	}

	registered, ok := msg.(*RegisteredMessage)
	if !ok {
		return fmt.Errorf("expected registered message, got %T", msg)
	}

	now := time.Now()
	c.mu.Lock()
	c.status = StatusConnected
	c.publicURL = registered.PublicURL
	c.connectedSince = &now
	c.mu.Unlock()

	c.logger.Info("Tunnel registered",
		zap.String("public_url", registered.PublicURL),
	)

	// Enter message relay loop
	return c.messageLoop(ctx, conn)
}

// messageLoop reads messages from the tunnel server and handles them.
func (c *Client) messageLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("websocket read error: %w", err)
		}

		msg, err := ParseMessage(data)
		if err != nil {
			c.logger.Error("Failed to parse tunnel message", zap.Error(err))
			continue
		}

		switch m := msg.(type) {
		case *RequestMessage:
			// Handle each request concurrently to support multiplexing
			go c.handleRequest(ctx, conn, m)
		case *ErrorMessage:
			c.logger.Error("Received error from tunnel server",
				zap.String("request_id", m.ID),
				zap.String("message", m.Message),
			)
		default:
			c.logger.Warn("Unexpected message type from tunnel server",
				zap.String("type", fmt.Sprintf("%T", msg)),
			)
		}
	}
}

// handleRequest processes a single relayed MCP request by forwarding it to the
// local MCP handler and streaming the response back over the WebSocket.
func (c *Client) handleRequest(ctx context.Context, conn *websocket.Conn, req *RequestMessage) {
	logger := c.logger.With(zap.String("request_id", req.ID))

	// Decode the base64-encoded request body
	body, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		logger.Error("Failed to decode request body", zap.Error(err))
		c.sendError(ctx, conn, req.ID, "failed to decode request body")
		return
	}

	// Build the local HTTP request to POST /mcp/{pid}
	localURL := fmt.Sprintf("%s/mcp/%s", c.localBaseURL, c.projectID.String())
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, localURL, strings.NewReader(string(body)))
	if err != nil {
		logger.Error("Failed to create local HTTP request", zap.Error(err))
		c.sendError(ctx, conn, req.ID, "failed to create local request")
		return
	}

	// Forward headers from the relay message to the local HTTP request.
	// This includes the Authorization header for defense-in-depth JWT validation.
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Ensure forwarded proxy headers are set so the engine knows the public origin.
	if httpReq.Header.Get("X-Forwarded-Proto") == "" {
		httpReq.Header.Set("X-Forwarded-Proto", "https")
	}
	if httpReq.Header.Get("X-Forwarded-Host") == "" {
		httpReq.Header.Set("X-Forwarded-Host", c.tunnelHost)
	}

	// If no Authorization header was forwarded, use the agent API key
	// for tunnel-internal authentication.
	if httpReq.Header.Get("Authorization") == "" && c.agentAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.agentAPIKey)
	}

	// Execute the local HTTP request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		logger.Error("Local MCP request failed", zap.Error(err))
		c.sendError(ctx, conn, req.ID, "local MCP request failed")
		return
	}
	defer resp.Body.Close()

	// Send response_start with status and headers
	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[strings.ToLower(k)] = resp.Header.Get(k)
	}

	startMsg := ResponseStartMessage{
		Type:    TypeResponseStart,
		ID:      req.ID,
		Status:  resp.StatusCode,
		Headers: respHeaders,
	}
	if err := c.writeJSON(ctx, conn, startMsg); err != nil {
		logger.Error("Failed to send response_start", zap.Error(err))
		return
	}

	// Stream the response body in chunks
	buf := make([]byte, 8192)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunkMsg := ResponseChunkMessage{
				Type: TypeResponseChunk,
				ID:   req.ID,
				Data: base64.StdEncoding.EncodeToString(buf[:n]),
			}
			if err := c.writeJSON(ctx, conn, chunkMsg); err != nil {
				logger.Error("Failed to send response_chunk", zap.Error(err))
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				logger.Error("Error reading local response body", zap.Error(readErr))
			}
			break
		}
	}

	// Send response_end
	endMsg := ResponseEndMessage{
		Type: TypeResponseEnd,
		ID:   req.ID,
	}
	if err := c.writeJSON(ctx, conn, endMsg); err != nil {
		logger.Error("Failed to send response_end", zap.Error(err))
	}
}

// sendError sends an error message back to the tunnel server for a specific request.
func (c *Client) sendError(ctx context.Context, conn *websocket.Conn, requestID, message string) {
	errMsg := ErrorMessage{
		Type:    TypeError,
		ID:      requestID,
		Message: message,
	}
	if err := c.writeJSON(ctx, conn, errMsg); err != nil {
		c.logger.Error("Failed to send error message", zap.Error(err))
	}
}

// writeJSON marshals a message to JSON and writes it to the WebSocket connection.
func (c *Client) writeJSON(ctx context.Context, conn *websocket.Conn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// backoffDuration calculates exponential backoff with jitter.
// Base: 1s, max: 60s, jitter: ±25%.
func backoffDuration(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt-1)) // 1, 2, 4, 8, ...
	seconds := math.Min(base, 60)
	// Add ±25% jitter
	jitter := seconds * 0.25 * (2*rand.Float64() - 1) //nolint:gosec
	return time.Duration((seconds + jitter) * float64(time.Second))
}

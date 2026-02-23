# PLAN: Add MCP Tunnel Application to ekaya-engine

**Status:** Complete
**Supersedes:** `WISHLIST-app-public-address.md`
**Architecture:** See `../ekaya-tunnel/plans/DESIGN-tunnel-architecture.md`

## Goal

Add a new "MCP Tunnel" engine application (`mcp-tunnel`) that, when installed and activated, establishes a persistent outbound WebSocket connection from the on-prem ekaya-engine to the ekaya-tunnel service (hosted at `mcp.ekaya.ai`). This gives the project's MCP server a public URL (`https://mcp.ekaya.ai/mcp/{project-uuid}`) accessible from outside the firewall without any TLS/cert configuration by the user.

## Background

ekaya-engine runs on-premises behind corporate firewalls. Currently, MCP clients must have direct network access to the engine's `POST /mcp/{pid}` endpoint. This limits adoption for users who cannot expose ports or configure TLS certificates.

The tunnel solves this by reversing the connection direction: the engine initiates an outbound WebSocket connection to a cloud-hosted relay (ekaya-tunnel behind a Global External ALB at `mcp.ekaya.ai`), and external MCP clients connect to the relay's public URL. The relay forwards requests through the WebSocket to the engine and streams responses back.

### Architecture

```
External MCP Client
        │
        │  POST https://mcp.ekaya.ai/mcp/{project-uuid}
        │  (JWT validated by tunnel pod against dev+prod JWKS)
        ▼
┌─────────────────────┐
│  Global External    │
│  Application LB     │  (mcp.ekaya.ai, TLS termination)
│  (no mTLS Phase 1)  │  (mTLS via TrustConfig in Phase 3)
└────────┬────────────┘
         │
         ▼
┌─────────────────────┐         ┌───────────┐
│   ekaya-tunnel      │────────▶│ Firestore │
│   (GKE pod)         │◀────────│ (registry)│
│   single pod v1     │         └───────────┘
└────────┬────────────┘
         │  WebSocket (outbound from engine, unauthenticated Phase 1)
         │  wss://mcp.ekaya.ai/tunnel/connect
         ▼
┌─────────────────────┐
│   ekaya-engine      │  (on-prem, behind firewall)
│   MCP server        │
└─────────────────────┘
```

**Phase 1 deploys a single GKE pod** with an in-memory tunnel registry. Firestore is added when scaling to multiple pods.

### Protocol: WebSocket-based Request/Response Relay

Why WebSocket over raw TCP (localtunnel-style):
- **Firewall-friendly** — outbound WSS (port 443) is rarely blocked
- **No connection timeout** — GKE pods have no request timeout limit (unlike Cloud Run's 1-hour max)
- **Multiplexed** — multiple concurrent MCP requests share one WebSocket connection
- **Simpler** — no TCP connection pool management needed

**Message format** (JSON over WebSocket):

```
Engine → Tunnel (register):
{
  "type": "register",
  "project_id": "uuid"
}
// Phase 1: Unauthenticated. Engine self-identifies with project ID.
// Phase 2: Adds tunnel_secret for validation against ekaya-central.
// Phase 3: mTLS client cert (CN=project-uuid) validated at ALB layer.

Tunnel → Engine (registered):
{
  "type": "registered",
  "project_id": "uuid",
  "public_url": "https://mcp.ekaya.ai/mcp/{project-uuid}"
}

Tunnel → Engine (request):
{
  "type": "request",
  "id": "req-uuid",
  "method": "POST",
  "headers": {"content-type": "application/json", ...},
  "body": "<base64-encoded>"
}

Engine → Tunnel (response header):
{
  "type": "response_start",
  "id": "req-uuid",
  "status": 200,
  "headers": {"content-type": "text/event-stream", ...}
}

Engine → Tunnel (response chunk — supports streaming):
{
  "type": "response_chunk",
  "id": "req-uuid",
  "data": "<base64-encoded-chunk>"
}

Engine → Tunnel (response end):
{
  "type": "response_end",
  "id": "req-uuid"
}
```

Streaming is essential because MCP over HTTP Streaming returns Server-Sent Events (SSE) — responses are chunked and long-lived. The architecture must not assume MCP requests are always isolated request/response pairs — future MCP protocol versions may support persistent connections.

### Reconnection Behavior

The engine's tunnel client reconnects indefinitely on disconnection. If the tunnel pod restarts, the engine will automatically reconnect and re-register. No persistent state is needed on either side.

### Billing

The mcp-tunnel app follows the same install → activate (billing via ekaya-central redirect) → active lifecycle as existing apps (ai-data-liaison, ai-agents). No billing-specific code is needed in the engine beyond registering the app ID.

## Scope

Changes are limited to dvx-ekaya-engine. The tunnel server (ekaya-tunnel) and infrastructure (ekaya-infra) are covered by separate plans.

## Tasks

### 1. Register the mcp-tunnel application
- [x] Add `AppIDMCPTunnel = "mcp-tunnel"` constant in `pkg/models/installed_app.go`
- [x] Add `AppIDMCPTunnel: true` to `KnownAppIDs` map

This is all that's needed for install/activate/uninstall — the existing `InstalledAppService`, handler routes, and central client integration handle everything generically.

### 2. Create the tunnel client package

Create `pkg/tunnel/client.go` — the WebSocket client that maintains the connection to ekaya-tunnel.

**Responsibilities:**
- Connect to `wss://{tunnel-server-url}/tunnel/connect`
- Send a `register` message with the project ID (no auth for Phase 1)
- Wait for `registered` confirmation from the tunnel server
- Receive incoming MCP request messages from the tunnel server
- Forward them to the local MCP handler (by making an internal HTTP request to `POST /mcp/{pid}`)
- Stream the MCP response back over the WebSocket as chunked messages
- Reconnect automatically on disconnection with **exponential backoff and jitter** (1s → 2s → 4s → ... → 60s max, with random jitter)
- **Retry indefinitely** — never give up. If the tunnel server is down, keep trying.
- Send periodic WebSocket ping to keep the connection alive
- Expose status (connected/disconnected/reconnecting) for the settings API

**Key design decisions:**
- The tunnel client does NOT call the MCP Go library directly — it makes a local HTTP request to the engine's own `/mcp/{pid}` endpoint. This ensures all existing middleware (auth, logging, audit) still applies.
- For the local HTTP request, the tunnel client uses the project's agent API key (from `engine_mcp_config.agent_api_key_encrypted`) or generates a tunnel-internal one. This keeps the MCP auth middleware unchanged.
- The tunnel client reads the response body in streaming fashion (chunked reads) and forwards chunks immediately over the WebSocket.

**Header forwarding (defense-in-depth JWT validation):**

The tunnel server validates JWTs before relaying requests, but the engine must also validate as defense in depth. This works automatically because the tunnel client makes a local HTTP request through the engine's existing auth middleware. The tunnel client must forward these headers from the relay message to the local HTTP request:
- `Authorization: Bearer <token>` — the original JWT from the MCP client, so the engine's auth middleware validates it independently
- `X-Forwarded-Proto: https` — so `buildBaseURL()` constructs correct public-facing URLs
- `X-Forwarded-Host: mcp.[dev.]ekaya.ai` — so OAuth discovery URLs reference the tunnel's public hostname, not the engine's internal hostname

**mTLS preparation (TODO — do not implement):**
- [x] Add `TLSCertFile` and `TLSKeyFile` fields to the tunnel client config struct (empty for Phase 1)
- [x] Add a comment block in the connection code where `tls.Config` with client certificates would be configured
- [ ] In Phase 3, ekaya-central issues a per-project client cert (CN=project-uuid) signed by the Ekaya CA. The engine presents this cert during the TLS handshake to the ALB, which validates it against a TrustConfig. The project identity is passed to the tunnel pod via `X-Client-Cert-Subject-Dn` header.

**Interface:**
```go
type TunnelClient interface {
    Start(ctx context.Context, projectID uuid.UUID, tunnelServerURL string) error
    Stop()
    Status() TunnelStatus  // "connected", "disconnected", "reconnecting"
}
```

### 3. Create the tunnel manager

Create `pkg/tunnel/manager.go` — manages tunnel clients across all projects that have the mcp-tunnel app activated.

**Responsibilities:**
- On engine startup, query all projects with mcp-tunnel activated and start a tunnel client for each
- Listen for app install/activate/uninstall events to start/stop tunnel clients dynamically
- Provide status for all active tunnels (for the settings API / UI)
- Graceful shutdown: close all WebSocket connections on engine shutdown

**Interface:**
```go
type TunnelManager interface {
    Start(ctx context.Context) error                    // Start tunnels for all activated projects
    StartTunnel(ctx context.Context, projectID uuid.UUID) error
    StopTunnel(projectID uuid.UUID)
    Status(projectID uuid.UUID) TunnelStatus
    Shutdown()
}
```

**Dependencies:**
- `InstalledAppRepository` — to list projects with mcp-tunnel activated
- `MCPConfigService` — to get agent API keys for local MCP requests
- `Config` — tunnel server URL from engine config
- `Logger`

### 4. Add tunnel server URL to engine config

- [x] Add `tunnel_server_url` field to `pkg/config/config.go` (`Config` struct)
- [x] Default: `"https://mcp.ekaya.ai"` (prod), configurable via YAML/env var `TUNNEL_SERVER_URL`
- [x] Add `tunnel_tls_cert_file` and `tunnel_tls_key_file` fields (empty strings for Phase 1, placeholder for mTLS)
- [x] Add to `config.yaml.example`

### 5. Wire tunnel manager into main.go

- [x] Create `TunnelManager` in `main.go` after services are initialized
- [x] Call `tunnelManager.Start(ctx)` during server startup (after DB migrations)
- [x] Call `tunnelManager.Shutdown()` during graceful shutdown
- [x] No new HTTP routes needed — the tunnel is outbound-only from the engine

### 6. Hook tunnel lifecycle into app install/activate/uninstall

- [x] After a successful `mcp-tunnel` activation (in the callback handler or service layer), call `tunnelManager.StartTunnel(projectID)`
- [x] After a successful `mcp-tunnel` uninstall, call `tunnelManager.StopTunnel(projectID)`
- [x] Approach: Add a post-action hook/callback mechanism to `InstalledAppService` or have the handler call the tunnel manager directly

### 7. Add tunnel status to the settings/status API

- [x] Add a `GET /api/projects/{pid}/apps/mcp-tunnel/status` endpoint (or include in the app's settings response) that returns:
  ```json
  {
    "tunnel_status": "connected",
    "public_url": "https://mcp.ekaya.ai/mcp/{project-uuid}",
    "connected_since": "2025-02-20T10:30:00Z"
  }
  ```
- [x] This gives the UI and user visibility into the tunnel state

### 8. Ensure agent API key exists for tunneled requests

When the tunnel app is activated, the engine needs an agent API key to authenticate the local MCP requests it forwards from the tunnel. If one doesn't already exist:

- [x] During tunnel activation, check if the project has an agent API key in `engine_mcp_config`
- [x] If not, auto-generate one and save it
- [x] The tunnel client uses this key for its internal `POST /mcp/{pid}` requests

### 9. Add tests

- [x] Unit tests for `pkg/tunnel/client.go` — WebSocket message serialization, reconnection logic
- [x] Unit tests for `pkg/tunnel/manager.go` — start/stop lifecycle, multi-project management
- [x] Integration test with a mock WebSocket tunnel server
- [ ] Test that a relayed request with a valid JWT is accepted (verifies auth middleware runs on the local HTTP request)
- [ ] Test that a relayed request with an invalid/expired JWT is rejected
- [ ] Test that a relayed request with no Authorization header is rejected
- [ ] Test that a relayed request with a project ID mismatch (JWT project != URL project) is rejected
- [ ] Test that well-known endpoints (`/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource`) return correct URLs when `X-Forwarded-Host` is `mcp.ekaya.ai` vs the engine's internal hostname

## Out of Scope

- UI changes (covered by frontend plan when needed)
- Tunnel server implementation (see `../ekaya-tunnel/plans/PLAN-add-tunnel-server.md`)
- Infrastructure deployment (see `../ekaya-infra/plans/PLAN-add-tunnel-infra.md`)
- Rate limiting or bandwidth throttling (future enhancement)
- Multiple concurrent WebSocket connections per project (single connection is sufficient)
- mTLS implementation (config fields stubbed, implementation in Phase 3)
- Tunnel Secret validation (Phase 2)
- WebSocket connection authentication (Phase 1 is unauthenticated — secured via mTLS in Phase 3)

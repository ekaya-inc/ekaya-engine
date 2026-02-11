# PLAN: MCP Server Accessible Checklist Item

## Status: IMPLEMENTED

## Problem

The AI Data Liaison page shows a "Share with Business Users" section with the MCP Server URL. When running on `localhost`, this URL is `http://localhost:3443/mcp/{project-id}` — which business users cannot reach. The data engineer needs clear guidance that:

1. The server must be reachable from business users' machines (not localhost)
2. HTTPS is required (OAuth PKCE uses Web Crypto API, which requires "secure contexts")
3. The server URL stored in ekaya-central must be updated to match

Currently there is no indication on the AI Data Liaison page that `localhost` is a problem for business users.

## Solution

Replace the "AI Data Liaison ready" checklist item with **"MCP Server accessible"** on the AI Data Liaison Setup Checklist (the existing checklist UI already shows an overall "complete" state when all items are done, so a separate "ready" item is redundant). When the server URL is `localhost` or `http://` (non-secure), this item shows as pending with a **[Configure]** button that navigates to a new configuration guide page within ekaya-engine.

The configuration guide page:
- Explains the three steps: (1) get a domain + cert, (2) update `config.yaml`, (3) restart the server
- Provides a "Verify" button that checks connectivity
- When the server is properly configured with HTTPS on a real domain, ekaya-engine calls ekaya-central's `PATCH /projects/:projectId` API to update the `serverUrl`

## Architecture Context

### How URLs flow through the system

1. **ekaya-engine** reads `base_url` from `config.yaml` (or auto-derives from `tls_cert_path` + port) at startup
2. **Frontend** fetches `base_url` from `GET /api/config/auth` → `ConfigResponse.BaseURL`
3. **MCP config** builds the MCP URL as `{baseURL}/mcp/{projectID}` in `pkg/services/mcp_config.go:341`
4. **ekaya-central** stores `serverUrl` per project in Firestore, used for redirects and MCP setup links

### Existing patterns

- `pkg/central/client.go` has `ProvisionProject()` and `GetProject()` — talks to ekaya-central using the user's JWT (from `papi` claim)
- ekaya-central's `PATCH /projects/:projectId` accepts `serverUrl` updates, requires admin role JWT
- `config.yaml.example` already documents TLS config: `base_url`, `tls_cert_path`, `tls_key_path`
- `certs/README.md` has self-signed cert generation instructions
- `plans/FIX-helpful-tls-error-messages.md` documents `NODE_EXTRA_CA_CERTS` for Node.js clients

### Key constraint

If the admin misconfigures TLS and the server crashes, they cannot view the configuration guide page. This is an accepted tradeoff — `config.yaml` changes are made before restarting, so the admin can revert if the server fails to start.

## Implementation Plan

### Task 1: Backend — Add `UpdateServerUrl` to central client

**Files:**
- `pkg/central/client.go` — Add `UpdateServerUrl(ctx, baseURL, projectID, serverUrl, token)` method

**Details:**
- Calls `PATCH /api/v1/projects/{projectID}` on ekaya-central with `{ "serverUrl": newURL }`
- Uses the admin user's JWT (from request context, same pattern as `ProvisionProject`)
- Returns the updated project info or error

### Task 2: Backend — Add server accessibility status endpoint

**Files:**
- `pkg/handlers/config.go` — Add `GetServerStatus` endpoint or extend existing `ConfigResponse`

**Details:**
- New endpoint: `GET /api/config/server-status` (authenticated, admin-only)
- Returns:
  ```json
  {
    "base_url": "http://localhost:3443",
    "is_localhost": true,
    "is_https": false,
    "accessible_for_business_users": false
  }
  ```
- Logic: `accessible_for_business_users` is `true` when `!is_localhost && is_https`
- This gives the frontend what it needs to show the checklist item status

### Task 3: Backend — Add endpoint to sync server URL to ekaya-central

**Files:**
- `pkg/handlers/projects.go` (or new `pkg/handlers/server_config.go`) — Add sync endpoint
- `pkg/services/projects.go` — Add service method

**Details:**
- New endpoint: `POST /api/projects/{pid}/sync-server-url` (authenticated, admin-only)
- Reads `config.BaseURL` from the running server
- Calls `central.Client.UpdateServerUrl()` with the admin's JWT
- Returns success/failure
- This is called from the configuration guide page after the admin verifies their setup

### Task 4: Frontend — Replace "AI Data Liaison ready" with "MCP Server accessible" checklist item

**Files:**
- `ui/src/pages/AIDataLiaisonPage.tsx` — Replace second checklist item

**Details:**
- Fetch server status from `GET /api/config/server-status` alongside existing data
- Remove the existing "AI Data Liaison ready" checklist item (the SetupChecklist component already shows an overall complete state with green icon and `completeDescription` when all items are done — a separate "ready" item is redundant)
- Replace with:
  ```typescript
  items.push({
    id: 'server-accessible',
    title: 'MCP Server accessible',
    description: isAccessible
      ? 'Server is reachable by business users over HTTPS'
      : 'Server is on localhost — business users cannot connect',
    status: loading ? 'loading' : isAccessible ? 'complete' : 'pending',
    link: `/projects/${pid}/server-setup`,
    linkText: isAccessible ? 'Review' : 'Configure',
  });
  ```
- The "Share with Business Users" section already shows the URL — no changes needed there (it will show the correct URL once `base_url` is updated)

### Task 5: Frontend — Create Server Setup configuration guide page

**Files:**
- `ui/src/pages/ServerSetupPage.tsx` — New page
- `ui/src/App.tsx` — Add route `server-setup`

**Details:**

The page has three sections:

**Section 1: Current Status**
- Shows current `base_url` and whether it's localhost/HTTP
- Green check or amber warning icon

**Section 2: Configuration Steps**
Step-by-step guide (content drawn from existing `config.yaml.example` and `certs/README.md`):

1. **Choose your approach:**
   - **Option A: Real certificates** (recommended for production) — Use Let's Encrypt, your org's PKI, or a cloud provider. Set `base_url`, `tls_cert_path`, `tls_key_path` in `config.yaml`.
   - **Option B: Self-signed certificates** (for internal/testing) — Generate with `openssl`, add to trust stores. Note: MCP clients using Node.js (Claude Code, Cursor) need `NODE_EXTRA_CA_CERTS` environment variable set.
   - **Option C: Reverse proxy** (Caddy, nginx, etc.) — Terminate TLS at the proxy, keep ekaya-engine on HTTP internally. Set `base_url` to the proxy's public HTTPS URL.

2. **Update `config.yaml`:**
   - Show the three fields: `base_url`, `tls_cert_path`, `tls_key_path`
   - Explain env var overrides: `BASE_URL`, `TLS_CERT_PATH`, `TLS_KEY_PATH`

3. **Restart the server**

**Section 3: Verify & Sync**
- "Verify" button: The page itself loading over HTTPS on the new domain proves it works
- "Update ekaya-central" button: Calls `POST /api/projects/{pid}/sync-server-url` to push the new `base_url` to ekaya-central so that redirect URLs and MCP setup links are correct
- Shows success/failure status

### Task 6: Handle the "already accessible" case gracefully

**Details:**
- When the server is already on HTTPS with a real domain, the checklist item shows as complete
- The [Review] link still goes to the setup page, which shows "Your server is properly configured" with the current URL
- The sync button is still available (in case the admin changed the domain and needs to re-sync)

## Sequencing

```
Task 1 (central client) → Task 3 (sync endpoint) depends on it
Task 2 (status endpoint) → Task 4 (checklist item) depends on it
Task 5 (setup page) depends on Task 2 + Task 3
Task 6 is part of Task 4 + Task 5

Parallel tracks:
  Track A: Task 1 → Task 3
  Track B: Task 2
  Then: Task 4 + Task 5 (frontend, after both tracks complete)
```

## What This Does NOT Cover

- **Automatic TLS provisioning** (e.g., built-in Let's Encrypt ACME) — out of scope, admins manage their own certs
- **Service token for ekaya-engine → ekaya-central** — the sync uses the admin's existing JWT (same as provision), not a service-to-service token. The `PATCH /projects/:projectId` endpoint on ekaya-central already accepts admin role JWTs.
- **Monitoring/alerting if the server becomes unreachable** — future work
- **Changes to ekaya-central** — the existing `PATCH /projects/:projectId` endpoint already accepts `serverUrl` updates; no ekaya-central changes needed

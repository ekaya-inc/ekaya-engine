# ISSUE: Server Setup page needs testing

**Status:** OPEN
**Page:** `/projects/{pid}/server-setup` (`ui/src/pages/ServerSetupPage.tsx`)

## Context

The Server Setup page guides admins through deploying the MCP Server over HTTPS on a reachable domain. It was originally linked only from the AI Data Liaison checklist. It is now linked from the MCP Server page's "Deployment" checklist section. Several changes were made to copy and navigation in this session that need verification.

## Items to Test

### 1. Navigation and back button

- [ ] Clicking "Configure" on the MCP Server page's Deployment checklist item navigates to `/projects/{pid}/server-setup`
- [ ] The back button (arrow) on Server Setup navigates to `/projects/{pid}/mcp-server` (not `/ai-data-liaison`)
- [ ] The back button aria-label reads "Back to MCP Server"

### 2. Localhost state (no HTTPS, no external domain)

- [ ] Page subtitle reads "Configure HTTPS on a reachable domain for other users"
- [ ] Current Status card shows warning icon (amber) with "Your server needs configuration before other users can connect"
- [ ] Base URL shows the current localhost URL (e.g., `http://localhost:3445`)
- [ ] Two status badges show: "HTTP only (HTTPS required)" and "Localhost (not reachable externally)" — both with amber warning icons
- [ ] Configuration Guide card is visible with all three options (A: Real certificates, B: Self-signed, C: Reverse proxy)
- [ ] Verify & Sync section title reads "Verify & Sync"

### 3. Deployed state (HTTPS on external domain)

This requires a deployed environment or a mock. When `accessible_for_business_users` is `true`:

- [ ] Current Status card shows green shield icon with "Your server is properly configured for other users"
- [ ] Both status badges show green checkmarks: "HTTPS enabled" and "External domain"
- [ ] Configuration Guide card is hidden (only shows when not accessible)
- [ ] Verify & Sync section shows green success banner: "This page loaded successfully over HTTPS on an external domain..."
- [ ] Verify & Sync title reads "Sync to Ekaya Service"

### 4. Buttons and actions

- [ ] "Copy" button on Base URL copies the URL to clipboard
- [ ] "Update Ekaya Service" button calls `syncServerUrl` API — shows spinner while syncing, success toast on completion
- [ ] "Refresh Status" button re-fetches server status from API and updates the display

### 5. Configuration Guide content accuracy

- [ ] Option A mentions `base_url`, `tls_cert_path`, `tls_key_path` in `config.yaml`
- [ ] Option B includes the `NODE_EXTRA_CA_CERTS` warning for Node.js MCP clients
- [ ] Option C mentions reverse proxy with `base_url` pointing to proxy's public HTTPS URL
- [ ] Step 2 shows correct `config.yaml` snippet with environment variable override note
- [ ] Step 3 instructs to restart ekaya-engine and navigate to this page on the new URL

### 6. Copy consistency

- [ ] No remaining references to "business users" in server setup copy (should say "other users")
- [ ] Exception: Configuration Guide description still says "Business users need HTTPS on a reachable domain" — verify whether this should also be updated

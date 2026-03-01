# PLAN: Add MCP Tunnel Application to UI

**Status:** In Progress
**Branch:** `ddanieli/add-tunnel-application`
**Depends on:** Backend already implemented (see `plans/PLAN-add-engine-tunnel.md`)

## Goal

Add the "MCP Tunnel" app to the engine UI so users can install, activate, and uninstall it from the Applications page — the same flow used by ai-data-liaison and ai-agents. Create a simple placeholder page for the app once installed.

## Background

The MCP Tunnel backend is fully implemented on this branch: the `mcp-tunnel` app ID is registered, the tunnel client/manager package exists in `pkg/tunnel/`, lifecycle hooks wire into install/activate/uninstall, and a status API endpoint exists. However, the UI has no awareness of this app — it cannot be installed or managed from the browser.

## Scope

UI-only changes. No backend changes needed.

## Files to Modify

### 1. [x] `ui/src/types/installedApp.ts`

Add the app ID constant alongside the existing ones (line 11):

```typescript
export const APP_ID_MCP_TUNNEL = 'mcp-tunnel';
```

### 2. [x] `ui/src/pages/ApplicationsPage.tsx`

**a) Add `Globe` to the lucide-react import** (line 2-12):
```typescript
import { ..., Globe, ... } from 'lucide-react';
```

**b) Add mcp-tunnel entry to the `applications` array** (after the `ai-agents` entry, around line 86):
```typescript
{
  id: 'mcp-tunnel',
  title: 'MCP Tunnel',
  subtitle: 'Give your MCP Server a public URL accessible from outside your firewall — no port forwarding or TLS configuration required',
  icon: Globe,
  color: 'green',
  available: true,
  installable: true,
  learnMoreUrl: 'https://try.ekaya.ai/',
},
```

**c) Fix `handleLearnMore` to support full URLs** (line 154-156):

Currently `handleLearnMore` always prepends `marketingOrigin` to the path. The mcp-tunnel `learnMoreUrl` is a full URL (`https://try.ekaya.ai/`). Update the function:
```typescript
const handleLearnMore = (urlOrPath: string) => {
  const url = urlOrPath.startsWith('http') ? urlOrPath : `${marketingOrigin}${urlOrPath}`;
  window.open(url, '_blank', 'noopener,noreferrer');
};
```

**d) Fix hardcoded "Configure" navigation** (line 205):

The "Configure" button for installed apps is hardcoded to navigate to `ai-data-liaison`. It should use the app's ID:
```typescript
// Change from:
onClick={() => navigate(`/projects/${pid}/ai-data-liaison`)}
// To:
onClick={() => navigate(`/projects/${pid}/${app.id}`)}
```

### 3. [x] `ui/src/pages/ProjectDashboard.tsx`

**a) Add imports** (lines 1-17):
- Add `Globe` to the lucide-react import
- Add `APP_ID_MCP_TUNNEL` to the installedApp import

**b) Add conditional MCP Tunnel tile** in the `applicationTiles` useMemo (after the AI Agents block, around line 206):
```typescript
// Add MCP Tunnel tile if installed
if (installedApps.some((app) => app.app_id === APP_ID_MCP_TUNNEL)) {
  tiles.push({
    title: 'MCP Tunnel',
    description: 'Your MCP Server has a public URL accessible from outside your firewall.',
    icon: Globe,
    path: `/projects/${pid}/mcp-tunnel`,
    disabled: !isConnected,
    disabledReason: 'Requires MCP Server to be enabled.',
    color: 'green',
  });
}
```

### 4. [x] `ui/src/App.tsx`

**a) Add import** (near the other page imports at the top):
```typescript
import MCPTunnelPage from './pages/MCPTunnelPage';
```

**b) Add route** (inside the `/projects/:pid` route group, around line 51):
```typescript
<Route path="mcp-tunnel" element={<MCPTunnelPage />} />
```

### 5. [ ] `ui/src/pages/MCPTunnelPage.tsx` (NEW FILE)

Create a simple placeholder page following the AIAgentsPage pattern (`ui/src/pages/AIAgentsPage.tsx` is the template to follow).

**Structure:**
- Back button navigating to `/projects/${pid}`
- Header: "MCP Tunnel" title
- Description: "Gives your MCP Server a public URL so external MCP clients (Claude Desktop, Cursor, etc.) can reach it without firewall changes or TLS configuration."
- A placeholder info card stating the tunnel connects automatically when the app is activated
- Danger Zone card with uninstall button + confirmation dialog (copy the exact pattern from AIAgentsPage lines 202-289, changing `ai-agents` to `mcp-tunnel` and updating the text)

**Key implementation details:**
- Use `useParams<{ pid: string }>()` for project ID
- Use `useNavigate()` for navigation
- Use `useToast()` for error notifications
- Call `engineApi.uninstallApp(pid, 'mcp-tunnel')` for uninstall
- After successful uninstall, navigate to `/projects/${pid}`
- Uninstall confirmation requires typing "uninstall application" (same as other apps)
- Import components from: `../components/ui/Button`, `../components/ui/Card`, `../components/ui/Dialog`, `../components/ui/Input`
- Import icons: `ArrowLeft`, `Globe`, `Loader2`, `Trash2` from lucide-react

## Existing Patterns to Reuse

- **App install/activate flow:** Already handled by `useInstallApp` hook (`ui/src/hooks/useInstalledApps.ts`) and `ApplicationsPage.tsx` `handleInstall` function. The install may redirect to ekaya-central for billing — this is handled generically.
- **Uninstall confirmation dialog:** Copy from `AIAgentsPage.tsx` lines 202-289 — requires typing "uninstall application" to confirm.
- **Dashboard tile pattern:** Follow the existing conditional tile pattern in `ProjectDashboard.tsx` lines 182-206.
- **API calls:** All needed API methods already exist in `engineApi.ts` (lines 862-971): `installApp`, `activateApp`, `uninstallApp`.

## Verification

1. `cd ui && npm run build` — no TypeScript or build errors
2. Navigate to `/projects/{pid}/applications` — MCP Tunnel tile appears with "Learn More" and "Install" buttons
3. "Learn More" opens `https://try.ekaya.ai/` in a new tab
4. Click "Install" — goes through normal install flow (may redirect to central)
5. After install, project dashboard shows MCP Tunnel tile under Applications
6. Click MCP Tunnel tile — navigates to placeholder page
7. Uninstall from danger zone — removes app, returns to dashboard

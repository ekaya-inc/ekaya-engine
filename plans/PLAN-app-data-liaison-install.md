# PLAN: AI Data Liaison Application Installation

## Purpose

Enable users to install/uninstall the AI Data Liaison application from the Applications page. This establishes the pattern for installable applications in the Ekaya platform.

---

## Current State

| Component | Current Behavior |
|-----------|------------------|
| Project Dashboard | Shows "MCP Server" tile under Applications; button says "Install Application" (singular) |
| Applications Page | Static showcase with "Contact Sales" buttons; title says "Install Application" |
| AI Data Liaison | Shows "Contact Sales" button, no install capability |
| Installed Apps Tracking | No mechanism exists |

---

## Implementation Overview

### Data Model

Create a new table `engine_installed_apps` to track which applications are installed per project:

```sql
CREATE TABLE engine_installed_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    app_id VARCHAR(50) NOT NULL,  -- e.g., 'ai-data-liaison', 'mcp-server'
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    installed_by VARCHAR(255),  -- User email/ID who installed
    settings JSONB DEFAULT '{}'::jsonb,  -- App-specific settings

    CONSTRAINT unique_project_app UNIQUE (project_id, app_id)
);

-- RLS for tenant isolation
ALTER TABLE engine_installed_apps ENABLE ROW LEVEL SECURITY;
CREATE POLICY installed_apps_access ON engine_installed_apps FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid);

-- Index for listing apps by project
CREATE INDEX idx_installed_apps_project ON engine_installed_apps(project_id);
```

**App IDs:**
- `mcp-server` - MCP Server (installed by default for all projects)
- `ai-data-liaison` - AI Data Liaison application

---

## Phase 1: Backend - Installed Apps Tracking [x]

### 1.1 Database Migration

**File:** `migrations/017_installed_apps.up.sql`

Create the `engine_installed_apps` table as shown above.

### 1.2 Model

**File:** `pkg/models/installed_app.go`

```go
type InstalledApp struct {
    ID          uuid.UUID              `json:"id"`
    ProjectID   uuid.UUID              `json:"project_id"`
    AppID       string                 `json:"app_id"`
    InstalledAt time.Time              `json:"installed_at"`
    InstalledBy string                 `json:"installed_by,omitempty"`
    Settings    map[string]interface{} `json:"settings"`
}

// AppID constants
const (
    AppIDMCPServer     = "mcp-server"
    AppIDAIDataLiaison = "ai-data-liaison"
)
```

### 1.3 Repository

**File:** `pkg/repositories/installed_app_repository.go`

```go
type InstalledAppRepository interface {
    // List returns all installed apps for a project
    List(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error)

    // Get returns a specific installed app (nil if not installed)
    Get(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error)

    // IsInstalled checks if an app is installed
    IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error)

    // Install adds an app to the project
    Install(ctx context.Context, app *models.InstalledApp) error

    // Uninstall removes an app from the project
    Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error

    // UpdateSettings updates app-specific settings
    UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]interface{}) error
}
```

### 1.4 Service

**File:** `pkg/services/installed_app.go`

```go
type InstalledAppService interface {
    // ListInstalled returns all installed apps for a project
    ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error)

    // IsInstalled checks if a specific app is installed
    IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error)

    // Install installs an app for a project
    Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*models.InstalledApp, error)

    // Uninstall removes an app and clears its settings
    Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error

    // GetSettings returns app-specific settings
    GetSettings(ctx context.Context, projectID uuid.UUID, appID string) (map[string]interface{}, error)

    // UpdateSettings updates app-specific settings
    UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]interface{}) error
}
```

**Install validation:**
- Validate `appID` is a known application
- Check app isn't already installed
- For `ai-data-liaison`: No prerequisites (can install anytime)

**Uninstall behavior:**
- Delete the row from `engine_installed_apps`
- Clear any app-specific data (future: defined per-app)

### 1.5 API Endpoints

**File:** `pkg/handlers/installed_app.go`

```
GET  /api/projects/{pid}/apps              → List installed apps
GET  /api/projects/{pid}/apps/{appId}      → Get app details (404 if not installed)
POST /api/projects/{pid}/apps/{appId}      → Install app
DELETE /api/projects/{pid}/apps/{appId}    → Uninstall app
PATCH /api/projects/{pid}/apps/{appId}     → Update app settings
```

**Install Response:**
```json
{
  "id": "uuid",
  "app_id": "ai-data-liaison",
  "installed_at": "2025-01-23T10:00:00Z",
  "installed_by": "user@example.com",
  "settings": {}
}
```

**List Response:**
```json
{
  "apps": [
    {
      "app_id": "mcp-server",
      "installed_at": "2025-01-01T00:00:00Z",
      "settings": {}
    },
    {
      "app_id": "ai-data-liaison",
      "installed_at": "2025-01-23T10:00:00Z",
      "settings": {}
    }
  ]
}
```

---

## Phase 2: UI Updates

### 2.1 Project Dashboard Changes

**File:** `ui/src/pages/ProjectDashboard.tsx`

**Change 1:** Rename button from "Install Application" to "Install Applications" (line ~571)

```tsx
// Before
<Button ...>
  <Plus className="h-4 w-4" />
  Install Application
</Button>

// After
<Button ...>
  <Plus className="h-4 w-4" />
  Install Applications
</Button>
```

**Change 2:** Show AI Data Liaison tile when installed

Update `applicationTiles` to be dynamic based on installed apps:

```tsx
// Fetch installed apps
const { data: installedApps } = useInstalledApps(pid);

// Build tiles dynamically
const applicationTiles: Tile[] = [
  // MCP Server is always shown (installed by default)
  {
    title: 'MCP Server',
    icon: Server,
    path: `/projects/${pid}/mcp-server`,
    disabled: !isConnected,
    color: 'cyan',
  },
];

// Add AI Data Liaison tile if installed
if (installedApps?.some(app => app.app_id === 'ai-data-liaison')) {
  applicationTiles.push({
    title: 'AI Data Liaison',
    icon: BrainCircuit,
    path: `/projects/${pid}/ai-data-liaison`,
    disabled: false,
    color: 'blue',
  });
}
```

### 2.2 Applications Page Changes

**File:** `ui/src/pages/ApplicationsPage.tsx`

**Change 1:** Update page title from "Install Application" to "Applications"

```tsx
// Before
<h1 className="text-2xl font-bold">Install Application</h1>
<p className="text-text-secondary">
  Choose an application to add to your project
</p>

// After
<h1 className="text-2xl font-bold">Applications</h1>
<p className="text-text-secondary">
  Choose an application to add to your project
</p>
```

**Change 2:** Update AI Data Liaison tile with [Learn More] and [Install] buttons

```tsx
// Fetch installed apps to determine state
const { data: installedApps, refetch } = useInstalledApps(pid);
const isDataLiaisonInstalled = installedApps?.some(app => app.app_id === 'ai-data-liaison');

// For AI Data Liaison tile:
{app.id === 'ai-data-liaison' ? (
  isDataLiaisonInstalled ? (
    // Installed state - show "Installed" badge and make tile clickable
    <CardFooter className="pt-0 flex gap-2">
      <span className="text-xs text-green-600 font-medium flex items-center gap-1">
        <Check className="h-3 w-3" />
        Installed
      </span>
      <Button
        variant="outline"
        size="sm"
        onClick={() => navigate(`/projects/${pid}/ai-data-liaison`)}
      >
        Configure
      </Button>
    </CardFooter>
  ) : (
    // Not installed - show Learn More and Install buttons
    <CardFooter className="pt-0 flex gap-2">
      <Button
        variant="outline"
        size="sm"
        onClick={() => window.open('https://ekaya.ai/enterprise/', '_blank')}
      >
        Learn More
      </Button>
      <Button
        variant="default"
        size="sm"
        onClick={() => handleInstall('ai-data-liaison')}
      >
        Install
      </Button>
    </CardFooter>
  )
) : (
  // Other apps keep existing behavior
  <CardFooter>...</CardFooter>
)}
```

**Change 3:** Add install handler

```tsx
const handleInstall = async (appId: string) => {
  try {
    await engineApi.installApp(pid, appId);
    // Navigate to the app's configuration page
    navigate(`/projects/${pid}/${appId}`);
  } catch (error) {
    // Show error toast
  }
};
```

**Change 4:** Make installed app tiles clickable to navigate to config

When a tile shows "Installed" state, clicking it navigates to `/projects/{pid}/ai-data-liaison`.

### 2.3 API Client Updates

**File:** `ui/src/services/engineApi.ts`

```typescript
// List installed apps
async listInstalledApps(projectId: string): Promise<ApiResponse<{ apps: InstalledApp[] }>>

// Install an app
async installApp(projectId: string, appId: string): Promise<ApiResponse<InstalledApp>>

// Uninstall an app
async uninstallApp(projectId: string, appId: string): Promise<ApiResponse<void>>

// Get app settings
async getAppSettings(projectId: string, appId: string): Promise<ApiResponse<InstalledApp>>

// Update app settings
async updateAppSettings(projectId: string, appId: string, settings: Record<string, unknown>): Promise<ApiResponse<InstalledApp>>
```

### 2.4 React Query Hook

**File:** `ui/src/hooks/useInstalledApps.ts`

```typescript
export function useInstalledApps(projectId: string) {
  return useQuery({
    queryKey: ['installed-apps', projectId],
    queryFn: () => engineApi.listInstalledApps(projectId),
  });
}

export function useInstallApp() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, appId }: { projectId: string; appId: string }) =>
      engineApi.installApp(projectId, appId),
    onSuccess: (_, { projectId }) => {
      queryClient.invalidateQueries(['installed-apps', projectId]);
    },
  });
}
```

---

## Phase 3: AI Data Liaison Configuration Page

### 3.1 New Page Component

**File:** `ui/src/pages/AIDataLiaisonPage.tsx`

```tsx
const AIDataLiaisonPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application') return;

    setIsUninstalling(true);
    try {
      await engineApi.uninstallApp(pid, 'ai-data-liaison');
      navigate(`/projects/${pid}`);
    } catch (error) {
      // Show error
    } finally {
      setIsUninstalling(false);
    }
  };

  return (
    <div className="mx-auto max-w-4xl space-y-8">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate(`/projects/${pid}`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">AI Data Liaison</h1>
          <p className="text-text-secondary">
            Configure your AI Data Liaison application
          </p>
        </div>
      </div>

      {/* Configuration content placeholder */}
      <Card>
        <CardHeader>
          <CardTitle>Configuration</CardTitle>
          <CardDescription>
            AI Data Liaison configuration options will appear here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-text-secondary text-sm">
            Coming soon: Configure how AI Data Liaison connects to your data and serves your business users.
          </p>
        </CardContent>
      </Card>

      {/* Danger Zone */}
      <Card className="border-red-200 dark:border-red-900">
        <CardHeader>
          <CardTitle className="text-red-600">Danger Zone</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-medium">Uninstall This Application</h3>
              <p className="text-sm text-text-secondary">
                Remove AI Data Liaison from this project. This will clear all settings.
              </p>
            </div>
            <Button
              variant="destructive"
              onClick={() => setShowUninstallDialog(true)}
            >
              Uninstall
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Uninstall Confirmation Dialog */}
      <Dialog open={showUninstallDialog} onOpenChange={setShowUninstallDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Uninstall AI Data Liaison?</DialogTitle>
            <DialogDescription>
              This will remove the AI Data Liaison application from your project and clear all its settings. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <p className="text-sm">
              Type <span className="font-mono font-bold">uninstall application</span> to confirm:
            </p>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder="uninstall application"
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setShowUninstallDialog(false);
                setConfirmText('');
              }}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={confirmText !== 'uninstall application' || isUninstalling}
              onClick={handleUninstall}
            >
              {isUninstalling ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                  Uninstalling...
                </>
              ) : (
                'Uninstall Application'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};
```

### 3.2 Add Route

**File:** `ui/src/App.tsx`

Add route for AI Data Liaison page:

```tsx
<Route path="/projects/:pid" element={<ProjectProvider>...</ProjectProvider>}>
  <Route index element={<ProjectDashboard />} />
  <Route path="applications" element={<ApplicationsPage />} />
  <Route path="mcp-server" element={<MCPServerPage />} />
  <Route path="ai-data-liaison" element={<AIDataLiaisonPage />} />  {/* NEW */}
  {/* ... other routes */}
</Route>
```

---

## Phase 4: Default App Installation

### 4.1 MCP Server Auto-Install

The MCP Server should be "installed" by default for all projects. Options:

**Option A (Recommended):** Treat MCP Server as always-installed in the UI without database tracking
- No migration needed for existing projects
- `ListInstalled` always includes `mcp-server` in response
- MCP Server cannot be uninstalled

**Option B:** Create migration to install MCP Server for all existing projects
- Requires data migration
- Adds complexity

**Recommendation:** Use Option A - The backend service always includes `mcp-server` in the list response:

```go
func (s *installedAppService) ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error) {
    apps, err := s.repo.List(ctx, projectID)
    if err != nil {
        return nil, err
    }

    // Always include MCP Server as "installed"
    hasMCP := false
    for _, app := range apps {
        if app.AppID == models.AppIDMCPServer {
            hasMCP = true
            break
        }
    }
    if !hasMCP {
        apps = append([]*models.InstalledApp{{
            AppID:       models.AppIDMCPServer,
            ProjectID:   projectID,
            InstalledAt: time.Time{}, // Zero time indicates "always installed"
        }}, apps...)
    }

    return apps, nil
}
```

---

## File Changes Summary

| Phase | File | Change |
|-------|------|--------|
| 1 | `migrations/017_installed_apps.up.sql` | Create table |
| 1 | `migrations/017_installed_apps.down.sql` | Drop table |
| 1 | `pkg/models/installed_app.go` | New model |
| 1 | `pkg/repositories/installed_app_repository.go` | New repository |
| 1 | `pkg/services/installed_app.go` | New service |
| 1 | `pkg/handlers/installed_app.go` | New handler |
| 1 | `main.go` | Wire up new handler |
| 2 | `ui/src/pages/ProjectDashboard.tsx` | Button rename, dynamic tiles |
| 2 | `ui/src/pages/ApplicationsPage.tsx` | Title change, install buttons |
| 2 | `ui/src/services/engineApi.ts` | New API methods |
| 2 | `ui/src/hooks/useInstalledApps.ts` | New hook |
| 3 | `ui/src/pages/AIDataLiaisonPage.tsx` | New page |
| 3 | `ui/src/App.tsx` | Add route |

---

## Testing Strategy

### Unit Tests
- Repository: CRUD operations for installed apps
- Service: Install/uninstall logic, MCP Server always-included behavior

### Integration Tests
- Install app → verify in database
- Uninstall app → verify removed
- List apps → verify MCP Server always present

### Manual Testing
1. Navigate to `/projects/{pid}` - verify "Install Applications" button (plural)
2. Click button → verify page title says "Applications"
3. On AI Data Liaison tile → verify [Learn More] and [Install] buttons
4. Click [Learn More] → verify opens https://ekaya.ai/enterprise/ in new tab
5. Click [Install] → verify redirects to `/projects/{pid}/ai-data-liaison`
6. Navigate back to dashboard → verify AI Data Liaison tile appears in Applications section
7. Navigate to Applications page → verify tile shows "Installed" state
8. Click installed tile → verify navigates to config page
9. On config page → verify Danger Zone with uninstall
10. Type "uninstall application" → verify uninstall works
11. After uninstall → verify app removed from dashboard, installable again on Applications page

---

## Open Questions

1. **Should uninstall require additional confirmation beyond typing "uninstall application"?**
   - Current: Type confirmation is sufficient
   - Alternative: Also require checkbox for "I understand this will delete all settings"

2. **Should we track who uninstalled an app?**
   - Currently not tracked (row is deleted)
   - Could add audit log if needed

3. **Should there be app-specific cleanup on uninstall?**
   - For AI Data Liaison v1: No additional data to clean up
   - Future: May need to clear query history, conversation logs, etc.

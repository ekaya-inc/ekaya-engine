# Plan: Applications Installation Feature

## Overview

Add an "Install Application" button to the Project Dashboard's Applications section, which navigates to a new screen where users can browse and select applications to install.

## Current State

**File:** `ui/src/pages/ProjectDashboard.tsx`

The Applications section (lines 552-561) displays a grid of application tiles. Currently only shows "MCP Server" which is the default application every project has.

```typescript
// Lines 425-434 - Current applicationTiles array
const applicationTiles: Tile[] = [
  {
    title: 'MCP Server',
    icon: Server,
    path: `/projects/${pid}/mcp-server`,
    disabled: !isConnected || !hasSelectedTables,
    color: 'cyan',
  },
];
```

The section renders using `renderApplicationTile()` (lines 436-508) which creates larger tiles (24x24 icons) compared to data tiles (16x16 icons).

## Implementation Tasks

### 1. [x] Add "+ Install Application" Button to Applications Section

**Status:** COMPLETE

**File:** `ui/src/pages/ProjectDashboard.tsx`

**What was done:**
- Added `Plus` icon import from `lucide-react` (line 13)
- Added `Button` component import from `../components/ui/Button` (line 23)
- Added the "Install Application" button after the applications grid (lines 563-570)
- Button navigates to `/projects/${pid}/applications` (route to be created in task 3)

**Implementation notes:**
- Button uses `variant="outline"` and `className="w-full mt-4"` for full-width styling with top margin
- Icon does NOT use `mr-2` margin - the Button component handles icon spacing internally
- The button appears below the existing MCP Server tile

### 2. Create Applications Catalog Page

**New File:** `ui/src/pages/ApplicationsPage.tsx`

Create a new page that displays available applications as tiles. The tiles should be **small-ish** to imply more applications are coming.

**Application Data:**

```typescript
interface ApplicationInfo {
  id: string;
  title: string;
  subtitle: string;
  description: string;
  icon: LucideIcon;
  color: string;
  available: boolean;  // false for "Coming Soon"
}

const applications: ApplicationInfo[] = [
  {
    id: 'ai-data-liaison',
    title: 'AI Data Liaison',
    subtitle: 'Make Better Business Decisions 10x Faster',
    description: 'AI-powered data analysis and insights for faster, smarter business decisions.',
    icon: BrainCircuit,  // or Lightbulb, TrendingUp from lucide-react
    color: 'blue',
    available: true,
  },
  {
    id: 'product-kit',
    title: 'Product Kit',
    subtitle: 'Enable AI Features in your existing SaaS Product',
    description: 'Integrate AI capabilities directly into your product with pre-built components and APIs.',
    icon: Package,  // or Boxes, Blocks from lucide-react
    color: 'purple',
    available: true,
  },
  {
    id: 'on-premise-chat',
    title: 'On-Premise Chat',
    subtitle: 'Deploy AI Chat where data never leaves your data boundary',
    description: 'Self-hosted chat solution for maximum data privacy and compliance.',
    icon: MessageSquare,  // or ShieldCheck, Lock
    color: 'green',
    available: true,
  },
  {
    id: 'more-coming',
    title: 'More Coming!',
    subtitle: 'Additional applications in development',
    description: 'We are building more applications to help you leverage your data.',
    icon: Sparkles,  // or Clock, Rocket
    color: 'gray',
    available: false,
  },
];
```

**Page Structure:**

```tsx
export function ApplicationsPage() {
  const navigate = useNavigate();
  const { pid } = useParams();

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header with back button */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate(`/projects/${pid}`)}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Install Application</h1>
          <p className="text-muted-foreground">Choose an application to add to your project</p>
        </div>
      </div>

      {/* Application tiles - 3 column grid with smaller tiles */}
      <div className="grid grid-cols-3 gap-4">
        {applications.map((app) => (
          <Card
            key={app.id}
            className={cn(
              "cursor-pointer transition-all hover:shadow-md",
              !app.available && "opacity-60 cursor-not-allowed"
            )}
            onClick={() => app.available && handleInstall(app.id)}
          >
            <CardHeader className="pb-2">
              <div className={cn(
                "h-12 w-12 rounded-lg flex items-center justify-center mb-2",
                `bg-${app.color}-500/10`
              )}>
                <app.icon className={cn("h-6 w-6", `text-${app.color}-500`)} />
              </div>
              <CardTitle className="text-base">{app.title}</CardTitle>
              <CardDescription className="text-xs line-clamp-2">
                {app.subtitle}
              </CardDescription>
            </CardHeader>
            {!app.available && (
              <CardFooter className="pt-0">
                <span className="text-xs text-muted-foreground">Coming Soon</span>
              </CardFooter>
            )}
          </Card>
        ))}
      </div>
    </div>
  );
}
```

**Tile Sizing Notes:**
- Use `grid-cols-3` to fit 3 tiles per row (smaller feeling)
- Icon container: `h-12 w-12` (smaller than dashboard's h-24 w-24)
- Icon size: `h-6 w-6` (smaller than dashboard)
- Title: `text-base` instead of `text-2xl`
- Description: `text-xs` for compact look

### 3. Add Route for Applications Page

**File:** `ui/src/App.tsx`

Add the route inside the project routes block (around line 30-42):

```tsx
<Route path="/projects/:pid" element={<ProjectProvider><ProjectDataLoader><Layout /></ProjectDataLoader></ProjectProvider>}>
  <Route index element={<ProjectDashboard />} />
  <Route path="applications" element={<ApplicationsPage />} />  {/* ADD THIS */}
  <Route path="datasource" element={<DatasourcePage />} />
  {/* ... rest of routes */}
</Route>
```

Import the new page component at the top of App.tsx.

### 4. Application Installation Flow (Future)

For MVP, clicking an available application tile should:
1. Navigate to a detail/configuration page for that application, OR
2. Show a dialog with installation confirmation

**Placeholder Implementation:**

```typescript
const handleInstall = (appId: string) => {
  // MVP: Navigate to app-specific page or show coming soon toast
  if (appId === 'product-kit') {
    navigate(`/projects/${pid}/product-kit`);
    // OR show toast: "Product Kit installation coming soon"
  } else if (appId === 'on-premise-chat') {
    navigate(`/projects/${pid}/on-premise-chat`);
    // OR show toast: "On-Premise Chat installation coming soon"
  }
};
```

### 5. Add AI Data Liaison Tile to Project Dashboard

**File:** `ui/src/pages/ProjectDashboard.tsx`

Add a new tile at the **beginning** of the `applicationTiles` array (before MCP Server):

```typescript
const applicationTiles: Tile[] = [
  {
    title: 'AI Data Liaison',
    icon: BrainCircuit,  // from lucide-react
    path: `/projects/${pid}/ai-data-liaison`,
    disabled: !isConnected || !hasSelectedTables,
    color: 'blue',
  },
  {
    title: 'MCP Server',
    icon: Server,
    path: `/projects/${pid}/mcp-server`,
    disabled: !isConnected || !hasSelectedTables,
    color: 'cyan',
  },
];
```

Import `BrainCircuit` from `lucide-react`.

**Note:** The AI Data Liaison page (`/projects/:pid/ai-data-liaison`) will need to be created separately. For now, clicking the tile can navigate to the page which can show a "Coming Soon" placeholder.

## File Changes Summary

| File | Change |
|------|--------|
| `ui/src/pages/ProjectDashboard.tsx` | Add AI Data Liaison tile + "+ Install Application" button |
| `ui/src/pages/ApplicationsPage.tsx` | NEW - Applications catalog page |
| `ui/src/App.tsx` | Add route for `/projects/:pid/applications` |

## Design Notes

- **Small tiles**: 3-column grid with compact sizing implies more apps coming (4 tiles wrap nicely)
- **"More Coming!" tile**: Disabled state, gray color, reinforces expansion
- **Consistent patterns**: Follow existing Card, Button, and navigation patterns
- **Color scheme**: AI Data Liaison (blue), Product Kit (purple), On-Premise Chat (green), More Coming (gray)

## Dependencies

- Existing UI components: Card, CardHeader, CardTitle, CardDescription, CardFooter, Button
- Icons from lucide-react: Plus, ArrowLeft, BrainCircuit, Package, MessageSquare, Sparkles
- Routing: react-router-dom (useNavigate, useParams)

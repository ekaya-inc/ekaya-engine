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

### 2. [x] Create Applications Catalog Page

**Status:** COMPLETE

**New File:** `ui/src/pages/ApplicationsPage.tsx`

**What was done:**
- Created `ui/src/pages/ApplicationsPage.tsx` with a 3-column responsive grid of application tiles
- Implemented 4 application tiles: AI Data Liaison (blue), Product Kit (purple), On-Premise Chat (green), More Coming (gray/disabled)
- Added back button navigation to project dashboard
- Used `getColorClasses()` helper function instead of template literals (Tailwind JIT requires explicit class names)
- Used `text-text-secondary` instead of `text-muted-foreground` to match existing codebase patterns
- Responsive grid: `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3`
- Component exports as default export

**Implementation notes:**
- Icons used: `BrainCircuit`, `Package`, `MessageSquare`, `Sparkles` from lucide-react
- `handleInstall()` navigates to app-specific routes (e.g., `/projects/${pid}/ai-data-liaison`)
- Disabled tiles (`available: false`) have `opacity-60` and `cursor-not-allowed`
- The route still needs to be added to `ui/src/App.tsx` (Task 3)

### 3. [x] Add Route for Applications Page

**Status:** COMPLETE

**File:** `ui/src/App.tsx`

**What was done:**
- Added `ApplicationsPage` import (line 22)
- Added route `<Route path="applications" element={<ApplicationsPage />} />` inside the project routes block (line 38)
- Route is accessible at `/projects/:pid/applications`

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

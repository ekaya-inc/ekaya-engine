# FIX: Unhide "AI Agents and Automation" Tile

**Status:** PENDING
**File:** `ui/src/pages/ApplicationsPage.tsx`
**Line:** ~283

## What Was Done

The "AI Agents and Automation" tile was temporarily hidden from the Install Applications screen by filtering it out of the rendered list. The tile definition still exists in the `applications` array (id: `ai-agents`, lines 77-86) but is excluded from rendering.

## How to Unhide

In `ui/src/pages/ApplicationsPage.tsx`, find this line:

```tsx
{applications.filter(app => app.id !== 'ai-agents').map((app) => {
```

Remove the `.filter(app => app.id !== 'ai-agents')` so it becomes:

```tsx
{applications.map((app) => {
```

That's it -- the tile will reappear with all its existing configuration (orange color, Bot icon, "Connect AI coding agents..." subtitle).

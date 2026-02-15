# FIX: Unhide Glossary Tile on Project Dashboard

**Status:** PENDING

## What Was Done

The Glossary tile was commented out from the Intelligence section of the Project Dashboard (`ui/src/pages/ProjectDashboard.tsx`, lines ~146-152). The tile config object is preserved in a comment block with a pointer back to this file.

## What To Do

Uncomment the Glossary tile in `ui/src/pages/ProjectDashboard.tsx` inside the `intelligenceTiles` array. Search for `HIDDEN: Glossary tile temporarily removed` to find the exact location.

## Checklist

- [ ] Uncomment the Glossary tile in `ProjectDashboard.tsx`
- [ ] Remove the `HIDDEN` comment marker
- [ ] Verify the tile renders correctly on the dashboard

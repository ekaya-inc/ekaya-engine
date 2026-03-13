# FIX: Unhide Glossary Tile on Project Dashboard

**Status:** FIXED

## What Was Done

The Glossary tile was restored in the Intelligence section of the Project Dashboard in `ui/src/pages/ProjectDashboard.tsx`.

The implementation:

- re-added the `Glossary` tile to the `intelligenceTiles` array
- restored the `BookOpen` icon import used by that tile
- kept the tile in the Ontology Forge grouping with the existing purple color scheme

## Checklist

- [x] Uncomment the Glossary tile in `ProjectDashboard.tsx`
- [x] Remove the `HIDDEN` comment marker
- [x] Verify the dashboard code compiles with the restored tile

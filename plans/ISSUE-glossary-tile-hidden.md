# ISSUE: Glossary tile hidden from Project Dashboard

**Status:** OPEN
**Date:** 2026-02-06
**Branch where hidden:** `ddanieli/end-to-end-ai-data-liaison`

## What was observed

The glossary feature output quality is not sufficient for launch. The tile was hidden from the Project Dashboard to prevent users from accessing it until the quality improves.

## What was done

- Commented out the Glossary tile in `ui/src/pages/ProjectDashboard.tsx` in the `intelligenceTiles` array
- Removed the `BookOpen` icon import (unused after commenting out the tile)
- All other glossary code left intact: route, `GlossaryPage.tsx`, `GlossaryTermEditor.tsx`, backend endpoints, etc.

## Steps to re-enable

1. Uncomment the Glossary tile in `ui/src/pages/ProjectDashboard.tsx` (search for "Glossary tile hidden")
2. Re-add `BookOpen` to the lucide-react import
3. Verify glossary extraction quality meets launch bar before shipping

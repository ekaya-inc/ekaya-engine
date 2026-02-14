# FIX: Unhide Help Icon in Header

**Status:** PENDING (waiting for help system implementation)

## Context

The Help icon (HelpCircle) was removed from the header menu bar because the help page (`/projects/{pid}/help`) has no content yet. Showing an empty help page creates a poor user experience.

## What Was Changed

- **`ui/src/components/Header.tsx`**: Removed the HelpCircle icon button and its import. The Settings icon remains.
- The `/help` route and `HelpPage` component still exist in the codebase and are untouched.

## What to Do When Ready

When the help system content is ready, restore the Help icon in the header:

1. In `ui/src/components/Header.tsx`:
   - Add `HelpCircle` back to the lucide-react import: `import { Settings, HelpCircle } from 'lucide-react';`
   - Add the button back after the Settings button inside the `flex items-center gap-2` div:
     ```tsx
     <button
       onClick={() => navigate(`/projects/${pid}/help`)}
       className="p-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-secondary transition-colors"
       title="Help"
     >
       <HelpCircle className="h-5 w-5" />
     </button>
     ```

2. Verify the HelpPage component (`ui/src/pages/HelpPage.tsx`) has actual content.

3. Delete this FIX file after restoring.

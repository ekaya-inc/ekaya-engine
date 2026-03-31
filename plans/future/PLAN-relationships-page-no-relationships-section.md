# PLAN: Add "No Relationships" Section to Relationships Page

**Date:** 2026-03-02
**Status:** TODO
**Priority:** MEDIUM

## Problem

The Relationships Manager page shows "2 tables without relationships" in a warning banner with a "Go to Ontology →" CTA. This CTA navigates away to the Ontology page, which doesn't fix the problem. The user has no way to see *which* tables lack relationships without using the Table filter dropdown, and there's no direct path to add relationships for those tables.

## Solution

1. Change the warning banner CTA from "Go to Ontology →" to scroll down to a new "No Relationships" section on the same page.
2. Add a "No Relationships" section below the existing Relationships card that lists the orphan tables (tables with data but no relationships) with an "Add Relationship" button per table.

## Current State

- **Backend already returns the data:** `GetRelationshipsResponse` includes `empty_tables` (0 rows) and `orphan_tables` (has data, no relationships). Both arrays of table name strings.
- **Frontend already has the data:** `emptyTables` and `orphanTables` arrays are available in `RelationshipsPage.tsx`.
- **Filter dropdown already works:** The Table dropdown has "No Relationships" and "Empty Tables" special filters that show these tables — but users must discover this themselves.
- **AddRelationshipDialog already exists:** Supports selecting source/target table and column.

## Changes

### `ui/src/pages/RelationshipsPage.tsx`

**1. Change the warning banner CTA** (lines 366-373):

Replace:
```tsx
<Button variant="outline" size="sm" onClick={() => navigate(`/projects/${pid}/ontology`)}>
  Go to Ontology
  <ArrowRight className="ml-2 h-4 w-4" />
</Button>
```

With a scroll-to-section action:
```tsx
<Button variant="outline" size="sm" onClick={() => {
  document.getElementById('no-relationships-section')?.scrollIntoView({ behavior: 'smooth' });
}}>
  View Tables
  <ArrowRight className="ml-2 h-4 w-4" />
</Button>
```

**2. Add "No Relationships" section** after the Relationships card (after line 638, before the AddRelationshipDialog):

New Card with `id="no-relationships-section"` containing:
- Title: "No Relationships" with count
- Subtitle explaining the two categories (empty tables vs orphan tables)
- List of **orphan tables** (has data, no relationships) — each row shows the table name and an "Add Relationship" button that opens the AddRelationshipDialog pre-filled with that table as the source
- List of **empty tables** (0 rows) — each row shows the table name with an "Empty" badge and a note that relationships can't be discovered without data
- Only render this section if `tablesWithoutRelationships > 0`

**3. Pre-fill AddRelationshipDialog with source table:**

The `AddRelationshipDialog` component needs a new optional prop `defaultSourceTable?: string` so that clicking "Add Relationship" on a specific orphan table pre-selects that table as the source.

### `ui/src/components/AddRelationshipDialog.tsx`

Add optional `defaultSourceTable` prop:
- When provided, pre-select the source table dropdown
- Reset when dialog closes

## Checklist

- [ ] Change warning banner CTA from "Go to Ontology" to scroll to "#no-relationships-section"
- [ ] Add "No Relationships" Card section below the Relationships card with orphan and empty table lists
- [ ] Add `defaultSourceTable` prop to `AddRelationshipDialog`
- [ ] Wire "Add Relationship" button on each orphan table row to open dialog with pre-filled source table
- [ ] Run frontend tests: `cd ui && npm test`
- [ ] Run frontend lint: `cd ui && npm run lint`

## Files to Modify

| File | Changes |
|------|---------|
| `ui/src/pages/RelationshipsPage.tsx` | Change CTA, add No Relationships section |
| `ui/src/components/AddRelationshipDialog.tsx` | Add `defaultSourceTable` prop |

# PLAN: Add Description to Relationships Screen

## Goal

Display LLM-generated relationship descriptions on the Relationships page.

## Current State

### Data Available
- `engine_entity_relationships.description` column exists and is populated by RelationshipEnrichment (DAG stage 6)
- Example: "Links a user to the channels they own through their account association. Each user's account can own and manage multiple content channels."

### What's Missing
- API does not return `description` field
- UI has no place to display it

### Key Files

| File | Current State |
|------|---------------|
| `pkg/handlers/entity_relationship_handler.go` | `EntityRelationshipResponse` struct has NO description field; `List` handler doesn't map it |
| `pkg/models/entity_relationship.go` | `EntityRelationship` model has `Description *string` field |
| `ui/src/types/schema.ts` | `RelationshipDetail` interface has NO description field |
| `ui/src/pages/RelationshipsPage.tsx` | No description rendering |

## Implementation

### 1. Backend: Add Description to API Response ✅

**Status**: COMPLETE

**Files Modified**:
- `pkg/handlers/entity_relationship_handler.go` - Added `Description` field to `EntityRelationshipResponse` struct, mapped it in `List` handler, added `deref` helper function
- `pkg/handlers/entity_relationship_handler_test.go` - Added comprehensive tests for description mapping including null handling

**Changes Made**:
1. Added `Description string` field to `EntityRelationshipResponse` struct (line 34)
2. Mapped `rel.Description` to response in `List` handler using `deref` helper (line 183)
3. Created `deref` helper function to safely handle nil string pointers (lines 202-206)
4. Added test `TestEntityRelationshipHandler_List_DescriptionMapping` covering:
   - Description present case
   - Description null case (returns empty string)
   - Multiple relationships with different descriptions

**Implementation Notes**:
- The `deref` helper converts `nil` to empty string, making it safe for JSON serialization
- Used `omitempty` tag so empty descriptions don't clutter the JSON response
- All tests passing, validates correct API contract

### 2. Frontend: Add Description to Type ✅

**Status**: COMPLETE

**File Modified**: `ui/src/types/schema.ts`

**Changes Made**:
- Added optional `description?: string` field to `RelationshipDetail` interface (line 118)
- Field is optional to handle relationships that don't have descriptions yet
- Positioned after `is_approved` field, before `created_at`

**Implementation Notes**:
- Used optional field (`?`) since descriptions are only populated after DAG stage 6 completes
- TypeScript type now matches the API response from backend

### 3. Frontend: Display Description in UI ✅

**Status**: COMPLETE

**File Modified**: `ui/src/pages/RelationshipsPage.tsx`

**Changes Made**:
- Added description rendering between column mapping and cardinality display (lines 595-599)
- Used conditional rendering `{rel.description && ...}` to only show description when present
- Styled with `text-sm text-text-secondary` classes and `mt-1` margin as specified
- Description appears as a paragraph element below the relationship column mapping

**Implementation Notes**:
- Positioned description between column mapping and cardinality for logical flow
- Conditional rendering ensures graceful handling of relationships without descriptions
- TypeScript type checking passes without errors

## UI Design

```
┌──────────────────────────────────────────────────────────────────────────┐
│ users (4 relationships)                                                  │
├──────────────────────────────────────────────────────────────────────────┤
│ 🔗 account_id (uuid) → accounts . account_id (uuid)      [Foreign Key]  │
│    Links a user to their account, establishing the primary membership    │
│    relationship. This represents the user's direct access and            │
│    permissions within their account.                                     │
│                                                                          │
│ 💡 account_id (uuid) → channels . channel_id (uuid)      [Inferred]     │
│    Links a user to the channels they own through their account           │
│    association. Each user's account can own and manage multiple          │
│    content channels.                                                     │
└──────────────────────────────────────────────────────────────────────────┘
```

## Testing

1. Verify `engine_entity_relationships.description` has data:
   ```sql
   SELECT source_column_table, source_column_name, description
   FROM engine_entity_relationships
   WHERE description IS NOT NULL LIMIT 5;
   ```

2. Check API response includes description:
   ```bash
   curl http://localhost:3443/api/projects/{pid}/relationships | jq '.data.relationships[0]'
   ```

3. Visual verification: Relationships page shows descriptions below each relationship

## Notes

- Description is only populated after RelationshipEnrichment (DAG stage 6) runs
- Relationships without descriptions should gracefully show nothing (already handled by `{rel.description && ...}`)

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

### 2. Frontend: Add Description to Type

**File**: `ui/src/types/schema.ts`

Add to `RelationshipDetail` interface (around line 104):
```typescript
export interface RelationshipDetail {
  // ... existing fields ...
  description?: string;  // ADD THIS
}
```

### 3. Frontend: Display Description in UI

**File**: `ui/src/pages/RelationshipsPage.tsx`

In the relationship row (around line 594), add description below the column mapping:

```typescript
{/* Relationship Details */}
<div className="flex-1 min-w-0">
  <div className="flex items-center gap-2 text-sm">
    {/* existing column mapping code */}
  </div>

  {/* ADD THIS: Description */}
  {rel.description && (
    <p className="mt-1 text-sm text-text-secondary">
      {rel.description}
    </p>
  )}

  {/* existing cardinality display */}
  {rel.cardinality && (
    <div className="mt-1 text-xs text-text-secondary">
      Cardinality: {rel.cardinality}
    </div>
  )}
</div>
```

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

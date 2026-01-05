# FIX: Remove Occurrences Table, Consolidate into Relationships

## Problem Statement

The `engine_ontology_entity_occurrences` table is redundant and incomplete:

1. **Only stores primary location**: Entity Discovery creates exactly 1 occurrence per entity (the PK column). No additional occurrences are ever created from relationships.

2. **Relationships already have the data**: `engine_entity_relationships` stores both source and target column locations. An occurrence is derivable: "Entity X appears at `source_column_*` because it references Entity X's primary table."

3. **Role field is orphaned**: Column Enrichment tries to call `UpdateOccurrenceRole()` to set FK roles like "visitor", "host", "owner" - but those occurrences don't exist because they're never created from relationships.

4. **UI shows "1 occurrence" always**: Users see every entity with "1 occurrence" even after relationships are discovered.

## Proposed Solution

### Rename "role" to "association"

The term "role" is confusing because it implies a human actor. "Association" better describes the semantic relationship:
- "the shopping cart **contains** items"
- "items **are in** shopping carts"
- "items **belong to** inventory management"
- "order **placed_by** user"
- "user **as host** in visits"

### Store bidirectional relationship rows

Currently a relationship is stored as one row: `orders.user_id → users.id`

Store TWO rows per relationship (one per direction), each with its own association:
- Forward: `Order → User` with association "placed_by" (or null)
- Reverse: `User → Order` with association "places" (or null)

This allows:
- "Show me all entities that reference User" (inbound)
- "Show me all entities that User references" (outbound)
- Natural language descriptions for both directions

### Calculate occurrences at runtime

Instead of a separate table, compute occurrences from:
1. The entity's primary location (`primary_schema.primary_table.primary_column`)
2. All inbound relationships where `target_entity_id = entity.id`

An entity with 0 inbound relationships has 0 additional occurrences (just its primary location). This is semantically correct.

### Drop the occurrences table

After migration, drop `engine_ontology_entity_occurrences` entirely.

---

## Implementation Plan

### Phase 1: Schema Changes

#### 1.1 Add `association` column to relationships ✅ COMPLETED

**Implementation Notes:**
- Migration 027 created: `migrations/027_add_relationship_association.up.sql`
- Down migration created: `migrations/027_add_relationship_association.down.sql`
- Integration test added: `pkg/repositories/entity_relationship_migration_test.go`
- Test verifies column exists, has correct type (VARCHAR 100), and has comment
- Column is nullable to support existing relationships (will be populated during enrichment)

```sql
-- Migration: add_relationship_association.up.sql
ALTER TABLE engine_entity_relationships
    ADD COLUMN association VARCHAR(100);

COMMENT ON COLUMN engine_entity_relationships.association IS
    'Semantic association describing this direction of the relationship (e.g., "placed_by", "contains", "as host")';
```

#### 1.2 Create reverse rows for existing relationships ✅ COMPLETED

**Implementation Notes:**
- Migration 028 created: `migrations/028_fix_relationship_unique_constraint.up.sql`
  - Fixed unique constraint to include target column names (source_column_name AND target_column_name)
  - Previous constraint caused collisions when multiple FKs from same source table referenced same target table
  - New constraint: `engine_entity_relationships_unique_relationship` includes all 9 key columns
- Migration 029 created: `migrations/029_create_reverse_relationships.up.sql`
  - Creates reverse rows for all existing relationships by swapping source/target entities and columns
  - Uses NOT EXISTS check to avoid creating duplicates
  - Sets association and description to NULL for new reverse rows (will be populated during enrichment)
  - Down migration removes reverse rows while keeping one direction
- Integration test added: `pkg/repositories/reverse_relationships_migration_test.go`
  - TestMigration028_FixRelationshipUniqueConstraint verifies constraint was updated
  - TestMigration029_ReverseRelationships verifies reverse rows are created correctly
  - Test validates entity/column swapping and bidirectional pair existence

For each existing relationship, create a reverse row:
- Source and target entities are swapped
- Source and target columns are swapped
- Each direction gets its own association (populated later by LLM enrichment)

### Phase 2: Code Changes

#### 2.1 Update relationship creation ✅ COMPLETED

**Implementation Notes:**
- Added `createBidirectionalRelationship()` helper method in `pkg/services/deterministic_relationship_service.go`
  - Creates both forward (FK direction) and reverse relationship rows in sequence
  - Swaps source/target entities and columns for reverse direction
  - Sets reverse description to nil (will be populated during Relationship Enrichment)
- Updated both FK discovery and PK-match discovery to use the new helper
- Added unit test `TestCreateBidirectionalRelationship` to verify both rows are created
- Updated existing PK-match tests to expect 2x relationship rows (2 logical relationships x 2 directions = 4 rows)

**Key Design Decision:**
- Relationships are created sequentially (forward then reverse) rather than in a transaction
- This is safe because the unique constraint includes all 9 columns (source/target entity + schema + table + column)
- If the reverse creation fails, the forward row remains (partial state) but subsequent enrichment/discovery will be consistent
- Future sessions can continue from any partial state

**Files modified:**
- `pkg/services/deterministic_relationship_service.go` - Added bidirectional creation logic
- `pkg/services/deterministic_relationship_service_test.go` - Added unit test and updated existing tests

#### 2.2 Generate associations during Relationship Enrichment ✅ COMPLETED

**Implementation Notes:**
- Updated `relationshipEnrichment` struct in `pkg/services/relationship_enrichment.go` to include `Association` field
- Modified LLM prompt to request both description (full sentence) and association (short verb/label like "placed_by", "owns", "manages")
- Updated JSON response parsing to extract both `description` and `association` fields
- Modified `enrichBatchInternal` to save association alongside description via new repository method
- Added `UpdateDescriptionAndAssociation` in `pkg/repositories/entity_relationship_repository.go` for atomic updates
- Implemented fallback to `UpdateDescription` if LLM response doesn't include association (backward compatibility)
- Updated all test mocks in:
  - `pkg/services/column_enrichment_test.go`
  - `pkg/services/deterministic_relationship_service_test.go`
  - `pkg/services/ontology_context_test.go`
  - `pkg/services/ontology_finalization_test.go`
  - `pkg/services/relationship_enrichment_test.go`
  - `pkg/repositories/entity_relationship_migration_test.go`
- All tests pass and `make check` succeeds

**Key Design Decisions:**
- LLM now generates associations for BOTH directions of each relationship during enrichment
- Associations are nullable to support partial enrichment states and backward compatibility
- The repository method validates that relationship exists before updating (fail-fast principle)

The `association` field is now generated alongside `description` during the existing Relationship Enrichment DAG step.

#### 2.3 Remove Column Enrichment occurrence role updates ✅ COMPLETED

**Implementation Notes:**
- Removed `updateOccurrenceRoles()` method and its call from `EnrichTable()` in `pkg/services/column_enrichment.go`
  - This method attempted to update occurrence roles based on FK role enrichments
  - It was orphaned code - tried to update occurrence roles that were never created from relationships
  - Removed 88 lines including the method implementation (lines 785-859)
  - Removed the method call from `EnrichTable()` (lines 220-228)
- Removed test `TestColumnEnrichmentService_UpdateOccurrenceRoles` from `pkg/services/column_enrichment_test.go`
  - Removed 176 lines including test and helper mocks (`testColEnrichmentEntityRepoWithRoleTracking`, `testColEnrichmentRelRepoWithFKs`)
- Associations are now ONLY set during Relationship Enrichment (task 2.2), not during Column Enrichment
- This completes the cleanup of occurrence-related code from the column enrichment flow
- All tests pass (`make check` succeeds)

**Key Design Decision:**
- Associations belong on relationships, not on column enrichments
- The FK role enrichment logic in column enrichment was attempting to bridge the gap to occurrences
- With bidirectional relationships and association generation during Relationship Enrichment, this bridge is no longer needed

#### 2.5 Compute occurrences at runtime in Entity Service

This is the key change that makes the Entities UI show correct occurrence counts. The UI calls `GET /api/projects/{pid}/entities`, which calls `EntityService.ListByProject()`. By computing occurrences from relationships here, the UI automatically displays accurate counts without any frontend changes to data fetching.

**Files to modify:**
- `pkg/services/entity_service.go:69-121` (ListByProject)
  - Remove: `s.entityRepo.GetOccurrencesByEntity(ctx, entity.ID)`
  - Add: Query relationships where `target_entity_id = entity.ID`
  - Build occurrence list from relationship source columns + associations

- `pkg/services/entity_service.go:123-161` (GetByID)
  - Same change

**New helper function:**
```go
func (s *entityService) computeOccurrences(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
    // Get inbound relationships for this entity
    rels, err := s.relationshipRepo.GetByTargetEntity(ctx, entityID)
    if err != nil {
        return nil, err
    }

    var occurrences []*models.OntologyEntityOccurrence
    for _, rel := range rels {
        occurrences = append(occurrences, &models.OntologyEntityOccurrence{
            EntityID:   entityID,
            SchemaName: rel.SourceColumnSchema,
            TableName:  rel.SourceColumnTable,
            ColumnName: rel.SourceColumnName,
            Role:       rel.Association, // renamed from Role
            Confidence: rel.Confidence,
        })
    }
    return occurrences, nil
}
```

#### 2.6 Update OntologyContextService

**Files to modify:**
- `pkg/services/ontology_context.go:59-167` (GetDomainContext)
  - Replace: `s.entityRepo.GetAllOccurrencesByProject(ctx, projectID)`
  - With: Compute occurrence counts from relationships

- `pkg/services/ontology_context.go:169-256` (GetEntitiesContext)
  - Replace occurrence fetching with relationship-based computation

#### 2.7 Remove occurrence repository methods

**Files to modify:**
- `pkg/repositories/ontology_entity_repository.go`
  - Remove: `CreateOccurrence`, `GetOccurrencesByEntity`, `GetOccurrencesByTable`, `GetAllOccurrencesByProject`, `UpdateOccurrenceRole`
  - Remove: `scanOntologyEntityOccurrence` helper

- `pkg/repositories/ontology_entity_repository_test.go`
  - Remove tests for occurrence methods

#### 2.8 Remove occurrence from Entity Discovery

**Files to modify:**
- `pkg/services/entity_discovery_service.go:177-198`
  - Remove: `CreateOccurrence` call after entity creation
  - The primary location is already stored in the entity itself

#### 2.9 Update relationship repository

**Files to modify:**
- `pkg/repositories/entity_relationship_repository.go`
  - Add: `GetByTargetEntity(ctx, entityID uuid.UUID) ([]*models.EntityRelationship, error)`
  - This returns all relationships where the entity is the target (inbound)

### Phase 3: Model Changes

#### 3.1 Update EntityRelationship model

**File:** `pkg/models/entity_relationship.go`
```go
type EntityRelationship struct {
    // ... existing fields ...
    Association *string `json:"association,omitempty"` // NEW: semantic association for this direction
}
```

#### 3.2 Keep OntologyEntityOccurrence as a derived type

**File:** `pkg/models/ontology_entity.go`

Keep `OntologyEntityOccurrence` struct but rename `Role` to `Association`:
```go
// OntologyEntityOccurrence represents a computed occurrence of an entity.
// No longer stored in database - derived from relationships at runtime.
type OntologyEntityOccurrence struct {
    ID          uuid.UUID `json:"id"`          // Can be relationship ID or generated
    EntityID    uuid.UUID `json:"entity_id"`
    SchemaName  string    `json:"schema_name"`
    TableName   string    `json:"table_name"`
    ColumnName  string    `json:"column_name"`
    Association *string   `json:"association,omitempty"` // renamed from Role
    Confidence  float64   `json:"confidence"`
    CreatedAt   time.Time `json:"created_at"`
}
```

### Phase 4: Migration

#### 4.1 Drop occurrences table

```sql
-- Migration: drop_occurrences_table.up.sql
DROP TABLE IF EXISTS engine_ontology_entity_occurrences;

-- Migration: drop_occurrences_table.down.sql
-- Recreate table (copy from migration 013 or 014)
CREATE TABLE engine_ontology_entity_occurrences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    schema_name VARCHAR(255) NOT NULL,
    table_name VARCHAR(255) NOT NULL,
    column_name VARCHAR(255) NOT NULL,
    role VARCHAR(100),
    confidence FLOAT NOT NULL DEFAULT 1.0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, schema_name, table_name, column_name)
);
```

### Phase 5: UI Changes

#### 5.1 Rename "role" to "association" in types

**File:** `ui/src/types/entity.ts`
```typescript
export interface EntityOccurrence {
  // ...
  association: string | null;  // renamed from role
  // ...
}
```

#### 5.2 Update EntitiesPage display

**File:** `ui/src/pages/EntitiesPage.tsx:296`
- Change `occ.role` to `occ.association`

### Phase 6: Test Updates

#### 6.1 Update integration tests

**Files to modify:**
- `pkg/services/ontology_context_integration_test.go` - Remove `CreateOccurrence` setup calls
- `pkg/services/pk_match_integration_test.go` - Remove occurrence assertions
- `pkg/handlers/entity_integration_test.go` - Update to test computed occurrences
- `pkg/handlers/glossary_integration_test.go` - Remove occurrence setup

#### 6.2 Update unit tests

**Files to modify:**
- `pkg/services/entity_discovery_service_test.go` (if exists)
- `pkg/services/column_enrichment_test.go` - Update mock to remove occurrence methods
- `pkg/services/ontology_context_test.go` - Update mock
- Various mock implementations that implement `OntologyEntityRepository`

---

## Files Summary

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/models/entity_relationship.go` | Add `Association` field |
| `pkg/models/ontology_entity.go` | Rename `Role` to `Association`, mark as computed |
| `pkg/repositories/ontology_entity_repository.go` | Remove occurrence methods |
| `pkg/repositories/entity_relationship_repository.go` | Add `GetByTargetEntity` |
| `pkg/services/entity_service.go` | Compute occurrences from relationships |
| `pkg/services/entity_discovery_service.go` | Remove occurrence creation |
| `pkg/services/relationship_enrichment.go` | Generate `association` alongside `description` in LLM prompt |
| `pkg/services/column_enrichment.go` | Remove `updateOccurrenceRoles()` entirely |
| `pkg/services/ontology_context.go` | Compute occurrence counts from relationships |
| `pkg/services/deterministic_relationship_service.go` | Create bidirectional rows |
| `pkg/handlers/entity_handler.go` | May need minor updates for response structure |
| `ui/src/types/entity.ts` | Rename `role` to `association` |
| `ui/src/pages/EntitiesPage.tsx` | Update display of association |

### Files to Create

| File | Purpose |
|------|---------|
| `migrations/XXX_add_relationship_association.up.sql` | Add association column |
| `migrations/XXX_add_relationship_association.down.sql` | Remove association column |
| `migrations/XXX_drop_occurrences_table.up.sql` | Drop occurrences table |
| `migrations/XXX_drop_occurrences_table.down.sql` | Recreate occurrences table |

### Test Files to Update

| File | Changes |
|------|---------|
| `pkg/repositories/ontology_entity_repository_test.go` | Remove occurrence tests |
| `pkg/services/relationship_enrichment_test.go` | Update to test association generation |
| `pkg/services/column_enrichment_test.go` | Remove occurrence role mocking |
| `pkg/services/ontology_context_test.go` | Update mock |
| `pkg/services/ontology_context_integration_test.go` | Remove occurrence setup |
| `pkg/services/pk_match_integration_test.go` | Update assertions |
| `pkg/handlers/entity_integration_test.go` | Update tests |

---

## Verification Checklist

After implementation:

- [ ] Entities page shows correct occurrence count (derived from relationships)
- [ ] Entities page expands to show occurrence details with associations
- [ ] MCP `get_ontology` returns correct entity occurrences
- [ ] Column enrichment sets associations on relationships
- [ ] No references to `engine_ontology_entity_occurrences` table remain
- [ ] All tests pass
- [ ] Build succeeds

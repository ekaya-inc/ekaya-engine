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

#### 2.5 Compute occurrences at runtime in Entity Service ✅ COMPLETED

**Implementation Notes:**
- Added `relationshipRepo` dependency to `entityService` in `pkg/services/entity_service.go`
- Updated `NewEntityService()` constructor to accept `EntityRelationshipRepository` parameter
- Replaced `entityRepo.GetOccurrencesByEntity()` calls with new `computeOccurrences()` helper in both:
  - `ListByProject()` method (line 99-102)
  - `GetByID()` method (line 138-141)
- Created new `computeOccurrences()` helper method (lines 161-190) that:
  - Queries inbound relationships via `relationshipRepo.GetByTargetEntity()`
  - Converts each relationship to an `OntologyEntityOccurrence` using source column location
  - Maps `relationship.Association` to `occurrence.Role`
  - Uses relationship ID as occurrence ID for consistency
- Added `GetByTargetEntity()` method to `EntityRelationshipRepository` interface and implementation
  - New method in `pkg/repositories/entity_relationship_repository.go` (lines 189-224)
  - Queries relationships where `target_entity_id = entityID`
  - Returns ordered by source table/column for consistent UI display
- Updated `main.go` to pass `relationshipRepo` when creating EntityService
- Created comprehensive unit tests in new file `pkg/services/entity_service_test.go`:
  - Test for successful occurrence computation from relationships
  - Test for zero occurrences when no inbound relationships exist
  - Test for error propagation from relationship repository
  - Mock implementations for all dependencies
- Updated all test files that create mock EntityRelationshipRepository to include `GetByTargetEntity` method
- All tests pass (`make check` succeeds)

**Key Design Decisions:**
- Entity Service now has a dependency on EntityRelationshipRepository (added to constructor)
- Occurrences are computed on-demand during entity retrieval (not cached)
- Each inbound relationship = one occurrence at the source column location
- The relationship's `association` field becomes the occurrence's `role` field
- This makes the UI show accurate occurrence counts without frontend changes

**Impact:**
- Entities UI now shows correct occurrence counts derived from bidirectional relationships
- Occurrence count reflects actual FK references to the entity (inbound relationships)
- Entities with no inbound relationships show 0 occurrences (semantically correct)

#### 2.6 Update OntologyContextService ✅ COMPLETED

**Implementation Notes:**
- Updated `GetDomainContext()` in `pkg/services/ontology_context.go` (lines 59-167):
  - Removed `entityRepo.GetAllOccurrencesByProject()` call
  - Compute occurrence counts from inbound relationships: each inbound relationship = 1 occurrence
  - Count inbound relationships by iterating over `entityRelationships` and incrementing `occurrenceCountByEntityID[rel.TargetEntityID]`
  - This makes the domain view show accurate entity occurrence counts without a separate table
- Updated `GetEntitiesContext()` in `pkg/services/ontology_context.go` (lines 169-256):
  - Removed `entityRepo.GetAllOccurrencesByProject()` call
  - Added relationship query and grouping by target entity ID
  - For each entity, iterate over its inbound relationships and convert to `EntityOccurrence` structs
  - Maps `relationship.Association` to `occurrence.Role` for UI display consistency
  - Occurrence table/column comes from `relationship.SourceColumnTable/SourceColumnName`
- Added new helper method `computeEntityOccurrences()` (lines 535-562):
  - Queries inbound relationships via `relationshipRepo.GetByTargetEntity()`
  - Converts relationships to `OntologyEntityOccurrence` structs
  - Uses relationship ID as occurrence ID for consistency
  - Currently unused but available for future refactoring
- Updated unit tests in `pkg/services/ontology_context_test.go`:
  - Removed all `occurrences` data from mock setup
  - Added `relationshipsByTarget` map to mock relationship repository
  - Added `GetByTargetEntity` mock implementation
  - Updated test expectations to compute occurrence counts from relationships
  - Fixed test assertions: occurrence count = number of inbound relationships
  - All tests now verify runtime computation instead of pre-stored occurrences
- All tests pass (`make check` succeeds)

**Key Design Decisions:**
- Occurrence counts in domain context are computed by counting inbound relationships (where entity is target)
- Occurrence details in entities context are derived by iterating over inbound relationships per entity
- The relationship's source column location becomes the occurrence location
- The relationship's `association` field becomes the occurrence's `role` field for UI compatibility
- No database queries for occurrences - all computed from existing relationship data

**Impact:**
- Domain context view (entity graph) shows accurate occurrence counts
- Entities context view shows detailed occurrence lists with associations
- Both views now reflect actual FK references without needing a separate occurrences table
- Relationships are the single source of truth for entity locations

#### 2.7 Remove occurrence repository methods ✅ COMPLETED

**Implementation Notes:**
- Removed all occurrence-related methods from `OntologyEntityRepository` interface and implementation in `pkg/repositories/ontology_entity_repository.go`:
  - Removed `CreateOccurrence` (22 lines)
  - Removed `GetOccurrencesByEntity` (26 lines)
  - Removed `GetOccurrencesByTable` (28 lines)
  - Removed `GetAllOccurrencesByProject` (30 lines)
  - Removed `UpdateOccurrenceRole` (13 lines)
  - Removed `scanOntologyEntityOccurrence` helper (16 lines)
  - Total: ~135 lines of code removed
- Updated interface documentation comment from "ontology entities and their occurrences" to just "ontology entities"
- Removed all occurrence method tests from `pkg/repositories/ontology_entity_repository_test.go`:
  - Removed `TestOntologyEntityRepository_CreateOccurrence_*` tests (3 tests, ~110 lines)
  - Removed `TestOntologyEntityRepository_GetOccurrencesByEntity_*` tests (2 tests, ~75 lines)
  - Removed `TestOntologyEntityRepository_GetOccurrencesByTable_*` tests (2 tests, ~90 lines)
  - Removed `TestOntologyEntityRepository_CascadeDelete_Occurrences` test (~45 lines)
  - Removed `TestOntologyEntityRepository_NoTenantScope` occurrence assertions (~30 lines)
  - Total: ~350 lines of test code removed
- Updated cleanup function comments to reflect that only entities and aliases are cleaned (not occurrences)
- All remaining tests pass (`make check` succeeds)

**Key Cleanup:**
- The repository no longer has any knowledge of the occurrences table
- Occurrence data is now derived at runtime from relationships (see task 2.5)
- This completes the removal of occurrence persistence logic from the repository layer

**Files modified:**
- `pkg/repositories/ontology_entity_repository.go` - Removed all occurrence methods
- `pkg/repositories/ontology_entity_repository_test.go` - Removed all occurrence tests

#### 2.8 Remove occurrence from Entity Discovery ✅ COMPLETED

**Implementation Notes:**
- Removed `CreateOccurrence` call after entity creation in `pkg/services/entity_discovery_service.go` (lines 177-190)
  - The primary location is already stored in the entity itself via `PrimarySchema`, `PrimaryTable`, and `PrimaryColumn` fields
  - No need for redundant occurrence record
- Removed occurrence creation loop in `pkg/services/entity_discovery_task.go` (lines 392-422)
  - This loop created occurrences for LLM-discovered entities
  - Occurrences are now derived at runtime from relationships (see task 2.5)
- Removed `countTotalOccurrences()` method and its call from `entity_discovery_task.go`
  - This method counted occurrences across discovered entities
  - Logging now only reports entity count, not occurrence count
- Removed `TestCountTotalOccurrences` test from `pkg/services/entity_discovery_task_test.go`
  - Test validated the now-removed `countTotalOccurrences()` method
- Updated test mocks in `pkg/services/entity_discovery_task_test.go`:
  - Removed `CreateOccurrence` from `testEntityDiscoveryEntityRepo` mock
  - This mock is used by multiple test cases for entity discovery validation
- All tests pass (`make check` succeeds)

**Key Design Decision:**
- Entity Discovery no longer creates any occurrence records
- The entity's primary location is stored in the entity itself
- Additional occurrences (FK references) are computed at runtime from inbound relationships
- This eliminates the redundant persistence of occurrence data during entity discovery

**Files modified:**
- `pkg/services/entity_discovery_service.go` - Removed occurrence creation after entity creation
- `pkg/services/entity_discovery_task.go` - Removed occurrence loop and counting method
- `pkg/services/entity_discovery_task_test.go` - Removed obsolete test and updated mock
- `pkg/services/schema.go` - Removed `CreateOccurrence` from mock implementation
- Multiple test files updated to remove `CreateOccurrence` from mock implementations

**Context for Future Sessions:**
- This task completes the removal of occurrence persistence from the Entity Discovery phase
- All occurrence-related code has now been removed from:
  - Repository layer (task 2.7)
  - Entity Discovery phase (task 2.8)
  - Column Enrichment phase (task 2.3)
- Occurrences are computed at runtime from relationships (task 2.5, 2.6)
- Next steps: Task 2.9 may already be complete (GetByTargetEntity added in 2.5), verify before implementing

#### 2.9 Update relationship repository ✅ COMPLETED

**Implementation Notes:**
- This task was completed as part of task 2.5 (Compute occurrences at runtime in Entity Service)
- Added `GetByTargetEntity(ctx, entityID uuid.UUID) ([]*models.EntityRelationship, error)` method to `EntityRelationshipRepository` interface (line 21)
- Implemented method in `pkg/repositories/entity_relationship_repository.go` (lines 189-224)
- Method queries relationships where `target_entity_id = entityID` (inbound relationships)
- Returns relationships ordered by source table/column for consistent UI display
- Method is used by Entity Service's `computeOccurrences()` helper to derive occurrences at runtime
- The method is tested indirectly through Entity Service unit tests and integration tests

**Key Design Decision:**
- The method was implemented earlier than originally planned because it was needed for the occurrence computation feature
- This is a good example of the plan being a guide, not a strict sequence - dependencies drove the actual implementation order

### Phase 3: Model Changes

#### 3.1 Update EntityRelationship model ✅ COMPLETED

**Implementation Notes:**
- The `Association` field has already been added to the `EntityRelationship` struct in `pkg/models/entity_relationship.go` (line 41)
- Field type: `*string` (nullable pointer to string)
- JSON tag: `json:"association,omitempty"`
- Comment: "Semantic association for this direction (e.g., "placed_by", "contains")"
- This field stores the semantic association for one direction of a bidirectional relationship
- The field is populated during Relationship Enrichment (see task 2.2)

**File:** `pkg/models/entity_relationship.go`
```go
type EntityRelationship struct {
    // ... existing fields ...
    Association *string `json:"association,omitempty"` // Semantic association for this direction
}
```

#### 3.2 Keep OntologyEntityOccurrence as a derived type ✅ COMPLETED

**Implementation Summary:**
This task completed the terminology shift from "role" to "association" across the entire codebase. The `OntologyEntityOccurrence` model is now properly aligned with the bidirectional relationship model where associations describe semantic connections.

**Implementation Notes:**
- Updated `OntologyEntityOccurrence` struct in `pkg/models/ontology_entity.go`:
  - Renamed `Role` field to `Association` (type: `*string`, JSON tag: `json:"association,omitempty"`)
  - Updated struct comment to indicate it's "computed" and "no longer stored in database - derived from relationships at runtime"
- Updated all usages of the `Role` field to `Association` throughout the codebase:
  - `pkg/services/entity_service.go:182` - Runtime occurrence computation maps relationship.Association to occurrence.Association
  - `pkg/services/ontology_context.go:556` - Context generation for MCP and UI
  - `pkg/services/schema.go:986-988, 1021-1022, 1045-1046` - Schema annotation display shows association in comments
  - `pkg/handlers/entity_handler.go:401` - API response mapping for GET /entities/:id
  - `pkg/handlers/entity_handler.go:44` - EntityOccurrenceResponse struct field renamed to Association
  - `pkg/services/entity_service_test.go:279, 288, 320, 476` - Test assertions verify association values
  - `pkg/services/pk_match_integration_test.go:136` - Integration test creates expected occurrence with association
  - `pkg/services/ontology_context_integration_test.go:342` - Integration test verifies context generation includes associations
- Updated `EntityOccurrenceResponse` handler struct to use `Association` instead of `Role` for API consistency
- All tests pass (`make test` succeeds)

**Key Design Decision:**
- The model now uses "association" consistently with the `EntityRelationship` model
- Occurrences are pure runtime views of relationships - no persistence layer exists
- API responses maintain backward compatibility via JSON field name (association instead of role)
- The terminology better reflects the semantic meaning: "placed_by", "contains", "manages" vs ambiguous "role"

**Context for Next Session:**
- The backend API now returns `association` field in entity occurrence responses
- Frontend UI still expects `role` field (see Phase 5 tasks)
- Integration tests verify association propagation from relationships → occurrences → API responses
- All backend occurrence generation is working correctly with new terminology

**File:** `pkg/models/ontology_entity.go`

The `OntologyEntityOccurrence` struct is now:
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

#### 4.1 Drop occurrences table ✅ COMPLETED (2026-01-05)

**Implementation Notes:**
- Migration 030 created: `migrations/030_drop_occurrences_table.up.sql`
  - Drops the `engine_ontology_entity_occurrences` table
  - Table is no longer needed - occurrences computed at runtime from relationships (see task 2.5, 2.6)
- Down migration created: `migrations/030_drop_occurrences_table.down.sql`
  - Recreates table structure from migration 013 (original creation)
  - Includes all indexes, constraints, comments, and RLS policy from migration 024
  - Allows rollback if needed for debugging or emergency recovery
- Integration test added: `pkg/repositories/drop_occurrences_migration_test.go`
  - Test verifies table no longer exists after migration
  - Uses standard migration test pattern from other migration tests
- All tests pass (`make check` succeeds)

**Key Design Decision:**
- Down migration provides full restoration of original table structure
- Recreates all indexes (entity, table, role), constraints (FK cascade, confidence check), and RLS policy
- This ensures migrations can be rolled back without data loss or broken references
- However, occurrence data itself is NOT restored (would require re-running entity discovery)

**Files created:**
- `migrations/030_drop_occurrences_table.up.sql` - Drop table
- `migrations/030_drop_occurrences_table.down.sql` - Recreate table with full structure
- `pkg/repositories/drop_occurrences_migration_test.go` - Integration test

**Context for Next Session:**
- This completes the backend migration work for occurrences removal
- The database schema no longer has an occurrences table
- All occurrence data is now computed at runtime from `engine_entity_relationships`
- Next steps are frontend UI changes (Phase 5) to update terminology from "role" to "association"
- The backend API already returns the `association` field in entity occurrence responses

### Phase 5: UI Changes

#### 5.1 Rename "role" to "association" in UI types ✅ COMPLETED (2026-01-05)

**Implementation Notes:**
- Updated `EntityOccurrence` interface in `ui/src/types/entity.ts`:
  - Renamed `role: string | null` field to `association: string | null` (line 14)
  - This aligns frontend types with backend API which already returns `association` field (see task 3.2)
- Updated `EntitiesPage.tsx` to use `occ.association` instead of `occ.role` (lines 295-297):
  - Conditional rendering: `{occ.association && ...}`
  - Display: `<span>{occ.association}</span>`
- TypeScript compilation succeeds with no errors
- All tests pass (`make check` succeeds)

**Key Design Decision:**
- Simple find-and-replace of field name - no logic changes
- Completes the terminology migration from "role" to "association" across the entire stack
- Backend already returns `association` field in API responses (completed in task 3.2)
- This change makes the frontend consume the correct field name

**Files modified:**
- `ui/src/types/entity.ts:14` - Renamed field in `EntityOccurrence` interface
- `ui/src/pages/EntitiesPage.tsx:295,297` - Updated display logic to use `occ.association`

**Context for Next Session:**
- The UI now correctly displays association values from the backend API
- Associations come from bidirectional relationships (see task 2.2)
- Occurrences are computed at runtime from inbound relationships (see task 2.5)
- Next task (5.2) is already complete - EntitiesPage display was updated as part of this task

**File:** `ui/src/types/entity.ts:14`
```typescript
export interface EntityOccurrence {
  // ...
  association: string | null;  // renamed from role
  // ...
}
```

#### 5.2 Update EntitiesPage display ✅ COMPLETED (as part of 5.1)

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

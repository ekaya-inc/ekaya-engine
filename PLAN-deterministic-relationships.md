# PLAN: Deterministic Entity Relationships

## Implementation Status

### What's Working Now

**Phase 1 (FK Discovery): ✅ FULLY IMPLEMENTED**
- Database migration: `017_entity_relationships`
- Backend stack: model → repository → service → handler
- API endpoints:
  - `POST /api/projects/{pid}/datasources/{dsid}/relationships/discover`
  - `GET /api/projects/{pid}/relationships`
- Integrated into relationship workflow (after graph connectivity phase)
- Tested: Returns 0 relationships for test DB (no FK constraints in schema)

### What's Not Yet Done

**Phase 2 (PK-Match Inference): ❌ NOT IMPLEMENTED**
- Algorithm designed but not coded
- Deferred as future work

**LLM-Based Discovery: ❌ COMMENTED OUT**
- 5 phases designed and implemented but commented out in `relationship_workflow.go:479-675`
- Deferred to focus on deterministic FK discovery first

**UI Integration: ❌ TODO**
- UI still reads from old `engine_relationship_candidates` table
- New `engine_entity_relationships` table not connected to frontend
- Workflow modal shows "0 relationships saved" because it checks old tables

---

## Overview

Refactor relationship discovery to be **Entity-centric**. Relationships connect two Entities, not arbitrary columns. This plan covers deterministic discovery only (no LLM).

### Core Principle

Every relationship is: **Source Entity → Target Entity**

Where:
- Source Entity has a column that references...
- Target Entity's primary key

### Two Types of Deterministic Relationships

| Type | Source | Confidence | Example | Status |
|------|--------|------------|---------|--------|
| **Explicit FK** | Database foreign key constraint | 1.0 | `orders.user_id` FK → `users.id` | ✅ Implemented |
| **Inferred PK-match** | Column matches entity PK by type + cardinality | 0.7-0.9 | `visits.host_id` (no FK) → `users.id` | ❌ Future work |

---

## Prerequisites

- Entity discovery completed (entities exist in `engine_ontology_entities`)
- Schema introspection completed (columns have `is_primary_key`, `is_unique`, data types)
- Column statistics available (`row_count`, `distinct_count` in `engine_schema_columns`)

---

## Phase 1: Explicit FK Relationships ✅ IMPLEMENTED

**Goal:** Create relationships from database foreign key constraints.

### Implementation Status

**Files:**
- Migration: `migrations/017_entity_relationships.up.sql`
- Model: `pkg/models/entity_relationship.go`
- Repository: `pkg/repositories/entity_relationship_repository.go`
- Service: `pkg/services/deterministic_relationship_service.go`
- Handler: `pkg/handlers/entity_relationship_handler.go`

**Endpoints:**
- `POST /api/projects/{pid}/datasources/{dsid}/relationships/discover` - Discover FK relationships
- `GET /api/projects/{pid}/relationships` - List all relationships

**Integration:**
- Called from `pkg/services/relationship_workflow.go` after graph connectivity phase
- Writes directly to `engine_entity_relationships` table (not via candidate system)

### Testing Notes

**Test Database Behavior:**
- The test database's user tables (`users`, `user_profiles`, `user_preferences`) do not have FK constraints
- FK discovery correctly returns `0 relationships` for this schema
- The workflow completes successfully, marking status as complete

**Known UI Issue:**
- Workflow modal shows "38 entities" but "0 relationships saved"
- This is expected because FK discovery writes to `engine_entity_relationships`, not to the candidate system
- The UI currently reads from the old candidate tables

### Algorithm

```
FOR each foreign key constraint in schema:
  source_column = FK source column
  target_column = FK target column (referenced)

  source_entity = find entity where occurrence includes source_column's table
  target_entity = find entity where primary_column = target_column

  IF both entities found:
    CREATE relationship(source_entity, target_entity, confidence=1.0, method='foreign_key')
```

### Data Sources

- `engine_schema_foreign_keys` table has FK constraints
- `engine_ontology_entities` has `primary_schema`, `primary_table`, `primary_column`
- `engine_ontology_entity_occurrences` maps columns to entities

---

## Phase 2: Inferred PK-Match Relationships ❌ NOT IMPLEMENTED

**Status:** Future work. Code is designed but not implemented.

**Goal:** Find columns that could reference entity PKs but lack explicit FKs.

### Algorithm

```
FOR each entity E:
  pk_column = E.primary_column
  pk_type = data type of pk_column

  FOR each column C in schema WHERE:
    - C is not E's primary column (not self-reference to PK)
    - C is not already in an FK relationship
    - C has compatible data type with pk_column
    - C has high cardinality (distinct_count / row_count > threshold)
    - C.column_name suggests reference (ends with _id, matches entity name pattern)

  IF C is a candidate:
    confidence = calculate_confidence(type_match, cardinality, name_similarity)
    CREATE relationship(C's entity, E, confidence, method='pk_match')
```

### Candidate Column Criteria

| Criterion | Requirement | Rationale |
|-----------|-------------|-----------|
| Type compatibility | Same or compatible type as PK | uuid→uuid, int→int, varchar→varchar |
| High cardinality | distinct/total > 0.1 | Low cardinality = enum/status, not FK |
| Not already FK | No existing FK constraint | Don't duplicate Phase 1 |
| Naming hint | `{entity}_id`, `{table}_id`, etc. | Increases confidence |

### Confidence Scoring

```
base_confidence = 0.7

IF column_name matches "{entity_name}_id": +0.15
IF column_name matches "{table_name}_id": +0.10
IF exact type match (not just compatible): +0.05
IF cardinality ratio > 0.5: +0.05

MAX confidence = 0.95 (reserve 1.0 for explicit FK)
```

### Implementation

1. Load all entities with their primary columns and types
2. Load all columns with stats (type, distinct_count, row_count)
3. Filter to candidate columns (high cardinality, compatible types)
4. For each candidate, find matching entity PK
5. Calculate confidence and create relationship candidate

---

## Database Changes

### Option A: Extend `engine_relationship_candidates`

Add fields to existing table:
```sql
ALTER TABLE engine_relationship_candidates
  ADD COLUMN source_entity_id UUID REFERENCES engine_ontology_entities(id),
  ADD COLUMN target_entity_id UUID REFERENCES engine_ontology_entities(id);
```

Candidates now reference entities directly. Existing `source_column_id`/`target_column_id` remain for column-level detail.

### Option B: New Entity Relationships Table

```sql
CREATE TABLE engine_entity_relationships (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,
  source_entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id),
  target_entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id),
  source_column_id UUID REFERENCES engine_schema_columns(id),
  target_column_id UUID REFERENCES engine_schema_columns(id),
  detection_method VARCHAR(50) NOT NULL, -- 'foreign_key', 'pk_match', 'llm'
  confidence DECIMAL(3,2) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'confirmed', -- 'confirmed', 'pending', 'rejected'
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(ontology_id, source_entity_id, target_entity_id, source_column_id)
);
```

**Recommendation:** Option B - cleaner separation, entity-first design.

---

## Service Changes

### New Service: `DeterministicRelationshipService`

```go
type DeterministicRelationshipService interface {
    // Discover relationships deterministically (FK + PK-match)
    DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*DiscoveryResult, error)

    // Get discovered relationships for a project
    GetRelationships(ctx context.Context, projectID uuid.UUID) ([]*EntityRelationship, error)
}

type EntityRelationship struct {
    ID             uuid.UUID
    SourceEntity   *Entity  // With name, primary location
    TargetEntity   *Entity
    SourceColumn   *Column  // Specific FK/reference column
    TargetColumn   *Column  // PK column
    DetectionMethod string  // "foreign_key" or "pk_match"
    Confidence     float64
    Status         string
}

type DiscoveryResult struct {
    FKRelationships     int
    InferredRelationships int
    TotalRelationships  int
}
```

### Implementation Steps

1. **Get entities** - Load all entities for project with their PKs
2. **Get FK constraints** - Query `engine_schema_foreign_keys`
3. **Phase 1: FK relationships** - Map FKs to entity pairs
4. **Get candidate columns** - Filter by type, cardinality, not-already-FK
5. **Phase 2: PK-match** - Match candidates to entity PKs
6. **Persist** - Save to `engine_entity_relationships`

---

## Handler Changes

### Simplify or Replace Workflow

Current workflow has 6 phases with LLM. For deterministic-only:

**Option A:** New endpoint, keep old workflow
```
POST /api/projects/{pid}/datasources/{dsid}/relationships/discover-deterministic
GET  /api/projects/{pid}/relationships  (entity relationships)
```

**Option B:** Replace workflow phases 1-4 with deterministic, keep LLM phase optional
- Phase 1: FK relationships (new)
- Phase 2: PK-match relationships (new)
- Phase 3: LLM enrichment (optional, future)

**Recommendation:** Option A for now - non-breaking, can deprecate old workflow later.

---

## UI Changes

### Minimal Changes to RelationshipsPage

1. **Update relationship display** to show Entity names instead of just table.column
2. **Add detection method badge** ("FK" vs "Inferred")
3. **Show confidence** for inferred relationships

### RelationshipDiscoveryProgress

- Simplify to 2 phases (FK scan, PK-match scan)
- Remove entity discovery display (entities already exist)
- Show relationship counts as they're found

---

## Implementation Order

| Step | Description | Files | Status |
|------|-------------|-------|--------|
| 1 | Create migration for `engine_entity_relationships` | `migrations/017_entity_relationships.up.sql` | ✅ Done |
| 2 | Create `EntityRelationship` model | `pkg/models/entity_relationship.go` | ✅ Done |
| 3 | Create repository | `pkg/repositories/entity_relationship_repository.go` | ✅ Done |
| 4 | Implement FK discovery | `pkg/services/deterministic_relationship_service.go` | ✅ Done |
| 5 | Implement PK-match discovery | Same file, second phase | ❌ Future |
| 6 | Create handler | `pkg/handlers/entity_relationship_handler.go` | ✅ Done |
| 7 | Wire in main.go | `main.go` | ✅ Done |
| 8 | Integrate into workflow | `pkg/services/relationship_workflow.go` | ✅ Done |
| 9 | Update UI types | `ui/src/types/relationships.ts` | ❌ TODO |
| 10 | Update RelationshipsPage | `ui/src/pages/RelationshipsPage.tsx` | ❌ TODO |

---

## Testing

**Backend (Phase 1 - FK Discovery):**
- [x] FK relationships created for all foreign keys
- [x] No duplicate relationships (same entity pair + column)
- [x] Workflow integration works (runs after graph connectivity)
- [x] Returns 0 relationships for test DB (no FKs in schema)
- [ ] PK-match finds inferred relationships with correct confidence (not implemented)
- [ ] Columns already in FK relationships excluded from PK-match (not implemented)

**Frontend:**
- [ ] UI displays entity names and detection methods (not connected to new tables yet)
- [ ] Workflow modal shows correct relationship counts (shows 0 due to reading old tables)

**Build:**
- [x] `make check` passes

---

## Deferred LLM-Based Phases

The following phases were designed but commented out in `pkg/services/relationship_workflow.go`:

1. **Column Scanning** - LLM analysis of column names and types
2. **Value Matching** - Sample-based value overlap detection
3. **Name Inference** - Pattern matching on column names (e.g., `user_id` → `users.id`)
4. **Test Joins** - Query-based join validation
5. **LLM Analysis** - Semantic relationship discovery

**Reason for deferral:** Focus on deterministic FK discovery first. These phases can be re-enabled when needed.

**Location in code:** See `pkg/services/relationship_workflow.go:479-675` (commented block)

---

## Future Enhancements (Out of Scope)

- LLM enrichment for relationship labels ("owns", "created by")
- LLM discovery for complex/semantic relationships
- Relationship cardinality detection (1:1, 1:N, N:M)
- Manual relationship creation from UI
- Phase 2: PK-Match inference (designed but not implemented)

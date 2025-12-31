# PLAN: Entity Discovery Workflow & Relationship Associations

> **Prerequisite:** Complete `PLAN-initial-entities-screen.md` first (migrations, CRUD, empty UI)

## Overview

This plan covers:
1. Moving entity discovery workflow to the Entities screen
2. Adding associations to relationships
3. MCP integration with entity semantics
4. Cleanup of legacy candidate-based code

---

## Phase 1: Entity Discovery Workflow

**Goal:** Wire up the existing entity discovery logic to the new Entities screen.

**Current state:**
- `EntityDiscoveryTask` exists in `pkg/services/entity_discovery_task.go`
- It's currently triggered from the Relationships workflow
- Phases 1-3 (stats, filtering, graph analysis) are implemented

**Changes needed:**

1. Create `pkg/services/entity_workflow.go`:
   - New workflow service for entity discovery
   - Reuses existing tasks: `collectColumnStatistics()`, `filterEntityCandidates()`, `analyzeGraphConnectivity()`, `EntityDiscoveryTask`

2. Create `pkg/handlers/entity_workflow_handler.go`:
   - `POST /api/projects/{pid}/datasources/{dsid}/entities/discover` - Start discovery
   - `GET /api/projects/{pid}/datasources/{dsid}/entities/status` - Workflow status
   - `POST /api/projects/{pid}/datasources/{dsid}/entities/save` - Save discovered entities

3. Update UI (`EntitiesPage.tsx`):
   - Add "Discover Entities" button
   - Show discovery modal with progress (reuse pattern from `RelationshipDiscoveryProgress.tsx`)
   - Display discovered entities for review before saving

**Test:** Click [Discover Entities], verify entities are discovered and persisted.

---

## Phase 2: Relationship Associations

**Goal:** Add semantic associations to relationships ("owns", "sold through", "in basket of").

**Database changes:**
```sql
CREATE TABLE engine_ontology_associations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_id UUID NOT NULL REFERENCES engine_schema_relationships(id) ON DELETE CASCADE,
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('forward', 'reverse')),
    label TEXT NOT NULL,
    source VARCHAR(50),  -- 'discovery', 'user', 'query'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(relationship_id, direction, label)
);
```

**Backend:**
- Add `AssociationRepository` with CRUD operations
- Update relationship response to include associations
- LLM can suggest associations based on column names (owner_id → "owns")

**UI:**
- Show associations on relationship cards
- Allow adding/editing associations manually

**Test:** Relationship between users→channels shows "owns" / "owned by" labels.

---

## Phase 3: Decouple Relationships from Entity Discovery

**Goal:** Relationships workflow should use discovered entities, not re-discover them.

**Changes:**
1. Relationships workflow checks for existing entities first
2. If no entities exist, prompt user to run entity discovery
3. Relationship detection uses entity occurrences to find connections
4. Remove entity discovery code from `relationship_workflow.go`

**Test:**
- With entities: Relationships workflow uses them
- Without entities: Relationships workflow shows "Run Entity Discovery first"

---

## Phase 4: MCP/Ontology Integration

**Goal:** Expose entity/role information to MCP clients for intelligent query generation.

**Why this matters:** When an MCP client asks "show me top 5 hosts by booking count," the LLM needs to know:
- `visits.host_id` represents entity "user" with role "host"
- `visits.visitor_id` represents entity "user" with role "visitor"
- These are different join paths to the same `users` table

**Changes:**
- Update schema context builder to include entity definitions
- Include associations in relationship metadata
- MCP tool descriptions expose entity/role/association semantics

**Test:** MCP schema context includes entity and association information.

---

## Phase 5: Cleanup Legacy Code

**Goal:** Remove candidate-based relationship detection code.

**Files to remove/modify:**
- Remove `ValueMatchTask` usage from relationship workflow
- Remove `NameInferenceTask` usage from relationship workflow
- Remove `TestJoinTask` (or repurpose for validation)
- Remove `AnalyzeRelationshipsTask` (replaced by entity-based approach)
- Consider deprecating `engine_relationship_candidates` table

**Test:** Full `make check` passes, relationship detection works without legacy code.

---

## Summary

| Phase | Description | Depends On |
|-------|-------------|------------|
| 1 | Entity discovery workflow | PLAN-initial-entities-screen.md |
| 2 | Relationship associations | Phase 1 |
| 3 | Decouple relationships | Phase 1 |
| 4 | MCP integration | Phase 2, 3 |
| 5 | Cleanup legacy code | Phase 3 |

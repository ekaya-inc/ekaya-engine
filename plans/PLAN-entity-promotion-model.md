# PLAN: Entity Promotion Model - Earned Entities vs Table Metadata

> **Updated 2026-01-30**: Revised after codebase review. Significant portions already implemented.
> Key changes: Removed duplicate tasks, corrected assumptions about "broken" features, focused on actual gap.

## Problem Statement

The current ontology extractor creates an "entity" for nearly every table, resulting in redundant abstraction. When Entity = Table 1:1, the entity layer adds no semantic value and creates maintenance overhead.

**Current state:** ~38 entities for tikr_production, nearly all are 1:1 with tables (one entity per table)
**Desired state:** 4-5 meaningful entities that aggregate tables or express roles, with everything else as enriched table/column metadata

## Design Decision

Entities should be **earned through semantic complexity**, not auto-created for every table.

### When to Create an Entity

| Condition | Example | Why Entity Adds Value |
|-----------|---------|----------------------|
| Multiple tables represent same concept | `users`, `user_profiles`, `user_settings` → User entity | Groups related tables logically |
| Multiple FKs reference same target with different roles | `host_id`, `visitor_id` both → User | Captures role semantics (host vs visitor) |
| Business aliases exist | "creator", "owner", "participant" all mean User | Maps business language to schema |
| Hub in relationship graph | User connects to 5+ other concepts | Worth visualizing as node |

### When NOT to Create an Entity

| Condition | Example | What to Do Instead |
|-----------|---------|-------------------|
| Entity name = table name, nothing more | Session entity = sessions table | Use table metadata only |
| No aliases or roles | BillingTransaction = billing_transactions | Table description suffices |
| Leaf node with single relationship | PayoutAccount connects only to User | FK metadata on column |

---

## What Already Exists (Codebase Review 2026-01-30)

### Entity Discovery ✅ ALREADY IMPLEMENTED

**File:** `pkg/services/entity_discovery_service.go`

The current system already:
1. **Discovers entities from DDL** (PKs, unique constraints) - NOT string pattern matching
2. **Groups similar tables** (`groupSimilarTables`, `selectPrimaryTable`) to avoid duplicates like `s1_users`, `test_users`
3. **LLM enriches** with clean names, descriptions, domains, key columns, and aliases

```
EntityDiscovery (DAG Node 3) - Deterministic DDL analysis
    ├─ Find PKs and unique constraints
    ├─ Group similar tables (s1_users, test_users → users)
    └─ Create ONE entity per concept group

EntityEnrichment (DAG Node 4) - LLM generates metadata
    ├─ Clean entity names (users → User)
    ├─ Descriptions, domains
    └─ Key columns and aliases
```

### Entity Model ✅ ALREADY IMPLEMENTED

**File:** `pkg/models/ontology_entity.go`

```go
type OntologyEntity struct {
    Name           string    // LLM-generated clean name (e.g., "User")
    Description    string    // LLM explanation
    Domain         string    // Business domain (e.g., "billing", "hospitality")
    PrimarySchema  string    // Schema where entity is primarily defined
    PrimaryTable   string    // Table where entity is primarily defined
    PrimaryColumn  string    // PK column
    Confidence     float64   // 0.0-1.0 (DDL=0.5, after enrichment=0.8)
    // + provenance tracking fields
}
```

### Occurrences ✅ ALREADY IMPLEMENTED (Computed from Relationships)

**File:** `pkg/services/entity_service.go:182-208`

Occurrences are **derived from relationships at runtime**, not stored separately:

```go
// computeOccurrences derives entity occurrences from inbound relationships.
func (s *entityService) computeOccurrences(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
    // Get all inbound relationships (where this entity is the target)
    relationships, err := s.relationshipRepo.GetByTargetEntity(ctx, entityID)
    // ... converts to occurrences with table, column, association
}
```

The `engine_ontology_entity_occurrences` table was **dropped in migration 030** because occurrences are now computed from `engine_entity_relationships`.

### MCP Tools ✅ ALREADY IMPLEMENTED

**File:** `pkg/mcp/tools/entity.go`

1. **`get_entity`** - Returns full entity with occurrences, relationships, aliases, key_columns
2. **`update_entity`** - Upsert with provenance tracking
3. **`delete_entity`** - Soft delete with relationship checking

### Relationships ✅ WORKING (Not Empty)

**File:** `pkg/services/deterministic_relationship_service.go`

Relationships ARE populated during extraction:
1. **Phase 1**: FK discovery from ColumnFeatures (data overlap analysis)
2. **Phase 2**: FK discovery from database constraints
3. **Phase 3**: PK match discovery (tests joins via SQL)

If `probe_relationship` returns empty, it's likely a **data issue** for that specific project, not a code bug. Check:
```sql
SELECT COUNT(*) FROM engine_entity_relationships WHERE project_id = '<project-id>';
```

---

## What's Actually Missing (Real Gap)

### Gap: No Promotion Filtering

**The core issue:** All tables with PKs become entities. There's no mechanism to filter by semantic value.

The plan proposes adding a **PromotionScore** function that evaluates each table before creating an entity. Tables scoring < 50 would NOT become entities.

### Gap: `is_promoted` Field

The entity model doesn't have an `is_promoted` flag to distinguish promoted entities from demoted ones (or to preserve manual promotions).

### Gap: Migration for Existing Ontologies

Existing ontologies would need a migration to score and potentially demote redundant entities.

---

## Revised Implementation Tasks

### Task 1: Add Promotion Scoring Function ✅

**File:** `pkg/services/entity_promotion.go` (new)

Create a function that scores whether a table should become an entity:

```go
// PromotionScore evaluates if a table warrants entity status.
// Returns score 0-100, where >= 50 means promote to entity.
type PromotionResult struct {
    Score   int
    Reasons []string
}

func PromotionScore(
    tableName string,
    allTables []*models.SchemaTable,
    relationships []*models.EntityRelationship,
) PromotionResult {
    score := 0
    var reasons []string

    // Criterion 1: Hub in relationship graph (30 points)
    // Count inbound relationships (how many other tables reference this one)
    inboundCount := countInboundRelationships(tableName, relationships)
    if inboundCount >= 5 {
        score += 30
        reasons = append(reasons, fmt.Sprintf("hub with %d inbound references", inboundCount))
    } else if inboundCount >= 3 {
        score += 20
        reasons = append(reasons, fmt.Sprintf("%d inbound references", inboundCount))
    }

    // Criterion 2: Multiple roles reference this table (25 points)
    // e.g., host_id and visitor_id both reference users
    roleRefs := findRoleBasedReferences(tableName, relationships)
    if len(roleRefs) >= 2 {
        score += 25
        reasons = append(reasons, fmt.Sprintf("%d distinct roles", len(roleRefs)))
    }

    // Criterion 3: Multiple tables share this concept (20 points)
    // Use existing groupSimilarTables logic
    relatedTables := findRelatedTables(tableName, allTables)
    if len(relatedTables) > 1 {
        score += 20
        reasons = append(reasons, fmt.Sprintf("aggregates %d tables", len(relatedTables)))
    }

    // Criterion 4: Has business aliases from LLM (15 points)
    // This would be post-enrichment scoring

    // Criterion 5: Outbound relationships (10 points)
    // Tables that connect to many other entities are worth naming
    outboundCount := countOutboundRelationships(tableName, relationships)
    if outboundCount >= 3 {
        score += 10
        reasons = append(reasons, fmt.Sprintf("%d outbound relationships", outboundCount))
    }

    return PromotionResult{Score: score, Reasons: reasons}
}
```

**Key insight:** Scoring happens AFTER relationship detection, not before. We need relationships to score hubs.

**Acceptance criteria:**
- Function returns score 0-100 for any table
- Score >= 50 means table should be promoted to entity
- Function returns list of reasons (for transparency)
- Unit tests cover each criterion

---

### Task 2: Add `is_promoted` Field to Entity Model ✅

**File:** `migrations/XXX_entity_promotion.up.sql` (new)

```sql
ALTER TABLE engine_ontology_entities
ADD COLUMN is_promoted BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE engine_ontology_entities
ADD COLUMN promotion_score INTEGER;

ALTER TABLE engine_ontology_entities
ADD COLUMN promotion_reasons TEXT[];

COMMENT ON COLUMN engine_ontology_entities.is_promoted IS
'True if entity meets promotion criteria or was manually promoted. False for demoted entities.';

COMMENT ON COLUMN engine_ontology_entities.promotion_score IS
'Computed promotion score (0-100) from PromotionScore function.';
```

**File:** `pkg/models/ontology_entity.go`

Add fields:
```go
IsPromoted       bool     `json:"is_promoted"`
PromotionScore   *int     `json:"promotion_score,omitempty"`
PromotionReasons []string `json:"promotion_reasons,omitempty"`
```

**Acceptance criteria:**
- Existing entities default to `is_promoted=true`
- Migration is idempotent
- Model changes are backwards compatible

---

### Task 3: Integrate Promotion Scoring into DAG

> **Note:** This task has been split into subtasks 3.1, 3.2, and 3.3.

```
Current DAG order:
1. KnowledgeSeeding
2. ColumnFeatureExtraction
3. EntityDiscovery ← creates entities for all tables with PKs
4. EntityEnrichment
5. FKDiscovery ← creates relationships
6. PKMatchDiscovery
7. RelationshipEnrichment
8. OntologyFinalization
9. ColumnEnrichment
10. GlossaryDiscovery
11. GlossaryEnrichment

Proposed change - insert after RelationshipEnrichment:
7. RelationshipEnrichment
8. EntityPromotion (NEW) ← scores and demotes low-value entities
9. OntologyFinalization
```

---

#### Task 3.1: Create EntityPromotion DAG Node Structure and Registration

Create the new DAG node file and register it in the DAG execution order. This subtask focuses on the node skeleton and DAG ordering, not the promotion logic itself.

**Files to create/modify:**

1. **Create `pkg/services/dag/entity_promotion_node.go`:**
   - Implement `EntityPromotionNode` struct with standard DAG node interface
   - Include `Name() string` returning "EntityPromotion"
   - Include `Execute(ctx context.Context, dag *models.OntologyDAG) error` with placeholder that calls the entity service (to be implemented in 3.2)
   - Follow the pattern established by other nodes like `pkg/services/dag/relationship_enrichment_node.go`

2. **Modify `pkg/services/dag/dag_executor.go` (or wherever nodes are ordered):**
   - Insert EntityPromotion node AFTER RelationshipEnrichment (currently node 7)
   - BEFORE OntologyFinalization (currently node 9, will become node 10)
   - Update any hardcoded node order constants

3. **Modify `pkg/services/dag/node_factory.go` (or node registration):**
   - Register the new EntityPromotionNode in the factory/registry

**Reference existing nodes for patterns:**
- `pkg/services/dag/relationship_enrichment_node.go` - similar structure
- `pkg/services/dag/entity_enrichment_node.go` - for entity iteration patterns

**Acceptance criteria:**
- [x] DAG executor includes EntityPromotion in the correct position (after node 7, before node 9)
- [x] Node executes without error (even if promotion logic is placeholder)
- [x] DAG status shows EntityPromotion node during extraction

---

#### Task 3.2: Implement EntityPromotion Service Method ✅

Create the service method that the DAG node calls to perform actual promotion scoring. This requires Task 1 (PromotionScore function) and Task 2 (is_promoted field) to be completed first.

**Files to create/modify:**

1. **Add method to `pkg/services/entity_service.go`:**
   ```go
   // ScoreAndPromoteEntities evaluates all entities for a project and updates is_promoted status.
   // Returns count of promoted and demoted entities.
   func (s *entityService) ScoreAndPromoteEntities(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error)
   ```

   Implementation should:
   - Fetch all entities for the project via `s.entityRepo.GetByProject(ctx, projectID)`
   - Fetch all relationships via relationship repository
   - For each entity, call `PromotionScore()` from `pkg/services/entity_promotion.go` (Task 1)
   - Update entity with `is_promoted`, `promotion_score`, and `promotion_reasons` fields (Task 2)
   - Skip entities where `Source == "manual"` (manual promotions/demotions persist)
   - Log each promotion decision with reasons at INFO level

2. **Add interface method to `pkg/services/interfaces.go`:**
   ```go
   // In EntityService interface
   ScoreAndPromoteEntities(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error)
   ```

3. **Update `pkg/services/dag/entity_promotion_node.go` (from 3.1):**
   - Replace placeholder with call to `entityService.ScoreAndPromoteEntities()`
   - Log summary: "EntityPromotion complete: X promoted, Y demoted"

**Dependencies:**
- Task 1 (PromotionScore function) must exist at `pkg/services/entity_promotion.go`
- Task 2 (is_promoted field) must exist in `pkg/models/ontology_entity.go` and database

**Acceptance criteria:**
- [x] Running ontology extraction executes EntityPromotion node after RelationshipEnrichment
- [x] Entities below threshold (score < 50) have `is_promoted=false`
- [x] Entities at or above threshold have `is_promoted=true`
- [x] Promotion scores and reasons are persisted to database
- [x] Manual promotions/demotions are preserved (not overwritten)
- [x] Demoted entities retain all metadata (description, aliases, key_columns, etc.)

---

#### Task 3.3: Add Integration Test for EntityPromotion DAG Node

Create integration tests that verify the EntityPromotion node works correctly within the DAG workflow.

**Files to create/modify:**

1. **Create `pkg/services/dag/entity_promotion_node_test.go`:**
   - Test that node executes in correct order (after RelationshipEnrichment)
   - Test with mock entity data that includes:
     - High-value entity (5+ inbound refs) → should be promoted
     - Low-value entity (0 refs, no roles) → should be demoted
     - Manually promoted entity (Source="manual", is_promoted=true) → should stay promoted
     - Manually demoted entity (Source="manual", is_promoted=false) → should stay demoted

2. **Add test helper in `pkg/testhelpers/` if needed:**
   - Helper to create test entities with relationships for promotion testing

**Test scenarios:**
```go
func TestEntityPromotionNode_Execute(t *testing.T) {
    // Setup: Create project with entities and relationships
    // - UserEntity with 6 inbound relationships (hub) → expect promoted
    // - SessionEntity with 0 relationships → expect demoted
    // - ManuallyPromotedEntity with Source="manual" → expect unchanged

    // Execute: Run EntityPromotion node

    // Assert: Check is_promoted, promotion_score, promotion_reasons
}

func TestEntityPromotionNode_PreservesManualDecisions(t *testing.T) {
    // Setup: Entity manually demoted (is_promoted=false, Source="manual")
    // Execute: Run promotion (entity would normally score 80+)
    // Assert: Entity remains demoted (manual overrides automatic)
}
```

**Acceptance criteria:**
- [x] Tests pass with real database (integration tests use testhelpers.GetEngineDB)
- [x] Test coverage for promotion, demotion, and manual override scenarios
- [x] Tests document expected behavior for edge cases

**Status: ✅ COMPLETE**

---

### Task 4: Filter Demoted Entities in `get_context` ✅ COMPLETE

**File:** `pkg/services/ontology_context.go`

Modify `GetEntitiesContext` to filter demoted entities:

```go
func (s *ontologyContextService) GetEntitiesContext(...) {
    // Get entities - only promoted ones
    entities, err := s.entityRepo.GetPromotedByProject(ctx, projectID)
    // ... rest unchanged
}
```

Add repository method:
```go
// GetPromotedByProject returns only promoted entities (is_promoted=true)
func (r *ontologyEntityRepository) GetPromotedByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error)
```

**Acceptance criteria:**
- `get_context(depth='entities')` returns only promoted entities
- `get_entity(name='DemotedEntity')` still works (for inspection)
- UI entity list can show all or only promoted (configurable)

---

### Task 5: Manual Promotion/Demotion API ✅

**File:** `pkg/mcp/tools/entity.go`

Enhance `update_entity` to support promotion:

```go
// update_entity now accepts is_promoted parameter
mcp.WithBoolean(
    "is_promoted",
    mcp.Description("Optional - Set to true to promote, false to demote. Manual changes persist across re-extraction."),
)
```

When `is_promoted` is explicitly set:
1. Update `is_promoted` field
2. Set `Source` to `manual` (highest precedence)
3. Manual promotions/demotions persist across re-extraction

**Acceptance criteria:**
- `update_entity(name='Session', is_promoted=false)` demotes Session entity
- Manual demotions prevent auto-promotion during re-extraction
- Manual promotions prevent auto-demotion during re-extraction

---

## Removed Tasks (Already Implemented)

| Original Task | Why Not Needed |
|---------------|----------------|
| Task 0: Fix Relationship Detection | Already works via `deterministic_relationship_service.go` |
| Task 4 (original): Update get_entity occurrences | Already computed from relationships |
| Task 4 (original): Update get_context occurrences | Already implemented |

---

## Expected Outcome for tikr_production

**Before (current):**
```
Entities: ~38 (nearly 1:1 with tables)
All tables with PKs become entities
```

**After (with promotion model):**
```
Promoted Entities: 4-5 (high semantic value)
- User (score: 85) - 12+ inbound refs, multiple roles (host/visitor/creator/payer/payee)
- Account (score: 60) - N:1 hub to User, appears in 5+ tables
- Engagement (score: 55) - central to billing, multiple relationships
- Media (score: 50) - has status enum, multiple references

Demoted Entities: ~33 (still exist, but filtered from default context)
- Session, PasswordReset, Notification, etc.
- Can be manually re-promoted if needed

Tables with rich metadata: 38 (unchanged)
- All tables still have descriptions, FK annotations, enum values
```

---

## Testing Strategy

1. **Unit tests** for PromotionScore function
   - Table with 5+ inbound references scores >= 30
   - Table with 2 role-based references scores >= 25
   - Combination scoring works correctly

2. **Integration test** on tikr_production
   - Run extraction with promotion model
   - Verify 4-5 entities promoted
   - Verify ~33 entities demoted (not deleted)
   - Verify `get_context` returns only promoted entities

3. **Regression test**
   - `get_entity(name='DemotedEntity')` still works
   - Manual promotion/demotion persists across re-extraction
   - Existing ontology queries still work

---

## Success Metrics

| Metric | Before | After |
|--------|--------|-------|
| Promoted entity count (tikr_production) | ~38 | 4-5 |
| Total entities (including demoted) | ~38 | ~38 |
| Tables with rich metadata | ~38 | ~38 (unchanged) |
| Entity:Table redundancy | ~95% | 0% (for promoted) |
| Promotion decisions explainable | No | Yes (logged reasons) |
| Occurrences populated | Yes (from relationships) | Yes (unchanged) |
| Relationships detected | Yes | Yes (unchanged) |

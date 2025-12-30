# PLAN: Entity-Based Relationship Discovery

## Goal
Replace the current candidate-based relationship detection (891 candidates, token limit exceeded) with entity-based discovery that builds a connected graph of tables using domain entities (user, account, order, etc.) with role semantics (visitor, host, owner).

## Testing Protocol
Each phase is tested by clicking **[Find Relationships]** in the UI. Phases are complete when the expected output is visible in either:
- Server logs (terminal running `make dev-server`)
- UI display
- Database tables (query with `psql`)

---

## Phase 1: Column Statistics Collection

**Goal:** Collect distinct counts for all columns across all tables BEFORE any LLM calls.

**Implementation:**
- Modify the relationship workflow to call `AnalyzeColumnStats` for every table
- Log the results: `INFO: Column stats collected for 21 tables, 156 columns`
- Store stats in workflow state or pass through pipeline

**Test:** Click [Find Relationships], check server logs for:
```
Column statistics: public.users (8 columns)
  - id: 100 distinct / 100 rows (100.0%)
  - email: 100 distinct / 100 rows (100.0%)
  - status: 4 distinct / 100 rows (4.0%)
  - created_at: 98 distinct / 100 rows (98.0%)
Column statistics: public.orders (6 columns)
  - id: 500 distinct / 500 rows (100.0%)
  - user_id: 95 distinct / 500 rows (19.0%)
  ...
Summary: Collected stats for 156 columns across 21 tables
```

**Success Criteria:** All tables/columns logged with distinct counts and ratios.

**Files to modify:**
- `pkg/services/relationship_workflow.go` - Add stats collection step
- May need new task type or inline in workflow

---

## Phase 2: Column Filtering (Entity Candidates)

**Goal:** Filter columns to identify entity candidates using heuristics. No LLM yet.

**Heuristics:**
- Include if: `distinct_count >= 20 AND distinct_count / row_count > 0.05`
- Include if: `IsPrimaryKey = true` OR `IsUnique = true`
- Include if: column name matches `*_id`, `*_uuid`, `*_key`, or is `id`
- Exclude if: type is BOOLEAN, TIMESTAMP, DATE
- Exclude if: name matches `*_at`, `*_date`, `is_*`, `has_*`, `*_status`, `*_type`, `*_flag`

**Test:** Click [Find Relationships], check server logs for:
```
Column filtering results:
  CANDIDATE: public.orders.user_id (bigint, 95 distinct, PK=false, Unique=false)
  CANDIDATE: public.orders.product_id (uuid, 200 distinct, PK=false, Unique=false)
  CANDIDATE: public.visits.visitor_id (uuid, 80 distinct, PK=false, Unique=false)
  CANDIDATE: public.visits.host_id (uuid, 45 distinct, PK=false, Unique=false)
  EXCLUDED: public.orders.status (4 distinct - below threshold)
  EXCLUDED: public.orders.created_at (timestamp type)
  EXCLUDED: public.users.is_active (boolean type)

Summary: 47 candidate columns, 109 excluded columns
```

**Success Criteria:** Candidates are reasonable entity references, excluded columns are attributes/enums.

**Files to modify:**
- `pkg/services/column_filter.go` (new) - Filtering logic
- `pkg/services/relationship_workflow.go` - Call filter after stats

---

## Phase 3: Connected Components (Graph Analysis)

**Goal:** Identify table islands using FK relationships. Pure Go, no SQL, no LLM.

**Implementation:**
- Build adjacency graph from `DiscoverForeignKeys()` results
- Run DFS/BFS to find connected components
- Log component membership

**Test:** Click [Find Relationships], check server logs for:
```
Graph connectivity analysis:
  Foreign keys: 14 relationships

  Component 1 (9 tables): users, orders, order_items, products, categories, ...
  Component 2 (3 tables): audit_logs, audit_events, audit_users
  Component 3 (1 table): standalone_config
  Island tables (8): visits, sessions, notifications, ...

Summary: 3 connected components, 8 island tables need bridging
```

**Success Criteria:** Components correctly identified based on FK edges.

**Files to modify:**
- `pkg/services/graph.go` (new) - Connected components algorithm
- `pkg/services/relationship_workflow.go` - Call after FK discovery

---

## Phase 4: Database Tables for Entities

**Goal:** Create tables to store discovered entities and their occurrences.

**Tables:**
```sql
CREATE TABLE engine_schema_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id),
    name TEXT NOT NULL,                    -- "user", "account", "order"
    description TEXT,                      -- LLM explanation
    primary_schema TEXT NOT NULL,
    primary_table TEXT NOT NULL,
    primary_column TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(ontology_id, name)
);

CREATE TABLE engine_schema_entity_occurrences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_schema_entities(id) ON DELETE CASCADE,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    column_name TEXT NOT NULL,
    role TEXT,                             -- "visitor", "host", "owner", NULL for generic
    confidence FLOAT DEFAULT 1.0,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(entity_id, schema_name, table_name, column_name)
);

CREATE INDEX idx_entity_occurrences_table
    ON engine_schema_entity_occurrences(schema_name, table_name);
```

**Test:** After migration, verify with:
```sql
\d engine_schema_entities
\d engine_schema_entity_occurrences
```

**Success Criteria:** Tables exist with correct schema.

**Files to modify:**
- `migrations/XXXXXX_create_schema_entities.up.sql` (new)
- `migrations/XXXXXX_create_schema_entities.down.sql` (new)

---

## Phase 5: Entity Repository

**Goal:** Create repository layer for entity CRUD operations.

**Interface:**
```go
type SchemaEntityRepository interface {
    Create(ctx context.Context, entity *models.SchemaEntity) error
    GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.SchemaEntity, error)
    GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.SchemaEntity, error)
    DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error

    CreateOccurrence(ctx context.Context, occ *models.SchemaEntityOccurrence) error
    GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.SchemaEntityOccurrence, error)
    GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.SchemaEntityOccurrence, error)
}
```

**Test:** Unit tests pass for repository methods.

**Success Criteria:** `make test` passes with new repository tests.

**Files to modify:**
- `pkg/models/schema_entity.go` (new)
- `pkg/repositories/schema_entity_repository.go` (new)
- `pkg/repositories/postgres/schema_entity_repository.go` (new)
- Tests for repository

---

## Phase 6: Entity Discovery Task (LLM)

**Goal:** Create LLM task that identifies entities from candidate columns.

**Input:** Filtered candidate columns with stats, existing FKs, excluded columns (for context)

**Prompt design:**
- Send schema summary: table names + candidate column names with stats
- Send existing FKs
- Send excluded columns list (context only, marked as excluded)
- Ask: "Identify domain entities and their occurrences with roles"

**Output:** JSON with entities, their primary location, and occurrences with roles

**Test:** Click [Find Relationships], check server logs for:
```
Entity discovery LLM call:
  Input tokens: ~8,000
  Output tokens: ~2,000

Discovered entities:
  - user (primary: public.users.id)
    - public.orders.user_id (role: null)
    - public.visits.visitor_id (role: visitor)
    - public.visits.host_id (role: host)
    - public.properties.owner_id (role: owner)
  - account (primary: public.accounts.id)
    - public.users.account_id (role: null)
    - public.invoices.account_id (role: null)
  - product (primary: public.products.id)
    - public.order_items.product_id (role: null)
    - public.inventory.product_id (role: null)

Summary: 8 entities discovered, 32 total occurrences
```

**Success Criteria:** Entities make semantic sense, roles are identified correctly, no attribute columns (email, password) appear.

**Files to modify:**
- `pkg/services/entity_discovery_task.go` (new)
- `pkg/services/relationship_workflow.go` - Wire up task

---

## Phase 7: Store Entities in Database

**Goal:** Persist discovered entities and occurrences to database.

**Test:** Click [Find Relationships], then query:
```sql
SELECT e.name, e.primary_table, e.primary_column, e.description
FROM engine_schema_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE o.is_active = true;

SELECT e.name, o.table_name, o.column_name, o.role
FROM engine_schema_entities e
JOIN engine_schema_entity_occurrences o ON o.entity_id = e.id
ORDER BY e.name, o.table_name;
```

**Success Criteria:** Entities and occurrences are persisted correctly.

**Files to modify:**
- `pkg/services/relationship_workflow.go` - Save entities after LLM discovery

---

## Phase 8: API Endpoints

**Goal:** Expose entities via REST API.

**Endpoints:**
- `GET /api/projects/{id}/entities` - List entities for project
- `GET /api/projects/{id}/entities/{entityId}/occurrences` - Get occurrences with roles

**Test:** After [Find Relationships] completes:
```bash
curl http://localhost:3443/api/projects/{project_id}/entities | jq
```

**Success Criteria:** API returns entities and occurrences correctly.

**Files to modify:**
- `pkg/handlers/entity_handler.go` (new)
- `main.go` - Register routes

---

## Phase 9: UI Display

**Goal:** Replace "891 candidates" view with entity list showing roles.

**UI changes:**
- Show entity cards with name, description, primary table
- Expand to show occurrences with roles
- Show connectivity status (connected vs islands)

**Test:** Click [Find Relationships], verify UI shows:
- Entity list instead of candidate list
- Roles displayed for each occurrence
- No more "891 candidates"

**Success Criteria:** UI displays entities with roles, workflow completes without token errors.

---

## Summary

| Phase | Description | Test Method | Output |
|-------|-------------|-------------|--------|
| 1 | Column stats | Server logs | Stats per column |
| 2 | Column filtering | Server logs | Candidates vs excluded |
| 3 | Connected components | Server logs | Component membership |
| 4 | Database tables | psql \d | Tables exist |
| 5 | Repository | make test | Tests pass |
| 6 | LLM entity discovery | Server logs | Entities discovered |
| 7 | Persist entities | psql query | Data in tables |
| 8 | API endpoints | curl | JSON response |
| 9 | UI display | Browser | Entities shown |

## Cleanup (After All Phases)

- Remove or deprecate `engine_relationship_candidates` table
- Remove `TestJoinTask` (or repurpose for validation)
- Remove `AnalyzeRelationshipsTask` (replaced by `EntityDiscoveryTask`)
- Consolidate migrations before launch

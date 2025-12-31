# Plan: Initial Entities Screen

## Status

| Step | Description | Status |
|------|-------------|--------|
| 1 | Database Migration | ✅ Complete |
| 2 | Update Go Models | ✅ Complete |
| 3 | Update Repository Layer | ✅ Complete |
| 4 | Create Entity Service | ✅ Complete |
| 5 | Create Entity Handler | ✅ Complete |
| 6 | Wire Up Handler in main.go | ✅ Complete |
| 7 | Add TypeScript Types | 🔲 Next |
| 8 | Add API Methods to engineApi.ts | 🔲 Pending |
| 9 | Create Entities Page Component | 🔲 Pending |
| 10 | Add Route to App.tsx | 🔲 Pending |
| 11 | Add Entities Tile to Dashboard | 🔲 Pending |
| 12 | Update Existing Code References | 🔲 Pending |

### Commits (Steps 1-6)
```
f7ed004 Rename entity tables to ontology namespace with soft delete support
88fb70f Add EntityService for entity management operations
d1299a1 Add entity handler with HTTP routes and integration test
```

### Files Created/Modified (Backend Complete)
- `migrations/014_ontology_entities.up.sql` - Table renames, soft delete, aliases
- `migrations/014_ontology_entities.down.sql` - Rollback
- `pkg/models/schema_entity.go` - Added IsDeleted, DeletionReason, OntologyEntityAlias
- `pkg/repositories/schema_entity_repository.go` - New table names, soft delete filtering, alias CRUD
- `pkg/repositories/schema_entity_repository_test.go` - Tests for new methods
- `pkg/services/entity_service.go` - Business logic layer
- `pkg/handlers/entity_handler.go` - HTTP routes
- `pkg/handlers/entity_integration_test.go` - Integration test for entity endpoints
- `main.go` - Wiring

### API Endpoints Available
```
GET    /api/projects/{pid}/entities              - List entities
GET    /api/projects/{pid}/entities/{eid}        - Get entity details
PUT    /api/projects/{pid}/entities/{eid}        - Update description
DELETE /api/projects/{pid}/entities/{eid}        - Soft delete
POST   /api/projects/{pid}/entities/{eid}/restore - Restore
POST   /api/projects/{pid}/entities/{eid}/aliases - Add alias
DELETE /api/projects/{pid}/entities/{eid}/aliases/{aid} - Remove alias
```

---

## Overview

Create the foundation for an "Entities" screen in the ekaya-engine project. This is a LIMITED scope implementation that sets up the database schema, backend layers, and UI for displaying entities - without implementing the full entity discovery workflow.

### Terminology
- `engine_schema_*` = database structure (tables, columns)
- `engine_ontology_*` = intelligence layer (entities, relationships, associations)

### Out of Scope
- Entity discovery workflow (deterministic detection)
- LLM integration for entity analysis
- Workflow modal/processing UI
- Associations on relationships

---

## Step 1: Database Migration - Rename and Extend Entity Tables ✅

**Files created:**
- `migrations/014_ontology_entities.up.sql`
- `migrations/014_ontology_entities.down.sql`

**Changes:**
1. Rename `engine_schema_entities` -> `engine_ontology_entities`
2. Rename `engine_schema_entity_occurrences` -> `engine_ontology_entity_occurrences`
3. Add soft delete fields to entities:
   - `is_deleted BOOLEAN NOT NULL DEFAULT FALSE`
   - `deletion_reason TEXT`
4. Create new `engine_ontology_entity_aliases` table:
   ```sql
   CREATE TABLE engine_ontology_entity_aliases (
       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
       alias TEXT NOT NULL,
       source VARCHAR(50),  -- 'discovery', 'user', 'query'
       created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
       UNIQUE(entity_id, alias)
   );
   ```

**Verification:** Run `make check` to ensure migrations apply cleanly.

---

## Step 2: Update Go Models ✅

**File modified:** `pkg/models/schema_entity.go`

**Changes:**
1. Add type alias for backward compatibility: `type SchemaEntity = OntologyEntity`
2. Add soft delete fields to entity:
   ```go
   IsDeleted       bool    `json:"is_deleted"`
   DeletionReason  *string `json:"deletion_reason,omitempty"`
   ```
3. Add new `OntologyEntityAlias` struct:
   ```go
   type OntologyEntityAlias struct {
       ID        uuid.UUID `json:"id"`
       EntityID  uuid.UUID `json:"entity_id"`
       Alias     string    `json:"alias"`
       Source    *string   `json:"source,omitempty"`
       CreatedAt time.Time `json:"created_at"`
   }
   ```

**Verification:** `go build ./...` succeeds.

---

## Step 3: Update Repository Layer ✅

**File modified:** `pkg/repositories/schema_entity_repository.go`

**Changes:**
1. Update all SQL queries to reference `engine_ontology_entities` and `engine_ontology_entity_occurrences`
2. Add soft delete filtering to read queries (`WHERE is_deleted = FALSE` or `WHERE NOT is_deleted`)
3. Add new methods to interface and implementation:
   - `SoftDelete(ctx, entityID uuid.UUID, reason string) error`
   - `Restore(ctx, entityID uuid.UUID) error`
   - `CreateAlias(ctx, alias *models.OntologyEntityAlias) error`
   - `GetAliasesByEntity(ctx, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error)`
   - `DeleteAlias(ctx, aliasID uuid.UUID) error`
   - `GetByProjectWithAliases(ctx, projectID uuid.UUID) ([]*EntityWithDetails, error)` - returns entities with aliases and occurrence counts

**Verification:** Existing tests pass, add new repository tests.

---

## Step 4: Create Entity Service ✅

**File created:** `pkg/services/entity_service.go`

**Service interface:**
```go
type EntityService interface {
    // List entities for a project (from active ontology)
    ListByProject(ctx context.Context, projectID uuid.UUID) ([]*EntityWithDetails, error)

    // Get single entity with occurrences and aliases
    GetByID(ctx context.Context, entityID uuid.UUID) (*EntityWithDetails, error)

    // Soft delete an entity
    Delete(ctx context.Context, entityID uuid.UUID, reason string) error

    // Restore a soft-deleted entity
    Restore(ctx context.Context, entityID uuid.UUID) error

    // Update entity description
    Update(ctx context.Context, entityID uuid.UUID, description string) error

    // Alias management
    AddAlias(ctx context.Context, entityID uuid.UUID, alias, source string) (*models.OntologyEntityAlias, error)
    RemoveAlias(ctx context.Context, aliasID uuid.UUID) error
}

type EntityWithDetails struct {
    Entity          *models.OntologyEntity
    Occurrences     []*models.OntologyEntityOccurrence
    Aliases         []*models.OntologyEntityAlias
    OccurrenceCount int
}
```

**Pattern to follow:** `pkg/services/relationship_workflow.go` for service structure with dependency injection.

**Verification:** Unit tests for all service methods.

---

## Step 5: Create Entity Handler ✅

**File created:** `pkg/handlers/entity_handler.go`

**Routes to implement:**
```
GET    /api/projects/{projectId}/entities                           - List entities for project
GET    /api/projects/{projectId}/entities/{entityId}                - Get entity details
PUT    /api/projects/{projectId}/entities/{entityId}                - Update entity description
DELETE /api/projects/{projectId}/entities/{entityId}                - Soft delete entity
POST   /api/projects/{projectId}/entities/{entityId}/restore        - Restore deleted entity
POST   /api/projects/{projectId}/entities/{entityId}/aliases        - Add alias
DELETE /api/projects/{projectId}/entities/{entityId}/aliases/{aliasId} - Remove alias
```

**Response types:**
```go
type EntityListResponse struct {
    Entities []EntityResponse `json:"entities"`
    Total    int              `json:"total"`
}

type EntityResponse struct {
    ID              string               `json:"id"`
    Name            string               `json:"name"`
    Description     string               `json:"description"`
    PrimarySchema   string               `json:"primary_schema"`
    PrimaryTable    string               `json:"primary_table"`
    PrimaryColumn   string               `json:"primary_column"`
    Occurrences     []OccurrenceResponse `json:"occurrences"`
    Aliases         []AliasResponse      `json:"aliases"`
    OccurrenceCount int                  `json:"occurrence_count"`
    IsDeleted       bool                 `json:"is_deleted"`
}

type OccurrenceResponse struct {
    ID         string   `json:"id"`
    SchemaName string   `json:"schema_name"`
    TableName  string   `json:"table_name"`
    ColumnName string   `json:"column_name"`
    Role       *string  `json:"role,omitempty"`
    Confidence float64  `json:"confidence"`
}

type AliasResponse struct {
    ID     string  `json:"id"`
    Alias  string  `json:"alias"`
    Source *string `json:"source,omitempty"`
}
```

**Pattern to follow:** `pkg/handlers/relationship_handler.go` for handler structure.

**Verification:** Handler tests with mock service.

---

## Step 6: Wire Up Handler in main.go ✅

**File modified:** `main.go`

**Changes:**
1. Create `entityService`:
   ```go
   entityService := services.NewEntityService(schemaEntityRepo, ontologyRepo, logger)
   ```
2. Create and register handler:
   ```go
   entityHandler := handlers.NewEntityHandler(entityService, logger)
   entityHandler.RegisterRoutes(apiRouter, authMiddleware, tenantMiddleware)
   ```

**Verification:** Server starts without errors, routes respond to requests.

---

## Step 7: Add TypeScript Types

**File to create:** `ui/src/types/entity.ts`

**Types to add:**
```typescript
export interface OntologyEntity {
    id: string;
    name: string;
    description: string;
    primary_schema: string;
    primary_table: string;
    primary_column: string;
    occurrences: EntityOccurrence[];
    aliases: EntityAlias[];
    occurrence_count: number;
    is_deleted: boolean;
}

export interface EntityOccurrence {
    id: string;
    schema_name: string;
    table_name: string;
    column_name: string;
    role: string | null;
    confidence: number;
}

export interface EntityAlias {
    id: string;
    alias: string;
    source: string | null;
}

export interface EntitiesListResponse {
    entities: OntologyEntity[];
    total: number;
}
```

**Verification:** TypeScript compilation succeeds.

---

## Step 8: Add API Methods to engineApi.ts

**File to modify:** `ui/src/services/engineApi.ts`

**Methods to add:**
```typescript
// Entity Management
async listEntities(projectId: string): Promise<ApiResponse<EntitiesListResponse>> {
    return this.makeRequest<EntitiesListResponse>(`/${projectId}/entities`);
}

async getEntity(projectId: string, entityId: string): Promise<ApiResponse<OntologyEntity>> {
    return this.makeRequest<OntologyEntity>(`/${projectId}/entities/${entityId}`);
}

async updateEntity(projectId: string, entityId: string, description: string): Promise<ApiResponse<OntologyEntity>> {
    return this.makeRequest<OntologyEntity>(`/${projectId}/entities/${entityId}`, {
        method: 'PUT',
        body: JSON.stringify({ description }),
    });
}

async deleteEntity(projectId: string, entityId: string, reason?: string): Promise<ApiResponse<void>> {
    return this.makeRequest<void>(`/${projectId}/entities/${entityId}`, {
        method: 'DELETE',
        body: reason ? JSON.stringify({ reason }) : undefined,
    });
}

async restoreEntity(projectId: string, entityId: string): Promise<ApiResponse<OntologyEntity>> {
    return this.makeRequest<OntologyEntity>(`/${projectId}/entities/${entityId}/restore`, {
        method: 'POST',
    });
}

async addEntityAlias(projectId: string, entityId: string, alias: string): Promise<ApiResponse<EntityAlias>> {
    return this.makeRequest<EntityAlias>(`/${projectId}/entities/${entityId}/aliases`, {
        method: 'POST',
        body: JSON.stringify({ alias }),
    });
}

async removeEntityAlias(projectId: string, entityId: string, aliasId: string): Promise<ApiResponse<void>> {
    return this.makeRequest<void>(`/${projectId}/entities/${entityId}/aliases/${aliasId}`, {
        method: 'DELETE',
    });
}
```

**Verification:** API methods callable from browser console.

---

## Step 9: Create Entities Page Component

**File to create:** `ui/src/pages/EntitiesPage.tsx`

**Component structure:**
1. Back navigation to dashboard (like RelationshipsPage)
2. Page header: "Entities" with description
3. Summary card showing:
   - Total entity count
   - Total occurrence count
4. Entity list (or empty state)
5. Each entity card shows:
   - Name and description
   - Primary location (schema.table.column)
   - Aliases as tags
   - Occurrence count (expandable to show details)

**Pattern to follow:** `ui/src/pages/RelationshipsPage.tsx` for overall structure.

**Key states:**
- Loading state with spinner
- Error state with retry button
- Empty state: "No entities discovered yet. Run entity discovery to identify domain concepts in your database."

**Verification:** Page renders correctly in all states.

---

## Step 10: Add Route to App.tsx

**File to modify:** `ui/src/App.tsx`

**Changes:**
1. Import EntitiesPage:
   ```tsx
   import EntitiesPage from './pages/EntitiesPage';
   ```
2. Add route inside project routes (near relationships route):
   ```tsx
   <Route path="entities" element={<EntitiesPage />} />
   ```

**Verification:** `/projects/{pid}/entities` route works.

---

## Step 11: Add Entities Tile to Dashboard

**File to modify:** `ui/src/pages/ProjectDashboard.tsx`

**Changes:**
1. Import appropriate icon (e.g., `Boxes` or `Component` from lucide-react)
2. Add Entities tile to intelligence section, positioned BEFORE Relationships:
   ```tsx
   {
       title: 'Entities',
       icon: Boxes,
       path: `/projects/${projectId}/entities`,
       disabled: !isConnected || !hasSelectedTables,
       color: 'green',
       description: 'Domain concepts discovered in your schema'
   }
   ```

**Verification:** Dashboard shows Entities tile, navigation works.

---

## Step 12: Update Existing Code References

**Files to check/modify:**
- `pkg/services/entity_discovery_task.go` - Uses SchemaEntityRepository
- `pkg/services/relationship_workflow.go` - Uses entity types
- Any other files referencing `engine_schema_entities` tables

**Changes:**
- Update table name references in SQL
- Update type references if renamed
- Ensure backward compatibility

**Verification:** Full `make check` passes.

---

## Testing Checklist

- [ ] Migrations apply cleanly (`make check`)
- [ ] Migrations rollback cleanly
- [ ] Repository CRUD operations work
- [ ] Repository tests pass
- [ ] Service methods return correct data
- [ ] Service tests pass
- [ ] Handler routes respond correctly (200, 404, etc.)
- [ ] Handler tests pass
- [ ] TypeScript types compile
- [ ] UI shows empty state when no entities
- [ ] UI shows entities when data exists (manual insert to test)
- [ ] Dashboard tile appears and navigates correctly
- [ ] Full `make check` passes

---

## Future Work (Separate Plans)

- **PLAN-entity-discovery-workflow.md** - Deterministic entity detection
- **PLAN-relationship-associations.md** - Associations on relationships
- **PLAN-ontology-chat.md** - Chat interface for ontology refinement

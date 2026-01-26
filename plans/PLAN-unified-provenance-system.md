# PLAN: Unified Provenance System for Ontology Objects

**Date:** 2026-01-25
**Status:** In Progress
**Priority:** High

## Tasks

- [x] Task 1: Update Migration Files
- [x] Task 2: Update Models
- [x] Task 3: Implement Provenance Context
- [x] Task 4: Update Repositories
  - [x] 4.1: Update Entity Repository with Provenance Support
  - [x] 4.2: Update Relationship Repository with Provenance Support
  - [x] 4.3: Update Glossary Repository with Provenance Support
  - [x] 4.4: Update Project Knowledge Repository with Provenance Support
- [x] Task 5: Implement Audit Service
- [ ] Task 6: Extract User ID from JWT
  - [x] 6.1: Update JWT Parsing to Extract User UUID
  - [x] 6.2: Add Provenance Middleware for HTTP Requests
  - [ ] 6.3: Add MCP Provenance Wrapper
  - [ ] 6.4: Add Inference Provenance Wrapper for DAG Steps

## Overview

Implement a unified provenance tracking system across all ontology objects (entities, relationships, glossary terms, project knowledge) that:

1. Tracks **source** (how: `inference`, `mcp`, `manual`) separately from **actor** (who: user UUID)
2. Supports re-extraction policy: DELETE `inference` items, KEEP `mcp`/`manual` items
3. Provides chronological audit trail across all object types
4. Uses a single implementation pattern, not duplicated 4 times

## Affected Objects

| Object | Table | Current Provenance Fields |
|--------|-------|---------------------------|
| Entities | `engine_ontology_entities` | `created_by`, `updated_by` (method, not user) |
| Relationships | `engine_entity_relationships` | `created_by`, `updated_by` (method, not user) |
| Glossary Terms | `engine_business_glossary` | `source` (method), no user tracking |
| Project Knowledge | `engine_project_knowledge` | None currently |

## Target Schema

### Provenance Fields (on each table)

```sql
-- Source tracking (how it was created/modified)
source           TEXT NOT NULL DEFAULT 'inference',  -- 'inference', 'mcp', 'manual'
last_edit_source TEXT,                               -- 'inference', 'mcp', 'manual' (null if never edited)

-- Actor tracking (who created/modified)
created_by       UUID NOT NULL REFERENCES engine_users(id),  -- who triggered the operation
updated_by       UUID REFERENCES engine_users(id),           -- null only if never edited

-- Timestamps
created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

-- Constraints
CONSTRAINT valid_source CHECK (source IN ('inference', 'mcp', 'manual')),
CONSTRAINT valid_last_edit_source CHECK (last_edit_source IS NULL OR last_edit_source IN ('inference', 'mcp', 'manual'))
```

### Audit Log Table (unified change history)

```sql
CREATE TABLE engine_audit_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- What changed
    entity_type     TEXT NOT NULL,  -- 'entity', 'relationship', 'glossary_term', 'project_knowledge'
    entity_id       UUID NOT NULL,  -- ID of the affected object
    action          TEXT NOT NULL,  -- 'create', 'update', 'delete'

    -- Who/how
    source          TEXT NOT NULL,  -- 'inference', 'mcp', 'manual'
    user_id         UUID NOT NULL REFERENCES engine_users(id),  -- who triggered the action

    -- What changed (for updates)
    changed_fields  JSONB,          -- {"description": {"old": "...", "new": "..."}}

    -- When
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT valid_entity_type CHECK (entity_type IN ('entity', 'relationship', 'glossary_term', 'project_knowledge')),
    CONSTRAINT valid_action CHECK (action IN ('create', 'update', 'delete')),
    CONSTRAINT valid_source CHECK (source IN ('inference', 'mcp', 'manual'))
);

CREATE INDEX idx_audit_log_project ON engine_audit_log(project_id);
CREATE INDEX idx_audit_log_entity ON engine_audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_user ON engine_audit_log(user_id);
CREATE INDEX idx_audit_log_time ON engine_audit_log(project_id, created_at DESC);
```

## Architecture

### Provenance Context

Create a unified provenance context that flows through all operations:

```go
// pkg/models/provenance.go

type ProvenanceSource string

const (
    SourceInference ProvenanceSource = "inference"
    SourceMCP       ProvenanceSource = "mcp"
    SourceManual    ProvenanceSource = "manual"
)

// ProvenanceContext carries source and actor information through operations
type ProvenanceContext struct {
    Source ProvenanceSource
    UserID uuid.UUID  // Always required - extracted from JWT
}

// Context key for passing provenance through context.Context
type provenanceKey struct{}

func WithProvenance(ctx context.Context, p ProvenanceContext) context.Context {
    return context.WithValue(ctx, provenanceKey{}, p)
}

func GetProvenance(ctx context.Context) (ProvenanceContext, bool) {
    p, ok := ctx.Value(provenanceKey{}).(ProvenanceContext)
    return p, ok
}
```

### Entry Points

| Entry Point | Source | User ID |
|-------------|--------|---------|
| DAG extraction steps | `inference` | From JWT of user who triggered extraction |
| MCP tool calls | `mcp` | From JWT in MCP request context |
| UI API endpoints | `manual` | From JWT in HTTP request |

All operations require a valid JWT. There is no anonymous/system-initiated path.

### Middleware/Interceptors

```go
// HTTP middleware for UI requests
func ProvenanceMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID := getUserIDFromJWT(r)  // Extract from JWT claims
        ctx := WithProvenance(r.Context(), ProvenanceContext{
            Source: SourceManual,
            UserID: userID,
        })
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// MCP tool wrapper
func WithMCPProvenance(ctx context.Context, userID uuid.UUID) context.Context {
    return WithProvenance(ctx, ProvenanceContext{
        Source: SourceMCP,
        UserID: userID,
    })
}

// DAG step wrapper (userID passed from whoever triggered extraction)
func WithInferenceProvenance(ctx context.Context, userID uuid.UUID) context.Context {
    return WithProvenance(ctx, ProvenanceContext{
        Source: SourceInference,
        UserID: userID,
    })
}
```

### Audit Service

```go
// pkg/services/audit_service.go

type AuditService interface {
    LogCreate(ctx context.Context, entityType string, entityID uuid.UUID) error
    LogUpdate(ctx context.Context, entityType string, entityID uuid.UUID, changes map[string]Change) error
    LogDelete(ctx context.Context, entityType string, entityID uuid.UUID) error
}

type Change struct {
    Old any `json:"old"`
    New any `json:"new"`
}

// Implementation extracts provenance from context
func (s *auditService) LogCreate(ctx context.Context, entityType string, entityID uuid.UUID) error {
    prov, _ := GetProvenance(ctx)
    projectID := getProjectIDFromContext(ctx)

    return s.repo.Create(ctx, &AuditLogEntry{
        ProjectID:   projectID,
        EntityType:  entityType,
        EntityID:    entityID,
        Action:      "create",
        Source:      string(prov.Source),
        UserID:      prov.UserID,
    })
}
```

### Repository Pattern

Each repository's Create/Update methods should:

1. Extract provenance from context
2. Set `source`/`last_edit_source` and `created_by`/`updated_by` on the model
3. Call audit service to log the change

```go
func (r *entityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
    prov, ok := models.GetProvenance(ctx)
    if !ok {
        return fmt.Errorf("provenance context required")
    }

    // Set provenance fields
    entity.Source = string(prov.Source)
    entity.CreatedBy = prov.UserID  // Always non-null

    // Insert...

    // Log to audit
    r.auditService.LogCreate(ctx, "entity", entity.ID)

    return nil
}

func (r *entityRepo) Update(ctx context.Context, entity *models.OntologyEntity, changes map[string]models.Change) error {
    prov, ok := models.GetProvenance(ctx)
    if !ok {
        return fmt.Errorf("provenance context required")
    }

    // Set provenance fields
    entity.LastEditSource = ptr(string(prov.Source))
    entity.UpdatedBy = &prov.UserID  // Always non-null on update
    entity.UpdatedAt = time.Now()

    // Update...

    // Log to audit
    r.auditService.LogUpdate(ctx, "entity", entity.ID, changes)

    return nil
}
```

## Re-extraction Policy

When re-extracting ontology (delete and rebuild):

```go
func (s *ontologyService) ReExtract(ctx context.Context, projectID uuid.UUID) error {
    // Delete ONLY inference-created items
    // Entities, relationships, glossary, project knowledge

    err := s.entityRepo.DeleteBySource(ctx, projectID, SourceInference)
    err = s.relationshipRepo.DeleteBySource(ctx, projectID, SourceInference)
    err = s.glossaryRepo.DeleteBySource(ctx, projectID, SourceInference)
    err = s.knowledgeRepo.DeleteBySource(ctx, projectID, SourceInference)

    // MCP and Manual items are preserved

    // Run extraction...
}
```

## Migration Plan

### Step 1: Update Migration Files

Modify existing migrations (no backward compatibility needed):

**Entities:**
- Rename `created_by` → `source`
- Rename `updated_by` → `last_edit_source`
- Add `created_by UUID REFERENCES engine_users(id)`
- Add `updated_by UUID REFERENCES engine_users(id)`

**Relationships:**
- Same changes as entities

**Glossary:**
- Already has `source`, add `last_edit_source`
- Add `created_by`, `updated_by`

**Project Knowledge:**
- Add all four fields

**New table:**
- Create `engine_audit_log`

### Step 2: Update Models

Update Go structs to match new schema.

### Step 3: Implement Provenance Context

- Create `pkg/models/provenance.go`
- Add middleware for HTTP requests
- Add wrapper for MCP calls
- Add wrapper for DAG steps

### Step 4: Update Repositories

#### 4.1 Update Entity Repository with Provenance Support

Update `pkg/repositories/ontology_entity_repository.go` to use provenance context:

**Files to modify:**
- `pkg/repositories/ontology_entity_repository.go`

**Changes required:**

1. Modify the `Create` method to:
   - Extract provenance from context using `models.GetProvenance(ctx)`
   - Return error if provenance context is missing
   - Set `entity.Source = string(prov.Source)`
   - Set `entity.CreatedBy = prov.UserID`
   - Update SQL INSERT to include `source` and `created_by` columns

2. Modify the `Update` method to:
   - Extract provenance from context
   - Set `entity.LastEditSource = ptr(string(prov.Source))`
   - Set `entity.UpdatedBy = &prov.UserID`
   - Update SQL UPDATE to include `last_edit_source` and `updated_by` columns

3. Add `DeleteBySource` method:
   ```go
   DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error
   ```
   This deletes all entities where `source` matches the given value, supporting re-extraction policy.

**Note:** Audit service integration will be added in Task 5 after the audit service is implemented.

---

#### 4.2 Update Relationship Repository with Provenance Support

Update `pkg/repositories/entity_relationship_repository.go` to use provenance context:

**Files to modify:**
- `pkg/repositories/entity_relationship_repository.go`

**Changes required:**

1. Modify the `Create` method to:
   - Extract provenance from context using `models.GetProvenance(ctx)`
   - Return error if provenance context is missing
   - Set `relationship.Source = string(prov.Source)`
   - Set `relationship.CreatedBy = prov.UserID`
   - Update SQL INSERT to include `source` and `created_by` columns

2. Modify the `Update` method to:
   - Extract provenance from context
   - Set `relationship.LastEditSource = ptr(string(prov.Source))`
   - Set `relationship.UpdatedBy = &prov.UserID`
   - Update SQL UPDATE to include `last_edit_source` and `updated_by` columns

3. Add `DeleteBySource` method:
   ```go
   DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error
   ```

**Pattern:** Follow the same pattern established in subtask 4.1 for entities.

---

#### 4.3 Update Glossary Repository with Provenance Support

Update the glossary repository (`pkg/repositories/glossary_repository.go` or similar) to use provenance context:

**Files to modify:**
- `pkg/repositories/glossary_repository.go` (or `business_glossary_repository.go`)

**Changes required:**

1. The glossary table already has a `source` field. Modify `Create` to also set:
   - `created_by = prov.UserID` (new field from migration)
   - Extract provenance from context, return error if missing

2. Modify the `Update` method to:
   - Extract provenance from context
   - Set `last_edit_source` (new field from migration)
   - Set `updated_by = &prov.UserID`
   - Update SQL to include these columns

3. Add `DeleteBySource` method:
   ```go
   DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error
   ```

**Note:** The glossary table (`engine_business_glossary`) already has a `source` column but the migration added `last_edit_source`, `created_by`, and `updated_by`.

---

#### 4.4 Update Project Knowledge Repository with Provenance Support

Update `pkg/repositories/project_knowledge_repository.go` to use provenance context:

**Files to modify:**
- `pkg/repositories/project_knowledge_repository.go`

**Changes required:**

1. Modify the `Create` method to:
   - Extract provenance from context using `models.GetProvenance(ctx)`
   - Return error if provenance context is missing
   - Set `knowledge.Source = string(prov.Source)`
   - Set `knowledge.CreatedBy = prov.UserID`
   - Update SQL INSERT to include all four provenance columns

2. Modify the `Update` method to:
   - Extract provenance from context
   - Set `knowledge.LastEditSource = ptr(string(prov.Source))`
   - Set `knowledge.UpdatedBy = &prov.UserID`
   - Update SQL UPDATE accordingly

3. Add `DeleteBySource` method:
   ```go
   DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error
   ```

**Note:** Project knowledge previously had NO provenance fields. The migration added all four fields (`source`, `last_edit_source`, `created_by`, `updated_by`), so this is a more significant change than the other repositories.

### Step 5: Implement Audit Service

- Create audit service and repository
- Wire into all object repositories

### Step 6: Extract User ID from JWT

#### 6.1 Update JWT Parsing to Extract User UUID

The current JWT parsing extracts user email but not the user UUID. Update the JWT claims structure and parsing to include the user ID.

**Files to modify:**
- `pkg/auth/middleware.go` - Update JWT claims struct and parsing

**Changes required:**

1. Locate the JWT claims struct (likely named `Claims` or similar) and add a `UserID` field of type `uuid.UUID` or `string`

2. Update the JWT parsing/validation logic to extract the user ID from the token claims. The user ID should be in the JWT as `sub` (subject) claim or a custom claim like `user_id`

3. Ensure the user ID is stored in the request context alongside the existing user email. Look for existing context key patterns (e.g., `userEmailKey`) and add a similar `userIDKey`

4. Add a helper function like `GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool)` if one doesn't exist

**Note:** The JWT is issued by `auth.ekaya.ai`. Check existing JWT handling to understand the token structure and which claims are available.

---

#### 6.2 Add Provenance Middleware for HTTP Requests

Create HTTP middleware that extracts the user ID from JWT and sets up the provenance context for all API requests. This enables the `manual` source for UI-initiated operations.

**Files to modify:**
- `pkg/server/middleware.go` - Add new provenance middleware

**Implementation:**

1. Create a new middleware function `ProvenanceMiddleware`:
```go
func ProvenanceMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID, ok := GetUserIDFromContext(r.Context())
        if !ok {
            // Handle unauthenticated requests appropriately
            // May need to skip for public endpoints
            next.ServeHTTP(w, r)
            return
        }
        ctx := models.WithProvenance(r.Context(), models.ProvenanceContext{
            Source: models.SourceManual,
            UserID: userID,
        })
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

2. Wire the middleware into the router chain, positioned AFTER authentication middleware (so JWT is already parsed) but BEFORE route handlers

3. The middleware should use `models.SourceManual` since HTTP API requests are user-initiated manual actions

**Dependencies:** Requires 6.1 (JWT user ID extraction) and Task 3 (provenance context implementation from `pkg/models/provenance.go`)

---

#### 6.3 Add MCP Provenance Wrapper

Update MCP tool handlers to extract user ID from the MCP request context and set up provenance with `mcp` source.

**Files to modify:**
- `pkg/mcp/tools/*.go` - All tool files that perform write operations

**Implementation:**

1. Create a helper function in `pkg/mcp/tools/` (perhaps in a common file like `tools.go` or `common.go`):
```go
func WithMCPProvenance(ctx context.Context, userID uuid.UUID) context.Context {
    return models.WithProvenance(ctx, models.ProvenanceContext{
        Source: models.SourceMCP,
        UserID: userID,
    })
}
```

2. Identify how MCP requests carry user context. Check:
   - `pkg/mcp/server.go` - How the MCP server handles requests
   - `pkg/handlers/mcp_handler.go` - How HTTP requests become MCP calls
   - The user ID may already be in the context from the HTTP layer

3. For each MCP tool that creates/updates ontology objects (entities, relationships, glossary, knowledge), wrap the context with MCP provenance before calling repository methods

4. Tools to check and potentially update:
   - `pkg/mcp/tools/entity.go`
   - `pkg/mcp/tools/relationship.go`
   - `pkg/mcp/tools/knowledge.go` (if exists)
   - `pkg/mcp/tools/access.go`
   - Any other tools that modify ontology objects

**Note:** Read-only tools (like `get_schema`, `list_approved_queries`) don't need provenance since they don't create audit logs.

---

#### 6.4 Add Inference Provenance Wrapper for DAG Steps

Update DAG task execution to set up provenance with `inference` source, using the user ID of whoever triggered the ontology extraction.

**Files to modify:**
- `pkg/services/*_task.go` - DAG step task files
- Potentially `pkg/services/incremental_dag_service.go` or wherever DAG execution is coordinated

**Implementation:**

1. Create a helper function (possibly in `pkg/services/` or reuse from MCP):
```go
func WithInferenceProvenance(ctx context.Context, userID uuid.UUID) context.Context {
    return models.WithProvenance(ctx, models.ProvenanceContext{
        Source: models.SourceInference,
        UserID: userID,
    })
}
```

2. Identify where DAG extraction is initiated:
   - Look in `pkg/services/incremental_dag_service.go`
   - Find the entry point where a user triggers extraction (likely an HTTP handler calling a service method)
   - The user ID should be captured at this entry point

3. Ensure the user ID flows through the DAG execution:
   - Check how context is passed through DAG steps
   - The user ID captured at extraction start should be available to all steps

4. For each task file (`*_task.go`), ensure the context used for repository calls has inference provenance set. This might be done:
   - At the DAG service level (once, before running steps)
   - Or at each task level (if tasks have independent contexts)

**Key insight:** The user who triggered extraction (via UI) should be recorded as `created_by` for all inference-created objects, even though the work happens asynchronously in DAG steps.

## Files to Modify/Create

| File | Change |
|------|--------|
| `migrations/XXX_provenance_schema.up.sql` | Schema changes |
| `pkg/models/provenance.go` | NEW: Provenance context |
| `pkg/models/ontology_entity.go` | Update fields |
| `pkg/models/entity_relationship.go` | Update fields |
| `pkg/models/glossary.go` | Update fields |
| `pkg/models/project_knowledge.go` | Update fields |
| `pkg/models/audit_log.go` | NEW: Audit log model |
| `pkg/repositories/*_repository.go` | Use provenance context |
| `pkg/repositories/audit_repository.go` | NEW: Audit repository |
| `pkg/services/audit_service.go` | NEW: Audit service |
| `pkg/server/middleware.go` | Add provenance middleware |
| `pkg/mcp/tools/*.go` | Add MCP provenance wrapper |
| `pkg/services/*_task.go` | Add inference provenance wrapper |

## Testing

1. Unit tests for provenance context
2. Integration tests verifying:
   - Inference creates with `source='inference'`, `created_by=<user_who_triggered>`
   - MCP creates with `source='mcp'`, `created_by=<user_id>`
   - Manual creates with `source='manual'`, `created_by=<user_id>`
   - All operations require valid user ID (no null created_by)
   - Re-extraction deletes only `inference` items
   - Audit log captures all changes chronologically

## Success Criteria

- [ ] All ontology objects have consistent provenance fields
- [ ] User UUID extracted from JWT and stored on create/update
- [ ] Audit log captures all changes across all object types
- [ ] Re-extraction preserves `mcp` and `manual` items
- [ ] Single provenance implementation, not duplicated per object type

## Open Questions

1. **Audit log retention:** How long to keep audit entries? Configurable per project?
2. **Audit log queries:** What queries does the UI need? (by user, by object, by time range?)
3. **Soft delete tracking:** Should audit log capture soft deletes differently from hard deletes?

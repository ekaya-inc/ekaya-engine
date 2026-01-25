# PLAN: Unified Provenance System for Ontology Objects

**Date:** 2026-01-25
**Status:** Planning
**Priority:** High

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

- Modify Create/Update methods to use provenance context
- Add `DeleteBySource` methods for re-extraction

### Step 5: Implement Audit Service

- Create audit service and repository
- Wire into all object repositories

### Step 6: Extract User ID from JWT

- Update JWT parsing to include user UUID (not just email)
- Ensure MCP requests carry user context

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

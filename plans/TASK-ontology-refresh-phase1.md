# TASK: Ontology Refresh Phase 1 - Provenance Fields

**Priority:** 2 (High)
**Status:** In Progress
**Parent:** PLAN-ontology-next.md
**Design Reference:** DESIGN-ontology-refresh.md (archived)

## Overview

Add provenance tracking to ontology entities and relationships so that user corrections survive refresh operations. This is the foundation for incremental refresh.

## Goal

Track the source and confidence of every ontology item so we can:
1. Preserve user-verified items during refresh
2. Distinguish between LLM-inferred, FK-derived, and user-corrected data
3. Know when items are stale and need re-evaluation

## Implementation

### Step 1: Migration

Create migration to add provenance fields:

```sql
-- Add provenance fields to entities
ALTER TABLE engine_ontology_entities
  ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'ddl',
  ADD COLUMN IF NOT EXISTS confidence FLOAT DEFAULT 0.5,
  ADD COLUMN IF NOT EXISTS verified_by_user BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS last_verified_at TIMESTAMP,
  ADD COLUMN IF NOT EXISTS is_stale BOOLEAN DEFAULT FALSE;

-- Add provenance fields to relationships
ALTER TABLE engine_entity_relationships
  ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'ddl',
  ADD COLUMN IF NOT EXISTS verified_by_user BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS last_verified_at TIMESTAMP,
  ADD COLUMN IF NOT EXISTS is_stale BOOLEAN DEFAULT FALSE;

-- Add schema fingerprint to ontologies
ALTER TABLE engine_ontologies
  ADD COLUMN IF NOT EXISTS schema_fingerprint TEXT,
  ADD COLUMN IF NOT EXISTS last_schema_check TIMESTAMP;

-- Constraints
ALTER TABLE engine_ontology_entities
  ADD CONSTRAINT valid_entity_source CHECK (source IN ('ddl', 'llm', 'user', 'mcp'));
ALTER TABLE engine_entity_relationships
  ADD CONSTRAINT valid_rel_source CHECK (source IN ('ddl', 'llm', 'user', 'mcp'));
```

### Step 2: Update Models ✓

**File:** `pkg/models/ontology_entity.go`
```go
type OntologyEntity struct {
    // ... existing fields ...
    Source          string     `json:"source"`           // "ddl", "llm", "user", "mcp"
    Confidence      float64    `json:"confidence"`       // 0.0-1.0
    VerifiedByUser  bool       `json:"verified_by_user"`
    LastVerifiedAt  *time.Time `json:"last_verified_at,omitempty"`
    IsStale         bool       `json:"is_stale"`
}
```

**File:** `pkg/models/entity_relationship.go`
```go
type EntityRelationship struct {
    // ... existing fields ...
    Source          string     `json:"source"`
    VerifiedByUser  bool       `json:"verified_by_user"`
    LastVerifiedAt  *time.Time `json:"last_verified_at,omitempty"`
    IsStale         bool       `json:"is_stale"`
}
```

### Step 3: Update Repositories ✓

Update INSERT/UPDATE queries to include new fields.
Update SELECT queries to return new fields.

### Step 4: Update Services ✓

**Entity Discovery:** Set `source='ddl'` for entities from schema, `confidence=0.5`
**Entity Enrichment:** Set `source='llm'`, `confidence=0.7-0.9` based on LLM response
**FK Discovery:** Set `source='ddl'`, `confidence=1.0` for FK-based relationships
**PK Match Discovery:** Set `source='llm'`, `confidence` from match rate

### Step 5: Update Re-extract Logic ✓

Modify re-extract to:
1. Check `verified_by_user` before deleting
2. Preserve user-verified entities/relationships
3. Mark non-verified items as stale instead of deleting
4. Re-enrich only stale items

## Files to Modify

| File | Change |
|------|--------|
| `migrations/0XX_provenance_fields.up.sql` | New migration |
| `pkg/models/ontology_entity.go` | Add provenance fields |
| `pkg/models/entity_relationship.go` | Add provenance fields |
| `pkg/repositories/ontology_entity_repository.go` | Update queries |
| `pkg/repositories/entity_relationship_repository.go` | Update queries |
| `pkg/services/entity_discovery.go` | Set source='ddl' |
| `pkg/services/entity_enrichment.go` | Set source='llm' |
| `pkg/services/deterministic_relationship_service.go` | Set source |
| `pkg/services/ontology_builder_service.go` | Re-extract logic |

## Testing

1. Run ontology extraction, verify entities have source='ddl'/'llm'
2. Manually update an entity (should set verified_by_user=true)
3. Re-extract ontology
4. Verify user-verified entity was preserved

## Success Criteria

- [x] Provenance fields exist on entities and relationships
- [x] New extractions populate source and confidence
- [x] User-verified items are preserved during re-extract
- [ ] API returns provenance fields

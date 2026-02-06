# SECURITY: RLS Bypass When Customer Uses Superuser Credentials

## Summary

PostgreSQL Row Level Security (RLS) is bypassed for superusers by default. If a customer configures their datasource with superuser credentials (`PGUSER`), queries that rely on RLS for tenant isolation will expose data from ALL projects.

## Root Cause

PostgreSQL tables have two RLS settings:
- `relrowsecurity` - enables RLS policies (we have this set to `true`)
- `relforcerowsecurity` - forces RLS even for table owners/superusers (we have this set to `false`)

When `relforcerowsecurity = false`, superusers bypass all RLS policies.

## Affected Components

### Vulnerable Repository Methods

These methods query by ID without explicit `project_id` filtering, relying solely on RLS:

#### 1. GlossaryRepository.GetByID
**File:** `pkg/repositories/glossary_repository.go:333-364`
```go
query := `
    SELECT ... FROM engine_business_glossary g
    WHERE g.id = $1  -- NO project_id filter
    ...`
```

#### 2. OntologyEntityRepository.GetByID
**File:** `pkg/repositories/ontology_entity_repository.go:410-435`
```go
query := `
    SELECT ... FROM engine_ontology_entities
    WHERE id = $1 AND NOT is_deleted  -- NO project_id filter
    ...`
```

#### 3. EntityRelationshipRepository.GetByID
**File:** `pkg/repositories/entity_relationship_repository.go:330-355`
```go
query := `
    SELECT ... FROM engine_entity_relationships
    WHERE id = $1  -- NO project_id filter
    ...`
```

#### 4. OntologyQuestionRepository.GetByID
**File:** `pkg/repositories/ontology_question_repository.go:220-242`
```go
query := `
    SELECT ... FROM engine_ontology_questions
    WHERE id = $1  -- NO project_id filter
    ...`
```

#### 5. SchemaRepository.GetColumnByName
**File:** `pkg/repositories/schema_repository.go:591-616`
```go
query := `
    WHERE schema_table_id = $1 AND column_name = $2 AND deleted_at IS NULL
    -- Only filters by table_id, no project_id verification`
```

### RLS-Protected Tables Summary

| Table | GetByID Vulnerable | Other Methods |
|-------|-------------------|---------------|
| `engine_business_glossary` | YES | Safe (include project_id) |
| `engine_ontology_entities` | YES | Safe |
| `engine_entity_relationships` | YES | Safe |
| `engine_ontology_questions` | YES | Safe |
| `engine_schema_columns` | N/A | GetColumnByName risky |
| `engine_ontology_chat_messages` | N/A | Safe |
| `engine_project_knowledge` | N/A | Safe |
| `engine_ontologies` | N/A | Safe |

## Recommended Fixes

### Option A: Force RLS on All Tables (Defense in Depth)

Add migration to force RLS for superusers:

```sql
ALTER TABLE engine_business_glossary FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_entities FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_entity_relationships FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_questions FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_chat_messages FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_project_knowledge FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_ontologies FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_schema_tables FORCE ROW LEVEL SECURITY;
ALTER TABLE engine_schema_columns FORCE ROW LEVEL SECURITY;
-- Add all other RLS-protected tables
```

**Pros:** Single fix, defense in depth
**Cons:** May break admin tooling that expects superuser access

### Option B: Add project_id to All GetByID Methods

Update each vulnerable method to require and filter by `project_id`:

```go
// Before
func (r *repo) GetByID(ctx context.Context, id uuid.UUID) (*Model, error)

// After
func (r *repo) GetByID(ctx context.Context, projectID, id uuid.UUID) (*Model, error)
```

Update WHERE clauses:
```sql
WHERE project_id = $1 AND id = $2
```

**Pros:** Explicit, no reliance on RLS
**Cons:** Requires updating all callers

### Option C: Both (Recommended)

1. Add `FORCE ROW LEVEL SECURITY` as defense in depth
2. Also add explicit `project_id` filtering to all queries

This follows the principle of defense in depth - even if one layer fails, the other protects.

## Verification Steps

To check current RLS force status:
```sql
SELECT relname, relrowsecurity, relforcerowsecurity
FROM pg_class
WHERE relname LIKE 'engine_%';
```

To test if a query bypasses RLS:
```sql
-- As superuser, without setting context
SELECT COUNT(*) FROM engine_project_knowledge;  -- Returns ALL rows

-- As superuser, with context (still returns all if not forced)
SELECT set_config('app.current_project_id', 'uuid-here', false);
SELECT COUNT(*) FROM engine_project_knowledge;  -- Still returns ALL rows
```

## Partial Mitigation

A WARN-level log was added at server startup (commit `4dba72e`) that detects superuser or bypassrls privileges on the engine database connection and alerts the operator. This makes the risk visible but does not enforce RLS — the substantive fixes (FORCE ROW LEVEL SECURITY migration, explicit `project_id` filtering) are still required.

## Priority

**HIGH** - This is a multi-tenant data isolation issue. If any customer uses superuser credentials, they could potentially access other customers' data through API calls that use the vulnerable GetByID methods.

## Discovery

Found during ontology extraction debugging on 2026-01-29. The issue was noticed when `psql` queries as superuser `damondanieli` returned data from multiple projects despite setting `app.current_project_id`.

# PLAN: Add Non-Entity Table Classification

## Problem

The current EntityDiscovery step only creates ontology entries for tables with primary keys or unique+non-null columns. Tables without these constraints are silently ignored, but they're still important:

1. **Join/bridge tables** - Many-to-many relationship tables (e.g., `user_roles`) that connect entities
2. **Event/log tables** - Append-only activity tables (e.g., `audit_log`, `page_views`)
3. **Report tables** - Aggregation/summary tables users need to query directly

From an MCP client perspective (e.g., Claude Code using `get_ontology`), we need visibility into ALL tables to write effective queries. A report table might be exactly what a user wants, even without join keys.

## Solution

Add a `classification` column to `engine_schema_tables` that categorizes every table:

| Classification | Description |
|----------------|-------------|
| `entity` | Tables with identity (PK or unique+not null) - these also get `engine_ontology_entities` entries |
| `relationship` | Join/bridge tables connecting entities |
| `event` | Append-only activity/log tables |
| `unclassified` | Tables that don't match heuristics - LLM classifies later |

## Deterministic Classification Heuristics

**NO string/name pattern matching** - only structural signals from DDL metadata:

### Entity (existing logic)
- Has single-column primary key, OR
- Has unique + non-null column

### Relationship
- Has 2+ foreign keys, AND
- Column count ≤ FK count + 2 (allows for timestamps/metadata), AND
- No single-column primary key

### Event
- Has timestamp column as primary key, OR
- Has timestamp column + high row count + no unique constraint

### Unclassified
- Everything else - candidates for later LLM classification with full context

## Database Migration

Create migration to add classification to `engine_schema_tables`:

```sql
-- Add classification column with default 'unclassified'
ALTER TABLE engine_schema_tables
ADD COLUMN classification TEXT NOT NULL DEFAULT 'unclassified';

-- Add CHECK constraint for valid values
ALTER TABLE engine_schema_tables
ADD CONSTRAINT engine_schema_tables_classification_check
CHECK (classification IN ('entity', 'relationship', 'event', 'unclassified'));

-- Add index for efficient filtering
CREATE INDEX idx_engine_schema_tables_classification
ON engine_schema_tables(project_id, datasource_id, classification);
```

## Code Changes

### 1. Models (`pkg/models/schema.go`)

Add classification constants and field:

```go
// Table classification types
const (
    TableClassificationEntity       = "entity"
    TableClassificationRelationship = "relationship"
    TableClassificationEvent        = "event"
    TableClassificationUnclassified = "unclassified"
)

// Add to SchemaTable struct:
Classification string `json:"classification"`
```

### 2. Repository (`pkg/repositories/schema_repository.go`)

Update CRUD operations to include `classification` column in SELECT, INSERT, UPDATE queries.

### 3. EntityDiscovery Service (`pkg/services/entity_discovery_service.go`)

Modify `IdentifyEntitiesFromDDL` to:

1. **Classify ALL tables** (not just entity candidates):
   - Apply relationship heuristic (2+ FKs, few columns, no single-column PK)
   - Apply event heuristic (timestamp PK or timestamp + high row count)
   - Default to `unclassified`

2. **Update schema_tables** with classification:
   - Set `classification = 'entity'` for tables that get ontology entities
   - Set `classification = 'relationship'` for join tables
   - Set `classification = 'event'` for event tables
   - Leave as `unclassified` for others

3. **Still create `engine_ontology_entities`** only for entity-classified tables (existing behavior)

### 4. MCP Ontology Output

Update `get_ontology` response to include all tables with their classification:

```json
{
  "entities": [...],
  "tables": [
    {
      "schema": "public",
      "name": "user_roles",
      "classification": "relationship",
      "columns": ["user_id", "role_id", "created_at"],
      "foreign_keys": [
        {"column": "user_id", "references": "users.id"},
        {"column": "role_id", "references": "roles.id"}
      ]
    },
    {
      "schema": "public",
      "name": "monthly_revenue",
      "classification": "unclassified",
      "columns": ["month", "total_revenue", "transaction_count"]
    }
  ]
}
```

## Implementation Order

1. **Migration** - Add `classification` column with constraint and index
2. **Models** - Add constants and field to `SchemaTable`
3. **Repository** - Update queries to handle new column
4. **Classification Logic** - Add heuristic functions to identify relationship/event tables
5. **EntityDiscovery** - Integrate classification into the discovery step
6. **MCP Output** - Include all tables in ontology response

## Testing

- Verify relationship detection: table with 2 FKs and 3 columns classified as `relationship`
- Verify event detection: table with timestamp PK classified as `event`
- Verify entity detection unchanged: tables with PK still get ontology entities
- Verify unclassified fallback: ambiguous tables stay `unclassified`
- Verify MCP output includes all tables with correct classifications

## Future: LLM Classification

Tables marked `unclassified` can be classified by LLM in a later DAG step. The LLM would have full context (column names, sample data, relationships) to make informed decisions about whether a table is a report, staging table, or something else.

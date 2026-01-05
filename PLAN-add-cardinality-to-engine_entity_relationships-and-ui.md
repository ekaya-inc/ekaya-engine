# PLAN: Add Cardinality to Entity Relationships and UI

## Goal

Store cardinality (1:1, 1:N, N:1, N:M) in `engine_entity_relationships` and display it in the UI. This is critical for MCP clients to understand join semantics.

## Current State

### Cardinality Data Flow
- **Source**: `engine_schema_relationships.cardinality` - populated during FK discovery
- **Gap**: `engine_entity_relationships` has NO cardinality column
- **Result**: Cardinality data exists but isn't propagated to entity relationships

### Verification
```sql
-- Schema relationships HAVE cardinality
SELECT DISTINCT cardinality, COUNT(*) FROM engine_schema_relationships GROUP BY cardinality;
-- Result: N:1 (14), unknown (1)

-- Entity relationships do NOT
\d engine_entity_relationships
-- No cardinality column
```

### Key Files

| File | Purpose |
|------|---------|
| `pkg/models/entity_relationship.go` | `EntityRelationship` model - needs Cardinality field |
| `pkg/repositories/entity_relationship_repository.go` | DB operations - needs to read/write cardinality |
| `pkg/services/deterministic_relationship_service.go` | FKDiscovery + PKMatchDiscovery - needs to populate cardinality |
| `pkg/handlers/entity_relationship_handler.go` | API response - already has field but not populated |
| `ui/src/types/schema.ts` | Already has `Cardinality` type and field in `RelationshipDetail` |
| `ui/src/pages/RelationshipsPage.tsx` | Already has cardinality display code (lines 595-599) |

## Implementation

### 1. Database Migration

**New migration file**: `migrations/XXXXXX_add_cardinality_to_entity_relationships.up.sql`

```sql
ALTER TABLE engine_entity_relationships
ADD COLUMN cardinality VARCHAR(10);

COMMENT ON COLUMN engine_entity_relationships.cardinality IS 'Relationship cardinality: 1:1, 1:N, N:1, N:M, or unknown';
```

**Down migration**: `migrations/XXXXXX_add_cardinality_to_entity_relationships.down.sql`

```sql
ALTER TABLE engine_entity_relationships DROP COLUMN cardinality;
```

### 2. Update Entity Relationship Model

**File**: `pkg/models/entity_relationship.go`

Add field to `EntityRelationship` struct:
```go
type EntityRelationship struct {
    // ... existing fields ...
    Cardinality      string     `json:"cardinality,omitempty"`
}
```

### 3. Update Repository

**File**: `pkg/repositories/entity_relationship_repository.go`

Update `Create` method to include cardinality in INSERT:
```go
// In the INSERT statement, add cardinality column
_, err := tx.ExecContext(ctx, `
    INSERT INTO engine_entity_relationships (
        id, ontology_id, source_entity_id, target_entity_id,
        source_column_schema, source_column_table, source_column_name,
        target_column_schema, target_column_table, target_column_name,
        detection_method, confidence, status, cardinality
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
`, rel.ID, rel.OntologyID, rel.SourceEntityID, rel.TargetEntityID,
   rel.SourceColumnSchema, rel.SourceColumnTable, rel.SourceColumnName,
   rel.TargetColumnSchema, rel.TargetColumnTable, rel.TargetColumnName,
   rel.DetectionMethod, rel.Confidence, rel.Status, rel.Cardinality)
```

Update SELECT queries to include cardinality in the column list.

### 4. Update FKDiscovery to Copy Cardinality

**File**: `pkg/services/deterministic_relationship_service.go`

In `DiscoverFKRelationships`, when creating entity relationships from schema relationships:

```go
// Around line 166 where EntityRelationship is created
entityRel := &models.EntityRelationship{
    // ... existing fields ...
    Cardinality:     schemaRel.Cardinality,  // ADD THIS
}
```

The schema relationship is already fetched and has cardinality populated.

### 5. Update PKMatchDiscovery to Compute Cardinality

**File**: `pkg/services/deterministic_relationship_service.go`

In `DiscoverPKMatchRelationships`, compute cardinality from column stats:

```go
// Helper function to infer cardinality from stats
func inferCardinality(sourceDistinct, targetDistinct int64, sourceRows, targetRows int64) string {
    if sourceDistinct == 0 || targetDistinct == 0 {
        return "unknown"
    }

    // If source distinct = source rows and target distinct = target rows → 1:1
    // If source distinct < source rows and target distinct = target rows → N:1
    // If source distinct = source rows and target distinct < target rows → 1:N
    // Otherwise → N:M or unknown

    sourceIsUnique := sourceRows > 0 && sourceDistinct == sourceRows
    targetIsUnique := targetRows > 0 && targetDistinct == targetRows

    if sourceIsUnique && targetIsUnique {
        return "1:1"
    } else if !sourceIsUnique && targetIsUnique {
        return "N:1"
    } else if sourceIsUnique && !targetIsUnique {
        return "1:N"
    }
    return "N:M"
}
```

When creating entity relationship for pk_match:
```go
entityRel := &models.EntityRelationship{
    // ... existing fields ...
    Cardinality: inferCardinality(sourceStats, targetStats),
}
```

### 6. Update API Handler

**File**: `pkg/handlers/entity_relationship_handler.go`

In `List` handler, map cardinality (around line 169):
```go
relResponses = append(relResponses, EntityRelationshipResponse{
    // ... existing fields ...
    Cardinality:      rel.Cardinality,  // ADD THIS (field already exists in response struct)
})
```

### 7. Frontend (Already Done!)

The frontend already supports cardinality:

**`ui/src/types/schema.ts`** (line 99):
```typescript
export type Cardinality = '1:1' | '1:N' | 'N:1' | 'N:M';
```

**`ui/src/types/schema.ts`** (line 113):
```typescript
cardinality: Cardinality | null;
```

**`ui/src/pages/RelationshipsPage.tsx`** (lines 595-599):
```typescript
{rel.cardinality && (
  <div className="mt-1 text-xs text-text-secondary">
    Cardinality: {rel.cardinality}
  </div>
)}
```

## Cardinality Constants

Use existing constants from `pkg/models/cardinality.go` (if exists) or define:

```go
const (
    Cardinality1To1   = "1:1"
    CardinalityNTo1   = "N:1"
    Cardinality1ToN   = "1:N"
    CardinalityNToM   = "N:M"
    CardinalityUnknown = "unknown"
)
```

## Testing

1. **Migration test**: Run migration, verify column exists
   ```sql
   \d engine_entity_relationships
   -- Should show: cardinality | character varying(10) |
   ```

2. **FKDiscovery test**: Run extraction, verify cardinality copied from schema_relationships
   ```sql
   SELECT er.source_column_table, er.cardinality, sr.cardinality as schema_cardinality
   FROM engine_entity_relationships er
   JOIN engine_schema_relationships sr ON er.source_column_table =
       (SELECT table_name FROM engine_schema_columns WHERE id = sr.source_column_id)
   WHERE er.detection_method = 'foreign_key';
   ```

3. **PKMatchDiscovery test**: Verify inferred relationships have cardinality
   ```sql
   SELECT source_column_table, target_column_table, cardinality
   FROM engine_entity_relationships
   WHERE detection_method = 'pk_match';
   ```

4. **API test**: Verify response includes cardinality
   ```bash
   curl http://localhost:3443/api/projects/{pid}/relationships | jq '.data.relationships[0].cardinality'
   ```

5. **UI test**: Relationships page shows cardinality below each relationship

## UI Design (Already Implemented)

```
┌──────────────────────────────────────────────────────────────────────────┐
│ users (4 relationships)                                                  │
├──────────────────────────────────────────────────────────────────────────┤
│ 🔗 account_id (uuid) → accounts . account_id (uuid)      [Foreign Key]  │
│    Cardinality: N:1                                                      │
│                                                                          │
│ 💡 user_id (uuid) → accounts . account_id (uuid)         [Inferred]     │
│    Cardinality: N:1                                                      │
└──────────────────────────────────────────────────────────────────────────┘
```

## Notes

- Cardinality is critical for MCP clients to generate correct SQL joins
- N:1 means "many source rows to one target row" (typical FK pattern)
- 1:N means "one source row to many target rows" (reverse FK perspective)
- Existing relationships without cardinality will show nothing (handled by conditional rendering)
- Consider backfilling existing relationships by re-running extraction or a one-time script

# PLAN: Deterministic Entity Relationships

## Implementation Status

### COMPLETED

**Phase 1 (FK Discovery): DONE**
- Database migration: `017_entity_relationships`
- Backend: model → repository → service → handler
- Reads from `engine_schema_relationships` (FKs imported during schema refresh)
- Creates entity relationships with `detection_method='foreign_key'`, `confidence=1.0`

**Phase 2 (PK-Match Inference): DONE**
- Tests joins from entity reference columns to filtered FK candidates
- Uses type matching, cardinality filtering, and join analysis
- Uniform-length text detection for UUID columns (migration 018)
- Orphan rate thresholds: <5% confirmed, 5-20% pending, >20% rejected

**UI Integration: DONE**
- Handler returns UI-compatible `RelationshipDetail` format
- UI calls `GET /api/projects/{pid}/relationships` (new entity relationships)
- Shows "14 Foreign Keys, 20 Inferred" correctly
- Fixed double-run-on-save bug in discovery modal

### Test Results

With test datasource (project `57368b25-816a-41e5-b495-7d6d5f794fce`):
- **14 FK relationships** from database foreign key constraints
- **20 inferred relationships** from PK-match with uniform-length text columns
- **34 total relationships across 14 tables**

---

## Files Changed

### Migrations
| File | Purpose |
|------|---------|
| `migrations/017_entity_relationships.up.sql` | Entity relationships table |
| `migrations/018_column_length_stats.up.sql` | Add min_length/max_length for text detection |

### Backend
| File | Purpose |
|------|---------|
| `pkg/models/entity_relationship.go` | EntityRelationship model |
| `pkg/models/schema.go` | Added MinLength/MaxLength to SchemaColumn |
| `pkg/repositories/entity_relationship_repository.go` | CRUD for entity relationships |
| `pkg/repositories/schema_repository.go` | Updated queries for length stats |
| `pkg/services/deterministic_relationship_service.go` | FK + PK-match discovery |
| `pkg/handlers/entity_relationship_handler.go` | API endpoints with UI-compatible response |
| `pkg/adapters/datasource/metadata.go` | ColumnStats with length fields |
| `pkg/adapters/datasource/postgres/schema.go` | Compute length stats in AnalyzeColumnStats |

### Frontend
| File | Purpose |
|------|---------|
| `ui/src/services/engineApi.ts` | Changed to call `/relationships` endpoint |
| `ui/src/components/RelationshipDiscoveryProgress.tsx` | Fixed double-run bug |

---

## Algorithm Summary

### Phase 1: FK Relationships (Gold Standard)
```
FOR each FK in engine_schema_relationships:
  source_entity = find entity with occurrence in FK source table
  target_entity = find entity with occurrence in FK target table

  IF both entities found AND not self-reference:
    CREATE relationship(confidence=1.0, method='foreign_key', status='confirmed')
```

### Phase 2: PK-Match Inference
```
FOR each entity reference column (PK, unique, high cardinality, uniform-length text):
  FOR each candidate column with matching type:
    IF not same table AND not PK-to-PK:
      RUN join analysis to calculate orphan rate

      IF orphan_rate < 5%:  status='confirmed', confidence=0.9
      IF orphan_rate < 20%: status='pending', confidence=0.5-0.9
      ELSE: skip (not a valid relationship)
```

### Uniform-Length Text Detection
Text columns are included as join candidates if `min_length == max_length > 0`:
- UUIDs have uniform 36-char length
- Allows discovery of text-based ID columns without relying on `_id` suffix

---

## API Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/api/projects/{pid}/datasources/{dsid}/relationships/discover` | Run FK + PK-match discovery |
| GET | `/api/projects/{pid}/relationships` | List all entity relationships |

---

## Deferred Work

**LLM-Based Discovery:** Commented out in `relationship_workflow.go:479-675`
- Column scanning, value matching, semantic analysis
- Can be re-enabled when needed

**Future Enhancements:**
- Relationship labels ("owns", "created by")
- Cardinality detection (1:1, 1:N, N:M)
- Manual relationship CRUD from UI

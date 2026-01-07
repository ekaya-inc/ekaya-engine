# PLAN: Show Enhanced Columns in Entities Screen

## Goal

Allow users to view enriched column metadata (descriptions, synonyms, semantic types, FK roles) directly from the Entities screen by expanding an entity row.

## Current State

### Data Available
- **Entities**: `engine_ontology_entities` table, displayed on Entities page
- **Column Enrichment**: `engine_ontologies.column_details` JSONB field, keyed by table name
- **Link**: Entity has `primary_table` field which maps to `column_details[primary_table]`

### What Column Enrichment Contains
```json
{
  "name": "visitor_id",
  "description": "Unique identifier of the visitor participating in the engagement.",
  "semantic_type": "identifier",
  "role": "dimension",
  "synonyms": ["user_id", "customer_id"],
  "is_primary_key": false,
  "is_foreign_key": true,
  "foreign_table": "users",
  "fk_role": "visitor"
}
```

### Current UI Gap
- Entities page shows: name, description, domain, primary_table
- No way to drill into column-level details
- Column enrichment data exists but is only exposed via MCP/API

## Design

### UI: Expandable Entity Rows

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Entities                                                      [Search] │
├─────────────────────────────────────────────────────────────────────────┤
│ ▶ User                    customer    A user profile within an account  │
│ ▼ Billing Engagement      billing     Represents a billable engagement  │
│   ┌──────────────────────────────────────────────────────────────────┐  │
│   │ Columns (12)                                        primary_table: billing_engagements │
│   ├──────────────────────────────────────────────────────────────────┤  │
│   │ id              PK   identifier                                  │  │
│   │                 Unique primary key identifier                     │  │
│   │                                                                   │  │
│   │ visitor_id      FK→users   dimension   role: visitor             │  │
│   │                 Unique identifier of the visitor                  │  │
│   │                 aka: user_id, customer_id                         │  │
│   │                                                                   │  │
│   │ host_id         FK→users   dimension   role: host                │  │
│   │                 Unique identifier of the host                     │  │
│   │                 aka: provider_id, service_provider_id             │  │
│   │                                                                   │  │
│   │ started_at      timestamp_utc   dimension                        │  │
│   │                 Timestamp when the engagement began               │  │
│   │                 aka: engagement_start, session_start_time         │  │
│   └──────────────────────────────────────────────────────────────────┘  │
│ ▶ Channel                 content     A content channel owned by user   │
└─────────────────────────────────────────────────────────────────────────┘
```

### Interaction
1. Click chevron (▶) or row to expand
2. Columns load (may need API call if not pre-loaded)
3. Click again to collapse
4. Multiple entities can be expanded simultaneously

### Badge/Chip Display
- **PK**: Primary key indicator (blue badge)
- **FK→{table}**: Foreign key with target table (purple badge)
- **role: {fk_role}**: Semantic role for FK columns (e.g., "visitor", "host")
- **{semantic_type}**: identifier, timestamp_utc, count, currency, etc. (gray badge)
- **{role}**: identifier, attribute, dimension (subtle text)
- **aka: {synonyms}**: Alternative names (italic, comma-separated)

## API Design

### Option A: Enhance Entities Endpoint (Recommended)

**Current**: `GET /api/projects/{pid}/entities`

**Enhanced**: `GET /api/projects/{pid}/entities?include=columns`

Response adds `columns` array to each entity:
```json
{
  "entities": [
    {
      "id": "...",
      "name": "Billing Engagement",
      "description": "...",
      "domain": "billing",
      "primary_table": "billing_engagements",
      "columns": [
        {
          "name": "visitor_id",
          "description": "...",
          "semantic_type": "identifier",
          "role": "dimension",
          "synonyms": ["user_id", "customer_id"],
          "is_primary_key": false,
          "is_foreign_key": true,
          "foreign_table": "users",
          "fk_role": "visitor"
        }
      ]
    }
  ]
}
```

### Option B: Lazy Load on Expand

**New endpoint**: `GET /api/projects/{pid}/entities/{entityId}/columns`

- Only fetch columns when user expands an entity
- Reduces initial payload
- Slightly more complex UI state management

**Recommendation**: Start with Option A for simplicity. If entity count is large (50+), consider Option B.

## Implementation

### Backend Changes

#### 1. Add Column Details to Entity Response

**File**: `pkg/handlers/entity_handler.go`

Add handler logic to:
1. Check for `include=columns` query param
2. Fetch active ontology for project
3. For each entity, look up `ontology.ColumnDetails[entity.PrimaryTable]`
4. Attach to response

```go
// In ListEntities handler
includeColumns := r.URL.Query().Get("include") == "columns"

if includeColumns {
    ontology, err := h.ontologyRepo.GetActive(ctx, projectID)
    if err == nil && ontology != nil {
        for i := range entities {
            if cols, ok := ontology.ColumnDetails[entities[i].PrimaryTable]; ok {
                entities[i].Columns = cols
            }
        }
    }
}
```

#### 2. Update Entity Model

**File**: `pkg/models/ontology_entity.go`

Add optional Columns field:
```go
type OntologyEntity struct {
    // ... existing fields ...

    // Columns contains enriched column details (populated when requested)
    Columns []ColumnDetail `json:"columns,omitempty"`
}
```

#### 3. Ensure ColumnDetail is Exported

**File**: `pkg/models/ontology.go`

Verify `ColumnDetail` struct has all fields and proper JSON tags:
```go
type ColumnDetail struct {
    Name          string   `json:"name"`
    Description   string   `json:"description,omitempty"`
    SemanticType  string   `json:"semantic_type,omitempty"`
    Role          string   `json:"role,omitempty"`
    Synonyms      []string `json:"synonyms,omitempty"`
    IsPrimaryKey  bool     `json:"is_primary_key"`
    IsForeignKey  bool     `json:"is_foreign_key"`
    ForeignTable  string   `json:"foreign_table,omitempty"`
    FKRole        string   `json:"fk_role,omitempty"`
}
```

### Frontend Changes

#### 1. Update Entity Type

**File**: `ui/src/types/entity.ts` or `ui/src/types/ontology.ts`

```typescript
interface ColumnDetail {
  name: string;
  description?: string;
  semantic_type?: string;
  role?: string;
  synonyms?: string[];
  is_primary_key: boolean;
  is_foreign_key: boolean;
  foreign_table?: string;
  fk_role?: string;
}

interface Entity {
  id: string;
  name: string;
  description: string;
  domain: string;
  primary_table: string;
  columns?: ColumnDetail[];  // Added
}
```

#### 2. Update API Call

**File**: `ui/src/services/entityApi.ts` or `engineApi.ts`

```typescript
export async function getEntities(
  projectId: string,
  includeColumns = false
): Promise<Entity[]> {
  const params = includeColumns ? '?include=columns' : '';
  const response = await fetch(`/api/projects/${projectId}/entities${params}`);
  return response.json();
}
```

#### 3. Create EntityColumns Component

**File**: `ui/src/components/entities/EntityColumns.tsx`

```typescript
interface EntityColumnsProps {
  columns: ColumnDetail[];
  tableName: string;
}

export function EntityColumns({ columns, tableName }: EntityColumnsProps) {
  return (
    <div className="pl-8 py-4 bg-gray-50 border-t">
      <div className="text-sm text-gray-500 mb-3">
        Columns ({columns.length}) • {tableName}
      </div>
      <div className="space-y-3">
        {columns.map((col) => (
          <ColumnRow key={col.name} column={col} />
        ))}
      </div>
    </div>
  );
}

function ColumnRow({ column }: { column: ColumnDetail }) {
  return (
    <div className="border-l-2 border-gray-200 pl-3">
      <div className="flex items-center gap-2 flex-wrap">
        <span className="font-mono font-medium">{column.name}</span>

        {column.is_primary_key && (
          <Badge variant="blue">PK</Badge>
        )}

        {column.is_foreign_key && column.foreign_table && (
          <Badge variant="purple">FK→{column.foreign_table}</Badge>
        )}

        {column.fk_role && (
          <span className="text-xs text-purple-600">role: {column.fk_role}</span>
        )}

        {column.semantic_type && (
          <Badge variant="gray">{column.semantic_type}</Badge>
        )}
      </div>

      {column.description && (
        <p className="text-sm text-gray-600 mt-1">{column.description}</p>
      )}

      {column.synonyms && column.synonyms.length > 0 && (
        <p className="text-xs text-gray-400 mt-1 italic">
          aka: {column.synonyms.join(', ')}
        </p>
      )}
    </div>
  );
}
```

#### 4. Update EntitiesPage with Expandable Rows

**File**: `ui/src/pages/EntitiesPage.tsx`

Add state for expanded entities and toggle logic:

```typescript
const [expandedEntities, setExpandedEntities] = useState<Set<string>>(new Set());

const toggleEntity = (entityId: string) => {
  setExpandedEntities(prev => {
    const next = new Set(prev);
    if (next.has(entityId)) {
      next.delete(entityId);
    } else {
      next.add(entityId);
    }
    return next;
  });
};

// In render, for each entity row:
<div key={entity.id}>
  <EntityRow
    entity={entity}
    expanded={expandedEntities.has(entity.id)}
    onToggle={() => toggleEntity(entity.id)}
  />
  {expandedEntities.has(entity.id) && entity.columns && (
    <EntityColumns columns={entity.columns} tableName={entity.primary_table} />
  )}
</div>
```

#### 5. Add Chevron Icon to Entity Row

Update entity row to show expand/collapse chevron:

```typescript
<button onClick={onToggle} className="mr-2">
  {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
</button>
```

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `pkg/handlers/entity_handler.go` | Modify | Add `include=columns` support |
| `pkg/models/ontology_entity.go` | Modify | Add `Columns` field |
| `ui/src/types/ontology.ts` | Modify | Add/verify `ColumnDetail` interface |
| `ui/src/services/entityApi.ts` | Modify | Add `includeColumns` param |
| `ui/src/components/entities/EntityColumns.tsx` | Create | New component for column display |
| `ui/src/pages/EntitiesPage.tsx` | Modify | Add expand/collapse logic |

## Testing

1. **API Test**: `GET /api/projects/{pid}/entities?include=columns` returns columns
2. **UI Test**: Click entity row → expands to show columns
3. **Display Test**: PK, FK, semantic_type badges render correctly
4. **Empty State**: Entity with no enriched columns shows graceful message
5. **Performance**: Page still loads quickly with 30+ entities

## Future Enhancements

1. **Edit Column Metadata**: Allow editing description/synonyms inline
2. **Column Search**: Filter columns within expanded entity
3. **Export**: Download entity + columns as JSON/CSV
4. **Link to Schema**: Click foreign_table badge to navigate to that entity

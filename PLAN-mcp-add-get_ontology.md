# PLAN: Add `get_ontology` MCP Tool

## Purpose

Create a new MCP tool `get_ontology` that allows the MCP client (Claude Code or similar LLM) to iteratively probe the ontology at different levels of depth. This provides structured, rich semantic information beyond what `get_schema` offers.

**Design Philosophy:** Progressive disclosure with structured JSON output. The client controls how deep to go based on its needs.

---

## Tool Specification

### `get_ontology`

```
Tool: get_ontology
Purpose: Get structured ontology information at configurable depth levels

Input:
{
  "depth": "domain" | "entities" | "tables" | "columns",  // Required
  "tables": ["orders", "users"],                          // Optional: filter to specific tables (for "tables" or "columns" depth)
  "include_relationships": true                           // Optional: include relationship graph (default: true)
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
- destructive_hint: false
- open_world_hint: false
```

### Depth Levels

#### `depth: "domain"` (~200-500 tokens)
High-level business context. Call once per session to understand what this database is about.

```json
{
  "domain": {
    "description": "E-commerce platform for B2B wholesale transactions...",
    "primary_domains": ["sales", "customer", "product"],
    "table_count": 38,
    "column_count": 612
  },
  "entities": [
    {
      "name": "user",
      "description": "Platform users including customers and internal staff",
      "primary_table": "users",
      "occurrence_count": 15
    },
    {
      "name": "order",
      "description": "Customer purchase orders",
      "primary_table": "orders",
      "occurrence_count": 8
    }
  ],
  "relationships": [
    {"from": "user", "to": "order", "label": "places", "cardinality": "1:N"},
    {"from": "order", "to": "product", "label": "contains", "cardinality": "N:M"}
  ]
}
```

#### `depth: "entities"` (~500-1500 tokens)
Entity summaries with key columns. Good for understanding which tables matter for a query.

```json
{
  "entities": {
    "user": {
      "primary_table": "users",
      "description": "Platform users including customers and internal staff",
      "synonyms": ["customer", "account", "member"],
      "key_columns": [
        {"name": "id", "synonyms": ["user_id"]},
        {"name": "email", "synonyms": ["email_address"]},
        {"name": "tier", "synonyms": ["level", "membership"]}
      ],
      "occurrences": [
        {"table": "orders", "column": "customer_id", "role": "customer"},
        {"table": "orders", "column": "sales_rep_id", "role": "sales_rep"},
        {"table": "visits", "column": "host_id", "role": "host"},
        {"table": "visits", "column": "visitor_id", "role": "visitor"}
      ]
    },
    "order": {
      "primary_table": "orders",
      "description": "Customer purchase orders",
      "synonyms": ["purchase", "transaction"],
      "key_columns": [
        {"name": "id", "synonyms": ["order_id", "order_number"]},
        {"name": "total_amount", "synonyms": ["revenue", "order_total"]},
        {"name": "status", "synonyms": ["order_status", "state"]}
      ],
      "occurrences": [
        {"table": "order_items", "column": "order_id", "role": null}
      ]
    }
  },
  "relationships": [
    {
      "from_entity": "user",
      "from_table": "users",
      "to_entity": "order",
      "to_table": "orders",
      "via_column": "customer_id",
      "cardinality": "1:N"
    }
  ]
}
```

#### `depth: "tables"` (variable, depends on filter)
Table-level summaries with column overview. Use `tables` parameter to filter.

```json
{
  "tables": {
    "orders": {
      "schema": "public",
      "business_name": "Customer Orders",
      "description": "Records of customer purchases with order details and status",
      "domain": "sales",
      "row_count": 1234567,
      "column_count": 12,
      "synonyms": ["purchases", "transactions"],
      "columns": [
        {"name": "id", "type": "uuid", "role": "identifier", "is_primary_key": true},
        {"name": "customer_id", "type": "uuid", "role": "identifier", "entity": "user", "entity_role": "customer"},
        {"name": "status", "type": "varchar", "role": "dimension", "has_enum_values": true},
        {"name": "total_amount", "type": "decimal", "role": "measure"},
        {"name": "created_at", "type": "timestamptz", "role": "dimension"}
      ],
      "relationships": [
        {"column": "customer_id", "references": "users.id", "cardinality": "N:1"}
      ]
    }
  }
}
```

#### `depth: "columns"` (detailed, use with table filter)
Full column details including enum values and synonyms. Always use with `tables` filter.

```json
{
  "tables": {
    "orders": {
      "schema": "public",
      "business_name": "Customer Orders",
      "description": "Records of customer purchases with order details and status",
      "columns": [
        {
          "name": "id",
          "type": "uuid",
          "description": "Unique order identifier",
          "semantic_type": "identifier",
          "role": "identifier",
          "is_primary_key": true,
          "is_nullable": false,
          "synonyms": ["order_id", "order_number"]
        },
        {
          "name": "status",
          "type": "varchar",
          "description": "Current order status",
          "semantic_type": "category",
          "role": "dimension",
          "is_nullable": false,
          "synonyms": ["order_status", "state"],
          "enum_values": [
            {"value": "pending", "label": "Pending", "description": "Order placed, awaiting processing"},
            {"value": "confirmed", "label": "Confirmed", "description": "Order confirmed by seller"},
            {"value": "shipped", "label": "Shipped", "description": "Order has been shipped"},
            {"value": "delivered", "label": "Delivered", "description": "Order delivered to customer"},
            {"value": "cancelled", "label": "Cancelled", "description": "Order was cancelled"}
          ]
        },
        {
          "name": "total_amount",
          "type": "decimal(10,2)",
          "description": "Total order value including tax",
          "semantic_type": "currency",
          "role": "measure",
          "is_nullable": false,
          "synonyms": ["order_total", "amount", "revenue"]
        }
      ]
    }
  }
}
```

---

## Comparison: `get_schema` vs `get_ontology`

| Aspect | `get_schema` | `get_ontology` |
|--------|--------------|----------------|
| **Output format** | Plain text string | Structured JSON |
| **Depth control** | All or selected | 4 levels (domain → columns) |
| **Table filtering** | selected_only flag | Explicit table list |
| **Synonyms** | No | Yes |
| **Semantic types** | No | Yes (measure, dimension, etc.) |
| **Enum values** | No | Yes |
| **Domain summary** | No | Yes |
| **Entity occurrences** | Yes (inline) | Yes (structured) |
| **Relationships** | Yes (text) | Yes (structured JSON) |
| **Tool group** | Developer Tools | Business User Tools |

**Recommendation:** Keep both tools. `get_schema` is concise for quick context. `get_ontology` is rich for iterative exploration.

---

## Data Sources

| Output Field | Source Table/Column |
|--------------|---------------------|
| Domain description | `engine_ontologies.domain_summary->description` |
| Primary domains | `engine_ontologies.domain_summary->domains` |
| Entity list | `engine_ontology_entities` |
| Entity synonyms/aliases | `engine_ontology_entity_aliases` |
| Entity occurrences | `engine_ontology_entity_occurrences` |
| Table business names | `engine_schema_tables.business_name` OR `engine_ontologies.entity_summaries[table].business_name` |
| Table descriptions | `engine_ontologies.entity_summaries[table].description` |
| Table synonyms | `engine_ontologies.entity_summaries[table].synonyms` |
| Column details | `engine_ontologies.column_details[table][]` |
| Column synonyms | `engine_ontologies.column_details[table][].synonyms` |
| Enum values | `engine_ontologies.column_details[table][].enum_values` |
| Relationships | `engine_schema_relationships` |

---

## Implementation Plan

### [x] Step 1: Add Tool Group Visibility ✅ COMPLETE

Update `pkg/mcp/tools/developer.go` to expose ontology tools when `approved_queries` is enabled.

**Files:**
- `pkg/mcp/tools/developer.go` - Add `ontologyToolNames` map, update `filterTools()`
- `pkg/mcp/tools/developer_filter_test.go` - Update tests to include `get_ontology`

**Changes:**
```go
var ontologyToolNames = map[string]bool{
    "get_ontology": true,
}

// In filterTools(), add:
if ontologyToolNames[tool.Name] && !showApprovedQueries {
    continue
}
```

**Implementation Notes:**
- Added `ontologyToolNames` map at line 75-78 in developer.go
- Updated `filterTools()` function at line 194-197 to check ontology tools
- Updated all relevant tests in developer_filter_test.go to include `get_ontology`
- Added dedicated test `TestFilterTools_OntologyToolsFilteredWithApprovedQueries`
- All tests pass successfully

### [x] Step 2: Create Ontology Tool Dependencies ✅ COMPLETE

Create a new deps struct for ontology tools.

**Files:**
- `pkg/mcp/tools/ontology.go` (new file)

```go
type OntologyToolDeps struct {
    DB               *database.DB
    MCPConfigService services.MCPConfigService
    ProjectService   services.ProjectService
    OntologyRepo     repositories.OntologyRepository
    EntityRepo       repositories.OntologyEntityRepository
    SchemaRepo       repositories.SchemaRepository
    Logger           *zap.Logger
}
```

**Implementation Notes:**
- Created `pkg/mcp/tools/ontology.go` with `OntologyToolDeps` struct
- Struct follows same pattern as `DeveloperToolDeps` in developer.go
- All dependencies are interface types for testability
- Created comprehensive tests in `pkg/mcp/tools/ontology_test.go`
- Tests verify struct fields and initialization
- All tests pass successfully

### [x] Step 3: Implement `get_ontology` Tool Handler ✅ COMPLETE

Create the tool registration and handler logic.

**Files:**
- `pkg/mcp/tools/ontology.go`

**Handler logic:**
1. Parse `depth` parameter (required)
2. Parse optional `tables` filter
3. Parse optional `include_relationships` (default true)
4. Based on depth, call appropriate service method
5. Return structured JSON response

**Implementation Notes:**
- Created `RegisterOntologyTools()` function to register `get_ontology` tool
- Implemented full tool handler with depth parameter parsing and validation
- Added support for optional `tables` filter (array of strings)
- Added `include_relationships` parameter (default: true)
- Tool properly annotated as read-only, idempotent, non-destructive
- Comprehensive tests added in `ontology_test.go`:
  - Test tool registration
  - Test parameter validation (required depth)
  - Test depth value validation (domain/entities/tables/columns)
  - Test tables filter parsing
  - Test include_relationships parameter
- All tests pass successfully
- Handler returns placeholder "not implemented" response for now (will be implemented in Step 4)

### Step 4: Create Ontology Context Service

Service layer to assemble ontology responses from multiple repositories.

**Files:**
- `pkg/services/ontology_context.go` (new file)

**Interface:**
```go
type OntologyContextService interface {
    // GetDomainContext returns high-level domain information
    GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error)

    // GetEntitiesContext returns entity summaries with occurrences
    GetEntitiesContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyEntitiesContext, error)

    // GetTablesContext returns table summaries, optionally filtered
    GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error)

    // GetColumnsContext returns full column details for specified tables
    GetColumnsContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyColumnsContext, error)
}
```

### Step 5: Add Response Models

Define structured response types.

**Files:**
- `pkg/models/ontology_context.go` (new file)

```go
type OntologyDomainContext struct {
    Domain        DomainInfo       `json:"domain"`
    Entities      []EntityBrief    `json:"entities"`
    Relationships []RelationshipEdge `json:"relationships,omitempty"`
}

type OntologyEntitiesContext struct {
    Entities      map[string]EntityDetail `json:"entities"`
    Relationships []EntityRelationship    `json:"relationships,omitempty"`
}

type OntologyTablesContext struct {
    Tables map[string]TableSummary `json:"tables"`
}

type OntologyColumnsContext struct {
    Tables map[string]TableDetail `json:"tables"`
}
```

### Step 6: Register Tool in MCP Server

Wire up the tool registration.

**Files:**
- `pkg/mcp/server.go` - Call `RegisterOntologyTools()`
- `pkg/handlers/mcp_handler.go` - Add deps to handler setup

### Step 7: Handle Missing Ontology Gracefully

When ontology hasn't been extracted yet, return minimal response.

```json
{
  "error": null,
  "has_ontology": false,
  "message": "Ontology not yet extracted. Use get_schema for raw schema information.",
  "domain": null,
  "entities": []
}
```

---

## Example Client Workflow

```
1. Client connects to MCP server

2. Client calls: get_ontology(depth: "domain")
   → Learns: e-commerce domain, entities are user/order/product, key relationships

3. User asks: "Show me revenue by customer tier"

4. Client calls: get_ontology(depth: "entities")
   → Learns: "revenue" likely maps to order.total_amount (synonym)
   → Learns: "tier" is on users table

5. Client calls: get_ontology(depth: "columns", tables: ["orders", "users"])
   → Gets: full column details, confirms total_amount is a measure
   → Gets: tier enum values (premium, standard, basic)

6. Client generates SQL with confidence
```

---

## Testing Strategy

1. **Unit tests** for each depth level handler
2. **Integration tests** with test_data ontology
3. **Edge cases:**
   - No ontology extracted yet
   - Partial ontology (some tables analyzed, others not)
   - Empty tables filter
   - Invalid table names in filter
4. **Response size tests** - ensure depth levels stay within expected token ranges

---

## Open Questions

1. **Should `depth: "columns"` require a table filter?**
   - Pro: Prevents accidentally returning 600 columns
   - Con: Extra call if client wants everything
   - Recommendation: Require filter, max 10 tables per call

2. **Should we add `depth: "relationships"` as a separate level?**
   - Could return just the relationship graph without table/column details
   - Useful for join path planning
   - Defer to v2

3. **Caching strategy?**
   - Ontology data is relatively static (only changes on re-extraction)
   - Could cache responses per project/depth/filter combination
   - Defer to v2

---

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `pkg/mcp/tools/ontology.go` | Create | Tool registration and handler |
| `pkg/services/ontology_context.go` | Create | Service to assemble responses |
| `pkg/models/ontology_context.go` | Create | Response type definitions |
| `pkg/mcp/tools/developer.go` | Modify | Add ontology tool filtering |
| `pkg/mcp/server.go` | Modify | Register ontology tools |
| `pkg/handlers/mcp_handler.go` | Modify | Wire up dependencies |

---

## Success Criteria

- [ ] `get_ontology(depth: "domain")` returns domain context in <100ms
- [ ] `get_ontology(depth: "entities")` returns all entities with occurrences
- [ ] `get_ontology(depth: "tables", tables: [...])` returns filtered table summaries
- [ ] `get_ontology(depth: "columns", tables: [...])` returns full column details with enums
- [x] Tool appears in tool list when `approved_queries` is enabled (Step 1 complete)
- [x] Tool hidden when `approved_queries` is disabled (Step 1 complete)
- [ ] Graceful response when ontology not yet extracted
- [ ] Response sizes match expected token ranges per depth level

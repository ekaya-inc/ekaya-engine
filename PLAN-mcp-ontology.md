# PLAN: MCP Ontology Interface for SQL Generation

## Purpose

Design the MCP tools that enable an LLM (Claude Code or similar) to understand the database schema and ontology well enough to generate accurate SQL from natural language. This is the foundation for text-to-SQL capabilities.

**Design Philosophy:** Progressive disclosure. Don't dump the entire schema at once. Provide tiered access that allows the LLM to start with domain understanding, then drill down to specifics as needed.

---

## Current State

The existing MCP server provides:

| Tool | Purpose | Limitation |
|------|---------|------------|
| `schema` | Raw schema (tables, columns, types) | No business context, no semantics |
| `get_schema_context` | Schema with entity/role annotations | Prompt-formatted string, not structured for programmatic use |
| `list_approved_queries` | Pre-approved queries | Pre-written SQL only, not generative |

**Gap:** No tools for semantic understanding, entity resolution, join path discovery, or column meaning lookup.

---

## Proposed Tool Architecture

### Tier 0: Domain Context (Call Once Per Session)

```
Tool: ontology_context
Purpose: Provide high-level domain understanding before any specific query

Input: (none - uses default datasource)

Output:
{
  "domain": {
    "description": "E-commerce platform for B2B wholesale transactions...",
    "key_concepts": ["customer", "order", "product", "invoice", "shipment"],
    "common_query_patterns": [
      "aggregations by time period",
      "customer segmentation",
      "order fulfillment tracking"
    ]
  },
  "entities": [
    {
      "name": "user",
      "description": "Platform users including customers and internal staff",
      "primary_table": "users",
      "synonyms": ["customer", "account", "member"],
      "occurrence_count": 15  // appears in 15 columns across schema
    },
    {
      "name": "order",
      "description": "Customer purchase orders",
      "primary_table": "orders",
      "synonyms": ["purchase", "transaction"],
      "occurrence_count": 8
    }
  ],
  "relationship_graph": [
    {"from": "user", "to": "order", "type": "has_many"},
    {"from": "order", "to": "product", "type": "has_many_through", "via": "order_items"}
  ],
  "table_count": 45,
  "dialect": "postgres"
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
- open_world_hint: false
```

**When to use:** Call once at the start of a session or when switching projects. This gives the LLM the "mental model" of the database.

---

### Tier 1: Entity Resolution

```
Tool: resolve_entity
Purpose: Map business terms from user's question to actual tables/columns

Input:
{
  "term": "customers",           // Required: the word from user's query
  "context": "orders from customers in Q4"  // Optional: surrounding context for disambiguation
}

Output:
{
  "primary_match": {
    "entity_name": "user",
    "confidence": 0.95,
    "primary_table": "users",
    "description": "Platform users including customers and internal staff",
    "match_reason": "synonym_match"  // synonym_match, alias_match, entity_name_match
  },
  "occurrences": [
    {
      "table": "users",
      "column": "id",
      "role": null,
      "is_primary": true
    },
    {
      "table": "orders",
      "column": "customer_id",
      "role": "customer",
      "is_primary": false
    },
    {
      "table": "invoices",
      "column": "billed_to_user_id",
      "role": "billed_to",
      "is_primary": false
    }
  ],
  "alternative_entities": [
    {
      "entity_name": "account",
      "confidence": 0.3,
      "reason": "Sometimes 'customer' refers to account, not user"
    }
  ]
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**When to use:** When parsing natural language and encountering business terms that need mapping to technical names.

---

### Tier 2: Table Details

```
Tool: get_table_details
Purpose: Get detailed information about specific tables needed for a query

Input:
{
  "tables": ["orders", "users"],  // Required: 1-5 table names
  "include_sample_values": true   // Optional: include sample data (default: false)
}

Output:
{
  "tables": [
    {
      "name": "orders",
      "schema": "public",
      "business_name": "Customer Orders",
      "description": "Records of customer purchases with order details and status",
      "row_count": 1234567,
      "columns": [
        {
          "name": "id",
          "type": "uuid",
          "semantic_type": "identifier",
          "role": "primary_key",
          "description": "Unique order identifier",
          "nullable": false,
          "is_primary_key": true,
          "synonyms": ["order_id", "order_number"]
        },
        {
          "name": "customer_id",
          "type": "uuid",
          "semantic_type": "identifier",
          "role": "foreign_key",
          "description": "Reference to the customer who placed the order",
          "nullable": false,
          "entity": "user",
          "entity_role": "customer",
          "foreign_table": "users",
          "foreign_column": "id"
        },
        {
          "name": "status",
          "type": "varchar",
          "semantic_type": "category",
          "role": "dimension",
          "description": "Current order status",
          "nullable": false,
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
          "semantic_type": "currency",
          "role": "measure",
          "description": "Total order value including tax",
          "nullable": false,
          "synonyms": ["order_total", "amount", "revenue"]
        },
        {
          "name": "created_at",
          "type": "timestamptz",
          "semantic_type": "timestamp",
          "role": "dimension",
          "description": "When the order was placed",
          "nullable": false,
          "synonyms": ["order_date", "placed_at"]
        }
      ],
      "sample_values": {  // Only if include_sample_values: true
        "status": ["pending", "confirmed", "shipped", "delivered"],
        "total_amount": [150.00, 299.99, 1250.00]
      }
    }
  ],
  "relationships_between_tables": [
    {
      "from_table": "orders",
      "from_column": "customer_id",
      "to_table": "users",
      "to_column": "id",
      "cardinality": "N:1",
      "description": "Orders belong to users"
    }
  ]
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**When to use:** After resolving entities, get detailed information about the specific tables you'll query.

---

### Tier 3: Column Search

```
Tool: search_columns
Purpose: Find columns by semantic meaning, name pattern, or type

Input:
{
  "query": "revenue",              // Required: search term
  "semantic_type": "measure",      // Optional: filter by type (identifier, category, measure, temporal)
  "tables": ["orders", "invoices"] // Optional: limit to specific tables
}

Output:
{
  "matches": [
    {
      "table": "orders",
      "column": "total_amount",
      "score": 0.95,
      "match_reason": "synonym",
      "data_type": "decimal(10,2)",
      "semantic_type": "currency",
      "role": "measure",
      "description": "Total order value including tax",
      "synonyms": ["order_total", "amount", "revenue"]
    },
    {
      "table": "order_items",
      "column": "line_total",
      "score": 0.75,
      "match_reason": "description",
      "data_type": "decimal(10,2)",
      "semantic_type": "currency",
      "role": "measure",
      "description": "Total for this line item (qty * unit_price)"
    },
    {
      "table": "invoices",
      "column": "amount",
      "score": 0.70,
      "match_reason": "name",
      "data_type": "decimal(10,2)",
      "semantic_type": "currency",
      "role": "measure"
    }
  ]
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**When to use:** When the user mentions a concept (like "revenue", "date", "status") and you need to find which column represents it.

---

### Tier 4: Join Path Discovery

```
Tool: find_join_path
Purpose: Discover how to connect tables for a query

Input:
{
  "source_table": "orders",
  "target_table": "products",
  "max_hops": 3  // Optional: default 3
}

Output:
{
  "paths": [
    {
      "hops": [
        {
          "from_table": "orders",
          "from_column": "id",
          "to_table": "order_items",
          "to_column": "order_id",
          "join_type": "LEFT"
        },
        {
          "from_table": "order_items",
          "from_column": "product_id",
          "to_table": "products",
          "to_column": "id",
          "join_type": "INNER"
        }
      ],
      "total_hops": 2,
      "cardinality": "1:N:1",
      "description": "Orders → Order Items → Products (one order has many items, each item references one product)"
    }
  ],
  "recommended_path_index": 0,
  "warning": null  // Or: "Multiple paths exist, verify business intent"
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**When to use:** When a query involves multiple tables and you need to know how to JOIN them.

---

### Tier 5: Multi-Table Join Planning

```
Tool: plan_joins
Purpose: Given multiple tables, return the optimal join plan

Input:
{
  "tables": ["orders", "users", "products", "order_items"]
}

Output:
{
  "join_plan": {
    "base_table": "orders",
    "joins": [
      {
        "table": "users",
        "alias": "u",
        "join_type": "INNER",
        "on": "orders.customer_id = u.id",
        "order": 1
      },
      {
        "table": "order_items",
        "alias": "oi",
        "join_type": "LEFT",
        "on": "orders.id = oi.order_id",
        "order": 2
      },
      {
        "table": "products",
        "alias": "p",
        "join_type": "INNER",
        "on": "oi.product_id = p.id",
        "order": 3
      }
    ]
  },
  "sql_fragment": "FROM orders\nINNER JOIN users u ON orders.customer_id = u.id\nLEFT JOIN order_items oi ON orders.id = oi.order_id\nINNER JOIN products p ON oi.product_id = p.id",
  "warnings": [
    "LEFT JOIN to order_items may produce NULL values for orders without items"
  ]
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**When to use:** When you know which tables are needed and want to generate the FROM/JOIN clause.

---

### Tier 6: SQL Validation

```
Tool: validate_sql
Purpose: Check SQL syntax and semantics before execution

Input:
{
  "sql": "SELECT u.name, COUNT(o.id) FROM users u JOIN orders o ON o.customer_id = u.id GROUP BY u.name"
}

Output:
{
  "is_valid": true,
  "errors": [],
  "warnings": [
    {
      "type": "missing_where",
      "message": "Query has no WHERE clause and may return large result set",
      "severity": "info"
    }
  ],
  "tables_used": ["users", "orders"],
  "estimated_rows": 50000,  // From EXPLAIN if available
  "query_type": "SELECT"
}

// Error case:
{
  "is_valid": false,
  "errors": [
    {
      "type": "column_not_found",
      "message": "Column 'usres.name' not found - did you mean 'users.name'?",
      "position": 7,
      "suggestion": "users.name"
    }
  ],
  "warnings": []
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**When to use:** Before executing any generated SQL to catch errors early.

---

## Complete Workflow Example

User asks: "Show me the top 10 customers by total order value last quarter"

```
1. LLM calls: ontology_context
   → Learns domain is e-commerce, "customer" maps to "user" entity, orders exist

2. LLM calls: resolve_entity("customers")
   → Confirms user entity, primary table "users", also appears in orders.customer_id

3. LLM calls: resolve_entity("order value")
   → Matches to orders.total_amount as the measure

4. LLM calls: get_table_details(["users", "orders"])
   → Gets full column details, confirms created_at for date filtering

5. LLM calls: plan_joins(["users", "orders"])
   → Gets: FROM users u INNER JOIN orders o ON o.customer_id = u.id

6. LLM generates SQL:
   SELECT u.id, u.name, SUM(o.total_amount) as total_value
   FROM users u
   INNER JOIN orders o ON o.customer_id = u.id
   WHERE o.created_at >= '2024-10-01' AND o.created_at < '2025-01-01'
   GROUP BY u.id, u.name
   ORDER BY total_value DESC
   LIMIT 10

7. LLM calls: validate_sql(sql)
   → Confirms valid, no errors

8. LLM calls: query(sql)  [existing tool]
   → Executes and returns results
```

---

## Data Sources for Each Tool

| Tool | Primary Data Source |
|------|---------------------|
| `ontology_context` | `engine_ontologies.domain_summary`, `engine_ontology_entities` |
| `resolve_entity` | `engine_ontology_entities`, `engine_ontology_entity_occurrences`, `engine_ontology_entity_aliases` |
| `get_table_details` | `engine_schema_tables`, `engine_schema_columns`, `engine_ontologies.entity_summaries`, `engine_ontologies.column_details` |
| `search_columns` | `engine_schema_columns`, `engine_ontologies.column_details` (synonyms, descriptions) |
| `find_join_path` | `engine_schema_relationships`, `engine_entity_relationships` |
| `plan_joins` | `engine_schema_relationships`, `engine_entity_relationships` |
| `validate_sql` | Query executor's EXPLAIN + column/table existence checks |

---

## Implementation Notes

### Tool Registration

All ontology tools should be registered as a new tool group `ontology` that is:
- Enabled by default when approved_queries is enabled
- Enabled by default when developer tools is enabled
- Can be independently disabled if admin wants queries-only mode

### Error Handling

All tools should return structured errors that help the LLM self-correct:

```json
{
  "error": true,
  "error_type": "entity_not_found",
  "message": "No entity matching 'custmer' found",
  "suggestions": ["customer", "user"],
  "hint": "Did you mean 'customer'? Try resolve_entity('customer')"
}
```

### Caching Considerations

- `ontology_context` results can be cached per project/version
- Table details can be cached with invalidation on schema refresh
- Entity resolution should be fast (indexed lookups)

### Token Efficiency

Each tool response should be:
- Minimal but complete for the use case
- Avoid redundant information
- Use concise field names

---

## File Changes Required

| Area | Changes |
|------|---------|
| `pkg/mcp/tools/` | New `ontology.go` file with all tools |
| `pkg/services/` | New `ontology_context.go` for data assembly |
| `pkg/repositories/` | May need new queries for join path discovery |
| `pkg/models/mcp_config.go` | Add `ontology` tool group |

---

## Testing Strategy

1. **Unit tests** for each tool handler
2. **Integration tests** with real ontology data from test_data
3. **Workflow tests** simulating complete query generation flows
4. **Error case tests** for malformed inputs, missing data

---

## Open Questions

1. **Should `ontology_context` be automatically called on connection?**
   - Pro: LLM always has context
   - Con: Adds latency to first request

2. **Should we combine `find_join_path` and `plan_joins`?**
   - Could be a single tool with different modes

3. **How to handle multi-datasource projects?**
   - Currently uses default datasource
   - May need datasource_id parameter on all tools

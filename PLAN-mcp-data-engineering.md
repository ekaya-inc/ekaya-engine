# PLAN: MCP Data Engineering Tools

## Purpose

Provide a comprehensive toolkit for Data Engineers who need full database access via MCP. This persona knows what they're doing and needs power tools, not guardrails.

**Target Users:**
- Data Engineers building ETL pipelines
- DBAs managing schema changes
- Analytics Engineers creating transformations
- DevOps running migrations

**Design Philosophy:** Trust but verify. Enable powerful operations with comprehensive audit logging, not with friction.

---

## Capabilities Overview

```
┌─────────────────────────────────────────────────────────────────┐
│               Data Engineering MCP Tool Suite                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Schema Operations                                               │
│  ├── CREATE TABLE/VIEW/INDEX                                     │
│  ├── ALTER TABLE (add/drop columns, constraints)                 │
│  ├── DROP TABLE/VIEW (with safety checks)                        │
│  └── Schema diff and comparison                                  │
│                                                                  │
│  Migration Operations                                            │
│  ├── List pending migrations                                     │
│  ├── Run specific migration                                      │
│  ├── Rollback migration                                          │
│  └── Migration status and history                                │
│                                                                  │
│  Ontology Operations                                             │
│  ├── Export current ontology                                     │
│  ├── Import/install ontology                                     │
│  ├── Validate ontology against schema                            │
│  └── Refresh ontology from schema changes                        │
│                                                                  │
│  Schema Discovery                                                │
│  ├── Full schema discovery (all tables, not just selected)       │
│  ├── Table statistics and health                                 │
│  ├── Foreign key analysis                                        │
│  └── Index usage and recommendations                             │
│                                                                  │
│  Bulk Operations                                                 │
│  ├── COPY data import/export                                     │
│  ├── Bulk INSERT                                                 │
│  ├── Table TRUNCATE                                              │
│  └── Batch UPDATE/DELETE                                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Tool Group: data_engineering

### Enablement

This is a separate tool group with strict controls:

```go
type DataEngineeringConfig struct {
    Enabled              bool     // Must be explicitly enabled
    AllowedUsers         []string // Optional: restrict to specific users
    RequireConfirmation  bool     // Require confirm_destructive for risky ops
    AuditMode            string   // "full" (log SQL) or "summary" (log action only)
}
```

**Enable via MCP Server page:**

```
┌─────────────────────────────────────────────────────────────────┐
│ Data Engineering Tools                                          │
├─────────────────────────────────────────────────────────────────┤
│ ⚠️ These tools provide full database access for data engineers. │
│    Enable only for trusted technical users.                     │
│                                                                 │
│ ● Enable Data Engineering Tools                          [OFF] │
│                                                                 │
│ ○ Require confirmation for destructive operations        [ON]  │
│                                                                 │
│ ○ Restrict to specific users (comma-separated emails)          │
│   [                                                        ]    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Schema Operations

### Tool: schema_discover

**Purpose:** Full schema discovery without filtering

```
Input:
{
  "schema": "public",  // Optional: filter to schema
  "include_system": false  // Include pg_* tables
}

Output:
{
  "schemas": ["public", "staging", "archive"],
  "tables": [
    {
      "schema": "public",
      "name": "users",
      "type": "table",  // table, view, materialized_view
      "row_count": 50000,
      "size_bytes": 15728640,
      "is_selected": true,  // In admin-approved schema
      "columns": [...],
      "indexes": [
        {"name": "users_pkey", "columns": ["id"], "unique": true},
        {"name": "users_email_idx", "columns": ["email"], "unique": true}
      ],
      "constraints": [
        {"name": "users_pkey", "type": "primary_key", "columns": ["id"]},
        {"name": "users_email_key", "type": "unique", "columns": ["email"]}
      ]
    }
  ],
  "foreign_keys": [
    {
      "name": "orders_customer_fkey",
      "source_table": "orders",
      "source_columns": ["customer_id"],
      "target_table": "users",
      "target_columns": ["id"],
      "on_delete": "CASCADE",
      "on_update": "NO ACTION"
    }
  ]
}
```

---

### Tool: schema_diff

**Purpose:** Compare current schema to a reference

```
Input:
{
  "reference": "file",  // "file" or "snapshot"
  "reference_id": "schema_v1.sql",  // File name or snapshot ID
  "schemas": ["public"]  // Optional filter
}

Output:
{
  "changes": [
    {
      "type": "table_added",
      "object": "public.new_table",
      "details": "Table exists in current but not in reference"
    },
    {
      "type": "column_added",
      "object": "public.users.phone",
      "details": "Column added: phone VARCHAR(20)"
    },
    {
      "type": "column_modified",
      "object": "public.users.email",
      "details": "Type changed: VARCHAR(100) → VARCHAR(255)"
    },
    {
      "type": "index_dropped",
      "object": "public.users_name_idx",
      "details": "Index no longer exists"
    }
  ],
  "summary": {
    "tables_added": 1,
    "tables_dropped": 0,
    "columns_added": 2,
    "columns_modified": 1,
    "indexes_changed": 1
  }
}
```

---

### Tool: create_table

**Purpose:** Create a new table with validation

```
Input:
{
  "schema": "staging",
  "name": "temp_import",
  "columns": [
    {"name": "id", "type": "UUID", "primary_key": true, "default": "gen_random_uuid()"},
    {"name": "data", "type": "JSONB", "nullable": true},
    {"name": "created_at", "type": "TIMESTAMPTZ", "default": "NOW()"}
  ],
  "if_not_exists": true
}

Output:
{
  "created": true,
  "table": "staging.temp_import",
  "sql_executed": "CREATE TABLE IF NOT EXISTS staging.temp_import (...)"
}
```

---

### Tool: alter_table

**Purpose:** Modify table structure

```
Input:
{
  "table": "public.users",
  "operations": [
    {"action": "add_column", "column": "phone", "type": "VARCHAR(20)"},
    {"action": "drop_column", "column": "legacy_id"},
    {"action": "rename_column", "old_name": "name", "new_name": "full_name"},
    {"action": "add_index", "columns": ["email", "created_at"], "name": "users_email_created_idx"},
    {"action": "add_constraint", "type": "unique", "columns": ["phone"], "name": "users_phone_key"}
  ],
  "confirm_destructive": true  // Required for drop operations
}

Output:
{
  "executed": true,
  "operations_completed": 5,
  "sql_executed": [
    "ALTER TABLE public.users ADD COLUMN phone VARCHAR(20)",
    "ALTER TABLE public.users DROP COLUMN legacy_id",
    ...
  ]
}
```

---

### Tool: drop_table

**Purpose:** Drop tables with safety checks

```
Input:
{
  "table": "staging.temp_import",
  "cascade": false,
  "confirm_destructive": true  // Required
}

Output:
{
  "dropped": true,
  "table": "staging.temp_import",
  "cascade_dropped": [],  // Empty unless cascade: true
  "warning": null
}

// If table has dependents and cascade: false
Output:
{
  "dropped": false,
  "error": "Table has dependent objects",
  "dependents": [
    {"type": "view", "name": "reports.user_summary"},
    {"type": "foreign_key", "name": "orders_temp_fkey"}
  ],
  "hint": "Use cascade: true to drop dependents, or drop them first"
}
```

---

## Migration Operations

### Tool: migrations_status

**Purpose:** Show migration status

```
Input: {}

Output:
{
  "current_version": "20241215_001",
  "migrations": [
    {
      "version": "20241201_001",
      "name": "add_phone_to_users",
      "applied_at": "2024-12-01T10:00:00Z",
      "status": "applied"
    },
    {
      "version": "20241215_001",
      "name": "create_audit_tables",
      "applied_at": "2024-12-15T10:00:00Z",
      "status": "applied"
    },
    {
      "version": "20241220_001",
      "name": "add_indexes_for_search",
      "status": "pending",
      "up_sql_preview": "CREATE INDEX CONCURRENTLY..."
    }
  ],
  "pending_count": 1
}
```

---

### Tool: migrations_run

**Purpose:** Apply pending migrations

```
Input:
{
  "target_version": "20241220_001",  // Optional: stop at this version
  "dry_run": false,
  "confirm": true
}

Output:
{
  "applied": [
    {
      "version": "20241220_001",
      "name": "add_indexes_for_search",
      "duration_ms": 5432,
      "sql_executed": "CREATE INDEX CONCURRENTLY..."
    }
  ],
  "new_version": "20241220_001"
}

// Dry run output
{
  "dry_run": true,
  "would_apply": [
    {
      "version": "20241220_001",
      "name": "add_indexes_for_search",
      "up_sql": "CREATE INDEX CONCURRENTLY idx_users_search ON users USING gin(name gin_trgm_ops)"
    }
  ]
}
```

---

### Tool: migrations_rollback

**Purpose:** Revert applied migrations

```
Input:
{
  "target_version": "20241215_001",  // Roll back to this version
  "confirm_destructive": true
}

Output:
{
  "rolled_back": [
    {
      "version": "20241220_001",
      "name": "add_indexes_for_search",
      "down_sql_executed": "DROP INDEX CONCURRENTLY idx_users_search"
    }
  ],
  "new_version": "20241215_001"
}
```

---

## Ontology Operations

### Tool: ontology_export

**Purpose:** Export current ontology as JSON

```
Input:
{
  "format": "json",  // "json" or "yaml"
  "include_workflow_state": false
}

Output:
{
  "ontology": {
    "version": 3,
    "domain_summary": {...},
    "entity_summaries": {...},
    "column_details": {...}
  },
  "entities": [
    {"name": "user", "primary_table": "users", "occurrences": [...]},
    ...
  ],
  "relationships": [...],
  "exported_at": "2024-12-15T10:00:00Z"
}
```

---

### Tool: ontology_import

**Purpose:** Import/install ontology definitions

```
Input:
{
  "ontology": {
    "domain_summary": {...},
    "entity_summaries": {...},
    "column_details": {...}
  },
  "merge_strategy": "replace",  // "replace", "merge", "validate_only"
  "backup_current": true
}

Output:
{
  "imported": true,
  "backup_id": "ontology_backup_20241215_100000",
  "changes": {
    "entities_added": 2,
    "entities_updated": 5,
    "columns_enriched": 45
  }
}
```

---

### Tool: ontology_validate

**Purpose:** Check ontology against current schema

```
Input: {}

Output:
{
  "valid": false,
  "issues": [
    {
      "severity": "error",
      "type": "missing_table",
      "message": "Entity 'product' references table 'products' which doesn't exist"
    },
    {
      "severity": "warning",
      "type": "column_mismatch",
      "message": "Column 'users.status' has enum values but column type is TEXT not enum"
    },
    {
      "severity": "info",
      "type": "unmatched_table",
      "message": "Table 'audit_logs' exists in schema but has no entity mapping"
    }
  ],
  "summary": {
    "errors": 1,
    "warnings": 3,
    "info": 5
  }
}
```

---

### Tool: ontology_refresh

**Purpose:** Trigger ontology refresh from schema changes

```
Input:
{
  "scope": "incremental",  // "full" or "incremental"
  "tables": ["new_table"]  // Optional: limit to specific tables
}

Output:
{
  "workflow_id": "uuid",
  "status": "started",
  "message": "Ontology refresh started. Use workflow status to monitor progress."
}
```

---

## Bulk Operations

### Tool: bulk_insert

**Purpose:** Efficient bulk data loading

```
Input:
{
  "table": "staging.imports",
  "columns": ["id", "name", "value"],
  "data": [
    ["uuid1", "Item 1", 100],
    ["uuid2", "Item 2", 200],
    ...
  ],
  "on_conflict": "ignore"  // "error", "ignore", "update"
}

Output:
{
  "inserted": 950,
  "ignored": 50,
  "execution_time_ms": 1234
}
```

---

### Tool: bulk_copy

**Purpose:** High-performance COPY operations

```
Input:
{
  "direction": "export",  // "export" or "import"
  "table": "public.orders",
  "format": "csv",
  "options": {
    "header": true,
    "delimiter": ","
  },
  // For export:
  "query": "SELECT * FROM orders WHERE created_at > '2024-01-01'",
  // For import:
  "data": "id,name,value\nuuid1,Item1,100\n..."
}

Output (export):
{
  "row_count": 10000,
  "data": "id,name,value\n...",  // or truncated with download link
  "size_bytes": 1048576,
  "truncated": false
}

Output (import):
{
  "rows_imported": 10000,
  "execution_time_ms": 5432
}
```

---

## Raw SQL Access

### Tool: raw_query

**Purpose:** Execute any SQL without restrictions

```
Input:
{
  "sql": "SELECT pg_size_pretty(pg_total_relation_size('users'))",
  "timeout_seconds": 60
}

Output:
{
  "columns": ["pg_size_pretty"],
  "rows": [{"pg_size_pretty": "15 MB"}],
  "row_count": 1,
  "execution_time_ms": 12
}
```

**No restrictions on SQL type** - the user is trusted.

---

### Tool: raw_execute

**Purpose:** Execute any DDL/DML without restrictions

```
Input:
{
  "sql": "VACUUM ANALYZE users",
  "timeout_seconds": 300,
  "confirm_destructive": true  // Still required for safety
}

Output:
{
  "executed": true,
  "message": "VACUUM",
  "execution_time_ms": 45000
}
```

---

## Database Admin Operations

### Tool: table_stats

**Purpose:** Get detailed table statistics

```
Input:
{
  "table": "public.orders"
}

Output:
{
  "table": "public.orders",
  "row_count": 1000000,
  "dead_tuples": 15000,
  "size": {
    "total": "256 MB",
    "table": "200 MB",
    "indexes": "56 MB"
  },
  "last_vacuum": "2024-12-14T10:00:00Z",
  "last_analyze": "2024-12-14T10:00:00Z",
  "seq_scans": 145,
  "idx_scans": 50000,
  "hot_update_ratio": 0.85,
  "indexes": [
    {
      "name": "orders_pkey",
      "size": "20 MB",
      "scans": 45000,
      "rows_fetched": 45000
    },
    {
      "name": "orders_customer_idx",
      "size": "18 MB",
      "scans": 5000,
      "rows_fetched": 25000
    }
  ],
  "recommendations": [
    "Consider VACUUM to reclaim 15000 dead tuples",
    "Index orders_created_idx has low usage (50 scans) - consider dropping"
  ]
}
```

---

### Tool: index_recommendations

**Purpose:** Suggest index improvements

```
Input:
{
  "table": "public.orders",  // Optional: all tables if not specified
  "analyze_queries": true  // Check pg_stat_statements if available
}

Output:
{
  "missing_indexes": [
    {
      "table": "orders",
      "columns": ["status", "created_at"],
      "reason": "Sequential scan on 1M rows with WHERE status = X AND created_at > Y",
      "suggested_sql": "CREATE INDEX CONCURRENTLY idx_orders_status_created ON orders(status, created_at)",
      "estimated_improvement": "10x faster for common queries"
    }
  ],
  "unused_indexes": [
    {
      "name": "orders_legacy_idx",
      "table": "orders",
      "size": "50 MB",
      "last_used": null,
      "suggestion": "DROP INDEX orders_legacy_idx"
    }
  ],
  "duplicate_indexes": [
    {
      "indexes": ["orders_customer_id_idx", "orders_customer_fkey"],
      "table": "orders",
      "suggestion": "Foreign key constraint already creates implicit index"
    }
  ]
}
```

---

## Tool Registration

All data engineering tools are registered under a single group:

```go
const dataEngineeringToolGroup = "data_engineering"

var dataEngineeringToolNames = map[string]bool{
    "schema_discover":        true,
    "schema_diff":            true,
    "create_table":           true,
    "alter_table":            true,
    "drop_table":             true,
    "migrations_status":      true,
    "migrations_run":         true,
    "migrations_rollback":    true,
    "ontology_export":        true,
    "ontology_import":        true,
    "ontology_validate":      true,
    "ontology_refresh":       true,
    "bulk_insert":            true,
    "bulk_copy":              true,
    "raw_query":              true,
    "raw_execute":            true,
    "table_stats":            true,
    "index_recommendations":  true,
}
```

---

## Audit Integration

All data engineering operations are logged with:
- Full SQL (no sanitization - these are trusted users)
- Execution time
- Rows affected
- Success/failure
- User identity

Special event types:
- `de_schema_change` - DDL operations
- `de_migration` - Migration runs
- `de_ontology_change` - Ontology modifications
- `de_bulk_operation` - Bulk data operations

---

## File Changes Summary

| Area | Changes |
|------|---------|
| `pkg/mcp/tools/` | New `data_engineering.go` with all tools |
| `pkg/services/` | Extend schema service for discovery, add migration service |
| `pkg/models/mcp_config.go` | Add data_engineering tool group |
| `ui/src/pages/MCPServerPage.tsx` | Add Data Engineering section |
| `migrations/` | If new tables needed for migration tracking |

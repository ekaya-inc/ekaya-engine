# PLAN: Ontology Probe Tools

**Author:** Claude (as MCP Client)
**Date:** 2025-01-08
**Status:** Design Proposal
**Related:** PLAN-claudes-wish-list.md

---

## Problem Statement

During ontology extraction, the system collects extensive data about the schema:
- Column statistics (distinct counts, null rates, cardinality)
- Sample values for enum detection
- Join analysis (match rates, orphan counts)
- Joinability classification

Much of this data is:
1. **Persisted but not exposed** via MCP tools
2. **Collected but discarded** after extraction

As an MCP client, when I need to understand "what values can `status` have?" or "is this FK relationship valid?", I currently have to write SQL queries—even though the system already computed this during extraction.

**Goal:** Surface pre-computed extraction data so I never have to query the database to learn what's already been learned.

---

## Current State: What's Collected vs Persisted vs Exposed

### Column Statistics

| Data Point | Collected | Persisted | Exposed via MCP |
|------------|-----------|-----------|-----------------|
| `distinct_count` | ✅ `AnalyzeColumnStats()` | ✅ `engine_schema_columns` | ❌ |
| `row_count` | ✅ | ✅ `engine_schema_columns` | ❌ |
| `non_null_count` | ✅ | ✅ `engine_schema_columns` | ❌ |
| `null_rate` | ✅ (computed) | ❌ (derivable) | ❌ |
| `cardinality_ratio` | ✅ (computed) | ❌ (derivable) | ❌ |
| `min_length`, `max_length` | ✅ | ✅ `engine_schema_columns` | ❌ |
| `is_joinable` | ✅ | ✅ `engine_schema_columns` | ❌ |
| `joinability_reason` | ✅ | ✅ `engine_schema_columns` | ❌ |

### Sample Values

| Data Point | Collected | Persisted | Exposed via MCP |
|------------|-----------|-----------|-----------------|
| Raw distinct values (up to 50) | ✅ `GetDistinctValues()` | ❌ **DISCARDED** | ❌ |
| LLM-refined enum labels | ✅ | ✅ `engine_ontologies.column_details` | ✅ (via `get_ontology`) |

**Issue:** Raw sample values are collected during column enrichment, passed to LLM for label generation, then thrown away. The refined labels are kept, but the original values are lost.

### Relationship Analysis

| Data Point | Collected | Persisted | Exposed via MCP |
|------------|-----------|-----------|-----------------|
| `match_rate` | ✅ `CheckValueOverlap()` | ✅ `engine_schema_relationships` | ❌ |
| `source_distinct` | ✅ | ✅ `engine_schema_relationships` | ❌ |
| `target_distinct` | ✅ | ✅ `engine_schema_relationships` | ❌ |
| `matched_count` | ✅ | ✅ `engine_schema_relationships` | ❌ |
| `orphan_count` | ✅ `AnalyzeJoin()` | ❌ **DISCARDED** | ❌ |
| `join_count` | ✅ | ❌ **DISCARDED** | ❌ |
| `cardinality_type` (1:1, 1:N, N:M) | ❌ Not computed | ❌ | ❌ |
| `rejection_reason` | ✅ | ✅ `engine_schema_relationships` | ❌ |

---

## Proposed Solution

### Part 1: MCP Tool Interface

#### 1.1 Enhance `get_context` with Optional Statistics

Add an `include` parameter to `get_context`:

```
get_context(
  depth: "domain" | "entities" | "tables" | "columns",
  tables?: string[],
  include_relationships?: boolean,
  include?: string[]  // NEW: ["statistics", "sample_values"]
)
```

**When `include` contains `"statistics"`:**

At `depth=tables`:
```json
{
  "tables": [
    {
      "name": "users",
      "entity": "User",
      "row_count": 1000,
      "column_count": 15,
      "statistics": {
        "null_heavy_columns": ["middle_name", "phone"],
        "low_cardinality_columns": ["status", "role"],
        "high_cardinality_columns": ["email", "user_id"]
      }
    }
  ]
}
```

At `depth=columns`:
```json
{
  "columns": [
    {
      "name": "status",
      "type": "VARCHAR",
      "statistics": {
        "distinct_count": 5,
        "row_count": 1000,
        "non_null_count": 980,
        "null_rate": 0.02,
        "cardinality_ratio": 0.005,
        "is_joinable": false,
        "joinability_reason": "low_cardinality"
      }
    }
  ]
}
```

**When `include` contains `"sample_values"`:**

Only returned for columns with `distinct_count <= 50`:
```json
{
  "columns": [
    {
      "name": "status",
      "sample_values": ["ACTIVE", "SUSPENDED", "BANNED", "PENDING", "DELETED"]
    }
  ]
}
```

#### 1.2 Add `probe_column` Tool

Deep-dive into specific columns without bloating the main context response.

```
probe_column(
  table: string,
  column: string
) -> {
  "table": "users",
  "column": "status",

  "statistics": {
    "distinct_count": 5,
    "row_count": 1000,
    "non_null_count": 980,
    "null_rate": 0.02,
    "cardinality_ratio": 0.005,
    "min_length": 6,
    "max_length": 9
  },

  "joinability": {
    "is_joinable": false,
    "reason": "low_cardinality",
    "explanation": "Only 5 distinct values across 1000 rows (0.5% cardinality)"
  },

  "sample_values": ["ACTIVE", "SUSPENDED", "BANNED", "PENDING", "DELETED"],

  "semantic": {
    "entity": "User",
    "role": "status",
    "description": "Account state indicating user's current status on the platform",
    "enum_labels": {
      "ACTIVE": "Normal active account",
      "SUSPENDED": "Temporarily disabled by admin",
      "BANNED": "Permanently banned",
      "PENDING": "Awaiting email verification",
      "DELETED": "Soft-deleted account"
    }
  }
}
```

**Batch variant:**

```
probe_columns(
  columns: [{table: "users", column: "status"}, {table: "users", column: "role"}]
) -> {
  "results": {
    "users.status": { ... },
    "users.role": { ... }
  }
}
```

#### 1.3 Add `probe_relationship` Tool

Deep-dive into relationships between entities or tables.

```
probe_relationship(
  from_entity?: string,  // Filter by source entity
  to_entity?: string,    // Filter by target entity
  from_table?: string,   // Or filter by source table
  to_table?: string      // Or filter by target table
) -> {
  "relationships": [
    {
      "from_entity": "Account",
      "to_entity": "User",
      "from_column": "accounts.default_user_id",
      "to_column": "users.user_id",
      "relationship_type": "fk",

      "cardinality": "N:1",
      "cardinality_explanation": "Many accounts can reference the same user",

      "data_quality": {
        "match_rate": 0.98,
        "source_distinct": 500,
        "target_distinct": 450,
        "matched_count": 490,
        "orphan_count": 10,
        "orphan_explanation": "10 accounts reference non-existent users"
      },

      "semantic": {
        "label": "default_user",
        "description": "The default user profile associated with this account",
        "association": "has default"
      }
    }
  ],

  "rejected_candidates": [
    {
      "from_column": "accounts.created_by",
      "to_column": "users.user_id",
      "rejection_reason": "low_match_rate",
      "match_rate": 0.12,
      "explanation": "Only 12% of values match, likely not a true FK"
    }
  ]
}
```

### Part 2: Persistence Changes

#### 2.1 Store Sample Values

**Current:** `GetDistinctValues()` in `pkg/adapters/datasource/postgres/schema.go` returns values that are passed to LLM then discarded.

**Change:** Persist in `engine_schema_columns.sample_values`

```sql
ALTER TABLE engine_schema_columns
ADD COLUMN IF NOT EXISTS sample_values JSONB;
-- Stores: ["ACTIVE", "SUSPENDED", "BANNED", ...] (up to 50 values)
```

**Implementation:**

```go
// In column_enrichment.go, after getting distinct values:
column.SampleValues = distinctValues // Store before passing to LLM
// Then persist to DB
```

#### 2.2 Store Join Analysis Details

**Current:** `AnalyzeJoin()` computes orphan counts and join counts, uses them for validation, then discards.

**Change:** Persist in `engine_schema_relationships`

```sql
ALTER TABLE engine_schema_relationships
ADD COLUMN IF NOT EXISTS orphan_count INTEGER,
ADD COLUMN IF NOT EXISTS join_count INTEGER,
ADD COLUMN IF NOT EXISTS source_max_value TEXT;
```

**Implementation:**

```go
// In deterministic_relationship_service.go, after AnalyzeJoin():
relationship.OrphanCount = analysisResult.OrphanCount
relationship.JoinCount = analysisResult.JoinCount
// Persist to DB
```

#### 2.3 Compute and Store Cardinality Type

**Current:** Not computed or stored.

**Change:** Derive cardinality from match analysis and store.

```sql
ALTER TABLE engine_entity_relationships
ADD COLUMN IF NOT EXISTS cardinality VARCHAR(10);
-- Values: "1:1", "1:N", "N:1", "N:M"
```

**Logic for derivation:**

```go
func deriveCardinality(sourceDistinct, targetDistinct, matchedCount int) string {
    sourceRatio := float64(matchedCount) / float64(sourceDistinct)
    targetRatio := float64(matchedCount) / float64(targetDistinct)

    // If each source maps to ~1 target and each target has ~1 source
    if sourceRatio > 0.9 && targetRatio > 0.9 {
        return "1:1"
    }
    // If each source maps to ~1 target but targets have many sources
    if sourceRatio > 0.9 && targetRatio < 0.5 {
        return "N:1"
    }
    // If sources map to many targets but each target has ~1 source
    if sourceRatio < 0.5 && targetRatio > 0.9 {
        return "1:N"
    }
    // Otherwise it's many-to-many
    return "N:M"
}
```

#### 2.4 Enhance Rejection Reason Storage

**Current:** `rejection_reason` is a string like "low_match_rate".

**Change:** Store full context as JSONB.

```sql
ALTER TABLE engine_schema_relationships
ALTER COLUMN rejection_reason TYPE JSONB USING jsonb_build_object('reason', rejection_reason);
-- New structure: {"reason": "low_match_rate", "match_rate": 0.12, "threshold": 0.5}
```

---

## Part 3: Implementation Plan

### Phase 1: Expose Already-Persisted Data (No DB Changes)

1. **Enhance `get_context`** to include statistics from `engine_schema_columns`:
   - Add `include` parameter
   - Query and return `distinct_count`, `row_count`, `is_joinable`, etc.

2. **Implement `probe_column`** tool:
   - Query `engine_schema_columns` for statistics
   - Query `engine_ontologies.column_details` for semantic info
   - Combine and return

3. **Implement `probe_relationship`** tool:
   - Query `engine_schema_relationships` for match metrics
   - Query `engine_entity_relationships` for semantic info
   - Combine and return

**Files to modify:**
- `pkg/mcp/tools/context.go` (new or extend `ontology.go`)
- `pkg/mcp/tools/probe.go` (new file)
- `pkg/services/mcp_tools_registry.go` (register new tools)

### Phase 2: Persist Currently-Discarded Data (DB Changes)

1. **Migration:** Add columns to `engine_schema_columns` and `engine_schema_relationships`

2. **Modify column enrichment** (`pkg/services/column_enrichment.go`):
   - Store `sample_values` before passing to LLM

3. **Modify relationship discovery** (`pkg/services/deterministic_relationship_service.go`):
   - Store `orphan_count`, `join_count` from `AnalyzeJoin()`
   - Compute and store `cardinality` type

4. **Re-extract existing projects** or backfill data

**Files to modify:**
- `migrations/XXX_probe_data.up.sql` (new migration)
- `pkg/services/column_enrichment.go`
- `pkg/services/deterministic_relationship_service.go`
- `pkg/models/schema.go` (add fields to models)

### Phase 3: Tool Access Control

Add probe tools to developer tools list (not business user tools):
- `probe_column`
- `probe_columns` (batch)
- `probe_relationship`

**File to modify:**
- `pkg/services/tool_access.go`

---

## Part 4: Usage Examples

### Example 1: Understanding an Enum Column

**Before (requires query):**
```
Me: What values can users.status have?
Action: query("SELECT DISTINCT status FROM users")
```

**After (no query needed):**
```
Me: What values can users.status have?
Action: probe_column(table: "users", column: "status")
Response: {
  "sample_values": ["ACTIVE", "SUSPENDED", "BANNED", "PENDING", "DELETED"],
  "semantic": {
    "enum_labels": {
      "ACTIVE": "Normal active account",
      ...
    }
  }
}
```

### Example 2: Checking Relationship Quality

**Before (no visibility):**
```
Me: Is the FK from billing_transactions.payee_user_id valid?
Action: query("SELECT COUNT(*) FROM billing_transactions bt LEFT JOIN users u ON bt.payee_user_id = u.user_id WHERE u.user_id IS NULL")
```

**After (pre-computed):**
```
Me: Is the FK from billing_transactions.payee_user_id valid?
Action: probe_relationship(from_table: "billing_transactions", to_table: "users")
Response: {
  "relationships": [{
    "data_quality": {
      "match_rate": 0.98,
      "orphan_count": 23,
      "orphan_explanation": "23 transactions reference non-existent users"
    }
  }]
}
```

### Example 3: Understanding Cardinality Before Writing JOINs

**Before (guess or query):**
```
Me: Will this JOIN fan out?
Action: query("SELECT payee_user_id, COUNT(*) FROM billing_transactions GROUP BY 1 HAVING COUNT(*) > 1 LIMIT 5")
```

**After (pre-computed):**
```
Me: What's the cardinality of User -> Billing Transaction?
Action: probe_relationship(from_entity: "User", to_entity: "Billing Transaction")
Response: {
  "cardinality": "1:N",
  "cardinality_explanation": "One user can have many transactions"
}
Me: Got it, I'll need to aggregate or use DISTINCT.
```

---

## Part 5: Response Size Considerations

### Token Efficiency

| Tool | Typical Response Size | When to Use |
|------|----------------------|-------------|
| `get_context(depth=entities)` | ~2k tokens | Initial exploration |
| `get_context(depth=tables, include=["statistics"])` | ~4k tokens | Before writing queries |
| `probe_column(table, column)` | ~200 tokens | Specific column deep-dive |
| `probe_columns([...5 columns...])` | ~1k tokens | Multiple column analysis |
| `probe_relationship(...)` | ~500 tokens | Relationship verification |

### Guidelines for Me (Claude)

1. **Don't request `include=["statistics", "sample_values"]` by default** - Only when I need them
2. **Use `probe_column` for specific questions** - "What values can status have?"
3. **Use `probe_relationship` before complex JOINs** - Verify cardinality and data quality
4. **Batch with `probe_columns`** when investigating multiple columns

---

## Part 6: Database Schema Changes Summary

```sql
-- Migration: XXX_probe_data.up.sql

-- 1. Store sample values for enum-like columns
ALTER TABLE engine_schema_columns
ADD COLUMN IF NOT EXISTS sample_values JSONB;

-- 2. Store join analysis details
ALTER TABLE engine_schema_relationships
ADD COLUMN IF NOT EXISTS orphan_count INTEGER,
ADD COLUMN IF NOT EXISTS join_count INTEGER;

-- 3. Store cardinality type
ALTER TABLE engine_entity_relationships
ADD COLUMN IF NOT EXISTS cardinality VARCHAR(10);

-- 4. Enhance rejection reason with full context
-- (Only if not already JSONB)
-- ALTER TABLE engine_schema_relationships
-- ALTER COLUMN rejection_reason TYPE JSONB;

-- Index for efficient probing
CREATE INDEX IF NOT EXISTS idx_schema_columns_stats
ON engine_schema_columns(project_id, table_name, column_name)
WHERE sample_values IS NOT NULL;
```

---

## Success Criteria

After implementation:

1. **Zero queries for enum discovery** - `probe_column` returns sample values
2. **Zero queries for cardinality checks** - `probe_relationship` returns cardinality
3. **Zero queries for data quality assessment** - Match rates and orphan counts available
4. **No additional extraction time** - Data already collected, just persisted
5. **Minimal token overhead** - Progressive disclosure, not dump

---

## Closing Thought

The ontology extraction process already does the hard work of scanning, sampling, and analyzing the database. This plan ensures that work isn't wasted—every insight collected during extraction becomes available for future queries without hitting the database again.

*"Query the ontology, not the database."*

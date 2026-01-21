# PLAN: Living Ontology System (Master Plan)

## Overview

Transform Ekaya Engine's ontology from a one-time extraction to a "living" system that stays current with schema and data changes. Support two modes of ontology building with clear precedence rules.

## Operating Modes

### Dev Tools Mode (Claude as Builder)
- Claude creates tables via `execute()`, syncs via `refresh_schema()`
- Claude builds ontology via `update_entity()`, `update_relationship()`, `update_column()`
- No AI Config required - Claude IS the semantic layer builder
- Use case: New databases, CSV imports, rapid prototyping

### AI Config Mode (Engine as Builder)
- Admin attaches AI Config to project
- Engine detects changes and runs targeted LLM enrichment
- Incremental updates instead of full re-extraction
- Use case: Existing databases, enterprise deployments

### Mixed Mode
- Both modes coexist on same project
- Precedence: **Admin (UI) > Claude (MCP) > Engine (inference)**
- Provenance tracked on every ontology element

## Sub-Plans

| Plan | Name | Description | Depends On | Status |
|------|------|-------------|------------|--------|
| [PLAN-living-ontology-01](./PLAN-living-ontology-01-refresh-schema.md) | refresh_schema MCP Tool | Sync DDL from datasource via MCP | - | ✅ Done |
| [PLAN-living-ontology-02](./PLAN-living-ontology-02-empty-ontology.md) | Empty Ontology on Project Creation | Enable immediate MCP tool use | - | ✅ Done |
| [PLAN-living-ontology-03](./PLAN-living-ontology-03-schema-change-detection.md) | Schema Change Detection | Detect DDL changes, queue for review | 01 | ✅ Done |
| [PLAN-living-ontology-04](./PLAN-living-ontology-04-data-change-detection.md) | Data Change Detection | Detect data-level changes (enums, cardinality) | 03 | ✅ Done |
| [PLAN-living-ontology-05](./PLAN-living-ontology-05-change-queue.md) | Ontology Change Queue & Precedence | Review/approve workflow, precedence model | 03, 04 | ✅ Done |
| [PLAN-living-ontology-06](./PLAN-living-ontology-06-incremental-dag.md) | Incremental DAG Execution | Targeted LLM enrichment for changes | 05 | |

## Dependency Graph

```
┌─────────┐     ┌─────────┐
│ PLAN-01 │     │ PLAN-02 │
│ refresh │     │  empty  │
│ _schema │     │ontology │
└────┬────┘     └─────────┘
     │
     ▼
┌─────────┐
│ PLAN-03 │
│ schema  │
│ detect  │
└────┬────┘
     │
     ├──────────────┐
     ▼              ▼
┌─────────┐   ┌─────────┐
│ PLAN-04 │   │ PLAN-05 │
│  data   │──►│  queue  │
│ detect  │   │ & prec  │
└─────────┘   └────┬────┘
                   │
                   ▼
              ┌─────────┐
              │ PLAN-06 │
              │  incr   │
              │   DAG   │
              └─────────┘
```

## Implementation Phases

### Phase A: Enable Dev Tools Mode (PLAN-01 + PLAN-02)
**Goal:** Claude can create tables and build ontology immediately

- `refresh_schema()` MCP tool syncs DDL
- Empty ontology created on project provision
- All ontology MCP tools work without extraction

**Milestone:** Claude Code can run full workflow:
```
execute(DDL) → refresh_schema() → update_entity() → query()
```

### Phase B: Change Detection (PLAN-03 + PLAN-04)
**Goal:** System detects what changed since last refresh

- DDL changes: new/dropped tables, columns, types
- Data changes: new enums, cardinality shifts, FK patterns
- Changes stored as pending queue

**Milestone:** After schema refresh, system reports:
```json
{
  "schema_changes": [
    {"type": "new_table", "table": "orders"},
    {"type": "new_column", "table": "users", "column": "status"}
  ],
  "data_changes": [
    {"type": "new_enum_values", "table": "users", "column": "status", "values": ["suspended"]}
  ]
}
```

### Phase C: Review & Approve Workflow (PLAN-05)
**Goal:** Changes reviewed before applying to ontology

- Pending changes queue with UI and MCP access
- Precedence model enforced on merge
- Provenance tracked on all elements

**Milestone:** Claude can:
```
list_pending_changes() → review → approve_change(id) or reject_change(id)
```

### Phase D: Incremental Enrichment (PLAN-06)
**Goal:** AI Config mode runs targeted LLM enrichment

- Approved changes feed into DAG nodes
- Only enrich what changed, not full re-extraction
- Results merged respecting precedence

**Milestone:** New table added → only that table's entities enriched via LLM

## Precedence Model

When multiple sources update the same ontology element:

| Priority | Source | How Applied |
|----------|--------|-------------|
| 1 (highest) | Admin via UI | Direct edit, always wins |
| 2 | Claude via MCP | `update_*` tools, wins over inference |
| 3 (lowest) | Engine inference | Auto-detected or LLM-generated |

**Implementation:**
- Every ontology element has `created_by` and `updated_by` fields
- Values: `admin`, `mcp`, `inference`
- Lower priority cannot overwrite higher priority unless forced

## Success Criteria

1. **Dev Tools Mode works end-to-end** without human intervention
2. **Schema changes detected** within seconds of refresh
3. **Data changes detected** on scheduled scan or manual trigger
4. **No ontology regression** - changes reviewed before applying
5. **Incremental enrichment** - new table enriched in <30s, not minutes
6. **Precedence respected** - Claude's work not overwritten by inference

## Open Questions

1. Should data change detection run automatically on schedule, or only on-demand?
2. How long to retain rejected changes in queue?
3. Should Claude be able to "force" overwrite admin changes? (Probably not)
4. What's the right granularity for change detection? (Table, column, value level?)

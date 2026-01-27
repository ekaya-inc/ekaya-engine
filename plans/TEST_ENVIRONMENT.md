# Test Environment Setup: MCP + Chrome Browser Automation

This document describes how to use Claude Code's MCP tools and Chrome browser automation to test the ekaya-engine UI and API synchronization.

## Prerequisites

1. **Dev servers running** (user manages these - never start/stop from Claude Code):
   ```bash
   # Terminal 1
   make dev-ui        # http://localhost:5173

   # Terminal 2
   make dev-server    # http://localhost:3443
   ```

2. **MCP Server connected** - The MCP tools (`mcp__test_data__*`) should be available

3. **Chrome browser** with Claude in Chrome extension installed

## Project URL

```
http://localhost:3443/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/
```

Key pages:
- Dashboard: `/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/`
- Entities: `/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/entities`
- Relationships: `/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/relationships`
- Glossary: `/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/glossary`
- Schema: `/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/schema`

## MCP Tools Available

### Read Operations
| Tool | Purpose |
|------|---------|
| `mcp__test_data__health` | Server health check |
| `mcp__test_data__get_schema` | Database schema with semantic annotations |
| `mcp__test_data__get_context` | Unified context at depth: domain, entities, tables, columns |
| `mcp__test_data__get_entity` | Full entity details by name |
| `mcp__test_data__probe_column` | Column statistics and semantic info |
| `mcp__test_data__probe_relationship` | Relationship metrics |
| `mcp__test_data__list_glossary` | List business glossary terms |
| `mcp__test_data__get_glossary_sql` | Get SQL for a glossary term |
| `mcp__test_data__list_pending_changes` | List pending ontology changes |
| `mcp__test_data__list_ontology_questions` | List ontology questions |
| `mcp__test_data__list_approved_queries` | List pre-approved SQL queries |
| `mcp__test_data__query` | Execute read-only SQL |
| `mcp__test_data__sample` | Quick data preview from table |

### Write Operations
| Tool | Purpose |
|------|---------|
| `mcp__test_data__update_entity` | Create/update entity (upsert by name) |
| `mcp__test_data__delete_entity` | Delete entity |
| `mcp__test_data__update_relationship` | Create/update relationship |
| `mcp__test_data__delete_relationship` | Delete relationship |
| `mcp__test_data__update_column` | Add/update column metadata |
| `mcp__test_data__delete_column_metadata` | Remove column metadata |
| `mcp__test_data__update_glossary_term` | Create/update glossary term |
| `mcp__test_data__delete_glossary_term` | Delete glossary term |
| `mcp__test_data__update_project_knowledge` | Create/update domain fact |
| `mcp__test_data__delete_project_knowledge` | Delete domain fact |
| `mcp__test_data__approve_change` | Approve pending change |
| `mcp__test_data__reject_change` | Reject pending change |
| `mcp__test_data__suggest_approved_query` | Suggest a new approved query |

## Chrome Browser Automation

### Initial Setup

```javascript
// 1. Get tab context (required first)
mcp__claude-in-chrome__tabs_context_mcp({ createIfEmpty: true })

// 2. Create a new tab for testing (recommended)
mcp__claude-in-chrome__tabs_create_mcp()

// 3. Navigate to the project
mcp__claude-in-chrome__navigate({
  url: "http://localhost:3443/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/",
  tabId: <tabId from step 1 or 2>
})
```

### Common Operations

```javascript
// Take screenshot
mcp__claude-in-chrome__computer({ action: "screenshot", tabId: <tabId> })

// Find element by description
mcp__claude-in-chrome__find({ query: "Add Entity button", tabId: <tabId> })

// Click at coordinates
mcp__claude-in-chrome__computer({ action: "left_click", coordinate: [x, y], tabId: <tabId> })

// Click by element reference
mcp__claude-in-chrome__computer({ action: "left_click", ref: "ref_1", tabId: <tabId> })

// Type text
mcp__claude-in-chrome__computer({ action: "type", text: "Hello", tabId: <tabId> })

// Scroll to element
mcp__claude-in-chrome__computer({ action: "scroll_to", ref: "ref_1", tabId: <tabId> })

// Read page accessibility tree
mcp__claude-in-chrome__read_page({ tabId: <tabId> })

// Get page text content
mcp__claude-in-chrome__get_page_text({ tabId: <tabId> })
```

## Test Pattern: MCP→UI Sync

Test that changes made via MCP appear in the UI:

```
1. Make change via MCP tool (e.g., update_entity)
2. Navigate to relevant UI page
3. Take screenshot or read_page to verify change appears
```

Example:
```javascript
// 1. Create entity via MCP
mcp__test_data__update_entity({
  name: "TestEntity2026",
  description: "Test entity for sync verification",
  aliases: ["test_ent"]
})

// 2. Navigate to entities page
mcp__claude-in-chrome__navigate({
  url: "http://localhost:3443/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/entities",
  tabId: <tabId>
})

// 3. Verify in UI
mcp__claude-in-chrome__computer({ action: "screenshot", tabId: <tabId> })
// Or search for it:
mcp__claude-in-chrome__find({ query: "TestEntity2026", tabId: <tabId> })
```

## Test Pattern: UI→MCP Sync

Test that changes made in UI appear via MCP:

```
1. Navigate to UI page
2. Interact with UI to make change (click, type, etc.)
3. Query via MCP tool to verify change persisted
```

Example:
```javascript
// 1. Navigate to glossary page
mcp__claude-in-chrome__navigate({
  url: "http://localhost:3443/projects/2bb984fc-a677-45e9-94ba-9f65712ade70/glossary",
  tabId: <tabId>
})

// 2. Click "Add Term" button, fill form, submit
// ... (multiple browser interactions)

// 3. Verify via MCP
mcp__test_data__list_glossary()
```

## Known Issues

### Glossary Term Creation
The "Create Term" button is disabled until SQL is tested. Workflow:
1. Fill in term name, definition, SQL
2. Click "Test SQL" button
3. Wait for "SQL is valid" confirmation
4. Then "Create Term" button becomes enabled

See: `ui/src/components/GlossaryTermEditor.tsx` lines 172-175, 247-252

### Entity Deletion
Entities with relationships cannot be deleted. Delete relationships first:
```javascript
mcp__test_data__delete_relationship({ from_entity: "EntityA", to_entity: "EntityB" })
mcp__test_data__delete_entity({ name: "EntityA" })
```

## Current Bugs (as of 2026-01-21)

See `plans/FIX-mcp-update-bugs.md`:

1. **update_glossary_term** - nil pointer on update (glossary.go:320)
2. **update_project_knowledge** - duplicate key when using fact_id

See `plans/BUGS-ontology-extraction.md` for ontology extraction issues.

---

## Understanding the Two Databases

There are **two separate databases** involved in testing:

### 1. Datasource Database (via MCP)
- Accessed via `mcp__test_data__*` tools
- Contains the actual business data (e.g., Tikr tables: users, billing_transactions, etc.)
- MCP tools query this database and return ontology-enriched results

### 2. Ekaya Engine Database (via psql)
- Accessed via `psql -d ekaya_engine`
- Contains Ekaya's internal metadata: ontologies, entities, glossary, project knowledge
- Use this to investigate ontology extraction issues or check for stale data

```
┌─────────────────────────┐     ┌─────────────────────────┐
│  Datasource (Tikr DB)   │     │  ekaya_engine DB        │
│  - users                │     │  - engine_ontologies    │
│  - billing_transactions │     │  - engine_ontology_*    │
│  - channels             │     │  - engine_business_*    │
│  - ...                  │     │  - engine_project_*     │
└───────────┬─────────────┘     └───────────┬─────────────┘
            │                               │
            │ MCP Server queries            │ psql queries
            ▼                               ▼
┌─────────────────────────────────────────────────────────┐
│                    Claude Code                          │
│  mcp__test_data__* tools    psql -d ekaya_engine       │
└─────────────────────────────────────────────────────────┘
```

---

## Ontology Tables (ekaya_engine database)

| Table | Purpose |
|-------|---------|
| `engine_ontology_dag` | DAG workflow state, status, current_node |
| `engine_dag_nodes` | Individual DAG node states (one per step) |
| `engine_ontologies` | Tiered ontology storage (domain_summary, entity_summaries, column_details) |
| `engine_ontology_entities` | Discovered domain entities (user, account, order, etc.) with descriptions |
| `engine_ontology_entity_occurrences` | Where entities appear across schema with role semantics |
| `engine_ontology_entity_aliases` | Alternative names for entities (for query matching) |
| `engine_ontology_entity_key_columns` | Important business columns per entity with synonyms |
| `engine_entity_relationships` | Entity-to-entity relationships from FK constraints or inference |
| `engine_ontology_questions` | Questions generated during analysis for user clarification |
| `engine_ontology_chat_messages` | Ontology refinement chat history |
| `engine_llm_conversations` | Verbatim LLM request/response logs for debugging and analytics |
| `engine_project_knowledge` | Project-level facts learned during refinement |
| `engine_business_glossary` | Business glossary terms with SQL definitions |

---

## Investigating Ontology Issues

### Check Current Ontology
```sql
-- Via psql (ekaya_engine database)
psql -d ekaya_engine -c "
  SELECT id, version, is_active, created_at
  FROM engine_ontologies
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
  ORDER BY created_at"
```

### Check for Stale Data from Prior Ontologies
When a datasource is changed and the old ontology is deleted, some tables may retain stale data because they're linked to `project_id` not `ontology_id`.

**Tables cleaned up on ontology delete (have `ontology_id` FK):**
- `engine_ontology_entities`
- `engine_entity_relationships`
- `engine_ontology_questions`
- `engine_ontology_chat_messages`
- `engine_ontology_dag`

**Tables NOT cleaned up (have only `project_id` FK):**
- `engine_project_knowledge`
- `engine_business_glossary`

```sql
-- Check for stale project knowledge
psql -d ekaya_engine -c "
  SELECT set_config('app.current_project_id', '2bb984fc-a677-45e9-94ba-9f65712ade70', false);
  SELECT fact_type, key, created_at
  FROM engine_project_knowledge
  ORDER BY created_at"

-- Check for stale glossary terms (compare created_at with ontology created_at)
psql -d ekaya_engine -c "
  SELECT set_config('app.current_project_id', '2bb984fc-a677-45e9-94ba-9f65712ade70', false);
  SELECT term, created_at
  FROM engine_business_glossary
  ORDER BY created_at"
```

### Verify MCP Returns Current Ontology Data
The MCP server always queries the current active ontology. If you suspect stale data:

1. Check what MCP returns: `mcp__test_data__get_context(depth="domain")`
2. Compare with psql queries to `ekaya_engine` tables
3. If psql shows older data than MCP, you have stale entries from a prior ontology

### Clean Up Stale Data Manually
If you find stale data from a prior datasource:
```sql
-- Delete stale project knowledge (verify dates first!)
psql -d ekaya_engine -c "
  DELETE FROM engine_project_knowledge
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
  AND created_at < '2026-01-21 21:26:50'"  -- before current ontology

-- Delete stale glossary terms
psql -d ekaya_engine -c "
  DELETE FROM engine_business_glossary
  WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
  AND created_at < '2026-01-21 21:26:50'"
```

---

## Cleanup After Testing

```javascript
// Delete test entities
mcp__test_data__delete_entity({ name: "TestEntity2026" })

// Delete test glossary terms
mcp__test_data__delete_glossary_term({ term: "TestTerm2026" })

// Delete test relationships
mcp__test_data__delete_relationship({ from_entity: "TestA", to_entity: "TestB" })
```

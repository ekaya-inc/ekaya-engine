# MCP Tool Test Suite

This directory contains repeatable tests for the ekaya-engine MCP server tools.

## Prerequisites

Before running tests:

1. **Dev server running**: `make dev-server` from project root (port 3443)
2. **Test database ready**: Docker container with test_data database
3. **Ontology extracted**: Schema saved and ontology extraction completed for project `2b5b014f-191a-41b4-b207-85f7d5c3b04b`

## Test Conventions

### Prompt File Naming

- `000-099`: Setup and verification
- `100-199`: Read operations (non-destructive)
- `200-299`: Write operations (create/update)
- `300-399`: Delete operations
- `900-999`: Cleanup and teardown

### Test Data Naming

All test-created data uses the `_MCP_TEST` suffix:
- Entities: `TestEntity_MCP_TEST`
- Glossary terms: `Test Term_MCP_TEST`
- Relationships: Between `*_MCP_TEST` entities only

This allows easy identification and cleanup.

### Running Tests

```bash
# Run all tests
make test

# Run single test interactively
make test-interactive PROMPT=010

# Run single test in print mode
make test-print PROMPT=010
```

### Expected Behavior

When running a test prompt:
1. Read the prompt file to understand what's being tested
2. Execute the MCP tool calls as specified
3. Report results clearly (PASS/FAIL with details)
4. Do NOT create files or make changes outside of MCP tool calls

## Project Details

- **Project ID**: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- **MCP Endpoint**: `http://localhost:3443/mcp/2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- **MCP Server Name**: `mcp_test_suite` (configured in `.mcp.json`)
- **UI URL**: `http://localhost:5173/projects/2b5b014f-191a-41b4-b207-85f7d5c3b04b/`

## MCP Tools Reference

### Read Operations
| Tool | Purpose |
|------|---------|
| `health` | Server health check |
| `echo` | Echo back input (testing) |
| `get_schema` | Database schema with semantic annotations |
| `get_ontology` | Structured ontology at configurable depth |
| `get_context` | Unified context: domain, entities, tables, columns |
| `get_entity` | Full entity details by name |
| `search_schema` | Full-text search across tables, columns, entities |
| `list_glossary` | List business glossary terms |
| `list_approved_queries` | List pre-approved SQL queries |
| `query` | Execute read-only SQL SELECT |
| `sample` | Quick data preview from table |

### Write Operations
| Tool | Purpose |
|------|---------|
| `update_entity` | Create/update entity (upsert by name) |
| `delete_entity` | Delete entity |
| `update_relationship` | Create/update relationship |
| `delete_relationship` | Delete relationship |
| `create_glossary_term` | Create new glossary term with SQL |
| `update_glossary_term` | Update existing glossary term |
| `delete_glossary_term` | Delete glossary term |

See `../../plans/TEST_ENVIRONMENT.md` for the complete tool list (42+ tools).

## Two Databases

```
┌─────────────────────────┐     ┌─────────────────────────┐
│  Datasource (test_data) │     │  ekaya_engine DB        │
│  - Business data        │     │  - engine_ontologies    │
│  - Tables being queried │     │  - engine_ontology_*    │
└───────────┬─────────────┘     └───────────┬─────────────┘
            │                               │
            │ MCP tools query               │ psql queries
            ▼                               ▼
┌─────────────────────────────────────────────────────────┐
│                    Claude Code                          │
│  mcp__mcp_test_suite__* tools    psql -d ekaya_engine  │
└─────────────────────────────────────────────────────────┘
```

- **Datasource** (via MCP): The actual business data being tested
- **ekaya_engine** (via psql): Ekaya's internal metadata (ontologies, entities, glossary)

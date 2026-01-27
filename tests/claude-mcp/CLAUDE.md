# MCP Tool Test Suite

This directory contains repeatable tests for the ekaya-engine MCP server tools.

## CRITICAL: Use the Correct MCP Server

**ALWAYS use `mcp__mcp_test_suite__*` tools, NOT `mcp__test_data__*` tools.**

There may be multiple MCP servers available. This test suite MUST use the `mcp_test_suite` server which connects to project `2b5b014f-191a-41b4-b207-85f7d5c3b04b`.

To load tools correctly:
```
ToolSearch: select:mcp__mcp_test_suite__health
ToolSearch: select:mcp__mcp_test_suite__query
ToolSearch: select:mcp__mcp_test_suite__execute
```

**DO NOT** use `mcp__test_data__*` - that connects to a different project.

## Quick Start

1. Start a new Claude Code session in this directory
2. Run `/mcp` to connect to the `mcp_test_suite` server
3. Run test prompts in numeric order: `000-setup.md`, `010-read-health.md`, `020-test-fixtures.md`, etc.

## Prerequisites

Before running tests:

1. **Dev server running**: `make dev-server` from project root (port 3443)
2. **Test database ready**: Docker container with test_data database
3. **Ontology extracted**: Schema saved and ontology extraction completed for project `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
4. **MCP connected**: Run `/mcp` in Claude Code to connect (configured in `.mcp.json`)

## Test Conventions

### Prompt File Naming

- `000-099`: Setup and verification (run first!)
- `100-199`: Read operations (non-destructive)
- `200-299`: Write operations (create/update)
- `300-399`: Delete operations
- `900-999`: Cleanup and teardown

**Important**: Run tests in numeric order. `020-test-fixtures.md` creates tables that later tests depend on.

### Test Data Naming

All test-created data uses the `_MCP_TEST` suffix:
- Entities: `TestEntity_MCP_TEST`
- Glossary terms: `Test Term_MCP_TEST`
- Relationships: Between `*_MCP_TEST` entities only
- Tables: `mcp_test_*` (e.g., `mcp_test_users`, `mcp_test_orders`)

This allows easy identification and cleanup.

### Test Fixture Tables

`020-test-fixtures.md` creates these tables with known data:
- `mcp_test_users` - 4 rows (Alice, Bob, Carol, Dave)
- `mcp_test_orders` - 4 rows with FK to users

These tables are used by 100-series tests for predictable query/probe/sample results.

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

### IMPORTANT: Run ALL Tests

**DO NOT skip tests.** Run every test in numeric order, taking whatever time is needed. If a test requires a tool that isn't loaded yet, load it with `ToolSearch`. There are no "prerequisites" that would prevent running a test - all tools are available via the MCP server.

Tests to run (in order):
- 000-setup.md
- 010-read-health.md
- 020-test-fixtures.md
- 100-schema.md
- 110-ontology.md
- 111-ontology-entities.md
- 120-probe-columns.md
- 121-probe-relationships.md
- 130-sample.md
- 140-query.md
- 141-query-validate.md
- 142-query-explain.md
- 150-glossary-read.md
- 160-lists.md
- 170-approved-queries.md
- 200-entity-create.md
- 201-entity-update.md
- 210-relationship-create.md
- 211-relationship-update.md
- 220-column-metadata.md
- 230-glossary-create.md
- 231-glossary-update.md
- 240-project-knowledge.md
- 250-change-management.md
- 260-question-management.md
- 270-schema-operations.md
- 280-execute.md
- 290-approved-queries-write.md
- 300-entity-delete.md
- 310-relationship-delete.md
- 320-column-metadata-delete.md
- 330-glossary-delete.md
- 340-project-knowledge-delete.md
- 350-approved-queries-delete.md
- 900-cleanup.md

### When Tests Fail

Create an issue file in `../../plans/ISSUE-<descriptive-name>.md` with:
- What was observed (actual behavior)
- What was expected
- Steps to reproduce (tool calls, parameters, responses)
- Any relevant state or context

**Do NOT**:
- Investigate root cause
- Propose fixes
- Create FIX files

This test suite is an "issue generator" only. A separate process reviews issues and creates FIX files.

### MCP Protocol Errors

**Any MCP protocol error (e.g., `MCP error -32603: ...`) should create an issue.**

MCP errors cause Claude Desktop to flash an error badge and indicate the server is not properly wrapping errors. All errors should be returned as JSON objects with success status, not as MCP protocol errors.

Example of problematic error:
```
Error: MCP error -32603: failed to lookup table: table not found: nonexistent_table_xyz
```

This should instead return:
```json
{"error": true, "code": "TABLE_NOT_FOUND", "message": "table not found: nonexistent_table_xyz"}
```

### Post-Test: Review Server Logs

**After completing all tests, review the server output log at `../../output.log.txt`** for:

1. **ERROR logs with stack traces** - Check if the error was caused by test input (expected) or a server bug (unexpected). Input validation errors should not generate ERROR + stack traces.

2. **WARN logs with stack traces** - Often indicate real bugs like missing database columns or schema mismatches.

3. **Repeated errors** - Patterns that occur on every tool call indicate systemic issues.

To search for errors:
```bash
grep -n "ERROR\|WARN" ../../output.log.txt | head -50
```

Create issues for any server-side problems discovered in the logs.

## Project Details

- **Project ID**: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- **MCP Endpoint**: `http://localhost:3443/mcp/2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- **MCP Server Name**: `mcp_test_suite` (configured in `.mcp.json`)
- **UI URL**: `http://localhost:5173/projects/2b5b014f-191a-41b4-b207-85f7d5c3b04b/`

## MCP Tools Reference

Tools are accessed as `mcp__mcp_test_suite__<tool_name>`. For example:
- `mcp__mcp_test_suite__health`
- `mcp__mcp_test_suite__query`
- `mcp__mcp_test_suite__execute`

Use `ToolSearch` to load tools before calling them.

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

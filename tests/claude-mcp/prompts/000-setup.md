# Test Setup Verification

Verify the test environment is ready before running other tests.

## Checks

1. **MCP Server Health**
   - Call `mcp__test_data__health`
   - Verify status is healthy
   - Verify datasource is connected

2. **Schema Available**
   - Call `mcp__test_data__get_schema`
   - Verify tables are returned (should have test_data tables)

3. **Ontology Extracted**
   - Call `mcp__test_data__get_context` with depth "domain"
   - Verify domain_summary exists
   - Verify entities are present

## Expected Results

- Health check returns `healthy` status
- Schema returns at least 5 tables
- Context returns domain summary with entities

## Report Format

```
=== 000-setup: Test Environment Verification ===

Health Check:
  Status: [healthy/unhealthy]
  Datasource: [connected/disconnected]
  RESULT: [PASS/FAIL]

Schema Check:
  Tables found: [count]
  RESULT: [PASS/FAIL]

Ontology Check:
  Domain summary: [present/missing]
  Entity count: [count]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

# Test: Health and Echo Tools

Test the basic health and diagnostic tools.

## Tools Under Test

- `mcp__test_data__health`
- `mcp__test_data__echo`

## Test Cases

### 1. Health Check
Call `mcp__test_data__health` and verify:
- Returns JSON with `engine` field
- `engine` value is "healthy"
- `datasource.status` is "connected"
- `project_id` matches `2b5b014f-191a-41b4-b207-85f7d5c3b04b`

### 2. Echo Test
Call `mcp__test_data__echo` with message "MCP_TEST_PING" and verify:
- Returns the exact message back
- Response format is correct

## Report Format

```
=== 010-read-health: Health and Echo Tools ===

Test 1: Health Check
  Engine status: [value]
  Datasource status: [value]
  Project ID match: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Echo
  Sent: MCP_TEST_PING
  Received: [value]
  Match: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

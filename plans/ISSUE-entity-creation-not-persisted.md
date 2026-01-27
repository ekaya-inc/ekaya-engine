# Issue: Entity Creation Not Persisted

## Observed Behavior

The `update_entity` MCP tool returns `{"created": true}` but the entity is not actually persisted to the database. Immediately calling `get_entity` with the same name returns `ENTITY_NOT_FOUND`.

## Expected Behavior

After `update_entity` returns `created: true`, the entity should be retrievable via `get_entity`.

## Steps to Reproduce

1. Call `mcp__mcp_test_suite__update_entity` with:
   ```json
   {
     "name": "BasicEntity_MCP_TEST",
     "description": "Test entity for MCP test suite"
   }
   ```

2. Response returns:
   ```json
   {"name":"BasicEntity_MCP_TEST","description":"Test entity for MCP test suite","created":true}
   ```

3. Immediately call `mcp__mcp_test_suite__get_entity` with:
   ```json
   {
     "name": "BasicEntity_MCP_TEST"
   }
   ```

4. Response returns:
   ```json
   {"error":true,"code":"ENTITY_NOT_FOUND","message":"entity \"BasicEntity_MCP_TEST\" not found"}
   ```

## Context

- Project ID: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- MCP Server: `mcp_test_suite`
- Tested multiple times with same result
- Same issue affects `update_relationship` which depends on entity existence

## Impact

- Cannot create test entities for MCP test suite
- Entity CRUD tests (200-series) cannot be completed
- Relationship tests (210-series) cannot be completed

## Possibly Related

- Entity creation may be writing to wrong project
- Entity creation may not be committing transaction
- Entity may be created but in "pending" state requiring approval

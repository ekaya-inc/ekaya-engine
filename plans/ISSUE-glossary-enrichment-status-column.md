# ISSUE: Glossary term creation fails - missing enrichment_status column

**Status:** FIXED (migration exists)

The `enrichment_status` column is defined in `migrations/010_business_glossary.up.sql:13`.
If this error occurs, ensure migrations have been applied.

## Summary (Historical)

Creating a glossary term via MCP tools fails because the glossary repository references a column `enrichment_status` that doesn't exist in the database schema.

## Error

```
MCP error -32603: failed to check for existing term: get glossary term by name:
failed to scan glossary term: ERROR: column g.enrichment_status does not exist (SQLSTATE 42703)
```

## Steps to Reproduce

1. Connect to MCP server via Claude Code (`/mcp`)
2. Attempt to create a glossary term:
   ```
   update_glossary_term(
     term: "Most Active Hosts",
     definition: "Hosts ranked by their total number of billing engagements...",
     sql: "SELECT u.username, COUNT(*) as engagement_count FROM billing_engagements be JOIN users u ON be.host_id = u.user_id...",
     aliases: ["active hosts", "top hosts"]
   )
   ```
3. Observe the error

## Root Cause

The glossary repository code (likely in `pkg/repositories/glossary.go`) references `g.enrichment_status` in a SELECT or scan operation, but this column doesn't exist in the `engine_glossary` table.

This suggests a migration was written to add the column but hasn't been applied, or the repository code was updated ahead of the migration.

## Test After Fix

1. Ensure migrations are applied
2. Connect to MCP server: `/mcp`
3. Create a glossary term:
   ```
   mcp__test_data__update_glossary_term with:
   - term: "Test Term"
   - definition: "A test glossary term"
   - sql: "SELECT 1"
   ```
4. Verify term was created:
   ```
   mcp__test_data__list_glossary
   ```
5. Verify term SQL can be retrieved:
   ```
   mcp__test_data__get_glossary_sql with term: "Test Term"
   ```
6. Clean up: delete the test term

## Related

- MCP tool: `update_glossary_term`
- Repository: `pkg/repositories/glossary.go`
- Table: `engine_glossary`

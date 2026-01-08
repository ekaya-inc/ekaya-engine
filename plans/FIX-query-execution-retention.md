# FIX: Query Execution Retention Policy

**Created:** 2025-01-08
**Status:** Ready for implementation
**Priority:** Medium (prevents unbounded table growth)

---

## Problem

The `engine_query_executions` table logs every query execution via MCP tools. Without a retention policy, this table will grow unbounded over time, potentially causing:
- Increased storage costs
- Slower query history lookups
- Database maintenance overhead

---

## Solution

Implement a retention policy to delete old query execution records.

### Option 1: Database-level (Recommended)

Add a scheduled job using pg_cron or similar:

```sql
-- Delete executions older than 30 days
CREATE OR REPLACE FUNCTION cleanup_old_query_executions()
RETURNS void AS $$
BEGIN
    DELETE FROM engine_query_executions
    WHERE executed_at < NOW() - INTERVAL '30 days';
END;
$$ LANGUAGE plpgsql;

-- Run daily at 3 AM
SELECT cron.schedule('cleanup_query_executions', '0 3 * * *',
    'SELECT cleanup_old_query_executions()');
```

### Option 2: Application-level

Add cleanup in the query execution logging goroutine:

```go
// In pkg/mcp/tools/queries.go, add cleanup after logging
const retentionDays = 30

func cleanupOldExecutions(ctx context.Context, db *database.DB, projectID uuid.UUID) {
    query := `
        DELETE FROM engine_query_executions
        WHERE project_id = $1
        AND executed_at < NOW() - INTERVAL '30 days'
    `
    _, _ = db.Exec(ctx, query, projectID) // Best-effort, ignore errors
}
```

---

## Implementation Steps

1. Choose retention period (recommend 30 days)
2. If using pg_cron:
   - Create migration to install pg_cron extension
   - Add cleanup function and schedule
3. If using application-level:
   - Add cleanup function to queries.go
   - Call occasionally (e.g., every 100th execution or randomly 1% of time)

---

## Configuration

Consider making retention period configurable:

```go
// In config/config.go
type Config struct {
    // ...
    QueryHistoryRetentionDays int `env:"QUERY_HISTORY_RETENTION_DAYS" env-default:"30"`
}
```

---

## Monitoring

Add a metric or log for:
- Number of executions deleted per cleanup run
- Current table row count

---

## Notes

- pg_cron requires superuser to install, may not be available on all managed Postgres services
- Application-level cleanup adds slight overhead but works everywhere
- Consider keeping aggregate statistics (total queries per day) even after deleting individual records

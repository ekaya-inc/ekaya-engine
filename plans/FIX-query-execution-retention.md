# FIX: Query Execution Retention Policy

**Created:** 2025-01-08
**Status:** Completed (2025-01-08)
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

- [x] **Task 1: Choose retention period and implementation approach**
  - **Decision:** 30-day retention period with application-level cleanup
  - **Approach:** Probabilistic cleanup (1% random trigger per query execution)
  - **Rationale:** pg_cron not available; application-level is portable and adds minimal overhead

- [ ] **Task 2: Deploy to DEV and monitor**
  - Merge to main branch to deploy to DEV environment
  - Monitor cleanup execution logs for proper operation
  - Verify cleanup only targets old records and respects retention period

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

---

## Implementation (Completed 2025-01-08)

**Decision:** Application-level cleanup with configurable retention period (30 days default)

**Files Modified:**
- `pkg/config/config.go` - Added `QueryHistoryRetentionDays` configuration field
- `pkg/mcp/tools/queries.go` - Added `cleanupOldQueryExecutions()` function and random trigger (1% probability)
- `main.go` - Updated QueryToolDeps initialization to pass retention configuration
- `pkg/mcp/tools/queries_test.go` - Added integration test `Test_Z_Destructive_CleanupOldQueryExecutions`

**How it works:**
1. Configuration: `QUERY_HISTORY_RETENTION_DAYS` environment variable (default: 30 days)
2. Cleanup is triggered randomly (1% probability) after each query execution
3. Deletes records older than the retention period for the current project only
4. Runs in a background goroutine with best-effort error handling
5. Logs the number of deleted records when cleanup occurs

**Reasoning for Application-level:**
- pg_cron extension is not installed and requires superuser privileges
- Application-level is portable across all PostgreSQL deployments
- Minimal overhead (1% random trigger means ~1 cleanup per 100 queries)
- Tenant-isolated (cleanup per project_id)

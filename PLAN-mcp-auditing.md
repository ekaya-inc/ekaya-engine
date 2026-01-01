# PLAN: MCP Auditing & Governance

## Purpose

Provide administrators with complete visibility into MCP access and queries for governance, compliance, and security purposes. This creates an audit trail of all AI-assisted database access.

**Key Stakeholders:**
- Security teams: Who accessed what data and when
- Compliance officers: Audit trails for SOC2, HIPAA, etc.
- Project admins: Understanding usage patterns and preventing abuse

---

## Audit Events to Capture

### Connection Events
| Event | Data Captured |
|-------|--------------|
| `mcp_session_start` | User ID, client info, timestamp, project |
| `mcp_session_end` | Duration, total queries, errors encountered |
| `mcp_auth_failure` | Attempted user, reason, client IP |

### Tool Invocations
| Event | Data Captured |
|-------|--------------|
| `tool_call` | Tool name, parameters (sanitized), user, timestamp |
| `tool_success` | Tool name, duration, result summary |
| `tool_error` | Tool name, error type, error message |

### Query Events
| Event | Data Captured |
|-------|--------------|
| `query_generated` | Natural language, generated SQL, tables accessed |
| `query_executed` | SQL, execution time, row count, success/failure |
| `query_blocked` | SQL, reason (policy violation, rate limit, etc.) |
| `approved_query_executed` | Query ID, parameters, result summary |

### Security Events
| Event | Data Captured |
|-------|--------------|
| `sql_injection_attempt` | Detected pattern, parameter values, user |
| `rate_limit_hit` | User, limit type, window |
| `unauthorized_table_access` | Table name, user, query |
| `sensitive_data_access` | Column flagged as sensitive, query context |

---

## Data Model

### New Table: `engine_mcp_audit_log`

```sql
CREATE TABLE engine_mcp_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),

    -- Who
    user_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(255),
    session_id VARCHAR(255),  -- Groups events in same session

    -- What
    event_type VARCHAR(50) NOT NULL,  -- See events above
    tool_name VARCHAR(100),

    -- Request details
    request_params JSONB,  -- Sanitized parameters
    natural_language TEXT,  -- If query-related
    sql_query TEXT,  -- If query-related (sanitized)

    -- Response details
    was_successful BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    result_summary JSONB,  -- {"row_count": 100, "tables_accessed": ["users"]}

    -- Performance
    duration_ms INTEGER,

    -- Security classification
    security_level VARCHAR(20) DEFAULT 'normal',  -- 'normal', 'warning', 'alert'
    security_flags TEXT[],  -- ['sensitive_data', 'large_result', 'new_table']

    -- Context
    client_info JSONB,  -- User agent, IP (if available), MCP client version
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Partitioning support
    partition_key DATE NOT NULL DEFAULT CURRENT_DATE
) PARTITION BY RANGE (partition_key);

-- Create monthly partitions (example)
CREATE TABLE engine_mcp_audit_log_2025_01 PARTITION OF engine_mcp_audit_log
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

-- Indexes for common queries
CREATE INDEX idx_audit_project_time ON engine_mcp_audit_log(project_id, created_at DESC);
CREATE INDEX idx_audit_user ON engine_mcp_audit_log(project_id, user_id, created_at DESC);
CREATE INDEX idx_audit_event_type ON engine_mcp_audit_log(project_id, event_type, created_at DESC);
CREATE INDEX idx_audit_security ON engine_mcp_audit_log(project_id, security_level, created_at DESC)
    WHERE security_level != 'normal';
```

### Supporting Table: `engine_mcp_audit_alerts`

```sql
CREATE TABLE engine_mcp_audit_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),

    alert_type VARCHAR(50) NOT NULL,  -- 'sql_injection', 'rate_limit', 'unusual_access'
    severity VARCHAR(20) NOT NULL,  -- 'info', 'warning', 'critical'

    -- Alert details
    title VARCHAR(255) NOT NULL,
    description TEXT,
    affected_user_id VARCHAR(255),
    related_audit_ids UUID[],  -- References to audit_log entries

    -- Resolution
    status VARCHAR(20) NOT NULL DEFAULT 'open',  -- 'open', 'acknowledged', 'resolved', 'dismissed'
    resolved_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,
    resolution_notes TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## UI: Audit Tile on Project Dashboard

### Dashboard Tile Design

```
┌─────────────────────────────────────────┐
│ 🔍 Audit                                │
│                                         │
│ 1,234 queries (last 7 days)             │
│ 12 users active                         │
│                                         │
│ ⚠️ 2 alerts require attention           │
└─────────────────────────────────────────┘
```

Tile shows:
- Query count in recent period
- Active user count
- Alert badge if unresolved alerts exist

### Audit Page Layout

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Audit Log                                                    [Export CSV]   │
├─────────────────────────────────────────────────────────────────────────────┤
│ Filters:                                                                    │
│ [Date Range ▼] [User ▼] [Event Type ▼] [Security Level ▼] [Search...]      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ ┌─ Summary Cards ────────────────────────────────────────────────────────┐ │
│ │ [Queries: 1,234] [Users: 12] [Errors: 23] [Avg Response: 145ms]       │ │
│ └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ ┌─ Alert Banner (if any) ────────────────────────────────────────────────┐ │
│ │ ⚠️ 2 security alerts require attention                    [View All]  │ │
│ └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Event Log:                                                                  │
│ ┌────────────────────────────────────────────────────────────────────────┐ │
│ │ 10:45:23  john@example.com  query_executed  ✓                          │ │
│ │           SELECT u.name FROM users u WHERE...  (145ms, 50 rows)        │ │
│ ├────────────────────────────────────────────────────────────────────────┤ │
│ │ 10:44:12  john@example.com  tool_call: ontology_context  ✓             │ │
│ │           (23ms)                                                        │ │
│ ├────────────────────────────────────────────────────────────────────────┤ │
│ │ 10:43:01  jane@example.com  query_blocked  ⚠️                          │ │
│ │           Attempted access to restricted table 'salary_data'            │ │
│ └────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ [Load More...]                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Event Detail Modal

Clicking an event shows full details:

```
┌─────────────────────────────────────────────────────────────────┐
│ Event Details                                              [×]  │
├─────────────────────────────────────────────────────────────────┤
│ Event Type:    query_executed                                   │
│ Timestamp:     2024-12-15 10:45:23 UTC                          │
│ User:          john@example.com                                 │
│ Session ID:    abc123...                                        │
│ Duration:      145ms                                            │
│ Status:        ✓ Success                                        │
│                                                                 │
│ Natural Language Query:                                         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ "Show me all users who signed up last week"                 │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ Generated SQL:                                                  │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ SELECT u.id, u.name, u.email, u.created_at                  │ │
│ │ FROM users u                                                │ │
│ │ WHERE u.created_at >= '2024-12-08'                          │ │
│ │   AND u.created_at < '2024-12-15'                           │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ Result Summary:                                                 │
│   Rows Returned: 47                                             │
│   Tables Accessed: users                                        │
│                                                                 │
│ [Copy SQL] [Re-run Query] [View User Activity]                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Alert System

### Alert Types

| Alert Type | Trigger | Severity |
|------------|---------|----------|
| `sql_injection_detected` | Injection pattern found in input | Critical |
| `unusual_query_volume` | User exceeds normal query rate by 5x | Warning |
| `sensitive_table_access` | Query touches flagged sensitive table | Warning |
| `large_data_export` | Query returns > 10K rows | Info |
| `after_hours_access` | Access outside business hours | Info |
| `new_user_high_volume` | New user runs many queries quickly | Warning |
| `repeated_errors` | Same error > 5 times in 10 min | Warning |

### Alert Configuration (Project Settings)

```
┌─────────────────────────────────────────────────────────────────┐
│ Audit Alert Settings                                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ ☑ SQL Injection Detection              [Critical]              │
│   Alert on any detected injection attempt                       │
│                                                                 │
│ ☑ Unusual Query Volume                 [Warning]               │
│   Threshold: [5x] normal rate within [10 min] window            │
│                                                                 │
│ ☐ Sensitive Table Access               [Warning]               │
│   Tables: [Select tables...]                                    │
│                                                                 │
│ ☑ Large Data Export                    [Info]                  │
│   Threshold: [10000] rows                                       │
│                                                                 │
│ ☐ After Hours Access                   [Info]                  │
│   Business hours: [9:00 AM] to [6:00 PM] [America/New_York]    │
│                                                                 │
│ Email notifications:                                            │
│   ☑ Critical alerts → [admin@company.com]                      │
│   ☐ Warning alerts → [                   ]                     │
│                                                                 │
│                                               [Save Settings]   │
└─────────────────────────────────────────────────────────────────┘
```

---

## API Endpoints

### List Audit Events
```
GET /api/projects/{pid}/audit/events
Query params:
  - start_date, end_date
  - user_id
  - event_type
  - security_level
  - limit, offset
```

### Get Audit Summary
```
GET /api/projects/{pid}/audit/summary
Query params:
  - start_date, end_date

Response:
{
  "query_count": 1234,
  "unique_users": 12,
  "error_count": 23,
  "avg_response_ms": 145,
  "top_tables": [{"table": "orders", "query_count": 456}],
  "hourly_distribution": [...]
}
```

### List Alerts
```
GET /api/projects/{pid}/audit/alerts
Query params:
  - status: open, resolved, all
  - severity
```

### Resolve Alert
```
POST /api/projects/{pid}/audit/alerts/{alert_id}/resolve
Body:
{
  "resolution": "dismissed",  // or "resolved"
  "notes": "Legitimate access by approved user"
}
```

### Export Audit Log
```
GET /api/projects/{pid}/audit/export
Query params:
  - format: csv, json
  - start_date, end_date
  - event_type
```

---

## Recording Implementation

### Middleware Approach

Create audit middleware that wraps MCP tool handlers:

```go
func AuditMiddleware(next ToolHandler, logger AuditLogger) ToolHandler {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        start := time.Now()

        // Record start
        auditEntry := &AuditEntry{
            EventType:     "tool_call",
            ToolName:      req.Tool,
            RequestParams: sanitizeParams(req.Params),
            StartedAt:     start,
        }

        // Execute
        result, err := next(ctx, req)

        // Record completion
        auditEntry.Duration = time.Since(start)
        auditEntry.WasSuccessful = err == nil
        if err != nil {
            auditEntry.ErrorMessage = err.Error()
        }

        // Async write (don't block response)
        go logger.Record(ctx, auditEntry)

        return result, err
    }
}
```

### Sanitization Rules

Before storing in audit log:
1. Truncate SQL > 10KB
2. Redact string literals if configured
3. Preserve structure but hide values in parameters
4. Hash user-provided values if sensitive

---

## Retention & Archival

### Default Retention
- Hot storage (queryable): 90 days
- Cold storage (archived): 2 years
- Configurable per project

### Archival Process
- Daily job moves old partitions to cold storage
- Cold storage is compressed, read-only
- Export available for compliance requests

### GDPR Considerations
- User deletion request: Anonymize user_id in audit logs
- Right to access: Export endpoint includes user's audit history
- Configurable: Option to not log certain fields

---

## Performance Considerations

### Write Optimization
- Async writes via channel/queue
- Batch inserts every 100ms or 100 records
- Don't block MCP responses on audit writes

### Read Optimization
- Partitioned by date for efficient range queries
- Appropriate indexes for common filters
- Consider materialized views for dashboard summaries

### Storage Estimation
- ~500 bytes per event (compressed)
- 1M queries/month = ~500MB/month
- Plan for growth with partitioning

---

## Implementation Phases

### Phase 1: Core Logging
- Create tables and migrations
- Add audit middleware to MCP tools
- Basic event recording (tool calls, queries)

### Phase 2: UI & Viewing
- Audit tile on dashboard
- Audit log page with filtering
- Event detail views

### Phase 3: Alerts
- Alert detection logic
- Alert table and management
- Email notifications

### Phase 4: Advanced
- Usage analytics dashboards
- Anomaly detection
- Export/compliance features

---

## File Changes Summary

| Area | Changes |
|------|---------|
| `migrations/` | New tables for audit_log, audit_alerts |
| `pkg/models/` | `audit.go` with event types and models |
| `pkg/repositories/` | `audit_repository.go` |
| `pkg/services/` | `audit_service.go` with recording logic |
| `pkg/mcp/` | Audit middleware for tool handlers |
| `pkg/handlers/` | `audit.go` API endpoints |
| `ui/src/pages/` | `AuditPage.tsx` |
| `ui/src/components/` | Audit-related components |

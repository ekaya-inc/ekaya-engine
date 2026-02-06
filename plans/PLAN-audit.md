# PLAN: Unified Auditing & Governance

## Purpose

Provide administrators with complete visibility into all project activity: query executions, ontology changes, schema changes, query approvals, and MCP tool usage. This serves governance, compliance, and security needs by creating a comprehensive audit trail accessible from the AI Data Liaison page.

**Key Stakeholders:**
- Data engineers: Visibility into what business users and AI are doing with their data
- Security teams: Who accessed what data and when
- Compliance officers: Audit trails for SOC2, HIPAA, etc.
- Project admins: Understanding usage patterns and preventing abuse

---

## Existing Data Sources

Several audit data sources already exist. No new tables needed for Phase 1.

| Data Source | Table | Key Fields |
|---|---|---|
| Query executions | `engine_query_executions` | user_id, sql, executed_at, execution_time_ms, row_count, is_modifying, success, error_message, source |
| Query approvals | `engine_queries` | status, suggested_by, reviewed_by, reviewed_at, rejection_reason |
| Ontology changes | `engine_audit_log` | entity_type, entity_id, action, source, user_id, changed_fields |
| Schema changes | `engine_ontology_pending_changes` | change_type, table_name, column_name, old_value, new_value, status, reviewed_by |
| Column metadata provenance | `engine_ontology_column_metadata` | source, last_edit_source, created_by, updated_by |
| Table metadata provenance | `engine_table_metadata` | source, last_edit_source, created_by, updated_by |

**Note:** `engine_query_executions.user_id` is TEXT (from JWT claims), not a UUID FK.

---

## Phase 1: Audit UI for Existing Data

Surfaces existing audit data through a tabbed UI accessible from the AI Data Liaison page. No new tables or middleware required.

### 1.1 AI Data Liaison Page: Add Auditing Tile

Add a new card to `AIDataLiaisonPage.tsx` (between "Enabled Tools" and "Danger Zone") linking to the auditing screen. Only visible when the setup checklist is complete.

### 1.2 Auditing Page: `/projects/{pid}/audit`

A new page with a tabbed interface. Each tab shows a filtered, sortable, paginated table.

#### Tab: Query Executions

Source: `engine_query_executions`

**Columns:** Time, User, Query Name (join to engine_queries), SQL (truncated), Duration, Rows, Success, Destructive

**Filters:**
- User (dropdown of distinct user_ids)
- Time range (preset: last 24h, 7d, 30d, custom)
- Success/failure
- Destructive only (is_modifying = true)
- Source (mcp, api, ui)
- Query ID (link to specific approved query)

**Highlight:** Destructive queries (is_modifying) with warning indicator. Failed queries with error styling.

#### Tab: Ontology Changes

Source: `engine_audit_log`

**Columns:** Time, User, Entity Type, Action, Source, Changed Fields (summary)

**Filters:**
- User
- Time range
- Entity type (entity, relationship, glossary_term, project_knowledge)
- Action (create, update, delete)
- Source (inferred, mcp, manual)

**Detail expand:** Click a row to see full `changed_fields` JSON diff (old/new values).

#### Tab: Schema Changes

Source: `engine_ontology_pending_changes`

**Columns:** Time, Change Type, Table, Column, Status, Reviewed By

**Filters:**
- Time range
- Change type (new_table, dropped_table, new_column, dropped_column, modified_column, etc.)
- Status (pending, approved, rejected, auto_applied)
- Table name

#### Tab: Query Approvals

Source: `engine_queries` where `suggested_by IS NOT NULL`

**Columns:** Time, Suggested By, Query Name, SQL (truncated), Status, Reviewed By, Reviewed At, Rejection Reason

**Filters:**
- Time range
- Status (pending, approved, rejected)
- Suggested by
- Reviewed by

### 1.3 Backend API Endpoints

All endpoints scoped to project via `{pid}` path param and RLS.

#### `GET /api/projects/{pid}/audit/query-executions`

Query params: `user_id`, `since`, `until`, `success` (bool), `is_modifying` (bool), `source`, `query_id`, `limit`, `offset`

Returns paginated `engine_query_executions` joined with `engine_queries.natural_language_prompt` for the query name.

#### `GET /api/projects/{pid}/audit/ontology-changes`

Query params: `user_id`, `since`, `until`, `entity_type`, `action`, `source`, `limit`, `offset`

Returns paginated `engine_audit_log` entries.

#### `GET /api/projects/{pid}/audit/schema-changes`

Query params: `since`, `until`, `change_type`, `status`, `table_name`, `limit`, `offset`

Returns paginated `engine_ontology_pending_changes`.

#### `GET /api/projects/{pid}/audit/query-approvals`

Query params: `since`, `until`, `status`, `suggested_by`, `reviewed_by`, `limit`, `offset`

Returns paginated `engine_queries` filtered to those with suggestion workflow activity.

#### `GET /api/projects/{pid}/audit/summary`

Returns aggregate counts for a dashboard header:
- Total query executions (last 30d)
- Failed query count
- Destructive query count
- Ontology changes count
- Pending schema changes count
- Pending query approvals count

### 1.4 Backend Files to Create/Modify

- [ ] `pkg/handlers/audit_handler.go` -- New handler with all audit endpoints
- [ ] `pkg/services/audit_service.go` -- New service aggregating queries across repositories
- [ ] `pkg/repositories/audit_repository.go` -- Already exists (for `engine_audit_log`). Add methods for summary queries.
- [ ] `pkg/repositories/query_execution_repository.go` -- New repository (or add to existing `query_repository.go`) for paginated execution queries with filters
- [ ] `pkg/server/routes.go` -- Register new audit routes under `/api/projects/{pid}/audit/`

### 1.5 Frontend Files to Create/Modify

- [ ] `ui/src/pages/AuditPage.tsx` -- New page with tabbed interface
- [ ] `ui/src/components/audit/QueryExecutionsTab.tsx` -- Query executions table with filters
- [ ] `ui/src/components/audit/OntologyChangesTab.tsx` -- Ontology audit log table
- [ ] `ui/src/components/audit/SchemaChangesTab.tsx` -- Schema changes table
- [ ] `ui/src/components/audit/QueryApprovalsTab.tsx` -- Query approval workflow table
- [ ] `ui/src/components/audit/AuditSummaryHeader.tsx` -- Summary counts across tabs
- [ ] `ui/src/types/audit.ts` -- TypeScript types for audit API responses
- [ ] `ui/src/services/engineApi.ts` -- Add audit API methods
- [ ] `ui/src/pages/AIDataLiaisonPage.tsx` -- Add Auditing tile/card
- [ ] `ui/src/App.tsx` -- Register `/projects/:pid/audit` route

### 1.6 Implementation Order

1. Backend API: audit handler, service, repository methods
2. Frontend types & API client
3. Audit page shell: route, page layout, tab navigation
4. Query Executions tab (most valuable -- shows who ran what)
5. Ontology Changes tab
6. Schema Changes tab
7. Query Approvals tab
8. Summary header
9. AI Data Liaison tile

---

## Phase 2: MCP Event Capture

Adds comprehensive MCP-specific event logging via middleware. Captures tool invocations, session events, and security-relevant activity that isn't covered by existing tables.

### 2.1 Audit Events to Capture

#### Connection Events
| Event | Data Captured |
|-------|--------------|
| `mcp_session_start` | User ID, client info, timestamp, project |
| `mcp_session_end` | Duration, total queries, errors encountered |
| `mcp_auth_failure` | Attempted user, reason, client IP |

#### Tool Invocations
| Event | Data Captured |
|-------|--------------|
| `tool_call` | Tool name, parameters (sanitized), user, timestamp |
| `tool_success` | Tool name, duration, result summary |
| `tool_error` | Tool name, error type, error message |

#### Query Events
| Event | Data Captured |
|-------|--------------|
| `query_generated` | Natural language, generated SQL, tables accessed |
| `query_executed` | SQL, execution time, row count, success/failure |
| `query_blocked` | SQL, reason (policy violation, rate limit, etc.) |
| `approved_query_executed` | Query ID, parameters, result summary |

#### Security Events
| Event | Data Captured |
|-------|--------------|
| `sql_injection_attempt` | Detected pattern, parameter values, user |
| `rate_limit_hit` | User, limit type, window |
| `unauthorized_table_access` | Table name, user, query |
| `sensitive_data_access` | Column flagged as sensitive, query context |

### 2.2 New Table: `engine_mcp_audit_log`

```sql
CREATE TABLE engine_mcp_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),

    -- Who
    user_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(255),
    session_id VARCHAR(255),

    -- What
    event_type VARCHAR(50) NOT NULL,
    tool_name VARCHAR(100),

    -- Request details
    request_params JSONB,
    natural_language TEXT,
    sql_query TEXT,

    -- Response details
    was_successful BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    result_summary JSONB,

    -- Performance
    duration_ms INTEGER,

    -- Security classification
    security_level VARCHAR(20) DEFAULT 'normal',
    security_flags TEXT[],

    -- Context
    client_info JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Partitioning support
    partition_key DATE NOT NULL DEFAULT CURRENT_DATE
) PARTITION BY RANGE (partition_key);

CREATE INDEX idx_audit_project_time ON engine_mcp_audit_log(project_id, created_at DESC);
CREATE INDEX idx_audit_user ON engine_mcp_audit_log(project_id, user_id, created_at DESC);
CREATE INDEX idx_audit_event_type ON engine_mcp_audit_log(project_id, event_type, created_at DESC);
CREATE INDEX idx_audit_security ON engine_mcp_audit_log(project_id, security_level, created_at DESC)
    WHERE security_level != 'normal';
```

### 2.3 Audit Middleware

Wraps MCP tool handlers to capture events without blocking responses:

```go
func AuditMiddleware(next ToolHandler, logger AuditLogger) ToolHandler {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        start := time.Now()
        auditEntry := &AuditEntry{
            EventType:     "tool_call",
            ToolName:      req.Tool,
            RequestParams: sanitizeParams(req.Params),
            StartedAt:     start,
        }

        result, err := next(ctx, req)

        auditEntry.Duration = time.Since(start)
        auditEntry.WasSuccessful = err == nil
        if err != nil {
            auditEntry.ErrorMessage = err.Error()
        }

        // Async write -- don't block response
        go logger.Record(ctx, auditEntry)

        return result, err
    }
}
```

### 2.4 Sanitization Rules

Before storing in audit log:
1. Truncate SQL > 10KB
2. Redact string literals if configured
3. Preserve structure but hide values in parameters
4. Hash user-provided values if sensitive

### 2.5 Add MCP Events Tab to Audit Page

New tab on the existing audit page showing `engine_mcp_audit_log` events.

#### `GET /api/projects/{pid}/audit/mcp-events`

Query params: `user_id`, `since`, `until`, `event_type`, `tool_name`, `security_level`, `limit`, `offset`

---

## Phase 3: Alerts & Security

### 3.1 New Table: `engine_mcp_audit_alerts`

```sql
CREATE TABLE engine_mcp_audit_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),

    alert_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL,

    title VARCHAR(255) NOT NULL,
    description TEXT,
    affected_user_id VARCHAR(255),
    related_audit_ids UUID[],

    status VARCHAR(20) NOT NULL DEFAULT 'open',
    resolved_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,
    resolution_notes TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 3.2 Alert Types

| Alert Type | Trigger | Severity |
|------------|---------|----------|
| `sql_injection_detected` | Injection pattern found in input | Critical |
| `unusual_query_volume` | User exceeds normal query rate by 5x | Warning |
| `sensitive_table_access` | Query touches flagged sensitive table | Warning |
| `large_data_export` | Query returns > 10K rows | Info |
| `after_hours_access` | Access outside business hours | Info |
| `new_user_high_volume` | New user runs many queries quickly | Warning |
| `repeated_errors` | Same error > 5 times in 10 min | Warning |

### 3.3 Alert API Endpoints

#### `GET /api/projects/{pid}/audit/alerts`
Query params: `status` (open, resolved, all), `severity`

#### `POST /api/projects/{pid}/audit/alerts/{alert_id}/resolve`
Body: `{ "resolution": "dismissed|resolved", "notes": "..." }`

### 3.4 Alert Configuration UI

Project settings page for configuring alert thresholds, which alerts are enabled, and email notification recipients.

---

## Phase 4: Advanced Features

- **Export:** CSV/JSON export for compliance reporting (`GET /api/projects/{pid}/audit/export`)
- **Retention:** Hot storage (90 days queryable), cold storage (2 years archived), configurable per project
- **GDPR:** User deletion anonymizes audit logs, right-to-access export endpoint
- **Analytics:** Usage dashboards, anomaly detection
- **Performance:** Batch inserts, partitioned tables, materialized views for dashboard summaries

---

## Open Questions

1. **Export in Phase 1?** Should any tab support CSV/JSON export immediately, or defer to Phase 4?
2. **Retention policy?** Should old audit records be pruned, or kept indefinitely until Phase 4?
3. **Real-time updates?** Should the audit page poll for new entries, or is manual refresh sufficient?

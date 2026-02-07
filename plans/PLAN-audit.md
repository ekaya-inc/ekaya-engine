# PLAN: Unified Auditing, Governance & Query Learning

## Purpose

Provide administrators with complete visibility into all project activity (query executions, ontology changes, schema changes, query approvals, MCP tool usage) while enabling the MCP client to learn from past queries — the user's own successful query history scoped to their account.

**Core Insight:** The best query for a new question is often an adaptation of a query that worked before. Auditing gives visibility; query history creates a feedback loop where the system gets smarter over time.

**Key Stakeholders:**
- Data engineers: Visibility into what business users and AI are doing with their data
- Security teams: Who accessed what data and when
- Compliance officers: Audit trails for SOC2, HIPAA, etc.
- Project admins: Understanding usage patterns and preventing abuse
- MCP users: Better query suggestions from accumulated history

---

## Decisions

These decisions were made upfront to keep the plan deterministic for automated implementation.

| Decision | Answer |
|----------|--------|
| Export in Phase 1? | No. Defer all export to Phase 5. |
| Retention policy? | Auto-prune at 90 days, settable by Admin. Applies to all audit/history tables. |
| Real-time updates? | Manual refresh only. No polling. |
| Record failed queries in history? | No. Only record successful queries. |
| Store edited SQL? | No. Store final (executed) SQL only. Drop `generated_sql`/`final_sql` split — single `sql` column. |
| Cross-user query visibility? | User's own history only. No cross-user similarity search. |
| Similarity search approach? | Remove all embedding/vector features from this plan. No pgvector, no `question_embedding` column. Similarity search and pattern detection are post-launch features to be planned separately. |

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

### 1.1 AI Data Liaison Page: Add Auditing Tile ✅

Add a new card to `AIDataLiaisonPage.tsx` (between "Enabled Tools" and "Danger Zone") linking to the auditing screen. Only visible when the setup checklist is complete.

### 1.2 Auditing Page: `/projects/{pid}/audit` ✅

A new page with a tabbed interface. Each tab shows a filtered, sortable, paginated table. Manual refresh only (no polling).

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

### 1.3 Backend API Endpoints ✅

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

- [x] `pkg/handlers/audit_handler.go` -- New handler with all audit endpoints
- [x] `pkg/services/audit_service.go` -- New service aggregating queries across repositories
- [x] `pkg/repositories/audit_repository.go` -- Already exists (for `engine_audit_log`). Add methods for summary queries.
- [x] `pkg/repositories/query_execution_repository.go` -- New repository (or add to existing `query_repository.go`) for paginated execution queries with filters
- [x] `pkg/server/routes.go` -- Register new audit routes under `/api/projects/{pid}/audit/`

### 1.5 Frontend Files to Create/Modify

- [x] `ui/src/pages/AuditPage.tsx` -- New page with tabbed interface
- [x] Query Executions, Ontology Changes, Schema Changes, Query Approvals tabs and Summary Header -- implemented inline in AuditPage.tsx
- [x] `ui/src/types/audit.ts` -- TypeScript types for audit API responses
- [x] `ui/src/services/engineApi.ts` -- Add audit API methods
- [x] `ui/src/pages/AIDataLiaisonPage.tsx` -- Add Auditing tile/card
- [x] `ui/src/App.tsx` -- Register `/projects/:pid/audit` route

### 1.6 Implementation Order (split into subtasks)

#### 1.6.1 [x] Backend API: Audit handler, service, and repository methods

Create the backend audit infrastructure: `pkg/handlers/audit_handler.go` (thin HTTP handler with endpoints for query-executions, ontology-changes, schema-changes, query-approvals, and summary), `pkg/services/audit_service.go` (business logic aggregating across repositories), repository methods in `pkg/repositories/audit_repository.go` (add summary query methods) and `pkg/repositories/query_execution_repository.go` (paginated execution queries with filters). Register all routes in `pkg/server/routes.go` under `/api/projects/{pid}/audit/`. Each endpoint accepts pagination (limit, offset) and filter query params as described in plan section 1.3. All endpoints are scoped to project via `{pid}` path param and RLS. Follow the project's clean architecture: handlers → services → repositories. No raw SQL in services.

#### 1.6.2 [x] Frontend types, API client, and audit page shell

Create `ui/src/types/audit.ts` with TypeScript types matching all backend audit API responses (query executions, ontology changes, schema changes, query approvals, summary). Add audit API methods to `ui/src/services/engineApi.ts` for all five endpoints. Create `ui/src/pages/AuditPage.tsx` as a new page with tabbed interface (Query Executions, Ontology Changes, Schema Changes, Query Approvals tabs — content can be placeholder initially). Register the route `/projects/:pid/audit` in `ui/src/App.tsx`. The page should include tab navigation using the project's existing UI patterns (React Router, TailwindCSS, Radix UI).

#### 1.6.3 [x] Query Executions and Ontology Changes tabs

Implement the Query Executions tab in `AuditPage.tsx` showing data from `GET /api/projects/{pid}/audit/query-executions`. Columns: Time, User, Query Name, SQL (truncated), Duration, Rows, Success, Destructive. Include filters: user dropdown, time range presets (24h, 7d, 30d, custom), success/failure, destructive only, source, query ID. Highlight destructive queries with warning indicator and failed queries with error styling. Then implement the Ontology Changes tab sourcing from `GET /api/projects/{pid}/audit/ontology-changes`. Columns: Time, User, Entity Type, Action, Source, Changed Fields summary. Filters: user, time range, entity type, action, source. Include expandable row detail showing full changed_fields JSON diff.

#### 1.6.4 [x] Schema Changes, Query Approvals tabs, and Summary header

Implement the Schema Changes tab sourcing from `GET /api/projects/{pid}/audit/schema-changes`. Columns: Time, Change Type, Table, Column, Status, Reviewed By. Filters: time range, change type, status, table name. Implement the Query Approvals tab sourcing from `GET /api/projects/{pid}/audit/query-approvals`. Columns: Time, Suggested By, Query Name, SQL (truncated), Status, Reviewed By, Reviewed At, Rejection Reason. Filters: time range, status, suggested by, reviewed by. Add the Summary header at the top of the audit page using `GET /api/projects/{pid}/audit/summary` showing: total query executions (30d), failed query count, destructive query count, ontology changes count, pending schema changes, pending query approvals.

#### 1.6.5 [x] AI Data Liaison auditing tile

Add a new card to `ui/src/pages/AIDataLiaisonPage.tsx` (between "Enabled Tools" and "Danger Zone") that links to `/projects/{pid}/audit`. The tile should only be visible when the setup checklist is complete. Use existing card patterns from the AI Data Liaison page (Lucide React icons, TailwindCSS styling consistent with other tiles).

---

## Phase 2: MCP Event Capture ✅

Adds comprehensive MCP-specific event logging via middleware. Captures tool invocations, session events, and security-relevant activity that isn't covered by existing tables.

### 2.1 Audit Events to Capture ✅

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

### 2.2 New Table: `engine_mcp_audit_log` ✅

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

### 2.3 Audit Middleware ✅

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

### 2.4 Sanitization Rules ✅

Before storing in audit log:
1. Truncate SQL > 10KB
2. Redact string literals if configured
3. Preserve structure but hide values in parameters
4. Hash user-provided values if sensitive

### 2.5 Add MCP Events Tab to Audit Page ✅

New tab on the existing audit page showing `engine_mcp_audit_log` events.

#### `GET /api/projects/{pid}/audit/mcp-events`

Query params: `user_id`, `since`, `until`, `event_type`, `tool_name`, `security_level`, `limit`, `offset`

---

## Phase 3: Query History & Learning ✅

Enables the MCP client to learn from past successful queries. Each user's history is private to their account. Only successful queries are recorded.

### 3.1 Use Cases

**Query Reuse:**
User asks: "Show me top customers by revenue"
System finds: Previous query "top 10 customers by order value" in user's history
LLM adapts: Reuses the query structure, adjusts column names if needed

**Pattern Discovery:**
User frequently queries: "X by month for last year"
System learns: This user prefers monthly aggregations with 12-month lookback

### 3.2 New Table: `engine_mcp_query_history`

```sql
CREATE TABLE engine_mcp_query_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    user_id VARCHAR(255) NOT NULL,  -- From auth claims

    -- The query itself
    natural_language TEXT NOT NULL,
    sql TEXT NOT NULL,  -- The SQL that was actually executed

    -- Execution details
    executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    execution_duration_ms INTEGER,
    row_count INTEGER,

    -- Learning signals
    user_feedback VARCHAR(20),  -- 'helpful', 'not_helpful', NULL
    feedback_comment TEXT,

    -- Query classification
    query_type VARCHAR(50),  -- 'aggregation', 'lookup', 'report', 'exploration'
    tables_used TEXT[],  -- ['users', 'orders']
    aggregations_used TEXT[],  -- ['SUM', 'COUNT', 'AVG']
    time_filters JSONB,  -- {"type": "relative", "period": "last_quarter"}

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_project FOREIGN KEY (project_id) REFERENCES engine_projects(id)
);

-- Indexes
CREATE INDEX idx_query_history_user ON engine_mcp_query_history(project_id, user_id, created_at DESC);
CREATE INDEX idx_query_history_tables ON engine_mcp_query_history USING GIN(tables_used);
```

**Note:** Only successful queries are inserted. Failed queries are not recorded. The `was_successful` column is omitted since all rows are successful by definition.

### 3.3 Query Recording Flow

When a query is executed successfully via MCP tools, automatically record it:

```
1. User sends natural language question
2. LLM generates SQL using ontology tools
3. LLM calls query() or execute_approved_query()
4. If execution succeeds, system records:
   - Natural language (from explicit parameter)
   - Final executed SQL
   - Execution result metadata (duration, row count)
   - Tables used (parsed from SQL)
5. Query ID returned in response for later feedback
```

#### Recording Integration Point

Modify the `query` tool to optionally accept natural language context:

```go
// query tool input
{
  "sql": "SELECT ...",
  "limit": 100,
  "natural_language_context": "top customers by revenue"  // Optional
}
```

If provided and the query succeeds, automatically creates a history entry.

### 3.4 MCP Tools

#### Tool: get_query_history

```
Purpose: Retrieve the user's recent successful query history (user's own queries only)

Input:
{
  "limit": 20,                    // Optional: default 20, max 100
  "tables": ["orders"],           // Optional: filter by tables used
  "since": "2024-01-01"           // Optional: filter by date
}

Output:
{
  "queries": [
    {
      "id": "uuid",
      "natural_language": "Show me top 10 customers by total orders",
      "sql": "SELECT u.name, COUNT(o.id) as order_count FROM users u JOIN orders o ON o.customer_id = u.id GROUP BY u.id, u.name ORDER BY order_count DESC LIMIT 10",
      "executed_at": "2024-12-15T10:30:00Z",
      "row_count": 10,
      "execution_duration_ms": 145,
      "tables_used": ["users", "orders"],
      "query_type": "aggregation",
      "user_feedback": "helpful"
    }
  ],
  "total_count": 156,
  "has_more": true
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

#### Tool: record_query_feedback

```
Purpose: Record whether a generated query was helpful

Input:
{
  "query_id": "uuid",             // Required: from query history
  "feedback": "helpful",          // Required: "helpful", "not_helpful"
  "comment": "Had to add date filter"  // Optional
}

Output:
{
  "recorded": true,
  "message": "Feedback recorded. Thank you for helping improve query suggestions."
}

MCP Annotations:
- read_only_hint: false
- destructive_hint: false
- idempotent_hint: true
```

### 3.5 Privacy & Security ✅

- Query history is scoped by `project_id` AND `user_id` — users only see their own history
- Admins see all users' query executions via the audit UI (Phase 1), not via MCP tools
- Auto-prune records older than 90 days (admin-configurable)
- SQL queries may contain literals (dates, IDs, etc.) — stored as-is for now

### 3.6 Retention ✅

All audit and history tables share the same retention policy:
- Default: 90 days
- Admin-configurable per project
- Implemented as a scheduled cleanup job

---

## Phase 4: Alerts & Security

### 4.1 [x] Backend: Alert table, repository, service, and API endpoints

Create the database migration for `engine_mcp_audit_alerts` table in `pkg/repositories/migrations/`. The table schema:

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

Add RLS policy scoped to `app.current_project_id` consistent with other engine tables.

Create `pkg/repositories/alert_repository.go` with methods:
- `ListAlerts(ctx, projectID, filters)` — paginated list with filters for status (open/resolved/all) and severity
- `GetAlertByID(ctx, projectID, alertID)` — single alert lookup
- `CreateAlert(ctx, alert)` — insert new alert
- `ResolveAlert(ctx, projectID, alertID, resolvedBy, resolution, notes)` — update status to resolved with resolver info and timestamp

Create `pkg/services/alert_service.go` with interface and implementation wrapping the repository. The service should validate inputs (e.g., resolution must be "dismissed" or "resolved", severity must be valid enum value).

Create `pkg/handlers/alert_handler.go` with two endpoints:
- `GET /api/projects/{pid}/audit/alerts` — query params: `status` (open, resolved, all — default "open"), `severity`, `limit`, `offset`. Returns paginated alerts sorted by created_at DESC.
- `POST /api/projects/{pid}/audit/alerts/{alert_id}/resolve` — JSON body: `{ "resolution": "dismissed|resolved", "notes": "..." }`. Sets resolved_by from auth context, resolved_at to now, status to the resolution value.

Register routes in `pkg/server/routes.go` under the existing audit route group. Wire up dependency injection in the server setup following the existing pattern (see how audit_handler, audit_service, audit_repository are wired).

Add alert model types to `pkg/models/` (or in the alert_service/repository files if that matches existing patterns — check how audit models are defined).

### 4.2 [x] Backend: Alert trigger detection in audit middleware

Implement alert trigger logic that analyzes MCP audit events and creates alerts when conditions are met. This builds on the existing audit middleware from Phase 2 (`pkg/mcp/` area — check where the audit middleware and `engine_mcp_audit_log` recording lives).

Create `pkg/services/alert_trigger_service.go` (or add to existing alert service) with a method `EvaluateEvent(ctx, auditEntry)` that checks each incoming MCP audit event against alert rules. The alert types and triggers:

| Alert Type | Trigger Logic | Severity |
|---|---|---|
| `sql_injection_detected` | Check `security_flags` array on audit entry contains injection-related flags, or `security_level` is "critical" | Critical |
| `unusual_query_volume` | Count user's queries in last hour; if > 5x their rolling average (or > 50 if no baseline), trigger | Warning |
| `sensitive_table_access` | Check if query touches tables flagged as sensitive in ontology table metadata | Warning |
| `large_data_export` | Check if query result `row_count` > 10,000 | Info |
| `after_hours_access` | Check if event timestamp is outside 6am-10pm in project timezone (default UTC) | Info |
| `new_user_high_volume` | User's first query was < 24h ago AND they've run > 20 queries | Warning |
| `repeated_errors` | Same error message appears > 5 times from same user in last 10 minutes | Warning |

Call `EvaluateEvent` from the audit middleware's async recording path (after the audit log entry is written). Alert creation should not block MCP responses — run in the same goroutine that records the audit entry (it's already async).

Each alert trigger should be idempotent within a reasonable window — don't create duplicate alerts for the same condition within 1 hour. Check for existing open alerts of the same type + affected_user before creating a new one.

Severity levels per alert type are defined in the table above. Title should be human-readable (e.g., "SQL injection pattern detected", "Unusual query volume from user@example.com").

### 4.3 [ ] Frontend: Alerts tab on Audit page and alert management

Add an "Alerts" tab to the existing audit page (`ui/src/pages/AuditPage.tsx`). This tab displays alerts from `GET /api/projects/{pid}/audit/alerts`.

**TypeScript types** — add to `ui/src/types/audit.ts`:
- `AuditAlert` type with fields matching the backend model (id, project_id, alert_type, severity, title, description, affected_user_id, related_audit_ids, status, resolved_by, resolved_at, resolution_notes, created_at, updated_at)
- `ResolveAlertRequest` type: `{ resolution: 'dismissed' | 'resolved'; notes?: string }`

**API methods** — add to `ui/src/services/engineApi.ts`:
- `getAuditAlerts(projectId, params)` — GET with status/severity/limit/offset params
- `resolveAuditAlert(projectId, alertId, body)` — POST to resolve endpoint

**Alerts tab columns:** Time, Alert Type (with badge styling by severity — critical=red, warning=yellow, info=blue), Title, Affected User, Status, Resolved By, Resolution Notes (truncated).

**Filters:** Status dropdown (Open, Resolved, All — default Open), Severity dropdown (All, Critical, Warning, Info).

**Row actions:** Each open alert row should have a "Resolve" button that opens a small inline form or modal with: resolution type (dismiss/resolve radio), notes text field, and confirm button. On success, refresh the list.

**Summary integration:** Update the audit summary header (from Phase 1) to include open alert counts by severity. Add to the existing `GET /api/projects/{pid}/audit/summary` endpoint — return `open_alerts_critical`, `open_alerts_warning`, `open_alerts_info` counts. Update the summary UI component accordingly.

### 4.4 [ ] Alert configuration UI on project settings page

Add an alert configuration section to the project settings page. Check the existing project settings page location (likely `ui/src/pages/` — search for project settings or MCP server config page referenced in plan Phase 2.5 at `/projects/{pid}/mcp-server`).

**Backend:** Add a `alert_config` JSONB column to `engine_projects` table (or create a new `engine_project_alert_config` table if project settings are stored separately — check existing pattern). The config schema:

```json
{
  "alerts_enabled": true,
  "alert_settings": {
    "sql_injection_detected": { "enabled": true, "severity": "critical" },
    "unusual_query_volume": { "enabled": true, "severity": "warning", "threshold_multiplier": 5 },
    "sensitive_table_access": { "enabled": true, "severity": "warning" },
    "large_data_export": { "enabled": true, "severity": "info", "row_threshold": 10000 },
    "after_hours_access": { "enabled": false, "severity": "info", "business_hours_start": "06:00", "business_hours_end": "22:00", "timezone": "UTC" },
    "new_user_high_volume": { "enabled": true, "severity": "warning", "query_threshold": 20 },
    "repeated_errors": { "enabled": true, "severity": "warning", "error_count": 5, "window_minutes": 10 }
  }
}
```

**Backend endpoints:**
- `GET /api/projects/{pid}/audit/alert-config` — returns current config (defaults if none set)
- `PUT /api/projects/{pid}/audit/alert-config` — updates config

Add handler method to `pkg/handlers/alert_handler.go`, service method to `pkg/services/alert_service.go`, and repository method to store/retrieve config.

**Frontend:** Create an Alert Configuration section showing each alert type as a card/row with: toggle (enabled/disabled), severity dropdown, and type-specific threshold inputs (e.g., row count for large_data_export, multiplier for unusual_query_volume, time range for after_hours_access). Include a master "Alerts Enabled" toggle at the top. Save button persists all changes via PUT endpoint.

**Integration with trigger service (subtask 4.2):** The alert trigger service from subtask 4.2 must read this config before evaluating triggers — skip disabled alerts, use configured thresholds instead of hardcoded values. If subtask 4.2 is implemented first with hardcoded values, this subtask should update it to read from config.


# PLAN: Enterprise Application Suite

## Executive Summary

The Enterprise Application is a bundled suite of governance, compliance, security, and auditing features sold as a single installable "Application" in the Ekaya catalog. Enterprises buy the full suite and enable the 60-80% of features they need.

**Business Model:**
- Single SKU: "Enterprise Suite"
- All features included, individually toggleable
- Upsell path: Start with Audit → Realize they need Access Control → Enable more modules
- Integration add-ons: SIEM, SSO, Webhooks sold separately or as "Enterprise Plus"

---

## Current State Analysis

### What Exists Today

| Capability | Status | Notes |
|------------|--------|-------|
| Multi-tenant isolation | ✅ Ready | RLS on all 24 tables |
| Role definitions | ✅ Schema exists | admin/data/user roles in `engine_users` |
| Role enforcement | ❌ Not implemented | Handlers don't check roles |
| Query execution tracking | ⚠️ Partial | `usage_count` only, no who/when/params |
| LLM audit trail | ✅ Ready | Full conversation logging |
| MCP query audit | ❌ Not implemented | No dedicated table |
| API key management | ⚠️ Basic | Single key per project, no rotation |
| Application catalog | ⚠️ MVP | UI only, no installation backend |

### Key Tables to Leverage

```
engine_users (project_id, user_id, role)     -- Role assignments
engine_queries (usage_count, last_used_at)   -- Basic tracking
engine_llm_conversations                     -- LLM audit pattern to follow
engine_mcp_config                            -- Tool enablement pattern
```

---

## Enterprise Feature Modules

### Module 1: Audit & Visibility

**The "must have" - every enterprise needs this first**

| Feature | Description | Sell |
|---------|-------------|------|
| Query Execution Log | Who ran what query, when, with what params | "See every AI query hitting your database" |
| User Activity Feed | Timeline of all user actions in project | "Know who did what" |
| Data Access Tracking | Which tables/columns were accessed | "Track sensitive data exposure" |
| Session Tracking | Group activity by session/conversation | "Understand full AI interactions" |
| Export to CSV/JSON | Compliance-ready exports | "SOC2 auditors love us" |
| Retention Policy | Configure how long query history is retained | "Control storage costs & compliance" |
| Data Minimization Alerts | Flag AI queries that fetch more columns/rows than necessary for their purpose | "Prove you're only accessing what you need" |
| Regulatory Report Templates | GDPR Article 30, EU AI Act, SOC 2 formatted exports on demand | "One click to auditor-ready reports" |
| AI System Registry | Fingerprint and catalog every AI system/service querying the database | "Know every AI touching your data" |

**Data Model:**

```sql
CREATE TABLE engine_audit_events (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    -- Who
    user_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(255),
    user_role VARCHAR(50),
    session_id VARCHAR(255),

    -- What
    event_type VARCHAR(50) NOT NULL,  -- 'query_executed', 'query_created', 'config_changed', etc.
    resource_type VARCHAR(50),        -- 'query', 'datasource', 'ontology', 'mcp_config'
    resource_id UUID,

    -- Details
    action VARCHAR(50),               -- 'execute', 'create', 'update', 'delete', 'view'
    request_data JSONB,               -- Sanitized input (params, etc.)
    response_summary JSONB,           -- {row_count, tables_accessed, duration_ms}

    -- Context
    natural_language TEXT,            -- If query came from NL prompt
    sql_executed TEXT,                -- The actual SQL (truncated if huge)
    tables_accessed TEXT[],           -- ['users', 'orders']

    -- Outcome
    was_successful BOOLEAN DEFAULT true,
    error_message TEXT,
    duration_ms INTEGER,
    rows_returned INTEGER,

    -- Classification
    security_level VARCHAR(20) DEFAULT 'normal',  -- 'normal', 'warning', 'alert'
    data_classifications TEXT[],      -- ['pii', 'financial', 'sensitive']

    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Partitioning for scale
    partition_date DATE DEFAULT CURRENT_DATE
) PARTITION BY RANGE (partition_date);
```

**UI Components:**
- Dashboard tile: "Audit" with query count, user count, alert badge
- Full audit log page with filters (user, date range, event type, security level)
- Event detail modal showing full context
- Export button (CSV/JSON)

---

### Module 2: Access Control (RBAC)

**The "aha moment" - enterprises realize they need this after seeing audit data**

| Feature | Description | Sell |
|---------|-------------|------|
| Role Enforcement | Restrict actions by role | "Finance sees finance queries only" |
| Query-Level Permissions | Control who can execute which queries | "Self-service without chaos" |
| Data Classification | Tag queries with sensitivity levels | "Know where your PII flows" |
| Column Masking | Hide sensitive columns from certain roles | "SSN visible to admins only" |
| Consent Boundary Enforcement | Cross-reference AI data access against user consent records; flag access outside consented scope | "AI respects what users agreed to" |

**Data Model Extensions:**

```sql
-- Query-level permissions
CREATE TABLE engine_query_permissions (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    query_id UUID NOT NULL REFERENCES engine_queries(id),

    -- Who can access
    permission_type VARCHAR(20) NOT NULL,  -- 'role', 'user', 'all'
    role VARCHAR(50),                       -- 'admin', 'data', 'user'
    user_id VARCHAR(255),                   -- Specific user override

    -- What they can do
    can_view BOOLEAN DEFAULT true,
    can_execute BOOLEAN DEFAULT true,
    can_edit BOOLEAN DEFAULT false,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Data classification on queries
ALTER TABLE engine_queries ADD COLUMN
    data_classifications TEXT[] DEFAULT '{}';  -- ['pii', 'financial', 'internal']

-- Column masking rules
CREATE TABLE engine_column_masks (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    datasource_id UUID NOT NULL,

    table_name VARCHAR(255) NOT NULL,
    column_name VARCHAR(255) NOT NULL,

    -- Classification
    classification VARCHAR(50) NOT NULL,  -- 'pii', 'financial', 'sensitive'

    -- Masking rule
    mask_type VARCHAR(50) NOT NULL,       -- 'redact', 'partial', 'hash', 'null'
    mask_config JSONB,                    -- {"show_last": 4} for partial SSN

    -- Who sees masked vs real
    visible_to_roles TEXT[] DEFAULT '{admin}',

    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

**Enforcement Points:**
1. Query list endpoint filters by user's permissions
2. Query execute endpoint checks `can_execute`
3. Result formatter applies column masks based on user role
4. MCP tools respect same permissions

---

### Module 3: Governance & Workflow

**The "mature enterprise" - approval workflows, change management**

| Feature | Description | Sell |
|---------|-------------|------|
| Query Approval Workflow | Suggested queries require admin approval | "No rogue queries in production" |
| Change Audit Trail | Who modified what query, when | "Full change history" |
| Version History | Previous versions of queries preserved | "Roll back mistakes" |
| Data Lineage | Track query → report → dashboard | "Impact analysis" |
| DPIA Trigger Workflow | Auto-detect when AI access patterns change enough to require a Data Protection Impact Assessment; pre-populate with observed data | "Never miss a required impact assessment" |
| Human Override Tracking | Log when AI-driven decisions are reviewed by humans vs. auto-approved; verify oversight is actually happening | "Prove humans are in the loop" |

**Data Model:**

```sql
-- Query change history
CREATE TABLE engine_query_versions (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    query_id UUID NOT NULL REFERENCES engine_queries(id),

    version_number INTEGER NOT NULL,

    -- Snapshot of query at this version
    natural_language_prompt TEXT,
    sql_query TEXT,
    parameters JSONB,

    -- Who/when
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    change_reason TEXT
);

-- Query suggestions (from MCP clients when allowClientSuggestions=true)
CREATE TABLE engine_query_suggestions (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    datasource_id UUID NOT NULL,

    -- The suggestion
    suggested_by VARCHAR(255) NOT NULL,   -- User who suggested
    suggested_at TIMESTAMPTZ DEFAULT NOW(),
    natural_language_prompt TEXT NOT NULL,
    suggested_sql TEXT,

    -- Approval workflow
    status VARCHAR(20) DEFAULT 'pending',  -- 'pending', 'approved', 'rejected'
    reviewed_by VARCHAR(255),
    reviewed_at TIMESTAMPTZ,
    review_notes TEXT,

    -- If approved, link to created query
    approved_query_id UUID REFERENCES engine_queries(id)
);

-- Data lineage (where queries are used)
CREATE TABLE engine_query_lineage (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    query_id UUID NOT NULL REFERENCES engine_queries(id),

    -- Where it's used
    consumer_type VARCHAR(50) NOT NULL,   -- 'dashboard', 'report', 'export', 'api'
    consumer_name VARCHAR(255),
    consumer_id VARCHAR(255),             -- External ID if applicable

    -- Metadata
    description TEXT,
    created_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

### Module 4: Security & Threat Detection

**The "security team buy-in" - anomaly detection, rate limiting**

| Feature | Description | Sell |
|---------|-------------|------|
| SQL Injection Detection | Detect injection attempts in parameters | "AI can't be tricked" |
| Rate Limiting | Per-user, per-query limits | "Prevent runaway costs" |
| Anomaly Detection | Flag unusual access patterns | "Know before breach" |
| API Key Management | Rotation, scoping, expiration | "Zero trust API access" |
| Risk Scoring | Real-time risk score per AI query based on data sensitivity, volume, identity, and human oversight presence | "Quantify your AI data risk" |
| Regulatory Incident Drafts | Pre-draft incident reports when AI access patterns trigger policy violations; formatted for supervisory authorities | "Ready for regulators in minutes, not weeks" |

**Data Model:**

```sql
-- Security alerts
CREATE TABLE engine_security_alerts (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    alert_type VARCHAR(50) NOT NULL,     -- 'injection', 'anomaly', 'rate_limit', 'unauthorized'
    severity VARCHAR(20) NOT NULL,       -- 'info', 'warning', 'critical'

    -- Details
    title VARCHAR(255) NOT NULL,
    description TEXT,
    affected_user_id VARCHAR(255),
    related_event_ids UUID[],            -- Links to audit_events

    -- Evidence
    evidence JSONB,                      -- {pattern_matched, threshold_exceeded, etc.}

    -- Resolution
    status VARCHAR(20) DEFAULT 'open',   -- 'open', 'acknowledged', 'resolved', 'dismissed'
    resolved_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,
    resolution_notes TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Rate limit configuration
CREATE TABLE engine_rate_limits (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    -- Scope
    limit_type VARCHAR(50) NOT NULL,     -- 'global', 'per_user', 'per_query', 'per_role'
    user_id VARCHAR(255),                -- If per_user
    query_id UUID,                       -- If per_query
    role VARCHAR(50),                    -- If per_role

    -- Limits
    max_requests INTEGER NOT NULL,
    window_seconds INTEGER NOT NULL,     -- 60 = per minute, 3600 = per hour
    max_rows_per_window INTEGER,         -- Optional: cap total rows returned

    -- Status
    is_enabled BOOLEAN DEFAULT true,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- API keys (enhanced from current single key)
CREATE TABLE engine_api_keys (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    -- Key details
    name VARCHAR(255) NOT NULL,          -- "Production Agent", "Dev Testing"
    key_hash VARCHAR(255) NOT NULL,      -- SHA256 of actual key
    key_prefix VARCHAR(10) NOT NULL,     -- First 8 chars for identification

    -- Scope
    scoped_to_queries UUID[],            -- Empty = all queries
    scoped_to_roles TEXT[],              -- Empty = all roles

    -- Lifecycle
    created_by VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    is_revoked BOOLEAN DEFAULT false,
    revoked_at TIMESTAMPTZ,
    revoked_by VARCHAR(255),

    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

### Module 5: Integrations

**The "enterprise plus" add-on - connect to existing tools**

| Integration | Description | Sell |
|-------------|-------------|------|
| SIEM Export | Stream events to Splunk, Datadog, Sumo Logic | "One security pane of glass" |
| Grafana/Prometheus | Metrics export for dashboards | "Ops visibility" |
| Slack/Teams | Alert notifications | "Know immediately" |
| Webhooks | Custom integrations | "Build your own" |
| SSO/SAML | Enterprise identity | "One login" |

**Data Model:**

```sql
CREATE TABLE engine_integrations (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    integration_type VARCHAR(50) NOT NULL,  -- 'siem', 'monitoring', 'alerting', 'webhook', 'sso'
    provider VARCHAR(50) NOT NULL,          -- 'splunk', 'datadog', 'grafana', 'slack', 'okta'

    -- Configuration (encrypted)
    config_encrypted TEXT NOT NULL,         -- {endpoint, api_key, etc.}

    -- What to send
    event_types TEXT[],                     -- ['query_executed', 'security_alert', 'all']

    -- Status
    is_enabled BOOLEAN DEFAULT true,
    last_sync_at TIMESTAMPTZ,
    last_error TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Webhook delivery tracking
CREATE TABLE engine_webhook_deliveries (
    id UUID PRIMARY KEY,
    integration_id UUID NOT NULL REFERENCES engine_integrations(id),

    -- What was sent
    event_type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,

    -- Delivery status
    status VARCHAR(20) NOT NULL,           -- 'pending', 'delivered', 'failed', 'retrying'
    attempts INTEGER DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    response_code INTEGER,
    response_body TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Application Installation Model

### How Applications Work

When admin installs "Enterprise Suite":

```sql
-- Track installed applications
CREATE TABLE engine_applications (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    app_id VARCHAR(100) NOT NULL,         -- 'enterprise-suite'
    app_version VARCHAR(50) NOT NULL,     -- '1.0.0'

    -- Installation state
    status VARCHAR(20) DEFAULT 'active',  -- 'active', 'suspended', 'uninstalled'
    installed_by VARCHAR(255) NOT NULL,
    installed_at TIMESTAMPTZ DEFAULT NOW(),

    -- Module enablement (the 60-80% they use)
    enabled_modules TEXT[] DEFAULT '{}',  -- ['audit', 'access_control', 'governance']

    -- Billing reference
    subscription_id VARCHAR(255),

    UNIQUE(project_id, app_id)
);

-- Module-specific settings
CREATE TABLE engine_application_settings (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    app_id VARCHAR(100) NOT NULL,
    module_id VARCHAR(100) NOT NULL,      -- 'audit', 'access_control', etc.

    settings JSONB NOT NULL DEFAULT '{}',

    updated_by VARCHAR(255),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(project_id, app_id, module_id)
);
```

### Installation Flow

1. Admin clicks "Install" on Enterprise Suite in catalog
2. Backend creates `engine_applications` record
3. Migrations check for app installation before creating tables
4. UI shows Enterprise features only when app installed
5. Module toggles enable/disable individual features

---

## UI Design

### Project Dashboard Tile

```
┌─────────────────────────────────────────┐
│ Enterprise Suite                         │
│                                          │
│ ● Audit           ● Access Control       │
│ ○ Governance      ○ Security             │
│ ○ Integrations                           │
│                                          │
│ 1,234 events (7d)  ⚠️ 2 alerts           │
│                                          │
│ [Configure]                              │
└─────────────────────────────────────────┘
```

### Enterprise Settings Page

```
/projects/{pid}/enterprise

┌─────────────────────────────────────────────────────────────────────────────┐
│ Enterprise Suite                                              [v1.2.0]      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ Modules                                                                     │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ ✅ Audit & Visibility                                    [Configure]   │ │
│ │    Track all queries, user activity, and data access                   │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ ✅ Access Control                                        [Configure]   │ │
│ │    Role-based permissions, query restrictions, column masking          │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ ☐ Governance & Workflow                                  [Enable]      │ │
│ │    Approval workflows, version history, change tracking                │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ ☐ Security & Threat Detection                            [Enable]      │ │
│ │    SQL injection detection, anomaly alerts, rate limiting              │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ ☐ Integrations                                           [Enable]      │ │
│ │    SIEM, Grafana, Slack, Webhooks, SSO                                 │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Quick Stats (last 7 days)                                                   │
│ ┌──────────────┬──────────────┬──────────────┬──────────────┐              │
│ │ Queries      │ Users        │ Alerts       │ Blocked      │              │
│ │ 1,234        │ 12           │ 2 ⚠️          │ 3            │              │
│ └──────────────┴──────────────┴──────────────┴──────────────┘              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Audit Log Page

```
/projects/{pid}/enterprise/audit

┌─────────────────────────────────────────────────────────────────────────────┐
│ Audit Log                                                    [Export CSV]   │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Last 7 days ▼] [All Users ▼] [All Events ▼] [🔍 Search...]                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ Today                                                                       │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ 10:45 AM  john@acme.com  executed  "Monthly Sales Report"        145ms │ │
│ │           → 847 rows from [orders, customers]                          │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ 10:42 AM  john@acme.com  executed  "Customer Lookup"              23ms │ │
│ │           → 1 row from [customers]  params: {customer_id: "abc123"}    │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ 10:38 AM  jane@acme.com  created   "New Revenue Query"                 │ │
│ │           → Added by admin                                              │ │
│ ├─────────────────────────────────────────────────────────────────────────┤ │
│ │ 10:15 AM  jane@acme.com  blocked   "Direct SQL"                   ⚠️   │ │
│ │           → Attempted access to restricted table [salaries]            │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ [Load More...]                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Pricing Strategy

### Tier Structure

| Tier | Modules | Target |
|------|---------|--------|
| **Essentials** | Audit only | "We need visibility" |
| **Professional** | Audit + Access Control + Governance | "We need compliance" |
| **Enterprise** | All modules | "We need everything" |
| **Enterprise Plus** | All + Integrations + Premium Support | "We need integrations" |

### Module Bundling Logic

```
Essentials ($X/mo):
  ✅ Audit & Visibility

Professional ($2X/mo):
  ✅ Audit & Visibility
  ✅ Access Control
  ✅ Governance & Workflow

Enterprise ($3X/mo):
  ✅ Audit & Visibility
  ✅ Access Control
  ✅ Governance & Workflow
  ✅ Security & Threat Detection

Enterprise Plus ($4X/mo):
  ✅ Everything above
  ✅ SIEM Integration
  ✅ Monitoring Integration
  ✅ SSO/SAML
  ✅ Premium Support
```

### Upsell Triggers

1. **Audit → Access Control**: "You have 5 users querying sensitive tables without restrictions"
2. **Access Control → Governance**: "12 queries were modified last month with no audit trail"
3. **Any → Security**: "We detected 3 suspicious query patterns"
4. **Any → Integrations**: "Export your audit data to your existing SIEM"

---

## Implementation Phases

### Phase 1: Foundation (Week 1-2)

**Goal:** Application installation framework + Audit core

1. Create `engine_applications` and `engine_application_settings` tables
2. Implement application installation API
3. Update Applications Catalog UI to install Enterprise Suite
4. Create `engine_audit_events` table with partitioning
5. Add audit middleware to query execution handlers
6. Build basic Audit Log UI page

**Files:**
- `migrations/XXX_create_applications.sql`
- `migrations/XXX_create_audit_events.sql`
- `pkg/models/application.go`
- `pkg/models/audit.go`
- `pkg/repositories/application_repository.go`
- `pkg/repositories/audit_repository.go`
- `pkg/services/application_service.go`
- `pkg/services/audit_service.go`
- `pkg/middleware/audit.go`
- `pkg/handlers/applications.go` (enhance existing)
- `pkg/handlers/audit.go`
- `ui/src/pages/EnterprisePage.tsx`
- `ui/src/pages/AuditLogPage.tsx`

### Phase 2: Access Control (Week 3-4)

**Goal:** RBAC enforcement + Query permissions

1. Create `engine_query_permissions` table
2. Create `engine_column_masks` table
3. Add data_classifications to queries
4. Implement permission checks in query handlers
5. Implement column masking in result formatter
6. Build Access Control settings UI

**Files:**
- `migrations/XXX_create_query_permissions.sql`
- `migrations/XXX_create_column_masks.sql`
- `pkg/models/permissions.go`
- `pkg/services/permission_service.go`
- `pkg/services/masking_service.go`
- `ui/src/pages/AccessControlPage.tsx`

### Phase 3: Governance (Week 5-6)

**Goal:** Approval workflows + Version history

1. Create `engine_query_versions` table
2. Create `engine_query_suggestions` table
3. Implement version tracking on query updates
4. Implement suggestion → approval workflow
5. Build Governance UI (suggestions list, approval flow)

**Files:**
- `migrations/XXX_create_query_versions.sql`
- `migrations/XXX_create_query_suggestions.sql`
- `pkg/models/governance.go`
- `pkg/services/governance_service.go`
- `pkg/handlers/suggestions.go`
- `ui/src/pages/GovernancePage.tsx`

### Phase 4: Security (Week 7-8)

**Goal:** Alerts + Rate limiting + API keys

1. Create `engine_security_alerts` table
2. Create `engine_rate_limits` table
3. Create `engine_api_keys` table (enhanced)
4. Implement alert detection logic
5. Implement rate limiting middleware
6. Implement API key rotation
7. Build Security settings UI

**Files:**
- `migrations/XXX_create_security_alerts.sql`
- `migrations/XXX_create_rate_limits.sql`
- `migrations/XXX_create_api_keys.sql`
- `pkg/models/security.go`
- `pkg/services/alert_service.go`
- `pkg/services/rate_limit_service.go`
- `pkg/services/api_key_service.go`
- `pkg/middleware/rate_limit.go`
- `ui/src/pages/SecurityPage.tsx`

### Phase 5: Integrations (Week 9-10)

**Goal:** SIEM + Webhooks + Alerting

1. Create `engine_integrations` table
2. Create `engine_webhook_deliveries` table
3. Implement SIEM export (Splunk HEC, Datadog)
4. Implement webhook delivery with retry
5. Implement Slack/email notifications
6. Build Integrations settings UI

**Files:**
- `migrations/XXX_create_integrations.sql`
- `pkg/models/integration.go`
- `pkg/services/integration_service.go`
- `pkg/services/webhook_service.go`
- `pkg/services/notification_service.go`
- `ui/src/pages/IntegrationsPage.tsx`

---

## API Endpoints

### Application Management

```
GET    /api/projects/{pid}/applications              → List installed apps
POST   /api/projects/{pid}/applications              → Install app
GET    /api/projects/{pid}/applications/{appId}      → Get app details
PATCH  /api/projects/{pid}/applications/{appId}      → Update (enable/disable modules)
DELETE /api/projects/{pid}/applications/{appId}      → Uninstall app

GET    /api/projects/{pid}/applications/{appId}/settings/{module}
PUT    /api/projects/{pid}/applications/{appId}/settings/{module}
```

### Audit

```
GET    /api/projects/{pid}/enterprise/audit/events   → List events (with filters)
GET    /api/projects/{pid}/enterprise/audit/events/{id} → Event detail
GET    /api/projects/{pid}/enterprise/audit/summary  → Stats for dashboard
GET    /api/projects/{pid}/enterprise/audit/export   → CSV/JSON export
```

### Access Control

```
GET    /api/projects/{pid}/enterprise/permissions                    → List all permissions
GET    /api/projects/{pid}/enterprise/permissions/queries/{qid}     → Permissions for query
PUT    /api/projects/{pid}/enterprise/permissions/queries/{qid}     → Set query permissions

GET    /api/projects/{pid}/enterprise/masks                          → List column masks
POST   /api/projects/{pid}/enterprise/masks                          → Create mask
DELETE /api/projects/{pid}/enterprise/masks/{id}                     → Remove mask
```

### Governance

```
GET    /api/projects/{pid}/enterprise/suggestions                    → List suggestions
POST   /api/projects/{pid}/enterprise/suggestions                    → Create suggestion
POST   /api/projects/{pid}/enterprise/suggestions/{id}/approve       → Approve
POST   /api/projects/{pid}/enterprise/suggestions/{id}/reject        → Reject

GET    /api/projects/{pid}/enterprise/queries/{qid}/versions         → Version history
POST   /api/projects/{pid}/enterprise/queries/{qid}/rollback/{ver}  → Rollback
```

### Security

```
GET    /api/projects/{pid}/enterprise/alerts                         → List alerts
PATCH  /api/projects/{pid}/enterprise/alerts/{id}                    → Update status

GET    /api/projects/{pid}/enterprise/rate-limits                    → List limits
POST   /api/projects/{pid}/enterprise/rate-limits                    → Create limit
DELETE /api/projects/{pid}/enterprise/rate-limits/{id}               → Remove limit

GET    /api/projects/{pid}/enterprise/api-keys                       → List keys
POST   /api/projects/{pid}/enterprise/api-keys                       → Create key
POST   /api/projects/{pid}/enterprise/api-keys/{id}/rotate           → Rotate key
DELETE /api/projects/{pid}/enterprise/api-keys/{id}                  → Revoke key
```

### Integrations

```
GET    /api/projects/{pid}/enterprise/integrations                   → List integrations
POST   /api/projects/{pid}/enterprise/integrations                   → Create integration
PATCH  /api/projects/{pid}/enterprise/integrations/{id}              → Update
DELETE /api/projects/{pid}/enterprise/integrations/{id}              → Remove
POST   /api/projects/{pid}/enterprise/integrations/{id}/test         → Test connection
```

---

## Feature Flags

Use `engine_applications.enabled_modules` to gate features:

```go
func (s *EnterpriseService) IsModuleEnabled(ctx context.Context, projectID uuid.UUID, module string) bool {
    app, err := s.appRepo.GetByProjectAndAppID(ctx, projectID, "enterprise-suite")
    if err != nil || app == nil {
        return false
    }
    return slices.Contains(app.EnabledModules, module)
}

// Usage in handlers
if !s.enterprise.IsModuleEnabled(ctx, projectID, "access_control") {
    return nil, ErrModuleNotEnabled
}
```

---

## Success Metrics

### Adoption
- % of projects with Enterprise Suite installed
- % of available modules enabled per project
- Time from install to first module enabled

### Engagement
- Audit events per project per day
- Alert acknowledgment rate
- Integration delivery success rate

### Upsell
- Conversion rate: Essentials → Professional
- Conversion rate: Professional → Enterprise
- Time to upgrade

### Value
- Blocked queries (value of access control)
- Alerts generated (value of security)
- Compliance exports downloaded (value of audit)

---

## Open Questions

1. **Pricing model** - Per project? Per user? Per query volume?
2. **Free tier** - Should Audit have a free tier with limited retention?
3. **Backfill** - When enterprise is installed, should we backfill historical query usage_count data into audit?
4. **SSO** - Build custom or use Auth0/Okta integration?
5. **Multi-region** - Should audit data stay in same region as datasource?

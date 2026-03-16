# DESIGN: Data Guardian — Integrations & Infrastructure

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Integration applets that connect Data Guardian to external security and monitoring tools, plus infrastructure applets for API management and rate limiting.

---

## Applets

### 1. SIEM Export

**Type:** Integration (streaming)
**Migrated from:** `PLAN-app-enterprise.md` Module 5

Stream audit events and security alerts to enterprise SIEM platforms:
- Splunk (HEC)
- Datadog
- Sumo Logic

---

### 2. Grafana/Prometheus Metrics

**Type:** Integration (pull-based)
**Migrated from:** `PLAN-app-enterprise.md` Module 5

Export Guardian metrics in Prometheus format for Grafana dashboards:
- Query volume by user/role
- Alert counts by severity
- Blocked query rate
- Data access patterns

---

### 3. Slack/Teams Alert Notifications

**Type:** Integration (push)
**Migrated from:** `PLAN-app-enterprise.md` Module 5

Route alert notifications to Slack channels or Microsoft Teams. Configurable:
- Which severity levels trigger notifications
- Which channels receive which alert types
- Notification formatting (summary vs. detailed)

---

### 4. Webhooks

**Type:** Integration (push)
**Migrated from:** `PLAN-app-enterprise.md` Module 5

Custom webhook endpoints for any event type. Delivery tracking with retry logic.

---

### 5. SSO/SAML

**Type:** Integration
**Migrated from:** `PLAN-app-enterprise.md` Module 5

Enterprise identity integration for single sign-on.

---

### 6. API Key Management

**Type:** Admin
**Migrated from:** `PLAN-app-enterprise.md` Module 4

Enhanced API key system beyond the current single-key-per-project model.

**Key capabilities:**
- Named keys ("Production Agent", "Dev Testing")
- Key rotation with zero-downtime
- Expiration and automatic revocation
- Scoping to specific queries or roles
- Last-used tracking

---

### 7. Rate Limiting

**Type:** Policy enforcement
**Migrated from:** `PLAN-app-enterprise.md` Module 4

Configurable rate limits to prevent abuse and runaway costs.

**Scoping options:**
- Per-user limits
- Per-query limits
- Per-role limits
- Global project limits

**Limit types:**
- Max requests per window (60s, 3600s, etc.)
- Max rows returned per window

---

## Data Models

### Integrations

```sql
CREATE TABLE engine_integrations (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    integration_type VARCHAR(50) NOT NULL,  -- 'siem', 'monitoring', 'alerting', 'webhook', 'sso'
    provider VARCHAR(50) NOT NULL,          -- 'splunk', 'datadog', 'grafana', 'slack', 'okta'
    config_encrypted TEXT NOT NULL,         -- {endpoint, api_key, etc.}
    event_types TEXT[],                     -- ['query_executed', 'security_alert', 'all']
    is_enabled BOOLEAN DEFAULT true,
    last_sync_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE engine_webhook_deliveries (
    id UUID PRIMARY KEY,
    integration_id UUID NOT NULL REFERENCES engine_integrations(id),
    event_type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL,           -- 'pending', 'delivered', 'failed', 'retrying'
    attempts INTEGER DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    response_code INTEGER,
    response_body TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Rate Limits

```sql
CREATE TABLE engine_rate_limits (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    limit_type VARCHAR(50) NOT NULL,     -- 'global', 'per_user', 'per_query', 'per_role'
    user_id VARCHAR(255),
    query_id UUID,
    role VARCHAR(50),
    max_requests INTEGER NOT NULL,
    window_seconds INTEGER NOT NULL,
    max_rows_per_window INTEGER,
    is_enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Enhanced API Keys

```sql
CREATE TABLE engine_api_keys (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(10) NOT NULL,
    scoped_to_queries UUID[],
    scoped_to_roles TEXT[],
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

## API Endpoints

```
GET    /api/projects/{pid}/guardian/integrations             → List integrations
POST   /api/projects/{pid}/guardian/integrations             → Create integration
PATCH  /api/projects/{pid}/guardian/integrations/{id}        → Update
DELETE /api/projects/{pid}/guardian/integrations/{id}        → Remove
POST   /api/projects/{pid}/guardian/integrations/{id}/test   → Test connection

GET    /api/projects/{pid}/guardian/rate-limits              → List limits
POST   /api/projects/{pid}/guardian/rate-limits              → Create limit
DELETE /api/projects/{pid}/guardian/rate-limits/{id}         → Remove limit

GET    /api/projects/{pid}/guardian/api-keys                 → List keys
POST   /api/projects/{pid}/guardian/api-keys                 → Create key
POST   /api/projects/{pid}/guardian/api-keys/{id}/rotate     → Rotate key
DELETE /api/projects/{pid}/guardian/api-keys/{id}            → Revoke key
```

---

## Related Plans

- `plans/guardian/PLAN-app-enterprise.md` — Enterprise suite Module 4 (Security) and Module 5 (Integrations)

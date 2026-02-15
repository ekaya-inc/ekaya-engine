# SECURITY: Sensitive Data Detection and Admin Approval

## Problem Statement

During MCP API testing, we discovered that `get_context` with `include: ["sample_values"]` exposes sensitive data. The `agent_data` column in the `users` table revealed LiveKit API keys and secrets in plaintext:

```json
{
  "column_name": "agent_data",
  "sample_values": [
    "{\"livekit_api_key\":\"API67e2wiyw3KvB\",\"livekit_api_secret\":\"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i\"}"
  ]
}
```

This is a critical security issue. However, simply blocking all potentially sensitive columns isn't the right solution - admins may legitimately need certain PII (like email addresses) for analytics while wanting to exclude others (like API keys).

## Proposed Solution

### Phase 1: Detection During Ontology Extraction

During column scanning (ontology extraction phase), detect potentially sensitive data using:

**Column Name Patterns** (case-insensitive):
- `(?i)(api[_-]?key|apikey)`
- `(?i)(api[_-]?secret|apisecret)`
- `(?i)(password|passwd|pwd)`
- `(?i)(secret[_-]?key|secretkey)`
- `(?i)(access[_-]?token|accesstoken)`
- `(?i)(private[_-]?key|privatekey)`
- `(?i)(credential|cred)`
- `(?i)(ssn|social_security)`
- `(?i)credit_card`

**Content Patterns** (scan sample values for):
- JSON keys containing: `api_key`, `api_secret`, `password`, `token`, `secret`, `credential`, `private_key`
- Patterns matching: API keys, JWT tokens, connection strings
- PII patterns: email addresses, phone numbers, SSN formats

**Classification Categories**:
| Category | Examples | Default Action |
|----------|----------|----------------|
| `secrets` | API keys, tokens, passwords | Block |
| `pii_identity` | SSN, passport numbers | Block |
| `pii_contact` | Email, phone, address | Flag for review |
| `pii_financial` | Credit card, bank account | Block |

### Phase 2: Surface in Schema Tile as Badge

On the project dashboard, the **Schema** tile should display a badge when sensitive columns are detected:

```
┌─────────────────────────────────┐
│  [Schema Icon]                  │
│                                 │
│  Schema            ⚠️ 3 flagged │
│                                 │
└─────────────────────────────────┘
```

Badge states:
- **Orange warning badge**: `⚠️ N flagged` - N columns detected as potentially sensitive, awaiting admin review
- **No badge**: All sensitive columns have been reviewed (approved or rejected)

Clicking the tile navigates to Schema Selection with sensitive columns highlighted.

### Phase 3: Schema Selection Screen with Approval/Rejection

In the Schema Selection screen, columns flagged as sensitive should be visually distinct:

```
┌─────────────────────────────────────────────────────────────────────┐
│ Schema Selection                                                     │
│ Select the tables and columns to include in your ontology           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│ ☑️ users                                              17 columns     │
│   ├─ ☑️ user_id                                                      │
│   ├─ ☑️ email                    🔶 PII: Contact     [Allow] [Block] │
│   ├─ ☑️ username                                                     │
│   ├─ ☐ agent_data               🔴 Secret detected  [Allow] [Block] │
│   └─ ...                                                             │
│                                                                      │
│ ☑️ accounts                                          15 columns     │
│   ├─ ☑️ account_id                                                   │
│   ├─ ☑️ email                    🔶 PII: Contact     [Allow] [Block] │
│   └─ ...                                                             │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

**Visual indicators**:
- 🔴 **Red badge** (`Secret detected`): High-confidence secrets (API keys, passwords)
- 🔶 **Orange badge** (`PII: Contact`): PII that may be needed for analytics
- `[Allow]` button: Explicitly approve this column for MCP access
- `[Block]` button: Exclude from sample_values and potentially from schema

**Behavior**:
- Columns marked `secrets` are **unchecked by default** (opt-in)
- Columns marked `pii_contact` are **checked but flagged** (opt-out)
- Admin decisions are **persisted** so re-extraction doesn't re-flag

### Phase 4: Persist Admin Decisions

Store admin decisions in the database so they survive re-extraction:

```sql
CREATE TABLE engine_sensitive_column_decisions (
    id BIGSERIAL PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES engine_projects(project_id),
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    column_name TEXT NOT NULL,
    detection_category TEXT NOT NULL,  -- 'secrets', 'pii_identity', 'pii_contact', 'pii_financial'
    detection_reason TEXT,             -- 'column_name_match', 'content_pattern', 'json_key_match'
    admin_decision TEXT NOT NULL,      -- 'allow', 'block', 'pending'
    decided_at TIMESTAMPTZ,
    decided_by TEXT,                   -- admin user ID
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(project_id, schema_name, table_name, column_name)
);
```

**On re-extraction**:
1. Scan columns for sensitive patterns
2. Check `engine_sensitive_column_decisions` for existing decisions
3. If decision exists → honor it silently
4. If no decision → flag for admin review (new detection)
5. If column no longer exists → mark decision as stale

### Phase 5: MCP Tool Behavior

**`get_context` with `include: ["sample_values"]`**:
- Check `admin_decision` for each column
- If `block` → exclude from sample_values, add note: `"sample_values": "[BLOCKED: Admin decision]"`
- If `allow` → include sample_values normally
- If `pending` → redact and add note: `"sample_values": "[PENDING REVIEW]"`

**`probe_column`**:
- Same logic as above
- Include `sensitive_status` in response: `{"sensitive": true, "category": "secrets", "decision": "blocked"}`

## User Flow Example

1. **Initial Setup**: Admin connects tikr_production database
2. **Extraction**: System scans columns, detects `agent_data` contains API keys
3. **Dashboard**: Schema tile shows `⚠️ 1 flagged`
4. **Review**: Admin clicks Schema tile, sees `agent_data` flagged as `🔴 Secret detected`
5. **Decision**: Admin clicks `[Block]` for `agent_data`, `[Allow]` for `email`
6. **Persistence**: Decisions saved to database
7. **MCP Usage**: Agents calling `get_context` see `email` values but `agent_data` shows `[BLOCKED]`
8. **Re-extraction**: Next ontology extraction honors saved decisions, no re-flagging

## Implementation Priority

1. **P0**: Detection logic during extraction (reuse sensitive.go from Issue #1)
2. **P0**: Persist decisions in database
3. **P1**: Schema tile badge on dashboard
4. **P1**: Schema Selection UI with Allow/Block buttons
5. **P2**: MCP tool integration (honor decisions in get_context, probe_column)

## Related Plans

- ISSUES-ontology-benchmark-2026-01-30.md #1: Sensitive Data Exposure in sample_values
- Subtasks 1.1, 1.2, 1.3 cover the redaction implementation
- **PLAN-alerts-tile-and-screen.md** — The `sensitive_table_access` alert trigger (Question 4) depends on this plan's detection and admin decision infrastructure. Implement together so that columns/tables marked sensitive here feed into the alert trigger there.

## Open Questions

1. Should blocked columns be excluded from schema entirely, or just have sample_values redacted?
2. Should there be a "super-admin" override to see blocked data?
3. How to handle columns where only some values are sensitive (mixed content)?
4. Should detection run on every query or only during extraction?

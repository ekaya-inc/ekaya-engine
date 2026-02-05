# WISHLIST: Business User Experience Improvements

**Source:** tikr-all ontology benchmarking session (2026-02-05)
**Goal:** Get business user (Marketing Manager, Finance, Support) experience from 7/10 → 9/10
**Key Insight:** Technical users can recover from errors; business users cannot.

---

## The Scenario

I (Claude Code) roleplayed as different business roles asking ad-hoc questions against tikr_production:

| Role | Question Asked | What Happened |
|------|----------------|---------------|
| Marketing Manager | "How many new hosts signed up this month?" | ✅ Worked |
| Finance | "Revenue by offer type?" | ✅ Worked (knew enum labels from benchmark) |
| Support | "Recent reports and blocks?" | ✅ Worked |
| Product | "Engagement funnel metrics?" | ⚠️ Tried wrong column (billing_state) |
| Operations | "Server capacity status?" | ❌ Guessed wrong table name (participant_servers vs e_servers) |
| Email Marketing | "Email send success rate?" | ❌ Guessed wrong table, assumed deleted_at existed |

**Pattern:** When I guessed wrong, I could recover using `search_schema()` and SQL knowledge. A Marketing Manager cannot do this.

---

## Current Rating Breakdown

| Category | Score | Blocker for Business Users |
|----------|-------|---------------------------|
| Schema Discovery | 8/10 | Requires knowing to call search_schema |
| Query Execution | 9/10 | - |
| Ontology Value | 7/10 | Context not surfaced at query time |
| Table/Column Guessing | 5/10 | **Hallucinations kill trust** |
| Self-Service for Non-Technical | 4/10 | **Requires SQL knowledge** |

---

## Feature Wishlist

### P0: Pre-Approved Query Library (Admin → Business User Bridge)

**The Model:**
- Admins create and approve parameterized queries
- Business users execute them by name with parameters
- No SQL knowledge required

**What I Want:**

#### 1. Query Discovery by Intent
```
User: "I want to see revenue metrics"
Ekaya: Here are approved queries matching "revenue":
  - revenue_by_offer_type(start_date, end_date)
  - revenue_by_host(host_id)
  - monthly_revenue_summary(month)
  - platform_fees_breakdown(start_date, end_date)
```

#### 2. Natural Language → Approved Query Matching
```
User: "How much did we make last week?"
Ekaya: Running approved query: weekly_revenue_summary
  Parameters: week = '2026-W05'

Result:
| offer_type | transactions | host_earnings | platform_revenue |
|------------|--------------|---------------|------------------|
| PAID       | 50           | $142.16       | $61.06           |
| TIP        | 4            | $35.95        | $15.39           |
```

#### 3. Query Suggestions Based on Role
```
User: [identified as Marketing role]
Ekaya: Suggested queries for Marketing:
  - new_hosts_this_month()
  - host_signup_funnel(date_range)
  - campaign_performance(campaign_id)
  - user_acquisition_by_source(start_date, end_date)
```

#### 4. Approved Query Categories/Tags
- `billing` - Revenue, fees, transactions
- `users` - Signups, retention, activity
- `engagement` - Calls, duration, completion rates
- `support` - Reports, blocks, moderation
- `marketing` - Campaigns, attribution, funnels

**Specific Queries I Needed Today:**

| Query Name | Parameters | Business Question |
|------------|------------|-------------------|
| `new_hosts_this_month` | none | "How many new hosts signed up?" |
| `revenue_by_offer_type` | start_date, end_date | "What's revenue by billing type?" |
| `engagement_funnel` | date_range | "What's our conversion funnel?" |
| `support_activity_recent` | days_back | "Any recent reports or blocks?" |
| `server_health` | none | "How are our servers doing?" |
| `email_delivery_stats` | date_range | "How are transactional emails performing?" |

---

### P1: Smart Error Recovery

**Problem:** When I used wrong table/column names, I got a raw error. Business users would be stuck.

#### 1. Auto-Suggest on Table Not Found
```
Error: relation "participant_servers" does not exist

Ekaya Enhancement:
  "Table 'participant_servers' not found. Did you mean:
   - e_servers (127 rows) - Server capacity and status
   - sessions (115 rows) - Session records with server associations"
```

#### 2. Auto-Suggest on Column Not Found
```
Error: column "billing_state" does not exist

Ekaya Enhancement:
  "Column 'billing_state' not found in 'billing_engagements'.
   This column exists in: tok_logs
   Similar columns in billing_engagements: offer_type, require_preauthorization"
```

#### 3. Soft-Delete Warning
```
Query: SELECT * FROM emails WHERE deleted_at IS NULL

Ekaya Enhancement:
  "Warning: Table 'emails' does not use soft-delete pattern.
   Removing 'deleted_at IS NULL' filter.

   Tables WITHOUT soft-delete: emails, e_servers, tok_logs, ..."
```

---

### P1: Query Result Enrichment

**Problem:** Raw query results show `offer_type = 2` but business users don't know what that means.

#### 1. Enum Label Substitution in Results
```
Before (current):
| offer_type | count |
|------------|-------|
| 2          | 50    |
| 5          | 7     |

After (enriched):
| offer_type          | count |
|---------------------|-------|
| PAID (per-minute)   | 50    |
| IMMEDIATE_PAYMENT   | 7     |
```

#### 2. Currency Formatting
```
Before: earned_amount = 14216
After: earned_amount = $142.16 USD

(Ontology knows currency_cents pattern → auto-format)
```

#### 3. Timestamp Humanization
```
Before: created_at = 2026-02-05T08:50:48+02:00
After: created_at = Feb 5, 2026 8:50 AM (6 hours ago)
```

---

### P2: Conversational Query Building

**Problem:** Business users can't write SQL. They need guided query building.

#### 1. Intent → Query Wizard
```
User: "I want to analyze revenue"

Ekaya: Let me help you build a revenue query.

1. Time period?
   [ ] Last 7 days
   [ ] Last 30 days
   [x] Custom range: 2026-01-01 to 2026-02-05

2. Group by?
   [x] Offer type
   [ ] Host
   [ ] Day/Week/Month

3. Include?
   [x] Transaction count
   [x] Host earnings
   [x] Platform fees
   [ ] Average per transaction

[Generate Query] → Creates and runs approved query
```

#### 2. Follow-up Questions
```
User: "Show me revenue by offer type"
[Results displayed]

Ekaya: "Would you like to:
  - Drill down into a specific offer type?
  - See this broken down by week?
  - Compare to previous period?
  - Export to CSV?"
```

---

### P2: Query Validation Before Execution

**Problem:** Bad queries waste time and confuse users.

#### 1. Pre-flight Check
```
Query submitted: SELECT * FROM users WHERE deleted_at IS NULL

Ekaya Pre-flight:
  ✅ Table 'users' exists (1,451 rows)
  ✅ Column 'deleted_at' exists
  ✅ Soft-delete filter applied correctly
  ⚠️ Warning: SELECT * returns 15 columns. Consider selecting specific columns.

  [Run Query] [Modify Query]
```

#### 2. Expensive Query Warning
```
Query: SELECT * FROM billing_transactions

Ekaya: "This query will return ~10,000 rows.
  Suggestions:
  - Add a date filter: WHERE created_at > '2026-01-01'
  - Add a LIMIT: LIMIT 100
  - Use an approved query: billing_transactions_summary(date_range)"
```

---

### P3: Query History & Favorites

#### 1. Recent Queries
```
Your recent queries:
1. revenue_by_offer_type (2 hours ago) ⭐
2. new_hosts_this_month (2 hours ago)
3. support_activity_recent (2 hours ago)

[Re-run] [Favorite] [Share with team]
```

#### 2. Team Query Sharing
```
Marketing Team Favorites:
- Weekly signup report (Sarah) - Mondays
- Campaign ROI calculator (Mike)
- Host activation funnel (Sarah)
```

---

### P3: Proactive Insights

**Problem:** Business users don't always know what questions to ask.

#### 1. Anomaly Alerts
```
Ekaya detected:
  ⚠️ New host signups down 30% vs last week
  ⚠️ 5 media reports in last 24 hours (unusual spike)
  ✅ Revenue on track for monthly target
```

#### 2. Suggested Explorations
```
Based on your role (Marketing), you might want to check:
  - Host activation rate this week
  - Campaign performance for active campaigns
  - New user acquisition by source
```

---

## Implementation Priority

| Priority | Feature | Effort | Impact |
|----------|---------|--------|--------|
| **P0** | Pre-approved query library with tags | M | High - Core business user workflow |
| **P0** | Natural language → approved query matching | L | High - Removes SQL barrier |
| **P1** | Smart error recovery (table/column suggestions) | M | High - Prevents user frustration |
| **P1** | Enum label substitution in results | S | Medium - Better readability |
| **P1** | Currency/timestamp formatting | S | Medium - Professional output |
| **P2** | Conversational query wizard | L | Medium - Guided experience |
| **P2** | Query validation pre-flight | M | Medium - Prevents errors |
| **P3** | Query history & favorites | M | Low - Convenience |
| **P3** | Proactive insights | L | Low - Nice to have |

---

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Business user can answer question without SQL | 20% | 90% |
| Queries that fail on first attempt | 40% | <10% |
| Time to answer business question | 2-5 min | <30 sec |
| User satisfaction (business users) | N/A | 4.5/5 |

---

## Starter Approved Queries for tikr_production

To bootstrap the approved query library, here are queries I needed today:

```sql
-- new_hosts_this_month
-- Tags: marketing, users, hosts
-- Description: Count of new host channels created this month
SELECT COUNT(*) as new_hosts, ROUND(AVG(avg_rating), 2) as avg_rating
FROM channels
WHERE created_at >= date_trunc('month', CURRENT_DATE)
  AND deleted_at IS NULL;

-- revenue_by_offer_type
-- Tags: finance, billing, revenue
-- Parameters: start_date DATE, end_date DATE
-- Description: Revenue breakdown by billing type
SELECT
  CASE offer_type
    WHEN 1 THEN 'FREE'
    WHEN 2 THEN 'PAID (per-minute)'
    WHEN 3 THEN 'START_FREE'
    WHEN 4 THEN 'CHARGE_IN_ENGAGEMENT'
    WHEN 5 THEN 'IMMEDIATE_PAYMENT'
    WHEN 6 THEN 'TIP'
  END as offer_type,
  COUNT(*) as transactions,
  ROUND(SUM(earned_amount) / 100.0, 2) as host_earnings_usd,
  ROUND(SUM(tikr_share) / 100.0, 2) as platform_revenue_usd
FROM billing_transactions
WHERE created_at BETWEEN {{start_date}} AND {{end_date}}
  AND deleted_at IS NULL
GROUP BY offer_type
ORDER BY platform_revenue_usd DESC;

-- support_activity_recent
-- Tags: support, moderation
-- Parameters: days_back INTEGER DEFAULT 7
-- Description: Recent reports and account blocks
SELECT 'Media Report' as type, created_at, report_reason as details
FROM media_reports WHERE created_at > NOW() - INTERVAL '{{days_back}} days'
UNION ALL
SELECT 'Account Block' as type, created_at, reason as details
FROM account_blocks WHERE created_at > NOW() - INTERVAL '{{days_back}} days'
ORDER BY created_at DESC;

-- server_health
-- Tags: operations, infrastructure
-- Description: Media server capacity and status
SELECT
  COUNT(*) as total_servers,
  COUNT(*) FILTER (WHERE unregistered_at IS NULL) as active,
  SUM(capacity) as total_capacity,
  SUM(num_sessions) as current_sessions
FROM e_servers;

-- engagement_summary
-- Tags: product, engagement
-- Parameters: start_date DATE, end_date DATE
-- Description: Engagement volume and billing breakdown
SELECT
  COUNT(*) as total_engagements,
  COUNT(*) FILTER (WHERE require_preauthorization) as preauth_required,
  ROUND(AVG(fee_per_minute) / 100.0, 2) as avg_fee_per_minute_usd
FROM billing_engagements
WHERE created_at BETWEEN {{start_date}} AND {{end_date}}
  AND deleted_at IS NULL;
```

---

## Summary

The gap between 7/10 and 9/10 is **removing the SQL barrier** for business users. Pre-approved queries are the foundation - they let admins curate safe, correct queries that business users can execute by name or natural language.

The other features (smart error recovery, result enrichment, conversational building) are enhancements that make the experience delightful rather than frustrating.

**Key principle:** Business users should never see a SQL error. They should only see:
1. A list of questions they can ask
2. Clear, formatted answers
3. Suggestions for follow-up questions

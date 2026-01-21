# BUGS: Ontology Extraction Issues

**Date:** 2026-01-21
**Project:** Tikr test database (`2bb984fc-a677-45e9-94ba-9f65712ade70`)
**MCP:** `mcp__test_data__*`

This document captures issues found when reviewing the ontology extraction results against the actual Tikr codebase at `/Users/damondanieli/go/src/github.com/tikr-media/tikr-services`.

---

## BUG-1: Sample/Test Tables Extracted as Real Entities

**Severity:** Critical
**Category:** Entity Discovery

### Problem
The database contains tables with prefixes `s1_` through `s10_` which are sample/test data for testing ontology extraction. These were incorrectly extracted as legitimate business entities.

### Affected Entities
| Prefix | Tables | Extracted Entities |
|--------|--------|-------------------|
| s1_ | s1_customers, s1_orders, s1_order_items | Customer, Order, Order Item |
| s2_ | s2_users, s2_posts, s2_comments | User, Post, Comment |
| s3_ | s3_students, s3_courses, s3_enrollments | Student, Course |
| s4_ | s4_employees, s4_categories | Employee, Category |
| s5_ | s5_users, s5_activities, s5_documents, s5_organizations | Activity, Document, Organization, s5_users |
| s6_ | s6_products, s6_inventory | Product, Inventory |
| s7_ | s7_tickets | Ticket |
| s8_ | s8_contracts, s8_messages, s8_people | Contract, Message, Person |
| s9_ | s9_currencies, s9_exchange_rates, s9_addresses, s9_countries | Currency, Exchange Rate, Address, Country |
| s10_ | s10_user_preferences, s10_events | User Preference, s10_events |

### Expected Behavior
The extraction should either:
1. Recognize these as test/sample tables (naming convention with prefixes)
2. Flag them for human review
3. Allow table exclusion patterns in configuration

### Repro
```
mcp__test_data__get_context(depth="entities")
```
Observe entities like "Customer", "Student", "Contract" that have no relation to Tikr's business model.

---

## BUG-2: Unnamed/Empty Description Entities

**Severity:** High
**Category:** Entity Enrichment

### Problem
Three entities have no descriptions and are just raw table names:

| Entity Name | Primary Table | Description |
|-------------|---------------|-------------|
| `users` | users | (empty) |
| `s10_events` | s10_events | (empty) |
| `s5_users` | s5_users | (empty) |

### Expected Behavior
All entities should have meaningful descriptions, or entities without descriptions should not be created.

### Repro
```
mcp__test_data__get_entity(name="users")
```
Returns entity with empty description.

---

## BUG-3: Duplicate/Conflicting User Entities

**Severity:** High
**Category:** Entity Discovery

### Problem
Multiple entities represent "users":

| Entity | Primary Table | Issue |
|--------|---------------|-------|
| `User` | s2_users | Points to sample table, not actual Tikr users |
| `users` | users | The REAL Tikr users table, but has no description |
| `s5_users` | s5_users | Sample table, no description |

The actual Tikr `users` table should be the primary User entity, but instead a sample table (`s2_users`) got that name.

### Repro
```
mcp__test_data__get_entity(name="User")     # Returns s2_users (wrong)
mcp__test_data__get_entity(name="users")    # Returns users (correct but unnamed)
```

---

## BUG-4: Spurious Column-to-Column Relationships

**Severity:** High
**Category:** Relationship Discovery

### Problem
The extraction created relationships based on column name matching rather than actual foreign key constraints. Many relationships are semantically incorrect.

### Specific Issues

#### 4a: Same Column Names Treated as FKs
```
accounts.email -> account_authentications.email
accounts.password -> account_authentications.hashed_password
```
These are NOT foreign keys. They just happen to store related data.

#### 4b: Wrong Column Mappings
```
Account Authentication -> Channel via account_id -> channel_id
```
`account_id` is NOT a FK to `channel_id`. These are different ID types.

```
Account Authentication -> users via account_id -> user_id
```
Similar issue - account_id does not reference user_id.

#### 4c: Reversed FK Direction
```
accounts.account_id -> account_password_resets.account_id
```
The schema shows this as FK from accounts to password_resets, but the actual FK is:
```
account_password_resets.account_id -> accounts.account_id
```

#### 4d: Bogus FK Targets
Schema shows incorrect FKs:
```
channels.channel_id -> account_authentications (bogus)
users.user_id -> account_authentications (bogus)
accounts.default_user_id -> channels (should be -> users)
```

### Repro
```
mcp__test_data__probe_relationship()
```
Observe relationships like `Account Authentication -> Channel` that don't exist.

---

## BUG-5: Missing Critical Tikr Domain Knowledge

**Severity:** High
**Category:** Domain Understanding

### Problem
The ontology doesn't capture critical Tikr-specific business concepts.

### Missing Concepts

#### 5a: "Tik" Billing Unit
A "Tik" is 6 seconds of engagement time. This is the fundamental billing unit.
- Source: `billing_helpers.go:413` - `DurationPerTik`
- Column: `billing_transactions.tiks`
- Calculation: `amount_per_tik = (fee_per_minute + 3) / 4` (line 321 in billing.go)

#### 5b: Host vs Visitor Roles
Users have distinct roles:
- **Host**: Content creator who receives payments (`host_id` columns)
- **Visitor**: Viewer who pays for engagements (`visitor_id` columns)

This role distinction appears in:
- `billing_engagements.visitor_id`, `billing_engagements.host_id`
- `billing_transactions.payer_user_id`, `billing_transactions.payee_user_id`
- `engagement_payment_intents.visitor_id`, `engagement_payment_intents.host_id`

#### 5c: Fee Structure
Revenue split:
```go
platformFees = totalAmount * 45 / 1000       // 4.5%
afterPlatformFees = totalAmount - platformFees
tikrShare = afterPlatformFees * 30 / 100     // 30% of remainder
earnedAmount = afterPlatformFees - tikrShare  // Host gets ~66.35%
```
Source: `billing_helpers.go:373-379`

### Repro
```
mcp__test_data__get_context(depth="domain")
```
No mention of "Tik", "Host", "Visitor", or fee structure.

---

## BUG-6: Missing Enum Value Extraction

**Severity:** Medium
**Category:** Column Enrichment

### Problem
Key enum columns don't have their values documented.

### Missing Enums

#### 6a: `offer_type` / `offer_type_string`
Tables: `offers`, `billing_engagements`, `billing_transactions`, `channels`
```
OFFER_TYPE_UNSPECIFIED = 0
OFFER_TYPE_FREE = 1                    # Free Engagement
OFFER_TYPE_PAID = 2                    # Preauthorized per-minute charge
OFFER_TYPE_START_FREE = 3              # Starts free then charges
OFFER_TYPE_CHARGE_IN_ENGAGEMENT = 4    # Charge during engagement
OFFER_TYPE_IMMEDIATE_PAYMENT = 5       # Immediate charge
OFFER_TYPE_TIP = 6                     # Visitor tips Host
```
Source: `/pb/gen/utobe/v1/utobe.pb.go:296-302`

#### 6b: `transaction_state`
Table: `billing_transactions`
```
TRANSACTION_STATE_UNSPECIFIED = 0
TRANSACTION_STATE_STARTED = 1          # Transaction started
TRANSACTION_STATE_ENDED = 2            # Transaction ended
TRANSACTION_STATE_WAITING = 3          # Awaiting chargeback period
TRANSACTION_STATE_AVAILABLE = 4        # Available for payout
TRANSACTION_STATE_PROCESSING = 5       # Processing payout
TRANSACTION_STATE_PAYING = 6           # Paying out
TRANSACTION_STATE_PAID = 7             # Paid out
TRANSACTION_STATE_ERROR = 8            # Error
```
Source: `/pb/gen/utobe/v1/utobe.pb.go:357-365`

#### 6c: `transaction_type`
Table: `billing_transactions`
```
unknown
engagement
payout
```
Source: `billingtables.go:43-47`

#### 6d: `activity` (BillingActivity)
Table: `billing_activity_messages`
```
confirmed
paused
resumed
refunded
```
Source: `billingtables.go:97-102`

### Repro
```
mcp__test_data__probe_column(table="billing_transactions", column="transaction_state")
```
No enum values returned.

---

## BUG-7: Generic SaaS Glossary Terms

**Severity:** Medium
**Category:** Glossary

### Problem
The glossary contains generic SaaS business metrics that don't reflect Tikr's actual business model.

### Incorrect/Irrelevant Terms
| Term | Issue |
|------|-------|
| Active Subscribers | Tikr uses engagements, not subscriptions |
| Churn Rate | Doesn't apply - Tikr is pay-per-use, not subscription |
| Customer Lifetime Value | Calculation won't work with Tikr's model |
| Average Order Value | Tikr has engagements, not orders |
| Inventory Turnover | Tikr has no inventory |

### What Should Be in Glossary
- **Tik**: A 6-second unit of billed engagement time
- **Engagement**: A paid video session between Host and Visitor
- **Earned Amount**: Host's earnings after platform fees and Tikr share
- **Tikr Share**: Platform's revenue share (~30% after fees)
- **Platform Fees**: Payment processing fees (~4.5%)
- **Preauthorization**: Hold on Visitor's card before engagement starts

### Repro
```
mcp__test_data__list_glossary()
```

---

## BUG-8: Test Data in Glossary

**Severity:** Low
**Category:** Data Quality

### Problem
Test term exists in production glossary:
```
term: "UITestTerm2026"
definition: "A test term created via UI to verify MCP sync"
```

### Repro
```
mcp__test_data__list_glossary()
```

---

## BUG-9: Missing Real Foreign Key Relationships

**Severity:** High
**Category:** Relationship Discovery

### Problem
Actual FK relationships in Tikr tables are not extracted or are missing.

### Evidence
```
mcp__test_data__probe_relationship(from_entity="Billing Engagement")
```
Returns: `{"relationships":[]}`

The "Billing Engagement" entity has ZERO relationships, despite having obvious FKs to users, sessions, and offers.

### Missing Relationships
| From Table | From Column | To Table | To Column |
|------------|-------------|----------|-----------|
| billing_engagements | visitor_id | users | user_id |
| billing_engagements | host_id | users | user_id |
| billing_engagements | session_id | sessions | session_id |
| billing_engagements | offer_id | offers | offer_id |
| billing_transactions | engagement_id | billing_engagements | engagement_id |
| billing_transactions | session_id | sessions | session_id |
| channels | owner_id | users | user_id |
| sessions | host_id | users | user_id |
| offers | owner_id | users | user_id |
| engagement_payment_intents | visitor_id | users | user_id |
| engagement_payment_intents | host_id | users | user_id |

These are "soft" FKs (not enforced at DB level) but are semantically important.

### Root Cause
Many Tikr ID columns use `text` type (UUIDs as strings) without DB-level FK constraints. The extraction only found relationships where:
1. Postgres FK constraints exist, OR
2. Column names matched between tables

But for Tikr tables like `billing_engagements`, the columns (`visitor_id`, `host_id`) don't match the target table's column (`user_id`), so no relationship was inferred.

### Repro
```
mcp__test_data__get_schema()
mcp__test_data__probe_relationship(from_entity="Billing Engagement")
mcp__test_data__probe_relationship(from_entity="Billing Transaction")
```

---

## BUG-10: Stale Data Not Cleaned on Ontology Delete

**Severity:** High
**Category:** Data Lifecycle

### Problem
When an ontology is deleted (e.g., before changing datasources), project-level data is NOT cleaned up because it's linked to `project_id` rather than `ontology_id`.

### How This Was Discovered
**Important:** The MCP server was returning correct data for the current ontology. This issue was only surfaced when querying the `ekaya_engine` database directly via `psql` to inspect `engine_project_knowledge` - which revealed stale facts from a prior datasource (claude_cowork) that should have been cleaned up when the ontology was deleted.

The MCP tools (`mcp__test_data__*`) correctly query the current ontology and datasource. The bug is in the **ontology deletion logic** which does not clean up project-level tables.

### Root Cause
When an ontology is deleted, tables with `ontology_id` FK get cascade-deleted:
- `engine_entity_relationships` ✓
- `engine_ontology_entities` ✓
- `engine_ontology_questions` ✓
- `engine_ontology_chat_messages` ✓
- `engine_ontology_dag` ✓

But tables with only `project_id` FK remain orphaned:
- `engine_project_knowledge` ✗
- `engine_business_glossary` ✗

### Evidence
Timeline for project `2bb984fc-a677-45e9-94ba-9f65712ade70`:

| Time | Event | Data |
|------|-------|------|
| 02:28 - 08:34 | OLD datasource (claude_cowork) | Project knowledge about "cross-product continuity" |
| 08:34 | OLD glossary terms | "Active Threads", "Recent Messages" |
| (gap) | Ontology deleted via UI, datasource changed to Tikr | |
| 21:26:50 | NEW ontology created (Tikr) | |
| 21:31:22 | NEW glossary terms | "Revenue", "Active Users", etc. |

The OLD project knowledge and glossary terms persisted across the datasource change because they weren't cleaned up.

### Fix Options
1. **Add `ontology_id` FK** to `engine_project_knowledge` and `engine_business_glossary` with CASCADE delete
2. **Explicit cleanup** in ontology delete handler - delete associated project knowledge and glossary when ontology is deleted
3. **Soft-delete tracking** - add `ontology_version` column to track which extraction created the data

---

## BUG-11: Wrong Project Knowledge Content

**Severity:** Medium
**Category:** Knowledge Capture

### Problem
Project knowledge facts exist but are from a prior datasource. This is a consequence of BUG-10 (stale data not cleaned on ontology delete).

**Note:** This was discovered via `psql` querying `ekaya_engine.engine_project_knowledge`, NOT through the MCP server which was returning correct ontology data.

```
psql -d ekaya_engine -c "SELECT fact_type, key FROM engine_project_knowledge WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'"
```

Returns:
| fact_type | key |
|-----------|-----|
| terminology | This database is shared across Claude Chat, Claude Code, and Claude Cowork... |
| terminology | This database stores conversation continuity data across Claude products |
| terminology | The claude_cowork database is designed for cross-product continuity... |
| convention | Entries use source field to track origin: user, claude_code, claude_chat... |

These facts are about "claude_cowork", NOT about Tikr. Either:
1. Extraction was run against a different database previously
2. Project knowledge wasn't cleared when switching datasources
3. Facts were added manually and are incorrect

### Expected Knowledge
- Currency convention: All amounts in cents (USD)
- Time zone handling: accounts.time_zone stores user timezone
- Soft delete pattern: `deleted_at` column indicates soft delete
- Billing minimum: MinCaptureAmount = 100 cents ($1.00)
- Payout minimum: MinPayoutAmount = 2000 cents ($20.00)
- Entity ID format: UUIDs stored as text strings
- Tik duration: 6 seconds per tik
- Fee structure: 4.5% platform fees, 30% Tikr share

### Repro
```bash
psql -d ekaya_engine -c "SELECT set_config('app.current_project_id', '2bb984fc-a677-45e9-94ba-9f65712ade70', false); SELECT fact_type, key, value FROM engine_project_knowledge"
```

---

## Summary

| Bug | Severity | Category |
|-----|----------|----------|
| BUG-1 | Critical | Sample tables as entities |
| BUG-2 | High | Empty entity descriptions |
| BUG-3 | High | Duplicate User entities |
| BUG-4 | High | Spurious relationships |
| BUG-5 | High | Missing domain knowledge |
| BUG-6 | Medium | Missing enum values |
| BUG-7 | Medium | Generic glossary terms |
| BUG-8 | Low | Test data in glossary |
| BUG-9 | High | Missing FK relationships |
| BUG-10 | High | Stale data not cleaned on ontology delete |
| BUG-11 | Medium | Wrong project knowledge content |

## Recommendations

1. **Table filtering**: Add configuration to exclude tables by pattern (e.g., `s*_`)
2. **Entity deduplication**: Detect when multiple entities represent the same concept
3. **FK inference**: Use column naming patterns (`*_id` referencing table `*`) to infer soft FKs
4. **Enum extraction**: Query distinct values for columns with limited cardinality
5. **Code analysis**: Integrate with codebase analysis to extract business logic (fee calculations, etc.)
6. **Knowledge seeding**: Allow manual knowledge injection before extraction

# PLAN: Ontology Extractor Auto-Inference Improvements

## Context

After answering 190 ontology questions for the tikr_production database, analysis revealed that many questions could have been auto-answered through data analysis alone, without requiring source code access. The extractor currently generates good questions but leaves inference opportunities on the table.

**Current Performance Assessment:**
| Category | Current | Target |
|----------|---------|--------|
| Question generation | 8/10 | 9/10 |
| Auto-inference from data | 4/10 | 9/10 |
| Relationship detection | 6/10 | 9/10 |
| Pattern recognition | 5/10 | 9/10 |

## Improvement Areas with Examples

### 1. Enum Value Distribution Analysis

**Problem:** Extractor detects enum columns but doesn't analyze value distribution.

**Example Question Generated:**
> "What do the integer values in billing_transactions.transaction_state represent?"

**Data Available:**
```sql
SELECT transaction_state, COUNT(*) as count
FROM billing_transactions
GROUP BY transaction_state
ORDER BY transaction_state;

-- Result:
-- 0: 45000  (likely "pending" - most common starting state)
-- 1: 2300
-- 2: 150
-- 3: 89
-- 4: 12000
-- 5: 38000  (likely terminal success state)
-- 6: 890
-- 7: 45     (rare - likely error/edge case)
```

**Auto-Inference Opportunity:**
- Label as `ENUM_STATE_0` through `ENUM_STATE_7`
- Add distribution metadata: "Values 0 and 5 comprise 87% of records"
- Flag 0 as likely "initial state" (highest count, often paired with NULL completion timestamps)
- Flag 5 as likely "success terminal" (high count, paired with non-NULL completion timestamps)
- Flag 7 as likely "error/rare" (lowest count)

**Implementation:**
```sql
-- Detect terminal states by checking if they correlate with completion timestamps
SELECT transaction_state,
       COUNT(*) as total,
       COUNT(completed_at) as has_completion,
       ROUND(100.0 * COUNT(completed_at) / COUNT(*), 1) as completion_pct
FROM billing_transactions
GROUP BY transaction_state;
```

States with 0% completion = pending/in-progress
States with ~100% completion = terminal states

---

### 2. Soft Delete Pattern Recognition

**Problem:** `deleted_at` columns appear across many tables but aren't auto-documented.

**Example Question Generated:**
> "What does a NULL vs non-NULL deleted_at indicate in the users table?"

**Data Available:**
```sql
SELECT
    COUNT(*) as total,
    COUNT(deleted_at) as deleted_count,
    ROUND(100.0 * COUNT(deleted_at) / COUNT(*), 2) as deleted_pct
FROM users;

-- Result: total=50000, deleted_count=1200, deleted_pct=2.4%
```

**Auto-Inference Opportunity:**
Pattern detection rule:
- Column named `deleted_at`
- Type is `timestamp` or `timestamptz`
- 90%+ values are NULL
- Non-NULL values are valid timestamps

Auto-generate description: "GORM soft delete flag. NULL = active record, timestamp = soft-deleted at that time. 97.6% of records are active."

**Implementation:**
```go
func detectSoftDelete(col Column, stats ColumnStats) *AutoDescription {
    if col.Name == "deleted_at" &&
       isTimestampType(col.Type) &&
       stats.NullRate > 0.90 {
        return &AutoDescription{
            Description: "Soft delete timestamp (GORM pattern). NULL = active, timestamp = deleted.",
            Confidence: 0.95,
            Pattern: "soft_delete",
        }
    }
    return nil
}
```

---

### 3. Monetary Column Detection

**Problem:** Amount columns aren't identified as monetary with their scale.

**Example Question Generated:**
> "What unit is billing_transactions.amount stored in? Dollars, cents, or another denomination?"

**Data Available:**
```sql
-- Check for paired currency column
SELECT column_name FROM information_schema.columns
WHERE table_name = 'billing_transactions' AND column_name LIKE '%currency%';
-- Result: currency

-- Check value ranges
SELECT MIN(amount), MAX(amount), AVG(amount) FROM billing_transactions;
-- Result: min=50, max=500000, avg=2340

-- Check currency values
SELECT DISTINCT currency FROM billing_transactions LIMIT 10;
-- Result: USD, EUR, GBP, CAD, AUD (3-char uppercase)
```

**Auto-Inference Opportunity:**
Pattern detection:
- Column named `amount`, `*_amount`, `*_share`, `total`, `price`
- Type is `bigint` or `integer`
- Same table has a `currency` column with ISO 4217 values
- Values are whole numbers (no decimals stored)

Auto-generate: "Monetary amount in minor units (cents/pence). Pair with `currency` column for denomination. ISO 4217 currency codes."

**Implementation:**
```sql
-- Detect ISO 4217 currency column
SELECT column_name
FROM columns
WHERE table_name = $1
  AND data_type IN ('text', 'varchar', 'char')
  AND (
    SELECT COUNT(DISTINCT val)
    FROM (SELECT DISTINCT column_value as val FROM table LIMIT 100)
  ) BETWEEN 10 AND 200
  AND (
    SELECT LENGTH(val) FROM (SELECT DISTINCT column_value as val LIMIT 1)
  ) = 3;
```

---

### 4. UUID Text Column Detection

**Problem:** Text columns containing UUIDs aren't labeled as such.

**Example Question Generated:**
> "Is channel_id a foreign key? What table does it reference?"

**Data Available:**
```sql
SELECT channel_id FROM some_table LIMIT 5;
-- Result:
-- 550e8400-e29b-41d4-a716-446655440000
-- 6ba7b810-9dad-11d1-80b4-00c04fd430c8
-- f47ac10b-58cc-4372-a567-0e02b2c3d479
```

**Auto-Inference Opportunity:**
```sql
-- Detect UUID format
SELECT
    column_name,
    COUNT(*) as total,
    COUNT(*) FILTER (
        WHERE column_value ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    ) as uuid_matches
FROM sample_values
GROUP BY column_name
HAVING uuid_matches::float / total > 0.99;
```

Auto-generate: "UUID stored as text (36 characters). Logical foreign key - no database constraint."

---

### 5. Timestamp Scale Detection (Seconds vs Nanoseconds)

**Problem:** `marker_at` columns store nanoseconds but look like regular timestamps.

**Example Question Generated:**
> "What is the purpose of marker_at? It appears to be a timestamp but values seem unusual."

**Data Available:**
```sql
SELECT created_at, marker_at FROM billing_transactions LIMIT 1;
-- created_at: 1704067200 (10 digits - seconds)
-- marker_at:  1704067200000000000 (19 digits - nanoseconds)
```

**Auto-Inference Opportunity:**
```sql
-- Detect timestamp scale
SELECT
    column_name,
    AVG(LENGTH(column_value::text)) as avg_digits,
    CASE
        WHEN AVG(LENGTH(column_value::text)) BETWEEN 9 AND 11 THEN 'seconds'
        WHEN AVG(LENGTH(column_value::text)) BETWEEN 12 AND 14 THEN 'milliseconds'
        WHEN AVG(LENGTH(column_value::text)) BETWEEN 15 AND 17 THEN 'microseconds'
        WHEN AVG(LENGTH(column_value::text)) BETWEEN 18 AND 20 THEN 'nanoseconds'
    END as likely_scale
FROM bigint_columns
WHERE column_name LIKE '%_at' OR column_name LIKE '%time%';
```

Auto-generate for marker_at: "Unix timestamp in nanoseconds. Used for cursor-based pagination ordering."

---

### 6. FK Target Resolution via Data Overlap

**Problem:** `host_id` and `visitor_id` both reference users, but this isn't confirmed.

**Example Question Generated:**
> "Does host_id reference the users table or accounts table?"

**Data Available:**
```sql
-- Check overlap with users.user_id
SELECT COUNT(*) as matches
FROM engagements e
JOIN users u ON e.host_id = u.user_id;
-- Result: 48000 (high match rate)

-- Check overlap with accounts.account_id
SELECT COUNT(*) as matches
FROM engagements e
JOIN accounts a ON e.host_id = a.account_id;
-- Result: 0 (no matches)
```

**Auto-Inference Opportunity:**
For each `*_id` column, run overlap analysis against all potential target tables:

```sql
WITH candidates AS (
    SELECT 'users' as target_table, 'user_id' as target_col
    UNION SELECT 'accounts', 'account_id'
    UNION SELECT 'sessions', 'session_id'
    -- ... all tables with primary keys
)
SELECT
    target_table,
    target_col,
    (SELECT COUNT(*) FROM source_table s
     JOIN target_table t ON s.source_col = t.target_col) as match_count,
    (SELECT COUNT(*) FROM source_table) as total
FROM candidates
ORDER BY match_count DESC;
```

Auto-generate: "Foreign key to users.user_id (98.5% match rate). No database constraint - logical reference only."

---

### 7. Cardinality Inference from Data

**Problem:** Account:User relationship cardinality not computed.

**Example Question Generated:**
> "Can an account have multiple users, or is it 1:1?"

**Data Available:**
```sql
SELECT
    account_id,
    COUNT(DISTINCT user_id) as users_per_account
FROM users
WHERE deleted_at IS NULL
GROUP BY account_id
ORDER BY users_per_account DESC
LIMIT 10;

-- Result:
-- acc_001: 8 users
-- acc_002: 5 users
-- acc_003: 4 users
-- ... (max observed: 10)

SELECT
    COUNT(DISTINCT account_id) as accounts,
    COUNT(DISTINCT user_id) as users,
    ROUND(COUNT(DISTINCT user_id)::numeric / COUNT(DISTINCT account_id), 2) as avg_users_per_account
FROM users WHERE deleted_at IS NULL;
-- Result: accounts=12000, users=35000, avg=2.92
```

**Auto-Inference Opportunity:**
```go
func inferCardinality(fromTable, toTable string, fkColumn string) Cardinality {
    // Query: How many distinct FK values per source record?
    // Query: How many source records per FK value?

    if avgSourcePerFK > 1.5 && maxSourcePerFK > 1 {
        return "1:N" // One target has many sources
    }
    if avgSourcePerFK <= 1.1 && maxSourcePerFK == 1 {
        return "1:1"
    }
    // ... etc
}
```

Auto-generate for users.account_id: "Many-to-one relationship. Average 2.9 users per account, max observed: 10."

---

### 8. Role Detection from Column Naming

**Problem:** `host_id` and `visitor_id` both map to User but roles aren't distinguished.

**Example Question Generated:**
> "What's the semantic difference between host_id and visitor_id?"

**Auto-Inference Opportunity:**
When multiple columns in the same table reference the same entity, flag as "role-based FKs":

```go
var rolePatterns = map[string][]string{
    "user": {"host", "visitor", "creator", "owner", "sender", "recipient", "payer", "payee"},
    "account": {"source", "destination", "from", "to"},
}

func detectRoles(table Table, entityRefs []ColumnRef) []RoleAnnotation {
    // If host_id and visitor_id both -> users
    // Label: "User entity with role: host" and "User entity with role: visitor"
}
```

Auto-generate:
- host_id: "FK to users.user_id. Role: host (content provider in engagement)"
- visitor_id: "FK to users.user_id. Role: visitor (content consumer in engagement)"

---

### 9. Boolean Naming Pattern Detection

**Problem:** `is_*` and `has_*` columns lack descriptions.

**Example Question Generated:**
> "What does is_pc_enabled mean?"

**Data Available:**
```sql
SELECT is_pc_enabled, COUNT(*)
FROM users
GROUP BY is_pc_enabled;
-- Result: true=12000, false=38000
```

**Auto-Inference Opportunity:**
For boolean columns, analyze:
- Distribution (what % true vs false?)
- Correlation with other columns
- Naming pattern

```go
func describeBooleanColumn(col Column, stats Stats) string {
    trueRate := stats.TrueCount / stats.TotalCount

    desc := fmt.Sprintf("Boolean flag. %.1f%% of records are true.", trueRate*100)

    // Add interpretation based on name
    if strings.HasPrefix(col.Name, "is_") {
        feature := strings.TrimPrefix(col.Name, "is_")
        desc += fmt.Sprintf(" Indicates whether %s is enabled/active.", humanize(feature))
    }
    return desc
}
```

Auto-generate: "Boolean flag. 24% of users have this enabled. Indicates whether pc (Private Channels?) feature is active."

---

### 10. Email/External ID Pattern Detection

**Problem:** `email_id` vs `linked_email_id` semantics unclear.

**Data Available:**
```sql
SELECT email_id, linked_email_id FROM emails LIMIT 3;
-- email_id: 550e8400-e29b-41d4-a716-446655440000 (UUID format)
-- linked_email_id: 0102018d1234abcd-12345678-1234-1234-1234-123456789012@email.amazonses.com
```

**Auto-Inference Opportunity:**
Pattern matching for known external ID formats:

```go
var externalIDPatterns = map[string]*regexp.Regexp{
    "AWS SES Message-ID": regexp.MustCompile(`^[0-9a-f-]+@email\.amazonses\.com$`),
    "Stripe ID": regexp.MustCompile(`^(pi_|pm_|ch_|cus_|sub_)[a-zA-Z0-9]+$`),
    "UUID": regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
}

func detectExternalIDFormat(sampleValues []string) string {
    for name, pattern := range externalIDPatterns {
        matchCount := 0
        for _, v := range sampleValues {
            if pattern.MatchString(v) { matchCount++ }
        }
        if float64(matchCount)/float64(len(sampleValues)) > 0.95 {
            return name
        }
    }
    return ""
}
```

Auto-generate for linked_email_id: "AWS SES Message-ID format. External reference to sent email in AWS SES."

---

## Implementation Priority

| Improvement | Impact | Effort | Priority |
|-------------|--------|--------|----------|
| Soft delete detection | High (many tables) | Low | P0 |
| UUID format detection | High (all FKs) | Low | P0 |
| Enum distribution analysis | High (business logic) | Medium | P0 |
| FK target resolution | High (relationships) | Medium | P1 |
| Monetary column detection | Medium | Low | P1 |
| Timestamp scale detection | Medium | Low | P1 |
| Cardinality inference | Medium | Medium | P1 |
| Role detection | Medium | Medium | P2 |
| Boolean descriptions | Low | Low | P2 |
| External ID patterns | Low | Medium | P2 |

## Success Metrics

After implementation, re-run ontology extraction on tikr_production:

- **Questions generated** should decrease by ~40% (auto-answered)
- **Auto-confidence scores** should average >0.85 for inferred answers
- **Manual research required** should drop from 60% to <15% of questions
- **Column descriptions populated** should increase from ~20% to >80%

## Testing Approach

1. Run current extractor on tikr_production, save question list
2. Implement each improvement incrementally
3. Re-run extractor after each improvement
4. Compare: questions generated, auto-answers provided, confidence scores
5. Validate auto-answers against manual research from the 190 Q&A session

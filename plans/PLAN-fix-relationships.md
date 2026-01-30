# PLAN: Fix Relationships - Verified Joins for MCP Clients

## Problem Statement

MCP clients (AI agents) are currently forced to **guess** at table relationships:

```
❌ Column naming conventions (guessing)
   host_id → probably joins to users.user_id

❌ Trial and error
   SELECT * FROM billing_engagements be
   JOIN users u ON be.host_id = u.user_id  -- hope this is right

❌ Manual exploration
   SELECT DISTINCT host_id FROM billing_engagements LIMIT 1;
   SELECT * FROM users WHERE user_id = 'that-uuid';
```

This leads to:
- Hallucinated columns/joins
- Wasted tokens on exploratory queries
- Incorrect query results
- Poor user experience

## Solution

**Ekaya should do the hard work during ontology extraction**, not the MCP client at query time.

This plan follows the established Column Features pattern:
1. **Deterministic data collection** (no LLM, no string matching)
2. **Data-driven FK candidate discovery** (overlap analysis between columns)
3. **LLM-assisted semantic classification** (determine role, meaning)
4. **Verification with actual data** (match rate, cardinality)
5. **Store verified relationships**
6. **Surface to MCP clients as facts, not guesses**

---

## Architecture: Data-Driven FK Discovery

### Principle: No String Pattern Matching

❌ **WRONG**: `if strings.HasSuffix(col.Name, "_id")`
❌ **WRONG**: Pattern rules like `{role}_id → users.user_id`
❌ **WRONG**: Hard-coded role lists (host, visitor, payer, payee)

✅ **RIGHT**: Use existing Column Features classification
✅ **RIGHT**: Data overlap analysis to discover relationships
✅ **RIGHT**: LLM determines semantic role from context

### Foundation: Column Features Already Exists

The Column Feature Extraction pipeline (DAG node 2) already classifies columns:

```go
// From pkg/models/column_features.go
type IdentifierFeatures struct {
    IdentifierType   string   // "internal_uuid", "foreign_key", "primary_key", etc.
    FKTargetTable    string   // Discovered via Phase 4 overlap analysis
    FKTargetColumn   string   // Discovered via Phase 4 overlap analysis
    FKConfidence     float64  // 0.0-1.0 based on match rate
    EntityReferenced string   // Semantic entity this FK points to
}
```

**Phase 4 (FK Resolution)** already does overlap analysis - but results are not being:
1. Stored persistently in a relationships table
2. Surfaced through MCP tools (probe_relationship, get_context)
3. Used to generate join paths

---

## Extraction Phase: Relationship Detection

### Step 1: Identify FK Candidates (Data-Driven)

Use Column Features classification to find FK candidates - **no string matching**:

```go
// From Phase 2 Column Classification output
func findFKCandidates(columns []SchemaColumn) []FKCandidate {
    var candidates []FKCandidate

    for _, col := range columns {
        features := col.GetColumnFeatures()
        if features == nil {
            continue
        }

        // Use classification, not column name patterns
        if features.IdentifierFeatures != nil {
            idFeatures := features.IdentifierFeatures

            // FK candidates are columns classified as foreign_key or internal_uuid
            // that are NOT primary keys
            if idFeatures.IdentifierType == "foreign_key" ||
               (idFeatures.IdentifierType == "internal_uuid" && features.Role != "primary_key") {
                candidates = append(candidates, FKCandidate{
                    SourceTableID:  col.TableID,
                    SourceColumnID: col.ID,
                    SourceColumn:   col.Name,
                    Features:       features,
                })
            }
        }
    }
    return candidates
}
```

### Step 2: Find Target Columns (Overlap Analysis)

For each FK candidate, find columns with matching values using **data overlap**:

```go
// Phase 4 FK Resolution - already exists in column_feature_extraction.go
func findTargetColumns(ctx context.Context, candidate FKCandidate, allColumns []SchemaColumn) []OverlapResult {
    var results []OverlapResult

    // Get sample values from source column (already collected in Phase 1)
    sourceValues := candidate.Features.Profile.SampleValues

    // For each potential target column (identifier columns with Role=primary_key)
    for _, targetCol := range allColumns {
        targetFeatures := targetCol.GetColumnFeatures()
        if targetFeatures == nil || targetFeatures.Role != "primary_key" {
            continue
        }

        // Skip same table
        if targetCol.TableID == candidate.SourceTableID {
            continue
        }

        // Check data type compatibility
        if !compatibleTypes(candidate.Features.Profile.DataType, targetFeatures.Profile.DataType) {
            continue
        }

        // Run overlap analysis - what % of source values exist in target?
        overlap := calculateOverlap(ctx, sourceValues, targetCol)
        if overlap.MatchRate >= 0.5 {
            results = append(results, overlap)
        }
    }

    return results
}

// Actual SQL overlap query
func calculateOverlap(ctx context.Context, sourceValues []string, targetCol SchemaColumn) OverlapResult {
    // Query: How many of our source sample values exist in the target column?
    sql := `
        WITH source_samples AS (
            SELECT unnest($1::text[]) AS sample_value
        ),
        matches AS (
            SELECT COUNT(DISTINCT s.sample_value) as matched
            FROM source_samples s
            WHERE EXISTS (
                SELECT 1 FROM %s t
                WHERE t.%s = s.sample_value
            )
        )
        SELECT
            matched,
            array_length($1, 1) as total,
            matched::float / NULLIF(array_length($1, 1), 0) as match_rate
        FROM matches
    `
    // ... execute query and return OverlapResult
}
```

### Step 3: LLM Semantic Classification

Once data overlap identifies candidate relationships, use LLM to determine semantic meaning:

```go
// Present deterministic data to LLM for semantic classification
type RelationshipClassificationInput struct {
    // Source column context
    SourceTable       string
    SourceColumn      string
    SourceDescription string   // From column features
    SourceSampleValues []string

    // Target column context
    TargetTable       string
    TargetColumn      string
    TargetDescription string   // From column features

    // Overlap metrics (deterministic)
    MatchRate         float64
    MatchedCount      int
    TotalSamples      int

    // Schema context
    OtherColumnsInSourceTable []string
    OtherColumnsInTargetTable []string
}

// LLM determines
type RelationshipClassificationOutput struct {
    // Is this actually a foreign key relationship?
    IsFK            bool      `json:"is_fk"`
    Confidence      float64   `json:"confidence"`

    // Semantic role (LLM infers from context, not column name)
    Role            string    `json:"role,omitempty"`    // "host", "visitor", "payer", etc.
    RoleDescription string    `json:"role_description,omitempty"`

    // Entity mapping
    SourceEntity    string    `json:"source_entity,omitempty"`  // "Engagement"
    TargetEntity    string    `json:"target_entity,omitempty"`  // "User"

    // Reasoning
    Reasoning       string    `json:"reasoning"`
}
```

**Example LLM Prompt**:
```
Given the following data overlap between two columns:

SOURCE: billing_engagements.host_id (text, UUID format)
  - Sample values: ["a1b2c3...", "d4e5f6...", ...]
  - Other columns in table: [engagement_id, visitor_id, started_at, ended_at, ...]

TARGET: users.user_id (text, UUID format, PRIMARY KEY)
  - Other columns in table: [username, email, account_id, ...]

OVERLAP: 98.5% of source values exist in target (985/1000 samples matched)

Questions:
1. Is this a foreign key relationship?
2. If yes, what semantic role does the source column represent?
3. What entities are involved?

Return JSON with: is_fk, confidence, role, role_description, source_entity, target_entity, reasoning
```

### Step 4: Verify with Full Data (Cardinality & Orphans)

After LLM classification, run full verification queries:

```sql
-- Calculate match rate on full data (not just samples)
WITH source_values AS (
    SELECT DISTINCT host_id AS fk_value
    FROM billing_engagements
    WHERE host_id IS NOT NULL
),
match_check AS (
    SELECT
        COUNT(DISTINCT sv.fk_value) AS source_distinct,
        COUNT(DISTINCT CASE WHEN u.user_id IS NOT NULL THEN sv.fk_value END) AS matched_count
    FROM source_values sv
    LEFT JOIN users u ON sv.fk_value = u.user_id
)
SELECT
    source_distinct,
    matched_count,
    source_distinct - matched_count AS orphan_count,
    ROUND(matched_count::numeric / NULLIF(source_distinct, 0) * 100, 2) AS match_rate
FROM match_check;

-- Determine cardinality
WITH cardinality_check AS (
    SELECT
        host_id,
        COUNT(*) AS row_count
    FROM billing_engagements
    WHERE host_id IS NOT NULL
    GROUP BY host_id
)
SELECT
    CASE
        WHEN MAX(row_count) = 1 THEN '1:1'
        ELSE 'N:1'  -- Many rows per FK value
    END AS cardinality,
    COUNT(DISTINCT host_id) AS distinct_fk_values,
    SUM(row_count) AS total_rows
FROM cardinality_check;
```

### Step 5: Store Verified Relationships

```sql
CREATE TABLE engine_relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(project_id),
    datasource_id UUID NOT NULL REFERENCES engine_datasources(datasource_id),

    -- Source side (from Column Features)
    source_table_id UUID REFERENCES engine_schema_tables(id),
    source_column_id UUID REFERENCES engine_schema_columns(id),
    source_table VARCHAR(255) NOT NULL,
    source_column VARCHAR(255) NOT NULL,
    source_entity VARCHAR(255),  -- From LLM classification
    source_role VARCHAR(255),    -- From LLM classification (e.g., 'host', 'visitor')

    -- Target side
    target_table_id UUID REFERENCES engine_schema_tables(id),
    target_column_id UUID REFERENCES engine_schema_columns(id),
    target_table VARCHAR(255) NOT NULL,
    target_column VARCHAR(255) NOT NULL,
    target_entity VARCHAR(255),  -- From LLM classification

    -- Verification results (deterministic)
    cardinality VARCHAR(10) NOT NULL,  -- '1:1', '1:N', 'N:1', 'N:M'
    match_rate NUMERIC(5,2),           -- 0.00 to 100.00
    source_distinct_count INTEGER,
    orphan_count INTEGER,

    -- Classification metadata
    llm_confidence NUMERIC(3,2),       -- 0.00 to 1.00
    llm_reasoning TEXT,

    -- Provenance
    detection_method VARCHAR(50) DEFAULT 'data_overlap',  -- 'data_overlap', 'llm', 'manual', 'admin'
    verified_at TIMESTAMPTZ DEFAULT NOW(),
    provenance VARCHAR(50) DEFAULT 'inference',

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(project_id, source_table, source_column, target_table, target_column)
);

-- RLS policy
ALTER TABLE engine_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY relationships_project_isolation ON engine_relationships
    USING (project_id = current_setting('app.current_project_id')::uuid);
```

---

## DAG Integration

### New/Enhanced DAG Nodes

The existing DAG has:
- Node 5: FKDiscovery
- Node 8: RelationshipEnrichment

**Enhance these nodes to:**
1. Use Column Features (from Node 2) as input - no column name scanning
2. Run data overlap analysis between identifier columns
3. Present overlap results to LLM for semantic classification
4. Store relationships in `engine_relationships` table
5. Update Column Features with FK target info

### Data Flow

```
Node 2: ColumnFeatureExtraction
    ├─ Phase 1: Collect profiles (sample values, data types)
    ├─ Phase 2: Classify columns (identifier, primary_key, foreign_key, etc.)
    └─ Output: IdentifierFeatures for each column

Node 5: FKDiscovery (ENHANCED)
    ├─ Input: Columns with IdentifierFeatures
    ├─ Filter: Non-PK identifier columns = FK candidates
    ├─ For each candidate: Find PK columns with matching data types
    ├─ Run overlap analysis (sample-based, deterministic)
    └─ Output: FKCandidate with OverlapResults

Node 8: RelationshipEnrichment (ENHANCED)
    ├─ Input: FKCandidates with high overlap (>=50%)
    ├─ Build LLM prompt with overlap metrics + schema context
    ├─ LLM classifies: is_fk, role, entities, confidence
    ├─ Run full verification queries (match_rate, cardinality, orphans)
    ├─ Store in engine_relationships
    └─ Update IdentifierFeatures with FKTargetTable/Column
```

---

## MCP Tools: Surfacing Relationships

### Tool 1: `get_context` Includes Relationships

At `depth='columns'`, FK columns show their verified target:

```json
{
  "table": "billing_engagements",
  "columns": [
    {
      "column_name": "host_id",
      "data_type": "text",
      "references": {
        "table": "users",
        "column": "user_id",
        "cardinality": "N:1",
        "match_rate": 99.8,
        "role": "host"
      }
    },
    {
      "column_name": "visitor_id",
      "data_type": "text",
      "references": {
        "table": "users",
        "column": "user_id",
        "cardinality": "N:1",
        "match_rate": 99.5,
        "role": "visitor"
      }
    }
  ]
}
```

### Tool 2: `probe_relationship` Returns Verified Data

Current state: Returns empty `{"relationships": []}`

Fixed state:
```json
{
  "relationships": [
    {
      "source": {
        "table": "billing_engagements",
        "column": "host_id",
        "entity": "Billing Engagement",
        "role": "host"
      },
      "target": {
        "table": "users",
        "column": "user_id",
        "entity": "User"
      },
      "cardinality": "N:1",
      "match_rate": 99.8,
      "orphan_count": 2,
      "verified_at": "2026-01-30T10:00:00Z"
    }
  ]
}
```

### Tool 3: `get_join_path` (New Tool)

Given two tables, return the verified path to join them:

```
get_join_path(from_table='billing_engagements', to_table='accounts')
```

Response:
```json
{
  "from_table": "billing_engagements",
  "to_table": "accounts",
  "paths": [
    {
      "description": "Via host user",
      "hops": [
        {
          "from": "billing_engagements.host_id",
          "to": "users.user_id",
          "cardinality": "N:1",
          "role": "host"
        },
        {
          "from": "users.account_id",
          "to": "accounts.account_id",
          "cardinality": "N:1"
        }
      ],
      "total_hops": 2,
      "sql_hint": "JOIN users ON host_id = users.user_id JOIN accounts ON users.account_id = accounts.account_id"
    },
    {
      "description": "Via visitor user",
      "hops": [
        {
          "from": "billing_engagements.visitor_id",
          "to": "users.user_id",
          "cardinality": "N:1",
          "role": "visitor"
        },
        {
          "from": "users.account_id",
          "to": "accounts.account_id",
          "cardinality": "N:1"
        }
      ],
      "total_hops": 2,
      "sql_hint": "JOIN users ON visitor_id = users.user_id JOIN accounts ON users.account_id = accounts.account_id"
    }
  ]
}
```

### Tool 4: `validate_query` Enhancement

Before executing a query, validate that JOINs are correct:

```
validate(sql="SELECT * FROM billing_engagements be JOIN users u ON be.host_id = u.user_id")
```

Response includes relationship validation:
```json
{
  "syntax_valid": true,
  "joins_valid": true,
  "join_details": [
    {
      "join": "billing_engagements.host_id = users.user_id",
      "verified": true,
      "cardinality": "N:1",
      "match_rate": 99.8
    }
  ]
}
```

---

## Implementation Tasks

### Task 1: Enhance Phase 4 FK Resolution

**File:** `pkg/services/column_feature_extraction.go`

Ensure Phase 4 properly discovers FK targets using data overlap (not column names).

**Acceptance criteria:**
- FK candidates from Phase 2 classification only
- Overlap analysis against PK columns in other tables
- Results stored in IdentifierFeatures.FKTarget*
- No string pattern matching on column names

---

### Task 2: Create Relationship Storage Table

**File:** `migrations/XXX_relationships.sql`

Create `engine_relationships` table (schema above).

**Acceptance criteria:**
- Stores source/target table/column references
- Stores verification data (match_rate, cardinality, orphans)
- Stores LLM classification (role, entities, confidence)
- RLS policy applied

---

### Task 3: Implement Relationship Repository

**File:** `pkg/repositories/relationship_repository.go`

CRUD operations for relationships.

**Acceptance criteria:**
- Upsert by (project, source_table, source_column, target_table, target_column)
- List by project, filter by table/entity
- Get verification metrics
- Find paths between tables (BFS/DFS graph traversal)

---

### Task 4: Enhance RelationshipEnrichment DAG Node

**File:** `pkg/services/dag/relationship_enrichment_node.go`

Run LLM classification and store relationships.

**Acceptance criteria:**
- Input: FKCandidates from FKDiscovery with overlap >= 50%
- LLM prompt includes overlap metrics, schema context
- Output: Classified relationships with roles/entities
- Store in engine_relationships table

---

### Task 5: Update `get_context` to Include FK References

**File:** `pkg/mcp/tools/context_tools.go`

At `depth='columns'`, include `references` for FK columns.

**Acceptance criteria:**
- Query engine_relationships for each column
- Add references object with table, column, cardinality, match_rate, role
- Null/missing relationships omitted

---

### Task 6: Fix `probe_relationship`

**File:** `pkg/mcp/tools/probe_tools.go`

Return actual relationship data from `engine_relationships` table.

**Acceptance criteria:**
- Query engine_relationships by from_entity/to_entity filters
- Return all verified relationships with full metadata
- Include verification_at, orphan_count, LLM confidence

---

### Task 7: Implement `get_join_path` Tool

**File:** `pkg/mcp/tools/relationship_tools.go`

New tool to find paths between tables.

**Acceptance criteria:**
- Build graph from engine_relationships
- BFS to find all paths up to 3 hops
- Return SQL hints for each path
- Handle multiple paths (via host vs via visitor)

---

## Expected Outcome

**Before:**
```
MCP Client: "How do I join billing_engagements to accounts?"
Action: Guess based on column names, trial and error
Result: May hallucinate, waste tokens, get wrong answer
```

**After:**
```
MCP Client: get_join_path(from='billing_engagements', to='accounts')
Response: {
  "paths": [
    {"sql_hint": "JOIN users ON host_id = user_id JOIN accounts ON account_id = account_id"}
  ]
}
Action: Use verified path
Result: Correct query, no guessing
```

---

## Success Metrics

| Metric | Before | After |
|--------|--------|-------|
| `probe_relationship` returns data | No (empty) | Yes |
| FK columns show references in `get_context` | No | Yes |
| Join paths discoverable | No | Yes (new tool) |
| MCP client needs to guess joins | Yes | No |
| Hallucinated joins | Common | Rare |
| Column name pattern matching | Yes ❌ | No ✅ |

---

## Key Design Decisions

### Why No Column Name Pattern Matching?

1. **Unreliable**: Not all FKs follow naming conventions
2. **False positives**: `status_id` might be an enum, not a FK to `statuses` table
3. **Misses valid FKs**: `creator` column → `users.user_id` wouldn't be found
4. **LLM already classifies**: Phase 2 determines `identifier` vs other types
5. **Data is ground truth**: If values match, it's a relationship - regardless of name

### Why Use Data Overlap?

1. **Deterministic**: 98% match rate is a fact, not a guess
2. **Universal**: Works regardless of naming conventions
3. **Verifiable**: MCP clients can trust the match_rate metric
4. **Efficient**: Sample-based overlap is fast (50 values per column)
5. **Scalable**: Full verification only runs on high-confidence candidates

---

## Dependencies

- **Column Features implemented** (DAG Node 2) ✅
- **Assumes Entities implemented** (PLAN-entity-promotion-model.md)
- Relationships should reference entities where applicable
- Role detection (host, visitor) determined by LLM from context

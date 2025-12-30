# PLAN: Intelligent Relationship Detection

## Problem Statement

Current relationship detection is inadequate:
- **Numeric IDs excluded** - Auto-increment IDs naturally overlap but represent real foreign keys
- **Value-matching only** - Misses semantic relationships (e.g., `user_id` → `users.id` when naming is clear)
- **No LLM intelligence** - Can't reason about column names, business context, or partial matches
- **No user verification** - Ambiguous relationships not surfaced for clarification
- **Runs separately from ontology** - Should share workflow infrastructure and column scan data

## Solution

Create a **unified workflow** with two phases:
1. **Relationships Phase** - Scan columns, detect relationships, get user approval
2. **Ontology Phase** - Reuse scan data, build tiered ontology

Relationships must complete before ontology can start. Both phases share:
- The same workflow table (`engine_ontology_workflows` with new `phase` field)
- The same entity state table (`engine_workflow_state` with column scan data)
- The same task queue infrastructure

---

## Key Design Decisions

### 1. Unified Workflow Table

Extend `engine_ontology_workflows` rather than creating a new table:

```sql
ALTER TABLE engine_ontology_workflows
ADD COLUMN phase VARCHAR(20) NOT NULL DEFAULT 'relationships';
-- phase: 'relationships' | 'ontology'
```

**Why:**
- Same state machine (pending, running, paused, completed, failed)
- Same task queue pattern
- Same progress tracking
- Same ownership/heartbeat mechanism
- Relationships phase completes → workflow stays completed until ontology is started

### 2. No Questions - Just Candidates

Instead of asking questions, the LLM creates **relationship candidates** with confidence scores:

| Confidence | Action | User Experience |
|------------|--------|-----------------|
| ≥ 0.85 (High) | Auto-accepted | Shown as confirmed, user can delete if wrong |
| 0.50-0.84 (Medium) | Requires review | `is_required=true`, user must accept/reject |
| < 0.50 (Low) | Auto-rejected | Not shown (logged for debugging) |

**Why no questions:**
- Questions create friction - users don't want to answer 20 questions
- LLM can make a decision with reasoning, user just validates
- Faster workflow - scan → analyze → done (with review)

### 3. Candidate Model

```go
type RelationshipCandidate struct {
    ID              uuid.UUID
    WorkflowID      uuid.UUID
    DatasourceID    uuid.UUID

    // Source and target
    SourceColumnID  uuid.UUID
    TargetColumnID  uuid.UUID

    // Detection results
    DetectionMethod string   // 'value_match', 'name_inference', 'llm'
    Confidence      float64  // 0.0-1.0
    LLMReasoning    string   // Why LLM thinks this is/isn't a relationship

    // Metrics from detection (sample-based)
    ValueMatchRate  *float64 // NULL if not value-matched
    NameSimilarity  *float64 // Column name similarity score

    // Metrics from test join (actual SQL join)
    Cardinality     *string  // "1:1", "1:N", "N:1", "N:M"
    JoinMatchRate   *float64 // Actual match rate from join
    OrphanRate      *float64 // % of source rows with no match
    TargetCoverage  *float64 // % of target rows that are referenced
    SourceRowCount  *int64   // Total source rows
    TargetRowCount  *int64   // Total target rows
    MatchedRows     *int64   // Source rows with matches
    OrphanRows      *int64   // Source rows without matches

    // Review state
    Status          string   // 'pending', 'accepted', 'rejected'
    IsRequired      bool     // Must user make a call before save?
    UserDecision    *string  // 'accepted', 'rejected' (after user action)

    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

### 4. User Flow

```
┌────────────────────────────────────────────────────────────────────┐
│                     Relationships Page                              │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  ┌─────────────────────┐   ┌────────────────────────────────────┐ │
│  │    Work Queue       │   │    Relationship Candidates          │ │
│  │                     │   │                                    │ │
│  │  ✓ users            │   │  CONFIRMED (3)                     │ │
│  │  ✓ orders           │   │  ┌────────────────────────────┐   │ │
│  │  ● products...      │   │  │ ✓ orders.user_id → users.id │   │ │
│  │  ◌ payments         │   │  │   Confidence: 95% (LLM)     │   │ │
│  │  ◌ line_items       │   │  │   [Delete]                  │   │ │
│  │                     │   │  └────────────────────────────┘   │ │
│  │  Progress: 2/5      │   │                                    │ │
│  │  ▓▓▓▓░░░░░░ 40%     │   │  NEEDS REVIEW (1)                 │ │
│  │                     │   │  ┌────────────────────────────┐   │ │
│  └─────────────────────┘   │  │ ? orders.product_id → ???   │   │ │
│                            │  │   Confidence: 65% (LLM)     │   │ │
│                            │  │   "Column name suggests FK   │   │ │
│                            │  │    but no matching table"   │   │ │
│                            │  │   [Accept] [Reject]         │   │ │
│                            │  └────────────────────────────┘   │ │
│                            │                                    │ │
│                            │  REJECTED (2) [collapsed]          │ │
│                            └────────────────────────────────────┘ │
│                                                                    │
├────────────────────────────────────────────────────────────────────┤
│  ⚠ 1 relationship needs review                                    │
│                                              [Cancel]  [Save]      │
└────────────────────────────────────────────────────────────────────┘
```

**Rules:**
- `[Save]` disabled while workflow running OR any `is_required && status='pending'` candidates exist
- `[Cancel]` discards all candidates and workflow state, returns to dashboard
- Back navigation blocked while dirty (same as Schema page)
- Confirmed relationships can be deleted by user (becomes rejected)
- Rejected relationships can be restored by user (becomes accepted)

---

## Architecture

### Workflow Phases

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Unified Workflow                             │
│                                                                     │
│  Phase 1: RELATIONSHIPS                 Phase 2: ONTOLOGY           │
│  ┌─────────────────────────────┐       ┌─────────────────────────┐ │
│  │                             │       │                         │ │
│  │  1. Scan columns            │       │  1. Skip scanning       │ │
│  │     (populate workflow_state│  ──►  │     (data already       │ │
│  │      with sample values)    │       │      in workflow_state) │ │
│  │                             │       │                         │ │
│  │  2. Deterministic matching  │       │  2. LLM entity analysis │ │
│  │     (value overlap)         │       │                         │ │
│  │                             │       │  3. Build tiered        │ │
│  │  3. Name inference          │       │     ontology            │ │
│  │     (user_id → users.id)    │       │                         │ │
│  │                             │       │                         │ │
│  │  4. LLM analysis            │       │                         │ │
│  │     (confirm/reject/infer)  │       │                         │ │
│  │                             │       │                         │ │
│  │  5. User review             │       │                         │ │
│  │     (accept/reject required)│       │                         │ │
│  │                             │       │                         │ │
│  │  6. Save relationships      │       │                         │ │
│  └─────────────────────────────┘       └─────────────────────────┘ │
│           │                                     │                   │
│           ▼                                     ▼                   │
│   engine_schema_relationships          engine_ontologies           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### State Reuse

The `engine_workflow_state` table stores column scan data:

```json
// engine_workflow_state.state_data.gathered (for column entity)
{
  "row_count": 1000,
  "non_null_count": 950,
  "distinct_count": 100,
  "null_percent": 5.0,
  "sample_values": ["val1", "val2", ...],
  "is_enum_candidate": false,
  "value_fingerprint": "sha256...",
  "scanned_at": "2024-..."
}
```

This data is:
1. Created during Relationships phase (scanning task)
2. Preserved when Relationships phase completes
3. Reused by Ontology phase (no re-scanning needed)
4. Deleted when NEW workflow starts (existing behavior)

---

## Database Changes

### Migration: Extend Workflow Table

```sql
-- Add phase to workflow
ALTER TABLE engine_ontology_workflows
ADD COLUMN phase VARCHAR(20) NOT NULL DEFAULT 'relationships';

-- Add check constraint
ALTER TABLE engine_ontology_workflows
ADD CONSTRAINT engine_ontology_workflows_phase_check
CHECK (phase IN ('relationships', 'ontology'));

-- Add datasource_id (relationships are per-datasource)
ALTER TABLE engine_ontology_workflows
ADD COLUMN datasource_id UUID REFERENCES engine_datasources(id);

COMMENT ON COLUMN engine_ontology_workflows.phase IS
  'Workflow phase: relationships (scanning + FK detection) or ontology (LLM analysis)';
```

### New Table: Relationship Candidates

```sql
CREATE TABLE engine_relationship_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES engine_ontology_workflows(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id),

    -- Source and target columns
    source_column_id UUID NOT NULL REFERENCES engine_schema_columns(id),
    target_column_id UUID NOT NULL REFERENCES engine_schema_columns(id),

    -- Detection metadata
    detection_method VARCHAR(20) NOT NULL,
        -- 'value_match': High value overlap
        -- 'name_inference': Column naming pattern (user_id → users.id)
        -- 'llm': LLM inferred relationship
        -- 'hybrid': Multiple methods agree

    -- Confidence and reasoning
    confidence DECIMAL(3,2) NOT NULL,  -- 0.00-1.00
    llm_reasoning TEXT,                 -- LLM explanation

    -- Metrics from sample-based detection
    value_match_rate DECIMAL(5,4),      -- Sample value overlap (0.0000-1.0000)
    name_similarity DECIMAL(3,2),       -- Column name similarity

    -- Metrics from test join (actual SQL join against datasource)
    cardinality VARCHAR(10),            -- "1:1", "1:N", "N:1", "N:M"
    join_match_rate DECIMAL(5,4),       -- Actual match rate from join
    orphan_rate DECIMAL(5,4),           -- % of source rows with no match
    target_coverage DECIMAL(5,4),       -- % of target rows that are referenced
    source_row_count BIGINT,            -- Total source rows
    target_row_count BIGINT,            -- Total target rows
    matched_rows BIGINT,                -- Source rows with matches
    orphan_rows BIGINT,                 -- Source rows without matches

    -- User review state
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
        -- 'pending': Awaiting user action (if is_required) or auto-decision
        -- 'accepted': Will be saved as relationship
        -- 'rejected': Will not be saved
    is_required BOOLEAN NOT NULL DEFAULT false,
        -- true: User must accept/reject before save
        -- false: Auto-decided based on confidence
    user_decision VARCHAR(20),
        -- 'accepted': User explicitly accepted
        -- 'rejected': User explicitly rejected
        -- NULL: Auto-decided

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(workflow_id, source_column_id, target_column_id)
);

CREATE INDEX idx_rel_candidates_workflow ON engine_relationship_candidates(workflow_id);
CREATE INDEX idx_rel_candidates_status ON engine_relationship_candidates(workflow_id, status);
CREATE INDEX idx_rel_candidates_required ON engine_relationship_candidates(workflow_id)
    WHERE is_required = true AND status = 'pending';

-- RLS
ALTER TABLE engine_relationship_candidates ENABLE ROW LEVEL SECURITY;
CREATE POLICY rel_candidates_access ON engine_relationship_candidates
    FOR ALL
    USING (
        datasource_id IN (
            SELECT id FROM engine_datasources
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- Trigger
CREATE TRIGGER update_rel_candidates_updated_at
    BEFORE UPDATE ON engine_relationship_candidates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
```

---

## Task Types

### Relationships Phase Tasks

```go
const (
    // Phase 1: Relationships
    TaskTypeScanColumn           = "scan_column"            // Scan column values (existing)
    TaskTypeMatchValues          = "match_values"           // Pairwise value matching
    TaskTypeInferFromNames       = "infer_from_names"       // Name-based inference
    TaskTypeTestJoin             = "test_join"              // SQL join to determine cardinality
    TaskTypeAnalyzeRelationships = "analyze_relationships"  // LLM analysis of candidates

    // Phase 2: Ontology (existing)
    TaskTypeProfileTable         = "profile_table"
    TaskTypeUnderstandSchema     = "understand_schema"
    TaskTypeBuildTier0And1       = "build_tier0_and_tier1"
    TaskTypeGenerateQuestions    = "generate_questions"
)
```

### Task Flow

```
Relationships Phase:
  1. scan_column (per column, parallel)     → Populates workflow_state.state_data.gathered
  2. match_values (single task)              → Creates candidates from value overlap
  3. infer_from_names (single task)          → Creates candidates from naming patterns
  4. test_join (per candidate, parallel)     → SQL join to get cardinality + orphan metrics
  5. analyze_relationships (single LLM task) → Confirms/rejects/adds candidates with reasoning

Ontology Phase:
  1. profile_table (per table, parallel)     → Reuses workflow_state scan data
  2. understand_schema (LLM)                 → Entity analysis
  3. build_tier0_and_tier1 (LLM)            → Build ontology
```

---

## Detection Pipeline

### Stage 1: Column Scanning

Reuse existing `ScanTableDataTask` logic from `ontology_tasks.go`:

```go
// Already implemented - stores in workflow_state.state_data.gathered:
// - row_count, non_null_count, distinct_count, null_percent
// - sample_values (up to 50)
// - is_enum_candidate
// - value_fingerprint
```

### Stage 1.5: Joinability Filtering

Determine which columns are candidates for FK relationships. Use **actual null_percent** from scan data rather than column type nullability:

```go
func filterJoinable(scans []ColumnScanData) []ColumnScanData {
    var joinable []ColumnScanData

    for _, scan := range scans {
        // Exclude by data type (types that can't meaningfully be FKs)
        if isExcludedType(scan.DataType) {
            continue
        }

        // Exclude by cardinality (too few distinct values = likely enum, not FK)
        // But allow if it's a primary key target
        if scan.DistinctCount < 3 && !scan.IsPrimaryKey {
            continue
        }

        // Include based on actual data, not schema nullability
        // High null rate is a soft signal, not a hard exclude
        scan.JoinabilityScore = computeJoinabilityScore(scan)
        scan.JoinabilityReason = computeJoinabilityReason(scan)

        joinable = append(joinable, scan)
    }
    return joinable
}

func computeJoinabilityScore(scan ColumnScanData) float64 {
    score := 1.0

    // Penalize high null rate (but don't exclude)
    // Many FKs are nullable (optional relationships)
    if scan.NullPercent > 50 {
        score *= 0.5  // 50%+ nulls = lower confidence but still possible
    } else if scan.NullPercent > 20 {
        score *= 0.8  // 20-50% nulls = slight penalty
    }
    // <20% nulls = no penalty (normal for optional FKs)

    // Boost for FK-like naming patterns
    if strings.HasSuffix(scan.ColumnName, "_id") {
        score *= 1.2
    }

    // Boost for high cardinality (more likely to be FK than enum)
    cardinalityRatio := float64(scan.DistinctCount) / float64(scan.RowCount)
    if cardinalityRatio > 0.1 {
        score *= 1.1
    }

    return min(score, 1.0)
}

func isExcludedType(dataType string) bool {
    excluded := []string{
        "boolean", "bool",
        "json", "jsonb", "xml",
        "bytea", "blob", "binary",
        "point", "line", "polygon", "geometry",
        "timestamp", "timestamptz", "date", "time",  // Temporal types
    }
    lower := strings.ToLower(dataType)
    for _, ex := range excluded {
        if strings.Contains(lower, ex) {
            return true
        }
    }
    return false
}
```

**Key principle:** Don't reject columns just because they're nullable in the schema. Check `null_percent` from actual data:
- 0% nulls = definitely include
- 1-20% nulls = likely optional FK, include
- 20-50% nulls = maybe FK, include with lower confidence
- 50%+ nulls = probably not FK, but still include for LLM review

### Stage 2: Value Matching

```go
func (s *RelationshipDetector) MatchValues(ctx context.Context, workflowID uuid.UUID) error {
    // Get all column scans from workflow_state
    scans, _ := s.stateRepo.GetColumnScans(ctx, workflowID)

    // Filter to joinable columns
    joinable := filterJoinable(scans)

    // Pairwise comparison
    for i, source := range joinable {
        for j, target := range joinable {
            if i >= j || source.TableName == target.TableName {
                continue
            }

            match := matchSampleValues(source.SampleValues, target.SampleValues)
            if match.Rate >= 0.30 { // Lower threshold for LLM review
                s.createCandidate(ctx, workflowID, source, target, "value_match", match)
            }
        }
    }
    return nil
}

func matchSampleValues(source, target []string) MatchResult {
    targetSet := make(map[string]struct{})
    for _, v := range target {
        targetSet[v] = struct{}{}
    }

    matches := 0
    for _, v := range source {
        if _, ok := targetSet[v]; ok {
            matches++
        }
    }

    return MatchResult{
        Rate:         float64(matches) / float64(len(source)),
        MatchedCount: matches,
    }
}
```

### Stage 3: Name Inference

```go
func (s *RelationshipDetector) InferFromNames(ctx context.Context, workflowID uuid.UUID) error {
    columns, _ := s.schemaRepo.GetColumns(ctx, datasourceID)
    tables, _ := s.schemaRepo.GetTables(ctx, datasourceID)

    // Build table lookup (singular + plural forms)
    tableLookup := buildTableLookup(tables)

    for _, col := range columns {
        // Pattern: {table}_id → {table}.id
        if strings.HasSuffix(col.Name, "_id") {
            targetName := strings.TrimSuffix(col.Name, "_id")
            if target, ok := tableLookup[targetName]; ok {
                s.createCandidate(ctx, workflowID, col, target.PKColumn, "name_inference", 0.8)
            }
        }

        // Pattern: column name matches table name
        if target, ok := tableLookup[col.Name]; ok {
            s.createCandidate(ctx, workflowID, col, target.PKColumn, "name_inference", 0.7)
        }
    }
    return nil
}

func buildTableLookup(tables []Table) map[string]*Table {
    lookup := make(map[string]*Table)
    for _, t := range tables {
        lookup[t.Name] = &t
        lookup[singularize(t.Name)] = &t
        lookup[pluralize(t.Name)] = &t
    }
    return lookup
}
```

### Stage 4: Test Join

Run actual SQL joins against the datasource to determine cardinality and orphan metrics:

```go
// JoinMetrics holds results from test join analysis
type JoinMetrics struct {
    Cardinality     string  // "1:1", "1:N", "N:1", "N:M"
    SourceRowCount  int64   // Total rows in source table
    TargetRowCount  int64   // Total rows in target table
    MatchedRows     int64   // Source rows that have matching target
    OrphanRows      int64   // Source rows with no matching target
    OrphanRate      float64 // OrphanRows / SourceRowCount
    MatchRate       float64 // MatchedRows / SourceRowCount
    TargetCoverage  float64 // Distinct matched targets / TargetRowCount
}

func (s *RelationshipDetector) TestJoin(ctx context.Context, candidate *RelationshipCandidate) (*JoinMetrics, error) {
    // Get table/column names for SQL
    sourceTable, sourceColumn := s.getTableColumn(candidate.SourceColumnID)
    targetTable, targetColumn := s.getTableColumn(candidate.TargetColumnID)

    // Query to determine cardinality and match metrics
    query := fmt.Sprintf(`
        WITH join_analysis AS (
            SELECT
                s.%[2]s as source_val,
                COUNT(DISTINCT s.%[2]s) OVER () as source_distinct,
                COUNT(DISTINCT t.%[4]s) OVER () as target_distinct,
                CASE WHEN t.%[4]s IS NOT NULL THEN 1 ELSE 0 END as has_match
            FROM %[1]s s
            LEFT JOIN %[3]s t ON s.%[2]s = t.%[4]s
        ),
        cardinality_check AS (
            SELECT
                source_val,
                COUNT(*) as match_count
            FROM %[1]s s
            JOIN %[3]s t ON s.%[2]s = t.%[4]s
            GROUP BY source_val
        ),
        reverse_check AS (
            SELECT
                t.%[4]s as target_val,
                COUNT(*) as source_count
            FROM %[3]s t
            JOIN %[1]s s ON s.%[2]s = t.%[4]s
            GROUP BY t.%[4]s
        )
        SELECT
            (SELECT COUNT(*) FROM %[1]s) as source_row_count,
            (SELECT COUNT(*) FROM %[3]s) as target_row_count,
            (SELECT COUNT(*) FROM join_analysis WHERE has_match = 1) as matched_rows,
            (SELECT COUNT(*) FROM join_analysis WHERE has_match = 0) as orphan_rows,
            (SELECT MAX(match_count) FROM cardinality_check) as max_source_matches,
            (SELECT MAX(source_count) FROM reverse_check) as max_target_matches,
            (SELECT COUNT(DISTINCT source_val) FROM join_analysis WHERE has_match = 1) as matched_source_distinct,
            (SELECT COUNT(DISTINCT target_val) FROM reverse_check) as matched_target_distinct
    `, sourceTable, sourceColumn, targetTable, targetColumn)

    var (
        sourceRowCount, targetRowCount   int64
        matchedRows, orphanRows          int64
        maxSourceMatches, maxTargetMatches int64
        matchedSourceDistinct, matchedTargetDistinct int64
    )

    err := s.datasource.QueryRow(ctx, query).Scan(
        &sourceRowCount, &targetRowCount,
        &matchedRows, &orphanRows,
        &maxSourceMatches, &maxTargetMatches,
        &matchedSourceDistinct, &matchedTargetDistinct,
    )
    if err != nil {
        return nil, fmt.Errorf("test join failed: %w", err)
    }

    // Determine cardinality from max match counts
    cardinality := determineCardinality(maxSourceMatches, maxTargetMatches)

    metrics := &JoinMetrics{
        Cardinality:    cardinality,
        SourceRowCount: sourceRowCount,
        TargetRowCount: targetRowCount,
        MatchedRows:    matchedRows,
        OrphanRows:     orphanRows,
        OrphanRate:     float64(orphanRows) / float64(sourceRowCount),
        MatchRate:      float64(matchedRows) / float64(sourceRowCount),
        TargetCoverage: float64(matchedTargetDistinct) / float64(targetRowCount),
    }

    // Update candidate with join metrics
    candidate.Cardinality = cardinality
    candidate.JoinMetrics = metrics
    s.repo.UpdateCandidate(ctx, candidate)

    return metrics, nil
}

func determineCardinality(maxSourceMatches, maxTargetMatches int64) string {
    sourceIsOne := maxSourceMatches <= 1
    targetIsOne := maxTargetMatches <= 1

    switch {
    case sourceIsOne && targetIsOne:
        return "1:1"
    case sourceIsOne && !targetIsOne:
        return "1:N"  // One source can have many targets
    case !sourceIsOne && targetIsOne:
        return "N:1"  // Many sources point to one target (typical FK)
    default:
        return "N:M"  // Many-to-many (junction table likely)
    }
}
```

**Cardinality interpretation:**
- `1:1` - Rare, usually indicates same entity split across tables
- `N:1` - **Most common FK pattern** - many orders point to one user
- `1:N` - Inverse of above, usually means we have the direction backwards
- `N:M` - Suggests a junction/association table, or data quality issue

**Orphan rate interpretation:**
- 0% - Perfect referential integrity
- 1-10% - Normal for optional FKs or data in transition
- >10% - Warning sign, may not be a real FK relationship
- >50% - Likely not a real relationship

### Stage 5: LLM Analysis

```go
type RelationshipAnalysisInput struct {
    Tables     []TableContext     `json:"tables"`
    Candidates []CandidateContext `json:"candidates"`
}

// TableContext provides full schema context for each table
type TableContext struct {
    Name      string          `json:"name"`
    RowCount  int64           `json:"row_count"`
    PKColumn  string          `json:"pk_column"`
    Columns   []ColumnContext `json:"columns"`
}

// ColumnContext provides column details including type and FK-likelihood
type ColumnContext struct {
    Name        string  `json:"name"`
    DataType    string  `json:"data_type"`
    IsNullable  bool    `json:"is_nullable"`
    NullPercent float64 `json:"null_percent"`      // Actual null rate from scan
    IsPK        bool    `json:"is_pk,omitempty"`
    IsFK        bool    `json:"is_fk,omitempty"`   // Already confirmed FK
    FKTarget    string  `json:"fk_target,omitempty"` // "table.column" if known FK
    LooksLikeFK bool    `json:"looks_like_fk,omitempty"` // Naming pattern suggests FK
}

type CandidateContext struct {
    ID                string   `json:"id"`
    SourceTable       string   `json:"source_table"`
    SourceColumn      string   `json:"source_column"`
    SourceColumnType  string   `json:"source_column_type"`  // e.g., "uuid", "integer"
    TargetTable       string   `json:"target_table"`
    TargetColumn      string   `json:"target_column"`
    TargetColumnType  string   `json:"target_column_type"`  // e.g., "uuid", "integer"
    DetectionMethod   string   `json:"detection_method"`
    ValueMatchRate    *float64 `json:"value_match_rate,omitempty"`
    SourceSamples     []string `json:"source_samples"`
    TargetSamples     []string `json:"target_samples"`

    // From column scan
    SourceNullPercent *float64 `json:"source_null_percent,omitempty"` // % null in source column

    // From test join
    Cardinality       *string  `json:"cardinality,omitempty"`        // "1:1", "1:N", "N:1", "N:M"
    JoinMatchRate     *float64 `json:"join_match_rate,omitempty"`    // Actual match % from SQL join
    OrphanRate        *float64 `json:"orphan_rate,omitempty"`        // % source rows with no match
    TargetCoverage    *float64 `json:"target_coverage,omitempty"`    // % target rows referenced
    SourceRowCount    *int64   `json:"source_row_count,omitempty"`
    TargetRowCount    *int64   `json:"target_row_count,omitempty"`

    // Other FK-like columns in source table (for context)
    OtherFKColumns    []string `json:"other_fk_columns,omitempty"`   // e.g., ["product_id", "status_id"]
}

type RelationshipAnalysisOutput struct {
    Decisions []CandidateDecision `json:"decisions"`
    NewRelationships []InferredRelationship `json:"new_relationships"`
}

type CandidateDecision struct {
    CandidateID string  `json:"candidate_id"`
    Action      string  `json:"action"`     // "confirm", "reject", "needs_review"
    Confidence  float64 `json:"confidence"` // 0.0-1.0
    Reasoning   string  `json:"reasoning"`
}

type InferredRelationship struct {
    SourceTable  string  `json:"source_table"`
    SourceColumn string  `json:"source_column"`
    TargetTable  string  `json:"target_table"`
    TargetColumn string  `json:"target_column"`
    Confidence   float64 `json:"confidence"`
    Reasoning    string  `json:"reasoning"`
}
```

**LLM Prompt:**

```
You are analyzing database relationships. Given the schema and candidate relationships, determine:

1. CONFIRM candidates that are clearly foreign key relationships
2. REJECT candidates that are coincidental (same values but not related)
3. Mark as NEEDS_REVIEW if uncertain - these require user decision
4. INFER new relationships not detected by value matching

## Schema
{{range .Tables}}
### {{.Name}} ({{.RowCount}} rows)
Primary Key: {{.PKColumn}}
| Column | Type | Nullable | Null% | Notes |
|--------|------|----------|-------|-------|
{{range .Columns}}- {{.Name}} | {{.DataType}} | {{if .IsNullable}}yes{{else}}no{{end}} | {{printf "%.1f" .NullPercent}}% | {{if .IsPK}}PK{{end}}{{if .IsFK}}FK→{{.FKTarget}}{{end}}{{if .LooksLikeFK}}looks like FK{{end}} |
{{end}}
{{end}}

## Candidates to Analyze
{{range .Candidates}}
### Candidate {{.ID}}
**{{.SourceTable}}.{{.SourceColumn}}** ({{.SourceColumnType}}) → **{{.TargetTable}}.{{.TargetColumn}}** ({{.TargetColumnType}})
- Detection method: {{.DetectionMethod}}
{{if .ValueMatchRate}}- Sample Value Match: {{printf "%.0f" (mul .ValueMatchRate 100)}}%{{end}}
{{if .SourceNullPercent}}- Source Null Rate: {{printf "%.1f" .SourceNullPercent}}%{{end}}
{{if .Cardinality}}- Cardinality: {{.Cardinality}}{{end}}
{{if .JoinMatchRate}}- Join Match Rate: {{printf "%.1f" (mul .JoinMatchRate 100)}}% ({{.SourceRowCount}} source rows){{end}}
{{if .OrphanRate}}- Orphan Rate: {{printf "%.1f" (mul .OrphanRate 100)}}% (source rows with no match){{end}}
{{if .TargetCoverage}}- Target Coverage: {{printf "%.1f" (mul .TargetCoverage 100)}}% (of {{.TargetRowCount}} target rows){{end}}
{{if .OtherFKColumns}}- Other FK-like columns in {{.SourceTable}}: {{join .OtherFKColumns ", "}}{{end}}
- Source samples: {{join .SourceSamples ", "}}
- Target samples: {{join .TargetSamples ", "}}
{{end}}

## Interpretation Guide
- **Cardinality N:1** = typical FK (many source rows → one target row)
- **Cardinality 1:N** = likely reverse direction, or source is the parent
- **Cardinality N:M** = suggests junction table or data quality issue
- **Source Null Rate high** = optional FK (normal for 1-20%, suspicious >50%)
- **Orphan Rate >10%** = warning sign, may not be real relationship
- **Orphan Rate >50%** = likely NOT a real FK relationship
- **Target Coverage low** = few target rows are actually used
- **Orphan Rate + Null Rate** = distinguish between optional FK (nulls) vs broken FK (orphans)

Respond in JSON:
{
  "decisions": [
    {
      "candidate_id": "...",
      "action": "confirm|reject|needs_review",
      "confidence": 0.95,
      "reasoning": "Column naming and value overlap strongly suggest FK relationship"
    }
  ],
  "new_relationships": [
    {
      "source_table": "orders",
      "source_column": "customer_id",
      "target_table": "customers",
      "target_column": "id",
      "confidence": 0.9,
      "reasoning": "Standard FK naming pattern, not detected due to low sample overlap"
    }
  ]
}
```

### Confidence → Status Mapping

```go
func (s *RelationshipDetector) ApplyLLMDecisions(ctx context.Context, decisions []CandidateDecision) error {
    for _, d := range decisions {
        candidate, _ := s.repo.GetCandidate(ctx, d.CandidateID)

        candidate.Confidence = d.Confidence
        candidate.LLMReasoning = d.Reasoning

        switch d.Action {
        case "confirm":
            if d.Confidence >= 0.85 {
                candidate.Status = "accepted"
                candidate.IsRequired = false
            } else {
                candidate.Status = "pending"
                candidate.IsRequired = true  // User must confirm
            }
        case "reject":
            if d.Confidence >= 0.85 {
                candidate.Status = "rejected"
                candidate.IsRequired = false
            } else {
                candidate.Status = "pending"
                candidate.IsRequired = true  // User must confirm rejection
            }
        case "needs_review":
            candidate.Status = "pending"
            candidate.IsRequired = true
        }

        s.repo.UpdateCandidate(ctx, candidate)
    }
    return nil
}
```

---

## API Endpoints

### Relationships Workflow

```go
// Start relationship detection
POST /api/projects/{pid}/datasources/{dsid}/relationships/detect
Response: { workflow_id, status }

// Get workflow status
GET /api/projects/{pid}/datasources/{dsid}/relationships/status
Response: {
    workflow_id,
    phase: "relationships",
    state,
    progress,
    task_queue,
    candidates: { confirmed: N, needs_review: N, rejected: N },
    can_save: bool  // false if needs_review > 0
}

// Get candidates
GET /api/projects/{pid}/datasources/{dsid}/relationships/candidates
Response: {
    confirmed: [...],
    needs_review: [...],
    rejected: [...]
}

// User decision on candidate
PUT /api/projects/{pid}/datasources/{dsid}/relationships/candidates/{cid}
Body: { decision: "accepted" | "rejected" }
Response: { candidate }

// Cancel workflow (discard all)
POST /api/projects/{pid}/datasources/{dsid}/relationships/cancel
Response: {}

// Save relationships
POST /api/projects/{pid}/datasources/{dsid}/relationships/save
Response: { saved_count }
// Fails if any is_required && status='pending' candidates exist
```

### Ontology Integration

```go
// Start ontology (checks relationships complete)
POST /api/projects/{pid}/ontology/extract
// Returns 400 if relationships phase not completed for datasource

// Get ontology status shows both phases
GET /api/projects/{pid}/ontology/status
Response: {
    relationships_phase: { state: "completed", ... },
    ontology_phase: { state: "running", ... }
}
```

---

## Frontend

### RelationshipsPage.tsx

```tsx
export function RelationshipsPage() {
  const { pid, dsid } = useParams();
  const navigate = useNavigate();

  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [candidates, setCandidates] = useState<CandidatesByStatus>({});
  const [isDirty, setIsDirty] = useState(false);

  // Poll for workflow status
  useWorkflowPolling(workflow?.id, (updated) => {
    setWorkflow(updated);
    if (updated.state === 'completed') {
      fetchCandidates();
    }
  });

  const canSave = useMemo(() => {
    return workflow?.state === 'completed' &&
           candidates.needs_review.length === 0;
  }, [workflow, candidates]);

  const canCancel = useMemo(() => {
    return isDirty || workflow?.state === 'pending';
  }, [isDirty, workflow]);

  const handleAccept = async (candidateId: string) => {
    await api.updateCandidate(candidateId, { decision: 'accepted' });
    setIsDirty(true);
    fetchCandidates();
  };

  const handleReject = async (candidateId: string) => {
    await api.updateCandidate(candidateId, { decision: 'rejected' });
    setIsDirty(true);
    fetchCandidates();
  };

  const handleSave = async () => {
    await api.saveRelationships(pid, dsid);
    navigate(`/projects/${pid}`);
  };

  const handleCancel = async () => {
    await api.cancelRelationshipWorkflow(pid, dsid);
    navigate(`/projects/${pid}`);
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex justify-between items-center p-4 border-b">
        <div>
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
            disabled={isDirty || candidates.needs_review.length > 0}
          >
            <ArrowLeft /> Back
          </Button>
          <h1>Relationship Detection</h1>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={handleCancel} disabled={!canCancel}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={!canSave}>
            Save Relationships
          </Button>
        </div>
      </div>

      {/* Main: Work Queue + Candidates */}
      <div className="flex flex-1">
        <div className="w-80 border-r p-4">
          <WorkQueue tasks={workflow?.task_queue} progress={workflow?.progress} />
        </div>
        <div className="flex-1 p-4 overflow-auto">
          <CandidateList
            confirmed={candidates.confirmed}
            needsReview={candidates.needs_review}
            rejected={candidates.rejected}
            onAccept={handleAccept}
            onReject={handleReject}
            onDelete={handleReject}  // Delete = reject for confirmed
            onRestore={handleAccept} // Restore = accept for rejected
          />
        </div>
      </div>

      {/* Footer warning */}
      {candidates.needs_review.length > 0 && (
        <div className="p-4 bg-amber-50 border-t border-amber-200">
          <span className="text-amber-700">
            {candidates.needs_review.length} relationship(s) need your review before saving
          </span>
        </div>
      )}
    </div>
  );
}
```

### CandidateCard.tsx

```tsx
interface CandidateCardProps {
  candidate: Candidate;
  variant: 'confirmed' | 'needs_review' | 'rejected';
  onAccept?: () => void;
  onReject?: () => void;
}

function CandidateCard({ candidate, variant, onAccept, onReject }: CandidateCardProps) {
  return (
    <div className={cn(
      "border rounded-lg p-4 mb-2",
      variant === 'confirmed' && "border-green-200 bg-green-50",
      variant === 'needs_review' && "border-amber-200 bg-amber-50",
      variant === 'rejected' && "border-gray-200 bg-gray-50 opacity-60"
    )}>
      <div className="flex justify-between items-start">
        <div>
          <div className="font-medium">
            {candidate.source_table}.{candidate.source_column}
            <span className="mx-2">→</span>
            {candidate.target_table}.{candidate.target_column}
          </div>
          <div className="text-sm text-gray-600 mt-1">
            Confidence: {Math.round(candidate.confidence * 100)}%
            ({candidate.detection_method})
          </div>
          {candidate.llm_reasoning && (
            <div className="text-sm text-gray-500 mt-2 italic">
              "{candidate.llm_reasoning}"
            </div>
          )}
        </div>
        <div className="flex gap-2">
          {variant === 'confirmed' && (
            <Button size="sm" variant="ghost" onClick={onReject}>
              Delete
            </Button>
          )}
          {variant === 'needs_review' && (
            <>
              <Button size="sm" variant="outline" onClick={onReject}>
                Reject
              </Button>
              <Button size="sm" onClick={onAccept}>
                Accept
              </Button>
            </>
          )}
          {variant === 'rejected' && (
            <Button size="sm" variant="ghost" onClick={onAccept}>
              Restore
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
```

---

## Implementation Order

### Milestone 1: Database & Models

1. [x] Create migration extending `engine_ontology_workflows` with `phase` and `datasource_id`
2. [x] Create migration for `engine_relationship_candidates` table
3. [x] Add Go models: `RelationshipCandidate`, update `OntologyWorkflow`
4. [x] Create `RelationshipCandidateRepository`
5. [x] Update `OntologyWorkflowRepository` for phase support

### Milestone 2: Detection Pipeline

1. [ ] Extract column scanning from `ontology_tasks.go` into shared `ColumnScanTask`
2. [ ] Implement `ValueMatchTask` - pairwise sample value comparison
3. [ ] Implement `NameInferenceTask` - naming pattern detection
4. [ ] Implement `TestJoinTask` - SQL join to determine cardinality and orphan metrics
5. [ ] Implement `AnalyzeRelationshipsTask` - LLM analysis with join metrics
6. [ ] Add LLM prompt for relationship analysis

### Milestone 3: Workflow Orchestration

1. [ ] Create `RelationshipWorkflowService` (or extend existing)
2. [ ] Implement relationships phase orchestration
3. [ ] Update `OntologyWorkflowService` to check relationships complete
4. [ ] Update `OntologyWorkflowService` to skip scanning (reuse data)

### Milestone 4: API Handlers

1. [ ] Add relationship workflow endpoints
2. [ ] Add candidate CRUD endpoints
3. [ ] Update ontology endpoints to check relationships prerequisite

### Milestone 5: Frontend

1. [ ] Create `RelationshipsPage`
2. [ ] Create `CandidateList` and `CandidateCard` components
3. [ ] Reuse `WorkQueue` component
4. [ ] Add route and navigation
5. [ ] Update dashboard to show relationship status

### Milestone 6: Integration & Testing

1. [ ] End-to-end test: relationships → ontology flow
2. [ ] Test candidate review UX
3. [ ] Test Cancel/Save behavior
4. [ ] Performance testing with large schemas

---

## Files to Create/Modify

### New Files
- `migrations/XXX_relationship_workflow.up.sql`
- `pkg/models/relationship_candidate.go`
- `pkg/repositories/relationship_candidate_repository.go`
- `pkg/services/relationship_detection.go` - ValueMatch, NameInference, TestJoin
- `pkg/services/relationship_workflow.go`
- `pkg/services/relationship_tasks.go` - Task implementations
- `pkg/prompts/relationship_analysis.go`
- `pkg/handlers/relationship_workflow.go`
- `ui/src/pages/RelationshipsPage.tsx`
- `ui/src/components/relationships/CandidateList.tsx`
- `ui/src/components/relationships/CandidateCard.tsx`
- `ui/src/services/relationshipApi.ts`
- `ui/src/types/relationship.ts`

### Modified Files
- `pkg/models/ontology_workflow.go` - Add `Phase`, `DatasourceID` fields
- `pkg/repositories/ontology_workflow_repository.go` - Support phase queries
- `pkg/services/ontology_workflow.go` - Check relationships prerequisite
- `pkg/services/ontology_tasks.go` - Extract/reuse scanning logic
- `pkg/handlers/ontology.go` - Check relationships before start
- `ui/src/App.tsx` - Add route
- `ui/src/pages/ProjectDashboard.tsx` - Show relationship status

---

## Success Criteria

1. **Detection Accuracy**: >90% of true FKs detected (including numeric IDs)
2. **Precision**: <10% false positive rate
3. **UX**: Clear confidence display, minimal required reviews
4. **Performance**: Full detection <3 minutes for 50-table schema
5. **Integration**: Ontology seamlessly uses scan data and relationships

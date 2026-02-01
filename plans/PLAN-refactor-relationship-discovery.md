# PLAN: Refactor Relationship Discovery to Use LLM Validation

## Problem Statement

The current relationship discovery code produces ~90% incorrect ("stupid") inferred relationships. Examples of bad inferences:

- `id → billing_activity_messages.nonce` (PK pointing to a nonce field)
- `id → channels.marker_at` (PK pointing to a timestamp)
- `account_id → channels.channel_id` (wrong direction)

The root cause: **magic thresholds and heuristics** that work on some datasets but fail mysteriously on others.

Current problematic patterns:
- `distinctCount >= 20` makes any high-cardinality column a potential FK target
- `reverseOrphanRate > 0.5` is an arbitrary threshold
- `areTypesCompatibleForFK()` allows text→text joins without semantic understanding
- Multiple overlapping services with unclear responsibilities

## Design Philosophy

1. **No magic numbers** - Remove all hardcoded thresholds (0.5, 0.8, 10, 20, 0.01, 0.05, 0.1, 0.95, 1.1, etc.)
2. **No column name pattern matching** - Per CLAUDE.md rule #5, we do NOT classify by name patterns
3. **LLM makes semantic decisions** - Deterministic code gathers data, LLM decides validity
4. **Small atomic LLM calls** - One relationship candidate per LLM call for parallelization and progress reporting
5. **User sees progress** - Frequent status updates, not 60+ second waits

## Target Architecture

Follow the pattern established in `column_enrichment.go` and `column_feature_extraction.go`:

```
Phase 1: Deterministic Data Collection (no LLM)
    ↓
Phase 2: LLM Validation (parallel, small requests)
    ↓
Phase 3: Store validated relationships
```

## Implementation Plan

### Task 1: Create `relationship_candidate.go` - Data Structures [x]

Create new data structures for relationship candidate evaluation:

```go
// RelationshipCandidate holds all data needed for LLM to evaluate a potential relationship
type RelationshipCandidate struct {
    // Source column info
    SourceTable      string
    SourceColumn     string
    SourceDataType   string
    SourceIsPK       bool
    SourceDistinct   int64
    SourceNullRate   float64
    SourceSamples    []string  // Up to 10 sample values

    // Target column info
    TargetTable      string
    TargetColumn     string
    TargetDataType   string
    TargetIsPK       bool
    TargetDistinct   int64
    TargetNullRate   float64
    TargetSamples    []string  // Up to 10 sample values

    // Join analysis results (from SQL)
    JoinCount        int64   // Rows that matched
    OrphanCount      int64   // Source values not in target
    ReverseOrphans   int64   // Target values not in source
    SourceMatched    int64   // Distinct source values that matched
    TargetMatched    int64   // Distinct target values that matched

    // Context from ColumnFeatures (if available)
    SourcePurpose    string  // "identifier", "timestamp", "measure", etc.
    SourceRole       string  // "primary_key", "foreign_key", "attribute"
    TargetPurpose    string
    TargetRole       string
}

// RelationshipValidationResult is the LLM response
type RelationshipValidationResult struct {
    IsValidFK        bool    `json:"is_valid_fk"`
    Confidence       float64 `json:"confidence"`        // 0.0-1.0
    Cardinality      string  `json:"cardinality"`       // "1:1", "N:1", "1:N", "N:M"
    Reasoning        string  `json:"reasoning"`         // Why valid/invalid
    SourceRole       string  `json:"source_role"`       // "owner", "creator", etc. (optional)
}
```

**Files:** `pkg/services/relationship_candidate.go`

---

### Task 2: Create `relationship_candidate_collector.go` - Deterministic Collection

Phase 1: Collect relationship candidates using only deterministic criteria. This task is broken into subtasks below.

**Files:** `pkg/services/relationship_candidate_collector.go`

---

#### Task 2.1: Create FK Source Identification Logic [x]

Implement the logic to identify columns that are potential FK sources based on ColumnFeatures data.

**File:** `pkg/services/relationship_candidate_collector.go`

**Context:** The relationship discovery pipeline needs to identify which columns could be foreign keys pointing to other tables. This is Phase 1 of the candidate collection process.

**Implementation:**

1. Create the file with package declaration and imports
2. Define the `RelationshipCandidateCollector` interface:
```go
type RelationshipCandidateCollector interface {
    CollectCandidates(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) ([]*RelationshipCandidate, error)
}
```

3. Create the struct with dependencies:
```go
type relationshipCandidateCollector struct {
    schemaRepo      repositories.SchemaRepository
    columnStatsRepo repositories.ColumnStatisticsRepository
    adapterFactory  adapters.DataSourceAdapterFactory
    logger          *slog.Logger
}

func NewRelationshipCandidateCollector(
    schemaRepo repositories.SchemaRepository,
    columnStatsRepo repositories.ColumnStatisticsRepository,
    adapterFactory adapters.DataSourceAdapterFactory,
    logger *slog.Logger,
) RelationshipCandidateCollector
```

4. Implement `identifyFKSources()` method that returns columns eligible to be FK sources:
   - Query `engine_schema_columns` for the project/datasource
   - Include columns where `column_features->>'role' = 'foreign_key'`
   - Include columns where `column_features->>'purpose' = 'identifier'`
   - Include columns marked as joinable in `engine_column_statistics` (`is_joinable = true`)
   - **Exclude:** Primary keys, timestamps, booleans, JSON types
   - Return `[]*models.SchemaColumn` with their ColumnFeatures populated

**Key constraint per CLAUDE.md rule #5:** Do NOT filter by column name patterns (like `_id` suffix). Use only ColumnFeatures data and explicit schema metadata.

**Test consideration:** This subtask creates the foundation. Tests will be added in Task 7.

---

#### Task 2.2: Create FK Target Identification Logic [x]

Implement the logic to identify columns that are valid FK targets (primary keys and unique columns only).

**File:** `pkg/services/relationship_candidate_collector.go` (continue from 2.1)

**Context:** FK targets must be restricted to PKs and unique columns. The current buggy implementation allows any high-cardinality column as a target, which causes 90% bad inferences.

**Implementation:**

1. Implement `identifyFKTargets()` method:
   - Query `engine_schema_columns` for columns where `is_primary_key = true`
   - Also include columns where a unique constraint exists (check `engine_schema_table_constraints` or `is_unique` flag if available)
   - Return `[]*models.SchemaColumn`

2. Add type compatibility checking helper:
```go
func areTypesCompatible(sourceType, targetType string) bool {
    // Compatible pairs:
    // - Same types (uuid→uuid, int→int, text→text)
    // - int variants (int4→int8, smallint→bigint)
    // - string variants (varchar→text, char→text)
    // Return false for clearly incompatible (text→int, bool→uuid)
}
```

**Key change from current approach:** Targets are ONLY PKs and unique columns - not "any column with 20+ distinct values" as currently implemented in `deterministic_relationship_service.go`.

---

#### Task 2.3: Generate Candidate Pairs with Type Compatibility [ ]

Implement the cross-product logic to generate all valid source→target pairs.

**File:** `pkg/services/relationship_candidate_collector.go` (continue from 2.2)

**Context:** Once sources and targets are identified, we generate all possible pairs filtered by type compatibility. No threshold-based filtering - all pairs go to LLM validation.

**Implementation:**

1. Implement `generateCandidatePairs()` method:
```go
func (c *relationshipCandidateCollector) generateCandidatePairs(
    sources []*models.SchemaColumn,
    targets []*models.SchemaColumn,
) []*RelationshipCandidate {
    var candidates []*RelationshipCandidate
    for _, source := range sources {
        for _, target := range targets {
            // Skip self-references (same table.column)
            if source.TableName == target.TableName && source.ColumnName == target.ColumnName {
                continue
            }
            // Skip if types are incompatible
            if !areTypesCompatible(source.DataType, target.DataType) {
                continue
            }
            candidates = append(candidates, &RelationshipCandidate{
                SourceTable:    source.TableName,
                SourceColumn:   source.ColumnName,
                SourceDataType: source.DataType,
                SourceIsPK:     source.IsPrimaryKey,
                // ... populate from source ColumnFeatures
                TargetTable:    target.TableName,
                TargetColumn:   target.ColumnName,
                TargetDataType: target.DataType,
                TargetIsPK:     target.IsPrimaryKey,
                // ... populate from target ColumnFeatures
            })
        }
    }
    return candidates
}
```

2. Populate ColumnFeatures-derived fields on each candidate:
   - `SourcePurpose`, `SourceRole` from `source.ColumnFeatures`
   - `TargetPurpose`, `TargetRole` from `target.ColumnFeatures`

**Key constraint:** No filtering by thresholds (distinct count, cardinality ratio, etc.). All type-compatible pairs become candidates for LLM validation.

---

#### Task 2.4: Implement Join Statistics Collection [ ]

Implement SQL-based join analysis to populate statistics for each candidate pair.

**File:** `pkg/services/relationship_candidate_collector.go` (continue from 2.3)

**Context:** For each candidate pair, we run SQL queries against the customer datasource to get join statistics. This data helps the LLM make informed decisions.

**Implementation:**

1. Implement `collectJoinStatistics()` method using the datasource adapter:
```go
func (c *relationshipCandidateCollector) collectJoinStatistics(
    ctx context.Context,
    adapter adapters.QueryExecutor,
    candidate *RelationshipCandidate,
) error {
    // Query 1: Join count and source matched
    // SELECT COUNT(*) as join_count, COUNT(DISTINCT s.{source_col}) as source_matched
    // FROM {source_table} s
    // JOIN {target_table} t ON s.{source_col} = t.{target_col}

    // Query 2: Orphan count (source values not in target)
    // SELECT COUNT(DISTINCT s.{source_col}) as orphan_count
    // FROM {source_table} s
    // LEFT JOIN {target_table} t ON s.{source_col} = t.{target_col}
    // WHERE t.{target_col} IS NULL AND s.{source_col} IS NOT NULL

    // Query 3: Reverse orphans (target values not referenced)
    // SELECT COUNT(DISTINCT t.{target_col}) as reverse_orphans
    // FROM {target_table} t
    // LEFT JOIN {source_table} s ON t.{target_col} = s.{source_col}
    // WHERE s.{source_col} IS NULL

    // Populate candidate fields: JoinCount, OrphanCount, ReverseOrphans, SourceMatched, TargetMatched
}
```

2. Add sample value collection:
```go
func (c *relationshipCandidateCollector) collectSampleValues(
    ctx context.Context,
    adapter adapters.QueryExecutor,
    candidate *RelationshipCandidate,
) error {
    // Get up to 10 sample values from source column
    // Get up to 10 sample values from target column
    // Populate candidate.SourceSamples and candidate.TargetSamples
}
```

**Architecture note per CLAUDE.md rule #3:** Customer datasource access MUST use adapters from `pkg/adapters/datasource/`. Get the adapter via `adapterFactory.NewQueryExecutor(ctx, datasource)`.

---

#### Task 2.5: Implement Main CollectCandidates Method [ ]

Wire together all the components and implement the public `CollectCandidates` method.

**File:** `pkg/services/relationship_candidate_collector.go` (complete the implementation)

**Context:** This is the main entry point that orchestrates the candidate collection process with progress reporting.

**Implementation:**

1. Implement `CollectCandidates()`:
```go
func (c *relationshipCandidateCollector) CollectCandidates(
    ctx context.Context,
    projectID, datasourceID uuid.UUID,
    progressCallback dag.ProgressCallback,
) ([]*RelationshipCandidate, error) {
    // Step 1: Get datasource and create adapter
    // progressCallback(0, 5, "Loading schema metadata")

    // Step 2: Identify FK sources
    sources, err := c.identifyFKSources(ctx, projectID, datasourceID)
    // progressCallback(1, 5, fmt.Sprintf("Found %d potential FK sources", len(sources)))

    // Step 3: Identify FK targets
    targets, err := c.identifyFKTargets(ctx, projectID, datasourceID)
    // progressCallback(2, 5, fmt.Sprintf("Found %d FK targets (PKs/unique)", len(targets)))

    // Step 4: Generate candidate pairs
    candidates := c.generateCandidatePairs(sources, targets)
    // progressCallback(3, 5, fmt.Sprintf("Generated %d candidate pairs", len(candidates)))

    // Step 5: Collect join statistics for each candidate (with sub-progress)
    adapter, err := c.adapterFactory.NewQueryExecutor(ctx, datasource)
    for i, candidate := range candidates {
        if err := c.collectJoinStatistics(ctx, adapter, candidate); err != nil {
            c.logger.Warn("failed to collect join stats", "source", candidate.SourceTable+"."+candidate.SourceColumn, "error", err)
            // Continue - missing stats is not fatal, LLM can still evaluate
        }
        if err := c.collectSampleValues(ctx, adapter, candidate); err != nil {
            c.logger.Warn("failed to collect samples", "error", err)
        }
        // Report progress for large candidate sets
        if len(candidates) > 10 && i%10 == 0 {
            progressCallback(4, 5, fmt.Sprintf("Analyzing candidates: %d/%d", i, len(candidates)))
        }
    }
    // progressCallback(5, 5, "Candidate collection complete")

    return candidates, nil
}
```

2. Add constructor registration in dependency injection (likely `pkg/server/dependencies.go` or similar)

3. Add logging throughout with structured fields per existing codebase patterns

**Error handling per CLAUDE.md:** Fail fast on fatal errors (can't load schema, can't create adapter). Log and continue on non-fatal errors (single candidate stats collection fails).

---

### Task 3: Create `relationship_validator.go` - LLM Validation

Phase 2: Validate each candidate with a small, focused LLM call:

```go
type RelationshipValidator interface {
    // ValidateCandidate asks LLM if this is a valid FK relationship.
    // One candidate per call for parallelization and progress reporting.
    ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error)

    // ValidateCandidates validates multiple candidates in parallel using worker pool.
    ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error)
}
```

LLM prompt structure:
```
## Relationship Candidate

Source: {table}.{column} ({data_type}, {purpose}, {role})
  - Distinct values: {n}, Null rate: {n}%
  - Samples: {val1}, {val2}, {val3}...

Target: {table}.{column} ({data_type}, {purpose}, {role})
  - Distinct values: {n}, Null rate: {n}%
  - Samples: {val1}, {val2}, {val3}...

## Join Analysis Results
- {source_matched} of {source_distinct} source values exist in target
- {orphan_count} source values have no match (orphan rate: {n}%)
- {reverse_orphans} target values not referenced by source

## Question
Is {source_table}.{source_column} a foreign key referencing {target_table}.{target_column}?

Consider:
- Do the sample values suggest these columns represent the same entity?
- Is the join direction correct (FK → PK)?
- Does the orphan rate suggest data integrity issues or a false positive?
- What semantic role does this FK represent (if any)?

## Response Format (JSON)
{
  "is_valid_fk": true/false,
  "confidence": 0.0-1.0,
  "cardinality": "N:1" | "1:1" | "1:N" | "N:M",
  "reasoning": "Brief explanation",
  "source_role": "owner" | "creator" | null
}
```

**Files:** `pkg/services/relationship_validator.go`

---

### Task 4: Create `relationship_discovery_service.go` - Orchestration

New unified service that replaces both `DeterministicRelationshipService` and `RelationshipDiscoveryService`:

```go
type RelationshipDiscoveryService interface {
    // DiscoverRelationships runs the full discovery pipeline:
    // 1. Collect candidates (deterministic)
    // 2. Validate with LLM (parallel)
    // 3. Store valid relationships
    DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*RelationshipDiscoveryResult, error)
}

type RelationshipDiscoveryResult struct {
    CandidatesEvaluated  int
    RelationshipsCreated int
    RelationshipsRejected int
    DurationMs           int64
}
```

Orchestration flow:
1. **Preserve existing FK relationships** - DB-declared FKs are always valid, skip LLM
2. **Preserve ColumnFeatures FK relationships** - High-confidence FKs from Phase 4
3. **Collect inference candidates** - For remaining potential relationships
4. **Validate in parallel** - Worker pool with progress reporting
5. **Store validated relationships** - With LLM-provided cardinality and role

**Files:** `pkg/services/relationship_discovery_service.go`

---

### Task 5: Update DAG Node

Update `pk_match_discovery_node.go` to use the new service (or rename to `relationship_discovery_node.go`):

```go
type RelationshipDiscoveryNode struct {
    *BaseNode
    discoverySvc RelationshipDiscoveryService
}

func (n *RelationshipDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
    result, err := n.discoverySvc.DiscoverRelationships(ctx, dag.ProjectID, dag.DatasourceID,
        func(current, total int, msg string) {
            n.ReportProgress(ctx, current, total, msg)
        })
    // ...
}
```

**Files:** `pkg/services/dag/relationship_discovery_node.go`

---

### Task 6: Deprecate Old Services

Mark for removal (but don't delete yet):
- `pkg/services/deterministic_relationship_service.go`
- `pkg/services/relationship_discovery.go` (legacy value-overlap)

Keep but simplify:
- `pkg/services/relationship_utils.go` - Keep `InferCardinality()` and `ReverseCardinality()`

---

### Task 7: Tests

Create comprehensive tests:
- `relationship_candidate_collector_test.go` - Unit tests for candidate collection
- `relationship_validator_test.go` - Unit tests with mocked LLM
- `relationship_discovery_service_test.go` - Integration tests

Test cases:
1. DB-declared FK → Preserved without LLM
2. ColumnFeatures FK (high confidence) → Preserved without LLM
3. UUID text column → PK text column → LLM validates
4. id column → timestamp column → LLM rejects
5. Orphan rate scenarios → LLM decides based on context

---

## Migration Strategy

1. **Feature flag** - Add config option `use_llm_relationship_validation: bool`
2. **Default off** - Ship with flag off, validate in staging
3. **Gradual rollout** - Enable per-project for testing
4. **Deprecation** - Remove old code after validation

## Success Criteria

- Relationship quality improves from ~10% to ~90% correct
- No magic thresholds in the codebase
- Progress updates every 1-2 seconds during discovery
- Total discovery time comparable to current (parallelization offsets LLM latency)

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| LLM latency | Parallel validation with worker pool |
| LLM cost | Filter candidates to only PK targets, skip known FKs |
| LLM errors | Circuit breaker, retry logic (already in place) |
| False negatives | Tune prompt, add few-shot examples if needed |

## File Summary

| File | Purpose |
|------|---------|
| `relationship_candidate.go` | Data structures |
| `relationship_candidate_collector.go` | Phase 1: Deterministic collection |
| `relationship_validator.go` | Phase 2: LLM validation |
| `relationship_discovery_service.go` | Orchestration |
| `dag/relationship_discovery_node.go` | DAG integration |

## Design Decisions

### UUID Columns as High-Priority FK Sources

Columns with `ClassificationPathUUID` should be treated as high-priority FK source candidates in Phase 1 collection. UUID-pattern detection is deterministic and reliable - if a column contains UUIDs, it's very likely an identifier that references something.

### Low-Confidence LLM Decisions

If LLM confidence is not high, **do not create the relationship and do not ask the user**. Given the current state where ~90% of inferred relationships are wrong, we should err on the side of caution. A false negative (missing a real FK) is better than a false positive (showing a stupid relationship).

No "review" workflow for low-confidence decisions - just reject silently.

### Phase 4 FK Resolution Integration

**Trust Phase 4 for target identification, validate with join statistics only.**

| Phase 4 (ColumnFeatures) | Relationship Discovery |
|-------------------------|------------------------|
| Answers: "Where does this FK point?" | Answers: "Is this FK relationship valid?" |
| Semantic inference from samples/context | Validation with actual join data |

For columns where Phase 4 already set `FKTargetTable` with high confidence:

1. **Accept the target** - Don't re-discover, Phase 4 already used LLM for this
2. **Run join analysis** - Get orphan count, cardinality from actual data
3. **If join stats pass** (zero orphans, sensible cardinality) → Store relationship without another LLM call
4. **If join stats fail** (orphans, weird cardinality) → Reject silently (Phase 4 was wrong, but don't ask user about our mistakes)

This saves LLM calls while still catching Phase 4 errors via data validation.

## Resolved Questions

1. **Store rejected candidates for debugging/audit?** - Yes, for internal debugging via `RejectionReason`, but don't surface to user
2. **Show orphan rate in UI?** - Only for accepted relationships, as context
3. **Review workflow for low-confidence?** - No, reject silently instead

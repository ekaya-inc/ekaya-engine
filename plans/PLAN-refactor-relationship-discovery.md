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

Phase 1: Collect relationship candidates using only deterministic criteria:

1. **Get all columns with their features** (ColumnFeatures from Phase 4)
2. **Identify potential FK sources:**
   - Columns with `Role=foreign_key` from ColumnFeatures
   - Columns with `Purpose=identifier` from ColumnFeatures
   - Columns marked as joinable (from stats collection)
   - Exclude: PKs (PKs are targets, not sources), timestamps, booleans, JSON
3. **Identify potential FK targets:**
   - PKs only (not just any high-cardinality column)
   - Unique columns explicitly marked in schema
4. **Generate candidate pairs** - Cross-product of sources × targets with type compatibility
5. **Run SQL join analysis** for each candidate pair to populate join statistics

```go
type RelationshipCandidateCollector interface {
    // CollectCandidates finds all potential FK relationships via deterministic analysis.
    // Returns candidates with join statistics populated.
    CollectCandidates(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) ([]*RelationshipCandidate, error)
}
```

Key changes from current approach:
- **Targets are only PKs/unique columns** - Not any column with 20+ distinct values
- **Sources are guided by ColumnFeatures** - Not arbitrary joinability heuristics
- **No filtering by thresholds** - All candidates go to LLM for validation

**Files:** `pkg/services/relationship_candidate_collector.go`

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

## Open Questions

1. Should we store rejected candidates for debugging/audit? (Currently we do via `RejectionReason`)
2. Should orphan rate be shown in UI with LLM reasoning?
3. Do we need a "review" workflow for low-confidence LLM decisions?

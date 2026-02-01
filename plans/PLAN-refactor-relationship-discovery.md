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

#### Task 2.3: Generate Candidate Pairs with Type Compatibility [x]

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

#### Task 2.4: Implement Join Statistics Collection [x]

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

#### Task 2.5: Implement Main CollectCandidates Method [x]

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

Phase 2: Validate each candidate with a small, focused LLM call. This task is broken into subtasks below.

**Files:** `pkg/services/relationship_validator.go`

---

#### Task 3.1: Create RelationshipValidator Interface and Data Structures [x]

Create the RelationshipValidator interface and ValidatedRelationship result type in a new file.

**File:** `pkg/services/relationship_validator.go`

**Context:** This is Phase 2 of the relationship discovery pipeline. Task 2 created `RelationshipCandidate` (input) and `RelationshipValidationResult` (LLM response). This task creates the validator interface that processes candidates.

**Implementation:**

1. Create the file with package declaration and imports:
```go
package services

import (
    "context"

    "github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
    "github.com/google/uuid"
)
```

2. Define `ValidatedRelationship` struct that combines candidate with validation result:
```go
// ValidatedRelationship combines a candidate with its LLM validation result.
// Only candidates where IsValidFK=true become relationships.
type ValidatedRelationship struct {
    Candidate  *RelationshipCandidate
    Result     *RelationshipValidationResult
    Validated  bool  // true if LLM was called, false if skipped (e.g., DB-declared FK)
}
```

3. Define the `RelationshipValidator` interface:
```go
// RelationshipValidator validates relationship candidates using LLM.
// Each candidate is evaluated independently for parallelization.
type RelationshipValidator interface {
    // ValidateCandidate asks LLM if this is a valid FK relationship.
    // One candidate per call for parallelization and progress reporting.
    ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error)

    // ValidateCandidates validates multiple candidates in parallel using worker pool.
    // Returns all results including rejected candidates (for debugging/audit).
    // progressCallback reports progress as candidates complete.
    ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error)
}
```

4. Add the struct definition (implementation will be in 3.2):
```go
type relationshipValidator struct {
    llmService LLMService
    logger     *slog.Logger
}

func NewRelationshipValidator(llmService LLMService, logger *slog.Logger) RelationshipValidator {
    return &relationshipValidator{
        llmService: llmService,
        logger:     logger,
    }
}
```

**Note:** The actual method implementations will be added in subtasks 3.2 and 3.3.

---

#### Task 3.2: Implement ValidateCandidate with LLM Prompt [x]

Implement the single-candidate validation method with a carefully designed LLM prompt.

**File:** `pkg/services/relationship_validator.go` (continue from 3.1)

**Context:** This method makes one LLM call per candidate. The prompt must present all relevant data and get a structured JSON response. Follow patterns from `column_enrichment.go` and `column_feature_extraction.go` for LLM interaction.

**Implementation:**

1. Create the prompt builder function:
```go
func (v *relationshipValidator) buildValidationPrompt(candidate *RelationshipCandidate) string {
    // Build prompt following this structure:
    /*
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
    */
}
```

2. Implement `ValidateCandidate`:
```go
func (v *relationshipValidator) ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error) {
    prompt := v.buildValidationPrompt(candidate)

    // Use LLMService to make the call (follow existing patterns)
    // Parse JSON response into RelationshipValidationResult
    // Handle parsing errors gracefully

    // Log the decision for debugging
    v.logger.Debug("validated relationship candidate",
        "source", candidate.SourceTable+"."+candidate.SourceColumn,
        "target", candidate.TargetTable+"."+candidate.TargetColumn,
        "is_valid", result.IsValidFK,
        "confidence", result.Confidence,
    )

    return result, nil
}
```

3. Look at existing LLM service usage in `column_feature_extraction.go` or `column_enrichment.go` to match the calling pattern (system prompt, user prompt, JSON parsing).

**Key design decision from plan:** If LLM confidence is low, treat as rejection. Do not create relationship, do not ask user.

---

#### Task 3.3: Implement ValidateCandidates with Worker Pool and Progress Reporting [x]

Implement parallel validation with a worker pool pattern and progress callbacks.

**File:** `pkg/services/relationship_validator.go` (continue from 3.2)

**Context:** For large schemas, there may be 50-200+ candidates. Running them sequentially would take too long. Use a worker pool with progress reporting so the UI can show "Validating relationships: 45/127".

**Implementation:**

1. Add worker pool configuration:
```go
const (
    defaultValidationWorkers = 5  // Parallel LLM calls
)
```

2. Implement `ValidateCandidates`:
```go
func (v *relationshipValidator) ValidateCandidates(
    ctx context.Context,
    projectID uuid.UUID,
    candidates []*RelationshipCandidate,
    progressCallback dag.ProgressCallback,
) ([]*ValidatedRelationship, error) {
    if len(candidates) == 0 {
        return nil, nil
    }

    total := len(candidates)
    progressCallback(0, total, "Starting relationship validation")

    // Create result channel and work channel
    type workItem struct {
        index     int
        candidate *RelationshipCandidate
    }

    workCh := make(chan workItem, total)
    resultCh := make(chan struct {
        index  int
        result *ValidatedRelationship
        err    error
    }, total)

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < defaultValidationWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for work := range workCh {
                result, err := v.ValidateCandidate(ctx, projectID, work.candidate)
                resultCh <- struct {
                    index  int
                    result *ValidatedRelationship
                    err    error
                }{
                    index: work.index,
                    result: &ValidatedRelationship{
                        Candidate: work.candidate,
                        Result:    result,
                        Validated: true,
                    },
                    err: err,
                }
            }
        }()
    }

    // Send work
    go func() {
        for i, c := range candidates {
            workCh <- workItem{index: i, candidate: c}
        }
        close(workCh)
    }()

    // Collect results with progress
    go func() {
        wg.Wait()
        close(resultCh)
    }()

    results := make([]*ValidatedRelationship, total)
    completed := 0
    var firstErr error

    for r := range resultCh {
        if r.err != nil && firstErr == nil {
            firstErr = r.err
            // Continue collecting results, don't fail fast on single LLM error
            v.logger.Warn("relationship validation failed",
                "candidate", candidates[r.index].SourceTable+"."+candidates[r.index].SourceColumn,
                "error", r.err,
            )
        }
        results[r.index] = r.result
        completed++

        // Report progress
        if completed%5 == 0 || completed == total {
            progressCallback(completed, total, fmt.Sprintf("Validated %d/%d candidates", completed, total))
        }
    }

    // Filter out nil results (from errors) and return
    var validResults []*ValidatedRelationship
    for _, r := range results {
        if r != nil {
            validResults = append(validResults, r)
        }
    }

    return validResults, firstErr
}
```

3. Consider context cancellation - if ctx is cancelled, workers should stop:
```go
select {
case work := <-workCh:
    // process
case <-ctx.Done():
    return
}
```

**Error handling per CLAUDE.md:** Don't fail the entire batch if one LLM call fails. Log the error, skip that candidate, continue with others. Only return error if ALL candidates failed or context was cancelled.

---

#### Task 3.4: Add Unit Tests for RelationshipValidator [x]

Create comprehensive unit tests with mocked LLM service.

**File:** `pkg/services/relationship_validator_test.go`

**Context:** Tests should verify the validator interface works correctly without making real LLM calls. Mock the LLMService to return predictable responses.

**Implementation:**

1. Create mock LLM service:
```go
type mockLLMService struct {
    responses map[string]*RelationshipValidationResult  // keyed by "source.col->target.col"
    callCount int
}

func (m *mockLLMService) // implement LLMService interface methods
```

2. Test cases to implement:

```go
func TestValidateCandidate_ValidFK(t *testing.T) {
    // Setup: candidate where user_id -> users.id
    // Mock LLM returns is_valid_fk=true, confidence=0.95, cardinality="N:1"
    // Assert: result matches expected
}

func TestValidateCandidate_InvalidFK(t *testing.T) {
    // Setup: candidate where id -> messages.nonce (bad inference)
    // Mock LLM returns is_valid_fk=false, reasoning="nonce is not an identifier"
    // Assert: result correctly rejected
}

func TestValidateCandidate_LowConfidence(t *testing.T) {
    // Setup: ambiguous candidate
    // Mock LLM returns is_valid_fk=true but confidence=0.4
    // Assert: result has low confidence (caller will reject)
}

func TestValidateCandidates_Parallel(t *testing.T) {
    // Setup: 10 candidates
    // Mock LLM with slight delay to verify parallel execution
    // Assert: all results returned, progress callback called
}

func TestValidateCandidates_PartialFailure(t *testing.T) {
    // Setup: 5 candidates, mock LLM fails on candidate 3
    // Assert: other 4 candidates still validated
    // Assert: error returned but results available
}

func TestValidateCandidates_ContextCancellation(t *testing.T) {
    // Setup: many candidates, cancel context after a few
    // Assert: workers stop, partial results returned
}

func TestBuildValidationPrompt(t *testing.T) {
    // Setup: candidate with all fields populated
    // Assert: prompt contains all expected sections
    // Assert: sample values are included
    // Assert: join statistics are formatted correctly
}
```

3. Test the prompt building separately to ensure all candidate data is included:
```go
func TestBuildValidationPrompt_IncludesAllData(t *testing.T) {
    candidate := &RelationshipCandidate{
        SourceTable: "orders",
        SourceColumn: "user_id",
        // ... fill all fields
    }

    prompt := validator.buildValidationPrompt(candidate)

    assert.Contains(t, prompt, "orders.user_id")
    assert.Contains(t, prompt, "Distinct values:")
    assert.Contains(t, prompt, "Join Analysis Results")
    // etc.
}
```

---

### Task 4: Create `relationship_discovery_service.go` - Orchestration [x]

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

### Task 5: Update DAG Node [x]

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

### Task 6: Deprecate Old Services [x]

Mark for removal (but don't delete yet):
- `pkg/services/deterministic_relationship_service.go`
- `pkg/services/relationship_discovery.go` (legacy value-overlap)

Keep but simplify:
- `pkg/services/relationship_utils.go` - Keep `InferCardinality()` and `ReverseCardinality()`

---

### Task 7: Tests

This task is split into subtasks below.

---

#### Task 7.1: Unit tests for RelationshipCandidateCollector [x]

Create comprehensive unit tests for the relationship candidate collection logic in `pkg/services/relationship_candidate_collector_test.go`.

**Context:** The `RelationshipCandidateCollector` (implemented in Task 2) identifies FK sources from ColumnFeatures, identifies FK targets (PKs and unique columns only), generates candidate pairs with type compatibility filtering, and collects join statistics from the customer datasource.

**Test cases to implement:**

1. **FK source identification:**
   - Columns with `column_features->>'role' = 'foreign_key'` are included
   - Columns with `column_features->>'purpose' = 'identifier'` are included
   - Columns with `is_joinable = true` in statistics are included
   - Primary keys are excluded from FK sources
   - Timestamps, booleans, JSON types are excluded

2. **FK target identification:**
   - Only PKs (`is_primary_key = true`) are included as targets
   - Only unique columns are included as targets
   - Non-unique, non-PK columns are excluded (even high-cardinality ones)

3. **Candidate pair generation:**
   - Self-references (same table.column) are skipped
   - Type-incompatible pairs are skipped (e.g., text→int, bool→uuid)
   - Type-compatible pairs are included (uuid→uuid, int→int, text→text, int4→int8)
   - ColumnFeatures-derived fields (SourcePurpose, SourceRole, TargetPurpose, TargetRole) are populated

4. **Join statistics collection:**
   - JoinCount, OrphanCount, ReverseOrphans, SourceMatched, TargetMatched are populated
   - Sample values (up to 10) are collected for source and target
   - Failure to collect stats for one candidate doesn't fail the entire batch

**Mock dependencies:**
- Mock `SchemaRepository` to return test columns with various ColumnFeatures
- Mock `ColumnStatisticsRepository` to return joinability data
- Mock `DataSourceAdapterFactory` and `QueryExecutor` to return join statistics

**File:** `pkg/services/relationship_candidate_collector_test.go`

---

#### Task 7.2: Unit tests for RelationshipValidator with mocked LLM [x]

Create comprehensive unit tests for the LLM-based relationship validation in `pkg/services/relationship_validator_test.go`.

**Context:** The `RelationshipValidator` (implemented in Task 3) validates relationship candidates using LLM calls. Each candidate gets a focused prompt with source/target info, join statistics, and sample values. The validator returns structured `RelationshipValidationResult` with is_valid_fk, confidence, cardinality, reasoning, and source_role.

**Test cases to implement:**

1. **ValidateCandidate - valid FK:**
   - Setup: candidate where `orders.user_id` → `users.id`
   - Mock LLM returns `is_valid_fk=true, confidence=0.95, cardinality="N:1"`
   - Assert: result matches expected values

2. **ValidateCandidate - invalid FK (bad inference):**
   - Setup: candidate where `id` → `messages.nonce` (PK pointing to nonce field)
   - Mock LLM returns `is_valid_fk=false, reasoning="nonce is not an identifier"`
   - Assert: result correctly rejected

3. **ValidateCandidate - low confidence:**
   - Setup: ambiguous candidate
   - Mock LLM returns `is_valid_fk=true` but `confidence=0.4`
   - Assert: result has low confidence (caller should reject)

4. **ValidateCandidates - parallel execution:**
   - Setup: 10 candidates
   - Mock LLM with configurable responses
   - Assert: all results returned, progress callback called with incremental updates

5. **ValidateCandidates - partial failure:**
   - Setup: 5 candidates, mock LLM fails on candidate 3
   - Assert: other 4 candidates still validated
   - Assert: error returned but results available for successful ones

6. **ValidateCandidates - context cancellation:**
   - Setup: many candidates, cancel context after a few complete
   - Assert: workers stop, partial results returned

7. **buildValidationPrompt - includes all data:**
   - Setup: candidate with all fields populated (samples, stats, ColumnFeatures)
   - Assert: prompt contains source/target info, join analysis results, sample values
   - Assert: response format instructions are included

**Mock dependencies:**
- Create `mockLLMService` implementing `LLMService` interface
- Map responses by source→target key for predictable behavior
- Track call count for parallelism verification

**File:** `pkg/services/relationship_validator_test.go`

---

#### Task 7.3: Integration tests for RelationshipDiscoveryService [ ]

Create integration tests for the full relationship discovery pipeline in `pkg/services/relationship_discovery_service_test.go`.

**Context:** The `RelationshipDiscoveryService` (implemented in Task 4) orchestrates the complete pipeline: preserve DB-declared FKs, preserve ColumnFeatures FKs, collect candidates, validate with LLM, store results. Tests should use real database connections via `testhelpers.GetTestDB(t)` and `testhelpers.GetEngineDB(t)`.

**Test cases to implement:**

1. **DB-declared FK preserved without LLM:**
   - Setup: Create schema with actual FK constraint in test database
   - Execute: Run DiscoverRelationships
   - Assert: FK relationship is preserved, no LLM call made for this relationship
   - Assert: `RelationshipsCreated` includes the DB-declared FK

2. **ColumnFeatures FK preserved without LLM:**
   - Setup: Create column with `column_features->>'role' = 'foreign_key'` and `column_features->>'fk_target_table'` set
   - Execute: Run DiscoverRelationships
   - Assert: Relationship created from ColumnFeatures, no LLM call for this one

3. **UUID text → PK text: LLM validates:**
   - Setup: Create `orders.user_id` (UUID text) and `users.id` (UUID text, PK)
   - Setup: Populate with matching sample data (realistic UUIDs)
   - Mock: LLM returns valid FK
   - Execute: Run DiscoverRelationships
   - Assert: Relationship created with LLM-provided cardinality

4. **id → timestamp: LLM rejects:**
   - Setup: Create column `events.id` (integer) and `logs.created_at` (timestamp)
   - Mock: LLM returns `is_valid_fk=false` (types semantically incompatible)
   - Execute: Run DiscoverRelationships
   - Assert: No relationship created, rejection logged

5. **Orphan rate scenarios - LLM decides based on context:**
   - Setup A: Low orphan rate (5%) - Mock LLM accepts
   - Setup B: High orphan rate (50%) - Mock LLM rejects
   - Setup C: High orphan rate but valid business reason (soft deletes) - Mock LLM accepts
   - Assert: Orphan data is passed to LLM in prompt, LLM decision is respected

6. **Progress callback invoked correctly:**
   - Setup: Multiple candidates requiring validation
   - Execute: Run DiscoverRelationships with progress callback
   - Assert: Callback invoked with incremental progress values
   - Assert: Final progress shows completion

7. **Result statistics correct:**
   - Setup: Mix of DB FKs, ColumnFeatures FKs, valid inferences, rejected inferences
   - Execute: Run DiscoverRelationships
   - Assert: `CandidatesEvaluated`, `RelationshipsCreated`, `RelationshipsRejected` counts are accurate

**Test infrastructure:**
- Use `testhelpers.GetTestDB(t)` for customer datasource simulation
- Use `testhelpers.GetEngineDB(t)` for engine metadata tables
- Create unique `project_id` per test for isolation
- Clean up test data in `t.Cleanup()`

**File:** `pkg/services/relationship_discovery_service_test.go`

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

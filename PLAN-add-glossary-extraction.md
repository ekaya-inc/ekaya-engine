# PLAN: Add Glossary Extraction to Ontology DAG

## Overview

Add automatic business glossary term extraction during ontology workflow. Uses a two-pass approach (Discovery → Enrichment) similar to the existing EntityDiscovery → EntityEnrichment pattern.

## Context

### Already Implemented
- Table: `engine_business_glossary` (migration 025)
- Model: `pkg/models/glossary.go`
- Repository: `pkg/repositories/glossary_repository.go`
- Service: `pkg/services/glossary_service.go` with `SuggestTerms()` method
- Handler: `pkg/handlers/glossary_handler.go`
- MCP Tool: `pkg/mcp/tools/glossary.go`

### Current SuggestTerms Implementation
From `pkg/services/glossary_service.go:159-215`:
- Fetches active ontology and entities
- Builds LLM prompt with domain summary, entities, and column details
- Calls LLM with temperature 0.3
- Parses JSON response into `BusinessGlossaryTerm` slice with source="suggested"
- **Does NOT save to database** - only returns suggestions

## Implementation Plan

### 1. Model Changes

**File:** `pkg/models/ontology_dag.go`

Add two new DAG node constants:
```
DAGNodeGlossaryDiscovery    DAGNodeName = "GlossaryDiscovery"
DAGNodeGlossaryEnrichment   DAGNodeName = "GlossaryEnrichment"
```

Update `DAGNodeOrder` map:
```
DAGNodeGlossaryDiscovery:  8,
DAGNodeGlossaryEnrichment: 9,
```

Update `AllDAGNodes()` function to append the two new nodes after `DAGNodeOntologyFinalization`.

### 2. Service Method Additions

**File:** `pkg/services/glossary_service.go`

Add two new methods to `GlossaryService` interface:

```go
// DiscoverGlossaryTerms identifies candidate business terms from ontology.
// Saves discovered terms to database with source="discovered".
// Returns count of terms discovered.
DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error)

// EnrichGlossaryTerms adds SQL patterns, filters, and aggregations to discovered terms.
// Processes terms in parallel via LLM calls.
// Only enriches terms with source="discovered" that lack enrichment.
EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error
```

**Implementation Notes:**

`DiscoverGlossaryTerms`:
- Similar to existing `SuggestTerms()` but saves to database
- Use `glossaryRepo.Create()` for each discovered term
- Set `source="discovered"` (not "suggested")
- Check for duplicates via `glossaryRepo.GetByTerm()` before inserting
- Return count of newly discovered terms

`EnrichGlossaryTerms`:
- Fetch all terms with `source="discovered"` and `sql_pattern IS NULL`
- Use parallel LLM calls (pattern from `entity_discovery_service.go:EnrichEntitiesWithLLM`)
- Build prompts with ontology context + specific term details
- Parse LLM response to populate `sql_pattern`, `base_table`, `columns_used`, `filters`, `aggregation`
- Update each term via `glossaryRepo.Update()`

### 3. DAG Node Executors

**File:** `pkg/services/dag/glossary_discovery_node.go` (NEW)

```go
// GlossaryDiscoveryMethods interface for DAG node
type GlossaryDiscoveryMethods interface {
    DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error)
}

// GlossaryDiscoveryNode struct
type GlossaryDiscoveryNode struct {
    *BaseNode
    glossaryDiscovery GlossaryDiscoveryMethods
}

// NewGlossaryDiscoveryNode constructor
// Execute() method:
//   - Report progress "Discovering business terms..."
//   - Call glossaryDiscovery.DiscoverGlossaryTerms()
//   - Report completion with term count
```

**File:** `pkg/services/dag/glossary_enrichment_node.go` (NEW)

```go
// GlossaryEnrichmentMethods interface for DAG node
type GlossaryEnrichmentMethods interface {
    EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error
}

// GlossaryEnrichmentNode struct
type GlossaryEnrichmentNode struct {
    *BaseNode
    glossaryEnrichment GlossaryEnrichmentMethods
}

// NewGlossaryEnrichmentNode constructor
// Execute() method:
//   - Report progress "Enriching glossary terms with SQL patterns..."
//   - Call glossaryEnrichment.EnrichGlossaryTerms()
//   - Report completion
```

### 4. DAG Adapters

**File:** `pkg/services/dag_adapters.go`

Add two adapter types:

```go
// GlossaryDiscoveryAdapter adapts GlossaryService for the dag package
type GlossaryDiscoveryAdapter struct {
    svc GlossaryService
}

func NewGlossaryDiscoveryAdapter(svc GlossaryService) dag.GlossaryDiscoveryMethods {
    return &GlossaryDiscoveryAdapter{svc: svc}
}

func (a *GlossaryDiscoveryAdapter) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
    return a.svc.DiscoverGlossaryTerms(ctx, projectID, ontologyID)
}

// GlossaryEnrichmentAdapter adapts GlossaryService for the dag package
type GlossaryEnrichmentAdapter struct {
    svc GlossaryService
}

func NewGlossaryEnrichmentAdapter(svc GlossaryService) dag.GlossaryEnrichmentMethods {
    return &GlossaryEnrichmentAdapter{svc: svc}
}

func (a *GlossaryEnrichmentAdapter) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
    return a.svc.EnrichGlossaryTerms(ctx, projectID, ontologyID)
}
```

### 5. Wire DAG Service

**File:** `pkg/services/ontology_dag_service.go`

Add two new method fields to `ontologyDAGService` struct:
```go
glossaryDiscoveryMethods  dag.GlossaryDiscoveryMethods
glossaryEnrichmentMethods dag.GlossaryEnrichmentMethods
```

Add two setter methods:
```go
func (s *ontologyDAGService) SetGlossaryDiscoveryMethods(methods dag.GlossaryDiscoveryMethods) {
    s.glossaryDiscoveryMethods = methods
}

func (s *ontologyDAGService) SetGlossaryEnrichmentMethods(methods dag.GlossaryEnrichmentMethods) {
    s.glossaryEnrichmentMethods = methods
}
```

Update `getNodeExecutor()` switch statement (around line 594) to add two new cases:

```go
case models.DAGNodeGlossaryDiscovery:
    if s.glossaryDiscoveryMethods == nil {
        return nil, fmt.Errorf("glossary discovery methods not set")
    }
    node := dag.NewGlossaryDiscoveryNode(s.dagRepo, s.glossaryDiscoveryMethods, s.logger)
    node.SetCurrentNodeID(nodeID)
    return node, nil

case models.DAGNodeGlossaryEnrichment:
    if s.glossaryEnrichmentMethods == nil {
        return nil, fmt.Errorf("glossary enrichment methods not set")
    }
    node := dag.NewGlossaryEnrichmentNode(s.dagRepo, s.glossaryEnrichmentMethods, s.logger)
    node.SetCurrentNodeID(nodeID)
    return node, nil
```

### 6. Wire in main.go

**File:** `main.go`

After line 238 (after `SetColumnEnrichmentMethods`), add:

```go
ontologyDAGService.SetGlossaryDiscoveryMethods(services.NewGlossaryDiscoveryAdapter(glossaryService))
ontologyDAGService.SetGlossaryEnrichmentMethods(services.NewGlossaryEnrichmentAdapter(glossaryService))
```

### 7. Testing Requirements

**File:** `pkg/services/dag/glossary_discovery_node_test.go` (NEW)
- Test successful discovery with mocked GlossaryDiscoveryMethods
- Test progress reporting
- Test error handling when methods return errors

**File:** `pkg/services/dag/glossary_enrichment_node_test.go` (NEW)
- Test successful enrichment with mocked GlossaryEnrichmentMethods
- Test progress reporting
- Test error handling when methods return errors

**File:** `pkg/services/glossary_service_test.go` (EXTEND)
- Test `DiscoverGlossaryTerms()` saves to database
- Test `DiscoverGlossaryTerms()` handles duplicates correctly
- Test `EnrichGlossaryTerms()` enriches only unenriched terms
- Test `EnrichGlossaryTerms()` populates all enrichment fields

**File:** `pkg/services/ontology_dag_service_test.go` (EXTEND)
- Test DAG execution includes glossary nodes
- Test glossary nodes run in correct order (after OntologyFinalization)
- Test getNodeExecutor returns correct node types for new node names

## Implementation Order

1. [x] Update `pkg/models/ontology_dag.go` (node constants, order, AllDAGNodes)
   - **COMPLETED**: Added `DAGNodeGlossaryDiscovery` and `DAGNodeGlossaryEnrichment` constants
   - Updated `DAGNodeOrder` map with entries 8 and 9 (after OntologyFinalization which is 7)
   - Added both nodes to `AllDAGNodes()` return slice
   - Pattern follows existing node definitions (EntityDiscovery, EntityEnrichment, etc.)
   - Backward compatibility maintained for deprecated RelationshipDiscovery node
2. [x] Add methods to `pkg/services/glossary_service.go` interface and implementation
   - **COMPLETED**: Added `DiscoverGlossaryTerms` and `EnrichGlossaryTerms` methods to GlossaryService interface
   - **DiscoverGlossaryTerms**: Reuses existing `SuggestTerms` logic (prompt building, LLM call, parsing) but saves to database with source="discovered"
   - Includes duplicate checking via `GetByTerm()` before creating terms
   - Returns count of newly discovered terms (skips duplicates)
   - **EnrichGlossaryTerms**: Fetches unenriched terms (source="discovered" + empty sql_pattern), builds enrichment prompts, calls LLM, parses response, updates database
   - Processes terms sequentially (comment notes parallelization can be added later if needed)
   - Enrichment response includes: sql_pattern, base_table, columns_used, filters, aggregation
   - **Testing**: Added comprehensive unit tests covering success cases, duplicate handling, empty states, and filtering logic
   - Tests verify: term creation with correct source, skipping duplicates, enrichment field population, selective processing of unenriched terms
3. [x] Create `pkg/services/dag/glossary_discovery_node.go`
   - **COMPLETED**: Created new file with GlossaryDiscoveryMethods interface, GlossaryDiscoveryNode struct, NewGlossaryDiscoveryNode constructor, and Execute() method
   - Execute() method reports progress "Discovering business terms..." at start
   - Validates ontology ID is present before proceeding
   - Calls DiscoverGlossaryTerms() service method and returns term count
   - Reports completion with "Discovered N business terms" message
   - Follows existing EntityDiscoveryNode pattern closely
   - **Testing**: Added comprehensive unit tests covering success case, missing ontology ID, discovery errors, progress reporting errors, and node name verification
   - All 5 tests pass successfully
4. [x] Create `pkg/services/dag/glossary_enrichment_node.go`
   - **COMPLETED**: Created new file with GlossaryEnrichmentMethods interface, GlossaryEnrichmentNode struct, NewGlossaryEnrichmentNode constructor, and Execute() method
   - Execute() method reports progress "Enriching glossary terms with SQL patterns..." at start
   - Validates ontology ID is present before proceeding
   - Calls EnrichGlossaryTerms() service method
   - Reports completion with "Glossary enrichment complete" message
   - Follows existing EntityEnrichmentNode and GlossaryDiscoveryNode patterns closely
   - **Testing**: Added comprehensive unit tests (5 tests) covering success case, missing ontology ID, enrichment errors, progress reporting errors, and node name verification
   - All tests pass successfully
5. [x] Update `pkg/services/dag_adapters.go` with two new adapters
   - **COMPLETED**: Added GlossaryDiscoveryAdapter and GlossaryEnrichmentAdapter types at pkg/services/dag_adapters.go:161-188
   - **GlossaryDiscoveryAdapter**: Wraps GlossaryService and implements dag.GlossaryDiscoveryMethods interface
   - Constructor: NewGlossaryDiscoveryAdapter(svc GlossaryService) returns dag.GlossaryDiscoveryMethods
   - Method: DiscoverGlossaryTerms(ctx, projectID, ontologyID) delegates to service
   - **GlossaryEnrichmentAdapter**: Wraps GlossaryService and implements dag.GlossaryEnrichmentMethods interface
   - Constructor: NewGlossaryEnrichmentAdapter(svc GlossaryService) returns dag.GlossaryEnrichmentMethods
   - Method: EnrichGlossaryTerms(ctx, projectID, ontologyID) delegates to service
   - Follows exact pattern of existing adapters (EntityDiscoveryAdapter, ColumnEnrichmentAdapter, etc.)
   - **Testing**: Added comprehensive unit tests in pkg/services/dag_adapters_test.go (6 tests total)
   - Tests cover: success cases, error handling, zero results, context cancellation
   - All 6 tests pass successfully
   - **For next session**: Both adapters are simple delegation wrappers. Ready to wire into ontology_dag_service.go via setter methods.
6. [x] Update `pkg/services/ontology_dag_service.go` (fields, setters, getNodeExecutor)
   - **COMPLETED**: Added glossaryDiscoveryMethods and glossaryEnrichmentMethods fields to ontologyDAGService struct at pkg/services/ontology_dag_service.go:58-59
   - **COMPLETED**: Added SetGlossaryDiscoveryMethods setter at pkg/services/ontology_dag_service.go:140-143
   - **COMPLETED**: Added SetGlossaryEnrichmentMethods setter at pkg/services/ontology_dag_service.go:145-148
   - **COMPLETED**: Updated getNodeExecutor() switch statement at pkg/services/ontology_dag_service.go:665-679
   - Added case for DAGNodeGlossaryDiscovery: checks if methods are set, creates NewGlossaryDiscoveryNode, sets current node ID
   - Added case for DAGNodeGlossaryEnrichment: checks if methods are set, creates NewGlossaryEnrichmentNode, sets current node ID
   - Follows exact pattern of existing node executor cases (EntityDiscovery, ColumnEnrichment, etc.)
   - **Testing**: Added comprehensive unit tests in pkg/services/ontology_dag_service_test.go:812-887
   - TestGetNodeExecutor_GlossaryDiscovery: verifies correct executor returned when methods are set
   - TestGetNodeExecutor_GlossaryDiscovery_NotSet: verifies error when methods are not set
   - TestGetNodeExecutor_GlossaryEnrichment: verifies correct executor returned when methods are set
   - TestGetNodeExecutor_GlossaryEnrichment_NotSet: verifies error when methods are not set
   - TestSetGlossaryMethods: verifies both setter methods work correctly
   - Added test helper implementations: testGlossaryDiscovery and testGlossaryEnrichment at pkg/services/ontology_dag_service_test.go:210-220
   - Extended TestNodeExecutorInterfaces_AreWellDefined to include glossary interfaces at pkg/services/ontology_dag_service_test.go:76-82
   - Extended TestDAGNodes_AllNodesHaveCorrectOrder to include both glossary nodes in expected order at pkg/services/ontology_dag_service_test.go:32-33
   - All tests pass successfully (5 new tests + 2 test extensions)
   - **For next session**: DAG service is now aware of glossary nodes and can instantiate them. Next step is to wire these methods in main.go (task 7).
7. [x] Wire in `main.go`
   - **COMPLETED**: Moved glossaryRepo and glossaryService initialization to line 224-225 (before DAG service creation)
   - Previously these were initialized at line 381-382, but needed to be moved earlier for DAG adapter wiring
   - Added glossary adapter wiring at lines 241-242 after SetColumnEnrichmentMethods
   - `ontologyDAGService.SetGlossaryDiscoveryMethods(services.NewGlossaryDiscoveryAdapter(glossaryService))`
   - `ontologyDAGService.SetGlossaryEnrichmentMethods(services.NewGlossaryEnrichmentAdapter(glossaryService))`
   - Removed duplicate glossaryRepo/glossaryService declarations from line 385-386
   - Updated mockGlossaryService in pkg/mcp/tools/glossary_test.go:55-62 to include DiscoverGlossaryTerms and EnrichGlossaryTerms methods
   - Build successful, all existing tests pass
   - **Architectural note**: The glossary service is now properly wired into both the DAG workflow (for automatic discovery/enrichment) and the HTTP handler layer (for manual term management via API). This dual integration enables both automated ontology-driven term discovery and manual business glossary curation.
   - **For next session**: DAG workflow is now fully wired with glossary nodes. The integration is complete from a wiring perspective. Task 8 (unit tests for nodes) and Task 9 (extending service tests) are remaining, but the core feature is functional. If you want to test the workflow end-to-end, you can start an ontology extraction and the glossary nodes will automatically run after OntologyFinalization.
8. [x] Add unit tests for nodes
   - **COMPLETED**: Created comprehensive test files for both glossary nodes
   - **pkg/services/dag/glossary_discovery_node_test.go**: 5 tests covering success case, missing ontology ID, discovery errors, progress reporting errors, and node name verification
   - TestGlossaryDiscoveryNode_Execute_Success: Verifies successful discovery with mocked methods and term count reporting
   - TestGlossaryDiscoveryNode_Execute_NoOntologyID: Ensures proper error when ontology ID is missing
   - TestGlossaryDiscoveryNode_Execute_DiscoveryError: Tests error handling when discovery fails
   - TestGlossaryDiscoveryNode_Execute_ProgressReportingError: Confirms execution succeeds even if progress reporting fails (non-critical)
   - TestGlossaryDiscoveryNode_Name: Verifies node returns correct name constant
   - **pkg/services/dag/glossary_enrichment_node_test.go**: 5 tests covering success case, missing ontology ID, enrichment errors, progress reporting errors, and node name verification
   - TestGlossaryEnrichmentNode_Execute_Success: Verifies successful enrichment with mocked methods
   - TestGlossaryEnrichmentNode_Execute_NoOntologyID: Ensures proper error when ontology ID is missing
   - TestGlossaryEnrichmentNode_Execute_EnrichmentError: Tests error handling when enrichment fails
   - TestGlossaryEnrichmentNode_Execute_ProgressReportingError: Confirms execution succeeds even if progress reporting fails (non-critical)
   - TestGlossaryEnrichmentNode_Name: Verifies node returns correct name constant
   - **Testing**: All 10 tests pass successfully
   - Both test files include comprehensive mock implementations of GlossaryDiscoveryMethods/GlossaryEnrichmentMethods and OntologyDAGRepository
   - Tests validate progress reporting with message tracking
   - Tests verify correct projectID and ontologyID are passed to service methods
   - Pattern follows existing node test patterns (EntityDiscoveryNode, etc.)
9. [ ] Extend existing service tests

## Key Design Decisions

- **Two-pass approach**: Discovery creates skeleton terms, enrichment adds SQL details (matches Entity pattern)
- **Incremental updates**: Discovery checks for duplicates via `GetByTerm()`, enrichment only processes unenriched terms
- **Parallel enrichment**: Use same pattern as EntityEnrichment for parallel LLM calls
- **Runs after finalization**: Ensures all entity/relationship data is available
- **Source tracking**: Use `source="discovered"` to distinguish from manually created or "suggested" terms

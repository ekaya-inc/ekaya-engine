# PLAN: Dead Code Removal — DAG & Relationship Pipeline

**Status:** TODO
**Branch:** TBD
**Created:** 2026-03-01

## Summary

Periodic dead code sweep targeting the DAG pipeline and relationship inference areas. ~3,200 lines of dead code identified across 10 files (full deletions) plus partial cleanups in 4 more files.

## Dead Code Inventory

### Group 1: `fk_semantic_evaluation` — Orphaned service (1,278 lines)

| File | Lines | Status |
|------|-------|--------|
| `pkg/services/fk_semantic_evaluation.go` | 520 | DELETE |
| `pkg/services/fk_semantic_evaluation_test.go` | 758 | DELETE |

**What it was:** Batch LLM evaluation service for FK candidates. Used the `FKCandidate` struct with table descriptions and domain knowledge. Was invoked by the old `pk_match_discovery_node.go` path.

**Why dead:** No imports outside its own test file. The DAG node that called it (`PKMatchDiscoveryNode`) is itself dead (see Group 4). The active pipeline uses `RelationshipValidator` (one-at-a-time LLM validation) instead.

**Dead types:** `FKCandidate`, `FKEvaluationResult`, `FKSemanticEvaluationService` interface, `fkSemanticEvaluationService` struct, `NewFKSemanticEvaluationService`, `FKCandidateFromAnalysis`.

### Group 2: `pkg/prompts/` — Orphaned package (419 lines)

| File | Lines | Status |
|------|-------|--------|
| `pkg/prompts/relationship_analysis.go` | 203 | DELETE |
| `pkg/prompts/relationship_analysis_test.go` | 216 | DELETE |

**What it was:** Prompt templates for relationship analysis with structured `TableContext`, `ColumnContext`, `CandidateContext` types.

**Why dead:** Zero imports of `"github.com/ekaya-inc/ekaya-engine/pkg/prompts"` anywhere in the codebase. All active relationship prompts are inline in `relationship_validator.go` and `column_feature_extraction.go`.

**Action:** Delete both files and the `pkg/prompts/` directory entirely.

### Group 3: Glossary DAG nodes — Never in pipeline (674 lines)

| File | Lines | Status |
|------|-------|--------|
| `pkg/services/dag/glossary_discovery_node.go` | 73 | DELETE |
| `pkg/services/dag/glossary_discovery_node_test.go` | 266 | DELETE |
| `pkg/services/dag/glossary_enrichment_node.go` | 69 | DELETE |
| `pkg/services/dag/glossary_enrichment_node_test.go` | 266 | DELETE |

**What they were:** DAG node wrappers for glossary discovery and enrichment.

**Why dead:** `AllDAGNodes()` in `pkg/models/ontology_dag.go:122-133` returns 7 nodes and does NOT include `DAGNodeGlossaryDiscovery` or `DAGNodeGlossaryEnrichment`. The `DAGNodeOrder` map (lines 112-120) also excludes them. The switch cases in `getNodeExecutor` (lines 668-684) are unreachable because `createNodes()` only creates nodes from `AllDAGNodes()`.

**Important:** The underlying `GlossaryService.DiscoverGlossaryTerms` and `EnrichGlossaryTerms` methods are NOT dead — they're called via `RunAutoGenerate` from the glossary HTTP handler. Only the DAG node wrappers are dead.

**Associated dead wiring (cleaned in Group 6):**
- `GlossaryDiscoveryAdapter` and `GlossaryEnrichmentAdapter` in `dag_adapters.go`
- `SetGlossaryDiscoveryMethods` and `SetGlossaryEnrichmentMethods` in `ontology_dag_service.go`
- Setter calls in `main.go`
- Model constants `DAGNodeGlossaryDiscovery` and `DAGNodeGlossaryEnrichment`

### Group 4: PKMatch discovery node — Dead fallback (279 lines)

| File | Lines | Status |
|------|-------|--------|
| `pkg/services/dag/pk_match_discovery_node.go` | 81 | DELETE |
| `pkg/services/dag/pk_match_discovery_node_test.go` | 198 | DELETE |

**What it was:** The original threshold-based PKMatch discovery DAG node. Replaced by `RelationshipDiscoveryNode` which uses the LLM-based pipeline.

**Why dead:** In `getNodeExecutor()` at `ontology_dag_service.go:637-650`, the `DAGNodePKMatchDiscovery` case first checks `s.llmRelationshipDiscoveryMethods != nil` and uses `RelationshipDiscoveryNode` if set. Since `main.go:238` always sets this, the fallback to `PKMatchDiscoveryNode` is unreachable.

**Associated dead wiring (cleaned in Group 6):**
- `PKMatchDiscoveryAdapter` in `dag_adapters.go`
- `SetPKMatchDiscoveryMethods` in `ontology_dag_service.go`
- Setter call in `main.go`

### Group 5: Dead code within `deterministic_relationship_service.go` (~470 lines partial)

| File | Lines affected | Status |
|------|---------------|--------|
| `pkg/services/deterministic_relationship_service.go` | ~470 of 1,188 | PARTIAL CLEANUP |

**The file header itself says "DEPRECATED: This file is scheduled for removal."**

**Live portions (still called from `FKDiscoveryAdapter` → `DiscoverFKRelationships`):**
- Lines 1-83: Types, interface, constructor
- Lines 84-607: `DiscoverFKRelationships` and helpers (`discoverSchemaRelationshipsFromColumnFeatures`, `collectColumnStats`, `classifyJoinability`, etc.)
- Lines 1033-1100: `areTypesCompatibleForFK` — called from `column_feature_extraction.go:2541`

**Dead portions (only called from `DiscoverPKMatchRelationships` which is unreachable):**
- Lines 35-38: `PKMatchDiscoveryResult` struct
- Lines 51-53: `DiscoverPKMatchRelationships` interface method
- Lines 608-995: `DiscoverPKMatchRelationships` implementation (~387 lines)
- Lines 996-1031: `isPKMatchExcludedType`
- Lines 1102-1188: `areColumnNamesSemanticallyCompatible`, `extractEntityFromColumnName`, `normalizeTableName` (~86 lines)

**Action:** Remove `DiscoverPKMatchRelationships` method, `PKMatchDiscoveryResult` struct, and the exclusive helper functions. Keep `areTypesCompatibleForFK` and everything needed by `DiscoverFKRelationships`. Remove `DiscoverPKMatchRelationships` from the interface definition.

### Group 6: Dead wiring in `ontology_dag_service.go`, `dag_adapters.go`, `main.go`, and `ontology_dag.go`

**`pkg/services/ontology_dag_service.go`:**
- Remove field `pkMatchDiscoveryMethods` (line 55)
- Remove field `glossaryDiscoveryMethods` (line 59)
- Remove field `glossaryEnrichmentMethods` (line 60)
- Remove setter `SetPKMatchDiscoveryMethods` (lines 125-127)
- Remove setter `SetGlossaryDiscoveryMethods` (lines 147-149)
- Remove setter `SetGlossaryEnrichmentMethods` (lines 151-153)
- Remove switch case for `DAGNodePKMatchDiscovery` fallback (lines 645-650 — keep the `llmRelationshipDiscoveryMethods` path, remove the `pkMatchDiscoveryMethods` fallback)
- Remove switch cases for `DAGNodeGlossaryDiscovery` (lines 668-675) and `DAGNodeGlossaryEnrichment` (lines 677-684)

**`pkg/services/dag_adapters.go`:**
- Remove `PKMatchDiscoveryAdapter` struct and `NewPKMatchDiscoveryAdapter` (lines 40-63)
- Remove `GlossaryDiscoveryAdapter` struct and `NewGlossaryDiscoveryAdapter` (lines 127-139)
- Remove `GlossaryEnrichmentAdapter` struct and `NewGlossaryEnrichmentAdapter` (lines 141-153)

**`main.go`:**
- Remove line 229: `ontologyDAGService.SetPKMatchDiscoveryMethods(services.NewPKMatchDiscoveryAdapter(deterministicRelationshipService))`
- Remove lines 241-242: `SetGlossaryDiscoveryMethods` and `SetGlossaryEnrichmentMethods` calls

**`pkg/models/ontology_dag.go`:**
- Remove constants `DAGNodeGlossaryDiscovery` (line 107) and `DAGNodeGlossaryEnrichment` (line 108)

### Group 7: `ExecutionContext` in node_executor.go (18 lines)

**File:** `pkg/services/dag/node_executor.go` lines 83-100

**Dead types:** `ExecutionContext` struct, `NewExecutionContext` function. Only referenced from test files that test the unused function itself.

**Action:** Remove both. Update test files that reference them.

## Checklist

### Full file deletions
- [ ] Delete `pkg/services/fk_semantic_evaluation.go`
- [ ] Delete `pkg/services/fk_semantic_evaluation_test.go`
- [ ] Delete `pkg/prompts/relationship_analysis.go`
- [ ] Delete `pkg/prompts/relationship_analysis_test.go`
- [ ] Delete `pkg/prompts/` directory
- [ ] Delete `pkg/services/dag/glossary_discovery_node.go`
- [ ] Delete `pkg/services/dag/glossary_discovery_node_test.go`
- [ ] Delete `pkg/services/dag/glossary_enrichment_node.go`
- [ ] Delete `pkg/services/dag/glossary_enrichment_node_test.go`
- [ ] Delete `pkg/services/dag/pk_match_discovery_node.go`
- [ ] Delete `pkg/services/dag/pk_match_discovery_node_test.go`

### Partial cleanups
- [ ] Remove dead code from `pkg/services/deterministic_relationship_service.go` (PKMatchDiscoveryResult, DiscoverPKMatchRelationships, isPKMatchExcludedType, areColumnNamesSemanticallyCompatible, extractEntityFromColumnName, normalizeTableName). Keep areTypesCompatibleForFK and DiscoverFKRelationships path.
- [ ] Remove dead adapters from `pkg/services/dag_adapters.go` (PKMatchDiscoveryAdapter, GlossaryDiscoveryAdapter, GlossaryEnrichmentAdapter)
- [ ] Remove dead fields, setters, and switch cases from `pkg/services/ontology_dag_service.go`
- [ ] Remove dead wiring from `main.go` (3 SetXxxMethods calls)
- [ ] Remove dead constants from `pkg/models/ontology_dag.go` (DAGNodeGlossaryDiscovery, DAGNodeGlossaryEnrichment)
- [ ] Remove `ExecutionContext` and `NewExecutionContext` from `pkg/services/dag/node_executor.go`
- [ ] Update any test files that reference removed types (node_executor_test.go, ontology_dag_service_test.go)

### Verification
- [ ] `go build ./...` — confirms no compile errors
- [ ] `go test ./pkg/services/...` — full service tests pass
- [ ] `go test ./pkg/services/dag/...` — DAG tests pass
- [ ] `go test ./pkg/models/...` — model tests pass
- [ ] `grep -r "fk_semantic_evaluation\|FKSemanticEvaluation\|FKCandidateFromAnalysis\|PKMatchDiscoveryResult\|PKMatchDiscoveryMethods\|GlossaryDiscoveryMethods\|GlossaryEnrichmentMethods\|ExecutionContext" --include="*.go" .` — zero matches confirms clean removal

## Notes

- ~3,200 lines total removal
- No behavioral changes — all removed code is unreachable at runtime
- `areTypesCompatibleForFK` in `deterministic_relationship_service.go` MUST be preserved — it's called from `column_feature_extraction.go:2541`
- The underlying glossary service methods (`DiscoverGlossaryTerms`, `EnrichGlossaryTerms`) are NOT dead — only their DAG node wrappers are
- The `DEPRECATED` header on `deterministic_relationship_service.go` remains accurate — the live portions should eventually be refactored out of this file, but that's a separate task

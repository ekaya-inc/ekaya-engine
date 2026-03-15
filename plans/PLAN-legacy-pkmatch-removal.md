# PLAN: Legacy PK-Match Removal

**Status:** COMPLETED
**Date:** 2026-03-15
**Parent:** `MASTER-PLAN-relationship-extraction-cleanup.md`

## Context

After active relationship-bootstrap responsibilities are extracted, the remaining PK-match path becomes a true legacy path:

- `SetPKMatchDiscoveryMethods(...)` wiring
- fallback selection in `ontology_dag_service.go`
- `PKMatchDiscoveryAdapter`
- `PKMatchDiscoveryNode`
- `DiscoverPKMatchRelationships(...)`
- orphaned FK semantic-evaluation service and prompt package

Today that code is still present, but the intended runtime path is the LLM-backed relationship discovery flow.

## Problem

1. The DAG still advertises and wires a legacy fallback that is not the intended architecture.
2. The deprecated deterministic service still carries a large unreachable PK-match implementation.
3. Orphaned helper packages and tests keep old semantics alive in the tree and make the relationship pipeline harder to reason about.

## Goals

- Remove the unreachable PK-match fallback path from the ontology DAG.
- Delete orphaned service/prompt code that no longer has an active caller.
- Remove the deprecated deterministic relationship service once its active responsibilities are fully extracted.
- Leave the runtime with a single relationship-extraction path per DAG stage.

## Non-Goals

- This plan does not rename the DAG node label `PKMatchDiscovery`.
- This plan does not rename stored `pk_match` inference-method values.
- This plan should not start until `PLAN-relationship-bootstrap-extraction.md` is complete.

## Preconditions

Before implementing this plan:

- early-stage bootstrap logic must no longer live in `deterministic_relationship_service.go`
- shared active helpers must already be moved out of that file
- `FKDiscovery` must already be wired to the new bootstrap service

If those conditions are not true, deleting the legacy service will remove live behavior.

## Removal Scope

### 1. Remove the PK-match fallback from DAG wiring

Delete:

- `pkMatchDiscoveryMethods` field from `pkg/services/ontology_dag_service.go`
- `SetPKMatchDiscoveryMethods(...)`
- the legacy fallback branch in `getNodeExecutor()`
- app wiring in `internal/app/app.go` that still sets PK-match methods

After this change, the late relationship stage should instantiate only the LLM-backed relationship-discovery node.

### 2. Remove the PK-match adapter and node

Delete:

- `PKMatchDiscoveryAdapter` from `pkg/services/dag_adapters.go`
- `pkg/services/dag/pk_match_discovery_node.go`
- `pkg/services/dag/pk_match_discovery_node_test.go`

The DAG should no longer contain a separate executable type for the old threshold-based PK-match implementation.

### 3. Remove the deterministic PK-match implementation

Delete the old deterministic PK-match implementation from the deprecated service.

If the bootstrap-extraction plan fully emptied `deterministic_relationship_service.go`, delete the file entirely.

If a small amount of active code still remains unexpectedly, stop and extract it first rather than leaving the file half-deprecated again.

### 4. Remove orphaned semantic-evaluation artifacts

Delete:

- `pkg/services/fk_semantic_evaluation.go`
- `pkg/services/fk_semantic_evaluation_test.go`
- `pkg/prompts/relationship_analysis.go`
- `pkg/prompts/relationship_analysis_test.go`

These files represent the superseded batch FK semantic-evaluation path and should not remain once the runtime no longer has a PK-match caller.

### 5. Remove dead executor/test helpers

Delete dead helpers whose only purpose was the removed path, including:

- `ExecutionContext`
- `NewExecutionContext`
- tests that exist only to exercise those helpers

## File Targets

Primary files expected to change or be deleted:

- `pkg/services/ontology_dag_service.go`
- `pkg/services/dag_adapters.go`
- `internal/app/app.go`
- `pkg/services/deterministic_relationship_service.go`
- `pkg/services/dag/pk_match_discovery_node.go`
- `pkg/services/fk_semantic_evaluation.go`
- `pkg/prompts/relationship_analysis.go`
- `pkg/services/dag/node_executor.go`
- related tests

## Checklist

- [x] Remove PK-match fallback wiring from ontology DAG service and app wiring
- [x] Remove the PK-match adapter and DAG node
- [x] Delete the deterministic PK-match implementation after active extraction is complete
- [x] Delete orphaned FK semantic-evaluation service/prompt files
- [x] Remove dead executor/test helpers that only supported the removed path

## Expected Automated Checks

- `go test ./pkg/services/...`
- `go test ./pkg/services/dag/...`
- `go test ./pkg/models/...`

# PLAN: Relationship Discovery Naming Cutover

**Status:** COMPLETED
**Date:** 2026-03-15
**Parent:** `MASTER-PLAN-relationship-extraction-cleanup.md`

## Context

The active late-stage relationship pipeline is no longer a PK-match algorithm in the old sense.

Current runtime facts:

- the implementation is `RelationshipDiscoveryNode`
- the node still reports itself as `PKMatchDiscovery`
- the UI still renders multi-phase progress specifically for `PKMatchDiscovery`
- LLM-validated inferred relationships are still stored with `inference_method = "pk_match"`

This naming drift is misleading, but it also touches persisted labels:

- DAG node names already stored in `engine_dag_nodes`
- inference-method values already stored in `engine_schema_relationships`
- frontend types and progress mapping

## Problem

1. The names exposed by the DAG and stored metadata no longer describe the implementation.
2. The codebase already contains both `PKMatchDiscovery` and `RelationshipDiscovery`, which creates drift instead of clarity.
3. A clean rename cannot be done halfway without either:
   - rewriting stored metadata
   - or carrying compatibility code for both names

## Goals

- End with one clear name for the late DAG stage.
- End with one clear inference-method name for LLM-validated inferred relationships.
- Avoid permanent dual-name support.

## Non-Goals

- This plan is not required to complete the runtime cleanup in the master plan.
- This plan should not be used to justify keeping legacy PK-match fallback code alive.

## Approval Requirement

This plan should only be implemented if Damond explicitly approves the cutover of persisted metadata labels.

Reason:

- historical DAG-node rows currently use `PKMatchDiscovery`
- historical relationship rows currently use `pk_match`
- a clean rename without compatibility shims requires rewriting those stored labels or accepting that historical data will remain legacy-labeled forever

Without that explicit approval, stop after the first three plans in the master plan.

## Proposed Cutover

### 1. Canonical DAG node name

Rename the late relationship stage from `PKMatchDiscovery` to `RelationshipDiscovery` across:

- backend DAG node constants
- handler/API responses
- frontend DAG types
- progress rendering and progress-message mapping
- tests

### 2. Canonical inference-method name

Rename the LLM-validated inferred relationship method from `pk_match` to a name that matches what the code actually does.

Recommended value:

- `relationship_discovery`

This is clearer than `pk_match` and does not imply the old deterministic pairwise-join algorithm.

### 3. One-shot stored-metadata cutover

If Damond approves the rename, perform a single cutover for persisted labels instead of supporting both names indefinitely.

That cutover would need to update:

- `engine_dag_nodes.node_name`
- any DAG tables that persist `current_node`
- `engine_schema_relationships.inference_method`

This plan intentionally does not prescribe the exact migration mechanism until approval is given.

## Completion Summary

This plan is complete.

Completed work:

- renamed the late DAG stage to `RelationshipDiscovery` across backend and frontend contracts
- renamed the LLM-inferred relationship method from `pk_match` to `relationship_discovery`
- applied a one-time local metadata cutover for existing `engine_dag_nodes` and `engine_schema_relationships` rows after explicit approval
- removed the duplicate runtime names so the codebase no longer exposes `PKMatchDiscovery` or `pk_match` as active contracts

## File Targets

Primary files expected to change if this plan is approved:

- `pkg/models/ontology_dag.go`
- `pkg/services/dag/relationship_discovery_node.go`
- `pkg/services/ontology_dag_service.go`
- `pkg/models/schema.go`
- `pkg/services/relationship_discovery_service.go`
- `ui/src/types/ontology.ts`
- `ui/src/components/ontology/ExtractionProgress.tsx`
- `ui/src/components/ontology/OntologyDAG.tsx`
- handler/UI tests that hardcode `PKMatchDiscovery` or `pk_match`

## Checklist

- [x] Rename the late DAG stage to `RelationshipDiscovery` across backend and frontend contracts
- [x] Rename the LLM-inferred relationship method from `pk_match` to a clearer canonical name
- [x] Apply a single stored-metadata cutover if and only if Damond explicitly approves it
- [x] Remove any remaining duplicate runtime names after the cutover

## Expected Automated Checks

- `go test ./pkg/services/...`
- `go test ./pkg/handlers/...`
- `go test ./ui/...` or the repo's standard frontend test command

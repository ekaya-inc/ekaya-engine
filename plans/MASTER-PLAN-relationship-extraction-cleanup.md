# MASTER PLAN: Relationship Extraction Cleanup

**Status:** COMPLETED
**Date:** 2026-03-15

## Context

The ontology DAG currently has two relationship-oriented stages:

1. `FKDiscovery`
2. `PKMatchDiscovery`

The current implementation no longer matches those names cleanly.

- The active late-stage relationship inference path is the LLM-backed collector/validator pipeline, but it still runs under the `PKMatchDiscovery` node name.
- The deprecated `deterministic_relationship_service.go` still owns active early-stage behavior that the system depends on:
  - creating `column_features` relationships
  - recomputing cardinality for declared datasource FKs
  - persisting joinability/stat fields later consumed by MCP/context surfaces
- The legacy PK-match fallback path still exists in DAG wiring even though the LLM path is the intended runtime path.
- Contract names have drifted:
  - code still defines declared FK inference method as `foreign_key`
  - schema sync writes `fk`
  - the local metadata store currently contains `fk`, `column_features`, `pk_match`, and `manual`, with zero `foreign_key` rows

This is not just dead-code cleanup. Some logic must be moved before anything can be safely deleted.

## Goals

- Preserve active relationship extraction behavior that downstream stages and tools rely on.
- Extract active early-stage relationship logic out of deprecated legacy services.
- Remove unreachable PK-match fallback code and orphaned legacy helpers.
- Eliminate contract drift where code reads different values than the metadata store writes.
- Keep DAG behavior understandable: an early relationship-bootstrap phase and a later inferred-relationship phase.
- Avoid compatibility layers and dual paths unless Damond explicitly asks for them.

## Non-Goals

- This effort does not redesign relationship semantics, cardinality rules, or MCP relationship-management features.
- This effort does not introduce a new end-user relationship workflow.
- This effort does not rewrite historical DAG or relationship metadata unless explicitly approved.

## Observed State

Code review and metadata inspection on 2026-03-15 found:

- `engine_dag_nodes` contains `PKMatchDiscovery` nodes for all recorded ontology DAG runs.
- `engine_schema_relationships.inference_method` currently contains:
  - `fk`
  - `column_features`
  - `pk_match`
  - `manual`
- `foreign_key` is defined in code but does not appear in the local metadata store.

## Plan Structure

### 1. Contract Alignment

File: `plans/PLAN-relationship-contract-alignment.md`

Align relationship inference contracts with the values the system actually stores today. This fixes the immediate `fk` vs `foreign_key` mismatch and establishes the canonical names used by the rest of the cleanup.

### 2. Active Bootstrap Extraction

File: `plans/PLAN-relationship-bootstrap-extraction.md`

Extract the still-live early-stage relationship logic from the deprecated deterministic service into a clean bootstrap service for the `FKDiscovery` stage, and move column-stat/joinability persistence to the layer that actually owns it.

### 3. Legacy PK-Match Removal

File: `plans/PLAN-legacy-pkmatch-removal.md`

After the active logic has been extracted, delete the dead PK-match fallback path, orphaned prompt/service files, and the remaining deprecated service code that no longer has runtime responsibility.

### 4. Optional Naming Cutover

File: `plans/PLAN-relationship-discovery-naming-cutover.md`

This is intentionally separate. Renaming `PKMatchDiscovery` and `pk_match` to cleaner names is desirable, but it touches persisted labels and UI/API contracts. It should only be implemented if Damond explicitly approves that cutover.

## Recommended Order

1. Implement `PLAN-relationship-contract-alignment.md`
2. Implement `PLAN-relationship-bootstrap-extraction.md`
3. Implement `PLAN-legacy-pkmatch-removal.md`
4. Only then decide whether to implement `PLAN-relationship-discovery-naming-cutover.md`

## Completion Summary

This master plan is complete.

Implemented child plans:

1. `PLAN-relationship-contract-alignment.md`
2. `PLAN-relationship-bootstrap-extraction.md`
3. `PLAN-legacy-pkmatch-removal.md`
4. `PLAN-relationship-discovery-naming-cutover.md`

The naming cutover remained intentionally separate until explicit approval was given because it changed persisted labels. That approval was later given, and the cutover was completed as a follow-on step.

## Key Decisions

- Do not delete `deterministic_relationship_service.go` first. It still owns active bootstrap behavior and stat persistence.
- Do not merge the early and late relationship stages into one DAG node in this effort. `TableFeatureExtraction` consumes relationships before the late inferred-relationship phase, so there is still a real sequencing boundary.
- Do not implement a rename cutover by keeping old and new labels alive together. If the rename happens, it should be a deliberate cutover, not a permanent compatibility layer.

## Approval Gate

Damond approved the stored-metadata decision for the naming cutover, and the system no longer exposes `PKMatchDiscovery` or `pk_match` as active contracts.

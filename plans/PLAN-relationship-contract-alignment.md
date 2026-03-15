# PLAN: Relationship Contract Alignment

**Status:** COMPLETED
**Date:** 2026-03-15
**Parent:** `MASTER-PLAN-relationship-extraction-cleanup.md`

## Context

Relationship extraction currently uses inconsistent contract values for declared datasource foreign keys.

Observed code paths:

- `pkg/services/schema.go` writes declared FK relationships with:
  - `relationship_type = "fk"`
  - `inference_method = "fk"`
- `pkg/models/schema.go` still defines `InferenceMethodForeignKey = "foreign_key"`
- relationship readers in `pkg/services/relationship_discovery_service.go` and `pkg/services/deterministic_relationship_service.go` filter using `models.InferenceMethodForeignKey`

Observed metadata-store state on 2026-03-15:

- `engine_schema_relationships.inference_method` contains `fk`
- `engine_schema_relationships.inference_method` contains zero `foreign_key` rows

This means the codebase currently has a real contract mismatch, not just a naming preference issue.

## Problem

1. Declared FK readers are not aligned with the values written by schema sync.
2. Comments and tests describe a contract the metadata store is not using.
3. The larger relationship-extraction cleanup cannot proceed safely until this contract is made explicit and consistent.

## Goals

- Choose one canonical inference-method value for declared datasource FKs.
- Make writers, readers, tests, and comments agree on that value.
- Fix current code to match live metadata without adding compatibility shims.
- Leave the `pk_match` rename decision for a separate plan.

## Non-Goals

- This plan does not rename `pk_match`.
- This plan does not rename the DAG node `PKMatchDiscovery`.
- This plan does not rewrite historical metadata.

## Proposed Decision

Canonicalize declared datasource FK inference method to `fk`.

Rationale:

- `schema.go` already writes `fk`
- the local metadata store already contains `fk`
- the UI already treats declared foreign-key relationships as `fk` at the relationship-type level
- aligning code to live data avoids a metadata rewrite in this phase

`foreign_key` should be removed as the declared-FK inference-method constant rather than supported indefinitely as a parallel alias.

## Scope

### 1. Canonicalize the model constant

Update `pkg/models/schema.go` so the declared-FK inference-method constant matches the live stored value:

- `InferenceMethodForeignKey` should resolve to `fk`

If the existing constant name becomes misleading after that change, rename the constant in code as part of the same cleanup rather than keeping stale semantics attached to a new value.

### 2. Align all declared-FK readers

Update readers that filter or count declared FKs so they query the canonical value.

Primary call sites from the review:

- `pkg/services/relationship_discovery_service.go`
- `pkg/services/deterministic_relationship_service.go`
- any repository/service tests that assert `foreign_key`

### 3. Align comments and repository/test fixtures

Update comments that currently describe `foreign_key` as the stored declared-FK inference method.

Primary files from the review:

- `pkg/services/schema.go`
- `pkg/services/relationship_discovery_service.go`
- `pkg/services/deterministic_relationship_service.go`
- `pkg/repositories/schema_repository.go`
- associated tests

### 4. Confirm that relationship bootstrap reads real data

After alignment, the declared-FK count/load path should work against the metadata values currently produced by schema sync.

This specifically unblocks:

- counting existing datasource FKs in the LLM discovery service
- recomputing cardinality for declared FKs in the early bootstrap phase

## Implementation Notes

- Do not add dual reads for both `fk` and `foreign_key`.
- Do not add transitional aliases in API responses.
- Keep the change narrowly scoped to declared-FK inference-method alignment.
- Leave `relationship_type = "fk"` unchanged.

## Checklist

- [x] Canonicalize the declared-FK inference-method constant to `fk`
- [x] Update all declared-FK readers to use the canonical value
- [x] Update stale comments that still describe the stored value as `foreign_key`
- [x] Update repository/service tests that assert the old value

## Expected Automated Checks

- `go test ./pkg/repositories/...`
- `go test ./pkg/services/...`

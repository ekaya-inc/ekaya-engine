# PLAN: Relationship Bootstrap Extraction

**Status:** DRAFT
**Date:** 2026-03-15
**Parent:** `MASTER-PLAN-relationship-extraction-cleanup.md`

## Context

The deprecated `pkg/services/deterministic_relationship_service.go` still owns active behavior that the ontology DAG depends on during the early relationship stage.

Today, the `FKDiscovery` stage is doing three different jobs:

1. Create `column_features` relationships from high-confidence `IdentifierFeatures`
2. Recompute cardinality for declared datasource FK relationships
3. Persist per-column joinability/stat fields that were originally needed by the old PK-match path

At the same time, the later LLM-based relationship service duplicates part of that work:

- `pkg/services/relationship_discovery_service.go` also materializes `column_features` relationships before candidate validation

This means active behavior is split across a deprecated early-stage service and a newer late-stage service, with duplicated ownership of ColumnFeatures relationship creation.

## Problem

1. Active relationship-bootstrap logic still lives inside a service the code explicitly marks deprecated.
2. ColumnFeatures relationship materialization is implemented twice.
3. Column-stat/joinability persistence is still attached to legacy PK-match sequencing rather than the layer that actually computes those stats.
4. Safe legacy deletion is blocked until the live behavior has a clean owner.

## Goals

- Extract a clean early-stage relationship-bootstrap service for the `FKDiscovery` DAG node.
- Preserve the ordering guarantee that `TableFeatureExtraction` can read relationships before the late inferred-relationship stage.
- Remove duplicate ownership of ColumnFeatures relationship creation.
- Move column-stat/joinability persistence to the correct owner without dropping MCP/context behavior.

## Non-Goals

- This plan does not remove the late LLM-based relationship discovery stage.
- This plan does not rename `PKMatchDiscovery`.
- This plan does not delete the legacy deterministic service yet; it only extracts the active logic out of it.

## Proposed Design

### 1. Introduce a focused early-stage bootstrap service

Create a dedicated service for the `FKDiscovery` stage. Recommended name:

```go
type RelationshipBootstrapService interface {
    Bootstrap(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*RelationshipBootstrapResult, error)
}
```

This service should own exactly two responsibilities:

1. Materialize high-confidence `column_features` relationships from `ColumnMetadata.IdentifierFeatures`
2. Recompute cardinality / validation metadata for declared datasource FKs already discovered during schema sync

It should not own PK-match candidate search, LLM validation, or orphaned legacy heuristics.

### 2. Move ColumnFeatures relationship creation to the bootstrap service only

`column_features` relationship creation currently exists in both:

- `pkg/services/deterministic_relationship_service.go`
- `pkg/services/relationship_discovery_service.go`

That duplication should end.

After extraction:

- the early bootstrap service is the only place that creates or refreshes `column_features` relationships
- the late LLM service only loads existing relationships, filters candidates against them, validates remaining candidates, and stores newly inferred relationships

### 3. Move joinability/stat persistence out of the deprecated relationship service

The persisted fields below are still used by active surfaces such as MCP `get_context` / `probe`:

- `row_count`
- `non_null_count`
- `distinct_count`
- `is_joinable`
- `joinability_reason`

`ColumnFeatureExtraction` already runs `AnalyzeColumnStats` and persists basic stats via `UpdateColumnStats`.

Preferred cleanup:

- keep raw stat collection in `ColumnFeatureExtraction`
- extend that stage, or a helper it owns, to also persist:
  - `row_count`
  - `non_null_count`
  - `is_joinable`
  - `joinability_reason`
- remove the dependency on `FKDiscovery` to populate those fields

If that update makes `ColumnFeatureExtraction` too crowded, extract a small helper/service owned by that stage rather than keeping the writeback in relationship bootstrap.

### 4. Extract shared active helpers from the deprecated service

At least one active helper currently leaks out of the deprecated file:

- `areTypesCompatibleForFK` is used by `pkg/services/column_feature_extraction.go`

Shared active helpers should be moved to a neutral home, for example:

- `pkg/services/relationship_type_compatibility.go`
- or `pkg/services/relationship_utils.go`

Do not keep active shared helpers stranded in a deprecated file after the bootstrap extraction is complete.

### 5. Rewire `FKDiscovery` to the new service

Once the bootstrap service exists:

- `pkg/services/dag_adapters.go` should adapt the new bootstrap service, not the deprecated deterministic service
- `internal/app/app.go` should wire `FKDiscovery` to the new bootstrap service

The `FKDiscovery` stage remains in the DAG because downstream `TableFeatureExtraction` expects relationship context before the later inferred-relationship stage.

## File Targets

Primary files expected to change:

- new: `pkg/services/relationship_bootstrap_service.go`
- `pkg/services/dag_adapters.go`
- `pkg/services/relationship_discovery_service.go`
- `pkg/services/column_feature_extraction.go`
- `internal/app/app.go`

Likely tests:

- `pkg/services/relationship_discovery_service_test.go`
- new bootstrap service tests
- `pkg/services/column_feature_extraction_test.go`
- DAG adapter/wiring tests as needed

## Checklist

- [ ] Create a focused bootstrap service for early relationship materialization
- [ ] Move `column_features` relationship creation into that bootstrap service
- [ ] Remove duplicate ColumnFeatures creation from the late LLM discovery service
- [ ] Move persisted joinability/stat writeback out of the deprecated relationship service
- [ ] Extract shared active helpers from `deterministic_relationship_service.go`
- [ ] Rewire `FKDiscovery` to the new bootstrap service

## Expected Automated Checks

- `go test ./pkg/services/...`
- `go test ./pkg/services/dag/...`

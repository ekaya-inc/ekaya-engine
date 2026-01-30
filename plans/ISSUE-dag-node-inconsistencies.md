# ISSUE: DAG Node Implementation Inconsistencies

## Summary

Architectural review identified three inconsistencies across DAG node implementations in `pkg/services/dag/`. The nodes generally follow good patterns (BaseNode inheritance, consistent interfaces, adapter pattern), but there are variations that may or may not be intentional.

## Observations

### 1. Error Handling Variance

Some nodes fail-fast on service errors, others log warnings and continue.

**Fail-fast nodes (return error immediately):**
- `entity_enrichment_node.go:56` - Returns error if enrichment fails
- `entity_discovery_node.go:64` - Returns error if DDL analysis fails
- `fk_discovery_node.go:62` - Returns error if discovery fails
- `ontology_finalization_node.go:49` - Returns error if finalization fails

**Graceful degradation nodes (log warning, continue with nil return):**
- `knowledge_seeding_node.go:67-73`:
  ```go
  if err != nil {
      n.Logger().Warn("Knowledge seeding failed, continuing with degraded state",
          zap.Error(err))
      factsStored = 0
  }
  // ... returns nil
  ```
- `glossary_discovery_node.go:60-66`:
  ```go
  if err != nil {
      n.Logger().Warn("Glossary discovery failed, continuing with degraded mode",
          zap.Error(err))
      // ... returns nil
  }
  ```
- `glossary_enrichment_node.go:58-64`:
  ```go
  if err != nil {
      n.Logger().Warn("Glossary enrichment failed, continuing with degraded state",
          zap.String("degradation_type", "glossary_enrichment"),
          zap.Error(err))
      // ... returns nil
  }
  ```

**Question:** Is this intentional (glossary/knowledge features are optional and shouldn't block the DAG) or a violation of the fail-fast philosophy?

### 2. OntologyID Validation Variance

Some nodes validate that `dag.OntologyID` is set before proceeding, others don't.

**Nodes that validate OntologyID:**
- `entity_discovery_node.go:59`
- `entity_enrichment_node.go:51`
- `glossary_discovery_node.go:53`
- `glossary_enrichment_node.go:53`

**Nodes that don't validate (use dag.ProjectID/DatasourceID directly):**
- `entity_promotion_node.go`
- `relationship_enrichment_node.go`
- `column_enrichment_node.go`

**Question:** Do the nodes without validation actually require OntologyID? If so, they could fail with confusing errors when OntologyID is nil.

### 3. Progress Callback Wrapper Duplication

Five nodes have identical progress callback wrapper code:

**Files with duplicated pattern:**
- `fk_discovery_node.go:54-58`
- `pk_match_discovery_node.go:54-58`
- `column_enrichment_node.go:56-60`
- `column_feature_extraction_node.go:74-78`
- `relationship_enrichment_node.go:59-63`

**Pattern (identical in all 5):**
```go
progressCallback := func(current, total int, message string) {
    if err := n.ReportProgress(ctx, current, total, message); err != nil {
        n.Logger().Warn("Failed to report progress", zap.Error(err))
    }
}
```

**Question:** Should this be extracted to `BaseNode` as a helper method to reduce duplication?

## Files to Review

| File | Issues |
|------|--------|
| `pkg/services/dag/knowledge_seeding_node.go` | Error handling (graceful degradation) |
| `pkg/services/dag/glossary_discovery_node.go` | Error handling (graceful degradation) |
| `pkg/services/dag/glossary_enrichment_node.go` | Error handling (graceful degradation) |
| `pkg/services/dag/entity_promotion_node.go` | Missing OntologyID validation |
| `pkg/services/dag/relationship_enrichment_node.go` | Missing OntologyID validation, progress callback duplication |
| `pkg/services/dag/column_enrichment_node.go` | Missing OntologyID validation, progress callback duplication |
| `pkg/services/dag/fk_discovery_node.go` | Progress callback duplication |
| `pkg/services/dag/pk_match_discovery_node.go` | Progress callback duplication |
| `pkg/services/dag/column_feature_extraction_node.go` | Progress callback duplication |
| `pkg/services/dag/node_executor.go` | Potential location for shared progress callback helper |

## Context

- The project follows a "fail-fast" error handling philosophy per CLAUDE.md
- All nodes inherit from `BaseNode` in `node_executor.go`
- The DAG orchestration is in `pkg/services/ontology_dag_service.go`
- Node order is defined in `pkg/models/ontology_dag.go`

## Next Steps

1. Review each graceful degradation case to determine if it's intentional (feature is optional) or a bug
2. Determine which nodes actually need OntologyID and add validation where missing
3. Decide whether to extract progress callback to BaseNode (code cleanliness vs over-engineering)

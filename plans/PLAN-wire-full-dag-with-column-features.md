# Plan: Wire Full DAG with Column Features Integration

## Goal

Re-enable the complete ontology extraction DAG and integrate the new ColumnFeatureExtraction output into all downstream tasks. After implementation, running ontology extraction from scratch should produce a complete, production-ready ontology.

## Current State

### DAG Node Status

| Order | Node | Current Status | Action Needed |
|-------|------|----------------|---------------|
| 1 | KnowledgeSeeding | ✅ Enabled | None |
| 2 | ColumnFeatureExtraction | ✅ Enabled | None |
| 3 | EntityDiscovery | ❌ Disabled | Re-enable |
| 4 | EntityEnrichment | ❌ Disabled | Re-enable |
| 5 | FKDiscovery | ❌ Disabled | Re-enable, leverage ColumnFeatures |
| 6 | ColumnEnrichment | ❌ Disabled | Re-enable, remove redundant work |
| 7 | PKMatchDiscovery | ❌ Disabled | Re-enable, leverage ColumnFeatures |
| 8 | RelationshipEnrichment | ❌ Disabled | Re-enable |
| 9 | OntologyFinalization | ❌ Disabled | Re-enable |
| 10 | GlossaryDiscovery | ❌ Disabled | Re-enable |
| 11 | GlossaryEnrichment | ❌ Disabled | Re-enable |

### What ColumnFeatureExtraction Now Provides

Stored in `SchemaColumn.Metadata["column_features"]`:

- `SemanticType`: soft_delete, audit_created, audit_updated, foreign_key, identifier, monetary, email, url, code, status_indicator, measure, count, name, description, etc.
- `Role`: primary_key, foreign_key, attribute, measure
- `Description`: LLM-generated business description
- `Confidence`: 0.0-1.0 classification confidence
- `ClassificationPath`: timestamp, boolean, enum, uuid, external_id, numeric, text, json
- `TimestampFeatures`: is_audit_field, is_soft_delete, timestamp_purpose
- `BooleanFeatures`: true_meaning, false_meaning, is_status_indicator
- `EnumFeatures`: labeled values with state semantics (initial/in_progress/terminal/error)
- `IdentifierFeatures`: identifier_type (primary_key/foreign_key), entity_referenced, fk_target_table, fk_target_column
- `MonetaryFeatures`: is_monetary, currency_unit, paired_currency_column
- `NeedsFKResolution`, `NeedsEnumAnalysis`, `NeedsCrossColumnCheck` flags

---

## Implementation Tasks

### Task 1: Re-enable All DAG Nodes

- [x] **File:** `pkg/models/ontology_dag.go`

Uncomment all nodes in `AllDAGNodes()` function (lines ~130-145).

**Verification:** DAG should execute all 11 nodes in order.

---

### Task 2: Clean Up ColumnEnrichment Service

- [x] **File:** `pkg/services/column_enrichment.go`

ColumnEnrichment currently duplicates work that ColumnFeatureExtraction now handles. Remove or skip these patterns when ColumnFeatures exist:

#### 2a. Remove Redundant Pattern Detection

| Function | Line | What It Does | ColumnFeatures Replacement |
|----------|------|--------------|---------------------------|
| `detectBooleanNamingPattern()` | 603-640 | Regex on `is_*`, `has_*`, `can_*` | `BooleanFeatures` from Phase 2 |
| `detectExternalIDPattern()` | ~680-720 | Stripe/Twilio patterns | `IdentifierFeatures.IdentifierType = "external_id"` |
| `detectTimestampScale()` | ~750-800 | Unix timestamp detection | `TimestampFeatures` from Phase 2 |
| `detectFKColumnPattern()` | 927-975 | `*_id` suffix detection | `IdentifierFeatures.IdentifierType = "foreign_key"` |
| `detectRoleFromColumnName()` | 852-873 | Column name → role | `Role` from Phase 2 |

#### 2b. Update Merge Logic

The existing merge (lines 1520-1533) already prefers ColumnFeatures:

```go
if features := col.GetColumnFeatures(); features != nil {
    if features.Description != "" && detail.Description == "" {
        detail.Description = features.Description
    }
    if features.SemanticType != "" {
        detail.SemanticType = features.SemanticType
    }
    if features.Role != "" {
        detail.Role = features.Role
    }
}
```

**Extend this to:**
- Skip LLM call entirely if ColumnFeatures has high confidence (>0.9)
- Copy `EnumFeatures.LabeledValues` to `ColumnDetail.EnumValues`
- Copy `IdentifierFeatures.EntityReferenced` to `ColumnDetail.FKAssociation`
- Copy `TimestampFeatures` semantics (soft_delete, audit_created, etc.)

#### 2c. Reduce LLM Calls ✅

For columns where ColumnFeatureExtraction already provides complete metadata:
- Skip the per-column LLM enrichment call
- Only call LLM for columns with low confidence or missing descriptions

**Estimated reduction:** 60-80% fewer LLM calls in ColumnEnrichment.

---

### Task 3: Update FKDiscovery to Leverage ColumnFeatures

- [x] **File:** `pkg/services/deterministic_relationship_service.go`

ColumnFeatureExtraction Phase 4 already performs FK resolution via data overlap analysis. FKDiscovery should:

1. **Check existing ColumnFeatures first:**
   ```go
   if features := col.GetColumnFeatures(); features != nil {
       if features.IdentifierFeatures != nil && features.IdentifierFeatures.FKTargetTable != "" {
           // Already resolved in Phase 4, create relationship directly
       }
   }
   ```

2. **Skip redundant SQL queries** for columns already resolved

3. **Focus on schema-level FK constraints** (PostgreSQL foreign keys) which are complementary to data-driven detection

---

### Task 4: Update PKMatchDiscovery to Leverage ColumnFeatures

**File:** `pkg/services/dag/pk_match_discovery_node.go`

PKMatchDiscovery detects FKs via data overlap (join analysis). ColumnFeatureExtraction Phase 4 does similar work.

1. **Check ColumnFeatures.IdentifierFeatures.FKConfidence**
   - If already high confidence (>0.8), skip redundant SQL analysis

2. **Use ColumnFeatures to prioritize candidates:**
   - Columns with `Role = "foreign_key"` are prime candidates
   - Columns with `IdentifierFeatures.EntityReferenced` suggest target entity

3. **Avoid duplicate relationship creation:**
   - Check if relationship already exists from FKDiscovery or Phase 4

---

### Task 5: Update RelationshipEnrichment to Use ColumnFeatures

**File:** `pkg/services/dag/relationship_enrichment_node.go`

RelationshipEnrichment generates LLM descriptions for relationships. It can leverage:

1. **Source column role context:**
   ```go
   if features := sourceCol.GetColumnFeatures(); features != nil {
       if features.IdentifierFeatures != nil {
           roleContext = features.IdentifierFeatures.EntityReferenced // "host", "visitor", "payer"
       }
   }
   ```

2. **Include column description in LLM prompt** for better relationship descriptions

---

### Task 6: Update OntologyFinalization

**File:** `pkg/services/dag/ontology_finalization_node.go`

OntologyFinalization aggregates the final ontology. Ensure it:

1. **Reads ColumnFeatures from SchemaColumn.Metadata**
2. **Includes feature-derived insights in domain_summary:**
   - "Soft-delete pattern detected on X tables"
   - "Monetary columns paired with currency codes"
   - "External ID integrations: Stripe, Twilio"

---

### Task 7: Verify End-to-End Flow

After all tasks complete, run full extraction and verify:

1. **DAG completes all 11 nodes** without errors
2. **ColumnFeatures populated** for all columns
3. **ColumnEnrichment** produces ColumnDetails without redundant LLM calls
4. **Relationships** created from both schema FKs and data-driven detection
5. **Ontology** contains complete entity/relationship/column information
6. **Glossary** terms discovered and enriched

---

## Testing Strategy

### Unit Tests

- Update `column_enrichment_test.go` to verify merge behavior
- Add tests for "skip LLM when ColumnFeatures complete" logic
- Test FKDiscovery/PKMatchDiscovery deduplication

### Integration Tests

- Run full DAG extraction on test dataset
- Verify column counts match expected
- Verify no duplicate relationships created
- Verify LLM call count reduced vs. baseline

### Manual Verification

- Run extraction on Tikr dataset
- Compare ontology quality before/after
- Verify MCP tools return complete data

---

## Success Criteria

1. Full DAG runs without errors (all 11 nodes complete)
2. ColumnFeatures present on 100% of columns
3. LLM calls in ColumnEnrichment reduced by 60%+
4. No duplicate FK/relationship detection
5. Ontology ready for production MCP queries

---

## File References

| File | Changes Needed |
|------|----------------|
| `pkg/models/ontology_dag.go` | Uncomment all nodes in `AllDAGNodes()` |
| `pkg/services/column_enrichment.go` | Remove redundant patterns, enhance merge |
| `pkg/services/dag/fk_discovery_node.go` | Check ColumnFeatures before SQL |
| `pkg/services/dag/pk_match_discovery_node.go` | Use ColumnFeatures for prioritization |
| `pkg/services/dag/relationship_enrichment_node.go` | Include column role context |
| `pkg/services/dag/ontology_finalization_node.go` | Aggregate ColumnFeatures insights |

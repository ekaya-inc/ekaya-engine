# PLAN: Revive Ignored Test Code

**Status:** IN PROGRESS - Reviewing files
**Branch:** TBD
**Context:** 16 test files (~30,856 lines) are excluded via `//go:build ignore`. Each file needs classification as dead code, duplicated, or missing coverage.

## Files to Review

### 1. `pkg/services/incremental_dag_service_test.go` (439 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `incremental_dag_service.go`. No active tests cover this service's logic (only mocked in `change_review_service_test.go` for the other side).

All 8 tests are missing coverage:
- `TestToTitleCase` — `toTitleCase` still exists (line 328), zero coverage
- `TestProcessChange_SkipsWithoutAIConfig` — guard clause, no coverage
- `TestProcessChange_SkipsWithAIConfigNone` — guard clause, no coverage
- `TestProcessChange_SkipsUnknownChangeType` — guard clause, no coverage
- `TestProcessChanges_GroupsByType` — batch nil/empty handling, no coverage
- `TestProcessEnumUpdate_MergesValues` — enum merge logic, no coverage
- `TestProcessEnumUpdate_RespectsExistingValues` — deduplication, no coverage
- `TestProcessEnumUpdate_SkipsDueToPrecedence` — precedence blocking, no coverage

**Fix effort:** Low. Struct fields (`columnMetadataRepo`, `changeReviewSvc`, `aiConfigSvc`) still exist. Mock interfaces may need method signature updates but test logic is structurally sound.

### 2. `pkg/services/ontology_finalization_test.go` (947 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `ontology_finalization.go`. `Finalize()` method exists (line 56). The only other test file referencing this service is `ontology_dag_service_test.go`, which only checks interface satisfaction — it doesn't test any finalization logic.

Two categories of breakage:

**Constructor signature change (affects all 12 tests):**
Tests call `NewOntologyFinalizationService` with 6 args. Current constructor takes 7 args: added `columnMetadataRepo` (param 3), `conversationRepo` (param 4), and `getTenantCtx` (param 6). Fix is mechanical — update all constructor calls.

**`SchemaColumn.Metadata` field removed (affects 3 tests):**
Tests at lines 730-947 set `SchemaColumn.Metadata` with nested `column_features` maps. This field no longer exists on `SchemaColumn`. Column features now live in `ColumnMetadata` (separate table, looked up by `ColumnMetadata.GetBySchemaColumnIDs`). These 3 tests need restructuring to mock `ColumnMetadataRepository` instead of setting inline metadata.

| Test | Status | Notes |
|------|--------|-------|
| `TestOntologyFinalization_GeneratesDomainDescription` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_SkipsIfNoTables` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_SkipsIfNoActiveOntology` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_LLMFailure` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_DiscoversSoftDelete_Timestamp` | REVIVE | Constructor fix only (pattern-based, no Metadata use) |
| `TestOntologyFinalization_DiscoversSoftDelete_Boolean` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_DiscoversSoftDelete_Coverage` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_DiscoversCurrency_Cents` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_DiscoversCurrency_Dollars` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_DiscoversAuditColumns_WithCoverage` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_NoConventions` | REVIVE | Constructor fix only |
| `TestOntologyFinalization_ExtractsColumnFeatureInsights_SoftDelete` | REVIVE | Needs restructuring — uses removed `SchemaColumn.Metadata`; must mock `ColumnMetadataRepository` instead |
| `TestOntologyFinalization_ExtractsColumnFeatureInsights_AuditColumns` | REVIVE | Same — mock `ColumnMetadataRepository` |
| `TestOntologyFinalization_FallsBackToPatternDetection_WhenNoColumnFeatures` | REVIVE | Same — mock `ColumnMetadataRepository` |

**Fix effort:** Low-Medium. 9 tests need only constructor signature update. 3 tests need restructuring to use `ColumnMetadataRepository` mock instead of `SchemaColumn.Metadata`.

### 3. `pkg/services/table_feature_extraction_test.go` (832 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `table_feature_extraction.go`. The DAG node test (`dag/table_feature_extraction_node_test.go`) only tests the DAG node wrapper — it mocks `ExtractTableFeatures` entirely and never exercises the service internals (`buildPrompt`, `parseResponse`, `buildTableContexts`, `writeColumnSummary`). Zero unit-level coverage for these methods.

**Breakage details:**

1. **Constructor signature changed:** Tests use `NewTableFeatureExtractionService(schemaRepo, metadataRepo, llmFactory, workerPool, tenantCtx, logger)` (6 args). Current constructor takes 7: added `columnMetadataRepo` as param 2.

2. **`SchemaColumn.Metadata` field removed:** Multiple tests set `Metadata: map[string]any{"column_features": ...}` on `SchemaColumn`. This field no longer exists. Column features are now in `ColumnMetadata` (separate model, looked up by column ID via `ColumnMetadataRepository`).

3. **Method signatures changed:**
   - `buildTableContexts` now takes additional `metadataByColumnID map[uuid.UUID]*models.ColumnMetadata` param
   - `writeColumnSummary` now takes `*models.ColumnMetadata` as second param
   - `parseResponse` now takes `schemaTableID uuid.UUID` as first param
   - `tableContext` struct now has `MetadataByColumnID` field

4. **`buildTableContexts` filtering logic changed:** Old test expects tables without `column_features` in `Metadata` to be excluded. Current code filters on whether `columnsByTable` has entries for the table — no longer depends on `Metadata`.

| Test | Status | Notes |
|------|--------|-------|
| `TestTableFeatureExtraction_BuildPrompt` | REVIVE | Remove `Metadata` usage, pass `ColumnMetadata` to `tableContext.MetadataByColumnID` |
| `TestTableFeatureExtraction_BuildPrompt_NoRelationships` | REVIVE | Same |
| `TestTableFeatureExtraction_BuildPrompt_ColumnGrouping` | REVIVE | Same |
| `TestTableFeatureExtraction_ParseResponse` | REVIVE | Add `schemaTableID` first arg to `parseResponse` calls |
| `TestTableFeatureExtraction_BuildTableContexts` | REVIVE | Remove `Metadata` check, add `metadataByColumnID` param, update filter expectations |
| `TestTableFeatureExtraction_WriteColumnSummary` | REVIVE | Remove `Metadata` from `SchemaColumn`, pass `*ColumnMetadata` as separate arg |
| `TestTableFeatureExtraction_ExtractTableFeatures_Success` | REVIVE | Constructor + `Metadata` removal + mock `ColumnMetadataRepository` |
| `TestTableFeatureExtraction_ExtractTableFeatures_NoTables` | REVIVE | Constructor fix + add `columnMetadataRepo` param |
| `TestTableFeatureExtraction_ExtractTableFeatures_NoColumnsWithFeatures` | REVIVE | Constructor fix + add `columnMetadataRepo` param |
| `TestTableFeatureExtraction_ExtractTableFeatures_ProgressCallback` | REVIVE | Constructor + `Metadata` removal + mock `ColumnMetadataRepository` |

**Fix effort:** Medium. All 10 tests need revival. Constructor change is mechanical. Most tests need `Metadata` field replaced with `ColumnMetadata` mock/lookup. Method signature changes need propagation through all call sites.

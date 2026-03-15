# FIX: Enum Classification Missing Sample Values + State Leakage Across Extractions

**Status:** FIXED
**Completed By:** `fix: hydrate enum classification inputs`
**Severity:** Medium — causes extraction failures with strict models (e.g., Anthropic Haiku), silent hallucination with lenient models (e.g., gpt-4o-mini, Qwen3)
**File:** `pkg/services/column_feature_extraction.go`

## Problem

Two related bugs in the ColumnFeatureExtraction DAG node (node 2):

### Bug 1: Enum classifier prompt omits actual distinct values

The enum classifier's `buildPrompt()` (line ~1332) tells the LLM "Analyze these values" but only includes a count (`**Distinct values:** 5`), not the actual values. The `SampleValues` field on `ColumnDataProfile` is never populated because sample values were intentionally removed from `engine_schema_columns` to avoid persisting PII (line 336-338 comment).

The prompt at line 1341 does have a conditional block:
```go
if len(profile.SampleValues) > 0 {
    sb.WriteString("\n**Values found:**\n")
    for _, val := range profile.SampleValues {
        sb.WriteString(fmt.Sprintf("- `%s`\n", val))
    }
}
```
But `SampleValues` is always empty, so this block never executes.

**Impact by model:**
- **Haiku:** Correctly refuses to guess, returns conversational text instead of JSON → parse error → DAG failure
- **gpt-4o-mini:** Guesses from column name, returns valid JSON → silently inaccurate
- **Qwen3:** Copies the example JSON from the prompt verbatim → silently wrong

### Bug 2: First extraction routes differently than subsequent extractions

`distinct_count` in `engine_schema_columns` is populated by `UpdateColumnJoinability` in the `deterministicRelationshipService.collectColumnStats()` method, which runs during **FKDiscovery (DAG node 3)**.

ColumnFeatureExtraction (DAG node 2) runs **before** FKDiscovery. On a fresh project, `distinct_count` is NULL. The routing logic at line 491:
```go
if profile.Cardinality < 0.01 && profile.DistinctCount <= 50 && profile.DistinctCount > 0 {
    return models.ClassificationPathEnum
}
```
...fails the `DistinctCount > 0` check when `distinct_count` is NULL (defaults to 0 in the Go struct), routing the column to **Text** instead of **Enum**.

After the first extraction completes, FKDiscovery populates `distinct_count`. When the user deletes the ontology and re-extracts, `engine_schema_columns` retains the stats from the previous run. Now node 2 sees `DistinctCount = 5`, routes to Enum, and hits Bug 1.

**Observed behavior:**
- 1st extraction (Qwen3): `event_type` → Text classifier (no enum prompt sent)
- 2nd extraction (gpt-4o-mini): `event_type` → Enum classifier (missing values, model guessed)
- 3rd extraction (Haiku): `event_type` → Enum classifier (missing values, model refused → DAG failed)

## Root Cause

The column data profiling in Phase 1 (`buildColumnProfile`, line 322) doesn't query the target datasource for sample values or column stats. It relies solely on what's already stored in `engine_schema_columns`, which:
- Has no sample values (intentionally removed for PII safety)
- May or may not have `distinct_count` depending on whether a previous extraction populated it

## Implemented Fix

### Fix 1: Populate SampleValues on-demand for enum-routed columns (Bug 1)

**Location:** `runPhase2ColumnClassification()` (line ~613)

**Implemented:** Before dispatching work items to classifiers, `runPhase2ColumnClassification()` now opens a `SchemaDiscoverer` connection and fetches distinct values for columns routed to the Enum path. It sets `profile.SampleValues` in-memory only (not persisted).

The service already has `datasourceService` and `adapterFactory` as fields (lines 45-46), and Phase 4 (FK resolution) at line 2481 already uses this exact pattern:
```go
discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
```

The adapter interface already has the method needed:
```go
// pkg/adapters/datasource/interfaces.go:91
GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error)
```

**Implementation details:**
1. In `runPhase2ColumnClassification`, after building profiles but before creating work items
2. Filter profiles where `ClassificationPath == ClassificationPathEnum` and `len(SchemaEnumValues) == 0`
3. Get the datasource for the project, create a `SchemaDiscoverer`
4. For each enum-routed profile, call `discoverer.GetDistinctValues(ctx, schemaName, tableName, columnName, 50)`
5. Set `profile.SampleValues = values`
6. Close the discoverer
7. The existing `buildPrompt` conditional at line 1341 will then include the values

**Note:** This also benefits the text classifier prompt (line 1889) which has the same conditional pattern for `SampleValues`.

### Fix 2: Compute column stats in Phase 1 (Bug 2)

**Location:** `runPhase1DataCollection()` (line ~225)

**Implemented:** When `distinct_count` is NULL in `engine_schema_columns`, `runPhase1DataCollection()` now queries the target datasource for basic column stats before finalizing routing. This makes routing consistent regardless of whether a prior extraction has run.

The adapter already has `AnalyzeColumnStats()` which returns `DistinctCount`, `RowCount`, `NonNullCount`, `MinLength`, `MaxLength`. This is the same method used by `deterministicRelationshipService.collectColumnStats()`.

**Implementation details:**
1. After building profiles in Phase 1, identify columns where `DistinctCount == 0` (i.e., stats not yet populated)
2. Open a `SchemaDiscoverer`, batch-query stats per table using `AnalyzeColumnStats()`
3. Update the profiles in-memory with the results
4. Optionally persist via `UpdateColumnStats()` so future phases benefit
5. Re-route any columns whose path changes after stats are available

## Tests Added

- [x] `TestRunPhase1DataCollection_BackfillsMissingStatsAndReroutesEnums` verifies Phase 1 backfills missing stats, persists them, and reroutes enum-eligible columns consistently
- [x] `TestRunPhase2ColumnClassification_HydratesEnumSampleValues` verifies Phase 2 fetches distinct values before classification and includes them in the enum prompt

## Files Modified

- `pkg/services/column_feature_extraction.go` — added missing-stats hydration in Phase 1 and enum sample-value hydration in Phase 2
- `pkg/services/column_feature_extraction_test.go` — added focused regression coverage for both bugs

## Key References

- Enum classifier: `column_feature_extraction.go:1250-1370`
- Phase 2 dispatch: `column_feature_extraction.go:611-717`
- Phase 1 profiling: `column_feature_extraction.go:225-319`
- Routing logic: `column_feature_extraction.go:478-496` (`routeTextColumn`)
- Adapter interface: `pkg/adapters/datasource/interfaces.go:91` (`GetDistinctValues`)
- Phase 4 adapter usage pattern: `column_feature_extraction.go:2481`
- Profile builder (no sample values): `column_feature_extraction.go:322-383`

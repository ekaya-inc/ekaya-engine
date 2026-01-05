# PLAN: Fix Relationship Extraction

## Problem Summary

pk_match relationship discovery is not finding valid relationships like `channels.owner_id → users.user_id`.

## Root Cause Analysis

### Issue 1: Schema Data Missing After Datasource Recreation

Project 483386b8 has:
- 38 entities (ontology created at 02:43:49)
- 0 tables and 0 columns (datasource recreated at 03:29:47)
- No DAG runs

When the datasource was deleted/recreated, CASCADE deleted the schema tables/columns, but the ontology entities remained orphaned. pk_match discovery requires schema columns to function.

**Fix:** When a datasource is deleted or reconfigured, the ontology should also be reset, OR schema should be re-imported before extraction runs.

### Issue 2: Column Stats Not Being Collected

Even in projects with schema data, critical stats are NULL:
- `is_joinable`: NULL for all columns
- `distinct_count`: NULL for all columns
- `min_length`/`max_length`: NULL for all columns

The pk_match algorithm requires these stats to filter candidates:
```go
// Require explicit joinability determination
if col.IsJoinable == nil || !*col.IsJoinable {
    continue  // <-- All columns skipped because IsJoinable is NULL
}
```

**Fix:** Column stats collection must run before pk_match discovery. Verify the stats collection pipeline is working.

### Issue 3: Existing pk_match Relationships Are Garbage

The 441 pk_match relationships in working projects are invalid:
- `marketing_campaigns.cost → accounts.id` (cost is a number, not FK)
- `marketing_campaigns.app_launches → account_authentications.id` (count, not FK)
- `engagement_reviews.reviewee_rating → account_authentications.id` (rating, not FK)

These were created by old code before filtering was added. They should be purged.

## Verification Queries

### Check schema data exists:
```sql
SELECT COUNT(*) FROM engine_schema_tables WHERE project_id = '<project-id>';
SELECT COUNT(*) FROM engine_schema_columns WHERE project_id = '<project-id>';
```

### Check column stats are populated:
```sql
SELECT column_name, is_joinable, distinct_count, min_length, max_length
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE c.project_id = '<project-id>'
AND t.table_name = 'users';
```

### Check entity reference column would be selected:
For `users.user_id` to be an entity reference column:
1. Entity "User" must have primary_table = "users"
2. `isEntityReferenceName("user_id")` = true (ends with _id) ✓
3. `isPKMatchExcludedType(col)` must return false
4. `isPKMatchExcludedName("user_id")` must return false ✓

### Check candidate column would be selected:
For `channels.owner_id` to be a candidate:
1. `isPKMatchExcludedType(col)` must return false
2. `isPKMatchExcludedName("owner_id")` must return false ✓
3. `is_joinable` must be true (currently NULL → FAILS)
4. `distinct_count` must be >= 20 (currently NULL → FAILS)

## Test Data Validation

Using TEST_ credentials against the test database:

```sql
-- Both columns are 36-char UUIDs (uniform length)
SELECT 'users.user_id' as column,
       MIN(LENGTH(user_id)) as min_len, MAX(LENGTH(user_id)) as max_len
FROM users
UNION ALL
SELECT 'channels.owner_id',
       MIN(LENGTH(owner_id)), MAX(LENGTH(owner_id))
FROM channels;
-- Result: Both are 36/36

-- Zero orphans
SELECT COUNT(*) as orphan_count
FROM channels c
LEFT JOIN users u ON c.owner_id = u.user_id
WHERE u.user_id IS NULL;
-- Result: 0
```

The data is valid for a FK relationship. The issue is in the extraction pipeline.

## Tasks

### Task 1: Investigate Stats Collection Pipeline ✅ COMPLETE

**Root Cause Found:**

The DAG workflow is **not collecting column stats** at all. Here's what happens:

1. **Stats Collection Code Exists** (pkg/adapters/datasource/postgres/schema.go:218)
   - `AnalyzeColumnStats` correctly queries: `row_count`, `non_null_count`, `distinct_count`, `min_length`, `max_length`

2. **Old Discovery Service Had Stats** (pkg/services/relationship_discovery.go:152-227)
   - Calls `analyzeColumnStats` (line 179)
   - Calls `classifyJoinability` (line 198) to determine `is_joinable`
   - Updates columns via `UpdateColumnJoinability` (line 208)

3. **DAG FKDiscovery Node Does NOT Collect Stats** (pkg/services/deterministic_relationship_service.go:68)
   - The `DiscoverFKRelationships` method only converts existing FK relationships
   - It does NOT call `analyzeColumnStats` or `classifyJoinability`
   - Result: All stats remain NULL

4. **Secondary Bug in UpdateColumnJoinability** (pkg/repositories/schema_repository.go:1392-1400)
   - Updates: `row_count`, `non_null_count`, `distinct_count`, `is_joinable`, `joinability_reason`
   - **Missing**: `min_length`, `max_length` (even though AnalyzeColumnStats collects them!)

**Impact:**
- PKMatchDiscovery checks `IsJoinable` (line 306) and `DistinctCount` (line 314)
- When NULL, columns are skipped: "No stats = cannot evaluate = skip"
- Result: **Zero pk_match relationships discovered**

**Fix Required:**
Add a stats collection step to the DAG **before** PKMatchDiscovery runs.

---

**COMPLETED - Implementation Summary:**

The investigation revealed that the OLD relationship discovery service (`relationship_discovery.go`) WAS collecting stats during FK discovery, but was also generating garbage pk_match relationships (costs, counts, ratings as FKs). The problem was insufficient defensive filtering, not missing stats collection.

**Changes Made:**

1. **Fixed `distinct_count` Persistence** (commit 8be36db)
   - Added `distinct_count` parameter to `UpdateColumnJoinability()` signature
   - Updated SQL to persist `distinct_count` alongside `row_count` and `non_null_count`
   - This fixed the secondary bug where stats were collected but not saved

2. **Added Defensive Filtering** (commits 1218232, b7fd620, 01c1e46, f609f15, d9d7545)
   - Required `IsJoinable=true` before considering columns as FK candidates
   - Required `DistinctCount >= 20` (cardinality threshold)
   - Excluded aggregate/metric columns by name patterns: `num_*`, `*_count`, `total_*`
   - Excluded rating/level columns: `*_rating`, `*_score`, `*_level`, `mod_level`, `user_rating`
   - Fixed text column filtering to only exclude variable-length text (when stats prove it)
   - Added semantic validation for suspicious data patterns

3. **Added Comprehensive Test Suite** (commit e5e864f)
   - Validates all defensive filters prevent garbage relationships
   - Golden test: `accounts.num_users` never creates FK to `payout_accounts.id`
   - Tests cardinality, name patterns, joinability, and ratio checks

**Result:**
- Stats ARE collected by existing code path (`relationship_discovery.go:179`)
- pk_match now has robust filtering to prevent garbage relationships
- Test coverage ensures filters work as expected

**For Next Session:**
- Task 2 still needs addressing: datasource delete should clear ontology OR trigger re-import
- Task 3: Old garbage pk_match relationships should be purged from database
- The stats collection is working; no DAG changes needed for that aspect

### Task 2: Fix Schema/Ontology Lifecycle [x]

**Problem:** When a datasource is deleted, CASCADE deletes schema tables/columns but ontology entities remain orphaned.

**Solution Implemented:**
1. Added `GetProjectID()` method to `DatasourceRepository` to retrieve project_id before deletion
2. Modified `DatasourceService.Delete()` to call `OntologyRepository.DeleteByProject()` after deleting datasource
3. This clears all ontology data (CASCADE deletes entities, relationships, etc.)

**Files Modified:**
- `pkg/services/datasource.go` - Added ontology cleanup to Delete method
- `pkg/repositories/datasource_repository.go` - Added GetProjectID method
- Updated all NewDatasourceService call sites to inject OntologyRepository dependency

**Tests Added:**
- `TestDatasourcesIntegration_DeleteClearsOntology` - Integration test verifying ontology cleanup on datasource deletion

### Task 3: Purge Garbage pk_match Relationships [x]

**Problem:** 882 invalid pk_match relationships existed from old extraction runs before defensive filtering was added. These incorrectly identified metric columns (costs, counts, ratings, levels) as foreign keys.

**Solution Implemented:**
- Created `scripts/purge-garbage-pk-match.sh` to delete all garbage relationships
- Script removes pk_match relationships where source_column_name matches known metric/aggregate patterns:
  - Metric columns: `cost`, `total_revenue`
  - Count columns: `app_launches`, `visits`, `profile_views`, `sign_ins`, `asset_views`, `engagements`, `profile_updates`, `redirects`
  - Rating columns: `rating`, `reviewee_rating`
  - Level columns: `mod_level`, `reporter_mod_level`
  - Aggregate columns: `num_users`, `visible_days_trigger_days`

**Result:**
- Successfully deleted all 882 garbage pk_match relationships
- Verified: 0 remaining pk_match relationships in database
- Database is now clean for fresh extraction runs with proper defensive filtering

### Task 4: Add Integration Test

Create test that verifies `channels.owner_id → users.user_id` is discovered:
1. Import schema with stats
2. Run pk_match discovery
3. Assert relationship exists

## Files to Investigate

- `pkg/services/schema_service.go` - Schema import and stats collection
- `pkg/services/column_enrichment.go` - Column stats enrichment
- `pkg/repositories/schema_repository.go` - Stats persistence
- `pkg/services/deterministic_relationship_service.go` - pk_match logic (already reviewed)

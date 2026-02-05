# Issues: Post-DAG Extraction Problems [COMPLETED]

**Status:** All issues fixed and verified with `make check`

After running full DAG extraction with all 9 nodes enabled, several issues were observed.

## Summary

| Category | Issue | Severity | Root Cause | Status |
|----------|-------|----------|------------|--------|
| DAG | ColumnEnrichment completes instantly (0.00s) | High | tableNames=nil causes early return | **FIXED** |
| DAG | OntologyFinalization completes instantly, no domain_summary | High | uuid.Nil returns 0 tables | **FIXED** |
| DAG | GlossaryDiscovery completes instantly, 0 terms | High | uuid.Nil returns 0 tables | **FIXED** |
| DAG | GlossaryEnrichment completes instantly | Low | Expected (no terms to enrich) | N/A |
| UI | Relationships page fails to load | High | API endpoint mismatch | **FIXED** |
| UI | Ontology Questions shows empty | Low | Expected or unrelated | N/A |

---

## Issue 1: ColumnEnrichment Short-Circuits on Empty tableNames

**Status:** FIXED
**Severity:** High
**Files:**
- `pkg/services/column_enrichment.go:112-115`
- `pkg/services/dag/column_enrichment_node.go:63`

**Problem:** The DAG node passes `nil` for `tableNames` (meaning "all tables"), but the service interprets empty/nil as "nothing to do".

**Fix Applied:** Modified `column_enrichment.go` to fetch all selected tables when `tableNames` is nil:
```go
if len(tableNames) == 0 {
    tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, uuid.Nil, true)
    if err != nil {
        return nil, fmt.Errorf("fetch selected tables: %w", err)
    }
    if len(tables) == 0 {
        s.logger.Info("No selected tables to enrich", ...)
        result.DurationMs = time.Since(startTime).Milliseconds()
        return result, nil
    }
    for _, t := range tables {
        tableNames = append(tableNames, t.TableName)
    }
}
```

---

## Issue 2: OntologyFinalization/GlossaryDiscovery Use uuid.Nil for Datasource

**Status:** FIXED
**Severity:** High (blocks finalization and glossary)
**Root Cause:** Services pass `uuid.Nil` to `ListTablesByDatasource()` expecting "all datasources", but the repository did an exact match.

**Affected Files:**
- `pkg/services/ontology_finalization.go:70`
- `pkg/services/glossary_service.go:483, 775, 920`

**Fix Applied:** Modified `schema_repository.go` to treat `uuid.Nil` as "all datasources":
```go
// Build query - uuid.Nil means "all datasources"
query := `SELECT ... FROM engine_schema_tables WHERE project_id = $1 AND deleted_at IS NULL`
args := []any{projectID}

if datasourceID != uuid.Nil {
    query += " AND datasource_id = $2"
    args = append(args, datasourceID)
}
```

Also applied the same fix to:
- `GetRelationshipDetails`
- `GetEmptyTables`
- `GetOrphanTables`

---

## Issue 3: Relationships Page API Mismatch

**Status:** FIXED
**Severity:** High (page fails to load)

**Root Cause:** API endpoint mismatch between UI and backend.

**Fix Applied:** Added project-level relationships endpoint:
- Route: `GET /api/projects/{pid}/relationships`
- Handler: `GetProjectRelationships` in `pkg/handlers/schema.go`
- Uses `uuid.Nil` as datasourceID to aggregate across all datasources

---

## Database State Summary

| Metric | Value | Expected |
|--------|-------|----------|
| Schema Tables | 38 | ✅ |
| Schema Columns | 564 | ✅ |
| Column Metadata | 564 | ✅ |
| Table Metadata | 38 | ✅ |
| Relationships | 63 | ✅ |
| Project Knowledge | 10 | ✅ |
| Ontology Questions | 0 | ❓ |
| Glossary Terms | 0 | ❌ (was bug, now fixed) |
| domain_summary | NULL | ❌ (was bug, now fixed) |

---

## Node Timing Analysis (Pre-Fix)

| Node | Duration | LLM Calls | Status |
|------|----------|-----------|--------|
| KnowledgeSeeding | 0.00s | 0 | ✅ (reused existing) |
| ColumnFeatureExtraction | 125.62s | ~800 | ✅ |
| FKDiscovery | 0.58s | 0 | ✅ |
| TableFeatureExtraction | 17.12s | ~100 | ✅ |
| PKMatchDiscovery | 128.59s | ~700 | ✅ |
| ColumnEnrichment | 0.00s | 0 | ❌ Bug: nil tableNames → **FIXED** |
| OntologyFinalization | 0.00s | 0 | ❌ Bug: uuid.Nil → **FIXED** |
| GlossaryDiscovery | 0.00s | 0 | ❌ Bug: uuid.Nil → **FIXED** |
| GlossaryEnrichment | 0.00s | 0 | Expected (no terms) |

---

## Notes

- KnowledgeSeeding 0.00s is expected - it found existing knowledge facts from a previous run and just updated timestamps
- The 1603 successful LLM calls were made by ColumnFeatureExtraction, TableFeatureExtraction, and PKMatchDiscovery
- No LLM errors during this run (previous errors were from Feb 4 when localhost:30000 was down)
- All fixes verified with `make check` passing
- **Completed:** 2026-02-05

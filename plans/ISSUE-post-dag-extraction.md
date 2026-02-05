# Issues: Post-DAG Extraction Problems

After running full DAG extraction with all 9 nodes enabled, several issues were observed.

## Summary

| Category | Issue | Severity | Type |
|----------|-------|----------|------|
| DAG | ColumnEnrichment completes instantly (0.00s) | High | Bug |
| DAG | OntologyFinalization completes instantly, no domain_summary | High | Bug |
| DAG | GlossaryDiscovery completes instantly, 0 terms | High | Bug |
| DAG | GlossaryEnrichment completes instantly (no terms to enrich) | Low | Expected |
| UI | Relationships page fails to load | High | Bug |
| UI | Ontology Questions shows empty | Medium | Investigate |
| UI | Glossary Items shows empty | Medium | Expected (DAG bug) |

## DAG Node Issues

### Issue 1: ColumnEnrichment Short-Circuits on Empty tableNames

**File:** `pkg/services/column_enrichment.go:112-115`

**Problem:** The DAG node passes `nil` for `tableNames` (meaning "all tables"), but the service interprets empty/nil as "nothing to do":

```go
// column_enrichment.go:112-115
if len(tableNames) == 0 {
    result.DurationMs = time.Since(startTime).Milliseconds()
    return result, nil  // Returns immediately!
}
```

The node at `pkg/services/dag/column_enrichment_node.go:63` calls:
```go
result, err := n.columnEnrichmentSvc.EnrichProject(ctx, dag.ProjectID, nil, progressCallback)
```

**Evidence:** Node completed in 0.00 seconds with 0 LLM calls

**Fix:** Either:
1. The node should fetch table names and pass them to the service, OR
2. The service should fetch all tables when `tableNames` is nil/empty

### Issue 2: OntologyFinalization Produces No domain_summary

**File:** `pkg/services/ontology_finalization.go`

**Problem:** The finalization node completed in 0.00 seconds but the ontology's `domain_summary` is NULL.

**Evidence:**
```sql
SELECT domain_summary IS NOT NULL as has_summary FROM engine_ontologies WHERE project_id = '...';
-- Result: has_summary = false
```

**Possible Causes:**
1. The service hits an early return (e.g., line 65-67: "No active ontology found")
2. The LLM call is skipped or fails silently
3. The domain_summary is generated but not saved

**Investigation Needed:** Add logging or check if there's an early return condition being hit.

### Issue 3: GlossaryDiscovery Produces 0 Terms

**File:** `pkg/services/glossary_service.go`

**Problem:** GlossaryDiscovery completed in 0.00 seconds with 0 glossary terms created.

**Evidence:**
```sql
SELECT COUNT(*) FROM engine_business_glossary WHERE project_id = '...';
-- Result: 0
```

**Possible Causes:**
1. The LLM is called but returns no terms
2. All terms are filtered out by `filterInapplicableTerms()`
3. There's an early return condition being hit

**Investigation Needed:** Check logs for "Starting glossary term discovery" and LLM response.

## UI Issues

### Issue 4: Relationships Page Fails to Load

**Symptom:** The relationships page at `/projects/{pid}/relationships` fails to load.

**Root Cause:** API endpoint mismatch

- **UI calls:** `GET /api/projects/{projectId}/relationships`
  - File: `ui/src/services/engineApi.ts:224-231`

- **Backend has:** `GET /api/projects/{pid}/datasources/{dsid}/schema/relationships`
  - File: `pkg/handlers/schema.go:181`

The UI endpoint doesn't include the datasource ID, but the backend requires it.

**Note:** The relationships data IS in the database (63 relationships found via SQL query). This is purely a routing/API issue.

**Fix Options:**
1. Add a project-level relationships endpoint that aggregates across all datasources
2. Update the UI to require/use datasource ID

### Issue 5: Ontology Questions Empty

**Status:** Needs investigation

```sql
SELECT COUNT(*) FROM engine_ontology_questions WHERE project_id = '...';
-- Result: 0
```

The questions table is empty. This could be:
1. Expected - questions are only generated in specific scenarios
2. A bug in the question generation logic
3. Related to the other DAG node issues

## Database State (for reference)

Extraction ran on 2026-02-05 and completed in ~5 minutes:

| Metric | Value |
|--------|-------|
| Schema Tables | 38 |
| Schema Columns | 564 |
| Column Metadata | 564 (correct) |
| Table Metadata | 38 (correct) |
| Relationships | 63 (correct) |
| Project Knowledge | 10 (correct) |
| Ontology Questions | 0 |
| Glossary Terms | 0 |
| domain_summary | NULL |

## Node Timing Analysis

| Node | Duration | Status |
|------|----------|--------|
| KnowledgeSeeding | 0.00s | ⚠️ Suspicious |
| ColumnFeatureExtraction | 125.62s | ✅ OK |
| FKDiscovery | 0.58s | ✅ OK |
| TableFeatureExtraction | 17.12s | ✅ OK |
| PKMatchDiscovery | 128.59s | ✅ OK |
| ColumnEnrichment | 0.00s | ❌ Bug |
| OntologyFinalization | 0.00s | ❌ Bug |
| GlossaryDiscovery | 0.00s | ❌ Bug |
| GlossaryEnrichment | 0.00s | Expected (no terms) |

## Priority

1. **High:** Fix ColumnEnrichment tableNames handling
2. **High:** Fix OntologyFinalization to generate domain_summary
3. **High:** Fix Relationships page API endpoint
4. **Medium:** Investigate GlossaryDiscovery (may work once ColumnEnrichment works)
5. **Low:** Investigate KnowledgeSeeding 0.00s timing

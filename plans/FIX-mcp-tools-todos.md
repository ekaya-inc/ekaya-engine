# FIX: MCP Tools Outstanding TODOs

**Created:** 2025-01-08
**Status:** Documentation of deferred work
**Priority:** Low-Medium

---

## Outstanding TODOs

### 1. ~~Sample Values in get_context (context.go:504)~~ ✅ COMPLETED

**Status:** Implemented (2025-01-08)
**Solution:** Added sample_values to get_context response when `include=["sample_values"]` is specified.

```go
// Sample values are persisted during ontology extraction for low-cardinality columns (≤50 distinct values)
if include.SampleValues && schemaCol != nil && len(schemaCol.SampleValues) > 0 {
    colDetail["sample_values"] = schemaCol.SampleValues
}
```

**Files:** `pkg/mcp/tools/context.go:503-507`

---

### 2. Multi-datasource Support in probe_relationship (probe.go:250)

```go
// TODO: Support multi-datasource projects
```

**Context:** Currently assumes single datasource per project. When multi-datasource is needed:
- Add datasource_id parameter to probe_relationship tool
- Update getSchemaRelationshipsWithMetrics to filter by datasource
- Handle case where relationships span datasources

**Priority:** Low - most projects have single datasource

---

### 3. UserID Extraction in developer.go (lines 490, 596, 684, 779)

```go
// TODO: Extract userID from context and datasourceID from getDefaultDatasourceConfig when step 8 is implemented
```

**Context:** These TODOs predate this implementation. They refer to extracting user ID for audit logging purposes.

**Solution when ready:**
```go
claims, _ := auth.GetClaims(ctx)
userID := claims.UserID // if available
```

**Priority:** Low - logging enhancement, not blocking functionality

---

## Notes

- All TODOs are non-blocking for current functionality
- Multi-datasource support should be planned holistically across all tools
- Sample values TODO is the quickest to implement (see item 1)

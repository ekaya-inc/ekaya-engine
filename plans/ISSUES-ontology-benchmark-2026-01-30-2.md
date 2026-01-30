# ISSUES: Ontology Benchmark - Additional Findings - 2026-01-30

Issues discovered during continued MCP API usage after initial testing.

---

## Feature Requests

### 7. Missing `update_table` Tool for Table-Level Metadata

**Severity**: MEDIUM (Feature Request)
**Status**: Open

**Description**: There is no tool to update table-level metadata. Currently available update tools are:
- `update_column` - column metadata
- `update_entity` - entity metadata
- `update_relationship` - relationships
- `update_project_knowledge` - general facts
- `update_glossary_term` - business terms

**Use Case**: Need to mark tables as "ephemeral" or "not for analytics". For example, the `sessions` table is transient and should not be used for analytics queries - `billing_engagements` should be used instead.

**Current Workaround**: Using `update_project_knowledge` to capture table-level guidance, but this is less discoverable than table metadata would be.

**Recommendation**: Add `update_table` tool with fields like:
- `description` - what the table represents
- `usage_notes` - when to use/not use this table
- `is_ephemeral` - boolean flag for transient tables
- `preferred_alternative` - table to use instead if ephemeral

---

### 8. `update_project_knowledge` Fact Field Has 255 Character Limit

**Severity**: LOW (UX)
**Status**: Open

**Description**: The `fact` parameter in `update_project_knowledge` is limited to 255 characters, which is too short for complex business rules.

**Reproduction**:
```
update_project_knowledge(
  fact="billing_engagements tracks ALL engagements (including free ones with no charges). billing_transactions tracks only engagements where money was actually charged. An engagement can exist without billing if: (1) it's a free engagement, (2) it ended before the first tik (15 seconds), or (3) the host set a $0 fee_per_minute.",
  category="business_rule"
)
```

**Observed error**:
```
ERROR: value too long for type character varying(255) (SQLSTATE 22001)
```

**Recommendation**:
- Increase `fact` column to TEXT or VARCHAR(1000)
- Or provide a clearer error message indicating the character limit

---

### 9. `create_approved_query` Requires Undiscoverable datasource_id

**Severity**: MEDIUM (UX)
**Status**: Open

**Description**: The `create_approved_query` tool requires a `datasource_id` parameter, but there's no obvious way to discover this UUID. The `health` endpoint returns `project_id` but not `datasource_id`.

**Reproduction**:
```
create_approved_query(
  name="Test Query",
  description="Test",
  sql="SELECT 1",
  datasource_id="???"  -- Where do I get this?
)
```

**Workaround**: Use `suggest_approved_query` instead, which auto-detects the datasource and doesn't require the parameter. Then approve the suggestion.

**Recommendation**: Either:
1. Add `datasource_id` to the `health` endpoint response
2. Make `datasource_id` optional in `create_approved_query` (auto-detect like `suggest_approved_query` does)
3. Add a `list_datasources` tool

### 10. `get_context` Does Not Surface Enriched Column Metadata

**Severity**: HIGH
**Status**: Open

**Description**: Column descriptions added via `update_column` are stored but not included in `get_context` output at the columns depth level. This defeats the purpose of metadata enrichment.

**Reproduction**:
```
# Add enriched metadata
update_column(table='billing_engagements', column='host_id',
  description='Use this to find all hosts who had engagements')

# Check get_context - description is missing
get_context(depth='columns', tables=['billing_engagements'])
# Returns columns with data types but NO descriptions

# Only probe_column shows the metadata
probe_column(table='billing_engagements', column='host_id')
# Returns: {"description": "Use this to find all hosts..."}
```

**Impact**: A fresh agent using the recommended progressive disclosure workflow (`get_context` at increasing depths) will never see enriched column metadata. They'd have to probe each column individually.

**Recommendation**: Include column descriptions, entity, and role in `get_context` columns output when available.

---

### 11. `get_context` Does Not Include Project Knowledge

**Severity**: HIGH
**Status**: Open

**Description**: Project knowledge (business rules added via `update_project_knowledge`) is not surfaced in `get_context`. Critical guidance like "sessions table is ephemeral" or "use billing_engagements not billing_transactions for engagement counts" is invisible to agents using the primary discovery workflow.

**Current depth levels**:
```
domain → entities → tables → columns
```

**Recommendation**: Add project knowledge to `get_context`. Options:
1. New depth level: `domain → project → entities → tables → columns`
2. Include at domain level as a `project_knowledge` section
3. Add as an `include` option: `include: ['project_knowledge']`

Option 2 (include at domain level) seems cleanest since project knowledge is high-level guidance about how to use the database.

---

## Summary

| Issue # | Type | Severity | Description |
|---------|------|----------|-------------|
| 7 | Feature Request | MEDIUM | Missing `update_table` tool for table-level metadata |
| 8 | UX | LOW | `update_project_knowledge` fact limited to 255 chars |
| 9 | UX | MEDIUM | `create_approved_query` requires undiscoverable datasource_id |
| 10 | Bug | HIGH | `get_context` doesn't surface enriched column metadata |
| 11 | Feature Gap | HIGH | `get_context` doesn't include project knowledge |

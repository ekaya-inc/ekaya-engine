# BUGS: Ontology Extraction Testing

Discovered during ad-hoc testing of ontology extraction scenarios from TEST-ontology-mix.md.

## Summary

| Bug | Description | Severity | Status |
|-----|-------------|----------|--------|
| 1 | `refresh_schema` auto_select doesn't update UI | Medium | ✅ FIXED |
| 2 | Entity name singularization incorrect | Low | ✅ FIXED |
| 3 | `probe_relationship` empty for MCP-created | Medium | ✅ FIXED |
| 4 | Approved column metadata not in `probe_column` | Medium | ✅ FIXED |
| 5 | Entity Discovery deletes manual entities | High | ✅ FIXED |
| 6 | `get_entity` not found for extraction entities | High | ✅ FIXED |
| 7 | Self-referential FK not discovered | High | ✅ FIXED |
| 8 | Junction table FK not discovered | Medium | ✅ FIXED |

**Progress: 8/8 fixed**

## Bug 1: MCP `refresh_schema` auto_select Does Not Update UI State

**Severity:** Medium
**Component:** MCP Server / Schema Selection
**Status:** ✅ FIXED (see FIX-bug1-refresh-schema-auto-select.md)

**Description:**
When calling `refresh_schema` with `auto_select: true`, the MCP server reports that tables were auto-selected, but the UI Schema Selection page shows them as unchecked.

**Root Cause:** Multiple issues - response didn't reflect actual success, new tables created with IsSelected=false before SelectAllTables ran.

**Fix Applied:**
- [x] Response now reflects actual success of SelectAllTables
- [x] IsSelected set during table creation when auto_select=true
- [x] Added explicit project_id filter to update query

---

## Bug 2: Entity Name Suggestions Have Incorrect Singularization

**Severity:** Low
**Component:** Schema Refresh / Entity Naming
**Status:** ✅ FIXED (see FIX-bug2-singularization-errors.md)

**Description:**
When `refresh_schema` suggests entity names from table names, the singularization logic produces incorrect results for some table names ending in "ies" or "es".

**Examples from pending changes:**
- `s4_categories` → suggested "S4_categorie" (should be "S4_category" or "Category")
- `s5_activities` → suggested "S5_activitie" (should be "S5_activity" or "Activity")

**Root Cause:** `toEntityName()` uses `strings.TrimSuffix(name, "s")` which only removes trailing "s".

**Recommended Fix:** Use `github.com/jinzhu/inflection` library for proper English singularization.

---

## Bug 3: probe_relationship Returns Empty for MCP-Created Relationships

**Severity:** Medium
**Component:** MCP Server / probe_relationship
**Status:** ✅ FIXED (see FIX-bug3-probe-relationship-mcp-created.md)

**Description:**
Relationships created via `update_relationship` MCP tool don't appear in `probe_relationship` results.

**Root Cause:** Entity lookup inconsistency - `probe_relationship` used `GetByOntology(ontology.ID)` requiring exact ontology ID match, while `get_ontology` uses `GetByProject` with JOIN to ANY active ontology.

**Fix Applied:**
- [x] Changed entity lookup in probe.go:533 from `GetByOntology(ctx, ontology.ID)` to `GetByProject(ctx, projectID)`
- Added comment explaining pattern consistency with get_ontology

---

## Bug 4: Approved Column Metadata Changes Not Reflected in probe_column

**Severity:** Medium
**Component:** MCP Server / Column Metadata
**Status:** ✅ FIXED (commit 59d7425, see FIX-bug4-approved-changes-probe-column.md)

**Description:**
After approving a pending change to add enum values to a column, `probe_column` does not return the enum information.

**Root Cause:** Two separate storage locations - `approve_change` wrote to `engine_column_metadata` table, but `probe_column` only read from `ontology.column_details` JSONB.

**Fix Applied:**
- [x] Updated probe_column to read from both data sources
- [x] Added integration test for full approve_change → probe_column flow

---

## Bug 5: Entity Discovery Deletes Manually Created Entities

**Severity:** High
**Component:** Ontology Extraction / Entity Management
**Status:** ✅ FIXED (see FIX-bug5-entity-discovery-deletes-manual.md)

**Description:**
When ontology extraction starts, the Entity Discovery step unconditionally deletes all existing entities, including those created manually via MCP tools.

**Root Cause:** `StartExtraction` called `DeleteByOntology` which was a hard delete of ALL entities regardless of `created_by` provenance.

**Fix Applied:**
- [x] Added `DeleteInferenceEntitiesByOntology` to repository (only deletes `created_by = 'inference'`)
- [x] Updated `StartExtraction` to use provenance-aware deletion
- Manual/MCP entities are now preserved during extraction

---

## Bug 6: get_entity Returns "Not Found" for Extraction-Created Entities

**Severity:** High
**Component:** MCP Server / Entity Queries
**Status:** ✅ FIXED (see FIX-bug6-get-entity-not-found.md)

**Description:**
Entities shown in `get_context` with depth=entities are not found by `get_entity`.

**Root Cause:** `get_entity` used `GetByName(ontology.ID, name)` requiring exact ontology ID match, while `get_context` used `GetByProject` with JOIN to ANY active ontology.

**Fix Applied:**
- [x] Added `GetByProjectAndName(ctx, projectID, name)` repository method
- [x] Updated `get_entity`, `update_entity`, `delete_entity` to use new method
- [x] Manually verified on 2026-01-21 with 50+ extraction-created entities

---

## Bug 7: Self-Referential FK Relationships Not Discovered

**Severity:** High
**Component:** FK Discovery
**Status:** ✅ FIXED (see FIX-bug7-self-referential-fk.md)

**Description:**
Tables with self-referential foreign keys (e.g., employee.manager_id → employee.id) are not detected by FK Discovery.

**Root Cause:** FK Discovery code explicitly skipped self-referential relationships with `if sourceEntity.ID == targetEntity.ID { continue }`.

**Fix Applied:**
- [x] Removed self-reference skip in FK Discovery (lines 195-198)
- [x] PK-match skip kept (intentional - avoids false positives)
- [x] Role labels handled by Column Enrichment via LLM (verified)

---

## Bug 8: Junction Table FK Relationships Not Discovered

**Severity:** Medium
**Component:** FK Discovery / Schema Discovery
**Status:** ✅ FIXED (see FIX-bug8-junction-table-fk.md)

**Description:**
Junction tables with composite primary keys have their FK relationships ignored.

**Root Cause:** Schema discovery at `pkg/adapters/datasource/postgres/schema.go` explicitly excluded composite PKs with `array_length(ix.indkey, 1) = 1`. This caused:
1. Composite PK columns marked as `IsPrimaryKey = false`
2. Entity Discovery finds no PK candidates → No entity created for junction table
3. FK Discovery skips relationships when source entity doesn't exist

**Fix Applied:**
- [x] Removed `array_length(ix.indkey, 1) = 1` filter from PK detection query
- [x] Removed same filter from unique constraint detection query
- All columns in composite PKs/unique constraints now properly detected

---

## Testing Status (Post Entity Enrichment)

### Scenario 1: Classic FK Relationships ✓ PASS
- **Tables:** s1_customers, s1_orders, s1_order_items
- **FK Detection:** ✓ Detected by `refresh_schema`
- **Ontology Extraction:**
  - ✓ "S1 Customer" entity with description "A customer record representing an end user..."
  - ✓ "Order" entity with customer_id → s1_customers occurrence
  - ✓ "Order Item" entity with order_id → s1_orders occurrence
  - ✓ Semantic names (not table names)
  - ✓ Key columns with synonyms

### Scenario 2: UUID PKs Without FK Constraints ⚠️ PARTIAL
- **Tables:** s2_users, s2_posts, s2_comments
- **FK Detection:** N/A (no FK constraints by design)
- **Ontology Extraction:**
  - ✓ "S2 User" entity created
  - ✓ "Post" entity with author_id as key column
  - ✓ "Comment" entity with post_id as key column
  - ✗ **NO UUID relationship inference** - Post.author_id doesn't show occurrence to S2 User
  - Waiting for PK-Match Discovery step to test this

### Scenario 3: Composite Primary Keys ⚠️ PARTIAL
- **Tables:** s3_students, s3_courses, s3_enrollments
- **FK Detection:** ✓ Detected by `refresh_schema`
- **Ontology Extraction:**
  - ✓ "Student" entity created
  - ✓ "Course" entity created
  - ✗ **NO Enrollment entity** - junction table skipped entirely
  - This may be intentional (junction tables aren't domain entities)

### Scenario 4: Self-Referential Relationships ⚠️ PARTIAL
- **Tables:** s4_employees, s4_categories
- **FK Detection:** ✓ Detected by `refresh_schema` (manager_id, parent_category_id)
- **Ontology Extraction:**
  - ✓ "Employee" entity with manager_id key column
  - ✓ "Category" entity with parent_category_id key column
  - ✗ **No self-referential occurrence** - Employee doesn't show occurrence to itself
  - Needs FK Discovery / Relationship Enrichment to complete

### Scenario 5: Polymorphic Associations ⚠️ PARTIAL
- **Tables:** s5_users, s5_organizations, s5_documents, s5_activities
- **FK Detection:** N/A (polymorphic pattern)
- **Ontology Extraction:**
  - ✓ "Activity" entity created
  - ✓ "Document" entity with owner_id key column
  - ✓ "Organization" entity created
  - ✗ **s5_users has no description** (conflict with main Users table?)
  - ✗ **No ontology questions generated** for polymorphic ambiguity

### Scenario 6: Soft Deletes and Audit Columns ⚠️ PENDING
- **Tables:** s6_products, s6_inventory
- **FK Detection:** ✓ product_id FK detected
- **Ontology Extraction:**
  - ✓ "Product" entity created
  - ✓ "Inventory Item" entity with product_id occurrence
  - Need to verify audit column recognition after Column Enrichment

### Scenario 7: Enum-Like Status Columns ⚠️ PENDING
- **Tables:** s7_tickets
- **FK Detection:** N/A
- **Ontology Extraction:** Pending - expecting enum detection for status, priority, ticket_type
- **Note:** Manually added enum metadata via MCP `update_column` - testing if extraction merges or overwrites

### Scenario 8: Multiple Relationships to Same Entity
- **Tables:** s8_people, s8_contracts, s8_messages
- **FK Detection:** ✓ All FKs detected (buyer_id, seller_id, witness_id, sender_id, recipient_id → s8_people)
- **Ontology Extraction:** Pending - critical test for role semantics

### Scenario 9: Natural Keys vs Surrogate Keys
- **Tables:** s9_countries, s9_currencies, s9_exchange_rates, s9_addresses
- **FK Detection:** ✓ CHAR FK relationships detected (country_code, currency_code)
- **Ontology Extraction:** Pending - testing non-integer PK handling

### Scenario 10: JSONB Columns
- **Tables:** s10_events, s10_user_preferences
- **FK Detection:** N/A
- **Ontology Extraction:** Pending - should recognize JSONB columns without deep analysis

---

## MCP Ontology Tools Testing

### update_entity
- **Status:** ✓ Working
- **Notes:** Successfully created S1Customer and S1Order entities with descriptions, aliases, key_columns

### get_entity
- **Status:** ✓ Working
- **Notes:** Returns entity with all metadata including occurrences from relationships

### update_relationship
- **Status:** ✓ Working
- **Notes:** Successfully created S1Order → S1Customer relationship with cardinality

### update_column
- **Status:** ✓ Working
- **Notes:** Successfully added enum_values to s7_tickets.status

### probe_column
- **Status:** ✓ Working
- **Notes:** Returns semantic info including manually-added enum_labels

### get_ontology
- **Status:** ✓ Working
- **Notes:** Returns manually created entities and relationships

---

## Questions for Further Investigation

1. **Extraction vs Manual Ontology:** Does extraction overwrite manually-created entities/relationships or merge?
2. **Provenance Tracking:** Are changes tracked by source (extraction vs MCP vs admin)?
3. **UUID Relationship Inference:** How well does PK-match detection work for s2_* tables?
4. **Polymorphic Pattern Recognition:** Will s5_documents owner_type/owner_id generate clarifying questions?
5. **Role Semantics:** Will s8_contracts correctly distinguish buyer/seller/witness roles?

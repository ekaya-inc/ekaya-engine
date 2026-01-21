# BUGS: Ontology Extraction Testing

Discovered during ad-hoc testing of ontology extraction scenarios from TEST-ontology-mix.md.

## Bug 1: MCP `refresh_schema` auto_select Does Not Update UI State

**Severity:** Medium
**Component:** MCP Server / Schema Selection

**Description:**
When calling `refresh_schema` with `auto_select: true`, the MCP server reports that tables were auto-selected, but the UI Schema Selection page shows them as unchecked.

**Steps to Reproduce:**
1. Create new tables in the datasource database
2. Call MCP `refresh_schema` with `auto_select: true`
3. Response shows `"auto_select_applied": true` and lists tables in `tables_added`
4. Navigate to UI Schema Selection page
5. Newly added tables show as unchecked

**Expected Behavior:**
Tables should be checked/selected in UI after `auto_select: true`

**Actual Behavior:**
Tables appear unchecked in UI despite MCP reporting auto_select was applied

**Workaround:**
Manually select tables in the UI

---

## Bug 2: Entity Name Suggestions Have Incorrect Singularization

**Severity:** Low
**Component:** Schema Refresh / Entity Naming

**Description:**
When `refresh_schema` suggests entity names from table names, the singularization logic produces incorrect results for some table names ending in "ies" or "es".

**Examples from pending changes:**
- `s4_categories` → suggested "S4_categorie" (should be "S4_category" or "Category")
- `s5_activities` → suggested "S5_activitie" (should be "S5_activity" or "Activity")

**Expected Behavior:**
Entity names should be correctly singularized (categories → category, activities → activity)

**Actual Behavior:**
Simple suffix stripping produces grammatically incorrect names

**Suggestion:**
Use a proper inflection library (e.g., Go's `inflect` package) for singularization

---

## Bug 3: probe_relationship Returns Empty for MCP-Created Relationships

**Severity:** Medium
**Component:** MCP Server / probe_relationship

**Description:**
Relationships created via `update_relationship` MCP tool don't appear in `probe_relationship` results.

**Steps to Reproduce:**
1. Create relationship via `update_relationship(from_entity='S1Order', to_entity='S1Customer', ...)`
2. Verify relationship exists via `get_ontology` (it shows up)
3. Call `probe_relationship(from_entity='S1Order', to_entity='S1Customer')`
4. Returns empty `{"relationships":[]}`

**Expected Behavior:**
`probe_relationship` should return the manually created relationship

**Possible Cause:**
`probe_relationship` may only query FK-based relationships with computed metrics, not manually created ontology relationships

---

## Bug 4: Approved Column Metadata Changes Not Reflected in probe_column

**Severity:** Medium
**Component:** MCP Server / Column Metadata

**Description:**
After approving a pending change to add enum values to a column, `probe_column` does not return the enum information.

**Steps to Reproduce:**
1. Call `scan_data_changes` which detects enum values in s7_tickets.ticket_type
2. Approve the change via `approve_change`
3. Response shows "Change approved and applied successfully"
4. Call `probe_column(table='s7_tickets', column='ticket_type')`
5. Returns minimal info without enum_labels

**Expected Behavior:**
`probe_column` should return the approved enum values

**Actual Behavior:**
Returns `{"table":"s7_tickets","column":"ticket_type"}` with no enum info

---

## Bug 5: Entity Discovery Deletes Manually Created Entities

**Severity:** High
**Component:** Ontology Extraction / Entity Management

**Description:**
When ontology extraction starts, the Entity Discovery step appears to delete or overwrite manually created entities.

**Steps to Reproduce:**
1. Create entity via `update_entity(name='S1Customer', description='...')`
2. Verify entity exists via `get_entity(name='S1Customer')` - works
3. Start ontology extraction via UI
4. After Entity Discovery completes, call `get_entity(name='S1Customer')`
5. Returns "entity not found"

**Expected Behavior:**
Manually created entities should be preserved or merged with extracted entities

**Actual Behavior:**
Manually created entities are deleted during extraction

**Impact:**
Users lose their manual ontology work when running extraction

---

## Bug 6: get_entity Returns "Not Found" for Extraction-Created Entities

**Severity:** High
**Component:** MCP Server / Entity Queries

**Description:**
Entities shown in `get_context` with depth=entities are not found by `get_entity`.

**Steps to Reproduce:**
1. Start ontology extraction
2. After Entity Discovery completes, call `get_context(depth='entities')`
3. Response shows entities like `"s1_customers": {"primary_table": "s1_customers", ...}`
4. Call `get_entity(name='s1_customers')`
5. Returns "entity not found"

**Expected Behavior:**
`get_entity` should find entities that appear in `get_context`

**Actual Behavior:**
`get_entity` returns "not found" for extraction-created entities

**Possible Cause:**
`get_context` and `get_entity` may query different data sources or tables

---

## Bug 7: Self-Referential FK Relationships Not Discovered

**Severity:** High
**Component:** FK Discovery

**Description:**
Tables with self-referential foreign keys (e.g., employee.manager_id → employee.id) are not detected by FK Discovery.

**Evidence:**
- s4_employees has FK: `manager_id INTEGER REFERENCES s4_employees(id)`
- s4_categories has FK: `parent_category_id INTEGER REFERENCES s4_categories(id)`
- `probe_relationship` returns no Employee→Employee or Category→Category relationships

**Expected Behavior:**
Self-referential FKs should be detected and added to relationships with appropriate role labels (e.g., "manager", "parent")

**Actual Behavior:**
Self-referential relationships are completely missing from probe_relationship results

**Impact:**
Hierarchical data models (org charts, category trees) won't have proper relationship modeling

---

## Bug 8: Junction Table FK Relationships Not Discovered

**Severity:** Medium
**Component:** FK Discovery

**Description:**
Junction tables with composite primary keys have their FK relationships ignored.

**Evidence:**
- s3_enrollments has FKs: `student_id → s3_students`, `course_code → s3_courses`
- `probe_relationship` returns no Student→Enrollment or Course→Enrollment relationships

**Expected Behavior:**
FK relationships from junction tables should be discovered normally

**Actual Behavior:**
Junction table FK relationships are missing

**Note:**
This may be related to Bug where junction tables don't create entities - if no entity exists, no relationship can be created.

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

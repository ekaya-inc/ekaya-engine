# FIX: Bug 7 - Self-Referential FK Relationships Not Discovered

**Priority:** High
**Component:** FK Discovery

## Problem Statement

Tables with self-referential foreign keys (e.g., `employee.manager_id → employee.id`) are not detected by FK Discovery.

**Evidence:**
- `s4_employees` has FK: `manager_id INTEGER REFERENCES s4_employees(id)`
- `s4_categories` has FK: `parent_category_id INTEGER REFERENCES s4_categories(id)`
- `probe_relationship` returns no Employee→Employee or Category→Category relationships

## Root Cause Analysis

The FK Discovery code **explicitly skips self-referential relationships** in two locations:

### Location 1: FK Discovery (pkg/services/deterministic_relationship_service.go:195-198)

```go
// Don't create self-referencing relationships
if sourceEntity.ID == targetEntity.ID {
    continue
}
```

This is in `DiscoverFKRelationships`, inside the loop that processes schema relationships from actual foreign key constraints.

### Location 2: PK-match Discovery - Table Check (pkg/services/deterministic_relationship_service.go:592-595)

```go
// Skip if same table (self-reference)
if candidate.schema == ref.schema && candidate.table == ref.table {
    continue
}
```

### Location 3: PK-match Discovery - Entity Check (pkg/services/deterministic_relationship_service.go:608-611)

```go
// Don't create self-referencing entity relationships
if sourceEntity.ID == ref.entity.ID {
    continue
}
```

### Why This Was Done

The skip was likely added to:
1. Avoid infinite loops or recursive processing issues
2. Simplify the relationship graph (self-references can be complex to display)
3. Prevent false positives in PK-match discovery

However, self-referential FKs are **valid and common patterns**:
- Hierarchies: `employee.manager_id → employee.id`
- Trees: `category.parent_id → category.id`
- Graphs: `document.related_to → document.id`

## Recommended Fix

### Step 1: Allow Self-References in FK Discovery (Required)

Self-referential FKs from actual database constraints are intentional and should be discovered:

**pkg/services/deterministic_relationship_service.go** (around line 195-198):

```go
// BEFORE:
// Don't create self-referencing relationships
if sourceEntity.ID == targetEntity.ID {
    continue
}

// AFTER:
// Allow self-referencing relationships - they represent hierarchies/trees
// (e.g., employee.manager_id → employee.id)
// Self-references from FK constraints are intentional
```

Simply remove lines 195-198 to allow FK-based self-references.

### Step 2: Keep PK-match Skip (Recommended)

The PK-match discovery skip (lines 592-595, 608-611) should **remain** because:
- Same-table PK matches are almost always false positives without explicit FK constraints
- A column matching the same table's PK is likely just a copy or different ID
- The FK Discovery handles legitimate self-references via actual constraints

### Step 3: Handle Role Labels for Self-References

Self-referential relationships need role labels to distinguish directions:

For `employee.manager_id → employee.id`:
- Forward: "manages" or "has_subordinates"
- Reverse: "reports_to" or "managed_by"

This is handled by Column Enrichment which assigns roles via LLM. Verify it works for self-references by checking the `association` field is populated.

## Files to Modify

1. **pkg/services/deterministic_relationship_service.go:195-198**
   - Remove or comment out the self-reference skip for FK Discovery
   - Add a comment explaining why self-references are allowed here

## Testing Verification

After implementing:

1. Create table with self-referential FK:
   ```sql
   CREATE TABLE test_employees (
       id SERIAL PRIMARY KEY,
       name VARCHAR(100),
       manager_id INTEGER REFERENCES test_employees(id)
   );
   INSERT INTO test_employees VALUES
     (1, 'CEO', NULL),
     (2, 'VP', 1),
     (3, 'Manager', 2),
     (4, 'Employee', 3);
   ```

2. Run schema refresh: `refresh_schema`

3. Run ontology extraction

4. Verify via `probe_relationship`:
   ```
   probe_relationship(from_entity='Test Employee', to_entity='Test Employee')
   ```

5. Expected: Relationship appears with:
   - `from_entity: "Test Employee"`
   - `to_entity: "Test Employee"`
   - `from_column: "test_employees.manager_id"`
   - `to_column: "test_employees.id"`
   - Appropriate role label (e.g., "manages" or "reports_to")

## Edge Cases to Consider

1. **Multiple self-referential FKs**: A table might have both `parent_id` AND `supervisor_id` pointing to itself
   - Each should create a separate relationship
   - Role labels should differentiate them

2. **Duplicate detection**: Ensure no duplicate relationships are created
   - The unique constraint on relationships should handle this

3. **UI display**: Self-referential relationships need special handling in graph visualization
   - Typically rendered as a curved arrow looping back to the same node

## Cardinality for Self-References

Self-referential relationships typically have:
- `N:1` cardinality (many employees have one manager)
- Could be `1:N` depending on perspective (one manager has many employees)

The cardinality detection in FK Discovery should handle this correctly based on column statistics.

# FIX: MCP Update Tool Bugs

**Date:** 2026-01-21
**Status:** Ready for Implementation
**Discovered:** MCP↔UI Integration Testing

## Summary

Two bugs in MCP update tools cause operations to fail:

| Bug | Severity | File | Root Cause |
|-----|----------|------|------------|
| 1 | High | `pkg/mcp/tools/glossary.go:320` | Nil pointer dereference |
| 2 | Medium | `pkg/mcp/tools/knowledge.go` + `pkg/repositories/knowledge_repository.go` | Wrong conflict key in upsert |

**Note:** Bug 1 from BUGS-ontology-testing.md (singularization) was stale data from before the fix. Current pending changes show correct singularization. That bug can be closed.

---

## Bug 1: update_glossary_term Nil Pointer Dereference

### Root Cause

In `pkg/mcp/tools/glossary.go`, line 320 accesses `term.Source` but `term` is nil at that point in the code flow:

```go
// Line 285: var term *models.BusinessGlossaryTerm  // nil
// Line 287: if existing == nil { ... create new ... }
// Line 317: } else {  // <-- Update existing path
// Line 320:     if !canModifyGlossaryTerm(term.Source, ...)  // BUG: term is nil!
```

### Fix

**File:** `pkg/mcp/tools/glossary.go`
**Line:** 320

Change:
```go
if !canModifyGlossaryTerm(term.Source, models.GlossarySourceMCP) {
```

To:
```go
if !canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP) {
```

### Test

```bash
# After fix, this should succeed:
# 1. Create a term via MCP
# 2. Update the same term via MCP
```

---

## Bug 2: update_project_knowledge Duplicate Key Error

### Root Cause

The `Upsert` method uses `ON CONFLICT (project_id, fact_type, key)` but when `fact_id` is provided:

1. `knowledge.go:149` sets `knowledgeFact.ID = factID`
2. `knowledge.go:134` sets `Key: fact` (the NEW fact text)
3. Repository attempts INSERT with:
   - Old ID (from fact_id parameter)
   - New key (from fact parameter)
4. No conflict on `(project_id, fact_type, key)` because key is different
5. INSERT fails with duplicate primary key

### Fix

**File:** `pkg/repositories/knowledge_repository.go`
**Function:** `Upsert`

The upsert needs to handle two cases:
1. **Without fact_id:** Upsert by `(project_id, fact_type, key)` - current behavior
2. **With fact_id:** Update by primary key `id`

Replace lines 35-67 with:

```go
func (r *knowledgeRepository) Upsert(ctx context.Context, fact *models.KnowledgeFact) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	fact.UpdatedAt = now

	// If ID is provided, update by ID (explicit update mode)
	if fact.ID != uuid.Nil {
		query := `
			UPDATE engine_project_knowledge
			SET fact_type = $2, key = $3, value = $4, context = $5, updated_at = $6
			WHERE id = $1
			RETURNING created_at`

		err := scope.Conn.QueryRow(ctx, query,
			fact.ID, fact.FactType, fact.Key, fact.Value, fact.Context, fact.UpdatedAt,
		).Scan(&fact.CreatedAt)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("fact with id %s not found", fact.ID)
			}
			return fmt.Errorf("failed to update knowledge fact: %w", err)
		}
		return nil
	}

	// No ID provided - upsert by (project_id, fact_type, key)
	fact.ID = uuid.New()
	fact.CreatedAt = now

	query := `
		INSERT INTO engine_project_knowledge (
			id, project_id, fact_type, key, value, context, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, fact_type, key)
		DO UPDATE SET
			value = EXCLUDED.value,
			context = EXCLUDED.context,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at`

	err := scope.Conn.QueryRow(ctx, query,
		fact.ID, fact.ProjectID, fact.FactType, fact.Key, fact.Value, fact.Context,
		fact.CreatedAt, fact.UpdatedAt,
	).Scan(&fact.ID, &fact.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert knowledge fact: %w", err)
	}

	return nil
}
```

### Test

```bash
# After fix, this should succeed:
# 1. Create fact: update_project_knowledge(fact="Test fact", category="business_rule")
# 2. Note the returned fact_id
# 3. Update: update_project_knowledge(fact_id="<uuid>", fact="Updated fact", context="new context")
```

---

## Implementation Checklist

- [x] Fix glossary.go:320 (`term.Source` → `existing.Source`)
- [x] Fix knowledge_repository.go `Upsert` method
- [x] Add test for glossary term update via MCP
- [x] Add test for project knowledge update with fact_id
- [ ] Run existing tests: `make test-short`
- [ ] Manual verification via MCP tools

---

## Additional Cleanup (Optional)

Update `plans/BUGS-ontology-testing.md`:
- Mark Bug 1 (singularization) as **CLOSED** - stale data issue
- Update Bug 2 and Bug 3 to reference this FIX file

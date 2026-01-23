# FIX: BUG-0 - MCP get_schema Ignores selected_only for Entities

**Bug Reference:** BUGS-ontology-extraction.md, BUG-0
**Severity:** High
**Type:** MCP Tool Bug

## Problem Summary

The MCP tool `get_schema` with `selected_only=true` correctly filters **tables and columns** but does **not** filter the **entity list**. Entities from deselected tables appear in the "DOMAIN ENTITIES:" section even though their tables are excluded from the schema output.

## Root Cause

**File:** `pkg/services/schema.go`

**Function:** `GetDatasourceSchemaWithEntities` (lines 1085-1200+)

The function correctly handles `selectedOnly` for tables (lines 1090-1094):
```go
if selectedOnly {
    schema, err = s.GetSelectedDatasourceSchema(ctx, projectID, datasourceID)
} else {
    schema, err = s.GetDatasourceSchema(ctx, projectID, datasourceID)
}
```

But then unconditionally fetches ALL entities (line 1100):
```go
entities, err := s.entityRepo.GetByProject(ctx, projectID)
```

And outputs all of them without filtering (lines 1126-1133):
```go
if len(entities) > 0 {
    sb.WriteString("DOMAIN ENTITIES:\n")
    for _, entity := range entities {
        sb.WriteString(fmt.Sprintf("  - %s: %s\n", entity.Name, entity.Description))
        sb.WriteString(fmt.Sprintf("    Primary location: %s.%s.%s\n",
            entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn))
    }
}
```

## The Fix

After fetching entities (line 1100), add filtering logic when `selectedOnly=true`:

```go
entities, err := s.entityRepo.GetByProject(ctx, projectID)
if err != nil {
    return "", fmt.Errorf("failed to get entities: %w", err)
}

// Filter entities to only include those from selected tables
if selectedOnly {
    selectedTableNames := make(map[string]bool)
    for _, table := range schema.Tables {
        selectedTableNames[table.TableName] = true
    }

    filteredEntities := make([]*models.OntologyEntity, 0)
    for _, entity := range entities {
        if selectedTableNames[entity.PrimaryTable] {
            filteredEntities = append(filteredEntities, entity)
        }
    }
    entities = filteredEntities
}
```

## Implementation Steps

1. **Edit `pkg/services/schema.go`:**
   - Locate `GetDatasourceSchemaWithEntities` function (line 1085)
   - After the entity fetch (line 1100-1103), add the filtering logic above
   - Insert the filtering block before the entity lookup maps are built (before line 1111)

2. **Add unit test:**
   - Create test in `pkg/services/schema_test.go`
   - Test that `GetDatasourceSchemaWithEntities(ctx, projectID, datasourceID, true)` excludes entities from deselected tables
   - Verify entity list matches tables shown in schema output

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/schema.go` | Add entity filtering in `GetDatasourceSchemaWithEntities` |
| `pkg/services/schema_test.go` | Add test for selectedOnly entity filtering |

## Testing

### Manual Verification

1. Deselect sample tables (s1_* through s10_*)
2. Call MCP `get_schema` with `selected_only=true`
3. Verify "DOMAIN ENTITIES:" section contains ONLY entities from selected tables
4. Verify no sample table entities (Activity, Address, Category, etc.) appear

### SQL Verification

```sql
-- After fix, this query should match entities in MCP output:
SELECT e.name, e.primary_table
FROM engine_ontology_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
JOIN engine_schema_tables t ON e.primary_table = t.table_name
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true
AND t.is_selected = true
AND t.project_id = o.project_id;
```

## Success Criteria

- [ ] `get_schema(selected_only=true)` returns only entities whose `PrimaryTable` is in selected tables
- [ ] Entity list matches tables shown in schema output
- [ ] Deselecting sample tables excludes their entities from output
- [ ] Unit test validates filtering behavior

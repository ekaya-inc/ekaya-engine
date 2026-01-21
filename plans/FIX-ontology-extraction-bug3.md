# FIX: BUG-3 - Duplicate/Conflicting User Entities

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-3
**Severity:** High
**Category:** Entity Discovery

## Problem Summary

Multiple entities represent the same business concept "users":
- `User` entity → points to `s2_users` (sample table - WRONG)
- `users` entity → points to `users` (real table, but no description)
- `s5_users` entity → points to `s5_users` (sample table)

The actual Tikr `users` table should be the primary User entity, but a sample table got that name.

## Root Cause

**No entity deduplication exists** at any level of the system:

### 1. Entities Created Per-Table Without Coordination

**File:** `pkg/services/entity_discovery_service.go:71-188`

```go
// Line 163-164: One entity per table, no dedup check
entity := OntologyEntity{
    Name:        tableName,  // Temporary name = table name
    Description: "",
}
```

### 2. LLM Enrichment Is Table-Centric, Not Ontology-Centric

**File:** `pkg/services/entity_discovery_service.go:355-408`

The enrichment prompt:
- Analyzes each table independently
- Never passes existing entity names to LLM
- No instruction to avoid duplicate names

```go
// No context about other entities or their names
prompt := fmt.Sprintf(`Analyze the following database tables and provide entity information...`)
```

### 3. No Uniqueness Validation in Repository

**File:** `pkg/repositories/ontology_entity_repository.go:61-97`

```go
// Line 80-85: INSERT without name uniqueness check
result, err := r.db.Exec(ctx, query, args...)
// No "ON CONFLICT" handling for duplicate names
```

### 4. No Post-Enrichment Merging

After enrichment completes, there's no pass to:
- Identify entities with similar names
- Merge duplicate concepts
- Flag conflicts for human review

## Fix Implementation

### Option A: Pre-Enrichment Entity Merging (Recommended)

Detect potential duplicates before LLM enrichment and handle them intelligently.

#### 1. Add Entity Grouping by Table Name Similarity

**File:** `pkg/services/entity_discovery_service.go`

```go
// Group tables that likely represent the same concept
func groupSimilarTables(tables []SchemaTable) map[string][]SchemaTable {
    groups := make(map[string][]SchemaTable)

    for _, t := range tables {
        // Extract core concept: "s1_users", "s2_users", "users" all → "users"
        concept := extractCoreConcept(t.TableName)
        groups[concept] = append(groups[concept], t)
    }
    return groups
}

func extractCoreConcept(tableName string) string {
    // Remove common prefixes: s1_, s2_, test_, tmp_, etc.
    name := tableName
    patterns := []string{`^s\d+_`, `^test_`, `^tmp_`, `^temp_`, `^staging_`}
    for _, p := range patterns {
        name = regexp.MustCompile(p).ReplaceAllString(name, "")
    }
    return strings.ToLower(name)
}
```

#### 2. Create Single Entity Per Concept Group

```go
func (s *entityDiscoveryService) IdentifyEntitiesFromDDL(...) {
    tables, _ := s.schemaRepo.ListTablesByDatasource(...)

    // Group similar tables
    groups := groupSimilarTables(tables)

    for concept, tableTables := range groups {
        // Pick the "primary" table (prefer non-prefixed)
        primaryTable := selectPrimaryTable(tableTables)

        // Create ONE entity for the concept
        entity := OntologyEntity{
            Name:         concept,
            PrimaryTable: primaryTable.TableName,
            Description:  "",
        }

        // Store other tables as "alternate tables" or aliases
        for _, alt := range tableTables {
            if alt.TableName != primaryTable.TableName {
                // Record as alias or secondary occurrence
                s.addEntityOccurrence(entity, alt.TableName, "alternate_table")
            }
        }
    }
}

func selectPrimaryTable(tables []SchemaTable) SchemaTable {
    // Prefer tables without test prefixes
    for _, t := range tables {
        if !hasTestPrefix(t.TableName) {
            return t
        }
    }
    return tables[0]  // Fallback to first
}
```

### Option B: Enrichment Prompt Includes Name Context

Pass existing entity names to LLM to prevent conflicts.

**File:** `pkg/services/entity_discovery_service.go:355-408`

```go
func (s *entityDiscoveryService) buildEntityEnrichmentPrompt(tables []SchemaTable, existingNames []string) string {
    // Add existing names to prompt
    prompt += fmt.Sprintf(`
EXISTING ENTITY NAMES (DO NOT REUSE):
%s

When naming entities, you MUST:
1. Check if a similar name already exists
2. Choose a distinct name if the concept is different
3. Merge tables representing the same concept under one name
`, strings.Join(existingNames, ", "))
}
```

### Option C: Post-Enrichment Deduplication

After LLM enrichment, detect and merge duplicates.

```go
func (s *entityDiscoveryService) deduplicateEntities(ctx context.Context, ontologyID string) error {
    entities, _ := s.entityRepo.ListByOntology(ctx, ontologyID)

    // Group by normalized name
    byName := make(map[string][]*OntologyEntity)
    for _, e := range entities {
        normalized := normalizeEntityName(e.Name)
        byName[normalized] = append(byName[normalized], e)
    }

    // Merge duplicates
    for _, group := range byName {
        if len(group) > 1 {
            primary := selectPrimaryEntity(group)
            for _, dup := range group {
                if dup.ID != primary.ID {
                    s.mergeEntities(ctx, primary, dup)
                }
            }
        }
    }
    return nil
}

func selectPrimaryEntity(entities []*OntologyEntity) *OntologyEntity {
    // Prefer entity with description, non-test primary table
    for _, e := range entities {
        if e.Description != "" && !hasTestPrefix(e.PrimaryTable) {
            return e
        }
    }
    return entities[0]
}
```

### Option D: Database Constraint + Conflict Resolution

Add uniqueness constraint and handle conflicts.

```sql
-- Add unique constraint
ALTER TABLE engine_ontology_entities
ADD CONSTRAINT unique_entity_name_per_ontology
UNIQUE (ontology_id, name);
```

Then handle conflicts in code:
```go
// On conflict, append suffix or merge
_, err := r.db.Exec(ctx, `
    INSERT INTO engine_ontology_entities (...)
    VALUES (...)
    ON CONFLICT (ontology_id, name) DO UPDATE
    SET description = COALESCE(EXCLUDED.description, engine_ontology_entities.description)
`, args...)
```

## Recommended Approach

**Combine Options A + D:**

1. **Option A** prevents the problem at source - group similar tables before entity creation
2. **Option D** provides a safety net at database level

This ensures:
- Test tables (`s1_users`, `s2_users`) don't create separate entities from real `users` table
- Even if duplicates slip through, database constraint catches them
- Primary entity is always the "real" table (non-prefixed)

## Testing

1. Create tables: `users`, `s1_users`, `s2_users`, `test_users`
2. Run ontology extraction
3. Verify: Only ONE "User" entity exists, pointing to `users` table
4. Verify: Other tables appear as occurrences/aliases, not separate entities

## Acceptance Criteria

- [x] One entity per business concept (no duplicates)
- [x] Real tables preferred over test/sample tables as primary
- [ ] Entity name uniqueness enforced at database level
- [ ] Test tables grouped under real table's entity as aliases
- [ ] LLM enrichment includes context about existing names

# FIX: BUG-2 - Unnamed/Empty Description Entities

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-2
**Severity:** High
**Category:** Entity Enrichment

## Problem Summary

Some entities have no descriptions and retain raw table names instead of enriched business names:
- `users` (primary table: users) - empty description
- `s10_events` (primary table: s10_events) - empty description
- `s5_users` (primary table: s5_users) - empty description

## Root Cause

There are **three distinct failure paths** that result in entities saved with raw table names and empty descriptions:

### 1. LLM Parse Failures Are Swallowed

**File:** `pkg/services/entity_discovery_service.go:270-285`

When LLM returns invalid/unparseable JSON:
```go
enrichments, err := parseEntityEnrichmentResponse(response.Content)
if err != nil {
    logger.Warn("Failed to parse entity enrichment response, keeping original names", "error", err)
    return nil  // ← Returns nil, not error! Workflow continues
}
```

**Impact:** Entities remain with temporary table names created at discovery phase (line 164: `Description: ""`).

### 2. Entities Not in LLM Response Are Silently Skipped

**File:** `pkg/services/entity_discovery_service.go:294-340`

```go
for _, entity := range entities {
    enrichmentData, found := enrichmentByTable[entity.PrimaryTable]
    if found {
        // Update entity...
    }
    // ← No else clause! Entity silently skipped if not found
}
```

**Impact:** If LLM response is truncated or incomplete, some entities get enriched while others don't.

### 3. LLM Call Failures Are Swallowed in DAG Node

**File:** `pkg/services/dag/entity_enrichment_node.go:56-60`

```go
if err != nil {
    logger.Warn("Failed to enrich entities with LLM", "error", err)
    // ← No error return! DAG node completes "successfully"
}
```

**Impact:** Entire enrichment phase can fail silently.

### Entity Creation Flow

1. **Discovery Phase** (line 163-164):
   ```go
   entity := OntologyEntity{
       Name:        tableName,     // Raw table name
       Description: "",            // Empty!
   }
   ```

2. **Enrichment Phase**:
   - If successful: Updates with LLM-generated name/description
   - If any failure: No update, database retains empty values

## Fix Implementation

### 1. Make Parse Failures Fail Fast

**File:** `pkg/services/entity_discovery_service.go`

Replace soft warning with hard error:
```go
enrichments, err := parseEntityEnrichmentResponse(response.Content)
if err != nil {
    // Record parse failure in LLM conversation
    s.recordParseFailure(ctx, projectID, conversationID, err)
    return fmt.Errorf("entity enrichment parse failure: %w", err)
}
```

### 2. Track Unenriched Entities

**File:** `pkg/services/entity_discovery_service.go`

Add tracking for entities not found in LLM response:
```go
var unenrichedTables []string

for _, entity := range entities {
    enrichmentData, found := enrichmentByTable[entity.PrimaryTable]
    if !found {
        unenrichedTables = append(unenrichedTables, entity.PrimaryTable)
        continue
    }
    // Update entity...
}

if len(unenrichedTables) > 0 {
    return fmt.Errorf("entity enrichment incomplete: %d entities not in LLM response: %v",
        len(unenrichedTables), unenrichedTables)
}
```

### 3. Propagate Errors from DAG Node

**File:** `pkg/services/dag/entity_enrichment_node.go`

```go
if err != nil {
    return fmt.Errorf("entity enrichment failed: %w", err)
}
```

### 4. Add Enrichment Validation

After enrichment, validate all entities have descriptions:
```go
func (s *entityDiscoveryService) validateEnrichment(ctx context.Context, ontologyID string) error {
    entities, _ := s.entityRepo.ListByOntology(ctx, ontologyID)

    var empty []string
    for _, e := range entities {
        if e.Description == "" {
            empty = append(empty, e.Name)
        }
    }

    if len(empty) > 0 {
        return fmt.Errorf("%d entities lack descriptions: %v", len(empty), empty)
    }
    return nil
}
```

### 5. Consider Retry with Batching

If many entities cause token limits, implement batched enrichment:
```go
// Split entities into batches of 20
const batchSize = 20
for i := 0; i < len(entities); i += batchSize {
    batch := entities[i:min(i+batchSize, len(entities))]
    if err := s.enrichBatch(ctx, batch); err != nil {
        return err
    }
}
```

## Detection Query

Find entities with missing enrichment:
```sql
SELECT id, name, description, primary_table, created_at
FROM engine_ontology_entities
WHERE description = '' OR description IS NULL
  OR name = primary_table  -- Raw table name retained
ORDER BY created_at DESC;
```

Check for corresponding LLM failures:
```sql
SELECT model, status, error_message, created_at
FROM engine_llm_conversations
WHERE status != 'success'
  AND context->>'step' = 'entity_enrichment'
ORDER BY created_at DESC;
```

## Acceptance Criteria

- [ ] All entities have non-empty descriptions after enrichment
- [ ] Entity names are human-readable (not raw table names)
- [x] Parse failures are surfaced as errors, not swallowed
- [ ] Partial enrichment (incomplete LLM response) fails the workflow
- [ ] Failed enrichment is visible in DAG status
- [ ] LLM conversation records show clear status for troubleshooting

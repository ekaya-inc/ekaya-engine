# PLAN: Entity Promotion Model - Earned Entities vs Table Metadata

## Problem Statement

The current ontology extractor creates an "entity" for nearly every table, resulting in redundant abstraction. When Entity = Table 1:1, the entity layer adds no semantic value and creates maintenance overhead.

**Current state:** ~15 entities for tikr_production, most are 1:1 with tables
**Desired state:** 4-5 meaningful entities that aggregate tables or express roles, with everything else as enriched table/column metadata

## Design Decision

Entities should be **earned through semantic complexity**, not auto-created for every table.

### When to Create an Entity

| Condition | Example | Why Entity Adds Value |
|-----------|---------|----------------------|
| Multiple tables represent same concept | `users`, `user_profiles`, `user_settings` → User entity | Groups related tables logically |
| Multiple FKs reference same target with different roles | `host_id`, `visitor_id` both → User | Captures role semantics (host vs visitor) |
| Business aliases exist | "creator", "owner", "participant" all mean User | Maps business language to schema |
| Hub in relationship graph | User connects to 5+ other concepts | Worth visualizing as node |

### When NOT to Create an Entity

| Condition | Example | What to Do Instead |
|-----------|---------|-------------------|
| Entity name = table name, nothing more | Session entity = sessions table | Use table metadata only |
| No aliases or roles | BillingTransaction = billing_transactions | Table description suffices |
| Leaf node with single relationship | PayoutAccount connects only to User | FK metadata on column |

## Implementation Tasks

### Task 1: Add Entity Promotion Criteria to Schema

**File:** `pkg/ontology/types.go` (or equivalent)

Add fields to track why an entity exists:

```go
type Entity struct {
    Name        string   `json:"name"`
    Description string   `json:"description,omitempty"`

    // Promotion criteria - why this entity exists
    PrimaryTable   string   `json:"primary_table"`            // Main table for this entity
    SecondaryTables []string `json:"secondary_tables,omitempty"` // Additional tables (user_profiles, etc.)
    Aliases        []string `json:"aliases,omitempty"`         // Business terms (host, visitor, creator)
    Roles          []string `json:"roles,omitempty"`           // Roles this entity plays in relationships

    // Computed during extraction
    RelationshipCount int  `json:"relationship_count"` // How many relationships involve this entity
    IsPromoted        bool `json:"is_promoted"`        // True if meets promotion criteria
}
```

**Acceptance criteria:**
- Entity struct has fields to capture promotion justification
- Existing entity serialization still works (backwards compatible)

---

### Task 2: Implement Promotion Scoring Function

**File:** `pkg/ontology/extractor.go` (or equivalent)

Create a function that scores whether a table should be promoted to entity:

```go
// PromotionScore evaluates if a table warrants entity status
// Returns score 0-100, where >= 50 means promote to entity
func PromotionScore(table Table, allTables []Table, relationships []Relationship) int {
    score := 0
    reasons := []string{}

    // Criterion 1: Multiple tables share this concept (30 points)
    // Look for tables with same prefix: user_*, account_*
    relatedTables := findRelatedTables(table.Name, allTables)
    if len(relatedTables) > 1 {
        score += 30
        reasons = append(reasons, fmt.Sprintf("aggregates %d tables", len(relatedTables)))
    }

    // Criterion 2: Multiple FKs reference this table with different roles (25 points)
    // e.g., host_id and visitor_id both reference users
    roleRefs := findRoleBasedReferences(table.Name, allTables)
    if len(roleRefs) >= 2 {
        score += 25
        reasons = append(reasons, fmt.Sprintf("%d role-based references", len(roleRefs)))
    }

    // Criterion 3: Hub in relationship graph (20 points)
    // Count relationships where this table is source or target
    relCount := countRelationships(table.Name, relationships)
    if relCount >= 4 {
        score += 20
        reasons = append(reasons, fmt.Sprintf("hub with %d relationships", relCount))
    }

    // Criterion 4: Has known business aliases (15 points)
    // Check against common alias patterns
    aliases := detectAliases(table.Name)
    if len(aliases) > 0 {
        score += 15
        reasons = append(reasons, fmt.Sprintf("aliases: %v", aliases))
    }

    // Criterion 5: Name differs from table name (10 points)
    // If we'd call the entity something different than the table
    entityName := singularize(table.Name) // users -> User
    if entityName != table.Name {
        score += 10
    }

    return score, reasons
}

// Helper: Find tables that likely belong to same entity
func findRelatedTables(tableName string, allTables []Table) []Table {
    prefix := extractPrefix(tableName) // "users" -> "user"
    var related []Table
    for _, t := range allTables {
        if strings.HasPrefix(t.Name, prefix) {
            related = append(related, t)
        }
    }
    return related
}

// Helper: Find columns that reference this table with role patterns
func findRoleBasedReferences(tableName string, allTables []Table) []RoleRef {
    var refs []RoleRef
    targetCol := guessPrimaryKey(tableName) // users -> user_id

    rolePatterns := []string{"host", "visitor", "creator", "owner", "sender",
                             "recipient", "payer", "payee", "source", "destination"}

    for _, t := range allTables {
        for _, col := range t.Columns {
            if !strings.HasSuffix(col.Name, "_id") {
                continue
            }
            // Check if column references our target with a role prefix
            for _, role := range rolePatterns {
                if strings.HasPrefix(col.Name, role) && fkTargetsTable(col, tableName) {
                    refs = append(refs, RoleRef{
                        Table:  t.Name,
                        Column: col.Name,
                        Role:   role,
                    })
                }
            }
        }
    }
    return refs
}
```

**Acceptance criteria:**
- Function returns score 0-100 for any table
- Score >= 50 means table should be promoted to entity
- Function returns list of reasons for promotion (for transparency)
- Unit tests cover each criterion

---

### Task 3: Update Extraction Pipeline

**File:** `pkg/ontology/extractor.go`

Modify the extraction flow to use promotion scoring:

```go
func ExtractOntology(schema Schema) (*Ontology, error) {
    ontology := &Ontology{
        Tables:   make([]TableMetadata, 0),
        Entities: make([]Entity, 0),
    }

    // Step 1: Extract all table metadata (always done)
    for _, table := range schema.Tables {
        tableMeta := extractTableMetadata(table)
        ontology.Tables = append(ontology.Tables, tableMeta)
    }

    // Step 2: Detect relationships from FK patterns
    relationships := detectRelationships(schema)

    // Step 3: Score each table for entity promotion
    for _, table := range schema.Tables {
        score, reasons := PromotionScore(table, schema.Tables, relationships)

        if score >= 50 {
            // Promote to entity
            entity := Entity{
                Name:              singularize(table.Name),
                PrimaryTable:      table.Name,
                SecondaryTables:   findRelatedTables(table.Name, schema.Tables),
                Aliases:           detectAliases(table.Name),
                Roles:             extractRoles(table.Name, schema.Tables),
                RelationshipCount: countRelationships(table.Name, relationships),
                IsPromoted:        true,
            }
            ontology.Entities = append(ontology.Entities, entity)

            // Log promotion decision
            log.Info("Promoted table to entity",
                "table", table.Name,
                "entity", entity.Name,
                "score", score,
                "reasons", reasons)
        } else {
            // Keep as table-only, log why not promoted
            log.Debug("Table not promoted to entity",
                "table", table.Name,
                "score", score)
        }
    }

    return ontology, nil
}
```

**Acceptance criteria:**
- Extraction produces both Tables (all) and Entities (promoted only)
- Promotion decisions are logged with reasons
- Backwards compatible with existing ontology consumers

---

### Task 4: Update get_context and get_ontology Tools

**File:** `pkg/mcp/tools.go` (or equivalent)

Modify the MCP tools to reflect the new model:

```go
// get_context at 'entities' depth should only return promoted entities
func handleGetContext(params GetContextParams) (*ContextResponse, error) {
    switch params.Depth {
    case "domain":
        // High-level: just entity names and domain facts
        return getDomainContext()

    case "entities":
        // Only promoted entities, not every table
        entities := getPromotedEntities()
        return &ContextResponse{
            Entities: entities,
            Note: "Showing promoted entities only. Use 'tables' depth for all tables.",
        }

    case "tables":
        // All tables with metadata
        return getTableContext(params.Tables)

    case "columns":
        // Full column details
        return getColumnContext(params.Tables)
    }
}
```

**Acceptance criteria:**
- `get_context(depth='entities')` returns only promoted entities (4-5 for tikr_production)
- `get_context(depth='tables')` returns all tables with rich metadata
- Response indicates when entities are filtered

---

### Task 5: Add Manual Promotion/Demotion API

**File:** `pkg/mcp/tools.go`

Allow humans to override promotion decisions:

```go
// promote_to_entity - manually promote a table to entity status
type PromoteToEntityParams struct {
    TableName   string   `json:"table_name"`
    EntityName  string   `json:"entity_name,omitempty"` // defaults to singularized table name
    Aliases     []string `json:"aliases,omitempty"`
    Description string   `json:"description,omitempty"`
}

func handlePromoteToEntity(params PromoteToEntityParams) (*Entity, error) {
    // Validate table exists
    table, err := getTable(params.TableName)
    if err != nil {
        return nil, fmt.Errorf("table not found: %s", params.TableName)
    }

    entityName := params.EntityName
    if entityName == "" {
        entityName = singularize(params.TableName)
    }

    entity := &Entity{
        Name:         entityName,
        PrimaryTable: params.TableName,
        Aliases:      params.Aliases,
        Description:  params.Description,
        IsPromoted:   true,
        Provenance:   "manual", // Track that human promoted this
    }

    return upsertEntity(entity)
}

// demote_entity - remove entity status, keep as table only
func handleDemoteEntity(entityName string) error {
    return deleteEntity(entityName)
}
```

**Acceptance criteria:**
- `promote_to_entity` tool available in MCP
- `demote_entity` tool available in MCP
- Manual promotions tracked with provenance="manual"
- Manual promotions persist across re-extraction

---

### Task 6: Migration for Existing Ontologies

**File:** `migrations/XXX_entity_promotion.go`

For existing ontologies, score and demote redundant entities:

```go
func MigrateToPromotionModel(ontology *Ontology) (*Ontology, error) {
    var promotedEntities []Entity
    var demotedTables []string

    for _, entity := range ontology.Entities {
        // Check if entity was manually created (preserve these)
        if entity.Provenance == "manual" || entity.Provenance == "admin" {
            entity.IsPromoted = true
            promotedEntities = append(promotedEntities, entity)
            continue
        }

        // Score existing entity
        table := findTableByName(entity.PrimaryTable, ontology.Tables)
        score, _ := PromotionScore(table, ontology.Tables, ontology.Relationships)

        if score >= 50 {
            entity.IsPromoted = true
            promotedEntities = append(promotedEntities, entity)
        } else {
            // Demote: merge entity metadata into table metadata
            mergeEntityToTable(entity, table)
            demotedTables = append(demotedTables, entity.Name)
        }
    }

    log.Info("Migration complete",
        "promoted", len(promotedEntities),
        "demoted", len(demotedTables))

    ontology.Entities = promotedEntities
    return ontology, nil
}
```

**Acceptance criteria:**
- Existing ontologies can be migrated without data loss
- Demoted entity metadata merges into table metadata
- Manual/admin entities are preserved
- Migration is idempotent

---

## Expected Outcome for tikr_production

**Before (current):**
```
Entities: 15
- User, Account, Session, Engagement, BillingTransaction,
- BillingEngagement, PayoutAccount, Media, Campaign,
- Notification, Email, EServer, Channel, Participant, Device
```

**After (with promotion model):**
```
Entities: 4-5 (promoted)
- User (score: 85) - aggregates user_*, has roles host/visitor/creator
- Account (score: 60) - 1:N relationship hub to User
- Engagement (score: 55) - central to billing, multiple relationships
- Media (score: 50) - has status enum, multiple references

Tables with rich metadata: 15+ (all tables)
- Each table has descriptions, FK annotations, enum values
- No entity overhead for simple tables
```

## Testing Strategy

1. **Unit tests** for PromotionScore function
   - Table with 3 related tables scores >= 30
   - Table with 2 role-based FKs scores >= 25
   - Table with 5 relationships scores >= 20
   - Combination scoring works correctly

2. **Integration test** on tikr_production
   - Run extraction with promotion model
   - Verify 4-5 entities promoted
   - Verify all tables have metadata
   - Verify promoted entities have correct aliases/roles

3. **Regression test**
   - Existing MCP tools still work
   - get_context returns expected structure
   - AI agents can still answer questions

## Success Metrics

| Metric | Before | After |
|--------|--------|-------|
| Entity count (tikr_production) | ~15 | 4-5 |
| Tables with rich metadata | ~15 | ~15 (unchanged) |
| Entity:Table redundancy | ~80% | 0% |
| Promotion decisions explainable | No | Yes (logged reasons) |

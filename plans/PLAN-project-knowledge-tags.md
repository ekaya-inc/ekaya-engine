# Plan: Incorporate Project Knowledge Tags into Prompts

## Problem Statement

Currently, project knowledge facts are stored with a single `fact_type` category (e.g., "business_rule", "terminology"). This limits our ability to:
1. Apply domain-specific facts to relevant LLM prompts
2. Build context-aware prompts that incorporate learned knowledge
3. Efficiently retrieve facts relevant to specific analysis tasks

## Proposed Solution

Replace the single `fact_type` with a multi-tag system where:
- Each fact has 1+ tags from a predefined set of 20-30 semantic tags
- LLM outputs tags from this set when creating/updating facts
- Prompt builders query facts by relevant tags and inject them as context
- Example: Column feature extraction for potential currency columns includes all facts tagged `money`

## Current State

**Table:** `engine_project_knowledge`
- `fact_type` (varchar) - single category: "business_rule", "terminology", "convention", etc.
- `key` (varchar) - unique identifier within project+fact_type
- `value` (text) - the fact content

**Files:**
- Model: `pkg/models/ontology_chat.go:207-228`
- Repository: `pkg/repositories/knowledge_repository.go`
- Service: `pkg/services/knowledge.go`
- Seeding: `pkg/services/knowledge_seeding.go`
- Prompts: `pkg/services/incremental_dag_prompts.go`

## Design

### 1. Tag Taxonomy (20-30 Tags)

**Category: Financial**
- `money` - Currency, amounts, pricing
- `billing` - Invoices, charges, payments
- `accounting` - Revenue, costs, ledger entries

**Category: Time & Dates**
- `temporal` - Timestamps, durations, schedules
- `fiscal` - Fiscal years, quarters, periods
- `lifecycle` - Created/updated/deleted patterns

**Category: Identity & Relationships**
- `user` - Users, accounts, profiles
- `organization` - Companies, teams, tenants
- `hierarchy` - Parent-child, org structures

**Category: Status & State**
- `status` - State machines, workflow states
- `boolean` - Flags, toggles, yes/no
- `enumeration` - Fixed value sets

**Category: Classification**
- `category` - Types, classifications
- `geography` - Countries, regions, addresses
- `product` - Products, SKUs, inventory

**Category: Technical**
- `identifier` - IDs, keys, references
- `measurement` - Quantities, metrics
- `percentage` - Rates, ratios

**Category: Domain-Specific**
- `terminology` - Domain-specific terms
- `business_rule` - Validation rules, constraints
- `convention` - Naming patterns, standards
- `calculation` - Formulas, derived values

**Category: Data Quality**
- `nullability` - NULL handling rules
- `cardinality` - One-to-many, uniqueness
- `format` - Data formats, patterns

### 2. Schema Changes

```sql
-- Migration: Add tags column, migrate fact_type to tags
ALTER TABLE engine_project_knowledge
ADD COLUMN tags text[] NOT NULL DEFAULT '{}';

-- Migrate existing fact_types to tags array
UPDATE engine_project_knowledge
SET tags = ARRAY[fact_type]
WHERE fact_type IS NOT NULL;

-- Create GIN index for efficient tag queries
CREATE INDEX idx_project_knowledge_tags
ON engine_project_knowledge USING GIN (tags);

-- Keep fact_type for backwards compatibility initially
-- Future: Remove fact_type column after migration verified
```

### 3. Model Changes

**File:** `pkg/models/ontology_chat.go`

```go
// Predefined knowledge tags
var ValidKnowledgeTags = []string{
    // Financial
    "money", "billing", "accounting",
    // Time
    "temporal", "fiscal", "lifecycle",
    // Identity
    "user", "organization", "hierarchy",
    // Status
    "status", "boolean", "enumeration",
    // Classification
    "category", "geography", "product",
    // Technical
    "identifier", "measurement", "percentage",
    // Domain
    "terminology", "business_rule", "convention", "calculation",
    // Data Quality
    "nullability", "cardinality", "format",
}

type KnowledgeFact struct {
    // ... existing fields ...
    Tags     []string  `json:"tags"`      // Replace FactType
    FactType string    `json:"fact_type"` // Deprecated, kept for migration
}

func ValidateTags(tags []string) error {
    validSet := make(map[string]bool)
    for _, t := range ValidKnowledgeTags {
        validSet[t] = true
    }
    for _, tag := range tags {
        if !validSet[tag] {
            return fmt.Errorf("invalid tag: %s", tag)
        }
    }
    return nil
}
```

### 4. Repository Changes

**File:** `pkg/repositories/knowledge_repository.go`

Add method to query by tags:

```go
// GetByTags returns facts matching ANY of the provided tags
func (r *knowledgeRepository) GetByTags(
    ctx context.Context,
    projectID uuid.UUID,
    tags []string,
) ([]*models.KnowledgeFact, error) {
    query := `
        SELECT id, project_id, fact_type, key, value, context, tags,
               source, last_edit_source, created_by, updated_by,
               created_at, updated_at
        FROM engine_project_knowledge
        WHERE project_id = $1
          AND tags && $2  -- Array overlap operator
          AND deleted_at IS NULL
        ORDER BY created_at`
    // ...
}
```

Update Upsert to handle tags array.

### 5. Service Changes

**File:** `pkg/services/knowledge.go`

```go
// StoreWithTags stores a fact with semantic tags
func (s *knowledgeService) StoreWithTags(
    ctx context.Context,
    projectID uuid.UUID,
    tags []string,
    key, value, contextInfo string,
    source string,
) error {
    if err := models.ValidateTags(tags); err != nil {
        return err
    }
    // ... store logic
}

// GetByTags retrieves facts matching any of the tags
func (s *knowledgeService) GetByTags(
    ctx context.Context,
    projectID uuid.UUID,
    tags []string,
) ([]*models.KnowledgeFact, error) {
    return s.repo.GetByTags(ctx, projectID, tags)
}
```

### 6. LLM Tag Extraction

When LLM creates knowledge facts (e.g., from project overview), include tag selection in the prompt.

**File:** `pkg/services/knowledge_seeding.go`

Update extraction prompt:

```go
const knowledgeExtractionSystemMessage = `
You are extracting business facts from a project overview.

For each fact, output:
- key: A unique identifier for this fact
- value: The fact content
- tags: Array of 1-3 tags from this list: [money, billing, accounting, temporal, fiscal, lifecycle, user, organization, hierarchy, status, boolean, enumeration, category, geography, product, identifier, measurement, percentage, terminology, business_rule, convention, calculation, nullability, cardinality, format]

Choose tags that describe WHAT the fact is about, not its source.
Example: "Revenue is calculated as gross_amount minus refunds" -> tags: ["money", "calculation"]
`
```

Update response struct:

```go
type ExtractedFact struct {
    Key   string   `json:"key"`
    Value string   `json:"value"`
    Tags  []string `json:"tags"`
}
```

### 7. Prompt Builder Integration

**File:** `pkg/services/incremental_dag_prompts.go` (new helper)

```go
// Tag-to-context mapping for prompt builders
var promptTagMapping = map[string][]string{
    "column_monetary":     {"money", "billing", "accounting"},
    "column_temporal":     {"temporal", "fiscal", "lifecycle"},
    "column_identifier":   {"identifier", "user", "organization"},
    "column_status":       {"status", "boolean", "enumeration"},
    "column_category":     {"category", "product", "geography"},
    "entity_discovery":    {"user", "organization", "hierarchy", "terminology"},
    "relationship":        {"cardinality", "hierarchy"},
}

// GetKnowledgeForPromptContext retrieves relevant facts for a prompt context
func GetKnowledgeForPromptContext(
    ctx context.Context,
    knowledgeSvc KnowledgeService,
    projectID uuid.UUID,
    promptContext string,
) (string, error) {
    tags, ok := promptTagMapping[promptContext]
    if !ok {
        return "", nil
    }

    facts, err := knowledgeSvc.GetByTags(ctx, projectID, tags)
    if err != nil {
        return "", err
    }

    if len(facts) == 0 {
        return "", nil
    }

    var sb strings.Builder
    sb.WriteString("\n## Relevant Domain Knowledge\n")
    for _, fact := range facts {
        sb.WriteString(fmt.Sprintf("- %s: %s\n", fact.Key, fact.Value))
    }
    return sb.String(), nil
}
```

### 8. Integration Points

Update these prompt builders to inject knowledge:

1. **Column Feature Extraction** (`pkg/services/column_feature_extraction.go`)
   - When classifying monetary columns, include `money`, `billing` facts
   - When classifying temporal columns, include `temporal`, `fiscal` facts

2. **Entity Discovery** (`pkg/services/entity_discovery.go`)
   - Include `user`, `organization`, `hierarchy`, `terminology` facts

3. **Relationship Enrichment** (`pkg/services/incremental_dag_prompts.go`)
   - Include `cardinality`, `hierarchy` facts

4. **MCP Tools** (`pkg/mcp/tools/`)
   - `update_project_knowledge` tool updated to accept tags
   - New tag suggestions in tool description

## Implementation Tasks

### Phase 1: Schema & Model (Foundation)
1. [ ] Create migration to add `tags` column
2. [ ] Create migration to populate `tags` from existing `fact_type`
3. [ ] Add GIN index for tag queries
4. [ ] Update `KnowledgeFact` model with `Tags` field
5. [ ] Add `ValidKnowledgeTags` constant and validation function
6. [ ] Update repository `Upsert` to handle tags
7. [ ] Add repository `GetByTags` method

### Phase 2: Service Layer
8. [ ] Add `StoreWithTags` to knowledge service
9. [ ] Add `GetByTags` to knowledge service
10. [ ] Update existing `Store` to map old fact_types to tags (backwards compat)

### Phase 3: LLM Tag Extraction
11. [ ] Update knowledge seeding prompt to include tag taxonomy
12. [ ] Update extraction response struct to include tags
13. [ ] Update parsing to extract tags from LLM response
14. [ ] Validate tags before storing

### Phase 4: Prompt Integration
15. [ ] Create `GetKnowledgeForPromptContext` helper
16. [ ] Define `promptTagMapping` for each prompt context
17. [ ] Integrate into column feature extraction prompts
18. [ ] Integrate into entity discovery prompts
19. [ ] Integrate into relationship enrichment prompts

### Phase 5: MCP Tools
20. [ ] Update `update_project_knowledge` MCP tool to accept tags
21. [ ] Update tool description with valid tag list
22. [ ] Update `get_context` to return facts with tags

### Phase 6: Cleanup
23. [ ] Remove deprecated `FactType` field after migration verified
24. [ ] Update tests

## Testing Strategy

1. **Unit Tests**
   - Tag validation
   - Repository GetByTags with various tag combinations
   - Prompt context mapping

2. **Integration Tests**
   - End-to-end: Store fact with tags, retrieve by tags
   - LLM extraction produces valid tags

3. **Manual Testing**
   - Run ontology extraction with project overview
   - Verify facts are tagged appropriately
   - Verify column classification uses relevant facts

## Rollback Plan

1. Tags column is additive; `fact_type` remains populated
2. Can revert to `fact_type`-based queries if issues arise
3. Migration down script restores original schema

## Open Questions

1. Should tags be hierarchical? (e.g., `money.currency` vs flat `money`)
2. Should we support user-defined tags or strict predefined set?
3. How many tags per fact is reasonable? (Suggest: 1-3)
4. Should we weight tags differently for relevance scoring?

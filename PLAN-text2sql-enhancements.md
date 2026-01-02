# PLAN: Text2SQL - Enhancements from claudontology

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

## Enhancements from claudontology Proof-of-Concept

The ekaya-claudontology project demonstrated several advanced concepts that improve text2SQL accuracy, performance, and user experience. This section outlines how to integrate these proven patterns.

### 1. Enhanced Pattern Cache with Multi-Dimensional Similarity

**Current gap:** Basic vector search for (question, SQL) pairs uses only embedding cosine distance.

**Enhancement from claudontology:**

Multi-dimensional similarity scoring combines multiple signals for better pattern matching:

| Dimension | Weight | Description |
|-----------|--------|-------------|
| Keyword overlap | 30% | Jaccard similarity on extracted keywords |
| Entity match | 25% | Tables/columns mentioned (exact match crucial) |
| Operation match | 20% | Query operation types (aggregate, filter, join, rank) |
| Structure match | 15% | Query structure patterns (single table, multi-join, subquery) |
| Parameter compatibility | 10% | Whether time/value parameters can be substituted |

**Confidence evolution system:**
- New patterns start at 0.7 confidence
- Confidence increases by 0.05 per successful execution (capped at 1.0)
- Patterns unused for 30+ days have confidence decayed by 5%
- Patterns with <0.5 confidence unused for 90+ days are auto-removed

**Decision thresholds:**
- Pattern confidence ≥0.85 AND similarity ≥0.9 → Use cached SQL directly (skip LLM)
- Pattern confidence ≥0.7 AND similarity ≥0.8 → Use as few-shot example
- Otherwise → Generate fresh SQL

**Database schema additions:**
```sql
ALTER TABLE vector_embeddings ADD COLUMN
    confidence FLOAT DEFAULT 0.7,
    success_count INT DEFAULT 0,
    failure_count INT DEFAULT 0,
    last_succeeded_at TIMESTAMPTZ,
    last_failed_at TIMESTAMPTZ,
    parameter_slots JSONB DEFAULT '{}'::jsonb;

CREATE INDEX idx_vector_embeddings_confidence
    ON vector_embeddings (project_id, collection_name, confidence DESC)
    WHERE collection_name = 'query_history';
```

### 2. Session Context for Implicit Filters

**Problem:** Users often have implicit context ("analyzing Q4 data", "focus on premium users") that applies to multiple queries but isn't repeated.

**Solution from claudontology:** Two-tier memory system with session-scoped context injection.

**Implementation approach:**

1. **Session context header:**
   ```
   X-Session-Context: {"time_focus": "Q4 2024", "user_segment": "premium", "exclude_test": true}
   ```

2. **Context model:**
   ```go
   type SessionContext struct {
       TimeFocus     string            `json:"time_focus,omitempty"`
       UserSegment   string            `json:"user_segment,omitempty"`
       ExcludeTest   bool              `json:"exclude_test,omitempty"`
       CustomFilters map[string]string `json:"custom_filters,omitempty"`
   }
   ```

3. **Automatic filter application in prompt:**
   ```go
   // In buildPrompt(), add Section 4.5: Session Context
   if sessionCtx := getSessionContext(ctx); sessionCtx != nil {
       prompt.WriteString("# Current Analysis Context\n")
       prompt.WriteString(fmt.Sprintf("- Time focus: %s\n", sessionCtx.TimeFocus))
       prompt.WriteString(fmt.Sprintf("- User segment: %s\n", sessionCtx.UserSegment))
       prompt.WriteString("Apply these filters to all queries unless explicitly overridden.\n\n")
   }
   ```

### 3. Ambiguity Detection with Confidence Thresholds

**Problem:** Vague terms like "recent", "active", "top", "high-performing" mean different things to different users. LLMs guess, often incorrectly.

**Solution from claudontology:** Detect ambiguous terms before SQL generation and either ask for clarification or auto-resolve using knowledge hierarchy.

**Ambiguity categories:**

| Category | Examples | Resolution Strategy |
|----------|----------|---------------------|
| Temporal | "recent", "latest", "current" | Check session context → project knowledge → ask |
| Quantitative | "top", "best", "high" | Check if metric specified → ask for threshold |
| State | "active", "valid", "good" | Check ontology column definitions → ask |
| Aggregation | "total", "average", "typical" | Usually safe to infer from context |

**Resolution hierarchy (from claudontology):**
1. Session context (highest priority)
2. Project knowledge (`engine_project_knowledge`)
3. Ontology column definitions (semantic types, descriptions)
4. Statistical inference (data patterns)
5. Ask user (lowest priority, most accurate)

**Confidence thresholds:**
- Confidence <0.7 → Stop and ask for clarification
- Confidence 0.7-0.9 → Proceed with stated assumption
- Confidence ≥0.9 → Auto-resolve using knowledge

**Response format when clarification needed:**
```json
{
    "needs_clarification": true,
    "clarifications": [
        {
            "term": "active",
            "question": "How do you define 'active' users?",
            "suggestions": [
                "Logged in within last 30 days",
                "Made a purchase within last 90 days",
                "Has non-zero session count"
            ]
        }
    ]
}
```

### 4. Ontology-Enhanced Schema Context

**Current gap:** The plan uses raw schema (table names, column types) for context. But ekaya-engine already has a rich 3-tier ontology with business semantics.

**Enhancement:** Leverage existing ontology tiers for better SQL generation.

**Ontology tiers to use:**

1. **Tier 0 - Domain Summary:**
   - Include domain context in system prompt
   - Helps LLM understand business domain (e.g., "e-commerce platform" vs. "healthcare system")

2. **Tier 1 - Entity Summaries:**
   - Use business names instead of technical table names
   - Include table descriptions in schema linking results
   - Use domain classifications for better table selection

3. **Tier 2 - Column Details:**
   - Use semantic types (dimension, measure, identifier) to guide aggregation
   - Use column descriptions for ambiguous terms
   - Use enum values for validation

**Schema indexing enhancement:**
```go
func (si *SchemaIndexer) buildTableDescription(table models.Table, columns []models.Column, ontology *models.Ontology) string {
    var desc strings.Builder

    // Use ontology business name if available
    entitySummary := ontology.GetEntitySummary(table.Name)
    if entitySummary != nil {
        desc.WriteString(fmt.Sprintf("Table: %s (Business name: %s)\n", table.Name, entitySummary.BusinessName))
        desc.WriteString(fmt.Sprintf("Description: %s\n", entitySummary.Description))
        desc.WriteString(fmt.Sprintf("Domain: %s\n", entitySummary.DomainClassification))
    } else {
        desc.WriteString(fmt.Sprintf("Table: %s\n", table.Name))
    }

    // Include column semantic types
    for _, col := range columns {
        colDetail := ontology.GetColumnDetail(table.Name, col.Name)
        if colDetail != nil {
            desc.WriteString(fmt.Sprintf("  - %s (%s, %s): %s\n",
                col.Name, col.DataType, colDetail.SemanticType, colDetail.Description))
        } else {
            desc.WriteString(fmt.Sprintf("  - %s (%s)\n", col.Name, col.DataType))
        }
    }

    return desc.String()
}
```

**Semantic type hints for aggregation:**
- Columns with `semantic_type: "measure"` → suggest SUM, AVG, COUNT
- Columns with `semantic_type: "dimension"` → suggest GROUP BY
- Columns with `semantic_type: "identifier"` → suggest JOINs

### 5. Updated Text2SQL Flow with Enhancements

```
User Question
    ↓
Ambiguity Detection (NEW) ←── If confidence < 0.7, return clarification request
    ↓
Pattern Cache Check (ENHANCED)
    ↓ If high confidence match (≥0.85), return cached SQL
    ↓ Otherwise, continue...
Schema Linking (with ontology enhancement)
    ↓
Few-Shot Retrieval (with multi-dimensional similarity)
    ↓
Build Prompt (with session context + domain summary)
    ↓
LLM Generation
    ↓
SQL Extraction
    ↓
Execute & Record Success/Failure → Update pattern confidence
```

### Summary of claudontology Enhancements

The claudontology proof-of-concept validated four key improvements to text2SQL systems:

1. **Smarter Pattern Matching** - Multi-dimensional similarity (not just embeddings) + confidence learning reduces unnecessary LLM calls and improves accuracy over time.

2. **Ambiguity Detection** - Identifying vague terms ("recent", "active", "high") before SQL generation prevents incorrect guesses and improves user trust.

3. **Session Context** - Implicit filters that apply to multiple queries reduce repetition and improve user experience.

4. **Ontology Leverage** - Using existing business semantics (table descriptions, semantic types) produces better SQL than raw schema alone.

**Implementation priority:** Pattern matching (#1) has highest ROI (performance + accuracy). Ambiguity detection (#2) has highest UX impact. Session context (#3) and ontology (#4) are incremental improvements.


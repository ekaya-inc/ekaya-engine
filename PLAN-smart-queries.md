# PLAN: LLM-Powered Smart Queries

**Status:** Future enhancement - requires LLM integration
**Current State:** Manual Pre-Approved Queries system works WITHOUT LLM (v0.0.1)

## Problem Statement

Manual query creation has friction points that LLM assistance can solve:

1. **Parameter validation is brittle** - Parameters inside string literals (`'hi {{name}}'`) fail at runtime, not authoring time
2. **PostgreSQL type inference** - Expressions like `$1 * $2` require explicit casts (`$1::decimal * $2::decimal`)
3. **Natural language → SQL gap** - Admin knows what they want, but writing correct SQL takes iteration
4. **Error messages are cryptic** - PostgreSQL errors require SQL expertise to interpret

## LLM-Powered Features

### 1. Generation Features

**Natural Language → SQL Query**
- Admin types prompt: "Show me all orders from last week with total > $100"
- LLM uses ontology context (`engine_ontologies.entity_summaries`, `column_details`) to generate accurate SQL
- LLM infers correct table names, column names, joins from ontology
- Generated query appears in SQL editor for Admin review/edit

**SQL Query → Natural Language Description**
- Admin pastes complex SQL into editor
- LLM analyzes query and generates plain English description
- Description auto-populates `natural_language_prompt` field

**Context-Aware Generation**
- LLM has access to:
  - `engine_ontologies` (entity summaries, column details)
  - `engine_project_knowledge` (business rules, terminology)
  - `engine_datasources.schema_cache` (table structure)
  - Existing queries (`engine_queries`) for pattern learning

### 2. Validation & Error Recovery

**Real-Time SQL Validation**
- As Admin types SQL, LLM validates in background
- Catches common issues BEFORE test execution:
  - Parameters inside string literals
  - Type casting requirements
  - Missing table aliases
  - Invalid column references

**Intelligent Error Fixing**
- When [Test Query] fails with PostgreSQL error
- LLM receives: failed SQL + error message + parameter types (`engine_queries.parameters`)
- LLM suggests fix with explanation
- Admin can accept suggestion or manually edit
- If accepted, LLM auto-retries [Test Query]

**Error Translation**
- PostgreSQL error: `operator does not exist: numeric * text`
- LLM translates: "The parameter 'quantity' needs to be a number, not text. Change its type to 'integer' or add a cast."

### 3. Learning & Feedback Loop

**Correction Learning**
- When Admin changes LLM-generated SQL:
  - Capture diff between LLM version and final version
  - Store as knowledge entry in `engine_project_knowledge`:
    - `fact_type: "query_correction"`
    - `key: <hash of original prompt>`
    - `value: <correction details>`
    - `context: <original SQL + corrected SQL>`
- LLM uses corrections to improve future suggestions

**Pattern Discovery**
- When Admin creates query manually (not from LLM):
  - Extract patterns: common joins, filters, parameter usage
  - Store as `fact_type: "query_pattern"` in `engine_project_knowledge`
- LLM learns project-specific SQL idioms

**Ontology Updates from Queries**
- If Admin's working query references relationships not in ontology:
  - Suggest ontology update via `engine_ontology_questions`
  - Example: Query joins `orders.customer_id = customers.id` but ontology missing this relationship
  - LLM generates question: "Should I record that orders.customer_id references customers.id?"

### 4. UX Considerations

**Graceful Degradation**
- All LLM features are OPTIONAL enhancements
- Without LLM configured: current manual workflow works
- With LLM configured: smart features appear as progressive enhancement

**LLM Availability Checks**
- Before showing "Generate from Prompt" button, check if project has LLM configured
- If no LLM: show tooltip explaining LLM requirement + link to setup

**Confidence Indicators**
- LLM-generated queries show confidence score
- High confidence (>90%): "Ready to test"
- Medium confidence (70-90%): "Review suggested query carefully"
- Low confidence (<70%): "Starting point - likely needs edits"

**User Control**
- Admin ALWAYS reviews before saving
- LLM suggestions are assistive, not authoritative
- [Test Query] button validates before enabling [Save]

## Technical Implementation

### Data Model Extensions

**New columns for `engine_queries`:**
```sql
-- Track LLM involvement
llm_generated BOOLEAN DEFAULT false           -- Was this query LLM-generated?
llm_confidence DECIMAL(5,2)                   -- Confidence score 0.00-100.00
llm_model VARCHAR(50)                         -- Which model generated it
generation_prompt TEXT                        -- Original NL prompt (if applicable)

-- Learning from corrections
correction_count INTEGER DEFAULT 0            -- How many times Admin corrected LLM
last_correction_at TIMESTAMPTZ               -- When last corrected
```

**New table: `engine_query_corrections`**
```sql
CREATE TABLE engine_query_corrections (
    id UUID PRIMARY KEY,
    project_id UUID REFERENCES engine_projects(id),
    query_id UUID REFERENCES engine_queries(id),

    -- What LLM suggested vs what Admin saved
    llm_version TEXT NOT NULL,
    final_version TEXT NOT NULL,

    -- Context for learning
    original_prompt TEXT,
    correction_reason TEXT,  -- Optional: why Admin changed it

    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Service Layer

**New service: `pkg/services/query_intelligence.go`**
```go
type QueryIntelligenceService interface {
    // Generation
    GenerateFromPrompt(ctx context.Context, projectID uuid.UUID, prompt string) (*GeneratedQuery, error)
    GenerateDescription(ctx context.Context, projectID uuid.UUID, sql string) (string, error)

    // Validation
    ValidateSQL(ctx context.Context, projectID uuid.UUID, sql string, params []QueryParameter) (*ValidationResult, error)

    // Error recovery
    SuggestFix(ctx context.Context, projectID uuid.UUID, sql string, errorMsg string, params []QueryParameter) (*FixSuggestion, error)

    // Learning
    RecordCorrection(ctx context.Context, queryID uuid.UUID, llmVersion, finalVersion string) error
    LearnFromQuery(ctx context.Context, query *Query) error
}
```

### LLM Context Assembly

**For query generation, LLM receives:**
1. Ontology summaries from `engine_ontologies` (active version only)
2. Project knowledge from `engine_project_knowledge`
3. Sample queries from `engine_queries` (successful, high-usage ones)
4. Schema structure from `engine_datasources.schema_cache`

**Context size management:**
- Tier 1: Entity summaries only (compact)
- Tier 2: + Column details for relevant tables
- Tier 3: + Sample queries
- Use progressive loading based on LLM context window

### API Endpoints

**New endpoints:**
```
POST /api/projects/{pid}/queries/generate
  Body: { "prompt": "...", "datasource_id": "..." }
  Response: { "sql": "...", "description": "...", "confidence": 85.5, "parameters": [...] }

POST /api/projects/{pid}/queries/describe
  Body: { "sql": "..." }
  Response: { "description": "..." }

POST /api/projects/{pid}/queries/suggest-fix
  Body: { "sql": "...", "error": "...", "parameters": [...] }
  Response: { "suggested_sql": "...", "explanation": "...", "changes": [...] }
```

### UI Components

**New UI elements in Pre-Approved Queries screen:**
- "Generate from Prompt" button (when LLM available)
- "Get Description" button (analyzes SQL in editor)
- Real-time validation indicator (green checkmark / yellow warning / red error)
- Error explanation panel (when test fails)
- "Apply Suggested Fix" button (when LLM suggests correction)
- Confidence badge on LLM-generated queries

## Success Metrics

**Adoption:**
- % of queries created via LLM generation vs manual
- % of queries with LLM-suggested fixes accepted

**Quality:**
- Error rate: LLM-generated queries vs manual queries
- Correction rate: How often Admin edits LLM suggestions
- Time saved: Compare authoring time with/without LLM

**Learning:**
- Number of corrections stored in knowledge base
- Improvement in confidence scores over time
- Reduction in correction rate as LLM learns

## Open Questions

1. **When to trigger validation?** On every keystroke? On blur? Manual button?
2. **How to handle multi-datasource projects?** Each datasource has different schema
3. **What if LLM suggests query for wrong datasource?** Need datasource scoping
4. **How to version the knowledge base?** Ontology changes might invalidate old patterns
5. **Should we suggest parameter extraction?** LLM detects hardcoded values and suggests parameterization

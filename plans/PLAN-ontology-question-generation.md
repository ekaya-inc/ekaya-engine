# PLAN: Ontology Question Generation During Extraction

## Problem Statement

The `engine_ontology_questions` table and MCP tools (`list_ontology_questions`, `resolve_ontology_question`, `skip_ontology_question`, `dismiss_ontology_question`, `escalate_ontology_question`) exist, but no questions are ever generated. The ontology extraction DAG runs to completion without surfacing ambiguities for user clarification.

A good ontology extraction should identify areas of uncertainty and generate questions for the domain expert to answer, improving accuracy over time.

## Current State

- **Table exists:** `engine_ontology_questions` with columns for question text, category, priority, context, status
- **MCP tools exist:** Full CRUD workflow for questions
- **No generation:** DAG steps complete without emitting questions
- **Result:** `list_ontology_questions` always returns empty

## Question Categories

Based on the table schema and typical ontology ambiguities:

| Category | Description | Example |
|----------|-------------|---------|
| `terminology` | Domain-specific terms needing clarification | "What does 'tik' mean in billing_engagements.tiks_count?" |
| `enumeration` | Unknown enum values needing labels | "What do status values 'A', 'P', 'C' represent in billing_transactions.status?" |
| `relationship` | Ambiguous or missing relationships | "Is users.referrer_id a self-referential relationship to users.user_id?" |
| `business_rules` | Implicit business logic | "Can a user be both a host and visitor in the same engagement?" |
| `temporal` | Time-based semantics | "Does deleted_at=NULL mean active, or does it require additional status checks?" |
| `data_quality` | Potential data issues | "Column users.phone has 80% NULL - is this expected or a data quality issue?" |

## Generation Points in DAG

### 1. EntityDiscovery Step

**Generate questions when:**
- Table name is ambiguous (e.g., `tbl_1`, `data`, `temp`)
- Table has no clear primary entity pattern
- Multiple tables could represent the same entity

**Example:**
```json
{
  "category": "terminology",
  "priority": 2,
  "question": "What business entity does the 'e_servers' table represent?",
  "context": {
    "table": "e_servers",
    "columns": ["id", "capacity", "current_participants", "created_at"],
    "inference": "Appears to be engagement/media servers but name is abbreviated"
  }
}
```

### 2. EntityEnrichment Step

**Generate questions when:**
- LLM confidence is below threshold for entity description
- Entity purpose is unclear from schema alone
- Domain-specific terminology detected

**Example:**
```json
{
  "category": "terminology",
  "priority": 1,
  "question": "What is a 'tik' in the context of billing engagements?",
  "context": {
    "table": "billing_engagements",
    "column": "tiks_count",
    "related_columns": ["tikr_share", "host_tiks", "visitor_tiks"],
    "inference": "Appears to be a unit of measurement for engagement duration or value"
  }
}
```

### 3. ColumnEnrichment Step

**Generate questions when:**
- Enum column has cryptic values (single letters, numbers, codes)
- Column purpose unclear from name
- Potential business rule encoded in column

**Example:**
```json
{
  "category": "enumeration",
  "priority": 1,
  "question": "What do the status values represent in billing_transactions?",
  "context": {
    "table": "billing_transactions",
    "column": "status",
    "distinct_values": ["pending", "completed", "failed", "refunded", "disputed"],
    "sample_counts": {"completed": 45000, "pending": 1200, "failed": 300}
  }
}
```

### 4. FKDiscovery / PKMatchDiscovery Steps

**Generate questions when:**
- Ambiguous FK target (multiple possible parent tables)
- Self-referential relationship detected
- Column naming suggests relationship but no FK exists

**Example:**
```json
{
  "category": "relationship",
  "priority": 2,
  "question": "Does users.referred_by_user_id reference users.user_id (self-referential) or a different user identifier?",
  "context": {
    "source_table": "users",
    "source_column": "referred_by_user_id",
    "candidate_targets": ["users.user_id", "users.account_id"],
    "inference": "Likely self-referential for referral tracking"
  }
}
```

### 5. RelationshipEnrichment Step

**Generate questions when:**
- Relationship cardinality is ambiguous
- Relationship semantics unclear (owns vs. references vs. contains)
- Bidirectional relationship detected

**Example:**
```json
{
  "category": "relationship",
  "priority": 3,
  "question": "What is the business meaning of the relationship between Account and User?",
  "context": {
    "from_entity": "Account",
    "to_entity": "User",
    "columns": "accounts.default_user_id -> users.user_id",
    "cardinality": "1:1",
    "inference": "Account has a default user, but unclear if user can belong to multiple accounts"
  }
}
```

## Implementation Steps

### Step 1: Add Question Generation to LLM Prompts

Modify the LLM prompts in each DAG step to request questions alongside inferences:

```go
// pkg/services/ontology/prompts/entity_enrichment.go

const EntityEnrichmentPrompt = `
... existing prompt ...

Additionally, identify any areas of uncertainty where user clarification would improve accuracy.
For each uncertainty, provide:
- category: terminology | enumeration | relationship | business_rules | temporal | data_quality
- priority: 1 (critical) | 2 (important) | 3 (nice-to-have)
- question: A clear question for the domain expert
- context: Relevant schema/data context

Return questions in the "questions" array of the response.
`
```

### Step 2: Update LLM Response Parsing

Each DAG step's response parser needs to extract questions:

```go
type EnrichmentResponse struct {
    // ... existing fields ...
    Questions []OntologyQuestion `json:"questions,omitempty"`
}

type OntologyQuestion struct {
    Category string          `json:"category"`
    Priority int             `json:"priority"`
    Question string          `json:"question"`
    Context  json.RawMessage `json:"context"`
}
```

### Step 3: Create Question Repository Method

```go
// pkg/repositories/ontology_repository.go

func (r *OntologyRepository) CreateQuestions(ctx context.Context, projectID string, questions []OntologyQuestion) error {
    // Batch insert questions
    // Deduplicate by question text hash to avoid duplicates across runs
}
```

### Step 4: Wire Up DAG Steps

Each step executor calls the repository after processing:

```go
// pkg/services/ontology/dag/entity_enrichment.go

func (s *EntityEnrichmentStep) Execute(ctx context.Context) error {
    // ... existing enrichment logic ...

    // Extract and store questions from LLM response
    if len(response.Questions) > 0 {
        if err := s.repo.CreateQuestions(ctx, s.projectID, response.Questions); err != nil {
            s.logger.Error("failed to store ontology questions", "error", err)
            // Non-fatal: continue even if question storage fails
        }
    }

    return nil
}
```

### Step 5: Add Deterministic Question Generation

Some questions can be generated without LLM based on data patterns:

```go
// pkg/services/ontology/questions/deterministic.go

func GenerateDeterministicQuestions(schema *Schema, stats *ColumnStats) []OntologyQuestion {
    var questions []OntologyQuestion

    // High NULL rate columns
    for _, col := range stats.Columns {
        if col.NullRate > 0.8 && !isKnownOptionalColumn(col.Name) {
            questions = append(questions, OntologyQuestion{
                Category: "data_quality",
                Priority: 3,
                Question: fmt.Sprintf("Column %s.%s has %.0f%% NULL values - is this expected?",
                    col.Table, col.Name, col.NullRate*100),
                Context: map[string]any{"null_rate": col.NullRate, "row_count": col.RowCount},
            })
        }
    }

    // Single-letter enum values
    for _, col := range stats.EnumColumns {
        if hasObscureValues(col.DistinctValues) {
            questions = append(questions, OntologyQuestion{
                Category: "enumeration",
                Priority: 1,
                Question: fmt.Sprintf("What do the values %v represent in %s.%s?",
                    col.DistinctValues, col.Table, col.Name),
            })
        }
    }

    return questions
}
```

### Step 6: Question Deduplication

Prevent duplicate questions across extraction runs:

```go
// Use content hash for deduplication
func (q *OntologyQuestion) ContentHash() string {
    h := sha256.New()
    h.Write([]byte(q.Category + "|" + q.Question))
    return hex.EncodeToString(h.Sum(nil))[:16]
}

// In repository: INSERT ... ON CONFLICT (project_id, content_hash) DO NOTHING
```

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/ontology/prompts/*.go` | Add question generation to prompts |
| `pkg/services/ontology/dag/*.go` | Parse and store questions from LLM responses |
| `pkg/repositories/ontology_repository.go` | Add `CreateQuestions` method |
| `pkg/services/ontology/questions/deterministic.go` | New file for rule-based questions |
| `migrations/XXXX_add_question_content_hash.sql` | Add content_hash column for deduplication |

## Question Lifecycle

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│    pending      │────>│    answered     │     │    dismissed    │
│  (generated)    │     │  (resolved)     │     │  (not relevant) │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                                               ▲
        │               ┌─────────────────┐             │
        └──────────────>│    skipped      │─────────────┘
                        │  (for later)    │
                        └─────────────────┘
                                │
                                ▼
                        ┌─────────────────┐
                        │   escalated     │
                        │ (needs human)   │
                        └─────────────────┘
```

## Success Criteria

1. After ontology extraction, `list_ontology_questions(status='pending')` returns relevant questions
2. Questions have appropriate categories and priorities
3. No duplicate questions across re-extractions
4. Questions include actionable context for the domain expert
5. Resolving a question via MCP updates the ontology (integration with `update_entity`, `update_column`, etc.)

## Open Questions

1. **Question limits:** Should we cap questions per extraction (e.g., max 50) to avoid overwhelming users?
2. **Auto-dismiss:** Should questions be auto-dismissed if the ontology is re-extracted and the ambiguity no longer exists?
3. **Priority tuning:** How do we calibrate priority levels based on user feedback?
4. **Integration:** When a question is resolved, should it automatically trigger an ontology refresh for the affected entity/column?

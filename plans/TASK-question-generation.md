# TASK: Question Generation During Extraction

**Priority:** 3 (High)
**Status:** Not Started
**Parent:** PLAN-ontology-next.md
**Design Reference:** PLAN-ontology-question-generation.md (archived)

## Overview

The `engine_ontology_questions` table and MCP tools exist, but no questions are ever generated. The ontology extraction DAG runs to completion without surfacing ambiguities for user clarification.

## Goal

Generate clarifying questions during ontology extraction so domain experts can improve accuracy.

## Question Categories

| Category | Description | Example |
|----------|-------------|---------|
| `terminology` | Domain-specific terms | "What does 'tik' mean in tiks_count?" |
| `enumeration` | Unknown enum values | "What do status values 'A', 'P', 'C' represent?" |
| `relationship` | Ambiguous relationships | "Is users.referrer_id a self-reference?" |
| `business_rules` | Implicit business logic | "Can a user be both host and visitor?" |
| `temporal` | Time-based semantics | "Does deleted_at=NULL mean active?" |
| `data_quality` | Potential issues | "Column phone has 80% NULL - expected?" |

## Implementation

### Step 1: Modify LLM Prompts to Request Questions

Add to entity/column/relationship enrichment prompts:

```
Additionally, identify any areas of uncertainty where user clarification would improve accuracy.
For each uncertainty, provide:
- category: terminology | enumeration | relationship | business_rules | temporal | data_quality
- priority: 1 (critical) | 2 (important) | 3 (nice-to-have)
- question: A clear question for the domain expert
- context: Relevant schema/data context

Return questions in the "questions" array of the response.
```

### Step 2: Update LLM Response Parsing

Add question extraction to response structs:

```go
type EnrichmentResponse struct {
    // ... existing fields ...
    Questions []OntologyQuestionInput `json:"questions,omitempty"`
}

type OntologyQuestionInput struct {
    Category string          `json:"category"`
    Priority int             `json:"priority"`
    Question string          `json:"question"`
    Context  json.RawMessage `json:"context"`
}
```

### Step 3: Wire Up Question Storage in DAG Steps

After each DAG step's LLM processing:

```go
// Extract and store questions from LLM response
if len(response.Questions) > 0 {
    questions := convertToModels(response.Questions, projectID, workflowID)
    if err := s.questionService.CreateQuestions(ctx, questions); err != nil {
        s.logger.Error("failed to store ontology questions", "error", err)
        // Non-fatal: continue even if question storage fails
    }
}
```

### Step 4: Add Deterministic Question Generation

Generate questions without LLM based on data patterns:

```go
func GenerateDeterministicQuestions(stats *ColumnStats) []*models.OntologyQuestion {
    var questions []*models.OntologyQuestion

    // High NULL rate columns (>80%)
    for _, col := range stats.Columns {
        if col.NullRate > 0.8 && !isKnownOptionalColumn(col.Name) {
            questions = append(questions, &models.OntologyQuestion{
                Category: "data_quality",
                Priority: 3,
                Question: fmt.Sprintf("Column %s.%s has %.0f%% NULL values - is this expected?",
                    col.Table, col.Name, col.NullRate*100),
            })
        }
    }

    // Cryptic enum values (single letters, numbers)
    for _, col := range stats.EnumColumns {
        if hasObscureValues(col.DistinctValues) {
            questions = append(questions, &models.OntologyQuestion{
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

### Step 5: Question Deduplication

Prevent duplicate questions across extraction runs:

```go
func (q *OntologyQuestion) ContentHash() string {
    h := sha256.New()
    h.Write([]byte(q.Category + "|" + q.Question))
    return hex.EncodeToString(h.Sum(nil))[:16]
}

// Add content_hash column and unique constraint
```

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/entity_enrichment.go` | Add questions to prompt, parse response |
| `pkg/services/column_enrichment.go` | Add questions to prompt, parse response |
| `pkg/services/relationship_enrichment.go` | Add questions to prompt, parse response |
| `pkg/services/deterministic_question_generation.go` | New file for pattern-based questions |
| `migrations/0XX_question_content_hash.sql` | Add content_hash for deduplication |

## Testing

1. Run ontology extraction
2. Verify `list_ontology_questions(status='pending')` returns questions
3. Verify questions have appropriate categories and priorities
4. Re-extract and verify no duplicate questions

## Success Criteria

- [ ] DAG generates at least some questions per extraction
- [ ] Questions have appropriate categories and priorities
- [ ] No duplicate questions across re-extractions
- [ ] Questions include actionable context

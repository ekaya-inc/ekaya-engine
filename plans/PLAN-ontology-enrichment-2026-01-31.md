# PLAN: Ontology Enrichment Improvements - 2026-01-31

This plan consolidates remaining work to reduce ontology questions and improve auto-inference, based on Benchmark #3 results from tikr_production.

## Context

**Benchmark Results (tikr_production):**
- Questions generated: 124 (target: <100)
- Duplicate questions: ~8% (target: <5%)
- Required code research: 20% (target achieved)
- Glossary success rate: 100% (target achieved)
- Extractor auto-inference: 6/10 (target: 9/10)

**Key Finding:** The extractor generates ~24 questions that could be auto-inferred from data patterns without LLM or human input.

---

## Task 1: Implement Soft-Delete Auto-Detection

**Priority:** P0
**Impact:** Eliminates ~17 questions per extraction
**Files:**
- `pkg/services/column_feature_extraction.go`
- `pkg/services/deterministic_question_generation.go`

### Problem
The extractor generates questions like "What does NULL in deleted_at mean?" for every table with a `deleted_at` column, despite this being a well-known GORM soft-delete pattern.

### Solution
In Phase 5 (Cross-Column Analysis), detect soft-delete pattern:

```go
// Soft-delete detection criteria:
// 1. Column name matches: deleted_at, deletedAt, DeletedAt
// 2. Type: timestamp with time zone OR timestamp
// 3. NULL rate > 90% (most records are active)
// 4. No non-NULL values in future (timestamps are in past)

func (s *ColumnFeatureService) detectSoftDeletePattern(col ColumnInfo, stats ColumnStats) bool {
    nameMatch := regexp.MustCompile(`(?i)^deleted_?at$`).MatchString(col.Name)
    isTimestamp := strings.Contains(strings.ToLower(col.DataType), "timestamp")
    highNullRate := stats.NullRate > 0.90

    return nameMatch && isTimestamp && highNullRate
}
```

When detected:
1. Set `ColumnFeatures.Purpose = "soft_delete"`
2. Set `ColumnFeatures.Description = "GORM soft-delete pattern: NULL=active, timestamp=deleted"`
3. Skip question generation for this pattern in `deterministic_question_generation.go`

### Acceptance Criteria
- [ ] Soft-delete columns auto-documented without questions
- [ ] tikr_production extraction generates 0 "deleted_at NULL" questions
- [ ] Existing manually-answered soft-delete knowledge preserved

---

## Task 2: Auto-Detect Rating Scales

**Priority:** P1
**Impact:** Eliminates ~5 questions per extraction
**Files:**
- `pkg/services/column_feature_extraction.go`
- `pkg/services/enum_value_analysis.go`

### Problem
Questions like "What is the range of values for reviewee_rating?" when data shows only values 1-5.

### Solution
In Phase 3 (Enum Value Analysis), detect rating patterns:

```go
// Rating scale detection criteria:
// 1. Column name contains: rating, score, quality, stars
// 2. Data type: integer or bigint
// 3. Values are consecutive integers in a small range
// 4. Common ranges: 1-5 (5-star), 1-10 (10-point), 0-100 (percentage)

func detectRatingScale(values []int) *RatingScale {
    min, max := minMax(values)

    switch {
    case min == 1 && max == 5:
        return &RatingScale{Min: 1, Max: 5, Description: "5-star rating scale"}
    case min == 1 && max == 10:
        return &RatingScale{Min: 1, Max: 10, Description: "10-point rating scale"}
    case min == 0 && max == 100:
        return &RatingScale{Min: 0, Max: 100, Description: "Percentage score (0-100)"}
    default:
        return nil // Unknown scale, generate question
    }
}
```

When detected:
1. Set `ColumnFeatures.SemanticType = "rating"`
2. Set `ColumnFeatures.EnumFeatures.Scale = {min, max, description}`
3. Skip "what is the rating scale" questions

### Acceptance Criteria
- [ ] 1-5 rating columns auto-labeled without questions
- [ ] Rating scale documented in column metadata
- [ ] Non-standard scales still generate questions

---

## Task 3: Skip UUID Format Questions

**Priority:** P1
**Impact:** Eliminates ~4 questions per extraction
**Files:**
- `pkg/services/column_feature_extraction.go` (lines 119-133 - samplePatterns)

### Problem
Questions about UUID format when data clearly shows UUID v4 pattern.

### Current State
The `samplePatterns` map already detects UUID format via regex. However, questions still get generated.

### Solution
Ensure UUID detection in Phase 1 flows through to skip question generation:

```go
// In deterministic_question_generation.go
func shouldGenerateFormatQuestion(col ColumnInfo, features ColumnFeatures) bool {
    // Skip if sample pattern already identified format
    if features.DetectedPattern == "uuid" {
        return false // Already documented
    }
    // ... other checks
}
```

### Acceptance Criteria
- [ ] UUID columns skip "what format is this ID" questions
- [ ] `ColumnFeatures.DetectedPattern = "uuid"` set automatically
- [ ] Description auto-populated: "UUID v4 format identifier"

---

## Task 4: Batch Similar Pattern Questions

**Priority:** P1
**Impact:** Reduces duplicate-feeling questions
**Files:**
- `pkg/services/deterministic_question_generation.go`
- `pkg/models/ontology_question.go`

### Problem
17 separate questions about "deleted_at NULL" across different tables feel redundant, even though they're technically about different columns.

### Solution
Group questions by pattern and generate one representative question:

```go
type QuestionGroup struct {
    Pattern     string   // e.g., "soft_delete", "rating_scale", "uuid_format"
    Tables      []string // Tables with this pattern
    SampleColumn string  // Representative column for the question
    Question    string   // Single question covering all instances
}

// Instead of 17 questions:
// "What does NULL in accounts.deleted_at mean?"
// "What does NULL in users.deleted_at mean?"
// ...

// Generate ONE grouped question:
// "The following 17 tables have a 'deleted_at' column with >90% NULL values:
//  accounts, users, channels, ...
//  Is this a soft-delete pattern where NULL=active and timestamp=deleted?"
```

### Acceptance Criteria
- [ ] Similar pattern questions grouped into one
- [ ] Question text lists all affected tables
- [ ] Answering once applies to all grouped columns

---

## Task 5: Add Confidence Scoring to Inferences

**Priority:** P2
**Impact:** Reduces questions for high-confidence inferences
**Files:**
- `pkg/models/column_features.go`
- `pkg/services/column_feature_extraction.go`

### Problem
The extractor generates questions even when it has high confidence about the answer.

### Solution
Add confidence field to ColumnFeatures:

```go
type ColumnFeatures struct {
    // ... existing fields

    // Confidence in the inferred metadata (0.0 - 1.0)
    // >= 0.9: Auto-apply without question
    // 0.7 - 0.9: Apply but flag for review
    // < 0.7: Generate question
    InferenceConfidence float64 `json:"inference_confidence"`
    InferenceReasoning  string  `json:"inference_reasoning"`
}
```

Confidence scoring rules:
| Pattern | Confidence | Reasoning |
|---------|------------|-----------|
| deleted_at + timestamp + >95% NULL | 0.95 | GORM soft-delete pattern |
| *_id + UUID regex match | 0.90 | Standard UUID identifier |
| rating/score + integers 1-5 | 0.85 | Common 5-star scale |
| *_at + timestamp type | 0.80 | Timestamp naming convention |
| amount + currency pair found | 0.85 | Monetary value pattern |

### Acceptance Criteria
- [ ] High-confidence inferences (≥0.9) auto-applied
- [ ] Medium-confidence (0.7-0.9) applied with review flag
- [ ] Low-confidence (<0.7) generates question
- [ ] Reasoning stored for audit trail

---

## Task 6: Leverage Project Knowledge for Skip Logic

**Priority:** P2
**Impact:** Questions answered once never asked again
**Files:**
- `pkg/services/ontology_dag_service.go`
- `pkg/services/deterministic_question_generation.go`
- `pkg/repositories/project_knowledge_repository.go`

### Problem
If a user answers "Tik = 15 seconds" via MCP, the next extraction should not ask "What is a Tik?" again.

### Solution
Before generating questions, check project_knowledge for existing answers:

```go
func (s *QuestionService) generateQuestions(ctx context.Context, projectID string) ([]Question, error) {
    // Load existing project knowledge
    knowledge, err := s.knowledgeRepo.GetByProject(ctx, projectID)

    // Check if question topic already answered
    for _, candidate := range candidateQuestions {
        if s.isAlreadyAnswered(candidate, knowledge) {
            continue // Skip - already have this knowledge
        }
        questions = append(questions, candidate)
    }
    return questions, nil
}

func (s *QuestionService) isAlreadyAnswered(q Question, knowledge []ProjectKnowledge) bool {
    for _, k := range knowledge {
        if k.Category == "terminology" && strings.Contains(q.Text, k.Fact) {
            return true
        }
        // Pattern matching for other categories
    }
    return false
}
```

### Acceptance Criteria
- [ ] Terminology questions skipped if term in project_knowledge
- [ ] Business rule questions skipped if rule documented
- [ ] Re-extraction after Q&A generates fewer questions

---

## Task 7: Improve Enum Auto-Labeling

**Priority:** P2
**Impact:** Better enum value descriptions without questions
**Files:**
- `pkg/services/enum_value_analysis.go`
- `pkg/llm/prompts/enum_analysis.go`

### Problem
Enum values like `0, 1, 2, 3` generate questions even when proto files define them.

### Solution
Enhance Phase 3 with proto-aware enum detection:

1. Check if column name matches known proto enum patterns
2. If proto file exists in linked repo, extract enum definitions
3. Auto-map database values to proto labels

```go
// For tikr_production, common enums:
var knownEnums = map[string][]EnumValue{
    "transaction_state": {
        {Value: 0, Label: "UNSPECIFIED"},
        {Value: 1, Label: "STARTED"},
        {Value: 2, Label: "ENDED"},
        // ...
    },
    "invitation_status": {
        {Value: 0, Label: "RINGING"},
        {Value: 1, Label: "ACCEPTED"},
        // ...
    },
}
```

For now, use heuristics:
- 0 = UNSPECIFIED (common protobuf pattern)
- Boolean-like (0,1) = false/true or no/yes
- Status-like = lifecycle states

### Acceptance Criteria
- [ ] Proto-defined enums auto-labeled
- [ ] Common patterns (0=UNSPECIFIED) auto-detected
- [ ] Questions only for truly ambiguous enums

---

## Task 8: Document Monetary Column Pairs

**Priority:** P3
**Impact:** Eliminates "what is the currency" questions
**Files:**
- `pkg/services/column_feature_extraction.go` (Phase 5)

### Problem
Questions about monetary columns when amount + currency pairs are obvious.

### Current State
Phase 5 (Cross-Column Analysis) already detects monetary pairs. Ensure this flows to skip questions.

### Solution
When monetary pair detected:
1. Set `ColumnFeatures.SemanticType = "currency_cents"`
2. Set `ColumnFeatures.RelatedColumns = ["currency_column_name"]`
3. Auto-document: "Monetary amount in cents, paired with {currency_column}"

### Acceptance Criteria
- [ ] Amount columns paired with currency columns auto-documented
- [ ] No "what is the currency" questions for paired columns
- [ ] Unpaired monetary columns still generate questions

---

## Implementation Order

| Task | Priority | Effort | Impact (Questions Saved) |
|------|----------|--------|--------------------------|
| 1. Soft-Delete Auto-Detection | P0 | Medium | ~17 |
| 2. Rating Scale Detection | P1 | Small | ~5 |
| 3. UUID Format Skip | P1 | Small | ~4 |
| 4. Batch Similar Questions | P1 | Medium | Reduces perceived duplicates |
| 5. Confidence Scoring | P2 | Large | Foundation for all skips |
| 6. Project Knowledge Skip | P2 | Medium | ~10 on re-extraction |
| 7. Enum Auto-Labeling | P2 | Medium | ~8 |
| 8. Monetary Pair Docs | P3 | Small | ~3 |

**Total Estimated Impact:** Reduce questions from 124 to <80

---

## Success Metrics

After implementing all tasks:

| Metric | Current | Target |
|--------|---------|--------|
| Questions generated | 124 | <80 |
| Duplicate questions | ~8% | <3% |
| Extractor auto-inference | 6/10 | 8/10 |
| Questions requiring code research | 20% | <15% |

---

## Related Files

| File | Purpose |
|------|---------|
| `PLAN-extracting-column-features.md` | Phase 1-6 architecture (reference) |
| `PLAN-ontology-next.md` | Current blockers and bugs |
| `PLAN-structured-question-metadata.md` | Answer schema design |
| `tikr-all/plans/BENCHMARK-ontology-enrichment-*.md` | Benchmark results |

---

## Testing Strategy

1. **Unit Tests:** Each detection function with edge cases
2. **Integration Test:** Run extraction on tikr_production
3. **Regression Test:** Ensure no valid questions are skipped
4. **Benchmark:** Compare question count before/after

Test data:
- tikr_production (production complexity)
- Test datasource with known patterns
- Edge cases: unusual soft-delete names, non-standard rating scales

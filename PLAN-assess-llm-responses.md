# Plan: assess-llm-responses

## Overview

Create a new deterministic assessment tool that evaluates LLM response quality during ontology extraction using ONLY deterministic checks (no LLM-as-judge). This tool assesses how well the LLM performed, not the code quality.

**Key Principle:** Use the same level of rigor as `assess-deterministic` (7 phases, weighted scoring, 100 achievable) but focus on LLM RESPONSE quality rather than code quality.

**Target Model:** Haiku 4.5 as baseline model being tested.

## Distinction from Existing Tools

| Tool | Purpose | Method | Score 100 Means |
|------|---------|--------|-----------------|
| **assess-deterministic** | Evaluates CODE quality (input prep + post-processing) | Deterministic checks only | Code is perfect |
| **assess-extraction** | Evaluates LLM output quality | LLM-as-judge (subjective) | Model did perfect job (subjective) |
| **assess-llm-responses** (NEW) | Evaluates LLM response quality | Deterministic checks only | Model produced perfect responses (objective) |

## Data Source

From `engine_llm_conversations` table (From `migrations/007_ontology.up.sql:193`):
- `id`, `project_id`
- `model`, `endpoint`
- `request_messages` (JSONB)
- `response_content` (TEXT)
- `prompt_tokens`, `completion_tokens`, `total_tokens`
- `duration_ms`
- `status` (success/failure)
- `error_message`
- `created_at`

## Assessment Philosophy

Deterministic LLM response checks focus on:
1. **Structural validity** - Is JSON parseable and well-formed?
2. **Schema compliance** - Does response match expected structure for prompt type?
3. **Hallucination detection** - Do referenced entities exist in actual schema? (deterministic check against `engine_schema_tables`/`engine_schema_columns`)
4. **Completeness** - Are all required fields present?
5. **Value validation** - Are enum values valid? Priority 1-5? Domains non-empty?
6. **Token efficiency** - Reasonable token usage for task complexity
7. **Error rate** - Percentage of failed conversations

**NOT assessed (these are subjective):**
- Quality of business names or descriptions (this is LLM-as-judge territory)
- Appropriateness of domain groupings
- Quality of reasoning or questions

## 7-Phase Structure

### Phase 1: Data Loading ✅
Load all data needed for assessment:
- LLM conversations from `engine_llm_conversations`
- Schema tables and columns from `engine_schema_tables`/`engine_schema_columns`
- Ontology from `engine_ontologies`
- Questions from `engine_ontology_questions`
- Prompt type detection (reuse logic from `assess-deterministic`)

### Phase 2: Prompt Type Detection ✅
Classify each conversation by prompt type (reuse from `assess-deterministic`):
- `entity_analysis` - Single table analysis
- `tier1_batch` - Multiple table batch
- `tier0_domain` - Domain summary
- `description_processing` - User description processing
- `unknown` - Unable to classify

### Phase 3: Per-Response Structural Checks ✅
For EACH conversation, check:

**3.1 JSON Parsing (20 points)**
- Is `response_content` valid JSON?
- Penalty: -20 if unparseable

**3.2 Response Status (10 points)**
- Is `status = 'success'`?
- Penalty: -10 if failed

**3.3 Completeness Check (20 points)**
- Are required top-level fields present based on prompt type?
- For `entity_analysis`: `business_name`, `description`, `domain`, `key_columns`, `questions`
- For `tier1_batch`: `entity_summaries` (object)
- For `tier0_domain`: `domain_summary` (object)
- For `description_processing`: `entity_hints` (array)

**3.4 Field Type Validation (10 points)**
- Do fields match expected types (string, array, object)?
- Penalty: -2 per type mismatch (max -10)

### Phase 4: Hallucination Detection (CRITICAL) ✅
**Most important check** - Do referenced entities actually exist?

Build deterministic lookup maps:
```go
validTables := map[string]bool // from engine_schema_tables
validColumns := map[string]map[string]bool // table -> columns from engine_schema_columns
```

**4.1 Entity Analysis Responses (40 points max penalty)**
Check each `entity_analysis` response:
- Parse `key_columns` array
- Verify each column exists in `engine_schema_columns` for that table
- Penalty: -10 per hallucinated column (max -40)

**4.2 Tier1 Batch Responses (30 points max penalty)**
Check `entity_summaries` object:
- Verify each table name key exists in `engine_schema_tables`
- For each entity, verify `key_columns` exist
- Penalty: -10 per hallucinated table, -5 per hallucinated column (max -30)

**4.3 Question Source Validation (10 points max penalty)**
Check `engine_ontology_questions.source_entity_key`:
- Verify table exists in schema
- Penalty: -2 per invalid source (max -10)

### Phase 5: Value Validation ✅
**5.1 Required String Fields (10 points)**
Check non-empty strings:
- `business_name` not empty
- `description` not empty (min 10 chars)
- `domain` not empty
- Penalty: -3 per empty required field (max -10)

**5.2 Priority Values (10 points)**
Check `questions` array in responses and `engine_ontology_questions`:
- `priority` in range 1-5
- Penalty: -2 per invalid priority (max -10)

**5.3 Boolean Fields (5 points)**
Check `is_required` field in questions:
- Is boolean (not string or null)
- Penalty: -5 if wrong type

**5.4 Category Values (5 points)**
Check `category` field is non-empty when present:
- Penalty: -1 per missing category (max -5)

### Phase 6: Token Efficiency Metrics
Calculate aggregate metrics (informational, minimal scoring impact):

**6.1 Token Statistics**
- Total tokens across all conversations
- Avg tokens per conversation
- Max tokens in single conversation
- Tokens per table analyzed

**6.2 Efficiency Score (10 points)**
Compare to reasonable benchmarks:
- Entity analysis: 2000-8000 tokens expected
- Tier1 batch: 4000-15000 tokens expected
- Penalty: -5 if average > 2x expected (suggests prompt bloat or repeated failures)

### Phase 7: Aggregate Scoring and Summary

**7.1 Per-Conversation Scoring**
Each conversation gets score 0-100 based on:
- Structure: 40 points (JSON valid, status success, fields present, types correct)
- Hallucinations: 40 points (no hallucinated entities)
- Value validity: 20 points (enums, priorities, non-empty strings)

**7.2 Final Score Calculation**
Weighted average across categories:

```go
const (
    // Category weights (must sum to 100)
    ScoreStructureWeight      = 25  // JSON parsing, field presence
    ScoreHallucinationWeight  = 50  // Most critical - no hallucinations
    ScoreValueValidityWeight  = 15  // Priority ranges, non-empty strings
    ScoreErrorRateWeight      = 10  // Percentage of failed calls
)
```

Formula:
```
structureScore = avg(all conversation structure scores)
hallucinationScore = 100 - (totalHallucinations * penalty)
valueScore = avg(all value validation scores)
errorRate = (successfulCalls / totalCalls) * 100

finalScore = (structureScore * 25 + hallucinationScore * 50 +
              valueScore * 15 + errorRate * 10) / 100
```

**7.3 Smart Summary Generation**
One-liner highlighting top issues:
- "Score 100/100 - Perfect! No hallucinations, all responses valid."
- "Score 85/100 - 3 hallucinated columns in entity summaries"
- "Score 72/100 - 5 hallucinations, 12% missing required fields"

**7.4 Detailed Issue Reporting**
Per-category breakdown similar to `assess-deterministic`:
```json
{
  "checks_summary": {
    "structure": {
      "score": 95,
      "conversations_checked": 47,
      "conversations_passed": 45,
      "issues": ["2 responses had invalid JSON"]
    },
    "hallucinations": {
      "score": 70,
      "total_hallucinations": 8,
      "issues": [
        "offer.key_columns references 'user_id' but actual column is 'owner_id'",
        "tier1 response included non-existent table 'user_sessions'"
      ]
    },
    "value_validity": {
      "score": 88,
      "issues": ["3 questions with priority=0 (invalid)"]
    },
    "error_rate": {
      "score": 100,
      "successful": 47,
      "failed": 0
    }
  }
}
```

## Output Structure

```go
type AssessmentResult struct {
    CommitInfo         string                    `json:"commit_info"`
    DatasourceName     string                    `json:"datasource_name"`
    ProjectID          string                    `json:"project_id"`
    ModelUnderTest     string                    `json:"model_under_test"`

    // Phase 2: Detection
    PromptTypeCounts   map[PromptType]int        `json:"prompt_type_counts"`

    // Phase 3-5: Per-conversation assessments
    ConversationChecks []ConversationCheck       `json:"conversation_checks"`

    // Phase 4: Hallucination details
    HallucinationReport HallucinationReport      `json:"hallucination_report"`

    // Phase 6: Token metrics
    TokenMetrics       TokenMetrics              `json:"token_metrics"`

    // Phase 7: Final scoring
    ChecksSummary      ChecksSummary             `json:"checks_summary"`
    FinalScore         int                       `json:"final_score"`
    Summary            string                    `json:"summary"`
    SmartSummary       string                    `json:"smart_summary"`
}

type ConversationCheck struct {
    ConversationID     string      `json:"conversation_id"`
    PromptType         PromptType  `json:"prompt_type"`
    StructureScore     int         `json:"structure_score"`      // 0-100
    HallucinationScore int         `json:"hallucination_score"`  // 0-100
    ValueScore         int         `json:"value_score"`          // 0-100
    OverallScore       int         `json:"overall_score"`        // 0-100
    Issues             []string    `json:"issues"`
    Hallucinations     []string    `json:"hallucinations"`
}

type HallucinationReport struct {
    TotalHallucinations       int      `json:"total_hallucinations"`
    HallucinatedTables        int      `json:"hallucinated_tables"`
    HallucinatedColumns       int      `json:"hallucinated_columns"`
    HallucinatedSources       int      `json:"hallucinated_sources"`
    Examples                  []string `json:"examples"` // First 5
    Score                     int      `json:"score"`    // 0-100
}

type TokenMetrics struct {
    TotalConversations    int     `json:"total_conversations"`
    TotalTokens           int     `json:"total_tokens"`
    TotalPromptTokens     int     `json:"total_prompt_tokens"`
    TotalCompletionTokens int     `json:"total_completion_tokens"`
    AvgTokensPerConv      float64 `json:"avg_tokens_per_conv"`
    MaxTokens             int     `json:"max_tokens"`
    EfficiencyScore       int     `json:"efficiency_score"` // 0-100
}

type ChecksSummary struct {
    Structure      *CategoryScore `json:"structure"`
    Hallucinations *CategoryScore `json:"hallucinations"`
    ValueValidity  *CategoryScore `json:"value_validity"`
    ErrorRate      *CategoryScore `json:"error_rate"`
}

type CategoryScore struct {
    Score              int      `json:"score"`
    ConversationsChecked int    `json:"conversations_checked,omitempty"`
    ConversationsPassed  int    `json:"conversations_passed,omitempty"`
    Issues             []string `json:"issues"`
}
```

## Implementation Details

### File Structure
```
scripts/assess-llm-responses/
├── main.go                 # Entry point, orchestration
├── detect.go               # Prompt type detection (reuse from assess-deterministic)
├── structure.go            # Phase 3: Structural checks
├── hallucination.go        # Phase 4: Hallucination detection
├── validation.go           # Phase 5: Value validation
├── tokens.go               # Phase 6: Token metrics
└── scoring.go              # Phase 7: Scoring and summary generation

scripts/assess-llm-responses.sh  # Wrapper script
```

### Shell Script Wrapper
```bash
#!/usr/bin/env bash
# scripts/assess-llm-responses.sh
set -euo pipefail

if [ $# -lt 1 ]; then
    echo "Usage: $0 <project-id>" >&2
    exit 1
fi

PROJECT_ID="$1"

# Build if needed
if [ ! -f "bin/assess-llm-responses" ]; then
    go build -o bin/assess-llm-responses ./scripts/assess-llm-responses/...
fi

# Run assessment
bin/assess-llm-responses "$PROJECT_ID"
```

### Database Queries

**Load Conversations:**
```sql
SELECT id, model, request_messages, response_content,
       prompt_tokens, completion_tokens, total_tokens,
       duration_ms, status, error_message
FROM engine_llm_conversations
WHERE project_id = $1
ORDER BY created_at ASC
```

**Load Schema for Hallucination Detection:**
```sql
-- Tables
SELECT id, table_name
FROM engine_schema_tables
WHERE project_id = $1 AND deleted_at IS NULL AND is_selected = true

-- Columns
SELECT schema_table_id, column_name
FROM engine_schema_columns
WHERE schema_table_id = ANY($1) AND deleted_at IS NULL AND is_selected = true
```

**Load Questions for Source Validation:**
```sql
SELECT id, source_entity_key, priority, category, is_required
FROM engine_ontology_questions
WHERE project_id = $1
```

### Key Algorithms

**Hallucination Detection:**
```go
func detectHallucinations(response map[string]interface{},
                          validTables map[string]bool,
                          validColumns map[string]map[string]bool,
                          tableName string) []string {
    var hallucinations []string

    // Check key_columns
    if keyColumns, ok := response["key_columns"].([]interface{}); ok {
        for _, kc := range keyColumns {
            if colName, ok := kc.(string); ok {
                if !validColumns[tableName][strings.ToLower(colName)] {
                    hallucinations = append(hallucinations,
                        fmt.Sprintf("Table '%s' references non-existent column '%s'",
                                    tableName, colName))
                }
            }
        }
    }

    return hallucinations
}
```

**Structure Validation:**
```go
func validateEntityAnalysisStructure(response map[string]interface{}) (score int, issues []string) {
    score = 100
    required := []string{"business_name", "description", "domain", "key_columns", "questions"}

    for _, field := range required {
        if _, exists := response[field]; !exists {
            issues = append(issues, fmt.Sprintf("Missing required field: %s", field))
            score -= 20
        }
    }

    // Type checks
    if bn, ok := response["business_name"].(string); ok {
        if bn == "" {
            issues = append(issues, "business_name is empty")
            score -= 5
        }
    } else {
        issues = append(issues, "business_name is not a string")
        score -= 10
    }

    // ... similar for other fields

    if score < 0 {
        score = 0
    }
    return score, issues
}
```

## Scoring Examples

### Perfect Score (100/100)
- All responses have valid JSON
- All responses status='success'
- All required fields present with correct types
- Zero hallucinated entities
- All priorities in range 1-5
- All required strings non-empty
- Reasonable token usage

### Good Score (85/100)
- 2-3 hallucinated column names (-15 points)
- All other checks pass

### Mediocre Score (70/100)
- 5 hallucinated entities (-25 points)
- 3 missing required fields (-5 points)
- 2 invalid priorities (-4 points)

### Poor Score (40/100)
- 10+ hallucinated entities (-50 points)
- 15% failed conversations (-10 points)
- Multiple structural issues (-10 points)

## Success Criteria

1. **Score is achievable:** A perfect extraction with Haiku 4.5 scores 100/100
2. **Deterministic only:** No LLM calls during assessment
3. **Rigorous:** Same 7-phase structure as `assess-deterministic`
4. **Actionable:** Issues pinpoint exact hallucinations and structural problems
5. **Comparable:** Can compare Haiku vs Sonnet by running on same project

## Usage

```bash
# Run assessment
./scripts/assess-llm-responses.sh <project-id>

# Compare models
./scripts/assess-llm-responses.sh <haiku-project-id> > haiku-results.json
./scripts/assess-llm-responses.sh <sonnet-project-id> > sonnet-results.json
jq '.final_score' haiku-results.json sonnet-results.json
```

## Testing Strategy

**No automated tests required.** This matches the established pattern for all assessment scripts in this project:
- `assess-deterministic` (2,259 lines) - no tests
- `assess-extraction` - no tests
- `assess-ontology` - no tests

Assessment scripts are self-contained diagnostic tools that use direct SQL queries. They're run manually on demand, not in CI pipelines. Manual validation against actual extractions is sufficient.

## Dependencies

- `github.com/google/uuid` - UUID parsing
- `github.com/jackc/pgx/v5` - PostgreSQL driver
- Standard library only (no LLM client needed)

## Future Enhancements

- **Confidence scoring:** Detect hedging language ("might", "could be")
- **Consistency checks:** Same domain assigned to similar tables
- **Relationship validation:** Check FK patterns in key_columns
- **Performance benchmarks:** Track token/sec, cost estimates

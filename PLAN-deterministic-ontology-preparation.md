# Plan: Improve Deterministic Ontology Input Preparation

## Status

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Fix Row Count -1 Handling | **DONE** |
| 2 | Enhance Schema Summary for Description Processing | **DONE** |
| 2b | Fix ProcessDescription column loading bug | **DONE** |
| 3 | Add Explicit Question Classification Guidance | **DONE** |
| 4 | Question Deduplication Strategy | Pending |
| 5 | Integration Testing and Validation | In Progress |

## Latest Assessment Results (Score: 74/100)

After Phases 1, 2, and 2b:

| Component | Baseline | Current | Target | Notes |
|-----------|----------|---------|--------|-------|
| Input Extraction | 75 | 72 | 95+ | Columns now included; remaining issues are FK/constraints (nice-to-have) |
| Question Classification | 75 | 72 | 90+ | Phase 3 work |
| LLM Output Quality | 95-98 | 95-98 | ~95 | Working well |
| Ontology Accuracy | 86 | 59 | N/A | LLM hallucination issues; relationships handled separately |
| **Final Score** | **83** | **74** | **92+** | |

**Note:** Ontology accuracy dropped due to LLM hallucinating column names (e.g., `price_per_minute` instead of actual `fee_per_minute`) and column counts (e.g., 95 instead of 19). This is an LLM output quality issue, not a deterministic input issue. The input is now correct.

## Overview

Enhance the deterministic code that prepares schema information for the LLM in the ontology extraction workflow. Based on assess-extraction results, this plan addresses specific gaps in input preparation and question classification that reduce extraction quality.

## Phase 2b: Bug Fix - ProcessDescription Column Loading

**Bug discovered:** `ProcessDescription` was calling `ListTablesByDatasource()` which loads tables WITHOUT columns. The schema summary showed "Columns (0):" for all tables.

**Root cause:** `ListTablesByDatasource()` only queries `engine_schema_tables` - it doesn't join or separately load columns from `engine_schema_columns`.

**Fix:** Changed `ProcessDescription` (line 1785) to use `loadTablesWithColumns()` which properly loads and attaches columns.

**Regression tests added:**
- `TestBuildSchemaSummaryForDescription_IncludesColumns` - Verifies data types, PK flags, column counts
- `TestBuildSchemaSummaryForDescription_EmptyColumns` - Documents buggy behavior for detection

## Original Assessment (Baseline Score: 83/100)

### Input Extraction: 75/100
**Issues identified:**
1. Row counts showing -1 (appears to be error/placeholder) - **FIXED**
2. No data type information in first LLM prompt (description processing) - **FIXED**
3. No primary key indicators in first LLM prompt - **FIXED**
4. No foreign key relationships extracted for first LLM prompt - **BY DESIGN** (see note below)
5. No constraints or default values extracted

**Note on #4:** Relationships are intentionally excluded from description processing. That prompt's purpose is to extract user intent and domain context from the description text - FK metadata doesn't help with that. Relationships ARE included in entity analysis where they affect understanding each table's role.

**Impact:** LLM forced to make assumptions about schema structure during description processing.

### Question Classification: 75/100
**Issues identified:**
1. Inferable relationships marked REQUIRED (should be OPTIONAL)
2. Duplicate questions across tables (e.g., "What is marker_at?" asked 3+ times)
3. Questions about visible data instead of asking WHY
4. Enum values with visible numeric codes marked OPTIONAL (should be REQUIRED)
5. Questions about obvious fields (created_at, updated_at) being generated
6. Priority doesn't match criticality (e.g., participant_id relationship is low priority but affects join logic)

**Impact:** Too many required questions create noise; missing required questions create gaps.

### LLM Output Quality: 95-98/100
**Status:** LLM is performing well given the input - this confirms the issue is input preparation, not LLM capability.

### Ontology Accuracy: 86/100
**Note:** Relationship issues are out of scope for this plan (handled in PLAN-review-relationships.md).

## Current State

From `pkg/services/ontology_builder.go`:

**Line 1879-1898:** `buildSchemaSummaryForDescription()` - First LLM prompt (description processing)
- Only provides column names as comma-separated strings
- Missing: data types, primary key indicators, nullable constraints, row count handling

**Line 883-1116:** `buildEntityAnalysisPrompt()` - Second LLM prompt (entity analysis)
- Already provides complete column information: name, data type, PK/nullable flags, sample values
- Token budget management implemented and working well

**Line 870-881:** `entityAnalysisSystemMessage()` - Guidance for question generation
- No explicit rules about when `is_required=true` vs `is_required=false`
- General guidance exists but needs specific classification criteria

## Implementation Phases

### Phase 1: Fix Row Count -1 Handling

**Goal:** Ensure row_count=-1 is either fixed at source or filtered/explained in prompts.

**Investigation needed:**
- Row count -1 likely means table statistics haven't been gathered
- Check where row_count is populated during schema sync
- Decision: Filter out or explain in prompt if -1

**Changes to `buildSchemaSummaryForDescription()` (line 1886-1887):**

```go
// Current code:
if table.RowCount != nil {
    sb.WriteString(fmt.Sprintf("Rows: %d\n", *table.RowCount))
}

// Replace with:
if table.RowCount != nil && *table.RowCount >= 0 {
    sb.WriteString(fmt.Sprintf("Rows: %d\n", *table.RowCount))
}
// Omit row count if -1 (statistics not available)
```

**Alternative approach:** If -1 is common, add explicit note:
```go
if table.RowCount != nil {
    if *table.RowCount >= 0 {
        sb.WriteString(fmt.Sprintf("Rows: %d\n", *table.RowCount))
    } else {
        sb.WriteString("Rows: (statistics not available)\n")
    }
}
```

**Expected impact:**
- No more confusing "Rows: -1" in prompts
- LLM doesn't see error values that suggest data collection failure
- assess-extraction: Minor improvement in input score

**Files:**
- `pkg/services/ontology_builder.go` (line 1886-1887)

**Test:**
- Run workflow with tables that have -1 row counts
- Verify prompt doesn't show -1
- Check assess-extraction input score improves

---

### Phase 2: Enhance Schema Summary for Description Processing

**Goal:** Provide same structured column information in first prompt that we already provide in second prompt.

**Changes to `buildSchemaSummaryForDescription()` (line 1879-1898):**

Replace comma-separated column names with structured per-column information matching the format used in `buildEntityAnalysisPrompt()`:

```go
func (s *ontologyBuilderService) buildSchemaSummaryForDescription(tables []*models.SchemaTable) string {
    var sb strings.Builder

    sb.WriteString(fmt.Sprintf("Total tables: %d\n\n", len(tables)))

    for _, table := range tables {
        sb.WriteString(fmt.Sprintf("### %s\n", table.TableName))

        // Include row count if available and valid
        if table.RowCount != nil && *table.RowCount >= 0 {
            sb.WriteString(fmt.Sprintf("Rows: %d\n", *table.RowCount))
        }

        // Provide structured column information
        sb.WriteString("Columns:\n")
        for _, col := range table.Columns {
            flags := []string{}
            if col.IsPrimaryKey {
                flags = append(flags, "PK")
            }
            if col.IsNullable {
                flags = append(flags, "nullable")
            }
            flagStr := ""
            if len(flags) > 0 {
                flagStr = " [" + strings.Join(flags, ", ") + "]"
            }
            sb.WriteString(fmt.Sprintf("  - %s: %s%s\n", col.ColumnName, col.DataType, flagStr))
        }
        sb.WriteString("\n")
    }

    return sb.String()
}
```

**Expected impact:**
- LLM sees complete schema structure in first prompt (description processing)
- Can distinguish ID columns from text columns when generating entity hints
- No hallucinations about column types
- Better entity hint generation based on actual data types
- assess-extraction input score: 75 → 95+

**Files:**
- `pkg/services/ontology_builder.go` (line 1879-1898)

**Test:**
- Run workflow and check LLM conversation for description processing task
- Verify prompt includes column data types and PK indicators
- Verify entity hints reference actual column types
- Run assess-extraction and verify input score ≥ 95

---

### Phase 3: Add Explicit Question Classification Guidance

**Goal:** Provide clear, explicit rules to LLM about when `is_required=true` vs `is_required=false`.

**Changes to `buildEntityAnalysisPrompt()` (insert after line 1072, before "## TASK"):**

Add a new section with specific classification criteria based on assess-extraction feedback:

```go
func (s *ontologyBuilderService) buildEntityAnalysisPrompt(
    table *models.SchemaTable,
    entitySummary *models.EntitySummary,
    domainContext *models.DomainContext,
    entityHint *models.EntityHint,
    relationships []*models.RelationshipDetail,
    columnGatheredData map[string]map[string]any,
) string {
    // ... existing code through line 1059 ...

    sb.WriteString(`
## QUESTION CLASSIFICATION RULES

When generating questions, set "is_required" based on these criteria:

### REQUIRED QUESTIONS (is_required: true)

Use REQUIRED for questions where the answer fundamentally changes how we understand or query this table:

1. **Enum meanings with numeric/coded values**
   - Example: status column with values [2, 4, 5, 6] - what do these numbers mean?
   - Example: type column with values ["A", "B", "C"] - what business logic do these represent?
   - Why: Cannot write correct WHERE clauses without knowing value meanings

2. **Financial/monetary column purposes**
   - Example: Is 'amount' gross or net? Pre-tax or post-tax?
   - Example: Does 'price' include shipping or exclude it?
   - Why: Misinterpretation has business impact on calculations

3. **Ambiguous foreign key relationships**
   - Example: Does user_id represent creator, assignee, or owner?
   - Example: What is the relationship between owner_id and entity_id?
   - Why: Affects join logic and query correctness

4. **Critical business rules encoded in data**
   - Example: What constitutes an 'active' record when there's no explicit status?
   - Example: Can entity_type be 'engagement' in addition to 'channel'?
   - Why: Fundamental to understanding the data model

### OPTIONAL QUESTIONS (is_required: false)

Use OPTIONAL for questions that provide helpful context but can be reasonably inferred:

1. **Self-explanatory enum values**
   - Example: status with values ["pending", "approved", "rejected"]
   - Why: Values are descriptive enough to understand meaning

2. **Inferable relationships from naming**
   - Example: Is channel_id a reference to the channels table? (obvious from naming)
   - Example: Is marker_at used for pagination? (reasonable assumption from "_at" suffix)
   - Why: We can make reasonable assumptions based on conventions

3. **Nice-to-know context**
   - Example: What department typically uses this table?
   - Example: How frequently is this data updated?
   - Why: Doesn't affect query correctness

4. **Schema design rationale**
   - Example: Why is this column nullable?
   - Example: Why are there separate created_at and updated_at columns?
   - Why: Design choices, not business logic

### NEVER ASK (don't generate these questions at all)

Do not generate questions for obvious or standard schema elements:

1. **Standard timestamp columns**
   - created_at, updated_at, deleted_at - these are self-explanatory

2. **Standard audit columns**
   - created_by, modified_by, version - standard audit trail fields

3. **Auto-increment/UUID primary keys**
   - id, uuid columns marked [PK] - their purpose is obvious

4. **Questions already answered by sample data**
   - If sample data shows status values ["active", "inactive"], don't ask what the values mean
   - If distinct_count is 2 and sample shows [true, false], it's obviously boolean logic

5. **Repetitive questions across similar columns**
   - If you've asked about marker_at in one table, don't ask the same question for marker_at in other tables
   - Reference the earlier question instead: "Same meaning as marker_at in X table?"

`)

    sb.WriteString(fmt.Sprintf(`
## TASK

Analyze the table "%s" and determine if there are any questions that would significantly improve understanding of this entity.

Focus on:
1. Enum/status columns - what do their VALUES mean (not obvious ones)?
2. Financial columns - what EXACTLY do they represent?
3. Foreign keys - what is the BUSINESS MEANING of the relationship?
4. Business rules - any CRITICAL constraints or workflows encoded?

IMPORTANT: Only generate questions if there is genuine ambiguity. Many tables are self-explanatory.

## OUTPUT FORMAT
...`, table.TableName))

    // ... rest of existing code ...
}
```

**Expected impact:**
- Fewer required questions (only true ambiguities)
- No questions for obvious fields (created_at, updated_at, id)
- No duplicate questions across tables
- Better signal-to-noise ratio for users
- assess-extraction question classification score: 75 → 90+

**Files:**
- `pkg/services/ontology_builder.go` (line 1060-1072, insert new section)

**Test:**
- Run workflow on schema with obvious columns (created_at, id, etc.)
- Verify these do NOT generate questions
- Run workflow on schema with numeric enum (status = 2,4,5,6)
- Verify this DOES generate REQUIRED question
- Run assess-extraction and verify question classification score ≥ 90

---

### Phase 4: Question Deduplication Strategy

**Goal:** Prevent asking the same question multiple times across different tables.

**Context:** The current prompt guidance tells LLM not to ask repetitive questions ("If you've asked about marker_at in one table, don't ask the same question for marker_at in other tables"). However:
- Each table is analyzed independently in separate LLM calls
- LLM doesn't have context of questions from other tables
- Post-processing deduplication is needed

**Implementation approach:**

**Option A (Recommended): Post-processing deduplication in workflow orchestration**
- After AnalyzeEntity returns questions, check against existing questions in workflow
- If similar question exists, either skip or consolidate
- Store in workflow state: "Asked about marker_at for table X"

**Option B: Pass existing questions as context**
- Modify `buildEntityAnalysisPrompt()` to include previously asked questions
- LLM can then say "Same as marker_at question for table X" or skip

**Recommendation:** Option A (post-processing) because:
- Keeps prompts shorter (better token usage)
- Simpler to implement and test
- Explicit deduplication logic is easier to reason about

**Changes needed (Option A):**

File: `pkg/services/ontology_tasks.go` (AnalyzeEntityTask execution)

Add deduplication logic after calling `AnalyzeEntity()`:

```go
// After: questions, err := task.ontologyBuilder.AnalyzeEntity(...)

// Load existing questions for this workflow
existingQuestions, err := task.workflowRepo.GetQuestionsByWorkflow(ctx, workflowID)
if err != nil {
    return fmt.Errorf("load existing questions: %w", err)
}

// Deduplicate questions
deduplicatedQuestions := deduplicateQuestions(questions, existingQuestions)

// Store only deduplicated questions
for _, q := range deduplicatedQuestions {
    if err := task.workflowRepo.CreateQuestion(ctx, q); err != nil {
        return fmt.Errorf("create question: %w", err)
    }
}
```

Add helper function:
```go
// deduplicateQuestions removes questions that are substantially similar to existing ones
func deduplicateQuestions(newQuestions []*models.OntologyQuestion, existingQuestions []*models.OntologyQuestion) []*models.OntologyQuestion {
    result := make([]*models.OntologyQuestion, 0, len(newQuestions))

    for _, newQ := range newQuestions {
        isDuplicate := false
        for _, existingQ := range existingQuestions {
            if isSimilarQuestion(newQ.Text, existingQ.Text) {
                isDuplicate = true
                break
            }
        }
        if !isDuplicate {
            result = append(result, newQ)
        }
    }

    return result
}

// isSimilarQuestion checks if two questions are asking about the same thing
func isSimilarQuestion(q1, q2 string) bool {
    // Simple approach: check if questions share significant words
    // More sophisticated: use edit distance or LLM-based similarity

    q1Lower := strings.ToLower(q1)
    q2Lower := strings.ToLower(q2)

    // Extract key terms (column names, table names)
    // If both mention "marker_at" and both ask "what is", they're similar

    // Example simple heuristic:
    if strings.Contains(q1Lower, "marker_at") && strings.Contains(q2Lower, "marker_at") {
        if strings.Contains(q1Lower, "what") && strings.Contains(q2Lower, "what") {
            return true
        }
    }

    // TODO: Implement more robust similarity detection
    return false
}
```

**Expected impact:**
- No duplicate questions like "What is marker_at?" appearing 3+ times
- First table asking about a concept gets the question, subsequent tables don't
- assess-extraction: Minor improvement in question quality score

**Files:**
- `pkg/services/ontology_tasks.go` (AnalyzeEntityTask execution)

**Test:**
- Run workflow on schema with marker_at in multiple tables
- Verify question only appears once
- Check workflow state shows which table "owns" the marker_at question

---

### Phase 5: Integration Testing and Validation

**Goal:** Verify all changes work together and achieve target scores.

**Test Plan:**

1. **Unit Tests**
   - Test `buildSchemaSummaryForDescription()` with various table/column configurations
   - Verify data types, PK flags, nullable are included
   - Verify row_count=-1 is handled correctly

2. **Integration Tests**
   - Run complete workflow on test database
   - Verify description processing prompt has full schema details
   - Verify entity analysis doesn't ask obvious questions
   - Verify enum values with numeric codes generate REQUIRED questions
   - Verify no duplicate questions across tables

3. **Manual Testing with assess-extraction**
   ```bash
   # Run workflow
   make dev-server  # Terminal 1
   make dev-ui      # Terminal 2
   # Create project, sync schema, start extraction

   # After extraction completes:
   ./scripts/assess-extraction.sh <project-id>

   # Expected scores:
   # - Input extraction: ≥ 95 (from 75)
   # - Question classification: ≥ 90 (from 75)
   # - LLM output quality: ~95 (no change expected)
   # - Overall score: ≥ 92 (from 83)
   ```

4. **Regression Testing**
   - Run existing integration tests
   - Update test expectations if question counts change
   - Verify no hallucinated columns in LLM output

**Quality Gates:**

Before merging, verify:
- [ ] assess-extraction input score ≥ 95
- [ ] assess-extraction question classification score ≥ 90
- [ ] assess-extraction overall score ≥ 92
- [ ] All existing integration tests pass
- [ ] Manual workflow test shows:
  - No row_count=-1 in prompts
  - Description processing has data types and PK flags
  - No questions for obvious fields (created_at, updated_at, id)
  - Enum numeric values generate REQUIRED questions
  - No duplicate questions across tables

**Files:**
- `pkg/services/ontology_builder_integration_test.go` (update test expectations)

---

## Success Criteria

1. **Input Extraction:** assess-extraction input score ≥ 95 (from 75)
   - All column metadata provided to first LLM prompt
   - No missing data types, PK flags, or nullable information
   - No row_count=-1 in prompts

2. **Question Classification:** assess-extraction question classification score ≥ 90 (from 75)
   - Required questions only for true ambiguities (enum meanings, financial columns, critical relationships)
   - No questions for obvious fields (created_at, updated_at, id)
   - No duplicate questions across tables

3. **Code Quality:**
   - No hallucinated columns in LLM output
   - Clear, documented classification rules in prompt
   - All existing tests pass with updated expectations

4. **Overall Score:** assess-extraction overall score ≥ 92 (from 83)

## Out of Scope

- **Relationship extraction improvements:** Handled in PLAN-review-relationships.md
- **LLM model changes:** This plan focuses on deterministic input preparation only
- **Frontend changes:** Backend provides data, frontend displays it
- **Sample value gathering:** Already implemented and working well (token budget management)

## File Summary

| File | Phase | Changes |
|------|-------|---------|
| `pkg/services/ontology_builder.go` | 1 | Fix row_count=-1 handling in `buildSchemaSummaryForDescription()` (line 1886-1887) |
| `pkg/services/ontology_builder.go` | 2 | Add data types, PK flags, nullable to `buildSchemaSummaryForDescription()` (line 1879-1898) |
| `pkg/services/ontology_builder.go` | 3 | Add question classification rules to `buildEntityAnalysisPrompt()` (insert after line 1059) |
| `pkg/services/ontology_tasks.go` | 4 | Add question deduplication logic in AnalyzeEntityTask |
| `pkg/services/ontology_builder_integration_test.go` | 5 | Update test expectations for new question behavior |

## Notes

- This plan focuses ONLY on deterministic preparation code (what we send to the LLM)
- LLM output quality is already high (95-98/100), confirming the issue is input preparation
- Changes are incremental and testable at each phase
- Each phase has clear success criteria based on assess-extraction scores

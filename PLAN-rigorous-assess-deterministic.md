# PLAN: Rigorous assess-deterministic Tool

## Current Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 | ✅ Complete | Preserve Workflow State Data |
| Phase 2 | ✅ Complete | Load Additional Data in assess-deterministic |
| Phase 3 | ✅ Complete | Prompt Type Detection |
| Phase 4 | ✅ Complete | Input Assessment - Rigorous Checks |
| Phase 5 | ✅ Complete | Output Assessment - Capture Verification |
| Phase 6 | ⏳ Pending | Scoring Methodology |
| Phase 7 | ⏳ Pending | Detailed Issue Reporting |

---

## Goal
Make assess-deterministic a rigorous tool where **score 100 = deterministic code is perfect**.

The tool must verify:
1. **Input to LLM is complete** - All available schema data is properly presented
2. **Output from LLM is captured correctly** - Responses are parsed and stored without loss

## Key Discovery

The `engine_workflow_state` table stores extracted data in `state_data.gathered`:
- `row_count`, `non_null_count`, `distinct_count`, `null_percent`
- `sample_values` (up to 50 distinct values)
- `is_enum_candidate`, `value_fingerprint`, `scanned_at`

**Problem**: This data was deleted when workflow completes.
**Solution**: ✅ Phase 1 complete - workflow_state now preserved after completion, cleaned up only when new extraction starts.

---

## Phase 1: Preserve Workflow State Data

**Status**: [x] Complete

### Goal
Keep workflow_state after completion so assess-deterministic can verify what data was available.

### Files
- `pkg/services/workflow_orchestrator.go` - Removed deletion in finalizeWorkflow()
- `pkg/services/ontology_workflow.go` - Added cleanup in StartExtraction()
- `pkg/repositories/workflow_state_repository.go` - Added DeleteByOntology method

### Tasks
- [x] 1.1 Find where workflow_state is deleted on completion (workflow_orchestrator.go:478)
- [x] 1.2 Modify to keep rows after workflow completion (removed DeleteByWorkflow call)
- [x] 1.3 Add cleanup logic: delete old workflow_state when NEW extraction starts (scoped to ontology)
- [x] 1.4 Updated mock in tool_executor_test.go to implement new interface method

---

## Phase 2: Load Additional Data in assess-deterministic

**Status**: [x] Complete

### Goal
Load all data needed for rigorous verification.

### Files
- `scripts/assess-deterministic/main.go`

### Current Data Loaded
- schema (tables + columns with stats)
- conversations
- ontology
- questions
- **NEW**: workflow_states (with gathered data: sample_values, distinct_count, etc.)
- **NEW**: relationships (approved schema relationships)

### Tasks Completed
- [x] 2.1 Added types: `WorkflowEntityState`, `GatheredData`, `SchemaRelationship`
- [x] 2.2 Added `loadWorkflowStates()` - loads from `engine_workflow_state` via active ontology
- [x] 2.3 Added `loadRelationships()` - loads approved relationships with table/column names
- [x] 2.4 Updated `loadSchema()` column query to include `distinct_count`, `null_count`
- [x] 2.5 Added `buildGatheredDataMap()` for O(1) lookup by entity_key
- [x] 2.6 Added `countColumnsWithStats()` helper
- [x] 2.7 Updated `AssessmentResult` with `workflow_state_count`, `relationship_count`, `columns_with_stats`
- [x] 2.8 Updated `main()` to call new loaders and populate result

---

## Phase 3: Prompt Type Detection

**Status**: [x] Complete

### Goal
Identify what type of prompt each conversation represents.

### Files
- `scripts/assess-deterministic/main.go`

### Prompt Types to Detect
| Type | Detection Pattern |
|------|------------------|
| description_processing | Contains user description + schema summary |
| entity_analysis | Single table analysis with sample values |
| tier1 | Batch entity summary task |
| tier0 | Domain summary task |

### Tasks Completed
- [x] 3.1 Added PromptType enum with 5 values (entity_analysis, tier1_batch, tier0_domain, description_processing, unknown)
- [x] 3.2 Added TaggedConversation struct
- [x] 3.3 Implemented detectPromptType() with marker-based detection (order matters: most specific first)
- [x] 3.4 Implemented extractTableName() regex helper for entity_analysis prompts
- [x] 3.5 Added tagConversations() and countPromptTypes() helper functions
- [x] 3.6 Updated AssessmentResult with PromptTypeCounts map
- [x] 3.7 Updated main() to tag conversations and log detection summary

---

## Phase 4: Input Assessment - Rigorous Checks

**Status**: [x] Complete

### Goal
For each prompt type, verify ALL required data is present.

### Files
- `scripts/assess-deterministic/main.go`

### Tasks Completed
- [x] 4.1 Added `EntityAnalysisCheck` struct with detailed check fields
- [x] 4.2 Updated `InputAssessment` to include `EntityAnalysisChecks` slice
- [x] 4.3 Implemented `assessEntityAnalysisPrompts()` function for rigorous checks:
  - Table name detection from tagged conversation or prompt content
  - Row count verification (when >= 0)
  - Column presence check (pattern: `- column_name:` to avoid false matches)
  - **Sample values verification against gathered data** (CRITICAL)
  - Relationship inclusion check
- [x] 4.4 Added `extractTableNameFromContent()` helper for fallback table name extraction
- [x] 4.5 Updated `assessInputPreparation()` signature to accept tagged conversations, gathered data map, and relationships
- [x] 4.6 Integrated entity analysis score into final input score (weighted 50%)
- [x] 4.7 Updated `main()` to build gathered data map and call with new parameters

### Code Review Fixes Applied
- [x] 4.8 Added `InputScoreEntityAnalysisWeight` constant, updated formula to use all constants
- [x] 4.9 Added validation that `taggedConvs` and `conversations` have same length
- [x] 4.10 Improved column detection pattern from `col:` to `- col:` (avoids false matches)
- [x] 4.11 Improved sample values search to use next column boundary instead of fixed 500 chars
- [x] 4.12 Added `gathered_data_available` check - warns when gathered data empty (skips critical check)
- [x] 4.13 Added `sample_values_coverage` check - shows "Sample values verified: X/Y columns (Z%)"
- [x] 4.14 Made 0 columns found = score 0 (fatal for that table)

### Scoring Weights (Constants)
```go
InputScoreEntityAnalysisWeight = 50 // Most critical - sample value verification
InputScoreConversationWeight   = 30
InputScoreChecksWeight         = 20
```

### New Checks Added to Output
- `gathered_data_available` - Fails if no gathered data (warns about skipped checks)
- `sample_values_coverage` - Shows exact coverage percentage
- `entity_analysis_complete` - Overall entity analysis pass/fail

### Deferred Checks
- Description processing, Tier1, Tier0 prompt type-specific checks (less critical)
- Question classification rules inclusion check

---

## Phase 5: Output Assessment - Capture Verification

**Status**: [x] Complete

### Goal
Verify LLM responses were captured correctly without data loss.

### Files
- `scripts/assess-deterministic/main.go`

### 5.1 Entity Summary Capture
For each selected table, verify in `engine_ontologies.entity_summaries`:
- [x] Entry exists for table
- [x] `business_name` is non-empty
- [x] `description` is non-empty
- [x] `domain` is valid (non-empty)
- [x] `key_columns` reference ONLY actual columns (no hallucinations)

### 5.2 Question Capture
For each question in `engine_ontology_questions`:
- [x] `text` is non-empty
- [x] `reasoning` is non-empty
- [x] `source_entity_key` references valid table (with invalid source detection)
- [x] `category` is set
- [x] `priority` is valid (1-5)

### 5.3 Domain Summary Capture
For `engine_ontologies.domain_summary`:
- [x] Domain summary object exists and non-empty

### 5.4 LLM Response Parsing
For each `engine_llm_conversations`:
- [x] status = "success" tracking
- [x] Truncation detection (incomplete JSON)
- [x] Parse success/failure counting

### Tasks Completed
- [x] 5.1 Added `EntitySummaryCheck` struct for detailed per-table output verification
- [x] 5.2 Added `QuestionCheck` struct for question capture completeness
- [x] 5.3 Added `ConversationParseCheck` struct for LLM response parsing verification
- [x] 5.4 Updated `OntologyQuestion` struct to include `category` and `priority` fields
- [x] 5.5 Updated `loadQuestions()` query to fetch `category` and `priority`
- [x] 5.6 Implemented `assessEntitySummaries()` - validates business_name, description, domain, key_columns
- [x] 5.7 Implemented `assessQuestionCapture()` - validates text, reasoning, source, category, priority
- [x] 5.8 Implemented `assessConversationParsing()` - detects failures and truncated responses
- [x] 5.9 Updated `PostProcessAssessment` struct with new detailed check fields
- [x] 5.10 Implemented weighted scoring: Entity 40%, Questions 30%, Domain 20%, Parse 10%
- [x] 5.11 Updated `assessPostProcessing()` to call new functions and use weighted scoring

### Scoring Weights (Constants)
```go
PostProcessEntitySummaryWeight = 40 // Entity summaries capture
PostProcessQuestionWeight      = 30 // Question capture
PostProcessDomainWeight        = 20 // Domain summary
PostProcessParseWeight         = 10 // LLM response parsing
```

### Code Review Fixes Applied
- [x] 5.12 Made question penalties proportional (percentage-based instead of flat)
- [x] 5.13 Made truncation penalty proportional (percentage of conversations truncated)
- [x] 5.14 Added error check on entity_summaries JSON parse failure
- [x] 5.15 Added `InvalidPriorities` field to track priorities outside 1-5 range separately
- [x] 5.16 Added `IsExtraTable` field to flag tables in entity_summaries not in schema (hallucination detection)

---

## Phase 6: Scoring Methodology

**Status**: [ ] Not Started

### Goal
Implement clear scoring where 100 = perfect.

### Scoring Weights
```
Input Score (50% of final):
  - Description prompt: 10%
  - Entity analysis prompts: 50% (most critical - sample values)
  - Tier1 prompts: 25%
  - Tier0 prompts: 10%
  - Relationships: 5%

Output Score (50% of final):
  - Entity summaries: 40%
  - Questions: 30%
  - Domain summary: 20%
  - Parse success: 10%

Final = (Input Score * 0.5) + (Output Score * 0.5)
```

### Score 100 Criteria
ALL of the following must be true:
1. Every selected table has entity summary
2. Every entity summary references only real columns
3. Every prompt includes all required schema data
4. Sample values appear when row_count > 0 and distinct_count > 0
5. All relationships included in prompts
6. All LLM responses parsed successfully
7. All questions have complete metadata

---

## Phase 7: Detailed Issue Reporting

**Status**: [ ] Not Started

### Goal
When score < 100, show exactly what failed and where.

### Output Format
```json
{
  "final_score": 87,
  "input_score": 82,
  "output_score": 92,
  "checks": {
    "description_prompt": {"score": 100, "issues": []},
    "entity_analysis": {
      "score": 75,
      "tables_checked": 38,
      "tables_passed": 35,
      "issues": [
        "Table 'offers': missing sample values (row_count=1000, distinct_count=5)",
        "Table 'users': missing relationship to channels",
        "Table 'payments': column 'amount' missing data type"
      ]
    },
    "tier1": {"score": 100, "issues": []},
    "tier0": {"score": 100, "issues": []},
    "entity_summaries": {"score": 90, "issues": ["Table 'offers': key_columns includes 'user_id' but actual column is 'owner_id'"]},
    "questions": {"score": 100, "issues": []},
    "domain_summary": {"score": 100, "issues": []}
  },
  "summary": "Score 87/100 - Sample values missing for 3 tables, 1 hallucinated column in entity summary"
}
```

---

## Files Modified

| File | Changes | Phase |
|------|---------|-------|
| `pkg/services/workflow_orchestrator.go` | Removed DeleteByWorkflow call in finalizeWorkflow() | 1 ✅ |
| `pkg/services/ontology_workflow.go` | Added DeleteByOntology cleanup in StartExtraction() | 1 ✅ |
| `pkg/repositories/workflow_state_repository.go` | Added DeleteByOntology method | 1 ✅ |
| `pkg/llm/tool_executor_test.go` | Updated mock to implement new interface | 1 ✅ |
| `scripts/assess-deterministic/main.go` | Added types, load functions, updated result struct | 2 ✅ |
| `scripts/assess-deterministic/main.go` | Prompt type detection, tagging, counts | 3 ✅ |
| `scripts/assess-deterministic/main.go` | Entity analysis rigorous checks, sample value verification | 4 ✅ |
| `scripts/assess-deterministic/main.go` | Output capture verification, weighted scoring | 5 ✅ |
| `scripts/assess-deterministic/main.go` | Scoring refinements, detailed issue reporting | 6-7 ⏳ |

---

## Implementation Order

1. ✅ Phase 1: Preserve workflow_state (prerequisite - without this, can't verify sample values)
2. ✅ Phase 2: Load additional data (workflow_states, relationships, column stats)
3. ✅ Phase 3: Prompt type detection (tags each conversation, counts by type)
4. ✅ Phase 4: Input checks implementation (entity analysis sample value verification)
5. ✅ Phase 5: Output checks implementation (entity summaries, questions, parsing)
6. ⏳ Phase 6: Scoring implementation
7. ⏳ Phase 7: Detailed issue reporting

---

## Test Cases

After implementation, verify with:
1. Run extraction on test project
2. Run assess-deterministic - should show issues if any
3. Fix issues in ontology_builder
4. Re-run extraction
5. Re-run assess-deterministic - score should improve
6. Repeat until score = 100

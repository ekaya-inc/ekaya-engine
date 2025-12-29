# PLAN: Rigorous assess-deterministic Tool

## Current Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 | ✅ Complete | Preserve Workflow State Data |
| Phase 2 | ⏳ Pending | Load Additional Data in assess-deterministic |
| Phase 3 | ⏳ Pending | Prompt Type Detection |
| Phase 4 | ⏳ Pending | Input Assessment - Rigorous Checks |
| Phase 5 | ⏳ Pending | Output Assessment - Capture Verification |
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
- `pkg/repositories/workflow_state_repository.go` - Added DeleteByProject method

### Tasks
- [x] 1.1 Find where workflow_state is deleted on completion (workflow_orchestrator.go:478)
- [x] 1.2 Modify to keep rows after workflow completion (removed DeleteByWorkflow call)
- [x] 1.3 Add cleanup logic: delete old workflow_state when NEW workflow starts for same project
- [x] 1.4 Updated mock in tool_executor_test.go to implement new interface method

---

## Phase 2: Load Additional Data in assess-deterministic

**Status**: [ ] Not Started

### Goal
Load all data needed for rigorous verification.

### Files
- `scripts/assess-deterministic/main.go`

### Current Data Loaded
- schema (tables + columns)
- conversations
- ontology
- questions

### New Data to Load
- [ ] 2.1 Load `engine_workflow_state` for gathered data (sample_values, distinct_count)
- [ ] 2.2 Load `engine_schema_relationships` for relationship verification
- [ ] 2.3 Ensure columns include all stats (distinct_count, null_count from schema_columns)

---

## Phase 3: Prompt Type Detection

**Status**: [ ] Not Started

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

### Tasks
- [ ] 3.1 Parse `request_messages` JSON structure
- [ ] 3.2 Implement detection heuristics for each prompt type
- [ ] 3.3 Tag each conversation with its prompt type

---

## Phase 4: Input Assessment - Rigorous Checks

**Status**: [ ] Not Started

### Goal
For each prompt type, verify ALL required data is present.

### Files
- `scripts/assess-deterministic/main.go`

### 4.1 Description Processing Prompt Checks
- [ ] All selected tables present
- [ ] Each table shows column count matching schema
- [ ] Each column shows: name, data_type, [PK] flag, [nullable] flag
- [ ] Row counts shown when >= 0 (not -1)

### 4.2 Entity Analysis Prompt Checks (CRITICAL)
For each table analyzed:
- [ ] Table name present
- [ ] Row count present (when >= 0)
- [ ] All selected columns listed with correct data_type, PK flag, nullable flag
- [ ] **Sample values present when row_count > 0 AND distinct_count > 0**
- [ ] Distinct count shown for enum candidates
- [ ] Relationships for this table included
- [ ] Question classification rules included

### 4.3 Tier1 Prompt Checks
- [ ] All tables in batch present
- [ ] Each table has columns with types and flags
- [ ] Related tables shown for each table
- [ ] Domain context included (if available)

### 4.4 Tier0 Prompt Checks
- [ ] Entity summaries grouped by domain
- [ ] All entities from Tier1 present
- [ ] Domain context included

### 4.5 Relationship Checks (All Prompts)
- [ ] All `engine_schema_relationships` with `is_approved=true` appear in relevant prompts
- [ ] Relationship format correct (source_table.column -> target_table.column)

---

## Phase 5: Output Assessment - Capture Verification

**Status**: [ ] Not Started

### Goal
Verify LLM responses were captured correctly without data loss.

### Files
- `scripts/assess-deterministic/main.go`

### 5.1 Entity Summary Capture
For each selected table, verify in `engine_ontologies.entity_summaries`:
- [ ] Entry exists for table
- [ ] `business_name` is non-empty
- [ ] `description` is non-empty
- [ ] `domain` is valid (from allowed list)
- [ ] `key_columns` reference ONLY actual columns (no hallucinations)
- [ ] `column_count` matches actual selected column count

### 5.2 Question Capture
For each question in `engine_ontology_questions`:
- [ ] `text` is non-empty
- [ ] `reasoning` is non-empty
- [ ] `source_entity_key` references valid table
- [ ] `is_required` flag is set (true or false, not null)
- [ ] `category` is set
- [ ] `priority` is valid (1-5)

### 5.3 Domain Summary Capture
For `engine_ontologies.domain_summary`:
- [ ] `description` exists and non-empty
- [ ] `domains` array exists
- [ ] `sample_questions` array exists (can be empty)

### 5.4 LLM Response Parsing
For each `engine_llm_conversations`:
- [ ] status = "success"
- [ ] response_content is valid JSON (parseable)
- [ ] No truncation detected (response appears complete)

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
| `pkg/services/ontology_workflow.go` | Added DeleteByProject cleanup in StartExtraction() | 1 ✅ |
| `pkg/repositories/workflow_state_repository.go` | Added DeleteByProject method | 1 ✅ |
| `pkg/llm/tool_executor_test.go` | Updated mock to implement new interface | 1 ✅ |
| `scripts/assess-deterministic/main.go` | Complete rewrite with rigorous checks | 2-7 ⏳ |

---

## Implementation Order

1. ✅ Phase 1: Preserve workflow_state (prerequisite - without this, can't verify sample values)
2. ⏳ Phase 2: Load additional data
3. ⏳ Phase 3: Prompt type detection
4. ⏳ Phase 4: Input checks implementation
5. ⏳ Phase 5: Output checks implementation
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

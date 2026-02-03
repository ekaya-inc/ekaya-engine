# FIX: Remove Dead Code - DeterministicQuestionGenerator

## Summary

The `DeterministicQuestionGenerator` is complete, tested code that is **never called in production**. It should be deleted because:

1. It's dead code - no production callers
2. It violates project philosophy: "Never send an ontology question from deterministic code when we have an LLM in the system"
3. It has the same NullCount bug (uses `col.NullCount` which is always nil)

## Files to Delete

```
pkg/services/deterministic_question_generation.go      (296 lines)
pkg/services/deterministic_question_generation_test.go (616 lines)
```

## Verification That Code Is Dead

### 1. No Production Callers

```bash
# Search for any usage of the generator
grep -r "DeterministicQuestion\|NewDeterministicQuestionGenerator\|GenerateFromSchema" \
  pkg/ \
  --include="*.go" \
  --exclude="*_test.go" \
  --exclude="deterministic_question_generation.go"

# Result: No matches
```

### 2. No Imports

```bash
# Check if any file imports the deterministic generator
grep -r "deterministic_question" pkg/ --include="*.go" | grep -v "deterministic_question_generation"

# Result: No matches
```

### 3. Not Wired Into DAG

The DAG nodes that generate questions use LLM-based generation:
- `entity_discovery_service.go:552` - LLM returns questions
- `column_enrichment.go:982` - LLM returns questions
- `relationship_enrichment.go:401` - LLM returns questions

None of them call `DeterministicQuestionGenerator`.

## What the Code Was Designed to Do

The generator was designed to create ontology questions for:

1. **High NULL rate columns** (>80%) - Ask user why column is mostly empty
2. **Cryptic enum values** - Ask user what single-letter codes mean

```go
// From deterministic_question_generation.go

// checkHighNullRate generates a question if a column has >80% NULL values
func (g *DeterministicQuestionGenerator) checkHighNullRate(...) *models.OntologyQuestion

// checkCrypticEnumValues generates a question if sample values appear to be
// cryptic codes (single letters, numbers, abbreviations)
func (g *DeterministicQuestionGenerator) checkCrypticEnumValues(...) *models.OntologyQuestion
```

## Why It Should Be Removed (Not Fixed)

Per project philosophy documented in the codebase:

> "Never send an ontology question from deterministic code when we have an LLM in the system. Deterministic triggers should feed INTO an LLM that decides whether a question makes sense."

If high NULL rates or cryptic enums need to trigger questions:
1. Detect the pattern deterministically during feature extraction
2. Include the pattern in the LLM prompt
3. Let the LLM decide whether to generate a question

This is already how the enrichment steps work - they pass context to the LLM and the LLM can return a `questions` array in its response.

---

## Implementation Steps

### Step 1: Verify Current State

```bash
# Run tests to confirm everything passes before deletion
make check
```

### Step 2: Delete the Files

```bash
rm pkg/services/deterministic_question_generation.go
rm pkg/services/deterministic_question_generation_test.go
```

### Step 3: Verify No Build Errors

```bash
go build ./...
```

### Step 4: Verify Tests Still Pass

```bash
make check
```

### Step 5: Clean Up Go Modules

```bash
go mod tidy
```

---

## Verification

### Pre-Deletion Checks

```bash
# Confirm files exist
ls -la pkg/services/deterministic_question_generation*.go

# Confirm no other code depends on them
grep -r "DeterministicQuestion" pkg/ --include="*.go" | grep -v deterministic_question_generation

# Run full test suite
make check
```

### Post-Deletion Checks

```bash
# Confirm files are gone
ls pkg/services/deterministic_question_generation*.go 2>&1 | grep "No such file"

# Confirm build succeeds
go build ./...

# Confirm tests pass
make check

# Confirm no dangling references
grep -r "deterministic_question" pkg/ --include="*.go"
```

---

## Future Consideration

If deterministic pattern detection is needed in the future, implement it as:

1. **Detection phase** - Add pattern flags to `ColumnFeatures`:
   ```go
   type ColumnFeatures struct {
       // ... existing fields ...

       // Pattern flags for LLM consideration
       HasHighNullRate    bool   `json:"has_high_null_rate,omitempty"`
       HasCrypticEnums    bool   `json:"has_cryptic_enums,omitempty"`
       PatternContext     string `json:"pattern_context,omitempty"`
   }
   ```

2. **LLM prompt inclusion** - Add detected patterns to classification prompts:
   ```
   **Detected patterns:**
   - High null rate (92%) - consider if this indicates optional field or data quality issue
   - Sample values appear cryptic: ["A", "B", "C", "X"]
   ```

3. **LLM decides** - Add `questions` field to classifier response schemas:
   ```json
   {
     "purpose": "enum",
     "confidence": 0.6,
     "questions": [
       {
         "text": "The values 'A', 'B', 'C', 'X' appear to be codes. What do they represent?",
         "category": "enumeration",
         "priority": 2
       }
     ]
   }
   ```

This follows the project philosophy: deterministic detection feeds into LLM decision-making.

---

## Checklist

```
[x] Run make check - verify tests pass before deletion
[x] Delete pkg/services/deterministic_question_generation.go
[x] Delete pkg/services/deterministic_question_generation_test.go
[x] Run go build ./... - verify no build errors
[x] Run make check - verify tests still pass
[x] Run go mod tidy - clean up modules
[x] Commit with message explaining removal rationale
```

## Commit Message

```
fix: remove dead code DeterministicQuestionGenerator

The DeterministicQuestionGenerator was a complete implementation with tests
that was never wired into the production DAG pipeline. It's being removed
because:

1. Dead code - no production callers exist
2. Violates project philosophy - deterministic code should not generate
   ontology questions directly; it should feed patterns into LLMs that
   decide whether questions are warranted
3. Had the same NullCount bug - would never work correctly anyway

If deterministic pattern detection is needed in the future, it should be
implemented as flags in ColumnFeatures that get included in LLM prompts,
letting the LLM decide whether to generate questions.
```

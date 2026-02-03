# FIX: Timestamp Classification Improvements

## Summary

After fixing the NullCount calculation bug (see `FIX-null-count-calculation.md`), additional improvements are needed for timestamp classification:

1. **Remove rigid threshold guidance** - "90-100% NULL = soft delete" is too prescriptive
2. **Classify all nullable timestamps** - Any nullable timestamp with mixed null/non-null values should be LLM-classified
3. **Enable LLM uncertainty escalation** - Allow classifiers to generate questions when uncertain

## Prerequisites

Complete `FIX-null-count-calculation.md` first - these improvements depend on correct null rate data.

---

## Issue 1: Rigid Threshold Guidance

### Current Behavior

**File:** `pkg/services/column_feature_extraction.go:857-860`

```go
sb.WriteString("**Classification rules:**\n")
sb.WriteString("- **90-100% NULL:** Likely soft delete or optional event timestamp\n")
sb.WriteString("- **0-5% NULL:** Likely required audit field (created_at, updated_at) or event time\n")
sb.WriteString("- **5-90% NULL:** Conditional timestamp (populated under certain conditions)\n")
```

### Problem

This fails for real-world cases:
- `users.deleted_at` with 2% deleted users → Still a soft delete, not "audit field"
- `profile_picture_updated_at` with 95% null → Optional event, not soft delete

### Fix

Replace threshold-based guidance with semantic guidance:

```go
sb.WriteString("**Analysis guidance:**\n")
sb.WriteString("- Consider what NULL vs non-NULL means semantically for this column\n")
sb.WriteString("- NULL might mean: 'not yet happened', 'never will happen', 'was removed/deleted', or 'unknown'\n")
sb.WriteString("- Non-NULL might mean: 'event occurred at this time', 'record was modified', 'soft deleted'\n")
sb.WriteString("- The null rate indicates frequency, not purpose - a 2% non-null rate for deleted_at still means soft delete\n")
sb.WriteString("- Column name provides context but DATA characteristics determine classification\n")
```

**Full prompt update in `buildPrompt()` (lines 854-872):**

```go
sb.WriteString("\n## Task\n\n")
sb.WriteString("Based on the column's data characteristics and semantic context, determine the timestamp's purpose.\n\n")

sb.WriteString("**Analysis guidance:**\n")
sb.WriteString("- Consider what NULL vs non-NULL means semantically for this column\n")
sb.WriteString("- NULL might mean: 'not yet happened', 'never will happen', 'was removed/deleted', or 'unknown'\n")
sb.WriteString("- Non-NULL might mean: 'event occurred at this time', 'record was modified', 'soft deleted'\n")
sb.WriteString("- The null rate indicates frequency, not purpose - a 2% non-null rate can still indicate soft delete\n")
sb.WriteString("- Column name provides context but DATA characteristics should inform your decision\n")
if timestampScale == "nanoseconds" {
	sb.WriteString("- Nanosecond precision suggests cursor/pagination use (high precision for ordering)\n")
}

sb.WriteString("\n**Possible purposes:**\n")
sb.WriteString("- `audit_created`: Records when the row was created (typically NOT NULL, set once)\n")
sb.WriteString("- `audit_updated`: Records when the row was last modified (typically NOT NULL, updated frequently)\n")
sb.WriteString("- `soft_delete`: Records when the row was logically deleted (NULL = active, non-NULL = deleted)\n")
sb.WriteString("- `event_time`: Records when a specific business event occurred\n")
sb.WriteString("- `scheduled_time`: Records when something is scheduled to happen\n")
sb.WriteString("- `expiration`: Records when something expires or becomes invalid\n")
sb.WriteString("- `cursor`: Used for pagination/ordering (typically high precision)\n")
```

---

## Issue 2: Classify All Nullable Timestamps

### Current Behavior

All timestamp columns go through LLM classification, but the rigid threshold guidance causes misclassification. Additionally, the `NeedsCrossColumnCheck` flag is only set when `IsSoftDelete` is true.

### Proposed Change

Flag ANY nullable timestamp with mixed null/non-null values for cross-column validation:

**File:** `pkg/services/column_feature_extraction.go:937-939`

**Current:**
```go
// Soft delete timestamps may need cross-column validation
if response.IsSoftDelete {
	features.NeedsCrossColumnCheck = true
}
```

**Proposed:**
```go
// Nullable timestamps with mixed null/non-null values may need cross-column validation
// to understand the semantic meaning of nullability
if profile.IsNullable && profile.NullRate > 0 && profile.NullRate < 1.0 {
	features.NeedsCrossColumnCheck = true
}
```

This ensures:
- `deleted_at` with 2% non-null gets validated (soft delete with rare deletions)
- `completed_at` with 50% non-null gets validated (event time for completed items)
- `created_at` with 0% null is NOT flagged (required audit field)

---

## Issue 3: Enable LLM Uncertainty Escalation

### Current Behavior

Timestamp classifier response schema:

```go
type timestampClassificationResponse struct {
	Purpose      string  `json:"purpose"`
	Confidence   float64 `json:"confidence"`
	IsSoftDelete bool    `json:"is_soft_delete"`
	IsAuditField bool    `json:"is_audit_field"`
	Description  string  `json:"description"`
}
```

The LLM must always return a definitive classification. Low confidence results get stored without user input.

### Proposed Change

Add optional question generation to the response schema:

**File:** `pkg/services/column_feature_extraction.go`

**New response struct (replace lines 888-895):**
```go
type timestampClassificationResponse struct {
	Purpose            string  `json:"purpose"`
	Confidence         float64 `json:"confidence"`
	IsSoftDelete       bool    `json:"is_soft_delete"`
	IsAuditField       bool    `json:"is_audit_field"`
	Description        string  `json:"description"`
	NeedsClarification bool    `json:"needs_clarification,omitempty"`
	ClarificationQuestion string `json:"clarification_question,omitempty"`
}
```

**Update prompt response format (lines 874-883):**
```go
sb.WriteString("\n## Response Format\n\n")
sb.WriteString("```json\n")
sb.WriteString("{\n")
sb.WriteString("  \"purpose\": \"audit_created\",\n")
sb.WriteString("  \"confidence\": 0.85,\n")
sb.WriteString("  \"is_soft_delete\": false,\n")
sb.WriteString("  \"is_audit_field\": true,\n")
sb.WriteString("  \"description\": \"Records when the record was created.\",\n")
sb.WriteString("  \"needs_clarification\": false,\n")
sb.WriteString("  \"clarification_question\": \"\"\n")
sb.WriteString("}\n")
sb.WriteString("```\n\n")
sb.WriteString("If you are uncertain (confidence < 0.7), set `needs_clarification: true` and provide a specific question.\n")
```

**Update parseResponse to handle questions (after line 939):**
```go
// If LLM is uncertain, generate an ontology question
if response.NeedsClarification && response.ClarificationQuestion != "" && response.Confidence < 0.7 {
	features.NeedsClarification = true
	features.ClarificationQuestion = response.ClarificationQuestion
}
```

**Add fields to ColumnFeatures model:**
```go
// pkg/models/column_features.go - add to ColumnFeatures struct

// Uncertainty escalation (set during classification when LLM is uncertain)
NeedsClarification     bool   `json:"needs_clarification,omitempty"`
ClarificationQuestion  string `json:"clarification_question,omitempty"`
```

**Process questions after classification (in ClassifyColumnsInBatch):**
```go
// After classifying all columns, collect questions from uncertain classifications
var questionInputs []QuestionInput
for _, features := range classifiedFeatures {
	if features.NeedsClarification && features.ClarificationQuestion != "" {
		questionInputs = append(questionInputs, QuestionInput{
			Text:     features.ClarificationQuestion,
			Category: models.QuestionCategoryTerminology,
			Priority: 3, // Medium priority
			Context: fmt.Sprintf("Column: %s.%s, Type: %s, Null Rate: %.1f%%",
				features.TableName, features.ColumnName, features.DataType, features.NullRate*100),
		})
	}
}

if len(questionInputs) > 0 && s.questionService != nil {
	questionModels := ConvertQuestionInputs(questionInputs, projectID, ontologyID, nil)
	if err := s.questionService.CreateQuestions(ctx, questionModels); err != nil {
		s.logger.Error("failed to store classification questions",
			zap.Int("count", len(questionModels)),
			zap.Error(err))
		// Non-fatal: continue even if question storage fails
	}
}
```

---

## Verification

### 1. Threshold Guidance Removal Test

```go
func Test_TimestampClassifier_LowNullRateSoftDelete(t *testing.T) {
	// A deleted_at column with only 2% non-null (98% null) should still be soft_delete
	profile := &models.ColumnDataProfile{
		ColumnName: "deleted_at",
		TableName:  "users",
		DataType:   "timestamp with time zone",
		NullRate:   0.98, // 98% null = 2% deleted
		RowCount:   10000,
	}

	classifier := &timestampClassifier{logger: zap.NewNop()}
	prompt := classifier.buildPrompt(profile)

	// Verify prompt doesn't say "0-5% NULL = audit field"
	assert.NotContains(t, prompt, "0-5% NULL")
	assert.NotContains(t, prompt, "90-100% NULL")

	// Verify prompt includes semantic guidance
	assert.Contains(t, prompt, "null rate indicates frequency, not purpose")
}
```

### 2. Nullable Timestamp Flagging Test

```go
func Test_NullableTimestamp_FlaggedForCrossColumn(t *testing.T) {
	// Any nullable timestamp with mixed values should be flagged
	tests := []struct {
		name       string
		nullRate   float64
		isNullable bool
		wantFlag   bool
	}{
		{"soft_delete_rare", 0.98, true, true},     // 2% deleted
		{"soft_delete_common", 0.50, true, true},   // 50% deleted
		{"completed_at", 0.30, true, true},         // 70% completed
		{"created_at_required", 0.0, false, false}, // NOT NULL
		{"all_null", 1.0, true, false},             // No data yet
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &models.ColumnDataProfile{
				IsNullable: tt.isNullable,
				NullRate:   tt.nullRate,
			}

			// Simulate the flagging logic
			shouldFlag := profile.IsNullable && profile.NullRate > 0 && profile.NullRate < 1.0

			assert.Equal(t, tt.wantFlag, shouldFlag)
		})
	}
}
```

### 3. Uncertainty Escalation Test

```go
func Test_TimestampClassifier_UncertaintyGeneratesQuestion(t *testing.T) {
	response := `{
		"purpose": "event_time",
		"confidence": 0.55,
		"is_soft_delete": false,
		"is_audit_field": false,
		"description": "Unclear purpose",
		"needs_clarification": true,
		"clarification_question": "Is this column used for soft deletes or tracking when an optional event occurred?"
	}`

	classifier := &timestampClassifier{logger: zap.NewNop()}
	features, err := classifier.parseResponse(profile, response, "test-model")

	require.NoError(t, err)
	assert.True(t, features.NeedsClarification)
	assert.NotEmpty(t, features.ClarificationQuestion)
	assert.Less(t, features.Confidence, 0.7)
}
```

---

## Implementation Order

1. **First:** Complete `FIX-null-count-calculation.md` - required for correct null rates
2. **Second:** Update timestamp prompt guidance (remove rigid thresholds)
3. **Third:** Flag all nullable timestamps for cross-column validation
4. **Fourth:** Add uncertainty escalation to response schema and processing

---

## Checklist

```
[x] Prerequisite: FIX-null-count-calculation.md is complete
[x] Update timestamp classification prompt (lines 854-872)
[x] Update NeedsCrossColumnCheck flagging logic (line 937-939)
[ ] Add NeedsClarification fields to response schema
[x] Add NeedsClarification fields to ColumnFeatures model
[x] Update parseResponse to handle clarification
[ ] Add question creation in ClassifyColumnsInBatch
[ ] Add unit tests for each change
[ ] Run make check - all tests pass
```

// validation.go implements Phase 5: Value Validation
// This phase validates field values beyond just type checking: non-empty strings,
// priority ranges, boolean types, and category presence.
package main

import (
	"fmt"
)

// =============================================================================
// Data Types for Phase 5: Value Validation
// =============================================================================

// ValueValidationResult contains results for value validation of a conversation
type ValueValidationResult struct {
	ConversationID string     `json:"conversation_id"`
	PromptType     PromptType `json:"prompt_type"`
	TargetTable    string     `json:"target_table,omitempty"`

	// Scoring breakdown (max 30 points total)
	StringFieldsScore int `json:"string_fields_score"` // 10 points max
	PriorityScore     int `json:"priority_score"`      // 10 points max
	BooleanScore      int `json:"boolean_score"`       // 5 points max
	CategoryScore     int `json:"category_score"`      // 5 points max
	TotalScore        int `json:"total_score"`         // 30 points max

	// Issues found during validation
	Issues []string `json:"issues"`
}

// ValueValidationSummary aggregates results from all value validation checks
type ValueValidationSummary struct {
	ConversationsChecked int                     `json:"conversations_checked"`
	ConversationsPassed  int                     `json:"conversations_passed"`
	AverageScore         float64                 `json:"average_score"` // Average of TotalScore (max 30)
	TotalIssues          int                     `json:"total_issues"`
	Results              []ValueValidationResult `json:"results"`

	// Per-check breakdown
	StringFieldIssues  int `json:"string_field_issues"` // Conversations with string issues
	PriorityIssues     int `json:"priority_issues"`     // Conversations with priority issues
	BooleanTypeIssues  int `json:"boolean_type_issues"` // Conversations with boolean issues
	CategoryMissing    int `json:"category_missing"`    // Conversations with missing categories
	QuestionsPriority  int `json:"questions_priority"`  // Questions checked for priority
	InvalidPriorities  int `json:"invalid_priorities"`  // Questions with invalid priority
	QuestionsBoolean   int `json:"questions_boolean"`   // Questions checked for boolean
	InvalidBooleans    int `json:"invalid_booleans"`    // Questions with invalid boolean type
	QuestionCategories int `json:"question_categories"` // Questions checked for category
	MissingCategories  int `json:"missing_categories"`  // Questions with missing category
}

// =============================================================================
// Phase 5: Value Validation Entry Point
// =============================================================================

// checkAllValueValidation runs value validation checks on all tagged conversations
// and stored questions.
func checkAllValueValidation(
	tagged []TaggedConversation,
	structureResults []StructureCheckResult,
	questions []OntologyQuestion,
) ValueValidationSummary {
	summary := ValueValidationSummary{
		Results: make([]ValueValidationResult, 0),
	}

	// Check conversations (entity_analysis responses have fields to validate)
	for i, tc := range tagged {
		// Skip if JSON parsing failed
		if structureResults[i].ParsedResponse == nil {
			continue
		}

		// Only validate entity_analysis responses - they have all the value fields
		if tc.PromptType != PromptTypeEntityAnalysis {
			continue
		}

		result := checkValueValidation(tc, structureResults[i].ParsedResponse)
		summary.Results = append(summary.Results, result)
		summary.ConversationsChecked++

		if result.TotalScore == 30 {
			summary.ConversationsPassed++
		}

		summary.TotalIssues += len(result.Issues)
		if result.StringFieldsScore < 10 {
			summary.StringFieldIssues++
		}
		if result.PriorityScore < 10 {
			summary.PriorityIssues++
		}
		if result.BooleanScore < 5 {
			summary.BooleanTypeIssues++
		}
		if result.CategoryScore < 5 {
			summary.CategoryMissing++
		}
	}

	// Also validate stored questions (priority, boolean, category checks)
	questionPriorityScore, priorityIssues, priorityChecked, invalidPriorities := checkQuestionPriorities(questions)
	questionBooleanScore, booleanIssues, booleanChecked, invalidBooleans := checkQuestionBooleans(questions)
	questionCategoryScore, categoryIssues, categoryChecked, missingCategories := checkQuestionCategories(questions)

	// Add question validation issues to summary
	summary.QuestionsPriority = priorityChecked
	summary.InvalidPriorities = invalidPriorities
	summary.QuestionsBoolean = booleanChecked
	summary.InvalidBooleans = invalidBooleans
	summary.QuestionCategories = categoryChecked
	summary.MissingCategories = missingCategories

	// Create a synthetic result for stored questions if there are any
	if len(questions) > 0 {
		questionResult := ValueValidationResult{
			ConversationID:    "stored_questions",
			PromptType:        "questions_validation",
			StringFieldsScore: 10, // N/A for stored questions, full score
			PriorityScore:     questionPriorityScore,
			BooleanScore:      questionBooleanScore,
			CategoryScore:     questionCategoryScore,
			Issues:            append(append(priorityIssues, booleanIssues...), categoryIssues...),
		}
		questionResult.TotalScore = questionResult.StringFieldsScore + questionResult.PriorityScore +
			questionResult.BooleanScore + questionResult.CategoryScore
		summary.Results = append(summary.Results, questionResult)
		summary.TotalIssues += len(questionResult.Issues)
	}

	// Calculate average score
	if len(summary.Results) > 0 {
		totalScore := 0
		for _, r := range summary.Results {
			totalScore += r.TotalScore
		}
		summary.AverageScore = float64(totalScore) / float64(len(summary.Results))
	}

	return summary
}

// checkValueValidation performs value validation for a single conversation.
func checkValueValidation(tc TaggedConversation, parsed map[string]interface{}) ValueValidationResult {
	result := ValueValidationResult{
		ConversationID: tc.Conversation.ID.String(),
		PromptType:     tc.PromptType,
		TargetTable:    tc.TargetTable,
		Issues:         []string{},
	}

	// 5.1 Required String Fields (10 points)
	result.StringFieldsScore = checkRequiredStringFields(parsed, &result.Issues)

	// 5.2 Priority Values (10 points) - check questions array in response
	result.PriorityScore = checkResponsePriorities(parsed, &result.Issues)

	// 5.3 Boolean Fields (5 points) - check is_required in questions array
	result.BooleanScore = checkResponseBooleans(parsed, &result.Issues)

	// 5.4 Category Values (5 points) - check category in questions array
	result.CategoryScore = checkResponseCategories(parsed, &result.Issues)

	result.TotalScore = result.StringFieldsScore + result.PriorityScore +
		result.BooleanScore + result.CategoryScore

	return result
}

// =============================================================================
// 5.1 Required String Fields (10 points)
// =============================================================================

// checkRequiredStringFields validates that required string fields are non-empty.
// - business_name: not empty
// - description: not empty (min 10 chars)
// - domain: not empty
// Penalty: -3 per empty required field (max -10, but clamped to score 0)
func checkRequiredStringFields(parsed map[string]interface{}, issues *[]string) int {
	score := 10
	const penaltyPerField = 3

	// Check business_name
	if businessName, ok := parsed["business_name"].(string); ok {
		if businessName == "" {
			*issues = append(*issues, "business_name is empty")
			score -= penaltyPerField
		}
	} else {
		// Not a string - may have been caught by type validation, but we still penalize for value
		if _, exists := parsed["business_name"]; exists {
			*issues = append(*issues, "business_name is not a valid string")
			score -= penaltyPerField
		}
	}

	// Check description (min 10 chars)
	if description, ok := parsed["description"].(string); ok {
		if description == "" {
			*issues = append(*issues, "description is empty")
			score -= penaltyPerField
		} else if len(description) < 10 {
			*issues = append(*issues, fmt.Sprintf("description is too short (%d chars, min 10)", len(description)))
			score -= penaltyPerField
		}
	} else {
		if _, exists := parsed["description"]; exists {
			*issues = append(*issues, "description is not a valid string")
			score -= penaltyPerField
		}
	}

	// Check domain
	if domain, ok := parsed["domain"].(string); ok {
		if domain == "" {
			*issues = append(*issues, "domain is empty")
			score -= penaltyPerField
		}
	} else {
		if _, exists := parsed["domain"]; exists {
			*issues = append(*issues, "domain is not a valid string")
			score -= penaltyPerField
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

// =============================================================================
// 5.2 Priority Values (10 points)
// =============================================================================

// checkResponsePriorities validates priority values in the questions array of a response.
// Priority must be in range 1-5.
// Penalty: -2 per invalid priority (max -10)
func checkResponsePriorities(parsed map[string]interface{}, issues *[]string) int {
	score := 10

	questions, ok := parsed["questions"].([]interface{})
	if !ok {
		// No questions array or wrong type - already caught by type validation
		return score
	}

	invalidCount := 0
	for i, q := range questions {
		qMap, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		// Check priority field
		if priority, exists := qMap["priority"]; exists {
			if priorityNum, ok := priority.(float64); ok {
				if priorityNum < 1 || priorityNum > 5 {
					*issues = append(*issues, fmt.Sprintf("questions[%d].priority is %v (must be 1-5)", i, int(priorityNum)))
					invalidCount++
				}
			} else {
				*issues = append(*issues, fmt.Sprintf("questions[%d].priority is not a number", i))
				invalidCount++
			}
		}
		// Note: missing priority is not penalized - it may be optional
	}

	penalty := invalidCount * 2
	if penalty > 10 {
		penalty = 10
	}
	score -= penalty

	if score < 0 {
		score = 0
	}
	return score
}

// checkQuestionPriorities validates priority values in stored questions.
// Returns score (0-10), issues list, count checked, and invalid count.
func checkQuestionPriorities(questions []OntologyQuestion) (int, []string, int, int) {
	score := 10
	var issues []string
	checkedCount := 0
	invalidCount := 0

	for _, q := range questions {
		if q.Priority == nil {
			continue
		}
		checkedCount++

		priority := *q.Priority
		if priority < 1 || priority > 5 {
			issues = append(issues, fmt.Sprintf("stored question '%s': priority is %d (must be 1-5)",
				truncateText(q.Text, 40), priority))
			invalidCount++
		}
	}

	penalty := invalidCount * 2
	if penalty > 10 {
		penalty = 10
	}
	score -= penalty

	if score < 0 {
		score = 0
	}
	return score, issues, checkedCount, invalidCount
}

// =============================================================================
// 5.3 Boolean Fields (5 points)
// =============================================================================

// checkResponseBooleans validates is_required field in questions array is boolean type.
// Penalty: -5 if any is_required is wrong type (not boolean)
func checkResponseBooleans(parsed map[string]interface{}, issues *[]string) int {
	score := 5

	questions, ok := parsed["questions"].([]interface{})
	if !ok {
		return score
	}

	for i, q := range questions {
		qMap, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		// Check is_required field type
		if isRequired, exists := qMap["is_required"]; exists {
			switch isRequired.(type) {
			case bool:
				// Correct type, no penalty
			case nil:
				*issues = append(*issues, fmt.Sprintf("questions[%d].is_required is null (should be boolean)", i))
				score = 0 // Full penalty for wrong type
			case string:
				*issues = append(*issues, fmt.Sprintf("questions[%d].is_required is a string (should be boolean)", i))
				score = 0
			default:
				*issues = append(*issues, fmt.Sprintf("questions[%d].is_required has wrong type (should be boolean)", i))
				score = 0
			}
		}
	}

	return score
}

// checkQuestionBooleans validates stored questions have proper boolean is_required.
// Since OntologyQuestion.IsRequired is typed as bool in Go, the value from DB is already boolean.
// This check is mainly for completeness - the schema enforces boolean type.
// Returns score (0-5), issues list, count checked, and invalid count.
func checkQuestionBooleans(questions []OntologyQuestion) (int, []string, int, int) {
	// In the Go struct, IsRequired is typed as bool, so it's always valid.
	// This validates that the DB schema properly enforces boolean type.
	// Since we can't get non-boolean values here (Go enforces the type),
	// we give full score. The real validation happens in checkResponseBooleans
	// for LLM response JSON where the type can be anything.
	return 5, []string{}, len(questions), 0
}

// =============================================================================
// 5.4 Category Values (5 points)
// =============================================================================

// checkResponseCategories validates category field is non-empty when present in questions.
// Penalty: -1 per missing/empty category (max -5)
func checkResponseCategories(parsed map[string]interface{}, issues *[]string) int {
	score := 5

	questions, ok := parsed["questions"].([]interface{})
	if !ok {
		return score
	}

	missingCount := 0
	for i, q := range questions {
		qMap, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		// Check category field
		if category, exists := qMap["category"]; exists {
			if categoryStr, ok := category.(string); ok {
				if categoryStr == "" {
					*issues = append(*issues, fmt.Sprintf("questions[%d].category is empty", i))
					missingCount++
				}
			} else if category == nil {
				*issues = append(*issues, fmt.Sprintf("questions[%d].category is null", i))
				missingCount++
			}
		}
		// Note: missing category field is not penalized - may be optional
	}

	penalty := missingCount
	if penalty > 5 {
		penalty = 5
	}
	score -= penalty

	if score < 0 {
		score = 0
	}
	return score
}

// checkQuestionCategories validates stored questions have non-empty category when present.
// Returns score (0-5), issues list, count checked, and missing count.
func checkQuestionCategories(questions []OntologyQuestion) (int, []string, int, int) {
	score := 5
	var issues []string
	checkedCount := 0
	missingCount := 0

	for _, q := range questions {
		// Category is a pointer - nil means not present (allowed)
		// Empty string means present but empty (penalized)
		if q.Category == nil {
			continue
		}
		checkedCount++

		if *q.Category == "" {
			issues = append(issues, fmt.Sprintf("stored question '%s': category is empty",
				truncateText(q.Text, 40)))
			missingCount++
		}
	}

	penalty := missingCount
	if penalty > 5 {
		penalty = 5
	}
	score -= penalty

	if score < 0 {
		score = 0
	}
	return score, issues, checkedCount, missingCount
}

// =============================================================================
// Helper Functions
// =============================================================================

// valueScoreToPercentage converts the 30-point value score to percentage (0-100)
func valueScoreToPercentage(totalScore int) int {
	// Max value validation score is 30 (10 + 10 + 5 + 5)
	return (totalScore * 100) / 30
}

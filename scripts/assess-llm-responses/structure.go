// structure.go implements Phase 3: Per-Response Structural Checks
// This phase validates JSON parsing, response status, field completeness, and type validation.
package main

import (
	"encoding/json"
	"fmt"
)

// =============================================================================
// Data Types for Phase 3: Structural Checks
// =============================================================================

// StructureCheckResult contains the results of structural validation for a conversation
type StructureCheckResult struct {
	ConversationID string     `json:"conversation_id"`
	PromptType     PromptType `json:"prompt_type"`
	TargetTable    string     `json:"target_table,omitempty"`

	// Scoring breakdown
	JSONParseScore      int `json:"json_parse_score"`      // 20 points max
	ResponseStatusScore int `json:"response_status_score"` // 10 points max
	CompletenessScore   int `json:"completeness_score"`    // 20 points max
	FieldTypeScore      int `json:"field_type_score"`      // 10 points max
	TotalScore          int `json:"total_score"`           // 60 points max

	// Parsed response (nil if JSON parsing failed)
	ParsedResponse map[string]interface{} `json:"-"`

	// Issues found during validation
	Issues []string `json:"issues"`
}

// =============================================================================
// Phase 3: Structural Check Entry Point
// =============================================================================

// checkStructure performs all structural checks for a tagged conversation.
// Returns a StructureCheckResult with scores and issues.
func checkStructure(tc TaggedConversation) StructureCheckResult {
	result := StructureCheckResult{
		ConversationID: tc.Conversation.ID.String(),
		PromptType:     tc.PromptType,
		TargetTable:    tc.TargetTable,
		Issues:         []string{},
	}

	// 3.1 JSON Parsing (20 points)
	result.JSONParseScore, result.ParsedResponse = checkJSONParsing(tc.Conversation.ResponseContent, &result.Issues)

	// 3.2 Response Status (10 points)
	result.ResponseStatusScore = checkResponseStatus(tc.Conversation, &result.Issues)

	// Only continue with content checks if JSON parsed successfully
	if result.ParsedResponse != nil {
		// 3.3 Completeness Check (20 points)
		result.CompletenessScore = checkCompleteness(tc.PromptType, result.ParsedResponse, &result.Issues)

		// 3.4 Field Type Validation (10 points)
		result.FieldTypeScore = checkFieldTypes(tc.PromptType, result.ParsedResponse, &result.Issues)
	} else {
		// If JSON parsing failed, mark completeness and type checks as zero
		result.CompletenessScore = 0
		result.FieldTypeScore = 0
	}

	// Calculate total (max 60 points)
	result.TotalScore = result.JSONParseScore + result.ResponseStatusScore +
		result.CompletenessScore + result.FieldTypeScore

	return result
}

// =============================================================================
// 3.1 JSON Parsing Check (20 points)
// =============================================================================

// checkJSONParsing validates that response_content is valid JSON.
// Returns score (20 if valid, 0 if not) and parsed response.
func checkJSONParsing(responseContent string, issues *[]string) (int, map[string]interface{}) {
	if responseContent == "" {
		*issues = append(*issues, "Response content is empty")
		return 0, nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(responseContent), &parsed); err != nil {
		*issues = append(*issues, fmt.Sprintf("JSON parsing failed: %v", err))
		return 0, nil
	}

	return 20, parsed
}

// =============================================================================
// 3.2 Response Status Check (10 points)
// =============================================================================

// checkResponseStatus validates that the conversation status is 'success'.
// Returns score (10 if success, 0 if not).
func checkResponseStatus(conv LLMConversation, issues *[]string) int {
	if conv.Status != "success" {
		errMsg := "unknown error"
		if conv.ErrorMessage != nil && *conv.ErrorMessage != "" {
			errMsg = *conv.ErrorMessage
		}
		*issues = append(*issues, fmt.Sprintf("Response status is '%s': %s", conv.Status, errMsg))
		return 0
	}
	return 10
}

// =============================================================================
// 3.3 Completeness Check (20 points)
// =============================================================================

// RequiredFields defines required top-level fields by prompt type
var requiredFieldsByPromptType = map[PromptType][]string{
	PromptTypeEntityAnalysis:        {"business_name", "description", "domain", "key_columns", "questions"},
	PromptTypeTier1Batch:            {"entity_summaries"},
	PromptTypeTier0Domain:           {"domain_summary"},
	PromptTypeDescriptionProcessing: {"entity_hints"},
}

// checkCompleteness validates that required top-level fields are present.
// Penalty: -4 points per missing field (20 / 5 max fields = 4 points each)
// Returns score (20 max, minimum 0).
func checkCompleteness(promptType PromptType, parsed map[string]interface{}, issues *[]string) int {
	requiredFields, ok := requiredFieldsByPromptType[promptType]
	if !ok {
		// Unknown prompt type - can't validate completeness, give full score
		return 20
	}

	score := 20
	penaltyPerField := 20 / len(requiredFields)
	if penaltyPerField < 1 {
		penaltyPerField = 1
	}

	for _, field := range requiredFields {
		if _, exists := parsed[field]; !exists {
			*issues = append(*issues, fmt.Sprintf("Missing required field: %s", field))
			score -= penaltyPerField
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

// =============================================================================
// 3.4 Field Type Validation (10 points)
// =============================================================================

// FieldTypeSpec specifies expected type for a field
type FieldTypeSpec struct {
	Field    string
	Expected string // "string", "array", "object"
}

// ExpectedFieldTypes defines expected types by prompt type
var expectedFieldTypesByPromptType = map[PromptType][]FieldTypeSpec{
	PromptTypeEntityAnalysis: {
		{Field: "business_name", Expected: "string"},
		{Field: "description", Expected: "string"},
		{Field: "domain", Expected: "string"},
		{Field: "key_columns", Expected: "array"},
		{Field: "questions", Expected: "array"},
	},
	PromptTypeTier1Batch: {
		{Field: "entity_summaries", Expected: "object"},
	},
	PromptTypeTier0Domain: {
		{Field: "domain_summary", Expected: "object"},
	},
	PromptTypeDescriptionProcessing: {
		{Field: "entity_hints", Expected: "array"},
	},
}

// checkFieldTypes validates that fields have expected types.
// Penalty: -2 points per type mismatch (max -10)
// Returns score (10 max, minimum 0).
func checkFieldTypes(promptType PromptType, parsed map[string]interface{}, issues *[]string) int {
	expectedTypes, ok := expectedFieldTypesByPromptType[promptType]
	if !ok {
		// Unknown prompt type - can't validate types, give full score
		return 10
	}

	score := 10
	for _, spec := range expectedTypes {
		value, exists := parsed[spec.Field]
		if !exists {
			// Field missing - already penalized in completeness check
			continue
		}

		actualType := getJSONType(value)
		if actualType != spec.Expected {
			*issues = append(*issues, fmt.Sprintf("Field '%s' has type '%s', expected '%s'",
				spec.Field, actualType, spec.Expected))
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

// getJSONType returns the JSON type name for a value
func getJSONType(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

// =============================================================================
// Aggregate Structure Check Results
// =============================================================================

// StructureCheckSummary aggregates results from all structural checks
type StructureCheckSummary struct {
	ConversationsChecked int                    `json:"conversations_checked"`
	ConversationsPassed  int                    `json:"conversations_passed"`
	AverageScore         float64                `json:"average_score"` // Average of TotalScore (max 60)
	TotalIssues          int                    `json:"total_issues"`
	Results              []StructureCheckResult `json:"results"`

	// Per-check breakdown
	JSONParseFailures   int `json:"json_parse_failures"`
	StatusFailures      int `json:"status_failures"`
	CompletenessIssues  int `json:"completeness_issues"`
	FieldTypeMismatches int `json:"field_type_mismatches"`
}

// checkAllStructures runs structural checks on all tagged conversations
func checkAllStructures(tagged []TaggedConversation) StructureCheckSummary {
	summary := StructureCheckSummary{
		Results: make([]StructureCheckResult, len(tagged)),
	}

	var totalScore int
	for i, tc := range tagged {
		result := checkStructure(tc)
		summary.Results[i] = result
		summary.ConversationsChecked++
		totalScore += result.TotalScore

		// Count as passed if score is max (60)
		if result.TotalScore == 60 {
			summary.ConversationsPassed++
		}

		// Tally issue types
		summary.TotalIssues += len(result.Issues)
		if result.JSONParseScore == 0 {
			summary.JSONParseFailures++
		}
		if result.ResponseStatusScore == 0 {
			summary.StatusFailures++
		}
		if result.CompletenessScore < 20 {
			summary.CompletenessIssues++
		}
		if result.FieldTypeScore < 10 {
			summary.FieldTypeMismatches++
		}
	}

	if summary.ConversationsChecked > 0 {
		summary.AverageScore = float64(totalScore) / float64(summary.ConversationsChecked)
	}

	return summary
}

// structureScoreToPercentage converts the 60-point structure score to percentage (0-100)
func structureScoreToPercentage(totalScore int) int {
	// Max structure score is 60 (20 + 10 + 20 + 10)
	return (totalScore * 100) / 60
}

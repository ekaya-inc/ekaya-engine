// scoring.go implements Phase 7: Aggregate Scoring and Summary
// This phase combines all previous phase results into a final weighted score
// with smart summary generation and detailed issue reporting.
package main

import (
	"fmt"
	"sort"
	"strings"
)

// =============================================================================
// Category Weights (must sum to 100)
// =============================================================================

const (
	// ScoreStructureWeight is the weight for JSON parsing and field presence checks
	ScoreStructureWeight = 25

	// ScoreHallucinationWeight is the weight for hallucination detection - most critical
	ScoreHallucinationWeight = 50

	// ScoreValueValidityWeight is the weight for priority ranges and non-empty strings
	ScoreValueValidityWeight = 15

	// ScoreErrorRateWeight is the weight for percentage of successful calls
	ScoreErrorRateWeight = 10
)

// =============================================================================
// Data Types for Phase 7: Aggregate Scoring
// =============================================================================

// CategoryScore contains scoring details for a single assessment category
type CategoryScore struct {
	Score                int      `json:"score"`
	ConversationsChecked int      `json:"conversations_checked,omitempty"`
	ConversationsPassed  int      `json:"conversations_passed,omitempty"`
	TotalHallucinations  int      `json:"total_hallucinations,omitempty"`
	Successful           int      `json:"successful,omitempty"`
	Failed               int      `json:"failed,omitempty"`
	Issues               []string `json:"issues"`
}

// ChecksSummary contains scores for all assessment categories
type ChecksSummary struct {
	Structure      *CategoryScore `json:"structure"`
	Hallucinations *CategoryScore `json:"hallucinations"`
	ValueValidity  *CategoryScore `json:"value_validity"`
	ErrorRate      *CategoryScore `json:"error_rate"`
}

// ScoringResult contains the final aggregate scoring output
type ScoringResult struct {
	ChecksSummary ChecksSummary `json:"checks_summary"`
	FinalScore    int           `json:"final_score"`
	SmartSummary  string        `json:"smart_summary"`
}

// ConversationScore tracks the aggregate score for each conversation
type ConversationScore struct {
	ConversationID     string   `json:"conversation_id"`
	PromptType         string   `json:"prompt_type"`
	StructureScore     int      `json:"structure_score"`     // 0-100
	HallucinationScore int      `json:"hallucination_score"` // 0-100
	ValueScore         int      `json:"value_score"`         // 0-100
	OverallScore       int      `json:"overall_score"`       // 0-100
	Issues             []string `json:"issues"`
}

// =============================================================================
// Phase 7: Aggregate Scoring Entry Point
// =============================================================================

// calculateFinalScoring computes the final weighted score from all phase results.
func calculateFinalScoring(
	structureSummary StructureCheckSummary,
	hallucinationReport HallucinationReport,
	valueSummary ValueValidationSummary,
	tokenMetrics TokenMetrics,
	tagged []TaggedConversation,
) ScoringResult {
	result := ScoringResult{}

	// 7.1 Build per-conversation scores
	conversationScores := buildConversationScores(
		structureSummary,
		hallucinationReport,
		valueSummary,
		tagged,
	)

	// 7.2 Calculate category scores and final score
	result.ChecksSummary = buildChecksSummary(
		structureSummary,
		hallucinationReport,
		valueSummary,
		tagged,
	)

	// 7.2 Calculate final weighted score
	result.FinalScore = calculateWeightedScore(
		result.ChecksSummary.Structure.Score,
		result.ChecksSummary.Hallucinations.Score,
		result.ChecksSummary.ValueValidity.Score,
		result.ChecksSummary.ErrorRate.Score,
	)

	// 7.3 Generate smart summary
	result.SmartSummary = generateSmartSummary(
		result.FinalScore,
		result.ChecksSummary,
		conversationScores,
	)

	return result
}

// =============================================================================
// 7.1 Per-Conversation Scoring
// =============================================================================

// buildConversationScores creates aggregate scores for each conversation.
// Each conversation gets score 0-100 based on:
// - Structure: 40 points (JSON valid, status success, fields present, types correct)
// - Hallucinations: 40 points (no hallucinated entities)
// - Value validity: 20 points (enums, priorities, non-empty strings)
func buildConversationScores(
	structureSummary StructureCheckSummary,
	hallucinationReport HallucinationReport,
	valueSummary ValueValidationSummary,
	tagged []TaggedConversation,
) []ConversationScore {
	scores := make([]ConversationScore, 0, len(tagged))

	// Build lookup maps for value validation results
	valueResultsByID := make(map[string]ValueValidationResult)
	for _, r := range valueSummary.Results {
		valueResultsByID[r.ConversationID] = r
	}

	// Process each conversation
	for i, tc := range tagged {
		if i >= len(structureSummary.Results) {
			continue
		}

		structResult := structureSummary.Results[i]
		convID := tc.Conversation.ID.String()

		cs := ConversationScore{
			ConversationID: convID,
			PromptType:     string(tc.PromptType),
			Issues:         []string{},
		}

		// Structure score: convert 60-point scale to 40-point contribution
		// structResult.TotalScore is 0-60, we want 0-40
		cs.StructureScore = (structResult.TotalScore * 100) / 60
		cs.Issues = append(cs.Issues, structResult.Issues...)

		// Hallucination score: This is per-conversation but we only have aggregate data
		// For simplicity, use the global hallucination score for all conversations
		// (A more sophisticated approach would track hallucinations per conversation)
		cs.HallucinationScore = hallucinationReport.Score

		// Value score: lookup by conversation ID
		if valueResult, exists := valueResultsByID[convID]; exists {
			// valueResult.TotalScore is 0-30, convert to 0-100
			cs.ValueScore = (valueResult.TotalScore * 100) / 30
			cs.Issues = append(cs.Issues, valueResult.Issues...)
		} else {
			// No value validation done (e.g., non-entity_analysis prompt type)
			cs.ValueScore = 100 // Assume perfect if not applicable
		}

		// Calculate overall score: weighted by 40/40/20 as per spec
		cs.OverallScore = (cs.StructureScore*40 + cs.HallucinationScore*40 + cs.ValueScore*20) / 100

		scores = append(scores, cs)
	}

	return scores
}

// =============================================================================
// 7.2 Final Score Calculation
// =============================================================================

// buildChecksSummary creates the per-category breakdown from all phase results.
func buildChecksSummary(
	structureSummary StructureCheckSummary,
	hallucinationReport HallucinationReport,
	valueSummary ValueValidationSummary,
	tagged []TaggedConversation,
) ChecksSummary {
	summary := ChecksSummary{}

	// Structure category
	structureScore := 100
	if structureSummary.ConversationsChecked > 0 {
		// Convert average 60-point score to percentage
		structureScore = structureScoreToPercentage(int(structureSummary.AverageScore))
	}
	structureIssues := buildStructureIssues(structureSummary)
	summary.Structure = &CategoryScore{
		Score:                structureScore,
		ConversationsChecked: structureSummary.ConversationsChecked,
		ConversationsPassed:  structureSummary.ConversationsPassed,
		Issues:               structureIssues,
	}

	// Hallucinations category
	hallucinationIssues := buildHallucinationIssues(hallucinationReport)
	summary.Hallucinations = &CategoryScore{
		Score:                hallucinationReport.Score,
		ConversationsChecked: hallucinationReport.ConversationsChecked,
		TotalHallucinations:  hallucinationReport.TotalHallucinations,
		Issues:               hallucinationIssues,
	}

	// Value validity category
	valueScore := 100
	if len(valueSummary.Results) > 0 {
		// Convert average 30-point score to percentage
		valueScore = valueScoreToPercentage(int(valueSummary.AverageScore))
	}
	valueIssues := buildValueIssues(valueSummary)
	summary.ValueValidity = &CategoryScore{
		Score:                valueScore,
		ConversationsChecked: valueSummary.ConversationsChecked,
		ConversationsPassed:  valueSummary.ConversationsPassed,
		Issues:               valueIssues,
	}

	// Error rate category
	totalCalls, successfulCalls, failedCalls := countCallSuccess(tagged)
	errorRateScore := 100
	if totalCalls > 0 {
		errorRateScore = (successfulCalls * 100) / totalCalls
	}
	errorRateIssues := []string{}
	if failedCalls > 0 {
		errorRateIssues = append(errorRateIssues,
			fmt.Sprintf("%d/%d conversations failed", failedCalls, totalCalls))
	}
	summary.ErrorRate = &CategoryScore{
		Score:      errorRateScore,
		Successful: successfulCalls,
		Failed:     failedCalls,
		Issues:     errorRateIssues,
	}

	return summary
}

// calculateWeightedScore computes the final score using category weights.
// Formula: (structure * 25 + hallucination * 50 + value * 15 + errorRate * 10) / 100
func calculateWeightedScore(structureScore, hallucinationScore, valueScore, errorRateScore int) int {
	weightedSum := structureScore*ScoreStructureWeight +
		hallucinationScore*ScoreHallucinationWeight +
		valueScore*ScoreValueValidityWeight +
		errorRateScore*ScoreErrorRateWeight

	// Weights sum to 100, so divide by 100 to get final score
	return weightedSum / 100
}

// countCallSuccess counts successful and failed calls from tagged conversations.
func countCallSuccess(tagged []TaggedConversation) (total, successful, failed int) {
	for _, tc := range tagged {
		total++
		if tc.Conversation.Status == "success" {
			successful++
		} else {
			failed++
		}
	}
	return total, successful, failed
}

// =============================================================================
// 7.3 Smart Summary Generation
// =============================================================================

// generateSmartSummary creates a one-liner highlighting top issues.
func generateSmartSummary(
	finalScore int,
	summary ChecksSummary,
	conversationScores []ConversationScore,
) string {
	// Perfect score case
	if finalScore == 100 {
		return "Score 100/100 - Perfect! No hallucinations, all responses valid."
	}

	// Collect top issues by severity
	issues := collectTopIssues(summary)

	// Build summary message
	var parts []string
	parts = append(parts, fmt.Sprintf("Score %d/100", finalScore))

	if len(issues) > 0 {
		// Join top 2-3 issues
		maxIssues := 3
		if len(issues) < maxIssues {
			maxIssues = len(issues)
		}
		parts = append(parts, strings.Join(issues[:maxIssues], ", "))
	}

	return strings.Join(parts, " - ")
}

// issueWithPriority pairs an issue message with its severity priority
type issueWithPriority struct {
	message  string
	priority int // Lower is more severe
}

// collectTopIssues gathers the most significant issues across all categories.
func collectTopIssues(summary ChecksSummary) []string {
	var prioritizedIssues []issueWithPriority

	// Hallucinations are most critical (weight 50)
	if summary.Hallucinations != nil && summary.Hallucinations.TotalHallucinations > 0 {
		msg := fmt.Sprintf("%d hallucination(s)", summary.Hallucinations.TotalHallucinations)
		prioritizedIssues = append(prioritizedIssues, issueWithPriority{msg, 1})
	}

	// Structure issues (weight 25)
	if summary.Structure != nil {
		checksPassed := summary.Structure.ConversationsPassed
		checksTotal := summary.Structure.ConversationsChecked
		if checksTotal > 0 && checksPassed < checksTotal {
			failedCount := checksTotal - checksPassed
			msg := fmt.Sprintf("%d/%d responses with structural issues", failedCount, checksTotal)
			prioritizedIssues = append(prioritizedIssues, issueWithPriority{msg, 2})
		}
	}

	// Value validity issues (weight 15)
	if summary.ValueValidity != nil {
		checksPassed := summary.ValueValidity.ConversationsPassed
		checksTotal := summary.ValueValidity.ConversationsChecked
		if checksTotal > 0 && checksPassed < checksTotal {
			failedCount := checksTotal - checksPassed
			msg := fmt.Sprintf("%d/%d responses with value issues", failedCount, checksTotal)
			prioritizedIssues = append(prioritizedIssues, issueWithPriority{msg, 3})
		}
	}

	// Error rate issues (weight 10)
	if summary.ErrorRate != nil && summary.ErrorRate.Failed > 0 {
		msg := fmt.Sprintf("%d failed conversation(s)", summary.ErrorRate.Failed)
		prioritizedIssues = append(prioritizedIssues, issueWithPriority{msg, 4})
	}

	// Sort by priority (most severe first)
	sort.Slice(prioritizedIssues, func(i, j int) bool {
		return prioritizedIssues[i].priority < prioritizedIssues[j].priority
	})

	// Extract just the messages
	result := make([]string, len(prioritizedIssues))
	for i, ip := range prioritizedIssues {
		result[i] = ip.message
	}
	return result
}

// =============================================================================
// 7.4 Detailed Issue Reporting Helpers
// =============================================================================

// buildStructureIssues creates summary issues for structure checks.
func buildStructureIssues(summary StructureCheckSummary) []string {
	var issues []string

	if summary.JSONParseFailures > 0 {
		issues = append(issues, fmt.Sprintf("%d responses had invalid JSON", summary.JSONParseFailures))
	}
	if summary.StatusFailures > 0 {
		issues = append(issues, fmt.Sprintf("%d responses had status failures", summary.StatusFailures))
	}
	if summary.CompletenessIssues > 0 {
		issues = append(issues, fmt.Sprintf("%d responses missing required fields", summary.CompletenessIssues))
	}
	if summary.FieldTypeMismatches > 0 {
		issues = append(issues, fmt.Sprintf("%d responses had field type mismatches", summary.FieldTypeMismatches))
	}

	return issues
}

// buildHallucinationIssues creates summary issues for hallucination checks.
func buildHallucinationIssues(report HallucinationReport) []string {
	var issues []string

	if report.HallucinatedTables > 0 {
		issues = append(issues, fmt.Sprintf("%d hallucinated table(s)", report.HallucinatedTables))
	}
	if report.HallucinatedColumns > 0 {
		issues = append(issues, fmt.Sprintf("%d hallucinated column(s)", report.HallucinatedColumns))
	}
	if report.HallucinatedSources > 0 {
		issues = append(issues, fmt.Sprintf("%d question(s) with invalid source", report.HallucinatedSources))
	}

	// Include specific examples (first 5)
	issues = append(issues, report.Examples...)

	return issues
}

// buildValueIssues creates summary issues for value validation checks.
func buildValueIssues(summary ValueValidationSummary) []string {
	var issues []string

	if summary.StringFieldIssues > 0 {
		issues = append(issues, fmt.Sprintf("%d responses with empty/short string fields", summary.StringFieldIssues))
	}
	if summary.PriorityIssues > 0 {
		issues = append(issues, fmt.Sprintf("%d responses with invalid priority values", summary.PriorityIssues))
	}
	if summary.BooleanTypeIssues > 0 {
		issues = append(issues, fmt.Sprintf("%d responses with non-boolean is_required", summary.BooleanTypeIssues))
	}
	if summary.CategoryMissing > 0 {
		issues = append(issues, fmt.Sprintf("%d responses with empty category", summary.CategoryMissing))
	}

	// Add stored question issues
	if summary.InvalidPriorities > 0 {
		issues = append(issues, fmt.Sprintf("%d stored questions with invalid priority", summary.InvalidPriorities))
	}
	if summary.MissingCategories > 0 {
		issues = append(issues, fmt.Sprintf("%d stored questions with empty category", summary.MissingCategories))
	}

	return issues
}

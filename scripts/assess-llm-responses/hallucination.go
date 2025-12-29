// hallucination.go implements Phase 4: Hallucination Detection
// This is the most critical check - validating that LLM-referenced entities actually exist in the schema.
package main

import (
	"fmt"
	"strings"
)

// =============================================================================
// Data Types for Phase 4: Hallucination Detection
// =============================================================================

// HallucinationCheckResult contains results for a single conversation
type HallucinationCheckResult struct {
	ConversationID string     `json:"conversation_id"`
	PromptType     PromptType `json:"prompt_type"`
	TargetTable    string     `json:"target_table,omitempty"`

	// Hallucinations found
	HallucinatedTables  []string `json:"hallucinated_tables,omitempty"`
	HallucinatedColumns []string `json:"hallucinated_columns,omitempty"`

	// Scoring (penalties applied)
	Score int `json:"score"` // 0-100, starts at 100 and penalties reduce it
}

// HallucinationReport aggregates hallucination detection results
type HallucinationReport struct {
	TotalHallucinations  int      `json:"total_hallucinations"`
	HallucinatedTables   int      `json:"hallucinated_tables"`
	HallucinatedColumns  int      `json:"hallucinated_columns"`
	HallucinatedSources  int      `json:"hallucinated_sources"`
	Examples             []string `json:"examples"`
	Score                int      `json:"score"` // 0-100
	ConversationsChecked int      `json:"conversations_checked"`
}

// =============================================================================
// Phase 4: Hallucination Detection Entry Point
// =============================================================================

// checkAllHallucinations runs hallucination detection on all tagged conversations
// and validates question sources.
func checkAllHallucinations(
	tagged []TaggedConversation,
	structureResults []StructureCheckResult,
	questions []OntologyQuestion,
	validTables map[string]bool,
	validColumns map[string]map[string]bool,
) HallucinationReport {
	report := HallucinationReport{
		Examples: []string{},
	}

	// Track all hallucinations with detailed messages for examples
	var allHallucinations []string

	// 4.1 Entity Analysis Responses
	entityAnalysisScore, entityHallucinations := checkEntityAnalysisHallucinations(
		tagged, structureResults, validTables, validColumns,
	)
	allHallucinations = append(allHallucinations, entityHallucinations...)

	// 4.2 Tier1 Batch Responses
	tier1Score, tier1TableHallucinations, tier1ColumnHallucinations := checkTier1BatchHallucinations(
		tagged, structureResults, validTables, validColumns,
	)
	allHallucinations = append(allHallucinations, tier1TableHallucinations...)
	allHallucinations = append(allHallucinations, tier1ColumnHallucinations...)

	// 4.3 Question Source Validation
	questionScore, sourceHallucinations := checkQuestionSourceHallucinations(questions, validTables)
	allHallucinations = append(allHallucinations, sourceHallucinations...)

	// Count hallucinations by type
	report.HallucinatedColumns = len(entityHallucinations) + len(tier1ColumnHallucinations)
	report.HallucinatedTables = len(tier1TableHallucinations)
	report.HallucinatedSources = len(sourceHallucinations)
	report.TotalHallucinations = report.HallucinatedColumns + report.HallucinatedTables + report.HallucinatedSources

	// Store first 5 examples
	for i := 0; i < len(allHallucinations) && i < 5; i++ {
		report.Examples = append(report.Examples, allHallucinations[i])
	}

	// Count conversations checked (entity_analysis + tier1_batch)
	for _, tc := range tagged {
		if tc.PromptType == PromptTypeEntityAnalysis || tc.PromptType == PromptTypeTier1Batch {
			report.ConversationsChecked++
		}
	}

	// Calculate final score based on weighted penalties
	// Max penalty from entity analysis: 40 points
	// Max penalty from tier1 batch: 30 points
	// Max penalty from question sources: 10 points
	// Total max penalty: 80 points, but score is 0-100
	//
	// We combine the sub-scores (each 0-100) weighted by their max penalties:
	// Entity analysis: 40 points weight
	// Tier1 batch: 30 points weight
	// Question sources: 10 points weight
	// Total weight: 80 (but we normalize to 100)
	//
	// Final score = weighted average of sub-scores
	if report.ConversationsChecked > 0 || len(questions) > 0 {
		// Weight each category by its importance
		const (
			entityWeight   = 50 // Entity analysis is most critical
			tier1Weight    = 40 // Tier1 batch is important
			questionWeight = 10 // Question sources are less critical
		)
		totalWeight := entityWeight + tier1Weight + questionWeight

		report.Score = (entityAnalysisScore*entityWeight +
			tier1Score*tier1Weight +
			questionScore*questionWeight) / totalWeight
	} else {
		// No relevant conversations to check - perfect score
		report.Score = 100
	}

	return report
}

// =============================================================================
// 4.1 Entity Analysis Hallucination Detection (40 points max penalty)
// =============================================================================

// checkEntityAnalysisHallucinations checks entity_analysis responses for hallucinated columns.
// Returns a score (0-100) and list of hallucination messages.
func checkEntityAnalysisHallucinations(
	tagged []TaggedConversation,
	structureResults []StructureCheckResult,
	validTables map[string]bool,
	validColumns map[string]map[string]bool,
) (int, []string) {
	var hallucinations []string
	entityAnalysisCount := 0

	for i, tc := range tagged {
		if tc.PromptType != PromptTypeEntityAnalysis {
			continue
		}
		entityAnalysisCount++

		// Skip if JSON parsing failed (no parsed response)
		if structureResults[i].ParsedResponse == nil {
			continue
		}

		// Get the target table name (lowercase for lookup)
		tableName := strings.ToLower(tc.TargetTable)
		if tableName == "" {
			continue
		}

		// Get valid columns for this table
		tableColumns, tableExists := validColumns[tableName]
		if !tableExists {
			// Table itself doesn't exist - this is a bigger problem but
			// we're checking columns here, not tables
			continue
		}

		// Check key_columns for hallucinated columns
		parsed := structureResults[i].ParsedResponse
		keyColumns, ok := parsed["key_columns"].([]interface{})
		if !ok {
			continue
		}

		for _, kc := range keyColumns {
			colName, ok := kc.(string)
			if !ok {
				continue
			}
			colNameLower := strings.ToLower(colName)

			if !tableColumns[colNameLower] {
				hallucinations = append(hallucinations,
					fmt.Sprintf("entity_analysis '%s': key_columns references non-existent column '%s'",
						tc.TargetTable, colName))
			}
		}
	}

	// Calculate score: -10 points per hallucinated column, max -40
	// Score starts at 100, minimum 0
	if entityAnalysisCount == 0 {
		return 100, hallucinations // No entity_analysis conversations to check
	}

	penalty := len(hallucinations) * 10
	if penalty > 40 {
		penalty = 40
	}
	score := 100 - penalty
	if score < 0 {
		score = 0
	}

	return score, hallucinations
}

// =============================================================================
// 4.2 Tier1 Batch Hallucination Detection (30 points max penalty)
// =============================================================================

// checkTier1BatchHallucinations checks tier1_batch responses for hallucinated tables and columns.
// Returns a score (0-100), list of table hallucinations, and list of column hallucinations.
func checkTier1BatchHallucinations(
	tagged []TaggedConversation,
	structureResults []StructureCheckResult,
	validTables map[string]bool,
	validColumns map[string]map[string]bool,
) (int, []string, []string) {
	var tableHallucinations []string
	var columnHallucinations []string
	tier1Count := 0

	for i, tc := range tagged {
		if tc.PromptType != PromptTypeTier1Batch {
			continue
		}
		tier1Count++

		// Skip if JSON parsing failed
		if structureResults[i].ParsedResponse == nil {
			continue
		}

		parsed := structureResults[i].ParsedResponse

		// Get entity_summaries object
		entitySummaries, ok := parsed["entity_summaries"].(map[string]interface{})
		if !ok {
			continue
		}

		// Check each table in entity_summaries
		for tableName, entityData := range entitySummaries {
			tableNameLower := strings.ToLower(tableName)

			// Check if table exists in schema
			if !validTables[tableNameLower] {
				tableHallucinations = append(tableHallucinations,
					fmt.Sprintf("tier1_batch: entity_summaries references non-existent table '%s'",
						tableName))
				continue // Skip column check for non-existent table
			}

			// Check key_columns for this entity
			entityMap, ok := entityData.(map[string]interface{})
			if !ok {
				continue
			}

			keyColumns, ok := entityMap["key_columns"].([]interface{})
			if !ok {
				continue
			}

			tableColumns := validColumns[tableNameLower]
			for _, kc := range keyColumns {
				colName, ok := kc.(string)
				if !ok {
					continue
				}
				colNameLower := strings.ToLower(colName)

				if !tableColumns[colNameLower] {
					columnHallucinations = append(columnHallucinations,
						fmt.Sprintf("tier1_batch '%s': key_columns references non-existent column '%s'",
							tableName, colName))
				}
			}
		}
	}

	// Calculate score:
	// -10 points per hallucinated table (max -30)
	// -5 points per hallucinated column (max -30)
	// Combined max penalty: 30 points
	if tier1Count == 0 {
		return 100, tableHallucinations, columnHallucinations
	}

	tablePenalty := len(tableHallucinations) * 10
	if tablePenalty > 30 {
		tablePenalty = 30
	}

	columnPenalty := len(columnHallucinations) * 5
	if columnPenalty > 30 {
		columnPenalty = 30
	}

	// Total penalty capped at 30
	totalPenalty := tablePenalty + columnPenalty
	if totalPenalty > 30 {
		totalPenalty = 30
	}

	score := 100 - totalPenalty
	if score < 0 {
		score = 0
	}

	return score, tableHallucinations, columnHallucinations
}

// =============================================================================
// 4.3 Question Source Validation (10 points max penalty)
// =============================================================================

// checkQuestionSourceHallucinations validates that question source_entity_key values
// reference tables that exist in the schema.
// Returns a score (0-100) and list of hallucination messages.
func checkQuestionSourceHallucinations(
	questions []OntologyQuestion,
	validTables map[string]bool,
) (int, []string) {
	var hallucinations []string

	for _, q := range questions {
		if q.SourceEntityKey == nil || *q.SourceEntityKey == "" {
			continue
		}

		// source_entity_key is typically the table name
		sourceKey := strings.ToLower(*q.SourceEntityKey)

		if !validTables[sourceKey] {
			hallucinations = append(hallucinations,
				fmt.Sprintf("question '%s': source_entity_key '%s' references non-existent table",
					truncateText(q.Text, 50), *q.SourceEntityKey))
		}
	}

	// Calculate score: -2 points per invalid source, max -10
	if len(questions) == 0 {
		return 100, hallucinations
	}

	penalty := len(hallucinations) * 2
	if penalty > 10 {
		penalty = 10
	}

	score := 100 - penalty
	if score < 0 {
		score = 0
	}

	return score, hallucinations
}

// =============================================================================
// Helper Functions
// =============================================================================

// truncateText truncates text to maxLen characters, adding "..." if truncated
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// detect.go contains prompt type detection logic for LLM conversations.
// This is reused from assess-deterministic/main.go.
package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

// =============================================================================
// Prompt Type Detection (reused from assess-deterministic)
// =============================================================================

// Pre-compiled regex patterns for performance
var (
	reAnalyzeTable = regexp.MustCompile(`(?i)analyze the table "([^"]+)"`)
)

// PromptType identifies the type of LLM prompt
type PromptType string

const (
	PromptTypeEntityAnalysis        PromptType = "entity_analysis"
	PromptTypeTier1Batch            PromptType = "tier1_batch"
	PromptTypeTier0Domain           PromptType = "tier0_domain"
	PromptTypeDescriptionProcessing PromptType = "description_processing"
	PromptTypeUnknown               PromptType = "unknown"
)

// parseMessages extracts user and system content from conversation request messages.
func parseMessages(conv LLMConversation) (userContent, systemContent string) {
	var messages []map[string]string
	if err := json.Unmarshal(conv.RequestMessages, &messages); err != nil {
		return "", ""
	}
	for _, msg := range messages {
		switch msg["role"] {
		case "user":
			userContent = msg["content"]
		case "system":
			systemContent = msg["content"]
		}
	}
	return userContent, systemContent
}

// detectPromptType analyzes request_messages to determine the prompt type.
// Returns the detected type and target table name (for entity_analysis only).
func detectPromptType(conv LLMConversation) (PromptType, string) {
	userContent, systemContent := parseMessages(conv)
	userContent = strings.ToLower(userContent)
	systemContent = strings.ToLower(systemContent)

	// Detection order matters - most specific first

	// 1. ENTITY_ANALYSIS: Single table with sample values
	// Markers: "## table schema" (singular), "question classification rules", "analyze the table"
	if strings.Contains(userContent, "## table schema") &&
		strings.Contains(userContent, "question classification rules") &&
		strings.Contains(userContent, "analyze the table") {
		targetTable := extractTableName(userContent)
		return PromptTypeEntityAnalysis, targetTable
	}

	// 2. TIER0_DOMAIN: Domain summary from entity summaries
	// Markers: "entities by domain", "entity descriptions", "domain summary" in system
	if strings.Contains(userContent, "entities by domain") &&
		strings.Contains(userContent, "entity descriptions") &&
		strings.Contains(systemContent, "domain summary") {
		return PromptTypeTier0Domain, ""
	}

	// 3. DESCRIPTION_PROCESSING: Process user's project description
	// Markers: "user's description", "database schema", "entity_hints"
	if strings.Contains(userContent, "user's description") &&
		strings.Contains(userContent, "database schema") &&
		strings.Contains(userContent, "entity_hints") {
		return PromptTypeDescriptionProcessing, ""
	}

	// 4. TIER1_BATCH: Multiple tables batch (fallback for table prompts)
	// Markers: "## tables", "entity summaries" in system
	if strings.Contains(userContent, "## tables") &&
		strings.Contains(systemContent, "entity summaries") {
		return PromptTypeTier1Batch, ""
	}

	return PromptTypeUnknown, ""
}

// extractTableName extracts the table name from an entity analysis prompt.
func extractTableName(content string) string {
	matches := reAnalyzeTable.FindStringSubmatch(content)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// tagConversations tags each conversation with its prompt type.
func tagConversations(conversations []LLMConversation) []TaggedConversation {
	tagged := make([]TaggedConversation, len(conversations))
	for i, conv := range conversations {
		promptType, targetTable := detectPromptType(conv)
		tagged[i] = TaggedConversation{
			Conversation: conv,
			PromptType:   promptType,
			TargetTable:  targetTable,
		}
	}
	return tagged
}

// countPromptTypes counts conversations by prompt type.
func countPromptTypes(tagged []TaggedConversation) map[PromptType]int {
	counts := make(map[PromptType]int)
	for _, tc := range tagged {
		counts[tc.PromptType]++
	}
	return counts
}

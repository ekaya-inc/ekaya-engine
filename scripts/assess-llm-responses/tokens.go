// tokens.go implements Phase 6: Token Efficiency Metrics
// This phase calculates aggregate token metrics and efficiency scoring.
package main

// =============================================================================
// Data Types for Phase 6: Token Efficiency Metrics
// =============================================================================

// TokenMetrics contains aggregate token usage metrics
type TokenMetrics struct {
	TotalConversations    int     `json:"total_conversations"`
	TotalTokens           int     `json:"total_tokens"`
	TotalPromptTokens     int     `json:"total_prompt_tokens"`
	TotalCompletionTokens int     `json:"total_completion_tokens"`
	AvgTokensPerConv      float64 `json:"avg_tokens_per_conv"`
	MaxTokens             int     `json:"max_tokens"`
	MaxTokensConvID       string  `json:"max_tokens_conv_id,omitempty"` // ID of conversation with max tokens
	TokensPerTable        float64 `json:"tokens_per_table"`             // Total tokens / tables analyzed
	TotalDurationMs       int64   `json:"total_duration_ms"`            // Sum of all conversation durations
	TokensPerSecond       float64 `json:"tokens_per_second"`            // Throughput: total_tokens / total_duration
	AvgTokensPerSecond    float64 `json:"avg_tokens_per_second"`        // Average of per-request tokens/sec

	// Per-prompt-type breakdowns
	ByPromptType map[PromptType]*PromptTypeTokenStats `json:"by_prompt_type"`

	// Issues found during analysis
	Issues []string `json:"issues,omitempty"`
}

// PromptTypeTokenStats contains token statistics for a specific prompt type
type PromptTypeTokenStats struct {
	Count                 int     `json:"count"`
	TotalTokens           int     `json:"total_tokens"`
	TotalPromptTokens     int     `json:"total_prompt_tokens"`
	TotalCompletionTokens int     `json:"total_completion_tokens"`
	AvgTokens             float64 `json:"avg_tokens"`
	MaxTokens             int     `json:"max_tokens"`
}

// =============================================================================
// Token Benchmark Configuration
// =============================================================================

// TokenBenchmarks defines expected token ranges by prompt type.
// These are reasonable benchmarks based on typical ontology extraction workloads.
type TokenBenchmark struct {
	MinExpected int // Lower bound of expected range
	MaxExpected int // Upper bound of expected range
}

// tokenBenchmarks maps prompt types to their expected token ranges.
// Values based on plan: entity_analysis 2000-8000, tier1_batch 4000-15000
var tokenBenchmarks = map[PromptType]TokenBenchmark{
	PromptTypeEntityAnalysis:        {MinExpected: 2000, MaxExpected: 8000},
	PromptTypeTier1Batch:            {MinExpected: 4000, MaxExpected: 15000},
	PromptTypeTier0Domain:           {MinExpected: 3000, MaxExpected: 12000},
	PromptTypeDescriptionProcessing: {MinExpected: 1000, MaxExpected: 5000},
}

// =============================================================================
// Phase 6: Token Metrics Entry Point
// =============================================================================

// calculateTokenMetrics computes aggregate token statistics from all conversations.
func calculateTokenMetrics(
	tagged []TaggedConversation,
	tableCount int,
) TokenMetrics {
	metrics := TokenMetrics{
		ByPromptType: make(map[PromptType]*PromptTypeTokenStats),
		Issues:       []string{},
	}

	// Initialize per-prompt-type stats
	for pt := range tokenBenchmarks {
		metrics.ByPromptType[pt] = &PromptTypeTokenStats{}
	}
	metrics.ByPromptType[PromptTypeUnknown] = &PromptTypeTokenStats{}

	// Track sum of per-request tokens/sec for averaging
	var sumTokensPerSec float64
	var countWithDuration int

	// Iterate through all conversations to collect token data
	for _, tc := range tagged {
		conv := tc.Conversation

		// Skip conversations without token data
		if conv.TotalTokens == nil {
			continue
		}

		metrics.TotalConversations++
		totalTokens := *conv.TotalTokens
		metrics.TotalTokens += totalTokens

		// Accumulate prompt and completion tokens if available
		if conv.PromptTokens != nil {
			metrics.TotalPromptTokens += *conv.PromptTokens
		}
		if conv.CompletionTokens != nil {
			metrics.TotalCompletionTokens += *conv.CompletionTokens
		}

		// Accumulate duration for throughput calculation
		metrics.TotalDurationMs += int64(conv.DurationMs)

		// Calculate per-request completion tokens/sec for averaging (LLM generation speed)
		if conv.DurationMs > 0 && conv.CompletionTokens != nil && *conv.CompletionTokens > 0 {
			perRequestTPS := float64(*conv.CompletionTokens) / (float64(conv.DurationMs) / 1000.0)
			sumTokensPerSec += perRequestTPS
			countWithDuration++
		}

		// Track max tokens
		if totalTokens > metrics.MaxTokens {
			metrics.MaxTokens = totalTokens
			metrics.MaxTokensConvID = conv.ID.String()
		}

		// Update per-prompt-type stats
		ptStats := metrics.ByPromptType[tc.PromptType]
		if ptStats == nil {
			ptStats = &PromptTypeTokenStats{}
			metrics.ByPromptType[tc.PromptType] = ptStats
		}
		ptStats.Count++
		ptStats.TotalTokens += totalTokens
		if conv.PromptTokens != nil {
			ptStats.TotalPromptTokens += *conv.PromptTokens
		}
		if conv.CompletionTokens != nil {
			ptStats.TotalCompletionTokens += *conv.CompletionTokens
		}
		if totalTokens > ptStats.MaxTokens {
			ptStats.MaxTokens = totalTokens
		}
	}

	// Calculate average tokens/sec per request
	if countWithDuration > 0 {
		metrics.AvgTokensPerSecond = sumTokensPerSec / float64(countWithDuration)
	}

	// Calculate averages
	if metrics.TotalConversations > 0 {
		metrics.AvgTokensPerConv = float64(metrics.TotalTokens) / float64(metrics.TotalConversations)
	}

	// Calculate per-prompt-type averages
	for _, ptStats := range metrics.ByPromptType {
		if ptStats.Count > 0 {
			ptStats.AvgTokens = float64(ptStats.TotalTokens) / float64(ptStats.Count)
		}
	}

	// Calculate tokens per table (only counting tables that were analyzed)
	if tableCount > 0 {
		metrics.TokensPerTable = float64(metrics.TotalTokens) / float64(tableCount)
	}

	// Calculate throughput (completion tokens per second - LLM generation speed)
	if metrics.TotalDurationMs > 0 && metrics.TotalCompletionTokens > 0 {
		metrics.TokensPerSecond = float64(metrics.TotalCompletionTokens) / (float64(metrics.TotalDurationMs) / 1000.0)
	}

	// Check for efficiency issues (for informational purposes, not scoring)
	checkEfficiencyIssues(metrics, &metrics.Issues)

	return metrics
}

// =============================================================================
// Efficiency Issue Detection (informational only, not scored)
// =============================================================================

// checkEfficiencyIssues identifies potential token usage issues.
// Issues are logged for informational purposes but do not affect scoring.
func checkEfficiencyIssues(metrics TokenMetrics, issues *[]string) {
	// Check each prompt type against benchmarks
	for promptType, benchmark := range tokenBenchmarks {
		ptStats := metrics.ByPromptType[promptType]
		if ptStats == nil || ptStats.Count == 0 {
			continue
		}

		// Check if average exceeds 2x the expected max (potential issue)
		threshold := benchmark.MaxExpected * 2
		if ptStats.AvgTokens > float64(threshold) {
			*issues = append(*issues, formatEfficiencyIssue(
				promptType,
				ptStats.AvgTokens,
				benchmark.MaxExpected,
			))
		}
	}
}

// formatEfficiencyIssue creates a descriptive message for a token efficiency issue.
func formatEfficiencyIssue(promptType PromptType, avgTokens float64, expectedMax int) string {
	return "prompt_type '" + string(promptType) + "' avg tokens " +
		formatFloat(avgTokens) + " exceeds 2x expected max (" +
		formatInt(expectedMax*2) + ") - suggests prompt bloat or repeated failures"
}

// formatFloat formats a float64 with one decimal place.
func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return formatInt(int(f))
	}
	// Use simple string formatting without importing strconv for consistency
	intPart := int(f)
	decPart := int((f - float64(intPart)) * 10)
	if decPart < 0 {
		decPart = -decPart
	}
	return formatInt(intPart) + "." + formatDigit(decPart)
}

// formatInt formats an integer with thousand separators would be nice,
// but for simplicity just convert to string.
func formatInt(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	result := ""
	for i > 0 {
		digit := i % 10
		result = formatDigit(digit) + result
		i /= 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

// formatDigit converts a single digit to string.
func formatDigit(d int) string {
	return string(rune('0' + d))
}

// formatPromptTypeStats converts the per-prompt-type stats map to a JSON-friendly format.
func formatPromptTypeStats(stats map[PromptType]*PromptTypeTokenStats) map[string]interface{} {
	result := make(map[string]interface{})
	for pt, s := range stats {
		if s == nil || s.Count == 0 {
			continue
		}
		result[string(pt)] = map[string]interface{}{
			"count":                   s.Count,
			"total_tokens":            s.TotalTokens,
			"total_prompt_tokens":     s.TotalPromptTokens,
			"total_completion_tokens": s.TotalCompletionTokens,
			"avg_tokens":              s.AvgTokens,
			"max_tokens":              s.MaxTokens,
		}
	}
	return result
}

// test-model-outputs tests LLM response parsing across multiple models.
// It sends the same prompt to each model and verifies the JSON extraction works.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
)

// Model defines a model endpoint to test
type Model struct {
	Name     string
	Endpoint string
	Model    string
	APIKey   string
}

var defaultModels = []Model{
	{
		Name:     "Qwen3-30B-A3B-NVFP4-self",
		Endpoint: "http://sparkone:30000/v1",
		Model:    "Qwen3-30B-A3B-NVFP4-self",
		APIKey:   "",
	},
	{
		Name:     "NVIDIA-Nemotron-3-Nano-30B-A3B-FP8",
		Endpoint: "http://sparktwo:30000/v1",
		Model:    "NVIDIA-Nemotron-3-Nano-30B-A3B-FP8",
		APIKey:   "",
	},
}

// Sample entity_analysis prompt (the most complex format)
const sampleSystemMessage = `You are a database analyst helping to understand a business domain by examining a single table in detail.
Your goal is to identify any clarifying questions that would help fully understand what this table represents and how it's used.

CRITICAL RULES:
- Focus on THIS SPECIFIC TABLE only
- Reference EXACT column names from the schema
- Focus on BUSINESS understanding, not technical details
- Only ask questions if there is genuine ambiguity or missing context
- Status/type/state columns often encode business rules worth asking about`

const samplePrompt = `## DOMAIN CONTEXT

E-commerce platform tracking orders, customers, and inventory.

## TABLE SCHEMA

Table: orders
Row count: 10000

Columns:
  - id: bigint [PK]
  - created_at: timestamp with time zone
  - customer_id: bigint [FK → customers.id]
  - status: text (values: ["pending", "shipped", "delivered", "cancelled"])
  - total_amount: numeric

## TASK

Analyze the table "orders" and determine if there are any questions that would significantly improve understanding of this entity.

## Output Format

Return ONLY raw JSON (no markdown code fences, no explanation before or after):

{
  "analysis": "Brief summary of what this table represents and any ambiguities found",
  "entity_summary": {
    "business_name": "Human-readable name for this entity",
    "description": "1-2 sentence description of what this table represents in business terms",
    "domain": "Business domain classification"
  },
  "questions": [
    {
      "text": "Question about the table?",
      "priority": 2,
      "category": "business_rules",
      "reasoning": "Why this question is important",
      "is_required": false,
      "affects_columns": ["column_name"]
    }
  ]
}`

func main() {
	// Parse flags
	timeout := flag.Duration("timeout", 120*time.Second, "Timeout for each model call")
	flag.Parse()

	// Create logger
	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	logger, _ := logConfig.Build()
	defer logger.Sync()

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("LLM Response Format Test")
	fmt.Println("Testing JSON extraction across multiple models")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	ctx := context.Background()

	results := make(map[string]TestResult)
	for _, model := range defaultModels {
		fmt.Printf("\n%s\n", strings.Repeat("-", 80))
		fmt.Printf("Testing: %s\n", model.Name)
		fmt.Printf("Endpoint: %s\n", model.Endpoint)
		fmt.Printf("%s\n\n", strings.Repeat("-", 80))

		result := testModel(ctx, model, logger, *timeout)
		results[model.Name] = result

		printResult(result)
	}

	// Print summary
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Println("SUMMARY")
	fmt.Printf("%s\n\n", strings.Repeat("=", 80))

	allPassed := true
	for name, result := range results {
		status := "✓ PASS"
		if !result.Success {
			status = "✗ FAIL"
			allPassed = false
		}
		fmt.Printf("%s: %s\n", status, name)
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
	}

	if allPassed {
		fmt.Println("\nAll models passed!")
		os.Exit(0)
	} else {
		fmt.Println("\nSome models failed.")
		os.Exit(1)
	}
}

type TestResult struct {
	Success          bool
	Error            string
	RawResponse      string
	ExtractedJSON    string
	HasEntitySummary bool
	HasQuestions     bool
	DurationMs       int64
	TokensPerSec     float64
}

func testModel(ctx context.Context, model Model, logger *zap.Logger, timeout time.Duration) TestResult {
	result := TestResult{}
	start := time.Now()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create client
	client, err := llm.NewClient(&llm.Config{
		Endpoint: model.Endpoint,
		Model:    model.Model,
		APIKey:   model.APIKey,
	}, logger)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create client: %v", err)
		return result
	}

	// Call model
	fmt.Println("Sending prompt...")
	resp, err := client.GenerateResponse(ctx, samplePrompt, sampleSystemMessage, 0.0, false)
	if err != nil {
		result.Error = fmt.Sprintf("API call failed: %v", err)
		return result
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.RawResponse = resp.Content

	// Calculate tokens/sec
	if result.DurationMs > 0 && resp.CompletionTokens > 0 {
		result.TokensPerSec = float64(resp.CompletionTokens) / (float64(result.DurationMs) / 1000.0)
	}

	// Print raw response (truncated)
	fmt.Println("\n--- Raw Response (first 800 chars) ---")
	truncated := resp.Content
	if len(truncated) > 800 {
		truncated = truncated[:800] + "..."
	}
	fmt.Println(truncated)
	fmt.Println("--- End Raw Response ---")
	fmt.Printf("\nTokens: prompt=%d, completion=%d, total=%d\n",
		resp.PromptTokens, resp.CompletionTokens, resp.TotalTokens)
	fmt.Printf("Duration: %dms, Throughput: %.1f tok/s\n", result.DurationMs, result.TokensPerSec)

	// Try to extract JSON
	fmt.Println("\n--- JSON Extraction ---")
	jsonStr, err := llm.ExtractJSON(resp.Content)
	if err != nil {
		result.Error = fmt.Sprintf("JSON extraction failed: %v", err)
		fmt.Printf("ERROR: %s\n", result.Error)
		return result
	}
	result.ExtractedJSON = jsonStr
	fmt.Println("JSON extraction: SUCCESS")

	// Parse and validate structure
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		result.Error = fmt.Sprintf("JSON parse failed: %v", err)
		return result
	}

	// Check for entity_summary
	if entitySummary, ok := parsed["entity_summary"].(map[string]interface{}); ok {
		result.HasEntitySummary = true
		fmt.Println("entity_summary: PRESENT")

		// Check nested fields
		for _, field := range []string{"business_name", "description", "domain"} {
			if val, ok := entitySummary[field].(string); ok && val != "" {
				fmt.Printf("  - %s: %q\n", field, truncateString(val, 50))
			} else {
				fmt.Printf("  - %s: MISSING or EMPTY\n", field)
			}
		}
	} else {
		fmt.Println("entity_summary: MISSING")
	}

	// Check for questions
	if questions, ok := parsed["questions"].([]interface{}); ok {
		result.HasQuestions = true
		fmt.Printf("questions: PRESENT (%d items)\n", len(questions))
	} else {
		fmt.Println("questions: MISSING or not an array")
	}

	// Determine success
	result.Success = result.HasEntitySummary && result.HasQuestions
	return result
}

func printResult(result TestResult) {
	fmt.Println("\n--- Test Result ---")
	if result.Success {
		fmt.Println("Status: ✓ PASS")
	} else {
		fmt.Println("Status: ✗ FAIL")
		if result.Error != "" {
			fmt.Printf("Error: %s\n", result.Error)
		}
	}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

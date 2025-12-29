// assess-llm-responses evaluates LLM RESPONSE quality during ontology extraction
// using ONLY deterministic checks (no LLM-as-judge).
//
// This tool assesses how well the LLM performed, not the code quality.
// Focus areas:
// - Structural validity: Is JSON parseable and well-formed?
// - Schema compliance: Does response match expected structure for prompt type?
// - Hallucination detection: Do referenced entities exist in actual schema?
// - Completeness: Are all required fields present?
// - Value validation: Are enum values valid? Priority 1-5? Domains non-empty?
//
// Usage: go run ./scripts/assess-llm-responses <project-id>
//
// Database connection: Uses standard PG* environment variables
//
// NOTE: This standalone assessment script uses direct SQL queries rather than
// the repository layer. This is intentional to keep the script self-contained
// and avoid circular dependencies.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// =============================================================================
// Data Types for Phase 1: Data Loading
// =============================================================================

// LLMConversation represents a stored LLM conversation with all fields needed for assessment
type LLMConversation struct {
	ID               uuid.UUID       `json:"id"`
	Model            string          `json:"model"`
	Endpoint         string          `json:"endpoint"`
	RequestMessages  json.RawMessage `json:"request_messages"`
	ResponseContent  string          `json:"response_content"`
	PromptTokens     *int            `json:"prompt_tokens"`
	CompletionTokens *int            `json:"completion_tokens"`
	TotalTokens      *int            `json:"total_tokens"`
	DurationMs       int             `json:"duration_ms"`
	Status           string          `json:"status"`
	ErrorMessage     *string         `json:"error_message"`
}

// SchemaTable represents a table in the schema
type SchemaTable struct {
	ID        uuid.UUID      `json:"id"`
	TableName string         `json:"table_name"`
	RowCount  *int64         `json:"row_count"`
	Columns   []SchemaColumn `json:"columns"`
}

// SchemaColumn represents a column
type SchemaColumn struct {
	ColumnName   string `json:"column_name"`
	DataType     string `json:"data_type"`
	IsPrimaryKey bool   `json:"is_primary_key"`
	IsNullable   bool   `json:"is_nullable"`
}

// Ontology represents the stored ontology
type Ontology struct {
	DomainSummary   json.RawMessage `json:"domain_summary"`
	EntitySummaries json.RawMessage `json:"entity_summaries"`
}

// OntologyQuestion represents a stored question
type OntologyQuestion struct {
	ID               uuid.UUID `json:"id"`
	Text             string    `json:"text"`
	Reasoning        *string   `json:"reasoning"`
	IsRequired       bool      `json:"is_required"`
	SourceEntityType *string   `json:"source_entity_type"`
	SourceEntityKey  *string   `json:"source_entity_key"`
	Category         *string   `json:"category"`
	Priority         *int      `json:"priority"`
}

// TaggedConversation wraps LLMConversation with detected type and extracted table
type TaggedConversation struct {
	Conversation LLMConversation
	PromptType   PromptType `json:"prompt_type"`
	TargetTable  string     `json:"target_table,omitempty"` // For entity_analysis only
}

// =============================================================================
// Main Entry Point
// =============================================================================

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <project-id>\n", os.Args[0])
		os.Exit(1)
	}

	projectID, err := uuid.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid project ID: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Connect to database
	connStr := buildConnString()
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// Get datasource name
	var datasourceName string
	if err := conn.QueryRow(ctx, `
		SELECT name FROM engine_datasources
		WHERE project_id = $1
		LIMIT 1
	`, projectID).Scan(&datasourceName); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get datasource name: %v\n", err)
		os.Exit(1)
	}

	// Get commit info
	commitInfo := getCommitInfo()

	// =========================================================================
	// Phase 1: Data Loading
	// =========================================================================
	fmt.Fprintf(os.Stderr, "Phase 1: Loading data...\n")

	// Load LLM conversations
	conversations, err := loadConversations(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load conversations: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  Loaded %d conversations\n", len(conversations))

	// Load schema tables and columns
	schema, err := loadSchema(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load schema: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  Loaded %d tables\n", len(schema))

	// Load ontology
	ontology, err := loadOntology(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load ontology: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  Ontology loaded\n")

	// Load questions
	questions, err := loadQuestions(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load questions: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  Loaded %d questions\n", len(questions))

	// Tag conversations by prompt type
	taggedConversations := tagConversations(conversations)
	promptTypeCounts := countPromptTypes(taggedConversations)

	fmt.Fprintf(os.Stderr, "  Prompt types: entity_analysis=%d, tier1_batch=%d, tier0_domain=%d, description_processing=%d, unknown=%d\n",
		promptTypeCounts[PromptTypeEntityAnalysis],
		promptTypeCounts[PromptTypeTier1Batch],
		promptTypeCounts[PromptTypeTier0Domain],
		promptTypeCounts[PromptTypeDescriptionProcessing],
		promptTypeCounts[PromptTypeUnknown])

	// Build lookup maps for hallucination detection
	validTables, validColumns := buildSchemaLookups(schema)
	fmt.Fprintf(os.Stderr, "  Built lookup maps: %d tables, %d total columns\n", len(validTables), countTotalColumns(validColumns))

	// Determine model under test (from first conversation)
	modelUnderTest := "unknown"
	if len(conversations) > 0 {
		modelUnderTest = conversations[0].Model
	}

	// =========================================================================
	// Phase 3: Per-Response Structural Checks
	// =========================================================================
	fmt.Fprintf(os.Stderr, "Phase 3: Running structural checks...\n")

	structureSummary := checkAllStructures(taggedConversations)
	fmt.Fprintf(os.Stderr, "  Checked %d conversations, %d passed (%.1f%% average score)\n",
		structureSummary.ConversationsChecked,
		structureSummary.ConversationsPassed,
		structureSummary.AverageScore)

	if structureSummary.JSONParseFailures > 0 {
		fmt.Fprintf(os.Stderr, "  JSON parse failures: %d\n", structureSummary.JSONParseFailures)
	}
	if structureSummary.StatusFailures > 0 {
		fmt.Fprintf(os.Stderr, "  Status failures: %d\n", structureSummary.StatusFailures)
	}
	if structureSummary.CompletenessIssues > 0 {
		fmt.Fprintf(os.Stderr, "  Completeness issues: %d conversations\n", structureSummary.CompletenessIssues)
	}
	if structureSummary.FieldTypeMismatches > 0 {
		fmt.Fprintf(os.Stderr, "  Field type mismatches: %d conversations\n", structureSummary.FieldTypeMismatches)
	}

	// =========================================================================
	// Phase 4: Hallucination Detection
	// =========================================================================
	fmt.Fprintf(os.Stderr, "Phase 4: Running hallucination detection...\n")

	hallucinationReport := checkAllHallucinations(
		taggedConversations,
		structureSummary.Results,
		questions,
		validTables,
		validColumns,
	)

	fmt.Fprintf(os.Stderr, "  Checked %d conversations, found %d hallucinations (score: %d/100)\n",
		hallucinationReport.ConversationsChecked,
		hallucinationReport.TotalHallucinations,
		hallucinationReport.Score)

	if hallucinationReport.HallucinatedTables > 0 {
		fmt.Fprintf(os.Stderr, "  Hallucinated tables: %d\n", hallucinationReport.HallucinatedTables)
	}
	if hallucinationReport.HallucinatedColumns > 0 {
		fmt.Fprintf(os.Stderr, "  Hallucinated columns: %d\n", hallucinationReport.HallucinatedColumns)
	}
	if hallucinationReport.HallucinatedSources > 0 {
		fmt.Fprintf(os.Stderr, "  Hallucinated question sources: %d\n", hallucinationReport.HallucinatedSources)
	}

	// =========================================================================
	// Phase 5: Value Validation
	// =========================================================================
	fmt.Fprintf(os.Stderr, "Phase 5: Running value validation...\n")

	valueSummary := checkAllValueValidation(taggedConversations, structureSummary.Results, questions)
	fmt.Fprintf(os.Stderr, "  Checked %d conversations, %d passed (%d%% average score)\n",
		valueSummary.ConversationsChecked,
		valueSummary.ConversationsPassed,
		valueScoreToPercentage(int(valueSummary.AverageScore)))

	if valueSummary.StringFieldIssues > 0 {
		fmt.Fprintf(os.Stderr, "  String field issues: %d conversations\n", valueSummary.StringFieldIssues)
	}
	if valueSummary.PriorityIssues > 0 {
		fmt.Fprintf(os.Stderr, "  Priority issues: %d conversations\n", valueSummary.PriorityIssues)
	}
	if valueSummary.BooleanTypeIssues > 0 {
		fmt.Fprintf(os.Stderr, "  Boolean type issues: %d conversations\n", valueSummary.BooleanTypeIssues)
	}
	if valueSummary.CategoryMissing > 0 {
		fmt.Fprintf(os.Stderr, "  Category missing: %d conversations\n", valueSummary.CategoryMissing)
	}
	if valueSummary.InvalidPriorities > 0 {
		fmt.Fprintf(os.Stderr, "  Stored questions with invalid priority: %d/%d\n",
			valueSummary.InvalidPriorities, valueSummary.QuestionsPriority)
	}
	if valueSummary.MissingCategories > 0 {
		fmt.Fprintf(os.Stderr, "  Stored questions with missing category: %d/%d\n",
			valueSummary.MissingCategories, valueSummary.QuestionCategories)
	}

	// =========================================================================
	// Phase 6: Token Efficiency Metrics
	// =========================================================================
	fmt.Fprintf(os.Stderr, "Phase 6: Calculating token efficiency metrics...\n")

	tokenMetrics := calculateTokenMetrics(taggedConversations, len(schema))
	fmt.Fprintf(os.Stderr, "  Total tokens: %d across %d conversations\n",
		tokenMetrics.TotalTokens, tokenMetrics.TotalConversations)
	fmt.Fprintf(os.Stderr, "  Avg tokens per conversation: %.1f\n", tokenMetrics.AvgTokensPerConv)
	fmt.Fprintf(os.Stderr, "  Max tokens in single conversation: %d\n", tokenMetrics.MaxTokens)
	fmt.Fprintf(os.Stderr, "  Tokens per table analyzed: %.1f\n", tokenMetrics.TokensPerTable)
	fmt.Fprintf(os.Stderr, "  Efficiency score: %d/10\n", tokenMetrics.EfficiencyScore)

	if len(tokenMetrics.Issues) > 0 {
		for _, issue := range tokenMetrics.Issues {
			fmt.Fprintf(os.Stderr, "  Issue: %s\n", issue)
		}
	}

	// =========================================================================
	// Phase 7: Aggregate Scoring and Summary
	// =========================================================================
	fmt.Fprintf(os.Stderr, "Phase 7: Calculating final score...\n")

	scoringResult := calculateFinalScoring(
		structureSummary,
		hallucinationReport,
		valueSummary,
		tokenMetrics,
		taggedConversations,
	)

	fmt.Fprintf(os.Stderr, "  Final score: %d/100\n", scoringResult.FinalScore)
	fmt.Fprintf(os.Stderr, "  %s\n", scoringResult.SmartSummary)

	// Suppress unused variable warnings
	_ = ontology

	// Build final output matching the plan's AssessmentResult structure
	result := map[string]interface{}{
		"commit_info":      commitInfo,
		"datasource_name":  datasourceName,
		"project_id":       projectID.String(),
		"model_under_test": modelUnderTest,

		// Phase 2: Detection
		"prompt_type_counts": promptTypeCounts,

		// Phase 3-6: Detailed phase outputs
		"data_loaded": map[string]interface{}{
			"conversations":   len(conversations),
			"tables":          len(schema),
			"questions":       len(questions),
			"ontology_loaded": ontology != nil,
		},
		"lookup_maps": map[string]interface{}{
			"valid_tables":        len(validTables),
			"total_valid_columns": countTotalColumns(validColumns),
		},
		"structure_checks": map[string]interface{}{
			"conversations_checked": structureSummary.ConversationsChecked,
			"conversations_passed":  structureSummary.ConversationsPassed,
			"average_score":         structureSummary.AverageScore,
			"average_score_pct":     structureScoreToPercentage(int(structureSummary.AverageScore)),
			"total_issues":          structureSummary.TotalIssues,
			"json_parse_failures":   structureSummary.JSONParseFailures,
			"status_failures":       structureSummary.StatusFailures,
			"completeness_issues":   structureSummary.CompletenessIssues,
			"field_type_mismatches": structureSummary.FieldTypeMismatches,
		},

		// Phase 4: Hallucination details
		"hallucination_report": map[string]interface{}{
			"total_hallucinations": hallucinationReport.TotalHallucinations,
			"hallucinated_tables":  hallucinationReport.HallucinatedTables,
			"hallucinated_columns": hallucinationReport.HallucinatedColumns,
			"hallucinated_sources": hallucinationReport.HallucinatedSources,
			"examples":             hallucinationReport.Examples,
			"score":                hallucinationReport.Score,
		},

		"value_validation": map[string]interface{}{
			"conversations_checked": valueSummary.ConversationsChecked,
			"conversations_passed":  valueSummary.ConversationsPassed,
			"average_score":         valueSummary.AverageScore,
			"average_score_pct":     valueScoreToPercentage(int(valueSummary.AverageScore)),
			"total_issues":          valueSummary.TotalIssues,
			"string_field_issues":   valueSummary.StringFieldIssues,
			"priority_issues":       valueSummary.PriorityIssues,
			"boolean_type_issues":   valueSummary.BooleanTypeIssues,
			"category_missing":      valueSummary.CategoryMissing,
			"stored_questions": map[string]interface{}{
				"priority_checked":   valueSummary.QuestionsPriority,
				"invalid_priorities": valueSummary.InvalidPriorities,
				"category_checked":   valueSummary.QuestionCategories,
				"missing_categories": valueSummary.MissingCategories,
			},
		},

		// Phase 6: Token metrics
		"token_metrics": map[string]interface{}{
			"total_conversations":     tokenMetrics.TotalConversations,
			"total_tokens":            tokenMetrics.TotalTokens,
			"total_prompt_tokens":     tokenMetrics.TotalPromptTokens,
			"total_completion_tokens": tokenMetrics.TotalCompletionTokens,
			"avg_tokens_per_conv":     tokenMetrics.AvgTokensPerConv,
			"max_tokens":              tokenMetrics.MaxTokens,
			"max_tokens_conv_id":      tokenMetrics.MaxTokensConvID,
			"tokens_per_table":        tokenMetrics.TokensPerTable,
			"efficiency_score":        tokenMetrics.EfficiencyScore,
			"by_prompt_type":          formatPromptTypeStats(tokenMetrics.ByPromptType),
			"issues":                  tokenMetrics.Issues,
		},

		// Phase 7: Final scoring
		"checks_summary": map[string]interface{}{
			"structure": map[string]interface{}{
				"score":                 scoringResult.ChecksSummary.Structure.Score,
				"conversations_checked": scoringResult.ChecksSummary.Structure.ConversationsChecked,
				"conversations_passed":  scoringResult.ChecksSummary.Structure.ConversationsPassed,
				"issues":                scoringResult.ChecksSummary.Structure.Issues,
			},
			"hallucinations": map[string]interface{}{
				"score":                scoringResult.ChecksSummary.Hallucinations.Score,
				"total_hallucinations": scoringResult.ChecksSummary.Hallucinations.TotalHallucinations,
				"issues":               scoringResult.ChecksSummary.Hallucinations.Issues,
			},
			"value_validity": map[string]interface{}{
				"score":                 scoringResult.ChecksSummary.ValueValidity.Score,
				"conversations_checked": scoringResult.ChecksSummary.ValueValidity.ConversationsChecked,
				"conversations_passed":  scoringResult.ChecksSummary.ValueValidity.ConversationsPassed,
				"issues":                scoringResult.ChecksSummary.ValueValidity.Issues,
			},
			"error_rate": map[string]interface{}{
				"score":      scoringResult.ChecksSummary.ErrorRate.Score,
				"successful": scoringResult.ChecksSummary.ErrorRate.Successful,
				"failed":     scoringResult.ChecksSummary.ErrorRate.Failed,
				"issues":     scoringResult.ChecksSummary.ErrorRate.Issues,
			},
		},
		"final_score":   scoringResult.FinalScore,
		"smart_summary": scoringResult.SmartSummary,
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
}

// =============================================================================
// Database Connection Helpers
// =============================================================================

func buildConnString() string {
	host := getEnvOrDefault("PGHOST", "localhost")
	port := getEnvOrDefault("PGPORT", "5432")
	user := getEnvOrDefault("PGUSER", "postgres")
	password := os.Getenv("PGPASSWORD")
	dbname := getEnvOrDefault("PGDATABASE", "ekaya_engine")

	connStr := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		host, port, user, dbname)
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}
	return connStr
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getCommitInfo() string {
	cmd := exec.Command("git", "describe", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// =============================================================================
// Phase 1: Data Loading Functions
// =============================================================================

// loadConversations loads all LLM conversations for a project with fields needed for assessment
func loadConversations(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]LLMConversation, error) {
	query := `
		SELECT id, model, endpoint, request_messages, COALESCE(response_content, ''),
		       prompt_tokens, completion_tokens, total_tokens,
		       duration_ms, status, error_message
		FROM engine_llm_conversations
		WHERE project_id = $1
		ORDER BY created_at ASC`

	rows, err := conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []LLMConversation
	for rows.Next() {
		var c LLMConversation
		if err := rows.Scan(
			&c.ID, &c.Model, &c.Endpoint, &c.RequestMessages, &c.ResponseContent,
			&c.PromptTokens, &c.CompletionTokens, &c.TotalTokens,
			&c.DurationMs, &c.Status, &c.ErrorMessage,
		); err != nil {
			return nil, err
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
}

// loadSchema loads schema tables and columns for a project
func loadSchema(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]SchemaTable, error) {
	tableQuery := `
		SELECT id, table_name, row_count
		FROM engine_schema_tables
		WHERE project_id = $1 AND deleted_at IS NULL AND is_selected = true
		ORDER BY table_name`

	rows, err := conn.Query(ctx, tableQuery, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []SchemaTable
	for rows.Next() {
		var t SchemaTable
		if err := rows.Scan(&t.ID, &t.TableName, &t.RowCount); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load columns for each table
	colQuery := `
		SELECT column_name, data_type, is_primary_key, is_nullable
		FROM engine_schema_columns
		WHERE schema_table_id = $1 AND deleted_at IS NULL AND is_selected = true
		ORDER BY ordinal_position`

	for i := range tables {
		colRows, err := conn.Query(ctx, colQuery, tables[i].ID)
		if err != nil {
			return nil, err
		}
		for colRows.Next() {
			var c SchemaColumn
			if err := colRows.Scan(&c.ColumnName, &c.DataType, &c.IsPrimaryKey, &c.IsNullable); err != nil {
				colRows.Close()
				return nil, err
			}
			tables[i].Columns = append(tables[i].Columns, c)
		}
		colRows.Close()
	}

	return tables, nil
}

// loadOntology loads the active ontology for a project
func loadOntology(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) (*Ontology, error) {
	query := `
		SELECT domain_summary, entity_summaries
		FROM engine_ontologies
		WHERE project_id = $1 AND is_active = true`

	var o Ontology
	err := conn.QueryRow(ctx, query, projectID).Scan(&o.DomainSummary, &o.EntitySummaries)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no active ontology found")
		}
		return nil, err
	}
	return &o, nil
}

// loadQuestions loads ontology questions for a project
func loadQuestions(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]OntologyQuestion, error) {
	query := `
		SELECT id, text, reasoning, is_required, source_entity_type, source_entity_key, category, priority
		FROM engine_ontology_questions
		WHERE project_id = $1
		ORDER BY is_required DESC, priority ASC`

	rows, err := conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []OntologyQuestion
	for rows.Next() {
		var q OntologyQuestion
		if err := rows.Scan(&q.ID, &q.Text, &q.Reasoning, &q.IsRequired, &q.SourceEntityType, &q.SourceEntityKey, &q.Category, &q.Priority); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

// =============================================================================
// Schema Lookup Maps for Hallucination Detection
// =============================================================================

// buildSchemaLookups creates lookup maps for hallucination detection.
// validTables: map[tableName]bool (lowercase)
// validColumns: map[tableName]map[columnName]bool (lowercase)
func buildSchemaLookups(schema []SchemaTable) (map[string]bool, map[string]map[string]bool) {
	validTables := make(map[string]bool)
	validColumns := make(map[string]map[string]bool)

	for _, t := range schema {
		tableLower := strings.ToLower(t.TableName)
		validTables[tableLower] = true
		validColumns[tableLower] = make(map[string]bool)

		for _, c := range t.Columns {
			colLower := strings.ToLower(c.ColumnName)
			validColumns[tableLower][colLower] = true
		}
	}

	return validTables, validColumns
}

// countTotalColumns counts total columns across all tables
func countTotalColumns(validColumns map[string]map[string]bool) int {
	total := 0
	for _, cols := range validColumns {
		total += len(cols)
	}
	return total
}

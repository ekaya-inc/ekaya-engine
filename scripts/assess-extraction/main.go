// assess-extraction evaluates LLM quality for model comparison.
//
// This tool uses LLM-as-judge to assess how well a model performed during
// ontology extraction. It evaluates the quality of extracted information
// and generated questions.
//
// A score of 100 means the model:
//   - Asked smart questions (things that can't be inferred from data)
//   - Correctly classified required vs optional questions
//   - Generated accurate, non-generic entity descriptions
//   - Correctly identified business domains and relationships
//
// Use this tool to compare models (Haiku vs Sonnet vs Opus) on the same project.
//
// Usage: go run ./scripts/assess-extraction <project-id>
//
// Requires: ANTHROPIC_API_KEY environment variable
// Database connection: Uses standard PG* environment variables
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
	"github.com/liushuangls/go-anthropic/v2"
)

// =============================================================================
// Category Weights (must sum to 100)
// =============================================================================

const (
	WeightQuestionQuality      = 30 // Key differentiator - better models ask better questions
	WeightExtractedInfoQuality = 25 // Does the model correctly infer business meaning?
	WeightDomainSummaryQuality = 20 // Are domain summaries useful and accurate?
	WeightConsistency          = 15 // Internal consistency of the ontology
	WeightEfficiency           = 10 // Token usage, completion rate
)

// Judge model to use for assessments
const JudgeModel = "claude-sonnet-4-5-20250929"

// =============================================================================
// Output Data Types
// =============================================================================

// AssessmentResult contains the full assessment output
type AssessmentResult struct {
	CommitInfo             string                 `json:"commit_info"`
	DatasourceName         string                 `json:"datasource_name"`
	ProjectID              string                 `json:"project_id"`
	ModelUnderTest         string                 `json:"model_under_test"`
	JudgeModel             string                 `json:"judge_model"`
	SchemaStats            SchemaStats            `json:"schema_stats"`
	ChecksSummary          ChecksSummary          `json:"checks_summary"`
	FinalScore             int                    `json:"final_score"`
	SmartSummary           string                 `json:"smart_summary"`
	ModelComparisonMetrics ModelComparisonMetrics `json:"model_comparison_metrics"`
	LLMJudgeCalls          int                    `json:"llm_judge_calls"`
	LLMJudgeTokens         int                    `json:"llm_judge_tokens"`
}

// SchemaStats contains basic schema statistics
type SchemaStats struct {
	TableCount        int `json:"table_count"`
	ColumnCount       int `json:"column_count"`
	RelationshipCount int `json:"relationship_count"`
}

// ChecksSummary contains scores for all assessment categories
type ChecksSummary struct {
	QuestionQuality      *QuestionQualityScore      `json:"question_quality"`
	ExtractedInfoQuality *ExtractedInfoQualityScore `json:"extracted_info_quality"`
	DomainSummaryQuality *DomainSummaryQualityScore `json:"domain_summary_quality"`
	Consistency          *ConsistencyScore          `json:"consistency"`
	Efficiency           *EfficiencyScore           `json:"efficiency"`
}

// QuestionQualityScore contains question assessment results
type QuestionQualityScore struct {
	Score              int      `json:"score"`
	Weight             int      `json:"weight"`
	QuestionsSampled   int      `json:"questions_sampled"`
	TotalQuestions     int      `json:"total_questions"`
	InferrableCount    int      `json:"inferrable_count"`
	MisclassifiedCount int      `json:"misclassified_count"`
	InsightfulCount    int      `json:"insightful_count"`
	Issues             []string `json:"issues"`
}

// ExtractedInfoQualityScore contains entity summary assessment results
type ExtractedInfoQualityScore struct {
	Score              int      `json:"score"`
	Weight             int      `json:"weight"`
	EntitiesSampled    int      `json:"entities_sampled"`
	TotalEntities      int      `json:"total_entities"`
	GenericCount       int      `json:"generic_count"`
	DomainErrors       int      `json:"domain_errors"`
	HallucinationCount int      `json:"hallucination_count"`
	InsightfulCount    int      `json:"insightful_count"`
	Issues             []string `json:"issues"`
}

// DomainSummaryQualityScore contains domain summary assessment results
type DomainSummaryQualityScore struct {
	Score                 int      `json:"score"`
	Weight                int      `json:"weight"`
	DescriptionAccuracy   int      `json:"description_accuracy"`
	DomainGroupingScore   int      `json:"domain_grouping_score"`
	RelationshipAccuracy  int      `json:"relationship_accuracy"`
	SampleQuestionQuality int      `json:"sample_question_quality"`
	Issues                []string `json:"issues"`
}

// ConsistencyScore contains consistency assessment results
type ConsistencyScore struct {
	Score                int      `json:"score"`
	Weight               int      `json:"weight"`
	CrossRefIssues       int      `json:"cross_ref_issues"`
	DomainGroupingIssues int      `json:"domain_grouping_issues"`
	Issues               []string `json:"issues"`
}

// EfficiencyScore contains efficiency metrics
type EfficiencyScore struct {
	Score             int      `json:"score"`
	Weight            int      `json:"weight"`
	TokensPerTable    float64  `json:"tokens_per_table"`
	QuestionsPerTable float64  `json:"questions_per_table"`
	CompletionRate    float64  `json:"completion_rate"`
	Issues            []string `json:"issues"`
}

// ModelComparisonMetrics contains normalized metrics for cross-project comparison
type ModelComparisonMetrics struct {
	TokensPerTable          float64 `json:"tokens_per_table"`
	QuestionsPerTable       float64 `json:"questions_per_table"`
	InsightfulQuestionsRate float64 `json:"insightful_questions_rate"`
	InferrableQuestionsRate float64 `json:"inferrable_questions_rate"`
	GenericDescriptionRate  float64 `json:"generic_description_rate"`
	HallucinationRate       float64 `json:"hallucination_rate"`
	CompletionRate          float64 `json:"completion_rate"`
}

// =============================================================================
// Database Types
// =============================================================================

// OntologyQuestion represents a stored question
type OntologyQuestion struct {
	ID               uuid.UUID `json:"id"`
	Text             string    `json:"text"`
	Reasoning        *string   `json:"reasoning"`
	Category         *string   `json:"category"`
	Priority         int       `json:"priority"`
	IsRequired       bool      `json:"is_required"`
	SourceEntityType *string   `json:"source_entity_type"`
	SourceEntityKey  *string   `json:"source_entity_key"`
	Status           string    `json:"status"`
}

// LLMConversation represents a stored conversation
type LLMConversation struct {
	ID               uuid.UUID       `json:"id"`
	Model            string          `json:"model"`
	RequestMessages  json.RawMessage `json:"request_messages"`
	ResponseContent  string          `json:"response_content"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
	DurationMs       int             `json:"duration_ms"`
	Status           string          `json:"status"`
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
}

// SchemaRelationship represents a FK relationship
type SchemaRelationship struct {
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column"`
}

// Ontology represents the stored ontology
type Ontology struct {
	DomainSummary   json.RawMessage `json:"domain_summary"`
	EntitySummaries json.RawMessage `json:"entity_summaries"`
}

// EntitySummary represents a parsed entity summary
type EntitySummary struct {
	TableName    string   `json:"table_name"`
	BusinessName string   `json:"business_name"`
	Description  string   `json:"description"`
	Domain       string   `json:"domain"`
	Synonyms     []string `json:"synonyms"`
	KeyColumns   []struct {
		Name     string   `json:"name"`
		Synonyms []string `json:"synonyms"`
	} `json:"key_columns"`
	Relationships []string `json:"relationships"`
}

// DomainSummary represents a parsed domain summary
type DomainSummary struct {
	Description       string   `json:"description"`
	Domains           []string `json:"domains"`
	RelationshipGraph []struct {
		From        string `json:"from"`
		To          string `json:"to"`
		Label       string `json:"label"`
		Cardinality string `json:"cardinality"`
	} `json:"relationship_graph"`
	SampleQuestions []string `json:"sample_questions"`
}

// =============================================================================
// Tracking for LLM Judge Usage
// =============================================================================

type judgeTracker struct {
	calls  int
	tokens int
}

func (t *judgeTracker) track(tokens int) {
	t.calls++
	t.tokens += tokens
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

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "ANTHROPIC_API_KEY environment variable required\n")
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

	// Phase 1: Load data
	fmt.Fprintf(os.Stderr, "Phase 1: Loading data...\n")

	var datasourceName string
	if err := conn.QueryRow(ctx, `
		SELECT name FROM engine_datasources
		WHERE project_id = $1
		LIMIT 1
	`, projectID).Scan(&datasourceName); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get datasource name: %v\n", err)
		os.Exit(1)
	}

	conversations, err := loadConversations(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load conversations: %v\n", err)
		os.Exit(1)
	}

	schema, err := loadSchema(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load schema: %v\n", err)
		os.Exit(1)
	}

	relationships, err := loadRelationships(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load relationships: %v\n", err)
		os.Exit(1)
	}

	ontology, err := loadOntology(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load ontology: %v\n", err)
		os.Exit(1)
	}

	questions, err := loadQuestions(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load questions: %v\n", err)
		os.Exit(1)
	}

	// Determine model under test
	modelUnderTest := "unknown"
	if len(conversations) > 0 {
		modelUnderTest = conversations[0].Model
	}

	// Calculate schema stats
	schemaStats := SchemaStats{
		TableCount:        len(schema),
		RelationshipCount: len(relationships),
	}
	for _, t := range schema {
		schemaStats.ColumnCount += len(t.Columns)
	}

	fmt.Fprintf(os.Stderr, "  Tables: %d, Columns: %d, Relationships: %d, Questions: %d\n",
		schemaStats.TableCount, schemaStats.ColumnCount, schemaStats.RelationshipCount, len(questions))

	// Create Anthropic client for assessments
	client := anthropic.NewClient(apiKey)
	tracker := &judgeTracker{}

	// Phase 2: Assess Question Quality (30%)
	fmt.Fprintf(os.Stderr, "Phase 2: Assessing question quality...\n")
	questionScore := assessQuestionQuality(ctx, client, tracker, questions, schema, ontology)

	// Phase 3: Assess Extracted Information Quality (25%)
	fmt.Fprintf(os.Stderr, "Phase 3: Assessing extracted information quality...\n")
	extractedInfoScore := assessExtractedInfoQuality(ctx, client, tracker, schema, ontology)

	// Phase 4: Assess Domain Summary Quality (20%)
	fmt.Fprintf(os.Stderr, "Phase 4: Assessing domain summary quality...\n")
	domainSummaryScore := assessDomainSummaryQuality(ctx, client, tracker, schema, relationships, ontology)

	// Phase 5: Assess Consistency (15%)
	fmt.Fprintf(os.Stderr, "Phase 5: Assessing consistency...\n")
	consistencyScore := assessConsistency(schema, relationships, ontology)

	// Phase 6: Calculate Efficiency Metrics (10%)
	fmt.Fprintf(os.Stderr, "Phase 6: Calculating efficiency metrics...\n")
	efficiencyScore := calculateEfficiencyMetrics(conversations, questions, schema)

	// Phase 7: Calculate final score and summary
	fmt.Fprintf(os.Stderr, "Phase 7: Calculating final score...\n")

	checksSummary := ChecksSummary{
		QuestionQuality:      questionScore,
		ExtractedInfoQuality: extractedInfoScore,
		DomainSummaryQuality: domainSummaryScore,
		Consistency:          consistencyScore,
		Efficiency:           efficiencyScore,
	}

	finalScore := calculateWeightedScore(checksSummary)
	smartSummary := generateSmartSummary(finalScore, checksSummary)
	comparisonMetrics := calculateComparisonMetrics(checksSummary, schemaStats)

	result := AssessmentResult{
		CommitInfo:             getCommitInfo(),
		DatasourceName:         datasourceName,
		ProjectID:              projectID.String(),
		ModelUnderTest:         modelUnderTest,
		JudgeModel:             JudgeModel,
		SchemaStats:            schemaStats,
		ChecksSummary:          checksSummary,
		FinalScore:             finalScore,
		SmartSummary:           smartSummary,
		ModelComparisonMetrics: comparisonMetrics,
		LLMJudgeCalls:          tracker.calls,
		LLMJudgeTokens:         tracker.tokens,
	}

	// Output JSON
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
}

// =============================================================================
// Data Loading Functions
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

func loadConversations(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]LLMConversation, error) {
	query := `
		SELECT id, model, request_messages, COALESCE(response_content, ''),
		       COALESCE(prompt_tokens, 0), COALESCE(completion_tokens, 0),
		       COALESCE(total_tokens, 0), COALESCE(duration_ms, 0), status
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
		if err := rows.Scan(&c.ID, &c.Model, &c.RequestMessages, &c.ResponseContent,
			&c.PromptTokens, &c.CompletionTokens, &c.TotalTokens, &c.DurationMs, &c.Status); err != nil {
			return nil, err
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
}

func loadSchema(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]SchemaTable, error) {
	tableQuery := `
		SELECT id, table_name, row_count
		FROM engine_schema_tables
		WHERE project_id = $1 AND deleted_at IS NULL
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

	colQuery := `
		SELECT column_name, data_type, is_primary_key
		FROM engine_schema_columns
		WHERE schema_table_id = $1 AND deleted_at IS NULL
		ORDER BY ordinal_position`

	for i := range tables {
		colRows, err := conn.Query(ctx, colQuery, tables[i].ID)
		if err != nil {
			return nil, err
		}
		for colRows.Next() {
			var c SchemaColumn
			if err := colRows.Scan(&c.ColumnName, &c.DataType, &c.IsPrimaryKey); err != nil {
				colRows.Close()
				return nil, err
			}
			tables[i].Columns = append(tables[i].Columns, c)
		}
		colRows.Close()
	}

	return tables, nil
}

func loadRelationships(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]SchemaRelationship, error) {
	query := `
		SELECT
			st.table_name as source_table,
			sc.column_name as source_column,
			tt.table_name as target_table,
			tc.column_name as target_column
		FROM engine_schema_relationships r
		JOIN engine_schema_tables st ON r.source_table_id = st.id
		JOIN engine_schema_columns sc ON r.source_column_id = sc.id
		JOIN engine_schema_tables tt ON r.target_table_id = tt.id
		JOIN engine_schema_columns tc ON r.target_column_id = tc.id
		WHERE r.project_id = $1 AND r.deleted_at IS NULL`

	rows, err := conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []SchemaRelationship
	for rows.Next() {
		var r SchemaRelationship
		if err := rows.Scan(&r.SourceTable, &r.SourceColumn, &r.TargetTable, &r.TargetColumn); err != nil {
			return nil, err
		}
		relationships = append(relationships, r)
	}
	return relationships, rows.Err()
}

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

func loadQuestions(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]OntologyQuestion, error) {
	query := `
		SELECT id, text, reasoning, category, priority, is_required,
		       source_entity_type, source_entity_key, status
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
		if err := rows.Scan(&q.ID, &q.Text, &q.Reasoning, &q.Category, &q.Priority,
			&q.IsRequired, &q.SourceEntityType, &q.SourceEntityKey, &q.Status); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

// =============================================================================
// Phase 2: Question Quality Assessment (30%)
// =============================================================================

func assessQuestionQuality(ctx context.Context, client *anthropic.Client, tracker *judgeTracker, questions []OntologyQuestion, schema []SchemaTable, ontology *Ontology) *QuestionQualityScore {
	score := &QuestionQualityScore{
		Weight:         WeightQuestionQuality,
		TotalQuestions: len(questions),
		Issues:         []string{},
	}

	if len(questions) == 0 {
		score.Score = 100 // No questions = nothing to penalize
		return score
	}

	// Sample questions (up to 10 or 25%)
	sampleSize := len(questions) / 4
	if sampleSize < 5 {
		sampleSize = min(5, len(questions))
	}
	if sampleSize > 10 {
		sampleSize = 10
	}

	// Select evenly distributed samples
	sampled := make([]OntologyQuestion, 0, sampleSize)
	step := len(questions) / sampleSize
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(questions) && len(sampled) < sampleSize; i += step {
		sampled = append(sampled, questions[i])
	}
	score.QuestionsSampled = len(sampled)

	// Build schema context for the judge
	schemaContext := buildSchemaContext(schema)

	// Assess each sampled question
	var inferrableCount, misclassifiedCount, insightfulCount int
	var issues []string

	for _, q := range sampled {
		result := assessSingleQuestion(ctx, client, tracker, q, schemaContext)
		if result.isInferrable {
			inferrableCount++
		}
		if result.isMisclassified {
			misclassifiedCount++
		}
		if result.isInsightful {
			insightfulCount++
		}
		if result.issue != "" {
			issues = append(issues, result.issue)
		}
	}

	score.InferrableCount = inferrableCount
	score.MisclassifiedCount = misclassifiedCount
	score.InsightfulCount = insightfulCount

	// Calculate score: start at 100, penalize issues, reward insights
	finalScore := 100
	finalScore -= inferrableCount * 10   // -10 per inferrable question
	finalScore -= misclassifiedCount * 5 // -5 per misclassified
	finalScore += insightfulCount * 5    // +5 per insightful

	// Clamp score
	if finalScore < 0 {
		finalScore = 0
	}
	if finalScore > 100 {
		finalScore = 100
	}
	score.Score = finalScore

	// Build issue summary
	if inferrableCount > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d question(s) could be inferred from data", inferrableCount))
	}
	if misclassifiedCount > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d question(s) misclassified as required/optional", misclassifiedCount))
	}
	score.Issues = append(score.Issues, issues...)

	return score
}

type questionAssessmentResult struct {
	isInferrable    bool
	isMisclassified bool
	isInsightful    bool
	issue           string
}

func assessSingleQuestion(ctx context.Context, client *anthropic.Client, tracker *judgeTracker, q OntologyQuestion, schemaContext string) questionAssessmentResult {
	prompt := fmt.Sprintf(`You are evaluating whether an LLM asked a smart question during database ontology extraction.

## Schema Context
%s

## Question Being Evaluated
Text: "%s"
Is Required: %v
Source Entity: %s

## Evaluation Criteria

1. **Could this be inferred from the schema/data?**
   - BAD questions ask about things obvious from column names (created_at, id, is_active)
   - GOOD questions ask about things that require domain knowledge (enum meanings, business rules)

2. **Is the required/optional classification correct?**
   - Required should be: genuinely unanswerable from schema (enum meanings, unclear abbreviations)
   - Optional should be: nice-to-have clarifications, confirmations

3. **Is this an insightful question?**
   - Does it identify a genuine ambiguity?
   - Would answering it significantly improve understanding?

Return JSON:
{
  "inferrable_score": 0-100,  // Higher = more inferrable from data (BAD)
  "is_misclassified": true/false,  // Is required/optional wrong?
  "should_be": "required" | "optional" | "correct",  // What it should be
  "is_insightful": true/false,  // Is this a genuinely smart question?
  "reasoning": "brief explanation"
}

Return ONLY JSON.`, schemaContext, q.Text, q.IsRequired, stringOrEmpty(q.SourceEntityKey))

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     JudgeModel,
		MaxTokens: 500,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return questionAssessmentResult{issue: fmt.Sprintf("Judge error: %v", err)}
	}

	// Track usage
	tracker.track(resp.Usage.InputTokens + resp.Usage.OutputTokens)

	// Parse response
	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)

	var result struct {
		InferrableScore int    `json:"inferrable_score"`
		IsMisclassified bool   `json:"is_misclassified"`
		ShouldBe        string `json:"should_be"`
		IsInsightful    bool   `json:"is_insightful"`
		Reasoning       string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return questionAssessmentResult{issue: fmt.Sprintf("Parse error: %v", err)}
	}

	assessment := questionAssessmentResult{
		isInferrable:    result.InferrableScore > 70,
		isMisclassified: result.IsMisclassified,
		isInsightful:    result.IsInsightful,
	}

	// Generate specific issue message
	if assessment.isInferrable {
		assessment.issue = fmt.Sprintf("Inferrable: \"%s\"", truncate(q.Text, 50))
	} else if assessment.isMisclassified {
		assessment.issue = fmt.Sprintf("Should be %s: \"%s\"", result.ShouldBe, truncate(q.Text, 50))
	}

	return assessment
}

// =============================================================================
// Phase 3: Extracted Information Quality Assessment (25%)
// =============================================================================

func assessExtractedInfoQuality(ctx context.Context, client *anthropic.Client, tracker *judgeTracker, schema []SchemaTable, ontology *Ontology) *ExtractedInfoQualityScore {
	score := &ExtractedInfoQualityScore{
		Weight:        WeightExtractedInfoQuality,
		TotalEntities: len(schema),
		Issues:        []string{},
	}

	// Parse entity summaries from ontology
	var entitySummaries map[string]EntitySummary
	if err := json.Unmarshal(ontology.EntitySummaries, &entitySummaries); err != nil {
		score.Issues = append(score.Issues, fmt.Sprintf("Failed to parse entity summaries: %v", err))
		score.Score = 50
		return score
	}

	if len(entitySummaries) == 0 {
		score.Issues = append(score.Issues, "No entity summaries found")
		score.Score = 0
		return score
	}

	// Sample entities (up to 5 or 20%)
	sampleSize := len(schema) / 5
	if sampleSize < 3 {
		sampleSize = min(3, len(schema))
	}
	if sampleSize > 5 {
		sampleSize = 5
	}

	// Select evenly distributed samples
	sampled := make([]SchemaTable, 0, sampleSize)
	step := len(schema) / sampleSize
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(schema) && len(sampled) < sampleSize; i += step {
		sampled = append(sampled, schema[i])
	}
	score.EntitiesSampled = len(sampled)

	// Assess each sampled entity
	var genericCount, domainErrors, hallucinationCount, insightfulCount int
	var issues []string

	for _, table := range sampled {
		entity, exists := entitySummaries[table.TableName]
		if !exists {
			issues = append(issues, fmt.Sprintf("Missing summary for %s", table.TableName))
			continue
		}

		result := assessSingleEntity(ctx, client, tracker, table, entity)
		if result.isGeneric {
			genericCount++
		}
		if result.hasDomainError {
			domainErrors++
		}
		if result.hasHallucination {
			hallucinationCount++
		}
		if result.isInsightful {
			insightfulCount++
		}
		if result.issue != "" {
			issues = append(issues, result.issue)
		}
	}

	score.GenericCount = genericCount
	score.DomainErrors = domainErrors
	score.HallucinationCount = hallucinationCount
	score.InsightfulCount = insightfulCount

	// Calculate score
	finalScore := 100
	finalScore -= genericCount * 5        // -5 per generic description
	finalScore -= domainErrors * 10       // -10 per domain error
	finalScore -= hallucinationCount * 15 // -15 per hallucination
	finalScore += insightfulCount * 5     // +5 per insightful inference

	if finalScore < 0 {
		finalScore = 0
	}
	if finalScore > 100 {
		finalScore = 100
	}
	score.Score = finalScore

	// Build issue summary
	if genericCount > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d entity description(s) are generic/boilerplate", genericCount))
	}
	if domainErrors > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d incorrect domain assignment(s)", domainErrors))
	}
	if hallucinationCount > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d hallucinated business purpose(s)", hallucinationCount))
	}
	score.Issues = append(score.Issues, issues...)

	return score
}

type entityAssessmentResult struct {
	isGeneric        bool
	hasDomainError   bool
	hasHallucination bool
	isInsightful     bool
	issue            string
}

func assessSingleEntity(ctx context.Context, client *anthropic.Client, tracker *judgeTracker, table SchemaTable, entity EntitySummary) entityAssessmentResult {
	// Build table schema description
	var schemaDesc strings.Builder
	schemaDesc.WriteString(fmt.Sprintf("Table: %s\n", table.TableName))
	if table.RowCount != nil {
		schemaDesc.WriteString(fmt.Sprintf("Row count: %d\n", *table.RowCount))
	}
	schemaDesc.WriteString("Columns:\n")
	for _, col := range table.Columns {
		pk := ""
		if col.IsPrimaryKey {
			pk = " [PK]"
		}
		schemaDesc.WriteString(fmt.Sprintf("  - %s: %s%s\n", col.ColumnName, col.DataType, pk))
	}

	// Build key columns list
	keyColNames := make([]string, 0, len(entity.KeyColumns))
	for _, kc := range entity.KeyColumns {
		keyColNames = append(keyColNames, kc.Name)
	}

	prompt := fmt.Sprintf(`You are evaluating whether an LLM correctly extracted business information from a database table.

## Actual Schema
%s

## LLM's Entity Summary
Business Name: %s
Description: %s
Domain: %s
Key Columns: %s
Synonyms: %v

## Evaluation Criteria

1. **Is business_name accurate?**
   - Does it correctly identify what this table represents?
   - Is it too generic (just the table name)?

2. **Is description accurate to the schema?**
   - Does it correctly describe the entity based on columns present?
   - Does it avoid making up purposes not evident in schema?
   - Is it specific enough to be useful?

3. **Is domain assignment correct?**
   - Does the domain match what the table represents?

4. **Are key_columns selections intelligent?**
   - Did it pick business-relevant columns, not just PKs?

Return JSON:
{
  "is_generic": true/false,  // Is the description too generic/boilerplate?
  "has_domain_error": true/false,  // Is the domain assignment wrong?
  "has_hallucination": true/false,  // Does it claim purposes not in schema?
  "is_insightful": true/false,  // Does it correctly identify non-obvious entity purpose?
  "reasoning": "brief explanation"
}

Return ONLY JSON.`, schemaDesc.String(), entity.BusinessName, entity.Description, entity.Domain, strings.Join(keyColNames, ", "), entity.Synonyms)

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     JudgeModel,
		MaxTokens: 500,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return entityAssessmentResult{issue: fmt.Sprintf("Judge error for %s: %v", table.TableName, err)}
	}

	tracker.track(resp.Usage.InputTokens + resp.Usage.OutputTokens)

	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)

	var result struct {
		IsGeneric        bool   `json:"is_generic"`
		HasDomainError   bool   `json:"has_domain_error"`
		HasHallucination bool   `json:"has_hallucination"`
		IsInsightful     bool   `json:"is_insightful"`
		Reasoning        string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return entityAssessmentResult{issue: fmt.Sprintf("Parse error for %s: %v", table.TableName, err)}
	}

	assessment := entityAssessmentResult{
		isGeneric:        result.IsGeneric,
		hasDomainError:   result.HasDomainError,
		hasHallucination: result.HasHallucination,
		isInsightful:     result.IsInsightful,
	}

	// Generate specific issue
	if result.HasHallucination {
		assessment.issue = fmt.Sprintf("Hallucination in %s: %s", table.TableName, truncate(result.Reasoning, 60))
	} else if result.HasDomainError {
		assessment.issue = fmt.Sprintf("Wrong domain for %s", table.TableName)
	} else if result.IsGeneric {
		assessment.issue = fmt.Sprintf("Generic description for %s", table.TableName)
	}

	return assessment
}

// =============================================================================
// Phase 4: Domain Summary Quality Assessment (20%)
// =============================================================================

func assessDomainSummaryQuality(ctx context.Context, client *anthropic.Client, tracker *judgeTracker, schema []SchemaTable, relationships []SchemaRelationship, ontology *Ontology) *DomainSummaryQualityScore {
	score := &DomainSummaryQualityScore{
		Weight: WeightDomainSummaryQuality,
		Issues: []string{},
	}

	// Parse domain summary
	var domainSummary DomainSummary
	if err := json.Unmarshal(ontology.DomainSummary, &domainSummary); err != nil {
		score.Issues = append(score.Issues, fmt.Sprintf("Failed to parse domain summary: %v", err))
		score.Score = 50
		return score
	}

	// Build schema overview
	var schemaOverview strings.Builder
	schemaOverview.WriteString("Tables:\n")
	for _, t := range schema {
		rowCount := "unknown"
		if t.RowCount != nil {
			rowCount = fmt.Sprintf("%d", *t.RowCount)
		}
		schemaOverview.WriteString(fmt.Sprintf("  - %s (%d columns, %s rows)\n", t.TableName, len(t.Columns), rowCount))
	}

	schemaOverview.WriteString("\nFK Relationships:\n")
	for _, r := range relationships {
		schemaOverview.WriteString(fmt.Sprintf("  - %s.%s -> %s.%s\n", r.SourceTable, r.SourceColumn, r.TargetTable, r.TargetColumn))
	}

	// Build relationship graph from ontology
	var graphStr strings.Builder
	for _, edge := range domainSummary.RelationshipGraph {
		graphStr.WriteString(fmt.Sprintf("  %s -> %s (%s, %s)\n", edge.From, edge.To, edge.Label, edge.Cardinality))
	}

	prompt := fmt.Sprintf(`You are evaluating the quality of a database domain summary generated by an LLM.

## Actual Schema
%s

## LLM's Domain Summary
Description: %s
Domains: %v
Relationship Graph:
%s
Sample Questions: %v

## Evaluation Criteria

1. **Description Accuracy (0-100)**
   - Does it correctly identify what this database is for?
   - Is it specific or generic boilerplate?

2. **Domain Grouping Quality (0-100)**
   - Do the domain categories make sense for this data?
   - Are there missing domains or incorrectly grouped concepts?

3. **Relationship Graph Accuracy (0-100)**
   - Do the edges match actual FK relationships?
   - Are cardinalities reasonable?

4. **Sample Question Usefulness (0-100)**
   - Could these questions be answered with SQL against this schema?
   - Do they demonstrate understanding of the data model?

Return JSON:
{
  "description_accuracy": 0-100,
  "domain_grouping_score": 0-100,
  "relationship_accuracy": 0-100,
  "sample_question_quality": 0-100,
  "issues": ["specific issues found"]
}

Return ONLY JSON.`, schemaOverview.String(), domainSummary.Description, domainSummary.Domains, graphStr.String(), domainSummary.SampleQuestions)

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     JudgeModel,
		MaxTokens: 1000,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		score.Issues = append(score.Issues, fmt.Sprintf("Judge error: %v", err))
		score.Score = 50
		return score
	}

	tracker.track(resp.Usage.InputTokens + resp.Usage.OutputTokens)

	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)

	var result struct {
		DescriptionAccuracy   int      `json:"description_accuracy"`
		DomainGroupingScore   int      `json:"domain_grouping_score"`
		RelationshipAccuracy  int      `json:"relationship_accuracy"`
		SampleQuestionQuality int      `json:"sample_question_quality"`
		Issues                []string `json:"issues"`
	}

	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		score.Issues = append(score.Issues, fmt.Sprintf("Parse error: %v", err))
		score.Score = 50
		return score
	}

	score.DescriptionAccuracy = result.DescriptionAccuracy
	score.DomainGroupingScore = result.DomainGroupingScore
	score.RelationshipAccuracy = result.RelationshipAccuracy
	score.SampleQuestionQuality = result.SampleQuestionQuality
	score.Issues = append(score.Issues, result.Issues...)

	// Calculate overall score (average of components)
	score.Score = (result.DescriptionAccuracy + result.DomainGroupingScore +
		result.RelationshipAccuracy + result.SampleQuestionQuality) / 4

	return score
}

// =============================================================================
// Phase 5: Consistency Assessment (15%)
// =============================================================================

func assessConsistency(schema []SchemaTable, relationships []SchemaRelationship, ontology *Ontology) *ConsistencyScore {
	score := &ConsistencyScore{
		Weight: WeightConsistency,
		Issues: []string{},
	}

	// Parse entity summaries
	var entitySummaries map[string]EntitySummary
	if err := json.Unmarshal(ontology.EntitySummaries, &entitySummaries); err != nil {
		score.Issues = append(score.Issues, "Failed to parse entity summaries")
		score.Score = 50
		return score
	}

	// Check 1: Cross-reference accuracy
	// Do entity relationships list match FK relationships?
	crossRefIssues := 0
	for _, rel := range relationships {
		sourceEntity, exists := entitySummaries[rel.SourceTable]
		if !exists {
			continue
		}

		// Check if relationship is mentioned in entity
		found := false
		for _, r := range sourceEntity.Relationships {
			if strings.Contains(strings.ToLower(r), strings.ToLower(rel.TargetTable)) {
				found = true
				break
			}
		}
		if !found {
			crossRefIssues++
		}
	}
	score.CrossRefIssues = crossRefIssues

	// Check 2: Domain grouping consistency
	// Are FK-related tables in the same or logically connected domains?
	domainGroupingIssues := 0
	for _, rel := range relationships {
		sourceEntity, sourceExists := entitySummaries[rel.SourceTable]
		targetEntity, targetExists := entitySummaries[rel.TargetTable]

		if sourceExists && targetExists {
			// Related tables should typically be in the same domain or related domains
			if sourceEntity.Domain != targetEntity.Domain {
				// This is a soft check - some cross-domain relationships are valid
				// Only flag if domains seem completely unrelated
				if !areDomainsRelated(sourceEntity.Domain, targetEntity.Domain) {
					domainGroupingIssues++
				}
			}
		}
	}
	score.DomainGroupingIssues = domainGroupingIssues

	// Calculate score
	totalChecks := len(relationships) * 2 // Two checks per relationship
	if totalChecks == 0 {
		score.Score = 100
		return score
	}

	totalIssues := crossRefIssues + domainGroupingIssues
	issueRate := float64(totalIssues) / float64(totalChecks)
	score.Score = int((1 - issueRate) * 100)
	if score.Score < 0 {
		score.Score = 0
	}

	// Build issue summary
	if crossRefIssues > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d relationship(s) not mentioned in entity summaries", crossRefIssues))
	}
	if domainGroupingIssues > 0 {
		score.Issues = append(score.Issues, fmt.Sprintf("%d FK-related tables in unrelated domains", domainGroupingIssues))
	}

	return score
}

func areDomainsRelated(d1, d2 string) bool {
	// Define domain relationships
	relatedDomains := map[string][]string{
		"sales":      {"customer", "product", "finance", "inventory"},
		"customer":   {"sales", "marketing", "operations"},
		"product":    {"sales", "inventory", "operations"},
		"finance":    {"sales", "operations"},
		"operations": {"sales", "customer", "product", "inventory"},
		"inventory":  {"sales", "product", "operations"},
		"marketing":  {"customer", "sales"},
		"analytics":  {"sales", "customer", "product", "finance", "operations"},
		"hr":         {"operations"},
	}

	d1Lower := strings.ToLower(d1)
	d2Lower := strings.ToLower(d2)

	if d1Lower == d2Lower {
		return true
	}

	related, exists := relatedDomains[d1Lower]
	if exists {
		for _, r := range related {
			if r == d2Lower {
				return true
			}
		}
	}

	return false
}

// =============================================================================
// Phase 6: Efficiency Metrics (10%)
// =============================================================================

func calculateEfficiencyMetrics(conversations []LLMConversation, questions []OntologyQuestion, schema []SchemaTable) *EfficiencyScore {
	score := &EfficiencyScore{
		Weight: WeightEfficiency,
		Issues: []string{},
	}

	if len(schema) == 0 {
		score.Score = 100
		return score
	}

	// Calculate tokens per table
	totalTokens := 0
	for _, c := range conversations {
		totalTokens += c.TotalTokens
	}
	score.TokensPerTable = float64(totalTokens) / float64(len(schema))

	// Calculate questions per table
	score.QuestionsPerTable = float64(len(questions)) / float64(len(schema))

	// Calculate completion rate (% of successful conversations)
	successfulCount := 0
	for _, c := range conversations {
		if c.Status == "success" {
			successfulCount++
		}
	}
	if len(conversations) > 0 {
		score.CompletionRate = float64(successfulCount) / float64(len(conversations)) * 100
	} else {
		score.CompletionRate = 100
	}

	// Score based on efficiency
	// Benchmarks (adjust based on experience):
	// - Good tokens/table: 500-2000
	// - Good questions/table: 1-3
	// - Good completion: 100%

	efficiencyScore := 100

	// Penalize if tokens/table is too high
	if score.TokensPerTable > 3000 {
		efficiencyScore -= 20
		score.Issues = append(score.Issues, "High token usage per table")
	} else if score.TokensPerTable > 2000 {
		efficiencyScore -= 10
		score.Issues = append(score.Issues, "Above-average token usage")
	}

	// Penalize if questions/table is too high (might indicate poor question targeting)
	if score.QuestionsPerTable > 5 {
		efficiencyScore -= 10
		score.Issues = append(score.Issues, "High number of questions per table")
	}

	// Penalize for failed conversations
	if score.CompletionRate < 100 {
		penalty := int((100 - score.CompletionRate) / 5) // -20 for each 100% drop
		efficiencyScore -= penalty
		score.Issues = append(score.Issues, fmt.Sprintf("%.0f%% completion rate", score.CompletionRate))
	}

	if efficiencyScore < 0 {
		efficiencyScore = 0
	}
	score.Score = efficiencyScore

	return score
}

// =============================================================================
// Phase 7: Final Score Calculation
// =============================================================================

func calculateWeightedScore(summary ChecksSummary) int {
	weightedSum := 0

	if summary.QuestionQuality != nil {
		weightedSum += summary.QuestionQuality.Score * WeightQuestionQuality
	}
	if summary.ExtractedInfoQuality != nil {
		weightedSum += summary.ExtractedInfoQuality.Score * WeightExtractedInfoQuality
	}
	if summary.DomainSummaryQuality != nil {
		weightedSum += summary.DomainSummaryQuality.Score * WeightDomainSummaryQuality
	}
	if summary.Consistency != nil {
		weightedSum += summary.Consistency.Score * WeightConsistency
	}
	if summary.Efficiency != nil {
		weightedSum += summary.Efficiency.Score * WeightEfficiency
	}

	return weightedSum / 100
}

func generateSmartSummary(finalScore int, summary ChecksSummary) string {
	if finalScore == 100 {
		return "Score 100/100 - Excellent! All questions relevant, accurate entity descriptions, good domain understanding."
	}

	var issues []string

	// Collect issues by weight priority
	if summary.QuestionQuality != nil && len(summary.QuestionQuality.Issues) > 0 {
		issues = append(issues, summary.QuestionQuality.Issues[0])
	}
	if summary.ExtractedInfoQuality != nil && len(summary.ExtractedInfoQuality.Issues) > 0 {
		issues = append(issues, summary.ExtractedInfoQuality.Issues[0])
	}
	if summary.DomainSummaryQuality != nil && len(summary.DomainSummaryQuality.Issues) > 0 {
		issues = append(issues, summary.DomainSummaryQuality.Issues[0])
	}

	// Build summary
	parts := []string{fmt.Sprintf("Score %d/100", finalScore)}
	if len(issues) > 0 {
		maxIssues := 3
		if len(issues) < maxIssues {
			maxIssues = len(issues)
		}
		parts = append(parts, strings.Join(issues[:maxIssues], ". "))
	}

	return strings.Join(parts, " - ")
}

func calculateComparisonMetrics(summary ChecksSummary, stats SchemaStats) ModelComparisonMetrics {
	metrics := ModelComparisonMetrics{}

	if summary.Efficiency != nil {
		metrics.TokensPerTable = summary.Efficiency.TokensPerTable
		metrics.QuestionsPerTable = summary.Efficiency.QuestionsPerTable
		metrics.CompletionRate = summary.Efficiency.CompletionRate / 100
	}

	if summary.QuestionQuality != nil && summary.QuestionQuality.TotalQuestions > 0 {
		total := float64(summary.QuestionQuality.TotalQuestions)
		metrics.InsightfulQuestionsRate = float64(summary.QuestionQuality.InsightfulCount) / total
		metrics.InferrableQuestionsRate = float64(summary.QuestionQuality.InferrableCount) / total
	}

	if summary.ExtractedInfoQuality != nil && summary.ExtractedInfoQuality.TotalEntities > 0 {
		total := float64(summary.ExtractedInfoQuality.TotalEntities)
		metrics.GenericDescriptionRate = float64(summary.ExtractedInfoQuality.GenericCount) / total
		metrics.HallucinationRate = float64(summary.ExtractedInfoQuality.HallucinationCount) / total
	}

	return metrics
}

// =============================================================================
// Utility Functions
// =============================================================================

func buildSchemaContext(schema []SchemaTable) string {
	var sb strings.Builder
	for _, t := range schema {
		sb.WriteString(fmt.Sprintf("Table: %s\n", t.TableName))
		for _, c := range t.Columns {
			pk := ""
			if c.IsPrimaryKey {
				pk = " [PK]"
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s%s\n", c.ColumnName, c.DataType, pk))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func extractTextFromResponse(resp anthropic.MessagesResponse) string {
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != nil {
			return *block.Text
		}
	}
	return ""
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

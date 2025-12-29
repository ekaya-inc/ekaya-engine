// assess-extraction evaluates the LLM's performance during ontology extraction.
// This tool assesses how well the model performed GIVEN the input it received.
//
// A score of 100 means the LLM did a perfect job with the input provided.
// Use this tool to compare different models (Haiku vs Sonnet vs Opus).
//
// Separate from assess-deterministic which evaluates the deterministic code
// (input preparation and post-processing).
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

// AssessmentResult contains the full assessment output
type AssessmentResult struct {
	CommitInfo          string                   `json:"commit_info"`
	DatasourceName      string                   `json:"datasource_name"`
	ProjectID           string                   `json:"project_id"`
	ModelUnderTest      string                   `json:"model_under_test"`
	LLMMetrics          LLMMetrics               `json:"llm_metrics"`
	OutputAssessments   []ConversationAssessment `json:"output_assessments"`
	QuestionsAssessment QuestionsAssessment      `json:"questions_assessment"`
	OntologyAssessment  OntologyAssessment       `json:"ontology_assessment"`
	FinalScore          int                      `json:"final_score"`
	Summary             string                   `json:"summary"`
}

type LLMMetrics struct {
	TotalConversations    int     `json:"total_conversations"`
	SuccessfulCalls       int     `json:"successful_calls"`
	FailedCalls           int     `json:"failed_calls"`
	TotalPromptTokens     int     `json:"total_prompt_tokens"`
	TotalCompletionTokens int     `json:"total_completion_tokens"`
	TotalTokens           int     `json:"total_tokens"`
	MaxPromptTokens       int     `json:"max_prompt_tokens"`
	TotalDurationMs       int     `json:"total_duration_ms"`
	AvgDurationMs         float64 `json:"avg_duration_ms"`
	TokensPerSecond       float64 `json:"tokens_per_second"`
}

type ConversationAssessment struct {
	ConversationID string   `json:"conversation_id"`
	OutputQuality  int      `json:"output_quality"` // 0-100
	Hallucinations int      `json:"hallucinations"` // Count of hallucinated items
	Issues         []string `json:"issues"`
}

type OntologyAssessment struct {
	DomainAccuracy        int      `json:"domain_accuracy"`       // 0-100
	EntityAccuracy        int      `json:"entity_accuracy"`       // 0-100
	KeyColumnAccuracy     int      `json:"key_column_accuracy"`   // 0-100
	RelationshipAccuracy  int      `json:"relationship_accuracy"` // 0-100
	OverallScore          int      `json:"overall_score"`         // 0-100
	Strengths             []string `json:"strengths"`
	Issues                []string `json:"issues"`
	HallucinationExamples []string `json:"hallucination_examples"`
}

type QuestionsAssessment struct {
	TotalQuestions    int      `json:"total_questions"`
	RequiredQuestions int      `json:"required_questions"`
	OptionalQuestions int      `json:"optional_questions"`
	QuestionRelevance int      `json:"question_relevance"` // 0-100: Are questions relevant?
	QuestionClarity   int      `json:"question_clarity"`   // 0-100: Are questions clear?
	OverallScore      int      `json:"overall_score"`      // 0-100
	Issues            []string `json:"issues"`
	Examples          []string `json:"examples"`
}

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

// Ontology represents the stored ontology
type Ontology struct {
	DomainSummary   json.RawMessage `json:"domain_summary"`
	EntitySummaries json.RawMessage `json:"entity_summaries"`
}

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

	// Load data
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

	// Get model under test from conversations
	modelUnderTest := "unknown"
	if len(conversations) > 0 {
		modelUnderTest = conversations[0].Model
	}

	// Calculate LLM metrics
	llmMetrics := calculateLLMMetrics(conversations)

	// Create Anthropic client for assessment
	client := anthropic.NewClient(apiKey)

	// Run assessments (LLM evaluates LLM output)
	fmt.Fprintf(os.Stderr, "Assessing LLM output quality...\n")
	outputAssessments := assessOutputQuality(ctx, client, conversations, schema)

	fmt.Fprintf(os.Stderr, "Assessing question quality...\n")
	questionsAssessment := assessQuestions(ctx, client, questions, schema, ontology)

	fmt.Fprintf(os.Stderr, "Assessing ontology accuracy...\n")
	ontologyAssessment := assessOntology(ctx, client, schema, ontology)

	// Calculate final score (weighted average)
	// - Output quality: 35% (did LLM correctly process each request?)
	// - Questions quality: 25% (did LLM generate good questions?)
	// - Ontology accuracy: 40% (is the final ontology accurate?)
	finalScore := calculateFinalScore(outputAssessments, questionsAssessment, ontologyAssessment)

	// Generate summary
	summary := generateSummary(modelUnderTest, outputAssessments, questionsAssessment, ontologyAssessment, finalScore)

	result := AssessmentResult{
		CommitInfo:          commitInfo,
		DatasourceName:      datasourceName,
		ProjectID:           projectID.String(),
		ModelUnderTest:      modelUnderTest,
		LLMMetrics:          llmMetrics,
		OutputAssessments:   outputAssessments,
		QuestionsAssessment: questionsAssessment,
		OntologyAssessment:  ontologyAssessment,
		FinalScore:          finalScore,
		Summary:             summary,
	}

	// Output JSON
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
}

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

func calculateLLMMetrics(conversations []LLMConversation) LLMMetrics {
	if len(conversations) == 0 {
		return LLMMetrics{}
	}

	var metrics LLMMetrics
	metrics.TotalConversations = len(conversations)

	for _, c := range conversations {
		if c.Status == "success" {
			metrics.SuccessfulCalls++
		} else {
			metrics.FailedCalls++
		}
		metrics.TotalPromptTokens += c.PromptTokens
		metrics.TotalCompletionTokens += c.CompletionTokens
		metrics.TotalTokens += c.TotalTokens
		metrics.TotalDurationMs += c.DurationMs
		if c.PromptTokens > metrics.MaxPromptTokens {
			metrics.MaxPromptTokens = c.PromptTokens
		}
	}

	n := float64(metrics.TotalConversations)
	metrics.AvgDurationMs = float64(metrics.TotalDurationMs) / n

	if metrics.TotalDurationMs > 0 {
		durationSec := float64(metrics.TotalDurationMs) / 1000.0
		metrics.TokensPerSecond = float64(metrics.TotalCompletionTokens) / durationSec
	}

	return metrics
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

func assessOutputQuality(ctx context.Context, client *anthropic.Client, conversations []LLMConversation, schema []SchemaTable) []ConversationAssessment {
	var assessments []ConversationAssessment

	// Sample up to 5 conversations for detailed assessment
	sampled := conversations
	if len(sampled) > 5 {
		sampled = []LLMConversation{
			conversations[0],
			conversations[len(conversations)/4],
			conversations[len(conversations)/2],
			conversations[3*len(conversations)/4],
			conversations[len(conversations)-1],
		}
	}

	for _, conv := range sampled {
		assessment := assessSingleConversation(ctx, client, conv, schema)
		assessments = append(assessments, assessment)
	}

	return assessments
}

func assessSingleConversation(ctx context.Context, client *anthropic.Client, conv LLMConversation, schema []SchemaTable) ConversationAssessment {
	// Build schema reference
	var schemaRef strings.Builder
	for _, t := range schema {
		schemaRef.WriteString(fmt.Sprintf("%s: ", t.TableName))
		var cols []string
		for _, c := range t.Columns {
			cols = append(cols, c.ColumnName)
		}
		schemaRef.WriteString(strings.Join(cols, ", "))
		schemaRef.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are assessing LLM output quality for an ontology extraction task.
Focus ONLY on how well the model performed given what it was provided.

## GROUND TRUTH: Actual Schema (column names per table)
%s

## MODEL INPUT: Request sent to the model being tested
%s

## MODEL OUTPUT: Response from the model being tested
%s

## ASSESSMENT TASK

Evaluate the MODEL'S OUTPUT quality. Did it:
1. Use ONLY column names that exist in the schema? (Hallucinations are severe failures)
2. Produce well-structured, parseable output?
3. Make reasonable inferences from the provided information?

Return JSON:
{
  "output_quality": 0-100,
  "hallucinations": <count of hallucinated column names or entities>,
  "issues": ["specific issues with model output"]
}

SCORING GUIDE:
- 100: Perfect output, no hallucinations, well-structured
- 80-99: Minor issues, no hallucinations
- 60-79: Some issues or 1-2 hallucinations
- 40-59: Multiple issues or several hallucinations
- 0-39: Major issues or many hallucinations

Return ONLY JSON.`, schemaRef.String(), string(conv.RequestMessages), conv.ResponseContent)

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 1000,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return ConversationAssessment{
			ConversationID: conv.ID.String(),
			Issues:         []string{fmt.Sprintf("Assessment failed: %v", err)},
		}
	}

	var result struct {
		OutputQuality  int      `json:"output_quality"`
		Hallucinations int      `json:"hallucinations"`
		Issues         []string `json:"issues"`
	}

	responseText := ""
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != nil {
			responseText = *block.Text
			break
		}
	}

	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return ConversationAssessment{
			ConversationID: conv.ID.String(),
			Issues:         []string{fmt.Sprintf("Parse error: %v", err)},
		}
	}

	return ConversationAssessment{
		ConversationID: conv.ID.String(),
		OutputQuality:  result.OutputQuality,
		Hallucinations: result.Hallucinations,
		Issues:         result.Issues,
	}
}

func assessOntology(ctx context.Context, client *anthropic.Client, schema []SchemaTable, ontology *Ontology) OntologyAssessment {
	var schemaDetail strings.Builder
	schemaDetail.WriteString("## ACTUAL DATABASE SCHEMA\n\n")
	for _, t := range schema {
		schemaDetail.WriteString(fmt.Sprintf("### %s\n", t.TableName))
		schemaDetail.WriteString(fmt.Sprintf("Columns (%d):\n", len(t.Columns)))
		for _, c := range t.Columns {
			pk := ""
			if c.IsPrimaryKey {
				pk = " [PK]"
			}
			schemaDetail.WriteString(fmt.Sprintf("  - %s (%s)%s\n", c.ColumnName, c.DataType, pk))
		}
		schemaDetail.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are assessing LLM performance in ontology extraction.
Focus ONLY on how well the model generated the ontology from the schema it was given.

%s

## GENERATED ONTOLOGY (by the model being tested)

### Domain Summary
%s

### Entity Summaries
%s

## ASSESSMENT TASK

Evaluate the MODEL'S ONTOLOGY generation:

1. **Domain Accuracy** (0-100): Does the domain summary correctly describe what CAN BE INFERRED from the schema?
2. **Entity Accuracy** (0-100): Do entity descriptions match what the schema shows?
3. **Key Column Accuracy** (0-100): Do key_columns reference ACTUAL columns from the schema?
   - Every hallucinated column name is a MAJOR penalty
   - Check each referenced column exists in the actual schema
4. **Relationship Accuracy** (0-100): Are relationships correctly identified from naming patterns?

Return JSON:
{
  "domain_accuracy": 0-100,
  "entity_accuracy": 0-100,
  "key_column_accuracy": 0-100,
  "relationship_accuracy": 0-100,
  "overall_score": 0-100,
  "strengths": ["what the model got right"],
  "issues": ["specific model failures"],
  "hallucination_examples": [
    "Entity X references column 'user_id' but actual column is 'owner_id'",
    "Entity Y includes non-existent column 'offer_value'"
  ]
}

A score of 100 means the model did a PERFECT job with the schema it was given.

Return ONLY JSON.`, schemaDetail.String(), string(ontology.DomainSummary), string(ontology.EntitySummaries))

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 4000,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return OntologyAssessment{
			Issues: []string{fmt.Sprintf("Assessment failed: %v", err)},
		}
	}

	responseText := ""
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != nil {
			responseText = *block.Text
			break
		}
	}

	var result OntologyAssessment
	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return OntologyAssessment{
			Issues: []string{fmt.Sprintf("Parse error: %v", err)},
		}
	}

	return result
}

func assessQuestions(ctx context.Context, client *anthropic.Client, questions []OntologyQuestion, schema []SchemaTable, ontology *Ontology) QuestionsAssessment {
	if len(questions) == 0 {
		return QuestionsAssessment{
			TotalQuestions:    0,
			RequiredQuestions: 0,
			OptionalQuestions: 0,
			QuestionRelevance: 100,
			QuestionClarity:   100,
			OverallScore:      100,
			Issues:            []string{},
		}
	}

	var required, optional int
	for _, q := range questions {
		if q.IsRequired {
			required++
		} else {
			optional++
		}
	}

	var schemaSummary strings.Builder
	for _, t := range schema {
		schemaSummary.WriteString(fmt.Sprintf("Table: %s\n", t.TableName))
		for _, c := range t.Columns {
			pk := ""
			if c.IsPrimaryKey {
				pk = " [PK]"
			}
			schemaSummary.WriteString(fmt.Sprintf("  - %s (%s)%s\n", c.ColumnName, c.DataType, pk))
		}
	}

	var questionsText strings.Builder
	questionsText.WriteString("## REQUIRED QUESTIONS\n")
	for _, q := range questions {
		if q.IsRequired {
			questionsText.WriteString(fmt.Sprintf("- %s\n", q.Text))
			if q.Reasoning != nil && *q.Reasoning != "" {
				questionsText.WriteString(fmt.Sprintf("  Reasoning: %s\n", *q.Reasoning))
			}
		}
	}
	questionsText.WriteString("\n## OPTIONAL QUESTIONS\n")
	for _, q := range questions {
		if !q.IsRequired {
			questionsText.WriteString(fmt.Sprintf("- %s\n", q.Text))
		}
	}

	prompt := fmt.Sprintf(`You are assessing LLM performance in generating questions during ontology extraction.
Focus on whether the MODEL generated good questions given the schema.

## DATABASE SCHEMA (what the model was given)
%s

## QUESTIONS GENERATED BY MODEL
%s

## ASSESSMENT TASK

Evaluate the MODEL'S question generation quality:

1. **Question Relevance** (0-100): Are questions relevant to understanding the schema?
   - Questions about ambiguous columns/relationships are GOOD
   - Questions about obvious things (created_at, id) are BAD
   - Repetitive questions across tables are BAD

2. **Question Clarity** (0-100): Are questions well-formed and answerable?
   - Clear, specific questions score high
   - Vague or confusing questions score low

Return JSON:
{
  "question_relevance": 0-100,
  "question_clarity": 0-100,
  "overall_score": 0-100,
  "issues": ["specific issues with model's question generation"],
  "examples": [
    "GOOD: 'What do status values 2,4,5 mean?' - asking about ambiguous enum",
    "BAD: 'What is the id column for?' - asking about obvious PK"
  ]
}

A score of 100 means the model asked exactly the right questions.

Return ONLY JSON.`, schemaSummary.String(), questionsText.String())

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 2000,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return QuestionsAssessment{
			TotalQuestions:    len(questions),
			RequiredQuestions: required,
			OptionalQuestions: optional,
			Issues:            []string{fmt.Sprintf("Assessment failed: %v", err)},
		}
	}

	responseText := ""
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != nil {
			responseText = *block.Text
			break
		}
	}

	var result struct {
		QuestionRelevance int      `json:"question_relevance"`
		QuestionClarity   int      `json:"question_clarity"`
		OverallScore      int      `json:"overall_score"`
		Issues            []string `json:"issues"`
		Examples          []string `json:"examples"`
	}

	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return QuestionsAssessment{
			TotalQuestions:    len(questions),
			RequiredQuestions: required,
			OptionalQuestions: optional,
			Issues:            []string{fmt.Sprintf("Parse error: %v", err)},
		}
	}

	return QuestionsAssessment{
		TotalQuestions:    len(questions),
		RequiredQuestions: required,
		OptionalQuestions: optional,
		QuestionRelevance: result.QuestionRelevance,
		QuestionClarity:   result.QuestionClarity,
		OverallScore:      result.OverallScore,
		Issues:            result.Issues,
		Examples:          result.Examples,
	}
}

func calculateFinalScore(outputs []ConversationAssessment, questions QuestionsAssessment, ontology OntologyAssessment) int {
	// Weights:
	// - Output quality: 35%
	// - Questions quality: 25%
	// - Ontology accuracy: 40%

	var outputAvg float64
	if len(outputs) > 0 {
		var sum int
		for _, o := range outputs {
			sum += o.OutputQuality
		}
		outputAvg = float64(sum) / float64(len(outputs))
	}

	questionsScore := questions.OverallScore
	ontologyScore := ontology.OverallScore

	finalScore := outputAvg*0.35 + float64(questionsScore)*0.25 + float64(ontologyScore)*0.40

	return int(finalScore)
}

func generateSummary(model string, outputs []ConversationAssessment, questions QuestionsAssessment, ontology OntologyAssessment, finalScore int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("LLM Extraction Assessment Score: %d/100\n", finalScore))
	sb.WriteString(fmt.Sprintf("Model Under Test: %s\n\n", model))

	// Output quality summary
	var outputAvg float64
	totalHallucinations := 0
	if len(outputs) > 0 {
		var sum int
		for _, o := range outputs {
			sum += o.OutputQuality
			totalHallucinations += o.Hallucinations
		}
		outputAvg = float64(sum) / float64(len(outputs))
	}
	sb.WriteString(fmt.Sprintf("Output Quality: %.0f/100 (%d hallucinations)\n", outputAvg, totalHallucinations))

	sb.WriteString(fmt.Sprintf("Questions Quality: %d/100\n", questions.OverallScore))
	sb.WriteString(fmt.Sprintf("Ontology Accuracy: %d/100\n", ontology.OverallScore))

	sb.WriteString("\n")

	if len(ontology.HallucinationExamples) > 0 {
		sb.WriteString("Hallucination Examples:\n")
		for _, ex := range ontology.HallucinationExamples[:min(3, len(ontology.HallucinationExamples))] {
			sb.WriteString(fmt.Sprintf("  - %s\n", ex))
		}
	}

	if finalScore >= 90 {
		sb.WriteString("\nExcellent LLM performance!")
	} else if finalScore >= 70 {
		sb.WriteString("\nGood LLM performance with some issues.")
	} else if finalScore >= 50 {
		sb.WriteString("\nModerate LLM performance - consider using a stronger model.")
	} else {
		sb.WriteString("\nPoor LLM performance - significant issues detected.")
	}

	return sb.String()
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

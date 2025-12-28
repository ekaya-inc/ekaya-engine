// assess-extraction evaluates how well the ontology extraction system worked
// with the data it was given. A score of 100 means perfect extraction - the
// system did everything possible with the available inputs.
//
// Unlike assess-ontology (which evaluates overall ontology quality including
// knowledge gaps), this tool focuses on extraction accuracy:
// - Did we extract the correct information to give to the LLM?
// - Did the LLM generate the ontology correctly from that input?
// - Are questions reasonable given what is known?
// - Is required vs optional classification appropriate?
//
// Knowledge gaps do NOT affect the score - those are input data problems,
// not extraction problems.
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
	ModelUsed           string                   `json:"model_used"`
	LLMMetrics          LLMMetrics               `json:"llm_metrics"`
	InputAssessment     InputAssessment          `json:"input_assessment"`
	OutputAssessments   []ConversationAssessment `json:"output_assessments"`
	QuestionsAssessment QuestionsAssessment      `json:"questions_assessment"`
	OntologyAssessment  OntologyAssessment       `json:"ontology_assessment"`
	FinalScore          int                      `json:"final_score"`
}

// ProjectKnowledge represents a stored knowledge fact
type ProjectKnowledge struct {
	ID       uuid.UUID `json:"id"`
	FactType string    `json:"fact_type"`
	Key      string    `json:"key"`
	Value    string    `json:"value"`
	Context  *string   `json:"context"`
}

type InputAssessment struct {
	SchemaIncluded  bool     `json:"schema_included"`
	ColumnsIncluded bool     `json:"columns_included"`
	WellFormed      bool     `json:"well_formed"`
	Issues          []string `json:"issues"`
	Score           int      `json:"score"` // 0-100
}

type ConversationAssessment struct {
	ConversationID string   `json:"conversation_id"`
	TaskType       string   `json:"task_type"`
	InputQuality   int      `json:"input_quality"`  // 0-100
	OutputQuality  int      `json:"output_quality"` // 0-100
	Issues         []string `json:"issues"`
}

type OntologyAssessment struct {
	DomainAccuracy       int      `json:"domain_accuracy"`       // 0-100
	EntityAccuracy       int      `json:"entity_accuracy"`       // 0-100
	KeyColumnAccuracy    int      `json:"key_column_accuracy"`   // 0-100
	RelationshipAccuracy int      `json:"relationship_accuracy"` // 0-100
	OverallScore         int      `json:"overall_score"`         // 0-100
	Strengths            []string `json:"strengths"`
	Issues               []string `json:"issues"`
	Examples             []string `json:"examples"` // Specific examples of good/bad
}

type QuestionsAssessment struct {
	TotalQuestions         int      `json:"total_questions"`
	RequiredQuestions      int      `json:"required_questions"`
	OptionalQuestions      int      `json:"optional_questions"`
	RequiredClassification int      `json:"required_classification"` // 0-100: Are required questions truly required?
	OptionalClassification int      `json:"optional_classification"` // 0-100: Are optional questions truly optional?
	QuestionQuality        int      `json:"question_quality"`        // 0-100: Are questions well-formed and useful?
	OverallScore           int      `json:"overall_score"`           // 0-100
	Issues                 []string `json:"issues"`
	Examples               []string `json:"examples"`
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

// LLMMetrics contains aggregated LLM performance metrics
type LLMMetrics struct {
	TotalConversations    int     `json:"total_conversations"`
	SuccessfulCalls       int     `json:"successful_calls"`
	FailedCalls           int     `json:"failed_calls"`
	TotalPromptTokens     int     `json:"total_prompt_tokens"`
	TotalCompletionTokens int     `json:"total_completion_tokens"`
	TotalTokens           int     `json:"total_tokens"`
	MaxPromptTokens       int     `json:"max_prompt_tokens"`
	TotalDurationMs       int     `json:"total_duration_ms"`
	AvgPromptTokens       float64 `json:"avg_prompt_tokens"`
	AvgCompletionTokens   float64 `json:"avg_completion_tokens"`
	AvgDurationMs         float64 `json:"avg_duration_ms"`
	TokensPerSecond       float64 `json:"tokens_per_second"`
	PromptTokensPerSec    float64 `json:"prompt_tokens_per_second"`
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

	// Get datasource name for this project
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

	// Get model used from conversations
	modelUsed := "unknown"
	if len(conversations) > 0 {
		modelUsed = conversations[0].Model
	}

	// Calculate LLM metrics
	llmMetrics := calculateLLMMetrics(conversations)

	// Create Anthropic client
	client := anthropic.NewClient(apiKey)

	// Run assessments
	fmt.Fprintf(os.Stderr, "Assessing input extraction quality...\n")
	inputAssessment := assessInputQuality(ctx, client, conversations, schema)

	fmt.Fprintf(os.Stderr, "Assessing LLM output quality...\n")
	outputAssessments := assessOutputQuality(ctx, client, conversations, schema)

	fmt.Fprintf(os.Stderr, "Assessing question classification...\n")
	questionsAssessment := assessQuestions(ctx, client, questions, schema, ontology)

	fmt.Fprintf(os.Stderr, "Assessing ontology extraction accuracy...\n")
	ontologyAssessment := assessOntology(ctx, client, schema, ontology)

	// Calculate final score (weighted average)
	finalScore := calculateFinalScore(inputAssessment, outputAssessments, questionsAssessment, ontologyAssessment)

	result := AssessmentResult{
		CommitInfo:          commitInfo,
		DatasourceName:      datasourceName,
		ProjectID:           projectID.String(),
		ModelUsed:           modelUsed,
		LLMMetrics:          llmMetrics,
		InputAssessment:     inputAssessment,
		OutputAssessments:   outputAssessments,
		QuestionsAssessment: questionsAssessment,
		OntologyAssessment:  ontologyAssessment,
		FinalScore:          finalScore,
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
		SELECT id, model, request_messages, response_content,
		       COALESCE(prompt_tokens, 0), COALESCE(completion_tokens, 0),
		       COALESCE(total_tokens, 0), duration_ms, status
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
		var responseContent *string
		if err := rows.Scan(&c.ID, &c.Model, &c.RequestMessages, &responseContent,
			&c.PromptTokens, &c.CompletionTokens, &c.TotalTokens, &c.DurationMs, &c.Status); err != nil {
			return nil, err
		}
		if responseContent != nil {
			c.ResponseContent = *responseContent
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

	// Calculate averages
	n := float64(metrics.TotalConversations)
	metrics.AvgPromptTokens = float64(metrics.TotalPromptTokens) / n
	metrics.AvgCompletionTokens = float64(metrics.TotalCompletionTokens) / n
	metrics.AvgDurationMs = float64(metrics.TotalDurationMs) / n

	// Calculate tokens per second (output generation speed)
	if metrics.TotalDurationMs > 0 {
		durationSec := float64(metrics.TotalDurationMs) / 1000.0
		metrics.TokensPerSecond = float64(metrics.TotalCompletionTokens) / durationSec
		metrics.PromptTokensPerSec = float64(metrics.TotalPromptTokens) / durationSec
	}

	return metrics
}

func loadSchema(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]SchemaTable, error) {
	// Load tables
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

	// Load columns for each table
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

func assessInputQuality(ctx context.Context, client *anthropic.Client, conversations []LLMConversation, schema []SchemaTable) InputAssessment {
	if len(conversations) == 0 {
		return InputAssessment{
			SchemaIncluded:  false,
			ColumnsIncluded: false,
			WellFormed:      false,
			Issues:          []string{"No conversations found"},
			Score:           0,
		}
	}

	// Build schema summary for comparison
	var schemaSummary strings.Builder
	for _, t := range schema {
		schemaSummary.WriteString(fmt.Sprintf("Table: %s (%d columns)\n", t.TableName, len(t.Columns)))
		for _, c := range t.Columns {
			pk := ""
			if c.IsPrimaryKey {
				pk = " [PK]"
			}
			schemaSummary.WriteString(fmt.Sprintf("  - %s (%s)%s\n", c.ColumnName, c.DataType, pk))
		}
	}

	// Always check the FIRST conversation - this is the domain extraction phase
	// which should include the full schema with column names
	sampleConv := conversations[0]

	prompt := fmt.Sprintf(`You are assessing whether the ontology extraction system correctly EXTRACTED and PROVIDED
schema information to the LLM. This is about extraction quality, not about the schema itself.

A score of 100 is achievable if the extraction system did everything correctly with the data available.

## ACTUAL DATABASE SCHEMA (what was available to extract)
%s

## LLM REQUEST THAT WAS SENT (what extraction system provided)
%s

## ASSESSMENT TASK
Evaluate whether the extraction system correctly provided the available schema information to the LLM.

Return a JSON object:
{
  "schema_included": true/false,  // Were table names from the schema included in the request?
  "columns_included": true/false, // Were actual column names from the schema included in the request?
  "well_formed": true/false,      // Was the prompt well-structured for LLM understanding?
  "issues": ["list of extraction failures - things the system could have done better"],
  "score": 0-100  // Extraction quality score (100 = perfect extraction of available data)
}

IMPORTANT: This assesses EXTRACTION quality, not schema completeness.
- If schema has 10 tables and all 10 appear in the request → schema_included = true
- If schema columns appear in the request → columns_included = true
- Issues should focus on what the extraction system failed to provide, NOT what's missing from the schema

Return ONLY the JSON object, no other text.`, schemaSummary.String(), string(sampleConv.RequestMessages))

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
		return InputAssessment{
			Issues: []string{fmt.Sprintf("Assessment failed: %v", err)},
			Score:  0,
		}
	}

	// Parse response
	var result InputAssessment
	responseText := ""
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != nil {
			responseText = *block.Text
			break
		}
	}

	// Extract JSON from response
	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return InputAssessment{
			Issues: []string{fmt.Sprintf("Failed to parse assessment: %v", err)},
			Score:  0,
		}
	}

	return result
}

func assessOutputQuality(ctx context.Context, client *anthropic.Client, conversations []LLMConversation, schema []SchemaTable) []ConversationAssessment {
	var assessments []ConversationAssessment

	// Sample up to 5 conversations for detailed assessment
	sampled := conversations
	if len(sampled) > 5 {
		// Take first, last, and 3 from middle
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

	prompt := fmt.Sprintf(`You are assessing whether the LLM produced ACCURATE output given the input it received.
This is about extraction quality - did the LLM correctly process what it was given?

A score of 100 is achievable if the LLM did everything correctly with the data it was provided.

## ACTUAL SCHEMA (column names per table - ground truth)
%s

## REQUEST SENT TO MODEL (what the LLM received)
%s

## MODEL'S RESPONSE (what the LLM produced)
%s

## ASSESSMENT TASK
Evaluate whether the LLM correctly processed the input it was given.

Return JSON:
{
  "task_type": "entity_summary|table_analysis|domain_summary|other",
  "input_quality": 0-100,   // Was the input well-formed and complete for this task?
  "output_quality": 0-100,  // Did the LLM produce accurate output GIVEN THE INPUT?
  "issues": ["specific extraction failures - e.g., 'LLM hallucinated column X when input showed column Y'"]
}

CRITICAL FOCUS:
- Hallucination = LLM mentioned something NOT in the input it received → severe penalty
- Accurate grounding = LLM only referenced what was in its input → high score
- Do NOT penalize for knowledge gaps in the input - only assess what the LLM did with what it got

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
		TaskType      string   `json:"task_type"`
		InputQuality  int      `json:"input_quality"`
		OutputQuality int      `json:"output_quality"`
		Issues        []string `json:"issues"`
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
		TaskType:       result.TaskType,
		InputQuality:   result.InputQuality,
		OutputQuality:  result.OutputQuality,
		Issues:         result.Issues,
	}
}

func assessOntology(ctx context.Context, client *anthropic.Client, schema []SchemaTable, ontology *Ontology) OntologyAssessment {
	// Build detailed schema
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

	prompt := fmt.Sprintf(`You are assessing whether the ontology extraction system correctly generated an ontology
from the available schema data. A score of 100 is achievable if extraction was perfect.

%s

## GENERATED ONTOLOGY

### Domain Summary
%s

### Entity Summaries
%s

## ASSESSMENT TASK

Evaluate whether the extraction system correctly used the available schema to generate the ontology.
This is about EXTRACTION ACCURACY, not about knowledge gaps or missing business context.

1. **Domain Accuracy** (0-100): Does the domain summary correctly describe what CAN BE INFERRED from the schema?
   - Should not penalize for missing business context that isn't in the schema
   - Should penalize for incorrect inferences or hallucinations

2. **Entity Accuracy** (0-100): Do entity summaries correctly describe each table based on schema evidence?
   - Business names should be reasonable inferences from table/column names
   - Descriptions should match what the schema shows

3. **Key Column Accuracy** (0-100): Do key_columns reference ACTUAL columns from the schema?
   - Hallucinated column names = severe penalty
   - All referenced columns must exist in the actual schema
   - Column counts should match

4. **Relationship Accuracy** (0-100): Are relationships correctly identified FROM THE SCHEMA?
   - Foreign key patterns (e.g., user_id → users) should be detected
   - Should not penalize for relationships that can't be inferred from schema

Return JSON:
{
  "domain_accuracy": 0-100,
  "entity_accuracy": 0-100,
  "key_column_accuracy": 0-100,
  "relationship_accuracy": 0-100,
  "overall_score": 0-100,
  "strengths": ["what extraction got right"],
  "issues": ["specific extraction failures - NOT missing business context"],
  "examples": [
    "GOOD: users key_columns correctly references user_id, username from actual schema",
    "BAD: offers.key_columns includes 'user_id' but actual column is 'owner_id' - hallucination"
  ]
}

IMPORTANT: A score of 100 means perfect extraction given the schema. Do NOT penalize for:
- Missing business context that isn't in the schema
- Relationships that can't be inferred from naming patterns
- Domain terminology that requires external knowledge

DO penalize for:
- Hallucinated column names
- Incorrect inferences from schema
- Missed obvious patterns (e.g., *_id columns not linked to their tables)

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
			TotalQuestions:         0,
			RequiredQuestions:      0,
			OptionalQuestions:      0,
			RequiredClassification: 100, // No questions is valid
			OptionalClassification: 100,
			QuestionQuality:        100,
			OverallScore:           100,
			Issues:                 []string{},
		}
	}

	// Count required vs optional
	var required, optional int
	for _, q := range questions {
		if q.IsRequired {
			required++
		} else {
			optional++
		}
	}

	// Build schema summary
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

	// Build questions list
	var questionsText strings.Builder
	questionsText.WriteString("## REQUIRED QUESTIONS\n")
	for _, q := range questions {
		if q.IsRequired {
			questionsText.WriteString(fmt.Sprintf("- %s\n", q.Text))
			if q.Reasoning != nil && *q.Reasoning != "" {
				questionsText.WriteString(fmt.Sprintf("  Reasoning: %s\n", *q.Reasoning))
			}
			if q.SourceEntityKey != nil {
				questionsText.WriteString(fmt.Sprintf("  Source: %s\n", *q.SourceEntityKey))
			}
		}
	}
	questionsText.WriteString("\n## OPTIONAL QUESTIONS\n")
	for _, q := range questions {
		if !q.IsRequired {
			questionsText.WriteString(fmt.Sprintf("- %s (priority: %d)\n", q.Text, q.Priority))
			if q.Reasoning != nil && *q.Reasoning != "" {
				questionsText.WriteString(fmt.Sprintf("  Reasoning: %s\n", *q.Reasoning))
			}
		}
	}

	prompt := fmt.Sprintf(`You are assessing whether questions generated during ontology extraction are correctly classified.
A score of 100 is achievable if the classification is perfect given the available information.

## DATABASE SCHEMA (available information)
%s

## DOMAIN SUMMARY
%s

## GENERATED QUESTIONS
%s

## ASSESSMENT TASK

Evaluate the CLASSIFICATION of questions, not whether questions exist:

1. **Required Classification** (0-100): Are REQUIRED questions correctly classified?
   - REQUIRED = Cannot proceed with ontology without this answer
   - Example GOOD required: "What do values 1,2,3,4 in status column mean?" - enums with visible values need business meaning
   - Example BAD required: "What is created_at?" - obvious timestamp field, should be optional or not asked
   - Example BAD required: Asking about something already visible in schema

2. **Optional Classification** (0-100): Are OPTIONAL questions correctly classified?
   - OPTIONAL = Nice to know, but we can make reasonable assumptions
   - Example GOOD optional: "Is marker_at used for pagination?" - we can assume yes based on naming
   - Example BAD optional: Should be required because the answer fundamentally changes understanding

3. **Question Quality** (0-100): Are questions well-formed and useful?
   - Questions should be clear and answerable
   - Should not ask about things already visible in schema
   - Should focus on business logic, not obvious schema facts

Return JSON:
{
  "required_classification": 0-100,
  "optional_classification": 0-100,
  "question_quality": 0-100,
  "overall_score": 0-100,
  "issues": ["specific classification errors"],
  "examples": [
    "GOOD: 'What are valid state values?' is correctly required - enum values need business meaning",
    "BAD: 'What is user_id?' is required but should not be a question at all - it's obviously a user identifier",
    "BAD: 'What do offer_type values 2,4,5,6 mean?' is optional but should be required - enum meanings are critical"
  ]
}

IMPORTANT: A score of 100 means perfect classification. Key principles:
- Enum values with visible numbers (1,2,3 or 2,4,5,6) → REQUIRED to ask their meaning
- Obvious fields (created_at, updated_at, id) → Should NOT be questions at all
- Business logic questions → REQUIRED if critical, OPTIONAL if inferable

Return ONLY JSON.`, schemaSummary.String(), string(ontology.DomainSummary), questionsText.String())

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
		RequiredClassification int      `json:"required_classification"`
		OptionalClassification int      `json:"optional_classification"`
		QuestionQuality        int      `json:"question_quality"`
		OverallScore           int      `json:"overall_score"`
		Issues                 []string `json:"issues"`
		Examples               []string `json:"examples"`
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
		TotalQuestions:         len(questions),
		RequiredQuestions:      required,
		OptionalQuestions:      optional,
		RequiredClassification: result.RequiredClassification,
		OptionalClassification: result.OptionalClassification,
		QuestionQuality:        result.QuestionQuality,
		OverallScore:           result.OverallScore,
		Issues:                 result.Issues,
		Examples:               result.Examples,
	}
}

func calculateFinalScore(input InputAssessment, outputs []ConversationAssessment, questions QuestionsAssessment, ontology OntologyAssessment) int {
	// Weights for extraction quality assessment:
	// - Input extraction: 20% (did we correctly provide data to LLM?)
	// - Output quality: 25% (did LLM correctly process the input?)
	// - Questions classification: 25% (are required/optional correctly classified?)
	// - Ontology accuracy: 30% (is the final ontology accurate given input?)
	inputScore := input.Score

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

	finalScore := float64(inputScore)*0.20 + outputAvg*0.25 + float64(questionsScore)*0.25 + float64(ontologyScore)*0.30

	return int(finalScore)
}

func extractJSON(s string) string {
	// Find JSON object in response
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// assess-ontology evaluates how well the final ontology enables SQL query generation.
//
// Goal: Rate how confidently an LLM (e.g., Sonnet 4.5) could navigate the database
// and create correct SQL queries to answer user questions or generate insights.
//
// A score of 100 means:
//   - The ontology is complete enough that there are no unknowns
//   - All relationships are documented or tables are clearly standalone
//   - No required questions are pending
//
// Key factors that reduce the score:
//   - Pending required questions (creates gaps in understanding)
//   - Missing relationships (unclear how tables connect)
//   - Ambiguous entity descriptions (LLM might misinterpret)
//   - Undocumented enumeration values (status/type columns)
//
// Usage: go run ./scripts/assess-ontology <project-id>
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
	CommitInfo             string                   `json:"commit_info"`
	DatasourceName         string                   `json:"datasource_name"`
	ProjectID              string                   `json:"project_id"`
	ModelUsed              string                   `json:"model_used"`
	LLMMetrics             LLMMetrics               `json:"llm_metrics"`
	PendingQuestionsImpact PendingQuestionsImpact   `json:"pending_questions_impact"`
	RelationshipCoverage   RelationshipCoverage     `json:"relationship_coverage"`
	EntityCompleteness     EntityCompletenessAssess `json:"entity_completeness"`
	SQLReadiness           SQLReadinessAssessment   `json:"sql_readiness"`
	FinalScore             int                      `json:"final_score"`
	FinalAssessment        string                   `json:"final_assessment"`
}

// PendingQuestionsImpact assesses what gaps exist due to unanswered questions
type PendingQuestionsImpact struct {
	TotalPending       int      `json:"total_pending"`
	RequiredPending    int      `json:"required_pending"`
	OptionalPending    int      `json:"optional_pending"`
	CriticalGaps       []string `json:"critical_gaps"`        // What we can't do without answers
	AffectedQueries    []string `json:"affected_queries"`     // Types of queries impacted
	EnabledWithAnswers []string `json:"enabled_with_answers"` // What becomes possible with answers
	ImpactScore        int      `json:"impact_score"`         // 0-100, higher = more impact (worse)
}

// RelationshipCoverage assesses how well tables are connected
type RelationshipCoverage struct {
	TotalTables         int               `json:"total_tables"`
	TablesWithRelations int               `json:"tables_with_relations"`
	OrphanTables        []OrphanTable     `json:"orphan_tables"`     // Tables with no relationships
	MissingRelations    []MissingRelation `json:"missing_relations"` // Suspected missing FKs
	CoverageScore       int               `json:"coverage_score"`    // 0-100
}

// OrphanTable is a table with no documented relationships
type OrphanTable struct {
	TableName   string `json:"table_name"`
	RowCount    *int64 `json:"row_count"`
	LikelyUsage string `json:"likely_usage"` // LLM's assessment of how it might be used
	Concern     string `json:"concern"`      // Why this is problematic
}

// MissingRelation is a suspected FK that isn't documented
type MissingRelation struct {
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
	TargetTable  string `json:"target_table"`
	Confidence   string `json:"confidence"` // high/medium/low
	Reasoning    string `json:"reasoning"`
}

// EntityCompletenessAssess evaluates entity documentation quality
type EntityCompletenessAssess struct {
	WellDocumented      int      `json:"well_documented"`      // Count of complete entities
	PartiallyDocumented int      `json:"partially_documented"` // Some gaps
	PoorlyDocumented    int      `json:"poorly_documented"`    // Major gaps
	UndocumentedEnums   []string `json:"undocumented_enums"`   // Status/type columns without values
	AmbiguousEntities   []string `json:"ambiguous_entities"`   // Entities that could confuse LLM
	CompletenessScore   int      `json:"completeness_score"`   // 0-100
}

// SQLReadinessAssessment is the core assessment - can LLM write correct SQL?
type SQLReadinessAssessment struct {
	ConfidenceLevel    string   `json:"confidence_level"`     // high/medium/low/very_low
	ConfidenceScore    int      `json:"confidence_score"`     // 0-100
	StrengthAreas      []string `json:"strength_areas"`       // What queries LLM can handle well
	WeakAreas          []string `json:"weak_areas"`           // Where LLM might fail
	SampleGoodQueries  []string `json:"sample_good_queries"`  // Example questions LLM can answer
	SampleRiskyQueries []string `json:"sample_risky_queries"` // Example questions that might fail
	Recommendations    []string `json:"recommendations"`      // What would improve the ontology
}

// LLMMetrics contains aggregated LLM performance metrics from extraction
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
	IsNullable   bool   `json:"is_nullable"`
}

// SchemaRelationship represents a FK relationship
type SchemaRelationship struct {
	SourceTableID  uuid.UUID
	SourceColumnID uuid.UUID
	TargetTableID  uuid.UUID
	TargetColumnID uuid.UUID
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

	// Get model used from conversations
	modelUsed := "unknown"
	if len(conversations) > 0 {
		modelUsed = conversations[0].Model
	}

	// Calculate LLM metrics
	llmMetrics := calculateLLMMetrics(conversations)

	// Create Anthropic client for assessments
	client := anthropic.NewClient(apiKey)

	// Run assessments
	fmt.Fprintf(os.Stderr, "Assessing pending questions impact...\n")
	pendingImpact := assessPendingQuestionsImpact(ctx, client, questions, schema, ontology)

	fmt.Fprintf(os.Stderr, "Assessing relationship coverage...\n")
	relationshipCoverage := assessRelationshipCoverage(ctx, client, schema, relationships, ontology)

	fmt.Fprintf(os.Stderr, "Assessing entity completeness...\n")
	entityCompleteness := assessEntityCompleteness(ctx, client, schema, ontology, questions)

	fmt.Fprintf(os.Stderr, "Assessing SQL readiness...\n")
	sqlReadiness := assessSQLReadiness(ctx, client, schema, ontology, questions, relationships)

	// Calculate final score
	// Weights: SQL Readiness 40%, Relationship Coverage 25%, Entity Completeness 20%, Pending Questions 15%
	// Note: pendingImpact.ImpactScore is inverted (higher = worse), so we use (100 - score)
	finalScore := int(
		float64(sqlReadiness.ConfidenceScore)*0.40 +
			float64(relationshipCoverage.CoverageScore)*0.25 +
			float64(entityCompleteness.CompletenessScore)*0.20 +
			float64(100-pendingImpact.ImpactScore)*0.15,
	)

	// Generate final assessment summary
	finalAssessment := generateFinalAssessment(finalScore, sqlReadiness, pendingImpact, relationshipCoverage)

	result := AssessmentResult{
		CommitInfo:             commitInfo,
		DatasourceName:         datasourceName,
		ProjectID:              projectID.String(),
		ModelUsed:              modelUsed,
		LLMMetrics:             llmMetrics,
		PendingQuestionsImpact: pendingImpact,
		RelationshipCoverage:   relationshipCoverage,
		EntityCompleteness:     entityCompleteness,
		SQLReadiness:           sqlReadiness,
		FinalScore:             finalScore,
		FinalAssessment:        finalAssessment,
	}

	// Output JSON
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
}

func generateFinalAssessment(score int, sql SQLReadinessAssessment, pending PendingQuestionsImpact, relations RelationshipCoverage) string {
	var assessment string

	switch {
	case score >= 90:
		assessment = "EXCELLENT: The ontology is highly complete. An LLM can confidently generate SQL queries for most business questions."
	case score >= 75:
		assessment = "GOOD: The ontology is well-structured with minor gaps. LLM can handle common queries but may struggle with edge cases."
	case score >= 60:
		assessment = "FAIR: The ontology has notable gaps. LLM can answer basic questions but will likely fail on complex queries."
	case score >= 40:
		assessment = "POOR: Significant gaps exist. LLM will frequently generate incorrect SQL or miss important relationships."
	default:
		assessment = "INADEQUATE: The ontology has critical gaps. LLM cannot reliably navigate this database."
	}

	// Add specific context
	if pending.RequiredPending > 0 {
		assessment += fmt.Sprintf(" %d required questions remain unanswered.", pending.RequiredPending)
	}
	if len(relations.OrphanTables) > 0 {
		assessment += fmt.Sprintf(" %d tables have no documented relationships.", len(relations.OrphanTables))
	}

	return assessment
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

	// Calculate tokens per second
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
		SELECT column_name, data_type, is_primary_key, is_nullable
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

func loadRelationships(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]SchemaRelationship, error) {
	query := `
		SELECT source_table_id, source_column_id, target_table_id, target_column_id
		FROM engine_schema_relationships
		WHERE project_id = $1 AND deleted_at IS NULL`

	rows, err := conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []SchemaRelationship
	for rows.Next() {
		var r SchemaRelationship
		if err := rows.Scan(&r.SourceTableID, &r.SourceColumnID, &r.TargetTableID, &r.TargetColumnID); err != nil {
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

func assessPendingQuestionsImpact(ctx context.Context, client *anthropic.Client, questions []OntologyQuestion, schema []SchemaTable, ontology *Ontology) PendingQuestionsImpact {
	// Count pending questions
	var required, optional int
	var pendingQuestions []OntologyQuestion
	for _, q := range questions {
		if q.Status == "pending" {
			pendingQuestions = append(pendingQuestions, q)
			if q.IsRequired {
				required++
			} else {
				optional++
			}
		}
	}

	if len(pendingQuestions) == 0 {
		return PendingQuestionsImpact{
			TotalPending:       0,
			RequiredPending:    0,
			OptionalPending:    0,
			CriticalGaps:       []string{},
			AffectedQueries:    []string{},
			EnabledWithAnswers: []string{},
			ImpactScore:        0, // No pending = no impact
		}
	}

	// Build questions list for LLM
	var questionsText strings.Builder
	questionsText.WriteString("## REQUIRED PENDING QUESTIONS\n")
	for _, q := range pendingQuestions {
		if q.IsRequired {
			questionsText.WriteString(fmt.Sprintf("- %s\n", q.Text))
			if q.SourceEntityKey != nil {
				questionsText.WriteString(fmt.Sprintf("  (Table: %s)\n", *q.SourceEntityKey))
			}
		}
	}
	questionsText.WriteString("\n## OPTIONAL PENDING QUESTIONS\n")
	for _, q := range pendingQuestions {
		if !q.IsRequired {
			questionsText.WriteString(fmt.Sprintf("- %s (priority: %d)\n", q.Text, q.Priority))
		}
	}

	prompt := fmt.Sprintf(`You are assessing the impact of unanswered questions on SQL query generation capability.

## ONTOLOGY DOMAIN SUMMARY
%s

## PENDING QUESTIONS
%s

## TASK
Analyze what gaps these unanswered questions create for an LLM trying to write SQL queries.

Return JSON:
{
  "critical_gaps": ["What the LLM cannot determine without answers - be specific"],
  "affected_queries": ["Types of queries that will likely fail or be incorrect"],
  "enabled_with_answers": ["What becomes possible once questions are answered"],
  "impact_score": 0-100  // Higher = more severe impact on SQL generation
}

Impact scoring guide:
- 0-20: Minor gaps, LLM can work around most issues
- 21-40: Moderate gaps, some query types will fail
- 41-60: Significant gaps, many queries will be incorrect
- 61-80: Severe gaps, LLM will frequently fail
- 81-100: Critical gaps, LLM cannot reliably generate SQL

Return ONLY JSON.`, string(ontology.DomainSummary), questionsText.String())

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
		return PendingQuestionsImpact{
			TotalPending:    len(pendingQuestions),
			RequiredPending: required,
			OptionalPending: optional,
			CriticalGaps:    []string{fmt.Sprintf("Assessment failed: %v", err)},
			ImpactScore:     50, // Default to moderate impact on error
		}
	}

	var result struct {
		CriticalGaps       []string `json:"critical_gaps"`
		AffectedQueries    []string `json:"affected_queries"`
		EnabledWithAnswers []string `json:"enabled_with_answers"`
		ImpactScore        int      `json:"impact_score"`
	}

	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return PendingQuestionsImpact{
			TotalPending:    len(pendingQuestions),
			RequiredPending: required,
			OptionalPending: optional,
			CriticalGaps:    []string{fmt.Sprintf("Parse error: %v", err)},
			ImpactScore:     50,
		}
	}

	return PendingQuestionsImpact{
		TotalPending:       len(pendingQuestions),
		RequiredPending:    required,
		OptionalPending:    optional,
		CriticalGaps:       result.CriticalGaps,
		AffectedQueries:    result.AffectedQueries,
		EnabledWithAnswers: result.EnabledWithAnswers,
		ImpactScore:        result.ImpactScore,
	}
}

func assessRelationshipCoverage(ctx context.Context, client *anthropic.Client, schema []SchemaTable, relationships []SchemaRelationship, ontology *Ontology) RelationshipCoverage {
	// Build table ID lookup
	tableIDToName := make(map[uuid.UUID]string)
	for _, t := range schema {
		tableIDToName[t.ID] = t.TableName
	}

	// Find which tables have relationships
	tablesWithRels := make(map[string]bool)
	for _, r := range relationships {
		if name, ok := tableIDToName[r.SourceTableID]; ok {
			tablesWithRels[name] = true
		}
		if name, ok := tableIDToName[r.TargetTableID]; ok {
			tablesWithRels[name] = true
		}
	}

	// Find orphan tables
	var orphanTableNames []string
	for _, t := range schema {
		if !tablesWithRels[t.TableName] {
			orphanTableNames = append(orphanTableNames, t.TableName)
		}
	}

	// Build schema summary
	var schemaSummary strings.Builder
	for _, t := range schema {
		hasRel := ""
		if tablesWithRels[t.TableName] {
			hasRel = " [HAS RELATIONSHIPS]"
		}
		schemaSummary.WriteString(fmt.Sprintf("### %s%s\n", t.TableName, hasRel))
		if t.RowCount != nil {
			schemaSummary.WriteString(fmt.Sprintf("Rows: %d\n", *t.RowCount))
		}
		for _, c := range t.Columns {
			pk := ""
			if c.IsPrimaryKey {
				pk = " [PK]"
			}
			schemaSummary.WriteString(fmt.Sprintf("  - %s (%s)%s\n", c.ColumnName, c.DataType, pk))
		}
		schemaSummary.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are assessing relationship coverage in a database for SQL query generation.

## ONTOLOGY DOMAIN SUMMARY
%s

## SCHEMA WITH RELATIONSHIP STATUS
%s

## TABLES WITHOUT DOCUMENTED RELATIONSHIPS
%s

## TASK
Analyze the relationship coverage and identify:
1. For each orphan table: Is it truly standalone, or are relationships missing?
2. Look for columns that LOOK like foreign keys (ending in _id, named similarly to other tables) but have no documented relationship

Return JSON:
{
  "orphan_tables": [
    {
      "table_name": "example_table",
      "likely_usage": "How this table is probably used in the business",
      "concern": "Why lack of relationships is problematic (or 'None - clearly standalone')"
    }
  ],
  "missing_relations": [
    {
      "source_table": "orders",
      "source_column": "customer_id",
      "target_table": "customers",
      "confidence": "high|medium|low",
      "reasoning": "Why this FK is likely missing"
    }
  ],
  "coverage_score": 0-100  // How well are relationships documented?
}

Coverage scoring guide:
- 90-100: All relationships documented, no suspicious orphan tables
- 70-89: Minor gaps, a few likely FKs undocumented
- 50-69: Moderate gaps, several important relationships missing
- 30-49: Significant gaps, many relationships undocumented
- 0-29: Poor coverage, LLM cannot understand table connections

Return ONLY JSON.`, string(ontology.DomainSummary), schemaSummary.String(), strings.Join(orphanTableNames, ", "))

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 3000,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return RelationshipCoverage{
			TotalTables:         len(schema),
			TablesWithRelations: len(tablesWithRels),
			OrphanTables:        []OrphanTable{{TableName: "error", Concern: err.Error()}},
			CoverageScore:       50,
		}
	}

	var result struct {
		OrphanTables     []OrphanTable     `json:"orphan_tables"`
		MissingRelations []MissingRelation `json:"missing_relations"`
		CoverageScore    int               `json:"coverage_score"`
	}

	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return RelationshipCoverage{
			TotalTables:         len(schema),
			TablesWithRelations: len(tablesWithRels),
			CoverageScore:       50,
		}
	}

	return RelationshipCoverage{
		TotalTables:         len(schema),
		TablesWithRelations: len(tablesWithRels),
		OrphanTables:        result.OrphanTables,
		MissingRelations:    result.MissingRelations,
		CoverageScore:       result.CoverageScore,
	}
}

func assessEntityCompleteness(ctx context.Context, client *anthropic.Client, schema []SchemaTable, ontology *Ontology, questions []OntologyQuestion) EntityCompletenessAssess {
	// Build schema summary with focus on status/type/enum columns
	var schemaSummary strings.Builder
	var enumCandidates []string

	for _, t := range schema {
		schemaSummary.WriteString(fmt.Sprintf("### %s\n", t.TableName))
		for _, c := range t.Columns {
			// Identify potential enum columns
			isEnumCandidate := strings.Contains(strings.ToLower(c.ColumnName), "status") ||
				strings.Contains(strings.ToLower(c.ColumnName), "type") ||
				strings.Contains(strings.ToLower(c.ColumnName), "state") ||
				strings.Contains(strings.ToLower(c.ColumnName), "category") ||
				strings.Contains(strings.ToLower(c.ColumnName), "role")

			marker := ""
			if isEnumCandidate {
				marker = " [ENUM?]"
				enumCandidates = append(enumCandidates, fmt.Sprintf("%s.%s", t.TableName, c.ColumnName))
			}
			schemaSummary.WriteString(fmt.Sprintf("  - %s: %s%s\n", c.ColumnName, c.DataType, marker))
		}
	}

	// Check which enum candidates have questions
	questionedColumns := make(map[string]bool)
	for _, q := range questions {
		if q.SourceEntityKey != nil {
			questionedColumns[*q.SourceEntityKey] = true
		}
	}

	prompt := fmt.Sprintf(`You are assessing entity documentation completeness for SQL query generation.

## ONTOLOGY
Domain Summary: %s

Entity Summaries: %s

## SCHEMA (columns marked [ENUM?] are potential enumerations)
%s

## POTENTIAL ENUM COLUMNS
%s

## TASK
Assess how well entities are documented for an LLM to generate correct SQL:

1. Are entity descriptions clear enough to understand their purpose?
2. Are key enum/status/type columns documented with their possible values?
3. Are there ambiguous entities that could confuse an LLM?

Return JSON:
{
  "well_documented": <count>,
  "partially_documented": <count>,
  "poorly_documented": <count>,
  "undocumented_enums": ["table.column values that are unknown"],
  "ambiguous_entities": ["entities with unclear purposes"],
  "completeness_score": 0-100
}

Completeness scoring:
- 90-100: All entities clear, enums documented, no ambiguity
- 70-89: Most entities clear, some enum values unknown
- 50-69: Several unclear entities or undocumented enums
- 30-49: Many entities ambiguous, critical enums unknown
- 0-29: Documentation insufficient for reliable SQL generation

Return ONLY JSON.`, string(ontology.DomainSummary), string(ontology.EntitySummaries), schemaSummary.String(), strings.Join(enumCandidates, ", "))

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
		return EntityCompletenessAssess{
			AmbiguousEntities: []string{fmt.Sprintf("Assessment failed: %v", err)},
			CompletenessScore: 50,
		}
	}

	var result EntityCompletenessAssess
	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return EntityCompletenessAssess{
			AmbiguousEntities: []string{fmt.Sprintf("Parse error: %v", err)},
			CompletenessScore: 50,
		}
	}

	return result
}

func assessSQLReadiness(ctx context.Context, client *anthropic.Client, schema []SchemaTable, ontology *Ontology, questions []OntologyQuestion, relationships []SchemaRelationship) SQLReadinessAssessment {
	// Build comprehensive context
	var schemaSummary strings.Builder
	for _, t := range schema {
		schemaSummary.WriteString(fmt.Sprintf("%s: ", t.TableName))
		var cols []string
		for _, c := range t.Columns {
			cols = append(cols, c.ColumnName)
		}
		schemaSummary.WriteString(strings.Join(cols, ", "))
		schemaSummary.WriteString("\n")
	}

	// Count pending required questions
	var pendingRequired int
	for _, q := range questions {
		if q.IsRequired && q.Status == "pending" {
			pendingRequired++
		}
	}

	prompt := fmt.Sprintf(`You are an expert SQL developer assessing whether an LLM can reliably generate SQL queries for this database.

## ONTOLOGY
Domain Summary: %s

Entity Summaries: %s

## SCHEMA (table: columns)
%s

## CONTEXT
- Total tables: %d
- Documented relationships: %d
- Pending required questions: %d

## TASK
Assess how confidently an LLM (like Claude Sonnet) could generate correct SQL queries for business questions.

Consider:
1. Are table purposes clear enough to select the right tables?
2. Are relationships clear enough to write correct JOINs?
3. Are column meanings clear enough to select the right fields?
4. Are there gaps that would cause incorrect SQL?

Return JSON:
{
  "confidence_level": "high|medium|low|very_low",
  "confidence_score": 0-100,
  "strength_areas": ["What the LLM can do well with this ontology"],
  "weak_areas": ["Where the LLM will likely struggle or fail"],
  "sample_good_queries": ["Example business questions LLM can answer correctly"],
  "sample_risky_queries": ["Example questions that might produce wrong SQL"],
  "recommendations": ["What would most improve SQL generation capability"]
}

Confidence scoring:
- 90-100: HIGH - LLM can confidently answer most business questions
- 70-89: MEDIUM - LLM handles common queries, struggles with complex ones
- 50-69: LOW - LLM will frequently produce incorrect or incomplete SQL
- 0-49: VERY LOW - LLM cannot reliably navigate this database

Return ONLY JSON.`, string(ontology.DomainSummary), string(ontology.EntitySummaries), schemaSummary.String(), len(schema), len(relationships), pendingRequired)

	resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 3000,
		Messages: []anthropic.Message{
			{Role: anthropic.RoleUser, Content: []anthropic.MessageContent{
				{Type: "text", Text: &prompt},
			}},
		},
	})

	if err != nil {
		return SQLReadinessAssessment{
			ConfidenceLevel: "unknown",
			ConfidenceScore: 50,
			WeakAreas:       []string{fmt.Sprintf("Assessment failed: %v", err)},
		}
	}

	var result SQLReadinessAssessment
	responseText := extractTextFromResponse(resp)
	responseText = extractJSON(responseText)
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return SQLReadinessAssessment{
			ConfidenceLevel: "unknown",
			ConfidenceScore: 50,
			WeakAreas:       []string{fmt.Sprintf("Parse error: %v", err)},
		}
	}

	return result
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
	// Find JSON object in response
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

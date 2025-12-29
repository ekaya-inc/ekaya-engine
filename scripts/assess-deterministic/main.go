// assess-deterministic evaluates the DETERMINISTIC portions of ontology extraction:
// - Input preparation: Did we correctly provide schema information to the LLM?
// - Post-processing: Did we correctly parse and store LLM responses?
//
// This tool does NOT use an LLM for assessment - all checks are deterministic.
// A score of 100 means the deterministic code is perfect. This is achievable.
//
// Separate from assess-extraction which evaluates LLM output quality.
//
// Usage: go run ./scripts/assess-deterministic <project-id>
//
// Database connection: Uses standard PG* environment variables
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AssessmentResult contains the full assessment output
type AssessmentResult struct {
	CommitInfo            string                `json:"commit_info"`
	DatasourceName        string                `json:"datasource_name"`
	ProjectID             string                `json:"project_id"`
	InputAssessment       InputAssessment       `json:"input_assessment"`
	PostProcessAssessment PostProcessAssessment `json:"post_process_assessment"`
	FinalScore            int                   `json:"final_score"`
	Summary               string                `json:"summary"`

	// Data availability (Phase 2)
	WorkflowStateCount int `json:"workflow_state_count"`
	RelationshipCount  int `json:"relationship_count"`
	ColumnsWithStats   int `json:"columns_with_stats"`
}

type InputAssessment struct {
	TotalConversations int              `json:"total_conversations"`
	Checks             []InputCheck     `json:"checks"`
	ByConversation     []ConversationQA `json:"by_conversation"`
	Score              int              `json:"score"` // 0-100
	Issues             []string         `json:"issues"`
}

type InputCheck struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Details     string `json:"details"`
}

type ConversationQA struct {
	Index           int      `json:"index"`
	TablesIncluded  bool     `json:"tables_included"`
	ColumnsIncluded bool     `json:"columns_included"`
	TypesIncluded   bool     `json:"types_included"`
	FlagsIncluded   bool     `json:"flags_included"`
	Issues          []string `json:"issues"`
	Score           int      `json:"score"`
}

type PostProcessAssessment struct {
	Checks []PostProcessCheck `json:"checks"`
	Score  int                `json:"score"` // 0-100
	Issues []string           `json:"issues"`
}

type PostProcessCheck struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Details     string `json:"details"`
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
	ColumnName    string `json:"column_name"`
	DataType      string `json:"data_type"`
	IsPrimaryKey  bool   `json:"is_primary_key"`
	IsNullable    bool   `json:"is_nullable"`
	DistinctCount *int64 `json:"distinct_count,omitempty"`
	NullCount     *int64 `json:"null_count,omitempty"`
}

// WorkflowEntityState represents a workflow state row
type WorkflowEntityState struct {
	ID         uuid.UUID       `json:"id"`
	EntityType string          `json:"entity_type"`
	EntityKey  string          `json:"entity_key"`
	Status     string          `json:"status"`
	StateData  json.RawMessage `json:"state_data"`
}

// GatheredData is the parsed gathered field for columns
type GatheredData struct {
	RowCount        *int64   `json:"row_count"`
	NonNullCount    *int64   `json:"non_null_count"`
	DistinctCount   *int64   `json:"distinct_count"`
	NullPercent     *float64 `json:"null_percent"`
	SampleValues    []any    `json:"sample_values"`
	IsEnumCandidate bool     `json:"is_enum_candidate"`
	ScannedAt       *string  `json:"scanned_at"`
}

// SchemaRelationship represents an approved relationship
type SchemaRelationship struct {
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column"`
	Type         string `json:"type"`
}

// LLMConversation represents a stored conversation
type LLMConversation struct {
	ID              uuid.UUID       `json:"id"`
	Model           string          `json:"model"`
	RequestMessages json.RawMessage `json:"request_messages"`
	ResponseContent string          `json:"response_content"`
	Status          string          `json:"status"`
}

// OntologyQuestion represents a stored question
type OntologyQuestion struct {
	ID               uuid.UUID `json:"id"`
	Text             string    `json:"text"`
	Reasoning        *string   `json:"reasoning"`
	IsRequired       bool      `json:"is_required"`
	SourceEntityType *string   `json:"source_entity_type"`
	SourceEntityKey  *string   `json:"source_entity_key"`
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
	schema, err := loadSchema(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load schema: %v\n", err)
		os.Exit(1)
	}

	conversations, err := loadConversations(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load conversations: %v\n", err)
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

	// Load additional data (Phase 2)
	workflowStates, err := loadWorkflowStates(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load workflow states: %v\n", err)
		os.Exit(1)
	}
	if len(workflowStates) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: No workflow state found - gathered data checks will be skipped\n")
	}

	relationships, err := loadRelationships(ctx, conn, projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load relationships: %v\n", err)
		os.Exit(1)
	}

	// Run deterministic assessments
	fmt.Fprintf(os.Stderr, "Assessing input preparation quality...\n")
	inputAssessment := assessInputPreparation(schema, conversations)

	fmt.Fprintf(os.Stderr, "Assessing post-processing quality...\n")
	postProcessAssessment := assessPostProcessing(schema, conversations, ontology, questions)

	// Calculate final score (50% input, 50% post-processing)
	finalScore := (inputAssessment.Score + postProcessAssessment.Score) / 2

	// Generate summary
	summary := generateSummary(inputAssessment, postProcessAssessment, finalScore)

	result := AssessmentResult{
		CommitInfo:            commitInfo,
		DatasourceName:        datasourceName,
		ProjectID:             projectID.String(),
		InputAssessment:       inputAssessment,
		PostProcessAssessment: postProcessAssessment,
		FinalScore:            finalScore,
		Summary:               summary,

		// Phase 2: Data availability counts
		WorkflowStateCount: len(workflowStates),
		RelationshipCount:  len(relationships),
		ColumnsWithStats:   countColumnsWithStats(schema),
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

	// Load columns for each table (including stats for Phase 2)
	colQuery := `
		SELECT column_name, data_type, is_primary_key, is_nullable, distinct_count, null_count
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
			if err := colRows.Scan(&c.ColumnName, &c.DataType, &c.IsPrimaryKey, &c.IsNullable, &c.DistinctCount, &c.NullCount); err != nil {
				colRows.Close()
				return nil, err
			}
			tables[i].Columns = append(tables[i].Columns, c)
		}
		colRows.Close()
	}

	return tables, nil
}

func loadConversations(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]LLMConversation, error) {
	query := `
		SELECT id, model, request_messages, COALESCE(response_content, ''), status
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
		if err := rows.Scan(&c.ID, &c.Model, &c.RequestMessages, &c.ResponseContent, &c.Status); err != nil {
			return nil, err
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
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
		SELECT id, text, reasoning, is_required, source_entity_type, source_entity_key
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
		if err := rows.Scan(&q.ID, &q.Text, &q.Reasoning, &q.IsRequired, &q.SourceEntityType, &q.SourceEntityKey); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

// loadWorkflowStates loads workflow entity states for the active ontology.
// Returns empty slice (not error) if no workflow state exists (pre-Phase 1 extractions).
func loadWorkflowStates(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]WorkflowEntityState, error) {
	query := `
		SELECT ws.id, ws.entity_type, ws.entity_key, ws.status, ws.state_data
		FROM engine_workflow_state ws
		JOIN engine_ontologies o ON ws.ontology_id = o.id
		WHERE o.project_id = $1 AND o.is_active = true
		ORDER BY ws.entity_type, ws.entity_key`

	rows, err := conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []WorkflowEntityState
	for rows.Next() {
		var s WorkflowEntityState
		if err := rows.Scan(&s.ID, &s.EntityType, &s.EntityKey, &s.Status, &s.StateData); err != nil {
			return nil, err
		}
		states = append(states, s)
	}
	return states, rows.Err()
}

// loadRelationships loads approved schema relationships.
func loadRelationships(ctx context.Context, conn *pgx.Conn, projectID uuid.UUID) ([]SchemaRelationship, error) {
	query := `
		SELECT
			st.table_name as source_table,
			sc.column_name as source_column,
			tt.table_name as target_table,
			tc.column_name as target_column,
			COALESCE(r.cardinality, 'unknown') as type
		FROM engine_schema_relationships r
		JOIN engine_schema_tables st ON r.source_table_id = st.id
		JOIN engine_schema_columns sc ON r.source_column_id = sc.id
		JOIN engine_schema_tables tt ON r.target_table_id = tt.id
		JOIN engine_schema_columns tc ON r.target_column_id = tc.id
		WHERE st.project_id = $1
		  AND r.is_approved = true
		  AND st.is_selected = true
		  AND st.deleted_at IS NULL
		ORDER BY st.table_name, sc.column_name`

	rows, err := conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []SchemaRelationship
	for rows.Next() {
		var r SchemaRelationship
		if err := rows.Scan(&r.SourceTable, &r.SourceColumn, &r.TargetTable, &r.TargetColumn, &r.Type); err != nil {
			return nil, err
		}
		relationships = append(relationships, r)
	}
	return relationships, rows.Err()
}

// buildGatheredDataMap creates a lookup from entity_key to gathered data.
// Entity keys are in format "table_name.column_name".
func buildGatheredDataMap(states []WorkflowEntityState) map[string]*GatheredData {
	result := make(map[string]*GatheredData)
	for _, state := range states {
		if state.EntityType != "column" || len(state.StateData) == 0 {
			continue
		}

		// Parse state_data to get gathered field
		var stateData struct {
			Gathered *GatheredData `json:"gathered"`
		}
		if err := json.Unmarshal(state.StateData, &stateData); err != nil {
			continue
		}
		if stateData.Gathered != nil {
			result[state.EntityKey] = stateData.Gathered
		}
	}
	return result
}

// countColumnsWithStats counts columns that have statistics available.
func countColumnsWithStats(schema []SchemaTable) int {
	count := 0
	for _, t := range schema {
		for _, c := range t.Columns {
			if c.DistinctCount != nil || c.NullCount != nil {
				count++
			}
		}
	}
	return count
}

func assessInputPreparation(schema []SchemaTable, conversations []LLMConversation) InputAssessment {
	var checks []InputCheck
	var issues []string
	var byConversation []ConversationQA

	if len(conversations) == 0 {
		return InputAssessment{
			TotalConversations: 0,
			Checks:             checks,
			Score:              0,
			Issues:             []string{"No conversations found"},
		}
	}

	// Build lookup maps for verification
	tableNames := make(map[string]bool)
	columnsByTable := make(map[string]map[string]SchemaColumn)
	for _, t := range schema {
		tableNames[strings.ToLower(t.TableName)] = true
		columnsByTable[strings.ToLower(t.TableName)] = make(map[string]SchemaColumn)
		for _, c := range t.Columns {
			columnsByTable[strings.ToLower(t.TableName)][strings.ToLower(c.ColumnName)] = c
		}
	}

	// Assess each conversation
	totalScore := 0
	for i, conv := range conversations {
		qa := assessConversationInput(i, conv, schema, tableNames, columnsByTable)
		byConversation = append(byConversation, qa)
		totalScore += qa.Score
		issues = append(issues, qa.Issues...)
	}

	avgScore := totalScore / len(conversations)

	// Global checks
	checks = append(checks, InputCheck{
		Name:        "conversations_exist",
		Description: "At least one LLM conversation was recorded",
		Passed:      len(conversations) > 0,
		Details:     fmt.Sprintf("%d conversations found", len(conversations)),
	})

	// Check first conversation (domain extraction) has full schema
	if len(conversations) > 0 {
		firstConv := conversations[0]
		requestStr := string(firstConv.RequestMessages)

		// Count how many tables are mentioned
		tablesFound := 0
		for tableName := range tableNames {
			if strings.Contains(strings.ToLower(requestStr), tableName) {
				tablesFound++
			}
		}

		allTablesIncluded := tablesFound == len(tableNames)
		checks = append(checks, InputCheck{
			Name:        "first_conv_has_all_tables",
			Description: "First conversation (domain extraction) includes all schema tables",
			Passed:      allTablesIncluded,
			Details:     fmt.Sprintf("%d/%d tables found in first conversation", tablesFound, len(tableNames)),
		})

		if !allTablesIncluded {
			issues = append(issues, fmt.Sprintf("First conversation missing %d tables", len(tableNames)-tablesFound))
		}
	}

	// Check for negative row counts being displayed (should be filtered)
	negativeRowCountShown := false
	for _, conv := range conversations {
		requestStr := string(conv.RequestMessages)
		// Look for patterns like "Row count: -1" or "Rows: -1"
		if matched, _ := regexp.MatchString(`(?i)(row\s*(count)?:?\s*-1|rows?:?\s*-1)`, requestStr); matched {
			negativeRowCountShown = true
			break
		}
	}
	checks = append(checks, InputCheck{
		Name:        "no_negative_row_counts",
		Description: "Negative row counts (-1) are not displayed in prompts",
		Passed:      !negativeRowCountShown,
		Details:     boolToStatus(!negativeRowCountShown, "No -1 row counts shown", "Found -1 row count in prompts"),
	})
	if negativeRowCountShown {
		issues = append(issues, "Negative row count (-1) shown in prompts - should be filtered")
	}

	// Calculate check score
	checksPassed := 0
	for _, c := range checks {
		if c.Passed {
			checksPassed++
		}
	}
	checksScore := 0
	if len(checks) > 0 {
		checksScore = (checksPassed * 100) / len(checks)
	}

	// Final score is weighted: 70% conversation quality, 30% global checks
	finalScore := (avgScore*70 + checksScore*30) / 100

	return InputAssessment{
		TotalConversations: len(conversations),
		Checks:             checks,
		ByConversation:     byConversation,
		Score:              finalScore,
		Issues:             dedupeStrings(issues),
	}
}

func assessConversationInput(index int, conv LLMConversation, schema []SchemaTable, tableNames map[string]bool, columnsByTable map[string]map[string]SchemaColumn) ConversationQA {
	requestStr := string(conv.RequestMessages)
	requestLower := strings.ToLower(requestStr)

	var issues []string
	score := 100 // Start at 100, deduct for issues

	// Check 1: Tables included
	tablesFound := 0
	for tableName := range tableNames {
		if strings.Contains(requestLower, tableName) {
			tablesFound++
		}
	}
	tablesIncluded := tablesFound > 0
	if !tablesIncluded {
		issues = append(issues, fmt.Sprintf("Conv %d: No table names found in request", index))
		score -= 25
	}

	// Check 2: Columns included
	columnsFound := 0
	totalColumns := 0
	for _, cols := range columnsByTable {
		for colName := range cols {
			totalColumns++
			if strings.Contains(requestLower, colName) {
				columnsFound++
			}
		}
	}
	columnsIncluded := columnsFound > 0
	if !columnsIncluded && totalColumns > 0 {
		issues = append(issues, fmt.Sprintf("Conv %d: No column names found in request", index))
		score -= 25
	}

	// Check 3: Data types included
	commonTypes := []string{"uuid", "text", "varchar", "integer", "int", "bigint", "boolean", "timestamp", "numeric", "jsonb"}
	typesFound := 0
	for _, dt := range commonTypes {
		if strings.Contains(requestLower, dt) {
			typesFound++
		}
	}
	typesIncluded := typesFound >= 2 // At least 2 different types
	if !typesIncluded {
		issues = append(issues, fmt.Sprintf("Conv %d: Insufficient data types in request", index))
		score -= 15
	}

	// Check 4: PK/nullable flags included
	flagsIncluded := strings.Contains(requestLower, "[pk]") ||
		strings.Contains(requestLower, "primary key") ||
		strings.Contains(requestLower, "nullable")
	if !flagsIncluded {
		issues = append(issues, fmt.Sprintf("Conv %d: No PK/nullable flags in request", index))
		score -= 10
	}

	if score < 0 {
		score = 0
	}

	return ConversationQA{
		Index:           index,
		TablesIncluded:  tablesIncluded,
		ColumnsIncluded: columnsIncluded,
		TypesIncluded:   typesIncluded,
		FlagsIncluded:   flagsIncluded,
		Issues:          issues,
		Score:           score,
	}
}

func assessPostProcessing(schema []SchemaTable, conversations []LLMConversation, ontology *Ontology, questions []OntologyQuestion) PostProcessAssessment {
	var checks []PostProcessCheck
	var issues []string

	// Check 1: All LLM responses were successful
	successfulConvs := 0
	for _, c := range conversations {
		if c.Status == "success" {
			successfulConvs++
		}
	}
	allSuccessful := successfulConvs == len(conversations)
	checks = append(checks, PostProcessCheck{
		Name:        "all_responses_successful",
		Description: "All LLM conversations completed successfully",
		Passed:      allSuccessful,
		Details:     fmt.Sprintf("%d/%d successful", successfulConvs, len(conversations)),
	})
	if !allSuccessful {
		issues = append(issues, fmt.Sprintf("%d conversations failed", len(conversations)-successfulConvs))
	}

	// Check 2: Ontology was created
	ontologyExists := ontology != nil
	checks = append(checks, PostProcessCheck{
		Name:        "ontology_created",
		Description: "Active ontology was created from LLM responses",
		Passed:      ontologyExists,
		Details:     boolToStatus(ontologyExists, "Ontology exists", "No active ontology"),
	})
	if !ontologyExists {
		issues = append(issues, "No active ontology was created")
	}

	// Check 3: Entity summaries match schema tables
	if ontologyExists {
		var entitySummaries map[string]interface{}
		if err := json.Unmarshal(ontology.EntitySummaries, &entitySummaries); err == nil {
			entitiesCreated := len(entitySummaries)
			expectedEntities := len(schema)
			allEntitiesCreated := entitiesCreated >= expectedEntities
			checks = append(checks, PostProcessCheck{
				Name:        "all_entities_created",
				Description: "Entity summaries created for all schema tables",
				Passed:      allEntitiesCreated,
				Details:     fmt.Sprintf("%d entities for %d tables", entitiesCreated, expectedEntities),
			})
			if !allEntitiesCreated {
				issues = append(issues, fmt.Sprintf("Missing entity summaries: %d created for %d tables", entitiesCreated, expectedEntities))
			}
		}
	}

	// Check 4: Questions have source entity references
	questionsWithSource := 0
	for _, q := range questions {
		if q.SourceEntityKey != nil && *q.SourceEntityKey != "" {
			questionsWithSource++
		}
	}
	allQuestionsHaveSource := len(questions) == 0 || questionsWithSource == len(questions)
	checks = append(checks, PostProcessCheck{
		Name:        "questions_have_source",
		Description: "All questions reference their source entity",
		Passed:      allQuestionsHaveSource,
		Details:     fmt.Sprintf("%d/%d questions have source", questionsWithSource, len(questions)),
	})
	if !allQuestionsHaveSource {
		issues = append(issues, fmt.Sprintf("%d questions missing source entity reference", len(questions)-questionsWithSource))
	}

	// Check 5: Questions have reasoning
	questionsWithReasoning := 0
	for _, q := range questions {
		if q.Reasoning != nil && *q.Reasoning != "" {
			questionsWithReasoning++
		}
	}
	allQuestionsHaveReasoning := len(questions) == 0 || questionsWithReasoning == len(questions)
	checks = append(checks, PostProcessCheck{
		Name:        "questions_have_reasoning",
		Description: "All questions include reasoning",
		Passed:      allQuestionsHaveReasoning,
		Details:     fmt.Sprintf("%d/%d questions have reasoning", questionsWithReasoning, len(questions)),
	})
	if !allQuestionsHaveReasoning {
		issues = append(issues, fmt.Sprintf("%d questions missing reasoning", len(questions)-questionsWithReasoning))
	}

	// Check 6: Domain summary exists and is non-empty
	domainSummaryExists := false
	if ontologyExists {
		var domainSummary interface{}
		if err := json.Unmarshal(ontology.DomainSummary, &domainSummary); err == nil && domainSummary != nil {
			if ds, ok := domainSummary.(map[string]interface{}); ok && len(ds) > 0 {
				domainSummaryExists = true
			}
		}
	}
	checks = append(checks, PostProcessCheck{
		Name:        "domain_summary_exists",
		Description: "Domain summary was extracted and stored",
		Passed:      domainSummaryExists,
		Details:     boolToStatus(domainSummaryExists, "Domain summary exists", "Missing or empty domain summary"),
	})
	if !domainSummaryExists {
		issues = append(issues, "Domain summary missing or empty")
	}

	// Calculate score
	passed := 0
	for _, c := range checks {
		if c.Passed {
			passed++
		}
	}
	score := 0
	if len(checks) > 0 {
		score = (passed * 100) / len(checks)
	}

	return PostProcessAssessment{
		Checks: checks,
		Score:  score,
		Issues: issues,
	}
}

func generateSummary(input InputAssessment, postProcess PostProcessAssessment, finalScore int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Deterministic Assessment Score: %d/100\n\n", finalScore))

	sb.WriteString(fmt.Sprintf("Input Preparation: %d/100\n", input.Score))
	if len(input.Issues) > 0 {
		sb.WriteString("  Issues:\n")
		for _, issue := range input.Issues[:min(3, len(input.Issues))] {
			sb.WriteString(fmt.Sprintf("    - %s\n", issue))
		}
	}

	sb.WriteString(fmt.Sprintf("\nPost-Processing: %d/100\n", postProcess.Score))
	if len(postProcess.Issues) > 0 {
		sb.WriteString("  Issues:\n")
		for _, issue := range postProcess.Issues[:min(3, len(postProcess.Issues))] {
			sb.WriteString(fmt.Sprintf("    - %s\n", issue))
		}
	}

	if finalScore == 100 {
		sb.WriteString("\nPERFECT SCORE: Deterministic code is working correctly!")
	} else if finalScore >= 90 {
		sb.WriteString("\nNear perfect - minor issues to address.")
	} else if finalScore >= 70 {
		sb.WriteString("\nGood but needs improvement in the areas listed above.")
	} else {
		sb.WriteString("\nSignificant issues need to be addressed.")
	}

	return sb.String()
}

func boolToStatus(b bool, trueMsg, falseMsg string) string {
	if b {
		return trueMsg
	}
	return falseMsg
}

func dedupeStrings(strs []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

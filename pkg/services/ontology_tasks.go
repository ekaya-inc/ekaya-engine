package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// TenantContextFunc acquires a tenant-scoped database connection for background work.
// Returns the scoped context, a cleanup function (MUST be called), and any error.
// Using a function type instead of interface - easiest to test with inline closures.
type TenantContextFunc func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error)

// NewTenantContextFunc creates a TenantContextFunc that uses the given database.
func NewTenantContextFunc(db *database.DB) TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := db.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}
}

// BuildTieredOntologyTask builds Tier 0 (domain summary) and Tier 1 (entity summaries).
// This is an LLM task - only one can run at a time.
type BuildTieredOntologyTask struct {
	workqueue.BaseTask
	builder      OntologyBuilderService
	getTenantCtx TenantContextFunc
	projectID    uuid.UUID
	workflowID   uuid.UUID
}

// NewBuildTieredOntologyTask creates a new build task.
func NewBuildTieredOntologyTask(
	builder OntologyBuilderService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
) *BuildTieredOntologyTask {
	return &BuildTieredOntologyTask{
		BaseTask:     workqueue.NewBaseTask("Build Tiered Ontology", true),
		builder:      builder,
		getTenantCtx: getTenantCtx,
		projectID:    projectID,
		workflowID:   workflowID,
	}
}

// Execute implements workqueue.Task.
func (t *BuildTieredOntologyTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	return t.builder.BuildTieredOntology(tenantCtx, t.projectID, t.workflowID)
}

// InitializeOntologyTask is a quick non-LLM task that sets up the extraction workflow.
// It loads tables from schema and enqueues child tasks for processing.
type InitializeOntologyTask struct {
	workqueue.BaseTask
	schemaRepo        repositories.SchemaRepository
	ontologyRepo      repositories.OntologyRepository
	workflowStateRepo repositories.WorkflowStateRepository
	dsSvc             DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	builder           OntologyBuilderService
	workflowService   OntologyWorkflowService
	getTenantCtx      TenantContextFunc
	projectID         uuid.UUID
	workflowID        uuid.UUID
	datasourceID      uuid.UUID
	description       string
}

// NewInitializeOntologyTask creates a new initialization task.
func NewInitializeOntologyTask(
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	workflowStateRepo repositories.WorkflowStateRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	builder OntologyBuilderService,
	workflowService OntologyWorkflowService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	description string,
) *InitializeOntologyTask {
	return &InitializeOntologyTask{
		BaseTask:          workqueue.NewBaseTask("Initialize Ontology", false), // Non-LLM task
		schemaRepo:        schemaRepo,
		ontologyRepo:      ontologyRepo,
		workflowStateRepo: workflowStateRepo,
		dsSvc:             dsSvc,
		adapterFactory:    adapterFactory,
		builder:           builder,
		workflowService:   workflowService,
		getTenantCtx:      getTenantCtx,
		projectID:         projectID,
		workflowID:        workflowID,
		datasourceID:      datasourceID,
		description:       description,
	}
}

// Execute implements workqueue.Task.
// This task loads tables from schema, updates workflow progress, and optionally
// enqueues the project description task. The orchestrator handles scanning/analyzing.
func (t *InitializeOntologyTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Load tables from schema
	tables, err := t.schemaRepo.ListTablesByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("load tables: %w", err)
	}
	tableCount := len(tables)

	// Load all columns and group by table name
	allColumns, err := t.schemaRepo.ListColumnsByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("load columns: %w", err)
	}

	// Build map of table ID -> column names
	tableIDToColumns := make(map[string][]string)
	for _, col := range allColumns {
		tableIDToColumns[col.SchemaTableID.String()] = append(tableIDToColumns[col.SchemaTableID.String()], col.ColumnName)
	}

	// Count total columns
	columnCount := len(allColumns)

	// Scanning phase entities = tables + columns (no global entity)
	scanningEntities := tableCount + columnCount

	// Update progress for Scanning phase
	_ = t.workflowService.UpdateProgress(tenantCtx, t.workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseScanning,
		Message:      fmt.Sprintf("Scanning %d tables, %d columns...", tableCount, columnCount),
		Current:      0,
		Total:        scanningEntities,
	})

	// Enqueue UnderstandProjectDescriptionTask if description provided (LLM task)
	// This runs before the orchestrator takes over, processing the user's description
	if t.description != "" {
		totalEntities := 1 + tableCount + columnCount // For description task progress
		descTask := NewUnderstandProjectDescriptionTask(
			t.builder,
			t.workflowService,
			t.getTenantCtx,
			t.projectID,
			t.workflowID,
			t.description,
			totalEntities,
		)
		enqueuer.Enqueue(descTask)
	}

	// NOTE: Task chaining removed in chunk 5.
	// The orchestrator will detect pending entities and enqueue scan/analyze tasks.

	return nil
}

// UnderstandProjectDescriptionTask processes the user's project description with LLM.
// This is an LLM task - only one can run at a time.
type UnderstandProjectDescriptionTask struct {
	workqueue.BaseTask
	builder         OntologyBuilderService
	workflowService OntologyWorkflowService
	getTenantCtx    TenantContextFunc
	projectID       uuid.UUID
	workflowID      uuid.UUID
	description     string
	tableCount      int
}

// NewUnderstandProjectDescriptionTask creates a new description processing task.
func NewUnderstandProjectDescriptionTask(
	builder OntologyBuilderService,
	workflowService OntologyWorkflowService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	description string,
	tableCount int,
) *UnderstandProjectDescriptionTask {
	return &UnderstandProjectDescriptionTask{
		BaseTask:        workqueue.NewBaseTask("Understand Project Description", true), // LLM task
		builder:         builder,
		workflowService: workflowService,
		getTenantCtx:    getTenantCtx,
		projectID:       projectID,
		workflowID:      workflowID,
		description:     description,
		tableCount:      tableCount,
	}
}

// Execute implements workqueue.Task.
func (t *UnderstandProjectDescriptionTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Update progress
	_ = t.workflowService.UpdateProgress(tenantCtx, t.workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseDescriptionProcessing,
		Message:      "Analyzing your project description...",
		Current:      0,
		Total:        t.tableCount,
	})

	// Process description with LLM
	_, err = t.builder.ProcessProjectDescription(tenantCtx, t.projectID, t.workflowID, t.description)
	if err != nil {
		// Non-fatal - continue without description context
		// ProcessProjectDescription stores results in metadata even on partial failure
	}

	// Update progress - transition to scanning phase
	_ = t.workflowService.UpdateProgress(tenantCtx, t.workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseScanning,
		Message:      "Scanning database tables...",
		Current:      0,
		Total:        t.tableCount,
	})

	return nil
}

// NOTE: UpdateEntityDataTask was removed in chunk 5.
// The orchestrator now enqueues UnderstandEntityTask directly when it sees scanned entities.

// UnderstandEntityTask uses LLM to analyze an entity and generate clarifying questions.
// This is an LLM task - only one can run at a time.
// Questions are written to the engine_ontology_questions table (decoupled from workflow lifecycle).
// Entities are always marked complete after analysis, regardless of whether questions exist.
type UnderstandEntityTask struct {
	workqueue.BaseTask
	ontologyRepo      repositories.OntologyRepository
	questionRepo      repositories.OntologyQuestionRepository
	workflowStateRepo repositories.WorkflowStateRepository
	builder           OntologyBuilderService
	workflowService   OntologyWorkflowService
	getTenantCtx      TenantContextFunc
	projectID         uuid.UUID
	workflowID        uuid.UUID
	ontologyID        uuid.UUID
	tableName         string
	schemaName        string
	columnNames       []string // Column names for this table
}

// NewUnderstandEntityTask creates a new entity analysis task.
func NewUnderstandEntityTask(
	ontologyRepo repositories.OntologyRepository,
	questionRepo repositories.OntologyQuestionRepository,
	workflowStateRepo repositories.WorkflowStateRepository,
	builder OntologyBuilderService,
	workflowService OntologyWorkflowService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	ontologyID uuid.UUID,
	tableName string,
	schemaName string,
	columnNames []string,
) *UnderstandEntityTask {
	displayName := tableName
	if schemaName != "" && schemaName != "public" {
		displayName = schemaName + "." + tableName
	}
	return &UnderstandEntityTask{
		BaseTask:          workqueue.NewBaseTask(fmt.Sprintf("Analyze %s", displayName), true), // LLM task
		ontologyRepo:      ontologyRepo,
		questionRepo:      questionRepo,
		workflowStateRepo: workflowStateRepo,
		builder:           builder,
		workflowService:   workflowService,
		getTenantCtx:      getTenantCtx,
		projectID:         projectID,
		workflowID:        workflowID,
		ontologyID:        ontologyID,
		tableName:         tableName,
		schemaName:        schemaName,
		columnNames:       columnNames,
	}
}

// Execute implements workqueue.Task.
// Analyzes the entity with LLM, writes questions to the questions table, and marks entity as complete.
// Questions are decoupled from workflow - the entity is always marked complete after analysis.
func (t *UnderstandEntityTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Update table workflow state status to analyzing
	tableWS, err := t.workflowStateRepo.GetByEntity(tenantCtx, t.workflowID, models.WorkflowEntityTypeTable, t.tableName)
	if err != nil {
		return fmt.Errorf("get table workflow state: %w", err)
	}
	if tableWS == nil {
		return fmt.Errorf("table workflow state not found: %s", t.tableName)
	}
	if err := t.workflowStateRepo.UpdateStatus(tenantCtx, tableWS.ID, models.WorkflowEntityStatusAnalyzing, nil); err != nil {
		return fmt.Errorf("update table workflow state status to analyzing: %w", err)
	}

	// Analyze entity with LLM - may generate clarifying questions
	questions, err := t.builder.AnalyzeEntity(tenantCtx, t.projectID, t.workflowID, t.tableName)
	if err != nil {
		return fmt.Errorf("analyze entity: %w", err)
	}

	// Deduplicate questions against existing ones in this ontology
	if len(questions) > 0 {
		existingQuestions, err := t.questionRepo.ListByOntologyID(tenantCtx, t.ontologyID)
		if err != nil {
			return fmt.Errorf("load existing questions for deduplication: %w", err)
		}
		questions = deduplicateQuestions(questions, existingQuestions)
	}

	// Write questions to the dedicated questions table (decoupled from workflow lifecycle)
	if len(questions) > 0 {
		// Populate project and ontology IDs for each question
		for _, q := range questions {
			q.ProjectID = t.projectID
			q.OntologyID = t.ontologyID
		}
		if err := t.questionRepo.CreateBatch(tenantCtx, questions); err != nil {
			return fmt.Errorf("create questions: %w", err)
		}
	}

	// Collect question IDs for audit trail
	questionIDs := make([]string, len(questions))
	hasRequiredQuestions := false
	for i, q := range questions {
		questionIDs[i] = q.ID.String()
		if q.IsRequired {
			hasRequiredQuestions = true
		}
	}

	// Refresh table workflow state to get latest version
	tableWS, err = t.workflowStateRepo.GetByEntity(tenantCtx, t.workflowID, models.WorkflowEntityTypeTable, t.tableName)
	if err != nil {
		return fmt.Errorf("refresh table workflow state: %w", err)
	}
	if tableWS == nil {
		return fmt.Errorf("table workflow state not found after refresh: %s", t.tableName)
	}

	// Always mark entity as complete - questions are handled separately
	tableWS.Status = models.WorkflowEntityStatusComplete
	if tableWS.StateData == nil {
		tableWS.StateData = &models.WorkflowStateData{}
	}
	tableWS.StateData.LLMAnalysis = map[string]any{
		"question_ids":           questionIDs,
		"question_count":         len(questions),
		"has_required_questions": hasRequiredQuestions,
		"analyzed_at":            time.Now(),
	}
	if err := t.workflowStateRepo.Update(tenantCtx, tableWS); err != nil {
		return fmt.Errorf("update table workflow state with LLM analysis: %w", err)
	}

	// Mark all columns as complete (entity is always complete now)
	for _, colName := range t.columnNames {
		colEntityKey := models.ColumnEntityKey(t.tableName, colName)
		colWS, err := t.workflowStateRepo.GetByEntity(tenantCtx, t.workflowID, models.WorkflowEntityTypeColumn, colEntityKey)
		if err != nil {
			return fmt.Errorf("get column workflow state for %s: %w", colName, err)
		}
		if colWS == nil {
			return fmt.Errorf("column workflow state not found: %s", colEntityKey)
		}
		if err := t.workflowStateRepo.UpdateStatus(tenantCtx, colWS.ID, models.WorkflowEntityStatusComplete, nil); err != nil {
			return fmt.Errorf("update column workflow state status for %s: %w", colName, err)
		}
	}

	return nil
}

// ScanTableDataTask scans all columns in a table to collect statistics.
// This is a non-LLM data task that runs during the Scanning phase.
// It collects row counts, distinct values, null percentages, and enum candidates.
type ScanTableDataTask struct {
	workqueue.BaseTask
	ontologyRepo      repositories.OntologyRepository
	workflowStateRepo repositories.WorkflowStateRepository
	dsSvc             DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	getTenantCtx      TenantContextFunc
	projectID         uuid.UUID
	workflowID        uuid.UUID
	datasourceID      uuid.UUID
	tableName         string
	schemaName        string
	columnNames       []string
}

// NewScanTableDataTask creates a new scan task for a table.
func NewScanTableDataTask(
	ontologyRepo repositories.OntologyRepository,
	workflowStateRepo repositories.WorkflowStateRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	tableName string,
	schemaName string,
	columnNames []string,
) *ScanTableDataTask {
	displayName := tableName
	if schemaName != "" && schemaName != "public" {
		displayName = schemaName + "." + tableName
	}
	return &ScanTableDataTask{
		BaseTask:          workqueue.NewBaseTask(fmt.Sprintf("Scan %s", displayName), false), // Non-LLM task
		ontologyRepo:      ontologyRepo,
		workflowStateRepo: workflowStateRepo,
		dsSvc:             dsSvc,
		adapterFactory:    adapterFactory,
		getTenantCtx:      getTenantCtx,
		projectID:         projectID,
		workflowID:        workflowID,
		datasourceID:      datasourceID,
		tableName:         tableName,
		schemaName:        schemaName,
		columnNames:       columnNames,
	}
}

// Execute implements workqueue.Task.
// Scans all columns in the table and updates their scan data in workflow state.
func (t *ScanTableDataTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Update table workflow state status to scanning (pending -> scanning transition)
	tableWS, err := t.workflowStateRepo.GetByEntity(tenantCtx, t.workflowID, models.WorkflowEntityTypeTable, t.tableName)
	if err != nil {
		return fmt.Errorf("get table workflow state: %w", err)
	}
	if tableWS == nil {
		return fmt.Errorf("table workflow state not found: %s", t.tableName)
	}
	if err := t.workflowStateRepo.UpdateStatus(tenantCtx, tableWS.ID, models.WorkflowEntityStatusScanning, nil); err != nil {
		return fmt.Errorf("update table workflow state status to scanning: %w", err)
	}

	// Get datasource with decrypted config
	ds, err := t.dsSvc.Get(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer for background task.
	// Background tasks use empty userID since they run outside user session context.
	// Connection manager pools by (projectID, userID, datasourceID), so empty userID
	// means all background tasks for this project share one connection pool.
	discoverer, err := t.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, t.projectID, t.datasourceID, "")
	if err != nil {
		return fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Get column statistics
	stats, err := discoverer.AnalyzeColumnStats(ctx, t.schemaName, t.tableName, t.columnNames)
	if err != nil {
		return fmt.Errorf("analyze column stats: %w", err)
	}

	// Build map for quick lookup
	statsMap := make(map[string]datasource.ColumnStats)
	for _, s := range stats {
		statsMap[s.ColumnName] = s
	}

	// Scan each column and update workflow state with gathered data
	for _, colName := range t.columnNames {
		colStats := statsMap[colName]

		// Get sample values (up to 50)
		sampleValues, err := discoverer.GetDistinctValues(ctx, t.schemaName, t.tableName, colName, 50)
		if err != nil {
			// Log but continue - some columns may not be scannable (e.g., binary types)
			sampleValues = nil
		}

		// Calculate null percentage
		nullPercent := 0.0
		if colStats.RowCount > 0 {
			nullPercent = float64(colStats.RowCount-colStats.NonNullCount) / float64(colStats.RowCount) * 100
		}

		// Determine if column is an enum candidate
		// Heuristic: distinct_count <= 50 AND distinct_count < row_count * 0.1
		isEnumCandidate := false
		if colStats.DistinctCount > 0 && colStats.DistinctCount <= 50 && colStats.RowCount > 0 {
			if float64(colStats.DistinctCount) < float64(colStats.RowCount)*0.1 {
				isEnumCandidate = true
			}
		}

		// Compute value fingerprint for change detection
		fingerprint := ""
		if len(sampleValues) > 0 {
			sorted := make([]string, len(sampleValues))
			copy(sorted, sampleValues)
			sort.Strings(sorted)
			hash := sha256.Sum256([]byte(fmt.Sprintf("%v", sorted)))
			fingerprint = fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes
		}

		scannedAt := time.Now()

		// Update column workflow state with gathered data
		colEntityKey := models.ColumnEntityKey(t.tableName, colName)
		ws, err := t.workflowStateRepo.GetByEntity(tenantCtx, t.workflowID, models.WorkflowEntityTypeColumn, colEntityKey)
		if err != nil {
			return fmt.Errorf("get column workflow state: %w", err)
		}
		if ws == nil {
			return fmt.Errorf("column workflow state not found: %s", colEntityKey)
		}

		ws.Status = models.WorkflowEntityStatusScanned
		ws.StateData = &models.WorkflowStateData{
			Gathered: map[string]any{
				"row_count":         colStats.RowCount,
				"non_null_count":    colStats.NonNullCount,
				"distinct_count":    colStats.DistinctCount,
				"null_percent":      nullPercent,
				"sample_values":     sampleValues,
				"is_enum_candidate": isEnumCandidate,
				"value_fingerprint": fingerprint,
				"scanned_at":        scannedAt,
			},
		}
		if err := t.workflowStateRepo.Update(tenantCtx, ws); err != nil {
			return fmt.Errorf("update column workflow state: %w", err)
		}
	}

	// Update table workflow state status to scanned
	if err := t.workflowStateRepo.UpdateStatus(tenantCtx, tableWS.ID, models.WorkflowEntityStatusScanned, nil); err != nil {
		return fmt.Errorf("update table workflow state status to scanned: %w", err)
	}

	return nil
}

// NOTE: TransitionToAnalyzingTask was removed in chunk 5.
// The orchestrator handles phase transitions and task enqueueing directly.

// ============================================================================
// Question Deduplication
// ============================================================================

// deduplicateQuestions removes questions that are substantially similar to existing ones.
// This prevents asking "What is marker_at?" multiple times across different tables.
func deduplicateQuestions(newQuestions []*models.OntologyQuestion, existingQuestions []*models.OntologyQuestion) []*models.OntologyQuestion {
	if len(existingQuestions) == 0 {
		return newQuestions
	}

	result := make([]*models.OntologyQuestion, 0, len(newQuestions))
	for _, newQ := range newQuestions {
		isDuplicate := false
		for _, existingQ := range existingQuestions {
			if isSimilarQuestion(newQ.Text, existingQ.Text) {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			result = append(result, newQ)
		}
	}

	return result
}

// columnNamePattern matches common column name patterns in questions
var columnNamePattern = regexp.MustCompile(`['"\x60]([a-z_][a-z0-9_]*)['"\x60]`)

// isSimilarQuestion checks if two questions are asking about the same thing.
// Two questions are considered similar if they:
// 1. Reference the same column name AND
// 2. Ask the same type of question (what/why/how)
func isSimilarQuestion(q1, q2 string) bool {
	q1Lower := strings.ToLower(q1)
	q2Lower := strings.ToLower(q2)

	// Extract column names from both questions
	cols1 := extractColumnNames(q1Lower)
	cols2 := extractColumnNames(q2Lower)

	// Find common column names
	commonCols := findCommonStrings(cols1, cols2)
	if len(commonCols) == 0 {
		return false
	}

	// Check if both questions have similar intent (what/why/how/when/does/is)
	intent1 := extractQuestionIntent(q1Lower)
	intent2 := extractQuestionIntent(q2Lower)

	return intent1 == intent2 && intent1 != ""
}

// extractColumnNames finds column names mentioned in a question.
// Looks for quoted identifiers like 'column_name', "column_name", or `column_name`
func extractColumnNames(question string) []string {
	matches := columnNamePattern.FindAllStringSubmatch(question, -1)
	cols := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			cols = append(cols, m[1])
		}
	}
	return cols
}

// extractQuestionIntent identifies the type of question being asked.
func extractQuestionIntent(question string) string {
	// Check if question starts with specific patterns (most reliable)
	if strings.HasPrefix(question, "what ") {
		return "what"
	}
	if strings.HasPrefix(question, "why ") {
		return "why"
	}
	if strings.HasPrefix(question, "how ") {
		return "how"
	}
	if strings.HasPrefix(question, "when ") {
		return "when"
	}
	if strings.HasPrefix(question, "does ") || strings.HasPrefix(question, "is ") ||
		strings.HasPrefix(question, "are ") || strings.HasPrefix(question, "can ") {
		return "does"
	}
	return ""
}

// findCommonStrings returns strings that appear in both slices.
func findCommonStrings(a, b []string) []string {
	set := make(map[string]bool)
	for _, s := range a {
		set[s] = true
	}

	common := make([]string, 0)
	for _, s := range b {
		if set[s] {
			common = append(common, s)
		}
	}
	return common
}

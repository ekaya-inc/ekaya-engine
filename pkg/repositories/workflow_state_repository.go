package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// QuestionCounts holds required and optional pending question counts.
type QuestionCounts struct {
	Required int `json:"required"`
	Optional int `json:"optional"`
}

// WorkflowStateRepository provides data access for workflow entity states.
type WorkflowStateRepository interface {
	// CRUD operations
	Create(ctx context.Context, state *models.WorkflowEntityState) error
	CreateBatch(ctx context.Context, states []*models.WorkflowEntityState) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.WorkflowEntityState, error)
	Update(ctx context.Context, state *models.WorkflowEntityState) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Query methods
	ListByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.WorkflowEntityState, error)
	ListByStatus(ctx context.Context, workflowID uuid.UUID, status models.WorkflowEntityStatus) ([]*models.WorkflowEntityState, error)
	GetByEntity(ctx context.Context, workflowID uuid.UUID, entityType models.WorkflowEntityType, entityKey string) (*models.WorkflowEntityState, error)

	// Batch operations
	DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error

	// State updates
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.WorkflowEntityStatus, lastError *string) error
	UpdateStateData(ctx context.Context, id uuid.UUID, stateData *models.WorkflowStateData) error
	IncrementRetryCount(ctx context.Context, id uuid.UUID) error

	// Question operations (for state_data.questions)
	AddQuestionsToEntity(ctx context.Context, id uuid.UUID, questions []models.WorkflowQuestion) error
	UpdateQuestionInEntity(ctx context.Context, id uuid.UUID, questionID string, status string, answer string) error
	RecordAnswerInEntity(ctx context.Context, id uuid.UUID, answer models.WorkflowAnswer) error
	GetNextPendingQuestion(ctx context.Context, workflowID uuid.UUID) (*models.WorkflowQuestion, uuid.UUID, error)
	GetPendingQuestions(ctx context.Context, projectID uuid.UUID, limit int) ([]models.WorkflowQuestion, error)
	GetPendingQuestionsCount(ctx context.Context, workflowID uuid.UUID) (required int, optional int, err error)
	GetPendingQuestionsCountByProject(ctx context.Context, projectID uuid.UUID) (int, error)
	FindQuestionByID(ctx context.Context, questionID string) (*models.WorkflowQuestion, *models.WorkflowEntityState, *models.OntologyWorkflow, error)
}

type workflowStateRepository struct{}

// NewWorkflowStateRepository creates a new WorkflowStateRepository.
func NewWorkflowStateRepository() WorkflowStateRepository {
	return &workflowStateRepository{}
}

var _ WorkflowStateRepository = (*workflowStateRepository)(nil)

// ============================================================================
// Create Operations
// ============================================================================

func (r *workflowStateRepository) Create(ctx context.Context, state *models.WorkflowEntityState) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	state.CreatedAt = now
	state.UpdatedAt = now
	if state.ID == uuid.Nil {
		state.ID = uuid.New()
	}
	if state.Status == "" {
		state.Status = models.WorkflowEntityStatusPending
	}

	stateDataJSON, err := marshalStateData(state.StateData)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO engine_workflow_state (
			id, project_id, ontology_id, workflow_id,
			entity_type, entity_key, status, state_data,
			data_fingerprint, last_error, retry_count,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err = scope.Conn.Exec(ctx, query,
		state.ID, state.ProjectID, state.OntologyID, state.WorkflowID,
		state.EntityType, state.EntityKey, state.Status, stateDataJSON,
		state.DataFingerprint, state.LastError, state.RetryCount,
		state.CreatedAt, state.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create workflow state: %w", err)
	}

	return nil
}

func (r *workflowStateRepository) CreateBatch(ctx context.Context, states []*models.WorkflowEntityState) error {
	if len(states) == 0 {
		return nil
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()

	// Use COPY for efficient batch insert
	columns := []string{
		"id", "project_id", "ontology_id", "workflow_id",
		"entity_type", "entity_key", "status", "state_data",
		"data_fingerprint", "last_error", "retry_count",
		"created_at", "updated_at",
	}

	rows := make([][]any, len(states))
	for i, state := range states {
		if state.ID == uuid.Nil {
			state.ID = uuid.New()
		}
		state.CreatedAt = now
		state.UpdatedAt = now
		if state.Status == "" {
			state.Status = models.WorkflowEntityStatusPending
		}

		stateDataJSON, err := marshalStateData(state.StateData)
		if err != nil {
			return err
		}

		rows[i] = []any{
			state.ID, state.ProjectID, state.OntologyID, state.WorkflowID,
			state.EntityType, state.EntityKey, state.Status, stateDataJSON,
			state.DataFingerprint, state.LastError, state.RetryCount,
			state.CreatedAt, state.UpdatedAt,
		}
	}

	_, err := scope.Conn.CopyFrom(
		ctx,
		pgx.Identifier{"engine_workflow_state"},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to batch create workflow states: %w", err)
	}

	return nil
}

// ============================================================================
// Read Operations
// ============================================================================

func (r *workflowStateRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.WorkflowEntityState, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, workflow_id,
		       entity_type, entity_key, status, state_data,
		       data_fingerprint, last_error, retry_count,
		       created_at, updated_at
		FROM engine_workflow_state
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, id)
	state, err := scanWorkflowStateRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

func (r *workflowStateRepository) GetByEntity(ctx context.Context, workflowID uuid.UUID, entityType models.WorkflowEntityType, entityKey string) (*models.WorkflowEntityState, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, workflow_id,
		       entity_type, entity_key, status, state_data,
		       data_fingerprint, last_error, retry_count,
		       created_at, updated_at
		FROM engine_workflow_state
		WHERE workflow_id = $1 AND entity_type = $2 AND entity_key = $3`

	row := scope.Conn.QueryRow(ctx, query, workflowID, entityType, entityKey)
	state, err := scanWorkflowStateRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

func (r *workflowStateRepository) ListByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.WorkflowEntityState, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, workflow_id,
		       entity_type, entity_key, status, state_data,
		       data_fingerprint, last_error, retry_count,
		       created_at, updated_at
		FROM engine_workflow_state
		WHERE workflow_id = $1
		ORDER BY entity_type, entity_key`

	rows, err := scope.Conn.Query(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow states: %w", err)
	}
	defer rows.Close()

	return scanWorkflowStateRows(rows)
}

func (r *workflowStateRepository) ListByStatus(ctx context.Context, workflowID uuid.UUID, status models.WorkflowEntityStatus) ([]*models.WorkflowEntityState, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, workflow_id,
		       entity_type, entity_key, status, state_data,
		       data_fingerprint, last_error, retry_count,
		       created_at, updated_at
		FROM engine_workflow_state
		WHERE workflow_id = $1 AND status = $2
		ORDER BY entity_type, entity_key`

	rows, err := scope.Conn.Query(ctx, query, workflowID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow states by status: %w", err)
	}
	defer rows.Close()

	return scanWorkflowStateRows(rows)
}

// ============================================================================
// Update Operations
// ============================================================================

func (r *workflowStateRepository) Update(ctx context.Context, state *models.WorkflowEntityState) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	state.UpdatedAt = time.Now()

	stateDataJSON, err := marshalStateData(state.StateData)
	if err != nil {
		return err
	}

	query := `
		UPDATE engine_workflow_state
		SET entity_type = $2,
		    entity_key = $3,
		    status = $4,
		    state_data = $5,
		    data_fingerprint = $6,
		    last_error = $7,
		    retry_count = $8,
		    updated_at = $9
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query,
		state.ID, state.EntityType, state.EntityKey, state.Status, stateDataJSON,
		state.DataFingerprint, state.LastError, state.RetryCount, state.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update workflow state: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WorkflowEntityStatus, lastError *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_workflow_state
		SET status = $2,
		    last_error = $3,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, status, lastError)
	if err != nil {
		return fmt.Errorf("failed to update workflow state status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) UpdateStateData(ctx context.Context, id uuid.UUID, stateData *models.WorkflowStateData) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	stateDataJSON, err := marshalStateData(stateData)
	if err != nil {
		return err
	}

	query := `
		UPDATE engine_workflow_state
		SET state_data = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, stateDataJSON)
	if err != nil {
		return fmt.Errorf("failed to update workflow state data: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_workflow_state
		SET retry_count = retry_count + 1,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

// ============================================================================
// Delete Operations
// ============================================================================

func (r *workflowStateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_workflow_state WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow state: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_workflow_state WHERE workflow_id = $1`

	_, err := scope.Conn.Exec(ctx, query, workflowID)
	if err != nil {
		return fmt.Errorf("failed to delete workflow states: %w", err)
	}

	return nil
}

func (r *workflowStateRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_workflow_state WHERE ontology_id = $1`

	_, err := scope.Conn.Exec(ctx, query, ontologyID)
	if err != nil {
		return fmt.Errorf("failed to delete workflow states by ontology: %w", err)
	}

	return nil
}

// ============================================================================
// Question Operations
// ============================================================================

func (r *workflowStateRepository) AddQuestionsToEntity(ctx context.Context, id uuid.UUID, questions []models.WorkflowQuestion) error {
	if len(questions) == 0 {
		return nil
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Get current state
	state, err := r.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get workflow state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("workflow state not found: %s", id)
	}

	// Initialize state_data if nil
	if state.StateData == nil {
		state.StateData = &models.WorkflowStateData{}
	}

	// Append questions
	state.StateData.Questions = append(state.StateData.Questions, questions...)

	stateDataJSON, err := marshalStateData(state.StateData)
	if err != nil {
		return err
	}

	query := `
		UPDATE engine_workflow_state
		SET state_data = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, stateDataJSON)
	if err != nil {
		return fmt.Errorf("failed to add questions to entity: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) UpdateQuestionInEntity(ctx context.Context, id uuid.UUID, questionID string, status string, answer string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Get current state
	state, err := r.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get workflow state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("workflow state not found: %s", id)
	}

	if state.StateData == nil || len(state.StateData.Questions) == 0 {
		return fmt.Errorf("no questions in entity state: %s", id)
	}

	// Find and update the question
	found := false
	for i := range state.StateData.Questions {
		if state.StateData.Questions[i].ID == questionID {
			state.StateData.Questions[i].Status = status
			if answer != "" {
				state.StateData.Questions[i].Answer = answer
				now := time.Now()
				state.StateData.Questions[i].AnsweredAt = &now
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("question %s not found in entity state: %s", questionID, id)
	}

	stateDataJSON, err := marshalStateData(state.StateData)
	if err != nil {
		return err
	}

	query := `
		UPDATE engine_workflow_state
		SET state_data = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, stateDataJSON)
	if err != nil {
		return fmt.Errorf("failed to update question in entity: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) RecordAnswerInEntity(ctx context.Context, id uuid.UUID, answer models.WorkflowAnswer) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Get current state
	state, err := r.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get workflow state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("workflow state not found: %s", id)
	}

	// Initialize state_data if nil
	if state.StateData == nil {
		state.StateData = &models.WorkflowStateData{}
	}

	// Append answer
	state.StateData.Answers = append(state.StateData.Answers, answer)

	stateDataJSON, err := marshalStateData(state.StateData)
	if err != nil {
		return err
	}

	query := `
		UPDATE engine_workflow_state
		SET state_data = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, stateDataJSON)
	if err != nil {
		return fmt.Errorf("failed to record answer in entity: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow state not found")
	}

	return nil
}

func (r *workflowStateRepository) GetNextPendingQuestion(ctx context.Context, workflowID uuid.UUID) (*models.WorkflowQuestion, uuid.UUID, error) {
	// Get all states for the workflow
	states, err := r.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("failed to list workflow states: %w", err)
	}

	// Find the highest priority pending question across all entities
	// Priority 1 = highest, so we look for the lowest number
	var bestQuestion *models.WorkflowQuestion
	var bestEntityID uuid.UUID
	bestPriority := 999

	for _, state := range states {
		if state.StateData == nil || len(state.StateData.Questions) == 0 {
			continue
		}

		for i := range state.StateData.Questions {
			q := &state.StateData.Questions[i]
			if q.Status == string(models.QuestionStatusPending) {
				// Prefer required questions over optional
				effectivePriority := q.Priority
				if !q.IsRequired {
					effectivePriority += 100 // Push optional questions lower
				}

				if effectivePriority < bestPriority {
					bestPriority = effectivePriority
					bestQuestion = q
					bestEntityID = state.ID
				}
			}
		}
	}

	if bestQuestion == nil {
		return nil, uuid.Nil, nil
	}

	return bestQuestion, bestEntityID, nil
}

func (r *workflowStateRepository) GetPendingQuestionsCount(ctx context.Context, workflowID uuid.UUID) (required int, optional int, err error) {
	// Get all states for the workflow
	states, err := r.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list workflow states: %w", err)
	}

	for _, state := range states {
		if state.StateData == nil || len(state.StateData.Questions) == 0 {
			continue
		}

		for _, q := range state.StateData.Questions {
			if q.Status == string(models.QuestionStatusPending) {
				if q.IsRequired {
					required++
				} else {
					optional++
				}
			}
		}
	}

	return required, optional, nil
}

func (r *workflowStateRepository) GetPendingQuestions(ctx context.Context, projectID uuid.UUID, limit int) ([]models.WorkflowQuestion, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Query all workflow states for this project with pending questions.
	// We extract questions from JSONB and filter for pending status.
	query := `
		SELECT q.value
		FROM engine_workflow_state s
		JOIN engine_ontology_workflows w ON s.workflow_id = w.id
		CROSS JOIN LATERAL jsonb_array_elements(COALESCE(s.state_data->'questions', '[]'::jsonb)) AS q
		WHERE w.project_id = $1
		  AND q.value->>'status' = 'pending'
		ORDER BY
			CASE WHEN (q.value->>'is_required')::boolean THEN 0 ELSE 1 END,
			COALESCE((q.value->>'priority')::int, 0) DESC
		LIMIT $2`

	rows, err := scope.Conn.Query(ctx, query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending questions: %w", err)
	}
	defer rows.Close()

	var questions []models.WorkflowQuestion
	for rows.Next() {
		var questionJSON []byte
		if err := rows.Scan(&questionJSON); err != nil {
			return nil, fmt.Errorf("failed to scan question: %w", err)
		}

		var q models.WorkflowQuestion
		if err := json.Unmarshal(questionJSON, &q); err != nil {
			return nil, fmt.Errorf("failed to unmarshal question: %w", err)
		}
		questions = append(questions, q)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating questions: %w", err)
	}

	return questions, nil
}

func (r *workflowStateRepository) GetPendingQuestionsCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	// Count pending questions across all workflows for this project.
	query := `
		SELECT COUNT(*)
		FROM engine_workflow_state s
		JOIN engine_ontology_workflows w ON s.workflow_id = w.id
		CROSS JOIN LATERAL jsonb_array_elements(COALESCE(s.state_data->'questions', '[]'::jsonb)) AS q
		WHERE w.project_id = $1
		  AND q.value->>'status' = 'pending'`

	var count int
	if err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count pending questions: %w", err)
	}

	return count, nil
}

func (r *workflowStateRepository) FindQuestionByID(ctx context.Context, questionID string) (*models.WorkflowQuestion, *models.WorkflowEntityState, *models.OntologyWorkflow, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, nil, nil, fmt.Errorf("no tenant scope in context")
	}

	// Search for the question in all entity states using JSONB query.
	// This is efficient because we're using @> containment operator on an indexed JSONB path.
	query := `
		SELECT s.id, s.project_id, s.ontology_id, s.workflow_id,
		       s.entity_type, s.entity_key, s.status, s.state_data,
		       s.data_fingerprint, s.last_error, s.retry_count,
		       s.created_at, s.updated_at,
		       w.id, w.project_id, w.ontology_id, w.state, w.error_message,
		       w.owner_id, w.last_heartbeat, w.created_at, w.updated_at
		FROM engine_workflow_state s
		JOIN engine_ontology_workflows w ON s.workflow_id = w.id
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(COALESCE(s.state_data->'questions', '[]'::jsonb)) q
			WHERE q->>'id' = $1
		)`

	row := scope.Conn.QueryRow(ctx, query, questionID)

	var ws models.WorkflowEntityState
	var wf models.OntologyWorkflow
	var stateDataJSON []byte

	err := row.Scan(
		&ws.ID, &ws.ProjectID, &ws.OntologyID, &ws.WorkflowID,
		&ws.EntityType, &ws.EntityKey, &ws.Status, &stateDataJSON,
		&ws.DataFingerprint, &ws.LastError, &ws.RetryCount,
		&ws.CreatedAt, &ws.UpdatedAt,
		&wf.ID, &wf.ProjectID, &wf.OntologyID, &wf.State, &wf.ErrorMessage,
		&wf.OwnerID, &wf.LastHeartbeat, &wf.CreatedAt, &wf.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, nil, fmt.Errorf("question not found: %s", questionID)
		}
		return nil, nil, nil, fmt.Errorf("failed to find question: %w", err)
	}

	// Parse state data
	if len(stateDataJSON) > 0 && string(stateDataJSON) != "{}" {
		ws.StateData = &models.WorkflowStateData{}
		if err := json.Unmarshal(stateDataJSON, ws.StateData); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to unmarshal state_data: %w", err)
		}
	}

	// Find the specific question
	if ws.StateData != nil {
		for i := range ws.StateData.Questions {
			if ws.StateData.Questions[i].ID == questionID {
				return &ws.StateData.Questions[i], &ws, &wf, nil
			}
		}
	}

	return nil, nil, nil, fmt.Errorf("question not found in parsed state: %s", questionID)
}

// ============================================================================
// Helper Functions
// ============================================================================

func marshalStateData(stateData *models.WorkflowStateData) ([]byte, error) {
	if stateData == nil {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(stateData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state_data: %w", err)
	}
	return data, nil
}

func scanWorkflowStateRow(row pgx.Row) (*models.WorkflowEntityState, error) {
	var ws models.WorkflowEntityState
	var stateDataJSON []byte

	err := row.Scan(
		&ws.ID, &ws.ProjectID, &ws.OntologyID, &ws.WorkflowID,
		&ws.EntityType, &ws.EntityKey, &ws.Status, &stateDataJSON,
		&ws.DataFingerprint, &ws.LastError, &ws.RetryCount,
		&ws.CreatedAt, &ws.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan workflow state: %w", err)
	}

	if len(stateDataJSON) > 0 && string(stateDataJSON) != "{}" {
		ws.StateData = &models.WorkflowStateData{}
		if err := json.Unmarshal(stateDataJSON, ws.StateData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state_data: %w", err)
		}
	}

	return &ws, nil
}

func scanWorkflowStateRows(rows pgx.Rows) ([]*models.WorkflowEntityState, error) {
	var states []*models.WorkflowEntityState
	for rows.Next() {
		var ws models.WorkflowEntityState
		var stateDataJSON []byte

		err := rows.Scan(
			&ws.ID, &ws.ProjectID, &ws.OntologyID, &ws.WorkflowID,
			&ws.EntityType, &ws.EntityKey, &ws.Status, &stateDataJSON,
			&ws.DataFingerprint, &ws.LastError, &ws.RetryCount,
			&ws.CreatedAt, &ws.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workflow state row: %w", err)
		}

		if len(stateDataJSON) > 0 && string(stateDataJSON) != "{}" {
			ws.StateData = &models.WorkflowStateData{}
			if err := json.Unmarshal(stateDataJSON, ws.StateData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal state_data: %w", err)
			}
		}

		states = append(states, &ws)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workflow state rows: %w", err)
	}

	return states, nil
}

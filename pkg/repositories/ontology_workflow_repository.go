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

// OntologyWorkflowRepository provides data access for ontology workflows.
type OntologyWorkflowRepository interface {
	Create(ctx context.Context, workflow *models.OntologyWorkflow) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyWorkflow, error)
	GetByOntology(ctx context.Context, ontologyID uuid.UUID) (*models.OntologyWorkflow, error)
	GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error)
	GetLatestByDatasourceAndPhase(ctx context.Context, datasourceID uuid.UUID, phase models.WorkflowPhaseType) (*models.OntologyWorkflow, error)
	Update(ctx context.Context, workflow *models.OntologyWorkflow) error
	UpdateState(ctx context.Context, id uuid.UUID, state models.WorkflowState, errorMsg string) error
	UpdateProgress(ctx context.Context, id uuid.UUID, progress *models.WorkflowProgress) error
	UpdateTaskQueue(ctx context.Context, id uuid.UUID, tasks []models.WorkflowTask) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error

	// Ownership methods for multi-server robustness
	ClaimOwnership(ctx context.Context, workflowID, ownerID uuid.UUID) (bool, error)
	UpdateHeartbeat(ctx context.Context, workflowID, ownerID uuid.UUID) error
	ReleaseOwnership(ctx context.Context, workflowID uuid.UUID) error
}

type ontologyWorkflowRepository struct{}

// NewOntologyWorkflowRepository creates a new OntologyWorkflowRepository.
func NewOntologyWorkflowRepository() OntologyWorkflowRepository {
	return &ontologyWorkflowRepository{}
}

var _ OntologyWorkflowRepository = (*ontologyWorkflowRepository)(nil)

func (r *ontologyWorkflowRepository) Create(ctx context.Context, workflow *models.OntologyWorkflow) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	workflow.CreatedAt = now
	workflow.UpdatedAt = now
	if workflow.ID == uuid.Nil {
		workflow.ID = uuid.New()
	}

	progressJSON, err := json.Marshal(workflow.Progress)
	if err != nil {
		return fmt.Errorf("failed to marshal progress: %w", err)
	}
	if workflow.Progress == nil {
		progressJSON = []byte("{}")
	}

	taskQueueJSON, err := json.Marshal(workflow.TaskQueue)
	if err != nil {
		return fmt.Errorf("failed to marshal task_queue: %w", err)
	}
	if workflow.TaskQueue == nil {
		taskQueueJSON = []byte("[]")
	}

	configJSON, err := json.Marshal(workflow.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if workflow.Config == nil {
		configJSON = []byte("{}")
	}

	query := `
		INSERT INTO engine_ontology_workflows (
			id, project_id, ontology_id, state, progress, task_queue, config,
			error_message, started_at, completed_at, phase, datasource_id,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err = scope.Conn.Exec(ctx, query,
		workflow.ID, workflow.ProjectID, workflow.OntologyID, workflow.State, progressJSON, taskQueueJSON, configJSON,
		workflow.ErrorMessage, workflow.StartedAt, workflow.CompletedAt, workflow.Phase, workflow.DatasourceID,
		workflow.CreatedAt, workflow.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create workflow: %w", err)
	}

	return nil
}

func (r *ontologyWorkflowRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyWorkflow, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, state, progress, task_queue, config,
		       error_message, started_at, completed_at, phase, datasource_id,
		       created_at, updated_at, owner_id, last_heartbeat
		FROM engine_ontology_workflows
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, id)
	return scanOntologyWorkflowRow(row)
}

func (r *ontologyWorkflowRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) (*models.OntologyWorkflow, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, state, progress, task_queue, config,
		       error_message, started_at, completed_at, phase, datasource_id,
		       created_at, updated_at, owner_id, last_heartbeat
		FROM engine_ontology_workflows
		WHERE ontology_id = $1`

	row := scope.Conn.QueryRow(ctx, query, ontologyID)
	workflow, err := scanOntologyWorkflowRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No workflow found
		}
		return nil, err
	}
	return workflow, nil
}

func (r *ontologyWorkflowRepository) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, state, progress, task_queue, config,
		       error_message, started_at, completed_at, phase, datasource_id,
		       created_at, updated_at, owner_id, last_heartbeat
		FROM engine_ontology_workflows
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, projectID)
	workflow, err := scanOntologyWorkflowRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No workflow found
		}
		return nil, err
	}
	return workflow, nil
}

func (r *ontologyWorkflowRepository) GetLatestByDatasourceAndPhase(ctx context.Context, datasourceID uuid.UUID, phase models.WorkflowPhaseType) (*models.OntologyWorkflow, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, state, progress, task_queue, config,
		       error_message, started_at, completed_at, phase, datasource_id,
		       created_at, updated_at, owner_id, last_heartbeat
		FROM engine_ontology_workflows
		WHERE datasource_id = $1 AND phase = $2
		ORDER BY created_at DESC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, datasourceID, phase)
	workflow, err := scanOntologyWorkflowRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No workflow found
		}
		return nil, err
	}
	return workflow, nil
}

func (r *ontologyWorkflowRepository) Update(ctx context.Context, workflow *models.OntologyWorkflow) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	workflow.UpdatedAt = time.Now()

	progressJSON, err := json.Marshal(workflow.Progress)
	if err != nil {
		return fmt.Errorf("failed to marshal progress: %w", err)
	}
	if workflow.Progress == nil {
		progressJSON = []byte("{}")
	}

	taskQueueJSON, err := json.Marshal(workflow.TaskQueue)
	if err != nil {
		return fmt.Errorf("failed to marshal task_queue: %w", err)
	}
	if workflow.TaskQueue == nil {
		taskQueueJSON = []byte("[]")
	}

	configJSON, err := json.Marshal(workflow.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if workflow.Config == nil {
		configJSON = []byte("{}")
	}

	query := `
		UPDATE engine_ontology_workflows
		SET state = $2,
		    progress = $3,
		    task_queue = $4,
		    config = $5,
		    error_message = $6,
		    started_at = $7,
		    completed_at = $8,
		    phase = $9,
		    datasource_id = $10,
		    updated_at = $11
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query,
		workflow.ID, workflow.State, progressJSON, taskQueueJSON, configJSON,
		workflow.ErrorMessage, workflow.StartedAt, workflow.CompletedAt,
		workflow.Phase, workflow.DatasourceID, workflow.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found")
	}

	return nil
}

func (r *ontologyWorkflowRepository) UpdateState(ctx context.Context, id uuid.UUID, state models.WorkflowState, errorMsg string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	var completedAt *time.Time
	if state.IsTerminal() {
		now := time.Now()
		completedAt = &now
	}

	query := `
		UPDATE engine_ontology_workflows
		SET state = $2,
		    error_message = $3,
		    completed_at = COALESCE($4, completed_at),
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, state, errorMsg, completedAt)
	if err != nil {
		return fmt.Errorf("failed to update workflow state: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found")
	}

	return nil
}

func (r *ontologyWorkflowRepository) UpdateProgress(ctx context.Context, id uuid.UUID, progress *models.WorkflowProgress) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	progressJSON, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("failed to marshal progress: %w", err)
	}

	query := `
		UPDATE engine_ontology_workflows
		SET progress = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, progressJSON)
	if err != nil {
		return fmt.Errorf("failed to update workflow progress: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found")
	}

	return nil
}

func (r *ontologyWorkflowRepository) UpdateTaskQueue(ctx context.Context, id uuid.UUID, tasks []models.WorkflowTask) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	taskQueueJSON, err := json.Marshal(tasks)
	if err != nil {
		return fmt.Errorf("failed to marshal task_queue: %w", err)
	}

	query := `
		UPDATE engine_ontology_workflows
		SET task_queue = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, taskQueueJSON)
	if err != nil {
		return fmt.Errorf("failed to update workflow task queue: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found")
	}

	return nil
}

func (r *ontologyWorkflowRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_workflows WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found")
	}

	return nil
}

func (r *ontologyWorkflowRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_workflows WHERE project_id = $1`

	_, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete workflows: %w", err)
	}

	return nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanOntologyWorkflowRow(row pgx.Row) (*models.OntologyWorkflow, error) {
	var w models.OntologyWorkflow
	var progressJSON, taskQueueJSON, configJSON []byte

	err := row.Scan(
		&w.ID, &w.ProjectID, &w.OntologyID, &w.State, &progressJSON, &taskQueueJSON, &configJSON,
		&w.ErrorMessage, &w.StartedAt, &w.CompletedAt, &w.Phase, &w.DatasourceID,
		&w.CreatedAt, &w.UpdatedAt, &w.OwnerID, &w.LastHeartbeat,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan workflow: %w", err)
	}

	if len(progressJSON) > 0 && string(progressJSON) != "{}" {
		w.Progress = &models.WorkflowProgress{}
		if err := json.Unmarshal(progressJSON, w.Progress); err != nil {
			return nil, fmt.Errorf("failed to unmarshal progress: %w", err)
		}
	}

	if len(taskQueueJSON) > 0 && string(taskQueueJSON) != "[]" {
		if err := json.Unmarshal(taskQueueJSON, &w.TaskQueue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task_queue: %w", err)
		}
	}

	if len(configJSON) > 0 && string(configJSON) != "{}" {
		w.Config = &models.WorkflowConfig{}
		if err := json.Unmarshal(configJSON, w.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}

	return &w, nil
}

// ============================================================================
// Ownership Methods
// ============================================================================

// ClaimOwnership atomically claims a workflow if it's unowned or already owned by this server.
// Returns true if ownership was claimed, false if owned by another server.
func (r *ontologyWorkflowRepository) ClaimOwnership(ctx context.Context, workflowID, ownerID uuid.UUID) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	// Atomic update: only claim if unowned or already owned by us
	query := `
		UPDATE engine_ontology_workflows
		SET owner_id = $2,
		    last_heartbeat = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND (owner_id IS NULL OR owner_id = $2)
		RETURNING id`

	var returnedID uuid.UUID
	err := scope.Conn.QueryRow(ctx, query, workflowID, ownerID).Scan(&returnedID)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No rows returned means another server owns it
			return false, nil
		}
		return false, fmt.Errorf("failed to claim ownership: %w", err)
	}

	return true, nil
}

// UpdateHeartbeat updates the last_heartbeat timestamp for an owned workflow.
// Only updates if the workflow is owned by the specified owner.
func (r *ontologyWorkflowRepository) UpdateHeartbeat(ctx context.Context, workflowID, ownerID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_workflows
		SET last_heartbeat = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND owner_id = $2`

	result, err := scope.Conn.Exec(ctx, query, workflowID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found or not owned by this server")
	}

	return nil
}

// ReleaseOwnership clears the owner_id for a workflow, allowing other servers to claim it.
func (r *ontologyWorkflowRepository) ReleaseOwnership(ctx context.Context, workflowID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_workflows
		SET owner_id = NULL,
		    last_heartbeat = NULL,
		    updated_at = NOW()
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, workflowID)
	if err != nil {
		return fmt.Errorf("failed to release ownership: %w", err)
	}

	return nil
}

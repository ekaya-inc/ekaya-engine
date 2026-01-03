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

// OntologyDAGRepository provides data access for ontology DAGs.
type OntologyDAGRepository interface {
	// DAG operations
	Create(ctx context.Context, dag *models.OntologyDAG) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error)
	GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error)
	GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)
	GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)
	Update(ctx context.Context, dag *models.OntologyDAG) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.DAGStatus, currentNode *string) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error

	// Ownership methods for multi-server robustness
	ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error)
	UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error
	ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error

	// Node operations
	CreateNodes(ctx context.Context, nodes []models.DAGNode) error
	GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error)
	UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMsg *string) error
	UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
	IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error
	GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error)
}

type ontologyDAGRepository struct{}

// NewOntologyDAGRepository creates a new OntologyDAGRepository.
func NewOntologyDAGRepository() OntologyDAGRepository {
	return &ontologyDAGRepository{}
}

var _ OntologyDAGRepository = (*ontologyDAGRepository)(nil)

// ============================================================================
// DAG Operations
// ============================================================================

func (r *ontologyDAGRepository) Create(ctx context.Context, dag *models.OntologyDAG) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	dag.CreatedAt = now
	dag.UpdatedAt = now
	if dag.ID == uuid.Nil {
		dag.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_ontology_dag (
			id, project_id, datasource_id, ontology_id,
			status, current_node, schema_fingerprint,
			owner_id, last_heartbeat,
			started_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	_, err := scope.Conn.Exec(ctx, query,
		dag.ID, dag.ProjectID, dag.DatasourceID, dag.OntologyID,
		dag.Status, dag.CurrentNode, dag.SchemaFingerprint,
		dag.OwnerID, dag.LastHeartbeat,
		dag.StartedAt, dag.CompletedAt, dag.CreatedAt, dag.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create DAG: %w", err)
	}

	return nil
}

func (r *ontologyDAGRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, ontology_id,
		       status, current_node, schema_fingerprint,
		       owner_id, last_heartbeat,
		       started_at, completed_at, created_at, updated_at
		FROM engine_ontology_dag
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, id)
	return scanDAGRow(row)
}

func (r *ontologyDAGRepository) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	dag, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if dag == nil {
		return nil, nil
	}

	nodes, err := r.GetNodesByDAG(ctx, id)
	if err != nil {
		return nil, err
	}
	dag.Nodes = nodes

	return dag, nil
}

func (r *ontologyDAGRepository) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, ontology_id,
		       status, current_node, schema_fingerprint,
		       owner_id, last_heartbeat,
		       started_at, completed_at, created_at, updated_at
		FROM engine_ontology_dag
		WHERE datasource_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, datasourceID)
	dag, err := scanDAGRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return dag, nil
}

func (r *ontologyDAGRepository) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, ontology_id,
		       status, current_node, schema_fingerprint,
		       owner_id, last_heartbeat,
		       started_at, completed_at, created_at, updated_at
		FROM engine_ontology_dag
		WHERE datasource_id = $1 AND status IN ('pending', 'running')
		ORDER BY created_at DESC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, datasourceID)
	dag, err := scanDAGRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return dag, nil
}

func (r *ontologyDAGRepository) Update(ctx context.Context, dag *models.OntologyDAG) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	dag.UpdatedAt = time.Now()

	query := `
		UPDATE engine_ontology_dag
		SET ontology_id = $2,
		    status = $3,
		    current_node = $4,
		    schema_fingerprint = $5,
		    owner_id = $6,
		    last_heartbeat = $7,
		    started_at = $8,
		    completed_at = $9,
		    updated_at = $10
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query,
		dag.ID, dag.OntologyID, dag.Status, dag.CurrentNode, dag.SchemaFingerprint,
		dag.OwnerID, dag.LastHeartbeat, dag.StartedAt, dag.CompletedAt, dag.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update DAG: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("DAG not found")
	}

	return nil
}

func (r *ontologyDAGRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.DAGStatus, currentNode *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	var completedAt *time.Time
	var startedAt *time.Time

	if status == models.DAGStatusRunning {
		now := time.Now()
		startedAt = &now
	}

	if status.IsTerminal() {
		now := time.Now()
		completedAt = &now
	}

	query := `
		UPDATE engine_ontology_dag
		SET status = $2,
		    current_node = $3,
		    started_at = COALESCE($4, started_at),
		    completed_at = COALESCE($5, completed_at),
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, status, currentNode, startedAt, completedAt)
	if err != nil {
		return fmt.Errorf("failed to update DAG status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("DAG not found")
	}

	return nil
}

func (r *ontologyDAGRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_dag WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete DAG: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("DAG not found")
	}

	return nil
}

func (r *ontologyDAGRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_dag WHERE project_id = $1`

	_, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete DAGs: %w", err)
	}

	return nil
}

// ============================================================================
// Ownership Methods
// ============================================================================

func (r *ontologyDAGRepository) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_dag
		SET owner_id = $2,
		    last_heartbeat = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND (owner_id IS NULL OR owner_id = $2)
		RETURNING id`

	var returnedID uuid.UUID
	err := scope.Conn.QueryRow(ctx, query, dagID, ownerID).Scan(&returnedID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to claim ownership: %w", err)
	}

	return true, nil
}

func (r *ontologyDAGRepository) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_dag
		SET last_heartbeat = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND owner_id = $2`

	result, err := scope.Conn.Exec(ctx, query, dagID, ownerID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("DAG not found or not owned by this server")
	}

	return nil
}

func (r *ontologyDAGRepository) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_dag
		SET owner_id = NULL,
		    last_heartbeat = NULL,
		    updated_at = NOW()
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, dagID)
	if err != nil {
		return fmt.Errorf("failed to release ownership: %w", err)
	}

	return nil
}

// ============================================================================
// Node Operations
// ============================================================================

func (r *ontologyDAGRepository) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	if len(nodes) == 0 {
		return nil
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	query := `
		INSERT INTO engine_dag_nodes (
			id, dag_id, node_name, node_order,
			status, progress,
			started_at, completed_at, duration_ms,
			error_message, retry_count,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	for i := range nodes {
		node := &nodes[i]
		node.CreatedAt = now
		node.UpdatedAt = now
		if node.ID == uuid.Nil {
			node.ID = uuid.New()
		}

		var progressJSON []byte
		var err error
		if node.Progress != nil {
			progressJSON, err = json.Marshal(node.Progress)
			if err != nil {
				return fmt.Errorf("failed to marshal progress: %w", err)
			}
		}

		_, err = scope.Conn.Exec(ctx, query,
			node.ID, node.DAGID, node.NodeName, node.NodeOrder,
			node.Status, progressJSON,
			node.StartedAt, node.CompletedAt, node.DurationMs,
			node.ErrorMessage, node.RetryCount,
			node.CreatedAt, node.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to create node %s: %w", node.NodeName, err)
		}
	}

	return nil
}

func (r *ontologyDAGRepository) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, dag_id, node_name, node_order,
		       status, progress,
		       started_at, completed_at, duration_ms,
		       error_message, retry_count,
		       created_at, updated_at
		FROM engine_dag_nodes
		WHERE dag_id = $1
		ORDER BY node_order`

	rows, err := scope.Conn.Query(ctx, query, dagID)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []models.DAGNode
	for rows.Next() {
		node, err := scanNodeRow(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating nodes: %w", err)
	}

	return nodes, nil
}

func (r *ontologyDAGRepository) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMsg *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	var startedAt *time.Time
	var completedAt *time.Time

	if status == models.DAGNodeStatusRunning {
		now := time.Now()
		startedAt = &now
	}

	if status.IsTerminal() {
		now := time.Now()
		completedAt = &now
	}

	query := `
		UPDATE engine_dag_nodes
		SET status = $2,
		    error_message = COALESCE($3, error_message),
		    started_at = COALESCE($4, started_at),
		    completed_at = COALESCE($5, completed_at),
		    duration_ms = CASE
		        WHEN $5 IS NOT NULL AND started_at IS NOT NULL
		        THEN EXTRACT(EPOCH FROM ($5::timestamptz - started_at)) * 1000
		        ELSE duration_ms
		    END,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, nodeID, status, errorMsg, startedAt, completedAt)
	if err != nil {
		return fmt.Errorf("failed to update node status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("node not found")
	}

	return nil
}

func (r *ontologyDAGRepository) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	var progressJSON []byte
	var err error
	if progress != nil {
		progressJSON, err = json.Marshal(progress)
		if err != nil {
			return fmt.Errorf("failed to marshal progress: %w", err)
		}
	}

	query := `
		UPDATE engine_dag_nodes
		SET progress = $2,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, nodeID, progressJSON)
	if err != nil {
		return fmt.Errorf("failed to update node progress: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("node not found")
	}

	return nil
}

func (r *ontologyDAGRepository) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_dag_nodes
		SET retry_count = retry_count + 1,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, nodeID)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("node not found")
	}

	return nil
}

func (r *ontologyDAGRepository) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, dag_id, node_name, node_order,
		       status, progress,
		       started_at, completed_at, duration_ms,
		       error_message, retry_count,
		       created_at, updated_at
		FROM engine_dag_nodes
		WHERE dag_id = $1 AND status = 'pending'
		ORDER BY node_order
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, dagID)
	node, err := scanNodeRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return node, nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanDAGRow(row pgx.Row) (*models.OntologyDAG, error) {
	var dag models.OntologyDAG

	err := row.Scan(
		&dag.ID, &dag.ProjectID, &dag.DatasourceID, &dag.OntologyID,
		&dag.Status, &dag.CurrentNode, &dag.SchemaFingerprint,
		&dag.OwnerID, &dag.LastHeartbeat,
		&dag.StartedAt, &dag.CompletedAt, &dag.CreatedAt, &dag.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan DAG: %w", err)
	}

	return &dag, nil
}

type nodeScanner interface {
	Scan(dest ...any) error
}

func scanNodeRow(row nodeScanner) (*models.DAGNode, error) {
	var node models.DAGNode
	var progressJSON []byte

	err := row.Scan(
		&node.ID, &node.DAGID, &node.NodeName, &node.NodeOrder,
		&node.Status, &progressJSON,
		&node.StartedAt, &node.CompletedAt, &node.DurationMs,
		&node.ErrorMessage, &node.RetryCount,
		&node.CreatedAt, &node.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan node: %w", err)
	}

	if len(progressJSON) > 0 {
		node.Progress = &models.DAGNodeProgress{}
		if err := json.Unmarshal(progressJSON, node.Progress); err != nil {
			return nil, fmt.Errorf("failed to unmarshal progress: %w", err)
		}
	}

	return &node, nil
}

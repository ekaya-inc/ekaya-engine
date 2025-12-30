package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// RelationshipCandidateRepository provides data access for relationship candidates.
type RelationshipCandidateRepository interface {
	Create(ctx context.Context, candidate *models.RelationshipCandidate) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.RelationshipCandidate, error)
	GetByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error)
	GetByWorkflowAndStatus(ctx context.Context, workflowID uuid.UUID, status models.RelationshipCandidateStatus) ([]*models.RelationshipCandidate, error)
	GetRequiredPending(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error)
	Update(ctx context.Context, candidate *models.RelationshipCandidate) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.RelationshipCandidateStatus, userDecision *models.UserDecision) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error
	CountByWorkflowAndStatus(ctx context.Context, workflowID uuid.UUID, status models.RelationshipCandidateStatus) (int, error)
	CountRequiredPending(ctx context.Context, workflowID uuid.UUID) (int, error)
}

type relationshipCandidateRepository struct{}

// NewRelationshipCandidateRepository creates a new RelationshipCandidateRepository.
func NewRelationshipCandidateRepository() RelationshipCandidateRepository {
	return &relationshipCandidateRepository{}
}

var _ RelationshipCandidateRepository = (*relationshipCandidateRepository)(nil)

func (r *relationshipCandidateRepository) Create(ctx context.Context, candidate *models.RelationshipCandidate) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	candidate.CreatedAt = now
	candidate.UpdatedAt = now
	if candidate.ID == uuid.Nil {
		candidate.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_relationship_candidates (
			id, workflow_id, datasource_id, source_column_id, target_column_id,
			detection_method, confidence, llm_reasoning,
			value_match_rate, name_similarity,
			cardinality, join_match_rate, orphan_rate, target_coverage,
			source_row_count, target_row_count, matched_rows, orphan_rows,
			status, is_required, user_decision,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23
		)`

	_, err := scope.Conn.Exec(ctx, query,
		candidate.ID, candidate.WorkflowID, candidate.DatasourceID,
		candidate.SourceColumnID, candidate.TargetColumnID,
		candidate.DetectionMethod, candidate.Confidence, candidate.LLMReasoning,
		candidate.ValueMatchRate, candidate.NameSimilarity,
		candidate.Cardinality, candidate.JoinMatchRate, candidate.OrphanRate, candidate.TargetCoverage,
		candidate.SourceRowCount, candidate.TargetRowCount, candidate.MatchedRows, candidate.OrphanRows,
		candidate.Status, candidate.IsRequired, candidate.UserDecision,
		candidate.CreatedAt, candidate.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create relationship candidate: %w", err)
	}

	return nil
}

func (r *relationshipCandidateRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.RelationshipCandidate, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, workflow_id, datasource_id, source_column_id, target_column_id,
		       detection_method, confidence, llm_reasoning,
		       value_match_rate, name_similarity,
		       cardinality, join_match_rate, orphan_rate, target_coverage,
		       source_row_count, target_row_count, matched_rows, orphan_rows,
		       status, is_required, user_decision,
		       created_at, updated_at
		FROM engine_relationship_candidates
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, id)
	return scanRelationshipCandidateRow(row)
}

func (r *relationshipCandidateRepository) GetByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, workflow_id, datasource_id, source_column_id, target_column_id,
		       detection_method, confidence, llm_reasoning,
		       value_match_rate, name_similarity,
		       cardinality, join_match_rate, orphan_rate, target_coverage,
		       source_row_count, target_row_count, matched_rows, orphan_rows,
		       status, is_required, user_decision,
		       created_at, updated_at
		FROM engine_relationship_candidates
		WHERE workflow_id = $1
		ORDER BY confidence DESC, created_at ASC`

	rows, err := scope.Conn.Query(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to query relationship candidates: %w", err)
	}
	defer rows.Close()

	return scanRelationshipCandidateRows(rows)
}

func (r *relationshipCandidateRepository) GetByWorkflowAndStatus(ctx context.Context, workflowID uuid.UUID, status models.RelationshipCandidateStatus) ([]*models.RelationshipCandidate, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, workflow_id, datasource_id, source_column_id, target_column_id,
		       detection_method, confidence, llm_reasoning,
		       value_match_rate, name_similarity,
		       cardinality, join_match_rate, orphan_rate, target_coverage,
		       source_row_count, target_row_count, matched_rows, orphan_rows,
		       status, is_required, user_decision,
		       created_at, updated_at
		FROM engine_relationship_candidates
		WHERE workflow_id = $1 AND status = $2
		ORDER BY confidence DESC, created_at ASC`

	rows, err := scope.Conn.Query(ctx, query, workflowID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query relationship candidates by status: %w", err)
	}
	defer rows.Close()

	return scanRelationshipCandidateRows(rows)
}

func (r *relationshipCandidateRepository) GetRequiredPending(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, workflow_id, datasource_id, source_column_id, target_column_id,
		       detection_method, confidence, llm_reasoning,
		       value_match_rate, name_similarity,
		       cardinality, join_match_rate, orphan_rate, target_coverage,
		       source_row_count, target_row_count, matched_rows, orphan_rows,
		       status, is_required, user_decision,
		       created_at, updated_at
		FROM engine_relationship_candidates
		WHERE workflow_id = $1 AND is_required = true AND status = 'pending'
		ORDER BY confidence DESC, created_at ASC`

	rows, err := scope.Conn.Query(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to query required pending candidates: %w", err)
	}
	defer rows.Close()

	return scanRelationshipCandidateRows(rows)
}

func (r *relationshipCandidateRepository) Update(ctx context.Context, candidate *models.RelationshipCandidate) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	candidate.UpdatedAt = time.Now()

	query := `
		UPDATE engine_relationship_candidates
		SET detection_method = $2,
		    confidence = $3,
		    llm_reasoning = $4,
		    value_match_rate = $5,
		    name_similarity = $6,
		    cardinality = $7,
		    join_match_rate = $8,
		    orphan_rate = $9,
		    target_coverage = $10,
		    source_row_count = $11,
		    target_row_count = $12,
		    matched_rows = $13,
		    orphan_rows = $14,
		    status = $15,
		    is_required = $16,
		    user_decision = $17,
		    updated_at = $18
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query,
		candidate.ID,
		candidate.DetectionMethod, candidate.Confidence, candidate.LLMReasoning,
		candidate.ValueMatchRate, candidate.NameSimilarity,
		candidate.Cardinality, candidate.JoinMatchRate, candidate.OrphanRate, candidate.TargetCoverage,
		candidate.SourceRowCount, candidate.TargetRowCount, candidate.MatchedRows, candidate.OrphanRows,
		candidate.Status, candidate.IsRequired, candidate.UserDecision,
		candidate.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update relationship candidate: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("relationship candidate not found")
	}

	return nil
}

func (r *relationshipCandidateRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.RelationshipCandidateStatus, userDecision *models.UserDecision) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_relationship_candidates
		SET status = $2,
		    user_decision = $3,
		    updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, status, userDecision)
	if err != nil {
		return fmt.Errorf("failed to update candidate status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("relationship candidate not found")
	}

	return nil
}

func (r *relationshipCandidateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_relationship_candidates WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete relationship candidate: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("relationship candidate not found")
	}

	return nil
}

func (r *relationshipCandidateRepository) DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_relationship_candidates WHERE workflow_id = $1`

	_, err := scope.Conn.Exec(ctx, query, workflowID)
	if err != nil {
		return fmt.Errorf("failed to delete relationship candidates: %w", err)
	}

	return nil
}

func (r *relationshipCandidateRepository) CountByWorkflowAndStatus(ctx context.Context, workflowID uuid.UUID, status models.RelationshipCandidateStatus) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT COUNT(*)
		FROM engine_relationship_candidates
		WHERE workflow_id = $1 AND status = $2`

	var count int
	err := scope.Conn.QueryRow(ctx, query, workflowID, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count candidates by status: %w", err)
	}

	return count, nil
}

func (r *relationshipCandidateRepository) CountRequiredPending(ctx context.Context, workflowID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT COUNT(*)
		FROM engine_relationship_candidates
		WHERE workflow_id = $1 AND is_required = true AND status = 'pending'`

	var count int
	err := scope.Conn.QueryRow(ctx, query, workflowID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count required pending candidates: %w", err)
	}

	return count, nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanRelationshipCandidateRow(row pgx.Row) (*models.RelationshipCandidate, error) {
	var c models.RelationshipCandidate

	err := row.Scan(
		&c.ID, &c.WorkflowID, &c.DatasourceID, &c.SourceColumnID, &c.TargetColumnID,
		&c.DetectionMethod, &c.Confidence, &c.LLMReasoning,
		&c.ValueMatchRate, &c.NameSimilarity,
		&c.Cardinality, &c.JoinMatchRate, &c.OrphanRate, &c.TargetCoverage,
		&c.SourceRowCount, &c.TargetRowCount, &c.MatchedRows, &c.OrphanRows,
		&c.Status, &c.IsRequired, &c.UserDecision,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan relationship candidate: %w", err)
	}

	return &c, nil
}

func scanRelationshipCandidateRows(rows pgx.Rows) ([]*models.RelationshipCandidate, error) {
	var candidates []*models.RelationshipCandidate

	for rows.Next() {
		var c models.RelationshipCandidate

		err := rows.Scan(
			&c.ID, &c.WorkflowID, &c.DatasourceID, &c.SourceColumnID, &c.TargetColumnID,
			&c.DetectionMethod, &c.Confidence, &c.LLMReasoning,
			&c.ValueMatchRate, &c.NameSimilarity,
			&c.Cardinality, &c.JoinMatchRate, &c.OrphanRate, &c.TargetCoverage,
			&c.SourceRowCount, &c.TargetRowCount, &c.MatchedRows, &c.OrphanRows,
			&c.Status, &c.IsRequired, &c.UserDecision,
			&c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relationship candidate row: %w", err)
		}

		candidates = append(candidates, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating relationship candidate rows: %w", err)
	}

	return candidates, nil
}

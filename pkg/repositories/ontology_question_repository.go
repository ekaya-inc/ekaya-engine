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

// QuestionCounts represents the count of required and optional pending questions.
type QuestionCounts struct {
	Required int `json:"required"`
	Optional int `json:"optional"`
}

// QuestionListFilters contains filtering and pagination options for listing questions.
type QuestionListFilters struct {
	Status   *models.QuestionStatus // Filter by status (nil = all)
	Category *string                // Filter by category (nil = all)
	Entity   *string                // Filter by entity in affects (nil = all)
	Priority *int                   // Filter by priority (nil = all)
	Limit    int                    // Max number of results (default 20)
	Offset   int                    // Offset for pagination (default 0)
}

// QuestionListResult contains paginated question results and counts by status.
type QuestionListResult struct {
	Questions      []*models.OntologyQuestion    `json:"questions"`
	TotalCount     int                           `json:"total_count"`
	CountsByStatus map[models.QuestionStatus]int `json:"counts_by_status"`
}

// OntologyQuestionRepository provides data access for ontology questions.
type OntologyQuestionRepository interface {
	// Create inserts a new question.
	Create(ctx context.Context, question *models.OntologyQuestion) error

	// CreateBatch inserts multiple questions in one transaction.
	CreateBatch(ctx context.Context, questions []*models.OntologyQuestion) error

	// GetByID retrieves a question by ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error)

	// ListPending returns all pending questions for a project, ordered by priority.
	ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error)

	// GetNextPending returns the next pending question (highest priority first).
	GetNextPending(ctx context.Context, projectID uuid.UUID) (*models.OntologyQuestion, error)

	// GetPendingCounts returns counts of required and optional pending questions.
	GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*QuestionCounts, error)

	// UpdateStatus updates the status of a question.
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.QuestionStatus) error

	// UpdateStatusWithReason updates the status of a question with a reason.
	UpdateStatusWithReason(ctx context.Context, id uuid.UUID, status models.QuestionStatus, reason string) error

	// SubmitAnswer records an answer for a question.
	SubmitAnswer(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error

	// ListByOntologyID returns all questions for an ontology (for deduplication).
	ListByOntologyID(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyQuestion, error)

	// DeleteByProject deletes all questions for a project.
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error

	// List returns filtered and paginated questions with counts by status.
	List(ctx context.Context, projectID uuid.UUID, filters QuestionListFilters) (*QuestionListResult, error)
}

type ontologyQuestionRepository struct{}

// NewOntologyQuestionRepository creates a new OntologyQuestionRepository.
func NewOntologyQuestionRepository() OntologyQuestionRepository {
	return &ontologyQuestionRepository{}
}

var _ OntologyQuestionRepository = (*ontologyQuestionRepository)(nil)

func (r *ontologyQuestionRepository) Create(ctx context.Context, question *models.OntologyQuestion) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	question.UpdatedAt = now
	if question.ID == uuid.Nil {
		question.ID = uuid.New()
	}
	if question.CreatedAt.IsZero() {
		question.CreatedAt = now
	}
	if question.Status == "" {
		question.Status = models.QuestionStatusPending
	}

	affectsJSON, err := json.Marshal(question.Affects)
	if err != nil {
		return fmt.Errorf("marshal affects: %w", err)
	}

	// Compute content hash for deduplication
	if question.ContentHash == "" {
		question.ContentHash = question.ComputeContentHash()
	}

	query := `
		INSERT INTO engine_ontology_questions (
			id, project_id, ontology_id, content_hash, text, reasoning, category,
			priority, is_required, affects, source_entity_type, source_entity_key,
			status, answer, answered_by, answered_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (ontology_id, content_hash) WHERE content_hash IS NOT NULL DO NOTHING`

	var sourceEntityType, sourceEntityKey *string
	// Extract source entity info from affects if available
	if question.Affects != nil && len(question.Affects.Tables) > 0 {
		t := "table"
		sourceEntityType = &t
		sourceEntityKey = &question.Affects.Tables[0]
	}

	_, err = scope.Conn.Exec(ctx, query,
		question.ID, question.ProjectID, question.OntologyID, question.ContentHash,
		question.Text, nullableString(question.Reasoning), nullableString(question.Category),
		question.Priority, question.IsRequired, affectsJSON,
		sourceEntityType, sourceEntityKey,
		string(question.Status), nullableString(question.Answer),
		question.AnsweredBy, question.AnsweredAt,
		question.CreatedAt, question.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert question: %w", err)
	}

	return nil
}

func (r *ontologyQuestionRepository) CreateBatch(ctx context.Context, questions []*models.OntologyQuestion) error {
	if len(questions) == 0 {
		return nil
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()

	// Build batch insert with ON CONFLICT for deduplication
	batch := &pgx.Batch{}
	query := `
		INSERT INTO engine_ontology_questions (
			id, project_id, ontology_id, content_hash, text, reasoning, category,
			priority, is_required, affects, source_entity_type, source_entity_key,
			status, answer, answered_by, answered_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (ontology_id, content_hash) WHERE content_hash IS NOT NULL DO NOTHING`

	for _, q := range questions {
		if q.ID == uuid.Nil {
			q.ID = uuid.New()
		}
		if q.CreatedAt.IsZero() {
			q.CreatedAt = now
		}
		q.UpdatedAt = now
		if q.Status == "" {
			q.Status = models.QuestionStatusPending
		}
		// Compute content hash for deduplication
		if q.ContentHash == "" {
			q.ContentHash = q.ComputeContentHash()
		}

		affectsJSON, err := json.Marshal(q.Affects)
		if err != nil {
			return fmt.Errorf("marshal affects: %w", err)
		}

		var sourceEntityType, sourceEntityKey *string
		if q.Affects != nil && len(q.Affects.Tables) > 0 {
			t := "table"
			sourceEntityType = &t
			sourceEntityKey = &q.Affects.Tables[0]
		}

		batch.Queue(query,
			q.ID, q.ProjectID, q.OntologyID, q.ContentHash,
			q.Text, nullableString(q.Reasoning), nullableString(q.Category),
			q.Priority, q.IsRequired, affectsJSON,
			sourceEntityType, sourceEntityKey,
			string(q.Status), nullableString(q.Answer),
			q.AnsweredBy, q.AnsweredAt,
			q.CreatedAt, q.UpdatedAt,
		)
	}

	br := scope.Conn.SendBatch(ctx, batch)
	defer br.Close()

	for range questions {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert question: %w", err)
		}
	}

	return nil
}

func (r *ontologyQuestionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, content_hash, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, status_reason, answer, answered_by, answered_at, created_at, updated_at
		FROM engine_ontology_questions
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, id)
	q, err := scanQuestionRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get question by id: %w", err)
	}
	return q, nil
}

func (r *ontologyQuestionRepository) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, content_hash, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, status_reason, answer, answered_by, answered_at, created_at, updated_at
		FROM engine_ontology_questions
		WHERE project_id = $1 AND status = 'pending'
		ORDER BY priority ASC, created_at ASC`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list pending questions: %w", err)
	}
	defer rows.Close()

	questions := make([]*models.OntologyQuestion, 0)
	for rows.Next() {
		q, err := scanQuestionRows(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating questions: %w", err)
	}

	return questions, nil
}

func (r *ontologyQuestionRepository) GetNextPending(ctx context.Context, projectID uuid.UUID) (*models.OntologyQuestion, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Return required questions first (priority 1-2), then by priority, then by creation order
	query := `
		SELECT id, project_id, ontology_id, content_hash, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, status_reason, answer, answered_by, answered_at, created_at, updated_at
		FROM engine_ontology_questions
		WHERE project_id = $1 AND status = 'pending'
		ORDER BY is_required DESC, priority ASC, created_at ASC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, projectID)
	q, err := scanQuestionRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get next pending question: %w", err)
	}
	return q, nil
}

func (r *ontologyQuestionRepository) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*QuestionCounts, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT
			COUNT(*) FILTER (WHERE is_required = true) AS required,
			COUNT(*) FILTER (WHERE is_required = false) AS optional
		FROM engine_ontology_questions
		WHERE project_id = $1 AND status = 'pending'`

	var counts QuestionCounts
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&counts.Required, &counts.Optional)
	if err != nil {
		return nil, fmt.Errorf("get pending counts: %w", err)
	}

	return &counts, nil
}

func (r *ontologyQuestionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.QuestionStatus) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_questions
		SET status = $2, updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, string(status))
	if err != nil {
		return fmt.Errorf("update question status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("question not found: %s", id)
	}

	return nil
}

func (r *ontologyQuestionRepository) UpdateStatusWithReason(ctx context.Context, id uuid.UUID, status models.QuestionStatus, reason string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_questions
		SET status = $2, status_reason = $3, updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, string(status), nullableString(reason))
	if err != nil {
		return fmt.Errorf("update question status with reason: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("question not found: %s", id)
	}

	return nil
}

func (r *ontologyQuestionRepository) SubmitAnswer(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_questions
		SET status = 'answered', answer = $2, answered_by = $3, answered_at = NOW(), updated_at = NOW()
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, answer, answeredBy)
	if err != nil {
		return fmt.Errorf("submit answer: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("question not found: %s", id)
	}

	return nil
}

func (r *ontologyQuestionRepository) ListByOntologyID(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyQuestion, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, content_hash, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, status_reason, answer, answered_by, answered_at, created_at, updated_at
		FROM engine_ontology_questions
		WHERE ontology_id = $1
		ORDER BY created_at ASC`

	rows, err := scope.Conn.Query(ctx, query, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("list questions by ontology: %w", err)
	}
	defer rows.Close()

	questions := make([]*models.OntologyQuestion, 0)
	for rows.Next() {
		q, err := scanQuestionRows(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating questions: %w", err)
	}

	return questions, nil
}

func (r *ontologyQuestionRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_questions WHERE project_id = $1`

	_, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("delete questions by project: %w", err)
	}

	return nil
}

func (r *ontologyQuestionRepository) List(ctx context.Context, projectID uuid.UUID, filters QuestionListFilters) (*QuestionListResult, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Build WHERE clause
	whereClauses := []string{"project_id = $1"}
	args := []interface{}{projectID}
	argIdx := 2

	if filters.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*filters.Status))
		argIdx++
	}

	if filters.Category != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, *filters.Category)
		argIdx++
	}

	if filters.Priority != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *filters.Priority)
		argIdx++
	}

	if filters.Entity != nil {
		// Search for entity name in affects.tables array or as source_entity_key
		whereClauses = append(whereClauses, fmt.Sprintf("(source_entity_key = $%d OR affects::text ILIKE $%d)", argIdx, argIdx+1))
		args = append(args, *filters.Entity)
		args = append(args, fmt.Sprintf("%%\"%s\"%%", *filters.Entity))
		argIdx += 2
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			whereClause += " AND " + whereClauses[i]
		}
	}

	// Get total count with filters
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_ontology_questions %s`, whereClause)
	var totalCount int
	err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("count questions: %w", err)
	}

	// Get counts by status for all questions (not filtered by status)
	statusWhereClause := "WHERE project_id = $1"
	statusArgs := []interface{}{projectID}
	if filters.Category != nil {
		statusWhereClause += " AND category = $2"
		statusArgs = append(statusArgs, *filters.Category)
	}
	if filters.Priority != nil {
		idx := len(statusArgs) + 1
		statusWhereClause += fmt.Sprintf(" AND priority = $%d", idx)
		statusArgs = append(statusArgs, *filters.Priority)
	}
	if filters.Entity != nil {
		idx := len(statusArgs) + 1
		statusWhereClause += fmt.Sprintf(" AND (source_entity_key = $%d OR affects::text ILIKE $%d)", idx, idx+1)
		statusArgs = append(statusArgs, *filters.Entity)
		statusArgs = append(statusArgs, fmt.Sprintf("%%\"%s\"%%", *filters.Entity))
	}

	statusCountQuery := fmt.Sprintf(`
		SELECT status, COUNT(*)
		FROM engine_ontology_questions
		%s
		GROUP BY status`, statusWhereClause)

	statusRows, err := scope.Conn.Query(ctx, statusCountQuery, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}
	defer statusRows.Close()

	countsByStatus := make(map[models.QuestionStatus]int)
	for statusRows.Next() {
		var status string
		var count int
		if err := statusRows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan status count: %w", err)
		}
		countsByStatus[models.QuestionStatus(status)] = count
	}
	if err := statusRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate status counts: %w", err)
	}

	// Apply pagination defaults
	limit := filters.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	// Get paginated questions
	query := fmt.Sprintf(`
		SELECT id, project_id, ontology_id, content_hash, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, status_reason, answer, answered_by, answered_at, created_at, updated_at
		FROM engine_ontology_questions
		%s
		ORDER BY priority ASC, created_at ASC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list questions: %w", err)
	}
	defer rows.Close()

	questions := make([]*models.OntologyQuestion, 0)
	for rows.Next() {
		q, err := scanQuestionRows(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate questions: %w", err)
	}

	return &QuestionListResult{
		Questions:      questions,
		TotalCount:     totalCount,
		CountsByStatus: countsByStatus,
	}, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanQuestionRow(row pgx.Row) (*models.OntologyQuestion, error) {
	var q models.OntologyQuestion
	var contentHash, reasoning, category, sourceEntityType, sourceEntityKey, statusReason, answer *string
	var status string
	var affectsJSON []byte

	err := row.Scan(
		&q.ID, &q.ProjectID, &q.OntologyID, &contentHash, &q.Text, &reasoning, &category,
		&q.Priority, &q.IsRequired, &affectsJSON, &sourceEntityType, &sourceEntityKey,
		&status, &statusReason, &answer, &q.AnsweredBy, &q.AnsweredAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if contentHash != nil {
		q.ContentHash = *contentHash
	}
	if reasoning != nil {
		q.Reasoning = *reasoning
	}
	if category != nil {
		q.Category = *category
	}
	if statusReason != nil {
		q.StatusReason = *statusReason
	}
	if answer != nil {
		q.Answer = *answer
	}
	q.Status = models.QuestionStatus(status)

	if len(affectsJSON) > 0 {
		var affects models.QuestionAffects
		if err := json.Unmarshal(affectsJSON, &affects); err == nil {
			q.Affects = &affects
		}
	}

	return &q, nil
}

func scanQuestionRows(rows pgx.Rows) (*models.OntologyQuestion, error) {
	var q models.OntologyQuestion
	var contentHash, reasoning, category, sourceEntityType, sourceEntityKey, statusReason, answer *string
	var status string
	var affectsJSON []byte

	err := rows.Scan(
		&q.ID, &q.ProjectID, &q.OntologyID, &contentHash, &q.Text, &reasoning, &category,
		&q.Priority, &q.IsRequired, &affectsJSON, &sourceEntityType, &sourceEntityKey,
		&status, &statusReason, &answer, &q.AnsweredBy, &q.AnsweredAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan question: %w", err)
	}

	if contentHash != nil {
		q.ContentHash = *contentHash
	}
	if reasoning != nil {
		q.Reasoning = *reasoning
	}
	if category != nil {
		q.Category = *category
	}
	if statusReason != nil {
		q.StatusReason = *statusReason
	}
	if answer != nil {
		q.Answer = *answer
	}
	q.Status = models.QuestionStatus(status)

	if len(affectsJSON) > 0 {
		var affects models.QuestionAffects
		if err := json.Unmarshal(affectsJSON, &affects); err == nil {
			q.Affects = &affects
		}
	}

	return &q, nil
}

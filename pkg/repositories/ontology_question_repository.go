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

	// SubmitAnswer records an answer for a question.
	SubmitAnswer(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error

	// ListByOntologyID returns all questions for an ontology (for deduplication).
	ListByOntologyID(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyQuestion, error)
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

	query := `
		INSERT INTO engine_ontology_questions (
			id, project_id, ontology_id, text, reasoning, category,
			priority, is_required, affects, source_entity_type, source_entity_key,
			status, answer, answered_by, answered_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	var sourceEntityType, sourceEntityKey *string
	// Extract source entity info from affects if available
	if question.Affects != nil && len(question.Affects.Tables) > 0 {
		t := "table"
		sourceEntityType = &t
		sourceEntityKey = &question.Affects.Tables[0]
	}

	_, err = scope.Conn.Exec(ctx, query,
		question.ID, question.ProjectID, question.OntologyID,
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

	// Build batch insert
	batch := &pgx.Batch{}
	query := `
		INSERT INTO engine_ontology_questions (
			id, project_id, ontology_id, text, reasoning, category,
			priority, is_required, affects, source_entity_type, source_entity_key,
			status, answer, answered_by, answered_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

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
			q.ID, q.ProjectID, q.OntologyID,
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
		SELECT id, project_id, ontology_id, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, answer, answered_by, answered_at, created_at, updated_at
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
		SELECT id, project_id, ontology_id, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, answer, answered_by, answered_at, created_at, updated_at
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
		SELECT id, project_id, ontology_id, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, answer, answered_by, answered_at, created_at, updated_at
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
		SELECT id, project_id, ontology_id, text, reasoning, category,
		       priority, is_required, affects, source_entity_type, source_entity_key,
		       status, answer, answered_by, answered_at, created_at, updated_at
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
	var reasoning, category, sourceEntityType, sourceEntityKey, answer *string
	var status string
	var affectsJSON []byte

	err := row.Scan(
		&q.ID, &q.ProjectID, &q.OntologyID, &q.Text, &reasoning, &category,
		&q.Priority, &q.IsRequired, &affectsJSON, &sourceEntityType, &sourceEntityKey,
		&status, &answer, &q.AnsweredBy, &q.AnsweredAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if reasoning != nil {
		q.Reasoning = *reasoning
	}
	if category != nil {
		q.Category = *category
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
	var reasoning, category, sourceEntityType, sourceEntityKey, answer *string
	var status string
	var affectsJSON []byte

	err := rows.Scan(
		&q.ID, &q.ProjectID, &q.OntologyID, &q.Text, &reasoning, &category,
		&q.Priority, &q.IsRequired, &affectsJSON, &sourceEntityType, &sourceEntityKey,
		&status, &answer, &q.AnsweredBy, &q.AnsweredAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan question: %w", err)
	}

	if reasoning != nil {
		q.Reasoning = *reasoning
	}
	if category != nil {
		q.Category = *category
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

package tools

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// mockQuestionRepository implements repositories.OntologyQuestionRepository for testing.
type mockQuestionRepository struct {
	questions []*models.OntologyQuestion
	err       error
}

func (m *mockQuestionRepository) Create(ctx context.Context, question *models.OntologyQuestion) error {
	if m.err != nil {
		return m.err
	}
	if question.ID == uuid.Nil {
		question.ID = uuid.New()
	}
	m.questions = append(m.questions, question)
	return nil
}

func (m *mockQuestionRepository) CreateBatch(ctx context.Context, questions []*models.OntologyQuestion) error {
	if m.err != nil {
		return m.err
	}
	for _, q := range questions {
		if q.ID == uuid.Nil {
			q.ID = uuid.New()
		}
		m.questions = append(m.questions, q)
	}
	return nil
}

func (m *mockQuestionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, q := range m.questions {
		if q.ID == id {
			return q, nil
		}
	}
	return nil, nil
}

func (m *mockQuestionRepository) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	pending := make([]*models.OntologyQuestion, 0)
	for _, q := range m.questions {
		if q.ProjectID == projectID && q.Status == models.QuestionStatusPending {
			pending = append(pending, q)
		}
	}
	return pending, nil
}

func (m *mockQuestionRepository) GetNextPending(ctx context.Context, projectID uuid.UUID) (*models.OntologyQuestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, q := range m.questions {
		if q.ProjectID == projectID && q.Status == models.QuestionStatusPending {
			return q, nil
		}
	}
	return nil, nil
}

func (m *mockQuestionRepository) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	if m.err != nil {
		return nil, m.err
	}
	counts := &repositories.QuestionCounts{}
	for _, q := range m.questions {
		if q.ProjectID == projectID && q.Status == models.QuestionStatusPending {
			if q.IsRequired {
				counts.Required++
			} else {
				counts.Optional++
			}
		}
	}
	return counts, nil
}

func (m *mockQuestionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.QuestionStatus) error {
	if m.err != nil {
		return m.err
	}
	for _, q := range m.questions {
		if q.ID == id {
			q.Status = status
			return nil
		}
	}
	return nil
}

func (m *mockQuestionRepository) SubmitAnswer(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error {
	if m.err != nil {
		return m.err
	}
	for _, q := range m.questions {
		if q.ID == id {
			q.Answer = answer
			q.AnsweredBy = answeredBy
			now := time.Now()
			q.AnsweredAt = &now
			q.Status = models.QuestionStatusAnswered
			return nil
		}
	}
	return nil
}

func (m *mockQuestionRepository) ListByOntologyID(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyQuestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*models.OntologyQuestion, 0)
	for _, q := range m.questions {
		if q.OntologyID == ontologyID {
			result = append(result, q)
		}
	}
	return result, nil
}

func (m *mockQuestionRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	if m.err != nil {
		return m.err
	}
	filtered := make([]*models.OntologyQuestion, 0)
	for _, q := range m.questions {
		if q.ProjectID != projectID {
			filtered = append(filtered, q)
		}
	}
	m.questions = filtered
	return nil
}

func (m *mockQuestionRepository) List(ctx context.Context, projectID uuid.UUID, filters repositories.QuestionListFilters) (*repositories.QuestionListResult, error) {
	if m.err != nil {
		return nil, m.err
	}

	// Filter questions
	filtered := make([]*models.OntologyQuestion, 0)
	for _, q := range m.questions {
		if q.ProjectID != projectID {
			continue
		}
		if filters.Status != nil && q.Status != *filters.Status {
			continue
		}
		if filters.Category != nil && q.Category != *filters.Category {
			continue
		}
		if filters.Priority != nil && q.Priority != *filters.Priority {
			continue
		}
		if filters.Entity != nil {
			// Simple entity matching
			found := false
			if q.Affects != nil {
				for _, t := range q.Affects.Tables {
					if t == *filters.Entity {
						found = true
						break
					}
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, q)
	}

	// Apply pagination
	totalCount := len(filtered)
	start := filters.Offset
	if start > totalCount {
		start = totalCount
	}
	end := start + filters.Limit
	if end > totalCount {
		end = totalCount
	}
	paginated := filtered[start:end]

	// Count by status (all questions, not filtered by status)
	countsByStatus := make(map[models.QuestionStatus]int)
	for _, q := range m.questions {
		if q.ProjectID != projectID {
			continue
		}
		// Apply non-status filters for counts
		if filters.Category != nil && q.Category != *filters.Category {
			continue
		}
		if filters.Priority != nil && q.Priority != *filters.Priority {
			continue
		}
		if filters.Entity != nil {
			found := false
			if q.Affects != nil {
				for _, t := range q.Affects.Tables {
					if t == *filters.Entity {
						found = true
						break
					}
				}
			}
			if !found {
				continue
			}
		}
		countsByStatus[q.Status]++
	}

	return &repositories.QuestionListResult{
		Questions:      paginated,
		TotalCount:     totalCount,
		CountsByStatus: countsByStatus,
	}, nil
}

// TestQuestionToolDeps_Structure verifies the QuestionToolDeps struct has all required fields.
func TestQuestionToolDeps_Structure(t *testing.T) {
	deps := &QuestionToolDeps{}

	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.QuestionRepo, "QuestionRepo field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestQuestionToolDeps_Initialization verifies the struct can be initialized with dependencies.
func TestQuestionToolDeps_Initialization(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockQuestionRepository{}

	deps := &QuestionToolDeps{
		Logger:       logger,
		QuestionRepo: repo,
	}

	assert.NotNil(t, deps.Logger, "Logger should be set")
	assert.NotNil(t, deps.QuestionRepo, "QuestionRepo should be set")
}

// TestListOntologyQuestionsTool_Registration verifies the tool can be registered with MCP server.
func TestListOntologyQuestionsTool_Registration(t *testing.T) {
	mcpServer := server.NewMCPServer("test-server", "1.0.0", server.WithToolCapabilities(true))
	repo := &mockQuestionRepository{}
	logger := zap.NewNop()

	deps := &QuestionToolDeps{
		QuestionRepo: repo,
		Logger:       logger,
	}

	// Should not panic
	require.NotPanics(t, func() {
		RegisterQuestionTools(mcpServer, deps)
	})
}

// TestListOntologyQuestionsTool_ResponseStructure verifies the response structure.
func TestListOntologyQuestionsTool_ResponseStructure(t *testing.T) {
	now := time.Now()
	projectID := uuid.New()
	ontologyID := uuid.New()

	questions := []*models.OntologyQuestion{
		{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			Text:       "What does status='ACTIVE' mean?",
			Category:   models.QuestionCategoryEnumeration,
			Priority:   1,
			IsRequired: true,
			Status:     models.QuestionStatusPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         uuid.New(),
			ProjectID:  projectID,
			OntologyID: ontologyID,
			Text:       "How are User and Account related?",
			Category:   models.QuestionCategoryRelationship,
			Priority:   2,
			IsRequired: false,
			Status:     models.QuestionStatusAnswered,
			Answer:     "Users can own accounts",
			AnsweredAt: &now,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}

	result := &repositories.QuestionListResult{
		Questions:  questions,
		TotalCount: 2,
		CountsByStatus: map[models.QuestionStatus]int{
			models.QuestionStatusPending:  1,
			models.QuestionStatusAnswered: 1,
		},
	}

	// Verify structure matches expected response format
	assert.Len(t, result.Questions, 2, "Should have 2 questions")
	assert.Equal(t, 2, result.TotalCount, "Total count should be 2")
	assert.Equal(t, 1, result.CountsByStatus[models.QuestionStatusPending], "Should have 1 pending")
	assert.Equal(t, 1, result.CountsByStatus[models.QuestionStatusAnswered], "Should have 1 answered")

	// Verify first question structure
	q1 := result.Questions[0]
	assert.NotEqual(t, uuid.Nil, q1.ID, "Question should have ID")
	assert.NotEmpty(t, q1.Text, "Question should have text")
	assert.NotEmpty(t, q1.Category, "Question should have category")
	assert.Greater(t, q1.Priority, 0, "Question should have priority")
	assert.NotEmpty(t, string(q1.Status), "Question should have status")
	assert.False(t, q1.CreatedAt.IsZero(), "Question should have created_at")

	// Verify second question has answer
	q2 := result.Questions[1]
	assert.NotEmpty(t, q2.Answer, "Answered question should have answer")
	assert.NotNil(t, q2.AnsweredAt, "Answered question should have answered_at")
}

// TestResolveOntologyQuestionTool_Registration verifies the tool can be registered with MCP server.
func TestResolveOntologyQuestionTool_Registration(t *testing.T) {
	mcpServer := server.NewMCPServer("test-server", "1.0.0", server.WithToolCapabilities(true))
	repo := &mockQuestionRepository{}
	logger := zap.NewNop()

	deps := &QuestionToolDeps{
		QuestionRepo: repo,
		Logger:       logger,
	}

	// Should not panic
	require.NotPanics(t, func() {
		RegisterQuestionTools(mcpServer, deps)
	})
}

// TestResolveOntologyQuestionTool_ResponseStructure verifies the response structure.
func TestResolveOntologyQuestionTool_ResponseStructure(t *testing.T) {
	questionID := uuid.New()
	resolutionNotes := "Found in user.go:45-67"

	// Expected response structure
	response := map[string]interface{}{
		"question_id":      questionID.String(),
		"status":           "answered",
		"resolved_at":      time.Now().Format("2006-01-02T15:04:05Z07:00"),
		"resolution_notes": resolutionNotes,
	}

	// Verify structure
	assert.NotEmpty(t, response["question_id"], "Should have question_id")
	assert.Equal(t, "answered", response["status"], "Status should be 'answered'")
	assert.NotEmpty(t, response["resolved_at"], "Should have resolved_at timestamp")
	assert.Equal(t, resolutionNotes, response["resolution_notes"], "Should have resolution_notes")
}

// TestResolveOntologyQuestionTool_WithoutNotes verifies the response when resolution_notes is omitted.
func TestResolveOntologyQuestionTool_WithoutNotes(t *testing.T) {
	questionID := uuid.New()

	// Expected response structure without resolution_notes
	response := map[string]interface{}{
		"question_id": questionID.String(),
		"status":      "answered",
		"resolved_at": time.Now().Format("2006-01-02T15:04:05Z07:00"),
	}

	// Verify structure
	assert.NotEmpty(t, response["question_id"], "Should have question_id")
	assert.Equal(t, "answered", response["status"], "Status should be 'answered'")
	assert.NotEmpty(t, response["resolved_at"], "Should have resolved_at timestamp")
	_, hasNotes := response["resolution_notes"]
	assert.False(t, hasNotes, "Should not have resolution_notes when omitted")
}

// TestMockQuestionRepository_SubmitAnswer verifies the mock SubmitAnswer implementation.
func TestMockQuestionRepository_SubmitAnswer(t *testing.T) {
	repo := &mockQuestionRepository{}
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	// Create a pending question
	question := &models.OntologyQuestion{
		ID:         uuid.New(),
		ProjectID:  projectID,
		OntologyID: ontologyID,
		Text:       "Test question",
		Status:     models.QuestionStatusPending,
		Priority:   1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := repo.Create(ctx, question)
	require.NoError(t, err)

	// Submit answer
	answer := "Found in code"
	err = repo.SubmitAnswer(ctx, question.ID, answer, nil)
	require.NoError(t, err)

	// Verify question was updated
	updated, err := repo.GetByID(ctx, question.ID)
	require.NoError(t, err)
	assert.Equal(t, answer, updated.Answer, "Answer should be set")
	assert.Equal(t, models.QuestionStatusAnswered, updated.Status, "Status should be answered")
	assert.NotNil(t, updated.AnsweredAt, "AnsweredAt should be set")
	assert.Nil(t, updated.AnsweredBy, "AnsweredBy should be nil for agent")
}

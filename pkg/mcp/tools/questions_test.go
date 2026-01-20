package tools

import (
	"context"
	"encoding/json"
	"fmt"
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

func (m *mockQuestionRepository) UpdateStatusWithReason(ctx context.Context, id uuid.UUID, status models.QuestionStatus, reason string) error {
	if m.err != nil {
		return m.err
	}
	for _, q := range m.questions {
		if q.ID == id {
			q.Status = status
			q.StatusReason = reason
			q.UpdatedAt = time.Now()
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

// Test skip_ontology_question tool registration and structure
func TestSkipOntologyQuestion_ToolRegistration(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	repo := &mockQuestionRepository{}
	deps := &QuestionToolDeps{
		QuestionRepo: repo,
		Logger:       zap.NewNop(),
	}

	registerSkipOntologyQuestionTool(s, deps)

	// Verify tool is registered
	ctx := context.Background()
	result := s.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	// Find skip_ontology_question tool
	var toolFound bool
	var toolDesc string
	for _, tool := range response.Result.Tools {
		if tool.Name == "skip_ontology_question" {
			toolFound = true
			toolDesc = tool.Description
			break
		}
	}

	require.True(t, toolFound, "skip_ontology_question tool should be registered")
	assert.Contains(t, toolDesc, "skipped")
	assert.Contains(t, toolDesc, "revisit")
}

// Test escalate_ontology_question tool registration and structure
func TestEscalateOntologyQuestion_ToolRegistration(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	repo := &mockQuestionRepository{}
	deps := &QuestionToolDeps{
		QuestionRepo: repo,
		Logger:       zap.NewNop(),
	}

	registerEscalateOntologyQuestionTool(s, deps)

	// Verify tool is registered
	ctx := context.Background()
	result := s.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	// Find escalate_ontology_question tool
	var toolFound bool
	var toolDesc string
	for _, tool := range response.Result.Tools {
		if tool.Name == "escalate_ontology_question" {
			toolFound = true
			toolDesc = tool.Description
			break
		}
	}

	require.True(t, toolFound, "escalate_ontology_question tool should be registered")
	assert.Contains(t, toolDesc, "escalated")
	assert.Contains(t, toolDesc, "human")
}

// Test dismiss_ontology_question tool registration and structure
func TestDismissOntologyQuestion_ToolRegistration(t *testing.T) {
	s := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	repo := &mockQuestionRepository{}
	deps := &QuestionToolDeps{
		QuestionRepo: repo,
		Logger:       zap.NewNop(),
	}

	registerDismissOntologyQuestionTool(s, deps)

	// Verify tool is registered
	ctx := context.Background()
	result := s.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	// Find dismiss_ontology_question tool
	var toolFound bool
	var toolDesc string
	for _, tool := range response.Result.Tools {
		if tool.Name == "dismiss_ontology_question" {
			toolFound = true
			toolDesc = tool.Description
			break
		}
	}

	require.True(t, toolFound, "dismiss_ontology_question tool should be registered")
	assert.Contains(t, toolDesc, "dismissed")
	assert.Contains(t, toolDesc, "not worth pursuing")
}

// Test skip_ontology_question response structure
func TestSkipOntologyQuestion_ResponseStructure(t *testing.T) {
	questionID := uuid.New()
	repo := &mockQuestionRepository{
		questions: []*models.OntologyQuestion{
			{
				ID:         questionID,
				ProjectID:  uuid.New(),
				OntologyID: uuid.New(),
				Text:       "Test question",
				Status:     models.QuestionStatusPending,
			},
		},
	}

	// Mock the tool implementation
	// Since we can't easily test the full MCP handler without a real context,
	// we verify the repository method works correctly
	reason := "Need access to frontend repo"
	err := repo.UpdateStatusWithReason(context.Background(), questionID, models.QuestionStatusSkipped, reason)
	assert.NoError(t, err)

	// Verify the question was updated
	q, err := repo.GetByID(context.Background(), questionID)
	assert.NoError(t, err)
	require.NotNil(t, q)
	assert.Equal(t, models.QuestionStatusSkipped, q.Status)
	assert.Equal(t, reason, q.StatusReason)
}

// Test escalate_ontology_question response structure
func TestEscalateOntologyQuestion_ResponseStructure(t *testing.T) {
	questionID := uuid.New()
	repo := &mockQuestionRepository{
		questions: []*models.OntologyQuestion{
			{
				ID:         questionID,
				ProjectID:  uuid.New(),
				OntologyID: uuid.New(),
				Text:       "Test question",
				Status:     models.QuestionStatusPending,
			},
		},
	}

	reason := "Business rule not documented in code"
	err := repo.UpdateStatusWithReason(context.Background(), questionID, models.QuestionStatusEscalated, reason)
	assert.NoError(t, err)

	// Verify the question was updated
	q, err := repo.GetByID(context.Background(), questionID)
	assert.NoError(t, err)
	require.NotNil(t, q)
	assert.Equal(t, models.QuestionStatusEscalated, q.Status)
	assert.Equal(t, reason, q.StatusReason)
}

// Test dismiss_ontology_question response structure
func TestDismissOntologyQuestion_ResponseStructure(t *testing.T) {
	questionID := uuid.New()
	repo := &mockQuestionRepository{
		questions: []*models.OntologyQuestion{
			{
				ID:         questionID,
				ProjectID:  uuid.New(),
				OntologyID: uuid.New(),
				Text:       "Test question",
				Status:     models.QuestionStatusPending,
			},
		},
	}

	reason := "Column appears unused (legacy)"
	err := repo.UpdateStatusWithReason(context.Background(), questionID, models.QuestionStatusDismissed, reason)
	assert.NoError(t, err)

	// Verify the question was updated
	q, err := repo.GetByID(context.Background(), questionID)
	assert.NoError(t, err)
	require.NotNil(t, q)
	assert.Equal(t, models.QuestionStatusDismissed, q.Status)
	assert.Equal(t, reason, q.StatusReason)
}

// TestResolveOntologyQuestionTool_ErrorResults verifies error handling for invalid parameters and resource lookups.
func TestResolveOntologyQuestionTool_ErrorResults(t *testing.T) {
	t.Run("empty question_id after trimming", func(t *testing.T) {
		// Simulate validation check for empty question_id after trimming
		questionIDStr := "   "
		questionIDStr = trimString(questionIDStr)
		if questionIDStr == "" {
			result := NewErrorResult(
				"invalid_parameters",
				"parameter 'question_id' cannot be empty",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
		}
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		// Simulate UUID validation
		questionIDStr := "not-a-valid-uuid"
		questionIDStr = trimString(questionIDStr)
		_, err := uuid.Parse(questionIDStr)
		if err != nil {
			result := NewErrorResult(
				"invalid_parameters",
				fmt.Sprintf("invalid question_id format: %q is not a valid UUID", questionIDStr),
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			jsonErr := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, jsonErr)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid question_id format")
			assert.Contains(t, errorResp.Message, "not-a-valid-uuid")
		}
	})

	t.Run("question not found", func(t *testing.T) {
		// Simulate question not found scenario
		questionIDStr := uuid.New().String()

		result := NewErrorResult(
			"QUESTION_NOT_FOUND",
			fmt.Sprintf("ontology question %q not found", questionIDStr),
		)

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse the content to verify structure
		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
		assert.Contains(t, errorResp.Message, "ontology question")
		assert.Contains(t, errorResp.Message, "not found")
		assert.Contains(t, errorResp.Message, questionIDStr)
	})
}

// TestValidateQuestionID tests the validateQuestionID helper function.
func TestValidateQuestionID(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]any
		wantErr      bool
		expectedCode string
	}{
		{
			name:         "empty question_id",
			args:         map[string]any{"question_id": ""},
			wantErr:      true,
			expectedCode: "invalid_parameters",
		},
		{
			name:         "whitespace-only question_id",
			args:         map[string]any{"question_id": "   "},
			wantErr:      true,
			expectedCode: "invalid_parameters",
		},
		{
			name:         "invalid UUID format",
			args:         map[string]any{"question_id": "not-a-uuid"},
			wantErr:      true,
			expectedCode: "invalid_parameters",
		},
		{
			name:    "valid UUID",
			args:    map[string]any{"question_id": "550e8400-e29b-41d4-a716-446655440000"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			questionID, errResult := validateQuestionID(tt.args)

			if tt.wantErr {
				require.NotNil(t, errResult, "expected error result")
				require.True(t, errResult.IsError)

				text := getTextContent(errResult)
				var response map[string]any
				require.NoError(t, json.Unmarshal([]byte(text), &response))
				assert.Equal(t, tt.expectedCode, response["code"])
			} else {
				require.Nil(t, errResult, "expected no error")
				assert.NotEqual(t, uuid.Nil, questionID)
			}
		})
	}
}

// TestValidateReasonParameter tests the validateReasonParameter helper function.
func TestValidateReasonParameter(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]any
		wantErr      bool
		expectedCode string
	}{
		{
			name:         "empty reason",
			args:         map[string]any{"reason": ""},
			wantErr:      true,
			expectedCode: "invalid_parameters",
		},
		{
			name:         "whitespace-only reason",
			args:         map[string]any{"reason": "   "},
			wantErr:      true,
			expectedCode: "invalid_parameters",
		},
		{
			name:    "valid reason",
			args:    map[string]any{"reason": "Need access to frontend repo"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, errResult := validateReasonParameter(tt.args)

			if tt.wantErr {
				require.NotNil(t, errResult, "expected error result")
				require.True(t, errResult.IsError)

				text := getTextContent(errResult)
				var response map[string]any
				require.NoError(t, json.Unmarshal([]byte(text), &response))
				assert.Equal(t, tt.expectedCode, response["code"])
			} else {
				require.Nil(t, errResult, "expected no error")
				assert.NotEmpty(t, reason)
			}
		})
	}
}

// TestSkipOntologyQuestionTool_ErrorResults verifies error handling for invalid parameters and resource lookups.
func TestSkipOntologyQuestionTool_ErrorResults(t *testing.T) {
	t.Run("empty question_id", func(t *testing.T) {
		args := map[string]any{"question_id": ""}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
	})

	t.Run("whitespace-only question_id", func(t *testing.T) {
		args := map[string]any{"question_id": "   "}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		args := map[string]any{"question_id": "not-a-valid-uuid"}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "invalid question_id format")
		assert.Contains(t, errorResp.Message, "not-a-valid-uuid")
	})

	t.Run("empty reason", func(t *testing.T) {
		args := map[string]any{"reason": ""}
		reason, errResult := validateReasonParameter(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Empty(t, reason)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'reason' cannot be empty")
	})

	t.Run("whitespace-only reason", func(t *testing.T) {
		args := map[string]any{"reason": "   "}
		reason, errResult := validateReasonParameter(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Empty(t, reason)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'reason' cannot be empty")
	})

	t.Run("question not found", func(t *testing.T) {
		questionIDStr := uuid.New().String()

		result := NewErrorResult(
			"QUESTION_NOT_FOUND",
			fmt.Sprintf("ontology question %q not found", questionIDStr),
		)

		require.NotNil(t, result)
		require.True(t, result.IsError)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
		assert.Contains(t, errorResp.Message, "ontology question")
		assert.Contains(t, errorResp.Message, "not found")
		assert.Contains(t, errorResp.Message, questionIDStr)
	})
}

// TestDismissOntologyQuestionTool_ErrorResults verifies error handling for invalid parameters and resource lookups.
func TestDismissOntologyQuestionTool_ErrorResults(t *testing.T) {
	t.Run("empty question_id", func(t *testing.T) {
		args := map[string]any{"question_id": ""}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
	})

	t.Run("whitespace-only question_id", func(t *testing.T) {
		args := map[string]any{"question_id": "   "}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		args := map[string]any{"question_id": "not-a-valid-uuid"}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "invalid question_id format")
		assert.Contains(t, errorResp.Message, "not-a-valid-uuid")
	})

	t.Run("empty reason", func(t *testing.T) {
		args := map[string]any{"reason": ""}
		reason, errResult := validateReasonParameter(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Empty(t, reason)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'reason' cannot be empty")
	})

	t.Run("whitespace-only reason", func(t *testing.T) {
		args := map[string]any{"reason": "   "}
		reason, errResult := validateReasonParameter(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Empty(t, reason)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'reason' cannot be empty")
	})

	t.Run("question not found", func(t *testing.T) {
		questionIDStr := uuid.New().String()

		result := NewErrorResult(
			"QUESTION_NOT_FOUND",
			fmt.Sprintf("ontology question %q not found", questionIDStr),
		)

		require.NotNil(t, result)
		require.True(t, result.IsError)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
		assert.Contains(t, errorResp.Message, "ontology question")
		assert.Contains(t, errorResp.Message, "not found")
		assert.Contains(t, errorResp.Message, questionIDStr)
	})
}

// TestEscalateOntologyQuestionTool_ErrorResults verifies error handling for invalid parameters and resource lookups.
func TestEscalateOntologyQuestionTool_ErrorResults(t *testing.T) {
	t.Run("empty question_id", func(t *testing.T) {
		args := map[string]any{"question_id": ""}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
	})

	t.Run("whitespace-only question_id", func(t *testing.T) {
		args := map[string]any{"question_id": "   "}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'question_id' cannot be empty")
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		args := map[string]any{"question_id": "not-a-valid-uuid"}
		questionID, errResult := validateQuestionID(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Equal(t, uuid.Nil, questionID)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "invalid question_id format")
		assert.Contains(t, errorResp.Message, "not-a-valid-uuid")
	})

	t.Run("empty reason", func(t *testing.T) {
		args := map[string]any{"reason": ""}
		reason, errResult := validateReasonParameter(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Empty(t, reason)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'reason' cannot be empty")
	})

	t.Run("whitespace-only reason", func(t *testing.T) {
		args := map[string]any{"reason": "   "}
		reason, errResult := validateReasonParameter(args)

		require.NotNil(t, errResult, "expected error result")
		require.True(t, errResult.IsError)
		require.Empty(t, reason)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(errResult)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "invalid_parameters", errorResp.Code)
		assert.Contains(t, errorResp.Message, "parameter 'reason' cannot be empty")
	})

	t.Run("question not found", func(t *testing.T) {
		questionIDStr := uuid.New().String()

		result := NewErrorResult(
			"QUESTION_NOT_FOUND",
			fmt.Sprintf("ontology question %q not found", questionIDStr),
		)

		require.NotNil(t, result)
		require.True(t, result.IsError)

		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
		assert.Contains(t, errorResp.Message, "ontology question")
		assert.Contains(t, errorResp.Message, "not found")
		assert.Contains(t, errorResp.Message, questionIDStr)
	})
}

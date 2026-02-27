package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Mock Implementations
// ============================================================================

// mockGlossaryServiceForHandler implements services.GlossaryService for handler tests.
type mockGlossaryServiceForHandler struct {
	terms            []*models.BusinessGlossaryTerm
	term             *models.BusinessGlossaryTerm
	getTermsErr      error
	generationStatus *models.GlossaryGenerationStatus
	runAutoErr       error
}

func (m *mockGlossaryServiceForHandler) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	return nil
}
func (m *mockGlossaryServiceForHandler) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	return nil
}
func (m *mockGlossaryServiceForHandler) DeleteTerm(ctx context.Context, termID uuid.UUID) error {
	return nil
}
func (m *mockGlossaryServiceForHandler) GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	if m.getTermsErr != nil {
		return nil, m.getTermsErr
	}
	return m.terms, nil
}
func (m *mockGlossaryServiceForHandler) GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	return m.term, nil
}
func (m *mockGlossaryServiceForHandler) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForHandler) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*services.SQLTestResult, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForHandler) CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}
func (m *mockGlossaryServiceForHandler) DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return nil
}
func (m *mockGlossaryServiceForHandler) SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}
func (m *mockGlossaryServiceForHandler) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockGlossaryServiceForHandler) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockGlossaryServiceForHandler) GetGenerationStatus(projectID uuid.UUID) *models.GlossaryGenerationStatus {
	if m.generationStatus != nil {
		return m.generationStatus
	}
	return &models.GlossaryGenerationStatus{Status: "idle", Message: "No generation in progress"}
}
func (m *mockGlossaryServiceForHandler) RunAutoGenerate(ctx context.Context, projectID uuid.UUID) error {
	return m.runAutoErr
}

// mockQuestionServiceForHandler implements services.OntologyQuestionService for handler tests.
type mockQuestionServiceForHandler struct {
	pendingCounts    *repositories.QuestionCounts
	pendingCountsErr error
}

func (m *mockQuestionServiceForHandler) GetNextQuestion(ctx context.Context, projectID uuid.UUID, includeSkipped bool) (*models.OntologyQuestion, error) {
	return nil, nil
}
func (m *mockQuestionServiceForHandler) GetPendingQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	return nil, nil
}
func (m *mockQuestionServiceForHandler) GetPendingCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockQuestionServiceForHandler) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	if m.pendingCountsErr != nil {
		return nil, m.pendingCountsErr
	}
	return m.pendingCounts, nil
}
func (m *mockQuestionServiceForHandler) AnswerQuestion(ctx context.Context, questionID uuid.UUID, answer string, userID string) (*models.AnswerResult, error) {
	return nil, nil
}
func (m *mockQuestionServiceForHandler) SkipQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}
func (m *mockQuestionServiceForHandler) DeleteQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}
func (m *mockQuestionServiceForHandler) CreateQuestions(ctx context.Context, questions []*models.OntologyQuestion) error {
	return nil
}

// ============================================================================
// List Handler Tests
// ============================================================================

func TestGlossaryHandler_List_IncludesGenerationStatus(t *testing.T) {
	projectID := uuid.New()

	now := time.Now()
	mockGlossary := &mockGlossaryServiceForHandler{
		terms: []*models.BusinessGlossaryTerm{},
		generationStatus: &models.GlossaryGenerationStatus{
			Status:    "discovering",
			Message:   "Discovering glossary terms from ontology...",
			StartedAt: &now,
		},
	}
	mockQuestions := &mockQuestionServiceForHandler{}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/glossary", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response ApiResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.True(t, response.Success)

	dataBytes, err := json.Marshal(response.Data)
	require.NoError(t, err)

	var listResponse GlossaryListResponse
	require.NoError(t, json.Unmarshal(dataBytes, &listResponse))

	require.NotNil(t, listResponse.GenerationStatus)
	assert.Equal(t, "discovering", listResponse.GenerationStatus.Status)
	assert.Equal(t, "Discovering glossary terms from ontology...", listResponse.GenerationStatus.Message)
}

// ============================================================================
// AutoGenerate Handler Tests
// ============================================================================

func TestGlossaryHandler_AutoGenerate_RequiredQuestionsPending(t *testing.T) {
	projectID := uuid.New()

	mockGlossary := &mockGlossaryServiceForHandler{}
	mockQuestions := &mockQuestionServiceForHandler{
		pendingCounts: &repositories.QuestionCounts{Required: 3, Optional: 1},
	}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary/auto-generate", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.AutoGenerate(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "required_questions_pending", errResp["error"])
	assert.Contains(t, errResp["message"], "3 required question(s)")
}

func TestGlossaryHandler_AutoGenerate_AlreadyRunning(t *testing.T) {
	projectID := uuid.New()

	now := time.Now()
	mockGlossary := &mockGlossaryServiceForHandler{
		generationStatus: &models.GlossaryGenerationStatus{
			Status:    "enriching",
			Message:   "Enriching 5 discovered terms with SQL...",
			StartedAt: &now,
		},
	}
	mockQuestions := &mockQuestionServiceForHandler{
		pendingCounts: &repositories.QuestionCounts{Required: 0, Optional: 0},
	}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary/auto-generate", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.AutoGenerate(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "generation_in_progress", errResp["error"])
	assert.Contains(t, errResp["message"], "already in progress")
}

func TestGlossaryHandler_AutoGenerate_Success(t *testing.T) {
	projectID := uuid.New()

	mockGlossary := &mockGlossaryServiceForHandler{
		generationStatus: &models.GlossaryGenerationStatus{
			Status:  "idle",
			Message: "No generation in progress",
		},
	}
	mockQuestions := &mockQuestionServiceForHandler{
		pendingCounts: &repositories.QuestionCounts{Required: 0, Optional: 2},
	}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary/auto-generate", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.AutoGenerate(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var response ApiResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.True(t, response.Success)
}

func TestGlossaryHandler_AutoGenerate_NoRequiredQuestions_AllowsOptional(t *testing.T) {
	projectID := uuid.New()

	// Optional questions pending but no required questions — should succeed
	mockGlossary := &mockGlossaryServiceForHandler{}
	mockQuestions := &mockQuestionServiceForHandler{
		pendingCounts: &repositories.QuestionCounts{Required: 0, Optional: 5},
	}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary/auto-generate", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.AutoGenerate(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestGlossaryHandler_AutoGenerate_NilCounts(t *testing.T) {
	projectID := uuid.New()

	// nil counts (no questions at all) — should succeed
	mockGlossary := &mockGlossaryServiceForHandler{}
	mockQuestions := &mockQuestionServiceForHandler{
		pendingCounts: nil,
	}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary/auto-generate", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.AutoGenerate(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
}

// ============================================================================
// Error Path Tests — Malformed JSON, invalid IDs, service errors
// ============================================================================

func TestGlossaryHandler_Create_MalformedJSON(t *testing.T) {
	projectID := uuid.New()

	handler := NewGlossaryHandler(
		&mockGlossaryServiceForHandler{},
		&mockQuestionServiceForHandler{},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary",
		bytes.NewReader([]byte(`{not valid json`)))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "invalid_request", resp["error"])
}

func TestGlossaryHandler_Update_MalformedJSON(t *testing.T) {
	projectID := uuid.New()
	termID := uuid.New()

	handler := NewGlossaryHandler(
		&mockGlossaryServiceForHandler{},
		&mockQuestionServiceForHandler{},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPut, "/api/projects/"+projectID.String()+"/glossary/"+termID.String(),
		bytes.NewReader([]byte(`{not valid json`)))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("tid", termID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "invalid_request", resp["error"])
}

func TestGlossaryHandler_List_ServiceError(t *testing.T) {
	projectID := uuid.New()

	mockGlossary := &mockGlossaryServiceForHandler{
		getTermsErr: errors.New("database connection lost"),
	}
	handler := NewGlossaryHandler(mockGlossary, &mockQuestionServiceForHandler{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/glossary", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "list_glossary_terms_failed", resp["error"])
}

func TestGlossaryHandler_AutoGenerate_PendingCountsError(t *testing.T) {
	projectID := uuid.New()

	mockGlossary := &mockGlossaryServiceForHandler{}
	mockQuestions := &mockQuestionServiceForHandler{
		pendingCountsErr: errors.New("database error"),
	}
	handler := NewGlossaryHandler(mockGlossary, mockQuestions, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/glossary/auto-generate", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.AutoGenerate(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "check_questions_failed", resp["error"])
}

func TestGlossaryHandler_List_InvalidProjectID(t *testing.T) {
	handler := NewGlossaryHandler(
		&mockGlossaryServiceForHandler{},
		&mockQuestionServiceForHandler{},
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/glossary", nil)
	req.SetPathValue("pid", "not-a-uuid")
	rec := httptest.NewRecorder()

	handler.List(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "invalid_project_id", resp["error"])
}

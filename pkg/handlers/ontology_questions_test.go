package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// mockQuestionService implements services.OntologyQuestionService for testing.
type mockQuestionService struct {
	pendingCounts    *repositories.QuestionCounts
	pendingCountsErr error
}

func (m *mockQuestionService) GetNextQuestion(_ context.Context, _ uuid.UUID, _ bool) (*models.OntologyQuestion, error) {
	return nil, nil
}

func (m *mockQuestionService) GetPendingQuestions(_ context.Context, _ uuid.UUID) ([]*models.OntologyQuestion, error) {
	return nil, nil
}

func (m *mockQuestionService) GetPendingCount(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockQuestionService) GetPendingCounts(_ context.Context, _ uuid.UUID) (*repositories.QuestionCounts, error) {
	return m.pendingCounts, m.pendingCountsErr
}

func (m *mockQuestionService) AnswerQuestion(_ context.Context, _ uuid.UUID, _ string, _ string) (*models.AnswerResult, error) {
	return nil, nil
}

func (m *mockQuestionService) SkipQuestion(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockQuestionService) DeleteQuestion(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockQuestionService) CreateQuestions(_ context.Context, _ []*models.OntologyQuestion) error {
	return nil
}

func TestCounts_Success(t *testing.T) {
	svc := &mockQuestionService{
		pendingCounts: &repositories.QuestionCounts{Required: 3, Optional: 5},
	}
	handler := NewOntologyQuestionsHandler(svc, zap.NewNop())

	projectID := uuid.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{pid}/ontology/questions/counts", handler.Counts)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/projects/%s/ontology/questions/counts", projectID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success=true, got false")
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}

	if int(data["required"].(float64)) != 3 {
		t.Errorf("expected required=3, got %v", data["required"])
	}
	if int(data["optional"].(float64)) != 5 {
		t.Errorf("expected optional=5, got %v", data["optional"])
	}
}

func TestCounts_ZeroCounts(t *testing.T) {
	svc := &mockQuestionService{
		pendingCounts: &repositories.QuestionCounts{Required: 0, Optional: 0},
	}
	handler := NewOntologyQuestionsHandler(svc, zap.NewNop())

	projectID := uuid.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{pid}/ontology/questions/counts", handler.Counts)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/projects/%s/ontology/questions/counts", projectID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ApiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data := resp.Data.(map[string]any)
	if int(data["required"].(float64)) != 0 {
		t.Errorf("expected required=0, got %v", data["required"])
	}
	if int(data["optional"].(float64)) != 0 {
		t.Errorf("expected optional=0, got %v", data["optional"])
	}
}

func TestCounts_NilCounts(t *testing.T) {
	svc := &mockQuestionService{
		pendingCounts: nil,
	}
	handler := NewOntologyQuestionsHandler(svc, zap.NewNop())

	projectID := uuid.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{pid}/ontology/questions/counts", handler.Counts)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/projects/%s/ontology/questions/counts", projectID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ApiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data := resp.Data.(map[string]any)
	if int(data["required"].(float64)) != 0 {
		t.Errorf("expected required=0 when nil, got %v", data["required"])
	}
	if int(data["optional"].(float64)) != 0 {
		t.Errorf("expected optional=0 when nil, got %v", data["optional"])
	}
}

func TestCounts_ServiceError(t *testing.T) {
	svc := &mockQuestionService{
		pendingCountsErr: fmt.Errorf("database connection failed"),
	}
	handler := NewOntologyQuestionsHandler(svc, zap.NewNop())

	projectID := uuid.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{pid}/ontology/questions/counts", handler.Counts)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/projects/%s/ontology/questions/counts", projectID), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCounts_InvalidProjectID(t *testing.T) {
	svc := &mockQuestionService{}
	handler := NewOntologyQuestionsHandler(svc, zap.NewNop())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{pid}/ontology/questions/counts", handler.Counts)

	req := httptest.NewRequest("GET", "/api/projects/not-a-uuid/ontology/questions/counts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

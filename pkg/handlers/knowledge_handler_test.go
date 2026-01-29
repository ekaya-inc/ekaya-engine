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
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockKnowledgeService is a mock for testing knowledge handler.
type mockKnowledgeService struct {
	facts        []*models.KnowledgeFact
	factsByType  []*models.KnowledgeFact
	storeFact    *models.KnowledgeFact
	storeErr     error
	updateErr    error
	getAllErr    error
	getByTypeErr error
	deleteErr    error
}

func (m *mockKnowledgeService) Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	if m.storeErr != nil {
		return nil, m.storeErr
	}
	if m.storeFact != nil {
		return m.storeFact, nil
	}
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
		Source:    "manual",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (m *mockKnowledgeService) StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo, source string) (*models.KnowledgeFact, error) {
	if m.storeErr != nil {
		return nil, m.storeErr
	}
	if m.storeFact != nil {
		return m.storeFact, nil
	}
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
		Source:    source,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (m *mockKnowledgeService) Update(ctx context.Context, projectID, id uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return &models.KnowledgeFact{
		ID:        id,
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
		Source:    "manual",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (m *mockKnowledgeService) GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	if m.getAllErr != nil {
		return nil, m.getAllErr
	}
	return m.facts, nil
}

func (m *mockKnowledgeService) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	if m.getByTypeErr != nil {
		return nil, m.getByTypeErr
	}
	return m.factsByType, nil
}

func (m *mockKnowledgeService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteErr
}

// mockKnowledgeParsingService is a mock for testing knowledge parsing.
type mockKnowledgeParsingService struct {
	facts    []*models.KnowledgeFact
	parseErr error
}

func (m *mockKnowledgeParsingService) ParseAndStore(ctx context.Context, projectID uuid.UUID, freeFormText string) ([]*models.KnowledgeFact, error) {
	if m.parseErr != nil {
		return nil, m.parseErr
	}
	return m.facts, nil
}

func TestKnowledgeHandler_List(t *testing.T) {
	projectID := uuid.New()

	t.Run("returns empty list when no facts", func(t *testing.T) {
		mockService := &mockKnowledgeService{facts: []*models.KnowledgeFact{}}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}

		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			t.Fatalf("failed to marshal data: %v", err)
		}

		var listResponse ProjectKnowledgeListResponse
		if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
			t.Fatalf("failed to unmarshal list response: %v", err)
		}

		if len(listResponse.Facts) != 0 {
			t.Fatalf("expected 0 facts, got %d", len(listResponse.Facts))
		}
		if listResponse.Total != 0 {
			t.Fatalf("expected total 0, got %d", listResponse.Total)
		}
	})

	t.Run("returns facts with correct data", func(t *testing.T) {
		factID := uuid.New()
		facts := []*models.KnowledgeFact{
			{
				ID:        factID,
				ProjectID: projectID,
				FactType:  "business_rule",
				Key:       "timezone_convention",
				Value:     "All timestamps are stored in UTC",
				Context:   "Inferred from analysis",
				Source:    "inference",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		mockService := &mockKnowledgeService{facts: facts}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}

		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			t.Fatalf("failed to marshal data: %v", err)
		}

		var listResponse ProjectKnowledgeListResponse
		if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
			t.Fatalf("failed to unmarshal list response: %v", err)
		}

		if len(listResponse.Facts) != 1 {
			t.Fatalf("expected 1 fact, got %d", len(listResponse.Facts))
		}
		if listResponse.Total != 1 {
			t.Fatalf("expected total 1, got %d", listResponse.Total)
		}

		fact := listResponse.Facts[0]
		if fact.FactType != "business_rule" {
			t.Errorf("expected fact_type=business_rule, got %s", fact.FactType)
		}
		if fact.Key != "timezone_convention" {
			t.Errorf("expected key=timezone_convention, got %s", fact.Key)
		}
	})

	t.Run("returns error on service failure", func(t *testing.T) {
		mockService := &mockKnowledgeService{getAllErr: errors.New("database error")}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("returns error for invalid project ID", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/invalid/project-knowledge", nil)
		req.SetPathValue("pid", "invalid")

		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

func TestKnowledgeHandler_Create(t *testing.T) {
	projectID := uuid.New()

	t.Run("creates fact successfully", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := CreateKnowledgeRequest{
			FactType: "business_rule",
			Key:      "currency_code",
			Value:    "Amounts are in cents (USD)",
			Context:  "User specified",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/project-knowledge", bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Create(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}
	})

	t.Run("returns error for missing fact_type", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := CreateKnowledgeRequest{
			Key:   "currency_code",
			Value: "Amounts are in cents (USD)",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/project-knowledge", bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Create(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("returns error for missing key", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := CreateKnowledgeRequest{
			FactType: "business_rule",
			Value:    "Amounts are in cents (USD)",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/project-knowledge", bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Create(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("returns error for missing value", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := CreateKnowledgeRequest{
			FactType: "business_rule",
			Key:      "currency_code",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/project-knowledge", bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Create(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("returns error on service failure", func(t *testing.T) {
		mockService := &mockKnowledgeService{storeErr: errors.New("database error")}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := CreateKnowledgeRequest{
			FactType: "business_rule",
			Key:      "currency_code",
			Value:    "Amounts are in cents (USD)",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/project-knowledge", bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Create(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})
}

func TestKnowledgeHandler_Update(t *testing.T) {
	projectID := uuid.New()
	factID := uuid.New()

	t.Run("updates fact successfully", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := UpdateKnowledgeRequest{
			FactType: "business_rule",
			Key:      "currency_code",
			Value:    "Amounts are in dollars (USD)",
			Context:  "Updated by user",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPut, "/api/projects/"+projectID.String()+"/project-knowledge/"+factID.String(), bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.SetPathValue("kid", factID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}
	})

	t.Run("returns 404 for non-existent fact", func(t *testing.T) {
		mockService := &mockKnowledgeService{updateErr: apperrors.ErrNotFound}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := UpdateKnowledgeRequest{
			FactType: "business_rule",
			Key:      "currency_code",
			Value:    "Amounts are in dollars (USD)",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPut, "/api/projects/"+projectID.String()+"/project-knowledge/"+factID.String(), bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.SetPathValue("kid", factID.String())
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("returns error for invalid knowledge ID", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		body := UpdateKnowledgeRequest{
			FactType: "business_rule",
			Key:      "currency_code",
			Value:    "Amounts are in dollars (USD)",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPut, "/api/projects/"+projectID.String()+"/project-knowledge/invalid", bytes.NewReader(bodyBytes))
		req.SetPathValue("pid", projectID.String())
		req.SetPathValue("kid", "invalid")
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

func TestKnowledgeHandler_Delete(t *testing.T) {
	projectID := uuid.New()
	factID := uuid.New()

	t.Run("deletes fact successfully", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/project-knowledge/"+factID.String(), nil)
		req.SetPathValue("pid", projectID.String())
		req.SetPathValue("kid", factID.String())

		rec := httptest.NewRecorder()
		handler.Delete(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}
	})

	t.Run("returns 404 for non-existent fact", func(t *testing.T) {
		mockService := &mockKnowledgeService{deleteErr: apperrors.ErrNotFound}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/project-knowledge/"+factID.String(), nil)
		req.SetPathValue("pid", projectID.String())
		req.SetPathValue("kid", factID.String())

		rec := httptest.NewRecorder()
		handler.Delete(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})

	t.Run("returns error for invalid knowledge ID", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/project-knowledge/invalid", nil)
		req.SetPathValue("pid", projectID.String())
		req.SetPathValue("kid", "invalid")

		rec := httptest.NewRecorder()
		handler.Delete(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

func TestKnowledgeHandler_GetOverview(t *testing.T) {
	projectID := uuid.New()

	t.Run("returns null overview when no overview exists", func(t *testing.T) {
		mockService := &mockKnowledgeService{factsByType: []*models.KnowledgeFact{}}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge/overview", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.GetOverview(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}

		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			t.Fatalf("failed to marshal data: %v", err)
		}

		var overviewResponse ProjectOverviewResponse
		if err := json.Unmarshal(dataBytes, &overviewResponse); err != nil {
			t.Fatalf("failed to unmarshal overview response: %v", err)
		}

		if overviewResponse.Overview != nil {
			t.Fatalf("expected overview=nil, got %v", *overviewResponse.Overview)
		}
	})

	t.Run("returns overview when project_overview fact exists", func(t *testing.T) {
		overviewText := "This is our e-commerce platform for B2B wholesale."
		mockService := &mockKnowledgeService{
			factsByType: []*models.KnowledgeFact{
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "overview",
					Key:       "project_overview",
					Value:     overviewText,
					Source:    "manual",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			},
		}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge/overview", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.GetOverview(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.Success {
			t.Fatal("expected success=true")
		}

		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			t.Fatalf("failed to marshal data: %v", err)
		}

		var overviewResponse ProjectOverviewResponse
		if err := json.Unmarshal(dataBytes, &overviewResponse); err != nil {
			t.Fatalf("failed to unmarshal overview response: %v", err)
		}

		if overviewResponse.Overview == nil {
			t.Fatal("expected overview to be non-nil")
		}
		if *overviewResponse.Overview != overviewText {
			t.Fatalf("expected overview=%q, got %q", overviewText, *overviewResponse.Overview)
		}
	})

	t.Run("finds project_overview among multiple facts", func(t *testing.T) {
		overviewText := "This is our e-commerce platform."
		mockService := &mockKnowledgeService{
			factsByType: []*models.KnowledgeFact{
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "overview",
					Key:       "other_key",
					Value:     "some other value",
					Source:    "manual",
				},
				{
					ID:        uuid.New(),
					ProjectID: projectID,
					FactType:  "overview",
					Key:       "project_overview",
					Value:     overviewText,
					Source:    "manual",
				},
			},
		}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge/overview", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.GetOverview(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var response ApiResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		dataBytes, err := json.Marshal(response.Data)
		if err != nil {
			t.Fatalf("failed to marshal data: %v", err)
		}

		var overviewResponse ProjectOverviewResponse
		if err := json.Unmarshal(dataBytes, &overviewResponse); err != nil {
			t.Fatalf("failed to unmarshal overview response: %v", err)
		}

		if overviewResponse.Overview == nil || *overviewResponse.Overview != overviewText {
			t.Fatalf("expected overview=%q, got %v", overviewText, overviewResponse.Overview)
		}
	})

	t.Run("returns error on service failure", func(t *testing.T) {
		mockService := &mockKnowledgeService{getByTypeErr: errors.New("database error")}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/project-knowledge/overview", nil)
		req.SetPathValue("pid", projectID.String())

		rec := httptest.NewRecorder()
		handler.GetOverview(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("returns error for invalid project ID", func(t *testing.T) {
		mockService := &mockKnowledgeService{}
		handler := NewKnowledgeHandler(mockService, nil, zap.NewNop())

		req := httptest.NewRequest(http.MethodGet, "/api/projects/invalid/project-knowledge/overview", nil)
		req.SetPathValue("pid", "invalid")

		rec := httptest.NewRecorder()
		handler.GetOverview(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

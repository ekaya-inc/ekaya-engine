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
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestDatasourcesHandler_List_Success(t *testing.T) {
	projectID := uuid.New()
	dsID := uuid.New()
	now := time.Now()

	service := &mockDatasourceService{
		datasources: []*models.Datasource{
			{
				ID:             dsID,
				ProjectID:      projectID,
				Name:           "mydb",
				DatasourceType: "postgres",
				Config:         map[string]any{"host": "localhost", "password": "secret123"},
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
	}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources", nil)
	req.SetPathValue("pid", projectID.String())

	claims := &auth.Claims{ProjectID: projectID.String()}
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ListDatasourcesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Datasources) != 1 {
		t.Fatalf("expected 1 datasource, got %d", len(resp.Datasources))
	}

	ds := resp.Datasources[0]
	if ds.DatasourceID != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), ds.DatasourceID)
	}
	if ds.Type != "postgres" {
		t.Errorf("expected type 'postgres', got %q", ds.Type)
	}

	// Verify password is masked
	if pw, ok := ds.Config["password"].(string); ok && pw != "********" {
		t.Errorf("expected password masked as '********', got %q", pw)
	}
}

func TestDatasourcesHandler_List_InvalidProjectID(t *testing.T) {
	handler := NewDatasourcesHandler(&mockDatasourceService{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/datasources", nil)
	req.SetPathValue("pid", "not-a-uuid")

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %q", resp["error"])
	}
}

func TestDatasourcesHandler_List_ServiceError(t *testing.T) {
	service := &mockDatasourceService{err: errors.New("database error")}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources", nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

func TestDatasourcesHandler_Create_Success(t *testing.T) {
	projectID := uuid.New()
	dsID := uuid.New()
	now := time.Now()

	service := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             dsID,
			ProjectID:      projectID,
			Name:           "mydb",
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost", "name": "mydb", "password": "secret"},
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	body := CreateDatasourceRequest{
		ProjectID: projectID.String(),
		Type:      "postgres",
		Config:    map[string]any{"host": "localhost", "name": "mydb", "password": "secret"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources", bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")

	claims := &auth.Claims{ProjectID: projectID.String()}
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp DatasourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.DatasourceID != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), resp.DatasourceID)
	}

	// Verify password is masked in response
	if pw, ok := resp.Config["password"].(string); ok && pw != "********" {
		t.Errorf("expected password masked, got %q", pw)
	}
}

func TestDatasourcesHandler_Create_MissingType(t *testing.T) {
	handler := NewDatasourcesHandler(&mockDatasourceService{}, zap.NewNop())

	projectID := uuid.New()
	body := CreateDatasourceRequest{
		Config: map[string]any{"host": "localhost", "name": "mydb"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources", bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "missing_type" {
		t.Errorf("expected error 'missing_type', got %q", resp["error"])
	}
}

func TestDatasourcesHandler_Create_MissingName(t *testing.T) {
	handler := NewDatasourcesHandler(&mockDatasourceService{}, zap.NewNop())

	projectID := uuid.New()
	body := CreateDatasourceRequest{
		Type:   "postgres",
		Config: map[string]any{"host": "localhost"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources", bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "missing_name" {
		t.Errorf("expected error 'missing_name', got %q", resp["error"])
	}
}

func TestDatasourcesHandler_Get_Success(t *testing.T) {
	projectID := uuid.New()
	dsID := uuid.New()
	now := time.Now()

	service := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             dsID,
			ProjectID:      projectID,
			Name:           "mydb",
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost", "password": "secret"},
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+dsID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", dsID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp DatasourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.DatasourceID != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), resp.DatasourceID)
	}

	// Verify password is masked
	if pw, ok := resp.Config["password"].(string); ok && pw != "********" {
		t.Errorf("expected password masked, got %q", pw)
	}
}

func TestDatasourcesHandler_Get_NotFound(t *testing.T) {
	service := &mockDatasourceService{err: apperrors.ErrNotFound}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	dsID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+dsID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", dsID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "not_found" {
		t.Errorf("expected error 'not_found', got %q", resp["error"])
	}
}

func TestDatasourcesHandler_Get_InvalidDatasourceID(t *testing.T) {
	handler := NewDatasourcesHandler(&mockDatasourceService{}, zap.NewNop())

	projectID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/not-a-uuid", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", "not-a-uuid")

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_datasource_id" {
		t.Errorf("expected error 'invalid_datasource_id', got %q", resp["error"])
	}
}

func TestDatasourcesHandler_Update_Success(t *testing.T) {
	service := &mockDatasourceService{}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	dsID := uuid.New()

	body := UpdateDatasourceRequest{
		Type:   "postgres",
		Config: map[string]any{"host": "newhost", "name": "mydb", "password": "newpass"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/projects/"+projectID.String()+"/datasources/"+dsID.String(), bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", dsID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["datasource_id"] != dsID.String() {
		t.Errorf("expected datasource_id %q, got %v", dsID.String(), resp["datasource_id"])
	}
}

func TestDatasourcesHandler_Update_NotFound(t *testing.T) {
	service := &mockDatasourceService{err: apperrors.ErrNotFound}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	dsID := uuid.New()

	body := UpdateDatasourceRequest{
		Type:   "postgres",
		Config: map[string]any{"host": "newhost", "name": "mydb"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/projects/"+projectID.String()+"/datasources/"+dsID.String(), bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", dsID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestDatasourcesHandler_Delete_Success(t *testing.T) {
	service := &mockDatasourceService{}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	dsID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/datasources/"+dsID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", dsID.String())

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp DeleteDatasourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
}

func TestDatasourcesHandler_Delete_NotFound(t *testing.T) {
	service := &mockDatasourceService{err: apperrors.ErrNotFound}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	dsID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/datasources/"+dsID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("id", dsID.String())

	rec := httptest.NewRecorder()
	handler.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestDatasourcesHandler_TestConnection_Success(t *testing.T) {
	service := &mockDatasourceService{}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	body := TestConnectionRequest{
		Type:   "postgres",
		Config: map[string]any{"host": "localhost", "port": 5432, "user": "test", "password": "pass"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/test", bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.TestConnection(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp TestConnectionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
	if resp.Message != "Connection successful" {
		t.Errorf("expected message 'Connection successful', got %q", resp.Message)
	}
}

func TestDatasourcesHandler_TestConnection_Failure(t *testing.T) {
	service := &mockDatasourceService{err: errors.New("connection refused")}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	body := TestConnectionRequest{
		Type:   "postgres",
		Config: map[string]any{"host": "badhost", "port": 5432},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/test", bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.TestConnection(rec, req)

	// Connection test failures still return 200 with success: false
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp TestConnectionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Success {
		t.Error("expected success to be false")
	}
	if resp.Message != "connection refused" {
		t.Errorf("expected error message 'connection refused', got %q", resp.Message)
	}
}

func TestDatasourcesHandler_TestConnection_MissingType(t *testing.T) {
	handler := NewDatasourcesHandler(&mockDatasourceService{}, zap.NewNop())

	projectID := uuid.New()
	body := TestConnectionRequest{
		Config: map[string]any{"host": "localhost"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/test", bytes.NewReader(bodyBytes))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.TestConnection(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "missing_type" {
		t.Errorf("expected error 'missing_type', got %q", resp["error"])
	}
}

func TestMaskSensitiveConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil config",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty config",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "config with password",
			input: map[string]any{
				"host":     "localhost",
				"password": "secret123",
			},
			expected: map[string]any{
				"host":     "localhost",
				"password": "********",
			},
		},
		{
			name: "config without password",
			input: map[string]any{
				"host": "localhost",
				"port": 5432,
			},
			expected: map[string]any{
				"host": "localhost",
				"port": 5432,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskSensitiveConfig(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("key %q: expected %v, got %v", k, v, result[k])
				}
			}
		})
	}
}

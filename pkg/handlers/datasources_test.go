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

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}

	// Extract data from wrapped response
	dataMap, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}
	datasources, ok := dataMap["datasources"].([]any)
	if !ok {
		t.Fatalf("expected datasources to be an array")
	}

	if len(datasources) != 1 {
		t.Fatalf("expected 1 datasource, got %d", len(datasources))
	}

	ds := datasources[0].(map[string]any)
	if ds["datasource_id"] != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), ds["datasource_id"])
	}
	if ds["type"] != "postgres" {
		t.Errorf("expected type 'postgres', got %q", ds["type"])
	}

	// Verify name is returned as explicit field
	if ds["name"] != "mydb" {
		t.Errorf("expected name 'mydb', got %q", ds["name"])
	}

	// Verify password is NOT masked (UI handles display masking)
	config := ds["config"].(map[string]any)
	if pw, ok := config["password"].(string); ok && pw != "secret123" {
		t.Errorf("expected password 'secret123', got %q", pw)
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
			Name:           "My Database",
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost", "database": "mydb", "password": "secret"},
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	body := CreateDatasourceRequest{
		ProjectID: projectID.String(),
		Name:      "My Database",
		Type:      "postgres",
		Config:    map[string]any{"host": "localhost", "database": "mydb", "password": "secret"},
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

	var apiResp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Error("expected success to be true")
	}

	// Extract data from wrapped response
	dataMap, ok := apiResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", apiResp.Data)
	}

	if dataMap["datasource_id"] != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), dataMap["datasource_id"])
	}

	// Verify name is returned as explicit field
	if dataMap["name"] != "My Database" {
		t.Errorf("expected name 'My Database', got %q", dataMap["name"])
	}

	// Verify password is NOT masked (UI handles display masking)
	config, ok := dataMap["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config to be a map")
	}
	if pw, ok := config["password"].(string); ok && pw != "secret" {
		t.Errorf("expected password 'secret', got %q", pw)
	}
}

func TestDatasourcesHandler_Create_MissingType(t *testing.T) {
	handler := NewDatasourcesHandler(&mockDatasourceService{}, zap.NewNop())

	projectID := uuid.New()
	body := CreateDatasourceRequest{
		Name:   "My Database",
		Config: map[string]any{"host": "localhost", "database": "mydb"},
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

	var apiResp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Error("expected success to be true")
	}

	// Extract data from wrapped response
	dataMap, ok := apiResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", apiResp.Data)
	}

	if dataMap["datasource_id"] != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), dataMap["datasource_id"])
	}

	// Verify name is returned as explicit field
	if dataMap["name"] != "mydb" {
		t.Errorf("expected name 'mydb', got %q", dataMap["name"])
	}

	// Verify password is NOT masked (UI handles display masking)
	config, ok := dataMap["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config to be a map")
	}
	if pw, ok := config["password"].(string); ok && pw != "secret" {
		t.Errorf("expected password 'secret', got %q", pw)
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
		Name:   "Updated Database",
		Type:   "postgres",
		Config: map[string]any{"host": "newhost", "database": "mydb", "password": "newpass"},
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

	var apiResp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Error("expected success to be true")
	}

	// Extract data from wrapped response
	dataMap, ok := apiResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", apiResp.Data)
	}

	if dataMap["datasource_id"] != dsID.String() {
		t.Errorf("expected datasource_id %q, got %v", dsID.String(), dataMap["datasource_id"])
	}

	// Verify name is returned
	if dataMap["name"] != "Updated Database" {
		t.Errorf("expected name 'Updated Database', got %q", dataMap["name"])
	}
}

func TestDatasourcesHandler_Update_NotFound(t *testing.T) {
	service := &mockDatasourceService{err: apperrors.ErrNotFound}
	handler := NewDatasourcesHandler(service, zap.NewNop())

	projectID := uuid.New()
	dsID := uuid.New()

	body := UpdateDatasourceRequest{
		Name:   "Updated Database",
		Type:   "postgres",
		Config: map[string]any{"host": "newhost", "database": "mydb"},
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

	var apiResp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Error("expected success to be true")
	}

	// Extract data from wrapped response
	dataMap, ok := apiResp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", apiResp.Data)
	}

	if dataMap["success"] != true {
		t.Error("expected data.success to be true")
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
		Type:     "postgres",
		Host:     "localhost",
		Port:     5432,
		User:     "test",
		Password: "pass",
		Name:     "testdb",
		SSLMode:  "disable",
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
		Type:    "postgres",
		Host:    "badhost",
		Port:    5432,
		User:    "test",
		Name:    "testdb",
		SSLMode: "disable",
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
		Host: "localhost",
		Port: 5432,
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

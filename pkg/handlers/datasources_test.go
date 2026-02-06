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

	var apiResp struct {
		Success bool                   `json:"success"`
		Data    TestConnectionResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Fatal("expected API success to be true")
	}
	if !apiResp.Data.Success {
		t.Error("expected success to be true")
	}
	if apiResp.Data.Message != "Connection successful" {
		t.Errorf("expected message 'Connection successful', got %q", apiResp.Data.Message)
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

	var apiResp struct {
		Success bool                   `json:"success"`
		Data    TestConnectionResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !apiResp.Success {
		t.Fatal("expected API success to be true")
	}
	if apiResp.Data.Success {
		t.Error("expected success to be false")
	}
	if apiResp.Data.Message != "connection refused" {
		t.Errorf("expected error message 'connection refused', got %q", apiResp.Data.Message)
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

// TestDatasourcesHandler_List_WithDecryptionFailure tests that the handler properly returns
// datasources with decryption_failed flag when credentials key mismatch occurs.
func TestDatasourcesHandler_List_WithDecryptionFailure(t *testing.T) {
	projectID := uuid.New()
	dsID := uuid.New()
	now := time.Now()

	// Mock service returns a datasource with decryption failure status
	service := &mockDatasourceService{
		datasourcesWithStatus: []*models.DatasourceWithStatus{
			{
				Datasource: &models.Datasource{
					ID:             dsID,
					ProjectID:      projectID,
					Name:           "mydb",
					DatasourceType: "postgres",
					Config:         nil, // Config is nil when decryption fails
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				DecryptionFailed: true,
				ErrorMessage:     "datasource credentials were encrypted with a different key",
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

	// Verify decryption_failed flag is set
	if decryptionFailed, ok := ds["decryption_failed"].(bool); !ok || !decryptionFailed {
		t.Errorf("expected decryption_failed to be true, got %v", ds["decryption_failed"])
	}

	// Verify error message is present
	if errorMsg, ok := ds["error_message"].(string); !ok || errorMsg == "" {
		t.Errorf("expected error_message to be non-empty, got %q", ds["error_message"])
	}

	// Verify datasource ID is still returned
	if ds["datasource_id"] != dsID.String() {
		t.Errorf("expected datasource_id %q, got %q", dsID.String(), ds["datasource_id"])
	}

	// Verify config is null when decryption fails
	if ds["config"] != nil {
		t.Errorf("expected config to be nil when decryption fails, got %v", ds["config"])
	}
}

// TestDatasourcesHandler_List_PartialDecryptionFailure tests that the handler returns
// both successful and failed datasources when partial decryption failures occur.
func TestDatasourcesHandler_List_PartialDecryptionFailure(t *testing.T) {
	projectID := uuid.New()
	dsID1 := uuid.New()
	dsID2 := uuid.New()
	now := time.Now()

	// Mock service returns one successful and one failed datasource
	service := &mockDatasourceService{
		datasourcesWithStatus: []*models.DatasourceWithStatus{
			{
				Datasource: &models.Datasource{
					ID:             dsID1,
					ProjectID:      projectID,
					Name:           "working-db",
					DatasourceType: "postgres",
					Config:         map[string]any{"host": "localhost", "password": "secret"},
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				DecryptionFailed: false,
			},
			{
				Datasource: &models.Datasource{
					ID:             dsID2,
					ProjectID:      projectID,
					Name:           "broken-db",
					DatasourceType: "postgres",
					Config:         nil,
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				DecryptionFailed: true,
				ErrorMessage:     "datasource credentials were encrypted with a different key",
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

	if len(datasources) != 2 {
		t.Fatalf("expected 2 datasources, got %d", len(datasources))
	}

	// First datasource should be working
	ds1 := datasources[0].(map[string]any)
	if ds1["datasource_id"] != dsID1.String() {
		t.Errorf("expected first datasource_id %q, got %q", dsID1.String(), ds1["datasource_id"])
	}
	if decryptionFailed, ok := ds1["decryption_failed"].(bool); ok && decryptionFailed {
		t.Error("expected first datasource decryption_failed to be false or absent")
	}
	if ds1["config"] == nil {
		t.Error("expected first datasource to have config")
	}

	// Second datasource should have decryption failure
	ds2 := datasources[1].(map[string]any)
	if ds2["datasource_id"] != dsID2.String() {
		t.Errorf("expected second datasource_id %q, got %q", dsID2.String(), ds2["datasource_id"])
	}
	if decryptionFailed, ok := ds2["decryption_failed"].(bool); !ok || !decryptionFailed {
		t.Errorf("expected second datasource decryption_failed to be true, got %v", ds2["decryption_failed"])
	}
	if errorMsg, ok := ds2["error_message"].(string); !ok || errorMsg == "" {
		t.Errorf("expected second datasource error_message to be non-empty, got %q", ds2["error_message"])
	}
	if ds2["config"] != nil {
		t.Errorf("expected second datasource config to be nil, got %v", ds2["config"])
	}
}

// TestDatasourcesHandler_List_AllDecryptionFailures tests the edge case where
// all datasources have decryption failures.
func TestDatasourcesHandler_List_AllDecryptionFailures(t *testing.T) {
	projectID := uuid.New()
	dsID1 := uuid.New()
	dsID2 := uuid.New()
	now := time.Now()

	// Mock service returns all failed datasources
	service := &mockDatasourceService{
		datasourcesWithStatus: []*models.DatasourceWithStatus{
			{
				Datasource: &models.Datasource{
					ID:             dsID1,
					ProjectID:      projectID,
					Name:           "db1",
					DatasourceType: "postgres",
					Config:         nil,
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				DecryptionFailed: true,
				ErrorMessage:     "datasource credentials were encrypted with a different key",
			},
			{
				Datasource: &models.Datasource{
					ID:             dsID2,
					ProjectID:      projectID,
					Name:           "db2",
					DatasourceType: "mssql",
					Config:         nil,
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				DecryptionFailed: true,
				ErrorMessage:     "datasource credentials were encrypted with a different key",
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

	if len(datasources) != 2 {
		t.Fatalf("expected 2 datasources, got %d", len(datasources))
	}

	// Verify all datasources have decryption failures
	for i, dsAny := range datasources {
		ds := dsAny.(map[string]any)
		if decryptionFailed, ok := ds["decryption_failed"].(bool); !ok || !decryptionFailed {
			t.Errorf("expected datasource %d decryption_failed to be true, got %v", i, ds["decryption_failed"])
		}
		if errorMsg, ok := ds["error_message"].(string); !ok || errorMsg == "" {
			t.Errorf("expected datasource %d error_message to be non-empty, got %q", i, ds["error_message"])
		}
		if ds["config"] != nil {
			t.Errorf("expected datasource %d config to be nil, got %v", i, ds["config"])
		}
	}
}

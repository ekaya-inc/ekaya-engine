package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestParseProjectID(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name       string
		pathValue  string
		wantOK     bool
		wantNilID  bool
		wantStatus int
		wantError  string
	}{
		{
			name:      "valid UUID",
			pathValue: "550e8400-e29b-41d4-a716-446655440000",
			wantOK:    true,
			wantNilID: false,
		},
		{
			name:       "invalid UUID",
			pathValue:  "not-a-uuid",
			wantOK:     false,
			wantNilID:  true,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_project_id",
		},
		{
			name:       "empty UUID",
			pathValue:  "",
			wantOK:     false,
			wantNilID:  true,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_project_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue("pid", tt.pathValue)
			rec := httptest.NewRecorder()

			id, ok := ParseProjectID(rec, req, logger)

			if ok != tt.wantOK {
				t.Errorf("ParseProjectID() ok = %v, want %v", ok, tt.wantOK)
			}

			if tt.wantNilID && id != uuid.Nil {
				t.Errorf("ParseProjectID() id = %v, want uuid.Nil", id)
			}

			if !tt.wantOK {
				if rec.Code != tt.wantStatus {
					t.Errorf("ParseProjectID() status = %v, want %v", rec.Code, tt.wantStatus)
				}

				var resp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp["error"] != tt.wantError {
					t.Errorf("ParseProjectID() error = %v, want %v", resp["error"], tt.wantError)
				}
			}
		})
	}
}

func TestParseDatasourceID(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name       string
		pathValue  string
		wantOK     bool
		wantNilID  bool
		wantStatus int
		wantError  string
	}{
		{
			name:      "valid UUID",
			pathValue: "550e8400-e29b-41d4-a716-446655440000",
			wantOK:    true,
			wantNilID: false,
		},
		{
			name:       "invalid UUID",
			pathValue:  "not-a-uuid",
			wantOK:     false,
			wantNilID:  true,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid_datasource_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue("dsid", tt.pathValue)
			rec := httptest.NewRecorder()

			id, ok := ParseDatasourceID(rec, req, logger)

			if ok != tt.wantOK {
				t.Errorf("ParseDatasourceID() ok = %v, want %v", ok, tt.wantOK)
			}

			if tt.wantNilID && id != uuid.Nil {
				t.Errorf("ParseDatasourceID() id = %v, want uuid.Nil", id)
			}

			if !tt.wantOK {
				if rec.Code != tt.wantStatus {
					t.Errorf("ParseDatasourceID() status = %v, want %v", rec.Code, tt.wantStatus)
				}

				var resp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp["error"] != tt.wantError {
					t.Errorf("ParseDatasourceID() error = %v, want %v", resp["error"], tt.wantError)
				}
			}
		})
	}
}

func TestParseEntityID(t *testing.T) {
	logger := zap.NewNop()
	validUUID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("eid", validUUID.String())
	rec := httptest.NewRecorder()

	id, ok := ParseEntityID(rec, req, logger)

	if !ok {
		t.Error("ParseEntityID() ok = false, want true")
	}
	if id != validUUID {
		t.Errorf("ParseEntityID() id = %v, want %v", id, validUUID)
	}
}

func TestParseEntityID_Invalid(t *testing.T) {
	logger := zap.NewNop()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("eid", "invalid")
	rec := httptest.NewRecorder()

	id, ok := ParseEntityID(rec, req, logger)

	if ok {
		t.Error("ParseEntityID() ok = true, want false")
	}
	if id != uuid.Nil {
		t.Errorf("ParseEntityID() id = %v, want uuid.Nil", id)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ParseEntityID() status = %v, want %v", rec.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "invalid_entity_id" {
		t.Errorf("ParseEntityID() error = %v, want invalid_entity_id", resp["error"])
	}
}

func TestParseAliasID(t *testing.T) {
	logger := zap.NewNop()
	validUUID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("aid", validUUID.String())
	rec := httptest.NewRecorder()

	id, ok := ParseAliasID(rec, req, logger)

	if !ok {
		t.Error("ParseAliasID() ok = false, want true")
	}
	if id != validUUID {
		t.Errorf("ParseAliasID() id = %v, want %v", id, validUUID)
	}
}

func TestParseAliasID_Invalid(t *testing.T) {
	logger := zap.NewNop()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("aid", "bad-id")
	rec := httptest.NewRecorder()

	id, ok := ParseAliasID(rec, req, logger)

	if ok {
		t.Error("ParseAliasID() ok = true, want false")
	}
	if id != uuid.Nil {
		t.Errorf("ParseAliasID() id = %v, want uuid.Nil", id)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "invalid_alias_id" {
		t.Errorf("ParseAliasID() error = %v, want invalid_alias_id", resp["error"])
	}
}

func TestParseQuestionID(t *testing.T) {
	logger := zap.NewNop()
	validUUID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("qid", validUUID.String())
	rec := httptest.NewRecorder()

	id, ok := ParseQuestionID(rec, req, logger)

	if !ok {
		t.Error("ParseQuestionID() ok = false, want true")
	}
	if id != validUUID {
		t.Errorf("ParseQuestionID() id = %v, want %v", id, validUUID)
	}
}

func TestParseQuestionID_Invalid(t *testing.T) {
	logger := zap.NewNop()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("qid", "wrong")
	rec := httptest.NewRecorder()

	id, ok := ParseQuestionID(rec, req, logger)

	if ok {
		t.Error("ParseQuestionID() ok = true, want false")
	}
	if id != uuid.Nil {
		t.Errorf("ParseQuestionID() id = %v, want uuid.Nil", id)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "invalid_question_id" {
		t.Errorf("ParseQuestionID() error = %v, want invalid_question_id", resp["error"])
	}
}

func TestParseQueryID(t *testing.T) {
	logger := zap.NewNop()
	validUUID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("qid", validUUID.String())
	rec := httptest.NewRecorder()

	id, ok := ParseQueryID(rec, req, logger)

	if !ok {
		t.Error("ParseQueryID() ok = false, want true")
	}
	if id != validUUID {
		t.Errorf("ParseQueryID() id = %v, want %v", id, validUUID)
	}
}

func TestParseQueryID_Invalid(t *testing.T) {
	logger := zap.NewNop()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("qid", "nope")
	rec := httptest.NewRecorder()

	id, ok := ParseQueryID(rec, req, logger)

	if ok {
		t.Error("ParseQueryID() ok = true, want false")
	}
	if id != uuid.Nil {
		t.Errorf("ParseQueryID() id = %v, want uuid.Nil", id)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "invalid_query_id" {
		t.Errorf("ParseQueryID() error = %v, want invalid_query_id", resp["error"])
	}
}

func TestParseProjectAndDatasourceIDs(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name         string
		projectID    string
		datasourceID string
		wantOK       bool
		wantStatus   int
		wantError    string
	}{
		{
			name:         "both valid",
			projectID:    uuid.New().String(),
			datasourceID: uuid.New().String(),
			wantOK:       true,
		},
		{
			name:         "invalid project ID",
			projectID:    "bad-project",
			datasourceID: uuid.New().String(),
			wantOK:       false,
			wantStatus:   http.StatusBadRequest,
			wantError:    "invalid_project_id",
		},
		{
			name:         "invalid datasource ID",
			projectID:    uuid.New().String(),
			datasourceID: "bad-datasource",
			wantOK:       false,
			wantStatus:   http.StatusBadRequest,
			wantError:    "invalid_datasource_id",
		},
		{
			name:         "both invalid - project checked first",
			projectID:    "bad-project",
			datasourceID: "bad-datasource",
			wantOK:       false,
			wantStatus:   http.StatusBadRequest,
			wantError:    "invalid_project_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue("pid", tt.projectID)
			req.SetPathValue("dsid", tt.datasourceID)
			rec := httptest.NewRecorder()

			projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(rec, req, logger)

			if ok != tt.wantOK {
				t.Errorf("ParseProjectAndDatasourceIDs() ok = %v, want %v", ok, tt.wantOK)
			}

			if tt.wantOK {
				expectedPID, _ := uuid.Parse(tt.projectID)
				expectedDSID, _ := uuid.Parse(tt.datasourceID)

				if projectID != expectedPID {
					t.Errorf("ParseProjectAndDatasourceIDs() projectID = %v, want %v", projectID, expectedPID)
				}
				if datasourceID != expectedDSID {
					t.Errorf("ParseProjectAndDatasourceIDs() datasourceID = %v, want %v", datasourceID, expectedDSID)
				}
			} else {
				if projectID != uuid.Nil {
					t.Errorf("ParseProjectAndDatasourceIDs() projectID = %v, want uuid.Nil", projectID)
				}
				if datasourceID != uuid.Nil {
					t.Errorf("ParseProjectAndDatasourceIDs() datasourceID = %v, want uuid.Nil", datasourceID)
				}

				if rec.Code != tt.wantStatus {
					t.Errorf("ParseProjectAndDatasourceIDs() status = %v, want %v", rec.Code, tt.wantStatus)
				}

				var resp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp["error"] != tt.wantError {
					t.Errorf("ParseProjectAndDatasourceIDs() error = %v, want %v", resp["error"], tt.wantError)
				}
			}
		})
	}
}

func TestParseUUID_PathParamVariations(t *testing.T) {
	logger := zap.NewNop()

	// Test that the internal parseUUID helper correctly uses different path params
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	validUUID := uuid.New()
	req.SetPathValue("custom_param", validUUID.String())
	rec := httptest.NewRecorder()

	id, ok := parseUUID(rec, req, "custom_param", "custom_error", "Custom error message", logger)

	if !ok {
		t.Error("parseUUID() ok = false, want true")
	}
	if id != validUUID {
		t.Errorf("parseUUID() id = %v, want %v", id, validUUID)
	}
}

func TestParseUUID_CustomErrorMessages(t *testing.T) {
	logger := zap.NewNop()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetPathValue("my_id", "not-valid")
	rec := httptest.NewRecorder()

	_, ok := parseUUID(rec, req, "my_id", "my_error_code", "My custom error message", logger)

	if ok {
		t.Error("parseUUID() ok = true, want false")
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "my_error_code" {
		t.Errorf("parseUUID() error = %v, want my_error_code", resp["error"])
	}
	if resp["message"] != "My custom error message" {
		t.Errorf("parseUUID() message = %v, want 'My custom error message'", resp["message"])
	}
}

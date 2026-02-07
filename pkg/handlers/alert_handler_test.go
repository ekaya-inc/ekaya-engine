package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockAlertService implements services.AlertService for handler testing.
type mockAlertService struct {
	alerts     []*models.AuditAlert
	listErr    error
	resolveErr error
}

func (m *mockAlertService) ListAlerts(_ context.Context, projectID uuid.UUID, filters models.AlertFilters) ([]*models.AuditAlert, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var result []*models.AuditAlert
	for _, a := range m.alerts {
		if a.ProjectID != projectID {
			continue
		}
		if filters.Status != "" && a.Status != filters.Status {
			continue
		}
		if filters.Severity != "" && a.Severity != filters.Severity {
			continue
		}
		result = append(result, a)
	}
	return result, len(result), nil
}

func (m *mockAlertService) GetAlertByID(_ context.Context, projectID uuid.UUID, alertID uuid.UUID) (*models.AuditAlert, error) {
	for _, a := range m.alerts {
		if a.ProjectID == projectID && a.ID == alertID {
			return a, nil
		}
	}
	return nil, nil
}

func (m *mockAlertService) CreateAlert(_ context.Context, alert *models.AuditAlert) error {
	alert.ID = uuid.New()
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()
	m.alerts = append(m.alerts, alert)
	return nil
}

func (m *mockAlertService) ResolveAlert(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string, _ string, _ string) error {
	return m.resolveErr
}

func makeAlertRequest(method, path string, body []byte, projectID uuid.UUID) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.SetPathValue("pid", projectID.String())
	return req
}

func makeAlertRequestWithAuth(method, path string, body []byte, projectID uuid.UUID, userID string) *http.Request {
	req := makeAlertRequest(method, path, body, projectID)
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = userID
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestAlertHandler_ListAlerts_Success(t *testing.T) {
	projectID := uuid.New()
	svc := &mockAlertService{
		alerts: []*models.AuditAlert{
			{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityCritical, Title: "Alert 1"},
			{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityWarning, Title: "Alert 2"},
			{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusResolved, Severity: models.AlertSeverityInfo, Title: "Alert 3"},
		},
	}
	handler := NewAlertHandler(svc, zap.NewNop())

	req := makeAlertRequest("GET", fmt.Sprintf("/api/projects/%s/audit/alerts", projectID), nil, projectID)
	rr := httptest.NewRecorder()

	handler.ListAlerts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ApiResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	data := resp.Data.(map[string]any)
	items := data["items"].([]any)
	// Default filter is "open", so should return 2
	assert.Len(t, items, 2)
	assert.Equal(t, float64(2), data["total"])
}

func TestAlertHandler_ListAlerts_FilterBySeverity(t *testing.T) {
	projectID := uuid.New()
	svc := &mockAlertService{
		alerts: []*models.AuditAlert{
			{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityCritical, Title: "Critical alert"},
			{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityWarning, Title: "Warning alert"},
		},
	}
	handler := NewAlertHandler(svc, zap.NewNop())

	req := makeAlertRequest("GET", fmt.Sprintf("/api/projects/%s/audit/alerts?severity=critical", projectID), nil, projectID)
	req.URL.RawQuery = "severity=critical"
	rr := httptest.NewRecorder()

	handler.ListAlerts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ApiResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)

	data := resp.Data.(map[string]any)
	items := data["items"].([]any)
	assert.Len(t, items, 1)
}

func TestAlertHandler_ListAlerts_EmptyResult(t *testing.T) {
	projectID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	req := makeAlertRequest("GET", fmt.Sprintf("/api/projects/%s/audit/alerts", projectID), nil, projectID)
	rr := httptest.NewRecorder()

	handler.ListAlerts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ApiResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	data := resp.Data.(map[string]any)
	items := data["items"].([]any)
	assert.Len(t, items, 0) // should be empty array, not null
}

func TestAlertHandler_ResolveAlert_Success(t *testing.T) {
	projectID := uuid.New()
	alertID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	body, _ := json.Marshal(resolveAlertRequest{Resolution: "resolved", Notes: "Fixed"})
	req := makeAlertRequestWithAuth("POST",
		fmt.Sprintf("/api/projects/%s/audit/alerts/%s/resolve", projectID, alertID),
		body, projectID, uuid.New().String())
	req.SetPathValue("alert_id", alertID.String())
	rr := httptest.NewRecorder()

	handler.ResolveAlert(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp ApiResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}

func TestAlertHandler_ResolveAlert_InvalidResolution(t *testing.T) {
	projectID := uuid.New()
	alertID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	body, _ := json.Marshal(resolveAlertRequest{Resolution: "invalid", Notes: "notes"})
	req := makeAlertRequestWithAuth("POST",
		fmt.Sprintf("/api/projects/%s/audit/alerts/%s/resolve", projectID, alertID),
		body, projectID, uuid.New().String())
	req.SetPathValue("alert_id", alertID.String())
	rr := httptest.NewRecorder()

	handler.ResolveAlert(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAlertHandler_ResolveAlert_InvalidAlertID(t *testing.T) {
	projectID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	body, _ := json.Marshal(resolveAlertRequest{Resolution: "resolved", Notes: "notes"})
	req := makeAlertRequestWithAuth("POST",
		fmt.Sprintf("/api/projects/%s/audit/alerts/not-a-uuid/resolve", projectID),
		body, projectID, uuid.New().String())
	req.SetPathValue("alert_id", "not-a-uuid")
	rr := httptest.NewRecorder()

	handler.ResolveAlert(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAlertHandler_ResolveAlert_InvalidBody(t *testing.T) {
	projectID := uuid.New()
	alertID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	req := makeAlertRequestWithAuth("POST",
		fmt.Sprintf("/api/projects/%s/audit/alerts/%s/resolve", projectID, alertID),
		[]byte("{invalid json"), projectID, uuid.New().String())
	req.SetPathValue("alert_id", alertID.String())
	rr := httptest.NewRecorder()

	handler.ResolveAlert(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAlertHandler_ResolveAlert_NoAuth(t *testing.T) {
	projectID := uuid.New()
	alertID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	body, _ := json.Marshal(resolveAlertRequest{Resolution: "resolved", Notes: "notes"})
	// Use makeAlertRequest (no auth) instead of makeAlertRequestWithAuth
	req := makeAlertRequest("POST",
		fmt.Sprintf("/api/projects/%s/audit/alerts/%s/resolve", projectID, alertID),
		body, projectID)
	req.SetPathValue("alert_id", alertID.String())
	rr := httptest.NewRecorder()

	handler.ResolveAlert(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAlertHandler_ResolveAlert_Dismissed(t *testing.T) {
	projectID := uuid.New()
	alertID := uuid.New()
	svc := &mockAlertService{}
	handler := NewAlertHandler(svc, zap.NewNop())

	body, _ := json.Marshal(resolveAlertRequest{Resolution: "dismissed", Notes: "False positive"})
	req := makeAlertRequestWithAuth("POST",
		fmt.Sprintf("/api/projects/%s/audit/alerts/%s/resolve", projectID, alertID),
		body, projectID, uuid.New().String())
	req.SetPathValue("alert_id", alertID.String())
	rr := httptest.NewRecorder()

	handler.ResolveAlert(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

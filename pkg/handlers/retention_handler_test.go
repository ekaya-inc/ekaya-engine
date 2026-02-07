package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockMCPConfigRepoForRetention implements the methods needed for retention testing.
type mockMCPConfigRepoForRetention struct {
	retentionDays *int
	err           error
}

func (m *mockMCPConfigRepoForRetention) Get(_ context.Context, _ uuid.UUID) (*models.MCPConfig, error) {
	return nil, m.err
}
func (m *mockMCPConfigRepoForRetention) Upsert(_ context.Context, _ *models.MCPConfig) error {
	return m.err
}
func (m *mockMCPConfigRepoForRetention) GetAgentAPIKey(_ context.Context, _ uuid.UUID) (string, error) {
	return "", m.err
}
func (m *mockMCPConfigRepoForRetention) SetAgentAPIKey(_ context.Context, _ uuid.UUID, _ string) error {
	return m.err
}
func (m *mockMCPConfigRepoForRetention) GetAuditRetentionDays(_ context.Context, _ uuid.UUID) (*int, error) {
	return m.retentionDays, m.err
}
func (m *mockMCPConfigRepoForRetention) SetAuditRetentionDays(_ context.Context, _ uuid.UUID, days *int) error {
	m.retentionDays = days
	return m.err
}

func setupRetentionTest(t *testing.T) (*RetentionHandler, *mockMCPConfigRepoForRetention, uuid.UUID) {
	t.Helper()
	repo := &mockMCPConfigRepoForRetention{}
	handler := NewRetentionHandler(repo, zap.NewNop())
	projectID := uuid.New()
	return handler, repo, projectID
}

// makeRetentionRequest creates a request with the project ID path value set.
func makeRetentionRequest(method, path string, body []byte, projectID uuid.UUID) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.SetPathValue("pid", projectID.String())
	return req
}

func TestRetentionHandler_GetRetention_Default(t *testing.T) {
	handler, _, projectID := setupRetentionTest(t)

	req := makeRetentionRequest("GET", fmt.Sprintf("/api/projects/%s/audit/retention", projectID), nil, projectID)
	rr := httptest.NewRecorder()

	handler.GetRetention(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp retentionResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, services.DefaultRetentionDays, resp.RetentionDays)
	assert.True(t, resp.IsDefault)
}

func TestRetentionHandler_GetRetention_Custom(t *testing.T) {
	handler, repo, projectID := setupRetentionTest(t)
	days := 30
	repo.retentionDays = &days

	req := makeRetentionRequest("GET", fmt.Sprintf("/api/projects/%s/audit/retention", projectID), nil, projectID)
	rr := httptest.NewRecorder()

	handler.GetRetention(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp retentionResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, 30, resp.RetentionDays)
	assert.False(t, resp.IsDefault)
}

func TestRetentionHandler_SetRetention_ValidDays(t *testing.T) {
	handler, repo, projectID := setupRetentionTest(t)

	days := 60
	body, _ := json.Marshal(setRetentionRequest{RetentionDays: &days})
	req := makeRetentionRequest("PUT", fmt.Sprintf("/api/projects/%s/audit/retention", projectID), body, projectID)
	rr := httptest.NewRecorder()

	handler.SetRetention(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, repo.retentionDays)
	assert.Equal(t, 60, *repo.retentionDays)

	var resp retentionResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, 60, resp.RetentionDays)
	assert.False(t, resp.IsDefault)
}

func TestRetentionHandler_SetRetention_ResetToDefault(t *testing.T) {
	handler, repo, projectID := setupRetentionTest(t)
	days := 30
	repo.retentionDays = &days

	body, _ := json.Marshal(setRetentionRequest{RetentionDays: nil})
	req := makeRetentionRequest("PUT", fmt.Sprintf("/api/projects/%s/audit/retention", projectID), body, projectID)
	rr := httptest.NewRecorder()

	handler.SetRetention(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Nil(t, repo.retentionDays)

	var resp retentionResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, services.DefaultRetentionDays, resp.RetentionDays)
	assert.True(t, resp.IsDefault)
}

func TestRetentionHandler_SetRetention_InvalidTooLow(t *testing.T) {
	handler, _, projectID := setupRetentionTest(t)

	days := 0
	body, _ := json.Marshal(setRetentionRequest{RetentionDays: &days})
	req := makeRetentionRequest("PUT", fmt.Sprintf("/api/projects/%s/audit/retention", projectID), body, projectID)
	rr := httptest.NewRecorder()

	handler.SetRetention(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRetentionHandler_SetRetention_InvalidTooHigh(t *testing.T) {
	handler, _, projectID := setupRetentionTest(t)

	days := 400
	body, _ := json.Marshal(setRetentionRequest{RetentionDays: &days})
	req := makeRetentionRequest("PUT", fmt.Sprintf("/api/projects/%s/audit/retention", projectID), body, projectID)
	rr := httptest.NewRecorder()

	handler.SetRetention(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

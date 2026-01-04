package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockAgentAPIKeyService is a mock for AgentAPIKeyService.
type mockAgentAPIKeyService struct {
	key         string
	generateErr error
	getErr      error
	regenErr    error
}

func (m *mockAgentAPIKeyService) GenerateKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	if m.generateErr != nil {
		return "", m.generateErr
	}
	if m.key == "" {
		m.key = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	}
	return m.key, nil
}

func (m *mockAgentAPIKeyService) GetKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	return m.key, nil
}

func (m *mockAgentAPIKeyService) RegenerateKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	if m.regenErr != nil {
		return "", m.regenErr
	}
	m.key = "new567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	return m.key, nil
}

func (m *mockAgentAPIKeyService) ValidateKey(ctx context.Context, projectID uuid.UUID, providedKey string) (bool, error) {
	return m.key == providedKey, nil
}

func TestAgentAPIKeyHandler_Get_Masked(t *testing.T) {
	testKey := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	mockService := &mockAgentAPIKeyService{key: testKey}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/"+projectID.String()+"/mcp/agent-key",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ApiResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "****", data["key"])
	assert.True(t, data["masked"].(bool))
}

func TestAgentAPIKeyHandler_Get_Revealed(t *testing.T) {
	testKey := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	mockService := &mockAgentAPIKeyService{key: testKey}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/"+projectID.String()+"/mcp/agent-key?reveal=true",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ApiResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, testKey, data["key"])
	assert.False(t, data["masked"].(bool))
}

func TestAgentAPIKeyHandler_Get_AutoGenerates(t *testing.T) {
	// Mock service starts with no key
	mockService := &mockAgentAPIKeyService{key: ""}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/"+projectID.String()+"/mcp/agent-key?reveal=true",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ApiResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)

	// Key should have been auto-generated
	key, ok := data["key"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, key)
	assert.NotEqual(t, "****", key)
}

func TestAgentAPIKeyHandler_Get_GetKeyError(t *testing.T) {
	mockService := &mockAgentAPIKeyService{
		getErr: errors.New("database error"),
	}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/"+projectID.String()+"/mcp/agent-key",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "internal_error", errResp["error"])
}

func TestAgentAPIKeyHandler_Get_GenerateKeyError(t *testing.T) {
	// No key exists, and generate fails
	mockService := &mockAgentAPIKeyService{
		key:         "",
		generateErr: errors.New("encryption error"),
	}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/"+projectID.String()+"/mcp/agent-key",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestAgentAPIKeyHandler_Get_InvalidProjectID(t *testing.T) {
	mockService := &mockAgentAPIKeyService{}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet,
		"/api/projects/invalid-uuid/mcp/agent-key",
		nil)
	req.SetPathValue("pid", "invalid-uuid")

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAgentAPIKeyHandler_Regenerate_Success(t *testing.T) {
	mockService := &mockAgentAPIKeyService{
		key: "oldkey",
	}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodPost,
		"/api/projects/"+projectID.String()+"/mcp/agent-key/regenerate",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Regenerate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ApiResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)

	// Should return new key (unmasked)
	key, ok := data["key"].(string)
	require.True(t, ok)
	assert.Equal(t, "new567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", key)
}

func TestAgentAPIKeyHandler_Regenerate_Error(t *testing.T) {
	mockService := &mockAgentAPIKeyService{
		regenErr: errors.New("database error"),
	}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodPost,
		"/api/projects/"+projectID.String()+"/mcp/agent-key/regenerate",
		nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Regenerate(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Equal(t, "internal_error", errResp["error"])
}

func TestAgentAPIKeyHandler_Regenerate_InvalidProjectID(t *testing.T) {
	mockService := &mockAgentAPIKeyService{}
	handler := NewAgentAPIKeyHandler(mockService, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost,
		"/api/projects/invalid-uuid/mcp/agent-key/regenerate",
		nil)
	req.SetPathValue("pid", "invalid-uuid")

	rec := httptest.NewRecorder()
	handler.Regenerate(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

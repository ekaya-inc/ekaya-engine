package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

type mockAgentService struct {
	createFn            func(ctx context.Context, projectID uuid.UUID, name string, queryIDs []uuid.UUID) (*models.Agent, string, error)
	listFn              func(ctx context.Context, projectID uuid.UUID) ([]*services.AgentWithQueries, error)
	getFn               func(ctx context.Context, projectID, agentID uuid.UUID) (*services.AgentWithQueries, error)
	getKeyFn            func(ctx context.Context, projectID, agentID uuid.UUID) (string, error)
	updateQueryAccessFn func(ctx context.Context, projectID, agentID uuid.UUID, queryIDs []uuid.UUID) error
	rotateKeyFn         func(ctx context.Context, projectID, agentID uuid.UUID) (string, error)
	deleteFn            func(ctx context.Context, projectID, agentID uuid.UUID) error
	validateKeyFn       func(ctx context.Context, projectID uuid.UUID, key string) (*models.Agent, error)
	hasQueryAccessFn    func(ctx context.Context, agentID, queryID uuid.UUID) (bool, error)
	getQueryAccessFn    func(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error)
}

func (m *mockAgentService) Create(ctx context.Context, projectID uuid.UUID, name string, queryIDs []uuid.UUID) (*models.Agent, string, error) {
	return m.createFn(ctx, projectID, name, queryIDs)
}

func (m *mockAgentService) List(ctx context.Context, projectID uuid.UUID) ([]*services.AgentWithQueries, error) {
	if m.listFn == nil {
		return nil, nil
	}
	return m.listFn(ctx, projectID)
}

func (m *mockAgentService) Get(ctx context.Context, projectID, agentID uuid.UUID) (*services.AgentWithQueries, error) {
	return m.getFn(ctx, projectID, agentID)
}

func (m *mockAgentService) GetKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error) {
	return m.getKeyFn(ctx, projectID, agentID)
}

func (m *mockAgentService) UpdateQueryAccess(ctx context.Context, projectID, agentID uuid.UUID, queryIDs []uuid.UUID) error {
	return m.updateQueryAccessFn(ctx, projectID, agentID, queryIDs)
}

func (m *mockAgentService) RotateKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error) {
	return m.rotateKeyFn(ctx, projectID, agentID)
}

func (m *mockAgentService) Delete(ctx context.Context, projectID, agentID uuid.UUID) error {
	return m.deleteFn(ctx, projectID, agentID)
}

func (m *mockAgentService) ValidateKey(ctx context.Context, projectID uuid.UUID, key string) (*models.Agent, error) {
	if m.validateKeyFn == nil {
		return nil, nil
	}
	return m.validateKeyFn(ctx, projectID, key)
}

func (m *mockAgentService) HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error) {
	if m.hasQueryAccessFn == nil {
		return false, nil
	}
	return m.hasQueryAccessFn(ctx, agentID, queryID)
}

func (m *mockAgentService) GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error) {
	if m.getQueryAccessFn == nil {
		return nil, nil
	}
	return m.getQueryAccessFn(ctx, agentID)
}

func TestAgentHandlerCreateRejectsEmptyQuerySelection(t *testing.T) {
	handler := NewAgentHandler(&mockAgentService{}, zap.NewNop())
	projectID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/agents", strings.NewReader(`{"name":"sales-bot","query_ids":[]}`))
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "query_ids")
}

func TestAgentHandlerCreateReturnsPlaintextKey(t *testing.T) {
	projectID := uuid.New()
	agentID := uuid.New()
	queryIDs := []uuid.UUID{uuid.New()}
	createdAt := time.Now().UTC()

	handler := NewAgentHandler(&mockAgentService{
		createFn: func(ctx context.Context, gotProjectID uuid.UUID, name string, gotQueryIDs []uuid.UUID) (*models.Agent, string, error) {
			assert.Equal(t, projectID, gotProjectID)
			assert.Equal(t, "sales-bot", name)
			assert.ElementsMatch(t, queryIDs, gotQueryIDs)
			return &models.Agent{
				ID:        agentID,
				ProjectID: gotProjectID,
				Name:      name,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}, "generated-api-key", nil
		},
	}, zap.NewNop())

	reqBody := `{"name":"sales-bot","query_ids":["` + queryIDs[0].String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/agents", strings.NewReader(reqBody))
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp ApiResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, agentID.String(), data["id"])
	assert.Equal(t, "sales-bot", data["name"])
	assert.Equal(t, "generated-api-key", data["api_key"])
}

func TestAgentHandlerGetKeySupportsMaskedAndRevealModes(t *testing.T) {
	projectID := uuid.New()
	agentID := uuid.New()
	handler := NewAgentHandler(&mockAgentService{
		getKeyFn: func(ctx context.Context, gotProjectID, gotAgentID uuid.UUID) (string, error) {
			assert.Equal(t, projectID, gotProjectID)
			assert.Equal(t, agentID, gotAgentID)
			return "super-secret-key", nil
		},
	}, zap.NewNop())

	maskedReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/agents/"+agentID.String()+"/key", nil)
	maskedReq.SetPathValue("pid", projectID.String())
	maskedReq.SetPathValue("aid", agentID.String())

	maskedRec := httptest.NewRecorder()
	handler.GetKey(maskedRec, maskedReq)

	require.Equal(t, http.StatusOK, maskedRec.Code)
	assert.Contains(t, maskedRec.Body.String(), `"key":"****"`)
	assert.Contains(t, maskedRec.Body.String(), `"masked":true`)

	revealReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/agents/"+agentID.String()+"/key?reveal=true", nil)
	revealReq.SetPathValue("pid", projectID.String())
	revealReq.SetPathValue("aid", agentID.String())

	revealRec := httptest.NewRecorder()
	handler.GetKey(revealRec, revealReq)

	require.Equal(t, http.StatusOK, revealRec.Code)
	assert.Contains(t, revealRec.Body.String(), `"key":"super-secret-key"`)
	assert.Contains(t, revealRec.Body.String(), `"masked":false`)
}

func TestAgentHandlerUpdateAndDelete(t *testing.T) {
	projectID := uuid.New()
	agentID := uuid.New()
	queryID := uuid.New()

	handler := NewAgentHandler(&mockAgentService{
		updateQueryAccessFn: func(ctx context.Context, gotProjectID, gotAgentID uuid.UUID, queryIDs []uuid.UUID) error {
			assert.Equal(t, projectID, gotProjectID)
			assert.Equal(t, agentID, gotAgentID)
			assert.ElementsMatch(t, []uuid.UUID{queryID}, queryIDs)
			return nil
		},
		getFn: func(ctx context.Context, gotProjectID, gotAgentID uuid.UUID) (*services.AgentWithQueries, error) {
			assert.Equal(t, projectID, gotProjectID)
			assert.Equal(t, agentID, gotAgentID)
			return &services.AgentWithQueries{
				Agent: models.Agent{
					ID:        agentID,
					ProjectID: projectID,
					Name:      "sales-bot",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				},
				QueryIDs: []uuid.UUID{queryID},
			}, nil
		},
		deleteFn: func(ctx context.Context, gotProjectID, gotAgentID uuid.UUID) error {
			assert.Equal(t, projectID, gotProjectID)
			assert.Equal(t, agentID, gotAgentID)
			return nil
		},
	}, zap.NewNop())

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/projects/"+projectID.String()+"/agents/"+agentID.String(), strings.NewReader(`{"query_ids":["`+queryID.String()+`"]}`))
	updateReq.SetPathValue("pid", projectID.String())
	updateReq.SetPathValue("aid", agentID.String())

	updateRec := httptest.NewRecorder()
	handler.Update(updateRec, updateReq)

	require.Equal(t, http.StatusOK, updateRec.Code)
	assert.Contains(t, updateRec.Body.String(), `"name":"sales-bot"`)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/agents/"+agentID.String(), nil)
	deleteReq.SetPathValue("pid", projectID.String())
	deleteReq.SetPathValue("aid", agentID.String())

	deleteRec := httptest.NewRecorder()
	handler.Delete(deleteRec, deleteReq)

	assert.Equal(t, http.StatusNoContent, deleteRec.Code)
}

func TestAgentHandlerRotateKeyHandlesServiceErrors(t *testing.T) {
	projectID := uuid.New()
	agentID := uuid.New()
	handler := NewAgentHandler(&mockAgentService{
		rotateKeyFn: func(ctx context.Context, gotProjectID, gotAgentID uuid.UUID) (string, error) {
			assert.Equal(t, projectID, gotProjectID)
			assert.Equal(t, agentID, gotAgentID)
			return "", errors.New("rotate failed")
		},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/agents/"+agentID.String()+"/rotate-key", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("aid", agentID.String())

	rec := httptest.NewRecorder()
	handler.RotateKey(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "rotate")
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestSchemaHandler_GetSchema_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	columnID := uuid.New()

	service := &mockSchemaService{
		schema: &models.DatasourceSchema{
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			Tables: []*models.DatasourceTable{
				{
					ID:         tableID,
					SchemaName: "public",
					TableName:  "users",
					IsSelected: true,
					RowCount:   100,
					Columns: []*models.DatasourceColumn{
						{
							ID:           columnID,
							ColumnName:   "id",
							DataType:     "uuid",
							IsPrimaryKey: true,
							IsSelected:   true,
						},
					},
				},
			},
		},
	}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	claims := &auth.Claims{ProjectID: projectID.String()}
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetSchema(rec, req)

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

	dataMap, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}

	tables, ok := dataMap["tables"].([]any)
	if !ok {
		t.Fatalf("expected tables to be an array")
	}

	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}

	table := tables[0].(map[string]any)
	if table["table_name"] != "users" {
		t.Errorf("expected table_name 'users', got %q", table["table_name"])
	}
	// Note: business_name is no longer included in the response - table metadata
	// is now in engine_ontology_table_metadata, not engine_schema_tables.
}

func TestSchemaHandler_GetSchema_InvalidProjectID(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{}, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/datasources/abc/schema", nil)
	req.SetPathValue("pid", "not-a-uuid")
	req.SetPathValue("dsid", uuid.New().String())

	rec := httptest.NewRecorder()
	handler.GetSchema(rec, req)

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

func TestSchemaHandler_GetSchema_InvalidDatasourceID(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{}, nil, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/not-a-uuid/schema", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", "not-a-uuid")

	rec := httptest.NewRecorder()
	handler.GetSchema(rec, req)

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

func TestSchemaHandler_GetSelectedSchema_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	service := &mockSchemaService{
		schema: &models.DatasourceSchema{
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			Tables:       []*models.DatasourceTable{},
		},
	}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/selected", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.GetSelectedSchema(rec, req)

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
}

func TestSchemaHandler_GetSchemaPrompt_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	service := &mockSchemaService{
		prompt: "CREATE TABLE users (id uuid PRIMARY KEY);",
	}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/prompt", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.GetSchemaPrompt(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	dataMap := resp.Data.(map[string]any)
	if dataMap["prompt"] != "CREATE TABLE users (id uuid PRIMARY KEY);" {
		t.Errorf("expected prompt text, got %q", dataMap["prompt"])
	}
}

func TestSchemaHandler_RefreshSchema_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	service := &mockSchemaService{
		refreshResult: &models.RefreshResult{
			TablesUpserted:       10,
			TablesDeleted:        2,
			ColumnsUpserted:      50,
			ColumnsDeleted:       5,
			RelationshipsCreated: 8,
			RelationshipsDeleted: 1,
			NewTableNames:        []string{"public.orders", "public.products"},
			RemovedTableNames:    []string{"public.old_table"},
		},
	}
	changeDetectionSvc := &mockSchemaChangeDetectionService{
		changes: []*models.PendingChange{
			{ID: uuid.New()},
			{ID: uuid.New()},
			{ID: uuid.New()},
		},
	}
	handler := NewSchemaHandler(service, changeDetectionSvc, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/refresh", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.RefreshSchema(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	dataMap := resp.Data.(map[string]any)
	if dataMap["tables_upserted"] != float64(10) {
		t.Errorf("expected tables_upserted 10, got %v", dataMap["tables_upserted"])
	}

	// Verify pending changes are included in response
	if dataMap["pending_changes_created"] != float64(3) {
		t.Errorf("expected pending_changes_created 3, got %v", dataMap["pending_changes_created"])
	}

	// Verify new_table_names and removed_table_names are included
	newTables, ok := dataMap["new_table_names"].([]any)
	if !ok {
		t.Fatalf("expected new_table_names to be an array, got %T", dataMap["new_table_names"])
	}
	if len(newTables) != 2 {
		t.Errorf("expected 2 new tables, got %d", len(newTables))
	}

	removedTables, ok := dataMap["removed_table_names"].([]any)
	if !ok {
		t.Fatalf("expected removed_table_names to be an array, got %T", dataMap["removed_table_names"])
	}
	if len(removedTables) != 1 {
		t.Errorf("expected 1 removed table, got %d", len(removedTables))
	}
}

func TestSchemaHandler_GetTable_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()

	service := &mockSchemaService{
		table: &models.DatasourceTable{
			ID:         tableID,
			SchemaName: "public",
			TableName:  "users",
			IsSelected: true,
			Columns:    []*models.DatasourceColumn{},
		},
	}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/tables/public.users", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	req.SetPathValue("tableName", "public.users")

	rec := httptest.NewRecorder()
	handler.GetTable(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	dataMap := resp.Data.(map[string]any)
	if dataMap["table_name"] != "users" {
		t.Errorf("expected table_name 'users', got %q", dataMap["table_name"])
	}
}

func TestSchemaHandler_GetTable_NotFound(t *testing.T) {
	service := &mockSchemaService{err: apperrors.ErrNotFound}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/tables/nonexistent", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	req.SetPathValue("tableName", "nonexistent")

	rec := httptest.NewRecorder()
	handler.GetTable(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

// Note: TestSchemaHandler_UpdateTableMetadata_Success and _NotFound removed because
// table metadata is now managed through MCP tools (update_table) and stored
// in engine_ontology_table_metadata. The PUT /schema/tables/{tableId}/metadata
// endpoint has been removed.

// Note: TestSchemaHandler_UpdateColumnMetadata_Success removed because
// column metadata is now managed through MCP tools (update_column) and stored
// in engine_ontology_column_metadata. The PUT /schema/columns/{columnId}/metadata
// endpoint has been removed.

func TestSchemaHandler_SaveSelections_Success(t *testing.T) {
	service := &mockSchemaService{}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()

	// Use table and column UUIDs instead of names
	body := fmt.Sprintf(`{"table_selections": {"%s": true}, "column_selections": {"%s": ["%s", "%s"]}}`,
		tableID.String(), tableID.String(), columnID1.String(), columnID2.String())
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/selections", bytes.NewBufferString(body))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.SaveSelections(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestSchemaHandler_SaveSelections_InvalidBody(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{}, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/selections", bytes.NewBufferString(body))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.SaveSelections(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestSchemaHandler_GetRelationships_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	// The handler now calls GetRelationshipsResponse which returns enriched data
	service := &mockSchemaService{}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.GetRelationships(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be an object, got %T", resp.Data)
	}

	// Check for relationships array in the response
	relationships, ok := data["relationships"].([]any)
	if !ok {
		t.Fatalf("expected relationships to be an array, got %T", data["relationships"])
	}

	// Mock returns empty relationships
	if len(relationships) != 0 {
		t.Errorf("expected 0 relationships from mock, got %d", len(relationships))
	}

	// Check for total_count
	if data["total_count"] != float64(0) {
		t.Errorf("expected total_count 0, got %v", data["total_count"])
	}
}

func TestSchemaHandler_AddRelationship_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	service := &mockSchemaService{
		relationship: &models.SchemaRelationship{
			ID:               uuid.New(),
			ProjectID:        projectID,
			SourceTableID:    uuid.New(),
			SourceColumnID:   uuid.New(),
			TargetTableID:    uuid.New(),
			TargetColumnID:   uuid.New(),
			RelationshipType: "manual",
			Cardinality:      "N:1",
			Confidence:       1.0,
		},
	}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	body := `{"source_table": "public.orders", "source_column": "user_id", "target_table": "public.users", "target_column": "id"}`
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships", bytes.NewBufferString(body))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.AddRelationship(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}
}

func TestSchemaHandler_AddRelationship_Conflict(t *testing.T) {
	service := &mockSchemaService{err: apperrors.ErrConflict}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()

	body := `{"source_table": "public.orders", "source_column": "user_id", "target_table": "public.users", "target_column": "id"}`
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships", bytes.NewBufferString(body))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.AddRelationship(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", rec.Code)
	}
}

func TestSchemaHandler_AddRelationship_MissingFields(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{}, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()

	body := `{"source_table": "public.orders"}`
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships", bytes.NewBufferString(body))
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.AddRelationship(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestSchemaHandler_RemoveRelationship_Success(t *testing.T) {
	service := &mockSchemaService{}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()
	relID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships/"+relID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	req.SetPathValue("relId", relID.String())

	rec := httptest.NewRecorder()
	handler.RemoveRelationship(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestSchemaHandler_RemoveRelationship_NotFound(t *testing.T) {
	service := &mockSchemaService{err: apperrors.ErrNotFound}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()
	relID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships/"+relID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	req.SetPathValue("relId", relID.String())

	rec := httptest.NewRecorder()
	handler.RemoveRelationship(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestSchemaHandler_RemoveRelationship_InvalidID(t *testing.T) {
	handler := NewSchemaHandler(&mockSchemaService{}, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema/relationships/not-a-uuid", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	req.SetPathValue("relId", "not-a-uuid")

	rec := httptest.NewRecorder()
	handler.RemoveRelationship(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestSchemaHandler_ServiceError(t *testing.T) {
	service := &mockSchemaService{err: errors.New("database error")}
	handler := NewSchemaHandler(service, nil, zap.NewNop())

	projectID := uuid.New()
	datasourceID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/schema", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())

	rec := httptest.NewRecorder()
	handler.GetSchema(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

type mockOntologyImportService struct {
	importBundleFn func(ctx context.Context, projectID, datasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error)
}

func (m *mockOntologyImportService) ImportBundle(ctx context.Context, projectID, datasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error) {
	return m.importBundleFn(ctx, projectID, datasourceID, bundleBytes)
}

func TestOntologyImportHandler_Import_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	payload := []byte(`{"format":"ekaya-ontology-export","version":1}`)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bundle.json")
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	handler := NewOntologyImportHandler(&mockOntologyImportService{
		importBundleFn: func(ctx context.Context, gotProjectID, gotDatasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error) {
			require.Equal(t, projectID, gotProjectID)
			require.Equal(t, datasourceID, gotDatasourceID)
			require.Equal(t, payload, bundleBytes)
			return &models.OntologyImportResult{
				ImportedAt:           time.Date(2026, time.March, 24, 10, 0, 0, 0, time.UTC),
				CompletionProvenance: models.OntologyCompletionProvenanceImported,
			}, nil
		},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/ontology/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Import(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var response ApiResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.True(t, response.Success)

	dataMap, ok := response.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "imported", dataMap["completion_provenance"])
	require.Equal(t, "2026-03-24T10:00:00Z", dataMap["imported_at"])
}

func TestOntologyImportHandler_Import_ValidationError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bundle.json")
	require.NoError(t, err)
	_, err = part.Write([]byte(`{"format":"ekaya-ontology-export","version":1}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	handler := NewOntologyImportHandler(&mockOntologyImportService{
		importBundleFn: func(ctx context.Context, projectID, datasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error) {
			return nil, &services.OntologyImportValidationError{
				StatusCode: http.StatusBadRequest,
				Code:       "schema_validation_failed",
				Message:    "Ontology bundle does not match the selected datasource schema",
				Report: models.OntologyImportValidationReport{
					MissingTables: []models.OntologyExportTableRef{{
						SchemaName: "public",
						TableName:  "orders",
					}},
				},
			}
		},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/ontology/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Import(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"error":"schema_validation_failed"`)
	require.Contains(t, rec.Body.String(), `"missing_tables":[{"schema_name":"public","table_name":"orders"}]`)
}

func TestOntologyImportHandler_Import_RejectsNonJSONFiles(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "bundle.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte(`not-json`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	handler := NewOntologyImportHandler(&mockOntologyImportService{
		importBundleFn: func(ctx context.Context, projectID, datasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error) {
			t.Fatal("import service should not be called for non-json files")
			return nil, nil
		},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/ontology/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Import(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"error":"invalid_file"`)
}

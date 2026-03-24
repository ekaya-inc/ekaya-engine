package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

type mockOntologyExportService struct {
	buildBundleFn       func(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyExportBundle, error)
	marshalBundleFn     func(bundle *models.OntologyExportBundle) ([]byte, error)
	suggestedFilenameFn func(bundle *models.OntologyExportBundle) string
}

func (m *mockOntologyExportService) BuildBundle(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyExportBundle, error) {
	return m.buildBundleFn(ctx, projectID, datasourceID)
}

func (m *mockOntologyExportService) MarshalBundle(bundle *models.OntologyExportBundle) ([]byte, error) {
	return m.marshalBundleFn(bundle)
}

func (m *mockOntologyExportService) SuggestedFilename(bundle *models.OntologyExportBundle) string {
	if m.suggestedFilenameFn != nil {
		return m.suggestedFilenameFn(bundle)
	}
	return "ontology-export.json"
}

func TestOntologyExportHandler_Export_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	bundle := &models.OntologyExportBundle{
		Format:     models.OntologyExportFormat,
		Version:    models.OntologyExportVersion,
		ExportedAt: time.Date(2026, time.March, 24, 10, 0, 0, 0, time.UTC),
		Project: models.OntologyExportProject{
			Name: "The Look Demo",
		},
	}
	payload := []byte("{\n  \"format\": \"ekaya-ontology-export\"\n}")

	handler := NewOntologyExportHandler(&mockOntologyExportService{
		buildBundleFn: func(ctx context.Context, gotProjectID, gotDatasourceID uuid.UUID) (*models.OntologyExportBundle, error) {
			if gotProjectID != projectID {
				t.Fatalf("unexpected project id: %s", gotProjectID)
			}
			if gotDatasourceID != datasourceID {
				t.Fatalf("unexpected datasource id: %s", gotDatasourceID)
			}
			return bundle, nil
		},
		marshalBundleFn: func(gotBundle *models.OntologyExportBundle) ([]byte, error) {
			if gotBundle != bundle {
				t.Fatal("marshal received unexpected bundle")
			}
			return payload, nil
		},
		suggestedFilenameFn: func(gotBundle *models.OntologyExportBundle) string {
			if gotBundle != bundle {
				t.Fatal("filename received unexpected bundle")
			}
			return "the-look-demo-export.json"
		},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/ontology/export", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Export(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json content type, got %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="the-look-demo-export.json"` {
		t.Fatalf("unexpected content disposition: %q", got)
	}
	if body := rec.Body.String(); body != string(payload) {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestOntologyExportHandler_Export_BuildError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	handler := NewOntologyExportHandler(&mockOntologyExportService{
		buildBundleFn: func(ctx context.Context, gotProjectID, gotDatasourceID uuid.UUID) (*models.OntologyExportBundle, error) {
			return nil, context.DeadlineExceeded
		},
		marshalBundleFn: func(bundle *models.OntologyExportBundle) ([]byte, error) {
			t.Fatal("marshal should not be called on build error")
			return nil, nil
		},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/datasources/"+datasourceID.String()+"/ontology/export", nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Export(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

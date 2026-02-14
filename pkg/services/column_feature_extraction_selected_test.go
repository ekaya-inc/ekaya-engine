package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// minimalSchemaRepo implements only the SchemaRepository methods needed by Phase 1.
// All other methods panic to catch unintended calls.
type minimalSchemaRepo struct {
	repositories.SchemaRepository // embed to satisfy interface; unused methods will panic
	tables                        []*models.SchemaTable
	columns                       []*models.SchemaColumn
	selectedOnlyArg               bool // captures the selectedOnly arg passed to ListTablesByDatasource
}

func (m *minimalSchemaRepo) ListTablesByDatasource(_ context.Context, _, _ uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	m.selectedOnlyArg = selectedOnly
	if selectedOnly {
		var out []*models.SchemaTable
		for _, t := range m.tables {
			if t.IsSelected {
				out = append(out, t)
			}
		}
		return out, nil
	}
	return m.tables, nil
}

func (m *minimalSchemaRepo) ListColumnsByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}

// minimalColumnMetadataRepo satisfies ColumnMetadataRepository without doing anything.
type minimalColumnMetadataRepo struct {
	repositories.ColumnMetadataRepository
}

func TestPhase1_OnlyProcessesSelectedColumns(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	selectedTableID := uuid.New()
	unselectedTableID := uuid.New()

	rowCount := int64(100)

	tables := []*models.SchemaTable{
		{ID: selectedTableID, ProjectID: projectID, DatasourceID: datasourceID, TableName: "users", IsSelected: true, RowCount: &rowCount},
		{ID: unselectedTableID, ProjectID: projectID, DatasourceID: datasourceID, TableName: "internal_logs", IsSelected: false, RowCount: &rowCount},
	}

	columns := []*models.SchemaColumn{
		// Selected table, selected columns
		{ID: uuid.New(), ProjectID: projectID, SchemaTableID: selectedTableID, ColumnName: "id", DataType: "uuid", IsSelected: true, IsPrimaryKey: true},
		{ID: uuid.New(), ProjectID: projectID, SchemaTableID: selectedTableID, ColumnName: "name", DataType: "text", IsSelected: true},
		// Selected table, unselected column
		{ID: uuid.New(), ProjectID: projectID, SchemaTableID: selectedTableID, ColumnName: "internal_note", DataType: "text", IsSelected: false},
		// Unselected table, selected column (should still be excluded)
		{ID: uuid.New(), ProjectID: projectID, SchemaTableID: unselectedTableID, ColumnName: "id", DataType: "uuid", IsSelected: true, IsPrimaryKey: true},
		// Unselected table, unselected column
		{ID: uuid.New(), ProjectID: projectID, SchemaTableID: unselectedTableID, ColumnName: "message", DataType: "text", IsSelected: false},
	}

	schemaRepo := &minimalSchemaRepo{tables: tables, columns: columns}

	svc := NewColumnFeatureExtractionService(
		schemaRepo,
		&minimalColumnMetadataRepo{},
		zap.NewNop(),
	)

	// Access the concrete type to call the unexported method
	concrete := svc.(*columnFeatureExtractionService)
	result, err := concrete.runPhase1DataCollection(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("runPhase1DataCollection failed: %v", err)
	}

	// Verify selectedOnly=true was passed
	if !schemaRepo.selectedOnlyArg {
		t.Error("ListTablesByDatasource was called with selectedOnly=false, expected true")
	}

	// Should only have 2 columns: users.id and users.name
	if result.TotalColumns != 2 {
		t.Errorf("expected 2 columns, got %d", result.TotalColumns)
	}

	// Verify the profiles contain only the expected columns
	profileNames := make(map[string]bool)
	for _, p := range result.Profiles {
		profileNames[p.TableName+"."+p.ColumnName] = true
	}

	expected := []string{"users.id", "users.name"}
	for _, name := range expected {
		if !profileNames[name] {
			t.Errorf("expected profile for %s, not found", name)
		}
	}

	unexpected := []string{"users.internal_note", "internal_logs.id", "internal_logs.message"}
	for _, name := range unexpected {
		if profileNames[name] {
			t.Errorf("unexpected profile for %s (should have been filtered)", name)
		}
	}
}

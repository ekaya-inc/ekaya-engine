package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	sqlpkg "github.com/ekaya-inc/ekaya-engine/pkg/sql"
	sqlvalidator "github.com/ekaya-inc/ekaya-engine/pkg/sql"
)

const ontologyImportDatasourceKey = "primary"

// OntologyImportService imports versioned ontology bundles into an existing datasource.
type OntologyImportService interface {
	ImportBundle(ctx context.Context, projectID, datasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error)
}

type ontologyImportProjectRepository interface {
	Get(ctx context.Context, id uuid.UUID) (*models.Project, error)
}

type ontologyImportDatasourceService interface {
	Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error)
}

type ontologyImportSchemaRepository interface {
	ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	ListAllTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error)
	ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
}

type ontologyImportDAGRepository interface {
	GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)
}

type ontologyImportInstalledAppService interface {
	IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error)
}

type ontologyImportService struct {
	projectRepo         ontologyImportProjectRepository
	datasourceService   ontologyImportDatasourceService
	schemaRepo          ontologyImportSchemaRepository
	dagRepo             ontologyImportDAGRepository
	installedAppService ontologyImportInstalledAppService
	logger              *zap.Logger
}

// OntologyImportValidationError is returned when a bundle cannot be imported safely.
type OntologyImportValidationError struct {
	StatusCode int
	Code       string
	Message    string
	Report     models.OntologyImportValidationReport
}

func (e *OntologyImportValidationError) Error() string {
	return e.Message
}

type ontologyImportPlan struct {
	bundle        *models.OntologyExportBundle
	project       *models.Project
	datasource    *models.Datasource
	importedAt    time.Time
	tableByKey    map[string]*models.SchemaTable
	columnByKey   map[string]*models.SchemaColumn
	tableIDs      []uuid.UUID
	columnIDs     []uuid.UUID
	queryIDsByKey map[string]uuid.UUID
}

// NewOntologyImportService creates a new ontology import service.
func NewOntologyImportService(
	projectRepo ontologyImportProjectRepository,
	datasourceService ontologyImportDatasourceService,
	schemaRepo ontologyImportSchemaRepository,
	dagRepo ontologyImportDAGRepository,
	installedAppService ontologyImportInstalledAppService,
	logger *zap.Logger,
) OntologyImportService {
	return &ontologyImportService{
		projectRepo:         projectRepo,
		datasourceService:   datasourceService,
		schemaRepo:          schemaRepo,
		dagRepo:             dagRepo,
		installedAppService: installedAppService,
		logger:              logger.Named("ontology-import"),
	}
}

func (s *ontologyImportService) ImportBundle(ctx context.Context, projectID, datasourceID uuid.UUID, bundleBytes []byte) (*models.OntologyImportResult, error) {
	plan, err := s.prepareImportPlan(ctx, projectID, datasourceID, bundleBytes)
	if err != nil {
		return nil, err
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin ontology import transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best effort cleanup

	if err := s.applyImport(ctx, tx, plan); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit ontology import: %w", err)
	}

	return &models.OntologyImportResult{
		ImportedAt:           plan.importedAt,
		CompletionProvenance: models.OntologyCompletionProvenanceImported,
	}, nil
}

func (s *ontologyImportService) prepareImportPlan(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	bundleBytes []byte,
) (*ontologyImportPlan, error) {
	report := models.OntologyImportValidationReport{}

	if len(bundleBytes) == 0 {
		report.Problems = append(report.Problems, models.OntologyImportProblem{
			Code:    "invalid_bundle",
			Message: "Ontology bundle file is empty.",
		})
		return nil, newOntologyImportValidationError(400, "invalid_bundle", "Invalid ontology bundle", report)
	}

	if len(bundleBytes) > models.OntologyImportMaxBytes {
		report.Problems = append(report.Problems, models.OntologyImportProblem{
			Code:    "file_too_large",
			Message: "Ontology bundle exceeds the 5 MB maximum size.",
		})
		return nil, newOntologyImportValidationError(400, "file_too_large", "Ontology bundle exceeds the 5 MB maximum size", report)
	}

	bundle, err := decodeOntologyImportBundle(bundleBytes)
	if err != nil {
		report.Problems = append(report.Problems, models.OntologyImportProblem{
			Code:    "invalid_bundle",
			Message: err.Error(),
		})
		return nil, newOntologyImportValidationError(400, "invalid_bundle", "Invalid ontology bundle", report)
	}

	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}

	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load datasource: %w", err)
	}

	if latestDAG, err := s.dagRepo.GetLatestByDatasource(ctx, datasourceID); err != nil {
		return nil, fmt.Errorf("load ontology state: %w", err)
	} else if latestDAG != nil {
		report.Problems = append(report.Problems, models.OntologyImportProblem{
			Code:    "ontology_state_exists",
			Message: "Delete the existing ontology state before importing a bundle.",
		})
		return nil, newOntologyImportValidationError(409, "ontology_state_exists", "Delete the existing ontology state before importing", report)
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}
	if completionState, err := loadOntologyCompletionState(ctx, scope.Conn, projectID, datasourceID); err != nil {
		return nil, fmt.Errorf("load ontology completion state: %w", err)
	} else if completionState.Provenance.IsValid() {
		report.Problems = append(report.Problems, models.OntologyImportProblem{
			Code:    "ontology_state_exists",
			Message: "Delete the existing ontology state before importing a bundle.",
		})
		return nil, newOntologyImportValidationError(409, "ontology_state_exists", "Delete the existing ontology state before importing", report)
	}

	importDatasource := bundle.Datasources[0]
	if ds.DatasourceType != importDatasource.DatasourceType {
		report.DatabaseTypeMismatch = &models.OntologyImportDatabaseTypeMismatch{
			BundleType: importDatasource.DatasourceType,
			TargetType: ds.DatasourceType,
		}
		return nil, newOntologyImportValidationError(400, "database_type_mismatch", "Datasource type does not match the ontology bundle", report)
	}

	for _, appID := range bundle.RequiredApps {
		if !models.KnownAppIDs[appID] {
			report.Problems = append(report.Problems, models.OntologyImportProblem{
				Code:    "invalid_required_app",
				Message: fmt.Sprintf("Bundle references unknown required app %q.", appID),
			})
			continue
		}
		installed, err := s.installedAppService.IsInstalled(ctx, projectID, appID)
		if err != nil {
			return nil, fmt.Errorf("check required app %s: %w", appID, err)
		}
		if !installed {
			report.MissingRequiredApps = append(report.MissingRequiredApps, appID)
		}
	}

	availableTables, err := s.schemaRepo.ListAllTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load datasource schema tables: %w", err)
	}

	availableTableByKey := make(map[string]*models.SchemaTable, len(availableTables))
	availableColumnsByTableKey := make(map[string]map[string]*models.SchemaColumn, len(availableTables))
	availableColumnByKey := make(map[string]*models.SchemaColumn)
	for _, table := range availableTables {
		if table == nil {
			continue
		}
		key := exportTableKey(table.SchemaName, table.TableName)
		availableTableByKey[key] = table

		tableColumns, err := s.schemaRepo.ListAllColumnsByTable(ctx, projectID, table.ID)
		if err != nil {
			return nil, fmt.Errorf("load datasource schema columns for %s.%s: %w", table.SchemaName, table.TableName, err)
		}

		availableColumnsByTableKey[key] = make(map[string]*models.SchemaColumn, len(tableColumns))
		for _, column := range tableColumns {
			if column == nil {
				continue
			}
			availableColumnsByTableKey[key][column.ColumnName] = column
			availableColumnByKey[exportColumnKey(table.SchemaName, table.TableName, column.ColumnName)] = column
		}
	}

	targetTableByKey := make(map[string]*models.SchemaTable, len(importDatasource.SelectedSchema.Tables))
	targetColumnByKey := make(map[string]*models.SchemaColumn)
	tableIDs := make([]uuid.UUID, 0, len(importDatasource.SelectedSchema.Tables))
	columnIDs := make([]uuid.UUID, 0)
	selectedTableIDs := make(map[uuid.UUID]struct{}, len(importDatasource.SelectedSchema.Tables))
	selectedColumnIDs := make(map[uuid.UUID]struct{})
	for _, table := range importDatasource.SelectedSchema.Tables {
		tableKey := exportTableKey(table.SchemaName, table.TableName)
		targetTable, ok := availableTableByKey[tableKey]
		if !ok {
			report.MissingTables = append(report.MissingTables, models.OntologyExportTableRef{
				SchemaName: table.SchemaName,
				TableName:  table.TableName,
			})
			continue
		}

		targetTableByKey[tableKey] = targetTable
		if _, alreadySelected := selectedTableIDs[targetTable.ID]; !alreadySelected {
			tableIDs = append(tableIDs, targetTable.ID)
			selectedTableIDs[targetTable.ID] = struct{}{}
		}

		targetColumnsForTable := availableColumnsByTableKey[tableKey]
		for _, column := range table.Columns {
			targetColumn, ok := targetColumnsForTable[column.ColumnName]
			if !ok {
				report.MissingColumns = append(report.MissingColumns, models.OntologyExportColumnRef{
					Table: models.OntologyExportTableRef{
						SchemaName: table.SchemaName,
						TableName:  table.TableName,
					},
					ColumnName: column.ColumnName,
				})
				continue
			}

			columnKey := exportColumnKey(table.SchemaName, table.TableName, column.ColumnName)
			targetColumnByKey[columnKey] = targetColumn
			if _, alreadySelected := selectedColumnIDs[targetColumn.ID]; !alreadySelected {
				columnIDs = append(columnIDs, targetColumn.ID)
				selectedColumnIDs[targetColumn.ID] = struct{}{}
			}
		}
	}

	for _, relationship := range importDatasource.SelectedSchema.Relationships {
		sourceKey := exportColumnKey(relationship.Source.Table.SchemaName, relationship.Source.Table.TableName, relationship.Source.ColumnName)
		targetKey := exportColumnKey(relationship.Target.Table.SchemaName, relationship.Target.Table.TableName, relationship.Target.ColumnName)
		if _, ok := targetColumnByKey[sourceKey]; !ok {
			message := "Source column is not included in the imported schema."
			if _, exists := availableColumnByKey[sourceKey]; !exists {
				message = "Source column does not exist on the target datasource."
			}
			report.UnresolvedRelationships = append(report.UnresolvedRelationships, models.OntologyImportRelationshipIssue{
				Source:  relationship.Source,
				Target:  relationship.Target,
				Message: message,
			})
			continue
		}
		if _, ok := targetColumnByKey[targetKey]; !ok {
			message := "Target column is not included in the imported schema."
			if _, exists := availableColumnByKey[targetKey]; !exists {
				message = "Target column does not exist on the target datasource."
			}
			report.UnresolvedRelationships = append(report.UnresolvedRelationships, models.OntologyImportRelationshipIssue{
				Source:  relationship.Source,
				Target:  relationship.Target,
				Message: message,
			})
		}
	}

	if report.HasProblems() {
		sortOntologyImportReport(&report)
		return nil, newOntologyImportValidationError(400, "schema_validation_failed", "Ontology bundle does not match the target datasource schema", report)
	}

	if err := normalizeBundleQueries(bundle, &report); err != nil {
		return nil, err
	}
	if report.HasProblems() {
		sortOntologyImportReport(&report)
		return nil, newOntologyImportValidationError(400, "invalid_bundle", "Ontology bundle contains invalid approved queries", report)
	}

	queryIDsByKey := make(map[string]uuid.UUID, len(bundle.ApprovedQueries))
	for _, query := range bundle.ApprovedQueries {
		queryIDsByKey[query.Key] = uuid.New()
	}

	return &ontologyImportPlan{
		bundle:        bundle,
		project:       project,
		datasource:    ds,
		importedAt:    time.Now().UTC(),
		tableByKey:    targetTableByKey,
		columnByKey:   targetColumnByKey,
		tableIDs:      tableIDs,
		columnIDs:     columnIDs,
		queryIDsByKey: queryIDsByKey,
	}, nil
}

func (s *ontologyImportService) applyImport(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	if err := s.replaceSelectedScope(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.clearExistingImportState(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertRelationships(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertTableMetadata(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertColumnMetadata(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertQuestions(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertKnowledge(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertGlossary(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.insertQueries(ctx, tx, plan); err != nil {
		return err
	}
	if err := s.updateProjectOntologyState(ctx, tx, plan); err != nil {
		return err
	}

	return nil
}

func (s *ontologyImportService) replaceSelectedScope(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	now := plan.importedAt
	if _, err := tx.Exec(ctx, `
		UPDATE engine_schema_columns
		SET is_selected = false, updated_at = $3
		WHERE project_id = $1
		  AND schema_table_id IN (
		    SELECT id FROM engine_schema_tables
		    WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
		  )
		  AND deleted_at IS NULL
	`, plan.project.ID, plan.datasource.ID, now); err != nil {
		return fmt.Errorf("clear selected columns: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE engine_schema_tables
		SET is_selected = false, updated_at = $3
		WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
	`, plan.project.ID, plan.datasource.ID, now); err != nil {
		return fmt.Errorf("clear selected tables: %w", err)
	}

	if len(plan.tableIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE engine_schema_tables
			SET is_selected = true, updated_at = $2
			WHERE id = ANY($1)
		`, plan.tableIDs, now); err != nil {
			return fmt.Errorf("select imported tables: %w", err)
		}
	}

	if len(plan.columnIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE engine_schema_columns
			SET is_selected = true, updated_at = $2
			WHERE id = ANY($1)
		`, plan.columnIDs, now); err != nil {
			return fmt.Errorf("select imported columns: %w", err)
		}
	}

	return nil
}

func (s *ontologyImportService) clearExistingImportState(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	projectID := plan.project.ID
	datasourceID := plan.datasource.ID

	if _, err := tx.Exec(ctx, `
		DELETE FROM engine_ontology_table_metadata
		WHERE project_id = $1
		  AND schema_table_id IN (
		    SELECT id FROM engine_schema_tables
		    WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
		  )
	`, projectID, datasourceID); err != nil {
		return fmt.Errorf("delete table metadata: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM engine_ontology_column_metadata
		WHERE project_id = $1
		  AND schema_column_id IN (
		    SELECT c.id
		    FROM engine_schema_columns c
		    JOIN engine_schema_tables t ON t.id = c.schema_table_id
		    WHERE c.project_id = $1
		      AND t.datasource_id = $2
		      AND c.deleted_at IS NULL
		      AND t.deleted_at IS NULL
		  )
	`, projectID, datasourceID); err != nil {
		return fmt.Errorf("delete column metadata: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE engine_schema_relationships
		SET deleted_at = $2, updated_at = $2
		WHERE project_id = $1
		  AND deleted_at IS NULL
		  AND source_table_id IN (
		    SELECT id FROM engine_schema_tables
		    WHERE project_id = $1 AND datasource_id = $3 AND deleted_at IS NULL
		  )
		  AND target_table_id IN (
		    SELECT id FROM engine_schema_tables
		    WHERE project_id = $1 AND datasource_id = $3 AND deleted_at IS NULL
		  )
	`, projectID, plan.importedAt, datasourceID); err != nil {
		return fmt.Errorf("clear relationships: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM engine_ontology_questions WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("delete ontology questions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM engine_project_knowledge WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("delete project knowledge: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM engine_business_glossary WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("delete glossary: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE engine_queries
		SET deleted_at = $3, updated_at = $3
		WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
	`, projectID, datasourceID, plan.importedAt); err != nil {
		return fmt.Errorf("soft delete existing queries: %w", err)
	}

	return nil
}

func (s *ontologyImportService) insertRelationships(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, relationship := range plan.bundle.Datasources[0].SelectedSchema.Relationships {
		sourceID := plan.columnByKey[exportColumnRefKey(relationship.Source)].ID
		targetID := plan.columnByKey[exportColumnRefKey(relationship.Target)].ID
		sourceTableID := plan.tableByKey[exportTableRefKey(relationship.Source.Table)].ID
		targetTableID := plan.tableByKey[exportTableRefKey(relationship.Target.Table)].ID

		validationJSON, err := marshalJSONText(relationship.Validation)
		if err != nil {
			return fmt.Errorf("marshal relationship validation: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_schema_relationships (
				id, project_id, source_table_id, source_column_id, target_table_id, target_column_id,
				relationship_type, cardinality, confidence, inference_method, is_validated,
				validation_results, is_approved, source, last_edit_source, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
			ON CONFLICT (source_column_id, target_column_id)
				WHERE deleted_at IS NULL
			DO UPDATE SET
				relationship_type = EXCLUDED.relationship_type,
				cardinality = EXCLUDED.cardinality,
				confidence = EXCLUDED.confidence,
				inference_method = EXCLUDED.inference_method,
				is_validated = EXCLUDED.is_validated,
				validation_results = EXCLUDED.validation_results,
				is_approved = EXCLUDED.is_approved,
				source = EXCLUDED.source,
				last_edit_source = EXCLUDED.last_edit_source,
				deleted_at = NULL,
				updated_at = EXCLUDED.updated_at
		`, uuid.New(), plan.project.ID, sourceTableID, sourceID, targetTableID, targetID,
			relationship.RelationshipType, relationship.Cardinality, relationship.Confidence,
			relationship.InferenceMethod, relationship.IsValidated, validationJSON, relationship.IsApproved,
			normalizeRelationshipBundleSource(relationship.ProvenanceSource), relationship.LastEditSource, plan.importedAt); err != nil {
			return fmt.Errorf("insert relationship %s -> %s: %w", exportColumnRefKey(relationship.Source), exportColumnRefKey(relationship.Target), err)
		}
	}

	return nil
}

func (s *ontologyImportService) insertTableMetadata(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, item := range plan.bundle.Ontology.TableMetadata {
		featuresJSON, err := marshalJSONText(item.Features)
		if err != nil {
			return fmt.Errorf("marshal table metadata features: %w", err)
		}

		var preferredAlternative *string
		if item.PreferredAlternative != nil {
			preferredAlternative = &item.PreferredAlternative.TableName
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_ontology_table_metadata (
				id, project_id, schema_table_id, table_type, description, usage_notes,
				is_ephemeral, preferred_alternative, confidence, features,
				source, last_edit_source, created_by, updated_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULL, NULL, $13, $13)
		`, uuid.New(), plan.project.ID, plan.tableByKey[exportTableRefKey(item.Table)].ID,
			item.TableType, item.Description, item.UsageNotes, item.IsEphemeral, preferredAlternative,
			item.Confidence, featuresJSON, normalizeBundleSource(item.Source), item.LastEditSource, plan.importedAt); err != nil {
			return fmt.Errorf("insert table metadata for %s: %w", exportTableRefKey(item.Table), err)
		}
	}

	return nil
}

func (s *ontologyImportService) insertColumnMetadata(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, item := range plan.bundle.Ontology.ColumnMetadata {
		featuresJSON, err := marshalJSONText(item.Features)
		if err != nil {
			return fmt.Errorf("marshal column metadata features: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_ontology_column_metadata (
				id, project_id, schema_column_id, classification_path, purpose, semantic_type, role,
				description, confidence, features, needs_enum_analysis, needs_fk_resolution,
				needs_cross_column_check, needs_clarification, clarification_question, is_sensitive,
				source, last_edit_source, created_by, updated_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NULL, NULL, $19, $19)
		`, uuid.New(), plan.project.ID, plan.columnByKey[exportColumnRefKey(item.Column)].ID,
			item.ClassificationPath, item.Purpose, item.SemanticType, item.Role, item.Description, item.Confidence,
			featuresJSON, item.NeedsEnumAnalysis, item.NeedsFKResolution, item.NeedsCrossColumnCheck,
			item.NeedsClarification, item.ClarificationQuestion, item.IsSensitive, normalizeBundleSource(item.Source),
			item.LastEditSource, plan.importedAt); err != nil {
			return fmt.Errorf("insert column metadata for %s: %w", exportColumnRefKey(item.Column), err)
		}
	}

	return nil
}

func (s *ontologyImportService) insertQuestions(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, question := range plan.bundle.Ontology.Questions {
		affects := &models.QuestionAffects{}
		if question.Affects != nil {
			for _, table := range question.Affects.Tables {
				affects.Tables = append(affects.Tables, table.TableName)
			}
			for _, column := range question.Affects.Columns {
				affects.Columns = append(affects.Columns, column.Table.TableName+"."+column.ColumnName)
			}
		}

		affectsJSON, err := marshalJSONText(affects)
		if err != nil {
			return fmt.Errorf("marshal question affects: %w", err)
		}

		var answeredAt *time.Time
		if question.Status == models.QuestionStatusAnswered && strings.TrimSpace(question.Answer) != "" {
			answerTime := plan.importedAt
			answeredAt = &answerTime
		}

		var sourceEntityType *string
		var sourceEntityKey *string
		if len(affects.Tables) > 0 {
			entityType := "table"
			sourceEntityType = &entityType
			sourceEntityKey = &affects.Tables[0]
		}

		contentHash := (&models.OntologyQuestion{
			Category: question.Category,
			Text:     question.Text,
		}).ComputeContentHash()

		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_ontology_questions (
				id, project_id, content_hash, text, reasoning, category, priority, is_required,
				affects, source_entity_type, source_entity_key, status, status_reason, answer,
				answered_by, answered_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NULL, $15, $16, $16)
		`, uuid.New(), plan.project.ID, contentHash, question.Text, nullableString(question.Reasoning),
			nullableString(question.Category), question.Priority, question.IsRequired, affectsJSON,
			sourceEntityType, sourceEntityKey, string(question.Status), nullableString(question.StatusReason),
			nullableString(question.Answer), answeredAt, plan.importedAt); err != nil {
			return fmt.Errorf("insert ontology question %q: %w", question.Text, err)
		}
	}

	return nil
}

func (s *ontologyImportService) insertKnowledge(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, fact := range plan.bundle.Ontology.ProjectKnowledge {
		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_project_knowledge (
				id, project_id, fact_type, value, context, source, last_edit_source,
				created_by, updated_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, NULL, $8, $8)
		`, uuid.New(), plan.project.ID, fact.FactType, fact.Value, nullableString(fact.Context),
			normalizeBundleSource(fact.Source), fact.LastEditSource, plan.importedAt); err != nil {
			return fmt.Errorf("insert project knowledge fact %q: %w", fact.Value, err)
		}
	}

	return nil
}

func (s *ontologyImportService) insertGlossary(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, term := range plan.bundle.Ontology.GlossaryTerms {
		outputColumnsJSON, err := marshalJSONText(term.OutputColumns)
		if err != nil {
			return fmt.Errorf("marshal glossary output columns: %w", err)
		}

		termID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_business_glossary (
				id, project_id, term, definition, defining_sql, base_table, output_columns,
				enrichment_status, enrichment_error, source, last_edit_source, created_by, updated_by,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL, NULL, $12, $12)
		`, termID, plan.project.ID, term.Term, term.Definition, term.DefiningSQL, nullableString(term.BaseTable),
			outputColumnsJSON, nullableString(term.EnrichmentStatus), nullableString(term.EnrichmentError),
			normalizeBundleSource(term.Source), term.LastEditSource, plan.importedAt); err != nil {
			return fmt.Errorf("insert glossary term %q: %w", term.Term, err)
		}

		for _, alias := range term.Aliases {
			if _, err := tx.Exec(ctx, `
				INSERT INTO engine_glossary_aliases (id, glossary_id, alias, created_at)
				VALUES ($1, $2, $3, $4)
			`, uuid.New(), termID, alias, plan.importedAt); err != nil {
				return fmt.Errorf("insert glossary alias %q: %w", alias, err)
			}
		}
	}

	return nil
}

func (s *ontologyImportService) insertQueries(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	for _, query := range plan.bundle.ApprovedQueries {
		parametersJSON, err := marshalJSONText(query.Parameters)
		if err != nil {
			return fmt.Errorf("marshal query parameters: %w", err)
		}
		outputColumnsJSON, err := marshalJSONText(query.OutputColumns)
		if err != nil {
			return fmt.Errorf("marshal query output columns: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO engine_queries (
				id, project_id, datasource_id, natural_language_prompt, additional_context,
				sql_query, dialect, is_enabled, usage_count, last_used_at, parameters, output_columns,
				constraints, status, suggested_by, suggestion_context, tags, allows_modification,
				reviewed_by, reviewed_at, rejection_reason, parent_query_id, created_at, updated_at, deleted_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, 0, NULL, $9, $10,
				$11, 'approved', NULL, NULL, $12, $13,
				NULL, NULL, NULL, NULL, $14, $14, NULL
			)
		`, plan.queryIDsByKey[query.Key], plan.project.ID, plan.datasource.ID, query.NaturalLanguagePrompt,
			query.AdditionalContext, query.SQL, query.Dialect, query.Enabled, parametersJSON, outputColumnsJSON,
			query.Constraints, query.Tags, query.AllowsModification, plan.importedAt); err != nil {
			return fmt.Errorf("insert approved query %q: %w", query.Key, err)
		}
	}

	return nil
}

func (s *ontologyImportService) updateProjectOntologyState(ctx context.Context, tx pgx.Tx, plan *ontologyImportPlan) error {
	parameters, err := loadProjectParameters(ctx, tx, plan.project.ID)
	if err != nil {
		return err
	}

	if err := setOntologyCompletionState(parameters, plan.datasource.ID, models.OntologyCompletionProvenanceImported, plan.importedAt); err != nil {
		return err
	}

	parametersJSON, err := json.Marshal(parameters)
	if err != nil {
		return fmt.Errorf("encode project parameters: %w", err)
	}

	var domainSummaryJSON *string
	if plan.bundle.Project.DomainSummary != nil {
		raw, err := marshalJSONText(plan.bundle.Project.DomainSummary)
		if err != nil {
			return fmt.Errorf("marshal domain summary: %w", err)
		}
		domainSummaryJSON = &raw
	}

	industryType := plan.bundle.Project.IndustryType
	if industryType == "" {
		industryType = plan.project.IndustryType
	}

	if _, err := tx.Exec(ctx, `
		UPDATE engine_projects
		SET parameters = $2, industry_type = $3, domain_summary = $4, updated_at = $5
		WHERE id = $1
	`, plan.project.ID, parametersJSON, industryType, domainSummaryJSON, plan.importedAt); err != nil {
		return fmt.Errorf("update imported project state: %w", err)
	}

	return nil
}

func decodeOntologyImportBundle(bundleBytes []byte) (*models.OntologyExportBundle, error) {
	var bundle models.OntologyExportBundle
	decoder := json.NewDecoder(strings.NewReader(string(bundleBytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("failed to parse ontology bundle JSON: %w", err)
	}

	if bundle.Format != models.OntologyExportFormat {
		return nil, fmt.Errorf("unsupported ontology bundle format %q", bundle.Format)
	}
	if bundle.Version != models.OntologyExportVersion {
		return nil, fmt.Errorf("unsupported ontology bundle version %d", bundle.Version)
	}
	if len(bundle.Datasources) != 1 {
		return nil, fmt.Errorf("ontology import requires exactly one datasource bundle")
	}
	if bundle.Security.IncludesDatasourceCredentials || bundle.Security.IncludesAIConfig || bundle.Security.IncludesAgentAPIKeys {
		return nil, fmt.Errorf("ontology bundle must not include datasource credentials, AI config, or agent API keys")
	}

	return &bundle, nil
}

func normalizeBundleQueries(bundle *models.OntologyExportBundle, report *models.OntologyImportValidationReport) error {
	for index := range bundle.ApprovedQueries {
		query := &bundle.ApprovedQueries[index]
		if query.DatasourceKey != ontologyImportDatasourceKey {
			report.Problems = append(report.Problems, models.OntologyImportProblem{
				Code:    "invalid_query_datasource_key",
				Message: fmt.Sprintf("Approved query %q references datasource key %q.", query.Key, query.DatasourceKey),
			})
			continue
		}

		validationResult := sqlvalidator.ValidateAndNormalize(query.SQL)
		if validationResult.Error != nil {
			report.Problems = append(report.Problems, models.OntologyImportProblem{
				Code:    "invalid_query_sql",
				Message: fmt.Sprintf("Approved query %q is invalid: %s", query.Key, validationResult.Error.Error()),
			})
			continue
		}
		query.SQL = validationResult.NormalizedSQL

		sqlType, err := ValidateSQLType(query.SQL, query.AllowsModification)
		if err != nil {
			report.Problems = append(report.Problems, models.OntologyImportProblem{
				Code:    "invalid_query_sql",
				Message: fmt.Sprintf("Approved query %q is invalid: %s", query.Key, err.Error()),
			})
			continue
		}
		if ShouldAutoCorrectAllowsModification(sqlType, query.AllowsModification) {
			query.AllowsModification = false
		}
		if err := sqlpkg.ValidateParameterDefinitions(query.SQL, query.Parameters); err != nil {
			report.Problems = append(report.Problems, models.OntologyImportProblem{
				Code:    "invalid_query_parameters",
				Message: fmt.Sprintf("Approved query %q has invalid parameter definitions: %s", query.Key, err.Error()),
			})
		}
	}

	return nil
}

func newOntologyImportValidationError(statusCode int, code, message string, report models.OntologyImportValidationReport) error {
	sortOntologyImportReport(&report)
	return &OntologyImportValidationError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
		Report:     report,
	}
}

func sortOntologyImportReport(report *models.OntologyImportValidationReport) {
	sort.Slice(report.Problems, func(i, j int) bool {
		if report.Problems[i].Code != report.Problems[j].Code {
			return report.Problems[i].Code < report.Problems[j].Code
		}
		return report.Problems[i].Message < report.Problems[j].Message
	})
	sort.Slice(report.MissingTables, func(i, j int) bool {
		return compareImportTableRefs(report.MissingTables[i], report.MissingTables[j]) < 0
	})
	sort.Slice(report.UnexpectedTables, func(i, j int) bool {
		return compareImportTableRefs(report.UnexpectedTables[i], report.UnexpectedTables[j]) < 0
	})
	sort.Slice(report.MissingColumns, func(i, j int) bool {
		return compareImportColumnRefs(report.MissingColumns[i], report.MissingColumns[j]) < 0
	})
	sort.Slice(report.UnexpectedColumns, func(i, j int) bool {
		return compareImportColumnRefs(report.UnexpectedColumns[i], report.UnexpectedColumns[j]) < 0
	})
	sort.Slice(report.UnresolvedRelationships, func(i, j int) bool {
		if cmp := compareImportColumnRefs(report.UnresolvedRelationships[i].Source, report.UnresolvedRelationships[j].Source); cmp != 0 {
			return cmp < 0
		}
		return compareImportColumnRefs(report.UnresolvedRelationships[i].Target, report.UnresolvedRelationships[j].Target) < 0
	})
	sort.Strings(report.MissingRequiredApps)
}

func compareImportTableRefs(left, right models.OntologyExportTableRef) int {
	if left.SchemaName != right.SchemaName {
		return strings.Compare(left.SchemaName, right.SchemaName)
	}
	return strings.Compare(left.TableName, right.TableName)
}

func compareImportColumnRefs(left, right models.OntologyExportColumnRef) int {
	if cmp := compareImportTableRefs(left.Table, right.Table); cmp != 0 {
		return cmp
	}
	return strings.Compare(left.ColumnName, right.ColumnName)
}

func exportTableKey(schemaName, tableName string) string {
	return strings.TrimSpace(schemaName) + "." + strings.TrimSpace(tableName)
}

func exportTableRefKey(ref models.OntologyExportTableRef) string {
	return exportTableKey(ref.SchemaName, ref.TableName)
}

func exportColumnKey(schemaName, tableName, columnName string) string {
	return exportTableKey(schemaName, tableName) + "." + strings.TrimSpace(columnName)
}

func exportColumnRefKey(ref models.OntologyExportColumnRef) string {
	return exportColumnKey(ref.Table.SchemaName, ref.Table.TableName, ref.ColumnName)
}

func marshalJSONText(value any) (string, error) {
	if value == nil {
		return "null", nil
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func normalizeBundleSource(source string) string {
	if models.ProvenanceSource(source).IsValid() {
		return source
	}
	return models.ProvenanceManual
}

func normalizeRelationshipBundleSource(source string) string {
	if models.ProvenanceSource(source).IsValid() {
		return source
	}
	return models.ProvenanceInferred
}

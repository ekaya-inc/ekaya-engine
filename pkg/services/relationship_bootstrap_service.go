package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// RelationshipBootstrapResult summarizes the FKDiscovery bootstrap stage.
type RelationshipBootstrapResult struct {
	FKRelationships            int `json:"fk_relationships"`
	ColumnFeatureRelationships int `json:"column_feature_relationships"`
	DeclaredFKRelationships    int `json:"declared_fk_relationships"`
}

// RelationshipBootstrapService owns the early FKDiscovery bootstrap stage.
type RelationshipBootstrapService interface {
	Bootstrap(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*RelationshipBootstrapResult, error)
}

type relationshipBootstrapService struct {
	datasourceService  DatasourceService
	adapterFactory     datasource.DatasourceAdapterFactory
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	logger             *zap.Logger
}

// NewRelationshipBootstrapService creates a bootstrap service for the FKDiscovery DAG stage.
func NewRelationshipBootstrapService(
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	logger *zap.Logger,
) RelationshipBootstrapService {
	return &relationshipBootstrapService{
		datasourceService:  datasourceService,
		adapterFactory:     adapterFactory,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		logger:             logger.Named("relationship-bootstrap"),
	}
}

func (s *relationshipBootstrapService) Bootstrap(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (*RelationshipBootstrapResult, error) {
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	tableByID := make(map[uuid.UUID]*models.SchemaTable, len(tables))
	tableByName := make(map[string]*models.SchemaTable, len(tables)*2)
	for _, table := range tables {
		tableByID[table.ID] = table
		tableByName[fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)] = table
		tableByName[table.TableName] = table
	}

	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}

	columnByID := make(map[uuid.UUID]*models.SchemaColumn, len(columns))
	columnIDs := make([]uuid.UUID, 0, len(columns))
	for _, column := range columns {
		columnByID[column.ID] = column
		columnIDs = append(columnIDs, column.ID)
	}

	metadataByColumnID := make(map[uuid.UUID]*models.ColumnMetadata, len(columnIDs))
	if s.columnMetadataRepo != nil && len(columnIDs) > 0 {
		metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, columnIDs)
		if err != nil {
			return nil, fmt.Errorf("get column metadata: %w", err)
		}
		for _, metadata := range metadataList {
			metadataByColumnID[metadata.SchemaColumnID] = metadata
		}
	}

	existingRelationships, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list schema relationships: %w", err)
	}
	declaredFKKeys := buildDeclaredFKRelationshipSet(existingRelationships)

	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	columnFeatureRelationships, err := s.bootstrapColumnFeatureRelationships(
		ctx,
		projectID,
		columns,
		tableByID,
		tableByName,
		metadataByColumnID,
		declaredFKKeys,
		discoverer,
		progressCallback,
	)
	if err != nil {
		return nil, fmt.Errorf("bootstrap column_features relationships: %w", err)
	}

	declaredFKRelationships, err := s.refreshDeclaredFKRelationships(
		ctx,
		existingRelationships,
		tableByID,
		columnByID,
		discoverer,
		progressCallback,
	)
	if err != nil {
		return nil, fmt.Errorf("refresh declared FK relationships: %w", err)
	}

	result := &RelationshipBootstrapResult{
		FKRelationships:            columnFeatureRelationships + declaredFKRelationships,
		ColumnFeatureRelationships: columnFeatureRelationships,
		DeclaredFKRelationships:    declaredFKRelationships,
	}

	s.logger.Info("Relationship bootstrap complete",
		zap.Int("column_feature_relationships", result.ColumnFeatureRelationships),
		zap.Int("declared_fk_relationships", result.DeclaredFKRelationships),
		zap.Int("total", result.FKRelationships),
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()))

	return result, nil
}

func (s *relationshipBootstrapService) bootstrapColumnFeatureRelationships(
	ctx context.Context,
	projectID uuid.UUID,
	columns []*models.SchemaColumn,
	tableByID map[uuid.UUID]*models.SchemaTable,
	tableByName map[string]*models.SchemaTable,
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
	declaredFKKeys map[string]struct{},
	discoverer datasource.SchemaDiscoverer,
	progressCallback dag.ProgressCallback,
) (int, error) {
	type fkColumn struct {
		column     *models.SchemaColumn
		identifier *models.IdentifierFeatures
	}

	fkColumns := make([]fkColumn, 0)
	for _, column := range columns {
		metadata := metadataByColumnID[column.ID]
		if metadata == nil {
			continue
		}

		identifier := metadata.GetIdentifierFeatures()
		if identifier == nil || identifier.FKTargetTable == "" {
			continue
		}

		confidence := identifier.FKConfidence
		if confidence == 0 {
			confidence = 0.9
		}
		if confidence < 0.8 {
			continue
		}

		fkColumns = append(fkColumns, fkColumn{column: column, identifier: identifier})
	}

	createdCount := 0
	for i, candidate := range fkColumns {
		sourceColumn := candidate.column
		identifier := candidate.identifier

		sourceTable := tableByID[sourceColumn.SchemaTableID]
		if sourceTable == nil {
			continue
		}

		targetTableKey := identifier.FKTargetTable
		if !strings.Contains(targetTableKey, ".") {
			targetTableKey = fmt.Sprintf("%s.%s", sourceTable.SchemaName, targetTableKey)
		}

		targetTable := tableByName[targetTableKey]
		if targetTable == nil {
			targetTable = tableByName[identifier.FKTargetTable]
		}
		if targetTable == nil {
			s.logger.Debug("Skipping column_features relationship with missing target table",
				zap.String("source_column_id", sourceColumn.ID.String()),
				zap.String("target_table", identifier.FKTargetTable))
			continue
		}

		targetColumnName := identifier.FKTargetColumn
		if targetColumnName == "" {
			targetColumnName = "id"
		}

		var targetColumn *models.SchemaColumn
		for _, column := range columns {
			if column.SchemaTableID == targetTable.ID && column.ColumnName == targetColumnName {
				targetColumn = column
				break
			}
		}
		if targetColumn == nil {
			s.logger.Debug("Skipping column_features relationship with missing target column",
				zap.String("target_table", targetTable.TableName),
				zap.String("target_column", targetColumnName))
			continue
		}

		if _, exists := declaredFKKeys[relationshipColumnKey(sourceColumn.ID, targetColumn.ID)]; exists {
			continue
		}

		if progressCallback != nil {
			progressCallback(i+1, len(fkColumns), fmt.Sprintf(
				"Materializing ColumnFeatures FK %s.%s -> %s.%s",
				sourceTable.TableName, sourceColumn.ColumnName, targetTable.TableName, targetColumn.ColumnName,
			))
		}

		cardinality := models.CardinalityNTo1
		joinResult, err := discoverer.AnalyzeJoin(
			ctx,
			sourceTable.SchemaName, sourceTable.TableName, sourceColumn.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetColumn.ColumnName,
		)
		if err == nil {
			cardinality = InferCardinality(sourceColumn.IsPrimaryKey, sourceColumn.IsUnique, joinResult)
		}

		confidence := identifier.FKConfidence
		if confidence == 0 {
			confidence = 0.9
		}
		inferenceMethod := models.InferenceMethodColumnFeatures
		rel := &models.SchemaRelationship{
			ProjectID:        projectID,
			SourceTableID:    sourceTable.ID,
			SourceColumnID:   sourceColumn.ID,
			TargetTableID:    targetTable.ID,
			TargetColumnID:   targetColumn.ID,
			RelationshipType: models.RelationshipTypeInferred,
			Cardinality:      cardinality,
			Confidence:       confidence,
			InferenceMethod:  &inferenceMethod,
			IsValidated:      true,
		}

		var sourceDistinct, targetDistinct int64
		if sourceColumn.DistinctCount != nil {
			sourceDistinct = *sourceColumn.DistinctCount
		}
		if targetColumn.DistinctCount != nil {
			targetDistinct = *targetColumn.DistinctCount
		}

		metrics := &models.DiscoveryMetrics{
			MatchRate:      confidence,
			SourceDistinct: sourceDistinct,
			TargetDistinct: targetDistinct,
		}

		if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
			return createdCount, fmt.Errorf("upsert column_features relationship: %w", err)
		}

		createdCount++
	}

	return createdCount, nil
}

func (s *relationshipBootstrapService) refreshDeclaredFKRelationships(
	ctx context.Context,
	relationships []*models.SchemaRelationship,
	tableByID map[uuid.UUID]*models.SchemaTable,
	columnByID map[uuid.UUID]*models.SchemaColumn,
	discoverer datasource.SchemaDiscoverer,
	progressCallback dag.ProgressCallback,
) (int, error) {
	declaredFKs := make([]*models.SchemaRelationship, 0)
	for _, relationship := range relationships {
		if relationship.InferenceMethod != nil && *relationship.InferenceMethod != models.InferenceMethodFK {
			continue
		}
		declaredFKs = append(declaredFKs, relationship)
	}

	updatedCount := 0
	for i, relationship := range declaredFKs {
		sourceTable := tableByID[relationship.SourceTableID]
		sourceColumn := columnByID[relationship.SourceColumnID]
		targetTable := tableByID[relationship.TargetTableID]
		targetColumn := columnByID[relationship.TargetColumnID]
		if sourceTable == nil || sourceColumn == nil || targetTable == nil || targetColumn == nil {
			continue
		}

		if progressCallback != nil {
			progressCallback(i+1, len(declaredFKs), fmt.Sprintf(
				"Refreshing declared FK %s.%s -> %s.%s",
				sourceTable.TableName, sourceColumn.ColumnName, targetTable.TableName, targetColumn.ColumnName,
			))
		}

		cardinality := models.CardinalityNTo1
		joinResult, err := discoverer.AnalyzeJoin(
			ctx,
			sourceTable.SchemaName, sourceTable.TableName, sourceColumn.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetColumn.ColumnName,
		)
		if err != nil {
			s.logger.Warn("Failed to analyze declared FK join; using default cardinality",
				zap.String("source", fmt.Sprintf("%s.%s.%s", sourceTable.SchemaName, sourceTable.TableName, sourceColumn.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s.%s", targetTable.SchemaName, targetTable.TableName, targetColumn.ColumnName)),
				zap.Error(err))
		} else {
			cardinality = InferCardinality(sourceColumn.IsPrimaryKey, sourceColumn.IsUnique, joinResult)
		}

		relationship.Cardinality = cardinality
		relationship.IsValidated = true
		if err := s.schemaRepo.UpsertRelationship(ctx, relationship); err != nil {
			return updatedCount, fmt.Errorf("update declared FK relationship: %w", err)
		}

		updatedCount++
	}

	return updatedCount, nil
}

func buildDeclaredFKRelationshipSet(relationships []*models.SchemaRelationship) map[string]struct{} {
	set := make(map[string]struct{})
	for _, relationship := range relationships {
		if relationship.InferenceMethod != nil && *relationship.InferenceMethod != models.InferenceMethodFK {
			continue
		}
		set[relationshipColumnKey(relationship.SourceColumnID, relationship.TargetColumnID)] = struct{}{}
	}
	return set
}

func relationshipColumnKey(sourceColumnID, targetColumnID uuid.UUID) string {
	return fmt.Sprintf("%s->%s", sourceColumnID, targetColumnID)
}

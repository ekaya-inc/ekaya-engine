package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// reconcileRelationshipBackedColumnMetadata makes schema relationships authoritative for the
// source column's inferred metadata. When a column is confirmed to reference another table,
// incompatible inferred classifications like enum should be replaced with FK semantics.
//
// Repository merge rules still apply when persisting the updated metadata.
func reconcileRelationshipBackedColumnMetadata(
	ctx context.Context,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	projectID uuid.UUID,
	sourceColumn *models.SchemaColumn,
	targetTable *models.SchemaTable,
	targetColumn *models.SchemaColumn,
	confidence float64,
) error {
	if columnMetadataRepo == nil || sourceColumn == nil || targetTable == nil || targetColumn == nil {
		return nil
	}

	existing, err := columnMetadataRepo.GetBySchemaColumnID(ctx, sourceColumn.ID)
	if err != nil {
		return fmt.Errorf("get source column metadata: %w", err)
	}

	meta := &models.ColumnMetadata{
		ProjectID:      projectID,
		SchemaColumnID: sourceColumn.ID,
	}
	if existing != nil {
		copied := *existing
		meta = &copied
		if meta.ProjectID == uuid.Nil {
			meta.ProjectID = projectID
		}
	}

	classificationPath := string(relationshipBackedClassificationPath(sourceColumn.DataType))
	purpose := models.PurposeIdentifier
	semanticType := models.PurposeIdentifier
	role := models.RoleForeignKey

	meta.ClassificationPath = &classificationPath
	meta.Purpose = &purpose
	meta.SemanticType = &semanticType
	meta.Role = &role
	meta.Confidence = floatPointer(maxFloat(pointerValue(meta.Confidence), confidence))

	if meta.Description == nil || *meta.Description == "" {
		description := fmt.Sprintf("References %s.%s.", targetTable.TableName, targetColumn.ColumnName)
		meta.Description = &description
	}

	meta.Features.TimestampFeatures = nil
	meta.Features.BooleanFeatures = nil
	meta.Features.EnumFeatures = nil
	meta.Features.MonetaryFeatures = nil
	if meta.Features.IdentifierFeatures == nil {
		meta.Features.IdentifierFeatures = &models.IdentifierFeatures{}
	}
	meta.Features.IdentifierFeatures.IdentifierType = models.IdentifierTypeForeignKey
	meta.Features.IdentifierFeatures.FKTargetTable = targetTable.TableName
	meta.Features.IdentifierFeatures.FKTargetColumn = targetColumn.ColumnName
	meta.Features.IdentifierFeatures.FKConfidence = confidence

	meta.NeedsEnumAnalysis = false
	meta.NeedsFKResolution = false
	meta.NeedsCrossColumnCheck = false

	return columnMetadataRepo.UpsertFromExtraction(ctx, meta)
}

func relationshipBackedClassificationPath(dataType string) models.ClassificationPath {
	switch {
	case isUUIDTypeForClassification(dataType):
		return models.ClassificationPathUUID
	case isIntegerType(dataType):
		return models.ClassificationPathNumeric
	case isTextType(dataType):
		return models.ClassificationPathText
	case isJSONTypeForClassification(dataType):
		return models.ClassificationPathJSON
	default:
		return models.ClassificationPathUnknown
	}
}

func pointerValue(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func floatPointer(v float64) *float64 {
	return &v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

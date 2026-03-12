package etl

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyMatcher matches inferred schema against existing ontology metadata.
type OntologyMatcher struct {
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	tableMetadataRepo  repositories.TableMetadataRepository
	logger             *zap.Logger
}

// NewOntologyMatcher creates a new ontology matcher.
func NewOntologyMatcher(
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	tableMetadataRepo repositories.TableMetadataRepository,
	logger *zap.Logger,
) *OntologyMatcher {
	return &OntologyMatcher{
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		tableMetadataRepo:  tableMetadataRepo,
		logger:             logger,
	}
}

// Match attempts to match inferred columns and a candidate table name against the project's ontology.
func (m *OntologyMatcher) Match(ctx context.Context, projectID uuid.UUID, candidateTable string, inferred []models.InferredColumn) (*models.MatchResult, error) {
	// Get all selected table names in the project
	tableNames, err := m.schemaRepo.GetSelectedTableNamesByProject(ctx, projectID)
	if err != nil {
		m.logger.Warn("failed to query ontology tables, treating as new table",
			zap.Error(err))
		return newTableResult(), nil
	}

	if len(tableNames) == 0 {
		return newTableResult(), nil
	}

	// Find the best matching table by name similarity
	bestTable, bestScore := findBestTableMatch(candidateTable, tableNames)
	if bestScore < 0.5 {
		return newTableResult(), nil
	}

	// Get columns for the matched table
	columnsByTable, err := m.schemaRepo.GetColumnsByTables(ctx, projectID, []string{bestTable})
	if err != nil {
		m.logger.Warn("failed to get columns for matched table",
			zap.String("table", bestTable), zap.Error(err))
		return newTableResult(), nil
	}

	existingColumns := columnsByTable[bestTable]
	if len(existingColumns) == 0 {
		return newTableResult(), nil
	}

	// Try to get column metadata for semantic matching
	var columnMetadata map[uuid.UUID]*models.ColumnMetadata
	allMeta, err := m.columnMetadataRepo.GetByProject(ctx, projectID)
	if err == nil {
		columnMetadata = make(map[uuid.UUID]*models.ColumnMetadata, len(allMeta))
		for _, meta := range allMeta {
			columnMetadata[meta.SchemaColumnID] = meta
		}
	}

	// Match columns
	mappings := matchColumns(inferred, existingColumns, columnMetadata)

	// Calculate overall confidence
	matchedCount := 0
	totalConfidence := 0.0
	for _, m := range mappings {
		if m.MappedName != "" {
			matchedCount++
			totalConfidence += m.Confidence
		}
	}

	confidence := 0.0
	if matchedCount > 0 {
		confidence = totalConfidence / float64(len(inferred))
	}

	// Only propose a match if enough columns match
	if float64(matchedCount)/float64(len(inferred)) < 0.6 {
		return newTableResult(), nil
	}

	if hasMissingNotNullColumns(existingColumns, mappings) {
		return newTableResult(), nil
	}

	return &models.MatchResult{
		MatchedTable:   bestTable,
		ColumnMappings: mappings,
		Confidence:     confidence,
		IsNewTable:     false,
	}, nil
}

func newTableResult() *models.MatchResult {
	return &models.MatchResult{
		IsNewTable: true,
		Confidence: 0,
	}
}

// hasMissingNotNullColumns returns true if the existing table has any NOT NULL columns
// (without defaults) that are not covered by the column mappings. Such columns would
// cause INSERT failures, so the match should be rejected in favor of a new table.
func hasMissingNotNullColumns(existing []*models.SchemaColumn, mappings []models.ColumnMapping) bool {
	mapped := make(map[string]bool, len(mappings))
	for _, m := range mappings {
		if m.MappedName != "" {
			mapped[m.MappedName] = true
		}
	}
	for _, col := range existing {
		if !col.IsNullable && col.DefaultValue == nil && !mapped[col.ColumnName] {
			return true
		}
	}
	return false
}

// findBestTableMatch finds the table name with the highest similarity to the candidate.
func findBestTableMatch(candidate string, tables []string) (string, float64) {
	normalized := normalizeIdentifier(candidate)
	bestTable := ""
	bestScore := 0.0

	for _, t := range tables {
		score := identifierSimilarity(normalized, normalizeIdentifier(t))
		if score > bestScore {
			bestScore = score
			bestTable = t
		}
	}
	return bestTable, bestScore
}

// matchColumns maps inferred columns to existing schema columns.
func matchColumns(inferred []models.InferredColumn, existing []*models.SchemaColumn, metadata map[uuid.UUID]*models.ColumnMetadata) []models.ColumnMapping {
	mappings := make([]models.ColumnMapping, len(inferred))

	// Build lookup of existing columns by normalized name
	existingByName := make(map[string]*models.SchemaColumn, len(existing))
	for _, col := range existing {
		existingByName[normalizeIdentifier(col.ColumnName)] = col
	}

	for i, inf := range inferred {
		mappings[i] = models.ColumnMapping{
			InferredName: inf.Name,
			InferredType: inf.SQLType,
		}

		normalizedInferred := normalizeIdentifier(inf.Name)

		// Exact match
		if col, ok := existingByName[normalizedInferred]; ok {
			mappings[i].MappedName = col.ColumnName
			mappings[i].MappedType = col.DataType
			mappings[i].Confidence = 1.0

			// Check semantic type from metadata
			if metadata != nil {
				if meta, ok := metadata[col.ID]; ok {
					mappings[i].SemanticMatch = meta.SemanticType != nil && *meta.SemanticType != ""
					classPath := ""
					if meta.ClassificationPath != nil {
						classPath = *meta.ClassificationPath
					}
					mappings[i].MappedType = mapSemanticToSQL(classPath, col.DataType)
				}
			}
			continue
		}

		// Fuzzy match
		bestCol := ""
		bestScore := 0.0
		for _, col := range existing {
			score := identifierSimilarity(normalizedInferred, normalizeIdentifier(col.ColumnName))
			if score > bestScore && score >= 0.7 {
				bestScore = score
				bestCol = col.ColumnName
				mappings[i].MappedType = col.DataType
			}
		}
		if bestCol != "" {
			mappings[i].MappedName = bestCol
			mappings[i].Confidence = bestScore
		}
	}

	return mappings
}

// normalizeIdentifier normalizes an identifier for comparison.
func normalizeIdentifier(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	// Remove double underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	return s
}

// identifierSimilarity computes similarity between two normalized identifiers (0..1).
func identifierSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}

	// Check if one contains the other
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter := len(a)
		if len(b) < shorter {
			shorter = len(b)
		}
		longer := len(a)
		if len(b) > longer {
			longer = len(b)
		}
		return float64(shorter) / float64(longer)
	}

	// Token-based similarity
	tokensA := strings.Split(a, "_")
	tokensB := strings.Split(b, "_")

	matches := 0
	for _, ta := range tokensA {
		for _, tb := range tokensB {
			if ta == tb {
				matches++
				break
			}
		}
	}

	maxTokens := len(tokensA)
	if len(tokensB) > maxTokens {
		maxTokens = len(tokensB)
	}
	if maxTokens == 0 {
		return 0.0
	}
	return float64(matches) / float64(maxTokens)
}

// mapSemanticToSQL maps ontology semantic types to better SQL types.
func mapSemanticToSQL(classificationPath, fallback string) string {
	switch classificationPath {
	case "numeric":
		return "NUMERIC"
	case "timestamp":
		return "TIMESTAMPTZ"
	case "boolean":
		return "BOOLEAN"
	case "uuid":
		return "UUID"
	default:
		return fallback
	}
}

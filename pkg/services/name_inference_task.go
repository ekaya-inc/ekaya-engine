package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

const (
	// Confidence scores for name-based inference
	// Per plan spec: 0.7-0.8 for name inference
	ConfidenceTableIDPattern   = 0.8 // {table}_id → {table}.id (e.g., user_id → users.id)
	ConfidenceColumnNameMatch  = 0.7 // column name matches table name (e.g., user → users.id)
)

// NameInferenceTask detects relationships based on column naming patterns.
// Supports:
//  - {table}_id → {table}.id (e.g., user_id → users.id)
//  - column name matches table name (singular/plural) (e.g., user → users.id)
type NameInferenceTask struct {
	workqueue.BaseTask
	candidateRepo repositories.RelationshipCandidateRepository
	schemaRepo    repositories.SchemaRepository
	getTenantCtx  TenantContextFunc
	projectID     uuid.UUID
	workflowID    uuid.UUID
	datasourceID  uuid.UUID
	logger        *zap.Logger
}

// NewNameInferenceTask creates a new name inference task.
func NewNameInferenceTask(
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	logger *zap.Logger,
) *NameInferenceTask {
	return &NameInferenceTask{
		BaseTask:      workqueue.NewBaseTask("Infer relationships from naming patterns", false), // Non-LLM task
		candidateRepo: candidateRepo,
		schemaRepo:    schemaRepo,
		getTenantCtx:  getTenantCtx,
		projectID:     projectID,
		workflowID:    workflowID,
		datasourceID:  datasourceID,
		logger:        logger,
	}
}

// Execute implements workqueue.Task.
// Performs naming pattern analysis to infer foreign key relationships.
func (t *NameInferenceTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Load all tables and columns
	tables, err := t.schemaRepo.ListTablesByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	columns, err := t.schemaRepo.ListColumnsByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}

	t.logger.Info("Starting name inference",
		zap.Int("tables", len(tables)),
		zap.Int("columns", len(columns)))

	// Build table lookup with singular/plural variants
	tableLookup := t.buildTableLookup(tables, columns)

	// Get existing candidates to avoid duplicates
	existingCandidates, err := t.candidateRepo.GetByWorkflow(tenantCtx, t.workflowID)
	if err != nil {
		return fmt.Errorf("get existing candidates: %w", err)
	}

	// Build set of existing candidate pairs
	existingPairs := make(map[string]bool)
	for _, c := range existingCandidates {
		key := fmt.Sprintf("%s-%s", c.SourceColumnID, c.TargetColumnID)
		existingPairs[key] = true
	}

	// Analyze each column for naming patterns
	candidatesCreated := 0
	for _, col := range columns {
		// Skip primary key columns (they are targets, not sources)
		if col.IsPrimaryKey {
			continue
		}

		// Skip if this column's table is not in our tables list (shouldn't happen)
		tableName := t.getTableName(col.SchemaTableID, tables)
		if tableName == "" {
			continue
		}

		// Pattern 1: {table}_id → {table}.id
		if strings.HasSuffix(col.ColumnName, "_id") {
			targetTableName := strings.TrimSuffix(col.ColumnName, "_id")
			if targetInfo, ok := tableLookup[normalizeTableName(targetTableName)]; ok {
				// Don't create self-referential relationships
				if targetInfo.tableName != tableName {
					// Check if candidate already exists
					pairKey := fmt.Sprintf("%s-%s", col.ID, targetInfo.pkColumnID)
					if !existingPairs[pairKey] {
						if err := t.createCandidate(tenantCtx, col.ID, targetInfo.pkColumnID, ConfidenceTableIDPattern); err != nil {
							t.logger.Error("Failed to create candidate for {table}_id pattern",
								zap.String("source_column", fmt.Sprintf("%s.%s", tableName, col.ColumnName)),
								zap.String("target_table", targetInfo.tableName),
								zap.Error(err))
						} else {
							candidatesCreated++
							t.logger.Debug("Created candidate for {table}_id pattern",
								zap.String("source", fmt.Sprintf("%s.%s", tableName, col.ColumnName)),
								zap.String("target", fmt.Sprintf("%s.%s", targetInfo.tableName, targetInfo.pkColumnName)))
						}
					}
				}
			}
		}

		// Pattern 2: column name matches table name (singular/plural)
		normalizedColName := normalizeTableName(col.ColumnName)
		if targetInfo, ok := tableLookup[normalizedColName]; ok {
			// Don't create self-referential relationships
			if targetInfo.tableName != tableName {
				// Check if candidate already exists
				pairKey := fmt.Sprintf("%s-%s", col.ID, targetInfo.pkColumnID)
				if !existingPairs[pairKey] {
					if err := t.createCandidate(tenantCtx, col.ID, targetInfo.pkColumnID, ConfidenceColumnNameMatch); err != nil {
						t.logger.Error("Failed to create candidate for column name match",
							zap.String("source_column", fmt.Sprintf("%s.%s", tableName, col.ColumnName)),
							zap.String("target_table", targetInfo.tableName),
							zap.Error(err))
					} else {
						candidatesCreated++
						t.logger.Debug("Created candidate for column name match",
							zap.String("source", fmt.Sprintf("%s.%s", tableName, col.ColumnName)),
							zap.String("target", fmt.Sprintf("%s.%s", targetInfo.tableName, targetInfo.pkColumnName)))
					}
				}
			}
		}
	}

	t.logger.Info("Name inference completed",
		zap.Int("candidates_created", candidatesCreated))

	return nil
}

// tablePKInfo holds table information for lookup.
type tablePKInfo struct {
	tableID      uuid.UUID
	tableName    string
	pkColumnID   uuid.UUID
	pkColumnName string
}

// buildTableLookup creates a normalized lookup map of table names to their primary key info.
// The map includes both singular and plural forms of table names.
func (t *NameInferenceTask) buildTableLookup(tables []*models.SchemaTable, columns []*models.SchemaColumn) map[string]*tablePKInfo {
	lookup := make(map[string]*tablePKInfo)

	// Build a map of table ID to columns
	columnsByTable := make(map[uuid.UUID][]*models.SchemaColumn)
	for _, col := range columns {
		columnsByTable[col.SchemaTableID] = append(columnsByTable[col.SchemaTableID], col)
	}

	for _, table := range tables {
		// Find the primary key column for this table
		cols := columnsByTable[table.ID]
		var pkColumn *models.SchemaColumn
		for _, col := range cols {
			if col.IsPrimaryKey {
				pkColumn = col
				break
			}
		}

		// Skip tables without a primary key
		if pkColumn == nil {
			t.logger.Debug("Skipping table without primary key",
				zap.String("table", table.TableName))
			continue
		}

		info := &tablePKInfo{
			tableID:      table.ID,
			tableName:    table.TableName,
			pkColumnID:   pkColumn.ID,
			pkColumnName: pkColumn.ColumnName,
		}

		// Add exact table name (normalized)
		normalized := normalizeTableName(table.TableName)
		lookup[normalized] = info

		// Add singular variant if table name is plural
		singular := singularize(normalized)
		if singular != normalized {
			// Don't overwrite if there's already an exact match
			if _, exists := lookup[singular]; !exists {
				lookup[singular] = info
			}
		}

		// Add plural variant if table name is singular
		plural := pluralize(normalized)
		if plural != normalized {
			// Don't overwrite if there's already an exact match
			if _, exists := lookup[plural]; !exists {
				lookup[plural] = info
			}
		}
	}

	return lookup
}

// getTableName returns the table name for a given table ID.
func (t *NameInferenceTask) getTableName(tableID uuid.UUID, tables []*models.SchemaTable) string {
	for _, table := range tables {
		if table.ID == tableID {
			return table.TableName
		}
	}
	return ""
}

// createCandidate creates a new relationship candidate with name_inference detection method.
func (t *NameInferenceTask) createCandidate(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID, confidence float64) error {
	candidate := &models.RelationshipCandidate{
		WorkflowID:      t.workflowID,
		DatasourceID:    t.datasourceID,
		SourceColumnID:  sourceColumnID,
		TargetColumnID:  targetColumnID,
		DetectionMethod: models.DetectionMethodNameInference,
		Confidence:      confidence,
		Status:          models.RelCandidateStatusPending,
		IsRequired:      false, // Will be set by LLM analysis task
	}

	// Store name similarity (in this case, it's implicit from the pattern match)
	// We use confidence as the similarity score
	nameSimilarity := confidence
	candidate.NameSimilarity = &nameSimilarity

	return t.candidateRepo.Create(ctx, candidate)
}

// normalizeTableName normalizes a table or column name for comparison.
// Converts to lowercase and removes common prefixes/suffixes.
func normalizeTableName(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)
	return name
}

// singularize attempts to convert a plural table name to singular.
// This is a simple implementation that handles common English pluralization rules.
// For production use, consider using a proper inflection library.
func singularize(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)

	// Common irregular plurals
	irregulars := map[string]string{
		"people":   "person",
		"children": "child",
		"men":      "man",
		"women":    "woman",
		"feet":     "foot",
		"teeth":    "tooth",
		"geese":    "goose",
		"mice":     "mouse",
		"oxen":     "ox",
		"criteria": "criterion",
		"data":     "datum",
	}
	if singular, ok := irregulars[name]; ok {
		return singular
	}

	// Handle common plural patterns
	if strings.HasSuffix(name, "ies") && len(name) > 3 {
		// companies → company
		return name[:len(name)-3] + "y"
	}
	if strings.HasSuffix(name, "ves") && len(name) > 3 {
		// wolves → wolf, knives → knife
		// Check if original was 'f' or 'fe'
		base := name[:len(name)-3]
		// Simple heuristic: if base ends in 'i', it was likely 'ife' → 'ives'
		if strings.HasSuffix(base, "i") {
			return base + "fe"
		}
		return base + "f"
	}
	if strings.HasSuffix(name, "ses") && len(name) > 3 {
		// classes → class, but NOT status → statu
		// Check if it's 'sses' (classes) vs 'tus' (status)
		if strings.HasSuffix(name, "sses") {
			return name[:len(name)-2]
		}
		// For 'xes', 'shes', 'ches', remove 'es'
		if strings.HasSuffix(name, "xes") || strings.HasSuffix(name, "shes") || strings.HasSuffix(name, "ches") {
			return name[:len(name)-2]
		}
		// Otherwise it might be a word that naturally ends in 's' (status, nexus, etc)
		// Don't singularize words ending in 'us'
		if strings.HasSuffix(name, "us") {
			return name
		}
		return name[:len(name)-2]
	}
	if strings.HasSuffix(name, "xes") && len(name) > 3 {
		// boxes → box
		return name[:len(name)-2]
	}
	if strings.HasSuffix(name, "zes") && len(name) > 3 {
		// quizzes → quiz (but be careful not to over-match)
		if len(name) > 4 && name[len(name)-4] == name[len(name)-3] {
			// Double consonant before "es"
			return name[:len(name)-3]
		}
		return name[:len(name)-2]
	}
	if strings.HasSuffix(name, "shes") || strings.HasSuffix(name, "ches") {
		// dishes → dish, matches → match
		return name[:len(name)-2]
	}
	if strings.HasSuffix(name, "s") && len(name) > 1 {
		// Default: remove trailing 's'
		// But avoid removing 's' from words that end in 'ss' or 'us'
		if strings.HasSuffix(name, "ss") || strings.HasSuffix(name, "us") {
			return name
		}
		return name[:len(name)-1]
	}

	return name
}

// pluralize attempts to convert a singular table name to plural.
// This is a simple implementation that handles common English pluralization rules.
func pluralize(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)

	// Common irregular plurals (reversed from singularize)
	irregulars := map[string]string{
		"person":    "people",
		"child":     "children",
		"man":       "men",
		"woman":     "women",
		"foot":      "feet",
		"tooth":     "teeth",
		"goose":     "geese",
		"mouse":     "mice",
		"ox":        "oxen",
		"criterion": "criteria",
		"datum":     "data",
	}
	if plural, ok := irregulars[name]; ok {
		return plural
	}

	// Handle common singular patterns
	if strings.HasSuffix(name, "y") && len(name) > 1 {
		// company → companies (but not boy → boies)
		if len(name) > 1 && !isVowel(rune(name[len(name)-2])) {
			return name[:len(name)-1] + "ies"
		}
	}
	if strings.HasSuffix(name, "f") && len(name) > 1 {
		// wolf → wolves
		return name[:len(name)-1] + "ves"
	}
	if strings.HasSuffix(name, "fe") && len(name) > 2 {
		// knife → knives
		return name[:len(name)-2] + "ves"
	}
	if strings.HasSuffix(name, "s") || strings.HasSuffix(name, "ss") ||
		strings.HasSuffix(name, "x") || strings.HasSuffix(name, "z") ||
		strings.HasSuffix(name, "ch") || strings.HasSuffix(name, "sh") {
		// class → classes, box → boxes, quiz → quizzes
		return name + "es"
	}
	if strings.HasSuffix(name, "o") && len(name) > 1 {
		// tomato → tomatoes (but not photo → photoes)
		// This is a simplification; proper handling would need a dictionary
		if !isVowel(rune(name[len(name)-2])) {
			return name + "es"
		}
	}

	// Default: add 's'
	return name + "s"
}

// isVowel checks if a rune is a vowel.
func isVowel(r rune) bool {
	vowels := "aeiouAEIOU"
	return strings.ContainsRune(vowels, r)
}

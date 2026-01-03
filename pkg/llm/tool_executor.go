package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyToolExecutor implements ToolExecutor for ontology chat and question answering.
// It provides access to schema metadata, data sampling, and ontology updates.
type OntologyToolExecutor struct {
	projectID          uuid.UUID
	datasourceID       uuid.UUID
	ontologyRepo       repositories.OntologyRepository
	knowledgeRepo      repositories.KnowledgeRepository
	schemaRepo         repositories.SchemaRepository
	ontologyEntityRepo repositories.OntologyEntityRepository
	entityRelRepo      repositories.EntityRelationshipRepository
	queryExecutor      datasource.QueryExecutor
	logger             *zap.Logger
}

// OntologyToolExecutorConfig holds dependencies for creating an OntologyToolExecutor.
type OntologyToolExecutorConfig struct {
	ProjectID          uuid.UUID
	DatasourceID       uuid.UUID
	OntologyRepo       repositories.OntologyRepository
	KnowledgeRepo      repositories.KnowledgeRepository
	SchemaRepo         repositories.SchemaRepository
	OntologyEntityRepo repositories.OntologyEntityRepository
	EntityRelRepo      repositories.EntityRelationshipRepository
	QueryExecutor      datasource.QueryExecutor
	Logger             *zap.Logger
}

// NewOntologyToolExecutor creates a new tool executor for ontology operations.
func NewOntologyToolExecutor(cfg *OntologyToolExecutorConfig) *OntologyToolExecutor {
	return &OntologyToolExecutor{
		projectID:          cfg.ProjectID,
		datasourceID:       cfg.DatasourceID,
		ontologyRepo:       cfg.OntologyRepo,
		knowledgeRepo:      cfg.KnowledgeRepo,
		schemaRepo:         cfg.SchemaRepo,
		ontologyEntityRepo: cfg.OntologyEntityRepo,
		entityRelRepo:      cfg.EntityRelRepo,
		queryExecutor:      cfg.QueryExecutor,
		logger:             cfg.Logger.Named("tool-executor"),
	}
}

// Ensure OntologyToolExecutor implements ToolExecutor.
var _ ToolExecutor = (*OntologyToolExecutor)(nil)

// ExecuteTool dispatches to the appropriate tool handler based on name.
func (e *OntologyToolExecutor) ExecuteTool(ctx context.Context, name string, arguments string) (string, error) {
	e.logger.Debug("Executing tool",
		zap.String("tool", name),
		zap.String("arguments", arguments))

	switch name {
	case "query_column_values":
		return e.queryColumnValues(ctx, arguments)
	case "query_schema_metadata":
		return e.querySchemaMetadata(ctx, arguments)
	case "store_knowledge":
		return e.storeKnowledge(ctx, arguments)
	case "update_entity":
		return e.updateEntity(ctx, arguments)
	case "update_column":
		return e.updateColumn(ctx, arguments)
	case "answer_question":
		return e.answerQuestion(ctx, arguments)
	case "get_pending_questions":
		return e.getPendingQuestions(ctx, arguments)
	case "create_domain_entity":
		return e.createDomainEntity(ctx, arguments)
	case "create_entity_relationship":
		return e.createEntityRelationship(ctx, arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// ============================================================================
// Tool: query_column_values
// ============================================================================

type queryColumnValuesArgs struct {
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	Limit      int    `json:"limit"`
}

func (e *OntologyToolExecutor) queryColumnValues(ctx context.Context, arguments string) (string, error) {
	var args queryColumnValuesArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.TableName == "" || args.ColumnName == "" {
		return "", fmt.Errorf("table_name and column_name are required")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	if e.queryExecutor == nil {
		return `{"error": "No query executor available - datasource may not be configured"}`, nil
	}

	// Build and execute the query
	// Use adapter's QuoteIdentifier for database-agnostic quoting
	quotedTable := e.queryExecutor.QuoteIdentifier(args.TableName)
	quotedCol := e.queryExecutor.QuoteIdentifier(args.ColumnName)
	query := fmt.Sprintf(
		`SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL LIMIT %d`,
		quotedCol, quotedTable, quotedCol, limit,
	)

	result, err := e.queryExecutor.ExecuteQuery(ctx, query, limit)
	if err != nil {
		e.logger.Error("Failed to query column values",
			zap.String("table", args.TableName),
			zap.String("column", args.ColumnName),
			zap.Error(err))
		return fmt.Sprintf(`{"error": "Query failed: %s"}`, err.Error()), nil
	}

	// Extract just the values
	values := make([]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		if v, ok := row[args.ColumnName]; ok {
			values = append(values, v)
		}
	}

	response := map[string]any{
		"table":   args.TableName,
		"column":  args.ColumnName,
		"values":  values,
		"count":   len(values),
		"limited": len(values) == limit,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: query_schema_metadata
// ============================================================================

type querySchemaMetadataArgs struct {
	TableName string `json:"table_name"`
}

func (e *OntologyToolExecutor) querySchemaMetadata(ctx context.Context, arguments string) (string, error) {
	var args querySchemaMetadataArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Get all tables for the datasource
	tables, err := e.schemaRepo.ListTablesByDatasource(ctx, e.projectID, e.datasourceID)
	if err != nil {
		return "", fmt.Errorf("failed to list tables: %w", err)
	}

	// If specific table requested, filter by table name
	if args.TableName != "" {
		filtered := make([]*models.SchemaTable, 0)
		for _, t := range tables {
			if t.TableName == args.TableName {
				filtered = append(filtered, t)
			}
		}
		tables = filtered
	}

	// Build response with table and column info
	type columnInfo struct {
		Name         string  `json:"name"`
		DataType     string  `json:"data_type"`
		IsPrimaryKey bool    `json:"is_primary_key"`
		IsNullable   bool    `json:"is_nullable"`
		BusinessName *string `json:"business_name,omitempty"`
		Description  *string `json:"description,omitempty"`
	}

	type tableInfo struct {
		Name         string       `json:"name"`
		RowCount     *int64       `json:"row_count,omitempty"`
		BusinessName *string      `json:"business_name,omitempty"`
		Description  *string      `json:"description,omitempty"`
		Columns      []columnInfo `json:"columns"`
	}

	result := make([]tableInfo, 0, len(tables))
	for _, t := range tables {
		info := tableInfo{
			Name:         t.TableName,
			RowCount:     t.RowCount,
			BusinessName: t.BusinessName,
			Description:  t.Description,
			Columns:      []columnInfo{},
		}

		// Get columns for this table
		columns, err := e.schemaRepo.ListColumnsByTable(ctx, e.projectID, t.ID)
		if err != nil {
			e.logger.Error("Failed to get columns for table",
				zap.String("table", t.TableName),
				zap.Error(err))
			continue
		}

		for _, c := range columns {
			info.Columns = append(info.Columns, columnInfo{
				Name:         c.ColumnName,
				DataType:     c.DataType,
				IsPrimaryKey: c.IsPrimaryKey,
				IsNullable:   c.IsNullable,
				BusinessName: c.BusinessName,
				Description:  c.Description,
			})
		}

		result = append(result, info)
	}

	responseJSON, err := json.Marshal(map[string]any{
		"tables":      result,
		"table_count": len(result),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: store_knowledge
// ============================================================================

type storeKnowledgeArgs struct {
	FactType string `json:"fact_type"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Context  string `json:"context"`
}

func (e *OntologyToolExecutor) storeKnowledge(ctx context.Context, arguments string) (string, error) {
	var args storeKnowledgeArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.FactType == "" || args.Key == "" || args.Value == "" {
		return "", fmt.Errorf("fact_type, key, and value are required")
	}

	// Validate fact type
	validTypes := map[string]bool{
		"terminology":       true,
		"business_rule":     true,
		"data_relationship": true,
		"constraint":        true,
		"context":           true,
	}
	if !validTypes[args.FactType] {
		return "", fmt.Errorf("invalid fact_type: %s", args.FactType)
	}

	fact := &models.KnowledgeFact{
		ProjectID: e.projectID,
		FactType:  args.FactType,
		Key:       args.Key,
		Value:     args.Value,
		Context:   args.Context,
	}

	if err := e.knowledgeRepo.Upsert(ctx, fact); err != nil {
		return "", fmt.Errorf("failed to store knowledge: %w", err)
	}

	e.logger.Info("Stored knowledge fact",
		zap.String("fact_type", args.FactType),
		zap.String("key", args.Key))

	response := map[string]any{
		"success":   true,
		"fact_id":   fact.ID.String(),
		"fact_type": args.FactType,
		"key":       args.Key,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: update_entity
// ============================================================================

type updateEntityArgs struct {
	TableName    string   `json:"table_name"`
	BusinessName string   `json:"business_name"`
	Description  string   `json:"description"`
	Domain       string   `json:"domain"`
	Synonyms     []string `json:"synonyms"`
}

func (e *OntologyToolExecutor) updateEntity(ctx context.Context, arguments string) (string, error) {
	var args updateEntityArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.TableName == "" {
		return "", fmt.Errorf("table_name is required")
	}

	// Get the active ontology
	ontology, err := e.ontologyRepo.GetActive(ctx, e.projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get ontology: %w", err)
	}
	if ontology == nil {
		return `{"error": "No active ontology found"}`, nil
	}

	// Get or create entity summary
	summary := ontology.EntitySummaries[args.TableName]
	if summary == nil {
		summary = &models.EntitySummary{
			TableName: args.TableName,
		}
	}

	// Apply updates
	if args.BusinessName != "" {
		summary.BusinessName = args.BusinessName
	}
	if args.Description != "" {
		summary.Description = args.Description
	}
	if args.Domain != "" {
		summary.Domain = args.Domain
	}
	if len(args.Synonyms) > 0 {
		summary.Synonyms = args.Synonyms
	}

	// Save the update
	if err := e.ontologyRepo.UpdateEntitySummary(ctx, e.projectID, args.TableName, summary); err != nil {
		return "", fmt.Errorf("failed to update entity: %w", err)
	}

	e.logger.Info("Updated entity",
		zap.String("table", args.TableName),
		zap.String("business_name", summary.BusinessName))

	response := map[string]any{
		"success":       true,
		"table_name":    args.TableName,
		"business_name": summary.BusinessName,
		"description":   summary.Description,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: update_column
// ============================================================================

type updateColumnArgs struct {
	TableName    string `json:"table_name"`
	ColumnName   string `json:"column_name"`
	BusinessName string `json:"business_name"`
	Description  string `json:"description"`
	SemanticType string `json:"semantic_type"`
}

func (e *OntologyToolExecutor) updateColumn(ctx context.Context, arguments string) (string, error) {
	var args updateColumnArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.TableName == "" || args.ColumnName == "" {
		return "", fmt.Errorf("table_name and column_name are required")
	}

	// Validate semantic type if provided
	if args.SemanticType != "" {
		validTypes := map[string]bool{
			"identifier": true, "name": true, "description": true,
			"amount": true, "quantity": true, "date": true,
			"timestamp": true, "status": true, "flag": true,
			"code": true, "reference": true, "other": true,
		}
		if !validTypes[args.SemanticType] {
			return "", fmt.Errorf("invalid semantic_type: %s", args.SemanticType)
		}
	}

	// Get the active ontology
	ontology, err := e.ontologyRepo.GetActive(ctx, e.projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get ontology: %w", err)
	}
	if ontology == nil {
		return `{"error": "No active ontology found"}`, nil
	}

	// Get or create column details for the table
	columns := ontology.ColumnDetails[args.TableName]
	if columns == nil {
		columns = []models.ColumnDetail{}
	}

	// Find or create the column detail
	found := false
	for i := range columns {
		if columns[i].Name == args.ColumnName {
			if args.Description != "" {
				columns[i].Description = args.Description
			}
			if args.SemanticType != "" {
				columns[i].SemanticType = args.SemanticType
			}
			// BusinessName maps to synonyms in this model
			if args.BusinessName != "" {
				columns[i].Synonyms = append(columns[i].Synonyms, args.BusinessName)
			}
			found = true
			break
		}
	}

	if !found {
		synonyms := []string{}
		if args.BusinessName != "" {
			synonyms = append(synonyms, args.BusinessName)
		}
		columns = append(columns, models.ColumnDetail{
			Name:         args.ColumnName,
			Description:  args.Description,
			SemanticType: args.SemanticType,
			Synonyms:     synonyms,
		})
	}

	// Save the update
	if err := e.ontologyRepo.UpdateColumnDetails(ctx, e.projectID, args.TableName, columns); err != nil {
		return "", fmt.Errorf("failed to update column: %w", err)
	}

	e.logger.Info("Updated column",
		zap.String("table", args.TableName),
		zap.String("column", args.ColumnName))

	response := map[string]any{
		"success":     true,
		"table_name":  args.TableName,
		"column_name": args.ColumnName,
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: answer_question
// ============================================================================

type answerQuestionArgs struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

func (e *OntologyToolExecutor) answerQuestion(ctx context.Context, arguments string) (string, error) {
	var args answerQuestionArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.QuestionID == "" || args.Answer == "" {
		return "", fmt.Errorf("question_id and answer are required")
	}

	// Note: The old workflow state system has been replaced by the DAG-based workflow.
	// This tool is deprecated and returns a not-found response.
	e.logger.Info("answerQuestion tool called (deprecated)",
		zap.String("question_id", args.QuestionID))

	return `{"error": "Question not found - workflow state system has been replaced by DAG-based workflow"}`, nil
}

// ============================================================================
// Tool: get_pending_questions
// ============================================================================

type getPendingQuestionsArgs struct {
	Limit int `json:"limit"`
}

func (e *OntologyToolExecutor) getPendingQuestions(ctx context.Context, arguments string) (string, error) {
	// Note: The old workflow state system has been replaced by the DAG-based workflow.
	// This tool is deprecated and returns an empty list.
	e.logger.Info("getPendingQuestions tool called (deprecated)")

	response := map[string]any{
		"questions":     []any{},
		"count":         0,
		"total_pending": 0,
		"message":       "Workflow state system has been replaced by DAG-based workflow",
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: create_domain_entity
// ============================================================================

type createDomainEntityArgs struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	PrimaryTable  string `json:"primary_table"`
	PrimaryColumn string `json:"primary_column"`
}

func (e *OntologyToolExecutor) createDomainEntity(ctx context.Context, arguments string) (string, error) {
	var args createDomainEntityArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Name == "" || args.Description == "" || args.PrimaryTable == "" || args.PrimaryColumn == "" {
		return "", fmt.Errorf("name, description, primary_table, and primary_column are required")
	}

	// Check if entity repository is available
	if e.ontologyEntityRepo == nil {
		return `{"error": "Entity creation not available - repository not configured"}`, nil
	}

	// Get the active ontology to get ontologyID
	ontology, err := e.ontologyRepo.GetActive(ctx, e.projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get ontology: %w", err)
	}
	if ontology == nil {
		return `{"error": "No active ontology found"}`, nil
	}

	// Create the domain entity
	entity := &models.OntologyEntity{
		OntologyID:    ontology.ID,
		Name:          args.Name,
		Description:   args.Description,
		PrimarySchema: "public", // Default schema
		PrimaryTable:  args.PrimaryTable,
		PrimaryColumn: args.PrimaryColumn,
	}

	if err := e.ontologyEntityRepo.Create(ctx, entity); err != nil {
		// Check if it's a duplicate
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return fmt.Sprintf(`{"error": "Entity '%s' already exists"}`, args.Name), nil
		}
		return "", fmt.Errorf("failed to create entity: %w", err)
	}

	e.logger.Info("Created domain entity via chat",
		zap.String("entity_name", args.Name),
		zap.String("primary_table", args.PrimaryTable))

	response := map[string]any{
		"success":        true,
		"entity_id":      entity.ID.String(),
		"name":           entity.Name,
		"description":    entity.Description,
		"primary_table":  entity.PrimaryTable,
		"primary_column": entity.PrimaryColumn,
		"message":        fmt.Sprintf("Created domain entity '%s'", args.Name),
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

// ============================================================================
// Tool: create_entity_relationship
// ============================================================================

type createEntityRelationshipArgs struct {
	SourceEntity string `json:"source_entity"`
	TargetEntity string `json:"target_entity"`
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column"`
	Description  string `json:"description"`
}

func (e *OntologyToolExecutor) createEntityRelationship(ctx context.Context, arguments string) (string, error) {
	var args createEntityRelationshipArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.SourceEntity == "" || args.TargetEntity == "" {
		return "", fmt.Errorf("source_entity and target_entity are required")
	}
	if args.SourceTable == "" || args.SourceColumn == "" || args.TargetTable == "" || args.TargetColumn == "" {
		return "", fmt.Errorf("source_table, source_column, target_table, and target_column are required")
	}

	// Check if repositories are available
	if e.ontologyEntityRepo == nil || e.entityRelRepo == nil {
		return `{"error": "Relationship creation not available - repositories not configured"}`, nil
	}

	// Get the active ontology
	ontology, err := e.ontologyRepo.GetActive(ctx, e.projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get ontology: %w", err)
	}
	if ontology == nil {
		return `{"error": "No active ontology found"}`, nil
	}

	// Find the source entity by name
	sourceEntity, err := e.ontologyEntityRepo.GetByName(ctx, ontology.ID, args.SourceEntity)
	if err != nil {
		return "", fmt.Errorf("failed to find source entity: %w", err)
	}
	if sourceEntity == nil {
		return fmt.Sprintf(`{"error": "Source entity '%s' not found. Create it first using create_domain_entity."}`, args.SourceEntity), nil
	}

	// Find the target entity by name
	targetEntity, err := e.ontologyEntityRepo.GetByName(ctx, ontology.ID, args.TargetEntity)
	if err != nil {
		return "", fmt.Errorf("failed to find target entity: %w", err)
	}
	if targetEntity == nil {
		return fmt.Sprintf(`{"error": "Target entity '%s' not found. Create it first using create_domain_entity."}`, args.TargetEntity), nil
	}

	// Create the relationship
	var description *string
	if args.Description != "" {
		description = &args.Description
	}

	relationship := &models.EntityRelationship{
		OntologyID:         ontology.ID,
		SourceEntityID:     sourceEntity.ID,
		TargetEntityID:     targetEntity.ID,
		SourceColumnSchema: "public", // Default schema
		SourceColumnTable:  args.SourceTable,
		SourceColumnName:   args.SourceColumn,
		TargetColumnSchema: "public", // Default schema
		TargetColumnTable:  args.TargetTable,
		TargetColumnName:   args.TargetColumn,
		DetectionMethod:    models.DetectionMethodManual,
		Confidence:         1.0, // User-created through chat
		Status:             models.RelationshipStatusConfirmed,
		Description:        description,
	}

	if err := e.entityRelRepo.Create(ctx, relationship); err != nil {
		// Check if it's a duplicate
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return fmt.Sprintf(`{"error": "Relationship between '%s' and '%s' already exists for this column"}`, args.SourceEntity, args.TargetEntity), nil
		}
		return "", fmt.Errorf("failed to create relationship: %w", err)
	}

	e.logger.Info("Created entity relationship via chat",
		zap.String("source_entity", args.SourceEntity),
		zap.String("target_entity", args.TargetEntity),
		zap.String("source_column", fmt.Sprintf("%s.%s", args.SourceTable, args.SourceColumn)),
		zap.String("target_column", fmt.Sprintf("%s.%s", args.TargetTable, args.TargetColumn)))

	response := map[string]any{
		"success":         true,
		"relationship_id": relationship.ID.String(),
		"source_entity":   args.SourceEntity,
		"target_entity":   args.TargetEntity,
		"source_column":   fmt.Sprintf("%s.%s", args.SourceTable, args.SourceColumn),
		"target_column":   fmt.Sprintf("%s.%s", args.TargetTable, args.TargetColumn),
		"message":         fmt.Sprintf("Created relationship: %s -> %s", args.SourceEntity, args.TargetEntity),
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(responseJSON), nil
}

package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyToolExecutor implements ToolExecutor for ontology chat and question answering.
// It provides access to schema metadata, data sampling, and ontology updates.
type OntologyToolExecutor struct {
	projectID     uuid.UUID
	ontologyID    uuid.UUID
	datasourceID  uuid.UUID
	ontologyRepo  repositories.OntologyRepository
	knowledgeRepo repositories.KnowledgeRepository
	schemaRepo    repositories.SchemaRepository
	queryExecutor datasource.QueryExecutor
	logger        *zap.Logger
}

// OntologyToolExecutorConfig holds dependencies for creating an OntologyToolExecutor.
type OntologyToolExecutorConfig struct {
	ProjectID     uuid.UUID
	OntologyID    uuid.UUID
	DatasourceID  uuid.UUID
	OntologyRepo  repositories.OntologyRepository
	KnowledgeRepo repositories.KnowledgeRepository
	SchemaRepo    repositories.SchemaRepository
	QueryExecutor datasource.QueryExecutor
	Logger        *zap.Logger
}

// NewOntologyToolExecutor creates a new tool executor for ontology operations.
func NewOntologyToolExecutor(cfg *OntologyToolExecutorConfig) *OntologyToolExecutor {
	return &OntologyToolExecutor{
		projectID:     cfg.ProjectID,
		ontologyID:    cfg.OntologyID,
		datasourceID:  cfg.DatasourceID,
		ontologyRepo:  cfg.OntologyRepo,
		knowledgeRepo: cfg.KnowledgeRepo,
		schemaRepo:    cfg.SchemaRepo,
		queryExecutor: cfg.QueryExecutor,
		logger:        cfg.Logger.Named("tool-executor"),
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
	case "update_column":
		return e.updateColumn(ctx, arguments)
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
		`SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL`,
		quotedCol, quotedTable, quotedCol,
	)

	// Adapter handles dialect-specific limit (LIMIT for PostgreSQL, TOP for SQL Server)
	result, err := e.queryExecutor.Query(ctx, query, limit)
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

	// Get selected tables for the datasource
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
		Name         string `json:"name"`
		DataType     string `json:"data_type"`
		IsPrimaryKey bool   `json:"is_primary_key"`
		IsNullable   bool   `json:"is_nullable"`
	}

	type tableInfo struct {
		Name     string       `json:"name"`
		RowCount *int64       `json:"row_count,omitempty"`
		Columns  []columnInfo `json:"columns"`
	}

	result := make([]tableInfo, 0, len(tables))
	for _, t := range tables {
		info := tableInfo{
			Name:     t.TableName,
			RowCount: t.RowCount,
			Columns:  []columnInfo{},
		}

		// Get columns for this table
		columns, err := e.schemaRepo.ListColumnsByTable(ctx, e.projectID, t.ID, false)
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
	Value    string `json:"value"`
	Context  string `json:"context"`
}

func (e *OntologyToolExecutor) storeKnowledge(ctx context.Context, arguments string) (string, error) {
	var args storeKnowledgeArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if args.FactType == "" || args.Value == "" {
		return "", fmt.Errorf("fact_type and value are required")
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
		Value:     args.Value,
		Context:   args.Context,
	}

	if err := e.knowledgeRepo.Create(ctx, fact); err != nil {
		return "", fmt.Errorf("failed to store knowledge: %w", err)
	}

	e.logger.Info("Stored knowledge fact",
		zap.String("fact_type", args.FactType))

	response := map[string]any{
		"success":   true,
		"fact_id":   fact.ID.String(),
		"fact_type": args.FactType,
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

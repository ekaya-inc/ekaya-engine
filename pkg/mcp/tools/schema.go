package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// SchemaToolDeps contains dependencies for schema context tools.
type SchemaToolDeps struct {
	BaseMCPToolDeps
	ProjectService services.ProjectService
	SchemaService  services.SchemaService
}

// SchemaToolNames lists all tools in the schema group.
var SchemaToolNames = map[string]bool{
	"get_schema": true,
}

// RegisterSchemaTools registers tools for schema context with entity semantics.
func RegisterSchemaTools(s *server.MCPServer, deps *SchemaToolDeps) {
	registerGetSchemaContextTool(s, deps)
}

// registerGetSchemaContextTool exposes database schema as structured JSON.
func registerGetSchemaContextTool(s *server.MCPServer, deps *SchemaToolDeps) {
	tool := mcp.NewTool(
		"get_schema",
		mcp.WithDescription(
			"Get database schema with tables, columns, types, primary keys, and relationships. "+
				"Returns structured JSON with entity/role semantic information for intelligent query generation. "+
				"For example, visits.host_id represents entity 'user' with role 'host' vs visits.visitor_id as 'user' with role 'visitor'. "+
				"Use this to understand the semantic meaning of foreign keys and generate accurate SQL joins.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_schema")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get default datasource
		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}

		// Get selected-only schema
		schema, err := deps.SchemaService.GetSelectedDatasourceSchema(tenantCtx, projectID, dsID)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema: %w", err)
		}

		// Build structured response
		type columnInfo struct {
			Name         string `json:"name"`
			DataType     string `json:"data_type"`
			IsPrimaryKey bool   `json:"is_primary_key"`
			IsNullable   bool   `json:"is_nullable"`
			OrdinalPos   int    `json:"ordinal_position"`
		}
		type tableInfo struct {
			Schema   string       `json:"schema"`
			Name     string       `json:"name"`
			RowCount int64        `json:"row_count"`
			Columns  []columnInfo `json:"columns"`
		}
		type relationshipInfo struct {
			SourceTable  string `json:"source_table"`
			SourceColumn string `json:"source_column"`
			TargetTable  string `json:"target_table"`
			TargetColumn string `json:"target_column"`
			Cardinality  string `json:"cardinality,omitempty"`
		}

		tables := make([]tableInfo, 0, len(schema.Tables))
		for _, t := range schema.Tables {
			columns := make([]columnInfo, 0, len(t.Columns))
			for _, c := range t.Columns {
				columns = append(columns, columnInfo{
					Name:         c.ColumnName,
					DataType:     c.DataType,
					IsPrimaryKey: c.IsPrimaryKey,
					IsNullable:   c.IsNullable,
					OrdinalPos:   c.OrdinalPosition,
				})
			}
			tables = append(tables, tableInfo{
				Schema:   t.SchemaName,
				Name:     t.TableName,
				RowCount: t.RowCount,
				Columns:  columns,
			})
		}

		relationships := make([]relationshipInfo, 0, len(schema.Relationships))
		for _, r := range schema.Relationships {
			rel := relationshipInfo{
				SourceTable:  r.SourceTableName,
				SourceColumn: r.SourceColumnName,
				TargetTable:  r.TargetTableName,
				TargetColumn: r.TargetColumnName,
			}
			if r.Cardinality != "" && r.Cardinality != "unknown" {
				rel.Cardinality = r.Cardinality
			}
			relationships = append(relationships, rel)
		}

		response := struct {
			Tables        []tableInfo        `json:"tables"`
			Relationships []relationshipInfo `json:"relationships"`
			TableCount    int                `json:"table_count"`
			ProjectID     string             `json:"project_id"`
			DatasourceID  string             `json:"datasource_id"`
		}{
			Tables:        tables,
			Relationships: relationships,
			TableCount:    len(tables),
			ProjectID:     projectID.String(),
			DatasourceID:  dsID.String(),
		}

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// getOptionalBool extracts an optional boolean parameter from the request.
func getOptionalBool(req mcp.CallToolRequest, key string) (bool, bool) {
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if val, ok := args[key].(bool); ok {
			return val, true
		}
	}
	return false, false
}

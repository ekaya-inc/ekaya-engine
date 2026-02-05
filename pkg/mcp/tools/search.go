// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// SearchToolDeps contains dependencies for search tools.
type SearchToolDeps struct {
	BaseMCPToolDeps
	SchemaRepo repositories.SchemaRepository
}

// RegisterSearchTools registers search MCP tools.
func RegisterSearchTools(s *server.MCPServer, deps *SearchToolDeps) {
	registerSearchSchemaTool(s, deps)
}

// registerSearchSchemaTool adds the search_schema tool for full-text search of tables/columns/entities.
func registerSearchSchemaTool(s *server.MCPServer, deps *SearchToolDeps) {
	tool := mcp.NewTool(
		"search_schema",
		mcp.WithDescription(
			"Full-text search across tables and columns using pattern matching. "+
				"Returns matching items ranked by relevance. Searches table names, column names, "+
				"and semantic descriptions from ontology metadata. "+
				"Example: search_schema(query='user') returns all tables and columns "+
				"related to users, ordered by relevance.",
		),
		mcp.WithString(
			"query",
			mcp.Required(),
			mcp.Description("Search query text (e.g., 'user', 'transaction', 'billing')"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Maximum number of results to return (default 20, max 100)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "search_schema")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get required query parameter
		query, err := req.RequireString("query")
		if err != nil {
			return nil, err
		}

		// Trim and validate query
		query = strings.TrimSpace(query)
		if query == "" {
			return NewErrorResult("invalid_parameters", "query parameter cannot be empty"), nil
		}

		// Get optional limit parameter
		limit := 20 // default
		if limitVal, ok := req.Params.Arguments.(map[string]any)["limit"]; ok {
			if limitFloat, ok := limitVal.(float64); ok {
				limit = int(limitFloat)
			}
		}

		// Enforce max limit
		if limit > 100 {
			limit = 100
		}

		// Search schema
		result, err := searchSchema(tenantCtx, deps, projectID, query, limit)
		if err != nil {
			return nil, err
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// searchResult contains the aggregated search results.
type searchResult struct {
	Query      string        `json:"query"`
	Tables     []tableMatch  `json:"tables"`
	Columns    []columnMatch `json:"columns"`
	TotalCount int           `json:"total_count"`
}

// tableMatch represents a matched table.
type tableMatch struct {
	SchemaName  string  `json:"schema_name"`
	TableName   string  `json:"table_name"`
	TableType   *string `json:"table_type,omitempty"`
	Description *string `json:"description,omitempty"`
	RowCount    *int64  `json:"row_count,omitempty"`
	MatchType   string  `json:"match_type"` // "table_name", "description"
	Relevance   float64 `json:"relevance"`
}

// columnMatch represents a matched column.
type columnMatch struct {
	SchemaName  string  `json:"schema_name"`
	TableName   string  `json:"table_name"`
	ColumnName  string  `json:"column_name"`
	DataType    string  `json:"data_type"`
	Purpose     *string `json:"purpose,omitempty"`
	Description *string `json:"description,omitempty"`
	MatchType   string  `json:"match_type"` // "column_name", "description"
	Relevance   float64 `json:"relevance"`
}

// searchSchema performs the full-text search across schema.
func searchSchema(ctx context.Context, deps *SearchToolDeps, projectID uuid.UUID, query string, limit int) (*searchResult, error) {
	result := &searchResult{
		Query:   query,
		Tables:  []tableMatch{},
		Columns: []columnMatch{},
	}

	// Normalize query for case-insensitive matching
	normalizedQuery := strings.ToLower(query)

	// Search tables
	tables, err := searchTables(ctx, deps, projectID, normalizedQuery, limit)
	if err != nil {
		deps.Logger.Warn("Failed to search tables",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	} else {
		result.Tables = tables
	}

	// Search columns
	columns, err := searchColumns(ctx, deps, projectID, normalizedQuery, limit)
	if err != nil {
		deps.Logger.Warn("Failed to search columns",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	} else {
		result.Columns = columns
	}

	result.TotalCount = len(result.Tables) + len(result.Columns)

	return result, nil
}

// searchTables searches for matching tables.
// Searches table_name in engine_schema_tables and description in engine_ontology_table_metadata.
func searchTables(ctx context.Context, _ *SearchToolDeps, projectID uuid.UUID, query string, limit int) ([]tableMatch, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant scope not found in context")
	}

	// Query to search tables using ILIKE for pattern matching
	// Join with ontology table metadata to search descriptions
	q := `
		SELECT DISTINCT
			st.schema_name,
			st.table_name,
			tm.table_type,
			tm.description,
			st.row_count,
			CASE
				WHEN LOWER(st.table_name) = $1 THEN 1.0
				WHEN LOWER(st.table_name) LIKE $1 || '%' THEN 0.9
				WHEN LOWER(tm.description) LIKE '%' || $1 || '%' THEN 0.6
				ELSE 0.5
			END as relevance
		FROM engine_schema_tables st
		LEFT JOIN engine_ontology_table_metadata tm ON tm.schema_table_id = st.id
		WHERE st.project_id = $2
			AND st.deleted_at IS NULL
			AND (
				LOWER(st.table_name) LIKE '%' || $1 || '%'
				OR LOWER(tm.description) LIKE '%' || $1 || '%'
			)
		ORDER BY relevance DESC, st.table_name ASC
		LIMIT $3
	`

	rows, err := scope.Conn.Query(ctx, q, query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search tables: %w", err)
	}
	defer rows.Close()

	var matches []tableMatch
	for rows.Next() {
		var m tableMatch
		if err := rows.Scan(
			&m.SchemaName,
			&m.TableName,
			&m.TableType,
			&m.Description,
			&m.RowCount,
			&m.Relevance,
		); err != nil {
			return nil, fmt.Errorf("failed to scan table match: %w", err)
		}

		// Determine match type
		if strings.Contains(strings.ToLower(m.TableName), query) {
			m.MatchType = "table_name"
		} else if m.Description != nil && strings.Contains(strings.ToLower(*m.Description), query) {
			m.MatchType = "description"
		}

		matches = append(matches, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table matches: %w", err)
	}

	return matches, nil
}

// searchColumns searches for matching columns.
// Searches column_name in engine_schema_columns and description/purpose in engine_ontology_column_metadata.
func searchColumns(ctx context.Context, _ *SearchToolDeps, projectID uuid.UUID, query string, limit int) ([]columnMatch, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant scope not found in context")
	}

	// Query to search columns using ILIKE for pattern matching
	// Join with ontology column metadata to search descriptions and purpose
	q := `
		SELECT DISTINCT
			st.schema_name,
			st.table_name,
			sc.column_name,
			sc.data_type,
			cm.purpose,
			cm.description,
			CASE
				WHEN LOWER(sc.column_name) = $1 THEN 1.0
				WHEN LOWER(sc.column_name) LIKE $1 || '%' THEN 0.9
				WHEN LOWER(cm.purpose) LIKE '%' || $1 || '%' THEN 0.7
				WHEN LOWER(cm.description) LIKE '%' || $1 || '%' THEN 0.6
				ELSE 0.5
			END as relevance
		FROM engine_schema_columns sc
		JOIN engine_schema_tables st ON st.id = sc.schema_table_id
		LEFT JOIN engine_ontology_column_metadata cm ON cm.schema_column_id = sc.id
		WHERE sc.project_id = $2
			AND sc.deleted_at IS NULL
			AND st.deleted_at IS NULL
			AND (
				LOWER(sc.column_name) LIKE '%' || $1 || '%'
				OR LOWER(cm.purpose) LIKE '%' || $1 || '%'
				OR LOWER(cm.description) LIKE '%' || $1 || '%'
			)
		ORDER BY relevance DESC, st.table_name ASC, sc.column_name ASC
		LIMIT $3
	`

	rows, err := scope.Conn.Query(ctx, q, query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search columns: %w", err)
	}
	defer rows.Close()

	var matches []columnMatch
	for rows.Next() {
		var m columnMatch
		if err := rows.Scan(
			&m.SchemaName,
			&m.TableName,
			&m.ColumnName,
			&m.DataType,
			&m.Purpose,
			&m.Description,
			&m.Relevance,
		); err != nil {
			return nil, fmt.Errorf("failed to scan column match: %w", err)
		}

		// Determine match type
		if strings.Contains(strings.ToLower(m.ColumnName), query) {
			m.MatchType = "column_name"
		} else if m.Purpose != nil && strings.Contains(strings.ToLower(*m.Purpose), query) {
			m.MatchType = "purpose"
		} else if m.Description != nil && strings.Contains(strings.ToLower(*m.Description), query) {
			m.MatchType = "description"
		}

		matches = append(matches, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating column matches: %w", err)
	}

	return matches, nil
}

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

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// SearchToolDeps contains dependencies for search tools.
type SearchToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	SchemaRepo       repositories.SchemaRepository
	OntologyRepo     repositories.OntologyRepository
	EntityRepo       repositories.OntologyEntityRepository
	Logger           *zap.Logger
}

// RegisterSearchTools registers search MCP tools.
func RegisterSearchTools(s *server.MCPServer, deps *SearchToolDeps) {
	registerSearchSchemaTool(s, deps)
}

// checkSearchToolEnabled verifies a specific search tool is enabled for the project.
func checkSearchToolEnabled(ctx context.Context, deps *SearchToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
	// Get claims from context
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return uuid.Nil, nil, nil, fmt.Errorf("authentication required")
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("invalid project ID: %w", err)
	}

	// Acquire connection with tenant scope
	scope, err := deps.DB.WithTenant(ctx, projectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}

	// Set tenant context for the query
	tenantCtx := database.SetTenantScope(ctx, scope)

	// Check if caller is an agent (API key authentication)
	isAgent := claims.Subject == "agent"

	// Get tool groups state and check access using the unified checker
	state, err := deps.MCPConfigService.GetToolGroupsState(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to get tool groups state",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	// Use the unified ToolAccessChecker for consistent access decisions
	checker := services.NewToolAccessChecker()
	if checker.IsToolAccessible(toolName, state, isAgent) {
		return projectID, tenantCtx, func() { scope.Close() }, nil
	}

	scope.Close()
	return uuid.Nil, nil, nil, fmt.Errorf("%s tool is not enabled for this project", toolName)
}

// registerSearchSchemaTool adds the search_schema tool for full-text search of tables/columns/entities.
func registerSearchSchemaTool(s *server.MCPServer, deps *SearchToolDeps) {
	tool := mcp.NewTool(
		"search_schema",
		mcp.WithDescription(
			"Full-text search across tables, columns, and entities using pattern matching. "+
				"Returns matching items ranked by relevance. Searches table names, column names, "+
				"business names, descriptions, and entity names/aliases. "+
				"Example: search_schema(query='user') returns all tables, columns, and entities "+
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
		projectID, tenantCtx, cleanup, err := checkSearchToolEnabled(ctx, deps, "search_schema")
		if err != nil {
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
			return nil, fmt.Errorf("query parameter cannot be empty")
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
	Entities   []entityMatch `json:"entities"`
	TotalCount int           `json:"total_count"`
}

// tableMatch represents a matched table.
type tableMatch struct {
	SchemaName   string  `json:"schema_name"`
	TableName    string  `json:"table_name"`
	BusinessName *string `json:"business_name,omitempty"`
	Description  *string `json:"description,omitempty"`
	RowCount     *int64  `json:"row_count,omitempty"`
	MatchType    string  `json:"match_type"` // "table_name", "business_name", "description"
	Relevance    float64 `json:"relevance"`
}

// columnMatch represents a matched column.
type columnMatch struct {
	SchemaName   string  `json:"schema_name"`
	TableName    string  `json:"table_name"`
	ColumnName   string  `json:"column_name"`
	DataType     string  `json:"data_type"`
	BusinessName *string `json:"business_name,omitempty"`
	Description  *string `json:"description,omitempty"`
	MatchType    string  `json:"match_type"` // "column_name", "business_name", "description"
	Relevance    float64 `json:"relevance"`
}

// entityMatch represents a matched entity.
type entityMatch struct {
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	PrimaryTable string   `json:"primary_table"`
	Domain       *string  `json:"domain,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	MatchType    string   `json:"match_type"` // "name", "alias", "description"
	Relevance    float64  `json:"relevance"`
}

// searchSchema performs the full-text search across schema and ontology.
func searchSchema(ctx context.Context, deps *SearchToolDeps, projectID uuid.UUID, query string, limit int) (*searchResult, error) {
	result := &searchResult{
		Query:    query,
		Tables:   []tableMatch{},
		Columns:  []columnMatch{},
		Entities: []entityMatch{},
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

	// Search entities (if ontology exists)
	entities, err := searchEntities(ctx, deps, projectID, normalizedQuery, limit)
	if err != nil {
		// It's normal for ontology to not exist yet
		deps.Logger.Debug("Failed to search entities (ontology may not exist)",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	} else {
		result.Entities = entities
	}

	result.TotalCount = len(result.Tables) + len(result.Columns) + len(result.Entities)

	return result, nil
}

// searchTables searches for matching tables.
func searchTables(ctx context.Context, deps *SearchToolDeps, projectID uuid.UUID, query string, limit int) ([]tableMatch, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant scope not found in context")
	}

	// Query to search tables using ILIKE for pattern matching
	q := `
		SELECT DISTINCT
			st.schema_name,
			st.table_name,
			st.business_name,
			st.description,
			st.row_count,
			CASE
				WHEN LOWER(st.table_name) = $1 THEN 1.0
				WHEN LOWER(st.table_name) LIKE $1 || '%' THEN 0.9
				WHEN LOWER(st.business_name) = $1 THEN 0.95
				WHEN LOWER(st.business_name) LIKE '%' || $1 || '%' THEN 0.8
				WHEN LOWER(st.description) LIKE '%' || $1 || '%' THEN 0.6
				ELSE 0.5
			END as relevance
		FROM engine_schema_tables st
		WHERE st.project_id = $2
			AND st.deleted_at IS NULL
			AND (
				LOWER(st.table_name) LIKE '%' || $1 || '%'
				OR LOWER(st.business_name) LIKE '%' || $1 || '%'
				OR LOWER(st.description) LIKE '%' || $1 || '%'
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
			&m.BusinessName,
			&m.Description,
			&m.RowCount,
			&m.Relevance,
		); err != nil {
			return nil, fmt.Errorf("failed to scan table match: %w", err)
		}

		// Determine match type
		if strings.Contains(strings.ToLower(m.TableName), query) {
			m.MatchType = "table_name"
		} else if m.BusinessName != nil && strings.Contains(strings.ToLower(*m.BusinessName), query) {
			m.MatchType = "business_name"
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
func searchColumns(ctx context.Context, deps *SearchToolDeps, projectID uuid.UUID, query string, limit int) ([]columnMatch, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant scope not found in context")
	}

	// Query to search columns using ILIKE for pattern matching
	q := `
		SELECT DISTINCT
			st.schema_name,
			st.table_name,
			sc.column_name,
			sc.data_type,
			sc.business_name,
			sc.description,
			CASE
				WHEN LOWER(sc.column_name) = $1 THEN 1.0
				WHEN LOWER(sc.column_name) LIKE $1 || '%' THEN 0.9
				WHEN LOWER(sc.business_name) = $1 THEN 0.95
				WHEN LOWER(sc.business_name) LIKE '%' || $1 || '%' THEN 0.8
				WHEN LOWER(sc.description) LIKE '%' || $1 || '%' THEN 0.6
				ELSE 0.5
			END as relevance
		FROM engine_schema_columns sc
		JOIN engine_schema_tables st ON st.id = sc.schema_table_id
		WHERE sc.project_id = $2
			AND sc.deleted_at IS NULL
			AND st.deleted_at IS NULL
			AND (
				LOWER(sc.column_name) LIKE '%' || $1 || '%'
				OR LOWER(sc.business_name) LIKE '%' || $1 || '%'
				OR LOWER(sc.description) LIKE '%' || $1 || '%'
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
			&m.BusinessName,
			&m.Description,
			&m.Relevance,
		); err != nil {
			return nil, fmt.Errorf("failed to scan column match: %w", err)
		}

		// Determine match type
		if strings.Contains(strings.ToLower(m.ColumnName), query) {
			m.MatchType = "column_name"
		} else if m.BusinessName != nil && strings.Contains(strings.ToLower(*m.BusinessName), query) {
			m.MatchType = "business_name"
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

// searchEntities searches for matching entities in the active ontology.
func searchEntities(ctx context.Context, deps *SearchToolDeps, projectID uuid.UUID, query string, limit int) ([]entityMatch, error) {
	// Get active ontology
	ontology, err := deps.OntologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return []entityMatch{}, nil // No ontology yet
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("tenant scope not found in context")
	}

	// Query to search entities and aliases using ILIKE for pattern matching
	q := `
		SELECT DISTINCT
			e.name,
			e.description,
			e.primary_table,
			e.domain,
			COALESCE(
				(SELECT array_agg(a.alias)
				 FROM engine_ontology_entity_aliases a
				 WHERE a.entity_id = e.id AND a.deleted_at IS NULL),
				ARRAY[]::text[]
			) as aliases,
			CASE
				WHEN LOWER(e.name) = $1 THEN 1.0
				WHEN LOWER(e.name) LIKE $1 || '%' THEN 0.9
				WHEN EXISTS (
					SELECT 1 FROM engine_ontology_entity_aliases a
					WHERE a.entity_id = e.id AND LOWER(a.alias) = $1
				) THEN 0.95
				WHEN EXISTS (
					SELECT 1 FROM engine_ontology_entity_aliases a
					WHERE a.entity_id = e.id AND LOWER(a.alias) LIKE '%' || $1 || '%'
				) THEN 0.8
				WHEN LOWER(e.description) LIKE '%' || $1 || '%' THEN 0.6
				ELSE 0.5
			END as relevance
		FROM engine_ontology_entities e
		WHERE e.project_id = $2
			AND e.ontology_id = $3
			AND e.is_deleted = false
			AND (
				LOWER(e.name) LIKE '%' || $1 || '%'
				OR LOWER(e.description) LIKE '%' || $1 || '%'
				OR EXISTS (
					SELECT 1 FROM engine_ontology_entity_aliases a
					WHERE a.entity_id = e.id AND LOWER(a.alias) LIKE '%' || $1 || '%'
				)
			)
		ORDER BY relevance DESC, e.name ASC
		LIMIT $4
	`

	rows, err := scope.Conn.Query(ctx, q, query, projectID, ontology.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search entities: %w", err)
	}
	defer rows.Close()

	var matches []entityMatch
	for rows.Next() {
		var m entityMatch
		if err := rows.Scan(
			&m.Name,
			&m.Description,
			&m.PrimaryTable,
			&m.Domain,
			&m.Aliases,
			&m.Relevance,
		); err != nil {
			return nil, fmt.Errorf("failed to scan entity match: %w", err)
		}

		// Determine match type
		if strings.Contains(strings.ToLower(m.Name), query) {
			m.MatchType = "name"
		} else if m.Description != nil && strings.Contains(strings.ToLower(*m.Description), query) {
			m.MatchType = "description"
		} else {
			// Check if any alias matches
			for _, alias := range m.Aliases {
				if strings.Contains(strings.ToLower(alias), query) {
					m.MatchType = "alias"
					break
				}
			}
		}

		matches = append(matches, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity matches: %w", err)
	}

	return matches, nil
}

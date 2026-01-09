// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// GlossaryToolDeps contains dependencies for glossary tools.
type GlossaryToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	GlossaryService  services.GlossaryService
	Logger           *zap.Logger
}

// GetDB implements ToolAccessDeps.
func (d *GlossaryToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *GlossaryToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *GlossaryToolDeps) GetLogger() *zap.Logger { return d.Logger }

// RegisterGlossaryTools registers glossary-related MCP tools.
func RegisterGlossaryTools(s *server.MCPServer, deps *GlossaryToolDeps) {
	registerListGlossaryTool(s, deps)
	registerGetGlossarySQLTool(s, deps)
	registerUpdateGlossaryTermTool(s, deps)
	registerDeleteGlossaryTermTool(s, deps)
}

// registerListGlossaryTool adds the list_glossary tool for discovering available business terms.
// Returns lightweight list of terms with definitions and aliases for discovery.
func registerListGlossaryTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"list_glossary",
		mcp.WithDescription(
			"List all business glossary terms for the project. "+
				"Returns term names, definitions, and aliases for discovery. "+
				"Use this to explore available business terms like 'Revenue', 'Active User', 'GMV', etc. "+
				"To get the SQL definition for a specific term, use get_glossary_sql.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "list_glossary")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get all glossary terms for the project
		terms, err := deps.GlossaryService.GetTerms(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get glossary terms: %w", err)
		}

		// Build lightweight response for discovery
		result := struct {
			Terms []listGlossaryResponse `json:"terms"`
			Count int                    `json:"count"`
		}{
			Terms: make([]listGlossaryResponse, 0, len(terms)),
			Count: len(terms),
		}

		for _, term := range terms {
			result.Terms = append(result.Terms, toListGlossaryResponse(term))
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerGetGlossarySQLTool adds the get_glossary_sql tool for retrieving SQL definitions.
// Accepts term name or alias and returns full entry with defining_sql for query composition.
func registerGetGlossarySQLTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"get_glossary_sql",
		mcp.WithDescription(
			"Get the SQL definition for a specific business term. "+
				"Accepts term name or alias (e.g., 'Revenue' or 'Total Revenue'). "+
				"Returns the complete SQL definition, output columns, and metadata needed for query composition. "+
				"Use list_glossary first to discover available terms.",
		),
		mcp.WithString(
			"term",
			mcp.Required(),
			mcp.Description("Business term name or alias to look up"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_glossary_sql")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get term parameter
		termName, err := req.RequireString("term")
		if err != nil {
			return nil, err
		}

		// Look up term by name or alias
		term, err := deps.GlossaryService.GetTermByName(tenantCtx, projectID, termName)
		if err != nil {
			return nil, fmt.Errorf("failed to get glossary term: %w", err)
		}

		// Handle not found case gracefully
		if term == nil {
			notFoundResult := struct {
				Error string `json:"error"`
				Term  string `json:"term"`
			}{
				Error: "Term not found",
				Term:  termName,
			}

			jsonResult, err := json.Marshal(notFoundResult)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}

			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Build full response with SQL definition
		result := toGetGlossarySQLResponse(term)

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// listGlossaryResponse is the lightweight response format for list_glossary tool.
// Used for discovery - only includes term, definition, and aliases.
type listGlossaryResponse struct {
	Term       string   `json:"term"`
	Definition string   `json:"definition"`
	Aliases    []string `json:"aliases,omitempty"`
}

// getGlossarySQLResponse is the full response format for get_glossary_sql tool.
// Includes all fields needed for query composition.
type getGlossarySQLResponse struct {
	Term          string                `json:"term"`
	Definition    string                `json:"definition"`
	DefiningSQL   string                `json:"defining_sql"`
	BaseTable     string                `json:"base_table,omitempty"`
	OutputColumns []models.OutputColumn `json:"output_columns,omitempty"`
	Aliases       []string              `json:"aliases,omitempty"`
}

// toListGlossaryResponse converts a model to list_glossary response format.
func toListGlossaryResponse(term *models.BusinessGlossaryTerm) listGlossaryResponse {
	return listGlossaryResponse{
		Term:       term.Term,
		Definition: term.Definition,
		Aliases:    term.Aliases,
	}
}

// toGetGlossarySQLResponse converts a model to get_glossary_sql response format.
func toGetGlossarySQLResponse(term *models.BusinessGlossaryTerm) getGlossarySQLResponse {
	return getGlossarySQLResponse{
		Term:          term.Term,
		Definition:    term.Definition,
		DefiningSQL:   term.DefiningSQL,
		BaseTable:     term.BaseTable,
		OutputColumns: term.OutputColumns,
		Aliases:       term.Aliases,
	}
}

// registerUpdateGlossaryTermTool adds the update_glossary_term tool for creating/updating business terms.
// Uses upsert semantics: creates new term if not found, updates existing term if found.
// The term name is the upsert key.
func registerUpdateGlossaryTermTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"update_glossary_term",
		mcp.WithDescription(
			"Create or update a business glossary term with upsert semantics. "+
				"If a term with the given name exists, it will be updated. Otherwise, a new term will be created. "+
				"The term name is the upsert key. All parameters except 'term' are optional. "+
				"Use this to add business definitions that AI agents discover during analysis.",
		),
		mcp.WithString(
			"term",
			mcp.Required(),
			mcp.Description("Business term name (upsert key)"),
		),
		mcp.WithString(
			"definition",
			mcp.Description("What the term means in business context"),
		),
		mcp.WithString(
			"sql",
			mcp.Description("SQL pattern to calculate the term (defining_sql)"),
		),
		mcp.WithArray(
			"aliases",
			mcp.Description("Alternative names for the term (e.g., 'AOV', 'Average Order Value')"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_glossary_term")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required term parameter
		termName, err := req.RequireString("term")
		if err != nil {
			return nil, err
		}

		// Get optional parameters
		definition := getOptionalString(req, "definition")
		sql := getOptionalString(req, "sql")
		var aliases []string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if aliasesRaw, ok := args["aliases"]; ok && aliasesRaw != nil {
				if aliasArray, ok := aliasesRaw.([]interface{}); ok {
					for _, alias := range aliasArray {
						if aliasStr, ok := alias.(string); ok {
							aliases = append(aliases, aliasStr)
						}
					}
				}
			}
		}

		// Check if term already exists
		existing, err := deps.GlossaryService.GetTermByName(tenantCtx, projectID, termName)
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing term: %w", err)
		}

		var created bool
		var term *models.BusinessGlossaryTerm

		if existing == nil {
			// Create new term
			if definition == "" {
				return nil, fmt.Errorf("definition is required when creating a new term")
			}
			if sql == "" {
				return nil, fmt.Errorf("sql is required when creating a new term")
			}

			term = &models.BusinessGlossaryTerm{
				ProjectID:   projectID,
				Term:        termName,
				Definition:  definition,
				DefiningSQL: sql,
				Aliases:     aliases,
				Source:      models.GlossarySourceClient, // Mark as client-created
			}

			if err := deps.GlossaryService.CreateTerm(tenantCtx, projectID, term); err != nil {
				deps.Logger.Error("Failed to create glossary term",
					zap.String("project_id", projectID.String()),
					zap.String("term", termName),
					zap.Error(err))
				return nil, fmt.Errorf("failed to create term: %w", err)
			}

			created = true
			deps.Logger.Info("Created glossary term via MCP",
				zap.String("project_id", projectID.String()),
				zap.String("term", termName))
		} else {
			// Update existing term
			term = existing

			// Update fields if provided (nil means keep existing)
			if definition != "" {
				term.Definition = definition
			}
			if sql != "" {
				term.DefiningSQL = sql
			}
			if aliases != nil {
				term.Aliases = aliases
			}

			// Update source to client if it was previously inferred
			if term.Source == models.GlossarySourceInferred {
				term.Source = models.GlossarySourceClient
			}

			if err := deps.GlossaryService.UpdateTerm(tenantCtx, term); err != nil {
				deps.Logger.Error("Failed to update glossary term",
					zap.String("project_id", projectID.String()),
					zap.String("term", termName),
					zap.Error(err))
				return nil, fmt.Errorf("failed to update term: %w", err)
			}

			created = false
			deps.Logger.Info("Updated glossary term via MCP",
				zap.String("project_id", projectID.String()),
				zap.String("term", termName))
		}

		// Build response
		response := struct {
			Term          string                `json:"term"`
			Definition    string                `json:"definition"`
			SQL           string                `json:"sql"`
			Aliases       []string              `json:"aliases,omitempty"`
			OutputColumns []models.OutputColumn `json:"output_columns,omitempty"`
			Created       bool                  `json:"created"`
		}{
			Term:          term.Term,
			Definition:    term.Definition,
			SQL:           term.DefiningSQL,
			Aliases:       term.Aliases,
			OutputColumns: term.OutputColumns,
			Created:       created,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerDeleteGlossaryTermTool adds the delete_glossary_term tool for removing business terms.
func registerDeleteGlossaryTermTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"delete_glossary_term",
		mcp.WithDescription(
			"Delete a business glossary term by name. "+
				"This permanently removes the term and its aliases. "+
				"Use this to remove terms that are no longer relevant or were added incorrectly.",
		),
		mcp.WithString(
			"term",
			mcp.Required(),
			mcp.Description("Business term name to delete"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_glossary_term")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required term parameter
		termName, err := req.RequireString("term")
		if err != nil {
			return nil, err
		}

		// Check if term exists
		term, err := deps.GlossaryService.GetTermByName(tenantCtx, projectID, termName)
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing term: %w", err)
		}

		if term == nil {
			// Term doesn't exist - idempotent success
			response := struct {
				Term    string `json:"term"`
				Deleted bool   `json:"deleted"`
			}{
				Term:    termName,
				Deleted: false,
			}

			jsonResult, err := json.Marshal(response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}

			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Delete the term
		if err := deps.GlossaryService.DeleteTerm(tenantCtx, term.ID); err != nil {
			deps.Logger.Error("Failed to delete glossary term",
				zap.String("project_id", projectID.String()),
				zap.String("term", termName),
				zap.Error(err))
			return nil, fmt.Errorf("failed to delete term: %w", err)
		}

		deps.Logger.Info("Deleted glossary term via MCP",
			zap.String("project_id", projectID.String()),
			zap.String("term", termName))

		// Build response
		response := struct {
			Term    string `json:"term"`
			Deleted bool   `json:"deleted"`
		}{
			Term:    termName,
			Deleted: true,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// GlossaryToolDeps contains dependencies for glossary tools.
type GlossaryToolDeps struct {
	BaseMCPToolDeps
	GlossaryService services.GlossaryService
}

// RegisterGlossaryTools registers glossary-related MCP tools.
func RegisterGlossaryTools(s *server.MCPServer, deps *GlossaryToolDeps) {
	registerListGlossaryTool(s, deps)
	registerGetGlossarySQLTool(s, deps)
	registerCreateGlossaryTermTool(s, deps)
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
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get term parameter
		termName, err := req.RequireString("term")
		if err != nil {
			return nil, err
		}
		// Validate term is not empty after trimming whitespace
		termName = trimString(termName)
		if termName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'term' cannot be empty"), nil
		}

		// Look up term by name or alias
		term, err := deps.GlossaryService.GetTermByName(tenantCtx, projectID, termName)
		if err != nil {
			return nil, fmt.Errorf("failed to get glossary term: %w", err)
		}

		// Handle not found case with error result
		if term == nil {
			return NewErrorResult("TERM_NOT_FOUND",
				fmt.Sprintf("term %q not found in glossary. Use list_glossary to see available terms.", termName)), nil
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
// Used for discovery - includes term, definition, aliases, and enrichment status.
type listGlossaryResponse struct {
	Term             string   `json:"term"`
	Definition       string   `json:"definition"`
	Aliases          []string `json:"aliases,omitempty"`
	EnrichmentStatus string   `json:"enrichment_status,omitempty"` // "pending", "success", "failed"
	EnrichmentError  string   `json:"enrichment_error,omitempty"`  // Error message if enrichment failed
}

// getGlossarySQLResponse is the full response format for get_glossary_sql tool.
// Includes all fields needed for query composition plus enrichment status.
type getGlossarySQLResponse struct {
	Term             string                `json:"term"`
	Definition       string                `json:"definition"`
	DefiningSQL      string                `json:"defining_sql"`
	BaseTable        string                `json:"base_table,omitempty"`
	OutputColumns    []models.OutputColumn `json:"output_columns,omitempty"`
	Aliases          []string              `json:"aliases,omitempty"`
	EnrichmentStatus string                `json:"enrichment_status,omitempty"` // "pending", "success", "failed"
	EnrichmentError  string                `json:"enrichment_error,omitempty"`  // Error message if enrichment failed
}

// toListGlossaryResponse converts a model to list_glossary response format.
func toListGlossaryResponse(term *models.BusinessGlossaryTerm) listGlossaryResponse {
	return listGlossaryResponse{
		Term:             term.Term,
		Definition:       term.Definition,
		Aliases:          term.Aliases,
		EnrichmentStatus: term.EnrichmentStatus,
		EnrichmentError:  term.EnrichmentError,
	}
}

// toGetGlossarySQLResponse converts a model to get_glossary_sql response format.
func toGetGlossarySQLResponse(term *models.BusinessGlossaryTerm) getGlossarySQLResponse {
	return getGlossarySQLResponse{
		Term:             term.Term,
		Definition:       term.Definition,
		DefiningSQL:      term.DefiningSQL,
		BaseTable:        term.BaseTable,
		OutputColumns:    term.OutputColumns,
		Aliases:          term.Aliases,
		EnrichmentStatus: term.EnrichmentStatus,
		EnrichmentError:  term.EnrichmentError,
	}
}

// registerCreateGlossaryTermTool adds the create_glossary_term tool for creating new business terms.
// Unlike update_glossary_term (which uses upsert semantics), this tool explicitly creates a new term
// and will fail if a term with the same name already exists.
func registerCreateGlossaryTermTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"create_glossary_term",
		mcp.WithDescription(
			"Create a new business glossary term. "+
				"If SQL is provided, it will be validated before saving. "+
				"Use this to add new business metrics like 'Revenue', 'Active Users', etc.",
		),
		mcp.WithString("term", mcp.Required(),
			mcp.Description("The business term name (e.g., 'Daily Active Users')")),
		mcp.WithString("definition", mcp.Required(),
			mcp.Description("Human-readable description of what this term means")),
		mcp.WithString("defining_sql",
			mcp.Description("SQL query that calculates this metric (optional — not all terms have direct SQL)")),
		mcp.WithString("base_table",
			mcp.Description("Primary table this term is derived from (optional)")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "create_glossary_term")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		term, _ := req.RequireString("term")
		definition, _ := req.RequireString("definition")
		definingSQL := getOptionalString(req, "defining_sql")
		baseTable := getOptionalString(req, "base_table")

		// Validate term is not empty after trimming whitespace
		term = trimString(term)
		if term == "" {
			return NewErrorResult("invalid_parameters", "parameter 'term' cannot be empty"), nil
		}

		// Reject test-like term names to prevent test data in glossary
		if services.IsTestTerm(term) {
			return NewErrorResult("invalid_parameters",
				"term name appears to be test data - use a real business term"), nil
		}

		glossaryTerm := &models.BusinessGlossaryTerm{
			ProjectID:   projectID,
			Term:        term,
			Definition:  definition,
			DefiningSQL: definingSQL,
			BaseTable:   baseTable,
			Source:      models.GlossarySourceMCP,
		}

		err = deps.GlossaryService.CreateTerm(tenantCtx, projectID, glossaryTerm)
		if err != nil {
			return NewErrorResult("create_failed", err.Error()), nil
		}

		deps.Logger.Info("Created glossary term via MCP",
			zap.String("project_id", projectID.String()),
			zap.String("term", term))

		// Return the created term
		response := struct {
			Success bool `json:"success"`
			Term    struct {
				ID         string `json:"id"`
				Term       string `json:"term"`
				Definition string `json:"definition"`
			} `json:"term"`
		}{
			Success: true,
			Term: struct {
				ID         string `json:"id"`
				Term       string `json:"term"`
				Definition string `json:"definition"`
			}{
				ID:         glossaryTerm.ID.String(),
				Term:       glossaryTerm.Term,
				Definition: glossaryTerm.Definition,
			},
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
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
			mcp.WithStringItems(),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_glossary_term")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get required term parameter
		termName, err := req.RequireString("term")
		if err != nil {
			return nil, err
		}
		// Validate term is not empty after trimming whitespace
		termName = trimString(termName)
		if termName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'term' cannot be empty"), nil
		}

		// Reject test-like term names to prevent test data in glossary
		if services.IsTestTerm(termName) {
			return NewErrorResult("invalid_parameters",
				"term name appears to be test data - use a real business term"), nil
		}

		// Get optional parameters
		definition := getOptionalString(req, "definition")
		sql := getOptionalString(req, "sql")

		// Extract and validate aliases array
		var aliases []string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			aliasSlice, aliasErr := extractStringSlice(args, "aliases", deps.Logger)
			if aliasErr != nil {
				return NewErrorResult("invalid_parameters", aliasErr.Error()), nil
			}
			if aliasSlice != nil {
				aliases = aliasSlice
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
				return NewErrorResult("missing_required",
					"definition is required when creating a new term"), nil
			}

			term = &models.BusinessGlossaryTerm{
				ProjectID:   projectID,
				Term:        termName,
				Definition:  definition,
				DefiningSQL: sql,
				Aliases:     aliases,
				Source:      models.GlossarySourceMCP, // Mark as MCP-created
			}

			if err := deps.GlossaryService.CreateTerm(tenantCtx, projectID, term); err != nil {
				// Use DEBUG for input errors (SQL validation, etc.), ERROR for server errors
				if IsInputError(err) {
					deps.Logger.Debug("Failed to create glossary term (input error)",
						zap.String("project_id", projectID.String()),
						zap.String("term", termName),
						zap.String("error", err.Error()))
				} else {
					deps.Logger.Error("Failed to create glossary term",
						zap.String("project_id", projectID.String()),
						zap.String("term", termName),
						zap.Error(err))
				}
				return nil, fmt.Errorf("failed to create term: %w", err)
			}

			created = true
			deps.Logger.Info("Created glossary term via MCP",
				zap.String("project_id", projectID.String()),
				zap.String("term", termName))
		} else {
			// Update existing term
			// Check precedence: can MCP modify this term?
			if !canModifyGlossaryTerm(existing.Source, models.GlossarySourceMCP) {
				return NewErrorResult("precedence_blocked",
					fmt.Sprintf("Cannot modify glossary term: precedence blocked (existing: %s, modifier: %s). "+
						"Manual terms cannot be overridden by MCP. Use the UI to modify or delete this term.",
						existing.Source, models.GlossarySourceMCP)), nil
			}

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

			// Update source to MCP if it was previously inferred
			if term.Source == models.GlossarySourceInferred {
				term.Source = models.GlossarySourceMCP
			}

			if err := deps.GlossaryService.UpdateTerm(tenantCtx, term); err != nil {
				// Use DEBUG for input errors (SQL validation, etc.), ERROR for server errors
				if IsInputError(err) {
					deps.Logger.Debug("Failed to update glossary term (input error)",
						zap.String("project_id", projectID.String()),
						zap.String("term", termName),
						zap.String("error", err.Error()))
				} else {
					deps.Logger.Error("Failed to update glossary term",
						zap.String("project_id", projectID.String()),
						zap.String("term", termName),
						zap.Error(err))
				}
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

// canModifyGlossaryTerm checks if a source can modify a glossary term based on precedence.
// Precedence hierarchy: Manual (3) > MCP (2) > Inferred (1)
// Returns true if the modification is allowed, false if blocked by higher precedence.
func canModifyGlossaryTerm(termSource string, modifierSource string) bool {
	modifierLevel := precedenceLevelGlossary(modifierSource)
	existingLevel := precedenceLevelGlossary(termSource)

	// Modifier can change if their level is >= existing level
	return modifierLevel >= existingLevel
}

// precedenceLevelGlossary returns the numeric precedence level for a glossary source.
// Higher number = higher precedence.
func precedenceLevelGlossary(source string) int {
	switch source {
	case models.GlossarySourceManual:
		return 3
	case models.GlossarySourceMCP:
		return 2
	case models.GlossarySourceInferred:
		return 1
	default:
		return 0 // Unknown source has lowest precedence
	}
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
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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

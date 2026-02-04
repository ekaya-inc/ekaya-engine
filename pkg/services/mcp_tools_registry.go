package services

import "github.com/ekaya-inc/ekaya-engine/pkg/models"

// ToolDefinition describes an MCP tool for display purposes.
type ToolDefinition struct {
	Name        string // Tool name as exposed via MCP (e.g., "get_schema")
	Description string // One-line description for UI display
	ToolGroup   string // "user", "developer", "agent", or "always"
	SubOption   string // Optional sub-option requirement (e.g., "enableExecute")
}

// ToolGroupDeveloper is the developer tools group identifier.
const ToolGroupDeveloper = "developer"

// ToolGroupAlways is the identifier for tools that are always available.
const ToolGroupAlways = "always"

// ToolGroupAgent is the identifier for agent-only tools.
const ToolGroupAgent = "agent"

// ToolRegistry contains all MCP tool definitions.
// This is the single source of truth for tool metadata, used by both
// the UI display (GetEnabledTools) and the MCP tool filter (pkg/mcp/tools).
var ToolRegistry = []ToolDefinition{
	// Developer tools - all tools are always available when developer is enabled
	{Name: "echo", Description: "Echo back input message for testing", ToolGroup: ToolGroupDeveloper},
	{Name: "execute", Description: "Execute DDL/DML statements", ToolGroup: ToolGroupDeveloper},
	{Name: "get_schema", Description: "Get database schema with semantic annotations", ToolGroup: ToolGroupDeveloper},
	{Name: "probe_column", Description: "Deep-dive into specific column with statistics, joinability, and semantic information", ToolGroup: ToolGroupDeveloper},
	{Name: "probe_columns", Description: "Batch variant of probe_column for analyzing multiple columns at once", ToolGroup: ToolGroupDeveloper},
	{Name: "get_column_metadata", Description: "Get current ontology metadata for a column (description, enum_values, role, schema info)", ToolGroup: ToolGroupDeveloper},
	{Name: "update_project_knowledge", Description: "Create or update domain facts (terminology, business rules, enumerations, conventions)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_project_knowledge", Description: "Remove incorrect or outdated domain facts", ToolGroup: ToolGroupDeveloper},
	{Name: "update_column", Description: "Add or update semantic information about a column (description, enum_values, role)", ToolGroup: ToolGroupDeveloper},
	{Name: "update_columns", Description: "Batch update metadata for multiple columns (up to 50) in a single transaction", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_column_metadata", Description: "Clear custom metadata for a column, reverting to schema-only information", ToolGroup: ToolGroupDeveloper},
	{Name: "update_table", Description: "Add or update table-level metadata (description, usage notes, ephemeral status, alternatives)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_table_metadata", Description: "Clear custom metadata for a table, removing semantic enrichment", ToolGroup: ToolGroupDeveloper},
	{Name: "list_ontology_questions", Description: "List ontology questions with filtering by status, category, priority, and pagination", ToolGroup: ToolGroupDeveloper},
	{Name: "resolve_ontology_question", Description: "Mark an ontology question as resolved after researching and updating the ontology", ToolGroup: ToolGroupDeveloper},
	{Name: "skip_ontology_question", Description: "Mark a question as skipped for revisiting later (e.g., 'Need access to frontend repo')", ToolGroup: ToolGroupDeveloper},
	{Name: "escalate_ontology_question", Description: "Mark a question as requiring human domain knowledge (e.g., 'Business rule not documented in code')", ToolGroup: ToolGroupDeveloper},
	{Name: "dismiss_ontology_question", Description: "Mark a question as not worth pursuing (e.g., 'Column appears unused, legacy')", ToolGroup: ToolGroupDeveloper},
	{Name: "search_schema", Description: "Full-text search across tables and columns using pattern matching with relevance ranking", ToolGroup: ToolGroupDeveloper},
	{Name: "explain_query", Description: "Analyze SQL query performance using EXPLAIN ANALYZE with execution plan and optimization hints", ToolGroup: ToolGroupDeveloper},
	{Name: "refresh_schema", Description: "Refresh schema from datasource and auto-select new tables/columns", ToolGroup: ToolGroupDeveloper},
	{Name: "scan_data_changes", Description: "Scan data for changes like new enum values and potential FK patterns", ToolGroup: ToolGroupDeveloper},
	{Name: "list_pending_changes", Description: "List pending ontology changes detected from schema or data analysis", ToolGroup: ToolGroupDeveloper},
	{Name: "approve_change", Description: "Approve a pending ontology change and apply it with precedence rules", ToolGroup: ToolGroupDeveloper},
	{Name: "reject_change", Description: "Reject a pending ontology change without applying it", ToolGroup: ToolGroupDeveloper},
	{Name: "approve_all_changes", Description: "Approve all pending changes that can be applied (respects precedence)", ToolGroup: ToolGroupDeveloper},

	// Business user tools (user group)
	// These read-only query tools enable business users to answer ad-hoc questions
	// when pre-approved queries don't match their request.
	{Name: "query", Description: "Execute read-only SQL SELECT statements", ToolGroup: ToolGroupUser},
	{Name: "sample", Description: "Quick data preview from a table", ToolGroup: ToolGroupUser},
	{Name: "validate", Description: "Check SQL syntax without executing", ToolGroup: ToolGroupUser},
	{Name: "get_context", Description: "Get unified database context with progressive depth (consolidates ontology, schema, glossary)", ToolGroup: ToolGroupUser},
	{Name: "get_ontology", Description: "Get business ontology for query generation", ToolGroup: ToolGroupUser},
	{Name: "list_glossary", Description: "List all business glossary terms", ToolGroup: ToolGroupUser},
	{Name: "get_glossary_sql", Description: "Get SQL definition for a business term", ToolGroup: ToolGroupUser},
	{Name: "update_glossary_term", Description: "Create or update a business glossary term with upsert semantics (definition, sql, aliases)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_glossary_term", Description: "Delete a business glossary term that's no longer relevant", ToolGroup: ToolGroupDeveloper},
	{Name: "list_approved_queries", Description: "List pre-approved SQL queries", ToolGroup: ToolGroupUser},
	{Name: "execute_approved_query", Description: "Execute a pre-approved query by ID", ToolGroup: ToolGroupUser},
	{Name: "suggest_approved_query", Description: "Suggest a reusable parameterized query for approval", ToolGroup: ToolGroupUser},
	{Name: "suggest_query_update", Description: "Suggest an update to an existing pre-approved query for review", ToolGroup: ToolGroupUser},
	{Name: "get_query_history", Description: "Get recent query execution history to avoid rewriting queries", ToolGroup: ToolGroupUser},

	// Dev query tools (developer group) - direct query management for administrators
	{Name: "list_query_suggestions", Description: "List pending query suggestions awaiting review", ToolGroup: ToolGroupDeveloper},
	{Name: "approve_query_suggestion", Description: "Approve a pending query suggestion", ToolGroup: ToolGroupDeveloper},
	{Name: "reject_query_suggestion", Description: "Reject a pending query suggestion with reason", ToolGroup: ToolGroupDeveloper},
	{Name: "create_approved_query", Description: "Create a new pre-approved query directly (no review required)", ToolGroup: ToolGroupDeveloper},
	{Name: "update_approved_query", Description: "Update an existing pre-approved query directly (no review required)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_approved_query", Description: "Delete a pre-approved query", ToolGroup: ToolGroupDeveloper},

	// Health is always available
	{Name: "health", Description: "Server health check", ToolGroup: ToolGroupAlways},
}

// DataLiaisonTools lists tools that require the AI Data Liaison app to be installed.
// These tools enable the suggest/approve query workflow between business users and developers.
// Used by both MCP tool filtering and UI enabled tools display.
var DataLiaisonTools = map[string]bool{
	// Business User tools - suggest queries for approval
	"suggest_approved_query": true,
	"suggest_query_update":   true,
	// Developer tools - manage query suggestions
	"list_query_suggestions":   true,
	"approve_query_suggestion": true,
	"reject_query_suggestion":  true,
	"create_approved_query":    true,
	"update_approved_query":    true,
	"delete_approved_query":    true,
}

// GetEnabledTools returns the list of tools enabled based on the current state.
// This computes which tools would be visible to a user (not an agent) based on
// the tool group configurations.
// Delegates to ToolAccessChecker to ensure consistency with tool execution checks.
func GetEnabledTools(state map[string]*models.ToolGroupConfig) []ToolDefinition {
	checker := NewToolAccessChecker()
	return checker.GetAccessibleTools(state, false) // false = user auth, not agent
}

// GetEnabledToolsForAgent returns the list of tools enabled for agent authentication.
// This is separate from GetEnabledTools because agents have different access rules.
func GetEnabledToolsForAgent(state map[string]*models.ToolGroupConfig) []ToolDefinition {
	checker := NewToolAccessChecker()
	return checker.GetAccessibleTools(state, true) // true = agent auth
}

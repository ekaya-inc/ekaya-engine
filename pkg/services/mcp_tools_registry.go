package services

import "github.com/ekaya-inc/ekaya-engine/pkg/models"

// ToolDefinition describes an MCP tool for display purposes.
type ToolDefinition struct {
	Name        string // Tool name as exposed via MCP (e.g., "get_schema")
	Description string // One-line description for UI display
	ToolGroup   string // "developer", "approved_queries", "agent_tools", or "always"
	SubOption   string // Optional sub-option requirement (e.g., "enableExecute")
}

// ToolGroupDeveloper is the developer tools group identifier.
const ToolGroupDeveloper = "developer"

// ToolRegistry contains all MCP tool definitions.
// This is the single source of truth for tool metadata, used by both
// the UI display (GetEnabledTools) and the MCP tool filter (pkg/mcp/tools).
var ToolRegistry = []ToolDefinition{
	// Developer tools - all tools are always available when developer is enabled
	{Name: "echo", Description: "Echo back input message for testing", ToolGroup: ToolGroupDeveloper},
	{Name: "execute", Description: "Execute DDL/DML statements", ToolGroup: ToolGroupDeveloper},
	{Name: "get_schema", Description: "Get database schema with entity semantics", ToolGroup: ToolGroupDeveloper},
	{Name: "get_entity", Description: "Retrieve full entity details including aliases, key columns, occurrences, and relationships", ToolGroup: ToolGroupDeveloper},
	{Name: "update_entity", Description: "Create or update entity metadata with upsert semantics (description, aliases, key_columns)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_entity", Description: "Remove an entity that was incorrectly identified (soft delete)", ToolGroup: ToolGroupDeveloper},
	{Name: "update_relationship", Description: "Create or update relationship between entities with upsert semantics (description, label, cardinality)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_relationship", Description: "Remove a relationship that doesn't exist or was incorrectly identified", ToolGroup: ToolGroupDeveloper},
	{Name: "probe_column", Description: "Deep-dive into specific column with statistics, joinability, and semantic information", ToolGroup: ToolGroupDeveloper},
	{Name: "probe_columns", Description: "Batch variant of probe_column for analyzing multiple columns at once", ToolGroup: ToolGroupDeveloper},
	{Name: "probe_relationship", Description: "Deep-dive into relationships between entities with cardinality and data quality metrics", ToolGroup: ToolGroupDeveloper},
	{Name: "update_project_knowledge", Description: "Create or update domain facts (terminology, business rules, enumerations, conventions)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_project_knowledge", Description: "Remove incorrect or outdated domain facts", ToolGroup: ToolGroupDeveloper},
	{Name: "update_column", Description: "Add or update semantic information about a column (description, enum_values, entity, role)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_column_metadata", Description: "Clear custom metadata for a column, reverting to schema-only information", ToolGroup: ToolGroupDeveloper},
	{Name: "list_ontology_questions", Description: "List ontology questions with filtering by status, category, entity, priority, and pagination", ToolGroup: ToolGroupDeveloper},
	{Name: "resolve_ontology_question", Description: "Mark an ontology question as resolved after researching and updating the ontology", ToolGroup: ToolGroupDeveloper},
	{Name: "skip_ontology_question", Description: "Mark a question as skipped for revisiting later (e.g., 'Need access to frontend repo')", ToolGroup: ToolGroupDeveloper},
	{Name: "escalate_ontology_question", Description: "Mark a question as requiring human domain knowledge (e.g., 'Business rule not documented in code')", ToolGroup: ToolGroupDeveloper},
	{Name: "dismiss_ontology_question", Description: "Mark a question as not worth pursuing (e.g., 'Column appears unused, legacy')", ToolGroup: ToolGroupDeveloper},
	{Name: "search_schema", Description: "Full-text search across tables, columns, and entities using pattern matching with relevance ranking", ToolGroup: ToolGroupDeveloper},
	{Name: "explain_query", Description: "Analyze SQL query performance using EXPLAIN ANALYZE with execution plan and optimization hints", ToolGroup: ToolGroupDeveloper},

	// Business user tools (approved_queries group)
	// These read-only query tools enable business users to answer ad-hoc questions
	// when pre-approved queries don't match their request.
	{Name: "query", Description: "Execute read-only SQL SELECT statements", ToolGroup: ToolGroupApprovedQueries},
	{Name: "sample", Description: "Quick data preview from a table", ToolGroup: ToolGroupApprovedQueries},
	{Name: "validate", Description: "Check SQL syntax without executing", ToolGroup: ToolGroupApprovedQueries},
	{Name: "get_context", Description: "Get unified database context with progressive depth (consolidates ontology, schema, glossary)", ToolGroup: ToolGroupApprovedQueries},
	{Name: "get_ontology", Description: "Get business ontology for query generation", ToolGroup: ToolGroupApprovedQueries},
	{Name: "list_glossary", Description: "List all business glossary terms", ToolGroup: ToolGroupApprovedQueries},
	{Name: "get_glossary_sql", Description: "Get SQL definition for a business term", ToolGroup: ToolGroupApprovedQueries},
	{Name: "update_glossary_term", Description: "Create or update a business glossary term with upsert semantics (definition, sql, aliases)", ToolGroup: ToolGroupDeveloper},
	{Name: "delete_glossary_term", Description: "Delete a business glossary term that's no longer relevant", ToolGroup: ToolGroupDeveloper},
	{Name: "list_approved_queries", Description: "List pre-approved SQL queries", ToolGroup: ToolGroupApprovedQueries},
	{Name: "execute_approved_query", Description: "Execute a pre-approved query by ID", ToolGroup: ToolGroupApprovedQueries},
	{Name: "suggest_approved_query", Description: "Suggest a reusable parameterized query for approval", ToolGroup: ToolGroupApprovedQueries},
	{Name: "get_query_history", Description: "Get recent query execution history to avoid rewriting queries", ToolGroup: ToolGroupApprovedQueries},

	// Health is always available
	{Name: "health", Description: "Server health check", ToolGroup: "always"},
}

// agentAllowedTools defines which tools are available when agent_tools is enabled.
// When agent_tools mode is active, only these specific tools are exposed.
var agentAllowedTools = map[string]bool{
	"echo":                   true,
	"list_approved_queries":  true,
	"execute_approved_query": true,
	"health":                 true,
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

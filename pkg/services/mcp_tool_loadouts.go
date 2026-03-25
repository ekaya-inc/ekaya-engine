package services

import "github.com/ekaya-inc/ekaya-engine/pkg/models"

// ToolSpec defines a tool with its name and description.
// Tools are defined once here and referenced by loadouts.
type ToolSpec struct {
	Name        string
	Description string
}

// Loadout names - use these constants when referencing loadouts
const (
	LoadoutDefault             = "default"
	LoadoutDeveloperCore       = "developer_core"
	LoadoutLimitedQuery        = "limited_query"
	LoadoutQuery               = "query"
	LoadoutOntologyMaintenance = "ontology_maintenance"
	LoadoutOntologyQuestions   = "ontology_questions"
)

// AllToolsOrdered defines the canonical order for tool presentation in UI and MCP.
// When merging loadouts, tools should be presented in this order.
var AllToolsOrdered = []ToolSpec{
	// Default
	{Name: "health", Description: "Server health check"},

	// Developer Core
	{Name: "echo", Description: "Echo back input message for testing"},
	{Name: "execute", Description: "Execute DDL/DML statements"},

	// Query tools
	{Name: "validate", Description: "Check SQL syntax without executing"},
	{Name: "query", Description: "Execute read-only SQL SELECT statements"},
	{Name: "explain_query", Description: "Analyze a read-only SQL query plan without executing it"},
	{Name: "list_approved_queries", Description: "List pre-approved SQL queries"},
	{Name: "execute_approved_query", Description: "Execute a pre-approved query by ID"},
	{Name: "suggest_approved_query", Description: "Suggest a reusable parameterized query for approval"},
	{Name: "suggest_query_update", Description: "Suggest an update to an existing pre-approved query for review"},
	// Query Management (Admin Tools) - grouped with other query tools
	{Name: "list_query_suggestions", Description: "List query suggestions awaiting review or previously rejected"},
	{Name: "approve_query_suggestion", Description: "Approve a pending query suggestion"},
	{Name: "reject_query_suggestion", Description: "Reject a pending query suggestion with reason"},
	{Name: "create_approved_query", Description: "Create a new pre-approved query directly (no review required)"},
	{Name: "update_approved_query", Description: "Update an existing pre-approved query directly (no review required)"},
	{Name: "delete_approved_query", Description: "Delete a pre-approved query"},
	{Name: "search_schema", Description: "Full-text search across tables and columns using pattern matching with relevance ranking"},
	{Name: "get_schema", Description: "Get database schema with semantic annotations"},
	{Name: "get_context", Description: "Get unified database context with progressive depth (consolidates ontology, schema, glossary)"},
	{Name: "get_column_metadata", Description: "Get current ontology metadata for a column (description, enum_values, role, schema info)"},
	{Name: "get_glossary_sql", Description: "Get a business term's glossary entry, including SQL when available"},
	{Name: "get_ontology", Description: "Get business ontology for query generation"},
	{Name: "get_query_history", Description: "Get recent query execution history to avoid rewriting queries"},
	{Name: "record_query_feedback", Description: "Record whether a generated query was helpful"},
	{Name: "list_glossary", Description: "List business glossary terms with definitions and SQL availability"},
	{Name: "probe_column", Description: "Deep-dive into specific column with statistics, joinability, and semantic information"},
	{Name: "probe_columns", Description: "Batch variant of probe_column for analyzing multiple columns at once"},
	{Name: "sample", Description: "Quick data preview from a table"},

	// Ontology Questions
	{Name: "list_ontology_questions", Description: "List ontology questions with filtering by status, category, priority, and pagination"},
	{Name: "dismiss_ontology_question", Description: "Mark a question as not worth pursuing (e.g., 'Column appears unused, legacy')"},
	{Name: "escalate_ontology_question", Description: "Mark a question as requiring human domain knowledge (e.g., 'Business rule not documented in code')"},
	{Name: "resolve_ontology_question", Description: "Mark an ontology question as resolved after researching and updating the ontology"},
	{Name: "skip_ontology_question", Description: "Mark a question as skipped for revisiting later (e.g., 'Need access to frontend repo')"},

	// Ontology Maintenance
	{Name: "create_glossary_term", Description: "Create a business glossary term with optional SQL"},
	{Name: "update_column", Description: "Add or update semantic information about a column (description, enum_values, role)"},
	{Name: "update_columns", Description: "Batch update metadata for multiple columns (up to 50) in a single transaction"},
	{Name: "update_glossary_term", Description: "Create or update a business glossary term with optional SQL and aliases"},
	{Name: "list_project_knowledge", Description: "List all project knowledge facts with fact IDs for discovery and maintenance"},
	{Name: "update_project_knowledge", Description: "Create or update writable domain facts (terminology, business rules, enumerations, conventions); project_overview is read-only here"},
	{Name: "update_table", Description: "Add or update table-level metadata (description, usage notes, ephemeral status, alternatives)"},
	{Name: "delete_column_metadata", Description: "Clear custom metadata for a column, reverting to schema-only information"},
	{Name: "delete_glossary_term", Description: "Delete a business glossary term that's no longer relevant"},
	{Name: "delete_project_knowledge", Description: "Remove writable domain facts by fact_id; project_overview is read-only here"},
	{Name: "delete_table_metadata", Description: "Clear custom metadata for a table, removing semantic enrichment"},
	{Name: "list_relationships", Description: "List schema relationships with semantic type and provenance details"},
	{Name: "create_relationship", Description: "Create a schema relationship between two columns"},
	{Name: "update_relationship", Description: "Update cardinality or approval state for an existing relationship"},
	{Name: "delete_relationship", Description: "Soft-delete a schema relationship while preserving the tombstone"},

	// Schema Management (Living Ontology)
	{Name: "refresh_schema", Description: "Refresh schema from datasource and detect changes (new tables, columns, etc.)"},
	{Name: "scan_data_changes", Description: "Scan for data-level changes (new enum values, FK patterns) in selected tables"},
	{Name: "list_pending_changes", Description: "List pending ontology changes awaiting review"},
	{Name: "approve_change", Description: "Approve a pending ontology change and apply it"},
	{Name: "reject_change", Description: "Reject a pending ontology change without applying it"},
	{Name: "approve_all_changes", Description: "Approve all pending ontology changes that can be applied"},
}

// Loadouts defines which tools belong to each loadout.
// Tools are referenced by name and looked up from AllToolsOrdered.
var Loadouts = map[string][]string{
	// Default -- Always available in every loadout
	LoadoutDefault: {
		"health",
	},

	// Developer Core -- specific to developer needs
	LoadoutDeveloperCore: {
		"echo",
		"execute",
	},

	// Limited Query -- only allows lockdown queries for agents
	LoadoutLimitedQuery: {
		"list_approved_queries",
		"execute_approved_query",
	},

	// Query -- allows ad-hoc queries
	LoadoutQuery: {
		"validate",
		"query",
		"explain_query",
		"list_approved_queries",
		"execute_approved_query",
		"suggest_approved_query",
		"suggest_query_update",
		"search_schema",
		"get_schema",
		"get_context",
		"get_column_metadata",
		"get_glossary_sql",
		"get_ontology",
		"get_query_history",
		"list_glossary",
		"probe_column",
		"probe_columns",
		"sample",
	},

	// Ontology Maintenance -- enable MCP Client to manage ontology and approved query catalogs
	LoadoutOntologyMaintenance: {
		"create_glossary_term",
		"update_column",
		"update_glossary_term",
		"list_project_knowledge",
		"update_project_knowledge",
		"update_table",
		"delete_column_metadata",
		"delete_glossary_term",
		"delete_project_knowledge",
		"delete_table_metadata",
		"list_relationships",
		"create_relationship",
		"update_relationship",
		"delete_relationship",
		// Schema management (living ontology) tools
		"refresh_schema",
		"scan_data_changes",
		"list_pending_changes",
		"approve_change",
		"reject_change",
		"approve_all_changes",
		// Approved query catalog management (Ontology Forge)
		"list_approved_queries",
		"execute_approved_query",
		"create_approved_query",
		"update_approved_query",
		"delete_approved_query",
		// Query suggestion review workflow (AI Data Liaison)
		"list_query_suggestions",
		"approve_query_suggestion",
		"reject_query_suggestion",
	},

	// Ontology Questions -- special mode for MCP Client to enumerate and answer pending questions
	LoadoutOntologyQuestions: {
		"list_ontology_questions",
		"dismiss_ontology_question",
		"escalate_ontology_question",
		"resolve_ontology_question",
		"skip_ontology_question",
	},
}

// allToolsIndex is a lookup map for tool order, built at init time.
var allToolsIndex map[string]int

func init() {
	allToolsIndex = make(map[string]int, len(AllToolsOrdered))
	for i, tool := range AllToolsOrdered {
		allToolsIndex[tool.Name] = i
	}
}

// GetToolSpec returns the ToolSpec for a given tool name, or nil if not found.
func GetToolSpec(name string) *ToolSpec {
	if idx, ok := allToolsIndex[name]; ok {
		return &AllToolsOrdered[idx]
	}
	return nil
}

// GetToolOrder returns the canonical order index for a tool, or -1 if not found.
func GetToolOrder(name string) int {
	if idx, ok := allToolsIndex[name]; ok {
		return idx
	}
	return -1
}

// MergeLoadouts combines multiple loadouts into a single deduplicated list of tools,
// ordered according to AllToolsOrdered.
func MergeLoadouts(loadoutNames ...string) []ToolSpec {
	// Collect unique tool names from all loadouts
	toolSet := make(map[string]bool)
	for _, loadoutName := range loadoutNames {
		if tools, ok := Loadouts[loadoutName]; ok {
			for _, toolName := range tools {
				toolSet[toolName] = true
			}
		}
	}

	// Build result in canonical order
	var result []ToolSpec
	for _, tool := range AllToolsOrdered {
		if toolSet[tool.Name] {
			result = append(result, tool)
		}
	}
	return result
}

// GetAllTools returns all tools in canonical order.
func GetAllTools() []ToolSpec {
	result := make([]ToolSpec, len(AllToolsOrdered))
	copy(result, AllToolsOrdered)
	return result
}

// ComputeEnabledToolsFromConfig computes the list of enabled tools based on config state.
// For admin/data roles (isAgent=false): Developer tools with config-driven loadouts.
// For agent authentication (isAgent=true): Limited query tools only.
func ComputeEnabledToolsFromConfig(state map[string]*models.ToolGroupConfig, isAgent bool) []ToolSpec {
	if state == nil {
		return MergeLoadouts(LoadoutDefault)
	}

	// Agent authentication: only limited query tools when agent_tools is enabled
	if isAgent {
		agentConfig := state[ToolGroupAgentTools]
		if agentConfig != nil && agentConfig.Enabled {
			return MergeLoadouts(LoadoutDefault, LoadoutLimitedQuery)
		}
		return MergeLoadouts(LoadoutDefault)
	}

	// Merge developer + user tools (union)
	devTools := ComputeDeveloperTools(state)
	userTools := ComputeUserTools(state)

	// Union into a set, then return in canonical order
	enabled := make(map[string]bool)
	for _, t := range devTools {
		enabled[t.Name] = true
	}
	for _, t := range userTools {
		enabled[t.Name] = true
	}

	return toolsInCanonicalOrder(enabled)
}

// IsToolInLoadout checks if a tool is in a specific loadout.
func IsToolInLoadout(toolName, loadoutName string) bool {
	tools, ok := Loadouts[loadoutName]
	if !ok {
		return false
	}
	for _, t := range tools {
		if t == toolName {
			return true
		}
	}
	return false
}

// ComputeUserTools returns the tool set for users with the "user" role.
// Uses per-app toggles to determine which user-facing tools are enabled.
func ComputeUserTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
	cfg := getToolsConfig(state)

	enabled := make(map[string]bool)
	// health always included
	for _, name := range Loadouts[LoadoutDefault] {
		enabled[name] = true
	}

	for _, toggle := range AppToggles {
		if toggle.Role != "user" {
			continue
		}
		if IsToggleEnabled(cfg, toggle.ToggleKey) {
			for _, toolName := range toggle.Tools {
				enabled[toolName] = true
			}
		}
	}

	return toolsInCanonicalOrder(enabled)
}

// ComputeDeveloperTools computes tools for admin/data/developer roles.
// Uses per-app toggles to determine which developer-facing tools are enabled.
func ComputeDeveloperTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
	cfg := getToolsConfig(state)

	enabled := make(map[string]bool)
	// health always included
	for _, name := range Loadouts[LoadoutDefault] {
		enabled[name] = true
	}

	for _, toggle := range AppToggles {
		if toggle.Role != "developer" {
			continue
		}
		if IsToggleEnabled(cfg, toggle.ToggleKey) {
			for _, toolName := range toggle.Tools {
				enabled[toolName] = true
			}
		}
	}

	return toolsInCanonicalOrder(enabled)
}

// getToolsConfig returns the supported per-app tools config.
func getToolsConfig(state map[string]*models.ToolGroupConfig) *models.ToolGroupConfig {
	if cfg, ok := state[ToolGroupTools]; ok && cfg != nil {
		return cfg
	}
	return nil
}

// toolsInCanonicalOrder returns ToolSpecs for the given tool names in AllToolsOrdered order.
func toolsInCanonicalOrder(enabled map[string]bool) []ToolSpec {
	var result []ToolSpec
	for _, tool := range AllToolsOrdered {
		if enabled[tool.Name] {
			result = append(result, tool)
		}
	}
	return result
}

// ComputeAgentTools computes tools for AI agents (API key authentication).
// Returns a limited set of tools:
// - Default loadout (health)
// - Limited Query loadout (list_approved_queries, execute_approved_query)
func ComputeAgentTools(_ map[string]*models.ToolGroupConfig) []ToolSpec {
	return MergeLoadouts(LoadoutDefault, LoadoutLimitedQuery)
}

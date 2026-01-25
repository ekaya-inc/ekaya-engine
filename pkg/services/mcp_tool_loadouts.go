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

// ToolGroupCustom is the identifier for the custom tools group.
// Other tool group constants are defined in mcp_config.go and mcp_tools_registry.go.
const ToolGroupCustom = "custom"

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
	{Name: "explain_query", Description: "Analyze SQL query performance using EXPLAIN ANALYZE with execution plan and optimization hints"},
	{Name: "list_approved_queries", Description: "List pre-approved SQL queries"},
	{Name: "execute_approved_query", Description: "Execute a pre-approved query by ID"},
	{Name: "suggest_approved_query", Description: "Suggest a reusable parameterized query for approval"},
	{Name: "search_schema", Description: "Full-text search across tables, columns, and entities using pattern matching with relevance ranking"},
	{Name: "get_schema", Description: "Get database schema with entity semantics"},
	{Name: "get_context", Description: "Get unified database context with progressive depth (consolidates ontology, schema, glossary)"},
	{Name: "get_entity", Description: "Retrieve full entity details including aliases, key columns, occurrences, and relationships"},
	{Name: "get_glossary_sql", Description: "Get SQL definition for a business term"},
	{Name: "get_ontology", Description: "Get business ontology for query generation"},
	{Name: "get_query_history", Description: "Get recent query execution history to avoid rewriting queries"},
	{Name: "list_glossary", Description: "List all business glossary terms"},
	{Name: "probe_column", Description: "Deep-dive into specific column with statistics, joinability, and semantic information"},
	{Name: "probe_columns", Description: "Batch variant of probe_column for analyzing multiple columns at once"},
	{Name: "probe_relationship", Description: "Deep-dive into relationships between entities with cardinality and data quality metrics"},
	{Name: "sample", Description: "Quick data preview from a table"},

	// Ontology Questions
	{Name: "list_ontology_questions", Description: "List ontology questions with filtering by status, category, entity, priority, and pagination"},
	{Name: "dismiss_ontology_question", Description: "Mark a question as not worth pursuing (e.g., 'Column appears unused, legacy')"},
	{Name: "escalate_ontology_question", Description: "Mark a question as requiring human domain knowledge (e.g., 'Business rule not documented in code')"},
	{Name: "resolve_ontology_question", Description: "Mark an ontology question as resolved after researching and updating the ontology"},
	{Name: "skip_ontology_question", Description: "Mark a question as skipped for revisiting later (e.g., 'Need access to frontend repo')"},

	// Ontology Maintenance
	{Name: "create_glossary_term", Description: "Create a new business glossary term with SQL definition"},
	{Name: "update_column", Description: "Add or update semantic information about a column (description, enum_values, entity, role)"},
	{Name: "update_entity", Description: "Create or update entity metadata with upsert semantics (description, aliases, key_columns)"},
	{Name: "update_glossary_term", Description: "Create or update a business glossary term with upsert semantics (definition, sql, aliases)"},
	{Name: "update_project_knowledge", Description: "Create or update domain facts (terminology, business rules, enumerations, conventions)"},
	{Name: "update_relationship", Description: "Create or update relationship between entities with upsert semantics (description, label, cardinality)"},
	{Name: "delete_column_metadata", Description: "Clear custom metadata for a column, reverting to schema-only information"},
	{Name: "delete_entity", Description: "Remove an entity that was incorrectly identified (soft delete)"},
	{Name: "delete_glossary_term", Description: "Delete a business glossary term that's no longer relevant"},
	{Name: "delete_project_knowledge", Description: "Remove incorrect or outdated domain facts"},
	{Name: "delete_relationship", Description: "Remove a relationship that doesn't exist or was incorrectly identified"},

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
		"search_schema",
		"get_schema",
		"get_context",
		"get_entity",
		"get_glossary_sql",
		"get_ontology",
		"get_query_history",
		"list_glossary",
		"probe_column",
		"probe_columns",
		"probe_relationship",
		"sample",
	},

	// Ontology Maintenance -- enable MCP Client to manage ontology
	LoadoutOntologyMaintenance: {
		"create_glossary_term",
		"update_column",
		"update_entity",
		"update_glossary_term",
		"update_project_knowledge",
		"update_relationship",
		"delete_column_metadata",
		"delete_entity",
		"delete_glossary_term",
		"delete_project_knowledge",
		"delete_relationship",
		// Schema management (living ontology) tools
		"refresh_schema",
		"scan_data_changes",
		"list_pending_changes",
		"approve_change",
		"reject_change",
		"approve_all_changes",
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

// UILoadoutMapping defines which loadouts are activated by UI selections.
// The key describes the UI state, the value is the list of loadouts to merge.
//
// UI States:
//   - All loadouts include: Default
//   - Business Tools selected: +Query
//   - Business Tools + Allow Usage to Improve Ontology: +Query +Ontology Maintenance
//   - Agent Tools selected: +Limited Query
//   - Developer Tools selected: +Developer Core
//   - Developer Tools + Add Query Tools: +Developer Core +Query
//   - Developer Tools + Add Ontology Maintenance: +Developer Core +Ontology Maintenance +Ontology Questions
//   - Developer Tools + Add Query Tools + Add Ontology Maintenance: +Developer Core +Query +Ontology Maintenance +Ontology Questions
//   - Custom Tools selected: All tools available (individual selection)
var UILoadoutMapping = map[string][]string{
	"business":                             {LoadoutDefault, LoadoutQuery},
	"business_with_ontology":               {LoadoutDefault, LoadoutQuery, LoadoutOntologyMaintenance},
	"agent":                                {LoadoutDefault, LoadoutLimitedQuery},
	"developer":                            {LoadoutDefault, LoadoutDeveloperCore},
	"developer_with_query":                 {LoadoutDefault, LoadoutDeveloperCore, LoadoutQuery},
	"developer_with_ontology_maintenance":  {LoadoutDefault, LoadoutDeveloperCore, LoadoutOntologyMaintenance, LoadoutOntologyQuestions},
	"developer_with_query_and_maintenance": {LoadoutDefault, LoadoutDeveloperCore, LoadoutQuery, LoadoutOntologyMaintenance, LoadoutOntologyQuestions},
	"custom":                               {}, // Custom mode uses individual tool selection
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
// This is the main entry point for determining which tools to expose.
// The isAgent parameter indicates if the caller is using agent authentication.
func ComputeEnabledToolsFromConfig(state map[string]*models.ToolGroupConfig, isAgent bool) []ToolSpec {
	if state == nil {
		// Only default loadout (health) when no state
		return MergeLoadouts(LoadoutDefault)
	}

	// Agent authentication: only limited query tools when agent_tools is enabled
	if isAgent {
		agentConfig := state[ToolGroupAgentTools]
		if agentConfig != nil && agentConfig.Enabled {
			return MergeLoadouts(LoadoutDefault, LoadoutLimitedQuery)
		}
		// Agent auth but agent_tools not enabled - only health
		return MergeLoadouts(LoadoutDefault)
	}

	// User authentication from here on

	// Custom tools mode: use individually selected tools
	customConfig := state[ToolGroupCustom]
	if customConfig != nil && customConfig.Enabled {
		return computeCustomTools(customConfig.CustomTools)
	}

	// Build loadout list based on enabled groups
	loadouts := []string{LoadoutDefault}

	// Business Tools (approved_queries)
	aqConfig := state[ToolGroupApprovedQueries]
	if aqConfig != nil && aqConfig.Enabled {
		loadouts = append(loadouts, LoadoutQuery)
		if aqConfig.AllowOntologyMaintenance {
			loadouts = append(loadouts, LoadoutOntologyMaintenance)
		}
	}

	// Note: Agent Tools setting only affects agent authentication (handled above).
	// From user perspective, enabling agent_tools doesn't add any tools.

	// Developer Tools
	devConfig := state[ToolGroupDeveloper]
	if devConfig != nil && devConfig.Enabled {
		loadouts = append(loadouts, LoadoutDeveloperCore)
		if devConfig.AddQueryTools {
			loadouts = append(loadouts, LoadoutQuery)
		}
		if devConfig.AddOntologyMaintenance {
			loadouts = append(loadouts, LoadoutOntologyMaintenance, LoadoutOntologyQuestions)
		}
	}

	return MergeLoadouts(loadouts...)
}

// computeCustomTools returns tools based on individually selected tool names.
// Always includes the default loadout (health).
func computeCustomTools(selectedTools []string) []ToolSpec {
	// Build set of selected tools
	selected := make(map[string]bool)
	for _, name := range selectedTools {
		selected[name] = true
	}

	// Always include default loadout tools
	for _, name := range Loadouts[LoadoutDefault] {
		selected[name] = true
	}

	// Return tools in canonical order
	var result []ToolSpec
	for _, tool := range AllToolsOrdered {
		if selected[tool.Name] {
			result = append(result, tool)
		}
	}
	return result
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

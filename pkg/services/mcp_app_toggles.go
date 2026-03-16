package services

import (
	"slices"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AppToggle defines a toggle that an application provides for a specific role.
type AppToggle struct {
	AppID       string   // e.g., "mcp-server", "ontology-forge", "ai-data-liaison"
	Role        string   // "developer" or "user"
	ToggleKey   string   // config field name, e.g., "addDirectDatabaseAccess"
	DisplayName string   // UI label, e.g., "Direct Database Access"
	Tools       []string // tool names controlled by this toggle
}

// AppToggles defines all per-app toggles in display order.
var AppToggles = []AppToggle{
	// MCP Server — Developer
	{
		AppID:       models.AppIDMCPServer,
		Role:        "developer",
		ToggleKey:   "addDirectDatabaseAccess",
		DisplayName: "Direct Database Access",
		Tools:       []string{"echo", "execute", "query"},
	},
	// Ontology Forge — Developer
	{
		AppID:       models.AppIDOntologyForge,
		Role:        "developer",
		ToggleKey:   "addOntologyMaintenanceTools",
		DisplayName: "Add Ontology Maintenance Tools",
		Tools: []string{
			"get_schema", "search_schema", "probe_column", "probe_columns", "get_column_metadata",
			"update_column", "update_columns", "update_table", "update_project_knowledge",
			"delete_column_metadata", "delete_table_metadata", "delete_project_knowledge",
			"refresh_schema", "scan_data_changes", "list_pending_changes",
			"approve_change", "reject_change", "approve_all_changes",
			"list_ontology_questions", "resolve_ontology_question", "skip_ontology_question",
			"escalate_ontology_question", "dismiss_ontology_question",
		},
	},
	// Ontology Forge — User
	{
		AppID:       models.AppIDOntologyForge,
		Role:        "user",
		ToggleKey:   "addOntologySuggestions",
		DisplayName: "Add Ontology Suggestions",
		Tools:       []string{"get_context", "get_ontology"},
	},
	// AI Data Liaison — Developer
	{
		AppID:       models.AppIDAIDataLiaison,
		Role:        "developer",
		ToggleKey:   "addApprovalTools",
		DisplayName: "Add Approval Tools",
		Tools: []string{
			"list_query_suggestions", "approve_query_suggestion", "reject_query_suggestion",
			"create_approved_query", "update_approved_query", "delete_approved_query",
			"create_glossary_term", "update_glossary_term", "delete_glossary_term",
			"explain_query",
		},
	},
	// AI Data Liaison — User
	{
		AppID:       models.AppIDAIDataLiaison,
		Role:        "user",
		ToggleKey:   "addRequestTools",
		DisplayName: "Add Request Tools",
		Tools: []string{
			"query", "sample", "validate",
			"list_glossary", "get_glossary_sql",
			"list_approved_queries", "execute_approved_query",
			"suggest_approved_query", "suggest_query_update",
			"get_query_history", "record_query_feedback",
		},
	},
}

// AppDisplayNames maps app IDs to their display names.
var AppDisplayNames = map[string]string{
	models.AppIDMCPServer:     "MCP Server",
	models.AppIDOntologyForge: "Ontology Forge",
	models.AppIDAIDataLiaison: "AI Data Liaison",
}

// GetToolAppID returns the app ID of the toggle controlling a tool for a given role.
// For "health", always returns models.AppIDMCPServer.
func GetToolAppID(toolName, role string) string {
	if toolName == "health" {
		return models.AppIDMCPServer
	}
	for _, toggle := range AppToggles {
		if toggle.Role != role {
			continue
		}
		if slices.Contains(toggle.Tools, toolName) {
			return toggle.AppID
		}
	}
	return models.AppIDMCPServer // fallback
}

// GetToolOwningAppIDs returns every app that can expose the tool through the
// toggle registry, independent of caller role.
func GetToolOwningAppIDs(toolName string) []string {
	if toolName == "health" {
		return []string{models.AppIDMCPServer}
	}

	appIDs := make([]string, 0, 2)
	for _, toggle := range AppToggles {
		if !slices.Contains(toggle.Tools, toolName) {
			continue
		}
		if slices.Contains(appIDs, toggle.AppID) {
			continue
		}
		appIDs = append(appIDs, toggle.AppID)
	}

	return appIDs
}

// GetEnabledToolOwningAppIDs returns app IDs for toggle paths that both own the
// tool and are currently enabled in the provided config state.
func GetEnabledToolOwningAppIDs(toolName string, state map[string]*models.ToolGroupConfig) []string {
	if toolName == "health" {
		return []string{models.AppIDMCPServer}
	}

	cfg := getToolsConfig(state)
	appIDs := make([]string, 0, 2)
	for _, toggle := range AppToggles {
		if !IsToggleEnabled(cfg, toggle.ToggleKey) {
			continue
		}
		if !slices.Contains(toggle.Tools, toolName) {
			continue
		}
		if slices.Contains(appIDs, toggle.AppID) {
			continue
		}
		appIDs = append(appIDs, toggle.AppID)
	}

	return appIDs
}

// IsToggleEnabled checks if a toggle is enabled in the config.
func IsToggleEnabled(cfg *models.ToolGroupConfig, toggleKey string) bool {
	if cfg == nil {
		return false
	}
	switch toggleKey {
	case "addDirectDatabaseAccess":
		return cfg.AddDirectDatabaseAccess
	case "addOntologyMaintenanceTools":
		return cfg.AddOntologyMaintenanceTools
	case "addOntologySuggestions":
		return cfg.AddOntologySuggestions
	case "addApprovalTools":
		return cfg.AddApprovalTools
	case "addRequestTools":
		return cfg.AddRequestTools
	default:
		return false
	}
}

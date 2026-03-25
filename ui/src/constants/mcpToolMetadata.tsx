// MCP tool group constants.
// This file provides the group IDs used for API calls and sub-option keys.

export const TOOL_GROUP_IDS = {
  AGENT: 'agent_tools',
  TOOLS: 'tools',
} as const;

// Sub-option keys for tool group configuration
export const TOOL_GROUP_SUB_OPTIONS = {
  // Per-app tool toggles
  ADD_DIRECT_DATABASE_ACCESS: 'addDirectDatabaseAccess',
  ADD_ONTOLOGY_MAINTENANCE_TOOLS: 'addOntologyMaintenanceTools',
  ADD_ONTOLOGY_SUGGESTIONS: 'addOntologySuggestions',
  ADD_APPROVAL_TOOLS: 'addApprovalTools',
  ADD_REQUEST_TOOLS: 'addRequestTools',
} as const;

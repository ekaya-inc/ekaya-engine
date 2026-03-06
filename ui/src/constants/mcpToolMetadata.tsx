// MCP tool group constants.
// UI strings for each section are now inline in the respective components
// (UserToolsSection, DeveloperToolsSection, AgentToolsSection).
// This file provides the group IDs used for API calls and sub-option keys.

export const TOOL_GROUP_IDS = {
  USER: 'user',
  DEVELOPER: 'developer',
  AGENT: 'agent_tools',
  TOOLS: 'tools',
} as const;

// Sub-option keys for tool group configuration
export const TOOL_GROUP_SUB_OPTIONS = {
  // User Tools sub-options
  ALLOW_ONTOLOGY_MAINTENANCE: 'allowOntologyMaintenance',
  // Developer Tools sub-options
  ADD_QUERY_TOOLS: 'addQueryTools',
  ADD_ONTOLOGY_MAINTENANCE: 'addOntologyMaintenance',
  // Per-app tool toggles
  ADD_DIRECT_DATABASE_ACCESS: 'addDirectDatabaseAccess',
  ADD_ONTOLOGY_MAINTENANCE_TOOLS: 'addOntologyMaintenanceTools',
  ADD_ONTOLOGY_SUGGESTIONS: 'addOntologySuggestions',
  ADD_APPROVAL_TOOLS: 'addApprovalTools',
  ADD_REQUEST_TOOLS: 'addRequestTools',
} as const;

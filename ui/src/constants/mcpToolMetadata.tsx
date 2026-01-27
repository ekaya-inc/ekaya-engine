// MCP tool group constants.
// UI strings for each section are now inline in the respective components
// (UserToolsSection, DeveloperToolsSection, AgentToolsSection).
// This file provides the group IDs used for API calls and sub-option keys.

export const TOOL_GROUP_IDS = {
  USER: 'user',
  DEVELOPER: 'developer',
  AGENT: 'agent_tools',
} as const;

// Sub-option keys for tool group configuration
export const TOOL_GROUP_SUB_OPTIONS = {
  // User Tools sub-options
  ALLOW_ONTOLOGY_MAINTENANCE: 'allowOntologyMaintenance',
  // Developer Tools sub-options
  ADD_QUERY_TOOLS: 'addQueryTools',
  ADD_ONTOLOGY_MAINTENANCE: 'addOntologyMaintenance',
} as const;

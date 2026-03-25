import type { ReactNode } from 'react';

/**
 * MCP Configuration Types
 */

// API Response Types (state only - no UI strings)

/**
 * ToolGroupState represents the configuration state from the API.
 * UI metadata (names, descriptions, warnings) is defined in constants/mcpToolMetadata.ts.
 */
export interface ToolGroupState {
  enabled: boolean;

  // Per-app tool toggles
  addDirectDatabaseAccess?: boolean;
  addOntologyMaintenanceTools?: boolean;
  addOntologySuggestions?: boolean;
  addApprovalTools?: boolean;
  addRequestTools?: boolean;
}

/**
 * EnabledToolInfo represents a tool that is currently enabled.
 * Returned by the API to show which tools are active based on current config.
 */
export interface EnabledToolInfo {
  name: string;
  description: string;
  appId: string;
}

export interface ServerStatusResponse {
  base_url: string;
  is_localhost: boolean;
  is_https: boolean;
  accessible_for_business_users: boolean;
}

export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupState>;
  userTools: EnabledToolInfo[];
  developerTools: EnabledToolInfo[];
  agentTools: EnabledToolInfo[];
  appNames: Record<string, string>;
}

// API Request Types

export interface ToolGroupConfigUpdate {
  enabled: boolean;
}

/**
 * UpdateMCPConfigRequest uses flat fields to update MCP configuration.
 * Only include fields you want to change - omitted fields are not modified.
 */
export interface UpdateMCPConfigRequest {
  // Per-app tool toggles
  addDirectDatabaseAccess?: boolean;
  addOntologyMaintenanceTools?: boolean;
  addOntologySuggestions?: boolean;
  addApprovalTools?: boolean;
  addRequestTools?: boolean;
}

// UI Rendering Types (state merged with frontend metadata)

export interface SubOptionInfo {
  enabled: boolean;
  name: string;
  description?: ReactNode | undefined;
  warning?: string | undefined;
  tip?: ReactNode | undefined;
}

export interface ToolGroupInfo {
  enabled: boolean;
  name: string;
  description: ReactNode;
  warning?: string;
  subOptions?: Record<string, SubOptionInfo>;
}

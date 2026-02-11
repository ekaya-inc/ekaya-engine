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

  // Business Tools (approved_queries) sub-options
  allowOntologyMaintenance?: boolean;

  // Developer Tools sub-options
  addQueryTools?: boolean;
  addOntologyMaintenance?: boolean;

  // Custom Tools - individually selected tool names
  customTools?: string[];

  // Legacy sub-options (backward compatibility)
  enableExecute?: boolean;
  forceMode?: boolean;
  allowClientSuggestions?: boolean;
}

/**
 * EnabledToolInfo represents a tool that is currently enabled.
 * Returned by the API to show which tools are active based on current config.
 */
export interface EnabledToolInfo {
  name: string;
  description: string;
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
  /** @deprecated Use userTools, developerTools, or agentTools instead. Kept for backward compatibility. */
  enabledTools: EnabledToolInfo[];
}

// API Request Types

export interface ToolGroupConfigUpdate {
  enabled: boolean;

  // Business Tools (approved_queries) sub-options
  allowOntologyMaintenance?: boolean;

  // Developer Tools sub-options
  addQueryTools?: boolean;
  addOntologyMaintenance?: boolean;

  // Custom Tools - individually selected tool names
  customTools?: string[];

  // Legacy sub-options (backward compatibility)
  enableExecute?: boolean;
  forceMode?: boolean;
  allowClientSuggestions?: boolean;
}

/**
 * UpdateMCPConfigRequest uses flat fields to update MCP configuration.
 * Only include fields you want to change - omitted fields are not modified.
 */
export interface UpdateMCPConfigRequest {
  // User Tools sub-option
  allowOntologyMaintenance?: boolean;
  // Developer Tools sub-options
  addQueryTools?: boolean;
  addOntologyMaintenance?: boolean;
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

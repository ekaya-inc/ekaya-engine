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
  enableExecute?: boolean;
  // approved_queries only
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

export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupState>;
  enabledTools: EnabledToolInfo[];
}

// API Request Types

export interface ToolGroupConfigUpdate {
  enabled: boolean;
  enableExecute?: boolean;
  // approved_queries only
  forceMode?: boolean;
  allowClientSuggestions?: boolean;
}

export interface UpdateMCPConfigRequest {
  toolGroups: Record<string, ToolGroupConfigUpdate>;
}

// UI Rendering Types (state merged with frontend metadata)

export interface SubOptionInfo {
  enabled: boolean;
  name: string;
  description?: ReactNode | undefined;
  warning?: string | undefined;
}

export interface ToolGroupInfo {
  enabled: boolean;
  name: string;
  description: ReactNode;
  warning?: string;
  subOptions?: Record<string, SubOptionInfo>;
}

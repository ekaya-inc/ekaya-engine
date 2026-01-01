/**
 * MCP Configuration Types
 */

export interface SubOptionInfo {
  enabled: boolean;
  name: string;
  description?: string;
  warning?: string;
}

export interface ToolGroupInfo {
  enabled: boolean;
  name: string;
  description: string;
  warning?: string;
  subOptions?: Record<string, SubOptionInfo>;
}

export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupInfo>;
}

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

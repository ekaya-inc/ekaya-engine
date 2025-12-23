/**
 * MCP Configuration Types
 */

export interface ToolGroupInfo {
  enabled: boolean;
  name: string;
  description: string;
  warning?: string;
}

export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupInfo>;
}

export interface UpdateMCPConfigRequest {
  toolGroups: Record<string, { enabled: boolean }>;
}

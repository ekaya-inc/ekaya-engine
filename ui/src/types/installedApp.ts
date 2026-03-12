/**
 * Installed App Types
 * Types for tracking installed applications per project
 */

/**
 * Known application IDs
 */
export const APP_ID_MCP_SERVER = 'mcp-server';
export const APP_ID_ONTOLOGY_FORGE = 'ontology-forge';
export const APP_ID_AI_DATA_LIAISON = 'ai-data-liaison';
export const APP_ID_AI_AGENTS = 'ai-agents';
export const APP_ID_FILE_LOADER = 'file-loader';
export const APP_ID_MCP_TUNNEL = 'mcp-tunnel';

/**
 * Installed application record
 */
export interface InstalledApp {
  id: string;
  project_id: string;
  app_id: string;
  installed_at: string;
  installed_by?: string;
  activated_at?: string;
  settings: Record<string, unknown>;
}

/**
 * Response from list installed apps endpoint
 */
export interface InstalledAppsResponse {
  apps: InstalledApp[];
}

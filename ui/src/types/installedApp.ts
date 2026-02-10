/**
 * Installed App Types
 * Types for tracking installed applications per project
 */

/**
 * Known application IDs
 */
export const APP_ID_MCP_SERVER = 'mcp-server';
export const APP_ID_AI_DATA_LIAISON = 'ai-data-liaison';
export const APP_ID_AI_AGENTS = 'ai-agents';

/**
 * Installed application record
 */
export interface InstalledApp {
  id: string;
  project_id: string;
  app_id: string;
  installed_at: string;
  installed_by?: string;
  settings: Record<string, unknown>;
}

/**
 * Response from list installed apps endpoint
 */
export interface InstalledAppsResponse {
  apps: InstalledApp[];
}

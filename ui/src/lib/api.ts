import { getCachedConfig } from '../services/config';

import { clearProjectToken, getProjectToken, isTokenExpired } from './auth-token';
import { initiateOAuthFlow } from './oauth';

/**
 * Extract project ID from current URL path
 *
 * Supports patterns:
 * - /projects/:id/...
 * - SDAP pattern: /sdap/v1/:id/...
 */
function extractProjectIdFromPath(): string | undefined {
  const projectsMatch = window.location.pathname.match(/\/projects\/([a-f0-9-]+)/);
  const sdapMatch = window.location.pathname.match(/\/sdap\/v1\/([a-f0-9-]+)\//);
  return projectsMatch?.[1] ?? sdapMatch?.[1];
}

/**
 * Authenticated fetch wrapper that automatically handles OAuth redirects
 *
 * Features:
 * - Sends JWT as Authorization Bearer header (tab-scoped authentication)
 * - Checks token expiry before making requests
 * - Detects 401 Unauthorized and 403 Forbidden responses
 * - Initiates OAuth 2.1 flow with PKCE on authentication failure or project mismatch
 * - Clears expired tokens from sessionStorage
 *
 * @example
 * const response = await fetchWithAuth('/api/projects');
 * if (response.ok) {
 *   const data = await response.json();
 * }
 */
export async function fetchWithAuth(url: string, options: RequestInit = {}): Promise<Response> {
  const token = getProjectToken();

  // Check if we have a valid token
  if (!token || isTokenExpired(token)) {
    console.log('No valid token - initiating OAuth flow');
    clearProjectToken();

    const config = getCachedConfig();
    if (!config) {
      console.error('Config not loaded - cannot initiate OAuth');
      throw new Error('Configuration not available');
    }

    const projectId = extractProjectIdFromPath();
    await initiateOAuthFlow(config, projectId);
    return new Promise(() => {}); // Redirecting
  }

  // Send token as Bearer header
  const response = await fetch(url, {
    ...options,
    headers: {
      ...options.headers,
      'Authorization': `Bearer ${token}`,
    },
  });

  // Handle 401 Unauthorized OR 403 Forbidden - clear token and re-auth
  // 401: No valid JWT token present
  // 403: JWT token present but project ID mismatch (wrong project)
  if (response.status === 401 || response.status === 403) {
    const statusText = response.status === 401 ? 'Unauthorized' : 'Forbidden (project mismatch)';
    console.log(`${response.status} ${statusText} detected - clearing token and re-authenticating`);
    clearProjectToken();

    const config = getCachedConfig();
    if (!config) {
      console.error('Config not loaded - cannot initiate OAuth');
      throw new Error('Configuration not available');
    }

    const projectId = extractProjectIdFromPath();
    await initiateOAuthFlow(config, projectId);
    return new Promise(() => {}); // Redirecting
  }

  return response;
}

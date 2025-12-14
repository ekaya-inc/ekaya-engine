import { getCachedConfig } from '../services/config';

import { initiateOAuthFlow } from './oauth';

/**
 * Authenticated fetch wrapper that automatically handles OAuth redirects
 *
 * Features:
 * - Sends httpOnly cookies automatically (credentials: 'include')
 * - Detects 401 Unauthorized and 403 Forbidden responses
 * - Initiates OAuth 2.1 flow with PKCE on authentication failure or project mismatch
 * - Clears stale JWT cookies before re-authenticating
 *
 * @example
 * const response = await fetchWithAuth('/api/projects');
 * if (response.ok) {
 *   const data = await response.json();
 * }
 */
export async function fetchWithAuth(url: string, options: RequestInit = {}): Promise<Response> {
  // Always include credentials (httpOnly cookies)
  const response = await fetch(url, {
    ...options,
    credentials: 'include',
  });

  // Handle 401 Unauthorized OR 403 Forbidden - initiate OAuth flow
  // 401: No valid JWT token present
  // 403: JWT token present but project ID mismatch (wrong project)
  if (response.status === 401 || response.status === 403) {
    const statusText = response.status === 401 ? 'Unauthorized' : 'Forbidden (project mismatch)';
    console.log(`${response.status} ${statusText} detected - initiating OAuth flow`);

    // Clear old JWT cookie to ensure fresh authentication
    // Set Max-Age=0 to expire the cookie immediately
    document.cookie = 'ekaya_jwt=; Max-Age=0; path=/; SameSite=Strict';

    // Get config (should be cached by now since ConfigProvider loads it on app startup)
    const config = getCachedConfig();
    if (!config) {
      console.error('Config not loaded - cannot initiate OAuth');
      throw new Error('Configuration not available');
    }

    // Extract project_id from URL if present
    // Try multiple patterns: /projects/:id/... or SDAP pattern /:uuid/datasources
    const projectsMatch = window.location.pathname.match(/\/projects\/([a-f0-9-]+)/);
    const sdapMatch = url.match(/\/sdap\/v1\/([a-f0-9-]+)\//);

    const projectId = projectsMatch?.[1] || sdapMatch?.[1];

    console.log(`Clearing stale cookie and re-authenticating for project: ${projectId || 'none'}`);

    // Initiate OAuth flow with project_id if found
    await initiateOAuthFlow(config, projectId);

    // Return a rejected promise since we're redirecting
    // This prevents further processing of the response
    return new Promise(() => {});
  }

  return response;
}

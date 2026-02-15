import type { OAuthDiscoveryMetadata } from '../types/api';

/**
 * Application configuration fetched from backend at runtime
 * Combines /api/config (client-specific) with OAuth discovery metadata (RFC 8414)
 */
export interface AppConfig {
  // From /api/config (client-specific, not in RFC 8414)
  oauthClientId: string;
  baseUrl: string;
  // From /.well-known/oauth-authorization-server (RFC 8414)
  authorizationEndpoint: string;
  tokenEndpoint: string;
  // Kept for backward compatibility (same as issuer from discovery)
  authServerUrl: string;
}

let cachedConfig: AppConfig | null = null;
let cachedAuthUrl: string | null = null;

/**
 * Extract project ID from the current page's URL path
 * Matches paths like /projects/{uuid}/...
 */
function getProjectIdFromPath(): string | null {
  const match = window.location.pathname.match(/\/projects\/([a-f0-9-]+)/);
  return match?.[1] ?? null;
}

/**
 * Save auth_url to project parameters in the backend
 * This persists the auth server URL so re-authentication uses the correct server
 */
async function saveAuthUrlToProject(
  projectId: string,
  authUrl: string
): Promise<void> {
  const response = await fetch(`/api/projects/${projectId}/auth-server-url`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ auth_server_url: authUrl }),
  });

  if (!response.ok) {
    throw new Error(`Failed to save auth_url: ${response.status}`);
  }
}

/**
 * Extract auth_url from the current page's URL query parameters
 * This is passed by ekaya-central when redirecting users to ekaya-engine
 */
export function getAuthUrlFromQuery(): string | null {
  const params = new URLSearchParams(window.location.search);
  return params.get('auth_url');
}

/**
 * Fetch application configuration from backend
 * Fetches both /api/config and OAuth discovery metadata in parallel
 * Results are cached in memory after first successful fetch
 *
 * If auth_url query parameter is present in the current URL, it will be
 * passed to the backend for validation. The backend validates auth_url
 * against a whitelist of allowed auth servers (JWKS endpoints).
 *
 * @throws Error if auth_url is provided but not in the backend's whitelist
 */
export async function fetchConfig(): Promise<AppConfig> {
  // Extract auth_url from current page URL and project_id from path
  const authUrl = getAuthUrlFromQuery();
  const projectId = getProjectIdFromPath();

  // Return cached config only if auth_url hasn't changed
  if (cachedConfig && cachedAuthUrl === authUrl) {
    return cachedConfig;
  }

  // Build query string with auth_url and/or project_id
  // project_id is needed so backend can look up saved auth_server_url when auth_url is not in URL
  const params = new URLSearchParams();
  if (authUrl) params.set('auth_url', authUrl);
  if (projectId) params.set('project_id', projectId);
  const queryString = params.toString() ? `?${params.toString()}` : '';

  // Fetch both endpoints in parallel
  // Note: auth_url is only needed for well-known endpoint (determines which auth server metadata to return)
  // /api/config/auth returns static client config (oauth_client_id, base_url)
  const [configResponse, discoveryResponse] = await Promise.all([
    fetch(`/api/config/auth`),
    fetch(`/.well-known/oauth-authorization-server${queryString}`),
  ]);

  if (!configResponse.ok) {
    throw new Error(`Failed to fetch config: ${configResponse.statusText}`);
  }

  if (!discoveryResponse.ok) {
    if (discoveryResponse.status === 400) {
      throw new Error('Invalid auth_url: not in allowed list');
    }
    throw new Error(`Failed to fetch OAuth discovery: ${discoveryResponse.statusText}`);
  }

  const configData = await configResponse.json();
  const discoveryData: OAuthDiscoveryMetadata = await discoveryResponse.json();

  cachedConfig = {
    // From /api/config
    oauthClientId: configData.oauth_client_id,
    baseUrl: configData.base_url,
    // From OAuth discovery (RFC 8414)
    authorizationEndpoint: discoveryData.authorization_endpoint,
    tokenEndpoint: discoveryData.token_endpoint,
    // Backward compatibility (issuer from discovery)
    authServerUrl: discoveryData.issuer,
  };
  cachedAuthUrl = authUrl;

  // If auth_url was provided via query param and matches the returned config,
  // persist it to the project so re-authentication uses the correct server
  if (authUrl && projectId && cachedConfig.authServerUrl === authUrl) {
    saveAuthUrlToProject(projectId, authUrl).catch((err) => {
      console.warn('Failed to persist auth_url to project:', err);
      // Non-fatal - continue with cached value
    });
  }

  return cachedConfig;
}

/**
 * Get cached config synchronously
 * Returns null if config hasn't been fetched yet
 * Use this in non-React code that can't use useConfig() hook
 */
export function getCachedConfig(): AppConfig | null {
  return cachedConfig;
}

/**
 * Clear cached config (useful for testing)
 */
export function clearConfigCache(): void {
  cachedConfig = null;
}

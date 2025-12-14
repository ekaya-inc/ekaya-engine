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
 * Extract auth_url from the current page's URL query parameters
 * This is passed by ekaya-central when redirecting users to ekaya-region
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
  // Extract auth_url from current page URL
  const authUrl = getAuthUrlFromQuery();

  // Return cached config only if auth_url hasn't changed
  if (cachedConfig && cachedAuthUrl === authUrl) {
    return cachedConfig;
  }

  // Build query string if auth_url is present
  const queryString = authUrl ? `?auth_url=${encodeURIComponent(authUrl)}` : '';

  // Fetch both endpoints in parallel with auth_url if provided
  const [configResponse, discoveryResponse] = await Promise.all([
    fetch(`/api/config${queryString}`),
    fetch(`/.well-known/oauth-authorization-server${queryString}`),
  ]);

  if (!configResponse.ok) {
    if (configResponse.status === 400) {
      throw new Error('Invalid auth_url: not in allowed list');
    }
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

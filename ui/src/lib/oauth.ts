import type { AppConfig } from '../services/config';

import { generatePKCE } from './auth';

/**
 * Generate cryptographically secure random string for OAuth state parameter
 */
function generateRandomString(length = 32): string {
  const array = new Uint8Array(length / 2);
  crypto.getRandomValues(array);
  return Array.from(array, byte => byte.toString(16).padStart(2, '0')).join('');
}

/**
 * Initiate OAuth 2.1 authorization flow with PKCE
 *
 * This function handles the complete OAuth redirect flow:
 * 1. Generates PKCE challenge/verifier pair
 * 2. Generates random state for CSRF protection
 * 3. Stores verifier and state in sessionStorage for callback verification
 * 4. Stores current URL to return to after authentication
 * 5. Builds authorization URL with all required parameters
 * 6. Redirects to authorization server
 *
 * Required parameters sent to auth server:
 * - response_type: 'code' (authorization code flow)
 * - client_id: From config (e.g., 'ekaya-region-localhost')
 * - redirect_uri: Where auth server sends user after authentication
 * - scope: 'project:access'
 * - state: Random CSRF token
 * - code_challenge: PKCE challenge (SHA-256 of verifier)
 * - code_challenge_method: 'S256'
 * - project_id: (optional) Project being accessed
 * - hostname: (optional) For localhost development
 * - port: (optional) For localhost development
 *
 * @param config - Application configuration from backend
 * @param projectId - Optional project ID being accessed
 *
 * @example
 * // From HomePage with config
 * const { config } = useConfig();
 * if (config) {
 *   await initiateOAuthFlow(config);
 * }
 *
 * @example
 * // From ProjectGuard when no JWT
 * const { config } = useConfig();
 * if (config) {
 *   await initiateOAuthFlow(config, pid);
 * }
 */
export async function initiateOAuthFlow(config: AppConfig, projectId?: string): Promise<void> {
  // Generate PKCE challenge and verifier
  const { code_verifier, code_challenge } = await generatePKCE();

  // Generate random state for CSRF protection
  const state = generateRandomString(32);

  // Store verifier and state in sessionStorage for callback
  sessionStorage.setItem('oauth_code_verifier', code_verifier);
  sessionStorage.setItem('oauth_state', state);

  // Store auth server URL for token exchange (required when using dynamic auth_url)
  sessionStorage.setItem('oauth_auth_server_url', config.authServerUrl);

  // Store current URL to return to after auth
  // BUT never store /oauth/callback as return URL (would cause infinite loop)
  const currentPath = window.location.pathname + window.location.search;
  const returnUrl = currentPath.startsWith('/oauth/callback') ? '/' : currentPath;
  sessionStorage.setItem('oauth_return_url', returnUrl);

  // Build authorization URL using discovery endpoint from RFC 8414
  const redirectUri = `${window.location.origin}/oauth/callback`;
  // Use hostname from window.location, fallback to localhost
  const hostname = window.location.hostname || 'localhost';
  // Use port from window.location (runtime), fallback to 5173 for dev server
  const port = window.location.port || '5173';

  const authUrl = new URL(config.authorizationEndpoint);
  authUrl.searchParams.set('response_type', 'code');
  authUrl.searchParams.set('client_id', config.oauthClientId);
  authUrl.searchParams.set('redirect_uri', redirectUri);
  authUrl.searchParams.set('scope', 'project:access');
  authUrl.searchParams.set('state', state);
  authUrl.searchParams.set('code_challenge', code_challenge);
  authUrl.searchParams.set('code_challenge_method', 'S256');
  authUrl.searchParams.set('hostname', hostname);
  authUrl.searchParams.set('port', port);

  // Add project_id if provided
  if (projectId) {
    authUrl.searchParams.set('project_id', projectId);
  }

  console.log('Initiating OAuth flow:', authUrl.toString());

  // Redirect to authorization server
  window.location.href = authUrl.toString();
}

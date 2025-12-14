import pkceChallenge from 'pkce-challenge';

export interface PKCEPair {
  code_verifier: string;
  code_challenge: string;
}

/**
 * Generate PKCE challenge and verifier pair for OAuth 2.1
 *
 * Uses pkce-challenge library which implements RFC 7636:
 * - Generates cryptographically secure random verifier (128 chars)
 * - Computes SHA-256 hash and base64url-encodes it as challenge
 *
 * @example
 * const { code_verifier, code_challenge } = await generatePKCE();
 * sessionStorage.setItem('oauth_code_verifier', code_verifier);
 * // Send code_challenge to auth server
 */
export async function generatePKCE(): Promise<PKCEPair> {
  // Library generates both verifier (128 chars) and challenge (base64url SHA-256)
  const { code_verifier, code_challenge } = await pkceChallenge();

  return {
    code_verifier,
    code_challenge
  };
}

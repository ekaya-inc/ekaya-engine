import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import * as auth from './auth';
import { initiateOAuthFlow } from './oauth';

// Mock the auth module
vi.mock('./auth', () => ({
  generatePKCE: vi.fn(),
}));

describe('initiateOAuthFlow', () => {
  let originalLocation: Location;
  let mockSessionStorage: { [key: string]: string };

  const mockConfig = {
    authServerUrl: 'http://localhost:5002',
    oauthClientId: 'ekaya-engine-localhost',
    baseUrl: 'http://localhost:3443',
    authorizationEndpoint: 'http://localhost:5002/authorize',
    tokenEndpoint: 'http://localhost:3443/mcp/oauth/token',
  };

  beforeEach(() => {
    // Save original location
    originalLocation = window.location;

    // Mock window.location
    delete (window as unknown as { location: unknown }).location;
    (window as unknown as { location: Partial<Location> }).location = {
      href: '',
      origin: 'http://localhost:3443',
      pathname: '/projects/test-pid-123',
      search: '',
      port: '3443',
    };

    // Mock sessionStorage (spy on instance for Node.js v25+ compat where native
    // Storage uses C++ dispatch that bypasses JS prototype spies)
    mockSessionStorage = {};
    vi.spyOn(sessionStorage, 'setItem').mockImplementation((key, value) => {
      mockSessionStorage[key] = value;
    });
    vi.spyOn(sessionStorage, 'getItem').mockImplementation((key) => {
      return mockSessionStorage[key] ?? null;
    });

    // Mock generatePKCE
    vi.mocked(auth.generatePKCE).mockResolvedValue({
      code_verifier: 'mock-verifier-12345',
      code_challenge: 'mock-challenge-67890',
    });

    // Mock crypto.getRandomValues for state generation
    vi.spyOn(crypto, 'getRandomValues').mockImplementation(<T extends ArrayBufferView | null>(array: T): T => {
      // Fill with predictable values for testing
      if (array && 'length' in array) {
        const uint8Array = array as unknown as Uint8Array;
        for (let i = 0; i < uint8Array.length; i++) {
          uint8Array[i] = i;
        }
      }
      return array;
    });
  });

  afterEach(() => {
    (window as unknown as { location: Location }).location = originalLocation;
    vi.restoreAllMocks();
  });

  it('should generate PKCE challenge and verifier', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    expect(auth.generatePKCE).toHaveBeenCalledOnce();
  });

  it('should store code_verifier in sessionStorage', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    expect(mockSessionStorage['oauth_code_verifier']).toBe('mock-verifier-12345');
  });

  it('should store state in sessionStorage', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    const state = mockSessionStorage['oauth_state'];
    expect(state).toBeDefined();
    expect(state).toMatch(/^[0-9a-f]{32}$/); // 32 hex characters
  });

  it('should store current URL in sessionStorage', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    expect(mockSessionStorage['oauth_return_url']).toBe('/projects/test-pid-123');
  });

  it('should store auth server URL in sessionStorage', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    expect(mockSessionStorage['oauth_auth_server_url']).toBe('http://localhost:5002');
  });

  it('should include all required OAuth 2.1 PKCE parameters', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    const redirectUrl = window.location.href;

    // Verify all required parameters are present
    expect(redirectUrl).toContain('response_type=code');
    expect(redirectUrl).toContain('client_id=ekaya-engine-localhost');
    expect(redirectUrl).toContain('redirect_uri=http%3A%2F%2Flocalhost%3A3443%2Foauth%2Fcallback');
    expect(redirectUrl).toContain('scope=project%3Aaccess');
    expect(redirectUrl).toContain('state='); // State should be present
    expect(redirectUrl).toContain('code_challenge=mock-challenge-67890');
    expect(redirectUrl).toContain('code_challenge_method=S256');
  });

  it('should include project_id when provided', async () => {
    await initiateOAuthFlow(mockConfig, 'my-project-abc-123');

    const redirectUrl = window.location.href;
    expect(redirectUrl).toContain('project_id=my-project-abc-123');
  });

  it('should not include project_id parameter when not provided', async () => {
    await initiateOAuthFlow(mockConfig);

    const redirectUrl = window.location.href;
    expect(redirectUrl).not.toContain('project_id=');
  });

  it('should redirect to auth server URL', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project');

    // Should redirect to auth server (checks that it includes /authorize)
    expect(window.location.href).toContain('/authorize');
    expect(window.location.href).toMatch(/^https?:\/\//); // Valid URL
  });

  it('should use authorizationEndpoint from OAuth discovery', async () => {
    const customConfig = {
      ...mockConfig,
      authorizationEndpoint: 'https://custom-auth.example.com/authorize',
    };

    await initiateOAuthFlow(customConfig, 'test-project');

    // Should use the provided discovery endpoint URL
    expect(window.location.href).toContain('custom-auth.example.com');
    expect(window.location.href).toContain('/authorize');
  });

  it('should include hostname and port parameters', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project');

    const redirectUrl = window.location.href;
    expect(redirectUrl).toContain('hostname=localhost');
    // Port comes from env or window.location.port, just verify it's present
    expect(redirectUrl).toMatch(/[&?]port=/);
  });

  it('should handle URL with query parameters', async () => {
    window.location.search = '?foo=bar&baz=qux';

    await initiateOAuthFlow(mockConfig, 'test-project');

    // Check that oauth_return_url was stored with query params
    expect(mockSessionStorage['oauth_return_url']).toBe('/projects/test-pid-123?foo=bar&baz=qux');
  });

  it('should use different state value on each call', async () => {
    // Clear previous mocks so we use real crypto for state generation
    vi.restoreAllMocks();

    // First call with real crypto
    await initiateOAuthFlow(mockConfig, 'project1');
    const firstState = sessionStorage.getItem('oauth_state');

    // Second call
    await initiateOAuthFlow(mockConfig, 'project2');
    const secondState = sessionStorage.getItem('oauth_state');

    // States should be different (CSRF protection)
    expect(firstState).toBeDefined();
    expect(secondState).toBeDefined();
    expect(firstState).not.toEqual(secondState);

    // Re-setup mocks for remaining tests
    vi.mocked(auth.generatePKCE).mockResolvedValue({
      code_verifier: 'mock-verifier-12345',
      code_challenge: 'mock-challenge-67890',
    });
  });

  it('should log OAuth initiation to console', async () => {
    const consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {});

    await initiateOAuthFlow(mockConfig, 'test-project');

    expect(consoleSpy).toHaveBeenCalledWith(
      expect.stringContaining('Initiating OAuth flow:'),
      expect.any(String)
    );

    consoleSpy.mockRestore();
  });

  // This test catches the bug we fixed where ProjectGuard was missing PKCE params
  it('should always include state and code_challenge (regression test)', async () => {
    await initiateOAuthFlow(mockConfig, 'test-project-id');

    const redirectUrl = window.location.href;
    const url = new URL(redirectUrl);

    // These were missing in the original ProjectGuard implementation
    expect(url.searchParams.has('state')).toBe(true);
    expect(url.searchParams.has('code_challenge')).toBe(true);
    expect(url.searchParams.has('code_challenge_method')).toBe(true);
    expect(url.searchParams.get('code_challenge_method')).toBe('S256');

    // Verify they have actual values, not empty strings
    expect(url.searchParams.get('state')).not.toBe('');
    expect(url.searchParams.get('code_challenge')).not.toBe('');
  });
});

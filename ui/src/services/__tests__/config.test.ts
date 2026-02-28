import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Dynamic import so we get a fresh module (with fresh cachedConfig) per test
/* eslint-disable @typescript-eslint/consistent-type-imports */
let fetchConfig: typeof import('../config').fetchConfig;
let getCachedConfig: typeof import('../config').getCachedConfig;
let getAuthUrlFromQuery: typeof import('../config').getAuthUrlFromQuery;
let clearConfigCache: typeof import('../config').clearConfigCache;
/* eslint-enable @typescript-eslint/consistent-type-imports */

// Helper to build mock responses
function mockResponse(body: object, opts: { ok?: boolean; status?: number; statusText?: string } = {}) {
  return {
    ok: opts.ok ?? true,
    status: opts.status ?? 200,
    statusText: opts.statusText ?? 'OK',
    json: () => Promise.resolve(body),
  } as Response;
}

const configPayload = { oauth_client_id: 'test-client', base_url: 'https://app.test' };
const discoveryPayload = {
  issuer: 'https://auth.test',
  authorization_endpoint: 'https://auth.test/authorize',
  token_endpoint: 'https://auth.test/token',
  jwks_uri: 'https://auth.test/.well-known/jwks.json',
  scopes_supported: ['openid'],
  response_types_supported: ['code'],
  grant_types_supported: ['authorization_code'],
  token_endpoint_auth_methods_supported: ['none'],
  code_challenge_methods_supported: ['S256'],
};

describe('config.ts', () => {
  let originalLocation: Location;

  beforeEach(async () => {
    vi.restoreAllMocks();
    // Save and replace window.location so we can control pathname/search
    originalLocation = window.location;
    delete (window as any).location;
    (window as any).location = {
      pathname: '/',
      search: '',
      href: 'http://localhost/',
      origin: 'http://localhost',
      hash: '',
    };

    // Fresh import each test to reset module-level cache
    vi.resetModules();
    const mod = await import('../config');
    fetchConfig = mod.fetchConfig;
    getCachedConfig = mod.getCachedConfig;
    getAuthUrlFromQuery = mod.getAuthUrlFromQuery;
    clearConfigCache = mod.clearConfigCache;
  });

  afterEach(() => {
    (window as any).location = originalLocation;
    vi.restoreAllMocks();
  });

  // ── getAuthUrlFromQuery ──────────────────────────────────────────────

  describe('getAuthUrlFromQuery', () => {
    it('returns auth_url when present in query string', () => {
      (window as any).location.search = '?auth_url=https://auth.example.com';
      expect(getAuthUrlFromQuery()).toBe('https://auth.example.com');
    });

    it('returns null when auth_url is not in query string', () => {
      (window as any).location.search = '?other=value';
      expect(getAuthUrlFromQuery()).toBeNull();
    });

    it('returns null when query string is empty', () => {
      (window as any).location.search = '';
      expect(getAuthUrlFromQuery()).toBeNull();
    });
  });

  // ── getCachedConfig ──────────────────────────────────────────────────

  describe('getCachedConfig', () => {
    it('returns null before fetchConfig is called', () => {
      expect(getCachedConfig()).toBeNull();
    });

    it('returns cached value after fetchConfig succeeds', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      await fetchConfig();

      const cached = getCachedConfig();
      expect(cached).not.toBeNull();
      expect(cached?.oauthClientId).toBe('test-client');
      expect(cached?.baseUrl).toBe('https://app.test');
      expect(cached?.authorizationEndpoint).toBe('https://auth.test/authorize');
      expect(cached?.tokenEndpoint).toBe('https://auth.test/token');
      expect(cached?.authServerUrl).toBe('https://auth.test');
    });
  });

  // ── fetchConfig ──────────────────────────────────────────────────────

  describe('fetchConfig', () => {
    it('fetches /api/config/auth and /.well-known/oauth-authorization-server in parallel', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      const result = await fetchConfig();

      expect(global.fetch).toHaveBeenCalledTimes(2);
      expect(global.fetch).toHaveBeenCalledWith('/api/config/auth');
      expect(global.fetch).toHaveBeenCalledWith('/.well-known/oauth-authorization-server');

      expect(result).toEqual({
        oauthClientId: 'test-client',
        baseUrl: 'https://app.test',
        authorizationEndpoint: 'https://auth.test/authorize',
        tokenEndpoint: 'https://auth.test/token',
        authServerUrl: 'https://auth.test',
      });
    });

    it('returns cached config on subsequent calls with same auth_url', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      const first = await fetchConfig();
      const second = await fetchConfig();

      // fetch should only have been called once (2 calls for the parallel fetch)
      expect(global.fetch).toHaveBeenCalledTimes(2);
      expect(second).toBe(first);
    });

    it('re-fetches when auth_url query param changes', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload))
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      // First fetch with no auth_url
      await fetchConfig();
      expect(global.fetch).toHaveBeenCalledTimes(2);

      // Change auth_url in query string
      (window as any).location.search = '?auth_url=https://new-auth.test';
      await fetchConfig();

      // Should have fetched again
      expect(global.fetch).toHaveBeenCalledTimes(4);
    });

    it('passes auth_url and project_id as query params to discovery endpoint', async () => {
      (window as any).location.search = '?auth_url=https://auth.example.com';
      (window as any).location.pathname = '/projects/abc12300-0000-0000-0000-0000000000ef';

      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      await fetchConfig();

      // /api/config/auth has no query string
      expect(global.fetch).toHaveBeenCalledWith('/api/config/auth');
      // discovery endpoint gets both params
      const discoveryCall = (global.fetch as any).mock.calls[1][0] as string;
      expect(discoveryCall).toContain('auth_url=');
      expect(discoveryCall).toContain('project_id=abc12300-0000-0000-0000-0000000000ef');
    });

    it('throws when /api/config/auth response is not ok', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse({}, { ok: false, status: 500, statusText: 'Internal Server Error' }))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      await expect(fetchConfig()).rejects.toThrow('Failed to fetch config: Internal Server Error');
    });

    it('throws specific message when discovery returns 400 (invalid auth_url)', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse({}, { ok: false, status: 400, statusText: 'Bad Request' }));

      await expect(fetchConfig()).rejects.toThrow('Invalid auth_url: not in allowed list');
    });

    it('throws when discovery response is not ok (non-400)', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse({}, { ok: false, status: 502, statusText: 'Bad Gateway' }));

      await expect(fetchConfig()).rejects.toThrow('Failed to fetch OAuth discovery: Bad Gateway');
    });

    it('persists auth_url to project when auth_url matches returned config', async () => {
      const authUrl = 'https://auth.test';
      (window as any).location.search = `?auth_url=${authUrl}`;
      (window as any).location.pathname = '/projects/a1b2c3d4-e5f6-0000-0000-000000000456';

      // Mock fetch: first two for config + discovery, third for saveAuthUrlToProject PATCH
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload))
        .mockResolvedValueOnce(mockResponse({}, { ok: true })); // PATCH response

      await fetchConfig();

      // Give the fire-and-forget saveAuthUrlToProject a tick to execute
      await new Promise(resolve => setTimeout(resolve, 10));

      // The third fetch call should be the PATCH to save auth_url
      expect(global.fetch).toHaveBeenCalledTimes(3);
      expect(global.fetch).toHaveBeenCalledWith(
        '/api/projects/a1b2c3d4-e5f6-0000-0000-000000000456/auth-server-url',
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ auth_server_url: authUrl }),
        })
      );
    });

    it('does not persist auth_url when auth_url does not match returned config', async () => {
      (window as any).location.search = '?auth_url=https://different-auth.test';
      (window as any).location.pathname = '/projects/a1b2c3d4-e5f6-0000-0000-000000000456';

      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      await fetchConfig();

      await new Promise(resolve => setTimeout(resolve, 10));

      // Only 2 calls (config + discovery), no PATCH
      expect(global.fetch).toHaveBeenCalledTimes(2);
    });
  });

  // ── clearConfigCache ─────────────────────────────────────────────────

  describe('clearConfigCache', () => {
    it('clears cached config so getCachedConfig returns null', async () => {
      global.fetch = vi.fn()
        .mockResolvedValueOnce(mockResponse(configPayload))
        .mockResolvedValueOnce(mockResponse(discoveryPayload));

      await fetchConfig();
      expect(getCachedConfig()).not.toBeNull();

      clearConfigCache();
      expect(getCachedConfig()).toBeNull();
    });
  });
});

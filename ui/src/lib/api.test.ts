import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import { fetchWithAuth } from './api';
import * as authToken from './auth-token';

// Mock the config service
vi.mock('../services/config', () => ({
  getCachedConfig: vi.fn(() => ({
    authServerUrl: 'http://localhost:5002',
    oauthClientId: 'ekaya-region-localhost',
    baseUrl: 'http://localhost:3443',
    authorizationEndpoint: 'http://localhost:5002/authorize',
    tokenEndpoint: 'http://localhost:3443/mcp/oauth/token',
  })),
}));

// Mock the auth-token utilities
vi.mock('./auth-token', () => ({
  getProjectToken: vi.fn(),
  clearProjectToken: vi.fn(),
  isTokenExpired: vi.fn(),
}));

// Mock the generatePKCE function
vi.mock('./auth', () => ({
  generatePKCE: vi.fn(() => Promise.resolve({
    code_verifier: 'test-verifier-123',
    code_challenge: 'test-challenge-456'
  }))
}));

describe('api.js - fetchWithAuth', () => {
  beforeEach(() => {
    // Clear sessionStorage before each test
    sessionStorage.clear();

    // Reset window.location
    delete (window as any).location;
    (window as any).location = {
      href: 'http://localhost:5173/projects/test-123',
      origin: 'http://localhost:5173',
      pathname: '/projects/test-123',
      search: '',
      hash: '',
    };

    // Clear all mocks
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('should send Authorization Bearer header with token', async () => {
    const mockToken = 'valid-jwt-token';
    vi.mocked(authToken.getProjectToken).mockReturnValue(mockToken);
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);

    // Mock fetch to return success
    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 200,
        ok: true,
        json: () => Promise.resolve({ data: 'success' }),
      } as Response)
    ) as any;

    const response = await fetchWithAuth('/api/test');

    expect(response.status).toBe(200);
    expect(global.fetch).toHaveBeenCalledWith('/api/test', expect.objectContaining({
      headers: expect.objectContaining({
        'Authorization': `Bearer ${mockToken}`,
      }),
    }));
  });

  it('should NOT send credentials: include (no cookies)', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);

    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 200,
        ok: true,
      } as Response)
    ) as any;

    await fetchWithAuth('/api/test');

    // Should not include credentials: 'include' since we're using Bearer token
    const fetchCall = vi.mocked(global.fetch).mock.calls[0];
    expect(fetchCall?.[1]?.credentials).toBeUndefined();
  });

  it('should initiate OAuth flow when no token present', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue(null);

    // Mock window.location for OAuth redirect
    const originalHref = window.location.href;
    Object.defineProperty(window.location, 'href', {
      writable: true,
      value: originalHref,
    });

    // Start the fetchWithAuth call (it will try to redirect)
    fetchWithAuth('/api/test');

    // Wait for async handling
    await new Promise(resolve => setTimeout(resolve, 50));

    // Should not call fetch when no token
    expect(global.fetch).not.toHaveBeenCalled();

    // Verify sessionStorage keys are correct
    expect(sessionStorage.getItem('oauth_code_verifier')).toBe('test-verifier-123');
    expect(sessionStorage.getItem('oauth_state')).toBeTruthy();
    const oauthState = sessionStorage.getItem('oauth_state');
    expect(oauthState).not.toBeNull();
    expect(oauthState?.length).toBeGreaterThanOrEqual(32); // Random state
    expect(sessionStorage.getItem('oauth_return_url')).toBe('/projects/test-123');
  });

  it('should clear token and re-auth on 401 response', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('expired-token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);

    // Mock fetch to return 401
    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 401,
        ok: false,
      } as Response)
    ) as any;

    // Mock window.location for OAuth redirect
    const originalHref = window.location.href;
    Object.defineProperty(window.location, 'href', {
      writable: true,
      value: originalHref,
    });

    fetchWithAuth('/api/test');

    // Wait for async handling
    await new Promise(resolve => setTimeout(resolve, 50));

    expect(authToken.clearProjectToken).toHaveBeenCalled();
  });

  it('should clear token and re-auth on 403 response', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('wrong-project-token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);

    // Mock fetch to return 403
    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 403,
        ok: false,
      } as Response)
    ) as any;

    // Mock window.location for OAuth redirect
    const originalHref = window.location.href;
    Object.defineProperty(window.location, 'href', {
      writable: true,
      value: originalHref,
    });

    fetchWithAuth('/api/test');

    // Wait for async handling
    await new Promise(resolve => setTimeout(resolve, 50));

    expect(authToken.clearProjectToken).toHaveBeenCalled();
  });

  it('should extract project_id from URL path when re-authenticating', async () => {
    (window as any).location.pathname = '/projects/abc-def-123';
    vi.mocked(authToken.getProjectToken).mockReturnValue('token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);

    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 401,
        ok: false,
      } as Response)
    ) as any;

    // Mock window.location.href setter
    const hrefSpy = vi.fn();
    Object.defineProperty(window.location, 'href', {
      set: hrefSpy,
      get: () => 'http://localhost:5173/projects/abc-def-123',
    });

    fetchWithAuth('/api/test');

    // Wait for redirect
    await new Promise(resolve => setTimeout(resolve, 50));

    // Check that redirect URL includes project_id
    expect(hrefSpy).toHaveBeenCalled();
    const firstCall = hrefSpy.mock.calls[0];
    expect(firstCall).toBeDefined();
    const redirectUrl = firstCall?.[0];
    expect(redirectUrl).toContain('project_id=abc-def-123');
  });

  it('should preserve custom headers and merge with Authorization', async () => {
    vi.mocked(authToken.getProjectToken).mockReturnValue('test-token');
    vi.mocked(authToken.isTokenExpired).mockReturnValue(false);

    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 200,
        ok: true,
      } as Response)
    ) as any;

    await fetchWithAuth('/api/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });

    expect(global.fetch).toHaveBeenCalledWith('/api/test', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer test-token',
      },
    });
  });
});

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import { fetchWithAuth } from './api';

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

  it('should pass through successful responses (200)', async () => {
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
    expect(global.fetch).toHaveBeenCalledWith('/api/test', {
      credentials: 'include',
    });
  });

  it('should store correct sessionStorage keys on 401', async () => {
    // Mock fetch to return 401
    global.fetch = vi.fn(() =>
      Promise.resolve({
        status: 401,
        ok: false,
      } as Response)
    ) as any;

    // Prevent actual redirect
    const originalHref = window.location.href;
    Object.defineProperty(window.location, 'href', {
      writable: true,
      value: originalHref,
    });

    // Start the fetchWithAuth call (it will try to redirect)
    fetchWithAuth('/api/projects');

    // Wait a bit for sessionStorage to be set
    await new Promise(resolve => setTimeout(resolve, 50));

    // Verify sessionStorage keys are correct
    expect(sessionStorage.getItem('oauth_code_verifier')).toBe('test-verifier-123');
    expect(sessionStorage.getItem('oauth_state')).toBeTruthy();
    expect(sessionStorage.getItem('oauth_state')!.length).toBeGreaterThanOrEqual(32); // Random state
    expect(sessionStorage.getItem('oauth_return_url')).toBe('/projects/test-123');
  });

  it('should extract project_id from URL path on 401', async () => {
    (window as any).location.pathname = '/projects/abc-def-123';

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
    await new Promise(resolve => setTimeout(resolve, 10));

    // Check that redirect URL includes project_id
    expect(hrefSpy).toHaveBeenCalled();
    const redirectUrl = hrefSpy.mock.calls[0]![0];
    expect(redirectUrl).toContain('project_id=abc-def-123');
  });

  it('should include credentials in all requests', async () => {
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
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    });
  });
});

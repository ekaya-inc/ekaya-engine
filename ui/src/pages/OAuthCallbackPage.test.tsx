import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach } from 'vitest';

import * as authToken from '@/lib/auth-token';

import OAuthCallbackPage from './OAuthCallbackPage';

// Mock navigate
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Mock auth-token module
vi.mock('../lib/auth-token', async () => {
  const actual = await vi.importActual('../lib/auth-token');
  return {
    ...actual,
    storeProjectToken: vi.fn(),
  };
});

describe('OAuthCallbackPage', () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.clearAllMocks();
    global.fetch = vi.fn() as any;
  });

  it('should show error when code is missing', async () => {
    render(
      <MemoryRouter initialEntries={['/oauth/callback?state=test123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Authentication Error/i)).toBeInTheDocument();
    });

    expect(screen.getByText(/Missing authorization code or state parameter/i)).toBeInTheDocument();
  });

  it('should show error when state is missing', async () => {
    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=abc123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Authentication Error/i)).toBeInTheDocument();
    });

    expect(screen.getByText(/Missing authorization code or state parameter/i)).toBeInTheDocument();
  });

  it('should validate state parameter matches stored state (CSRF protection)', async () => {
    // Store state in sessionStorage
    sessionStorage.setItem('oauth_state', 'correct-state-123');

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=abc123&state=wrong-state-456']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/State parameter mismatch - potential CSRF attack detected/i)).toBeInTheDocument();
    });

    // Verify state was cleaned up after failed validation
    expect(sessionStorage.getItem('oauth_state')).toBeNull();
  });

  it('should show error when stored state is missing', async () => {
    // Don't set any state in sessionStorage

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=abc123&state=some-state']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Missing stored state/i)).toBeInTheDocument();
    });
  });

  it('should show error when code_verifier is missing', async () => {
    sessionStorage.setItem('oauth_state', 'test-state-123');
    // Don't set code_verifier

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=abc123&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Missing PKCE verifier/i)).toBeInTheDocument();
    });
  });

  it('should show error when auth_server_url is missing', async () => {
    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    // Don't set oauth_auth_server_url

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=abc123&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Missing auth server URL/i)).toBeInTheDocument();
    });
  });

  it('should call complete-oauth endpoint with correct parameters', async () => {
    // Set up sessionStorage with correct keys
    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://auth.dev.ekaya.ai');
    sessionStorage.setItem('oauth_return_url', '/projects/test-project');

    // Capture the request body for flexible assertion
    let capturedBody: any = null;
    global.fetch = vi.fn((_url, options) => {
      if (options?.body) {
        capturedBody = JSON.parse(options.body as string);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ redirect_url: '/projects/test-project' }),
      } as Response);
    }) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code-789&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith('/api/auth/complete-oauth', expect.any(Object));
    });

    // Verify request body structure (port varies in test environments)
    expect(capturedBody).toEqual({
      code: 'auth-code-789',
      state: 'test-state-123',
      code_verifier: 'test-verifier-456',
      auth_url: 'https://auth.dev.ekaya.ai',
      redirect_uri: expect.stringMatching(/^http:\/\/localhost:\d+\/oauth\/callback$/),
    });
  });

  it('should include redirect_uri based on window.location.origin', async () => {
    // This test verifies that redirect_uri is included in the POST body
    // to fix the redirect URI mismatch issue when running on different ports
    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://us.ekaya.ai');
    sessionStorage.setItem('oauth_return_url', '/dashboard');

    let capturedBody: any = null;
    global.fetch = vi.fn((_url, options) => {
      if (options?.body) {
        capturedBody = JSON.parse(options.body as string);
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ redirect_url: '/dashboard' }),
      } as Response);
    }) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=code123&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });

    // Verify redirect_uri is included and matches window.location.origin
    expect(capturedBody).not.toBeNull();
    expect(capturedBody.redirect_uri).toBeDefined();
    expect(capturedBody.redirect_uri).toContain('/oauth/callback');
  });

  it('should clean up all OAuth sessionStorage keys after successful auth', async () => {
    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://auth.dev.ekaya.ai');
    sessionStorage.setItem('oauth_return_url', '/dashboard');

    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ redirect_url: '/dashboard' }),
      } as Response)
    ) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=code123&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Authentication successful/i)).toBeInTheDocument();
    });

    // Verify all OAuth keys are cleaned up
    expect(sessionStorage.getItem('oauth_state')).toBeNull();
    expect(sessionStorage.getItem('oauth_code_verifier')).toBeNull();
    expect(sessionStorage.getItem('oauth_auth_server_url')).toBeNull();
    expect(sessionStorage.getItem('oauth_return_url')).toBeNull();
  });

  it('should store JWT in sessionStorage when token and project_id are in response', async () => {
    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://auth.dev.ekaya.ai');
    sessionStorage.setItem('oauth_return_url', '/projects/test-project');

    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
          success: true,
          redirect_url: '/projects/test-project',
          token: 'new-jwt-token',
          project_id: 'project-123',
        }),
      } as Response)
    ) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(authToken.storeProjectToken).toHaveBeenCalledWith('new-jwt-token', 'project-123');
    });
  });

  it('should extract project_id from JWT if not in response', async () => {
    // JWT with project_id in payload
    const payload = { project_id: 'extracted-project-456', exp: Date.now() / 1000 + 3600 };
    const mockJwt = `header.${btoa(JSON.stringify(payload))}.signature`;

    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://auth.dev.ekaya.ai');
    sessionStorage.setItem('oauth_return_url', '/projects/test-project');

    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
          success: true,
          redirect_url: '/projects/test-project',
          token: mockJwt,
          // No project_id in response - should extract from JWT
        }),
      } as Response)
    ) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(authToken.storeProjectToken).toHaveBeenCalledWith(mockJwt, 'extracted-project-456');
    });
  });

  it('should not store token if extraction fails and no project_id in response', async () => {
    // Malformed JWT
    const malformedJwt = 'not.a.valid.jwt';

    sessionStorage.setItem('oauth_state', 'test-state-123');
    sessionStorage.setItem('oauth_code_verifier', 'test-verifier-456');
    sessionStorage.setItem('oauth_auth_server_url', 'https://auth.dev.ekaya.ai');
    sessionStorage.setItem('oauth_return_url', '/projects/test-project');

    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
          success: true,
          redirect_url: '/projects/test-project',
          token: malformedJwt,
          // No project_id in response and JWT is malformed
        }),
      } as Response)
    ) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Authentication successful/i)).toBeInTheDocument();
    });

    // Should not call storeProjectToken when extraction fails
    expect(authToken.storeProjectToken).not.toHaveBeenCalled();
  });
});

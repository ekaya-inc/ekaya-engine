import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach } from 'vitest';

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

    // Mock successful fetch
    global.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ redirect_url: '/projects/test-project' }),
      } as Response)
    ) as any;

    render(
      <MemoryRouter initialEntries={['/oauth/callback?code=auth-code-789&state=test-state-123']}>
        <OAuthCallbackPage />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith('/api/auth/complete-oauth', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify({
          code: 'auth-code-789',
          state: 'test-state-123',
          code_verifier: 'test-verifier-456',
          auth_url: 'https://auth.dev.ekaya.ai',
        }),
      });
    });
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
});

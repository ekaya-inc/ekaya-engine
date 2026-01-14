import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import { ConfigProvider } from '../../contexts/ConfigContext';
import ProjectsRedirect from '../ProjectsRedirect';

// Mock fetchConfig to return test config
vi.mock('../../services/config', () => ({
  fetchConfig: vi.fn().mockResolvedValue({
    oauthClientId: 'test-client-id',
    baseUrl: 'http://localhost:3443',
    authorizationEndpoint: 'https://us.ekaya.ai/authorize',
    tokenEndpoint: 'https://us.ekaya.ai/token',
    authServerUrl: 'https://us.ekaya.ai',
  }),
  getAuthUrlFromQuery: vi.fn().mockReturnValue(null),
  getCachedConfig: vi.fn().mockReturnValue(null),
  clearConfigCache: vi.fn(),
}));

describe('ProjectsRedirect', () => {
  const originalHref = window.location.href;

  beforeEach(() => {
    // Mock window.location.href using Object.defineProperty
    Object.defineProperty(window, 'location', {
      value: { href: '' },
      writable: true,
    });
  });

  afterEach(() => {
    // Restore original window.location
    Object.defineProperty(window, 'location', {
      value: { href: originalHref },
      writable: true,
    });
    vi.clearAllMocks();
  });

  it('should show redirecting message while loading', async () => {
    render(
      <ConfigProvider>
        <MemoryRouter initialEntries={['/projects']}>
          <ProjectsRedirect />
        </MemoryRouter>
      </ConfigProvider>
    );

    // Should show loading/redirecting message
    await waitFor(() => {
      expect(screen.getByText(/Redirecting to project selection/i)).toBeInTheDocument();
    });
  });

  it('should redirect to auth server projects page when config is loaded', async () => {
    render(
      <ConfigProvider>
        <MemoryRouter initialEntries={['/projects']}>
          <ProjectsRedirect />
        </MemoryRouter>
      </ConfigProvider>
    );

    // Wait for config to load and redirect to happen
    await waitFor(() => {
      expect(window.location.href).toBe('https://us.ekaya.ai/projects');
    });
  });
});

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { MCPConfigResponse } from '../../types';
import AIDataLiaisonPage from '../AIDataLiaisonPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    uninstallApp: vi.fn(),
    activateApp: vi.fn(),
    completeAppCallback: vi.fn(),
    getInstalledApp: vi.fn(),
    getMCPConfig: vi.fn(),
    updateMCPConfig: vi.fn(),
  },
}));

const mockToast = vi.fn();
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({ toast: mockToast }),
}));

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('../../contexts/ProjectContext', () => ({
  useProject: () => ({
    urls: { projectsPageUrl: 'https://us.ekaya.ai/projects' },
  }),
}));

vi.mock('../../contexts/ConfigContext', () => ({
  useConfig: () => ({
    config: {
      oauthClientId: 'test-client',
      baseUrl: 'http://localhost:3443',
      authorizationEndpoint: 'http://localhost:8080/authorize',
      tokenEndpoint: 'http://localhost:8080/token',
      authServerUrl: 'http://localhost:8080',
    },
    loading: false,
    error: null,
  }),
}));

const mockMCPConfig: MCPConfigResponse = {
  serverUrl: 'https://example.com/mcp/proj-1',
  userTools: [],
  developerTools: [],
  agentTools: [],
  toolGroups: {
    tools: {
      enabled: true,
      addApprovalTools: true,
      addRequestTools: true,
    },
  },
  appNames: {},
};

function setupMocks(options: { isActivated?: boolean } = {}) {
  const { isActivated = false } = options;

  vi.mocked(engineApi.getInstalledApp).mockResolvedValue({
    success: true,
    data: {
      id: 'inst-1',
      project_id: 'proj-1',
      app_id: 'ai-data-liaison',
      installed_at: '2024-01-01',
      settings: {},
      ...(isActivated ? { activated_at: '2024-01-02' } : {}),
    },
  });
  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });
  vi.mocked(engineApi.activateApp).mockResolvedValue({
    success: true,
    data: {},
  });
  vi.mocked(engineApi.uninstallApp).mockResolvedValue({
    success: true,
    data: { status: 'uninstalled' },
  });
  vi.mocked(engineApi.completeAppCallback).mockResolvedValue({
    success: true,
    data: { action: 'activate', status: 'success' },
  });
  vi.mocked(engineApi.updateMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });
}

async function renderPage(initialEntry = '/projects/proj-1/ai-data-liaison') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/projects/:pid/ai-data-liaison" element={<AIDataLiaisonPage />} />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
  });
}

describe('AIDataLiaisonPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
  });

  it('shows application status and no page-owned setup checklist', async () => {
    await renderPage();

    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.getByText('Application Status')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Setup' })).toHaveAttribute(
      'href',
      '/projects/proj-1/setup'
    );
  });

  it('calls activateApp when Activate is clicked', async () => {
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: 'Activate' }));

    await waitFor(() => {
      expect(engineApi.activateApp).toHaveBeenCalledWith('proj-1', 'ai-data-liaison');
    });
  });

  it('redirects to central when activation returns a redirect url', async () => {
    vi.mocked(engineApi.activateApp).mockResolvedValue({
      success: true,
      data: { redirectUrl: 'https://central.example.com/activate' },
    });

    const originalLocation = window.location;
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    });

    await renderPage();
    fireEvent.click(screen.getByRole('button', { name: 'Activate' }));

    await waitFor(() => {
      expect(window.location.href).toBe('https://central.example.com/activate');
    });

    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  it('shows the auditing card only after activation', async () => {
    await renderPage();
    expect(screen.queryByText('Auditing')).not.toBeInTheDocument();

    setupMocks({ isActivated: true });
    await renderPage();
    expect(screen.getByText('Auditing')).toBeInTheDocument();
  });

  it('completes lifecycle callbacks from the URL', async () => {
    await renderPage(
      '/projects/proj-1/ai-data-liaison?callback_action=activate&callback_state=test-state&callback_app=ai-data-liaison&callback_status=success'
    );

    await waitFor(() => {
      expect(engineApi.completeAppCallback).toHaveBeenCalledWith(
        'proj-1',
        'ai-data-liaison',
        'activate',
        'success',
        'test-state'
      );
    });
  });
});

import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import MCPTunnelPage from '../MCPTunnelPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    activateApp: vi.fn(),
    completeAppCallback: vi.fn(),
    getInstalledApp: vi.fn(),
    getTunnelStatus: vi.fn(),
    uninstallApp: vi.fn(),
  },
}));

const mockToast = vi.fn();
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock('../../contexts/ProjectContext', () => ({
  useProject: () => ({
    urls: { projectsPageUrl: 'https://us.ekaya.ai/projects' },
  }),
}));

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

function setupMocks(options: {
  isActivated?: boolean;
  tunnelStatus?: 'disconnected' | 'connecting' | 'connected' | 'reconnecting';
  publicURL?: string;
} = {}) {
  const {
    isActivated = false,
    tunnelStatus = 'disconnected',
    publicURL,
  } = options;

  vi.mocked(engineApi.getInstalledApp).mockResolvedValue({
    success: true,
    data: {
      id: 'inst-1',
      project_id: 'proj-1',
      app_id: 'mcp-tunnel',
      installed_at: '2026-03-12T10:00:00Z',
      settings: {},
      ...(isActivated ? { activated_at: '2026-03-12T10:05:00Z' } : {}),
    },
  });
  vi.mocked(engineApi.getTunnelStatus).mockResolvedValue({
    success: true,
    data: {
      tunnel_status: tunnelStatus,
      ...(publicURL ? { public_url: publicURL } : {}),
    },
  });
  vi.mocked(engineApi.activateApp).mockResolvedValue({
    success: true,
    data: { status: 'activated' },
  });
  vi.mocked(engineApi.uninstallApp).mockResolvedValue({
    success: true,
    data: { status: 'uninstalled' },
  });
  vi.mocked(engineApi.completeAppCallback).mockResolvedValue({
    success: true,
    data: { action: 'activate', status: 'success' },
  });
}

async function renderPage(initialEntry = '/projects/proj-1/mcp-tunnel') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/projects/:pid/mcp-tunnel" element={<MCPTunnelPage />} />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
  });
}

describe('MCPTunnelPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows activation controls in the status card instead of the old setup checklist', async () => {
    await renderPage();

    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Activate MCP Tunnel' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Setup' })).toHaveAttribute(
      'href',
      '/projects/proj-1/setup'
    );
  });

  it('calls activateApp when the activate button is clicked', async () => {
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: 'Activate MCP Tunnel' }));

    await waitFor(() => {
      expect(engineApi.activateApp).toHaveBeenCalledWith('proj-1', 'mcp-tunnel');
    });
  });

  it('redirects to central when activateApp returns a redirect url', async () => {
    vi.mocked(engineApi.activateApp).mockResolvedValue({
      success: true,
      data: {
        redirectUrl: 'https://central.example.com/billing',
        status: 'pending_activation',
      },
    });

    const originalLocation = window.location;
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    });

    await renderPage();
    fireEvent.click(screen.getByRole('button', { name: 'Activate MCP Tunnel' }));

    await waitFor(() => {
      expect(window.location.href).toBe('https://central.example.com/billing');
    });

    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  it('links to the MCP Server page when the tunnel is connected', async () => {
    setupMocks({
      isActivated: true,
      tunnelStatus: 'connected',
      publicURL: 'https://mcp.ekaya.ai/mcp/proj-1',
    });

    await renderPage();

    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(
      screen.getByRole('link', {
        name: 'Find your MCP URL and setup instructions on the MCP Server page.',
      })
    ).toHaveAttribute('href', '/projects/proj-1/mcp-server');
  });

  it('polls status while the tunnel is still connecting', async () => {
    vi.mocked(engineApi.getInstalledApp).mockResolvedValue({
      success: true,
      data: {
        id: 'inst-1',
        project_id: 'proj-1',
        app_id: 'mcp-tunnel',
        installed_at: '2026-03-12T10:00:00Z',
        activated_at: '2026-03-12T10:05:00Z',
        settings: {},
      },
    });
    vi.mocked(engineApi.getTunnelStatus)
      .mockResolvedValueOnce({
        success: true,
        data: {
          tunnel_status: 'connecting',
          public_url: 'https://mcp.ekaya.ai/mcp/proj-1',
        },
      })
      .mockResolvedValueOnce({
        success: true,
        data: {
          tunnel_status: 'connected',
          public_url: 'https://mcp.ekaya.ai/mcp/proj-1',
        },
      });

    await renderPage();
    expect(screen.getByText('Connecting')).toBeInTheDocument();

    await act(async () => {
      await new Promise((resolve) => {
        window.setTimeout(resolve, 2100);
      });
    });

    await waitFor(() => {
      expect(screen.getByText('Connected')).toBeInTheDocument();
    });
  });
});

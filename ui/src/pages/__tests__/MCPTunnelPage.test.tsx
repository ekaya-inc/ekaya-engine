import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

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
  useToast: () => ({
    toast: mockToast,
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

async function renderMCPTunnelPage(
  initialEntry = '/projects/proj-1/mcp-tunnel'
) {
  const result = render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/projects/:pid/mcp-tunnel" element={<MCPTunnelPage />} />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
  });

  return result;
}

function getConfirmButton() {
  const buttons = screen.getAllByRole('button', {
    name: /uninstall application/i,
  });
  const confirmButton = buttons.at(-1);
  if (!confirmButton) {
    throw new Error('Confirm uninstall button not found');
  }
  return confirmButton;
}

describe('MCPTunnelPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
  });

  it('shows the activation checklist step when the tunnel is installed but not activated', async () => {
    await renderMCPTunnelPage();

    expect(screen.getByText('1. Activate MCP Tunnel')).toBeInTheDocument();
    expect(
      screen.getByText('Activate the application so the engine starts the outbound tunnel client.')
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^activate$/i })).toBeInTheDocument();
  });

  it('calls activateApp when the Activate button is clicked', async () => {
    await renderMCPTunnelPage();

    fireEvent.click(screen.getByRole('button', { name: /^activate$/i }));

    await waitFor(() => {
      expect(engineApi.activateApp).toHaveBeenCalledWith('proj-1', 'mcp-tunnel');
    });
  });

  it('redirects to central when activateApp returns a redirect URL', async () => {
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

    await renderMCPTunnelPage();
    fireEvent.click(screen.getByRole('button', { name: /^activate$/i }));

    await waitFor(() => {
      expect(window.location.href).toBe('https://central.example.com/billing');
    });

    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  it('shows the public URL when the tunnel is connected', async () => {
    setupMocks({
      isActivated: true,
      tunnelStatus: 'connected',
      publicURL: 'https://mcp.ekaya.ai/mcp/proj-1',
    });

    await renderMCPTunnelPage();

    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText('https://mcp.ekaya.ai/mcp/proj-1')).toBeInTheDocument();
    expect(screen.getByText('2. Confirm tunnel connection')).toBeInTheDocument();
    expect(screen.getByText('Tunnel connected and public URL assigned')).toBeInTheDocument();
  });

  it('completes lifecycle callbacks from the URL', async () => {
    await renderMCPTunnelPage(
      '/projects/proj-1/mcp-tunnel?callback_action=activate&callback_state=test-state&callback_app=mcp-tunnel&callback_status=success'
    );

    await waitFor(() => {
      expect(engineApi.completeAppCallback).toHaveBeenCalledWith(
        'proj-1',
        'mcp-tunnel',
        'activate',
        'success',
        'test-state'
      );
    });
  });

  it('redirects to central when uninstallApp returns a redirect URL', async () => {
    vi.mocked(engineApi.uninstallApp).mockResolvedValue({
      success: true,
      data: {
        redirectUrl: 'https://central.example.com/cancel-billing',
        status: 'pending_uninstall',
      },
    });

    const originalLocation = window.location;
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    });

    await renderMCPTunnelPage();

    fireEvent.click(screen.getByRole('button', { name: 'Uninstall Application' }));
    fireEvent.change(screen.getByPlaceholderText('uninstall application'), {
      target: { value: 'uninstall application' },
    });
    fireEvent.click(getConfirmButton());

    await waitFor(() => {
      expect(window.location.href).toBe('https://central.example.com/cancel-billing');
    });

    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });
});

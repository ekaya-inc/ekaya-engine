import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../lib/auth-token';
import engineApi from '../../services/engineApi';
import type { MCPConfigResponse } from '../../types';
import MCPServerPage from '../MCPServerPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    getMCPConfig: vi.fn(),
    getTunnelStatus: vi.fn(),
    updateMCPConfig: vi.fn(),
    getServerStatus: vi.fn(),
  },
}));

const mockToast = vi.fn();
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock('../../lib/auth-token', () => ({
  getUserRoles: vi.fn(() => ['admin']),
}));

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
      addDirectDatabaseAccess: true,
    },
  },
  appNames: {},
};

function setupMocks(options: {
  accessible?: boolean;
  publicTunnelUrl?: string;
} = {}) {
  const { accessible = false, publicTunnelUrl } = options;

  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });
  vi.mocked(engineApi.getServerStatus).mockResolvedValue({
    accessible_for_business_users: accessible,
  } as any);
  vi.mocked(engineApi.getTunnelStatus).mockResolvedValue({
    success: true,
    data: publicTunnelUrl
      ? { tunnel_status: 'connected', public_url: publicTunnelUrl }
      : { tunnel_status: 'disconnected' },
  });
}

async function renderPage() {
  render(
    <MemoryRouter initialEntries={['/projects/proj-1/mcp-server']}>
      <Routes>
        <Route path="/projects/:pid/mcp-server" element={<MCPServerPage />} />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(screen.getByText('MCP Server')).toBeInTheDocument();
  });
}

describe('MCPServerPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
  });

  it('shows deployment guidance without the old setup checklist', async () => {
    await renderPage();

    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.getByText('Deployment')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Configure' })).toHaveAttribute(
      'href',
      '/projects/proj-1/server-setup'
    );
  });

  it('shows full config UI for admin users', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
    await renderPage();
    expect(screen.getByText('Tool Configuration')).toBeInTheDocument();
  });

  it('shows full config UI for data users', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['data']);
    await renderPage();
    expect(screen.getByText('Tool Configuration')).toBeInTheDocument();
  });

  it('shows the address-only view for user sessions', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
    await renderPage();

    expect(screen.getByText('MCP Server Addresses')).toBeInTheDocument();
    expect(screen.queryByText('Tool Configuration')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: /private mcp setup instructions/i })).toHaveAttribute(
      'href',
      expect.stringContaining('/mcp-setup')
    );
  });

  it('shows public and private addresses when a tunnel URL exists', async () => {
    setupMocks({ publicTunnelUrl: 'https://public.example.com/mcp/proj-1' });
    await renderPage();

    expect(screen.getByText('Public Address (Tunnel)')).toBeInTheDocument();
    expect(screen.getByText('Private Address (Server)')).toBeInTheDocument();
    expect(screen.getByText('https://public.example.com/mcp/proj-1')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/mcp/proj-1')).toBeInTheDocument();
  });
});

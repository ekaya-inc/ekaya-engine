import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../lib/auth-token';
import engineApi from '../../services/engineApi';
import type { Datasource, MCPConfigResponse } from '../../types';
import MCPServerPage from '../MCPServerPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    getMCPConfig: vi.fn(),
    getTunnelStatus: vi.fn(),
    listDataSources: vi.fn(),
    updateMCPConfig: vi.fn(),
    getServerStatus: vi.fn(),
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

const mockDatasource: Datasource = {
  datasource_id: 'ds-1',
  project_id: 'proj-1',
  name: 'Test DB',
  type: 'postgres',
  config: {
    host: 'localhost',
    port: 5432,
    name: 'test_db',
    ssl_mode: 'disable',
  },
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
};

const mockMCPConfig: MCPConfigResponse = {
  serverUrl: 'https://example.com/mcp/proj-1',
  userTools: [],
  developerTools: [],
  agentTools: [],
  toolGroups: {},
  appNames: {},
};

const setupMocks = (options: {
  hasDatasource?: boolean;
  publicTunnelUrl?: string;
} = {}) => {
  const {
    hasDatasource = true,
    publicTunnelUrl,
  } = options;

  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });

  vi.mocked(engineApi.getServerStatus).mockResolvedValue(null);
  vi.mocked(engineApi.getTunnelStatus).mockResolvedValue({
    success: true,
    data: publicTunnelUrl
      ? {
          tunnel_status: 'connected',
          public_url: publicTunnelUrl,
        }
      : {
          tunnel_status: 'disconnected',
        },
  });

  vi.mocked(engineApi.listDataSources).mockResolvedValue({
    success: true,
    data: { datasources: hasDatasource ? [mockDatasource] : [] },
  });
};

const renderPage = async () => {
  const result = render(
    <MemoryRouter initialEntries={['/projects/proj-1/mcp-server']}>
      <Routes>
        <Route path="/projects/:pid/mcp-server" element={<MCPServerPage />} />
      </Routes>
    </MemoryRouter>,
  );
  await waitFor(() => {
    expect(screen.queryByText(/loading/i) ?? screen.getByText('MCP Server')).toBeInTheDocument();
  });
  return result;
};

describe('MCPServerPage - Setup Checklist', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // Helper to find an action button within a checklist step by step title
  const getStepButton = (stepTitle: string) => {
    const stepEl = screen.getByText(new RegExp(stepTitle)).closest('[class*="rounded-lg"]');
    return stepEl?.querySelector('a button, button');
  };

  it('datasource step always shows action button', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('Datasource configured')).toBeInTheDocument();
    });
  });

  it('datasource step shows Manage when configured', async () => {
    setupMocks({ hasDatasource: true });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('Datasource configured');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Manage');
    });
  });

  it('datasource step shows Configure when not configured', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('Datasource configured');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Configure');
    });
  });

  it('only has datasource checklist step', async () => {
    setupMocks({ hasDatasource: true });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Datasource configured/)).toBeInTheDocument();
    });
    // These steps should no longer exist
    expect(screen.queryByText(/Schema selected/)).not.toBeInTheDocument();
    expect(screen.queryByText(/AI configured/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Ontology extracted/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Critical Ontology Questions/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Create Pre-Approved Queries/)).not.toBeInTheDocument();
  });

  it('shows "MCP Server is ready" when datasource is configured', async () => {
    setupMocks({ hasDatasource: true });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText('MCP Server is ready')).toBeInTheDocument();
    });
  });

  it('does not show "MCP Server is ready" when datasource is not configured', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Datasource configured/)).toBeInTheDocument();
    });
    expect(screen.queryByText('MCP Server is ready')).not.toBeInTheDocument();
  });
});

describe('MCPServerPage - Role-based access', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows full config UI for admin role', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Setup Checklist')).toBeInTheDocument();
    });
    expect(screen.getByText('MCP Server Addresses')).toBeInTheDocument();
  });

  it('shows full config UI for data role', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['data']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Setup Checklist')).toBeInTheDocument();
    });
    expect(screen.getByText('MCP Server Addresses')).toBeInTheDocument();
  });

  it('shows addresses page for user role without config UI', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });

    // Should NOT show the admin config UI
    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.queryByText('Tool Configuration')).not.toBeInTheDocument();

    // Should show the address card with instructions
    expect(screen.getByText('MCP Server Addresses')).toBeInTheDocument();
    expect(screen.getByText('Private Address (Server)')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/mcp/proj-1')).toBeInTheDocument();
    const setupLink = screen.getByRole('link', { name: /Private MCP Setup Instructions/ });
    expect(setupLink).toHaveAttribute('href', expect.stringContaining('/mcp-setup'));
  });

  it('includes project private MCP URL in setup link for user role', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });

    const setupLink = screen.getByRole('link', { name: /Private MCP Setup Instructions/ });
    expect(setupLink).toHaveAttribute(
      'href',
      expect.stringContaining(encodeURIComponent('https://example.com/mcp/proj-1')),
    );
  });

  it('shows public tunnel and private server addresses when the tunnel URL exists', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
    setupMocks({ publicTunnelUrl: 'https://mcp.dev.ekaya.ai/mcp/proj-1' });
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server Addresses')).toBeInTheDocument();
    });

    expect(screen.getByText('Public Address (Tunnel)')).toBeInTheDocument();
    expect(screen.getByText('https://mcp.dev.ekaya.ai/mcp/proj-1')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Public MCP Setup Instructions' })).toHaveAttribute(
      'href',
      expect.stringContaining(encodeURIComponent('https://mcp.dev.ekaya.ai/mcp/proj-1')),
    );

    expect(screen.getByText('Private Address (Server)')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/mcp/proj-1')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Private MCP Setup Instructions' })).toHaveAttribute(
      'href',
      expect.stringContaining(encodeURIComponent('https://example.com/mcp/proj-1')),
    );
  });

  it('shows instructions page for empty roles (defaults to user)', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue([]);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });

    // Should NOT show admin config UI
    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.queryByText('Tool Configuration')).not.toBeInTheDocument();
    expect(screen.getByText('MCP Server Addresses')).toBeInTheDocument();
  });
});

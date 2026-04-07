import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../lib/auth-token';
import engineApi from '../../services/engineApi';
import type { MCPConfigResponse } from '../../types';
import OntologyForgePage from '../OntologyForgePage';

vi.mock('../../services/engineApi', () => ({
  default: {
    getMCPConfig: vi.fn(),
    updateMCPConfig: vi.fn(),
    completeAppCallback: vi.fn(),
    uninstallApp: vi.fn(),
    activateApp: vi.fn(),
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

const mockMCPConfig: MCPConfigResponse = {
  serverUrl: 'https://example.com/mcp/proj-1',
  userTools: [],
  developerTools: [],
  agentTools: [],
  toolGroups: {
    tools: {
      enabled: true,
      addOntologyMaintenanceTools: true,
      addOntologySuggestions: true,
    },
  },
  appNames: {},
};

function setupMocks(mcpConfig: MCPConfigResponse = mockMCPConfig) {
  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mcpConfig,
  });
  vi.mocked(engineApi.completeAppCallback).mockResolvedValue({
    success: true,
    data: { action: 'activate', status: 'success' },
  });
  vi.mocked(engineApi.activateApp).mockResolvedValue({
    success: true,
    data: {},
  });
}

async function renderPage(initialEntry = '/projects/proj-1/ontology-forge') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/projects/:pid/ontology-forge" element={<OntologyForgePage />} />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
  });
}

describe('OntologyForgePage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
  });

  it('shows tool configuration and no page-owned setup checklist for admin users', async () => {
    await renderPage();

    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.getByText('Tool Configuration')).toBeInTheDocument();
    expect(screen.getByText(/approved query catalog/i)).toBeInTheDocument();
  });

  it('does not activate ontology forge as a page-load side effect', async () => {
    await renderPage();

    expect(engineApi.activateApp).not.toHaveBeenCalled();
  });

  it('completes lifecycle callbacks from the URL', async () => {
    await renderPage(
      '/projects/proj-1/ontology-forge?callback_action=activate&callback_state=test-state&callback_app=ontology-forge&callback_status=success'
    );

    await waitFor(() => {
      expect(engineApi.completeAppCallback).toHaveBeenCalledWith(
        'proj-1',
        'ontology-forge',
        'activate',
        'success',
        'test-state'
      );
    });
  });

  it('shows the instruction view for user-only sessions', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);

    await renderPage();

    expect(screen.getByText('Connect to the MCP Server')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /mcp setup instructions/i })).toHaveAttribute(
      'href',
      expect.stringContaining('/mcp-setup')
    );
    expect(screen.queryByText('Tool Configuration')).not.toBeInTheDocument();
  });
});

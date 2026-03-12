import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { MCPConfigResponse } from '../../types';
import AIAgentsPage from '../AIAgentsPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    getInstalledApp: vi.fn(),
    getMCPConfig: vi.fn(),
    listDataSources: vi.fn(),
    listQueries: vi.fn(),
    listAgents: vi.fn(),
    createAgent: vi.fn(),
    getAgent: vi.fn(),
    getAgentKey: vi.fn(),
    updateAgentQueries: vi.fn(),
    rotateAgentKey: vi.fn(),
    deleteAgent: vi.fn(),
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
  toolGroups: {},
  appNames: {},
  enabledTools: [],
};

function setupPageMocks(options: { agents?: Array<{ id: string; name: string; query_ids: string[]; created_at: string }>; approvedQueries?: Array<{ query_id: string; natural_language_prompt: string }>; } = {}) {
  const {
    agents = [],
    approvedQueries = [
      { query_id: 'query-1', natural_language_prompt: 'Top customers' },
      { query_id: 'query-2', natural_language_prompt: 'Monthly revenue' },
    ],
  } = options;

  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });
  vi.mocked(engineApi.getInstalledApp).mockResolvedValue({
    success: true,
    data: {
      id: 'inst-1',
      project_id: 'proj-1',
      app_id: 'ontology-forge',
      installed_at: '2024-01-01',
      activated_at: '2024-01-02',
      settings: {},
    },
  });
  vi.mocked(engineApi.listDataSources).mockResolvedValue({
    success: true,
    data: {
      datasources: [{ datasource_id: 'ds-1', name: 'Primary', datasource_type: 'postgres' }],
    },
  } as any);
  vi.mocked(engineApi.listQueries).mockResolvedValue({
    success: true,
    data: {
      queries: approvedQueries.map((query) => ({
        ...query,
        project_id: 'proj-1',
        datasource_id: 'ds-1',
        sql_query: 'SELECT 1',
        dialect: 'postgres',
        is_enabled: true,
        allows_modification: false,
        usage_count: 0,
        last_used_at: null,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
        parameters: [],
        status: 'approved',
      })),
    },
  } as any);
  vi.mocked(engineApi.listAgents).mockResolvedValue({
    success: true,
    data: { agents },
  } as any);
  vi.mocked(engineApi.getAgent).mockImplementation(async (_pid: string, agentId: string) => ({
    success: true,
    data: agents.find((agent) => agent.id === agentId) ?? null,
  }) as any);
  vi.mocked(engineApi.getAgentKey).mockResolvedValue({
    success: true,
    data: { key: 'revealed-key', masked: false },
  });
  vi.mocked(engineApi.createAgent).mockResolvedValue({
    success: true,
    data: {
      id: 'agent-1',
      name: 'sales-bot',
      query_ids: ['query-1'],
      created_at: '2024-01-01T00:00:00Z',
      api_key: 'created-key',
    },
  } as any);
  vi.mocked(engineApi.updateAgentQueries).mockResolvedValue({
    success: true,
    data: {
      id: 'agent-1',
      name: 'sales-bot',
      query_ids: ['query-1'],
      created_at: '2024-01-01T00:00:00Z',
    },
  } as any);
  vi.mocked(engineApi.rotateAgentKey).mockResolvedValue({
    success: true,
    data: { api_key: 'rotated-key' },
  });
  vi.mocked(engineApi.deleteAgent).mockResolvedValue({ success: true });
  vi.mocked(engineApi.uninstallApp).mockResolvedValue({ success: true });
}

async function renderPage() {
  render(
    <MemoryRouter initialEntries={['/projects/proj-1/ai-agents']}>
      <Routes>
        <Route path="/projects/:pid/ai-agents" element={<AIAgentsPage />} />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
}

describe('AIAgentsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupPageMocks();
  });

  it('allows opening add agent dialog and cancelling without side effects', async () => {
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /\+ add agent/i }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(engineApi.createAgent).not.toHaveBeenCalled();
  });

  it('requires selecting at least one pre-approved query before save is enabled', async () => {
    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /\+ add agent/i }));
    fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'sales-bot' } });

    const saveButton = screen.getByRole('button', { name: /^save$/i });
    expect(saveButton).toBeDisabled();

    fireEvent.click(screen.getByLabelText(/top customers/i));
    expect(saveButton).toBeEnabled();
  });

  it('shows read-only name on edit and requires exact delete phrase', async () => {
    setupPageMocks({
      agents: [
        {
          id: 'agent-1',
          name: 'sales-bot',
          query_ids: ['query-1'],
          created_at: '2024-01-01T00:00:00Z',
        },
      ],
    });

    await renderPage();

    fireEvent.click(screen.getByRole('button', { name: /edit sales-bot/i }));
    const nameInput = await screen.findByLabelText(/name/i);
    expect(nameInput).toHaveAttribute('readonly');

    fireEvent.click(screen.getByRole('button', { name: /delete sales-bot/i }));
    const deleteButton = screen.getByRole('button', { name: /^delete$/i });
    expect(deleteButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/type delete agent to confirm/i), {
      target: { value: 'delete' },
    });
    expect(deleteButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/type delete agent to confirm/i), {
      target: { value: 'delete agent' },
    });
    expect(deleteButton).toBeEnabled();
  });
});

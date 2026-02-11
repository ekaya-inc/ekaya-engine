import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { Datasource, MCPConfigResponse } from '../../types';
import MCPServerPage from '../MCPServerPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    getMCPConfig: vi.fn(),
    listDataSources: vi.fn(),
    getAIConfig: vi.fn(),
    getSchema: vi.fn(),
    getOntologyDAGStatus: vi.fn(),
    getOntologyQuestionCounts: vi.fn(),
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
  enabledTools: [],
};

const setupMocks = (options: {
  hasDatasource?: boolean;
  hasSelectedTables?: boolean;
  hasAIConfig?: boolean;
  hasOntology?: boolean;
  questionCounts?: { required: number; optional: number } | null;
} = {}) => {
  const {
    hasDatasource = true,
    hasSelectedTables = true,
    hasAIConfig = true,
    hasOntology = true,
    questionCounts = { required: 0, optional: 0 },
  } = options;

  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });

  vi.mocked(engineApi.listDataSources).mockResolvedValue({
    success: true,
    data: { datasources: hasDatasource ? [mockDatasource] : [] },
  });

  vi.mocked(engineApi.getAIConfig).mockResolvedValue(
    hasAIConfig
      ? { success: true, data: { project_id: 'proj-1', config_type: 'anthropic' } }
      : { success: true },
  );

  vi.mocked(engineApi.getSchema).mockResolvedValue({
    success: true,
    data: {
      tables: hasSelectedTables
        ? [{ table_name: 'users', schema_name: 'public', is_selected: true, columns: [] }]
        : [],
      total_tables: hasSelectedTables ? 1 : 0,
      relationships: [],
    },
  });

  if (hasOntology) {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: { dag_id: 'dag-1', status: 'completed', nodes: [] },
    });
  } else {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: null,
    });
  }

  if (questionCounts !== null) {
    vi.mocked(engineApi.getOntologyQuestionCounts).mockResolvedValue({
      success: true,
      data: questionCounts,
    });
  } else {
    vi.mocked(engineApi.getOntologyQuestionCounts).mockRejectedValue(
      new Error('No counts available'),
    );
  }
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

describe('MCPServerPage - Questions checklist item', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows questions checklist item as complete when no required questions', async () => {
    setupMocks({ questionCounts: { required: 0, optional: 2 } });
    await renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/Critical Ontology Questions answered/),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText('All critical questions about your schema have been answered'),
    ).toBeInTheDocument();
  });

  it('shows questions checklist item as pending with count when required questions exist', async () => {
    setupMocks({ questionCounts: { required: 3, optional: 1 } });
    await renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/Critical Ontology Questions answered/),
      ).toBeInTheDocument();
    });
    expect(screen.getByText('3 critical questions need answers')).toBeInTheDocument();
  });

  it('shows singular form for 1 critical question', async () => {
    setupMocks({ questionCounts: { required: 1, optional: 0 } });
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('1 critical question needs answer')).toBeInTheDocument();
    });
  });

  it('shows "MCP Server is ready" when all 5 items are complete', async () => {
    setupMocks({
      hasDatasource: true,
      hasSelectedTables: true,
      hasAIConfig: true,
      hasOntology: true,
      questionCounts: { required: 0, optional: 0 },
    });
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server is ready')).toBeInTheDocument();
    });
  });

  it('does not show "MCP Server is ready" when questions are pending', async () => {
    setupMocks({
      hasDatasource: true,
      hasSelectedTables: true,
      hasAIConfig: true,
      hasOntology: true,
      questionCounts: { required: 2, optional: 0 },
    });
    await renderPage();

    await waitFor(() => {
      expect(
        screen.getByText(/Critical Ontology Questions answered/),
      ).toBeInTheDocument();
    });
    expect(screen.queryByText('MCP Server is ready')).not.toBeInTheDocument();
  });

  it('fetches question counts from the API', async () => {
    setupMocks({ questionCounts: { required: 5, optional: 3 } });
    await renderPage();

    await waitFor(() => {
      expect(engineApi.getOntologyQuestionCounts).toHaveBeenCalledWith('proj-1');
    });
  });

  it('handles question counts API failure gracefully', async () => {
    setupMocks({ questionCounts: null });
    await renderPage();

    // Should still render the page without crashing
    await waitFor(() => {
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });
  });
});

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
    listDataSources: vi.fn(),
    getAIConfig: vi.fn(),
    getSchema: vi.fn(),
    getOntologyDAGStatus: vi.fn(),
    getOntologyQuestionCounts: vi.fn(),
    updateMCPConfig: vi.fn(),
    listQueries: vi.fn(),
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
  hasApprovedQueries?: boolean;
} = {}) => {
  const {
    hasDatasource = true,
    hasSelectedTables = true,
    hasAIConfig = true,
    hasOntology = true,
    questionCounts = { required: 0, optional: 0 },
    hasApprovedQueries = false,
  } = options;

  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mockMCPConfig,
  });

  vi.mocked(engineApi.getServerStatus).mockResolvedValue(null);

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

  vi.mocked(engineApi.listQueries).mockResolvedValue({
    success: true,
    data: {
      queries: hasApprovedQueries
        ? [{
            query_id: 'q-1',
            project_id: 'proj-1',
            datasource_id: 'ds-1',
            natural_language_prompt: 'Test Query',
            additional_context: null,
            sql_query: 'SELECT 1',
            dialect: 'postgres',
            is_enabled: true,
            allows_modification: false,
            usage_count: 0,
            last_used_at: null,
            created_at: '2024-01-01',
            updated_at: '2024-01-01',
            parameters: [],
            status: 'approved',
          }]
        : [],
    },
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

// These tests verify the MCP Server checklist step prerequisites match the
// dashboard tile enable/disable logic in ProjectDashboard.tsx:
//   - Datasource tile: always enabled
//   - Schema tile: disabled when !isConnected
//   - AI config: disabled when !isConnected || !hasSelectedTables
//   - Intelligence tiles (Ontology, etc.): disabled when !isConnected || !hasSelectedTables || !activeAIConfig
//   - Questions: disabled when ontology not complete
//
// The checklist enforces the same gates by only setting item.link (which renders
// the action button) when prerequisites are met.

describe('MCPServerPage - Checklist step prerequisites match dashboard tiles', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // Helper to find an action button within a checklist step by step title
  const getStepButton = (stepTitle: string) => {
    const stepEl = screen.getByText(new RegExp(stepTitle)).closest('[class*="rounded-lg"]');
    return stepEl?.querySelector('a button, button');
  };

  // -- Step 1: Datasource -- always has an action button
  it('datasource step always shows action button', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('Datasource configured')).toBeInTheDocument();
    });
  });

  // -- Step 2: Schema -- requires datasource (matches Schema tile: disabled when !isConnected)
  it('schema step shows Configure button when datasource exists', async () => {
    setupMocks({ hasDatasource: true, hasSelectedTables: false });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('Schema selected')).toBeInTheDocument();
    });
  });

  it('schema step hides action button when no datasource', async () => {
    setupMocks({ hasDatasource: false, hasSelectedTables: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Schema selected/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/Schema selected/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  // -- Step 3: Pre-Approved Queries (optional) -- requires datasource
  it('queries step shows Configure button when datasource exists but no queries', async () => {
    setupMocks({ hasDatasource: true, hasApprovedQueries: false });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('Create Pre-Approved Queries');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Configure');
    });
  });

  it('queries step shows Manage button when approved queries exist', async () => {
    setupMocks({ hasDatasource: true, hasApprovedQueries: true });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('Create Pre-Approved Queries');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Manage');
    });
  });

  it('queries step hides action button when no datasource', async () => {
    setupMocks({ hasDatasource: false, hasApprovedQueries: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Create Pre-Approved Queries/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/Create Pre-Approved Queries/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  // -- Step 4: AI configured -- requires datasource + schema (matches AI config prerequisite)
  it('AI step shows Configure button when datasource and schema exist', async () => {
    setupMocks({ hasDatasource: true, hasSelectedTables: true, hasAIConfig: false });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('AI configured')).toBeInTheDocument();
    });
  });

  it('AI step hides action button when no schema selected', async () => {
    setupMocks({ hasDatasource: true, hasSelectedTables: false, hasAIConfig: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/AI configured/)).toBeInTheDocument();
    });
    expect(screen.getByText('Configure datasource and select schema first')).toBeInTheDocument();
    const stepEl = screen.getByText(/AI configured/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  it('AI step hides action button when no datasource', async () => {
    setupMocks({ hasDatasource: false, hasSelectedTables: false, hasAIConfig: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/AI configured/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/AI configured/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  // -- Step 5: Ontology -- requires datasource + schema + AI (matches Intelligence tile logic)
  it('ontology step shows Configure button when datasource, schema, and AI are all configured', async () => {
    setupMocks({ hasDatasource: true, hasSelectedTables: true, hasAIConfig: true, hasOntology: false });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('Ontology extracted')).toBeInTheDocument();
    });
  });

  it('ontology step hides action button when AI is not configured', async () => {
    setupMocks({ hasDatasource: true, hasSelectedTables: true, hasAIConfig: false, hasOntology: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Ontology extracted/)).toBeInTheDocument();
    });
    expect(screen.getByText('Configure datasource, select schema, and configure AI first')).toBeInTheDocument();
    const stepEl = screen.getByText(/Ontology extracted/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  it('ontology step hides action button when schema is not selected', async () => {
    setupMocks({ hasDatasource: true, hasSelectedTables: false, hasAIConfig: true, hasOntology: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Ontology extracted/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/Ontology extracted/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  it('ontology step hides action button when no datasource', async () => {
    setupMocks({ hasDatasource: false, hasSelectedTables: false, hasAIConfig: false, hasOntology: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Ontology extracted/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/Ontology extracted/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });

  // -- Step 6: Questions -- requires ontology complete
  it('questions step shows Answer button when ontology is complete and questions exist', async () => {
    setupMocks({ hasOntology: true, questionCounts: { required: 2, optional: 0 } });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('Critical Ontology Questions answered')).toBeInTheDocument();
    });
  });

  it('questions step hides action button when ontology is not complete', async () => {
    setupMocks({ hasOntology: false, questionCounts: null });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Critical Ontology Questions answered/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/Critical Ontology Questions answered/).closest('[class*="rounded-lg"]');
    const btn = stepEl?.querySelector('a button');
    expect(btn).toBeNull();
  });
});

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

  it('shows "MCP Server is ready" when all required items are complete (optional queries step ignored)', async () => {
    setupMocks({
      hasDatasource: true,
      hasSelectedTables: true,
      hasAIConfig: true,
      hasOntology: true,
      questionCounts: { required: 0, optional: 0 },
      hasApprovedQueries: false,
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
    expect(screen.getByText('Your MCP Server URL')).toBeInTheDocument();
  });

  it('shows full config UI for data role', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['data']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Setup Checklist')).toBeInTheDocument();
    });
    expect(screen.getByText('Your MCP Server URL')).toBeInTheDocument();
  });

  it('shows instructions page for user role instead of config UI', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });

    // Should NOT show the admin config UI
    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.queryByText('Your MCP Server URL')).not.toBeInTheDocument();

    // Should show instructions with a link to MCP setup
    expect(screen.getByText(/MCP Setup Instructions/)).toBeInTheDocument();
    const setupLink = screen.getByRole('link', { name: /MCP Setup Instructions/ });
    expect(setupLink).toHaveAttribute('href', expect.stringContaining('/mcp-setup'));
  });

  it('includes project MCP URL in setup link for user role', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });

    const setupLink = screen.getByRole('link', { name: /MCP Setup Instructions/ });
    expect(setupLink).toHaveAttribute(
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
    expect(screen.queryByText('Your MCP Server URL')).not.toBeInTheDocument();
  });
});

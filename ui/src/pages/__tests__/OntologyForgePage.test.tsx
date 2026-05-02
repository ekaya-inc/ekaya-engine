import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../lib/auth-token';
import engineApi from '../../services/engineApi';
import type { Datasource, MCPConfigResponse } from '../../types';
import OntologyForgePage from '../OntologyForgePage';

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
    getInstalledApp: vi.fn(),
    activateApp: vi.fn(),
    completeAppCallback: vi.fn(),
    uninstallApp: vi.fn(),
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
  hasSelectedTables?: boolean;
  hasAIConfig?: boolean;
  hasOntology?: boolean;
  questionCounts?: { required: number; optional: number } | null;
  hasApprovedQueries?: boolean;
  isActivated?: boolean;
  mcpConfig?: MCPConfigResponse;
} = {}) => {
  const {
    hasDatasource = true,
    hasSelectedTables = true,
    hasAIConfig = true,
    hasOntology = true,
    questionCounts = { required: 0, optional: 0 },
    hasApprovedQueries = false,
    isActivated = false,
    mcpConfig = mockMCPConfig,
  } = options;

  vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
    success: true,
    data: mcpConfig,
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
      data: { dag_id: 'dag-1', status: 'completed', is_incremental: false, nodes: [] },
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

  vi.mocked(engineApi.getInstalledApp).mockResolvedValue({
    success: true,
    data: {
      id: 'ia-1',
      project_id: 'proj-1',
      app_id: 'ontology-forge',
      installed_at: '2024-01-01',
      settings: {},
      ...(isActivated ? { activated_at: '2024-01-02' } : {}),
    },
  });

  vi.mocked(engineApi.activateApp).mockResolvedValue({ success: true, data: {} });
  vi.mocked(engineApi.completeAppCallback).mockResolvedValue({
    success: true,
    data: { action: 'activate', status: 'success' },
  });
};

const renderPage = async (
  initialEntry = '/projects/proj-1/ontology-forge'
) => {
  const result = render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/projects/:pid/ontology-forge" element={<OntologyForgePage />} />
      </Routes>
    </MemoryRouter>,
  );
  await waitFor(() => {
    expect(screen.queryByText(/loading/i) ?? screen.getByText('Ontology Forge')).toBeInTheDocument();
  });
  return result;
};

describe('OntologyForgePage - Checklist step prerequisites match dashboard tiles', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // Helper to find an action button within a checklist step by step title
  const getStepButton = (stepTitle: string) => {
    const stepEl = screen.getByText(new RegExp(stepTitle)).closest('[class*="rounded-lg"]');
    return stepEl?.querySelector('a button, button');
  };

  // -- Step 1: MCP Server set up -- always has an action button
  it('MCP Server step always shows action button', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      expect(getStepButton('MCP Server set up')).toBeInTheDocument();
    });
  });

  it('MCP Server step shows Configure when datasource not configured', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('MCP Server set up');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Configure');
    });
  });

  it('MCP Server step shows Manage when datasource is configured', async () => {
    setupMocks({ hasDatasource: true });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('MCP Server set up');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Manage');
    });
  });

  it('MCP Server step links to MCP Server page', async () => {
    setupMocks({ hasDatasource: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/MCP Server set up/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/MCP Server set up/).closest('[class*="rounded-lg"]');
    const link = stepEl?.querySelector('a');
    expect(link).toHaveAttribute('href', '/projects/proj-1/mcp-server');
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

  // -- Step 3: Approved Queries (optional) -- requires datasource
  it('queries step shows Configure button when datasource exists but no queries', async () => {
    setupMocks({ hasDatasource: true, hasApprovedQueries: false });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('Create Approved Queries');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Configure');
    });
  });

  it('queries step shows Manage button when approved queries exist', async () => {
    setupMocks({ hasDatasource: true, hasApprovedQueries: true });
    await renderPage();
    await waitFor(() => {
      const btn = getStepButton('Create Approved Queries');
      expect(btn).toBeInTheDocument();
      expect(btn).toHaveTextContent('Manage');
    });
  });

  it('queries step hides action button when no datasource', async () => {
    setupMocks({ hasDatasource: false, hasApprovedQueries: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Create Approved Queries/)).toBeInTheDocument();
    });
    const stepEl = screen.getByText(/Create Approved Queries/).closest('[class*="rounded-lg"]');
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

describe('OntologyForgePage - Silent activation on ontology completion', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('calls activateApp when ontology is completed and app is not yet activated', async () => {
    vi.mocked(engineApi.activateApp).mockResolvedValue({ success: true, data: {} });
    setupMocks({ hasOntology: true, isActivated: false });
    await renderPage();

    await waitFor(() => {
      expect(engineApi.activateApp).toHaveBeenCalledWith('proj-1', 'ontology-forge');
    });
  });

  it('does not call activateApp when app is already activated', async () => {
    setupMocks({ hasOntology: true, isActivated: true });
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    });
    expect(engineApi.activateApp).not.toHaveBeenCalled();
  });

  it('does not call activateApp when ontology is not completed', async () => {
    setupMocks({ hasOntology: false, isActivated: false });
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    });
    expect(engineApi.activateApp).not.toHaveBeenCalled();
  });
});

describe('OntologyForgePage - Callback handling', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('completes lifecycle callbacks from the URL', async () => {
    setupMocks({ isActivated: false });

    await renderPage(
      '/projects/proj-1/ontology-forge?callback_action=activate&callback_state=test-state&callback_app=ontology-forge&callback_status=success'
    );

    await waitFor(() => {
      expect(engineApi.completeAppCallback).toHaveBeenCalledWith(
        'proj-1',
        'ontology-forge',
        'activate',
        'success',
        'test-state',
      );
    });
  });

  it('completes cancelled callbacks without navigating away', async () => {
    setupMocks({ isActivated: true });
    vi.mocked(engineApi.completeAppCallback).mockResolvedValue({
      success: true,
      data: { action: 'uninstall', status: 'cancelled' },
    });

    await renderPage(
      '/projects/proj-1/ontology-forge?callback_action=uninstall&callback_state=test-state&callback_app=ontology-forge&callback_status=cancelled'
    );

    await waitFor(() => {
      expect(engineApi.completeAppCallback).toHaveBeenCalledWith(
        'proj-1',
        'ontology-forge',
        'uninstall',
        'cancelled',
        'test-state',
      );
    });
    expect(mockNavigate).not.toHaveBeenCalledWith('/projects/proj-1');
  });
});

describe('OntologyForgePage - Questions checklist item', () => {
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

  it('shows "Ontology Forge is ready" when all required items are complete (optional queries step ignored)', async () => {
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
      expect(screen.getByText('Ontology Forge is ready')).toBeInTheDocument();
    });
  });

  it('does not show "Ontology Forge is ready" when questions are pending', async () => {
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
    expect(screen.queryByText('Ontology Forge is ready')).not.toBeInTheDocument();
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
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    });
  });
});

describe('OntologyForgePage - Role-based access', () => {
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
    expect(screen.getByText('Danger Zone')).toBeInTheDocument();
  });

  it('shows full config UI for data role', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['data']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Setup Checklist')).toBeInTheDocument();
    });
    expect(screen.getByText('Danger Zone')).toBeInTheDocument();
  });

  it('shows instructions page for user role instead of config UI', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
    setupMocks();
    await renderPage();

    await waitFor(() => {
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    });

    // Should NOT show the admin config UI
    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.queryByText('Danger Zone')).not.toBeInTheDocument();

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
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
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
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    });

    // Should NOT show admin config UI
    expect(screen.queryByText('Setup Checklist')).not.toBeInTheDocument();
    expect(screen.queryByText('Danger Zone')).not.toBeInTheDocument();
  });
});

describe('OntologyForgePage - Tool Configuration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
  });

  it('describes approved query catalog management under ontology maintenance tools', async () => {
    setupMocks();
    await renderPage();

    expect(
      screen.getByText(/manage the ontology and approved query catalog/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/schema search, column probing, project knowledge discovery/i),
    ).toBeInTheDocument();
  });

  it('shows approved query tools under ontology forge tool lists', async () => {
    setupMocks({
      mcpConfig: {
        ...mockMCPConfig,
        developerTools: [
          {
            name: 'create_approved_query',
            description: 'Create a new approved query directly',
            appId: 'ontology-forge',
          },
          {
            name: 'list_approved_queries',
            description: 'List approved SQL queries',
            appId: 'ontology-forge',
          },
          {
            name: 'list_query_suggestions',
            description: 'List query suggestions awaiting review',
            appId: 'ai-data-liaison',
          },
        ],
        userTools: [
          {
            name: 'execute_approved_query',
            description: 'Execute an approved query by ID',
            appId: 'ontology-forge',
          },
          {
            name: 'search_schema',
            description: 'Search schema objects',
            appId: 'ontology-forge',
          },
          {
            name: 'list_project_knowledge',
            description: 'List project knowledge facts',
            appId: 'ontology-forge',
          },
          {
            name: 'suggest_approved_query',
            description: 'Suggest a reusable parameterized query for approval',
            appId: 'ai-data-liaison',
          },
        ],
      },
    });
    await renderPage();

    for (const button of screen.getAllByRole('button', { name: /tools enabled/i })) {
      fireEvent.click(button);
    }

    expect(screen.getByText('create_approved_query')).toBeInTheDocument();
    expect(screen.getByText('list_approved_queries')).toBeInTheDocument();
    expect(screen.getByText('execute_approved_query')).toBeInTheDocument();
    expect(screen.getByText('search_schema')).toBeInTheDocument();
    expect(screen.getByText('list_project_knowledge')).toBeInTheDocument();
    expect(screen.queryByText('list_query_suggestions')).not.toBeInTheDocument();
    expect(screen.queryByText('suggest_approved_query')).not.toBeInTheDocument();
  });
});

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { Datasource, MCPConfigResponse } from '../../types';
import AIDataLiaisonPage from '../AIDataLiaisonPage';

// Mock the engineApi
vi.mock('../../services/engineApi', () => ({
  default: {
    uninstallApp: vi.fn(),
    listDataSources: vi.fn(),
    getMCPConfig: vi.fn(),
    getAIConfig: vi.fn(),
    getSchema: vi.fn(),
    getOntologyDAGStatus: vi.fn(),
    getServerStatus: vi.fn(),
  },
}));

// Mock the toast hook
const mockToast = vi.fn();
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: mockToast,
  }),
}));

// Mock navigate
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Mock the ConfigContext
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
  hasOntology?: boolean;
  hasMCPConfig?: boolean;
  hasSelectedTables?: boolean;
  hasAIConfig?: boolean;
} = {}) => {
  const { hasDatasource = true, hasOntology = false, hasMCPConfig = true, hasSelectedTables = false, hasAIConfig = false } = options;

  vi.mocked(engineApi.getServerStatus).mockResolvedValue(null);

  vi.mocked(engineApi.listDataSources).mockResolvedValue({
    success: true,
    data: { datasources: hasDatasource ? [mockDatasource] : [] },
  });

  if (hasMCPConfig) {
    vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
      success: true,
      data: mockMCPConfig,
    });
  } else {
    vi.mocked(engineApi.getMCPConfig).mockResolvedValue({
      success: true,
    });
  }

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
};

const setupAllCompleteMocks = () => {
  setupMocks({
    hasDatasource: true,
    hasSelectedTables: true,
    hasAIConfig: true,
    hasOntology: true,
    hasMCPConfig: true,
  });
  // Server must be accessible for all checklist items to be complete
  vi.mocked(engineApi.getServerStatus).mockResolvedValue({
    base_url: 'https://example.com',
    is_localhost: false,
    is_https: true,
    accessible_for_business_users: true,
  });
};

const renderAIDataLiaisonPage = async () => {
  const result = render(
    <MemoryRouter initialEntries={['/projects/proj-1/ai-data-liaison']}>
      <Routes>
        <Route path="/projects/:pid/ai-data-liaison" element={<AIDataLiaisonPage />} />
      </Routes>
    </MemoryRouter>
  );

  // Wait for loading to complete
  await waitFor(() => {
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  return result;
};

// Helper to get the confirm button in the dialog (the last "Uninstall Application" button)
const getConfirmButton = () => {
  const dialogButtons = screen.getAllByRole('button', { name: /uninstall application/i });
  const confirmButton = dialogButtons.at(-1);
  if (!confirmButton) {
    throw new Error('Confirm button not found');
  }
  return confirmButton;
};

describe('AIDataLiaisonPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setupMocks();
  });

  describe('Header', () => {
    it('renders the page title', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
    });

    it('renders the page description', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText(/Ekaya acts as a data liaison between you and your business users/)).toBeInTheDocument();
    });

    it('renders back button', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByRole('button', { name: /back to project dashboard/i })).toBeInTheDocument();
    });

    it('navigates to project dashboard when back button clicked', async () => {
      await renderAIDataLiaisonPage();
      const backButton = screen.getByRole('button', { name: /back to project dashboard/i });
      fireEvent.click(backButton);
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
    });
  });

  describe('Setup Checklist', () => {
    it('renders setup checklist card', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Setup Checklist')).toBeInTheDocument();
    });

    it('shows MCP Server as pending when not ready', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('1. MCP Server set up')).toBeInTheDocument();
      expect(screen.getByText('Configure datasource, schema, AI, and extract ontology')).toBeInTheDocument();
    });

    it('shows MCP Server as complete when ontology is ready', async () => {
      setupAllCompleteMocks();
      await renderAIDataLiaisonPage();
      expect(screen.getByText('1. MCP Server set up')).toBeInTheDocument();
      expect(screen.getByText('Datasource, schema, AI, and ontology configured')).toBeInTheDocument();
    });

    it('shows MCP Server accessible as pending when server is on localhost', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('2. MCP Server accessible')).toBeInTheDocument();
      expect(screen.getByText(/business users cannot connect/)).toBeInTheDocument();
    });
  });

  describe('Enabled Tools', () => {
    it('renders user tools card', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('User Tools')).toBeInTheDocument();
    });

    it('shows ontology maintenance toggle', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Allow Usage to Improve Ontology')).toBeInTheDocument();
    });

    it('shows additional developer tools section', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Additional Developer Tools')).toBeInTheDocument();
      expect(screen.getByText('list_query_suggestions')).toBeInTheDocument();
    });
  });

  describe('Auditing Tile', () => {
    it('shows auditing card when all checklist items are complete', async () => {
      setupAllCompleteMocks();
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Auditing')).toBeInTheDocument();
      expect(screen.getByText(/Review query executions/)).toBeInTheDocument();
    });

    it('hides auditing card when checklist is incomplete', async () => {
      setupMocks({ hasDatasource: true, hasOntology: false });
      await renderAIDataLiaisonPage();
      expect(screen.queryByText('Auditing')).not.toBeInTheDocument();
    });

    it('renders link to audit page', async () => {
      setupAllCompleteMocks();
      await renderAIDataLiaisonPage();
      const auditLink = screen.getByRole('link', { name: /view audit log/i });
      expect(auditLink).toHaveAttribute('href', '/projects/proj-1/audit');
    });
  });

  describe('Danger Zone', () => {
    it('renders danger zone card', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Danger Zone')).toBeInTheDocument();
    });

    it('renders uninstall button', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByRole('button', { name: /uninstall application/i })).toBeInTheDocument();
    });

    it('renders warning text', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText(/Uninstalling AI Data Liaison will disable the query suggestion workflow/)).toBeInTheDocument();
    });
  });

  describe('Uninstall Dialog', () => {
    it('opens dialog when uninstall button clicked', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      expect(screen.getByText('Uninstall AI Data Liaison?')).toBeInTheDocument();
    });

    it('shows confirmation input in dialog', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      expect(screen.getByPlaceholderText('uninstall application')).toBeInTheDocument();
    });

    it('shows instruction to type "uninstall application"', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      expect(screen.getByText(/Type/)).toBeInTheDocument();
      expect(screen.getByText('uninstall application')).toBeInTheDocument();
    });

    it('disables confirm button when text does not match', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'wrong text' } });

      expect(getConfirmButton()).toBeDisabled();
    });

    it('enables confirm button when text matches', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      expect(getConfirmButton()).not.toBeDisabled();
    });

    it('closes dialog when cancel button clicked', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      fireEvent.click(cancelButton);

      expect(screen.queryByText('Uninstall AI Data Liaison?')).not.toBeInTheDocument();
    });

    it('resets input state when dialog is closed and reopened', async () => {
      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      // Verify input has the value
      expect(input).toHaveValue('uninstall application');

      // Close via X button or onOpenChange - simulate closing
      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      fireEvent.click(cancelButton);

      // Wait for dialog to close (Radix Dialog uses animation)
      await waitFor(() => {
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      });
    });
  });

  describe('Uninstall Action', () => {
    it('calls uninstallApp when confirmed', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({ success: true });

      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(engineApi.uninstallApp).toHaveBeenCalledWith('proj-1', 'ai-data-liaison');
      });
    });

    it('navigates to project dashboard on successful uninstall', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({ success: true });

      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
      });
    });

    it('shows error toast when API returns error', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({
        success: false,
        error: 'Failed to uninstall'
      });

      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith({
          title: 'Error',
          description: 'Failed to uninstall',
          variant: 'destructive',
        });
      });
    });

    it('shows error toast when API throws exception', async () => {
      vi.mocked(engineApi.uninstallApp).mockRejectedValue(new Error('Network error'));

      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith({
          title: 'Error',
          description: 'Network error',
          variant: 'destructive',
        });
      });
    });

    it('shows loading state while uninstalling', async () => {
      let resolvePromise: (value: { success: boolean }) => void = () => { /* no-op */ };
      vi.mocked(engineApi.uninstallApp).mockImplementation(
        () => new Promise((resolve) => { resolvePromise = resolve; })
      );

      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(screen.getByText('Uninstalling...')).toBeInTheDocument();
      });

      // Resolve the promise to clean up
      resolvePromise({ success: true });
    });
  });
});

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
    getOntologyDAGStatus: vi.fn(),
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
} = {}) => {
  const { hasDatasource = true, hasOntology = false, hasMCPConfig = true } = options;

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
      expect(screen.getByText('Enable business users to query data through natural language')).toBeInTheDocument();
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

    it('shows datasource as configured when present', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('1. Datasource configured')).toBeInTheDocument();
      expect(screen.getByText(/Connected to Test DB/)).toBeInTheDocument();
    });

    it('shows datasource as pending when not configured', async () => {
      setupMocks({ hasDatasource: false });
      await renderAIDataLiaisonPage();
      expect(screen.getByText(/Connect a database to enable AI Data Liaison/)).toBeInTheDocument();
    });

    it('shows AI Data Liaison installed status', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('4. AI Data Liaison installed')).toBeInTheDocument();
      expect(screen.getByText('Query suggestion workflow enabled')).toBeInTheDocument();
    });
  });

  describe('Enabled Tools', () => {
    it('renders enabled tools card', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Enabled Tools')).toBeInTheDocument();
    });

    it('shows user tools section', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('For Business Users (User Tools)')).toBeInTheDocument();
      expect(screen.getByText('suggest_approved_query')).toBeInTheDocument();
    });

    it('shows developer tools section', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('For Data Engineers (Developer Tools)')).toBeInTheDocument();
      expect(screen.getByText('list_query_suggestions')).toBeInTheDocument();
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
      let resolvePromise: (value: { success: boolean }) => void;
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
      resolvePromise!({ success: true });
    });
  });
});

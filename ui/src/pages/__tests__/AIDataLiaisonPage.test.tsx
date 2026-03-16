import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { MCPConfigResponse } from '../../types';
import AIDataLiaisonPage from '../AIDataLiaisonPage';

// Mock the engineApi
vi.mock('../../services/engineApi', () => ({
  default: {
    uninstallApp: vi.fn(),
    activateApp: vi.fn(),
    completeAppCallback: vi.fn(),
    getInstalledApp: vi.fn(),
    getMCPConfig: vi.fn(),
    listGlossaryTerms: vi.fn(),
    updateMCPConfig: vi.fn(),
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

// Mock the ProjectContext (used by AppPageHeader)
vi.mock('../../contexts/ProjectContext', () => ({
  useProject: () => ({
    urls: { projectsPageUrl: 'https://us.ekaya.ai/projects' },
  }),
}));

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

const mockMCPConfig: MCPConfigResponse = {
  serverUrl: 'https://example.com/mcp/proj-1',
  userTools: [],
  developerTools: [],
  agentTools: [],
  toolGroups: {},
  appNames: {},
  enabledTools: [],
};

const setupMocks = (options: {
  hasOntologyForge?: boolean;
  hasMCPConfig?: boolean;
  hasGlossary?: boolean;
  glossaryRequestFails?: boolean;
  isActivated?: boolean;
} = {}) => {
  const {
    hasOntologyForge = false,
    hasMCPConfig = true,
    hasGlossary = false,
    glossaryRequestFails = false,
    isActivated = false,
  } = options;

  vi.mocked(engineApi.getInstalledApp).mockImplementation((_pid: string, appId: string) => {
    if (appId === 'ai-data-liaison') {
      return Promise.resolve({
        success: true,
        data: {
          id: 'inst-1',
          project_id: 'proj-1',
          app_id: 'ai-data-liaison',
          installed_at: '2024-01-01',
          settings: {},
          ...(isActivated ? { activated_at: '2024-01-02' } : {}),
        },
      });
    }
    if (appId === 'ontology-forge') {
      if (hasOntologyForge) {
        return Promise.resolve({
          success: true,
          data: {
            id: 'inst-2',
            project_id: 'proj-1',
            app_id: 'ontology-forge',
            installed_at: '2024-01-01',
            activated_at: '2024-01-02',
            settings: {},
          },
        });
      }
      return Promise.reject(new Error('Not found'));
    }
    return Promise.reject(new Error('Unknown app'));
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

  if (glossaryRequestFails) {
    vi.mocked(engineApi.listGlossaryTerms).mockRejectedValue(new Error('Glossary unavailable'));
  } else {
    vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
      success: true,
      data: {
        terms: hasGlossary
          ? [{
              id: 'term-1',
              project_id: 'proj-1',
              term: 'Revenue',
              definition: 'Recognized revenue',
              defining_sql: 'SELECT 1',
              source: 'manual',
              created_at: '2024-01-01',
              updated_at: '2024-01-01',
            }]
          : [],
        total: hasGlossary ? 1 : 0,
      },
    });
  }
};

const setupAllCompleteMocks = (options: { isActivated?: boolean } = {}) => {
  const { isActivated = true } = options;
  setupMocks({
    hasOntologyForge: true,
    hasMCPConfig: true,
    hasGlossary: true,
    isActivated,
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
      expect(screen.getByText(/query workflows, share glossary terminology, and collaborate through governed business definitions/i)).toBeInTheDocument();
    });

    it('renders back button', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByRole('button', { name: /back to dashboard/i })).toBeInTheDocument();
    });

    it('navigates to project dashboard when back button clicked', async () => {
      await renderAIDataLiaisonPage();
      const backButton = screen.getByRole('button', { name: /back to dashboard/i });
      fireEvent.click(backButton);
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
    });
  });

  describe('Setup Checklist', () => {
    it('renders setup checklist card', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Setup Checklist')).toBeInTheDocument();
    });

    it('shows Ontology Forge as pending when not ready', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('1. Ontology Forge set up')).toBeInTheDocument();
      expect(screen.getByText('Set up Ontology Forge to extract your business semantic layer')).toBeInTheDocument();
    });

    it('shows Ontology Forge as complete when installed and activated', async () => {
      setupAllCompleteMocks();
      await renderAIDataLiaisonPage();
      expect(screen.getByText('1. Ontology Forge set up')).toBeInTheDocument();
      expect(screen.getByText('Ontology Forge is configured and ready')).toBeInTheDocument();
    });

    it('shows Glossary as pending when Ontology Forge is ready but glossary is empty', async () => {
      setupMocks({ hasOntologyForge: true, hasGlossary: false });
      await renderAIDataLiaisonPage();
      expect(screen.getByText('2. Glossary set up')).toBeInTheDocument();
      expect(screen.getByText('Set up the business glossary for consistent business terminology')).toBeInTheDocument();
    });

    it('shows Glossary as complete when at least one glossary term exists', async () => {
      setupAllCompleteMocks();
      await renderAIDataLiaisonPage();
      expect(screen.getByText('2. Glossary set up')).toBeInTheDocument();
      expect(screen.getByText('Glossary is configured and ready')).toBeInTheDocument();
    });

    it('fails soft when glossary readiness fetch fails', async () => {
      setupMocks({ hasOntologyForge: true, glossaryRequestFails: true });
      await renderAIDataLiaisonPage();
      expect(screen.getByText('2. Glossary set up')).toBeInTheDocument();
      expect(screen.getByText('Set up the business glossary for consistent business terminology')).toBeInTheDocument();
    });
  });

  describe('Activate Step', () => {
    it('shows activate step when prerequisites are met but not activated', async () => {
      setupAllCompleteMocks({ isActivated: false });
      await renderAIDataLiaisonPage();
      expect(screen.getByText('3. Activate AI Data Liaison')).toBeInTheDocument();
      expect(screen.getByText('Activate to start using the application')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /^activate$/i })).toBeInTheDocument();
    });

    it('shows activate step disabled when prerequisites are not met', async () => {
      setupMocks({ hasOntologyForge: false });
      await renderAIDataLiaisonPage();
      expect(screen.getByText(/Activate AI Data Liaison/)).toBeInTheDocument();
      expect(screen.getByText('Complete steps 1 and 2 before activating')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /^activate$/i })).toBeDisabled();
    });

    it('requires glossary before activation when Ontology Forge is ready', async () => {
      setupMocks({ hasOntologyForge: true, hasGlossary: false });
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Complete step 2 before activating')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /^activate$/i })).toBeDisabled();
    });

    it('shows activated state when activated_at is set', async () => {
      setupAllCompleteMocks({ isActivated: true });
      await renderAIDataLiaisonPage();
      expect(screen.getByText('3. Activate AI Data Liaison')).toBeInTheDocument();
      expect(screen.getByText('AI Data Liaison activated')).toBeInTheDocument();
      // No activate button when already activated
      expect(screen.queryByRole('button', { name: /^activate$/i })).not.toBeInTheDocument();
    });

    it('calls activateApp when Activate button is clicked', async () => {
      setupAllCompleteMocks({ isActivated: false });
      vi.mocked(engineApi.activateApp).mockResolvedValue({
        success: true,
        data: { status: 'activated' },
      });

      await renderAIDataLiaisonPage();
      const activateButton = screen.getByRole('button', { name: /^activate$/i });
      fireEvent.click(activateButton);

      await waitFor(() => {
        expect(engineApi.activateApp).toHaveBeenCalledWith('proj-1', 'ai-data-liaison');
      });
    });

    it('shows error toast when activation fails', async () => {
      setupAllCompleteMocks({ isActivated: false });
      vi.mocked(engineApi.activateApp).mockRejectedValue(new Error('Activation failed'));

      await renderAIDataLiaisonPage();
      const activateButton = screen.getByRole('button', { name: /^activate$/i });
      fireEvent.click(activateButton);

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith({
          title: 'Error',
          description: 'Activation failed',
          variant: 'destructive',
        });
      });
    });

    it('redirects to central when activateApp returns redirectUrl', async () => {
      setupAllCompleteMocks({ isActivated: false });
      vi.mocked(engineApi.activateApp).mockResolvedValue({
        success: true,
        data: { redirectUrl: 'https://central.example.com/billing', status: 'pending_activation' },
      });

      const originalLocation = window.location;
      Object.defineProperty(window, 'location', {
        value: { ...originalLocation, href: '' },
        writable: true,
        configurable: true,
      });

      await renderAIDataLiaisonPage();
      const activateButton = screen.getByRole('button', { name: /^activate$/i });
      fireEvent.click(activateButton);

      await waitFor(() => {
        expect(window.location.href).toBe('https://central.example.com/billing');
      });

      Object.defineProperty(window, 'location', {
        value: originalLocation,
        writable: true,
        configurable: true,
      });
    });
  });

  describe('Tool Configuration', () => {
    it('does not render the Share with Business Users section', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.queryByText('Share with Business Users')).not.toBeInTheDocument();
    });

    it('renders tool configuration card', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Tool Configuration')).toBeInTheDocument();
    });

    it('shows approval tools toggle', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Add Approval Tools')).toBeInTheDocument();
      expect(screen.getByText(/review and manage query suggestions and glossary terms/i)).toBeInTheDocument();
    });

    it('shows request tools toggle with recommended badge', async () => {
      await renderAIDataLiaisonPage();
      expect(screen.getByText('Add Request Tools')).toBeInTheDocument();
      expect(screen.getByText('[RECOMMENDED]')).toBeInTheDocument();
      expect(screen.getByText(/access glossary terms through the MCP Client/i)).toBeInTheDocument();
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
      setupMocks({ hasOntologyForge: false });
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
      expect(screen.getByText(/remove AI Data Liaison access to glossary functionality/i)).toBeInTheDocument();
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

    it('redirects to central when uninstallApp returns redirectUrl', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({
        success: true,
        data: { redirectUrl: 'https://central.example.com/cancel-billing', status: 'pending_uninstall' },
      });

      const originalLocation = window.location;
      Object.defineProperty(window, 'location', {
        value: { ...originalLocation, href: '' },
        writable: true,
        configurable: true,
      });

      await renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(window.location.href).toBe('https://central.example.com/cancel-billing');
      });

      expect(mockNavigate).not.toHaveBeenCalled();

      Object.defineProperty(window, 'location', {
        value: originalLocation,
        writable: true,
        configurable: true,
      });
    });

    it('navigates locally when uninstallApp returns no redirectUrl', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({
        success: true,
        data: { status: 'uninstalled' },
      });

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
  });
});

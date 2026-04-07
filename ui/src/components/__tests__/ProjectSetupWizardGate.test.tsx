import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, Outlet, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ProjectProvider, useProject } from '../../contexts/ProjectContext';
import type { SetupStatus } from '../../types';
import ProjectSetupWizardGate from '../ProjectSetupWizardGate';

const mockActivateApp = vi.fn();
const mockEmbeddedBackHandler = vi.fn();
const mockToast = vi.fn();
const mockRefetch = vi.fn();

let mockSetupStatus: SetupStatus | null = null;
let mockSetupLoading = false;
let mockSetupError: string | null = null;

vi.mock('../../hooks/useSetupStatus', () => ({
  useSetupStatus: () => ({
    status: mockSetupStatus,
    isLoading: mockSetupLoading,
    error: mockSetupError,
    refetch: mockRefetch,
  }),
}));

vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock('../../services/engineApi', () => ({
  default: {
    activateApp: (...args: unknown[]) => mockActivateApp(...args),
  },
}));

vi.mock('../DatasourceSetupFlow', () => ({
  default: function DatasourceSetupFlowMock({
    onEmbeddedBackNavigationChange,
  }: {
    onEmbeddedBackNavigationChange?: (handler: (() => void) | null) => void;
  }) {
    return (
      <div>
        <div>Datasource setup flow</div>
        <button
          type="button"
          onClick={() => {
            onEmbeddedBackNavigationChange?.(() => {
              mockEmbeddedBackHandler();
            });
          }}
        >
          Provide back handler
        </button>
      </div>
    );
  },
}));

function WizardHarness({
  assignedAppIds = [],
  justProvisioned = true,
}: {
  assignedAppIds?: string[];
  justProvisioned?: boolean;
}) {
  const { setProjectInfo, shouldShowSetupWizard } = useProject();

  useEffect(() => {
    setProjectInfo(
      'proj-1',
      'Wizard Project',
      {},
      {
        justProvisioned,
        assignedAppIds,
      }
    );
  }, [assignedAppIds, justProvisioned, setProjectInfo]);

  return (
    <>
      <div>{shouldShowSetupWizard ? 'Wizard visible' : 'Wizard hidden'}</div>
      <Outlet />
    </>
  );
}

function renderWizard(
  options: {
    assignedAppIds?: string[];
    justProvisioned?: boolean;
    initialPath?: string;
  } = {}
) {
  const {
    assignedAppIds = [],
    justProvisioned = true,
    initialPath = '/projects/proj-1/setup',
  } = options;

  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <ProjectProvider>
        <Routes>
          <Route
            path="/projects/:pid"
            element={
              <WizardHarness
                assignedAppIds={assignedAppIds}
                justProvisioned={justProvisioned}
              />
            }
          >
            <Route index element={<div>Project home</div>} />
            <Route path="setup" element={<ProjectSetupWizardGate />} />
          </Route>
        </Routes>
      </ProjectProvider>
    </MemoryRouter>
  );
}

describe('ProjectSetupWizardGate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSetupLoading = false;
    mockSetupError = null;
    mockSetupStatus = {
      steps: {
        datasource_configured: false,
      },
      incomplete_count: 1,
      next_step: 'datasource_configured',
    };
    mockRefetch.mockResolvedValue(mockSetupStatus);
    mockActivateApp.mockResolvedValue({ success: true, data: {} });
  });

  it('renders the datasource step selected from backend setup status', async () => {
    renderWizard({ assignedAppIds: ['mcp-server'] });

    expect(await screen.findByText('Setup')).toBeInTheDocument();
    expect(screen.getByText('Datasource setup flow')).toBeInTheDocument();
    expect(screen.getByText('1 required step remaining')).toBeInTheDocument();
  });

  it('stores the embedded datasource back handler without invoking it immediately', async () => {
    renderWizard({ assignedAppIds: ['mcp-server'] });

    fireEvent.click(await screen.findByRole('button', { name: 'Provide back handler' }));

    const backButton = await screen.findByRole('button', { name: 'Back' });

    expect(mockEmbeddedBackHandler).not.toHaveBeenCalled();

    await waitFor(() => {
      expect(backButton).not.toBeDisabled();
    });

    fireEvent.click(backButton);

    expect(mockEmbeddedBackHandler).toHaveBeenCalledTimes(1);
  });

  it('shows included steps in registry order', async () => {
    mockSetupStatus = {
      steps: {
        datasource_configured: true,
        schema_selected: false,
        agents_queries_created: false,
      },
      incomplete_count: 2,
      next_step: 'schema_selected',
    };

    renderWizard({
      assignedAppIds: ['mcp-server', 'ontology-forge', 'ai-agents'],
    });

    const sidebar = await screen.findByText('Datasource');
    const asideText = sidebar.closest('aside')?.textContent ?? '';

    expect(asideText.indexOf('Datasource')).toBeLessThan(asideText.indexOf('Schema'));
    expect(asideText.indexOf('Schema')).toBeLessThan(asideText.indexOf('Agent queries'));
    expect(screen.getByRole('heading', { name: 'Select the schema to model' })).toBeInTheDocument();
  });

  it('renders on the setup route even when the project was not just provisioned', async () => {
    renderWizard({ justProvisioned: false });

    expect(await screen.findByText('Setup')).toBeInTheDocument();
    expect(screen.getByText('Wizard hidden')).toBeInTheDocument();
  });

  it('closes the wizard and returns to the project route', async () => {
    renderWizard();

    expect(await screen.findByText('Wizard visible')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    await waitFor(() => {
      expect(screen.getByText('Wizard hidden')).toBeInTheDocument();
    });
    expect(screen.getByText('Project home')).toBeInTheDocument();
  });

  it('activates inline activation steps', async () => {
    mockSetupStatus = {
      steps: {
        glossary_setup: true,
        adl_activated: false,
      },
      incomplete_count: 1,
      next_step: 'adl_activated',
    };

    renderWizard({ assignedAppIds: ['ai-data-liaison'] });

    fireEvent.click(await screen.findByRole('button', { name: 'Activate' }));

    await waitFor(() => {
      expect(mockActivateApp).toHaveBeenCalledWith('proj-1', 'ai-data-liaison');
    });
    expect(mockRefetch).toHaveBeenCalled();
  });
});

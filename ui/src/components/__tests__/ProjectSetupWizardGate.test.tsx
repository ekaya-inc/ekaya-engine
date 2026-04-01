import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useEffect, useRef } from 'react';
import { MemoryRouter, Outlet, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { DatasourceConnectionProvider, useDatasourceConnection } from '../../contexts/DatasourceConnectionContext';
import { ProjectProvider, useProject } from '../../contexts/ProjectContext';
import type { ConnectionDetails } from '../../types';
import ProjectSetupWizardGate from '../ProjectSetupWizardGate';

const mockCreateDataSource = vi.fn();
const mockDeleteDataSource = vi.fn();
const mockRenameDatasource = vi.fn();
const mockTestDatasourceConnection = vi.fn();
const mockUpdateDataSource = vi.fn();
const mockValidateConnectionDetails = vi.fn();
const mockToast = vi.fn();

let mockInstalledApps: Array<{ app_id: string }> = [];

vi.mock('../../hooks/useInstalledApps', () => ({
  useInstalledApps: () => ({
    apps: mockInstalledApps,
    isLoading: false,
    error: null,
    refetch: vi.fn().mockResolvedValue(undefined),
    isInstalled: (appId: string) => mockInstalledApps.some((app) => app.app_id === appId),
  }),
}));

vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock('../../services/engineApi', () => ({
  default: {
    createDataSource: (...args: unknown[]) => mockCreateDataSource(...args),
    deleteDataSource: (...args: unknown[]) => mockDeleteDataSource(...args),
    renameDatasource: (...args: unknown[]) => mockRenameDatasource(...args),
    testDatasourceConnection: (...args: unknown[]) => mockTestDatasourceConnection(...args),
    updateDataSource: (...args: unknown[]) => mockUpdateDataSource(...args),
    validateConnectionDetails: (...args: unknown[]) => mockValidateConnectionDetails(...args),
  },
}));

function WizardHarness({
  assignedAppIds,
  initialDatasources = [],
  justProvisioned = true,
}: {
  assignedAppIds: string[];
  initialDatasources?: ConnectionDetails[];
  justProvisioned?: boolean;
}) {
  const { setProjectInfo, shouldShowSetupWizard } = useProject();
  const { connect, isConnected } = useDatasourceConnection();
  const hasSeededDatasources = useRef(false);

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

  useEffect(() => {
    if (hasSeededDatasources.current) {
      return;
    }

    hasSeededDatasources.current = true;
    initialDatasources.forEach((datasource) => connect(datasource));
  }, [connect, initialDatasources]);

  return (
    <>
      <div>{shouldShowSetupWizard ? 'Wizard visible' : 'Wizard hidden'}</div>
      <div>{isConnected ? 'Datasource connected' : 'Datasource disconnected'}</div>
      <Outlet />
    </>
  );
}

const renderWizard = (
  assignedAppIds: string[],
  initialDatasources: ConnectionDetails[] = [],
  initialPath = '/projects/proj-1/setup',
  justProvisioned = true
) =>
  render(
    <MemoryRouter initialEntries={[initialPath]}>
      <ProjectProvider>
        <DatasourceConnectionProvider>
          <Routes>
            <Route
              path="/projects/:pid"
              element={
                <WizardHarness
                  assignedAppIds={assignedAppIds}
                  initialDatasources={initialDatasources}
                  justProvisioned={justProvisioned}
                />
              }
            >
              <Route index element={<div>Project home</div>} />
              <Route path="setup" element={<ProjectSetupWizardGate />} />
            </Route>
          </Routes>
        </DatasourceConnectionProvider>
      </ProjectProvider>
    </MemoryRouter>
  );

describe('ProjectSetupWizardGate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockInstalledApps = [];
    mockTestDatasourceConnection.mockResolvedValue({
      success: true,
      message: 'Connection successful',
    });
    mockCreateDataSource.mockResolvedValue({
      success: true,
      data: {
        datasource_id: 'ds-1',
        project_id: 'proj-1',
        name: 'Supabase',
        provider: 'supabase',
      },
    });
    mockUpdateDataSource.mockResolvedValue({ success: true });
    mockDeleteDataSource.mockResolvedValue({ success: true });
    mockRenameDatasource.mockResolvedValue({ success: true });
    mockValidateConnectionDetails.mockImplementation(() => undefined);

    global.fetch = vi.fn(async (input: string | URL | Request) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;

      if (url === '/api/config/datasource-types') {
        return {
          ok: true,
          json: async () => [
            {
              type: 'postgres',
              display_name: 'PostgreSQL',
              description: 'PostgreSQL database',
              icon: 'postgres',
            },
          ],
        } as Response;
      }

      if (url === '/api/auth/me') {
        return {
          ok: true,
          json: async () => ({ hasAzureToken: false, email: '' }),
        } as Response;
      }

      throw new Error(`Unexpected fetch: ${url}`);
    }) as typeof fetch;
  });

  it('shows scratch mode copy when only MCP Server is provisioned', async () => {
    renderWizard(['mcp-server']);

    expect(await screen.findByText('Setup')).toBeInTheDocument();
    expect(screen.queryByText('Scratch mode')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Skip setup' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Finish' })).toBeDisabled();
  });

  it('orders provisioned applications by setup dependency in the sidebar', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-agents' }];
    renderWizard(['mcp-server', 'ai-agents', 'ontology-forge']);

    expect(await screen.findByText('Setup')).toBeInTheDocument();

    const sidebar = screen.getByText('Setup').closest('aside');
    expect(sidebar).not.toBeNull();

    const sidebarText = sidebar?.textContent ?? '';
    expect(sidebarText.indexOf('MCP Server')).toBeGreaterThan(-1);
    expect(sidebarText.indexOf('Ontology Forge')).toBeGreaterThan(-1);
    expect(sidebarText.indexOf('AI Agents')).toBeGreaterThan(-1);
    expect(sidebarText.indexOf('Ontology Forge')).toBeLessThan(sidebarText.indexOf('AI Agents'));
  });

  it('renders on the setup route even when the project was not just provisioned', async () => {
    renderWizard(['mcp-server'], [], '/projects/proj-1/setup', false);

    expect(await screen.findByText('Setup')).toBeInTheDocument();
    expect(screen.getByText('Wizard hidden')).toBeInTheDocument();
  });

  it('cancels the wizard and leaves the current UI visible', async () => {
    renderWizard(['mcp-server']);

    expect(await screen.findByText('Wizard visible')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.getByText('Wizard hidden')).toBeInTheDocument();
    });
    expect(screen.getByText('Datasource disconnected')).toBeInTheDocument();
    expect(screen.getByText('Project home')).toBeInTheDocument();
  });

  it('keeps the saved datasource connected after cancelling the wizard', async () => {
    renderWizard(['mcp-server']);

    expect(await screen.findByText('Setup')).toBeInTheDocument();

    fireEvent.click(await screen.findByText('Supabase'));

    fireEvent.change(screen.getByLabelText(/^Host/), {
      target: { value: 'db.supabase.example' },
    });
    fireEvent.change(screen.getByLabelText(/^Username/), {
      target: { value: 'postgres' },
    });
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'secret' },
    });
    fireEvent.change(screen.getByLabelText(/^Database Name/), {
      target: { value: 'postgres' },
    });

    fireEvent.click(screen.getByRole('button', { name: /test connection/i }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /save datasource/i })).toBeEnabled();
    });

    fireEvent.click(screen.getByRole('button', { name: /save datasource/i }));

    await waitFor(() => {
      expect(screen.getByText('Datasource connected')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: 'Finish' })).toBeEnabled();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.getByText('Wizard hidden')).toBeInTheDocument();
    });
    expect(screen.getByText('Datasource connected')).toBeInTheDocument();
    expect(screen.getByText('Project home')).toBeInTheDocument();
  });

  it('keeps finish disabled when the only datasource is unusable', async () => {
    renderWizard(['mcp-server'], [
      {
        datasourceId: 'ds-broken',
        projectId: 'proj-1',
        type: 'postgres',
        provider: 'supabase',
        displayName: 'Broken datasource',
        host: 'db.supabase.example',
        port: 5432,
        user: 'postgres',
        password: 'secret',
        name: 'postgres',
        ssl_mode: 'require',
        decryption_failed: true,
        error_message: 'ciphertext authentication failed',
      },
    ]);

    expect(await screen.findByText('Datasource connected')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Finish' })).toBeDisabled();
    });
  });

  it('returns to adapter selection from the embedded configuration flow', async () => {
    renderWizard(['mcp-server']);

    expect(await screen.findByText('Select Your Database')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled();

    fireEvent.click(await screen.findByText('Supabase'));

    expect(await screen.findByText('Configure Supabase')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Back' })).toBeEnabled();
    });

    fireEvent.click(screen.getByRole('button', { name: 'Back' }));

    await waitFor(() => {
      expect(screen.getByText('Select Your Database')).toBeInTheDocument();
    });
  });
});

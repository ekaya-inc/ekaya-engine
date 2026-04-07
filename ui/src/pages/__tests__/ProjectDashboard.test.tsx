import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../lib/auth-token';
import type { AuditSummary } from '../../types/audit';
import ProjectDashboard from '../ProjectDashboard';

const mockNavigate = vi.fn();
const mockGetAuditSummary = vi.fn();

type MockInstalledApp = {
  app_id: string;
  activated_at?: string;
};

let mockInstalledApps: MockInstalledApp[] = [];
let mockIsConnected = true;
let mockHasSelectedTables = true;
let mockSetupStatus: { incomplete_count: number; steps: Record<string, boolean>; next_step?: string } | null = null;

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('../../hooks/useInstalledApps', () => ({
  useInstalledApps: () => ({
    apps: mockInstalledApps,
    isLoading: false,
    error: null,
    refetch: vi.fn().mockResolvedValue(undefined),
    isInstalled: (appId: string) => mockInstalledApps.some((app) => app.app_id === appId),
  }),
}));

vi.mock('../../hooks/useSetupStatus', () => ({
  useSetupStatus: () => ({
    status: mockSetupStatus,
    isLoading: false,
    error: null,
    refetch: vi.fn().mockResolvedValue(mockSetupStatus),
  }),
}));

vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => ({
    isConnected: mockIsConnected,
    hasSelectedTables: mockHasSelectedTables,
  }),
}));

vi.mock('../../services/ontologyService', () => ({
  ontologyService: {
    subscribe: vi.fn(() => () => {}),
  },
}));

vi.mock('../../lib/auth-token', () => ({
  getUserRoles: vi.fn(() => ['admin']),
}));

vi.mock('../../services/engineApi', () => ({
  default: {
    getAuditSummary: (...args: unknown[]) => mockGetAuditSummary(...args),
  },
}));

vi.mock('../../components/AIConfigWidget', () => ({
  default: function AIConfigWidgetMock(
    { onConfigChange }: { onConfigChange: (option: 'byok') => void }
  ) {
    useEffect(() => {
      onConfigChange('byok');
    }, [onConfigChange]);

    return <div data-testid="ai-config-widget" />;
  },
}));

const renderDashboard = () => {
  return render(
    <MemoryRouter initialEntries={['/projects/proj-1']}>
      <Routes>
        <Route path="/projects/:pid" element={<ProjectDashboard />} />
      </Routes>
    </MemoryRouter>
  );
};

const createAuditSummary = (overrides: Partial<AuditSummary> = {}): AuditSummary => ({
  total_query_executions: 0,
  failed_query_count: 0,
  ontology_changes_count: 0,
  pending_schema_changes: 0,
  pending_query_approvals: 0,
  mcp_events_count: 0,
  open_alerts_critical: 0,
  open_alerts_warning: 0,
  open_alerts_info: 0,
  ...overrides,
});

describe('ProjectDashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockInstalledApps = [{ app_id: 'ontology-forge' }];
    mockIsConnected = true;
    mockHasSelectedTables = true;
    mockSetupStatus = null;
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
    mockGetAuditSummary.mockResolvedValue({
      success: true,
      data: createAuditSummary(),
    });
  });

  it('does not show the Glossary tile when AI Data Liaison is not installed', async () => {
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Ontology Extraction')).toBeInTheDocument();
    });

    expect(screen.queryByText('Glossary')).not.toBeInTheDocument();
  });

  it('shows a setup button when setup is incomplete', async () => {
    mockSetupStatus = {
      incomplete_count: 2,
      steps: {
        datasource_configured: false,
        schema_selected: false,
      },
      next_step: 'datasource_configured',
    };

    renderDashboard();

    expect(await screen.findByRole('button', { name: /setup/i })).toBeInTheDocument();
    expect(screen.getByLabelText('2 incomplete setup steps')).toBeInTheDocument();
  });

  it('keeps the setup button visible when no required setup steps remain', async () => {
    mockSetupStatus = {
      incomplete_count: 0,
      steps: {
        datasource_configured: true,
      },
    };

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Applications')).toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: /setup/i })).toBeInTheDocument();
    expect(screen.queryByLabelText(/incomplete setup step/i)).not.toBeInTheDocument();
  });

  it('shows the Glossary tile when AI Data Liaison is installed', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Glossary')).toBeInTheDocument();
    });
  });

  it('shows the Audit Log tile in the Data section when AI Data Liaison is installed', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Audit Log')).toBeInTheDocument();
    });

    const dataSection = screen.getByRole('heading', { name: 'Data' }).closest('section');
    const intelligenceSection = screen.getByRole('heading', { name: 'Intelligence' }).closest('section');

    expect(dataSection).not.toBeNull();
    expect(intelligenceSection).not.toBeNull();

    const pendingQueriesTile = within(dataSection as HTMLElement).getByText('Pending Queries');
    const auditLogTile = within(dataSection as HTMLElement).getByText('Audit Log');

    expect(auditLogTile).toBeInTheDocument();
    expect(pendingQueriesTile.compareDocumentPosition(auditLogTile) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(intelligenceSection as HTMLElement).queryByText('Audit Log')).not.toBeInTheDocument();
  });

  it('shows a pending queries badge count when pending suggestions exist', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    mockGetAuditSummary.mockResolvedValue({
      success: true,
      data: createAuditSummary({ pending_query_approvals: 3 }),
    });

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByLabelText('3 pending query suggestions')).toBeInTheDocument();
    });

    expect(mockGetAuditSummary).toHaveBeenCalledWith('proj-1');
  });

  it('does not show a pending queries badge when there are no pending suggestions', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Pending Queries')).toBeInTheDocument();
    });

    expect(screen.queryByLabelText(/pending query suggestion/)).not.toBeInTheDocument();
  });

  it('keeps the pending queries tile visible when the badge count fetch fails', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    mockGetAuditSummary.mockRejectedValue(new Error('network error'));

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Pending Queries')).toBeInTheDocument();
    });

    expect(screen.queryByLabelText(/pending query suggestion/)).not.toBeInTheDocument();
  });

  it('does not fetch the pending queries badge for user-only sessions', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);

    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Pending Queries')).toBeInTheDocument();
    });

    expect(mockGetAuditSummary).not.toHaveBeenCalled();
    expect(screen.queryByLabelText(/pending query suggestion/)).not.toBeInTheDocument();
  });

  it('renders the dashboard before the pending queries badge resolves', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];

    let resolveSummary: ((value: { success: boolean; data: AuditSummary }) => void) | undefined;
    mockGetAuditSummary.mockReturnValue(
      new Promise((resolve) => {
        resolveSummary = resolve;
      })
    );

    renderDashboard();

    expect(screen.getByText('Pending Queries')).toBeInTheDocument();
    expect(screen.queryByLabelText(/pending query suggestion/)).not.toBeInTheDocument();

    resolveSummary?.({
      success: true,
      data: createAuditSummary({ pending_query_approvals: 2 }),
    });

    await waitFor(() => {
      expect(screen.getByLabelText('2 pending query suggestions')).toBeInTheDocument();
    });
  });

  it('navigates to the glossary page from the AI Data Liaison tile', async () => {
    mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Glossary')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Glossary'));
    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/glossary');
  });

  describe('application tiles', () => {
    it('always renders MCP Server tile', () => {
      renderDashboard();
      expect(screen.getByText('MCP Server')).toBeInTheDocument();
    });

    it('does not render MCP Tunnel tile when not installed', () => {
      renderDashboard();
      expect(screen.queryByText('MCP Tunnel')).not.toBeInTheDocument();
    });

    it('renders MCP Tunnel tile when installed', () => {
      mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'mcp-tunnel' }];
      renderDashboard();

      expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
      expect(
        screen.getByText('Open MCP Tunnel to activate it and create a public URL for your MCP Server.')
      ).toBeInTheDocument();
    });

    it('renders connected MCP Tunnel copy after activation', () => {
      mockInstalledApps = [
        { app_id: 'ontology-forge' },
        { app_id: 'mcp-tunnel', activated_at: '2024-01-02' },
      ];
      renderDashboard();

      expect(
        screen.getByText('Your MCP Server has a public URL accessible from outside your firewall.')
      ).toBeInTheDocument();
    });

    it('navigates to MCP Tunnel page even when datasource is not connected', () => {
      mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'mcp-tunnel' }];
      mockIsConnected = false;
      renderDashboard();

      fireEvent.click(screen.getByText('MCP Tunnel'));
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/mcp-tunnel');
    });

    it('does not render AI Data Liaison tile when not installed', () => {
      renderDashboard();
      expect(screen.queryByText('AI Data Liaison')).not.toBeInTheDocument();
    });

    it('renders AI Data Liaison tile when installed', async () => {
      mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
      renderDashboard();

      await waitFor(() => {
        expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
      });
    });

    it('does not render AI Agents tile when not installed', () => {
      renderDashboard();
      expect(screen.queryByText('AI Agents')).not.toBeInTheDocument();
    });

    it('renders AI Agents tile when installed', () => {
      mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-agents' }];
      renderDashboard();
      expect(screen.getByText('AI Agents')).toBeInTheDocument();
    });

    it('renders all application tiles when all apps are installed', async () => {
      mockInstalledApps = [
        { app_id: 'ontology-forge' },
        { app_id: 'ai-data-liaison' },
        { app_id: 'ai-agents' },
        { app_id: 'mcp-tunnel' },
        { app_id: 'file-loader' },
      ];
      renderDashboard();

      await waitFor(() => {
        expect(screen.getByText('MCP Server')).toBeInTheDocument();
        expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
        expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
        expect(screen.getByText('AI Agents')).toBeInTheDocument();
        expect(screen.getByText('Spreadsheet Loader [BETA]')).toBeInTheDocument();
        expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
      });
    });
  });
});

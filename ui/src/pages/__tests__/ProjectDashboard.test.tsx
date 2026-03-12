import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ProjectDashboard from '../ProjectDashboard';

const mockNavigate = vi.fn();

type MockInstalledApp = {
  app_id: string;
  activated_at?: string;
};

let mockInstalledApps: MockInstalledApp[] = [];
let mockIsConnected = true;
let mockHasSelectedTables = true;

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

describe('ProjectDashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockInstalledApps = [{ app_id: 'ontology-forge' }];
    mockIsConnected = true;
    mockHasSelectedTables = true;
  });

  it('does not show the Glossary tile when AI Data Liaison is not installed', async () => {
    renderDashboard();

    await waitFor(() => {
      expect(screen.getByText('Ontology Extraction')).toBeInTheDocument();
    });

    expect(screen.queryByText('Glossary')).not.toBeInTheDocument();
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

    it('renders AI Data Liaison tile when installed', () => {
      mockInstalledApps = [{ app_id: 'ontology-forge' }, { app_id: 'ai-data-liaison' }];
      renderDashboard();
      expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
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

    it('renders all application tiles when all apps are installed', () => {
      mockInstalledApps = [
        { app_id: 'ontology-forge' },
        { app_id: 'ai-data-liaison' },
        { app_id: 'ai-agents' },
        { app_id: 'mcp-tunnel' },
        { app_id: 'file-loader' },
      ];
      renderDashboard();

      expect(screen.getByText('MCP Server')).toBeInTheDocument();
      expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
      expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
      expect(screen.getByText('AI Agents')).toBeInTheDocument();
      expect(screen.getByText('Spreadsheet Loader [BETA]')).toBeInTheDocument();
      expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
    });
  });
});

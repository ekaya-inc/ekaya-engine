import { fireEvent, render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ProjectDashboard from '../ProjectDashboard';

// Mock react-router-dom hooks
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Mock useInstalledApps hook
let mockInstalledApps: string[] = [];
vi.mock('../../hooks/useInstalledApps', () => ({
  useInstalledApps: () => ({
    apps: mockInstalledApps.map((id) => ({ app_id: id })),
    isLoading: false,
    error: null,
    refetch: vi.fn().mockResolvedValue(undefined),
    isInstalled: (appId: string) => mockInstalledApps.includes(appId),
  }),
}));

// Mock useDatasourceConnection context
let mockIsConnected = true;
let mockHasSelectedTables = true;
vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => ({
    isConnected: mockIsConnected,
    hasSelectedTables: mockHasSelectedTables,
    datasources: [],
    selectedDatasource: null,
    connectionDetails: null,
    connectionStatus: null,
    connect: vi.fn(),
    disconnect: vi.fn(),
    testConnection: vi.fn(),
    saveDataSource: vi.fn(),
    selectDatasource: vi.fn(),
    isLoading: false,
    error: null,
  }),
}));

// Mock ontologyService
vi.mock('../../services/ontologyService', () => ({
  ontologyService: {
    subscribe: (callback: (status: unknown) => void) => {
      callback(null);
      return () => {};
    },
  },
}));

// Mock AIConfigWidget — must call onConfigChange in useEffect to avoid infinite re-render
vi.mock('../../components/AIConfigWidget', () => ({
  default: ({ onConfigChange }: { onConfigChange: (val: unknown) => void }) => {
    useEffect(() => {
      onConfigChange({ provider: 'openai' });
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
    mockInstalledApps = [];
    mockIsConnected = true;
    mockHasSelectedTables = true;
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
      mockInstalledApps = ['mcp-tunnel'];
      renderDashboard();
      expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
      expect(
        screen.getByText('Your MCP Server has a public URL accessible from outside your firewall.')
      ).toBeInTheDocument();
    });

    it('renders MCP Tunnel tile as enabled when datasource is connected', () => {
      mockInstalledApps = ['mcp-tunnel'];
      mockIsConnected = true;
      renderDashboard();

      const tunnelCard = screen.getByText('MCP Tunnel').closest('.transition-all');
      expect(tunnelCard?.className).toContain('cursor-pointer');
      expect(tunnelCard?.className).not.toContain('cursor-not-allowed');
    });

    it('renders MCP Tunnel tile as disabled when datasource is not connected', () => {
      mockInstalledApps = ['mcp-tunnel'];
      mockIsConnected = false;
      renderDashboard();

      const tunnelCard = screen.getByText('MCP Tunnel').closest('.transition-all');
      expect(tunnelCard?.className).toContain('cursor-not-allowed');
      expect(screen.getByText('Requires MCP Server to be enabled.')).toBeInTheDocument();
    });

    it('navigates to MCP Tunnel page when clicking enabled tile', () => {
      mockInstalledApps = ['mcp-tunnel'];
      renderDashboard();

      fireEvent.click(screen.getByText('MCP Tunnel'));
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/mcp-tunnel');
    });

    it('does not navigate when clicking disabled MCP Tunnel tile', () => {
      mockInstalledApps = ['mcp-tunnel'];
      mockIsConnected = false;
      renderDashboard();

      fireEvent.click(screen.getByText('MCP Tunnel'));
      expect(mockNavigate).not.toHaveBeenCalled();
    });

    it('does not render AI Data Liaison tile when not installed', () => {
      renderDashboard();
      expect(screen.queryByText('AI Data Liaison')).not.toBeInTheDocument();
    });

    it('renders AI Data Liaison tile when installed', () => {
      mockInstalledApps = ['ai-data-liaison'];
      renderDashboard();
      expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
    });

    it('does not render AI Agents tile when not installed', () => {
      renderDashboard();
      expect(screen.queryByText('AI Agents and Automation')).not.toBeInTheDocument();
    });

    it('renders AI Agents tile when installed', () => {
      mockInstalledApps = ['ai-agents'];
      renderDashboard();
      expect(screen.getByText('AI Agents and Automation')).toBeInTheDocument();
    });

    it('renders all app tiles when all apps are installed', () => {
      mockInstalledApps = ['ai-data-liaison', 'ai-agents', 'mcp-tunnel'];
      renderDashboard();

      expect(screen.getByText('MCP Server')).toBeInTheDocument();
      expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
      expect(screen.getByText('AI Agents and Automation')).toBeInTheDocument();
      expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
    });
  });

  describe('navigation', () => {
    it('navigates to applications page when clicking Install Applications', () => {
      renderDashboard();

      fireEvent.click(screen.getByText('Install Applications'));
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/applications');
    });
  });
});

import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import PendingQueriesPage from '../PendingQueriesPage';

// Mock the DatasourceConnectionContext
const mockUseDatasourceConnection = vi.fn();
vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => mockUseDatasourceConnection(),
}));

// Mock engineApi
const mockListPendingQueries = vi.fn();
vi.mock('../../services/engineApi', () => ({
  default: {
    listPendingQueries: () => mockListPendingQueries(),
  },
}));

// Mock react-router-dom hooks
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Mock the QueriesView component to simplify testing
vi.mock('../../components/QueriesView', () => ({
  default: ({ onPendingCountChange: _onPendingCountChange }: { onPendingCountChange?: () => void }) => (
    <div data-testid="queries-view">QueriesView</div>
  ),
}));

describe('PendingQueriesPage - Provider Display', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListPendingQueries.mockResolvedValue({
      success: true,
      data: { queries: [], count: 0 },
    });
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/pending-queries']}>
        <Routes>
          <Route path="/projects/:pid/pending-queries" element={<PendingQueriesPage />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it('displays Pending Queries title with green icon', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    renderPage();

    expect(screen.getByText('Pending Queries')).toBeInTheDocument();
  });

  it('displays provider name when provider is set', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My Supabase DB',
        type: 'postgres',
        provider: 'supabase',
      },
      isConnected: true,
    });

    renderPage();

    expect(screen.getByText('Supabase')).toBeInTheDocument();
    expect(screen.getByText('My Supabase DB')).toBeInTheDocument();
  });

  it('shows no datasource message when not connected', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: null,
      isConnected: false,
    });

    renderPage();

    expect(screen.getByText('No Datasource Connected')).toBeInTheDocument();
    expect(screen.queryByTestId('queries-view')).not.toBeInTheDocument();
  });

  it('shows only Pending Approval and Rejected tabs (no Approved tab)', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    renderPage();

    expect(screen.getByText('Pending Approval')).toBeInTheDocument();
    expect(screen.getByText('Rejected')).toBeInTheDocument();
    expect(screen.queryByText('Approved')).not.toBeInTheDocument();
  });
});

describe('PendingQueriesPage - Pending Count Badge', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/pending-queries']}>
        <Routes>
          <Route path="/projects/:pid/pending-queries" element={<PendingQueriesPage />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it('shows pending badge when there are pending queries', async () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    mockListPendingQueries.mockResolvedValue({
      success: true,
      data: { queries: [{}, {}, {}], count: 3 },
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('3')).toBeInTheDocument();
      expect(screen.getByText('Pending Approval')).toBeInTheDocument();
    });
  });

  it('does not show pending badge when count is zero', async () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    mockListPendingQueries.mockResolvedValue({
      success: true,
      data: { queries: [], count: 0 },
    });

    renderPage();

    await waitFor(() => {
      expect(mockListPendingQueries).toHaveBeenCalled();
    });

    expect(screen.getByText('Pending Approval')).toBeInTheDocument();
    const pendingTab = screen.getByText('Pending Approval').closest('button');
    expect(pendingTab?.querySelector('.rounded-full')).toBeNull();
  });

  it('does not show pending badge when API fails', async () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    mockListPendingQueries.mockRejectedValue(new Error('API error'));

    renderPage();

    await waitFor(() => {
      expect(mockListPendingQueries).toHaveBeenCalled();
    });

    expect(screen.getByText('Pending Approval')).toBeInTheDocument();
    const pendingTab = screen.getByText('Pending Approval').closest('button');
    expect(pendingTab?.querySelector('.rounded-full')).toBeNull();
  });

  it('shows singular pending count correctly', async () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    mockListPendingQueries.mockResolvedValue({
      success: true,
      data: { queries: [{}], count: 1 },
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('1')).toBeInTheDocument();
    });
  });

  it('fetches pending count on mount', async () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My DB',
        type: 'postgres',
      },
      isConnected: true,
    });

    mockListPendingQueries.mockResolvedValue({
      success: true,
      data: { queries: [], count: 0 },
    });

    renderPage();

    await waitFor(() => {
      expect(mockListPendingQueries).toHaveBeenCalledTimes(1);
    });
  });
});

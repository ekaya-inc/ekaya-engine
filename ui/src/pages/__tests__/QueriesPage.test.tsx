import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import QueriesPage from '../QueriesPage';

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

describe('QueriesPage - Provider Display', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: no pending queries
    mockListPendingQueries.mockResolvedValue({
      success: true,
      data: { queries: [], count: 0 },
    });
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/queries']}>
        <Routes>
          <Route path="/projects/:pid/queries" element={<QueriesPage />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it('displays provider name when provider is set (Supabase)', () => {
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

    // Should show provider name, not generic "postgres"
    expect(screen.getByText('Supabase')).toBeInTheDocument();
    expect(screen.queryByText('postgres')).not.toBeInTheDocument();
    expect(screen.getByText('My Supabase DB')).toBeInTheDocument();
  });

  it('displays provider name when provider is set (Neon)', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My Neon DB',
        type: 'postgres',
        provider: 'neon',
      },
      isConnected: true,
    });

    renderPage();

    // Should show provider name
    expect(screen.getByText('Neon')).toBeInTheDocument();
    expect(screen.getByText('My Neon DB')).toBeInTheDocument();
  });

  it('displays provider icon when provider is set', () => {
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

    // Should show provider icon
    const icon = screen.getByAltText('Supabase');
    expect(icon).toBeInTheDocument();
    expect(icon).toHaveAttribute('src', '/icons/adapters/Supabase.png');
  });

  it('falls back to adapter info when no provider is set', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'postgres',
        displayName: 'My PostgreSQL DB',
        type: 'postgres',
        // provider not set
      },
      isConnected: true,
    });

    renderPage();

    // Should show generic PostgreSQL info
    expect(screen.getByText('PostgreSQL')).toBeInTheDocument();
    expect(screen.getByText('My PostgreSQL DB')).toBeInTheDocument();

    // Should show PostgreSQL icon
    const icon = screen.getByAltText('PostgreSQL');
    expect(icon).toBeInTheDocument();
  });

  it('displays adapter name for non-postgres datasources', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'mydb',
        displayName: 'My MySQL DB',
        type: 'mysql',
      },
      isConnected: true,
    });

    renderPage();

    expect(screen.getByText('MySQL')).toBeInTheDocument();
    expect(screen.getByText('My MySQL DB')).toBeInTheDocument();
  });

  it('displays CockroachDB provider name correctly', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'defaultdb',
        displayName: 'Cockroach Cloud',
        type: 'postgres',
        provider: 'cockroachdb',
      },
      isConnected: true,
    });

    renderPage();

    expect(screen.getByText('CockroachDB')).toBeInTheDocument();
    expect(screen.getByText('Cockroach Cloud')).toBeInTheDocument();
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

  it('uses displayName if available, falls back to name', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'mydb',
        type: 'postgres',
        provider: 'postgres',
        // displayName not set
      },
      isConnected: true,
    });

    renderPage();

    // Should fall back to name
    expect(screen.getByText('mydb')).toBeInTheDocument();
  });
});

describe('QueriesPage - Pending Count Badge', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/queries']}>
        <Routes>
          <Route path="/projects/:pid/queries" element={<QueriesPage />} />
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
      expect(screen.getByText('3 pending')).toBeInTheDocument();
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

    // Wait for the API call to complete
    await waitFor(() => {
      expect(mockListPendingQueries).toHaveBeenCalled();
    });

    // Badge should not be shown
    expect(screen.queryByText(/pending/)).not.toBeInTheDocument();
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

    // Wait for the API call to complete
    await waitFor(() => {
      expect(mockListPendingQueries).toHaveBeenCalled();
    });

    // Badge should not be shown on error
    expect(screen.queryByText(/pending/)).not.toBeInTheDocument();
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
      expect(screen.getByText('1 pending')).toBeInTheDocument();
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

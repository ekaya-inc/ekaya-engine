import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import QueriesPage from '../QueriesPage';

// Mock the DatasourceConnectionContext
const mockUseDatasourceConnection = vi.fn();
vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => mockUseDatasourceConnection(),
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
  default: () => <div data-testid="queries-view">QueriesView</div>,
}));

describe('QueriesPage - Provider Display', () => {
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

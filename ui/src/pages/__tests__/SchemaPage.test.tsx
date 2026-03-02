import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import SchemaPage from '../SchemaPage';

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

// Mock the useToast hook
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

// Mock the engineApi
vi.mock('../../services/engineApi', () => ({
  default: {
    getSchema: vi.fn(),
    saveSchemaSelections: vi.fn(),
    refreshSchema: vi.fn(),
    rejectPendingChanges: vi.fn(),
  },
}));

const makeSchemaResponse = (pendingChanges: Record<string, { change_id: string; change_type: string; status: string }> = {}) => ({
  success: true,
  data: {
    tables: [
      {
        table_name: 'users',
        is_selected: true,
        columns: [
          { column_name: 'id', data_type: 'uuid', is_selected: true },
          { column_name: 'name', data_type: 'varchar', is_selected: true },
        ],
      },
    ],
    total_tables: 1,
    relationships: [],
    pending_changes: pendingChanges,
  },
});

describe('SchemaPage - Button Labels', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: { datasourceId: 'ds-1', name: 'Test DB' },
      refreshSchemaSelections: vi.fn(),
    });
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/schema']}>
        <Routes>
          <Route path="/projects/:pid/schema" element={<SchemaPage />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it('shows "Save Schema" and "Cancel" when pending_changes is empty', async () => {
    vi.mocked(engineApi.getSchema).mockResolvedValue(makeSchemaResponse({}));

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Save Schema')).toBeInTheDocument();
    });
    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  it('shows "Cancel" button (not "Reject Changes") when pending_changes is empty', async () => {
    vi.mocked(engineApi.getSchema).mockResolvedValue(makeSchemaResponse({}));

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Cancel')).toBeInTheDocument();
    });
    expect(screen.queryByText('Reject Changes')).not.toBeInTheDocument();
  });

  it('shows "Approve Changes" and "Reject Changes" when pending changes with status=pending exist', async () => {
    vi.mocked(engineApi.getSchema).mockResolvedValue(makeSchemaResponse({
      'public.invoices': { change_id: 'c1', change_type: 'new_table', status: 'pending' },
    }));

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Approve Changes')).toBeInTheDocument();
    });
    expect(screen.getByText('Reject Changes')).toBeInTheDocument();
  });

  it('shows default labels when pending_changes only has auto_applied entries', async () => {
    vi.mocked(engineApi.getSchema).mockResolvedValue(makeSchemaResponse({
      'public.orders': { change_id: 'c2', change_type: 'dropped_table', status: 'auto_applied' },
    }));

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Save Schema')).toBeInTheDocument();
    });
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.queryByText('Approve Changes')).not.toBeInTheDocument();
    expect(screen.queryByText('Reject Changes')).not.toBeInTheDocument();
  });
});

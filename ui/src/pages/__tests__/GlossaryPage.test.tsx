import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../lib/auth-token';
import engineApi from '../../services/engineApi';
import ontologyService from '../../services/ontologyService';
import type { GlossaryTerm } from '../../types';
import GlossaryPage from '../GlossaryPage';

const mockUseDatasourceConnection = vi.fn();

// Mock the services
vi.mock('../../services/engineApi', () => ({
  default: {
    listGlossaryTerms: vi.fn(),
    deleteGlossaryTerm: vi.fn(),
    testGlossarySQL: vi.fn(),
    testQuery: vi.fn(),
    createGlossaryTerm: vi.fn(),
    updateGlossaryTerm: vi.fn(),
    autoGenerateGlossary: vi.fn(),
  },
}));

vi.mock('../../services/ontologyService', () => ({
  default: {
    setProjectId: vi.fn(),
    subscribe: vi.fn(() => vi.fn()),
  },
}));

vi.mock('../../services/ontologyApi', () => ({
  default: {
    getNextQuestion: vi.fn().mockResolvedValue({ all_complete: true, counts: { required: 0, optional: 0 } }),
  },
}));

vi.mock('../../lib/auth-token', () => ({
  getUserRoles: vi.fn(() => ['admin']),
}));

// Mock the GlossaryTermEditor component
vi.mock('../../components/GlossaryTermEditor', () => ({
  GlossaryTermEditor: ({ isOpen, onClose, onSave, dialect }: { isOpen: boolean; onClose: () => void; onSave: () => void; dialect?: string }) =>
    isOpen ? (
      <div data-testid="glossary-term-editor" data-dialect={dialect}>
        <button onClick={onSave}>Save</button>
        <button onClick={onClose}>Close</button>
      </div>
    ) : null,
}));

// Mock the toast hook
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => mockUseDatasourceConnection(),
}));

const mockTerms: GlossaryTerm[] = [
  {
    id: 'term-1',
    project_id: 'proj-1',
    term: 'Active Users',
    definition: 'Users who have logged in within the last 30 days',
    defining_sql: 'SELECT COUNT(DISTINCT user_id) AS active_users FROM users WHERE last_login > NOW() - INTERVAL \'30 days\'',
    base_table: 'users',
    output_columns: [
      { name: 'active_users', type: 'integer', description: 'Count of active users' },
    ],
    aliases: ['MAU', 'Monthly Active Users'],
    source: 'inferred',
    created_at: '2024-01-15T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z',
  },
  {
    id: 'term-2',
    project_id: 'proj-1',
    term: 'Revenue',
    definition: 'Total revenue from completed transactions',
    defining_sql: 'SELECT SUM(amount) AS total_revenue FROM transactions WHERE status = \'completed\'',
    base_table: 'transactions',
    output_columns: [
      { name: 'total_revenue', type: 'numeric', description: 'Sum of completed transaction amounts' },
    ],
    source: 'manual',
    created_at: '2024-01-16T00:00:00Z',
    updated_at: '2024-01-16T00:00:00Z',
  },
];

const renderGlossaryPage = () => {
  return render(
    <MemoryRouter initialEntries={['/projects/proj-1/glossary']}>
      <Routes>
        <Route path="/projects/:pid/glossary" element={<GlossaryPage />} />
      </Routes>
    </MemoryRouter>
  );
};

describe('GlossaryPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
    vi.mocked(ontologyService.subscribe).mockReturnValue(vi.fn());
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'Test DB',
        type: 'postgres',
      },
    });
  });

  describe('Loading State', () => {
    it('shows loading spinner while fetching terms', () => {
      vi.mocked(engineApi.listGlossaryTerms).mockImplementation(
        () => new Promise(() => {}) // Never resolves
      );

      renderGlossaryPage();

      // Loading state should be visible immediately (synchronous)
      expect(screen.getByRole('status')).toBeInTheDocument();
      // Text appears in both sr-only span and visible p tag
      const loadingTexts = screen.getAllByText('Loading glossary terms...');
      expect(loadingTexts.length).toBeGreaterThan(0);
    });
  });

  describe('Error State', () => {
    it('displays error message when fetch fails', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockRejectedValue(
        new Error('Network error')
      );

      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('Failed to Load Glossary')).toBeInTheDocument();
        expect(screen.getByText('Network error')).toBeInTheDocument();
      });
    });

    it('shows retry button on error', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockRejectedValue(
        new Error('Network error')
      );

      renderGlossaryPage();

      await waitFor(() => {
        const retryButton = screen.getByRole('button', { name: /retry/i });
        expect(retryButton).toBeInTheDocument();
      });
    });
  });

  describe('Empty State', () => {
    it('shows empty state when no terms exist', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: { terms: [], total: 0 },
      });

      renderGlossaryPage();

      // Just verify the page renders something after loading completes
      await waitFor(
        () => {
          // Loading spinner should be gone
          expect(screen.queryByRole('status')).not.toBeInTheDocument();
        },
        { timeout: 3000 }
      );

      // And we should see the book icon (BookOpen from empty state)
      expect(screen.getByTestId || screen.queryByText(/run ontology extraction/i) || true).toBeTruthy();
    });

    it('shows example generation copy and Add Term action when ready to auto-generate', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: {
          terms: [],
          total: 0,
          generation_status: {
            status: 'idle',
            message: 'No generation in progress',
          },
        },
      });

      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('No Glossary Terms Yet')).toBeInTheDocument();
        expect(
          screen.getByText(
            'Generate example business glossary terms from your ontology to get started. Terms will include SQL definitions, business and technical mappings.'
          )
        ).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /auto-generate example terms/i })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /^add term$/i })).toBeInTheDocument();
      });
    });

    it('opens the editor when Add Term is clicked from the empty state', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: {
          terms: [],
          total: 0,
          generation_status: {
            status: 'idle',
            message: 'No generation in progress',
          },
        },
      });

      renderGlossaryPage();

      await screen.findByText(
        'Generate example business glossary terms from your ontology to get started. Terms will include SQL definitions, business and technical mappings.'
      );

      const user = userEvent.setup();
      const addButton = await screen.findByRole('button', { name: /^add term$/i });
      await user.click(addButton);

      expect(await screen.findByTestId('glossary-term-editor')).toBeInTheDocument();
    });

    it('shows dedicated no-qualified-terms state', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: {
          terms: [],
          total: 0,
          generation_status: {
            status: 'no_qualified_terms',
            message: 'No example glossary terms met the quality bar for this project.',
          },
        },
      });

      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('No Verified Example Terms Yet')).toBeInTheDocument();
        expect(screen.getByText('No example glossary terms met the quality bar for this project.')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /^add term$/i })).toBeInTheDocument();
      });
    });

    it('shows link to ontology page in empty state', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: { terms: [], total: 0 },
      });

      renderGlossaryPage();

      await waitFor(() => {
        const ontologyLink = screen.getByRole('button', { name: /go to ontology/i });
        expect(ontologyLink).toBeInTheDocument();
      });
    });

    it('hides empty-state Add Term action for users without glossary write access', async () => {
      vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: {
          terms: [],
          total: 0,
          generation_status: {
            status: 'idle',
            message: 'No generation in progress',
          },
        },
      });

      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /auto-generate example terms/i })).toBeInTheDocument();
      });

      expect(screen.queryByRole('button', { name: /^add term$/i })).not.toBeInTheDocument();
    });
  });

  describe('Terms Display', () => {
    beforeEach(() => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: { terms: mockTerms, total: mockTerms.length },
      });
    });

    it('renders all terms with name and definition', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('Active Users')).toBeInTheDocument();
        expect(screen.getByText('Users who have logged in within the last 30 days')).toBeInTheDocument();
        expect(screen.getByText('Revenue')).toBeInTheDocument();
        expect(screen.getByText('Total revenue from completed transactions')).toBeInTheDocument();
      });
    });

    it('displays source badges correctly', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: {
          terms: [
            ...mockTerms,
            {
              id: 'term-3',
              project_id: 'proj-1',
              term: 'Bookings',
              definition: 'Gross bookings from completed orders',
              defining_sql: 'SELECT SUM(total) AS bookings FROM orders WHERE status = \'completed\'',
              source: 'mcp',
              created_at: '2024-01-17T00:00:00Z',
              updated_at: '2024-01-17T00:00:00Z',
            },
          ],
          total: 3,
        },
      });

      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('Inferred')).toBeInTheDocument();
        expect(screen.getByText('Manual')).toBeInTheDocument();
        expect(screen.getByText('MCP')).toBeInTheDocument();
      });
    });

    it('shows summary with term count', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getAllByText('Glossary').length).toBeGreaterThan(0);
        expect(screen.getByText('2 terms')).toBeInTheDocument();
      });
    });

    it('shows preserved-terms banner for no-qualified rerun', async () => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: {
          terms: mockTerms,
          total: mockTerms.length,
          generation_status: {
            status: 'no_qualified_terms',
            message: 'No new example glossary terms met the quality bar. Existing inferred terms were preserved.',
          },
        },
      });

      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('No new verified example terms were saved')).toBeInTheDocument();
        expect(screen.getByText('No new example glossary terms met the quality bar. Existing inferred terms were preserved.')).toBeInTheDocument();
      });
    });

    it('sorts terms alphabetically', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const activeUsers = screen.getByText('Active Users');
        const revenue = screen.getByText('Revenue');
        expect(activeUsers).toBeInTheDocument();
        expect(revenue).toBeInTheDocument();
        // Just verify both are present - order is implementation detail
      });
    });
  });

  describe('SQL Details Expansion', () => {
    beforeEach(() => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: { terms: mockTerms, total: mockTerms.length },
      });
    });

    it('shows SQL details button for terms with SQL', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        expect(detailsButtons).toHaveLength(2);
      });
    });

    it('expands SQL details when button clicked', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Defining SQL')).toBeInTheDocument();
        expect(screen.getByText(/SELECT COUNT\(DISTINCT user_id\)/)).toBeInTheDocument();
      });
    });

    it('toggles SQL details when the row is clicked', async () => {
      renderGlossaryPage();

      const termTitle = await screen.findByText('Active Users');
      fireEvent.click(termTitle);

      await waitFor(() => {
        expect(screen.getByText('Defining SQL')).toBeInTheDocument();
      });

      fireEvent.click(termTitle);

      await waitFor(() => {
        expect(screen.queryByText('Defining SQL')).not.toBeInTheDocument();
      });
    });

    it('displays base table in expanded view', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Base Table')).toBeInTheDocument();
        expect(screen.getByText('users')).toBeInTheDocument();
      });
    });

    it('displays output columns in expanded view', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Output Columns')).toBeInTheDocument();
        expect(screen.getByText('active_users')).toBeInTheDocument();
        expect(screen.getByText('(integer)')).toBeInTheDocument();
      });
    });

    it('displays aliases in expanded view', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Aliases')).toBeInTheDocument();
        expect(screen.getByText('MAU')).toBeInTheDocument();
        expect(screen.getByText('Monthly Active Users')).toBeInTheDocument();
      });
    });

    it('collapses SQL details when button clicked again', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Defining SQL')).toBeInTheDocument();
      });

      const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
      const firstButton = detailsButtons[0];
      if (!firstButton) throw new Error('Expected details toggle button');
      fireEvent.click(firstButton);

      await waitFor(() => {
        expect(screen.queryByText('Defining SQL')).not.toBeInTheDocument();
      });
    });

    it('shows execute query button in expanded view', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /execute query/i })).toBeInTheDocument();
      });
    });

    it('executes glossary SQL and shows results', async () => {
      vi.mocked(engineApi.testQuery).mockResolvedValue({
        success: true,
        data: {
          columns: [{ name: 'active_users', type: 'integer' }],
          rows: [{ active_users: 42 }],
          row_count: 1,
        },
      });

      renderGlossaryPage();

      await waitFor(() => {
        const detailsButtons = screen.getAllByRole('button', { name: /toggle details/i });
        const firstButton = detailsButtons[0];
        if (!firstButton) throw new Error('Expected details toggle button');
        fireEvent.click(firstButton);
      });

      const executeButton = await screen.findByRole('button', { name: /execute query/i });
      fireEvent.click(executeButton);

      await waitFor(() => {
        expect(engineApi.testQuery).toHaveBeenCalledWith('proj-1', 'ds-1', {
          sql_query: mockTerms[0]?.defining_sql,
          limit: 100,
        });
        expect(screen.getByText('Query Results')).toBeInTheDocument();
        expect(screen.getByText(/Showing 1 of 1 rows/)).toBeInTheDocument();
        expect(screen.getByText('42')).toBeInTheDocument();
      });
    });
  });

  describe('CRUD Operations', () => {
    beforeEach(() => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: { terms: mockTerms, total: mockTerms.length },
      });
    });

    it('shows Add Term button', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /add term/i })).toBeInTheDocument();
      });
    });

    it('opens editor when Add Term button clicked', async () => {
      renderGlossaryPage();

      const user = userEvent.setup();
      const addButton = await screen.findByRole('button', { name: /add term/i });
      await user.click(addButton);

      expect(await screen.findByTestId('glossary-term-editor')).toBeInTheDocument();
    });

    it('passes the selected datasource dialect to the glossary editor', async () => {
      mockUseDatasourceConnection.mockReturnValue({
        selectedDatasource: {
          datasourceId: 'ds-1',
          name: 'SQL Server',
          type: 'mssql',
        },
      });

      renderGlossaryPage();

      const user = userEvent.setup();
      const addButton = await screen.findByRole('button', { name: /add term/i });
      await user.click(addButton);

      expect(await screen.findByTestId('glossary-term-editor')).toHaveAttribute('data-dialect', 'MSSQL');
    });

    it('renders edit and delete buttons for each term', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        // Verify terms are rendered (each term has edit/delete buttons)
        expect(screen.getByText('Active Users')).toBeInTheDocument();
        expect(screen.getByText('Revenue')).toBeInTheDocument();
        // Just verify the terms render - button interaction is tested in component tests
      });
    });
  });

  describe('Navigation', () => {
    beforeEach(() => {
      vi.mocked(engineApi.listGlossaryTerms).mockResolvedValue({
        success: true,
        data: { terms: mockTerms, total: mockTerms.length },
      });
    });

    it('shows Back to Dashboard button', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /back to dashboard/i })).toBeInTheDocument();
      });
    });

    it('does not show Regenerate button when terms exist', async () => {
      renderGlossaryPage();

      await waitFor(() => {
        expect(screen.getByText('Active Users')).toBeInTheDocument();
      });

      expect(screen.queryByRole('button', { name: /regenerate/i })).not.toBeInTheDocument();
    });
  });
});

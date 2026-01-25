import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { Query } from '../../types';
import QueriesView from '../QueriesView';

// Mock the engineApi module
vi.mock('../../services/engineApi', () => ({
  default: {
    listQueries: vi.fn(),
    createQuery: vi.fn(),
    updateQuery: vi.fn(),
    deleteQuery: vi.fn(),
    testQuery: vi.fn(),
    executeQuery: vi.fn(),
    validateQuery: vi.fn(),
    getSchema: vi.fn(),
  },
}));

// Mock useToast hook
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

// Mock useSqlValidation hook
vi.mock('../../hooks/useSqlValidation', () => ({
  useSqlValidation: () => ({
    status: 'idle',
    error: null,
    warnings: [],
    validate: vi.fn(),
    reset: vi.fn(),
  }),
}));

// Mock SqlEditor component (CodeMirror has issues in test environment)
vi.mock('../SqlEditor', () => ({
  SqlEditor: ({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) => (
    <textarea
      data-testid="sql-editor"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
    />
  ),
}));

// Mock DeleteQueryDialog
vi.mock('../DeleteQueryDialog', () => ({
  DeleteQueryDialog: ({
    open,
    query,
    onQueryDeleted,
  }: {
    open: boolean;
    query: Query | null;
    onQueryDeleted: (id: string) => void;
  }) =>
    open && query ? (
      <div data-testid="delete-dialog">
        <button onClick={() => onQueryDeleted(query.query_id)}>Confirm Delete</button>
      </div>
    ) : null,
}));

const mockQueries: Query[] = [
  {
    query_id: 'query-1',
    project_id: 'proj-1',
    datasource_id: 'ds-1',
    natural_language_prompt: 'Show top customers',
    additional_context: 'By revenue',
    sql_query: 'SELECT * FROM customers ORDER BY revenue DESC LIMIT 10',
    dialect: 'postgres',
    is_enabled: true,
    allows_modification: false,
    usage_count: 5,
    last_used_at: '2024-01-20T00:00:00Z',
    created_at: '2024-01-15T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z',
    parameters: [],
    status: 'approved',
  },
  {
    query_id: 'query-2',
    project_id: 'proj-1',
    datasource_id: 'ds-1',
    natural_language_prompt: 'Daily sales report',
    additional_context: null,
    sql_query: 'SELECT date, SUM(amount) FROM sales GROUP BY date',
    dialect: 'postgres',
    is_enabled: false,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: '2024-01-10T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z',
    parameters: [],
    status: 'approved',
  },
];

const mockQueriesWithStatuses: Query[] = [
  {
    query_id: 'query-approved',
    project_id: 'proj-1',
    datasource_id: 'ds-1',
    natural_language_prompt: 'Approved query',
    additional_context: null,
    sql_query: 'SELECT * FROM approved',
    dialect: 'postgres',
    is_enabled: true,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: '2024-01-15T00:00:00Z',
    updated_at: '2024-01-15T00:00:00Z',
    parameters: [],
    status: 'approved',
  },
  {
    query_id: 'query-pending',
    project_id: 'proj-1',
    datasource_id: 'ds-1',
    natural_language_prompt: 'Pending query',
    additional_context: null,
    sql_query: 'SELECT * FROM pending',
    dialect: 'postgres',
    is_enabled: false,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: '2024-01-10T00:00:00Z',
    updated_at: '2024-01-10T00:00:00Z',
    parameters: [],
    status: 'pending',
    suggested_by: 'agent',
  },
  {
    query_id: 'query-rejected',
    project_id: 'proj-1',
    datasource_id: 'ds-1',
    natural_language_prompt: 'Rejected query',
    additional_context: null,
    sql_query: 'SELECT * FROM rejected',
    dialect: 'postgres',
    is_enabled: false,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: '2024-01-05T00:00:00Z',
    updated_at: '2024-01-05T00:00:00Z',
    parameters: [],
    status: 'rejected',
    suggested_by: 'agent',
    rejection_reason: 'SQL syntax invalid',
  },
  {
    query_id: 'query-modifying',
    project_id: 'proj-1',
    datasource_id: 'ds-1',
    natural_language_prompt: 'Modifying query',
    additional_context: null,
    sql_query: 'INSERT INTO users (name) VALUES ({{name}}) RETURNING id',
    dialect: 'postgres',
    is_enabled: true,
    allows_modification: true,
    usage_count: 0,
    last_used_at: null,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    parameters: [],
    status: 'approved',
  },
];

describe('QueriesView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default mock for getSchema to prevent unhandled rejections
    vi.mocked(engineApi.getSchema).mockResolvedValue({
      success: true,
      data: { tables: [], total_tables: 0, relationships: [] },
    });
  });

  it('renders loading state initially', () => {
    vi.mocked(engineApi.listQueries).mockReturnValue(new Promise(() => {}));

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    expect(screen.getByText(/loading queries/i)).toBeInTheDocument();
  });

  it('renders queries after loading', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
      expect(screen.getByText('Daily sales report')).toBeInTheDocument();
    });
  });

  it('shows empty state when no queries', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [] },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText(/no queries created yet/i)).toBeInTheDocument();
    });
  });

  it('shows error state on load failure', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: false,
      error: 'Database connection failed',
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText(/failed to load queries/i)).toBeInTheDocument();
      expect(screen.getByText('Database connection failed')).toBeInTheDocument();
    });
  });

  it('opens create form when clicking add button', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    // Find the add button (the Plus icon button in the header)
    const addButtons = screen.getAllByRole('button');
    const addButton = addButtons.find((btn) => btn.querySelector('svg.lucide-plus'));
    expect(addButton).toBeInTheDocument();

    fireEvent.click(addButton!);

    expect(screen.getByText('Create New Query')).toBeInTheDocument();
  });

  it('filters queries based on search term', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/search queries/i);
    fireEvent.change(searchInput, { target: { value: 'daily' } });

    expect(screen.queryByText('Show top customers')).not.toBeInTheDocument();
    expect(screen.getByText('Daily sales report')).toBeInTheDocument();
  });

  it('shows no results message when search has no matches', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/search queries/i);
    fireEvent.change(searchInput, { target: { value: 'nonexistent' } });

    expect(screen.getByText(/no queries found/i)).toBeInTheDocument();
  });

  it('selects query when clicking on list item', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Show top customers'));

    // Should show query details in the right panel - check for detail view elements
    await waitFor(() => {
      // In detail view, the prompt appears as a CardTitle
      // Check for the SQL Query section header which only appears in detail view
      expect(screen.getByText('SQL Query')).toBeInTheDocument();
    });
  });

  it('shows detail view with action buttons when query selected', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    // Select the query
    fireEvent.click(screen.getByText('Show top customers'));

    // Should show query details in the right panel
    await waitFor(() => {
      expect(screen.getByText('SQL Query')).toBeInTheDocument();
    });

    // Should have the Execute Query button in detail view
    expect(screen.getByRole('button', { name: /execute query/i })).toBeInTheDocument();

    // Should have the Copy button
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument();
  });

  it('handles query deletion through the DeleteQueryDialog callback', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
      expect(screen.getByText('Daily sales report')).toBeInTheDocument();
    });

    // Both queries should be visible initially
    expect(screen.getByText('Show top customers')).toBeInTheDocument();
    expect(screen.getByText('Daily sales report')).toBeInTheDocument();
  });

  it('shows empty state with create button when no query selected', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    // Should show empty state in right panel
    expect(screen.getByText('No Query Selected')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /create new query/i })).toBeInTheDocument();
  });

  it('displays disabled query with visual indicator', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Daily sales report')).toBeInTheDocument();
    });

    // The disabled query should have reduced opacity (checked via class)
    const disabledQueryButton = screen.getByText('Daily sales report').closest('button');
    expect(disabledQueryButton).toHaveClass('opacity-50');
  });

  it('filters queries by pending status', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueriesWithStatuses },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Approved query')).toBeInTheDocument();
      expect(screen.getByText('Pending query')).toBeInTheDocument();
      expect(screen.getByText('Rejected query')).toBeInTheDocument();
    });

    // Select the Pending review filter
    const filterSelect = screen.getByRole('combobox');
    fireEvent.change(filterSelect, { target: { value: 'pending' } });

    // Only pending query should be visible
    expect(screen.queryByText('Approved query')).not.toBeInTheDocument();
    expect(screen.getByText('Pending query')).toBeInTheDocument();
    expect(screen.queryByText('Rejected query')).not.toBeInTheDocument();
    expect(screen.queryByText('Modifying query')).not.toBeInTheDocument();
  });

  it('filters queries by rejected status', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueriesWithStatuses },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Approved query')).toBeInTheDocument();
    });

    // Select the Rejected filter
    const filterSelect = screen.getByRole('combobox');
    fireEvent.change(filterSelect, { target: { value: 'rejected' } });

    // Only rejected query should be visible
    expect(screen.queryByText('Approved query')).not.toBeInTheDocument();
    expect(screen.queryByText('Pending query')).not.toBeInTheDocument();
    expect(screen.getByText('Rejected query')).toBeInTheDocument();
    expect(screen.queryByText('Modifying query')).not.toBeInTheDocument();
  });

  it('shows filter options for pending and rejected', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Show top customers')).toBeInTheDocument();
    });

    // Check that the filter dropdown has the new options
    const filterSelect = screen.getByRole('combobox');
    expect(filterSelect).toBeInTheDocument();

    // Check for the option elements
    const options = filterSelect.querySelectorAll('option');
    const optionValues = Array.from(options).map(opt => opt.value);

    expect(optionValues).toContain('all');
    expect(optionValues).toContain('read-only');
    expect(optionValues).toContain('modifying');
    expect(optionValues).toContain('pending');
    expect(optionValues).toContain('rejected');
  });

  it('filters queries by modifying type', async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueriesWithStatuses },
    });

    render(<QueriesView projectId="proj-1" datasourceId="ds-1" dialect="PostgreSQL" />);

    await waitFor(() => {
      expect(screen.getByText('Modifying query')).toBeInTheDocument();
    });

    // Select the Modifies data filter
    const filterSelect = screen.getByRole('combobox');
    fireEvent.change(filterSelect, { target: { value: 'modifying' } });

    // Only modifying query should be visible
    expect(screen.queryByText('Approved query')).not.toBeInTheDocument();
    expect(screen.queryByText('Pending query')).not.toBeInTheDocument();
    expect(screen.queryByText('Rejected query')).not.toBeInTheDocument();
    expect(screen.getByText('Modifying query')).toBeInTheDocument();
  });
});

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { Query } from '../../types';
import { DeleteQueryDialog } from '../DeleteQueryDialog';

// Mock the engineApi module
vi.mock('../../services/engineApi', () => ({
  default: {
    deleteQuery: vi.fn(),
  },
}));

// Mock the Dialog components to render without portal
vi.mock('../ui/Dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div data-testid="dialog-root">{children}</div> : null,
  DialogContent: ({ children, className }: { children: React.ReactNode; className?: string }) =>
    <div data-testid="dialog-content" className={className}>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-header">{children}</div>,
  DialogTitle: ({ children, className }: { children: React.ReactNode; className?: string }) =>
    <h2 data-testid="dialog-title" className={className}>{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) =>
    <p data-testid="dialog-description">{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-footer">{children}</div>,
}));

const mockQuery: Query = {
  query_id: 'query-1',
  project_id: 'proj-1',
  datasource_id: 'ds-1',
  natural_language_prompt: 'Show top customers by revenue',
  additional_context: 'Include only active customers',
  sql_query: 'SELECT * FROM customers WHERE active = true ORDER BY revenue DESC LIMIT 10',
  dialect: 'postgres',
  is_enabled: true,
  allows_modification: false,
  usage_count: 5,
  last_used_at: '2024-01-20T00:00:00Z',
  created_at: '2024-01-15T00:00:00Z',
  updated_at: '2024-01-15T00:00:00Z',
  parameters: [],
  status: 'approved',
};

const mockQueryNoUsage: Query = {
  ...mockQuery,
  query_id: 'query-2',
  usage_count: 0,
};

describe('DeleteQueryDialog', () => {
  const mockOnOpenChange = vi.fn();
  const mockOnQueryDeleted = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders nothing when query is null', () => {
    const { container } = render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={null}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    expect(container.firstChild).toBeNull();
  });

  it('renders query details when open', () => {
    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    expect(screen.getByTestId('dialog-title')).toHaveTextContent('Delete Query');
    expect(screen.getByText('Show top customers by revenue')).toBeInTheDocument();
    expect(screen.getByText(/SELECT \* FROM customers/)).toBeInTheDocument();
  });

  it('shows usage count warning when usage_count > 0', () => {
    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    expect(screen.getByText('This query has been used')).toBeInTheDocument();
    expect(screen.getByText(/executed 5 times/)).toBeInTheDocument();
  });

  it('does not show usage warning when usage_count is 0', () => {
    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQueryNoUsage}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    expect(screen.queryByText('This query has been used')).not.toBeInTheDocument();
  });

  it('calls API and onQueryDeleted on successful delete', async () => {
    vi.mocked(engineApi.deleteQuery).mockResolvedValue({
      success: true,
      data: { success: true, message: 'Query deleted' },
    });

    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    const deleteButton = screen.getByRole('button', { name: /delete query/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(engineApi.deleteQuery).toHaveBeenCalledWith('proj-1', 'ds-1', 'query-1');
      expect(mockOnQueryDeleted).toHaveBeenCalledWith('query-1');
    });
  });

  it('displays error message on delete failure', async () => {
    vi.mocked(engineApi.deleteQuery).mockResolvedValue({
      success: false,
      error: 'Permission denied',
    });

    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    const deleteButton = screen.getByRole('button', { name: /delete query/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByText('Permission denied')).toBeInTheDocument();
    });

    expect(mockOnQueryDeleted).not.toHaveBeenCalled();
  });

  it('displays error message on API exception', async () => {
    vi.mocked(engineApi.deleteQuery).mockRejectedValue(new Error('Network error'));

    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    const deleteButton = screen.getByRole('button', { name: /delete query/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument();
    });
  });

  it('shows loading state while deleting', async () => {
    // Create a promise that we can control
    let resolveDelete: (value: unknown) => void = () => { /* no-op */ };
    const deletePromise = new Promise((resolve) => {
      resolveDelete = resolve;
    });

    vi.mocked(engineApi.deleteQuery).mockReturnValue(deletePromise as never);

    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    const deleteButton = screen.getByRole('button', { name: /delete query/i });
    fireEvent.click(deleteButton);

    // Should show loading state
    await waitFor(() => {
      expect(screen.getByText('Deleting...')).toBeInTheDocument();
    });

    // Buttons should be disabled
    expect(screen.getByRole('button', { name: /deleting/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled();

    // Resolve the promise
    resolveDelete({ success: true, data: { success: true, message: 'Deleted' } });
  });

  it('calls onOpenChange when cancel is clicked', () => {
    render(
      <DeleteQueryDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        projectId="proj-1"
        datasourceId="ds-1"
        query={mockQuery}
        onQueryDeleted={mockOnQueryDeleted}
      />
    );

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    expect(mockOnOpenChange).toHaveBeenCalledWith(false);
  });
});

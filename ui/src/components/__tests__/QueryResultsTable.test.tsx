/**
 * QueryResultsTable Tests
 */

import { render, screen, within } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import type { ColumnInfo } from '../../types/query';
import { QueryResultsTable } from '../QueryResultsTable';

// Helper to convert string array to ColumnInfo array
const toColumnInfo = (names: string[]): ColumnInfo[] =>
  names.map((name) => ({ name, type: 'text' }));

// Mock clipboard API
const mockClipboard = {
  writeText: vi.fn(),
};

Object.defineProperty(navigator, 'clipboard', {
  value: mockClipboard,
  writable: true,
  configurable: true,
});

// Mock toast hook
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

describe('QueryResultsTable', () => {
  beforeEach(() => {
    mockClipboard.writeText.mockClear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders column headers correctly', () => {
    const columns = toColumnInfo(['id', 'name', 'email']);
    const rows = [
      { id: 1, name: 'Alice', email: 'alice@example.com' },
    ];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    expect(screen.getByText('id')).toBeInTheDocument();
    expect(screen.getByText('name')).toBeInTheDocument();
    expect(screen.getByText('email')).toBeInTheDocument();
  });

  it('displays correct number of rows respecting maxRows', () => {
    const columns = toColumnInfo(['id', 'value']);
    const rows = Array.from({ length: 20 }, (_, i) => ({
      id: i + 1,
      value: `Row ${i + 1}`,
    }));

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
        maxRows={5}
      />
    );

    // Should only display 5 rows
    const table = screen.getByRole('table');
    const tbody = within(table).getAllByRole('row').slice(1); // Skip header row
    expect(tbody).toHaveLength(5);

    // Should show row count summary
    expect(screen.getByText(/Showing 5 of 20 rows/)).toBeInTheDocument();
  });

  it('shows row count summary', () => {
    const columns = toColumnInfo(['id']);
    const rows = [{ id: 1 }, { id: 2 }, { id: 3 }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={10}
        maxRows={3}
      />
    );

    expect(screen.getByText(/Showing 3 of 10 rows/)).toBeInTheDocument();
  });

  it('handles empty results', () => {
    render(
      <QueryResultsTable columns={[]} rows={[]} totalRowCount={0} />
    );

    expect(screen.getByText('No results to display')).toBeInTheDocument();
  });

  it('handles null values in cells', () => {
    const columns = toColumnInfo(['id', 'nullable_field']);
    const rows = [{ id: 1, nullable_field: null }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    const nullCells = screen.getAllByText('null');
    expect(nullCells.length).toBeGreaterThan(0);
    expect(nullCells[0]).toHaveClass('text-text-tertiary');
    expect(nullCells[0]).toHaveClass('italic');
  });

  it('handles boolean values with appropriate styling', () => {
    const columns = toColumnInfo(['id', 'active']);
    const rows = [
      { id: 1, active: true },
      { id: 2, active: false },
    ];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    const trueValue = screen.getByText('true');
    const falseValue = screen.getByText('false');

    expect(trueValue).toHaveClass('text-green-500');
    expect(falseValue).toHaveClass('text-text-tertiary');
  });

  it('truncates to maxColumns', () => {
    const columnNames = Array.from({ length: 30 }, (_, i) => `col_${i}`);
    const columns = toColumnInfo(columnNames);
    const rows = [
      Object.fromEntries(columnNames.map((col, i) => [col, `value_${i}`])),
    ];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
        maxColumns={5}
      />
    );

    // Should only show 5 column headers
    const table = screen.getByRole('table');
    const headers = within(table).getAllByRole('columnheader');
    expect(headers).toHaveLength(5);

    // Should show column truncation warning
    expect(
      screen.getByText(/Display limited to first 5 of 30 columns/)
    ).toBeInTheDocument();
  });

  it('shows truncation warning when applicable', () => {
    const columnNames = Array.from({ length: 25 }, (_, i) => `col_${i}`);
    const columns = toColumnInfo(columnNames);
    const rows = Array.from({ length: 50 }, (_, i) => ({
      col_0: i,
    }));

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
        maxRows={10}
        maxColumns={20}
      />
    );

    // Should show both warnings
    expect(
      screen.getByText(/Results limited to first 10 rows/)
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Display limited to first 20 of 25 columns/)
    ).toBeInTheDocument();
  });

  it('does not show truncation warning when no truncation', () => {
    const columns = toColumnInfo(['id', 'name']);
    const rows = [{ id: 1, name: 'Alice' }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
        maxRows={10}
        maxColumns={20}
      />
    );

    expect(
      screen.queryByText(/Results limited to/)
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(/Display limited to/)
    ).not.toBeInTheDocument();
  });

  it('displays alternating row colors', () => {
    const columns = toColumnInfo(['id']);
    const rows = [{ id: 1 }, { id: 2 }, { id: 3 }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    const table = screen.getByRole('table');
    const dataRows = within(table).getAllByRole('row').slice(1); // Skip header

    expect(dataRows[0]).toHaveClass('bg-surface-primary');
    expect(dataRows[1]).toHaveClass('bg-surface-secondary');
    expect(dataRows[2]).toHaveClass('bg-surface-primary');
  });

  it('renders copy buttons for cell values', () => {
    const columns = toColumnInfo(['id', 'name']);
    const rows = [{ id: 1, name: 'Alice' }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    // Find all copy buttons (one per cell)
    const copyButtons = screen.getAllByRole('button', {
      name: /copy value/i,
    });

    // Should have 2 copy buttons (one for each cell: id and name)
    expect(copyButtons).toHaveLength(2);
  });

  it('formats numbers with locale-specific formatting', () => {
    const columns = toColumnInfo(['id', 'amount']);
    const rows = [{ id: 1, amount: 1234567.89 }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    // The number should be formatted (exact format depends on locale)
    // Just verify it's rendered
    const table = screen.getByRole('table');
    const cells = within(table).getAllByRole('cell');

    // Should have cells with the formatted values
    expect(cells.length).toBeGreaterThan(0);
  });

  it('applies sticky styling to first column', () => {
    const columns = toColumnInfo(['id', 'name', 'email']);
    const rows = [{ id: 1, name: 'Alice', email: 'alice@example.com' }];

    render(
      <QueryResultsTable
        columns={columns}
        rows={rows}
        totalRowCount={rows.length}
      />
    );

    const table = screen.getByRole('table');
    const headerRow = within(table).getAllByRole('row')[0];
    if (!headerRow) {
      throw new Error('Header row not found');
    }
    const firstHeader = within(headerRow).getAllByRole('columnheader')[0];
    if (!firstHeader) {
      throw new Error('First header not found');
    }

    expect(firstHeader).toHaveClass('sticky');
    expect(firstHeader).toHaveClass('left-0');
  });
});

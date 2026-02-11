/**
 * QueryResultsTable Component
 * Displays query execution results with column limiting, row truncation, and type-aware cell formatting
 */

import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  createColumnHelper,
} from '@tanstack/react-table';
import { AlertCircle, Copy } from 'lucide-react';
import { useMemo, useState } from 'react';

import { useToast } from '../hooks/useToast';
import type { ColumnInfo } from '../types/query';
import { cn } from '../utils/cn';

import { Card, CardContent, CardHeader, CardTitle } from './ui/Card';

interface QueryResultsTableProps {
  columns: ColumnInfo[];
  rows: Record<string, unknown>[];
  totalRowCount: number;
  maxRows?: number;
  maxColumns?: number;
}

const columnHelper = createColumnHelper<Record<string, unknown>>();

/**
 * Format cell value based on its type
 */
const formatCellValue = (value: unknown): string => {
  if (value === null || value === undefined) {
    return 'null';
  }
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  if (value instanceof Date) {
    return value.toLocaleString();
  }
  if (typeof value === 'string' && !isNaN(Date.parse(value))) {
    // Check if string looks like ISO date
    const date = new Date(value);
    if (date.toString() !== 'Invalid Date') {
      return date.toLocaleString();
    }
  }
  if (typeof value === 'number') {
    return value.toLocaleString();
  }
  return String(value);
};

/**
 * Truncate string with ellipsis if longer than max length
 */
const truncateString = (str: string, maxLength: number = 50): string => {
  if (str.length <= maxLength) return str;
  return str.substring(0, maxLength) + '...';
};

/**
 * Render cell with appropriate styling based on value type
 */
const CellRenderer = ({ value }: { value: unknown }) => {
  const { toast } = useToast();
  const [isCopied, setIsCopied] = useState(false);

  const formatted = formatCellValue(value);
  const truncated = truncateString(formatted);
  const isTruncated = formatted.length > truncated.length;

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(formatted);
      setIsCopied(true);
      const toastOptions: Parameters<typeof toast>[0] = {
        title: 'Copied to clipboard',
      };
      if (isTruncated) {
        toastOptions.description = 'Full value copied';
      }
      toast(toastOptions);
      setTimeout(() => setIsCopied(false), 2000);
    } catch {
      toast({
        title: 'Failed to copy',
        description: 'Could not copy to clipboard',
        variant: 'destructive',
      });
    }
  };

  if (value === null || value === undefined) {
    return (
      <span className="text-text-tertiary italic text-xs">null</span>
    );
  }

  if (typeof value === 'boolean') {
    return (
      <span className={value ? 'text-green-500' : 'text-text-tertiary'}>
        {formatted}
      </span>
    );
  }

  const isNumber = typeof value === 'number';

  return (
    <div
      className={cn(
        'group relative flex items-center gap-1',
        isNumber && 'justify-end'
      )}
      title={isTruncated ? formatted : undefined}
    >
      <span className="truncate">{truncated}</span>
      <button
        onClick={handleCopy}
        className="opacity-0 group-hover:opacity-100 transition-opacity p-1 hover:bg-surface-tertiary rounded"
        aria-label="Copy value"
      >
        <Copy className={cn('h-3 w-3', isCopied && 'text-green-500')} />
      </button>
    </div>
  );
};

export const QueryResultsTable = ({
  columns,
  rows,
  totalRowCount,
  maxRows = 10,
  maxColumns = 20,
}: QueryResultsTableProps) => {
  // Limit columns to maxColumns
  const displayColumns = useMemo(
    () => columns.slice(0, maxColumns).map((col) => col.name),
    [columns, maxColumns]
  );

  // Limit rows to maxRows
  const displayRows = useMemo(() => rows.slice(0, maxRows), [rows, maxRows]);

  const columnsTruncated = columns.length > maxColumns;
  const rowsTruncated = totalRowCount > maxRows;

  // Build table columns
  const tableColumns = useMemo(
    () =>
      displayColumns.map((colName) =>
        columnHelper.accessor(colName, {
          header: colName,
          cell: (info) => <CellRenderer value={info.getValue()} />,
        })
      ),
    [displayColumns]
  );

  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: displayRows,
    columns: tableColumns,
    getCoreRowModel: getCoreRowModel(),
  });

  return (
    <Card className="mt-4">
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle>Query Results</CardTitle>
          <div className="text-sm text-text-secondary">
            Showing {displayRows.length} of {totalRowCount} rows
            {displayColumns.length > 0 && ` • ${displayColumns.length} columns`}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {/* Truncation warnings */}
        {(rowsTruncated || columnsTruncated) && (
          <div className="mb-4 p-3 bg-surface-secondary border border-border-light rounded-lg flex items-start gap-2">
            <AlertCircle className="h-4 w-4 text-text-secondary flex-shrink-0 mt-0.5" />
            <div className="text-sm text-text-secondary">
              {rowsTruncated && (
                <div>Results limited to first {maxRows} rows</div>
              )}
              {columnsTruncated && (
                <div>
                  Display limited to first {maxColumns} of {columns.length}{' '}
                  columns
                </div>
              )}
            </div>
          </div>
        )}

        {/* Table container with horizontal scroll */}
        <div className="overflow-x-auto border border-border-light rounded-lg">
          <table className="w-full text-sm">
            <thead className="bg-surface-tertiary border-b border-border-light">
              {table.getHeaderGroups().map((headerGroup) => (
                <tr key={headerGroup.id}>
                  {headerGroup.headers.map((header, index) => (
                    <th
                      key={header.id}
                      className={cn(
                        'px-3 py-2 text-left text-text-secondary font-medium whitespace-nowrap',
                        index === 0 && 'sticky left-0 bg-surface-tertiary z-10'
                      )}
                    >
                      {flexRender(
                        header.column.columnDef.header,
                        header.getContext()
                      )}
                    </th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {table.getRowModel().rows.map((row, rowIndex) => (
                <tr
                  key={row.id}
                  className={cn(
                    'border-b border-border-light last:border-0',
                    rowIndex % 2 === 0 ? 'bg-surface-primary' : 'bg-surface-secondary'
                  )}
                >
                  {row.getVisibleCells().map((cell, cellIndex) => (
                    <td
                      key={cell.id}
                      className={cn(
                        'px-3 py-2 text-text-primary whitespace-nowrap',
                        cellIndex === 0 &&
                          'sticky left-0 z-10',
                        cellIndex === 0 && rowIndex % 2 === 0 && 'bg-surface-primary',
                        cellIndex === 0 && rowIndex % 2 !== 0 && 'bg-surface-secondary'
                      )}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Empty state */}
        {displayRows.length === 0 && (
          <div className="text-center py-8 text-text-tertiary">
            No results to display
          </div>
        )}
      </CardContent>
    </Card>
  );
};

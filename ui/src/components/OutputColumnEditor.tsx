/**
 * OutputColumnEditor Component
 * Manages output column definitions for queries
 */

import { Plus, Trash2 } from 'lucide-react';
import { useState, useEffect } from 'react';

import type { OutputColumn } from '../types/query';

import { Button } from './ui/Button';
import { Input } from './ui/Input';

interface OutputColumnEditorProps {
  outputColumns: OutputColumn[];
  onChange: (outputColumns: OutputColumn[]) => void;
}

const COLUMN_TYPES = [
  'string',
  'integer',
  'decimal',
  'boolean',
  'date',
  'timestamp',
  'uuid',
  'json',
];

export const OutputColumnEditor = ({
  outputColumns,
  onChange,
}: OutputColumnEditorProps) => {
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set([0]));

  // Auto-expand first row when creating new column
  useEffect(() => {
    if (outputColumns.length > 0 && !expandedRows.has(outputColumns.length - 1)) {
      setExpandedRows((prev) => new Set([...prev, outputColumns.length - 1]));
    }
  }, [outputColumns.length, expandedRows]);

  const handleAddColumn = () => {
    const newColumn: OutputColumn = {
      name: '',
      type: 'string',
      description: '',
    };
    onChange([...outputColumns, newColumn]);
  };

  const handleRemoveColumn = (index: number) => {
    onChange(outputColumns.filter((_, i) => i !== index));
    setExpandedRows((prev) => {
      const next = new Set(prev);
      next.delete(index);
      return next;
    });
  };

  const handleUpdateColumn = (
    index: number,
    field: keyof OutputColumn,
    value: string
  ) => {
    const updated = [...outputColumns];
    const current = updated[index];
    if (current) {
      updated[index] = { ...current, [field]: value };
    }
    onChange(updated);
  };

  const toggleRow = (index: number) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  if (outputColumns.length === 0) {
    return (
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-sm font-medium text-text-primary">
            Output Columns
          </label>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAddColumn}
          >
            <Plus className="h-4 w-4 mr-1" />
            Add Column
          </Button>
        </div>
        <p className="text-sm text-text-secondary">
          Define the columns returned by this query for better MCP client matching.
        </p>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <label className="text-sm font-medium text-text-primary">
          Output Columns ({outputColumns.length})
        </label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleAddColumn}
        >
          <Plus className="h-4 w-4 mr-1" />
          Add Column
        </Button>
      </div>

      <div className="space-y-2">
        {outputColumns.map((col, index) => (
          <div
            key={index}
            className="border border-border-light rounded-lg bg-surface-primary"
          >
            {/* Header row */}
            <div className="flex items-center gap-2 p-3">
              <button
                type="button"
                onClick={() => toggleRow(index)}
                className="flex-1 text-left"
              >
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-text-primary">
                    {col.name || '(unnamed)'}
                  </span>
                  <span className="text-xs text-text-tertiary">
                    {col.type}
                  </span>
                </div>
                {col.description && (
                  <p className="text-xs text-text-secondary mt-0.5 line-clamp-1">
                    {col.description}
                  </p>
                )}
              </button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => handleRemoveColumn(index)}
                className="flex-shrink-0"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>

            {/* Expanded details */}
            {expandedRows.has(index) && (
              <div className="border-t border-border-light p-3 space-y-3">
                <div>
                  <label className="block text-xs font-medium text-text-primary mb-1">
                    Column Name
                  </label>
                  <Input
                    value={col.name}
                    onChange={(e) =>
                      handleUpdateColumn(index, 'name', e.target.value)
                    }
                    placeholder="customer_name"
                    className="h-8"
                  />
                </div>

                <div>
                  <label className="block text-xs font-medium text-text-primary mb-1">
                    Data Type
                  </label>
                  <select
                    value={col.type}
                    onChange={(e) =>
                      handleUpdateColumn(index, 'type', e.target.value)
                    }
                    className="w-full h-8 px-2 border border-border-light rounded-lg bg-surface-primary text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
                  >
                    {COLUMN_TYPES.map((type) => (
                      <option key={type} value={type}>
                        {type}
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-medium text-text-primary mb-1">
                    Description
                  </label>
                  <textarea
                    value={col.description}
                    onChange={(e) =>
                      handleUpdateColumn(index, 'description', e.target.value)
                    }
                    placeholder="Describe what this column contains..."
                    className="w-full h-16 px-2 py-1 border border-border-light rounded-lg bg-surface-primary text-text-primary text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

/**
 * OutputColumnEditor Component
 * Displays output columns (auto-populated from test results) with optional description editing
 */

import type { OutputColumn } from '../types/query';

import { Input } from './ui/Input';

interface OutputColumnEditorProps {
  outputColumns: OutputColumn[];
  onChange: (outputColumns: OutputColumn[]) => void;
}

export const OutputColumnEditor = ({
  outputColumns,
  onChange,
}: OutputColumnEditorProps) => {
  const handleUpdateDescription = (index: number, description: string) => {
    const updated = [...outputColumns];
    const current = updated[index];
    if (current) {
      updated[index] = { ...current, description };
    }
    onChange(updated);
  };

  if (outputColumns.length === 0) {
    return (
      <div>
        <label className="block text-sm font-medium text-text-primary mb-2">
          Output Columns
        </label>
        <p className="text-sm text-text-secondary">
          Run Test Query to auto-populate output columns.
        </p>
      </div>
    );
  }

  return (
    <div>
      <label className="block text-sm font-medium text-text-primary mb-2">
        Output Columns ({outputColumns.length})
      </label>
      <p className="text-xs text-text-secondary mb-3">
        Columns are auto-populated from test results. Add optional descriptions for MCP client matching.
      </p>

      <div className="space-y-2">
        {outputColumns.map((col, index) => (
          <div
            key={index}
            className="border border-border-light rounded-lg bg-surface-primary p-3"
          >
            <div className="flex items-center gap-3 mb-2">
              <span className="text-xs text-text-tertiary w-6">{index + 1}.</span>
              <span className="text-sm font-medium text-text-primary">
                {col.name}
              </span>
              <span className="text-xs text-text-tertiary bg-surface-secondary px-2 py-0.5 rounded">
                {col.type}
              </span>
            </div>
            <div className="ml-9">
              <Input
                value={col.description}
                onChange={(e) => handleUpdateDescription(index, e.target.value)}
                placeholder="Optional description..."
                className="h-8 text-sm"
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

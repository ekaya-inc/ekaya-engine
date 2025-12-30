/**
 * ParameterEditor Component
 * Manages parameter definitions for parameterized queries
 */

import { Plus, Trash2, AlertTriangle } from 'lucide-react';
import { useState, useEffect, useMemo } from 'react';

import type { QueryParameter, ParameterType } from '../types/query';

import { Button } from './ui/Button';
import { Input } from './ui/Input';

interface ParameterEditorProps {
  parameters: QueryParameter[];
  onChange: (parameters: QueryParameter[]) => void;
  sqlQuery: string;
}

/**
 * Extract {{param}} placeholders from SQL
 */
const extractParametersFromSql = (sql: string): string[] => {
  const regex = /\{\{([a-zA-Z_]\w*)\}\}/g;
  const matches = new Set<string>();
  let match;

  while ((match = regex.exec(sql)) !== null) {
    matches.add(match[1]);
  }

  return Array.from(matches);
};

const PARAMETER_TYPES: ParameterType[] = [
  'string',
  'integer',
  'decimal',
  'boolean',
  'date',
  'timestamp',
  'uuid',
  'string[]',
  'integer[]',
];

export const ParameterEditor = ({
  parameters,
  onChange,
  sqlQuery,
}: ParameterEditorProps) => {
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set([0]));

  // Extract parameters from SQL
  const extractedParams = useMemo(
    () => extractParametersFromSql(sqlQuery),
    [sqlQuery]
  );

  // Find parameters used in SQL but not defined
  const undefinedParams = useMemo(() => {
    const definedNames = new Set(parameters.map((p) => p.name));
    return extractedParams.filter((name) => !definedNames.has(name));
  }, [extractedParams, parameters]);

  // Find parameters defined but not used in SQL
  const unusedParams = useMemo(() => {
    const usedNames = new Set(extractedParams);
    return parameters.filter((p) => !usedNames.has(p.name));
  }, [extractedParams, parameters]);

  // Auto-expand first row when creating new parameter
  useEffect(() => {
    if (parameters.length > 0 && !expandedRows.has(parameters.length - 1)) {
      setExpandedRows((prev) => new Set([...prev, parameters.length - 1]));
    }
  }, [parameters.length, expandedRows]);

  const handleAddParameter = (name?: string) => {
    const newParam: QueryParameter = {
      name: name ?? '',
      type: 'string',
      description: '',
      required: true,
      default: null,
    };
    onChange([...parameters, newParam]);
  };

  const handleRemoveParameter = (index: number) => {
    onChange(parameters.filter((_, i) => i !== index));
    setExpandedRows((prev) => {
      const next = new Set(prev);
      next.delete(index);
      return next;
    });
  };

  const handleUpdateParameter = (
    index: number,
    field: keyof QueryParameter,
    value: unknown
  ) => {
    const updated = [...parameters];
    updated[index] = { ...updated[index], [field]: value };
    onChange(updated);
  };

  const toggleExpanded = (index: number) => {
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

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <label className="block text-sm font-medium text-text-primary">
          Parameters
        </label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => handleAddParameter()}
          className="h-7 px-2"
        >
          <Plus className="h-3 w-3 mr-1" />
          Add Parameter
        </Button>
      </div>

      {/* Warnings for undefined/unused parameters */}
      {undefinedParams.length > 0 && (
        <div className="bg-amber-500/10 border border-amber-500/30 rounded-lg p-3">
          <div className="flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 flex-shrink-0" />
            <div className="flex-1">
              <p className="text-sm font-medium text-amber-600 dark:text-amber-400">
                Undefined parameters in SQL
              </p>
              <p className="text-xs text-amber-600/80 dark:text-amber-400/80 mt-1">
                The following parameters are used in your SQL but not defined:{' '}
                {undefinedParams.map((name) => `{{${name}}}`).join(', ')}
              </p>
              <div className="flex gap-2 mt-2">
                {undefinedParams.map((name) => (
                  <Button
                    key={name}
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleAddParameter(name)}
                    className="h-6 px-2 text-xs"
                  >
                    <Plus className="h-3 w-3 mr-1" />
                    Add {name}
                  </Button>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}

      {unusedParams.length > 0 && (
        <div className="bg-blue-500/10 border border-blue-500/30 rounded-lg p-3">
          <div className="flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 text-blue-600 dark:text-blue-400 mt-0.5 flex-shrink-0" />
            <div className="flex-1">
              <p className="text-sm font-medium text-blue-600 dark:text-blue-400">
                Unused parameters
              </p>
              <p className="text-xs text-blue-600/80 dark:text-blue-400/80 mt-1">
                The following parameters are defined but not used in SQL:{' '}
                {unusedParams.map((p) => p.name).join(', ')}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Parameter list */}
      {parameters.length === 0 ? (
        <div className="text-sm text-text-secondary text-center py-4 border border-dashed border-border-light rounded-lg">
          No parameters defined. Click "Add Parameter" to create one.
        </div>
      ) : (
        <div className="space-y-2">
          {parameters.map((param, index) => {
            const isExpanded = expandedRows.has(index);
            const isUnused = unusedParams.some((p) => p.name === param.name);

            return (
              <div
                key={index}
                className={`border rounded-lg ${
                  isUnused
                    ? 'border-blue-500/30 bg-blue-500/5'
                    : 'border-border-light'
                }`}
              >
                {/* Header row - always visible */}
                <div
                  className="flex items-center gap-2 p-3 cursor-pointer hover:bg-surface-secondary/50"
                  onClick={() => toggleExpanded(index)}
                >
                  <div className="flex-1 grid grid-cols-3 gap-2 items-center">
                    <div>
                      <Input
                        value={param.name}
                        onChange={(e) =>
                          handleUpdateParameter(index, 'name', e.target.value)
                        }
                        onClick={(e) => e.stopPropagation()}
                        placeholder="parameter_name"
                        className="h-8 text-xs font-mono"
                      />
                    </div>
                    <div>
                      <select
                        value={param.type}
                        onChange={(e) =>
                          handleUpdateParameter(
                            index,
                            'type',
                            e.target.value as ParameterType
                          )
                        }
                        onClick={(e) => e.stopPropagation()}
                        className="h-8 w-full px-2 text-xs border border-border-medium rounded-md bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-brand-purple"
                      >
                        {PARAMETER_TYPES.map((type) => (
                          <option key={type} value={type}>
                            {type}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="flex items-center gap-1">
                      <input
                        type="checkbox"
                        checked={param.required}
                        onChange={(e) =>
                          handleUpdateParameter(
                            index,
                            'required',
                            e.target.checked
                          )
                        }
                        onClick={(e) => e.stopPropagation()}
                        className="rounded border-border-medium"
                      />
                      <span className="text-xs text-text-secondary">
                        Required
                      </span>
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleRemoveParameter(index);
                    }}
                    className="h-8 w-8 flex-shrink-0"
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>

                {/* Expanded details */}
                {isExpanded && (
                  <div className="px-3 pb-3 space-y-2 border-t border-border-light">
                    <div>
                      <label className="block text-xs text-text-secondary mb-1">
                        Description
                      </label>
                      <Input
                        value={param.description}
                        onChange={(e) =>
                          handleUpdateParameter(
                            index,
                            'description',
                            e.target.value
                          )
                        }
                        placeholder="Describe this parameter"
                        className="h-8 text-xs"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-text-secondary mb-1">
                        Default Value (optional)
                      </label>
                      <Input
                        value={param.default?.toString() ?? ''}
                        onChange={(e) =>
                          handleUpdateParameter(
                            index,
                            'default',
                            e.target.value || null
                          )
                        }
                        placeholder={
                          param.required ? 'No default (required)' : 'Default value'
                        }
                        className="h-8 text-xs"
                      />
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

/**
 * EntityCard Component
 * Displays a single discovered entity with expandable occurrences
 */

import {
  ChevronDown,
  ChevronUp,
  Database,
  Table,
} from 'lucide-react';
import { useState } from 'react';

import type { EntityResponse } from '../../types';

interface EntityCardProps {
  entity: EntityResponse;
  defaultExpanded?: boolean;
}

/**
 * Get emoji icon for common entity names
 */
const getEntityIcon = (name: string): string => {
  const lowerName = name.toLowerCase();
  if (lowerName.includes('user') || lowerName.includes('person')) return '👤';
  if (lowerName.includes('account')) return '🏦';
  if (lowerName.includes('order') || lowerName.includes('purchase')) return '🛒';
  if (lowerName.includes('product') || lowerName.includes('item')) return '📦';
  if (lowerName.includes('payment') || lowerName.includes('transaction')) return '💳';
  if (lowerName.includes('invoice')) return '🧾';
  if (lowerName.includes('company') || lowerName.includes('organization')) return '🏢';
  if (lowerName.includes('property') || lowerName.includes('listing')) return '🏠';
  if (lowerName.includes('visit') || lowerName.includes('booking')) return '📅';
  return '📊';
};

export function EntityCard({
  entity,
  defaultExpanded = false,
}: EntityCardProps): React.ReactElement {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  return (
    <div className="border border-border-light rounded-lg overflow-hidden bg-white dark:bg-gray-900">
      {/* Entity header */}
      <div className="p-4 border-b border-border-light">
        <div className="flex items-start justify-between gap-3">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-2">
              <span className="text-2xl" role="img" aria-label={entity.name}>
                {getEntityIcon(entity.name)}
              </span>
              <h3 className="text-lg font-semibold text-text-primary">
                {entity.name}
              </h3>
            </div>
            {entity.description && (
              <p className="text-sm text-text-secondary italic mb-2">
                &ldquo;{entity.description}&rdquo;
              </p>
            )}
            <div className="flex items-center gap-2 text-sm text-text-secondary">
              <Database className="h-3 w-3" />
              <span className="font-medium">Primary:</span>
              <span className="font-mono text-text-primary">
                {entity.primary_table}.{entity.primary_column}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Occurrences section */}
      <div>
        <button
          type="button"
          onClick={() => setIsExpanded(!isExpanded)}
          className="flex items-center justify-between w-full px-4 py-3 text-left hover:bg-surface-secondary/30 transition-colors"
        >
          <div className="flex items-center gap-2">
            {isExpanded ? (
              <ChevronUp className="h-4 w-4 text-text-tertiary" />
            ) : (
              <ChevronDown className="h-4 w-4 text-text-tertiary" />
            )}
            <span className="text-sm font-medium text-text-primary">
              {entity.occurrences.length} occurrence{entity.occurrences.length !== 1 ? 's' : ''}
            </span>
          </div>
        </button>

        {isExpanded && (
          <div className="px-4 pb-4 space-y-1">
            {entity.occurrences.map((occurrence) => (
              <div
                key={occurrence.id}
                className="flex items-center gap-3 px-3 py-2 rounded bg-surface-secondary/20 hover:bg-surface-secondary/40 transition-colors"
              >
                <Table className="h-3 w-3 text-text-tertiary flex-shrink-0" />
                <span className="font-mono text-sm text-text-primary">
                  {occurrence.table_name}.{occurrence.column_name}
                </span>
                {occurrence.role && (
                  <span className="ml-auto px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                    {occurrence.role}
                  </span>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

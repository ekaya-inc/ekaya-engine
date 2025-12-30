/**
 * EntityList Component
 * Displays all discovered entities from the entity-based discovery workflow
 */

import { Loader2 } from 'lucide-react';

import type { EntityResponse } from '../../types';

import { EntityCard } from './EntityCard';

interface EntityListProps {
  entities: EntityResponse[];
  isLoading?: boolean;
}

export function EntityList({
  entities,
  isLoading = false,
}: EntityListProps): React.ReactElement {
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-center">
          <Loader2 className="h-8 w-8 animate-spin text-blue-500 mx-auto mb-3" />
          <p className="text-sm text-text-secondary">
            Discovering entities...
          </p>
        </div>
      </div>
    );
  }

  if (entities.length === 0) {
    return (
      <div className="text-center py-8 text-text-secondary">
        <p>No entities discovered yet.</p>
        <p className="text-sm mt-1">
          Entities will appear here as the discovery process completes.
        </p>
      </div>
    );
  }

  // Calculate total occurrences
  const totalOccurrences = entities.reduce(
    (sum, entity) => sum + entity.occurrences.length,
    0
  );

  return (
    <div className="space-y-4">
      {/* Summary */}
      <div className="text-sm text-text-secondary mb-4">
        <span className="font-medium text-text-primary">{entities.length}</span> entities discovered with{' '}
        <span className="font-medium text-text-primary">{totalOccurrences}</span> column mappings
      </div>

      {/* Entity cards */}
      {entities.map((entity) => (
        <EntityCard
          key={entity.id}
          entity={entity}
          defaultExpanded={false}
        />
      ))}
    </div>
  );
}

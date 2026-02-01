import { AlertTriangle, Loader2 } from 'lucide-react';
import { useState } from 'react';

import engineApi from '../services/engineApi';
import type { RelationshipDetail } from '../types';

import { Button } from './ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';

interface RemoveRelationshipDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  datasourceId: string;
  relationship: RelationshipDetail | null;
  onRelationshipRemoved: (relationshipId: string) => void;
}

/**
 * Confirmation dialog for removing a relationship.
 * Removing a relationship soft-deletes it, hiding it from the UI and excluding
 * it from ontology generation. The soft-deleted record prevents re-discovery
 * on subsequent extractions. To reset and allow rediscovery, remove the source
 * column from the schema, save, then re-add it (the new column gets a new ID).
 */
export const RemoveRelationshipDialog = ({
  open,
  onOpenChange,
  projectId,
  datasourceId,
  relationship,
  onRelationshipRemoved,
}: RemoveRelationshipDialogProps) => {
  const [isRemoving, setIsRemoving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleRemove = async (): Promise<void> => {
    if (!relationship) return;

    setIsRemoving(true);
    setError(null);

    try {
      const response = await engineApi.removeRelationship(projectId, datasourceId, relationship.id);
      if (response.success) {
        onRelationshipRemoved(relationship.id);
      } else {
        setError(response.error ?? 'Failed to remove relationship');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred');
    } finally {
      setIsRemoving(false);
    }
  };

  const handleOpenChange = (newOpen: boolean): void => {
    if (!isRemoving) {
      setError(null);
      onOpenChange(newOpen);
    }
  };

  if (!relationship) return null;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-amber-500" />
            Remove Relationship
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to remove this relationship?
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <div className="rounded-md bg-surface-secondary p-3 text-sm">
            <div className="font-medium text-text-primary">
              {relationship.source_table_name}.{relationship.source_column_name}
              {' → '}
              {relationship.target_table_name}.{relationship.target_column_name}
            </div>
            <div className="mt-1 text-text-secondary text-xs">
              Type: {relationship.relationship_type}
              {relationship.cardinality && ` • Cardinality: ${relationship.cardinality}`}
            </div>
          </div>

          <p className="mt-4 text-sm text-text-secondary">
            This relationship will be hidden from the UI and excluded from ontology generation.
            To restore it, remove the source column from the schema, save, then re-add it.
          </p>

          {error && (
            <div className="mt-4 flex items-center gap-2 text-sm text-red-600">
              <AlertTriangle className="h-4 w-4" />
              {error}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isRemoving}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleRemove}
            disabled={isRemoving}
          >
            {isRemoving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Removing...
              </>
            ) : (
              'Remove'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

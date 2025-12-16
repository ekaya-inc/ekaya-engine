/**
 * Delete Query Confirmation Dialog
 * Shows query preview and usage count warning before deletion
 */

import { AlertTriangle, Loader2 } from 'lucide-react';
import { useState } from 'react';

import engineApi from '../services/engineApi';
import type { Query } from '../types';

import { Button } from './ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';

interface DeleteQueryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  datasourceId: string;
  query: Query | null;
  onQueryDeleted: (queryId: string) => void;
}

/**
 * Confirmation dialog for deleting a query.
 * Shows the query prompt and warns about usage count.
 */
export const DeleteQueryDialog = ({
  open,
  onOpenChange,
  projectId,
  datasourceId,
  query,
  onQueryDeleted,
}: DeleteQueryDialogProps) => {
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleDelete = async (): Promise<void> => {
    if (!query) return;

    setIsDeleting(true);
    setError(null);

    try {
      const response = await engineApi.deleteQuery(
        projectId,
        datasourceId,
        query.query_id
      );
      if (response.success) {
        onQueryDeleted(query.query_id);
      } else {
        setError(response.error ?? 'Failed to delete query');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred');
    } finally {
      setIsDeleting(false);
    }
  };

  const handleOpenChange = (newOpen: boolean): void => {
    // Prevent closing while delete is in progress
    if (!isDeleting) {
      setError(null);
      onOpenChange(newOpen);
    }
  };

  if (!query) return null;

  const hasUsage = query.usage_count > 0;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-red-500" />
            Delete Query
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to delete this query? This action cannot be
            undone.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <div className="rounded-md bg-surface-secondary p-3 text-sm">
            <div className="font-medium text-text-primary line-clamp-3">
              {query.natural_language_prompt}
            </div>
            <div className="mt-2 text-text-secondary text-xs font-mono overflow-hidden">
              <div className="line-clamp-2">{query.sql_query}</div>
            </div>
          </div>

          {hasUsage && (
            <div className="mt-4 flex items-start gap-2 p-3 rounded-md bg-amber-50 dark:bg-amber-950 border border-amber-200 dark:border-amber-800">
              <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
              <div className="text-sm text-amber-700 dark:text-amber-300">
                <p className="font-medium">This query has been used</p>
                <p className="mt-1">
                  This query has been executed {query.usage_count} time
                  {query.usage_count !== 1 ? 's' : ''}. Deleting it may affect
                  existing integrations.
                </p>
              </div>
            </div>
          )}

          {error && (
            <div className="mt-4 flex items-center gap-2 text-sm text-red-600 dark:text-red-400">
              <AlertTriangle className="h-4 w-4" />
              {error}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isDeleting}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={isDeleting}
          >
            {isDeleting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Deleting...
              </>
            ) : (
              'Delete Query'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

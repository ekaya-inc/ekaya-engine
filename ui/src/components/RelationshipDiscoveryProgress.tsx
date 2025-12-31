/**
 * RelationshipDiscoveryProgress - Workflow-based relationship discovery dialog
 *
 * Shows a modal with:
 * - Task queue progress (left side)
 * - Candidate list for review (right side)
 * - Save/Cancel actions
 *
 * Uses polling to track workflow progress and fetch candidates.
 */

import {
  AlertCircle,
  AlertTriangle,
  Brain,
  Check,
  Circle,
  Loader2,
  X,
} from 'lucide-react';
import { useState, useEffect, useCallback, useRef } from 'react';

import relationshipWorkflowApi from '../services/relationshipWorkflowApi';
import type {
  EntitiesResponse,
  RelationshipWorkflowStatusResponse,
} from '../types';

import { EntityList } from './relationships/EntityList';
import { Button } from './ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/Card';

interface RelationshipDiscoveryProgressProps {
  projectId: string;
  datasourceId: string;
  isOpen: boolean;
  onClose: () => void;
  onComplete: () => void;
}

// Polling interval in milliseconds
const POLL_INTERVAL_MS = 2000;

export const RelationshipDiscoveryProgress = ({
  projectId,
  datasourceId,
  isOpen,
  onClose,
  onComplete,
}: RelationshipDiscoveryProgressProps): React.ReactElement | null => {
  // Workflow state
  const [workflowStarted, setWorkflowStarted] = useState(false);
  const [status, setStatus] = useState<RelationshipWorkflowStatusResponse | null>(null);
  const [entities, setEntities] = useState<EntitiesResponse>({
    entities: [],
    island_tables: [],
  });
  const [error, setError] = useState<string | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);
  const [saveResult, setSaveResult] = useState<number | null>(null);

  // Polling ref
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const isMountedRef = useRef(true);

  // Derived state
  const isWorkflowRunning = status?.state === 'running' || status?.state === 'pending';
  const isWorkflowComplete = status?.state === 'completed';
  const isWorkflowFailed = status?.state === 'failed';
  const canSave = status?.can_save === true && isWorkflowComplete;
  const hasEntities = entities.entities.length > 0;

  // Stop polling
  const stopPolling = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
      pollingRef.current = null;
    }
  }, []);

  // Fetch status and entities
  const fetchStatusAndEntities = useCallback(async () => {
    if (!projectId || !datasourceId) return;

    try {
      const [statusResponse, entitiesResponse] = await Promise.all([
        relationshipWorkflowApi.getStatus(projectId, datasourceId),
        relationshipWorkflowApi.getEntities(projectId, datasourceId),
      ]);

      if (!isMountedRef.current) return;

      setStatus(statusResponse);
      setEntities(entitiesResponse);

      // Stop polling when workflow completes or fails
      if (statusResponse.state === 'completed' || statusResponse.state === 'failed') {
        stopPolling();
      }
    } catch (err) {
      if (!isMountedRef.current) return;
      // 404 means no workflow exists (cancelled or not started)
      const errorWithStatus = err as Error & { status?: number };
      if (errorWithStatus.status === 404) {
        stopPolling();
        setStatus(null);
      } else {
        console.error('Failed to fetch workflow status:', err);
      }
    }
  }, [projectId, datasourceId, stopPolling]);

  // Start polling
  const startPolling = useCallback(() => {
    stopPolling();
    // Fetch immediately
    fetchStatusAndEntities();
    // Then poll
    pollingRef.current = setInterval(fetchStatusAndEntities, POLL_INTERVAL_MS);
  }, [fetchStatusAndEntities, stopPolling]);

  // Start workflow
  const startWorkflow = useCallback(async () => {
    try {
      setError(null);
      setSaveResult(null);
      await relationshipWorkflowApi.startDetection(projectId, datasourceId);
      setWorkflowStarted(true);
      startPolling();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to start workflow';
      console.error('Failed to start relationship detection:', err);
      setError(errorMessage);
    }
  }, [projectId, datasourceId, startPolling]);

  // Handle Cancel
  const handleCancel = useCallback(async () => {
    // If no workflow is running (e.g., start failed with error), just close the modal
    if (!status || error) {
      stopPolling();
      onClose();
      return;
    }

    setIsCancelling(true);
    try {
      await relationshipWorkflowApi.cancel(projectId, datasourceId);
      stopPolling();
      onClose();
    } catch (err) {
      console.error('Failed to cancel workflow:', err);
      // Still close on error - the workflow may not exist
      stopPolling();
      onClose();
    } finally {
      setIsCancelling(false);
    }
  }, [projectId, datasourceId, stopPolling, onClose, status, error]);

  // Handle Save
  const handleSave = useCallback(async () => {
    setIsSaving(true);
    try {
      const response = await relationshipWorkflowApi.save(projectId, datasourceId);
      setSaveResult(response.saved_count);
      onComplete();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to save relationships';
      console.error('Failed to save relationships:', err);
      setError(errorMessage);
    } finally {
      setIsSaving(false);
    }
  }, [projectId, datasourceId, onComplete]);

  // Start workflow when modal opens (only once per modal open)
  useEffect(() => {
    if (isOpen && !workflowStarted && !status && saveResult === null) {
      startWorkflow();
    }
  }, [isOpen, workflowStarted, status, startWorkflow, saveResult]);

  // Reset state when modal closes
  useEffect(() => {
    if (!isOpen) {
      stopPolling();
      setWorkflowStarted(false);
      setStatus(null);
      setEntities({ entities: [], island_tables: [] });
      setError(null);
      setSaveResult(null);
    }
  }, [isOpen, stopPolling]);

  // Refetch on tab visibility change (when user switches back to this tab)
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible' && isWorkflowRunning) {
        fetchStatusAndEntities();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, [isWorkflowRunning, fetchStatusAndEntities]);

  // Cleanup on unmount
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
      stopPolling();
    };
  }, [stopPolling]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <Card className="w-full max-w-4xl mx-4 max-h-[90vh] flex flex-col">
        {/* Header */}
        <CardHeader className="flex flex-row items-start justify-between space-y-0 border-b flex-shrink-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              {isWorkflowRunning && (
                <Loader2 className="h-5 w-5 animate-spin text-amber-500" />
              )}
              {isWorkflowComplete && saveResult === null && (
                <Check className="h-5 w-5 text-green-500" />
              )}
              {saveResult !== null && (
                <Check className="h-5 w-5 text-green-500" />
              )}
              {isWorkflowFailed && <AlertCircle className="h-5 w-5 text-red-500" />}
              {error && !isWorkflowFailed && (
                <AlertCircle className="h-5 w-5 text-red-500" />
              )}
              Finding Relationships
            </CardTitle>
            <CardDescription>
              {isWorkflowRunning && 'Analyzing schema and detecting relationships...'}
              {isWorkflowComplete && saveResult === null && 'Review and save detected relationships'}
              {saveResult !== null && `Saved ${saveResult} relationship${saveResult !== 1 ? 's' : ''}`}
              {isWorkflowFailed && 'Workflow failed'}
              {error && !isWorkflowFailed && 'An error occurred'}
            </CardDescription>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-8 w-8 p-0"
            onClick={onClose}
            disabled={isWorkflowRunning}
          >
            <X className="h-4 w-4" />
            <span className="sr-only">Close</span>
          </Button>
        </CardHeader>

        {/* Main content */}
        <CardContent className="flex-1 overflow-hidden p-0">
          {/* Error state */}
          {error && (
            <div className="m-4 rounded-lg bg-red-50 dark:bg-red-950/20 p-4">
              <div className="flex items-center gap-2 text-red-700 dark:text-red-300 mb-2">
                <AlertCircle className="h-4 w-4" />
                <span className="font-medium">Error</span>
              </div>
              <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
            </div>
          )}

          {/* Save success state */}
          {saveResult !== null && (
            <div className="m-4 rounded-lg bg-green-50 dark:bg-green-950/20 p-4">
              <div className="flex items-center gap-2 text-green-700 dark:text-green-300 mb-2">
                <Check className="h-4 w-4" />
                <span className="font-medium">Relationships saved successfully</span>
              </div>
              <p className="text-sm text-green-600 dark:text-green-400">
                {saveResult} relationship{saveResult !== 1 ? 's were' : ' was'} added to
                your schema.
              </p>
            </div>
          )}

          {/* Main workflow content */}
          {!error && saveResult === null && (
            <div className="grid grid-cols-1 lg:grid-cols-3 h-full">
              {/* Left: Discovery Steps */}
              <div className="border-r border-border-light p-4 overflow-y-auto">
                <div className="space-y-4">
                  {/* Header with progress */}
                  <div>
                    <h3 className="font-semibold text-text-primary">Discovery Steps</h3>
                  </div>

                  {/* Discovery phases */}
                  <div className="space-y-2">
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      <Check className="h-4 w-4 text-green-500" />
                      <span className="text-sm text-text-primary">Collecting column statistics</span>
                    </div>
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      <Check className="h-4 w-4 text-green-500" />
                      <span className="text-sm text-text-primary">Filtering entity candidates</span>
                    </div>
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      <Check className="h-4 w-4 text-green-500" />
                      <span className="text-sm text-text-primary">Analyzing graph connectivity</span>
                    </div>
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      {isWorkflowComplete ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : isWorkflowRunning ? (
                        <Brain className="h-4 w-4 text-purple-500 animate-pulse" />
                      ) : (
                        <Circle className="h-4 w-4 text-text-tertiary" />
                      )}
                      <span className="text-sm text-text-primary">Discovering entities (LLM)</span>
                    </div>
                  </div>

                  {/* Connectivity info */}
                  {entities.island_tables && entities.island_tables.length > 0 && (
                    <div className="mt-4 pt-4 border-t border-border-light">
                      <div className="text-xs font-medium text-text-secondary mb-2">Connectivity:</div>
                      <div className="flex items-center gap-2 text-sm">
                        <AlertTriangle className="h-3 w-3 text-amber-500" />
                        <span className="text-text-secondary">
                          {entities.island_tables.length} disconnected table{entities.island_tables.length !== 1 ? 's' : ''}
                        </span>
                      </div>
                    </div>
                  )}
                </div>
              </div>

              {/* Right: Discovered Entities */}
              <div className="lg:col-span-2 p-4 overflow-y-auto">
                <h3 className="font-semibold text-text-primary mb-4">
                  Discovered Entities
                </h3>
                <EntityList
                  entities={entities.entities}
                  isLoading={isWorkflowRunning && entities.entities.length === 0}
                />
              </div>
            </div>
          )}
        </CardContent>

        {/* Footer */}
        <div className="border-t border-border-light p-4 flex items-center justify-between flex-shrink-0">
          {/* Entity summary */}
          <div className="flex-1">
            {hasEntities && isWorkflowComplete && (
              <div className="text-sm text-text-secondary">
                {entities.entities.length} entities discovered with{' '}
                {entities.entities.reduce((sum, e) => sum + e.occurrences.length, 0)} column mappings
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex gap-2">
            {saveResult === null ? (
              <>
                <Button
                  variant="outline"
                  onClick={handleCancel}
                  disabled={isCancelling || isSaving}
                >
                  {isCancelling ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Cancelling...
                    </>
                  ) : (
                    'Cancel'
                  )}
                </Button>
                <Button
                  variant="default"
                  onClick={handleSave}
                  disabled={!canSave || isSaving || isCancelling}
                >
                  {isSaving ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      Saving...
                    </>
                  ) : !hasEntities ? (
                    'No Entities to Save'
                  ) : (
                    `Save ${entities.entities.length} Entity${entities.entities.length !== 1 ? ' Relationships' : ' Relationship'}`
                  )}
                </Button>
              </>
            ) : (
              <Button variant="default" onClick={onClose}>
                Close
              </Button>
            )}
          </div>
        </div>
      </Card>
    </div>
  );
};

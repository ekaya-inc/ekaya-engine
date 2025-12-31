/**
 * EntityDiscoveryProgress - Standalone entity discovery workflow dialog
 *
 * Shows a modal with:
 * - Discovery steps progress (left side)
 * - Discovered entities list (right side)
 * - Close/Cancel actions
 *
 * Uses polling to track workflow progress.
 */

import {
  AlertCircle,
  Brain,
  Check,
  Circle,
  Loader2,
  X,
  Boxes,
} from 'lucide-react';
import { useState, useEffect, useCallback, useRef } from 'react';

import engineApi from '../services/engineApi';
import type { EntityDiscoveryStatus, EntityDetail } from '../types';

import { Button } from './ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/Card';

interface EntityDiscoveryProgressProps {
  projectId: string;
  datasourceId: string;
  isOpen: boolean;
  onClose: () => void;
  onComplete: () => void;
}

// Polling interval in milliseconds
const POLL_INTERVAL_MS = 2000;

export const EntityDiscoveryProgress = ({
  projectId,
  datasourceId,
  isOpen,
  onClose,
  onComplete,
}: EntityDiscoveryProgressProps): React.ReactElement | null => {
  // Workflow state
  const [workflowStarted, setWorkflowStarted] = useState(false);
  const [status, setStatus] = useState<EntityDiscoveryStatus | null>(null);
  const [entities, setEntities] = useState<EntityDetail[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isCancelling, setIsCancelling] = useState(false);

  // Polling ref
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const isMountedRef = useRef(true);

  // Derived state
  const isWorkflowRunning = status?.state === 'running' || status?.state === 'pending';
  const isWorkflowComplete = status?.state === 'completed';
  const isWorkflowFailed = status?.state === 'failed';
  const hasEntities = entities.length > 0;

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
      const statusResponse = await engineApi.getEntityDiscoveryStatus(projectId, datasourceId);

      if (!isMountedRef.current) return;

      if (statusResponse.data) {
        setStatus(statusResponse.data);

        // Stop polling when workflow completes or fails
        if (statusResponse.data.state === 'completed' || statusResponse.data.state === 'failed') {
          stopPolling();

          // Fetch entities when complete
          if (statusResponse.data.state === 'completed') {
            const entitiesResponse = await engineApi.listEntities(projectId);
            if (entitiesResponse.data) {
              setEntities(entitiesResponse.data.entities);
            }
          }
        }
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
      await engineApi.startEntityDiscovery(projectId, datasourceId);
      setWorkflowStarted(true);
      startPolling();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to start workflow';
      console.error('Failed to start entity discovery:', err);
      setError(errorMessage);
    }
  }, [projectId, datasourceId, startPolling]);

  // Handle Cancel
  const handleCancel = useCallback(async () => {
    setIsCancelling(true);
    try {
      await engineApi.cancelEntityDiscovery(projectId, datasourceId);
      stopPolling();
      onClose();
    } catch (err) {
      console.error('Failed to cancel workflow:', err);
    } finally {
      setIsCancelling(false);
    }
  }, [projectId, datasourceId, stopPolling, onClose]);

  // Handle Close (when complete)
  const handleClose = useCallback(() => {
    onComplete();
    onClose();
  }, [onComplete, onClose]);

  // Start workflow when modal opens
  useEffect(() => {
    if (isOpen && !workflowStarted && !status) {
      startWorkflow();
    }
  }, [isOpen, workflowStarted, status, startWorkflow]);

  // Reset state when modal closes
  useEffect(() => {
    if (!isOpen) {
      stopPolling();
      setWorkflowStarted(false);
      setStatus(null);
      setEntities([]);
      setError(null);
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

  // Calculate progress percentage
  const progressPercent = status?.progress?.current ?? 0;

  // Get phase-specific step states
  const isPhaseComplete = (threshold: number) => progressPercent >= threshold;
  const isPhaseActive = (min: number, max: number) =>
    progressPercent >= min && progressPercent < max && isWorkflowRunning;

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
              {isWorkflowComplete && (
                <Check className="h-5 w-5 text-green-500" />
              )}
              {isWorkflowFailed && <AlertCircle className="h-5 w-5 text-red-500" />}
              {error && !isWorkflowFailed && (
                <AlertCircle className="h-5 w-5 text-red-500" />
              )}
              Discovering Entities
            </CardTitle>
            <CardDescription>
              {isWorkflowRunning && (status?.progress?.message ?? 'Analyzing schema and detecting entities...')}
              {isWorkflowComplete && `Discovered ${entities.length} entities`}
              {isWorkflowFailed && 'Workflow failed'}
              {error && !isWorkflowFailed && 'An error occurred'}
            </CardDescription>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-8 w-8 p-0"
            onClick={isWorkflowComplete ? handleClose : onClose}
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

          {/* Main workflow content */}
          {!error && (
            <div className="grid grid-cols-1 lg:grid-cols-3 h-full">
              {/* Left: Discovery Steps */}
              <div className="border-r border-border-light p-4 overflow-y-auto">
                <div className="space-y-4">
                  {/* Header with progress */}
                  <div>
                    <h3 className="font-semibold text-text-primary">Discovery Steps</h3>
                    {isWorkflowRunning && (
                      <div className="mt-2 h-2 w-full bg-surface-secondary rounded-full overflow-hidden">
                        <div
                          className="h-full bg-amber-500 transition-all duration-500"
                          style={{ width: `${progressPercent}%` }}
                        />
                      </div>
                    )}
                  </div>

                  {/* Discovery phases */}
                  <div className="space-y-2">
                    {/* Phase 0: Collecting statistics */}
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      {isPhaseComplete(20) ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : isPhaseActive(0, 20) ? (
                        <Loader2 className="h-4 w-4 text-amber-500 animate-spin" />
                      ) : (
                        <Circle className="h-4 w-4 text-text-tertiary" />
                      )}
                      <span className="text-sm text-text-primary">Collecting column statistics</span>
                    </div>

                    {/* Phase 0.5: Filtering candidates */}
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      {isPhaseComplete(35) ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : isPhaseActive(20, 35) ? (
                        <Loader2 className="h-4 w-4 text-amber-500 animate-spin" />
                      ) : (
                        <Circle className="h-4 w-4 text-text-tertiary" />
                      )}
                      <span className="text-sm text-text-primary">Filtering entity candidates</span>
                    </div>

                    {/* Phase 0.75: Graph connectivity */}
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      {isPhaseComplete(50) ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : isPhaseActive(35, 50) ? (
                        <Loader2 className="h-4 w-4 text-amber-500 animate-spin" />
                      ) : (
                        <Circle className="h-4 w-4 text-text-tertiary" />
                      )}
                      <span className="text-sm text-text-primary">Analyzing graph connectivity</span>
                    </div>

                    {/* Phase 1: LLM entity discovery */}
                    <div className="flex items-center gap-3 px-3 py-2 rounded">
                      {isPhaseComplete(100) ? (
                        <Check className="h-4 w-4 text-green-500" />
                      ) : isPhaseActive(50, 100) ? (
                        <Brain className="h-4 w-4 text-purple-500 animate-pulse" />
                      ) : (
                        <Circle className="h-4 w-4 text-text-tertiary" />
                      )}
                      <span className="text-sm text-text-primary">Discovering entities (LLM)</span>
                    </div>
                  </div>

                  {/* Entity count summary */}
                  {(status?.entity_count ?? 0) > 0 && (
                    <div className="mt-4 pt-4 border-t border-border-light">
                      <div className="text-xs font-medium text-text-secondary mb-2">Summary:</div>
                      <div className="flex items-center gap-2 text-sm">
                        <Boxes className="h-3 w-3 text-green-500" />
                        <span className="text-text-secondary">
                          {status?.entity_count ?? 0} entities, {status?.occurrence_count ?? 0} occurrences
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

                {/* Loading state */}
                {isWorkflowRunning && entities.length === 0 && (
                  <div className="flex flex-col items-center justify-center py-12 text-text-secondary">
                    <Loader2 className="h-8 w-8 animate-spin mb-4" />
                    <p className="text-sm">Discovering entities...</p>
                  </div>
                )}

                {/* Empty state (workflow complete but no entities) */}
                {isWorkflowComplete && entities.length === 0 && (
                  <div className="flex flex-col items-center justify-center py-12 text-text-secondary">
                    <Boxes className="h-12 w-12 mb-4" />
                    <p className="text-sm">No entities discovered</p>
                  </div>
                )}

                {/* Entity list */}
                {entities.length > 0 && (
                  <div className="space-y-3">
                    {entities.map((entity) => (
                      <div key={entity.id} className="border border-border-light rounded-lg p-3">
                        <div className="flex items-start justify-between">
                          <div>
                            <h4 className="font-medium text-text-primary">{entity.name}</h4>
                            {entity.description && (
                              <p className="text-sm text-text-secondary mt-1">{entity.description}</p>
                            )}
                            <p className="text-xs text-text-tertiary mt-1 font-mono">
                              {entity.primary_schema}.{entity.primary_table}.{entity.primary_column}
                            </p>
                          </div>
                          <span className="text-xs text-text-tertiary">
                            {entity.occurrence_count} occurrence{entity.occurrence_count !== 1 ? 's' : ''}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
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
                {entities.length} entities discovered with{' '}
                {entities.reduce((sum, e) => sum + e.occurrence_count, 0)} total occurrences
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex gap-2">
            {isWorkflowRunning ? (
              <Button
                variant="outline"
                onClick={handleCancel}
                disabled={isCancelling}
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
            ) : (
              <Button variant="default" onClick={handleClose}>
                {isWorkflowComplete ? 'Done' : 'Close'}
              </Button>
            )}
          </div>
        </div>
      </Card>
    </div>
  );
};

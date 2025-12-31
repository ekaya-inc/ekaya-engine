/**
 * EntityDiscoveryProgress - Standalone entity discovery workflow dialog
 *
 * Shows a modal with discovery steps progress.
 * When complete, user clicks "Done" to see entities on the main page.
 *
 * Uses polling to track workflow progress.
 */

import {
  AlertCircle,
  Check,
  Circle,
  Loader2,
  X,
  Boxes,
} from 'lucide-react';
import { useState, useEffect, useCallback, useRef } from 'react';

import engineApi from '../services/engineApi';
import type { EntityDiscoveryStatus } from '../types';

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
  const [error, setError] = useState<string | null>(null);
  const [isCancelling, setIsCancelling] = useState(false);

  // Polling ref
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const isMountedRef = useRef(true);

  // Derived state
  const isWorkflowRunning = status?.state === 'running' || status?.state === 'pending';
  const isWorkflowComplete = status?.state === 'completed';
  const isWorkflowFailed = status?.state === 'failed';

  // Stop polling
  const stopPolling = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
      pollingRef.current = null;
    }
  }, []);

  // Fetch status
  const fetchStatus = useCallback(async () => {
    if (!projectId || !datasourceId) return;

    try {
      const statusResponse = await engineApi.getEntityDiscoveryStatus(projectId, datasourceId);

      if (!isMountedRef.current) return;

      if (statusResponse.data) {
        setStatus(statusResponse.data);

        // Stop polling when workflow completes or fails
        if (statusResponse.data.state === 'completed' || statusResponse.data.state === 'failed') {
          stopPolling();
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
    fetchStatus();
    // Then poll
    pollingRef.current = setInterval(fetchStatus, POLL_INTERVAL_MS);
  }, [fetchStatus, stopPolling]);

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
      setError(null);
    }
  }, [isOpen, stopPolling]);

  // Refetch on tab visibility change (when user switches back to this tab)
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible' && isWorkflowRunning) {
        fetchStatus();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, [isWorkflowRunning, fetchStatus]);

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

  // Entity counts from status
  const entityCount = status?.entity_count ?? 0;
  const occurrenceCount = status?.occurrence_count ?? 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <Card className="w-full max-w-md mx-4 flex flex-col">
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
              {isWorkflowComplete && `Discovered ${entityCount} entities`}
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
        <CardContent className="p-4">
          {/* Error state */}
          {error && (
            <div className="rounded-lg bg-red-50 dark:bg-red-950/20 p-4">
              <div className="flex items-center gap-2 text-red-700 dark:text-red-300 mb-2">
                <AlertCircle className="h-4 w-4" />
                <span className="font-medium">Error</span>
              </div>
              <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
            </div>
          )}

          {/* Main workflow content */}
          {!error && (
            <div className="space-y-4">
              {/* Progress bar */}
              {isWorkflowRunning && (
                <div className="h-2 w-full bg-surface-secondary rounded-full overflow-hidden">
                  <div
                    className="h-full bg-amber-500 transition-all duration-500"
                    style={{ width: `${progressPercent}%` }}
                  />
                </div>
              )}

              {/* Discovery phases */}
              <div className="space-y-2">
                {/* Phase 1: Analyzing schema constraints (0-50%) */}
                <div className="flex items-center gap-3 px-3 py-2 rounded">
                  {isPhaseComplete(50) ? (
                    <Check className="h-4 w-4 text-green-500" />
                  ) : isWorkflowRunning && progressPercent < 50 ? (
                    <Loader2 className="h-4 w-4 text-amber-500 animate-spin" />
                  ) : (
                    <Circle className="h-4 w-4 text-text-tertiary" />
                  )}
                  <span className="text-sm text-text-primary">Analyzing schema constraints</span>
                </div>
                {/* Phase 2: Generating entity names (50-100%) */}
                <div className="flex items-center gap-3 px-3 py-2 rounded">
                  {isPhaseComplete(100) ? (
                    <Check className="h-4 w-4 text-green-500" />
                  ) : isWorkflowRunning && progressPercent >= 50 ? (
                    <Loader2 className="h-4 w-4 text-amber-500 animate-spin" />
                  ) : (
                    <Circle className="h-4 w-4 text-text-tertiary" />
                  )}
                  <span className="text-sm text-text-primary">Generating entity names and descriptions</span>
                </div>
              </div>

              {/* Entity count summary */}
              {entityCount > 0 && (
                <div className="pt-4 border-t border-border-light">
                  <div className="flex items-center gap-2 text-sm">
                    <Boxes className="h-4 w-4 text-green-500" />
                    <span className="text-text-secondary">
                      {entityCount} entities, {occurrenceCount} occurrences
                    </span>
                  </div>
                </div>
              )}
            </div>
          )}
        </CardContent>

        {/* Footer */}
        <div className="border-t border-border-light p-4 flex justify-end">
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
      </Card>
    </div>
  );
};

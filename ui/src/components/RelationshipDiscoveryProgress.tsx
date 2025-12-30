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
  Pause,
  X,
  XCircle,
} from 'lucide-react';
import { useState, useEffect, useCallback, useRef } from 'react';

import relationshipWorkflowApi from '../services/relationshipWorkflowApi';
import type {
  CandidatesResponse,
  RelationshipWorkflowStatusResponse,
} from '../types';

import { CandidateList } from './relationships/CandidateList';
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

// Task status helpers
type TaskStatus = 'queued' | 'processing' | 'complete' | 'failed' | 'paused';

const getTaskStatusIcon = (status: TaskStatus, requiresLlm: boolean) => {
  switch (status) {
    case 'complete':
      return <Check className="h-4 w-4 text-green-500" />;
    case 'processing':
      return requiresLlm ? (
        <Brain className="h-4 w-4 text-purple-500 animate-pulse" />
      ) : (
        <Loader2 className="h-4 w-4 text-blue-500 animate-spin" />
      );
    case 'queued':
      return <Circle className="h-4 w-4 text-text-tertiary" />;
    case 'paused':
      return <Pause className="h-4 w-4 text-amber-500" />;
    case 'failed':
      return <XCircle className="h-4 w-4 text-red-500" />;
    default:
      return <Circle className="h-4 w-4 text-text-tertiary" />;
  }
};

const getTaskRowBackground = (status: TaskStatus): string => {
  switch (status) {
    case 'processing':
      return 'bg-blue-500/5 border-l-2 border-l-blue-500';
    case 'paused':
      return 'bg-amber-500/5 border-l-2 border-l-amber-500';
    case 'failed':
      return 'bg-red-500/5 border-l-2 border-l-red-500';
    default:
      return 'border-l-2 border-l-transparent';
  }
};

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
  const [candidates, setCandidates] = useState<CandidatesResponse>({
    confirmed: [],
    needs_review: [],
    rejected: [],
  });
  const [error, setError] = useState<string | null>(null);
  const [candidateError, setCandidateError] = useState<string | null>(null);
  const [loadingCandidateId, setLoadingCandidateId] = useState<string | null>(null);
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
  const hasNeedsReview = candidates.needs_review.length > 0;

  // Stop polling
  const stopPolling = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
      pollingRef.current = null;
    }
  }, []);

  // Fetch status and candidates
  const fetchStatusAndCandidates = useCallback(async () => {
    if (!projectId || !datasourceId) return;

    try {
      const [statusResponse, candidatesResponse] = await Promise.all([
        relationshipWorkflowApi.getStatus(projectId, datasourceId),
        relationshipWorkflowApi.getCandidates(projectId, datasourceId),
      ]);

      if (!isMountedRef.current) return;

      setStatus(statusResponse);
      setCandidates(candidatesResponse);

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
    fetchStatusAndCandidates();
    // Then poll
    pollingRef.current = setInterval(fetchStatusAndCandidates, POLL_INTERVAL_MS);
  }, [fetchStatusAndCandidates, stopPolling]);

  // Start workflow
  const startWorkflow = useCallback(async () => {
    try {
      setError(null);
      setCandidateError(null);
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

  // Handle Accept candidate
  const handleAccept = useCallback(
    async (candidateId: string) => {
      setLoadingCandidateId(candidateId);
      setCandidateError(null);
      try {
        await relationshipWorkflowApi.updateCandidate(
          projectId,
          datasourceId,
          candidateId,
          'accepted'
        );
        // Refresh candidates and status
        const [candidatesResponse, statusResponse] = await Promise.all([
          relationshipWorkflowApi.getCandidates(projectId, datasourceId),
          relationshipWorkflowApi.getStatus(projectId, datasourceId),
        ]);
        setCandidates(candidatesResponse);
        setStatus(statusResponse);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to accept relationship';
        console.error('Failed to accept candidate:', err);
        setCandidateError(errorMessage);
      } finally {
        setLoadingCandidateId(null);
      }
    },
    [projectId, datasourceId]
  );

  // Handle Reject candidate
  const handleReject = useCallback(
    async (candidateId: string) => {
      setLoadingCandidateId(candidateId);
      setCandidateError(null);
      try {
        await relationshipWorkflowApi.updateCandidate(
          projectId,
          datasourceId,
          candidateId,
          'rejected'
        );
        // Refresh candidates and status
        const [candidatesResponse, statusResponse] = await Promise.all([
          relationshipWorkflowApi.getCandidates(projectId, datasourceId),
          relationshipWorkflowApi.getStatus(projectId, datasourceId),
        ]);
        setCandidates(candidatesResponse);
        setStatus(statusResponse);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to reject relationship';
        console.error('Failed to reject candidate:', err);
        setCandidateError(errorMessage);
      } finally {
        setLoadingCandidateId(null);
      }
    },
    [projectId, datasourceId]
  );

  // Handle Cancel
  const handleCancel = useCallback(async () => {
    setIsCancelling(true);
    try {
      await relationshipWorkflowApi.cancel(projectId, datasourceId);
      stopPolling();
      onClose();
    } catch (err) {
      console.error('Failed to cancel workflow:', err);
    } finally {
      setIsCancelling(false);
    }
  }, [projectId, datasourceId, stopPolling, onClose]);

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
      setCandidates({ confirmed: [], needs_review: [], rejected: [] });
      setError(null);
      setCandidateError(null);
      setLoadingCandidateId(null);
      setSaveResult(null);
    }
  }, [isOpen, stopPolling]);

  // Refetch on tab visibility change (when user switches back to this tab)
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible' && isWorkflowRunning) {
        fetchStatusAndCandidates();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, [isWorkflowRunning, fetchStatusAndCandidates]);

  // Cleanup on unmount
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
      stopPolling();
    };
  }, [stopPolling]);

  if (!isOpen) return null;

  // Calculate task counts
  const taskQueue = status?.task_queue ?? [];
  const taskCounts = taskQueue.reduce(
    (acc, task) => {
      acc[task.status] = (acc[task.status] ?? 0) + 1;
      return acc;
    },
    {} as Record<string, number>
  );

  // Calculate progress percentage
  const completedTasks = taskCounts['complete'] ?? 0;
  const totalTasks = taskQueue.length;
  const progressPercent = totalTasks > 0 ? Math.round((completedTasks / totalTasks) * 100) : 0;

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
              {/* Left: Work Queue */}
              <div className="border-r border-border-light p-4 overflow-y-auto">
                <div className="space-y-4">
                  {/* Header with progress */}
                  <div>
                    <h3 className="font-semibold text-text-primary">Work Queue</h3>
                    <div className="mt-2 flex flex-wrap gap-3 text-sm">
                      {(taskCounts['complete'] ?? 0) > 0 && (
                        <span className="flex items-center gap-1 text-green-600">
                          <Check className="h-3 w-3" />
                          {taskCounts['complete']} complete
                        </span>
                      )}
                      {(taskCounts['processing'] ?? 0) > 0 && (
                        <span className="flex items-center gap-1 text-blue-600">
                          <Loader2 className="h-3 w-3 animate-spin" />
                          {taskCounts['processing']} processing
                        </span>
                      )}
                      {(taskCounts['queued'] ?? 0) > 0 && (
                        <span className="flex items-center gap-1 text-text-secondary">
                          <Circle className="h-3 w-3" />
                          {taskCounts['queued']} queued
                        </span>
                      )}
                      {(taskCounts['failed'] ?? 0) > 0 && (
                        <span className="flex items-center gap-1 text-red-600">
                          <XCircle className="h-3 w-3" />
                          {taskCounts['failed']} failed
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Progress bar */}
                  <div>
                    <div className="flex justify-between text-xs text-text-secondary mb-1">
                      <span>Progress</span>
                      <span>{progressPercent}%</span>
                    </div>
                    <div className="h-2 bg-surface-secondary rounded-full overflow-hidden">
                      <div
                        className="h-full bg-blue-500 transition-all duration-300"
                        style={{ width: `${progressPercent}%` }}
                      />
                    </div>
                  </div>

                  {/* Task list */}
                  <div className="space-y-1">
                    {taskQueue
                      .filter((task) => task.status !== 'complete')
                      .map((task) => (
                        <div
                          key={task.id}
                          className={`flex items-center gap-3 px-3 py-2 rounded ${getTaskRowBackground(task.status as TaskStatus)}`}
                        >
                          {getTaskStatusIcon(task.status as TaskStatus, task.requires_llm)}
                          <span className="text-sm text-text-primary flex-1 truncate">
                            {task.name}
                          </span>
                        </div>
                      ))}
                    {taskQueue.length === 0 && isWorkflowRunning && (
                      <div className="text-sm text-text-secondary text-center py-4">
                        Initializing workflow...
                      </div>
                    )}
                    {taskQueue.filter((t) => t.status !== 'complete').length === 0 &&
                      taskQueue.length > 0 && (
                        <div className="text-sm text-green-600 text-center py-4">
                          All tasks complete
                        </div>
                      )}
                  </div>
                </div>
              </div>

              {/* Right: Candidates */}
              <div className="lg:col-span-2 p-4 overflow-y-auto">
                <h3 className="font-semibold text-text-primary mb-4">
                  Relationship Candidates
                </h3>
                {/* Candidate action error */}
                {candidateError && (
                  <div className="mb-4 rounded-lg bg-red-50 dark:bg-red-950/20 p-3">
                    <div className="flex items-center gap-2 text-red-700 dark:text-red-300 text-sm">
                      <AlertCircle className="h-4 w-4 flex-shrink-0" />
                      <span>{candidateError}</span>
                      <button
                        type="button"
                        onClick={() => setCandidateError(null)}
                        className="ml-auto text-red-500 hover:text-red-700"
                      >
                        <X className="h-4 w-4" />
                      </button>
                    </div>
                  </div>
                )}
                <CandidateList
                  candidates={candidates}
                  onAccept={handleAccept}
                  onReject={handleReject}
                  loadingCandidateId={loadingCandidateId}
                />
              </div>
            </div>
          )}
        </CardContent>

        {/* Footer */}
        <div className="border-t border-border-light p-4 flex items-center justify-between flex-shrink-0">
          {/* Warning message */}
          <div className="flex-1">
            {hasNeedsReview && isWorkflowComplete && (
              <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400 text-sm">
                <AlertTriangle className="h-4 w-4" />
                <span>
                  {candidates.needs_review.length} relationship
                  {candidates.needs_review.length !== 1 ? 's need' : ' needs'} your review
                  before saving
                </span>
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
                  ) : candidates.confirmed.length === 0 ? (
                    'No Relationships to Save'
                  ) : (
                    `Save ${candidates.confirmed.length} Relationship${candidates.confirmed.length !== 1 ? 's' : ''}`
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

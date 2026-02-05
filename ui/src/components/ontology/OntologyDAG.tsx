/**
 * OntologyDAG Component
 * Displays the DAG-based ontology extraction workflow with real-time status updates.
 * Shows a visual representation of the extraction pipeline with progress indicators.
 */

import {
  AlertCircle,
  ArrowDown,
  Brain,
  Check,
  Circle,
  Loader2,
  Network,
  RefreshCw,
  Trash2,
  X,
} from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';

import engineApi from '../../services/engineApi';
import type { DAGNodeName, DAGNodeStatus, DAGStatusResponse, DAGStatus } from '../../types';
import { DAGNodeDescriptions } from '../../types';
import { Button } from '../ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/Dialog';
import { Input } from '../ui/Input';

import { ExtractionProgress } from './ExtractionProgress';

interface OntologyDAGProps {
  projectId: string;
  datasourceId: string;
  onComplete?: () => void;
  onError?: (error: string) => void;
  onStatusChange?: (hasOntology: boolean) => void;
}

// Polling interval in milliseconds (2 seconds as per plan)
const POLL_INTERVAL_MS = 2000;

// Helper to get status icon for a node
const getNodeStatusIcon = (status: DAGNodeStatus, isCurrentNode: boolean) => {
  switch (status) {
    case 'completed':
      return <Check className="h-5 w-5 text-green-500" />;
    case 'running':
      return isCurrentNode ? (
        <Brain className="h-5 w-5 text-purple-500 animate-pulse" />
      ) : (
        <Loader2 className="h-5 w-5 text-blue-500 animate-spin" />
      );
    case 'pending':
      return <Circle className="h-5 w-5 text-gray-300" />;
    case 'failed':
      return <AlertCircle className="h-5 w-5 text-red-500" />;
    case 'skipped':
      return <Circle className="h-5 w-5 text-gray-400" />;
    default:
      return <Circle className="h-5 w-5 text-gray-300" />;
  }
};

// Helper to get connector line color
const getConnectorColor = (prevStatus: DAGNodeStatus, currentStatus: DAGNodeStatus) => {
  if (prevStatus === 'completed') {
    return 'bg-green-500';
  }
  if (prevStatus === 'running' || currentStatus === 'running') {
    return 'bg-blue-500';
  }
  return 'bg-gray-200';
};

// Helper to get node background color
const getNodeBackground = (status: DAGNodeStatus, isCurrentNode: boolean) => {
  if (status === 'running') {
    return isCurrentNode
      ? 'bg-purple-50 border-purple-300 dark:bg-purple-900/20 dark:border-purple-700'
      : 'bg-blue-50 border-blue-300 dark:bg-blue-900/20 dark:border-blue-700';
  }
  if (status === 'completed') {
    return 'bg-green-50 border-green-300 dark:bg-green-900/20 dark:border-green-700';
  }
  if (status === 'failed') {
    return 'bg-red-50 border-red-300 dark:bg-red-900/20 dark:border-red-700';
  }
  return 'bg-surface-secondary border-border-light';
};

// Helper to determine if DAG is terminal (not running)
const isTerminalStatus = (status: DAGStatus): boolean => {
  return status === 'completed' || status === 'failed' || status === 'cancelled';
};

export const OntologyDAG = ({
  projectId,
  datasourceId,
  onComplete,
  onError,
  onStatusChange,
}: OntologyDAGProps) => {
  const [dagStatus, setDagStatus] = useState<DAGStatusResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isStarting, setIsStarting] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showReextractDialog, setShowReextractDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [projectOverview, setProjectOverview] = useState('');
  const [isLoadingOverview, setIsLoadingOverview] = useState(true);

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const isMountedRef = useRef(true);

  // Derived state
  const isRunning = dagStatus?.status === 'running' || dagStatus?.status === 'pending';
  const isComplete = dagStatus?.status === 'completed';
  const isFailed = dagStatus?.status === 'failed';
  const isCancelled = dagStatus?.status === 'cancelled';

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
      const response = await engineApi.getOntologyDAGStatus(projectId, datasourceId);

      if (!isMountedRef.current) return;

      if (response.data) {
        setDagStatus(response.data);

        // Stop polling when DAG reaches terminal state
        if (isTerminalStatus(response.data.status)) {
          stopPolling();

          if (response.data.status === 'completed') {
            onComplete?.();
          }
        }
      } else {
        // No DAG exists
        setDagStatus(null);
        stopPolling();
      }

      setError(null);
    } catch (err) {
      if (!isMountedRef.current) return;
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch status';
      console.error('Failed to fetch DAG status:', err);
      setError(errorMessage);
      onError?.(errorMessage);
    }
  }, [projectId, datasourceId, stopPolling, onComplete, onError]);

  // Start polling
  const startPolling = useCallback(() => {
    stopPolling();
    // Fetch immediately
    void fetchStatus();
    // Then poll every 2 seconds
    pollingRef.current = setInterval(() => void fetchStatus(), POLL_INTERVAL_MS);
  }, [fetchStatus, stopPolling]);

  // Start extraction
  const handleStart = useCallback(async () => {
    try {
      setIsStarting(true);
      setError(null);

      const response = await engineApi.startOntologyExtraction(
        projectId,
        datasourceId,
        projectOverview || undefined
      );

      if (!isMountedRef.current) return;

      if (response.data) {
        setDagStatus(response.data);
        // Start polling if DAG is running
        if (!isTerminalStatus(response.data.status)) {
          startPolling();
        }
      }
    } catch (err) {
      if (!isMountedRef.current) return;
      const errorMessage = err instanceof Error ? err.message : 'Failed to start extraction';
      console.error('Failed to start extraction:', err);
      setError(errorMessage);
      onError?.(errorMessage);
    } finally {
      if (isMountedRef.current) {
        setIsStarting(false);
      }
    }
  }, [projectId, datasourceId, projectOverview, startPolling, onError]);

  // Cancel extraction
  const handleCancel = useCallback(async () => {
    try {
      setIsCancelling(true);

      await engineApi.cancelOntologyDAG(projectId, datasourceId);

      if (!isMountedRef.current) return;

      stopPolling();
      // Refresh status to show cancelled state
      await fetchStatus();
    } catch (err) {
      if (!isMountedRef.current) return;
      const errorMessage = err instanceof Error ? err.message : 'Failed to cancel';
      console.error('Failed to cancel DAG:', err);
      setError(errorMessage);
    } finally {
      if (isMountedRef.current) {
        setIsCancelling(false);
      }
    }
  }, [projectId, datasourceId, stopPolling, fetchStatus]);

  // Delete ontology
  const handleDelete = useCallback(async () => {
    try {
      setIsDeleting(true);

      await engineApi.deleteOntology(projectId, datasourceId);

      if (!isMountedRef.current) return;

      // Reset state
      setDagStatus(null);
      setShowDeleteDialog(false);
      setDeleteConfirmText('');
      stopPolling();
    } catch (err) {
      if (!isMountedRef.current) return;
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete ontology';
      console.error('Failed to delete ontology:', err);
      setError(errorMessage);
      onError?.(errorMessage);
    } finally {
      if (isMountedRef.current) {
        setIsDeleting(false);
      }
    }
  }, [projectId, datasourceId, stopPolling, onError]);

  // Initial load
  useEffect(() => {
    const init = async () => {
      if (!projectId || !datasourceId) return;

      setIsLoading(true);
      setIsLoadingOverview(true);

      try {
        // Fetch DAG status and project overview in parallel
        const [dagResponse, overviewResponse] = await Promise.all([
          engineApi.getOntologyDAGStatus(projectId, datasourceId),
          engineApi.getProjectOverview(projectId),
        ]);

        if (!isMountedRef.current) return;

        // Handle project overview
        if (overviewResponse.data?.overview) {
          setProjectOverview(overviewResponse.data.overview);
        }

        // Handle DAG status
        if (dagResponse.data) {
          setDagStatus(dagResponse.data);

          // Use response data directly instead of state to avoid stale closure
          if (!isTerminalStatus(dagResponse.data.status)) {
            startPolling();
          }

          if (dagResponse.data.status === 'completed') {
            onComplete?.();
          }
        } else {
          // No DAG exists
          setDagStatus(null);
        }

        setError(null);
      } catch (err) {
        if (!isMountedRef.current) return;
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch status';
        console.error('Failed to fetch DAG status:', err);
        setError(errorMessage);
        onError?.(errorMessage);
      } finally {
        if (isMountedRef.current) {
          setIsLoading(false);
          setIsLoadingOverview(false);
        }
      }
    };

    void init();

    return () => {
      isMountedRef.current = false;
      stopPolling();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, datasourceId]);

  // Start polling when DAG starts running
  const dagStatusValue = dagStatus?.status;
  useEffect(() => {
    if (dagStatusValue && !isTerminalStatus(dagStatusValue)) {
      startPolling();
    }
  }, [dagStatusValue, startPolling]);

  // Refetch on tab visibility change
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible' && isRunning) {
        void fetchStatus();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, [isRunning, fetchStatus]);

  // Notify parent of ontology existence status
  useEffect(() => {
    onStatusChange?.(dagStatus !== null);
  }, [dagStatus, onStatusChange]);

  // Count completed nodes
  const completedNodes = dagStatus?.nodes.filter((n) => n.status === 'completed').length ?? 0;
  const totalNodes = dagStatus?.nodes.length ?? 6;

  // Get button state
  const getActionButton = () => {
    if (isLoading) {
      return null;
    }

    if (!dagStatus || isCancelled) {
      return (
        <Button
          onClick={() => void handleStart()}
          disabled={isStarting}
          className="bg-purple-600 hover:bg-purple-700 text-white"
        >
          {isStarting ? (
            <>
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              Starting...
            </>
          ) : (
            <>
              <Brain className="h-4 w-4 mr-2" />
              Start Extraction
            </>
          )}
        </Button>
      );
    }

    if (isRunning) {
      return (
        <Button
          variant="outline"
          onClick={() => void handleCancel()}
          disabled={isCancelling}
          className="text-red-600 hover:text-red-700 hover:bg-red-50"
        >
          {isCancelling ? (
            <>
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              Cancelling...
            </>
          ) : (
            <>
              <X className="h-4 w-4 mr-2" />
              Cancel
            </>
          )}
        </Button>
      );
    }

    if (isComplete || isFailed) {
      return (
        <Button
          onClick={() => {
            if (isComplete) {
              // Show confirmation dialog for re-extraction
              setShowReextractDialog(true);
            } else {
              // Failed extraction - retry without confirmation
              void handleStart();
            }
          }}
          disabled={isStarting}
          className="bg-purple-600 hover:bg-purple-700 text-white"
        >
          {isStarting ? (
            <>
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
              Starting...
            </>
          ) : (
            <>
              <RefreshCw className="h-4 w-4 mr-2" />
              {isFailed ? 'Retry Extraction' : 'Re-extract Ontology'}
            </>
          )}
        </Button>
      );
    }

    return null;
  };

  // Get status banner
  const getStatusBanner = () => {
    if (isComplete) {
      return (
        <div className="mb-4 flex items-center gap-2 p-3 rounded-lg bg-green-50 border border-green-200 dark:bg-green-900/20 dark:border-green-800">
          <Check className="h-5 w-5 text-green-600" />
          <span className="text-green-800 dark:text-green-200 font-medium">
            Ontology extraction complete
          </span>
        </div>
      );
    }

    if (isFailed) {
      const failedNode = dagStatus?.nodes.find((n) => n.status === 'failed');
      return (
        <div className="mb-4 p-3 rounded-lg bg-red-50 border border-red-200 dark:bg-red-900/20 dark:border-red-800">
          <div className="flex items-center gap-2">
            <AlertCircle className="h-5 w-5 text-red-600" />
            <span className="text-red-800 dark:text-red-200 font-medium">Extraction failed</span>
          </div>
          {failedNode?.error && (
            <p className="mt-1 text-sm text-red-600 dark:text-red-300">{failedNode.error}</p>
          )}
        </div>
      );
    }

    if (isCancelled) {
      return (
        <div className="mb-4 flex items-center gap-2 p-3 rounded-lg bg-amber-50 border border-amber-200 dark:bg-amber-900/20 dark:border-amber-800">
          <AlertCircle className="h-5 w-5 text-amber-600" />
          <span className="text-amber-800 dark:text-amber-200 font-medium">
            Extraction was cancelled
          </span>
        </div>
      );
    }

    if (isRunning) {
      return (
        <div className="mb-4 flex items-center gap-2 p-3 rounded-lg bg-blue-50 border border-blue-200 dark:bg-blue-900/20 dark:border-blue-800">
          <Loader2 className="h-5 w-5 text-blue-600 animate-spin" />
          <span className="text-blue-800 dark:text-blue-200 font-medium">
            Extracting ontology... ({completedNodes}/{totalNodes} nodes complete)
          </span>
        </div>
      );
    }

    return null;
  };

  // Loading state
  if (isLoading) {
    return (
      <div className="rounded-lg border border-border-light bg-surface-primary p-8 shadow-sm">
        <div className="flex items-center justify-center">
          <Loader2 className="h-6 w-6 text-text-tertiary animate-spin" />
          <span className="ml-2 text-text-secondary">Loading...</span>
        </div>
      </div>
    );
  }

  // Error state (when fetch fails completely)
  if (error && !dagStatus) {
    return (
      <div className="rounded-lg border border-border-light bg-surface-primary p-8 shadow-sm">
        <div className="text-center">
          <AlertCircle className="h-12 w-12 text-red-400 mx-auto mb-4" />
          <h3 className="text-lg font-semibold text-text-primary mb-2">Unable to load status</h3>
          <p className="text-text-secondary mb-4">{error}</p>
          <Button variant="outline" onClick={() => void fetchStatus()}>
            <RefreshCw className="h-4 w-4 mr-2" />
            Retry
          </Button>
        </div>
      </div>
    );
  }

  // Empty state (no DAG has been run yet)
  if (!dagStatus) {
    const isOverviewValid = projectOverview.length >= 20;
    return (
      <div className="rounded-lg border border-border-light bg-surface-primary p-12 shadow-sm">
        <div className="text-center">
          <Network className="h-16 w-16 text-purple-300 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-text-primary mb-2">
            Ready to Extract Ontology
          </h2>
          <p className="text-text-secondary max-w-2xl mx-auto mb-6">
            Before we analyze your schema, tell us about your application. Who uses it and what do
            they do with this data? This context helps build a more accurate business ontology.
          </p>

          {/* Overview textarea */}
          <div className="max-w-2xl mx-auto mb-6 text-left">
            <label className="block text-sm font-medium text-text-primary mb-2">
              Describe your application
            </label>
            <textarea
              value={projectOverview}
              onChange={(e) => setProjectOverview(e.target.value.slice(0, 500))}
              placeholder="Example: This is our e-commerce platform for B2B wholesale. Customers are businesses that purchase products in bulk, while Users are employee accounts that manage orders..."
              className="w-full h-32 p-3 border border-border-light rounded-lg bg-surface-secondary text-text-primary placeholder:text-text-tertiary resize-none focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent"
              maxLength={500}
              disabled={isLoadingOverview}
            />
            <div className="flex justify-between mt-1 text-sm text-text-tertiary">
              <span>
                {projectOverview.length < 20
                  ? `${20 - projectOverview.length} more characters required`
                  : 'Ready to start'}
              </span>
              <span>{projectOverview.length}/500</span>
            </div>
          </div>

          {/* Button - disabled until 20 chars */}
          <Button
            onClick={() => void handleStart()}
            disabled={isStarting || !isOverviewValid || isLoadingOverview}
            className="bg-purple-600 hover:bg-purple-700 text-white disabled:opacity-50"
          >
            {isStarting ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Starting...
              </>
            ) : (
              <>
                <Brain className="h-4 w-4 mr-2" />
                Start Extraction
              </>
            )}
          </Button>
        </div>
      </div>
    );
  }

  // DAG visualization
  return (
    <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-border-light">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-purple-500/10">
            <Brain className="h-5 w-5 text-purple-500" />
          </div>
          <div>
            <h3 className="font-semibold text-text-primary">Ontology Extraction</h3>
            <p className="text-sm text-text-secondary">
              {completedNodes}/{totalNodes} steps complete
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {dagStatus && !isRunning && (
            <Button
              variant="outline"
              onClick={() => setShowDeleteDialog(true)}
              disabled={isDeleting}
              className="text-red-600 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-900/20"
            >
              <Trash2 className="h-4 w-4 mr-2" />
              Delete Ontology
            </Button>
          )}
          {getActionButton()}
        </div>
      </div>

      {/* Status banner */}
      <div className="p-4 pb-0">{getStatusBanner()}</div>

      {/* DAG nodes */}
      <div className="p-4">
        <div className="space-y-0">
          {dagStatus.nodes.map((node, index) => {
            const nodeDescription = DAGNodeDescriptions[node.name as DAGNodeName];
            const isCurrentNode = dagStatus.current_node === node.name;
            const prevNode = index > 0 ? dagStatus.nodes[index - 1] : null;

            return (
              <div key={node.name}>
                {/* Connector line */}
                {index > 0 && (
                  <div className="flex justify-center py-1">
                    <div className="flex flex-col items-center">
                      <div
                        className={`w-0.5 h-6 ${getConnectorColor(prevNode?.status ?? 'pending', node.status)}`}
                      />
                      <ArrowDown
                        className={`h-4 w-4 -mt-1 ${
                          prevNode?.status === 'completed'
                            ? 'text-green-500'
                            : prevNode?.status === 'running' || node.status === 'running'
                              ? 'text-blue-500'
                              : 'text-gray-300'
                        }`}
                      />
                    </div>
                  </div>
                )}

                {/* Node */}
                <div
                  className={`flex items-center gap-4 p-4 rounded-lg border transition-colors ${getNodeBackground(node.status, isCurrentNode)}`}
                >
                  {/* Status icon */}
                  <div className="flex-shrink-0">
                    {getNodeStatusIcon(node.status, isCurrentNode)}
                  </div>

                  {/* Node info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-text-primary">
                        {nodeDescription?.title ?? node.name}
                      </span>
                      {isCurrentNode && node.status === 'running' && (
                        <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300">
                          Current
                        </span>
                      )}
                    </div>
                    <p className="text-sm text-text-secondary">
                      {node.status === 'running' && node.progress?.message
                        ? node.progress.message
                        : nodeDescription?.description ?? ''}
                    </p>
                    {/* Multi-phase progress for ColumnFeatureExtraction node */}
                    {node.name === 'ColumnFeatureExtraction' &&
                      (node.status === 'running' || node.status === 'completed') && (
                        <ExtractionProgress
                          progress={node.progress}
                          nodeStatus={node.status}
                          nodeType="ColumnFeatureExtraction"
                          className="mt-2"
                        />
                      )}
                    {/* Multi-phase progress for PKMatchDiscovery node */}
                    {node.name === 'PKMatchDiscovery' &&
                      (node.status === 'running' || node.status === 'completed') && (
                        <ExtractionProgress
                          progress={node.progress}
                          nodeStatus={node.status}
                          nodeType="PKMatchDiscovery"
                          className="mt-2"
                        />
                      )}
                    {/* Standard progress bar for other nodes */}
                    {node.name !== 'ColumnFeatureExtraction' &&
                      node.name !== 'PKMatchDiscovery' &&
                      node.status === 'running' &&
                      node.progress &&
                      node.progress.total > 0 && (
                        <div className="mt-2">
                          <div className="flex items-center gap-2 text-xs text-text-tertiary mb-1">
                            <span>
                              {node.progress.current}/{node.progress.total}
                            </span>
                          </div>
                          <div className="h-1.5 w-full bg-gray-200 rounded-full overflow-hidden dark:bg-gray-700">
                            <div
                              className="h-full bg-purple-500 rounded-full transition-all duration-300"
                              style={{
                                width: `${Math.round((node.progress.current / node.progress.total) * 100)}%`,
                              }}
                            />
                          </div>
                        </div>
                      )}
                    {node.status === 'failed' && node.error && (
                      <p className="mt-1 text-sm text-red-600 dark:text-red-400">{node.error}</p>
                    )}
                  </div>

                  {/* Status badge */}
                  <div className="flex-shrink-0">
                    <span
                      className={`px-2 py-1 rounded-full text-xs font-medium ${
                        node.status === 'completed'
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                          : node.status === 'running'
                            ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                            : node.status === 'failed'
                              ? 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                              : node.status === 'skipped'
                                ? 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
                                : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-500'
                      }`}
                    >
                      {node.status.charAt(0).toUpperCase() + node.status.slice(1)}
                    </span>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Re-extraction confirmation dialog */}
      <Dialog open={showReextractDialog} onOpenChange={setShowReextractDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Re-extract Ontology?</DialogTitle>
            <DialogDescription>
              This will start a complete re-extraction of your ontology from scratch, which typically
              takes 10-15 minutes. All existing ontology data will be replaced.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <div className="rounded-lg bg-amber-50 border border-amber-200 p-4 dark:bg-amber-900/20 dark:border-amber-800">
              <div className="flex items-start gap-3">
                <AlertCircle className="h-5 w-5 text-amber-600 flex-shrink-0 mt-0.5" />
                <div className="text-sm text-amber-800 dark:text-amber-200">
                  <p className="font-medium mb-1">This is a full re-extraction</p>
                  <p>
                    If you&apos;re looking to update the ontology with recent schema changes, this feature
                    is not yet implemented. For now, re-extraction will analyze your entire database
                    from the beginning.
                  </p>
                </div>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowReextractDialog(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => {
                setShowReextractDialog(false);
                void handleStart();
              }}
              className="bg-purple-600 hover:bg-purple-700 text-white"
            >
              Start Re-extraction
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete ontology confirmation dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={(open) => {
        setShowDeleteDialog(open);
        if (!open) {
          setDeleteConfirmText('');
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Ontology?</DialogTitle>
            <DialogDescription>
              This will permanently delete all ontology data, including table metadata, relationships,
              questions, and chat history. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4 space-y-4">
            <div className="rounded-lg bg-red-50 border border-red-200 p-4 dark:bg-red-900/20 dark:border-red-800">
              <div className="flex items-start gap-3">
                <AlertCircle className="h-5 w-5 text-red-600 flex-shrink-0 mt-0.5" />
                <div className="text-sm text-red-800 dark:text-red-200">
                  <p className="font-medium mb-1">Warning: This is a destructive action</p>
                  <p>
                    All extracted knowledge will be permanently deleted. You will need to run a full
                    re-extraction to restore the ontology.
                  </p>
                </div>
              </div>
            </div>
            <div>
              <label htmlFor="delete-confirm" className="block text-sm font-medium text-text-primary mb-2">
                Type <code className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 rounded text-xs font-mono">delete ontology</code> to confirm
              </label>
              <Input
                id="delete-confirm"
                type="text"
                value={deleteConfirmText}
                onChange={(e) => setDeleteConfirmText(e.target.value)}
                placeholder="delete ontology"
                className="w-full"
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setShowDeleteDialog(false);
                setDeleteConfirmText('');
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={() => void handleDelete()}
              disabled={deleteConfirmText !== 'delete ontology' || isDeleting}
              className="bg-red-600 hover:bg-red-700 text-white disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isDeleting ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Deleting...
                </>
              ) : (
                'Delete Ontology'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default OntologyDAG;

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
  Download,
  Loader2,
  Network,
  RefreshCw,
  Trash2,
  Upload,
  X,
} from 'lucide-react';
import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react';

import { getUserRoles } from '../../lib/auth-token';
import engineApi from '../../services/engineApi';
import type {
  DAGNodeName,
  DAGNodeStatus,
  DAGStatusResponse,
  DAGStatus,
  OntologyImportColumnRef,
  OntologyImportRelationshipIssue,
  OntologyImportTableRef,
  OntologyImportValidationReport,
  OntologyStatusResponse,
} from '../../types';
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
const DEFAULT_EXPORT_FILENAME = 'ontology-export.json';
const MAX_IMPORT_BYTES = 5 * 1024 * 1024;

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

// Helper to format change summary into a human-readable string
const formatChangeSummary = (summary: { tables_added: number; tables_modified: number; tables_deleted: number; columns_added: number; columns_modified: number; columns_deleted: number }): string => {
  const parts: string[] = [];
  if (summary.tables_added > 0) parts.push(`${summary.tables_added} new table${summary.tables_added !== 1 ? 's' : ''}`);
  if (summary.tables_modified > 0) parts.push(`${summary.tables_modified} modified table${summary.tables_modified !== 1 ? 's' : ''}`);
  if (summary.tables_deleted > 0) parts.push(`${summary.tables_deleted} deleted table${summary.tables_deleted !== 1 ? 's' : ''}`);
  if (summary.columns_added > 0) parts.push(`${summary.columns_added} new column${summary.columns_added !== 1 ? 's' : ''}`);
  if (summary.columns_modified > 0) parts.push(`${summary.columns_modified} modified column${summary.columns_modified !== 1 ? 's' : ''}`);
  if (summary.columns_deleted > 0) parts.push(`${summary.columns_deleted} deleted column${summary.columns_deleted !== 1 ? 's' : ''}`);
  return parts.join(', ');
};

export const OntologyDAG = ({
  projectId,
  datasourceId,
  onComplete,
  onError,
  onStatusChange,
}: OntologyDAGProps) => {
  const [dagStatus, setDagStatus] = useState<DAGStatusResponse | null>(null);
  const [ontologyStatus, setOntologyStatus] = useState<OntologyStatusResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isStarting, setIsStarting] = useState(false);
  const [isCancelling, setIsCancelling] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [isExporting, setIsExporting] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [importErrorMessage, setImportErrorMessage] = useState<string | null>(null);
  const [importReport, setImportReport] = useState<OntologyImportValidationReport | null>(null);
  const [projectOverview, setProjectOverview] = useState('');
  const [isLoadingOverview, setIsLoadingOverview] = useState(true);

  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const isMountedRef = useRef(true);
  const importInputRef = useRef<HTMLInputElement | null>(null);

  // Derived state
  const roles = getUserRoles();
  const isAdmin = roles.includes('admin');
  const isRunning = dagStatus?.status === 'running' || dagStatus?.status === 'pending';
  const isComplete = dagStatus?.status === 'completed';
  const isFailed = dagStatus?.status === 'failed';
  const isCancelled = dagStatus?.status === 'cancelled';
  const hasOntology = Boolean(ontologyStatus?.has_ontology);
  const isImportedComplete =
    hasOntology && ontologyStatus?.completion_provenance === 'imported';

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
            // Refresh ontology status to clear schema change badge
            try {
              const statusResp = await engineApi.getOntologyStatus(projectId, datasourceId);
              if (isMountedRef.current && statusResp.data) {
                setOntologyStatus(statusResp.data);
              }
            } catch {
              // Non-critical, ignore
            }
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

  const clearImportFeedback = useCallback(() => {
    setImportErrorMessage(null);
    setImportReport(null);
  }, []);

  // Delete ontology
  const handleDelete = useCallback(async () => {
    try {
      setIsDeleting(true);

      await engineApi.deleteOntology(projectId, datasourceId);

      if (!isMountedRef.current) return;

      // Reset state
      setDagStatus(null);
      setOntologyStatus({
        has_ontology: false,
        schema_changed_since_build: false,
      });
      setShowDeleteDialog(false);
      setDeleteConfirmText('');
      clearImportFeedback();
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
  }, [clearImportFeedback, projectId, datasourceId, stopPolling, onError]);

  const handleExport = useCallback(async () => {
    try {
      setIsExporting(true);
      setError(null);

      const blob = await engineApi.exportOntologyBundle(projectId, datasourceId);
      downloadBlob(blob, DEFAULT_EXPORT_FILENAME);

      if (!isMountedRef.current) return;
    } catch (err) {
      if (!isMountedRef.current) return;
      if (isAbortError(err)) {
        return;
      }
      const errorMessage = err instanceof Error ? err.message : 'Failed to export ontology bundle';
      console.error('Failed to export ontology bundle:', err);
      setError(errorMessage);
      onError?.(errorMessage);
    } finally {
      if (isMountedRef.current) {
        setIsExporting(false);
      }
    }
  }, [datasourceId, onError, projectId]);

  const handleImport = useCallback(async (file: File) => {
    try {
      setIsImporting(true);
      setError(null);
      clearImportFeedback();

      const response = await engineApi.importOntologyBundle(projectId, datasourceId, file);

      if (!isMountedRef.current) return;

      if (response.data) {
        stopPolling();
        setDagStatus(null);
        setOntologyStatus({
          has_ontology: true,
          last_built_at: response.data.imported_at,
          completion_provenance: response.data.completion_provenance,
          schema_changed_since_build: false,
        });
      }
    } catch (err) {
      if (!isMountedRef.current) return;
      const failure = getImportFailure(err);
      setImportErrorMessage(failure.message);
      setImportReport(failure.report);
    } finally {
      if (isMountedRef.current) {
        setIsImporting(false);
      }
    }
  }, [clearImportFeedback, datasourceId, projectId, stopPolling]);

  const handleImportInputChange = useCallback(async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) {
      return;
    }

    if (!file.name.toLowerCase().endsWith('.json')) {
      setImportErrorMessage('Ontology bundle must be a .json file');
      setImportReport({
        problems: [
          {
            code: 'invalid_file',
            message: 'Ontology bundle must use the .json file extension.',
          },
        ],
      });
      return;
    }

    if (file.size > MAX_IMPORT_BYTES) {
      setImportErrorMessage('Ontology bundle exceeds the 5 MB maximum size');
      setImportReport({
        problems: [
          {
            code: 'file_too_large',
            message: 'Ontology bundle exceeds the 5 MB maximum size.',
          },
        ],
      });
      return;
    }

    await handleImport(file);
  }, [handleImport]);

  const openImportPicker = useCallback(() => {
    clearImportFeedback();
    importInputRef.current?.click();
  }, [clearImportFeedback]);

  // Initial load
  useEffect(() => {
    const init = async () => {
      if (!projectId || !datasourceId) return;

      setIsLoading(true);
      setIsLoadingOverview(true);

      try {
        // Fetch DAG status, project overview, and ontology status in parallel
        const [dagResponse, overviewResponse, ontologyStatusResponse] = await Promise.all([
          engineApi.getOntologyDAGStatus(projectId, datasourceId),
          engineApi.getProjectOverview(projectId),
          engineApi.getOntologyStatus(projectId, datasourceId),
        ]);

        if (!isMountedRef.current) return;

        // Handle project overview
        if (overviewResponse.data?.overview) {
          setProjectOverview(overviewResponse.data.overview);
        }

        // Handle ontology status (change detection)
        if (ontologyStatusResponse.data) {
          setOntologyStatus(ontologyStatusResponse.data);
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
    onStatusChange?.(hasOntology);
  }, [hasOntology, onStatusChange]);

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

    if (isFailed) {
      return (
        <Button
          onClick={() => {
            void handleStart();
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
              Retry Extraction
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
        <div className="mb-4 space-y-2">
          <div className="flex items-center gap-2 p-3 rounded-lg bg-green-50 border border-green-200 dark:bg-green-900/20 dark:border-green-800">
            <Check className="h-5 w-5 text-green-600" />
            <span className="text-green-800 dark:text-green-200 font-medium">
              Ontology extraction complete
              {dagStatus?.is_incremental && ' (incremental refresh)'}
            </span>
          </div>
          {ontologyStatus?.schema_changed_since_build && ontologyStatus.change_summary && (
            <div className="flex items-center gap-2 p-3 rounded-lg bg-amber-50 border border-amber-200 dark:bg-amber-900/20 dark:border-amber-800">
              <AlertCircle className="h-5 w-5 text-amber-600" />
              <span className="text-amber-800 dark:text-amber-200 text-sm">
                Schema changes detected: {formatChangeSummary(ontologyStatus.change_summary)}.
                Click <strong>Refresh Ontology</strong> to update.
              </span>
            </div>
          )}
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
      const changeSummaryText = dagStatus?.is_incremental && dagStatus.change_summary
        ? `Refreshing ontology: ${formatChangeSummary(dagStatus.change_summary)}`
        : `Extracting ontology...`;
      return (
        <div className="mb-4 flex items-center gap-2 p-3 rounded-lg bg-blue-50 border border-blue-200 dark:bg-blue-900/20 dark:border-blue-800">
          <Loader2 className="h-5 w-5 text-blue-600 animate-spin" />
          <span className="text-blue-800 dark:text-blue-200 font-medium">
            {changeSummaryText} ({completedNodes}/{totalNodes} nodes complete)
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
  if (error && !dagStatus && !hasOntology) {
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

  if (isImportedComplete) {
    return (
      <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm">
        <div className="flex items-center justify-between p-4 border-b border-border-light">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-green-500/10">
              <Check className="h-5 w-5 text-green-600" />
            </div>
            <div>
              <h3 className="font-semibold text-text-primary">Ontology Import Complete</h3>
              <p className="text-sm text-text-secondary">
                Imported ontology bundle is active for this datasource
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={() => setShowDeleteDialog(true)}
              disabled={isDeleting}
              className="text-red-600 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-900/20"
            >
              <Trash2 className="h-4 w-4 mr-2" />
              Delete Ontology
            </Button>
            <Button
              variant="outline"
              onClick={() => void handleExport()}
              disabled={isExporting}
            >
              {isExporting ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Exporting...
                </>
              ) : (
                <>
                  <Download className="h-4 w-4 mr-2" />
                  Export Ontology
                </>
              )}
            </Button>
          </div>
        </div>

        <div className="p-6 space-y-4">
          <div className="flex items-start gap-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20">
            <Check className="h-5 w-5 flex-shrink-0 text-green-600" />
            <div>
              <p className="font-medium text-green-800 dark:text-green-200">
                Ontology bundle imported successfully
              </p>
            </div>
          </div>

          {ontologyStatus?.last_built_at && (
            <p className="text-sm text-text-secondary">
              Imported on {formatTimestamp(ontologyStatus.last_built_at)}.
            </p>
          )}

          {ontologyStatus?.schema_changed_since_build && ontologyStatus.change_summary && (
            <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20">
              <AlertCircle className="h-5 w-5 flex-shrink-0 text-amber-600" />
              <p className="text-sm text-amber-800 dark:text-amber-200">
                Schema changes detected since import: {formatChangeSummary(ontologyStatus.change_summary)}.
              </p>
            </div>
          )}
        </div>

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
                      All imported ontology knowledge will be permanently deleted. You will need to
                      import again or run a fresh extraction to restore it.
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
  }

  // Empty state (no DAG has been run yet)
  if (!dagStatus && !hasOntology) {
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

          <input
            ref={importInputRef}
            type="file"
            accept=".json,application/json"
            className="hidden"
            onChange={(event) => void handleImportInputChange(event)}
          />

          {(importErrorMessage !== null || importReport !== null) && (
            <div className="mx-auto mb-6 max-w-3xl rounded-lg border border-amber-200 bg-amber-50 p-4 text-left dark:border-amber-800 dark:bg-amber-900/20">
              <div className="flex items-start justify-between gap-4">
                <div className="flex items-start gap-3">
                  <AlertCircle className="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-600" />
                  <div className="space-y-3">
                    <div>
                      <p className="font-medium text-amber-800 dark:text-amber-200">
                        {importErrorMessage ?? 'Unable to import ontology bundle'}
                      </p>
                      <p className="mt-1 text-sm text-amber-700 dark:text-amber-300">
                        Fix the bundle or target datasource mismatch, then try again.
                      </p>
                    </div>
                    <ImportValidationDetails report={importReport} />
                  </div>
                </div>
                <Button variant="ghost" size="sm" onClick={clearImportFeedback}>
                  Dismiss
                </Button>
              </div>
            </div>
          )}

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

          <div className="flex flex-wrap items-center justify-center gap-3">
            <Button
              onClick={() => void handleStart()}
              disabled={isStarting || isImporting || !isOverviewValid || isLoadingOverview}
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
            {isAdmin && (
              <Button
                variant="outline"
                onClick={openImportPicker}
                disabled={isImporting || isStarting || isLoadingOverview}
              >
                {isImporting ? (
                  <>
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    Importing...
                  </>
                ) : (
                  <>
                    <Upload className="h-4 w-4 mr-2" />
                    Import Ontology
                  </>
                )}
              </Button>
            )}
          </div>
        </div>
      </div>
    );
  }

  if (!dagStatus) {
    return null;
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
          {isComplete && (
            <Button
              variant="outline"
              onClick={() => void handleExport()}
              disabled={isExporting}
            >
              {isExporting ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Exporting...
                </>
              ) : (
                <>
                  <Download className="h-4 w-4 mr-2" />
                  Export Ontology
                </>
              )}
            </Button>
          )}
          {isComplete && ontologyStatus?.schema_changed_since_build && (
            <Button
              onClick={() => void handleStart()}
              disabled={isStarting}
              variant="outline"
              className="text-purple-600 hover:text-purple-700 hover:bg-purple-50 dark:hover:bg-purple-900/20"
            >
              {isStarting ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Refreshing...
                </>
              ) : (
                <>
                  <RefreshCw className="h-4 w-4 mr-2" />
                  Refresh Ontology
                </>
              )}
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
                    {/* Multi-phase progress for RelationshipDiscovery node */}
                    {node.name === 'RelationshipDiscovery' &&
                      (node.status === 'running' || node.status === 'completed') && (
                        <ExtractionProgress
                          progress={node.progress}
                          nodeStatus={node.status}
                          nodeType="RelationshipDiscovery"
                          className="mt-2"
                        />
                      )}
                    {/* Standard progress bar for other nodes */}
                    {node.name !== 'ColumnFeatureExtraction' &&
                      node.name !== 'RelationshipDiscovery' &&
                      node.status === 'running' &&
                      node.progress &&
                      node.progress.total > 1 && (
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

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

function downloadBlob(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = objectUrl;
  link.download = filename;
  link.style.display = 'none';
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(objectUrl);
}

function formatTimestamp(value: string): string {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return value;
  }
  return timestamp.toLocaleString();
}

function getImportFailure(error: unknown): {
  message: string;
  report: OntologyImportValidationReport | null;
} {
  if (typeof error === 'object' && error !== null) {
    const maybeMessage = 'message' in error ? error.message : undefined;
    const maybeReport = 'report' in error ? error.report : undefined;

    return {
      message: typeof maybeMessage === 'string' ? maybeMessage : 'Failed to import ontology bundle',
      report: isOntologyImportValidationReportValue(maybeReport) ? maybeReport : null,
    };
  }

  return {
    message: error instanceof Error ? error.message : 'Failed to import ontology bundle',
    report: null,
  };
}

function isOntologyImportValidationReportValue(
  value: unknown
): value is OntologyImportValidationReport {
  return typeof value === 'object' && value !== null;
}

function ImportValidationDetails({
  report,
}: {
  report: OntologyImportValidationReport | null;
}) {
  if (!report) {
    return null;
  }

  const rows: Array<{ label: string; items: string[] }> = [];

  if (report.database_type_mismatch) {
    rows.push({
      label: 'Database type mismatch',
      items: [
        `Bundle: ${report.database_type_mismatch.bundle_type}, target: ${report.database_type_mismatch.target_type}`,
      ],
    });
  }
  if (report.problems?.length) {
    rows.push({
      label: 'Problems',
      items: report.problems.map((problem) => problem.message),
    });
  }
  if (report.missing_required_apps?.length) {
    rows.push({
      label: 'Missing required apps',
      items: report.missing_required_apps,
    });
  }
  if (report.missing_tables?.length) {
    rows.push({
      label: 'Missing tables',
      items: report.missing_tables.map(formatTableRef),
    });
  }
  if (report.unexpected_tables?.length) {
    rows.push({
      label: 'Unexpected selected tables',
      items: report.unexpected_tables.map(formatTableRef),
    });
  }
  if (report.missing_columns?.length) {
    rows.push({
      label: 'Missing columns',
      items: report.missing_columns.map(formatColumnRef),
    });
  }
  if (report.unexpected_columns?.length) {
    rows.push({
      label: 'Unexpected selected columns',
      items: report.unexpected_columns.map(formatColumnRef),
    });
  }
  if (report.unresolved_relationships?.length) {
    rows.push({
      label: 'Unresolved relationships',
      items: report.unresolved_relationships.map(formatRelationshipIssue),
    });
  }

  if (rows.length === 0) {
    return null;
  }

  return (
    <div className="space-y-3">
      {rows.map((row) => (
        <div key={row.label}>
          <p className="text-sm font-medium text-amber-900 dark:text-amber-100">{row.label}</p>
          <ul className="mt-1 list-disc pl-5 text-sm text-amber-800 dark:text-amber-200">
            {row.items.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}

function formatTableRef(ref: OntologyImportTableRef): string {
  if (ref.schema_name) {
    return `${ref.schema_name}.${ref.table_name}`;
  }
  return ref.table_name;
}

function formatColumnRef(ref: OntologyImportColumnRef): string {
  return `${formatTableRef(ref.table)}.${ref.column_name}`;
}

function formatRelationshipIssue(issue: OntologyImportRelationshipIssue): string {
  return `${formatColumnRef(issue.source)} -> ${formatColumnRef(issue.target)}: ${issue.message}`;
}

/**
 * OntologyPage - Tiered Ontology Extraction UI
 * Living document with work queue model
 */

import { AlertTriangle, ArrowLeft, ArrowRight, Brain, CheckCircle, Play, RefreshCw, X } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import ChatPane from '../components/ontology/ChatPane';
import OntologyStatus from '../components/ontology/OntologyStatus';
import QuestionPanel from '../components/ontology/QuestionPanel';
import WorkQueue from '../components/ontology/WorkQueue';
import { Button } from '../components/ui/Button';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import engineApi from '../services/engineApi';
import ontologyApi from '../services/ontologyApi';
import { ontologyService } from '../services/ontologyService';
import relationshipWorkflowApi from '../services/relationshipWorkflowApi';
import type { OntologyWorkflowStatus } from '../types';

// Prerequisites status for ontology extraction
interface PrerequisitesStatus {
  entitiesComplete: boolean;
  relationshipsComplete: boolean;
  entityCount: number;
  relationshipCount: number;
  loading: boolean;
  error: string | null;
}

const OntologyPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { selectedDatasource } = useDatasourceConnection();

  const [status, setStatus] = useState<OntologyWorkflowStatus | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [hasPendingQuestions, setHasPendingQuestions] = useState(false);
  const [allQuestionsComplete, setAllQuestionsComplete] = useState(false);
  const [projectDescription, setProjectDescription] = useState('');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showCancelConfirm, setShowCancelConfirm] = useState(false);

  // Prerequisites status for ontology extraction
  const [prerequisites, setPrerequisites] = useState<PrerequisitesStatus>({
    entitiesComplete: false,
    relationshipsComplete: false,
    entityCount: 0,
    relationshipCount: 0,
    loading: true,
    error: null,
  });

  // Initialize and subscribe to status updates
  useEffect(() => {
    let isMounted = true;

    const initializeService = async () => {
      if (!pid) return;

      setIsLoading(true);
      setError(null);
      setFetchError(null);

      try {
        // First, check if the API is reachable before initializing the service
        // This catches connection errors that initialize() swallows internally
        const statusResponse = await ontologyApi.getStatus(pid);

        if (!isMounted) return;

        // API is reachable, now initialize the service
        await ontologyService.initialize(pid);

        if (!isMounted) return;

        // Check for pending questions
        const nextQuestionResponse = await ontologyApi.getNextQuestion(pid);

        if (!isMounted) return;

        // Only show "all complete" if ontology is actually ready
        const ontologyIsReady = statusResponse.ontology_ready ?? false;

        if (nextQuestionResponse.all_complete && ontologyIsReady) {
          setHasPendingQuestions(false);
          setAllQuestionsComplete(true);
        } else if (nextQuestionResponse.question) {
          setHasPendingQuestions(true);
          setAllQuestionsComplete(false);
        } else {
          // No questions yet, and ontology not ready - we're still building
          setHasPendingQuestions(false);
          setAllQuestionsComplete(false);
        }
      } catch (e) {
        if (!isMounted) return;
        console.error('Failed to initialize ontology service:', e);
        const errorMessage = e instanceof Error && e.message.includes('Failed to fetch')
          ? 'Service is currently down.'
          : 'Unable to connect to the ontology service.';
        setFetchError(errorMessage);
      } finally {
        if (isMounted) {
          setIsLoading(false);
        }
      }
    };

    void initializeService();

    // Subscribe to status updates
    const unsubscribe = ontologyService.subscribe((newStatus) => {
      if (isMounted) {
        setStatus(newStatus);
      }
    });

    return () => {
      isMounted = false;
      unsubscribe();
      ontologyService.stop();
    };
  }, [pid]);

  // Check prerequisites (entities and relationships phases must be complete)
  useEffect(() => {
    let isMounted = true;

    const checkPrerequisites = async () => {
      if (!pid || !selectedDatasource) {
        setPrerequisites(prev => ({ ...prev, loading: false }));
        return;
      }

      const datasourceId = selectedDatasource.datasourceId;
      if (!datasourceId) {
        setPrerequisites(prev => ({ ...prev, loading: false }));
        return;
      }

      try {
        // Check entities status and relationships status in parallel
        const [entitiesStatusResponse, relationshipsStatusResponse, entitiesListResponse] = await Promise.all([
          engineApi.getEntityDiscoveryStatus(pid, datasourceId),
          relationshipWorkflowApi.getStatus(pid, datasourceId),
          engineApi.listEntities(pid),
        ]);

        if (!isMounted) return;

        const entitiesStatus = entitiesStatusResponse.data;
        const relationshipsStatus = relationshipsStatusResponse;
        const entitiesList = entitiesListResponse.data;

        setPrerequisites({
          entitiesComplete: entitiesStatus?.state === 'completed',
          relationshipsComplete: relationshipsStatus.state === 'completed',
          entityCount: entitiesList?.entities?.filter(e => !e.is_deleted).length ?? 0,
          relationshipCount: relationshipsStatus.confirmed_count ?? 0,
          loading: false,
          error: null,
        });
      } catch (e) {
        if (!isMounted) return;
        console.error('Failed to check prerequisites:', e);
        // Not a fatal error - just assume prerequisites not met
        setPrerequisites({
          entitiesComplete: false,
          relationshipsComplete: false,
          entityCount: 0,
          relationshipCount: 0,
          loading: false,
          error: null, // Don't show error - just show prerequisites needed
        });
      }
    };

    void checkPrerequisites();

    return () => {
      isMounted = false;
    };
  }, [pid, selectedDatasource]);

  const handleStart = useCallback(async () => {
    setError(null);

    try {
      await ontologyService.startExtraction(projectDescription);
    } catch (e) {
      console.error('Failed to start extraction:', e);
      // Handle the "no_datasource_configured" error specially
      const error = e as Error & { data?: { error?: string } };
      if (error.data?.error === 'no_datasource_configured') {
        setError('No datasource configured. Please set up a database connection first.');
      } else {
        setError('Failed to start extraction. Please try again.');
      }
    }
  }, [projectDescription]);

  const handleCancel = useCallback(() => {
    setShowCancelConfirm(true);
  }, []);

  const handleCancelConfirm = useCallback(async () => {
    try {
      await ontologyService.cancel();
      setShowCancelConfirm(false);
    } catch (e) {
      console.error('Failed to cancel:', e);
      setShowCancelConfirm(false);
      setError('Failed to cancel extraction. Please try again.');
    }
  }, []);

  const handleDeleteOntology = useCallback(async () => {
    try {
      await ontologyService.deleteOntology();
      setShowDeleteConfirm(false);
      setHasPendingQuestions(false);
      setAllQuestionsComplete(false);
    } catch (e) {
      console.error('Failed to delete ontology:', e);
    }
  }, []);

  // TODO: handleRefresh removed - refresh button hidden until incremental refresh is implemented
  // Workaround: Delete and rebuild from scratch

  // Handlers for real-time ontology updates from chat
  const handleOntologyUpdate = useCallback((update: { entity: string; field: string; summary: string }) => {
    console.log('Ontology updated:', update);
    // Could trigger a refresh of the ontology display here
  }, []);

  const handleKnowledgeStored = useCallback((fact: { factType: string; key: string; value: string }) => {
    console.log('Knowledge stored:', fact);
    // Could show a toast notification here
  }, []);

  // Handler for when all questions are complete
  const handleAllQuestionsComplete = useCallback(() => {
    setHasPendingQuestions(false);
    setAllQuestionsComplete(true);
  }, []);

  // Handler for when a question is answered
  const handleQuestionAnswered = useCallback((questionId: string, actionsSummary: string) => {
    console.log('Question answered:', questionId, actionsSummary);
  }, []);

  // Handler for retrying after fetch error
  const handleRetry = useCallback(async () => {
    if (!pid) return;

    setIsLoading(true);
    setFetchError(null);

    try {
      // First check if API is reachable
      const statusResponse = await ontologyApi.getStatus(pid);

      // API is reachable, initialize the service
      await ontologyService.initialize(pid);

      // Check for pending questions
      const nextQuestionResponse = await ontologyApi.getNextQuestion(pid);

      const ontologyIsReady = statusResponse.ontology_ready ?? false;

      if (nextQuestionResponse.all_complete && ontologyIsReady) {
        setHasPendingQuestions(false);
        setAllQuestionsComplete(true);
      } else if (nextQuestionResponse.question) {
        setHasPendingQuestions(true);
        setAllQuestionsComplete(false);
      } else {
        setHasPendingQuestions(false);
        setAllQuestionsComplete(false);
      }
    } catch (e) {
      console.error('Retry failed:', e);
      const errorMessage = e instanceof Error && e.message.includes('Failed to fetch')
        ? 'Service is currently down.'
        : 'Unable to connect to the ontology service.';
      setFetchError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  }, [pid]);

  const isIdle = status?.progress.state === 'idle';
  const isError = status?.progress.state === 'error';
  const isComplete = status?.progress.state === 'complete';
  const isRunning =
    status?.progress.state === 'building' || status?.progress.state === 'initializing';
  const isOntologyReady = status?.ontologyReady ?? false;

  // Re-check for pending questions when workflow completes
  useEffect(() => {
    if (!pid || !isComplete || !isOntologyReady) return;

    const checkPendingQuestions = async () => {
      try {
        const nextQuestionResponse = await ontologyApi.getNextQuestion(pid);
        if (nextQuestionResponse.question) {
          setHasPendingQuestions(true);
          setAllQuestionsComplete(false);
        } else if (nextQuestionResponse.all_complete) {
          setHasPendingQuestions(false);
          setAllQuestionsComplete(true);
        }
      } catch (e) {
        console.error('Failed to check pending questions:', e);
      }
    };

    void checkPendingQuestions();
  }, [pid, isComplete, isOntologyReady]);

  if (isLoading) {
    return (
      <div className="mx-auto max-w-7xl">
        <div className="flex items-center justify-center h-64">
          <div className="text-text-secondary">Loading...</div>
        </div>
      </div>
    );
  }

  // Error state - unable to fetch ontology status (initial load or polling error)
  if (fetchError || isError) {
    const errorMessage = fetchError ?? status?.lastError ?? 'Unable to connect to the ontology service.';
    return (
      <div className="mx-auto max-w-7xl">
        <div className="mb-6">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>

        <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm p-12">
          <AlertTriangle className="h-16 w-16 text-red-400 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-text-primary mb-2 text-center">
            Unable to Get Ontology
          </h2>
          <p className="text-text-secondary max-w-md mx-auto mb-6 text-center">
            {errorMessage}
          </p>
          <div className="text-center">
            <Button
              onClick={() => void handleRetry()}
              variant="outline"
              className="gap-2"
            >
              <RefreshCw className="h-4 w-4" />
              Retry
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6">
        <div className="mb-4">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>

        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
              <Brain className="h-8 w-8 text-purple-500" />
              Ontology Extraction
            </h1>
            <p className="mt-2 text-text-secondary">
              Extract business knowledge from your database schema
            </p>
          </div>
        </div>

        {/* Error banner - client-side errors */}
        {error && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4">
            <p className="text-red-800 text-sm">{error}</p>
          </div>
        )}

        {/* Error banner - workflow errors from backend */}
        {status?.lastError && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4">
            <p className="text-red-800 text-sm font-semibold">Workflow Error</p>
            <p className="text-red-700 text-sm mt-1">{status.lastError}</p>
          </div>
        )}
      </div>

      {/* Status bar - show when not idle */}
      {status && !isIdle && (
        <div className="mb-6">
          <OntologyStatus
            progress={status.progress}
            pendingQuestionCount={status.pendingQuestionCount}
            onCancel={handleCancel}
            onDelete={() => setShowDeleteConfirm(true)}
          />
        </div>
      )}

      {/* Main content area */}
      {status && !isIdle ? (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Work Queue - left column */}
          <div>
            <WorkQueue
              items={status.workQueue}
              taskItems={status.taskQueue}
              maxHeight="500px"
            />
          </div>

          {/* Right column (2/3 width) - Question Panel or Chat */}
          <div className="lg:col-span-2">
            {/* Always show QuestionPanel - it handles its own disabled/enabled state */}
            <QuestionPanel
              projectId={pid ?? ''}
              onAllComplete={handleAllQuestionsComplete}
              onQuestionAnswered={handleQuestionAnswered}
              pollForQuestions={isRunning}
              workflowComplete={isComplete || isOntologyReady}
            />

            {/* Show completion banner + Chat when all questions are done AND ontology is ready */}
            {allQuestionsComplete && isOntologyReady && (
              <>
                <div className="mb-6 rounded-lg border border-green-200 bg-green-50 p-4 flex items-center gap-3">
                  <CheckCircle className="h-6 w-6 text-green-600 flex-shrink-0" />
                  <div>
                    <p className="text-green-800 font-medium">All questions answered!</p>
                    <p className="text-green-700 text-sm">
                      Use the chat below for any additional questions or to refine the ontology.
                    </p>
                  </div>
                </div>
                <ChatPane
                  projectId={pid ?? ''}
                  onOntologyUpdate={handleOntologyUpdate}
                  onKnowledgeStored={handleKnowledgeStored}
                />
              </>
            )}

            {/* Show Chat only when ontology is ready and no questions exist */}
            {isOntologyReady && !hasPendingQuestions && !allQuestionsComplete && (
              <ChatPane
                projectId={pid ?? ''}
                onOntologyUpdate={handleOntologyUpdate}
                onKnowledgeStored={handleKnowledgeStored}
              />
            )}
          </div>
        </div>
      ) : prerequisites.loading ? (
        /* Loading prerequisites check */
        <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm p-12">
          <div className="flex items-center justify-center">
            <div className="text-text-secondary">Checking prerequisites...</div>
          </div>
        </div>
      ) : !prerequisites.entitiesComplete || !prerequisites.relationshipsComplete ? (
        /* Prerequisites not met - show guidance */
        <div className="rounded-lg border border-amber-200 bg-amber-50 shadow-sm p-12">
          <AlertTriangle className="h-16 w-16 text-amber-400 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-text-primary mb-2 text-center">
            Prerequisites Required
          </h2>
          <p className="text-text-secondary max-w-3xl mx-auto mb-6 text-center">
            Before building the ontology, complete these steps. The ontology combines your
            schema, entities, and relationships into a unified business understanding.
          </p>

          <div className="max-w-lg mx-auto space-y-4">
            {/* Entities prerequisite */}
            <div className={`flex items-center gap-4 p-4 rounded-lg border ${
              prerequisites.entitiesComplete
                ? 'border-green-200 bg-green-50'
                : 'border-gray-200 bg-white'
            }`}>
              <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center ${
                prerequisites.entitiesComplete
                  ? 'bg-green-100 text-green-600'
                  : 'bg-gray-100 text-gray-400'
              }`}>
                {prerequisites.entitiesComplete ? (
                  <CheckCircle className="h-5 w-5" />
                ) : (
                  <span className="text-lg font-semibold">1</span>
                )}
              </div>
              <div className="flex-1">
                <p className={`font-medium ${
                  prerequisites.entitiesComplete ? 'text-green-800' : 'text-text-primary'
                }`}>
                  {prerequisites.entitiesComplete
                    ? `Entities discovered (${prerequisites.entityCount} entities)`
                    : 'Discover entities'}
                </p>
                <p className="text-sm text-text-secondary">
                  {prerequisites.entitiesComplete
                    ? 'Domain entities have been identified in your schema'
                    : 'Run entity discovery to identify domain concepts like User, Order, Product'}
                </p>
              </div>
              {!prerequisites.entitiesComplete && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => navigate(`/projects/${pid}/entities`)}
                >
                  Go to Entities
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              )}
            </div>

            {/* Relationships prerequisite */}
            <div className={`flex items-center gap-4 p-4 rounded-lg border ${
              prerequisites.relationshipsComplete
                ? 'border-green-200 bg-green-50'
                : 'border-gray-200 bg-white'
            }`}>
              <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center ${
                prerequisites.relationshipsComplete
                  ? 'bg-green-100 text-green-600'
                  : 'bg-gray-100 text-gray-400'
              }`}>
                {prerequisites.relationshipsComplete ? (
                  <CheckCircle className="h-5 w-5" />
                ) : (
                  <span className="text-lg font-semibold">2</span>
                )}
              </div>
              <div className="flex-1">
                <p className={`font-medium ${
                  prerequisites.relationshipsComplete ? 'text-green-800' : 'text-text-primary'
                }`}>
                  {prerequisites.relationshipsComplete
                    ? `Relationships defined (${prerequisites.relationshipCount} relationships)`
                    : 'Define relationships'}
                </p>
                <p className="text-sm text-text-secondary">
                  {prerequisites.relationshipsComplete
                    ? 'Table relationships have been detected and confirmed'
                    : 'Run relationship detection to understand how tables connect'}
                </p>
              </div>
              {!prerequisites.relationshipsComplete && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => navigate(`/projects/${pid}/relationships`)}
                >
                  Go to Relationships
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              )}
            </div>
          </div>
        </div>
      ) : (
        /* Prerequisites met - Ready to extract ontology */
        <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm p-12">
          <Brain className="h-16 w-16 text-purple-300 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-text-primary mb-2 text-center">
            Ready to Build Ontology
          </h2>
          <p className="text-text-secondary max-w-3xl mx-auto mb-4 text-center">
            Found: {prerequisites.entityCount} entities, {prerequisites.relationshipCount} relationships
          </p>
          <p className="text-text-secondary max-w-3xl mx-auto mb-6 text-center">
            Before we analyze your schema, tell us about your application. Who uses it and what do
            they do with this data? This context helps us build a more accurate business ontology.
          </p>

          {/* Description textarea */}
          <div className="max-w-3xl mx-auto mb-6">
            <label className="block text-sm font-medium text-text-primary mb-2">
              Describe your application
            </label>
            <textarea
              value={projectDescription}
              onChange={(e) => setProjectDescription(e.target.value.slice(0, 500))}
              placeholder="Example: This is our e-commerce platform for B2B wholesale. Customers are businesses that purchase products in bulk, while Users are employee accounts that manage orders. We track inventory levels, pricing tiers, and order fulfillment status..."
              className="w-full h-32 p-3 border border-border-light rounded-lg bg-surface-secondary text-text-primary placeholder:text-text-tertiary resize-none focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent"
              maxLength={500}
            />
            <div className="flex justify-between mt-1 text-sm text-text-tertiary">
              <span>
                {projectDescription.length < 20
                  ? `${20 - projectDescription.length} more characters required`
                  : 'Ready to start'}
              </span>
              <span>{projectDescription.length}/500</span>
            </div>
          </div>

          <div className="text-center">
            <Button
              onClick={handleStart}
              size="lg"
              disabled={projectDescription.length < 20}
              className="bg-purple-600 hover:bg-purple-700 text-white disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Play className="mr-2 h-5 w-5" />
              Start Building Ontology
            </Button>
          </div>
        </div>
      )}

      {/* Info panel when ontology is ready - chat is available */}
      {isOntologyReady && (
        <div className="mt-6 rounded-lg border border-purple-200 bg-purple-50 p-4">
          <p className="text-purple-800 text-sm">
            <strong>Chat available:</strong> Use the chat panel to answer questions about your data.
            The AI assistant will help clarify business rules and update the ontology in real-time.
          </p>
        </div>
      )}

      {/* Cancel Extraction Confirmation Modal */}
      {showCancelConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-surface-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
            <h3 className="text-lg font-semibold text-text-primary">Cancel Extraction?</h3>
            <p className="text-text-secondary mt-2">
              The workflow will be stopped. Any entities already analyzed will be preserved.
            </p>
            <div className="flex justify-end gap-3 mt-6">
              <Button
                variant="outline"
                onClick={() => setShowCancelConfirm(false)}
              >
                Keep Running
              </Button>
              <Button
                onClick={() => void handleCancelConfirm()}
                className="bg-red-600 hover:bg-red-700 text-white"
              >
                Cancel Extraction
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Ontology Confirmation Modal */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-surface-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-text-primary">Delete Ontology?</h3>
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="text-text-tertiary hover:text-text-primary"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <p className="text-text-secondary mb-6">
              This will delete all ontology data, workflows, and questions for this project.
              You&apos;ll need to start extraction from scratch. This action cannot be undone.
            </p>
            <div className="flex justify-end gap-3">
              <Button
                variant="outline"
                onClick={() => setShowDeleteConfirm(false)}
              >
                Cancel
              </Button>
              <Button
                onClick={handleDeleteOntology}
                className="bg-red-600 hover:bg-red-700 text-white"
              >
                Delete Ontology
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default OntologyPage;

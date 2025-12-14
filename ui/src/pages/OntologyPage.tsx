/**
 * OntologyPage - Tiered Ontology Extraction UI
 * Living document with work queue model
 */

import { ArrowLeft, Brain, CheckCircle, Play } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import ChatPane from '../components/ontology/ChatPane';
import OntologyStatus from '../components/ontology/OntologyStatus';
import QuestionPanel from '../components/ontology/QuestionPanel';
import WorkQueue from '../components/ontology/WorkQueue';
import { Button } from '../components/ui/Button';
import ontologyApi from '../services/ontologyApi';
import { realOntologyService } from '../services/realOntologyService';
import type { OntologyWorkflowStatus } from '../types';

const OntologyPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  const [status, setStatus] = useState<OntologyWorkflowStatus | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hasPendingQuestions, setHasPendingQuestions] = useState(false);
  const [allQuestionsComplete, setAllQuestionsComplete] = useState(false);

  // Initialize and subscribe to status updates
  useEffect(() => {
    const initializeService = async () => {
      setIsLoading(true);
      setError(null);

      try {
        if (pid) {
          await realOntologyService.initialize(pid);

          // Check for pending questions
          const nextQuestionResponse = await ontologyApi.getNextQuestion(pid);
          if (nextQuestionResponse.all_complete) {
            setHasPendingQuestions(false);
            setAllQuestionsComplete(true);
          } else if (nextQuestionResponse.question) {
            setHasPendingQuestions(true);
            setAllQuestionsComplete(false);
          }
        }
      } catch (e) {
        console.error('Failed to initialize ontology service:', e);
        // Don't show error - service will show idle state
      } finally {
        setIsLoading(false);
      }
    };

    void initializeService();

    // Subscribe to status updates
    const unsubscribe = realOntologyService.subscribe((newStatus) => {
      setStatus(newStatus);
    });

    return () => {
      unsubscribe();
      realOntologyService.stop();
    };
  }, [pid]);

  const handleStart = useCallback(async () => {
    setError(null);
    try {
      await realOntologyService.startExtraction();
    } catch (e) {
      console.error('Failed to start extraction:', e);
      setError('Failed to start extraction. Please try again.');
    }
  }, []);

  const handlePause = useCallback(async () => {
    try {
      await realOntologyService.cancel();
    } catch (e) {
      console.error('Failed to pause:', e);
    }
  }, []);

  const handleResume = useCallback(async () => {
    try {
      await realOntologyService.startExtraction();
    } catch (e) {
      console.error('Failed to resume:', e);
    }
  }, []);

  const handleRestart = useCallback(async () => {
    setError(null);
    try {
      await realOntologyService.restart();
    } catch (e) {
      console.error('Failed to restart:', e);
      setError('Failed to restart extraction. Please try again.');
    }
  }, []);

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

  const isIdle = status?.progress.state === 'idle';
  const isRunning =
    status?.progress.state === 'building' || status?.progress.state === 'initializing';

  if (isLoading) {
    return (
      <div className="mx-auto max-w-7xl">
        <div className="flex items-center justify-center h-64">
          <div className="text-text-secondary">Loading...</div>
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

          {/* Start button when idle */}
          {isIdle && (
            <Button
              onClick={handleStart}
              className="bg-purple-600 hover:bg-purple-700 text-white"
            >
              <Play className="mr-2 h-4 w-4" />
              Start Extraction
            </Button>
          )}
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
            onPause={handlePause}
            onResume={handleResume}
            onRestart={handleRestart}
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
            {/* Show QuestionPanel when there are pending questions */}
            {hasPendingQuestions && (
              <QuestionPanel
                projectId={pid || ''}
                onAllComplete={handleAllQuestionsComplete}
                onQuestionAnswered={handleQuestionAnswered}
              />
            )}

            {/* Show completion banner + Chat when all questions are done */}
            {allQuestionsComplete && (
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
                  projectId={pid || ''}
                  onOntologyUpdate={handleOntologyUpdate}
                  onKnowledgeStored={handleKnowledgeStored}
                />
              </>
            )}

            {/* Show Chat only when no questions exist at all (legacy behavior) */}
            {!hasPendingQuestions && !allQuestionsComplete && (
              <ChatPane
                projectId={pid || ''}
                onOntologyUpdate={handleOntologyUpdate}
                onKnowledgeStored={handleKnowledgeStored}
              />
            )}
          </div>
        </div>
      ) : (
        /* Empty state when idle */
        <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm p-12 text-center">
          <Brain className="h-16 w-16 text-purple-300 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-text-primary mb-2">
            Ready to Extract Ontology
          </h2>
          <p className="text-text-secondary max-w-md mx-auto mb-6">
            The extraction process will analyze your database schema and build a comprehensive
            business ontology. You may be asked questions to clarify business rules along the way.
          </p>
          <Button
            onClick={handleStart}
            size="lg"
            className="bg-purple-600 hover:bg-purple-700 text-white"
          >
            <Play className="mr-2 h-5 w-5" />
            Start Extraction
          </Button>
        </div>
      )}

      {/* Info panel when running - chat is available */}
      {isRunning && (
        <div className="mt-6 rounded-lg border border-purple-200 bg-purple-50 p-4">
          <p className="text-purple-800 text-sm">
            <strong>Chat available:</strong> Use the chat panel to answer questions about your data.
            The AI assistant will help clarify business rules and update the ontology in real-time.
          </p>
        </div>
      )}
    </div>
  );
};

export default OntologyPage;
